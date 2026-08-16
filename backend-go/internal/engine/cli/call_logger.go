package cli

import (
	"unicode/utf8"

	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// truncateUTF8Bytes 按字节截断字符串，并保证不会切断多字节 UTF-8 字符。
func truncateUTF8Bytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// RecordCall 将 Claude CLI 调用结果记录到 llm_call_log 表。
// 由各调用方（PlanGenerator、Decomposer、TaskExecutor 等）在执行完成后调用。
func RecordCall(db *gorm.DB, callType string, projectID, taskID *int64, prompt string, result ExecutionResult, workDir string) {
	// 截断非法字符，避免 MySQL Error 1366: Incorrect string value。
	truncatedPrompt := truncateUTF8Bytes(prompt, 50000)
	truncatedRaw := truncateUTF8Bytes(result.RawOutput, 500000)
	errorMsg := truncateUTF8Bytes(result.Error, 10000)

	log := &model.LlmCallLog{
		ProjectID:  projectID,
		TaskID:     taskID,
		CallType:   callType,
		Prompt:     truncatedPrompt,
		RawOutput:  truncatedRaw,
		ExitCode:   result.ExitCode,
		Success:    result.ExitCode == 0,
		WorkDir:    workDir,
		DurationMs: result.DurationMs,
		ErrorMsg:   errorMsg,
	}
	if err := db.Create(log).Error; err != nil {
		// 日志记录失败不应阻塞主流程，静默忽略
		_ = err
	}
}
