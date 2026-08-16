package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// 部署状态常量
const (
	DeployStatusNotDeployed = "not_deployed"
	DeployStatusDeploying   = "deploying"
	DeployStatusRunning     = "running"
	DeployStatusStopped     = "stopped"
	DeployStatusFailed      = "failed"
	DeployStatusUndeployed  = "undeployed"
)

// DeployService 部署编排服务。
//
// 通过 SSH 连接到远程部署服务器，编排完整部署流程：
// 分配端口 -> 供给数据库 -> 构建前端 -> 写入 Nginx 配置 -> 重载 Nginx ->
// 启动后端 -> 更新部署记录 -> 生成访问 URL。
type DeployService struct {
	db            *gorm.DB
	cfg           *config.Config
	portAllocator *PortAllocator
	dbProvisioner *DatabaseProvisioner
	nginxManager  *NginxManager
	sshClient     *SSHClient

	mu       sync.Mutex
	backends map[int64]int // projectID -> 远程后端进程 PID
}

// NewDeployService 构造部署编排服务，内部组装各子服务并建立 SSH 连接。
func NewDeployService(db *gorm.DB, cfg *config.Config) *DeployService {
	sshClient, err := NewSSHClient(&cfg.Infra)
	if err != nil {
		if sshClient == nil {
			fmt.Printf("⚠ SSH 客户端创建失败，部署功能将不可用: %v\n", err)
		} else {
			fmt.Printf("⚠ SSH 初次连接失败（将在部署时自动重试）: %v\n", err)
		}
	}

	return &DeployService{
		db:            db,
		cfg:           cfg,
		portAllocator: NewPortAllocator(db, &cfg.Infra),
		dbProvisioner: NewDatabaseProvisioner(db, &cfg.Database),
		nginxManager:  NewNginxManager(&cfg.Infra, sshClient),
		sshClient:     sshClient,
		backends:      make(map[int64]int),
	}
}

// Deploy 执行完整部署流程，onProgress 用于回调每一步进度。
//
// force 为 true 时即使项目已运行也强制重新部署（先停旧后端进程，复用既有端口，访问地址不变）。
//
// 流程：
//  1. 检查是否已部署（已运行且非 force 则直接返回）
//  2. 分配前端 + 后端端口（force 时复用既有端口）
//  3. 供给项目数据库
//  4. 在远程服务器创建部署目录
//  5. 本地构建前端（npm run build）并上传产物到远程部署目录
//  6. 本地构建后端（go build）并上传二进制到远程部署目录
//  7. 生成并写入 Nginx 配置到远程服务器
//  8. 通过 SSH 重载 Nginx
//  9. 通过 SSH 启动后端二进制（nohup 后台运行）
// 10. 更新 ProjectDeployment 记录并生成访问 URL
//
// 任意步骤失败将回滚已分配的端口与数据库，并将记录置为 failed。
func (s *DeployService) Deploy(projectID int64, force bool, onProgress func(string)) (*model.ProjectDeployment, error) {
	if s.sshClient == nil {
		return nil, fmt.Errorf("SSH 客户端未初始化：请配置 INFRA_SSH_PEM（私钥内容）或 INFRA_SSH_KEY_PATH（私钥文件路径）环境变量，并确保 INFRA_SERVER_HOST 和 INFRA_SSH_USER 正确")
	}

	var logBuf strings.Builder
	progress := func(msg string) {
		logBuf.WriteString(msg)
		logBuf.WriteByte('\n')
		if onProgress != nil {
			onProgress(msg)
		}
	}

	// 1. 检查是否已部署
	existing, err := s.GetDeployment(projectID)
	if err != nil {
		return nil, fmt.Errorf("查询现有部署记录失败: %w", err)
	}
	if existing != nil && existing.Status == DeployStatusRunning && !force {
		// 验证远程进程是否真的存活
		if existing.BackendPID > 0 {
			alive, _ := s.sshClient.IsProcessAlive(existing.BackendPID)
			if alive {
				progress("项目已处于运行状态，跳过部署")
				// 保证测试脚本自解析所需的固定存储始终存在（文件丢失时可自愈）
				s.persistAccessURL(projectID, existing.AccessURL)
				return existing, nil
			}
			progress("检测到后端进程已退出，将重新部署")
		} else {
			progress("项目已处于运行状态，跳过部署")
			s.persistAccessURL(projectID, existing.AccessURL)
			return existing, nil
		}
	}
	// 强制重新部署：先停掉旧后端进程，避免复用端口时被占用
	if force && existing != nil && existing.Status == DeployStatusRunning {
		progress("强制重新部署：停止旧后端进程...")
		if existing.BackendPID > 0 {
			if err := s.stopBackendRemote(projectID, existing.BackendPID); err != nil {
				progress("停止旧后端进程告警: " + err.Error())
			}
		}
	}

	// 查询项目信息以获取工作目录与名称
	var project model.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		return nil, fmt.Errorf("查询项目 %d 失败: %w", projectID, err)
	}

	deployDir := filepath.Join(s.cfg.Infra.DeployBaseDir, strconv.FormatInt(projectID, 10))
	buildDir := filepath.Join(deployDir, "dist")

	deployment := &model.ProjectDeployment{
		ProjectID: projectID,
		BuildDir:  buildDir,
		Status:    DeployStatusDeploying,
	}
	if existing != nil {
		deployment.ID = existing.ID
	}

	// 先落库一条 deploying 记录
	deployment.DeployLog = logBuf.String()
	if err := s.saveDeployment(deployment); err != nil {
		return nil, fmt.Errorf("写入部署记录失败: %w", err)
	}

	// 回滚资源追踪
	var allocatedPorts []int
	var dbProvisioned bool
	rollback := func(reason string) {
		progress("部署失败，开始回滚: " + reason)
		for _, p := range allocatedPorts {
			_ = s.portAllocator.ReleasePort(p)
		}
		if dbProvisioned {
			_ = s.dbProvisioner.DropDatabase(projectID)
		}
		deployment.Status = DeployStatusFailed
		deployment.DeployLog = logBuf.String()
		_ = s.saveDeployment(deployment)
	}

	// 2. 分配端口（force 且已有端口时复用，访问地址保持不变）
	var frontendPort, backendPort int
	if force && existing != nil && existing.FrontendPort > 0 && existing.BackendPort > 0 {
		frontendPort = existing.FrontendPort
		backendPort = existing.BackendPort
		// 同步到 deployment 结构体，避免 saveDeployment 用零值覆盖已有端口记录
		deployment.FrontendPort = frontendPort
		deployment.BackendPort = backendPort
		progress(fmt.Sprintf("复用已有端口: 前端 %d / 后端 %d", frontendPort, backendPort))
	} else {
		progress("正在分配前端端口...")
		frontendPort, err = s.portAllocator.AllocatePort(projectID, "frontend", "项目前端端口")
		if err != nil {
			rollback("分配前端端口失败: " + err.Error())
			return nil, fmt.Errorf("分配前端端口失败: %w", err)
		}
		allocatedPorts = append(allocatedPorts, frontendPort)
		deployment.FrontendPort = frontendPort

		progress("正在分配后端端口...")
		backendPort, err = s.portAllocator.AllocatePort(projectID, "backend", "项目后端端口")
		if err != nil {
			rollback("分配后端端口失败: " + err.Error())
			return nil, fmt.Errorf("分配后端端口失败: %w", err)
		}
		allocatedPorts = append(allocatedPorts, backendPort)
		deployment.BackendPort = backendPort
	}

	// 3. 供给数据库
	progress("正在创建项目数据库...")
	pdb, err := s.dbProvisioner.ProvisionDatabase(projectID)
	if err != nil {
		rollback("创建数据库失败: " + err.Error())
		return nil, fmt.Errorf("创建数据库失败: %w", err)
	}
	dbProvisioned = true
	progress(fmt.Sprintf("数据库 %s 已就绪", pdb.DBName))

	// 4. 在远程服务器创建部署目录
	progress("正在创建远程部署目录...")
	if err := s.sshClient.MkdirRemote(deployDir); err != nil {
		rollback("创建部署目录失败: " + err.Error())
		return nil, fmt.Errorf("创建部署目录失败: %w", err)
	}

	// 5. 本地构建前端并上传产物到远程部署目录
	// （远程服务器无 node/go 工具链，必须本地构建后上传）
	progress("正在本地构建前端 (npm run build)...")
	distDir, err := s.buildFrontendLocal(project.WorkDir)
	if err != nil {
		rollback("构建前端失败: " + err.Error())
		return nil, fmt.Errorf("构建前端失败: %w", err)
	}
	progress("正在上传前端产物到远程服务器...")
	if err := s.uploadTree(distDir, filepath.Join(deployDir, "dist"), true); err != nil {
		rollback("上传前端产物失败: " + err.Error())
		return nil, fmt.Errorf("上传前端产物失败: %w", err)
	}

	// 6. 本地构建后端并上传二进制到远程部署目录
	progress("正在本地构建后端 (go build)...")
	_, tmpDir, err := s.buildBackendLocal(project.WorkDir)
	if err != nil {
		rollback("构建后端失败: " + err.Error())
		return nil, fmt.Errorf("构建后端失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	progress("正在上传后端二进制到远程服务器...")
	if err := s.uploadTree(tmpDir, deployDir, false); err != nil {
		rollback("上传后端二进制失败: " + err.Error())
		return nil, fmt.Errorf("上传后端二进制失败: %w", err)
	}

	// 放开部署目录权限：tar 解包会保留本地 MkdirTemp 的 0700 权限，
	// 而 nginx worker 以非属主用户运行，无法遍历 0700 目录（前端 500）。
	if _, err := s.sshClient.RunCommand(fmt.Sprintf(
		"chmod 755 %s && chmod -R a+rX %s", deployDir, filepath.Join(deployDir, "dist"))); err != nil {
		rollback("设置部署目录权限失败: " + err.Error())
		return nil, fmt.Errorf("设置部署目录权限失败: %w", err)
	}

	// 7. 生成并写入 Nginx 配置到远程服务器
	progress("正在生成并写入 Nginx 配置...")
	configContent, err := s.nginxManager.GenerateConfig(projectID, project.Name, frontendPort, backendPort, deployDir)
	if err != nil {
		rollback("生成 Nginx 配置失败: " + err.Error())
		return nil, fmt.Errorf("生成 Nginx 配置失败: %w", err)
	}
	configPath := s.nginxManager.ConfigPath(projectID)
	if err := s.nginxManager.WriteConfig(configPath, configContent); err != nil {
		rollback("写入 Nginx 配置失败: " + err.Error())
		return nil, fmt.Errorf("写入 Nginx 配置失败: %w", err)
	}
	deployment.NginxConfigPath = configPath

	// 8. 通过 SSH 重载 Nginx
	progress("正在重载 Nginx...")
	if err := s.nginxManager.ReloadNginx(); err != nil {
		rollback("重载 Nginx 失败: " + err.Error())
		return nil, fmt.Errorf("重载 Nginx 失败: %w", err)
	}

	// 9. 通过 SSH 启动后端
	progress("正在远程启动后端服务...")
	backendBinary := filepath.Join(deployDir, "backend")
	pid, err := s.startBackendRemote(projectID, backendBinary, backendPort, deployDir, pdb)
	if err != nil {
		rollback("启动后端失败: " + err.Error())
		return nil, fmt.Errorf("启动后端失败: %w", err)
	}
	deployment.BackendBinary = backendBinary
	deployment.BackendPID = pid
	progress(fmt.Sprintf("后端进程已启动 (PID=%d)", pid))

	// 10. 更新部署记录并生成访问 URL
	now := time.Now()
	deployment.Status = DeployStatusRunning
	deployment.LastDeployedAt = &now
	deployment.AccessURL = fmt.Sprintf("http://%s:%d", s.cfg.Infra.ServerHost, frontendPort)
	deployment.DeployLog = logBuf.String()
	if err := s.saveDeployment(deployment); err != nil {
		return nil, fmt.Errorf("更新部署记录失败: %w", err)
	}

	// 把访问地址持久化到项目 workdir 的固定存储（tests/.base_url），
	// 供生成的测试脚本在未注入 BASE_URL 时自解析——全自动测试不依赖调用方传 URL。
	s.persistAccessURL(projectID, deployment.AccessURL)

	progress("部署完成: " + deployment.AccessURL)
	return deployment, nil
}

// persistAccessURL 把部署访问地址写入项目工作目录固定存储 tests/.base_url。
// 生成的 API/Playwright 测试脚本在未注入 BASE_URL 时会回退读取该文件，实现全自动测试自解析。
// 失败仅告警不阻断部署。
func (s *DeployService) persistAccessURL(projectID int64, accessURL string) {
	if strings.TrimSpace(accessURL) == "" {
		return
	}
	var project model.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		fmt.Printf("⚠ 持久化访问地址失败（项目 %d 查询失败）: %v\n", projectID, err)
		return
	}
	if strings.TrimSpace(project.WorkDir) == "" {
		return
	}
	path := filepath.Join(project.WorkDir, "tests", ".base_url")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Printf("⚠ 持久化访问地址失败（创建目录）: %v\n", err)
		return
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(accessURL)+"\n"), 0o644); err != nil {
		fmt.Printf("⚠ 持久化访问地址失败: %v\n", err)
	}
}

// removeAccessURLFile 清理部署时写入的访问地址固定存储（Undeploy 时调用）。
func (s *DeployService) removeAccessURLFile(projectID int64) {
	var project model.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		return
	}
	if strings.TrimSpace(project.WorkDir) == "" {
		return
	}
	_ = os.Remove(filepath.Join(project.WorkDir, "tests", ".base_url"))
}

// Undeploy 卸载项目部署：停止后端、移除 Nginx 配置、释放端口、删除数据库，
// 并将部署记录状态置为 undeployed。无部署记录时幂等返回。
func (s *DeployService) Undeploy(projectID int64) error {
	deployment, err := s.GetDeployment(projectID)
	if err != nil {
		return fmt.Errorf("查询部署记录失败: %w", err)
	}
	if deployment == nil {
		return nil
	}

	// 1. 停止远程后端进程
	if err := s.stopBackendRemote(projectID, deployment.BackendPID); err != nil {
		fmt.Printf("停止后端进程告警: %v\n", err)
	}

	// 2. 移除 Nginx 配置并重载
	if deployment.NginxConfigPath != "" {
		if err := s.nginxManager.RemoveConfig(deployment.NginxConfigPath); err != nil {
			fmt.Printf("移除 Nginx 配置告警: %v\n", err)
		}
		if err := s.nginxManager.ReloadNginx(); err != nil {
			fmt.Printf("重载 Nginx 告警: %v\n", err)
		}
	}

	// 3. 释放端口
	ports, err := s.portAllocator.GetProjectPorts(projectID)
	if err != nil {
		fmt.Printf("查询项目端口告警: %v\n", err)
	}
	for _, p := range ports {
		if err := s.portAllocator.ReleasePort(p.Port); err != nil {
			fmt.Printf("释放端口 %d 告警: %v\n", p.Port, err)
		}
	}

	// 4. 删除数据库
	if err := s.dbProvisioner.DropDatabase(projectID); err != nil {
		fmt.Printf("删除项目数据库告警: %v\n", err)
	}

	// 5. 更新部署记录状态
	deployment.Status = DeployStatusUndeployed
	deployment.BackendPID = 0
	if err := s.saveDeployment(deployment); err != nil {
		return fmt.Errorf("更新部署记录状态失败: %w", err)
	}

	// 清理访问地址固定存储，避免卸载后测试脚本仍误连旧地址
	s.removeAccessURLFile(projectID)
	return nil
}

// GetDeployment 查询指定项目的部署记录。
// 不存在时返回 (nil, nil)。
func (s *DeployService) GetDeployment(projectID int64) (*model.ProjectDeployment, error) {
	var deployment model.ProjectDeployment
	err := s.db.Where("project_id = ?", projectID).First(&deployment).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &deployment, nil
}

// GetDeployStatus 返回指定项目的部署状态。
// 无记录返回 not_deployed；若记录为 running 但远程进程已退出，则返回 stopped。
func (s *DeployService) GetDeployStatus(projectID int64) (string, error) {
	deployment, err := s.GetDeployment(projectID)
	if err != nil {
		return "", err
	}
	if deployment == nil {
		return DeployStatusNotDeployed, nil
	}

	if deployment.Status == DeployStatusRunning && s.sshClient != nil {
		// 检查本进程缓存中的 PID
		s.mu.Lock()
		pid, hasLocal := s.backends[projectID]
		s.mu.Unlock()

		checkPID := pid
		if !hasLocal {
			checkPID = deployment.BackendPID
		}

		if checkPID > 0 {
			alive, _ := s.sshClient.IsProcessAlive(checkPID)
			if !alive {
				return DeployStatusStopped, nil
			}
		}
	}

	return deployment.Status, nil
}

// startBackendRemote 通过 SSH 在远程服务器上以后台方式启动后端二进制。
// 返回远程进程的 PID。
//
// 生成的后端遵循环境变量配置约定（见项目 CLAUDE.md），启动时注入：
//   - PORT: 分配的后端端口（--port 参数同时传递，兼容解析命令行标志的实现）
//   - JWT_SECRET: 由 loafer 主密钥 + 项目 ID 派生，重部署保持稳定
//   - APP_ENV_VARS: 项目独立数据库的 MySQL DSN
//   - DB_PATH: SQLite 类项目的库文件路径（置于部署目录内）
//
// 进程输出写入部署目录 backend.log，启动即退出时回传日志尾部便于排查。
func (s *DeployService) startBackendRemote(projectID int64, binary string, port int, deployDir string, pdb *model.ProjectDatabase) (int, error) {
	// 检查后端二进制是否存在
	exists, err := s.sshClient.FileExists(binary)
	if err != nil {
		return 0, fmt.Errorf("检查后端二进制失败: %w", err)
	}
	if !exists {
		return 0, fmt.Errorf("后端二进制不存在: %s（请确保后端已编译）", binary)
	}

	// 赋予执行权限
	_, _ = s.sshClient.RunCommand(fmt.Sprintf("chmod +x %s", binary))

	envPairs := []string{
		"PORT=" + shellQuote(strconv.Itoa(port)),
		"JWT_SECRET=" + shellQuote(s.derivedJWTSecret(projectID)),
		"DB_PATH=" + shellQuote(filepath.Join(deployDir, "data", "app.db")),
	}
	if pdb != nil {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			pdb.DBUsername, pdb.DBPassword, pdb.DBHost, pdb.DBPort, pdb.DBName)
		envPairs = append(envPairs, "APP_ENV_VARS="+shellQuote(dsn))
	}

	// env 执行后会 exec 目标二进制，$! 即为后端进程 PID
	cmd := fmt.Sprintf("env %s %s --port %d", strings.Join(envPairs, " "), binary, port)
	logPath := filepath.Join(deployDir, "backend.log")
	pid, err := s.sshClient.RunCommandBackgroundLogged(cmd, logPath)
	if err != nil {
		return 0, fmt.Errorf("启动后端进程失败: %w", err)
	}

	// 记录 PID
	s.mu.Lock()
	s.backends[projectID] = pid
	s.mu.Unlock()

	// 等待片刻验证进程是否启动成功
	time.Sleep(2 * time.Second)
	alive, _ := s.sshClient.IsProcessAlive(pid)
	if !alive {
		tail, _ := s.sshClient.RunCommand(fmt.Sprintf("tail -n 20 %s 2>/dev/null", logPath))
		return 0, fmt.Errorf("后端进程启动后立即退出（PID=%d），启动日志: %s", pid, strings.TrimSpace(tail))
	}

	return pid, nil
}

// derivedJWTSecret 由 loafer 主 JWT 密钥与项目 ID 派生项目级 JWT 密钥，
// 同一项目多次重部署保持一致，避免已有令牌意外失效。
func (s *DeployService) derivedJWTSecret(projectID int64) string {
	sum := sha256.Sum256([]byte(s.cfg.JWT.Secret + ":proj:" + strconv.FormatInt(projectID, 10)))
	return hex.EncodeToString(sum[:])
}

// shellQuote 将字符串包裹为 shell 单引号字面量，内部单引号转义为 '\''。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// stopBackendRemote 通过 SSH 停止远程服务器上的后端进程。
func (s *DeployService) stopBackendRemote(projectID int64, pid int) error {
	s.mu.Lock()
	cachedPID, hasLocal := s.backends[projectID]
	delete(s.backends, projectID)
	s.mu.Unlock()

	targetPID := pid
	if hasLocal && cachedPID > 0 {
		targetPID = cachedPID
	}

	if targetPID <= 0 {
		return nil
	}

	if err := s.sshClient.KillProcess(targetPID); err != nil {
		return fmt.Errorf("停止远程后端进程 %d 失败: %w", targetPID, err)
	}
	return nil
}

// saveDeployment 保存部署记录：存在 ID 则全量更新，否则新增。
// 更新路径不覆盖 created_at：调用方可能只回填了 ID（如重新部署已有失败记录），
// 此时结构体 CreatedAt 为零值，全量 Save 会写入 '0000-00-00' 被 MySQL 严格模式拒绝（Error 1292）。
func (s *DeployService) saveDeployment(d *model.ProjectDeployment) error {
	if d.ID > 0 {
		return s.db.Model(d).Select("*").Omit("created_at").Updates(d).Error
	}
	return s.db.Create(d).Error
}

// GetSSHClient 返回内部 SSH 客户端实例（供其他服务复用）。
func (s *DeployService) GetSSHClient() *SSHClient {
	return s.sshClient
}
