package dto

// LoginRequest 登录请求（对应 dto.LoginRequest）。
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 登录响应（对应 dto.LoginResponse）。
type LoginResponse struct {
	Token      string `json:"token"`
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	Role       string `json:"role"`
}

// TerminalInputMessage 终端输入消息（对应 dto.TerminalInputMessage）。
type TerminalInputMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

// MentionResult @提及结果（对应 dto.MentionResult）。
type MentionResult struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ModuleWithTasksDTO 模块及其任务（对应 dto.ModuleWithTasksDTO）。
type ModuleWithTasksDTO struct {
	Module interface{} `json:"module"`
	Tasks  interface{} `json:"tasks"`
}
