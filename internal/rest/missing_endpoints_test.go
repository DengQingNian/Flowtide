package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
)

const restDeadLetterXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="deadLetter" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="svc"/><serviceTask id="svc" flowable:delegateExpression="${failing}" flowable:asyncBefore="true"/><sequenceFlow id="f2" sourceRef="svc" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_BatchDocumentsAndDelete 验证批次文档、分片文档与批次删除。
// @param t 测试上下文
func TestREST_BatchDocumentsAndDelete(t *testing.T) {
	fixture := newBatchFixture(t)
	instanceID := fixture.startInstance(t)
	response := batchMigrate(t, fixture, fixture.v2, []string{instanceID}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("批量迁移失败: %d", response.StatusCode)
	}
	var batch struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&batch)
	_ = response.Body.Close()

	response, _ = http.Get(fixture.httpServer.URL + "/service/management/batches/" + batch.ID + "/batch-document")
	document, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("批次文档响应错误: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if !strings.Contains(string(document), batch.ID) || !strings.Contains(string(document), fixture.v2) {
		t.Fatalf("批次文档内容错误: %s", document)
	}

	response, _ = http.Get(fixture.httpServer.URL + "/service/management/batches/" + batch.ID + "/batch-parts")
	var parts struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&parts)
	_ = response.Body.Close()
	if len(parts.Data) != 1 {
		t.Fatalf("批次分片错误: %+v", parts)
	}
	response, _ = http.Get(fixture.httpServer.URL + "/service/management/batch-parts/" + parts.Data[0].ID + "/batch-part-document")
	partDocument, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(partDocument), parts.Data[0].ID) {
		t.Fatalf("分片文档错误: %d %s", response.StatusCode, partDocument)
	}

	for _, path := range []string{
		"/service/management/batches/missing/batch-document",
		"/service/management/batch-parts/missing/batch-part-document",
	} {
		response, _ = http.Get(fixture.httpServer.URL + path)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s 应 404: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}

	request, _ := http.NewRequest(http.MethodDelete, fixture.httpServer.URL+"/service/management/batches/"+batch.ID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除批次失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	for _, path := range []string{
		"/service/management/batches/" + batch.ID,
		"/service/management/batches/" + batch.ID + "/batch-document",
		"/service/management/batches/" + batch.ID + "/batch-parts",
		"/service/management/batch-parts/" + parts.Data[0].ID + "/batch-part-document",
	} {
		response, _ = http.Get(fixture.httpServer.URL + path)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("删除后 %s 应 404: %d", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	request, _ = http.NewRequest(http.MethodDelete, fixture.httpServer.URL+"/service/management/batches/"+batch.ID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除批次应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// 跨租户不可见时删除与读取文档都应 404
	tenantRequest, _ := http.NewRequest(http.MethodDelete, fixture.httpServer.URL+"/service/management/batches/missing", nil)
	tenantRequest.Header.Set("X-Tenant-Id", "other")
	response, _ = http.DefaultClient.Do(tenantRequest)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户删除批次应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TimerJobActionsAndDelete 验证定时器作业 move、reschedule 与删除。
// @param t 测试上下文
func TestREST_TimerJobActionsAndDelete(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restTimerJobXML))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs")
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("定时作业列表错误: %+v", page)
	}
	jobID, _ := page.Data[0]["id"].(string)

	response = postJSON(t, server.URL+"/service/management/timer-jobs/"+jobID, map[string]any{"action": "move"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("move 定时器作业失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs/" + jobID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("move 后定时器详情应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs/" + jobID)
	var moved map[string]any
	_ = json.NewDecoder(response.Body).Decode(&moved)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || moved["executable"] != true {
		t.Fatalf("move 后可执行作业错误: %d %+v", response.StatusCode, moved)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/management/timer-jobs/"+jobID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("move 后跨桶删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/management/timer-jobs/"+jobID, map[string]any{"action": "move"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("move 已移出作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// 重新部署产生新的定时器作业，验证 reschedule
	response, _ = http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restTimerJobXML))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("第二次定时作业列表错误: %+v", page)
	}
	secondID, _ := page.Data[0]["id"].(string)
	response = postJSON(t, server.URL+"/service/management/timer-jobs/"+secondID, map[string]any{"action": "reschedule", "dueDate": "2030-01-02T03:04:05Z"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("reschedule 失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/timer-jobs/" + secondID)
	var rescheduled map[string]any
	_ = json.NewDecoder(response.Body).Decode(&rescheduled)
	_ = response.Body.Close()
	if dueDate, _ := rescheduled["dueDate"].(string); !strings.HasPrefix(dueDate, "2030-01-02T03:04:05") {
		t.Fatalf("reschedule 到期时间错误: %+v", rescheduled)
	}
	for _, payload := range []map[string]any{
		{"action": "reschedule"},
		{"action": "reschedule", "dueDate": "not-a-date"},
		{"action": "unknown"},
	} {
		response = postJSON(t, server.URL+"/service/management/timer-jobs/"+secondID, payload)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法动作应 400 %+v: %d", payload, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	response = postJSON(t, server.URL+"/service/management/timer-jobs/missing", map[string]any{"action": "move"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定时器作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/timer-jobs/"+secondID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除定时器作业失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_DeadLetterJobActions 验证死信作业 move、删除与不支持动作。
// @param t 测试上下文
func TestREST_DeadLetterJobActions(t *testing.T) {
	server, app := newTestServer(t)
	app.RegisterDelegate("failing", func(ctx context.Context, execution *engine.Execution) error {
		return errors.New("始终失败")
	})
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restDeadLetterXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "deadLetter"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs")
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("可执行作业列表错误: %+v", page)
	}
	jobID, _ := page.Data[0]["id"].(string)
	if err := app.ManagementService().SetJobRetries(context.Background(), jobID, 1); err != nil {
		t.Fatalf("设置重试失败: %v", err)
	}
	if _, err := app.ManagementService().RunDueJobs(context.Background()); err != nil {
		t.Fatalf("执行作业失败: %v", err)
	}
	response, _ = http.Get(server.URL + "/service/management/deadletter-jobs")
	deadPage := decodeQueryPage[map[string]any](t, response)
	if deadPage.Total != 1 {
		t.Fatalf("死信作业列表错误: %+v", deadPage)
	}
	deadID, _ := deadPage.Data[0]["id"].(string)

	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/management/jobs/"+deadID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("死信作业不应从可执行桶删除: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/management/deadletter-jobs/"+deadID, map[string]any{"action": "move"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("move 死信作业失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs/" + deadID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("move 后作业应可执行: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/deadletter-jobs/" + deadID)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("move 后死信详情应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/deadletter-jobs/"+deadID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("move 后删除死信应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_DeadLetterUnsupportedActions 验证死信不支持动作返回 400。
// @param t 测试上下文
func TestREST_DeadLetterUnsupportedActions(t *testing.T) {
	server, app := newTestServer(t)
	app.RegisterDelegate("failing", func(ctx context.Context, execution *engine.Execution) error {
		return errors.New("始终失败")
	})
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restDeadLetterXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "deadLetter"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/jobs")
	page := decodeQueryPage[map[string]any](t, response)
	jobID, _ := page.Data[0]["id"].(string)
	_ = app.ManagementService().SetJobRetries(context.Background(), jobID, 1)
	_, _ = app.ManagementService().RunDueJobs(context.Background())
	response, _ = http.Get(server.URL + "/service/management/deadletter-jobs")
	deadPage := decodeQueryPage[map[string]any](t, response)
	deadID, _ := deadPage.Data[0]["id"].(string)

	for _, payload := range []map[string]any{{"action": "moveToHistoryJob"}, {"action": "unknown"}} {
		response = postJSON(t, server.URL+"/service/management/deadletter-jobs/"+deadID, payload)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("不支持动作应 400 %+v: %d", payload, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/management/deadletter-jobs/"+deadID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除死信作业失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/management/deadletter-jobs/"+deadID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除死信应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_InstanceUserIdentityLink 验证字面量 users 身份链接读写。
// @param t 测试上下文
func TestREST_InstanceUserIdentityLink(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceID := startQueryInstance(t, server.URL, "rest", "bk-link", "Link")
	response := postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/identitylinks", map[string]any{"type": "participant", "userId": "alice"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加实例身份链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	path := server.URL + "/service/runtime/process-instances/" + instanceID + "/identitylinks/users/alice/participant"
	response, _ = http.Get(path)
	var link map[string]any
	_ = json.NewDecoder(response.Body).Decode(&link)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || link["user"] != "alice" || link["type"] != "participant" {
		t.Fatalf("RestIdentityLink 内容错误: %d %+v", response.StatusCode, link)
	}
	if linkURL, _ := link["url"].(string); !strings.Contains(linkURL, "/identitylinks/users/alice/participant") {
		t.Fatalf("RestIdentityLink url 错误: %+v", link)
	}
	response, _ = http.Get(server.URL + "/service/runtime/process-instances/" + instanceID + "/identitylinks/users/missing/participant")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失身份链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, path, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除用户身份链接失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, path, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除身份链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}
