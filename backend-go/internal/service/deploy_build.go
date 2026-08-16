package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// buildFrontendLocal 在本地构建前端，返回构建产物目录（dist 或 build）。
// node_modules 缺失时先执行 npm install。
func (s *DeployService) buildFrontendLocal(workDir string) (string, error) {
	frontendDir, err := DetectFrontendDir(workDir)
	if err != nil {
		return "", err
	}

	if info, statErr := os.Stat(filepath.Join(frontendDir, "node_modules")); statErr != nil || !info.IsDir() {
		out, err := RunLocalCommand(BuildEnvPrefix()+fmt.Sprintf(
			"cd %s && npm install --production=false 2>&1", frontendDir), 15*time.Minute)
		if err != nil {
			return "", fmt.Errorf("npm install 失败: %w, output: %s", err, out)
		}
	}

	out, err := RunLocalCommand(BuildEnvPrefix()+fmt.Sprintf(
		"cd %s && npm run build 2>&1", frontendDir), 10*time.Minute)
	if err != nil {
		return "", fmt.Errorf("npm run build 失败: %w, output: %s", err, out)
	}

	// 优先 dist（Vite 默认），其次 build（CRA 默认）
	for _, name := range []string{"dist", "build"} {
		p := filepath.Join(frontendDir, name)
		if info, statErr := os.Stat(p); statErr == nil && info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("构建产物目录不存在（已检查 %s 和 %s）",
		filepath.Join(frontendDir, "dist"), filepath.Join(frontendDir, "build"))
}

// buildBackendLocal 在本地构建后端二进制，返回二进制路径与临时目录。
// 调用方负责 os.RemoveAll(tmpDir)。
func (s *DeployService) buildBackendLocal(workDir string) (binPath, tmpDir string, err error) {
	backendDir, err := DetectBackendDir(workDir)
	if err != nil {
		return "", "", err
	}
	mainPkg, err := DetectGoMainPackage(backendDir)
	if err != nil {
		return "", "", err
	}

	tmpDir, err = os.MkdirTemp("", "loafer-backend-build-*")
	if err != nil {
		return "", "", fmt.Errorf("创建临时构建目录失败: %w", err)
	}
	binPath = filepath.Join(tmpDir, "backend")

	// CGO_ENABLED=1：生成的项目可能引入 cgo 依赖（如 gorm.io/driver/sqlite
	// 默认使用 mattn/go-sqlite3），CGO_ENABLED=0 产出的 stub 二进制运行即报错。
	// 构建机与部署机 glibc 版本一致（均为 2.28），动态链接可正常运行。
	out, buildErr := RunLocalCommand(BuildEnvPrefix()+fmt.Sprintf(
		"cd %s && CGO_ENABLED=1 go build -o %s %s 2>&1", backendDir, binPath, mainPkg), 10*time.Minute)
	if buildErr != nil {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("go build 失败: %w, output: %s", buildErr, out)
	}
	return binPath, tmpDir, nil
}

// uploadTree 将本地目录内容以 tar.gz 流上传到远程目录。
// clean=true 时先清空远程目录（用于 dist 全量替换）；
// clean=false 仅解包覆盖（用于向 deployDir 投放 backend 二进制，不能误删 dist）。
func (s *DeployService) uploadTree(localDir, remoteDir string, clean bool) error {
	// 本地打包到内存（产物与二进制均为数十 MB 量级，可接受）
	var buf bytes.Buffer
	tarCmd := exec.Command("tar", "-czf", "-", "-C", localDir, ".")
	tarCmd.Stdout = &buf
	var tarErr bytes.Buffer
	tarCmd.Stderr = &tarErr
	if err := tarCmd.Run(); err != nil {
		return fmt.Errorf("本地打包 %s 失败: %w, stderr: %s", localDir, err, tarErr.String())
	}

	remoteCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
	if clean {
		remoteCmd += fmt.Sprintf(" && find %s -mindepth 1 -delete", remoteDir)
	}
	remoteCmd += fmt.Sprintf(" && tar -xzf - -C %s", remoteDir)

	out, err := s.sshClient.RunCommandWithStdin(remoteCmd, &buf, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("上传到 %s 失败: %w, output: %s", remoteDir, err, out)
	}
	return nil
}
