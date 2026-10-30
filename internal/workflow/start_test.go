// start_test.go 发起域服务单元测试（P1-D）。
// 覆盖：五参校验、未部署拦截（租户隔离）、变量注入、首任务执行者解析全分支
// （assignee/candidateUsers/candidateGroups/executorList/vo 变量）、组展开、
// 步骤未设置执行者阻断与补偿回滚、Draft 步骤开始时间、空备注跳过意见。
package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// startCorpusBase 是发起测试基础语料：首任务办理人=${initiator}，第二步模型默认 u9。
const startCorpusBase = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="startProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="approve"/>
    <userTask id="approve" name="审批" flowable:assignee="u9"/>
    <sequenceFlow id="f2" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startCorpusCandidateUsers 首任务候选人多人类语料。
const startCorpusCandidateUsers = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="usersProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:candidateUsers="u2,u3"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startCorpusGroups 首任务候选组语料（组展开经引擎身份库）。
const startCorpusGroups = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="groupProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:candidateGroups="g1"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startCorpusExecutorList 首任务 executorList 优先语料（person 与 org 混合）。
const startCorpusExecutorList = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="listProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="u9"
              executorList='[{"identityId":"u2","executorType":"person"},{"executorType":"org","orgId":"g1"}]'/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startCorpusBlank 首任务未设置执行者语料。
const startCorpusBlank = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="blankProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startCorpusDraft 首任务为 Draft 节点语料（表单开始时间断言）。
const startCorpusDraft = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="draftProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="Draft"/>
    <userTask id="Draft" name="草稿" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="Draft" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startCorpusNoTask 无用户任务语料（发起后直接结束）。
const startCorpusNoTask = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="noTaskProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// startEngineInstances 是 InstanceQuery 的引擎组合适配器（运行态 + 历史实例）。
type startEngineInstances struct {
	runtime api.RuntimeService // 运行态实例查询
	history api.HistoryService // 历史实例查询
}

// GetProcessInstance 查询运行中实例。
// @param ctx 操作上下文
// @param id 实例 ID
// @return 实例
// @return 错误
func (a *startEngineInstances) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	return a.runtime.GetProcessInstance(ctx, id)
}

// GetHistoricProcessInstance 查询历史实例。
// @param ctx 操作上下文
// @param processInstanceID 实例 ID
// @return 历史实例
// @return 错误
func (a *startEngineInstances) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	return a.history.GetHistoricProcessInstance(ctx, processInstanceID)
}

// newStartEnv 构造发起域服务测试环境（内存引擎 + MemStores + 注册执行人重写钩子）。
// @param t 测试上下文
// @return 引擎
// @return 内存存储聚合
// @return 发起域服务
// @return 执行人重写钩子（已注册）
func newStartEnv(t *testing.T) (api.Engine, *MemStores, *StartService, *ExecutorRewriteHook) {
	t.Helper()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	stores := NewMemStores(nil)
	meta := NewModelMetaService(eng)
	next := NewNextActivityService(stores.Next, nil)
	comments := NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService())
	operations := NewOperationRecordService(stores.Operation, eng.HistoryService(), &startEngineInstances{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), nil)
	messages := NewMessageExtendService(stores.Message, nil)
	histories := NewHistoryExtendService(stores.History, nil)
	documents := NewDocumentService(stores.Document, stores.Message, stores.Operation, &startEngineInstances{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), nil)
	start := NewStartService(eng, meta, next, comments, operations, messages, histories, documents)
	hook := NewExecutorRewriteHook(meta, next)
	hook.Register(eng)
	return eng, stores, start, hook
}

// deployStartResource 部署语料到指定租户并预热钩子缓存。
// @param t 测试上下文
// @param eng 引擎
// @param hook 执行人重写钩子
// @param tenantID 租户
// @param resource 资源名
// @param content 语料内容
// @return 流程定义 ID
func deployStartResource(t *testing.T, eng api.Engine, hook *ExecutorRewriteHook, tenantID, resource, content string) string {
	t.Helper()
	// 引擎多租户语义：部署与定义查询均按上下文租户鉴权，需注入目标租户
	deployCtx := tenant.With(context.Background(), tenantID)
	if _, err := eng.RepositoryService().Deploy(deployCtx, api.DeploymentRequest{
		Name:      resource,
		TenantID:  tenantID,
		Resources: map[string][]byte{resource: []byte(content)},
	}); err != nil {
		t.Fatalf("部署语料失败: %v", err)
	}
	definitions, err := eng.RepositoryService().ListProcessDefinitions(deployCtx)
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	definitionID := ""
	for _, definition := range definitions {
		// 按资源名定位刚部署的定义
		if definition.ResourceName == resource || definition.Key != "" && definition.ResourceName == resource {
			definitionID = definition.ID
			break
		}
	}
	if definitionID == "" {
		t.Fatalf("未找到刚部署的定义: %s", resource)
	}
	// 接线约定：部署后预热钩子元数据缓存
	if hook != nil {
		if err := hook.Warmup(context.Background(), definitionID); err != nil {
			t.Fatalf("预热元数据失败: %v", err)
		}
	}
	return definitionID
}

// seedStartIdentities 创建测试用户与组（u1 张三/u2 李四/u3 王五，组 g1 含 u2、u3）。
// @param t 测试上下文
// @param eng 引擎
func seedStartIdentities(t *testing.T, eng api.Engine) {
	t.Helper()
	ctx := tenant.With(context.Background(), "oa")
	for _, user := range []api.User{
		{ID: "u1", DisplayName: "张三"},
		{ID: "u2", DisplayName: "李四"},
		{ID: "u3", DisplayName: "王五"},
	} {
		if _, err := eng.IdentityService().CreateUser(ctx, user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	if _, err := eng.IdentityService().CreateGroup(ctx, api.Group{ID: "g1", Name: "审批组"}); err != nil {
		t.Fatalf("创建组失败: %v", err)
	}
	for _, member := range []string{"u2", "u3"} {
		if err := eng.IdentityService().AddMembership(ctx, member, "g1"); err != nil {
			t.Fatalf("添加组成员失败: %v", err)
		}
	}
}

// startVO 构造标准发起请求。
// @param key 定义 Key
// @param operator 发起人
// @return 发起请求
func startVO(key, operator string) *StartProcessInstanceVo {
	return &StartProcessInstanceVo{
		ProcessDefinitionKey: key,
		Title:                "测试标题",
		Operator:             operator,
		TenantID:             "oa",
		BusinessKey:          "biz-1",
		Remark:               "请审批",
	}
}

// TestStartParamValidation 验证五参非空校验（对位 Java 逐参 BusinessException）。
// @param t 测试上下文
func TestStartParamValidation(t *testing.T) {
	_, _, start, _ := newStartEnv(t)
	ctx := tenant.With(context.Background(), "oa")
	if _, err := start.Start(ctx, nil); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空请求应报错: %v", err)
	}
	cases := map[string]func(vo *StartProcessInstanceVo){
		"processDefinitionKey": func(vo *StartProcessInstanceVo) { vo.ProcessDefinitionKey = "" },
		"title":                func(vo *StartProcessInstanceVo) { vo.Title = "" },
		"operator":             func(vo *StartProcessInstanceVo) { vo.Operator = "" },
		"tenantId":             func(vo *StartProcessInstanceVo) { vo.TenantID = "" },
		"businessKey":          func(vo *StartProcessInstanceVo) { vo.BusinessKey = "" },
	}
	for name, mutate := range cases {
		vo := startVO("startProc", "u1")
		mutate(vo)
		if _, err := start.Start(ctx, vo); err == nil {
			t.Fatalf("参数 %s 为空应报错", name)
		}
	}
}

// TestStartNotDeployed 验证未部署（含租户隔离）时拦截并返回领域错误。
// @param t 测试上下文
func TestStartNotDeployed(t *testing.T) {
	eng, _, start, hook := newStartEnv(t)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	// 租户不存在该定义：未部署
	other := startVO("startProc", "u1")
	other.TenantID = "other"
	if _, err := start.Start(tenant.With(context.Background(), "other"), other); err == nil {
		t.Fatalf("未部署定义应报错")
	}
	// 定义 Key 不存在：未部署
	vo := startVO("startProc", "u1")
	vo.TenantID = "oa"
	vo.ProcessDefinitionKey = "missing"
	if _, err := start.Start(ctx, vo); err == nil {
		t.Fatalf("不存在的定义 Key 应报错")
	}
	// 正租户正 Key：IsProcessDeployed 直接命中
	definition, err := start.IsProcessDeployed(ctx, "oa", "startProc")
	if err != nil || definition.Key != "startProc" {
		t.Fatalf("已部署定义应可查询: %v %+v", err, definition)
	}
}

// TestStartVariableInjection 验证协议变量注入、实例命名与返回结果三元组。
// @param t 测试上下文
func TestStartVariableInjection(t *testing.T) {
	eng, _, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	vo := startVO("startProc", "u1")
	vo.Variables = map[string]any{"entity": map[string]any{"amount": 3}, "tableName": "t_order", "title": "业务自带标题"}
	result, err := start.Start(ctx, vo)
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	if result.ProcessInstanceID == "" || result.ProcessDefinitionID == "" || result.TaskID == "" || result.TaskName != "申请" || result.TaskKey != "apply" {
		t.Fatalf("发起结果不符: %+v", result)
	}
	// 实例名称=标题、发起人=operator
	instance, err := eng.RuntimeService().GetProcessInstance(ctx, result.ProcessInstanceID)
	if err != nil {
		t.Fatalf("查询实例失败: %v", err)
	}
	if instance.Name != "测试标题" || instance.StartUserID != "u1" || instance.BusinessKey != "biz-1" || instance.TenantID != "oa" {
		t.Fatalf("实例属性不符: %+v", instance)
	}
	// 协议变量注入且覆盖业务同名键；业务变量透传
	variables, err := eng.RuntimeService().GetVariables(ctx, result.ProcessInstanceID)
	if err != nil {
		t.Fatalf("查询变量失败: %v", err)
	}
	if variables[VarInitiator] != "u1" || variables[VarBusinessKey] != "biz-1" || variables[VarTitle] != "测试标题" {
		t.Fatalf("协议变量不符: %+v", variables)
	}
	if variables[VarProcessDefinitionKey] != "startProc" || variables[VarRemark] != "请审批" {
		t.Fatalf("协议变量不符: %+v", variables)
	}
	if _, ok := variables["entity"]; !ok {
		t.Fatalf("业务变量 entity 应透传: %+v", variables)
	}
	if variables[VarTableName] != "t_order" {
		t.Fatalf("tableName 应透传: %+v", variables)
	}
}

// TestStartFirstTaskInitiatorRecord 验证首任务 assignee=${initiator} 生效且首步预生成 used=1。
// @param t 测试上下文
func TestStartFirstTaskInitiatorRecord(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("startProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	task, err := eng.TaskService().GetTask(ctx, result.TaskID)
	if err != nil {
		t.Fatalf("查询首任务失败: %v", err)
	}
	// 首任务办理人 = 发起人（${initiator} 表达式经发起变量解析）
	if task.Assignee != "u1" {
		t.Fatalf("首任务办理人应为 u1: %q", task.Assignee)
	}
	// 首步预生成：used=1、taskId=首任务、执行人=发起人
	used, err := stores.Next.ListUsedByInstance(ctx, result.ProcessInstanceID)
	if err != nil || len(used) != 1 {
		t.Fatalf("首步预生成记录缺失: %v %+v", err, used)
	}
	if used[0].IdentityID != "u1" || used[0].TaskID != result.TaskID || used[0].Used != 1 || used[0].NextActivityID != "apply" {
		t.Fatalf("首步预生成内容不符: %+v", used[0])
	}
	if used[0].SendIdentityID != "u1" || used[0].SendUserName != "张三" {
		t.Fatalf("发送人信息不符: %+v", used[0])
	}
}

// TestStartCandidateUsersFirstTask 验证首任务候选人多人类解析（每人一条记录，owner 回退首位）。
// @param t 测试上下文
func TestStartCandidateUsersFirstTask(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "users.bpmn20.xml", startCorpusCandidateUsers)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("usersProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	used, _ := stores.Next.ListUsedByInstance(ctx, result.ProcessInstanceID)
	if len(used) != 2 {
		t.Fatalf("候选人应预生成两条记录: %+v", used)
	}
	ids := map[string]bool{}
	for _, record := range used {
		ids[record.IdentityID] = true
	}
	if !ids["u2"] || !ids["u3"] {
		t.Fatalf("记录执行人应为 u2/u3: %+v", used)
	}
	// 待办 owner 回退解析执行人首位
	message, found, err := stores.Message.GetByTaskID(ctx, result.TaskID)
	if err != nil || !found {
		t.Fatalf("待办扩展缺失: %v %v", err, found)
	}
	if message.OwnerID != "u2" || message.OwnerName != "李四" {
		t.Fatalf("待办 owner 回退不符: %+v", message)
	}
}

// TestStartCandidateGroupsExpansion 验证候选组经引擎身份库展开组内用户（组配对落库）。
// @param t 测试上下文
func TestStartCandidateGroupsExpansion(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "group.bpmn20.xml", startCorpusGroups)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("groupProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	used, _ := stores.Next.ListUsedByInstance(ctx, result.ProcessInstanceID)
	if len(used) != 2 {
		t.Fatalf("组展开应生成两条记录: %+v", used)
	}
	for _, record := range used {
		// 组展开的执行人应配对所属组
		if record.GroupID != "g1" || (record.IdentityID != "u2" && record.IdentityID != "u3") {
			t.Fatalf("组展开记录不符: %+v", record)
		}
	}
}

// TestStartExecutorListPriority 验证 executorList 优先于 assignee，且 org 类型经组展开。
// @param t 测试上下文
func TestStartExecutorListPriority(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "list.bpmn20.xml", startCorpusExecutorList)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("listProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	used, _ := stores.Next.ListUsedByInstance(ctx, result.ProcessInstanceID)
	// executorList 优先：u2(person) + g1 组展开(u2,u3)，模型 assignee u9 不生效
	if len(used) != 3 {
		t.Fatalf("executorList 应生成 3 条记录: %+v", used)
	}
	u9Found := false
	for _, record := range used {
		if record.IdentityID == "u9" {
			u9Found = true
		}
	}
	if u9Found {
		t.Fatalf("executorList 优先时模型 assignee 不应生效: %+v", used)
	}
}

// TestStartVariablesExecutorListFallback 验证模型全空时 vo.Variables.executorList 兜底生效。
// @param t 测试上下文
func TestStartVariablesExecutorListFallback(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "blank.bpmn20.xml", startCorpusBlank)
	ctx := tenant.With(context.Background(), "oa")
	vo := startVO("blankProc", "u1")
	vo.Variables = map[string]any{VarExecutorList: []string{"u2", "u3"}}
	if _, err := start.Start(ctx, vo); err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	instances, _ := eng.RuntimeService().ListProcessInstances(ctx)
	if len(instances) != 1 {
		t.Fatalf("应存在一个运行实例: %+v", instances)
	}
	used, _ := stores.Next.ListUsedByInstance(ctx, instances[0].ID)
	if len(used) != 2 {
		t.Fatalf("vo executorList 应生成两条记录: %+v", used)
	}
	for _, record := range used {
		if record.IdentityID != "u2" && record.IdentityID != "u3" {
			t.Fatalf("记录执行人应为 u2/u3: %+v", record)
		}
	}
}

// TestStartNoExecutorBlocks 验证执行者全空按 Java 语义报错阻断并补偿回滚实例。
// @param t 测试上下文
func TestStartNoExecutorBlocks(t *testing.T) {
	eng, _, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "blank.bpmn20.xml", startCorpusBlank)
	ctx := tenant.With(context.Background(), "oa")
	_, err := start.Start(ctx, startVO("blankProc", "u1"))
	if err == nil {
		t.Fatalf("步骤未设置执行者应报错阻断")
	}
	// 补偿回滚：实例已删除
	instances, listErr := eng.RuntimeService().ListProcessInstances(ctx)
	if listErr != nil || len(instances) != 0 {
		t.Fatalf("发起失败应补偿删除实例: %v %+v", listErr, instances)
	}
}

// TestStartNoTaskBlocks 验证定义无用户任务时报错并补偿回滚。
// @param t 测试上下文
func TestStartNoTaskBlocks(t *testing.T) {
	eng, _, start, hook := newStartEnv(t)
	deployStartResource(t, eng, hook, "oa", "notask.bpmn20.xml", startCorpusNoTask)
	ctx := tenant.With(context.Background(), "oa")
	if _, err := start.Start(ctx, startVO("noTaskProc", "u1")); err == nil {
		t.Fatalf("无用户任务定义应报错阻断")
	}
	instances, listErr := eng.RuntimeService().ListProcessInstances(ctx)
	if listErr != nil || len(instances) != 0 {
		t.Fatalf("发起失败应补偿删除实例: %v %+v", listErr, instances)
	}
}

// TestStartWritesAllExtensions 验证发起落位五件套（意见/操作记录/待办/已办镜像/表单状态）。
// @param t 测试上下文
func TestStartWritesAllExtensions(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	result, err := start.Start(ctx, startVO("startProc", "u1"))
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	// TJ 意见（引擎评论）
	comments, err := eng.TaskService().ListComments(ctx, result.TaskID)
	if err != nil || len(comments) != 1 {
		t.Fatalf("TJ 意见缺失: %v %+v", err, comments)
	}
	if comments[0].Message != "请审批" || comments[0].UserID != "u1" {
		t.Fatalf("TJ 意见内容不符: %+v", comments[0])
	}
	// 操作记录（TJ，步骤/分类补全）
	records, err := stores.Operation.ListByInstance(ctx, result.ProcessInstanceID)
	if err != nil || len(records) != 1 {
		t.Fatalf("操作记录缺失: %v %+v", err, records)
	}
	record := records[0]
	if record.Type != CommentTypeTJ || record.OwnerID != "u1" || record.TaskStepCode != "apply" || record.TaskStepName != "申请" {
		t.Fatalf("操作记录不符: %+v", record)
	}
	if record.FlowCategoryCode != "startProc" || record.BusinessID != "biz-1" || record.TenantID != "oa" {
		t.Fatalf("操作记录补全列不符: %+v", record)
	}
	// 待办扩展
	message, found, err := stores.Message.GetByTaskID(ctx, result.TaskID)
	if err != nil || !found {
		t.Fatalf("待办扩展缺失: %v %v", err, found)
	}
	if message.Title != "测试标题" || message.BusinessID != "biz-1" || message.TaskStepCode != "apply" {
		t.Fatalf("待办扩展内容不符: %+v", message)
	}
	if message.OwnerID != "u1" || message.DocumentOwnerID != "u1" || message.SenderID != "u1" || message.TenantID != "oa" {
		t.Fatalf("待办扩展人员列不符: %+v", message)
	}
	// 已办镜像
	history, found, err := stores.History.GetByTaskID(ctx, result.TaskID)
	if err != nil || !found {
		t.Fatalf("已办镜像缺失: %v %v", err, found)
	}
	if history.OwnerID != "u1" || history.TaskID != result.TaskID {
		t.Fatalf("已办镜像内容不符: %+v", history)
	}
	// 表单状态：未提交、标题、实例绑定、分类
	document, found, err := stores.Document.GetByBusinessID(ctx, "biz-1", "oa")
	if err != nil || !found {
		t.Fatalf("表单状态缺失: %v %v", err, found)
	}
	if document.FlowStatus != FlowStatusNotSubmitted || document.FlowTitle != "测试标题" || document.FlowInstanceID != result.ProcessInstanceID {
		t.Fatalf("表单状态不符: %+v", document)
	}
	if document.FlowCategoryID != "startProc" || document.DocumentTable != "" {
		t.Fatalf("表单分类不符: %+v", document)
	}
}

// TestStartDraftStepRecordsStartTime 验证首任务为 Draft 节点时表单记录流程开始时间。
// @param t 测试上下文
func TestStartDraftStepRecordsStartTime(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "draft.bpmn20.xml", startCorpusDraft)
	ctx := tenant.With(context.Background(), "oa")
	if _, err := start.Start(ctx, startVO("draftProc", "u1")); err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	document, found, err := stores.Document.GetByBusinessID(ctx, "biz-1", "oa")
	if err != nil || !found {
		t.Fatalf("表单状态缺失: %v %v", err, found)
	}
	// Draft 步骤提交记流程开始时间
	if document.FlowStartTime == nil {
		t.Fatalf("Draft 节点应记录流程开始时间: %+v", document)
	}
	if document.FlowStatus != FlowStatusNotSubmitted {
		t.Fatalf("发起后状态应为未提交: %+v", document)
	}
}

// TestStartEmptyRemarkSkipsComment 验证备注为空时跳过 TJ 意见（引擎拒绝空白评论）但操作记录照写。
// @param t 测试上下文
func TestStartEmptyRemarkSkipsComment(t *testing.T) {
	eng, stores, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "base.bpmn20.xml", startCorpusBase)
	ctx := tenant.With(context.Background(), "oa")
	vo := startVO("startProc", "u1")
	vo.Remark = ""
	result, err := start.Start(ctx, vo)
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	comments, err := eng.TaskService().ListComments(ctx, result.TaskID)
	if err != nil || len(comments) != 0 {
		t.Fatalf("空备注不应写意见: %v %+v", err, comments)
	}
	records, _ := stores.Operation.ListByInstance(ctx, result.ProcessInstanceID)
	if len(records) != 1 || records[0].Type != CommentTypeTJ {
		t.Fatalf("空备注仍应写操作记录: %+v", records)
	}
}

// TestStartGroupExpansionEmptyBlocks 验证组展开后无有效成员时按 Java 语义报错阻断。
// @param t 测试上下文
func TestStartGroupExpansionEmptyBlocks(t *testing.T) {
	eng, _, start, hook := newStartEnv(t)
	seedStartIdentities(t, eng)
	deployStartResource(t, eng, hook, "oa", "group.bpmn20.xml", startCorpusGroups)
	ctx := tenant.With(context.Background(), "oa")
	// 移除组成员后组展开为空
	if err := eng.IdentityService().RemoveMembership(ctx, "u2", "g1"); err != nil {
		t.Fatalf("移除组成员失败: %v", err)
	}
	if err := eng.IdentityService().RemoveMembership(ctx, "u3", "g1"); err != nil {
		t.Fatalf("移除组成员失败: %v", err)
	}
	if _, err := start.Start(ctx, startVO("groupProc", "u1")); err == nil {
		t.Fatalf("组展开为空应报错阻断")
	}
}

// TestResolveExecutorExpression 验证执行者表达式解析（${变量}/裸变量/字面量）。
// @param t 测试上下文
func TestResolveExecutorExpression(t *testing.T) {
	variables := map[string]any{"initiator": "u1", "count": 3, "owner": "u2"}
	cases := []struct {
		raw      string // 原始表达式
		expected string // 期望结果
	}{
		{"${initiator}", "u1"},
		{"initiator", "u1"},
		{"${owner}", "u2"},
		{"${count}", "3"},
		{"u5", "u5"},
		{"${missing}", ""},
		{"", ""},
	}
	for _, item := range cases {
		if got := resolveExecutorExpression(item.raw, variables); got != item.expected {
			t.Fatalf("表达式 %q 解析不符: got %q want %q", item.raw, got, item.expected)
		}
	}
}

// TestExecutorConfigsFromVariable 验证 vo 变量 executorList 的多形态归一。
// @param t 测试上下文
func TestExecutorConfigsFromVariable(t *testing.T) {
	// []string 形态
	configs := executorConfigsFromVariable([]string{"u1", "u2"})
	if len(configs) != 2 || configs[0].IdentityID != "u1" || configs[0].ExecutorType != "person" {
		t.Fatalf("[]string 归一不符: %+v", configs)
	}
	// []any 字符串形态
	configs = executorConfigsFromVariable([]any{"u3", "u4"})
	if len(configs) != 2 || configs[1].IdentityID != "u4" {
		t.Fatalf("[]any 字符串归一不符: %+v", configs)
	}
	// []any 映射形态
	configs = executorConfigsFromVariable([]any{map[string]any{"identityId": "u5"}})
	if len(configs) != 1 || configs[0].IdentityID != "u5" {
		t.Fatalf("[]any 映射归一不符: %+v", configs)
	}
	// JSON 字符串形态
	configs = executorConfigsFromVariable(`[{"identityId":"u6"}]`)
	if len(configs) != 1 || configs[0].IdentityID != "u6" {
		t.Fatalf("JSON 串归一不符: %+v", configs)
	}
	// 逗号串形态
	configs = executorConfigsFromVariable("u7, u8")
	if len(configs) != 2 || configs[1].IdentityID != "u8" {
		t.Fatalf("逗号串归一不符: %+v", configs)
	}
	// 未知形态
	if configs := executorConfigsFromVariable(42); configs != nil {
		t.Fatalf("未知形态应返回 nil: %+v", configs)
	}
}
