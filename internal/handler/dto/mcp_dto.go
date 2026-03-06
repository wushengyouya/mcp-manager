package dto

// UpsertServiceRequest 定义服务创建和更新请求。
type UpsertServiceRequest struct {
	Name          string            `json:"name" binding:"required"`
	Description   string            `json:"description"`
	TransportType string            `json:"transport_type" binding:"required,oneof=stdio streamable_http sse"`
	Command       string            `json:"command"`
	Args          []string          `json:"args"`
	Env           map[string]string `json:"env"`
	URL           string            `json:"url"`
	BearerToken   string            `json:"bearer_token"`
	CustomHeaders map[string]string `json:"custom_headers"`
	SessionMode   string            `json:"session_mode" binding:"omitempty,oneof=auto required disabled"`
	CompatMode    string            `json:"compat_mode" binding:"omitempty,oneof=off allow_legacy_sse"`
	ListenEnabled bool              `json:"listen_enabled"`
	Timeout       int               `json:"timeout"`
	Tags          []string          `json:"tags"`
}
