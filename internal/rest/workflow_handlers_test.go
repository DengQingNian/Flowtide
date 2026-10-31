package rest

// workflow_handlers_test.go 覆盖 /workflow/** 已接线端点：
// 每类端点至少 1 正例（HTTP 200 + success=true）与 1 个参数缺失反例；
// 保留 501 的端点在服务注入后仍为 501；未注入服务时全部 501（见 workflow_routes_test.go）。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	"github.com/DengQingNian/Flowtide/internal/tenant"
	"github.com/DengQingNian/Flowtide/internal/workflow"
)

// workflowFacadeBPMN 是 REST 门面测试语料（发起人申请 → 审批 → 结束，与门面包 facadeFixture 同构）。
const workflowFacadeBPMN = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="facadeProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}" stepCode="S00"/>
    <sequenceFlow id="f2" sourceRef="apply" targetRef="approve"/>
    <userTask id="approve" name="审批" flowable:assignee="u2" stepCode="S01"/>
    <sequenceFlow id="f3" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// workflowFacadeEnv 是注入 workflow 门面服务的 REST 测试环境。
type workflowFacadeEnv struct {
	server  *httptest.Server  // REST 测试服务
	engine  api.Engine        // 引擎门面（部署语料/建模型用）
	service *workflow.Service // workflow 门面服务
}

// newWorkflowFacadeEnv 构造注入门面服务的 REST 测试环境：引擎 + 语料部署 + 预置身份 + 钩子注册。
// @param t 测试上下文
// @return 测试环境
func newWorkflowFacadeEnv(t *testing.T) *workflowFacadeEnv {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	ctx := tenant.With(context.Background(), "bizA")
	// 预置发起人与审批人身份（意见/待办姓名补全依赖）
	for _, user := range []api.User{{ID: "u1", FirstName: "张", LastName: "三"}, {ID: "u2", FirstName: "李", LastName: "四"}} {
		if _, err := eng.IdentityService().CreateUser(ctx, user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	if _, err := eng.RepositoryService().Deploy(ctx, api.DeploymentRequest{
		Name:      "facade",
		TenantID:  "bizA",
		Resources: map[string][]byte{"facade.bpmn20.xml": []byte(workflowFacadeBPMN)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	service := workflow.NewWorkflowService(eng)
	if err := service.RegisterExecutorRewriteHook(ctx); err != nil {
		t.Fatalf("注册钩子失败: %v", err)
	}
	server := httptest.NewServer(New(eng, WithWorkflowService(service)).Handler())
	t.Cleanup(server.Close)
	return &workflowFacadeEnv{server: server, engine: eng, service: service}
}

// doWorkflowJSON 执行带 JSON body 的 workflow 端点请求。
// @param t 测试上下文
// @param method HTTP 方法
// @param url 完整 URL
// @param body JSON 请求体（空串表示无 body）
// @return HTTP 响应
func doWorkflowJSON(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s 请求失败: %v", method, url, err)
	}
	return response
}

// requireWorkflowOK 断言响应为 HTTP 200 且 success=true，返回协议体。
// @param t 测试上下文
// @param response HTTP 响应
// @return 协议体
func requireWorkflowOK(t *testing.T, response *http.Response) workflowRBody {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("应返回 200，实际 %d", response.StatusCode)
	}
	var body workflowRBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if !body.Success {
		t.Fatalf("应成功，实际失败: %+v", body)
	}
	return body
}

// requireWorkflowBizFail 断言响应为 HTTP 200 的业务失败（success=false + ErrorMessage）。
// @param t 测试上下文
// @param response HTTP 响应
func requireWorkflowBizFail(t *testing.T, response *http.Response) {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("业务失败应返回 200，实际 %d", response.StatusCode)
	}
	var body workflowRBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if body.Success || body.ErrorMessage == "" || body.Code != "500" {
		t.Fatalf("应为业务失败结构: %+v", body)
	}
}

// requireWorkflowParamFail 断言响应为 HTTP 500 的参数错误（RErr 结构）。
// @param t 测试上下文
// @param response HTTP 响应
func requireWorkflowParamFail(t *testing.T, response *http.Response) {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("参数缺失应返回 500，实际 %d", response.StatusCode)
	}
	var body workflowRBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}
	if body.Success || body.ErrorMessage == "" {
		t.Fatalf("应为参数失败结构: %+v", body)
	}
}

// rDataMap 取协议体 data 的对象形态。
// @param t 测试上下文
// @param body 协议体
// @return data 对象
func rDataMap(t *testing.T, body workflowRBody) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body.Data)
	if err != nil {
		t.Fatalf("序列化 data 失败: %v", err)
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("data 应为对象: %+v", body.Data)
	}
	return data
}

// rDataSlice 取协议体 data 的数组形态。
// @param t 测试上下文
// @param body 协议体
// @return data 数组
func rDataSlice(t *testing.T, body workflowRBody) []any {
	t.Helper()
	raw, err := json.Marshal(body.Data)
	if err != nil {
		t.Fatalf("序列化 data 失败: %v", err)
	}
	data := []any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("data 应为数组: %+v", body.Data)
	}
	return data
}

// workflowApply 经 REST 发起流程实例，返回实例 Id 与首任务 Id。
// @param t 测试上下文
// @param env 测试环境
// @param businessKey 业务单据 Id
// @return 实例 Id
// @return 首任务 Id
func workflowApply(t *testing.T, env *workflowFacadeEnv, businessKey string) (string, string) {
	t.Helper()
	response := doWorkflowJSON(t, http.MethodPost, env.server.URL+"/workflow/process/apply/task?tenantId=bizA",
		`{"processDefinitionKey":"facadeProc","title":"请假申请","operator":"u1","businessKey":"`+businessKey+`"}`)
	body := requireWorkflowOK(t, response)
	data := rDataMap(t, body)
	taskID, _ := data["taskId"].(string)
	instanceID, _ := data["processInstanceId"].(string)
	if taskID == "" || instanceID == "" {
		t.Fatalf("发起结果缺失: %+v", data)
	}
	return instanceID, taskID
}

// workflowComplete 经 REST 办结任务并预存下一步（approve 节点给 u2），返回下一任务 Id。
// @param t 测试上下文
// @param env 测试环境
// @param instanceID 实例 Id
// @param taskID 待办任务 Id
// @return 下一任务 Id
func workflowComplete(t *testing.T, env *workflowFacadeEnv, instanceID, taskID string) string {
	t.Helper()
	response := doWorkflowJSON(t, http.MethodPost, env.server.URL+"/workflow/process/complete/task?tenantId=bizA",
		`{"taskId":"`+taskID+`","userCode":"u1","message":"同意","processInstanceId":"`+instanceID+`","type":"SP",`+
			`"variables":{"operationType":"agreeComplete"},`+
			`"request":{"taskId":"`+taskID+`","nextActivityList":[{"activityId":"approve","executorIds":["u2"]}],`+
			`"sendIdentityId":"u1","sendUserName":"张三"}}`)
	body := requireWorkflowOK(t, response)
	next := rDataMap(t, body)
	ids, _ := next["nextTaskIds"].([]any)
	if len(ids) != 1 {
		t.Fatalf("办结结果应含 1 个下一任务: %+v", next)
	}
	return ids[0].(string)
}

// workflowTodoIDs 查询身份待办任务 Id 列表。
// @param t 测试上下文
// @param env 测试环境
// @param identityID 身份 Id
// @return 待办任务 Id 列表
func workflowTodoIDs(t *testing.T, env *workflowFacadeEnv, identityID string) []string {
	t.Helper()
	response, err := http.Get(env.server.URL + "/workflow/process/task/all/todoList?tenantId=bizA&identityId=" + identityID)
	if err != nil {
		t.Fatalf("待办查询失败: %v", err)
	}
	body := requireWorkflowOK(t, response)
	page := rDataMap(t, body)
	items, _ := page["data"].([]any)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]any)
		if taskID, _ := entry["TaskID"].(string); taskID != "" {
			ids = append(ids, taskID)
		}
	}
	return ids
}

// workflowFirstTodo 返回身份的首条待办任务 Id。
// @param t 测试上下文
// @param env 测试环境
// @param identityID 身份 Id
// @return 首条待办任务 Id
func workflowFirstTodo(t *testing.T, env *workflowFacadeEnv, identityID string) string {
	t.Helper()
	ids := workflowTodoIDs(t, env, identityID)
	if len(ids) == 0 {
		t.Fatalf("身份 %s 应有待办", identityID)
	}
	return ids[0]
}

// workflowInstanceTaskID 返回实例当前首个活动任务 Id（引擎侧状态断言用：
// 撤回等动作恢复的任务不再生成待办镜像，须直查引擎）。
// @param t 测试上下文
// @param env 测试环境
// @param instanceID 实例 Id
// @return 活动任务 Id
func workflowInstanceTaskID(t *testing.T, env *workflowFacadeEnv, instanceID string) string {
	t.Helper()
	page, err := env.engine.TaskService().ListTasks(tenant.With(context.Background(), "bizA"),
		api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
	if err != nil {
		t.Fatalf("查询实例任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("实例 %s 应存在活动任务", instanceID)
	}
	return page.Data[0].ID
}

// TestWorkflowHandlersProcessLifecycle 覆盖 /process/** 与 /task/** 核心链路：
// 发起 → 办结 → 撤回 → 再办结 → 退回 → 调整处理人 → 挂起/激活 → 软终止 → 删除，
// 途中校验待办/已办/任务详情/历史/高亮与参数缺失反例。
// @param t 测试上下文
func TestWorkflowHandlersProcessLifecycle(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	base := env.server.URL

	// ── 发起（正例 + 空参反例）──
	instanceID, taskID := workflowApply(t, env, "BIZ-100")
	failed := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/apply/task?tenantId=bizA", `{}`)
	requireWorkflowBizFail(t, failed)

	// ── 办结（正例 + 缺 taskId 反例）──
	nextTaskID := workflowComplete(t, env, instanceID, taskID)
	badComplete := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/complete/task?tenantId=bizA",
		`{"userCode":"u1","message":"同意"}`)
	requireWorkflowBizFail(t, badComplete)

	// ── 待办/已办（正例 + 缺 identityId 反例）──
	if todos := workflowTodoIDs(t, env, "u2"); len(todos) != 1 {
		t.Fatalf("u2 待办应为 1，实际 %v", todos)
	}
	if dones := workflowTodoIDs(t, env, "__none__"); len(dones) != 0 {
		t.Fatalf("无待办身份应为空: %v", dones)
	}
	doneResp, err := http.Get(base + "/workflow/process/task/all/doneList?tenantId=bizA&identityId=u1")
	if err != nil {
		t.Fatalf("已办查询失败: %v", err)
	}
	doneBody := requireWorkflowOK(t, doneResp)
	donePage := rDataMap(t, doneBody)
	doneItems, _ := donePage["data"].([]any)
	if len(doneItems) != 1 {
		t.Fatalf("u1 已办应为 1，实际 %+v", donePage)
	}
	missingTodo, err := http.Get(base + "/workflow/process/task/all/todoList?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingTodo)

	// ── 任务详情 / 历史按任务 / 历史按实例与业务键 / 高亮 ──
	taskInfo, err := http.Get(base + "/workflow/process/task/list?tenantId=bizA&taskId=" + nextTaskID)
	if err != nil {
		t.Fatalf("任务详情请求失败: %v", err)
	}
	requireWorkflowOK(t, taskInfo)
	missingTaskInfo, err := http.Get(base + "/workflow/process/task/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingTaskInfo)

	historyStep, err := http.Get(base + "/workflow/process/history/step/list/" + nextTaskID + "?tenantId=bizA")
	if err != nil {
		t.Fatalf("按任务查历史失败: %v", err)
	}
	requireWorkflowOK(t, historyStep)

	historyByPI, err := http.Get(base + "/workflow/process/history/task/list?tenantId=bizA&processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("按实例查历史失败: %v", err)
	}
	requireWorkflowOK(t, historyByPI)
	historyByBK, err := http.Get(base + "/workflow/process/history/task/list?tenantId=bizA&businessKey=BIZ-100")
	if err != nil {
		t.Fatalf("按业务键查历史失败: %v", err)
	}
	requireWorkflowOK(t, historyByBK)
	missingHistory, err := http.Get(base + "/workflow/process/history/task/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingHistory)

	highlight := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/highlight/img?tenantId=bizA&processInstanceId="+instanceID, "")
	requireWorkflowOK(t, highlight)
	highlightByTask := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/normal/img?tenantId=bizA&taskId="+nextTaskID, "")
	requireWorkflowOK(t, highlightByTask)
	missingHighlight := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/highlight/img?tenantId=bizA", "")
	requireWorkflowParamFail(t, missingHighlight)

	// ── 撤回（正例 + 缺 taskId 反例）：撤回已办 apply 任务（任务恢复至 u1 待办态，
	// 但待办镜像已被办结消耗，引擎侧直查活动任务）──
	rollback := doWorkflowJSON(t, http.MethodPost, base+"/workflow/task/rollback?tenantId=bizA",
		`{"processInstanceId":"`+instanceID+`","taskId":"`+taskID+`","operator":"u1"}`)
	requireWorkflowOK(t, rollback)
	missingRollback := doWorkflowJSON(t, http.MethodPost, base+"/workflow/task/rollback?tenantId=bizA",
		`{"processInstanceId":"`+instanceID+`","operator":"u1"}`)
	requireWorkflowParamFail(t, missingRollback)
	reapplyTask := workflowInstanceTaskID(t, env, instanceID)

	// ── 再次办结 → 退回（approve → apply）→ 待办回到 u1 ──
	approveTask2 := workflowComplete(t, env, instanceID, reapplyTask)
	back := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/back/process?tenantId=bizA",
		`{"taskId":"`+approveTask2+`","userCode":"u2","targetActivityId":"apply","message":"材料不全"}`)
	requireWorkflowOK(t, back)
	missingBack := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/back/process?tenantId=bizA", `{}`)
	requireWorkflowParamFail(t, missingBack)
	backedTask := workflowFirstTodo(t, env, "u1")
	if backedTask == "" || backedTask == approveTask2 {
		t.Fatalf("退回后应生成新的 apply 待办: %q", backedTask)
	}

	// ── 调整处理人（apply 节点转给 u2，正例 + 缺 toOwner 反例；引擎侧断言办理人）──
	transfer := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/transfer/task?tenantId=bizA",
		`{"processInstanceId":"`+instanceID+`","taskKey":"apply","toOwner":"u2","operator":"u1"}`)
	requireWorkflowOK(t, transfer)
	missingTransfer := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/transfer/task?tenantId=bizA",
		`{"processInstanceId":"`+instanceID+`","taskKey":"apply","operator":"u1"}`)
	requireWorkflowParamFail(t, missingTransfer)
	transferred, err := env.engine.TaskService().GetTask(tenant.With(context.Background(), "bizA"), backedTask)
	if err != nil {
		t.Fatalf("查询调整后任务失败: %v", err)
	}
	if transferred.Assignee != "u2" {
		t.Fatalf("调整处理人未生效: %q", transferred.Assignee)
	}

	// ── 直送（先补一个新实例，直送 apply → approve；正例 + 缺 targetActivityId 反例）──
	_, directTask := workflowApply(t, env, "BIZ-101")
	direct := doWorkflowJSON(t, http.MethodPost, base+"/workflow/task/completeToTargetActivity?tenantId=bizA",
		`{"taskId":"`+directTask+`","userCode":"u1","targetActivityId":"approve","message":"直送","type":"SP",`+
			`"request":{"taskId":"`+directTask+`","nextActivityList":[{"activityId":"approve","executorIds":["u2"]}]}}`)
	requireWorkflowOK(t, direct)
	missingDirect := doWorkflowJSON(t, http.MethodPost, base+"/workflow/task/completeToTargetActivity?tenantId=bizA",
		`{"taskId":"`+directTask+`","userCode":"u1","message":"缺目标"}`)
	requireWorkflowBizFail(t, missingDirect)

	// ── 挂起/激活（正例 + 空路径反例用不存在实例触发业务失败）──
	suspend := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/suspend/processInstance/"+instanceID+"?tenantId=bizA&operator=u1", "")
	requireWorkflowOK(t, suspend)
	activate := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/activate/processInstance/"+instanceID+"?tenantId=bizA&operator=u1", "")
	requireWorkflowOK(t, activate)
	suspendMissing := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/suspend/processInstance/no-such?tenantId=bizA&operator=u1", "")
	requireWorkflowBizFail(t, suspendMissing)

	// ── 软终止（正例 + 不存在实例反例）──
	stop := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/stop/processInstance/"+instanceID+"?tenantId=bizA&operator=u1&message=%E7%BB%88%E6%AD%A2", "")
	requireWorkflowOK(t, stop)
	stopMissing := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/stop/processInstance/no-such?tenantId=bizA&operator=u1", "")
	requireWorkflowBizFail(t, stopMissing)

	// ── 按实例删除（软终止后的实例；正例 + 不存在实例反例）──
	deleteByID := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/instance/all/delete/"+instanceID+"?tenantId=bizA&operator=u1", "")
	requireWorkflowOK(t, deleteByID)
	deleteMissing := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/instance/all/delete/no-such?tenantId=bizA&operator=u1", "")
	requireWorkflowBizFail(t, deleteMissing)

	// ── 按业务单据删除（正例 + 缺 businessKey 反例）──
	workflowApply(t, env, "BIZ-102")
	deleteByBK := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/delete/process?tenantId=bizA&businessKey=BIZ-102&operator=u1", "")
	requireWorkflowOK(t, deleteByBK)
	missingDeleteByBK := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/delete/process?tenantId=bizA&operator=u1", "")
	requireWorkflowParamFail(t, missingDeleteByBK)
}

// TestWorkflowHandlersMgmtLifecycle 覆盖 /model、/category、/step、/template 管理链路
// 与 /process/deploy 部署、/hisstep 快照查询，含参数缺失反例。
// @param t 测试上下文
func TestWorkflowHandlersMgmtLifecycle(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	base := env.server.URL

	// ── 模型新增（正例 + 空参反例）──
	addResp := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/add?tenantId=bizA",
		`{"modelCode":"cat-hr","modelName":"人事分类","tenantId":"bizA"}`)
	addBody := requireWorkflowOK(t, addResp)
	category := rDataMap(t, addBody)
	categoryID, _ := category["ID"].(string)
	if categoryID == "" {
		t.Fatalf("新增分类应返回主键: %+v", category)
	}
	missingAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/add?tenantId=bizA", `{}`)
	requireWorkflowBizFail(t, missingAdd)

	// ── 分类页同语义投影（category/add 与 list）──
	categoryAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/category/add?tenantId=bizA",
		`{"modelCode":"cat-fin","modelName":"财务分类","tenantId":"bizA"}`)
	requireWorkflowOK(t, categoryAdd)
	categoryList, err := http.Get(base + "/workflow/category/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("分类列表失败: %v", err)
	}
	requireWorkflowOK(t, categoryList)

	// ── 模型编辑 / 列表 / 树（正例 + 缺 id 反例）──
	edit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/edit?tenantId=bizA",
		`{"id":"`+categoryID+`","modelName":"人事分类V2","modelCode":"cat-hr"}`)
	requireWorkflowOK(t, edit)
	missingEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/edit?tenantId=bizA", `{"modelName":"无主键"}`)
	requireWorkflowBizFail(t, missingEdit)

	modelList, err := http.Get(base + "/workflow/model/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("模型列表失败: %v", err)
	}
	requireWorkflowOK(t, modelList)
	tree, err := http.Get(base + "/workflow/model/tree/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("模型树失败: %v", err)
	}
	requireWorkflowOK(t, tree)
	missingList, err := http.Get(base + "/workflow/model/list")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	// 租户缺失由服务端判业务失败（模型列表缺少租户）
	requireWorkflowBizFail(t, missingList)

	// ── 引擎侧建模型并绑定，供部署链路使用 ──
	engineCtx := tenant.With(context.Background(), "bizA")
	engineModel, err := env.engine.RepositoryService().CreateModel(engineCtx, api.CreateModelRequest{
		Name: "facadeProc", Key: "facadeProc", TenantID: "bizA",
	})
	if err != nil {
		t.Fatalf("创建引擎模型失败: %v", err)
	}
	if err := env.engine.RepositoryService().SetModelSource(engineCtx, engineModel.ID, []byte(workflowFacadeBPMN)); err != nil {
		t.Fatalf("写入模型源失败: %v", err)
	}
	modelAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/add?tenantId=bizA",
		`{"modelCode":"facadeProc","modelName":"请假模型","tenantId":"bizA","engineModelId":"`+engineModel.ID+`"}`)
	modelBody := requireWorkflowOK(t, modelAdd)
	modelData := rDataMap(t, modelBody)
	modelID, _ := modelData["ID"].(string)
	if modelID == "" {
		t.Fatalf("新增模型应返回主键: %+v", modelData)
	}

	// ── 设计时步骤（正例 + 缺 modelId 的列表反例）──
	stepAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/step/add?tenantId=bizA",
		`{"modelId":"`+modelID+`","stepCode":"apply","name":"申请","tenantId":"bizA"}`)
	stepBody := requireWorkflowOK(t, stepAdd)
	stepData := rDataMap(t, stepBody)
	stepID, _ := stepData["ID"].(string)
	if stepID == "" {
		t.Fatalf("新增步骤应返回主键: %+v", stepData)
	}
	stepEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/step/edit?tenantId=bizA",
		`{"id":"`+stepID+`","modelId":"`+modelID+`","stepCode":"apply","name":"申请（改）"}`)
	requireWorkflowOK(t, stepEdit)
	stepList, err := http.Get(base + "/workflow/step/list?tenantId=bizA&modelId=" + modelID)
	if err != nil {
		t.Fatalf("步骤列表失败: %v", err)
	}
	stepListBody := requireWorkflowOK(t, stepList)
	if steps := rDataSlice(t, stepListBody); len(steps) != 1 {
		t.Fatalf("步骤列表应为 1: %+v", stepListBody)
	}
	missingStepList, err := http.Get(base + "/workflow/step/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingStepList)

	// ── 部署（deplay 兼容拼写；正例 + 缺 modelId 反例）──
	deploy := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/deplay?tenantId=bizA&modelId="+modelID, "")
	deployBody := requireWorkflowOK(t, deploy)
	deployData := rDataMap(t, deployBody)
	deploymentID, _ := deployData["deploymentId"].(string)
	if deploymentID == "" {
		t.Fatalf("部署应返回 deploymentId: %+v", deployData)
	}
	missingDeploy := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/deploy?tenantId=bizA", "")
	requireWorkflowParamFail(t, missingDeploy)

	// ── 部署快照查询（正例 + 缺 deploymentId 反例）──
	hisSteps, err := http.Get(base + "/workflow/hisstep/list?tenantId=bizA&deploymentId=" + deploymentID)
	if err != nil {
		t.Fatalf("快照查询失败: %v", err)
	}
	requireWorkflowOK(t, hisSteps)
	missingHisSteps, err := http.Get(base + "/workflow/hisstep/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingHisSteps)

	// ── 模板（正例 + 空参反例）──
	tplAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/template/add?tenantId=bizA",
		`{"name":"默认模板","tenantId":"bizA"}`)
	tplBody := requireWorkflowOK(t, tplAdd)
	tplData := rDataMap(t, tplBody)
	tplID, _ := tplData["ID"].(string)
	if tplID == "" {
		t.Fatalf("新增模板应返回主键: %+v", tplData)
	}
	missingTpl := doWorkflowJSON(t, http.MethodPost, base+"/workflow/template/add?tenantId=bizA", `{}`)
	requireWorkflowBizFail(t, missingTpl)
	tplEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/template/edit?tenantId=bizA",
		`{"id":"`+tplID+`","name":"默认模板V2"}`)
	requireWorkflowOK(t, tplEdit)
	tplList, err := http.Get(base + "/workflow/template/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("模板列表失败: %v", err)
	}
	requireWorkflowOK(t, tplList)

	// ── 批量删除（步骤/模板/模型；正例 + 空路径反例不可能为空路径，用 405/404 之外的语义不做）──
	stepDelete := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/step/delete/"+stepID+"?tenantId=bizA", "")
	requireWorkflowOK(t, stepDelete)
	tplDelete := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/template/delete/"+tplID+"?tenantId=bizA", "")
	requireWorkflowOK(t, tplDelete)
	modelDelete := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/delete/"+modelID+","+categoryID+"?tenantId=bizA", "")
	requireWorkflowOK(t, modelDelete)
}

// TestWorkflowHandlersSignFamily 覆盖加签（add/execution）、减签（del/execution）、
// 会签结论（complete/sign）端点，含参数缺失反例。
// @param t 测试上下文
func TestWorkflowHandlersSignFamily(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	base := env.server.URL
	instanceID, taskID := workflowApply(t, env, "BIZ-201")

	// ── 加签（body 正例 + assignees 走 query 逗号分隔兜底；空参反例）──
	addSign := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/add/execution?tenantId=bizA&assignees=u2",
		`{"taskId":"`+taskID+`","operator":"u1","scopeType":"after","message":"请协助办理"}`)
	addBody := requireWorkflowOK(t, addSign)
	addData := rDataMap(t, addBody)
	subTasks, _ := addData["SubTaskIDs"].([]any)
	if len(subTasks) != 1 {
		t.Fatalf("加签应生成 1 个子任务: %+v", addData)
	}
	missingAddSign := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/add/execution?tenantId=bizA", `{}`)
	requireWorkflowBizFail(t, missingAddSign)

	// ── 减签（正例 + 缺 subTaskId 反例）──
	delSign := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/del/execution?tenantId=bizA",
		`{"parentTaskId":"`+taskID+`","subTaskId":"`+subTasks[0].(string)+`","operator":"u1"}`)
	requireWorkflowOK(t, delSign)
	missingDelSign := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/del/execution?tenantId=bizA",
		`{"parentTaskId":"`+taskID+`","operator":"u1"}`)
	requireWorkflowBizFail(t, missingDelSign)

	// ── 会签结论（无会签规则实例返回未超线；正例 + 缺 countersignResult 反例）──
	sign := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/complete/sign?tenantId=bizA&taskId="+taskID+"&countersignResult=1", "")
	signBody := requireWorkflowOK(t, sign)
	if passed, _ := signBody.Data.(bool); passed {
		t.Fatalf("无会签规则不应判定通过: %+v", signBody)
	}
	missingSign := doWorkflowJSON(t, http.MethodPost, base+"/workflow/process/complete/sign?tenantId=bizA&taskId="+taskID, "")
	requireWorkflowParamFail(t, missingSign)

	// 减签后实例状态应仍可查询（流程图高亮作为存活探针）
	stillAlive := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/process/highlight/img?tenantId=bizA&processInstanceId="+instanceID, "")
	requireWorkflowOK(t, stillAlive)
}

// TestWorkflowHandlersDelegateRemindExceptionDict 覆盖 /delegate、/remind、/exception、/dict 端点。
// @param t 测试上下文
func TestWorkflowHandlersDelegateRemindExceptionDict(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	base := env.server.URL

	// 预置一个模型供催办配置挂载
	modelAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/model/add?tenantId=bizA",
		`{"modelCode":"remind-model","modelName":"催办模型","tenantId":"bizA"}`)
	modelBody := requireWorkflowOK(t, modelAdd)
	modelData := rDataMap(t, modelBody)
	modelID, _ := modelData["ID"].(string)

	// ── 委托规则新增（正例 + 缺委托人反例）──
	delegateAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/delegate/add?tenantId=bizA",
		`{"ownerId":"u1","ownerName":"张三","tenantId":"bizA",`+
			`"vaildFrom":"2026-10-01T00:00:00Z","vaildTo":"2026-12-01T00:00:00Z",`+
			`"consigners":[{"consignerId":"u2","consignerName":"李四"}]}`)
	delegateBody := requireWorkflowOK(t, delegateAdd)
	delegateData := rDataMap(t, delegateBody)
	delegateID, _ := delegateData["id"].(string)
	if delegateID == "" {
		t.Fatalf("新增委托应返回规则 Id: %+v", delegateData)
	}
	missingDelegate := doWorkflowJSON(t, http.MethodPost, base+"/workflow/delegate/add?tenantId=bizA",
		`{"tenantId":"bizA","vaildFrom":"2026-10-01T00:00:00Z","vaildTo":"2026-12-01T00:00:00Z",`+
			`"consigners":[{"consignerId":"u2"}]}`)
	requireWorkflowBizFail(t, missingDelegate)

	// ── 委托规则列表（正例 + 缺 ownerId 反例）──
	delegateList, err := http.Get(base + "/workflow/delegate/list?tenantId=bizA&ownerId=u1")
	if err != nil {
		t.Fatalf("委托列表失败: %v", err)
	}
	delegateListBody := requireWorkflowOK(t, delegateList)
	if rules := rDataSlice(t, delegateListBody); len(rules) != 1 {
		t.Fatalf("委托规则应为 1: %+v", delegateListBody)
	}
	missingDelegateList, err := http.Get(base + "/workflow/delegate/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingDelegateList)

	// ── 委托规则编辑与删除（正例 + 缺 id 反例）──
	delegateEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/delegate/edit?tenantId=bizA",
		`{"id":"`+delegateID+`","remark":"调整委托"}`)
	requireWorkflowOK(t, delegateEdit)
	missingDelegateEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/delegate/edit?tenantId=bizA", `{}`)
	requireWorkflowBizFail(t, missingDelegateEdit)
	delegateDelete := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/delegate/delete/"+delegateID+"?tenantId=bizA", "")
	requireWorkflowOK(t, delegateDelete)

	// ── 催办配置新增（正例 + 缺 remind.pid 反例）──
	remindAdd := doWorkflowJSON(t, http.MethodPost, base+"/workflow/remind/add?tenantId=bizA",
		`{"remind":{"pid":"`+modelID+`","tenantId":"bizA"}}`)
	remindBody := requireWorkflowOK(t, remindAdd)
	remindData := rDataMap(t, remindBody)
	remindID, _ := remindData["ID"].(string)
	if remindID == "" {
		t.Fatalf("新增催办应返回主键: %+v", remindData)
	}
	missingRemind := doWorkflowJSON(t, http.MethodPost, base+"/workflow/remind/add?tenantId=bizA", `{}`)
	requireWorkflowParamFail(t, missingRemind)

	// ── 催办分页 / 详情 / 编辑 / 删除（正例 + 缺 processInstanceId 类反例）──
	remindPage, err := http.Get(base + "/workflow/remind/getPageList?tenantId=bizA&modelId=" + modelID)
	if err != nil {
		t.Fatalf("催办分页失败: %v", err)
	}
	requireWorkflowOK(t, remindPage)
	remindGet, err := http.Get(base + "/workflow/remind/getWorkflowRemind/" + remindID + "?tenantId=bizA")
	if err != nil {
		t.Fatalf("催办详情失败: %v", err)
	}
	requireWorkflowOK(t, remindGet)
	remindEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/remind/edit?tenantId=bizA",
		`{"remind":{"id":"`+remindID+`","enablePre":1}}`)
	requireWorkflowOK(t, remindEdit)
	missingRemindEdit := doWorkflowJSON(t, http.MethodPost, base+"/workflow/remind/edit?tenantId=bizA", `{}`)
	requireWorkflowParamFail(t, missingRemindEdit)
	remindDelete := doWorkflowJSON(t, http.MethodDelete, base+"/workflow/remind/delete/"+remindID+"?tenantId=bizA", "")
	requireWorkflowOK(t, remindDelete)

	// ── 异常列表（空数据正例 + 缺实例反例）与批量处理（不存在 Id 为无操作正例）──
	exceptionList, err := http.Get(base + "/workflow/exception/list?tenantId=bizA&processInstanceId=pi-none")
	if err != nil {
		t.Fatalf("异常列表失败: %v", err)
	}
	exceptionBody := requireWorkflowOK(t, exceptionList)
	if items := rDataSlice(t, exceptionBody); len(items) != 0 {
		t.Fatalf("异常列表应为空: %+v", exceptionBody)
	}
	missingException, err := http.Get(base + "/workflow/exception/list?tenantId=bizA")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowParamFail(t, missingException)
	exceptionHandle := doWorkflowJSON(t, http.MethodPost, base+"/workflow/exception/delete/ex-none?tenantId=bizA&handled=true", "")
	requireWorkflowOK(t, exceptionHandle)

	// ── 字典（正例 + 未知名反例）──
	dict, err := http.Get(base + "/workflow/dict/messageType")
	if err != nil {
		t.Fatalf("字典查询失败: %v", err)
	}
	dictBody := requireWorkflowOK(t, dict)
	if items := rDataMap(t, dictBody); items == nil {
		t.Fatalf("字典应返回条目: %+v", dictBody)
	}
	unknownDict, err := http.Get(base + "/workflow/dict/no-such-dict")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	requireWorkflowBizFail(t, unknownDict)
}

// TestWorkflowHandlersRetainedStub501 验证服务注入后保留 501 的端点仍为桩响应。
// @param t 测试上下文
func TestWorkflowHandlersRetainedStub501(t *testing.T) {
	env := newWorkflowFacadeEnv(t)
	cases := []struct {
		method string // HTTP 方法
		path   string // 端点路径
	}{
		{http.MethodPost, "/workflow/process/add/model"},             // P6 设计器
		{http.MethodGet, "/workflow/process/getModelJsonByType"},     // P6 设计器
		{http.MethodGet, "/workflow/process/instance/list"},          // Service 无实例分页
		{http.MethodPost, "/workflow/process/suspend/processes"},     // Service 无批量挂起
		{http.MethodPost, "/workflow/process/activate/processes"},    // Service 无批量激活
		{http.MethodPost, "/workflow/process/public/model/id"},       // P6 发布族
		{http.MethodPost, "/workflow/process/delegate/task"},         // 引擎委托语义，与规则 CRUD 不同
		{http.MethodPost, "/workflow/process/resolve/task"},          // Service 无签收能力
		{http.MethodPost, "/workflow/process/claim/task"},            // Service 无签收能力
		{http.MethodPost, "/workflow/process/inject/node"},           // P6 节点注入
		{http.MethodPost, "/workflow/process/wait/node/list"},        // P6 等待节点
		{http.MethodGet, "/workflow/model/getUserTaskJsonInfo"},      // P6 设计器元数据
		{http.MethodGet, "/workflow/model/findModelAndStepPageList"}, // P6 联合分页
		{http.MethodGet, "/workflow/step/findById"},                  // P6 步骤主键查询
		{http.MethodPost, "/workflow/hisstep/add"},                   // 快照只读
		{http.MethodPost, "/workflow/extend/save"},                   // 审阅绑定属域服务内部端口
		{http.MethodGet, "/workflow/extend/getActByProcessDefKeyAndActId"},
		{http.MethodPost, "/workflow/weight/save"},                  // 会签权重属域服务内部端口
		{http.MethodGet, "/workflow/task/operationButtonList/pi-1"}, // Service 无按钮聚合
		{http.MethodGet, "/workflow/identity/tree"},                 // D10 身份库树另行评估
	}
	for _, testCase := range cases {
		response := doWorkflowJSON(t, testCase.method, env.server.URL+testCase.path, "")
		if response.StatusCode != http.StatusNotImplemented {
			_ = response.Body.Close()
			t.Errorf("%s %s 应保留 501，实际 %d", testCase.method, testCase.path, response.StatusCode)
			continue
		}
		var body workflowRBody
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			_ = response.Body.Close()
			t.Errorf("%s %s 解析 JSON 失败: %v", testCase.method, testCase.path, err)
			continue
		}
		_ = response.Body.Close()
		if body.Code != "500" || body.ErrorMessage != workflowNotImplementedMsg || body.Success {
			t.Errorf("%s %s 协议体应为 501 桩结构: %+v", testCase.method, testCase.path, body)
		}
	}
}

// TestWorkflowHandlersNotInjectedSpotCheck 抽查未注入 workflow 服务时已接线端点回 501。
// @param t 测试上下文
func TestWorkflowHandlersNotInjectedSpotCheck(t *testing.T) {
	server, _ := newWorkflowTestServer(t)
	cases := []struct {
		method string // HTTP 方法
		path   string // 端点路径
	}{
		{http.MethodPost, "/workflow/process/apply/task"},
		{http.MethodGet, "/workflow/model/list"},
		{http.MethodGet, "/workflow/delegate/list"},
		{http.MethodGet, "/workflow/dict/messageType"},
	}
	for _, testCase := range cases {
		response := doWorkflowJSON(t, testCase.method, server.URL+testCase.path, "")
		assertWorkflowNotImplemented(t, response)
		_ = response.Body.Close()
	}
}

// TestWorkflowParamsUnit 覆盖 /workflow/** 参数读取器的分支：query 优先、body 兜底、
// 数值/布尔/数组形态、非法输入容错与分页规范化。
// @param t 测试上下文
func TestWorkflowParamsUnit(t *testing.T) {
	// 非 JSON 对象体报错
	request := httptest.NewRequest(http.MethodPost, "/workflow/process/apply/task", strings.NewReader(`[1,2]`))
	if _, err := newWorkflowParams(request); err == nil {
		t.Fatalf("数组 body 应报错")
	}
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/apply/task", strings.NewReader(`null`))
	if _, err := newWorkflowParams(request); err == nil {
		t.Fatalf("null body 应报错")
	}
	// 空体与合法对象体
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/apply/task?start=-5&size=5000&flag=notbool", strings.NewReader(`{"taskId":"t1","count":3,"on":true,"ids":["a","b",1],"note":"n"}`))
	params, err := newWorkflowParams(request)
	if err != nil {
		t.Fatalf("合法对象体不应报错: %v", err)
	}
	if params.get("taskId") != "t1" || params.get("missing") != "" {
		t.Fatalf("get 结果异常: %q", params.get("taskId"))
	}
	// body 数值/布尔串化
	if v := params.get("count"); v != "3" {
		t.Fatalf("body 数值应串化: %q", v)
	}
	if v := params.get("on"); v != "true" {
		t.Fatalf("body 布尔应串化: %q", v)
	}
	// query 优先：query 同名参数覆盖 body
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/x?taskId=q1&count=9", strings.NewReader(`{"taskId":"t1","count":3}`))
	params, _ = newWorkflowParams(request)
	if params.get("taskId") != "q1" || params.get("count") != "9" {
		t.Fatalf("query 应优先于 body")
	}
	// getRequired 缺失报错
	request = httptest.NewRequest(http.MethodGet, "/workflow/process/x", nil)
	params, _ = newWorkflowParams(request)
	if _, err := params.getRequired("taskId"); err == nil {
		t.Fatalf("缺失必填参数应报错")
	}
	// getInt：非法回落缺省
	request = httptest.NewRequest(http.MethodGet, "/workflow/process/x?start=abc&size=-3", nil)
	params, _ = newWorkflowParams(request)
	if params.getInt("start", 7) != 7 {
		t.Fatalf("非法整数应回落缺省")
	}
	if params.getInt("size", 7) != -3 {
		t.Fatalf("负数是合法整数应原样返回")
	}
	// getBool：query 非法回落缺省；body 三形态
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/x?flag=notbool", strings.NewReader(`{"b1":true,"b2":0,"b3":"false"}`))
	params, _ = newWorkflowParams(request)
	if !params.getBool("b1", false) || params.getBool("b2", true) || params.getBool("b3", true) {
		t.Fatalf("body 布尔解析异常")
	}
	if params.getBool("flag", true) != true {
		t.Fatalf("query 非法布尔应回落缺省")
	}
	if params.getBool("missing", true) != true {
		t.Fatalf("缺失布尔应回落缺省")
	}
	// getSlice：query 逗号分隔 / body 数组（混合元素）/ body 字符串 / 缺失
	request = httptest.NewRequest(http.MethodGet, "/workflow/process/x?ids=a,%20b,,c", nil)
	params, _ = newWorkflowParams(request)
	if got := params.getSlice("ids"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("query 数组解析异常: %v", got)
	}
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/x", strings.NewReader(`{"ids":["a","b",1]}`))
	params, _ = newWorkflowParams(request)
	if got := params.getSlice("ids"); len(got) != 3 || got[2] != "1" {
		t.Fatalf("body 数组解析异常: %v", got)
	}
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/x", strings.NewReader(`{"ids":"a,b"}`))
	params, _ = newWorkflowParams(request)
	if got := params.getSlice("ids"); len(got) != 2 {
		t.Fatalf("body 字符串数组解析异常: %v", got)
	}
	request = httptest.NewRequest(http.MethodPost, "/workflow/process/x", strings.NewReader(`{"ids":42}`))
	params, _ = newWorkflowParams(request)
	if got := params.getSlice("ids"); got != nil {
		t.Fatalf("不支持的 body 形态应返回 nil: %v", got)
	}
	// pageQuery：负偏移归零、非正数量取默认、超限截断
	request = httptest.NewRequest(http.MethodGet, "/workflow/process/x?start=-5&size=5000", nil)
	params, _ = newWorkflowParams(request)
	page := params.pageQuery()
	if page.Start != 0 || page.Size != workflowMaxPageSize {
		t.Fatalf("分页规范化异常: %+v", page)
	}
	request = httptest.NewRequest(http.MethodGet, "/workflow/process/x", nil)
	params, _ = newWorkflowParams(request)
	if page := params.pageQuery(); page.Start != 0 || page.Size != 10 {
		t.Fatalf("分页缺省异常: %+v", page)
	}
	// decode：空体不操作、类型不匹配报错
	params = &workflowParams{}
	if err := params.decode(&api.BackTaskVo{}); err != nil {
		t.Fatalf("空体 decode 不应报错: %v", err)
	}
	params = &workflowParams{raw: []byte(`{"taskId":123}`)}
	if err := params.decode(&api.BackTaskVo{}); err == nil {
		t.Fatalf("类型不匹配 decode 应报错")
	}
	// splitWorkflowIDs：去空白去空项
	if got := splitWorkflowIDs(" a , ,b "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Id 拆分异常: %v", got)
	}
	// workflowErrMessage：普通错误取原始串
	if msg := workflowErrMessage(io.EOF); msg != io.EOF.Error() {
		t.Fatalf("普通错误文案异常: %q", msg)
	}
}

// TestWorkflowHandlersDefensiveBranches 直调 handler 覆盖路径参数为空的防御分支（500 参数错误）。
// @param t 测试上下文
func TestWorkflowHandlersDefensiveBranches(t *testing.T) {
	// 直接构造 Server：防御分支在触达引擎/门面前返回，无需真实依赖
	server := New(nil, WithWorkflowService(&workflow.Service{}))
	cases := []struct {
		name    string          // 用例名
		handler workflowHandler // 被测 handler
		method  string          // 请求方法
		path    string          // 请求路径
		body    string          // 请求体
	}{
		{"停止缺实例", server.handleWorkflowStopProcess, http.MethodPost, "/workflow/process/end/process", ""},
		{"按实例删除缺主键", server.handleWorkflowDeleteInstance, http.MethodDelete, "/workflow/process/instance/all/delete/", ""},
		{"挂起缺主键", server.handleWorkflowSuspendInstance, http.MethodPost, "/workflow/process/suspend/processInstance/", ""},
		{"激活缺主键", server.handleWorkflowActivateInstance, http.MethodPost, "/workflow/process/activate/processInstance/", ""},
		{"按任务历史缺主键", server.handleWorkflowHistoryStepList, http.MethodGet, "/workflow/process/history/step/list/", ""},
		{"模型删除缺主键", server.handleWorkflowModelDelete, http.MethodDelete, "/workflow/model/delete/", ""},
		{"步骤删除缺主键", server.handleWorkflowStepDelete, http.MethodDelete, "/workflow/step/delete/", ""},
		{"模板删除缺主键", server.handleWorkflowTemplateDelete, http.MethodDelete, "/workflow/template/delete/", ""},
		{"委托删除缺主键", server.handleWorkflowDelegateDelete, http.MethodDelete, "/workflow/delegate/delete/", ""},
		{"催办详情缺主键", server.handleWorkflowRemindGet, http.MethodGet, "/workflow/remind/getWorkflowRemind/", ""},
		{"催办删除缺主键", server.handleWorkflowRemindDelete, http.MethodDelete, "/workflow/remind/delete/", ""},
		{"异常处理缺主键", server.handleWorkflowExceptionDelete, http.MethodDelete, "/workflow/exception/delete/", ""},
		{"已办缺身份", server.handleWorkflowDoneList, http.MethodGet, "/workflow/process/task/all/doneList", ""},
		{"发起体非法", server.handleWorkflowApplyTask, http.MethodPost, "/workflow/process/apply/task", `{"title":123}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
			recorder := httptest.NewRecorder()
			params, err := newWorkflowParams(request)
			if err != nil {
				// 体非法时 handler 内部同样应判参数错误：直调并以错误参数路径触发
				params = &workflowParams{query: request.URL.Query()}
			}
			testCase.handler(recorder, request, params)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("应返回 500，实际 %d", recorder.Code)
			}
		})
	}
}
