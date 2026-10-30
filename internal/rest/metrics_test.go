package rest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// TestHTTPMetricsRender 验证指标文本。
// @param t 测试上下文
func TestHTTPMetricsRender(t *testing.T) {
	metrics := newHTTPMetrics()
	metrics.begin()
	metrics.end()
	metrics.observe(http.MethodGet, http.StatusOK, 10*time.Millisecond)
	text := metrics.render()
	if !strings.Contains(text, `flowable_http_requests_total{method="GET",status="200"} 1`) {
		t.Fatalf("请求指标错误: %s", text)
	}
	if !strings.Contains(text, "flowable_http_request_duration_seconds_count 1") {
		t.Fatalf("耗时指标错误: %s", text)
	}
	if !strings.Contains(text, "flowable_http_requests_in_flight 0") {
		t.Fatalf("并发指标错误: %s", text)
	}
}

// TestMetricsEndpoint 验证指标端点。
// @param t 测试上下文
func TestMetricsEndpoint(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithMetrics()).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", strings.NewReader(restProcess))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/healthz")
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/metrics")
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !strings.Contains(string(body), "flowable_http_requests_total") {
		t.Fatalf("指标端点缺少指标: %s", body)
	}
	for _, metric := range []string{"flowable_deployments 1", "flowable_process_definitions 1", "flowable_process_instances_active 1", "flowable_tasks_active 1"} {
		if !strings.Contains(string(body), metric) {
			t.Fatalf("引擎指标缺少 %s: %s", metric, body)
		}
	}
}
