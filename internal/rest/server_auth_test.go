package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
)

// TestBasicAuth 验证 Basic Auth。
// @param t 测试上下文
func TestBasicAuth(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithBasicAuth(map[string]string{"alice": "secret"})).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	response, err := http.Get(server.URL + "/service/management/engine")
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未认证应 401: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("alice", "secret")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("认证后应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.SetBasicAuth("alice", "bad")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误密码应 401: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/healthz")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("健康检查应公开: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestBearerAuth 验证 Bearer Token。
// @param t 测试上下文
func TestBearerAuth(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithBearerToken("token-1")).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.Header.Set("Authorization", "Bearer token-1")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Bearer 认证后应 200: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/management/engine", nil)
	request.Header.Set("Authorization", "Bearer bad")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("错误令牌应 401: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
