package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"loafer-agent/internal/config"

	"golang.org/x/crypto/ssh"
)

// SSHClient 封装到远程部署服务器的 SSH 连接。
// 支持通过 PEM 私钥内容或私钥文件路径进行认证，
// 提供远程命令执行和远程文件写入能力。
//
// 连接为懒重连模式：启动时拨号失败（如网络抖动）不会永久不可用，
// 后续调用会自动重新拨号；已建立的连接断开后也会在下次调用时重连一次。
type SSHClient struct {
	mu     sync.Mutex
	client *ssh.Client
	cfg    *config.InfraConfig
	signer ssh.Signer
}

// NewSSHClient 根据基础设施配置创建 SSH 客户端。
// 优先使用 SSHPemContent（直接内容），其次使用 SSHKeyPath（文件路径）。
//
// 配置错误（缺地址/用户/密钥）返回 (nil, err)；
// 初次拨号失败返回 (client, err)：client 仍可使用，调用时会自动重试连接。
func NewSSHClient(cfg *config.InfraConfig) (*SSHClient, error) {
	if cfg.ServerHost == "" {
		return nil, fmt.Errorf("SSH 服务器地址未配置")
	}
	if cfg.SSHUser == "" {
		return nil, fmt.Errorf("SSH 用户名未配置")
	}

	var signer ssh.Signer
	var err error

	// 优先使用 PEM 内容
	if cfg.SSHPemContent != "" {
		signer, err = ssh.ParsePrivateKey([]byte(cfg.SSHPemContent))
		if err != nil {
			return nil, fmt.Errorf("解析 PEM 私钥内容失败: %w", err)
		}
	} else if cfg.SSHKeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.SSHKeyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥文件 %s 失败: %w", cfg.SSHKeyPath, err)
		}
		signer, err = ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("解析私钥文件失败: %w", err)
		}
	} else {
		return nil, fmt.Errorf("SSH 认证方式未配置：请设置 INFRA_SSH_PEM 或 INFRA_SSH_KEY_PATH")
	}

	c := &SSHClient{cfg: cfg, signer: signer}
	if err := c.dial(); err != nil {
		return c, err
	}
	return c, nil
}

// dial 建立 SSH 连接（调用方须持有 c.mu 或保证单线程）。
func (c *SSHClient) dial() error {
	sshCfg := &ssh.ClientConfig{
		User:            c.cfg.SSHUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(c.signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:22", c.cfg.ServerHost)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return fmt.Errorf("连接 SSH 服务器 %s 失败: %w", addr, err)
	}
	c.client = client
	return nil
}

// newSession 创建 SSH 会话：未连接时先懒重连；会话创建失败（连接已断开）
// 时重拨一次再试。
func (c *SSHClient) newSession() (*ssh.Session, error) {
	c.mu.Lock()
	if c.client == nil {
		if err := c.dial(); err != nil {
			c.mu.Unlock()
			return nil, err
		}
	}
	client := c.client
	c.mu.Unlock()

	session, err := client.NewSession()
	if err == nil {
		return session, nil
	}

	// 连接可能已断开：置 nil 后重拨一次
	c.mu.Lock()
	if c.client == client {
		c.client = nil
	}
	if c.client == nil {
		if dialErr := c.dial(); dialErr != nil {
			c.mu.Unlock()
			return nil, fmt.Errorf("创建 SSH 会话失败且重连失败: %w（原错误: %v）", dialErr, err)
		}
	}
	client = c.client
	c.mu.Unlock()

	session, err = client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	return session, nil
}

// RunCommand 在远程服务器上执行命令，返回合并的输出和错误。
// 超时时间默认为 10 分钟。
func (c *SSHClient) RunCommand(cmd string) (string, error) {
	return c.RunCommandWithTimeout(cmd, 10*time.Minute)
}

// RunCommandWithTimeout 在远程服务器上执行命令，使用指定的超时时间。
func (c *SSHClient) RunCommandWithTimeout(cmd string, timeout time.Duration) (string, error) {
	session, err := c.newSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err := <-done:
		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n[stderr]\n" + stderr.String()
		}
		if err != nil {
			return output, fmt.Errorf("命令执行失败: %w, output: %s", err, output)
		}
		return output, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("命令执行超时 (%v): %s", timeout, cmd)
	}
}

// RunCommandWithStdin 在远程服务器上执行命令，stdin 由 reader 流式提供（用于 tar 流上传）。
func (c *SSHClient) RunCommandWithStdin(cmd string, stdin io.Reader, timeout time.Duration) (string, error) {
	session, err := c.newSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	session.Stdin = stdin
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(cmd)
	}()

	select {
	case err := <-done:
		output := stdout.String()
		if stderr.Len() > 0 {
			output += "\n[stderr]\n" + stderr.String()
		}
		if err != nil {
			return output, fmt.Errorf("命令执行失败: %w, output: %s", err, output)
		}
		return output, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("命令执行超时 (%v): %s", timeout, cmd)
	}
}

// RunCommandBackground 在远程服务器上以后台方式执行命令（nohup）。
// 返回远程进程的 PID。
func (c *SSHClient) RunCommandBackground(cmd string) (int, error) {
	return c.RunCommandBackgroundLogged(cmd, "/dev/null")
}

// RunCommandBackgroundLogged 在远程服务器上以后台方式执行命令（nohup），
// stdout/stderr 追加写入 logPath，便于排查后台进程启动失败的原因。
// 返回远程进程的 PID。
func (c *SSHClient) RunCommandBackgroundLogged(cmd, logPath string) (int, error) {
	// 使用 nohup 启动，并通过 echo $! 获取 PID
	wrappedCmd := fmt.Sprintf("nohup %s >> %s 2>&1 & echo $!", cmd, logPath)
	output, err := c.RunCommand(wrappedCmd)
	if err != nil {
		return 0, err
	}

	pidStr := strings.TrimSpace(output)
	// 取最后一行作为 PID（前面可能有 nohup 的输出）
	lines := strings.Split(pidStr, "\n")
	pidStr = strings.TrimSpace(lines[len(lines)-1])

	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		return 0, fmt.Errorf("解析 PID 失败: %w, output: %s", err, output)
	}
	return pid, nil
}

// WriteRemoteFile 将内容写入远程服务器的指定路径。
// 使用 base64 编码传输，避免特殊字符转义问题。
func (c *SSHClient) WriteRemoteFile(remotePath string, content []byte, mode string) error {
	encoded := base64.StdEncoding.EncodeToString(content)
	cmd := fmt.Sprintf("echo '%s' | base64 -d > %s && chmod %s %s",
		encoded, remotePath, mode, remotePath)

	_, err := c.RunCommand(cmd)
	if err != nil {
		return fmt.Errorf("写入远程文件 %s 失败: %w", remotePath, err)
	}
	return nil
}

// UploadFile 将本地文件上传到远程服务器。
func (c *SSHClient) UploadFile(localPath, remotePath string, mode string) error {
	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("读取本地文件 %s 失败: %w", localPath, err)
	}
	return c.WriteRemoteFile(remotePath, content, mode)
}

// MkdirRemote 在远程服务器上创建目录（递归）。
func (c *SSHClient) MkdirRemote(path string) error {
	_, err := c.RunCommand(fmt.Sprintf("mkdir -p %s", path))
	return err
}

// RemoveRemoteFile 删除远程服务器上的文件。
func (c *SSHClient) RemoveRemoteFile(path string) error {
	_, err := c.RunCommand(fmt.Sprintf("rm -f %s", path))
	return err
}

// FileExists 检查远程文件是否存在。
func (c *SSHClient) FileExists(path string) (bool, error) {
	output, err := c.RunCommand(fmt.Sprintf("test -f %s && echo EXISTS || echo NOTFOUND", path))
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "EXISTS"), nil
}

// IsProcessAlive 检查远程服务器上指定 PID 的进程是否存活。
func (c *SSHClient) IsProcessAlive(pid int) (bool, error) {
	output, err := c.RunCommand(fmt.Sprintf("kill -0 %d 2>/dev/null && echo ALIVE || echo DEAD", pid))
	if err != nil {
		return false, err
	}
	return strings.Contains(output, "ALIVE"), nil
}

// KillProcess 终止远程服务器上指定 PID 的进程。
func (c *SSHClient) KillProcess(pid int) error {
	_, err := c.RunCommand(fmt.Sprintf("kill %d 2>/dev/null; sleep 1; kill -9 %d 2>/dev/null; echo done", pid, pid))
	return err
}

// Close 关闭 SSH 连接。
func (c *SSHClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}

// DialTest 测试 TCP 端口连通性（用于验证端口是否可用）。
func DialTest(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// copyToReader 将字节切片包装为 io.Reader（用于 SCP 等场景）。
func copyToReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}
