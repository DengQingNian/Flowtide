package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// decodeTask 解析任务响应。
// @param t 测试上下文
// @param response 响应
// @return 任务
func decodeTask(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	var task map[string]any
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatalf("解析任务失败: %v", err)
	}
	_ = response.Body.Close()
	return task
}

// TestREST_StandaloneTaskLifecycle 验证独立任务 REST 生命周期。
// @param t 测试上下文
func TestREST_StandaloneTaskLifecycle(t *testing.T) {
	server, _ := newTestServer(t)
	response := postQuery(t, server.URL, "/service/runtime/tasks", map[string]any{
		"name":        "Review",
		"description": "desc",
		"assignee":    "alice",
		"priority":    70,
		"dueDate":     "2026-11-01T00:00:00Z",
		"category":    "cat",
		"formKey":     "form",
	}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建任务失败: %d", response.StatusCode)
	}
	task := decodeTask(t, response)
	taskID, _ := task["id"].(string)
	if taskID == "" || task["priority"].(float64) != 70 || task["dueDate"] == nil {
		t.Fatalf("任务字段错误: %+v", task)
	}

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("查询任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	updateBody, _ := json.Marshal(map[string]any{
		"name":        "Review v2",
		"assignee":    "carol",
		"dueDate":     nil,
		"unknown":     "ignored",
		"description": "new",
	})
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks/"+taskID, bytes.NewReader(updateBody))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("更新任务失败: %d", response.StatusCode)
	}
	updated := decodeTask(t, response)
	if updated["name"] != "Review v2" || updated["assignee"] != "carol" || updated["dueDate"] != nil {
		t.Fatalf("更新结果错误: %+v", updated)
	}

	response = postQuery(t, server.URL, "/service/runtime/tasks", map[string]any{"name": "Child", "parentTaskId": taskID}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建子任务失败: %d", response.StatusCode)
	}
	child := decodeTask(t, response)
	childID, _ := child["id"].(string)
	if child["parentTaskId"] != taskID {
		t.Fatalf("子任务父 ID 错误: %+v", child)
	}

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/subtasks")
	var subtasks []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&subtasks)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(subtasks) != 1 || subtasks[0]["id"] != childID {
		t.Fatalf("子任务查询错误: %d %+v", response.StatusCode, subtasks)
	}

	response = postQuery(t, server.URL, "/service/runtime/tasks/"+taskID, map[string]any{
		"action":    "complete",
		"userId":    "carol",
		"variables": []map[string]any{{"name": "approved", "value": true}},
	}, "")
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		t.Fatalf("完成任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("完成后任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+childID+"?cascadeHistory=true", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除子任务失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + childID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除后任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TaskLifecycleErrors 验证任务生命周期错误与权限。
// @param t 测试上下文
func TestREST_TaskLifecycleErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response := postQuery(t, server.URL, "/service/runtime/tasks", map[string]any{"name": ""}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空名称应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/runtime/tasks", map[string]any{"name": "x", "dueDate": "bad"}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法到期时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/runtime/tasks", map[string]any{"name": "x", "delegationState": "PENDING"}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("未知字段应忽略: %d", response.StatusCode)
	}
	task := decodeTask(t, response)
	taskID, _ := task["id"].(string)

	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks/"+taskID, bytes.NewBufferString(`{"priority":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法优先级应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks/missing", bytes.NewBufferString(`{"name":"x"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务更新应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/missing/subtasks")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失父任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	deployREST(t, server)
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) == 0 {
		t.Fatalf("流程任务为空")
	}
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+page.Data[0].ID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("删除流程任务应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/runtime/tasks", map[string]any{"name": "TenantTask"}, "tenant-a")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建租户任务失败: %d", response.StatusCode)
	}
	tenantTask := decodeTask(t, response)
	tenantTaskID, _ := tenantTask["id"].(string)
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/runtime/tasks/"+tenantTaskID, nil)
	request.Header.Set("X-Tenant-Id", "tenant-b")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户查询应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
