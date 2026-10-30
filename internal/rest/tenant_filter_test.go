package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// deployTenantREST 在指定租户下部署测试流程。
// @param t 测试上下文
// @param server 测试服务器
// @param tenantID 部署租户
func deployTenantREST(t *testing.T, server *httptest.Server, tenantID string) {
	t.Helper()
	response, err := http.Post(server.URL+"/service/repository/deployments?deploymentName=test-"+tenantID+"&tenantId="+tenantID, "application/xml", bytes.NewBufferString(restProcess))
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("部署状态错误: %d %s", response.StatusCode, body)
	}
	_ = response.Body.Close()
}

// startTenantInstance 在指定租户下启动流程实例并返回实例 ID。
// @param t 测试上下文
// @param serverURL 服务地址
// @param tenantID 实例租户
// @param businessKey 业务键
// @return 实例 ID
func startTenantInstance(t *testing.T, serverURL, tenantID, businessKey string) string {
	t.Helper()
	response := postQuery(t, serverURL, "/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"businessKey":          businessKey,
		"name":                 "Order",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	}, tenantID)
	defer response.Body.Close()
	var instance struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("解析启动响应失败: %v", err)
	}
	if instance.ID == "" {
		t.Fatalf("启动租户实例失败: %d", response.StatusCode)
	}
	return instance.ID
}

// TestREST_TenantFilterPushdown 验证默认租户引擎下实例、历史与任务查询的 tenantId 过滤。
// @param t 测试上下文
func TestREST_TenantFilterPushdown(t *testing.T) {
	server, _ := newTestServer(t, "t1")
	deployREST(t, server)
	startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	startQueryInstance(t, server.URL, "rest", "bk2", "Order B")

	// POST 实例查询按请求体 tenantId 过滤，命中默认租户与陌生租户两侧
	response := postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"tenantId": "t1"}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 2 {
		t.Fatalf("t1 实例查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"tenantId": "t2"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("t2 实例查询应无结果: %+v", page)
	}

	// 历史实例查询按同一 tenantId 语义过滤
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"tenantId": "t1"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 2 {
		t.Fatalf("t1 历史实例查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"tenantId": "t2"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("t2 历史实例查询应无结果: %+v", page)
	}

	// POST 任务查询按请求体 tenantId 过滤
	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"tenantId": "t1"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 2 {
		t.Fatalf("t1 任务查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"tenantId": "t2"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("t2 任务查询应无结果: %+v", page)
	}

	// GET 运行任务列表按查询参数 tenantId 过滤
	response, err := http.Get(server.URL + "/service/runtime/tasks?tenantId=t1")
	if err != nil {
		t.Fatalf("GET 任务查询失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("GET t1 任务查询失败: %+v", page)
	}
	response, err = http.Get(server.URL + "/service/runtime/tasks?tenantId=t2")
	if err != nil {
		t.Fatalf("GET 任务查询失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("GET t2 任务查询应无结果: %+v", page)
	}

	// GET 历史实例列表按查询参数 tenantId 过滤
	response, err = http.Get(server.URL + "/service/history/historic-process-instances?tenantId=t1")
	if err != nil {
		t.Fatalf("GET 历史实例查询失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("GET t1 历史实例查询失败: %+v", page)
	}
	response, err = http.Get(server.URL + "/service/history/historic-process-instances?tenantId=t2")
	if err != nil {
		t.Fatalf("GET 历史实例查询失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("GET t2 历史实例查询应无结果: %+v", page)
	}
}

// TestREST_TenantFilterMultiTenant 验证无默认租户引擎下跨租户数据的 tenantId 过滤互不串扰。
// 请求租户决定上下文可见范围，tenantId 条件在可见范围内进一步收敛。
// @param t 测试上下文
func TestREST_TenantFilterMultiTenant(t *testing.T) {
	server, _ := newTestServer(t)
	deployTenantREST(t, server, "t1")
	deployTenantREST(t, server, "t2")
	firstID := startTenantInstance(t, server.URL, "t1", "bk1")
	secondID := startTenantInstance(t, server.URL, "t2", "bk2")

	// 租户条件与请求租户一致时只命中对应租户实例
	response := postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"tenantId": "t1"}, "t1")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || page.Data[0]["id"] != firstID {
		t.Fatalf("t1 实例查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{"tenantId": "t2"}, "t2")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["id"] != secondID {
		t.Fatalf("t2 实例查询失败: %+v", page)
	}
	// 空租户条件保持上下文租户语义，只返回请求租户实例
	response = postQuery(t, server.URL, "/service/query/process-instances", map[string]any{}, "t1")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("空租户实例查询失败: %+v", page)
	}

	// 历史实例查询按租户各自命中
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"tenantId": "t1"}, "t1")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["id"] != firstID {
		t.Fatalf("t1 历史实例查询失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-process-instances", map[string]any{"tenantId": "t2"}, "t2")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["id"] != secondID {
		t.Fatalf("t2 历史实例查询失败: %+v", page)
	}

	// POST 任务查询只返回对应租户任务
	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"tenantId": "t1"}, "t1")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 {
		t.Fatalf("t1 任务查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/tasks", map[string]any{"tenantId": "t2"}, "t2")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("t2 任务查询失败: %+v", page)
	}

	// GET 任务与历史列表按查询参数各自命中
	response, err := http.Get(server.URL + "/service/runtime/tasks?tenantId=t2")
	if err != nil {
		t.Fatalf("GET 任务查询失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("GET t2 任务查询失败: %+v", page)
	}
	response, err = http.Get(server.URL + "/service/history/historic-process-instances?tenantId=t1")
	if err != nil {
		t.Fatalf("GET 历史实例查询失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("GET t1 历史实例查询失败: %+v", page)
	}
}
