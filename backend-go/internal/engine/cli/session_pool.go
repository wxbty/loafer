package cli

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionPool 对应 Java SessionPool，管理 Claude Code CLI 会话池。
type SessionPool struct {
	mu sync.RWMutex

	pm *ProcessManager

	sessions        map[string]*SessionHandle
	projectSessions map[string][]string
	taskSessions    map[int64][]string

	maxPoolSize   int
	idleTimeout   time.Duration
	reservedCount int
}

// NewSessionPool 构造会话池。
func NewSessionPool(pm *ProcessManager, maxPoolSize, idleTimeoutMin int) *SessionPool {
	return &SessionPool{
		pm:              pm,
		sessions:        make(map[string]*SessionHandle),
		projectSessions: make(map[string][]string),
		taskSessions:    make(map[int64][]string),
		maxPoolSize:     maxPoolSize,
		idleTimeout:     time.Duration(idleTimeoutMin) * time.Minute,
	}
}

// CreateSession 创建新会话。
func (sp *SessionPool) CreateSession(projectID string, taskID *int64, workDir, claudeSessionUUID, profileID string) (string, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// 检查池是否已满
	if sp.reservedCount >= sp.maxPoolSize {
		return "", fmt.Errorf("会话池已满")
	}
	sp.reservedCount++

	sessionID := fmt.Sprintf("%s_%d_%s", projectID, time.Now().Unix(), uuid.New().String()[:8])
	resumed := claudeSessionUUID != ""

	handle := NewSessionHandle(sessionID, projectID, workDir, taskID, claudeSessionUUID, resumed)

	// 启动进程
	if err := sp.pm.StartSession(sessionID, workDir, claudeSessionUUID, profileID, nil); err != nil {
		sp.reservedCount--
		return "", fmt.Errorf("启动会话失败: %w", err)
	}

	sp.sessions[sessionID] = handle
	sp.projectSessions[projectID] = append(sp.projectSessions[projectID], sessionID)
	if taskID != nil {
		sp.taskSessions[*taskID] = append(sp.taskSessions[*taskID], sessionID)
	}

	return sessionID, nil
}

// DestroySession 销毁会话。
func (sp *SessionPool) DestroySession(sessionID string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	handle, ok := sp.sessions[sessionID]
	if !ok {
		return false
	}

	// 停止进程
	sp.pm.StopSession(sessionID)

	// 从映射中移除
	delete(sp.sessions, sessionID)
	if handle.ProjectID() != "" {
		sessions := sp.projectSessions[handle.ProjectID()]
		for i, sid := range sessions {
			if sid == sessionID {
				sp.projectSessions[handle.ProjectID()] = append(sessions[:i], sessions[i+1:]...)
				break
			}
		}
	}
	if handle.TaskID() != nil {
		sessions := sp.taskSessions[*handle.TaskID()]
		for i, sid := range sessions {
			if sid == sessionID {
				sp.taskSessions[*handle.TaskID()] = append(sessions[:i], sessions[i+1:]...)
				break
			}
		}
	}
	sp.reservedCount--
	return true
}

// GetSession 获取会话。
func (sp *SessionPool) GetSession(sessionID string) *SessionHandle {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.sessions[sessionID]
}

// GetSessionsByProject 获取项目的所有会话。
func (sp *SessionPool) GetSessionsByProject(projectID string) []*SessionHandle {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	ids := sp.projectSessions[projectID]
	result := make([]*SessionHandle, 0, len(ids))
	for _, id := range ids {
		if h, ok := sp.sessions[id]; ok {
			result = append(result, h)
		}
	}
	return result
}

// GetSessionsByTask 获取任务的所有会话。
func (sp *SessionPool) GetSessionsByTask(taskID int64) []*SessionHandle {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	ids := sp.taskSessions[taskID]
	result := make([]*SessionHandle, 0, len(ids))
	for _, id := range ids {
		if h, ok := sp.sessions[id]; ok {
			result = append(result, h)
		}
	}
	return result
}

// WaitForSessionReady 等待会话就绪（桩实现：直接等待 1 秒后返回 true）。
func (sp *SessionPool) WaitForSessionReady(sessionID string, timeoutSec int) bool {
	time.Sleep(1 * time.Second)
	return true
}

// GetStatusInfo 获取会话池状态信息。
func (sp *SessionPool) GetStatusInfo() string {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return fmt.Sprintf("pool: %d/%d sessions", len(sp.sessions), sp.maxPoolSize)
}

// CleanupExpiredSessions 清理过期会话。
func (sp *SessionPool) CleanupExpiredSessions() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	now := time.Now()
	for id, handle := range sp.sessions {
		if now.Sub(handle.LastActiveAt()) > sp.idleTimeout {
			sp.pm.StopSession(id)
			delete(sp.sessions, id)
			sp.reservedCount--
		}
	}
}

// SetMaxPoolSize 设置最大池大小。
func (sp *SessionPool) SetMaxPoolSize(size int) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.maxPoolSize = size
}

// SetIdleTimeout 设置空闲超时。
func (sp *SessionPool) SetIdleTimeout(min int) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.idleTimeout = time.Duration(min) * time.Minute
}

// PoolStatusInfo 会话池结构化状态信息（供前端设置页使用）。
type PoolStatusInfo struct {
	ActiveCount        int  `json:"activeCount"`
	PendingCount       int  `json:"pendingCount"`
	MaxPoolSize        int  `json:"maxPoolSize"`
	IdleTimeoutMinutes int  `json:"idleTimeoutMinutes"`
	IsFull             bool `json:"isFull"`
}

// GetDetailedStatusInfo 获取结构化的会话池状态。
func (sp *SessionPool) GetDetailedStatusInfo() PoolStatusInfo {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	active := len(sp.sessions)
	return PoolStatusInfo{
		ActiveCount:        active,
		PendingCount:       0,
		MaxPoolSize:        sp.maxPoolSize,
		IdleTimeoutMinutes: int(sp.idleTimeout.Minutes()),
		IsFull:             active >= sp.maxPoolSize,
	}
}

// GetAllSessions 获取所有活跃会话的句柄。
func (sp *SessionPool) GetAllSessions() []*SessionHandle {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	result := make([]*SessionHandle, 0, len(sp.sessions))
	for _, handle := range sp.sessions {
		result = append(result, handle)
	}
	return result
}

// IsCLIAvailable 检查 Claude Code CLI 是否可用（委托给包级函数）。
func (sp *SessionPool) IsCLIAvailable() bool {
	return IsCLIAvailable()
}

// Close 关闭会话池，清理所有会话。
func (sp *SessionPool) Close() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for id := range sp.sessions {
		sp.pm.StopSession(id)
	}
	sp.sessions = make(map[string]*SessionHandle)
	sp.projectSessions = make(map[string][]string)
	sp.taskSessions = make(map[int64][]string)
}
