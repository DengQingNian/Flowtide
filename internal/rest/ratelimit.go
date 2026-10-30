package rest

import (
	"net/http"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// rateLimiter 是令牌桶限流器。
type rateLimiter struct {
	mu     sync.Mutex // 保护令牌
	rate   float64    // 每秒补充令牌
	burst  float64    // 桶容量
	tokens float64    // 当前令牌
	last   time.Time  // 上次补充时间
}

// newRateLimiter 创建限流器。
// @param rate 每秒令牌数
// @param burst 桶容量
// @return 限流器
func newRateLimiter(rate float64, burst int) *rateLimiter {
	return &rateLimiter{rate: rate, burst: float64(burst), tokens: float64(burst), last: time.Now()}
}

// allow 判断是否允许一次请求。
// @param now 当前时间
// @return 是否允许
func (l *rateLimiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rate > 0 {
		elapsed := now.Sub(l.last).Seconds()
		l.tokens += elapsed * l.rate
		if l.tokens > l.burst {
			l.tokens = l.burst
		}
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// rateLimit 对 /service 路径执行限流。
// @param next 下游处理器
// @return 处理器
func (s *Server) rateLimit(next http.Handler) http.Handler {
	if s.limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /workflow/** 与 /service/** 同受限流（006 §6）
		if !isEngineAPIPath(r.URL.Path) || s.limiter.allow(time.Now()) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, api.NewError("REST-0429", "请求过多", api.ErrTooManyRequests))
	})
}
