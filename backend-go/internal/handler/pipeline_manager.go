package handler

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ProgressWriter 统一进度写入接口。
// *util.SSEWriter（直接 SSE 推送）和 *RunnerWriter（后台执行时写入事件总线）均实现此接口。
type ProgressWriter interface {
	SendOutput(data string)
	SendOutputJSON(data interface{})
	SendDone(data interface{})
	SendDoneRaw(data string)
	SendError(err string)
	Close()
}

// PipelineEvent 表示流水线执行过程中的一个事件。
type PipelineEvent struct {
	Type    string `json:"type"`    // "output", "error", "done"
	Payload string `json:"payload"` // 原始文本或 JSON 字符串
}

// PipelineRunner 管理单个项目的流水线执行状态和事件分发。
// 核心设计：
//   - 事件存储在内存切片中，支持客户端断线重连后回放
//   - 通过 per-subscriber 的通知 channel 实现实时推送
//   - 后台 goroutine 通过 RunnerWriter 发布事件，SSE handler 订阅事件
//   - 客户端断开不影响后台执行
type PipelineRunner struct {
	mu          sync.RWMutex
	events      []PipelineEvent
	done        bool
	err         error
	started     bool
	nextSubID   int64
	subscribers map[int64]chan struct{}
}

// NewPipelineRunner 创建流水线运行器。
func NewPipelineRunner() *PipelineRunner {
	return &PipelineRunner{
		subscribers: make(map[int64]chan struct{}),
	}
}

// IsDone 返回流水线是否已完成。
func (r *PipelineRunner) IsDone() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.done
}

// IsStarted 返回流水线是否已启动。
func (r *PipelineRunner) IsStarted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started
}

// SetStarted 标记流水线已启动。
func (r *PipelineRunner) SetStarted() {
	r.mu.Lock()
	r.started = true
	r.mu.Unlock()
}

// Publish 发布一个事件到所有订阅者。
func (r *PipelineRunner) Publish(evt PipelineEvent) {
	r.mu.Lock()
	r.events = append(r.events, evt)
	subs := make([]chan struct{}, 0, len(r.subscribers))
	for _, ch := range r.subscribers {
		subs = append(subs, ch)
	}
	r.mu.Unlock()

	// 非阻塞通知每个订阅者
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default: // 通知信号已 pending，无需重复
		}
	}
}

// MarkDone 标记流水线执行完成。
func (r *PipelineRunner) MarkDone(err error) {
	r.mu.Lock()
	r.done = true
	r.err = err
	subs := make([]chan struct{}, 0, len(r.subscribers))
	for _, ch := range r.subscribers {
		subs = append(subs, ch)
	}
	r.mu.Unlock()

	// 通知所有订阅者流水线已完成
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Subscribe 订阅流水线事件。
// 返回：订阅 ID、当前事件数量、是否已完成、错误信息。
// 调用方应先回放 GetEventsSince(0) 获取的历史事件，然后循环调用 GetEventsSince(index) 获取增量。
func (r *PipelineRunner) Subscribe() (int64, int, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextSubID
	r.nextSubID++
	r.subscribers[id] = make(chan struct{}, 1)

	return id, len(r.events), r.done, r.err
}

// GetNotifyCh 返回订阅者的通知 channel。
// 当有新事件发布或流水线完成时，会向此 channel 发送信号。
func (r *PipelineRunner) GetNotifyCh(id int64) chan struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.subscribers[id]
}

// GetEventsSince 返回从指定索引开始的所有事件。
// 返回：事件列表、当前事件总数、是否已完成、错误信息。
func (r *PipelineRunner) GetEventsSince(index int) ([]PipelineEvent, int, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if index >= len(r.events) {
		return nil, len(r.events), r.done, r.err
	}
	events := make([]PipelineEvent, len(r.events)-index)
	copy(events, r.events[index:])
	return events, len(r.events), r.done, r.err
}

// Unsubscribe 取消订阅。
func (r *PipelineRunner) Unsubscribe(id int64) {
	r.mu.Lock()
	delete(r.subscribers, id)
	r.mu.Unlock()
}

// RunnerWriter 实现 ProgressWriter 接口，将事件发布到 PipelineRunner。
// 用于后台 goroutine 执行流水线时的进度写入。
type RunnerWriter struct {
	runner *PipelineRunner
}

// SendOutput 发布一个 output 事件（纯文本）。
func (w *RunnerWriter) SendOutput(data string) {
	w.runner.Publish(PipelineEvent{Type: "output", Payload: data})
}

// SendOutputJSON 发布一个 output 事件（JSON 序列化后的对象）。
func (w *RunnerWriter) SendOutputJSON(data interface{}) {
	jsonBytes, _ := json.Marshal(data)
	w.runner.Publish(PipelineEvent{Type: "output", Payload: string(jsonBytes)})
}

// SendDone 发布一个 done 事件并标记流水线完成。
func (w *RunnerWriter) SendDone(data interface{}) {
	jsonBytes, _ := json.Marshal(data)
	w.runner.Publish(PipelineEvent{Type: "done", Payload: string(jsonBytes)})
	w.runner.MarkDone(nil)
}

// SendDoneRaw 发布一个 done 事件（原始字符串）并标记流水线完成。
func (w *RunnerWriter) SendDoneRaw(data string) {
	w.runner.Publish(PipelineEvent{Type: "done", Payload: data})
	w.runner.MarkDone(nil)
}

// SendError 发布一个 error 事件（不标记完成，后续仍需调用 SendDone）。
func (w *RunnerWriter) SendError(err string) {
	w.runner.Publish(PipelineEvent{Type: "error", Payload: err})
}

// Close 如果流水线尚未完成，标记为完成（安全兜底）。
func (w *RunnerWriter) Close() {
	if !w.runner.IsDone() {
		w.runner.MarkDone(fmt.Errorf("writer closed without SendDone"))
	}
}

// PipelineManager 管理所有项目的流水线运行器。
type PipelineManager struct {
	mu      sync.Mutex
	runners map[int64]*PipelineRunner
}

// NewPipelineManager 创建流水线管理器。
func NewPipelineManager() *PipelineManager {
	return &PipelineManager{
		runners: make(map[int64]*PipelineRunner),
	}
}

// GetOrCreate 获取或创建项目的流水线运行器。
// 如果不存在或上一个运行器已完成，创建新的运行器。
func (m *PipelineManager) GetOrCreate(projectID int64) *PipelineRunner {
	m.mu.Lock()
	defer m.mu.Unlock()

	runner, ok := m.runners[projectID]
	if !ok || runner.IsDone() {
		runner = NewPipelineRunner()
		m.runners[projectID] = runner
	}
	return runner
}
