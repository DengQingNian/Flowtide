package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestREST_VariableInstances 验证运行时变量实例端点。
// @param t 测试上下文
func TestREST_VariableInstances(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")

	response, _ := http.Get(server.URL + "/service/runtime/variable-instances?processInstanceId=" + instanceID)
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total == 0 {
		t.Fatalf("变量实例列表错误: %d %+v", response.StatusCode, page)
	}
	var variableID string
	for _, item := range page.Data {
		if item["name"] == "initiator" {
			variableID, _ = item["id"].(string)
			if item["type"] != "string" || item["scope"] != "global" {
				t.Fatalf("变量实例字段错误: %+v", item)
			}
		}
	}
	if variableID == "" {
		t.Fatalf("未找到 initiator 变量实例")
	}
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances/" + variableID + "/data")
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(data) != "alice" {
		t.Fatalf("变量实例数据错误: %d %s", response.StatusCode, data)
	}

	for _, query := range []string{"variableName=initiator", "nameLike=%25iti%25"} {
		response, _ = http.Get(server.URL + "/service/runtime/variable-instances?processInstanceId=" + instanceID + "&" + query)
		page = decodeQueryPage[map[string]any](t, response)
		if page.Total == 0 {
			t.Fatalf("变量过滤失败 %s: %+v", query, page)
		}
	}
	for _, field := range []string{"id", "name", "type", "processInstanceId", "executionId", "taskId"} {
		response, _ = http.Get(server.URL + "/service/runtime/variable-instances?sort=" + field)
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("变量排序失败 %s: %d", field, response.StatusCode)
		}
	}
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances?sort=missing")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances?order=sideways")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序方向应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/variable-instances", map[string]any{
		"processInstanceId": instanceID,
		"variableName":      "initiator",
	}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 {
		t.Fatalf("变量实例查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/variable-instances", map[string]any{"start": 0, "size": 1}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if len(page.Data) > 1 {
		t.Fatalf("变量实例分页失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/variable-instances", map[string]any{"sort": "missing"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/variable-instances", map[string]any{"start": "x"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法分页应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/query/variable-instances", map[string]any{"order": "sideways"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序方向应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	invalidRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/service/query/variable-instances", bytes.NewBufferString("{"))
	invalidRequest.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(invalidRequest)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法查询体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	numericBody, _ := json.Marshal([]map[string]any{{"name": "count", "value": 3}})
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables", bytes.NewReader(numericBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances?processInstanceId=" + instanceID + "&variableName=count")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("数值变量未找到: %+v", page)
	}
	numericID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances/" + numericID + "/data")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非文本变量数据应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	task := createRESTTask(t, server.URL, map[string]any{"name": "Task"}, "")
	taskID, _ := task["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/variables", map[string]any{"name": "payload", "value": "body"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances?taskId=" + taskID)
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["scope"] != "local" {
		t.Fatalf("任务变量实例错误: %+v", page)
	}
	taskVariableID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances/" + taskVariableID + "/data")
	data, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(data) != "body" {
		t.Fatalf("任务变量数据错误: %d %s", response.StatusCode, data)
	}
	response, _ = http.Get(server.URL + "/service/runtime/variable-instances/missing/data")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失变量实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
