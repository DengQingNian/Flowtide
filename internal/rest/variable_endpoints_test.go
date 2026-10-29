package rest

import (
	"bytes"
	"encoding/json"
	"github.com/flowable-go/flowable/internal/engine"
	"io"
	"net/http"
	"testing"
)

// TestREST_ExecutionVariableEndpoints 验证执行/实例变量端点。
// @param t 测试上下文
func TestREST_ExecutionVariableEndpoints(t *testing.T) {
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
	if len(executions.Data) == 0 {
		t.Fatalf("执行列表为空")
	}
	executionID := executions.Data[0].ID

	putBody, _ := json.Marshal(map[string]any{"value": "hello", "type": "string"})
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables/note", bytes.NewReader(putBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("设置执行变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables/note")
	var variable map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variable)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || variable["value"] != "hello" {
		t.Fatalf("执行变量查询失败: %d %+v", response.StatusCode, variable)
	}
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables/note/data")
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(data) != "hello" {
		t.Fatalf("执行变量数据错误: %d %s", response.StatusCode, data)
	}
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/executions/"+executionID+"/variables/note", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除执行变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables/note")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除后变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	putBody, _ = json.Marshal(map[string]any{"value": true})
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/executions/"+executionID+"/variables/flag", bytes.NewReader(putBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/executions/"+executionID+"/variables", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("清空执行变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID + "/variables")
	var executionVariables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&executionVariables)
	_ = response.Body.Close()
	if len(executionVariables) != 0 {
		t.Fatalf("清空后执行变量应为空: %+v", executionVariables)
	}

	bulkBody, _ := json.Marshal([]map[string]any{
		{"name": "a", "value": "A"},
		{"name": "b", "value": 2},
	})
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables", bytes.NewReader(bulkBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("批量设置变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables/a/data")
	data, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(data) != "A" {
		t.Fatalf("实例变量数据错误: %d %s", response.StatusCode, data)
	}
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables/b/data")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非文本变量应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("清空实例变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/variables")
	var variables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&variables)
	_ = response.Body.Close()
	if len(variables) != 0 {
		t.Fatalf("清空后实例变量应为空: %+v", variables)
	}
}

// TestREST_TaskAndHistoricVariableData 验证任务/历史变量数据端点。
// @param t 测试上下文
func TestREST_TaskAndHistoricVariableData(t *testing.T) {
	server, _ := newHistoryTestServer(t, engine.HistoryFull)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")

	response, _ := http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID + "/variables/initiator/data")
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "alice" {
		t.Fatalf("实例历史变量数据错误: %d %s", response.StatusCode, body)
	}
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID + "/variables/missing/data")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/history/historic-detail?processInstanceId=" + instanceID)
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total == 0 {
		t.Fatalf("历史明细为空")
	}
	detailID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-detail/" + detailID + "/data")
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "alice" {
		t.Fatalf("历史明细数据错误: %d %s", response.StatusCode, body)
	}

	task := createRESTTask(t, server.URL, map[string]any{"name": "Task"}, "")
	taskID, _ := task["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/variables", map[string]any{"name": "payload", "value": "hello"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/variables/payload/data")
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "hello" {
		t.Fatalf("任务变量数据错误: %d %s", response.StatusCode, body)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/variables", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("清空任务变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/variables")
	var taskVariables []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&taskVariables)
	_ = response.Body.Close()
	if len(taskVariables) != 0 {
		t.Fatalf("清空后任务变量应为空: %+v", taskVariables)
	}

	complete := postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{
		"action":         "complete",
		"localVariables": []map[string]any{{"name": "y", "value": "z"}},
	})
	_ = complete.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/" + taskID + "/variables/y/data")
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "z" {
		t.Fatalf("历史任务变量数据错误: %d %s", response.StatusCode, body)
	}
	response, _ = http.Get(server.URL + "/service/history/historic-variable-instances")
	page = decodeQueryPage[map[string]any](t, response)
	var target string
	for _, item := range page.Data {
		if item["name"] == "y" {
			target, _ = item["id"].(string)
		}
	}
	if target == "" {
		t.Fatalf("未找到任务历史变量")
	}
	response, _ = http.Get(server.URL + "/service/history/historic-variable-instances/" + target + "/data")
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "z" {
		t.Fatalf("历史变量数据错误: %d %s", response.StatusCode, body)
	}
}

// TestREST_VariableErrors 验证变量端点错误路径。
// @param t 测试上下文
func TestREST_VariableErrors(t *testing.T) {
	server, _ := newTestServer(t)
	cases := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{"GET", "/service/runtime/executions/missing/variables/x", "", http.StatusNotFound},
		{"PUT", "/service/runtime/executions/missing/variables/x", "{\"value\":1}", http.StatusNotFound},
		{"DELETE", "/service/runtime/executions/missing/variables/x", "", http.StatusNotFound},
		{"DELETE", "/service/runtime/executions/missing/variables", "", http.StatusNotFound},
		{"GET", "/service/runtime/executions/missing/variables/x/data", "", http.StatusNotFound},
		{"PUT", "/service/runtime/process-instances/missing/variables", "", http.StatusBadRequest},
		{"PUT", "/service/runtime/process-instances/missing/variables", "{", http.StatusBadRequest},
		{"PUT", "/service/runtime/process-instances/missing/variables", "[]", http.StatusBadRequest},
		{"PUT", "/service/runtime/process-instances/missing/variables", "[{\"name\":\"x\",\"value\":1}]", http.StatusNotFound},
		{"DELETE", "/service/runtime/process-instances/missing/variables", "", http.StatusNotFound},
		{"GET", "/service/runtime/process-instances/missing/variables/x/data", "", http.StatusNotFound},
		{"DELETE", "/service/runtime/tasks/missing/variables", "", http.StatusNotFound},
		{"GET", "/service/runtime/tasks/missing/variables/x/data", "", http.StatusNotFound},
		{"GET", "/service/history/historic-variable-instances/missing/data", "", http.StatusNotFound},
		{"GET", "/service/history/historic-task-instances/missing/variables/x/data", "", http.StatusNotFound},
		{"GET", "/service/history/historic-detail/missing/data", "", http.StatusNotFound},
		{"POST", "/service/history/historic-process-instances/missing/comments", "{\"message\":\"x\"}", http.StatusNotFound},
		{"DELETE", "/service/history/historic-process-instances/missing/comments/x", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		var reader *bytes.Reader
		if tc.body == "" {
			reader = bytes.NewReader(nil)
		} else {
			reader = bytes.NewReader([]byte(tc.body))
		}
		request, err := http.NewRequest(tc.method, server.URL+tc.path, reader)
		if err != nil {
			t.Fatalf("创建请求失败: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		if response.StatusCode != tc.want {
			t.Fatalf("%s %s 期望 %d 实际 %d", tc.method, tc.path, tc.want, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestREST_HistoricProcessComments 验证历史实例评论增删。
// @param t 测试上下文
func TestREST_HistoricProcessComments(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response := postJSON(t, server.URL+"/service/history/historic-process-instances/"+instanceID+"/comments", map[string]any{"message": "hi", "userId": "alice"})
	var comment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&comment)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated || comment["message"] != "hi" {
		t.Fatalf("历史评论创建失败: %d %+v", response.StatusCode, comment)
	}
	commentID, _ := comment["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID + "/comments")
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("历史评论列表错误: %+v", page)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/history/historic-process-instances/"+instanceID+"/comments/"+commentID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("历史评论删除失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID + "/comments")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("删除后评论应为空: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/missing/comments")
	_ = response.Body.Close()
}
