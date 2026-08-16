package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"loafer-agent/internal/engine/cli"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// TerminalWebSocketHandler 对应原 Java TerminalWebSocketController，
// 使用原生 WebSocket 实现终端 I/O，支持多客户端连接同一会话。
type TerminalWebSocketHandler struct {
	pool *cli.SessionPool
	pm   *cli.ProcessManager

	mu      sync.RWMutex
	// 会话ID与WebSocket连接集合的映射（支持多客户端连接同一会话）
	sessionConns map[string]map[*websocket.Conn]bool
	// 会话活跃时间戳（节流更新，避免高频输出频繁触发）
	lastTouch map[string]time.Time
}

const touchThrottle = 60 * time.Second

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，对应原 Spring Boot 的跨域配置
	},
}

// NewTerminalWebSocketHandler 构造终端 WebSocket 处理器。
func NewTerminalWebSocketHandler(pool *cli.SessionPool, pm *cli.ProcessManager) *TerminalWebSocketHandler {
	return &TerminalWebSocketHandler{
		pool:         pool,
		pm:           pm,
		sessionConns: make(map[string]map[*websocket.Conn]bool),
		lastTouch:    make(map[string]time.Time),
	}
}

// RegisterRoutes 注册 WebSocket 路由（/ws/terminal/:sessionId）。
func (h *TerminalWebSocketHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/ws/terminal/:sessionId", h.HandleTerminal)
}

// wsMessage 是 WebSocket 消息的 JSON 结构。
type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// HandleTerminal 处理 WebSocket 连接。
func (h *TerminalWebSocketHandler) HandleTerminal(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的会话ID"})
		return
	}

	// 检查会话是否存在
	handle := h.pool.GetSession(sessionID)
	if handle == nil {
		// 会话不存在，返回包含可恢复提示的错误
		h.upgradeAndSend(c, wsMessage{Type: "ERROR", Data: "会话不存在或已关闭，可通过恢复功能重新连接"})
		return
	}

	if !handle.ProcessAlive() {
		// 进程已退出但 SessionHandle 还在，返回包含 Claude session UUID 的错误信息
		claudeUUID := handle.ClaudeSessionUUID()
		data := "进程已退出"
		if claudeUUID != "" {
			data = "进程已退出|claudeSessionUuid:" + claudeUUID
		}
		h.upgradeAndSend(c, wsMessage{Type: "ERROR", Data: data})
		return
	}

	// 升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket 升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 注册连接
	h.addConn(sessionID, conn)
	defer h.removeConn(sessionID, conn)

	// 设置输出回调，将 PTY 输出转发到所有连接的客户端
	h.pm.SetOutputCallback(sessionID, func(data []byte) {
		h.broadcastOutput(sessionID, string(data))
		h.touchSessionActive(sessionID)
	})

	// 发送连接成功消息
	h.sendMessage(conn, wsMessage{Type: "CONNECTED", Data: "Terminal connected"})

	log.Printf("Terminal WebSocket connected for sessionId: %s", sessionID)

	// 读取客户端消息
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket 读取错误 (sessionId=%s): %v", sessionID, err)
			}
			break
		}

		switch msg.Type {
		case "INPUT":
			if msg.Data != "" {
				h.pm.WriteToStdinRaw(sessionID, msg.Data)
				h.touchSessionActive(sessionID)
			}
		case "HEARTBEAT":
			h.sendMessage(conn, wsMessage{Type: "HEARTBEAT", Data: "pong"})
			h.touchSessionActive(sessionID)
		case "RESIZE":
			if msg.Cols > 0 && msg.Rows > 0 {
				h.pm.ResizeTerminal(sessionID, msg.Cols, msg.Rows)
				h.touchSessionActive(sessionID)
			}
		}
	}

	log.Printf("Terminal WebSocket closed for sessionId: %s", sessionID)
}

// upgradeAndSend 升级连接并发送单条消息后关闭。
func (h *TerminalWebSocketHandler) upgradeAndSend(c *gin.Context, msg wsMessage) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	h.sendMessage(conn, msg)
}

// addConn 添加 WebSocket 连接到会话集合。
func (h *TerminalWebSocketHandler) addConn(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessionConns[sessionID] == nil {
		h.sessionConns[sessionID] = make(map[*websocket.Conn]bool)
	}
	h.sessionConns[sessionID][conn] = true
}

// removeConn 从会话集合中移除 WebSocket 连接。
func (h *TerminalWebSocketHandler) removeConn(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.sessionConns[sessionID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.sessionConns, sessionID)
			delete(h.lastTouch, sessionID)
		}
	}
}

// sendMessage 向单个 WebSocket 连接发送消息。
func (h *TerminalWebSocketHandler) sendMessage(conn *websocket.Conn, msg wsMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("WebSocket 发送失败: %v", err)
	}
}

// broadcastOutput 向会话的所有连接广播终端输出。
func (h *TerminalWebSocketHandler) broadcastOutput(sessionID, output string) {
	h.mu.RLock()
	conns := h.sessionConns[sessionID]
	if len(conns) == 0 {
		h.mu.RUnlock()
		return
	}
	// 复制连接列表，避免在写锁外长时间持有读锁
	connList := make([]*websocket.Conn, 0, len(conns))
	for conn := range conns {
		connList = append(connList, conn)
	}
	h.mu.RUnlock()

	msg := wsMessage{Type: "OUTPUT", Data: output}
	data, _ := json.Marshal(msg)
	for _, conn := range connList {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("WebSocket 广播失败: %v", err)
		}
	}
}

// touchSessionActive 更新会话最后活跃时间（节流：距离上次不足 60 秒时跳过）。
func (h *TerminalWebSocketHandler) touchSessionActive(sessionID string) {
	h.mu.Lock()
	now := time.Now()
	last := h.lastTouch[sessionID]
	if !last.IsZero() && now.Sub(last) < touchThrottle {
		h.mu.Unlock()
		return
	}
	h.lastTouch[sessionID] = now
	h.mu.Unlock()

	// 更新 SessionHandle 的活跃时间
	if handle := h.pool.GetSession(sessionID); handle != nil {
		handle.UpdateActive()
	}
}

// DisconnectTerminal 向会话的所有连接发送断开消息。
func (h *TerminalWebSocketHandler) DisconnectTerminal(sessionID string) {
	h.mu.RLock()
	conns := h.sessionConns[sessionID]
	h.mu.RUnlock()

	msg := wsMessage{Type: "DISCONNECTED", Data: "会话已断开"}
	for conn := range conns {
		h.sendMessage(conn, msg)
	}
}

// IsTerminalConnected 检查终端是否有客户端连接。
func (h *TerminalWebSocketHandler) IsTerminalConnected(sessionID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns := h.sessionConns[sessionID]
	return len(conns) > 0
}

// SendTerminalOutput 向终端发送输出（供外部调用）。
func (h *TerminalWebSocketHandler) SendTerminalOutput(sessionID, output string) {
	if strings.TrimSpace(output) != "" {
		h.broadcastOutput(sessionID, output)
	}
}
