package api

import "time"

// AuditEvent 是 HTTP 审计事件。
type AuditEvent struct {
	ID             int64     `json:"id"`                   // 事件 ID
	Time           time.Time `json:"time"`                 // 时间
	UserID         string    `json:"userId,omitempty"`     // 用户
	TenantID       string    `json:"tenantId,omitempty"`   // 租户
	Method         string    `json:"method"`               // HTTP 方法
	Path           string    `json:"path"`                 // 路径
	Status         int       `json:"status"`               // 状态码
	DurationMillis int64     `json:"durationMillis"`       // 耗时
	RemoteAddr     string    `json:"remoteAddr,omitempty"` // 客户端地址
}
