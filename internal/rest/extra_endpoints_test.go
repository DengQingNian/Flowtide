package rest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
)

const restMessageCatchXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><message id="m" name="approvedMessage"/><process id="msgCatch" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="wait"/><intermediateCatchEvent id="wait"><messageEventDefinition messageRef="m"/></intermediateCatchEvent><sequenceFlow id="f2" sourceRef="wait" targetRef="e"/><endEvent id="e"/></process></definitions>`

// createRESTTask 通过 REST 创建独立任务。
// @param t 测试上下文
// @param server 测试服务器
// @param body 请求体
// @param tenant 租户
// @return 任务
func createRESTTask(t *testing.T, serverURL string, body map[string]any, tenant string) map[string]any {
	t.Helper()
	response := postQuery(t, serverURL, "/service/runtime/tasks", body, tenant)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建任务失败: %d", response.StatusCode)
	}
	defer response.Body.Close()
	var task map[string]any
	if err := json.NewDecoder(response.Body).Decode(&task); err != nil {
		t.Fatalf("解析任务失败: %v", err)
	}
	return task
}

// TestREST_EventSubscriptions 验证事件订阅端点。
// @param t 测试上下文
func TestREST_EventSubscriptions(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restMessageCatchXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "msgCatch"})
	var instance struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	if instance.ID == "" {
		t.Fatalf("启动实例失败")
	}

	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?processInstanceId=" + instance.ID)
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 {
		t.Fatalf("订阅列表错误: %d %+v", response.StatusCode, page)
	}
	subscription := page.Data[0]
	if subscription["eventType"] != "message" || subscription["eventName"] != "approvedMessage" || subscription["url"] == "" {
		t.Fatalf("订阅字段错误: %+v", subscription)
	}
	subscriptionID, _ := subscription["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions/" + subscriptionID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("单条订阅查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?eventName=missing")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("事件名过滤失败: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?withoutProcessInstanceId=true")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("无实例订阅过滤失败: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?sort=created&order=desc")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Order != "desc" {
		t.Fatalf("订阅排序失败: %+v", page)
	}
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?sort=missing")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序字段应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?order=sideways")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法排序方向应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?createdAfter=bad")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?createdBefore=bad")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法上界时间应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	for _, query := range []string{
		"createdAfter=2020-01-01T00:00:00Z",
		"createdBefore=2040-01-01T00:00:00Z",
		"id=" + subscriptionID,
		"activityId=wait",
		"executionId=" + subscription["executionId"].(string),
		"processDefinitionId=" + subscription["processDefinitionId"].(string),
		"withoutProcessDefinitionId=true",
		"withoutTenantId=true",
	} {
		response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?" + query)
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("订阅过滤失败 %s: %d", query, response.StatusCode)
		}
	}
	for _, field := range []string{"id", "created", "executionId", "processInstanceId", "processDefinitionId", "tenantId"} {
		response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions?sort=" + field)
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK || page.Total != 1 {
			t.Fatalf("订阅排序失败 %s: %d %+v", field, response.StatusCode, page)
		}
	}
	response, _ = http.Get(server.URL + "/service/runtime/event-subscriptions/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失订阅应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TaskCommentAttachmentDetails 验证单条评论/附件与参与者分组。
// @param t 测试上下文
func TestREST_TaskCommentAttachmentDetails(t *testing.T) {
	server, _ := newTestServer(t)
	task := createRESTTask(t, server.URL, map[string]any{"name": "Task", "assignee": "alice"}, "")
	taskID, _ := task["id"].(string)
	other := createRESTTask(t, server.URL, map[string]any{"name": "Other"}, "")
	otherID, _ := other["id"].(string)

	response := postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/comments", map[string]any{"message": "hello", "userId": "alice"})
	var comment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&comment)
	_ = response.Body.Close()
	commentID, _ := comment["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/comments/" + commentID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("单条评论查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + otherID + "/comments/" + commentID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("评论任务不匹配应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/comments/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失评论应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]any{"name": "a.txt", "url": "http://example.com"})
	var attachment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&attachment)
	_ = response.Body.Close()
	attachmentID, _ := attachment["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/attachments/" + attachmentID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("单个附件查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + otherID + "/attachments/" + attachmentID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("附件任务不匹配应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/attachments/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失附件应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks", map[string]any{"type": "candidate", "userId": "bob"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks", map[string]any{"type": "candidate", "groupId": "g1"})
	_ = response.Body.Close()

	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks/users")
	var users []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&users)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(users) != 2 {
		t.Fatalf("用户参与者查询失败: %d %+v", response.StatusCode, users)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks/groups")
	var groups []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&groups)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(groups) != 1 {
		t.Fatalf("组参与者查询失败: %d %+v", response.StatusCode, groups)
	}
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks/teams")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法参与者类型应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/missing/identitylinks/users")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务参与者应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_BulkDeleteInstances 验证批量删除实例。
// @param t 测试上下文
func TestREST_BulkDeleteInstances(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	first := startQueryInstance(t, server.URL, "rest", "bk1", "One")
	second := startQueryInstance(t, server.URL, "rest", "bk2", "Two")
	response := postQuery(t, server.URL, "/service/runtime/process-instances/delete", map[string]any{
		"processInstanceIds": []string{first, second},
		"deleteReason":       "bulk",
	}, "")
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("批量删除实例失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/process-instances")
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("批量删除后实例应为空: %+v", page)
	}

	response, _ = http.Get(server.URL + "/service/history/historic-process-instances")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 2 {
		t.Fatalf("删除后历史应保留: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/history/historic-process-instances/delete", map[string]any{
		"processInstanceIds": []string{first, second},
	}, "")
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("批量删除历史失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("批量删除后历史应为空: %+v", page)
	}

	response = postQuery(t, server.URL, "/service/runtime/process-instances/delete", map[string]any{}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空 ID 列表应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/history/historic-process-instances/delete", map[string]any{}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空历史 ID 列表应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/runtime/process-instances/delete", map[string]any{
		"processInstanceIds": []string{"missing"},
	}, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例批量删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postQuery(t, server.URL, "/service/history/historic-process-instances/delete", map[string]any{
		"processInstanceIds": []string{"missing"},
	}, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史批量删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestEventSubscriptionSortKey 验证订阅排序字段。
// @param t 测试上下文
func TestEventSubscriptionSortKey(t *testing.T) {
	item := api.EventSubscription{ID: "s", Created: time.Now(), ExecutionID: "e", ProcessInstanceID: "i", ProcessDefinitionID: "d", TenantID: "t"}
	for _, field := range []string{"id", "created", "executionId", "processInstanceId", "processDefinitionId", "tenantId"} {
		key, ok := eventSubscriptionSortKey(field)
		if !ok {
			t.Fatalf("订阅排序字段缺失: %s", field)
		}
		_ = key(item)
	}
	if _, ok := eventSubscriptionSortKey("missing"); ok {
		t.Fatalf("未知订阅排序字段应不支持")
	}
}

// TestREST_DefinitionResourceData 验证定义原始资源下载。
// @param t 测试上下文
func TestREST_DefinitionResourceData(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response, _ := http.Get(server.URL + "/service/repository/process-definitions")
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if len(page.Data) == 0 {
		t.Fatalf("流程定义为空")
	}
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/" + page.Data[0].ID + "/resourcedata")
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "<definitions") {
		t.Fatalf("资源下载失败: %d %s", response.StatusCode, body)
	}
	if !strings.Contains(response.Header.Get("Content-Disposition"), "attachment") ||
		response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("下载响应头错误: %v", response.Header)
	}
	response, _ = http.Get(server.URL + "/service/repository/process-definitions/missing/resourcedata")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定义应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
