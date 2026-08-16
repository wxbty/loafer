package cli

import (
	"sync"
	"time"
)

// SessionStatus 会话状态（对应 Java CliProcessManager.SessionStatus 枚举）。
type SessionStatus string

const (
	StatusRunning    SessionStatus = "RUNNING"
	StatusStopped    SessionStatus = "STOPPED"
	StatusError      SessionStatus = "ERROR"
	StatusNotStarted SessionStatus = "NOT_STARTED"
)

// SessionHandle 对应 Java SessionHandle，表示一个 Claude Code CLI 会话句柄。
type SessionHandle struct {
	mu                sync.RWMutex
	sessionID         string
	projectID         string
	taskID            *int64
	workDir           string
	claudeSessionUUID string
	createdAt         time.Time
	lastActiveAt      time.Time
	resumed           bool
	processAlive      bool
}

// NewSessionHandle 构造会话句柄。
func NewSessionHandle(sessionID, projectID, workDir string, taskID *int64, claudeSessionUUID string, resumed bool) *SessionHandle {
	now := time.Now()
	return &SessionHandle{
		sessionID:         sessionID,
		projectID:         projectID,
		taskID:            taskID,
		workDir:           workDir,
		claudeSessionUUID: claudeSessionUUID,
		createdAt:         now,
		lastActiveAt:      now,
		resumed:           resumed,
		processAlive:      true,
	}
}

// Getter 方法
func (h *SessionHandle) SessionID() string         { return h.sessionID }
func (h *SessionHandle) ProjectID() string         { return h.projectID }
func (h *SessionHandle) TaskID() *int64            { return h.taskID }
func (h *SessionHandle) WorkDir() string           { return h.workDir }
func (h *SessionHandle) ClaudeSessionUUID() string { return h.claudeSessionUUID }
func (h *SessionHandle) CreatedAt() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.createdAt
}
func (h *SessionHandle) LastActiveAt() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastActiveAt
}
func (h *SessionHandle) Resumed() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resumed
}
func (h *SessionHandle) ProcessAlive() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.processAlive
}

// UpdateActive 更新最后活跃时间。
func (h *SessionHandle) UpdateActive() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastActiveAt = time.Now()
}

// SetProcessAlive 设置进程存活状态。
func (h *SessionHandle) SetProcessAlive(alive bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.processAlive = alive
}
