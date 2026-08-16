package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ExecutionResult 离线执行结果，与 claude_sprint OfflineTerminalExecutor.ExecutionResult 对齐。
type ExecutionResult struct {
	Response          string // 可展示文本拼接（含系统事件/思考/完成标记等，供前端展示进度）
	AIResponse        string // AI 的纯文本输出（仅 content_block_delta 的 text，供程序解析）
	RawOutput         string // 原始 stream-json 输出（每行一个 JSON 对象，用于持久化和排查）
	ClaudeSessionUUID string // 从 stream-json 中提取的 session_id
	ExitCode          int    // CLI 进程退出码
	Error             string // 错误信息
	DurationMs        int64  // 执行耗时（毫秒），从 stream-json result 事件提取
}

// OfflineExecutor 基于 claude --print --output-format stream-json 的非交互式执行器。
// 参考 claude_sprint 的 OfflineTerminalExecutor 实现，替代 PTY 交互式会话模式。
// 关键区别：--print 模式下 Claude CLI 执行完 prompt 后自动退出，不存在交互式 REPL 不退出的问题。
type OfflineExecutor struct {
	mu       sync.Mutex
	processes map[int64]*exec.Cmd // jobID -> 活跃进程
}

// NewOfflineExecutor 构造离线执行器。
func NewOfflineExecutor() *OfflineExecutor {
	return &OfflineExecutor{
		processes: make(map[int64]*exec.Cmd),
	}
}

// Execute 在项目工作目录中以非交互模式执行 Claude CLI。
// 参数：
//   - jobID: 任务标识，用于进程管理和取消
//   - workDir: 工作目录
//   - prompt: 用户提示词（作为命令行参数传递给 claude --print）
//   - resumeUUID: 可选，上次 Claude 会话 UUID，用于 --resume 续跑
//   - onOutput: 流式输出回调
//
// 返回执行结果。
func (e *OfflineExecutor) Execute(jobID int64, workDir, prompt, resumeUUID string, onOutput func(string)) ExecutionResult {
	if prompt == "" {
		if onOutput != nil {
			onOutput("[错误] 输入为空")
		}
		return ExecutionResult{ExitCode: -1, Error: "输入为空"}
	}

	// 解析 claude 可执行文件路径
	claudePath := resolveClaudePath()
	if claudePath == "" {
		msg := "claude 命令未找到，请确保已安装 Claude Code CLI"
		if onOutput != nil {
			onOutput("[错误] " + msg)
		}
		return ExecutionResult{ExitCode: -1, Error: msg}
	}

	// 确保工作目录存在
	if workDir != "" {
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			if mkErr := os.MkdirAll(workDir, 0755); mkErr != nil {
				msg := fmt.Sprintf("创建工作目录失败 %s: %v", workDir, mkErr)
				if onOutput != nil {
					onOutput("[错误] " + msg)
				}
				return ExecutionResult{ExitCode: -1, Error: msg}
			}
		}
	}

	// 构建命令：claude --print --output-format stream-json --dangerously-skip-permissions <prompt>
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
		"--append-system-prompt", "你必须始终使用中文进行输出。所有分析、描述、计划、摘要、注释都必须使用中文。技术名词可保留英文原文，但所有解释和说明必须使用中文。",
	}
	if resumeUUID != "" {
		args = append(args, "--resume", resumeUUID)
	}
	args = append(args, prompt)

	cmd := exec.Command(claudePath, args...)
	cmd.Dir = workDir
	cmd.Env = buildClaudeEnv()
	// 将 CLI 及其子进程放入独立进程组，便于在主进程退出后清理残留子进程
	// （插件/Hook 可能派生子进程持有 stdout 管道，导致读取循环无法收到 EOF）
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// 将 stdin 重定向到 /dev/null，防止 CLI 读取 stdin
	devNull, _ := os.Open(os.DevNull)
	if devNull != nil {
		cmd.Stdin = devNull
	}

	// 合并 stderr 到 stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		msg := fmt.Sprintf("获取 stdout 管道失败: %v", err)
		if onOutput != nil {
			onOutput("[错误] " + msg)
		}
		return ExecutionResult{ExitCode: -1, Error: msg}
	}
	cmd.Stderr = cmd.Stdout

	if onOutput != nil {
		onOutput("[系统] 正在启动 Claude Code CLI（非交互模式）…")
	}

	if err := cmd.Start(); err != nil {
		msg := fmt.Sprintf("启动 Claude CLI 失败: %v", err)
		if onOutput != nil {
			onOutput("[错误] " + msg)
		}
		return ExecutionResult{ExitCode: -1, Error: msg}
	}

	// 注册进程
	e.mu.Lock()
	e.processes[jobID] = cmd
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.processes, jobID)
		e.mu.Unlock()
		if devNull != nil {
			devNull.Close()
		}
	}()

	if onOutput != nil {
		onOutput(fmt.Sprintf("[系统] CLI 进程已启动 (PID: %d)，等待输出…", cmd.Process.Pid))
	}

	// 读取 stream-json 输出（在 goroutine 中执行）。
	// 注意：CLI 的插件/Hook 可能派生子进程持有 stdout 管道，即使主进程退出，
	// 读取侧也无法收到 EOF。因此读取循环放在 goroutine 中，由主进程退出后
	// 杀掉整个进程组来释放管道，使读取循环能够退出。
	var response strings.Builder
	var aiText strings.Builder // 只收集 content_block_delta 的 text，供程序解析
	var rawOutput strings.Builder
	var streamUUID string
	var lastError string
	var durationMs int64

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				line = strings.TrimRight(line, "\r\n")

				// 记录原始输出
				if rawOutput.Len() > 0 {
					rawOutput.WriteByte('\n')
				}
				rawOutput.WriteString(line)

				// 提取 session UUID
				if uuid := extractClaudeSessionID(line); uuid != "" {
					streamUUID = uuid
				}

				// 提取错误信息
				if errMsg := extractStreamError(line); errMsg != "" {
					lastError = errMsg
				}

				// 提取执行耗时
				if ms := extractDurationMs(line); ms > 0 {
					durationMs = ms
				}

				// 提取可展示文本
				if display := extractDisplayText(line); display != "" {
					if onOutput != nil {
						onOutput(display)
					}
					if response.Len() > 0 {
						response.WriteByte('\n')
					}
					response.WriteString(display)
				}

				// 提取 AI 纯文本（仅 content_block_delta 的 text 字段）
				// 这是 AI 实际生成的文本，不含系统事件/思考/完成标记，供程序解析
				if aiChunk := extractAIText(line); aiChunk != "" {
					aiText.WriteString(aiChunk)
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// 等待进程退出（带超时）
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var exitCode int
	timedOut := false
	select {
	case waitErr := <-done:
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
	case <-time.After(120 * time.Minute):
		timedOut = true
		exitCode = -1
	}

	// 主进程退出或超时后，杀掉整个进程组（含残留子进程），释放被持有的 stdout 管道，
	// 使读取 goroutine 收到 EOF 并退出。这是修复"提交需求后卡住直到 network error"的关键。
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if timedOut {
		<-done // 等待进程真正退出
	}
	<-readDone // 等待读取 goroutine 结束

	if timedOut {
		msg := "[错误] 执行超时（>120 分钟）"
		if onOutput != nil {
			onOutput(msg)
		}
		return ExecutionResult{
			Response:   response.String(),
			AIResponse: aiText.String(),
			RawOutput:  rawOutput.String(),
			ExitCode:   -1,
			Error:      "执行超时",
		}
	}

	// 处理空输出 + 错误的情况
	if response.Len() == 0 && lastError != "" {
		errText := "[错误] " + lastError
		if onOutput != nil {
			onOutput(errText)
		}
		response.WriteString(errText)
	}

	// 非零退出码
	if exitCode != 0 && response.Len() == 0 {
		msg := lastError
		if msg == "" {
			msg = fmt.Sprintf("CLI 退出码 %d", exitCode)
		}
		errText := "[错误] " + msg
		if onOutput != nil {
			onOutput(errText)
		}
		return ExecutionResult{
			Response:   response.String(),
			AIResponse: aiText.String(),
			RawOutput:  rawOutput.String(),
			ExitCode:   exitCode,
			Error:      msg,
		}
	}

	// 如果 content_block_delta 没有输出文本（--print 模式通常不输出 content_block_delta），
	// 从 result 事件的 result 字段提取 AI 完整文本
	aiResponse := aiText.String()
	if aiResponse == "" {
		aiResponse = extractResultTextFromRawOutput(rawOutput.String())
	}

	return ExecutionResult{
		Response:          response.String(),
		AIResponse:        aiResponse,
		RawOutput:         rawOutput.String(),
		ClaudeSessionUUID: streamUUID,
		ExitCode:          exitCode,
		DurationMs:        durationMs,
	}
}

// ExecuteSimple 简化的执行接口，不需要 jobID 管理，用于一次性调用。
func (e *OfflineExecutor) ExecuteSimple(workDir, prompt string, onOutput func(string)) ExecutionResult {
	return e.Execute(0, workDir, prompt, "", onOutput)
}

// Cancel 取消指定任务的 CLI 进程。
func (e *OfflineExecutor) Cancel(jobID int64) {
	e.mu.Lock()
	cmd, ok := e.processes[jobID]
	e.mu.Unlock()
	if ok && cmd.Process != nil {
		cmd.Process.Kill()
	}
}

// StopAll 杀死所有正在运行的 CLI 进程。
// 返回被杀掉的进程数量。用于流水线一键停止场景。
func (e *OfflineExecutor) StopAll() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := 0
	for jobID, cmd := range e.processes {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			count++
		}
		delete(e.processes, jobID)
	}
	return count
}

// RunningCount 返回当前正在运行的 CLI 进程数量。
func (e *OfflineExecutor) RunningCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.processes)
}

// ===== stream-json 解析工具函数 =====

// extractDurationMs 从 stream-json 的 result 事件中提取执行耗时。
func extractDurationMs(line string) int64 {
	if line == "" {
		return 0
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &root); err != nil {
		return 0
	}
	typ, _ := parseStringField(root["type"])
	if typ != "result" {
		return 0
	}
	raw, ok := root["duration_ms"]
	if !ok {
		return 0
	}
	var ms int64
	if json.Unmarshal(raw, &ms) == nil {
		return ms
	}
	return 0
}

// extractClaudeSessionID 从 stream-json 行中提取 session_id。
func extractClaudeSessionID(line string) string {
	if line == "" {
		return ""
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &root); err != nil {
		return ""
	}
	raw, ok := root["session_id"]
	if !ok {
		return ""
	}
	var sid string
	if err := json.Unmarshal(raw, &sid); err == nil && sid != "" {
		return sid
	}
	return ""
}

// extractStreamError 从 stream-json 行中提取错误信息（type=result 且 is_error=true）。
func extractStreamError(line string) string {
	if line == "" {
		return ""
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(line), &root); err != nil {
		return ""
	}
	if typ, _ := root["type"].(string); typ != "result" {
		return ""
	}
	if isErr, _ := root["is_error"].(bool); !isErr {
		return ""
	}
	if r, ok := root["result"]; ok {
		if s, ok := r.(string); ok {
			return strings.TrimSpace(s)
		}
		b, _ := json.Marshal(r)
		return strings.TrimSpace(string(b))
	}
	if m, ok := root["message"]; ok {
		if s, ok := m.(string); ok {
			return strings.TrimSpace(s)
		}
		b, _ := json.Marshal(m)
		return strings.TrimSpace(string(b))
	}
	return ""
}

// extractAIText 从 stream-json 单行提取 AI 的纯文本输出。
// 只处理 content_block_delta 类型，提取其 delta.text 字段。
// 这是 AI 实际生成的文本片段，不含系统事件/思考/工具调用等元数据。
func extractAIText(line string) string {
	if line == "" {
		return ""
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &root); err != nil {
		return ""
	}
	typ, _ := parseStringField(root["type"])
	if typ != "content_block_delta" {
		return ""
	}
	delta := root["delta"]
	if delta == nil {
		return ""
	}
	var deltaMap map[string]json.RawMessage
	if err := json.Unmarshal(delta, &deltaMap); err != nil {
		return ""
	}
	text, _ := parseStringField(deltaMap["text"])
	return text
}

// extractResultTextFromRawOutput 从原始 stream-json 输出中提取 result 事件的 result 字段。
// --print 模式下 AI 文本通常通过 result 事件输出（而非 content_block_delta），
// 此函数按行扫描 rawOutput，找到 type=result 的行并提取 result 字段的纯文本。
func extractResultTextFromRawOutput(rawOutput string) string {
	for _, line := range strings.Split(rawOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &root); err != nil {
			continue
		}
		typ, _ := parseStringField(root["type"])
		if typ != "result" {
			continue
		}
		r := root["result"]
		if r == nil {
			continue
		}
		var s string
		if json.Unmarshal(r, &s) == nil {
			return s
		}
	}
	return ""
}

// extractDisplayText 从 stream-json 单行提取可展示文本。
// 与 claude_sprint 的 OfflineTerminalExecutor.extractDisplayText 对齐。
func extractDisplayText(line string) string {
	if line == "" {
		return ""
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &root); err != nil {
		return ""
	}

	typ, _ := parseStringField(root["type"])

	switch typ {
	case "assistant":
		return extractAssistantDisplay(root)
	case "user":
		msg := root["message"]
		var s string
		if msg != nil {
			s = string(msg)
		} else {
			s = line
		}
		if len(s) > 800 {
			s = s[:800] + "…"
		}
		return "[工具结果] " + s

	case "result":
		return extractResultDisplay(root)

	case "system":
		subtype, _ := parseStringField(root["subtype"])
		// 过滤高频无意义系统事件。thinking_tokens 是 CLI 的 token 计数器，执行期间
		// 每秒多次触发，若全量转成 SSE 输出会让日志飞速增长、断线重连时后端全量回放
		// 暴涨（实测 1.5 分钟达 2.6MB，其中 98.5% 是该噪声），最终拖垮前端主线程，
		// 表现为任务运行中白屏/页面无响应、点击续跑后状态丢失。
		switch subtype {
		case "thinking_tokens":
			return ""
		}
		if subtype == "init" {
			model, _ := parseStringField(root["model"])
			return "[系统] 模型: " + model
		}
		raw := line
		if len(raw) > 400 {
			raw = raw[:400] + "…"
		}
		return "[系统] " + subtype + " " + raw

	case "content_block_delta":
		delta := root["delta"]
		if delta == nil {
			return ""
		}
		var deltaMap map[string]json.RawMessage
		if err := json.Unmarshal(delta, &deltaMap); err != nil {
			return ""
		}
		text, _ := parseStringField(deltaMap["text"])
		return text

	case "message_delta", "content_block_stop", "message_stop":
		return ""
	}

	return ""
}

// extractAssistantDisplay 从 type=assistant 的行提取展示文本。
func extractAssistantDisplay(root map[string]json.RawMessage) string {
	msg := root["message"]
	if msg == nil {
		return ""
	}
	var msgObj map[string]json.RawMessage
	if err := json.Unmarshal(msg, &msgObj); err != nil {
		return ""
	}
	contentArr := msgObj["content"]
	if contentArr == nil {
		return ""
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(contentArr, &items); err != nil {
		return ""
	}

	var sb strings.Builder
	for _, item := range items {
		blockType, _ := parseStringField(item["type"])
		switch blockType {
		case "text":
			text, _ := parseStringField(item["text"])
			sb.WriteString(text)
		case "tool_use":
			name, _ := parseStringField(item["name"])
			sb.WriteString("\n[工具] ")
			sb.WriteString(name)
			if input := item["input"]; input != nil {
				in := string(input)
				if len(in) > 400 {
					in = in[:400] + "…"
				}
				sb.WriteString(" ")
				sb.WriteString(in)
			}
		case "thinking", "redacted_thinking":
			sb.WriteString("\n[思考] …")
		default:
			if blockType != "" {
				sb.WriteString("\n[块] ")
				sb.WriteString(blockType)
			}
		}
	}

	return strings.TrimSpace(sb.String())
}

// extractResultDisplay 从 type=result 的行提取展示文本。
func extractResultDisplay(root map[string]json.RawMessage) string {
	var sb strings.Builder
	sb.WriteString("[完成]")

	if subtype, _ := parseStringField(root["subtype"]); subtype != "" {
		sb.WriteString(" ")
		sb.WriteString(subtype)
	}

	if isErr, _ := parseBoolField(root["is_error"]); isErr {
		sb.WriteString(" 错误")
	}

	if dur := root["duration_ms"]; dur != nil {
		var ms int64
		if json.Unmarshal(dur, &ms) == nil && ms > 0 {
			sb.WriteString(fmt.Sprintf(" 耗时%dms", ms))
		}
	}

	if r := root["result"]; r != nil {
		var s string
		if json.Unmarshal(r, &s) == nil {
			sb.WriteString("\n")
			sb.WriteString(s)
		} else {
			raw := string(r)
			if len(raw) > 3500 {
				raw = raw[:3500] + "…"
			}
			sb.WriteString("\n")
			sb.WriteString(raw)
		}
	} else if m := root["message"]; m != nil {
		var s string
		if json.Unmarshal(m, &s) == nil {
			if len(s) > 3500 {
				s = s[:3500] + "…"
			}
			sb.WriteString("\n")
			sb.WriteString(s)
		} else {
			raw := string(m)
			if len(raw) > 3500 {
				raw = raw[:3500] + "…"
			}
			sb.WriteString("\n")
			sb.WriteString(raw)
		}
	}

	return strings.TrimSpace(sb.String())
}

// parseStringField 从 json.RawMessage 解析字符串字段。
func parseStringField(raw json.RawMessage) (string, bool) {
	if raw == nil {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// parseBoolField 从 json.RawMessage 解析布尔字段。
func parseBoolField(raw json.RawMessage) (bool, bool) {
	if raw == nil {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

// ExecuteWithCancel 带 context 取消的执行，支持外部取消。
func (e *OfflineExecutor) ExecuteWithCancel(ctx context.Context, jobID int64, workDir, prompt, resumeUUID string, onOutput func(string)) ExecutionResult {
	resultCh := make(chan ExecutionResult, 1)
	go func() {
		resultCh <- e.Execute(jobID, workDir, prompt, resumeUUID, onOutput)
	}()

	select {
	case result := <-resultCh:
		return result
	case <-ctx.Done():
		e.Cancel(jobID)
		return ExecutionResult{
			ExitCode: -1,
			Error:    "执行已取消: " + ctx.Err().Error(),
		}
	}
}
