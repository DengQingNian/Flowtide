package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// mustJSON 序列化测试请求体。
// @param t 测试上下文
// @param value 值
// @return JSON 字符串
func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return string(data)
}

// TestREST_GroupUpdate 验证组更新端点。
// @param t 测试上下文
func TestREST_GroupUpdate(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/groups", map[string]any{"id": "team", "name": "Team", "type": "assignment"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建组失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/identity/groups/team", bytes.NewBufferString(`{"name":"New Team","type":"security"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("更新组失败: %d", response.StatusCode)
	}
	var group map[string]any
	_ = json.NewDecoder(response.Body).Decode(&group)
	_ = response.Body.Close()
	if group["name"] != "New Team" || group["type"] != "security" {
		t.Fatalf("组更新结果错误: %+v", group)
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/groups/team", bytes.NewBufferString(`{"id":"other","name":"X"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("组 ID 不一致应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/groups/missing", bytes.NewBufferString(`{"name":"X"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失组更新应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_BulkUpdateTasks 验证批量任务更新。
// @param t 测试上下文
func TestREST_BulkUpdateTasks(t *testing.T) {
	server, _ := newTestServer(t)
	first := createRESTTask(t, server.URL, map[string]any{"name": "First"}, "")
	second := createRESTTask(t, server.URL, map[string]any{"name": "Second"}, "")
	firstID, _ := first["id"].(string)
	secondID, _ := second["id"].(string)
	payload := map[string]any{
		"taskIds":  []string{firstID, secondID},
		"name":     "Bulk",
		"assignee": "alice",
		"priority": 60,
	}
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks", bytes.NewBufferString(mustJSON(t, payload)))
	request.Header.Set("Content-Type", "application/json")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("批量更新失败: %d", response.StatusCode)
	}
	var page queryPageResult[map[string]any]
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) != 2 || page.Data[0]["name"] != "Bulk" || page.Data[0]["assignee"] != "alice" {
		t.Fatalf("批量更新结果错误: %+v", page)
	}
	for _, body := range []map[string]any{
		{"name": "x"},
		{"taskIds": "bad"},
		{"taskIds": []string{}, "name": "x"},
		{"taskIds": []string{firstID}, "priority": "bad"},
	} {
		request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks", bytes.NewBufferString(mustJSON(t, body)))
		request.Header.Set("Content-Type", "application/json")
		response, _ := http.DefaultClient.Do(request)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法批量请求应 400 %+v: %d", body, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks", bytes.NewBufferString(mustJSON(t, map[string]any{"taskIds": []string{"missing"}, "name": "x"})))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_UpdateExecutionVariables 验证执行变量批量更新。
// @param t 测试上下文
func TestREST_UpdateExecutionVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response, _ := http.Get(server.URL + "/service/runtime/executions?processInstanceId=" + instanceID)
	var executions struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	executionID := executions.Data[0].ID
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables", bytes.NewBufferString(`[{"name":"x","value":1},{"name":"y","value":"Y"}]`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("执行变量更新失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables")
	var variables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	if len(variables) < 2 {
		t.Fatalf("执行变量未更新: %+v", variables)
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法变量体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/missing/variables", bytes.NewBufferString(`[{"name":"x","value":1}]`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_NestedDeploymentResource 验证嵌套资源路径。
// @param t 测试上下文
func TestREST_NestedDeploymentResource(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments?resourceName=forms/nested.bpmn20.xml", "application/xml", bytes.NewBufferString(restProcess))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("嵌套资源部署失败: %d", response.StatusCode)
	}
	var deployment struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&deployment)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deployment.ID + "/resources/forms/nested.bpmn20.xml")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("嵌套资源读取失败: %d", response.StatusCode)
	}
	var resource bytes.Buffer
	_, _ = resource.ReadFrom(response.Body)
	_ = response.Body.Close()
	if !strings.Contains(resource.String(), "definitions") {
		t.Fatalf("嵌套资源内容错误: %s", resource.String())
	}
}
