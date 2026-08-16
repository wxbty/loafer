package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/creack/pty"
)

// ProcessManager 对应 Java CliProcessManager，管理 Claude Code CLI 子进程的启动、停止与状态监控。
type ProcessManager struct {
	mu sync.RWMutex

	// 会话ID与PTY进程的映射
	ptyMap map[string]*os.File
	// 会话ID与命令的映射
	cmdMap map[string]*exec.Cmd
	// 会话ID与项目ID的映射
	projectMap map[string]string
	// 会话ID与任务ID的映射
	taskMap map[string]*int64
	// 会话ID与工作目录的映射
	workDirMap map[string]string
	// 会话ID与退出码的映射
	exitCodeMap map[string]int
	// 会话ID与错误信息的映射
	errorMap map[string]string
	// 会话ID与输出缓冲的映射
	outputMap map[string][]byte
	// 会话ID与输出回调的映射
	outputCallbacks map[string]func(data []byte)
}

// NewProcessManager 构造进程管理器。
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		ptyMap:          make(map[string]*os.File),
		cmdMap:          make(map[string]*exec.Cmd),
		projectMap:      make(map[string]string),
		taskMap:         make(map[string]*int64),
		workDirMap:      make(map[string]string),
		exitCodeMap:     make(map[string]int),
		errorMap:        make(map[string]string),
		outputMap:       make(map[string][]byte),
		outputCallbacks: make(map[string]func(data []byte)),
	}
}

// StartSession 启动一个 Claude Code CLI 会话。
// workDir: 工作目录
// claudeSessionUUID: 若不为空，使用 --resume 恢复已有会话
// profileID: 模型配置方案ID（可选）
// onOutput: 输出回调函数（可为 nil）
func (pm *ProcessManager) StartSession(sessionID, workDir, claudeSessionUUID, profileID string, onOutput func(data []byte)) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 解析 claude 可执行文件路径（5级查找链，与 claude_sprint 对齐）
	claudePath := resolveClaudePath()
	if claudePath == "" {
		return fmt.Errorf("claude 命令未找到，请确保已安装 Claude Code CLI（可通过 CLAUDE_CODE_PATH 环境变量指定路径）")
	}

	// 确保工作目录存在，若不存在则创建
	if workDir != "" {
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			if mkErr := os.MkdirAll(workDir, 0755); mkErr != nil {
				return fmt.Errorf("创建工作目录失败 %s: %w", workDir, mkErr)
			}
		}
	}

	// 构建命令参数：claude code --dangerously-skip-permissions [--resume <uuid>] [--model <profileID>]
	args := []string{"code", "--dangerously-skip-permissions"}
	if claudeSessionUUID != "" {
		args = append(args, "--resume", claudeSessionUUID)
	}
	if profileID != "" {
		args = append(args, "--model", profileID)
	}

	cmd := exec.Command(claudePath, args...)
	cmd.Dir = workDir

	// 构建环境变量（与 claude_sprint 对齐）
	env := buildClaudeEnv()
	cmd.Env = env

	// 启动 PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		// fork/exec 返回 "no such file or directory" 时，实际原因通常是工作目录不存在
		// 而非可执行文件不存在（后者已在上面通过 resolveClaudePath 验证）
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such file or directory") {
			return fmt.Errorf("启动 PTY 失败（工作目录 %s 不存在或可执行文件路径无效）: %w", workDir, err)
		}
		return fmt.Errorf("启动 PTY 失败: %w", err)
	}

	pm.ptyMap[sessionID] = ptmx
	pm.cmdMap[sessionID] = cmd
	pm.workDirMap[sessionID] = workDir
	pm.outputMap[sessionID] = []byte{}
	if onOutput != nil {
		pm.outputCallbacks[sessionID] = onOutput
	}

	// 启动 goroutine 读取输出
	go pm.readOutput(sessionID, ptmx)

	// 启动 goroutine 等待进程退出
	go pm.waitProcess(sessionID, cmd)

	return nil
}

// readOutput 持续读取 PTY 输出。
func (pm *ProcessManager) readOutput(sessionID string, ptmx *os.File) {
	buf := make([]byte, 4096)
	reader := bufio.NewReader(ptmx)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			pm.mu.Lock()
			pm.outputMap[sessionID] = append(pm.outputMap[sessionID], data...)
			callback := pm.outputCallbacks[sessionID]
			pm.mu.Unlock()

			if callback != nil {
				callback(data)
			}
		}
		if err != nil {
			break
		}
	}
}

// waitProcess 等待进程退出并记录退出码。
func (pm *ProcessManager) waitProcess(sessionID string, cmd *exec.Cmd) {
	err := cmd.Wait()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if exitErr, ok := err.(*exec.ExitError); ok {
		pm.exitCodeMap[sessionID] = exitErr.ExitCode()
	} else if err != nil {
		pm.errorMap[sessionID] = err.Error()
		pm.exitCodeMap[sessionID] = -1
	} else {
		pm.exitCodeMap[sessionID] = 0
	}
	// 关闭 PTY
	if ptmx, ok := pm.ptyMap[sessionID]; ok {
		ptmx.Close()
	}
}

// WriteToStdin 向会话的 stdin 写入数据。
func (pm *ProcessManager) WriteToStdin(sessionID string, data []byte) error {
	pm.mu.RLock()
	ptmx, ok := pm.ptyMap[sessionID]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("会话 %s 不存在", sessionID)
	}
	_, err := ptmx.Write(data)
	return err
}

// WriteToStdinRaw 向会话的 stdin 写入原始数据，返回是否成功（对应 Java writeToStdinRaw）。
func (pm *ProcessManager) WriteToStdinRaw(sessionID string, data string) bool {
	pm.mu.RLock()
	ptmx, ok := pm.ptyMap[sessionID]
	pm.mu.RUnlock()
	if !ok {
		return false
	}
	_, err := ptmx.Write([]byte(data))
	return err == nil
}

// ResizeTerminal 调整终端窗口大小（对应 Java resizeTerminal）。
func (pm *ProcessManager) ResizeTerminal(sessionID string, cols, rows int) bool {
	pm.mu.RLock()
	ptmx, ok := pm.ptyMap[sessionID]
	pm.mu.RUnlock()
	if !ok {
		return false
	}
	return pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}) == nil
}

// StopSession 停止会话进程。
func (pm *ProcessManager) StopSession(sessionID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	cmd, ok := pm.cmdMap[sessionID]
	if !ok {
		return fmt.Errorf("会话 %s 不存在", sessionID)
	}
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
	return nil
}

// GetStatus 获取会话状态。
func (pm *ProcessManager) GetStatus(sessionID string) SessionStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	cmd, ok := pm.cmdMap[sessionID]
	if !ok {
		return StatusNotStarted
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		if pm.exitCodeMap[sessionID] == 0 {
			return StatusStopped
		}
		return StatusError
	}
	return StatusRunning
}

// IsRunning 检查会话是否正在运行。
func (pm *ProcessManager) IsRunning(sessionID string) bool {
	return pm.GetStatus(sessionID) == StatusRunning
}

// GetExitCode 获取进程退出码。
func (pm *ProcessManager) GetExitCode(sessionID string) *int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	code, ok := pm.exitCodeMap[sessionID]
	if !ok {
		return nil
	}
	return &code
}

// GetErrorInfo 获取错误信息。
func (pm *ProcessManager) GetErrorInfo(sessionID string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.errorMap[sessionID]
}

// GetOutput 获取会话输出缓冲。
func (pm *ProcessManager) GetOutput(sessionID string) []byte {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.outputMap[sessionID]
}

// SetOutputCallback 设置输出回调。
func (pm *ProcessManager) SetOutputCallback(sessionID string, callback func(data []byte)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.outputCallbacks[sessionID] = callback
}

// Close 清理所有会话。
func (pm *ProcessManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for sessionID, ptmx := range pm.ptyMap {
		if cmd, ok := pm.cmdMap[sessionID]; ok && cmd.Process != nil {
			cmd.Process.Kill()
		}
		ptmx.Close()
	}
}

// resolveClaudePath 解析 Claude Code CLI 可执行文件的绝对路径。
// 采用与 claude_sprint 项目 ClaudePathResolver 一致的5级查找链：
//  1. CLAUDE_CODE_PATH 环境变量覆盖
//  2. which claude（当前用户 PATH 中查找）
//  3. ~/.local/bin/claude（官方安装脚本默认位置）
//  4. nvm 安装路径：~/.nvm/versions/node/*/bin/claude
//  5. 系统常见路径：/usr/local/bin/claude, /usr/bin/claude, /opt/claude/bin/claude
//  6. Windows 路径：%USERPROFILE%\.claude\bin\claude.exe, %APPDATA%\npm\claude.cmd
//
// 找不到返回空字符串。
func resolveClaudePath() string {
	// 1. 环境变量覆盖
	if override := os.Getenv("CLAUDE_CODE_PATH"); override != "" {
		if isExecutable(override) {
			return override
		}
	}

	candidates := []string{}

	// 2. which claude
	if whichPath := whichClaude(); whichPath != "" {
		candidates = append(candidates, whichPath)
	}

	// 3 & 4. 用户家目录下的安装位置
	home := getUserHome()
	if home != "" {
		// ~/.local/bin/claude（官方安装脚本默认位置）
		candidates = append(candidates, filepath.Join(home, ".local", "bin", "claude"))
		// nvm 安装路径
		candidates = append(candidates, findNvmClaudePaths(home)...)
	}

	// 5. 系统级常见路径
	candidates = append(candidates,
		"/usr/local/bin/claude",
		"/usr/bin/claude",
		"/opt/claude/bin/claude",
	)

	// 6. Windows 路径
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(os.Getenv("USERPROFILE"), ".claude", "bin", "claude.exe"),
			filepath.Join(os.Getenv("APPDATA"), "npm", "claude.cmd"),
		)
	}

	for _, candidate := range candidates {
		if isExecutable(candidate) {
			return candidate
		}
	}

	return ""
}

// whichClaude 通过执行 which claude 查找可执行文件路径。
func whichClaude() string {
	cmd := exec.Command("which", "claude")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(output))
	// which 找不到时可能输出 "which: no claude in (...)" 之类的信息
	if path == "" || strings.Contains(path, "no ") || strings.HasPrefix(path, "which:") {
		return ""
	}
	if isExecutable(path) {
		return path
	}
	return ""
}

// findNvmClaudePaths 在指定用户家目录下查找 nvm 安装的 claude。
// 查找 ~/.nvm/versions/node/*/bin/claude
func findNvmClaudePaths(home string) []string {
	nvmDir := filepath.Join(home, ".nvm", "versions", "node")
	entries, err := os.ReadDir(nvmDir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(nvmDir, entry.Name(), "bin", "claude")
		paths = append(paths, candidate)
	}
	// 按目录名降序排列（优先使用最新 Node 版本）
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	return paths
}

// isExecutable 检查路径是否存在且可执行。
func isExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Windows 上只要有文件即可
	if runtime.GOOS == "windows" {
		return !info.IsDir()
	}
	// Unix: 检查可执行权限
	return !info.IsDir() && info.Mode().Perm()&0111 != 0
}

// getUserHome 获取当前用户的家目录。
func getUserHome() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if runtime.GOOS == "windows" {
		if home := os.Getenv("USERPROFILE"); home != "" {
			return home
		}
	}
	return ""
}

// buildClaudeEnv 构建 Claude CLI 子进程的环境变量。
// 与 claude_sprint 的 CliProcessManager 对齐：
//   - 继承当前环境变量
//   - 设置 TERM=xterm-256color
//   - 设置 LANG/LC_ALL=C.UTF-8（非 Windows）
//   - 移除 CLAUDECODE 环境变量（防止嵌套会话）
//   - 确保 ~/.local/bin 在 PATH 中（官方安装位置）
func buildClaudeEnv() []string {
	env := os.Environ()

	// 过滤掉 CLAUDECODE 环境变量
	filtered := make([]string, 0, len(env)+4)
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}

	// 添加终端和语言设置
	filtered = append(filtered, "TERM=xterm-256color")
	if runtime.GOOS != "windows" {
		filtered = append(filtered, "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	}

	// 确保 ~/.local/bin 在 PATH 中
	home := getUserHome()
	if home != "" {
		localBin := filepath.Join(home, ".local", "bin")
		pathValue := os.Getenv("PATH")
		if !strings.Contains(pathValue, localBin) {
			filtered = append(filtered, "PATH="+localBin+":"+pathValue)
		}
	}

	return filtered
}

// IsCLIAvailable 检查 Claude Code CLI 是否可用。
// 使用与 StartSession 相同的路径解析逻辑。
func IsCLIAvailable() bool {
	return resolveClaudePath() != ""
}

// GetCLIPath 返回 Claude CLI 的解析路径（供诊断用），未找到则返回空字符串。
func GetCLIPath() string {
	return resolveClaudePath()
}

// ResolveProjectWorkDir 解析项目工作目录（桩实现）。
func (pm *ProcessManager) ResolveProjectWorkDir(projectID string) (string, error) {
	return "", fmt.Errorf("未实现")
}
