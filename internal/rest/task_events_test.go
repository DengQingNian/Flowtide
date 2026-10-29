package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// TestREST_TaskEvents 验证任务事件端点。
// @param t 测试上下文
func TestREST_TaskEvents(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response, _ := http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	var tasks struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) == 0 {
		t.Fatalf("任务为空")
	}
	taskID := tasks.Data[0].ID

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/events")
	var events []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&events)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(events) == 0 || events[0]["action"] != "create" {
		t.Fatalf("任务事件列表错误: %d %+v", response.StatusCode, events)
	}
	eventID := int64(events[0]["id"].(float64))
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/events/" + strconv.FormatInt(eventID, 10))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("任务事件查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/events/abc")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法事件 ID 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/events/"+strconv.FormatInt(eventID, 10), nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除任务事件失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/events/" + strconv.FormatInt(eventID, 10))
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除后事件应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/missing/events")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务事件应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_HistoricTaskForm 验证历史任务表单端点。
// @param t 测试上下文
func TestREST_HistoricTaskForm(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restFormXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "restForm",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instance.ID)
	var tasks struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) == 0 {
		t.Fatalf("任务为空")
	}
	taskID := tasks.Data[0].ID
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/" + taskID + "/form")
	var form map[string]any
	_ = json.NewDecoder(response.Body).Decode(&form)
	_ = response.Body.Close()
	// 原版流程引擎未内置表单引擎，历史表单按原版返回 400。
	if response.StatusCode != http.StatusBadRequest || form["exception"] != "Form engine is not initialized" {
		t.Fatalf("历史任务表单错误: %d %+v", response.StatusCode, form)
	}
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "complete"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/" + taskID + "/form")
	// 原版引擎未内置表单引擎，完成后历史表单同样 400。
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("完成后历史表单应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/missing/form")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk2", "NoForm")
	response, _ = http.Get(server.URL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/" + tasks.Data[0].ID + "/form")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("无表单任务应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_HistoricProcessCommentDetail 验证单条历史评论。
// @param t 测试上下文
func TestREST_HistoricProcessCommentDetail(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk1", "Order")
	response := postJSON(t, server.URL+"/service/history/historic-process-instances/"+instanceID+"/comments", map[string]any{"message": "hi", "userId": "alice"})
	var comment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&comment)
	_ = response.Body.Close()
	commentID, _ := comment["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID + "/comments/" + commentID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("单条历史评论查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/missing/comments/" + commentID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例评论应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID + "/comments/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失评论应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
