package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flowable-go/flowable/internal/engine"
)

// TestRateLimiter 验证令牌桶。
// @param t 测试上下文
func TestRateLimiter(t *testing.T) {
	limiter := newRateLimiter(1, 1)
	now := time.Now()
	if !limiter.allow(now) {
		t.Fatalf("首个请求应允许")
	}
	if limiter.allow(now) {
		t.Fatalf("同时间第二个请求应拒绝")
	}
	if !limiter.allow(now.Add(time.Second)) {
		t.Fatalf("补充令牌后应允许")
	}
}

// TestRateLimitMiddleware 验证 HTTP 限流。
// @param t 测试上下文
func TestRateLimitMiddleware(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithRateLimit(1, 1)).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	response, _ := http.Get(server.URL + "/service/repository/deployments")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("首个请求应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/deployments")
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("第二个请求应 429: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/healthz")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("健康检查不应限流: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
