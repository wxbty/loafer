package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SSEWriter 封装 Server-Sent Events 写入逻辑，对应原 Java SseEmitter。
// 使用方式：
//   sse := util.NewSSEWriter(c)
//   sse.SendOutput("正在执行...")
//   sse.SendDone(resultData)
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSEWriter 创建 SSE 写入器并写入响应头。
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	return &SSEWriter{w: w, flusher: flusher}
}

// SendEvent 发送指定类型的 SSE 事件。
func (s *SSEWriter) SendEvent(eventType, data string) {
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, data)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// SendOutput 发送 output 事件（对应 SseEmitter.event().name("output").data(line)）。
func (s *SSEWriter) SendOutput(data string) {
	s.SendEvent("output", data)
}

// SendDone 发送 done 事件并关闭连接。
// data 会被 JSON 序列化（对应 Java 的 JSON.toJSONString）。
func (s *SSEWriter) SendDone(data interface{}) {
	jsonBytes, _ := json.Marshal(data)
	s.SendEvent("done", string(jsonBytes))
}

// SendDoneRaw 发送 done 事件，data 为原始字符串。
func (s *SSEWriter) SendDoneRaw(data string) {
	s.SendEvent("done", data)
}

// SendError 发送 error 事件并关闭连接。
func (s *SSEWriter) SendError(err string) {
	s.SendEvent("error", err)
}

// SendOutputJSON 发送 output 事件，data 为 JSON 序列化后的对象。
func (s *SSEWriter) SendOutputJSON(data interface{}) {
	jsonBytes, _ := json.Marshal(data)
	s.SendEvent("output", string(jsonBytes))
}

// Close 关闭连接。
func (s *SSEWriter) Close() {
	if closer, ok := s.w.(io.Closer); ok {
		closer.Close()
	}
}
