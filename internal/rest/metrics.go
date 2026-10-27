package rest

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// httpMetrics 是 HTTP 指标。
type httpMetrics struct {
	mu       sync.Mutex       // 保护指标
	requests map[string]int64 // method|status -> 次数
	duration float64          // 总耗时秒
	count    int64            // 请求数
	inFlight int64            // 进行中
}

// newHTTPMetrics 创建指标。
// @return 指标
func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{requests: make(map[string]int64)}
}

// observe 记录一次请求。
// @param method 方法
// @param status 状态码
// @param elapsed 耗时
func (m *httpMetrics) observe(method string, status int, elapsed time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s|%d", method, status)
	m.requests[key]++
	m.duration += elapsed.Seconds()
	m.count++
}

// begin 增加进行中计数。
func (m *httpMetrics) begin() {
	m.mu.Lock()
	m.inFlight++
	m.mu.Unlock()
}

// end 减少进行中计数。
func (m *httpMetrics) end() {
	m.mu.Lock()
	m.inFlight--
	m.mu.Unlock()
}

// render 输出 Prometheus 文本。
// @return 文本
func (m *httpMetrics) render() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var builder strings.Builder
	builder.WriteString("# HELP flowable_http_requests_total HTTP 请求总数\n")
	builder.WriteString("# TYPE flowable_http_requests_total counter\n")
	keys := make([]string, 0, len(m.requests))
	for key := range m.requests {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, "|", 2)
		fmt.Fprintf(&builder, "flowable_http_requests_total{method=%q,status=%q} %d\n", parts[0], parts[1], m.requests[key])
	}
	builder.WriteString("# HELP flowable_http_request_duration_seconds_sum HTTP 请求总耗时\n")
	builder.WriteString("# TYPE flowable_http_request_duration_seconds_sum counter\n")
	fmt.Fprintf(&builder, "flowable_http_request_duration_seconds_sum %f\n", m.duration)
	fmt.Fprintf(&builder, "flowable_http_request_duration_seconds_count %d\n", m.count)
	fmt.Fprintf(&builder, "flowable_http_requests_in_flight %d\n", m.inFlight)
	return builder.String()
}

// metrics 记录 HTTP 指标。
// @param next 下游处理器
// @return 处理器
func (s *Server) metrics(next http.Handler) http.Handler {
	if s.httpMetrics == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		s.httpMetrics.begin()
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		s.httpMetrics.end()
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		s.httpMetrics.observe(r.Method, status, time.Since(start))
	})
}

// handleMetrics 输出指标。
// @param w 响应
// @param r 请求
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	if s.httpMetrics != nil {
		_, _ = w.Write([]byte(s.httpMetrics.render()))
	}
}
