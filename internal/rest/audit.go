package rest

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// auditIdentityKey 标记当前请求独有的审计身份容器。
type auditIdentityKey struct{}

// auditIdentity 由认证及租户中间件填充，拒绝将未验证的请求头当作身份。
type auditIdentity struct {
	user   string // 通过认证的主体
	tenant string // 通过校验的实际租户
}

// auditLog 是内存审计日志。
type auditLog struct {
	mu     sync.Mutex       // 保护事件
	events []api.AuditEvent // 事件
	next   int64            // 下一个 ID
	size   int              // 容量
}

// newAuditLog 创建审计日志。
// @param size 容量
// @return 审计日志
func newAuditLog(size int) *auditLog {
	return &auditLog{events: make([]api.AuditEvent, 0, size), size: size}
}

// record 记录事件。
// @param event 事件
// @return 事件 ID
func (a *auditLog) record(event api.AuditEvent) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next++
	event.ID = a.next
	a.events = append(a.events, event)
	if len(a.events) > a.size {
		a.events = a.events[len(a.events)-a.size:]
	}
	return event.ID
}

// list 返回事件副本。
// @return 事件列表
func (a *auditLog) list() []api.AuditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]api.AuditEvent(nil), a.events...)
}

// statusWriter 记录响应状态。
type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader 记录状态码。
// @param code 状态码
func (w *statusWriter) WriteHeader(code int) {
	// HTTP 只发送首次终态响应；临时响应允许后续终态覆盖。
	if w.status != 0 {
		return
	}
	if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Write 写入响应。
// @param data 数据
// @return 写入字节数
// @return 错误
func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}

// audit 记录 /service 请求。
// @param next 下游处理器
// @return 处理器
func (s *Server) audit(next http.Handler) http.Handler {
	if s.auditLog == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/service/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		identity := &auditIdentity{}
		r = r.WithContext(context.WithValue(r.Context(), auditIdentityKey{}, identity))
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		s.auditLog.record(api.AuditEvent{
			Time:           start,
			UserID:         identity.user,
			TenantID:       identity.tenant,
			Method:         r.Method,
			Path:           r.URL.Path,
			Status:         status,
			DurationMillis: time.Since(start).Milliseconds(),
			RemoteAddr:     r.RemoteAddr,
		})
	})
}
