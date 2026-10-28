package rest

import (
	"net/http"
	"testing"
)

// TestREST_EvaluateConditionsRoute 验证条件求值路由。
// @param t 测试上下文
func TestREST_EvaluateConditionsRoute(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response := postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/evaluate-conditions", map[string]any{})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("条件求值失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/evaluate-conditions", map[string]any{})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
