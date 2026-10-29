package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/flowable-go/flowable/internal/engine"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestREST_HistoricQueries 验证历史端点。
// @param t 测试上下文
func TestREST_HistoricQueries(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)

	response, err := http.Get(server.URL + "/service/history/historic-process-instances")
	if err != nil {
		t.Fatalf("历史流程查询失败: %v", err)
	}
	var processPage struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&processPage)
	_ = response.Body.Close()
	if processPage.Total != 1 {
		t.Fatalf("期望 1 个历史流程，实际 %d", processPage.Total)
	}

	response, err = http.Get(server.URL + "/service/history/historic-task-instances?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("历史任务查询失败: %v", err)
	}
	var taskPage struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&taskPage)
	_ = response.Body.Close()
	if taskPage.Total != 1 {
		t.Fatalf("期望 1 个历史任务，实际 %d", taskPage.Total)
	}
}

// TestREST_MoreErrors 验证更多错误响应。
// @param t 测试上下文
func TestREST_MoreErrors(t *testing.T) {
	server, _ := newTestServer(t)
	requests := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{"GET", "/service/runtime/process-instances/missing/variables", "", http.StatusNotFound},
		{"DELETE", "/service/runtime/process-instances/missing", "", http.StatusNotFound},
		{"GET", "/service/runtime/tasks/missing", "", http.StatusNotFound},
		{"POST", "/service/runtime/tasks/missing", `{"action":"complete"}`, http.StatusNotFound},
		{"POST", "/service/runtime/tasks/missing", `{"action":"unknown"}`, http.StatusBadRequest},
		{"POST", "/service/runtime/process-instances", `{"processDefinitionKey":"missing"}`, http.StatusNotFound},
		{"GET", "/service/repository/deployments/missing", "", http.StatusNotFound},
		{"GET", "/service/repository/deployments/missing/resources", "", http.StatusNotFound},
		{"GET", "/service/repository/deployments/missing/resources/x", "", http.StatusNotFound},
		{"DELETE", "/service/repository/deployments/missing", "", http.StatusNotFound},
	}
	for _, tc := range requests {
		request, err := http.NewRequest(tc.method, server.URL+tc.path, bytes.NewBufferString(tc.body))
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		if response.StatusCode != tc.want {
			t.Fatalf("%s %s 期望 %d，实际 %d", tc.method, tc.path, tc.want, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

// TestStatusFor 验证状态码映射。
// @param t 测试上下文
func TestStatusFor(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{api.ErrBadRequest, http.StatusBadRequest},
		{api.ErrForbidden, http.StatusForbidden},
		{api.ErrNotFound, http.StatusNotFound},
		{api.ErrConflict, http.StatusConflict},
		{api.ErrEngineClosed, http.StatusServiceUnavailable},
		{api.NewError("BPMN-1", "x", nil), http.StatusBadRequest},
		{api.NewError("REST-1", "x", nil), http.StatusBadRequest},
		{errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := statusFor(tc.err); got != tc.want {
			t.Fatalf("statusFor(%v) 期望 %d，实际 %d", tc.err, tc.want, got)
		}
	}
}

// TestReflectSliceDefault 验证未知切片类型。
// @param t 测试上下文
func TestReflectSliceDefault(t *testing.T) {
	if got := reflectSlice(1); len(got) != 0 {
		t.Fatalf("未知类型应返回空切片")
	}
}

// TestRequestUserID 验证操作人解析。
// @param t 测试上下文
func TestRequestUserID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := requestUserID(request); got != "" {
		t.Fatalf("无认证应返回空")
	}
	request.Header.Set("X-User-Id", "alice")
	if got := requestUserID(request); got != "alice" {
		t.Fatalf("请求头解析错误: %s", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("bob", "secret")
	if got := requestUserID(request); got != "bob" {
		t.Fatalf("Basic Auth 解析错误: %s", got)
	}
}

// TestParseVariablesEdges 验证变量解析边界。
// @param t 测试上下文
func TestParseVariablesEdges(t *testing.T) {
	if values, err := parseVariables(nil); err != nil || values != nil {
		t.Fatalf("nil 变量应返回空")
	}
	if values, err := parseVariables(json.RawMessage("null")); err != nil || values != nil {
		t.Fatalf("null 变量应返回空")
	}
	if _, err := parseVariables(json.RawMessage("{")); err == nil {
		t.Fatalf("非法变量应报错")
	}
	values, err := parseVariables(json.RawMessage(`{"b":2,"a":1}`))
	if err != nil || len(values) != 2 || values[0].Name != "a" {
		t.Fatalf("对象变量解析错误: %v %+v", err, values)
	}
}

// TestDecodeJSONUnknownField 验证未知字段。
// @param t 测试上下文
func TestDecodeJSONUnknownField(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"unknown":1}`))
	var payload startPayload
	if err := decodeJSON(request, &payload); err == nil {
		t.Fatalf("未知字段应报错")
	}
}

// TestPageParams 验证分页参数。
// @param t 测试上下文
func TestPageParams(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/?start=-1&size=2000", nil)
	start, size := pageParams(request)
	if start != 0 || size != 1000 {
		t.Fatalf("分页参数规范化错误: %d %d", start, size)
	}
}

// TestServerString 验证服务描述。
// @param t 测试上下文
func TestServerString(t *testing.T) {
	server := New(nil)
	if server.String() == "" {
		t.Fatalf("服务描述为空")
	}
}

// TestREST_RunJobs 验证定时器执行端点。
// @param t 测试上下文
func TestREST_RunJobs(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/management/jobs/run", map[string]any{})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("执行作业失败: %d", response.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(response.Body).Decode(&body)
	_ = response.Body.Close()
	if body["executed"] != float64(0) {
		t.Fatalf("期望执行 0 个作业，实际 %+v", body)
	}
}

const restMessageXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <message id="rm" name="restMessage"/>
  <signal id="rs" name="restSignal"/>
  <process id="restMessageProcess" isExecutable="true">
    <startEvent id="s"><messageEventDefinition messageRef="rm"/></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="§{assignee}"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestREST_MessagesAndSignals 验证消息与信号端点。
// @param t 测试上下文
func TestREST_MessagesAndSignals(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restMessageXML))
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("部署失败: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/messages", map[string]any{
		"messageName": "restMessage",
		"variables":   []map[string]any{{"name": "assignee", "value": "alice"}},
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("消息投递失败: %d", response.StatusCode)
	}
	var result map[string]any
	_ = json.NewDecoder(response.Body).Decode(&result)
	_ = response.Body.Close()
	if result["triggered"] != float64(1) {
		t.Fatalf("期望触发 1 个实例: %+v", result)
	}
	response = postJSON(t, server.URL+"/service/runtime/signals", map[string]any{"signalName": "unknown"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("信号端点失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, err = http.Post(server.URL+"/service/runtime/messages", "application/json", bytes.NewBufferString("{"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法消息请求应返回 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_Suspension 验证挂起与激活端点。
// @param t 测试上下文
func TestREST_Suspension(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response, _ := http.Get(server.URL + "/service/repository/process-definitions")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	definitionID, _ := page.Data[0]["id"].(string)

	putJSON := func(path string, body any) *http.Response {
		data, _ := json.Marshal(body)
		request, _ := http.NewRequest(http.MethodPut, server.URL+path, bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("PUT 请求失败: %v", err)
		}
		return response
	}

	response = putJSON("/service/repository/process-definitions/"+definitionID, map[string]any{"action": "suspend"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("挂起定义失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "rest"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("挂起定义后启动应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = putJSON("/service/repository/process-definitions/"+definitionID, map[string]any{"action": "activate"})
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	taskID, _ := tasks.Data[0]["id"].(string)

	response = putJSON("/service/runtime/process-instances/"+instanceID, map[string]any{"action": "suspend"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("挂起实例失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "complete", "userId": "alice"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("挂起实例任务应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = putJSON("/service/runtime/process-instances/"+instanceID, map[string]any{"action": "activate"})
	_ = response.Body.Close()
	response = putJSON("/service/runtime/process-instances/"+instanceID, map[string]any{"action": "invalid"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法实例动作应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = putJSON("/service/repository/process-definitions/"+definitionID, map[string]any{"action": "invalid"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法定义动作应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ActionErrors 验证动作错误分支。
// @param t 测试上下文
func TestREST_ActionErrors(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "complete", "userId": "bob"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("错误处理人应 403: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID, map[string]any{"action": "delegate", "userId": ""})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空被委托人应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/repository/process-definitions/missing", bytes.NewBufferString(`{"action":"suspend"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失定义应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/runtime/process-instances/missing", bytes.NewBufferString(`{"action":"suspend"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/signals", "not-an-object")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法信号体应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

const restMigrateV1 = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="restMigrate" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="oldTask"/><userTask id="oldTask"/><sequenceFlow id="f2" sourceRef="oldTask" targetRef="e"/><endEvent id="e"/></process></definitions>`
const restMigrateV2 = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="restMigrate" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="newTask"/><userTask id="newTask"/><sequenceFlow id="f2" sourceRef="newTask" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_Migrate 验证迁移端点。
// @param t 测试上下文
func TestREST_Migrate(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restMigrateV1))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	v1, _ := page.Data[0]["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionId": v1})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)

	response, _ = http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restMigrateV2))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	v2, _ := page.Data[len(page.Data)-1]["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/migrate", map[string]any{
		"targetProcessDefinitionId": v2,
		"activityMappings":          map[string]string{"oldTask": "newTask"},
	})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("迁移失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if len(tasks.Data) != 1 || tasks.Data[0]["taskDefinitionKey"] != "newTask" {
		t.Fatalf("迁移后任务错误: %+v", tasks.Data)
	}
}

// TestREST_Deployments 验证部署端点。
// @param t 测试上下文
func TestREST_Deployments(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response, err := http.Get(server.URL + "/service/repository/deployments")
	if err != nil {
		t.Fatalf("部署列表失败: %v", err)
	}
	var page struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("期望 1 个部署，实际 %d", page.Total)
	}
	deploymentID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deploymentID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("部署详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deploymentID + "/resources")
	var resources []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&resources)
	_ = response.Body.Close()
	if len(resources) != 1 {
		t.Fatalf("资源列表错误: %+v", resources)
	}
	name, _ := resources[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/repository/deployments/" + deploymentID + "/resources/" + name)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("资源元数据失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/repository/deployments/"+deploymentID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("未级联删除应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/repository/deployments/"+deploymentID+"?cascade=true", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("级联删除失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_HistoricVariables 验证历史变量端点。
// @param t 测试上下文
func TestREST_HistoricVariables(t *testing.T) {
	server, _ := newHistoryTestServer(t, engine.HistoryFull)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)
	response, err := http.Get(server.URL + "/service/history/historic-variable-instances?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("历史变量查询失败: %v", err)
	}
	var page struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total != 1 {
		t.Fatalf("期望 1 个历史变量，实际 %d", page.Total)
	}
	response, _ = http.Get(server.URL + "/service/history/historic-detail?processInstanceId=" + instanceID)
	var detailPage struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&detailPage)
	_ = response.Body.Close()
	if detailPage.Total == 0 {
		t.Fatalf("历史明细为空")
	}
}

// TestREST_Identity 验证身份端点。
// @param t 测试上下文
func TestREST_Identity(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice", "displayName": "Alice"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建用户失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/users")
	var users struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&users)
	_ = response.Body.Close()
	if len(users.Data) != 1 {
		t.Fatalf("用户列表错误")
	}
	response, _ = http.Get(server.URL + "/service/identity/users/alice")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("查询用户失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups", map[string]any{"id": "dev", "name": "Dev"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建组失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups/dev/members", map[string]any{"userId": "alice"})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("添加成员失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/groups/dev/members")
	var members []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&members)
	_ = response.Body.Close()
	if len(members) != 1 {
		t.Fatalf("成员列表错误: %+v", members)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/identity/groups/dev/members/alice", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("移除成员失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/identity/users/alice", nil)
	response, _ = http.DefaultClient.Do(request)
	_ = response.Body.Close()
	response, err := http.Get(server.URL + "/service/identity/users/alice")
	if err != nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("删除用户后应 404: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_IdentityErrors 验证身份错误与组查询。
// @param t 测试上下文
func TestREST_IdentityErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("重复用户应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/users", map[string]any{"displayName": "x"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空用户 ID 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups", map[string]any{"id": "dev"})
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups", map[string]any{"id": "dev"})
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("重复组应 409: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/groups")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("组列表失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/groups/dev")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("组查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/identity/users/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失用户应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/identity/groups/dev/members", map[string]any{"userId": "missing"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失成员应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/identity/groups/dev", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除组失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/identity/groups/dev", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("重复删除组应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_MigrateErrors 验证迁移错误。
// @param t 测试上下文
func TestREST_MigrateErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/service/runtime/process-instances/missing/migrate", "application/json", bytes.NewBufferString("{"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法迁移体应 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/migrate", map[string]any{"targetProcessDefinitionId": "x"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_Jobs 验证作业管理端点。
// @param t 测试上下文
func TestREST_Jobs(t *testing.T) {
	server, _ := newTestServer(t)
	for _, path := range []string{"/service/management/jobs", "/service/management/deadletter-jobs"} {
		response, err := http.Get(server.URL + path)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("%s 请求失败: %v %d", path, err, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	response := postJSON(t, server.URL+"/service/management/jobs/missing", map[string]any{"action": "moveToExecutable"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失死信作业应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/management/jobs/missing", map[string]any{"action": "unknown"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法作业动作应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_ExecutionAndHistoryDetails 验证执行与历史详情端点。
// @param t 测试上下文
func TestREST_ExecutionAndHistoryDetails(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)

	response, _ = http.Get(server.URL + "/service/runtime/executions?processInstanceId=" + instanceID)
	var executions struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&executions)
	_ = response.Body.Close()
	if len(executions.Data) == 0 {
		t.Fatalf("执行列表为空")
	}
	executionID, _ := executions.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/executions/" + executionID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("执行详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/" + instanceID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("历史流程详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/history/historic-task-instances/" + taskID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("历史任务详情失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/executions/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失执行应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/history/historic-process-instances/missing")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失历史流程应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_CommentsAttachments 验证评论与附件端点。
// @param t 测试上下文
func TestREST_CommentsAttachments(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/comments", map[string]any{"message": "ok", "userId": "alice"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加评论失败: %d", response.StatusCode)
	}
	var comment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&comment)
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/comments")
	var comments struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&comments)
	_ = response.Body.Close()
	if comments.Total != 1 {
		t.Fatalf("评论列表错误: %d", comments.Total)
	}
	commentID, _ := comment["id"].(string)
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/comments/"+commentID, nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除评论失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]any{"name": "doc", "url": "http://example.com"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加附件失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/attachments")
	var attachments struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&attachments)
	_ = response.Body.Close()
	if attachments.Total != 1 {
		t.Fatalf("附件列表错误: %d", attachments.Total)
	}
}

// TestREST_CommentsErrors 验证评论附件错误。
// @param t 测试上下文
func TestREST_CommentsErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/runtime/tasks/missing/comments", map[string]any{"message": "x"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务评论应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	deployREST(t, server)
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/comments", map[string]any{"message": ""})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空评论应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/comments/missing", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失评论删除应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/attachments", map[string]any{"description": "x"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空附件名应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

const restFormXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="restForm" isExecutable="true"><startEvent id="s" flowable:formKey="start"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" flowable:formKey="task" flowable:assignee="§{initiator}"><extensionElements><flowable:formProperty id="approved" name="通过" type="boolean" required="true"/></extensionElements></userTask><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_FormData 验证表单端点。
// @param t 测试上下文
func TestREST_FormData(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restFormXML))
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/repository/process-definitions")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	definitionID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/form/form-data?processDefinitionId=" + definitionID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("启动表单失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "restForm",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var tasks struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	taskID, _ := tasks.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/form/form-data?taskId=" + taskID)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("任务表单失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/form/form-data", map[string]any{"taskId": taskID, "values": map[string]any{"approved": true}})
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("提交表单失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/form/form-data")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少参数应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_FormErrors 验证表单错误。
// @param t 测试上下文
func TestREST_FormErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response := postJSON(t, server.URL+"/service/form/form-data", map[string]any{})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少 taskId 应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/form/form-data", map[string]any{"taskId": "missing"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, err := http.Post(server.URL+"/service/form/form-data", "application/json", bytes.NewBufferString("{"))
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法表单体应 400: %v %d", err, response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TaskVariables 验证任务变量端点。
// @param t 测试上下文
func TestREST_TaskVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)

	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/variables", map[string]any{"name": "local", "value": "v"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建任务变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/variables")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("任务变量查询失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/tasks/"+taskID+"/variables/local", bytes.NewBufferString(`{"value":"v2"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("更新任务变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/variables/local", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除任务变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/missing/variables", map[string]any{"name": "x", "value": 1})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

const restIdentityLinkXML = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="restIdentityLink" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" flowable:candidateUsers="alice" flowable:candidateGroups="dev"/><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`

// TestREST_IdentityLinks 验证任务身份链接端点。
// @param t 测试上下文
func TestREST_IdentityLinks(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Post(server.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(restIdentityLinkXML))
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionKey": "restIdentityLink"})
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks")
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	taskID, _ := page.Data[0]["id"].(string)
	response, _ = http.Get(server.URL + "/service/runtime/tasks/" + taskID + "/identitylinks")
	var links []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&links)
	_ = response.Body.Close()
	if len(links) != 2 {
		t.Fatalf("身份链接数量错误: %+v", links)
	}
	response, _ = http.Get(server.URL + "/service/history/historic-identitylinks?taskId=" + taskID)
	var historicLinks struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&historicLinks)
	_ = response.Body.Close()
	if historicLinks.Total != 2 {
		t.Fatalf("历史身份链接数量错误: %d", historicLinks.Total)
	}
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks", map[string]any{"type": "candidate", "userId": "bob"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("添加候选用户失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks/users/bob", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除候选用户失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/identitylinks", map[string]any{"type": "invalid"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法类型应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_IdentityLinkErrors 验证身份链接错误。
// @param t 测试上下文
func TestREST_IdentityLinkErrors(t *testing.T) {
	server, _ := newTestServer(t)
	response, _ := http.Get(server.URL + "/service/runtime/tasks/missing/identitylinks")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/missing/identitylinks", map[string]any{"type": "candidate", "userId": "x"})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失任务添加链接应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/tasks/x/identitylinks", map[string]any{"type": "candidate"})
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺少用户组应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/tasks/x/identitylinks/other/y", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法族应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_InstanceVariables 验证实例变量增删改。
// @param t 测试上下文
func TestREST_InstanceVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	response := postJSON(t, server.URL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": "rest",
		"variables":            []map[string]any{{"name": "initiator", "value": "alice"}},
	})
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)
	response = postJSON(t, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables", map[string]any{"name": "amount", "value": 10})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建实例变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables/amount", bytes.NewBufferString(`{"value":20}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("更新实例变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	request, _ = http.NewRequest(http.MethodDelete, server.URL+"/service/runtime/process-instances/"+instanceID+"/variables/amount", nil)
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("删除实例变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = postJSON(t, server.URL+"/service/runtime/process-instances/missing/variables", map[string]any{"name": "x", "value": 1})
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失实例变量应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

// TestREST_TenantFiltering 验证租户过滤。
// @param t 测试上下文
func TestREST_TenantFiltering(t *testing.T) {
	server, _ := newTestServer(t)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/service/repository/deployments", bytes.NewBufferString(restProcess))
	request.Header.Set("Content-Type", "application/xml")
	request.Header.Set("X-Tenant-Id", "t1")
	response, _ := http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("租户部署失败: %d", response.StatusCode)
	}
	var deployment map[string]any
	_ = json.NewDecoder(response.Body).Decode(&deployment)
	_ = response.Body.Close()
	if deployment["tenantId"] != "t1" {
		t.Fatalf("部署租户错误: %+v", deployment)
	}
	for tenant, want := range map[string]int{"t1": 1, "t2": 0} {
		request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/repository/deployments", nil)
		request.Header.Set("X-Tenant-Id", tenant)
		response, _ = http.DefaultClient.Do(request)
		var page struct {
			Total int `json:"total"`
		}
		_ = json.NewDecoder(response.Body).Decode(&page)
		_ = response.Body.Close()
		if page.Total != want {
			t.Fatalf("租户 %s 部署数量错误: %d", tenant, page.Total)
		}
	}
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/service/runtime/process-instances", bytes.NewBufferString(`{"processDefinitionKey":"rest","variables":[{"name":"initiator","value":"alice"}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-Id", "t1")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("租户启动失败: %d", response.StatusCode)
	}
	var instance map[string]any
	_ = json.NewDecoder(response.Body).Decode(&instance)
	_ = response.Body.Close()
	instanceID, _ := instance["id"].(string)
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/service/runtime/process-instances/"+instanceID, nil)
	request.Header.Set("X-Tenant-Id", "t2")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户实例应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?tenantId=t1")
	var tasks struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if tasks.Total != 1 {
		t.Fatalf("租户任务数量错误: %d", tasks.Total)
	}
	taskData := struct {
		Data []map[string]any `json:"data"`
	}{}
	response, _ = http.Get(server.URL + "/service/runtime/tasks?tenantId=t1")
	_ = json.NewDecoder(response.Body).Decode(&taskData)
	_ = response.Body.Close()
	taskID, _ := taskData.Data[0]["id"].(string)
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/service/runtime/tasks/"+taskID, bytes.NewBufferString(`{"action":"complete"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-Id", "t2")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户任务应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/runtime/tasks?tenantId=t2")
	_ = json.NewDecoder(response.Body).Decode(&tasks)
	_ = response.Body.Close()
	if tasks.Total != 0 {
		t.Fatalf("其他租户任务不应可见: %d", tasks.Total)
	}
}

// TestREST_Password 验证密码修改与重置。
// @param t 测试上下文
func TestREST_Password(t *testing.T) {
	server, app := newTestServer(t)
	response := postJSON(t, server.URL+"/service/identity/users", map[string]any{"id": "alice", "password": "secret"})
	_ = response.Body.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/password", bytes.NewBufferString(`{"oldPassword":"secret","newPassword":"new"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("修改密码失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	if err := app.IdentityService().CheckPassword(context.Background(), "alice", "new"); err != nil {
		t.Fatalf("新密码校验失败: %v", err)
	}
	request, _ = http.NewRequest(http.MethodPut, server.URL+"/service/identity/users/alice/password", bytes.NewBufferString(`{"newPassword":"reset"}`))
	request.Header.Set("Content-Type", "application/json")
	response, _ = http.DefaultClient.Do(request)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("重置密码失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	if err := app.IdentityService().CheckPassword(context.Background(), "alice", "reset"); err != nil {
		t.Fatalf("重置密码校验失败: %v", err)
	}
}
