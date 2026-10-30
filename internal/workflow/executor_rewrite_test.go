// executor_rewrite_test.go 执行人重写钩子单元测试（P1-D）。
// 覆盖：门控变量缺失不触发（负向）、非会签单人/多人/组回填、无记录兜底保留模型默认、
// 会签并行 2 实例按序消费 2 条且办理人各归位、组会签匹配、used=1+taskId 回写、
// 钩子查询失败导致引擎命令回滚（任务消失）。
package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// rewriteFixture 是执行人重写测试语料：三个独立流程分别覆盖非会签、人会签、组会签。
const rewriteFixturePlain = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="rewriteProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="next"/>
    <userTask id="next" name="下一步" flowable:assignee="u9"/>
    <sequenceFlow id="f2" sourceRef="next" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// rewriteFixtureCountersign 是人员会签语料：集合变量 executorList，元素变量 signer 驱动办理人。
const rewriteFixtureCountersign = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="countersignProc" isExecutable="true">
    <startEvent id="csStart"/>
    <sequenceFlow id="cf0" sourceRef="csStart" targetRef="cs"/>
    <userTask id="cs" name="会签" flowable:assignee="${signer}" executorType="person">
      <multiInstanceLoopCharacteristics isSequential="false"
          flowable:collection="${executorList}" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="cf1" sourceRef="cs" targetRef="csEnd"/>
    <endEvent id="csEnd"/>
  </process>
</definitions>`

// rewriteFixtureGroupCountersign 是组会签语料：集合变量 groupList，元素变量 grpVar 驱动候选组。
const rewriteFixtureGroupCountersign = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="groupCountersignProc" isExecutable="true">
    <startEvent id="gsStart"/>
    <sequenceFlow id="gf0" sourceRef="gsStart" targetRef="gs"/>
    <userTask id="gs" name="组会签" executorType="group" flowable:candidateGroups="${grpVar}">
      <multiInstanceLoopCharacteristics isSequential="false"
          flowable:collection="${groupList}" flowable:elementVariable="grpVar"/>
    </userTask>
    <sequenceFlow id="gf1" sourceRef="gs" targetRef="gsEnd"/>
    <endEvent id="gsEnd"/>
  </process>
</definitions>`

// deployRewriteResource 部署单个语料资源并返回流程定义 ID（可选附带钩子元数据预热）。
// @param t 测试上下文
// @param eng 引擎
// @param hook 执行人重写钩子（非 nil 时部署后预热该定义缓存）
// @param resource 资源名
// @param content 语料内容
// @return 流程定义 ID
func deployRewriteResource(t *testing.T, eng api.Engine, hook *ExecutorRewriteHook, resource, content string) string {
	t.Helper()
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:      resource,
		Resources: map[string][]byte{resource: []byte(content)},
	}); err != nil {
		t.Fatalf("部署语料失败: %v", err)
	}
	definitions, err := eng.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	definitionID := ""
	for _, definition := range definitions {
		// 按部署名定位刚部署的定义（每次部署独立，避免多流程同文件干扰）
		if strings.HasSuffix(definition.ResourceName, resource) || definition.ResourceName == resource {
			definitionID = definition.ID
			break
		}
	}
	if definitionID == "" {
		t.Fatalf("未找到刚部署的定义: %s", resource)
	}
	// 钩子元数据预热（接线约定：命令外预热，命令内只读缓存）
	if hook != nil {
		if err := hook.Warmup(context.Background(), definitionID); err != nil {
			t.Fatalf("预热元数据失败: %v", err)
		}
	}
	return definitionID
}

// fakeHookCtl 是 api.HookControl 的测试桩（记录改写调用）。
type fakeHookCtl struct {
	assigneeTaskID string   // SetAssignee 的任务 ID
	assignee       *string  // SetAssignee 的办理人（指针区分未调用/空串清空）
	candidateUsers []string // SetCandidateUsers 记录
	candidateGrps  []string // SetCandidateGroups 记录
	variables      map[string]any
	deferred       []func(ctx context.Context) error
	failSet        string // 非空时对应方法返回错误
}

// SetAssignee 记录办理人改写。
// @param taskID 任务 ID
// @param userID 用户 ID
// @return 错误
func (f *fakeHookCtl) SetAssignee(taskID, userID string) error {
	// 桩故障注入：按方法名返回错误
	if f.failSet == "assignee" {
		return api.NewError("FAKE-101", "改写办理人失败", nil)
	}
	f.assigneeTaskID = taskID
	copied := userID
	f.assignee = &copied
	return nil
}

// SetCandidateUsers 记录候选用户重写。
// @param taskID 任务 ID
// @param ids 候选用户列表
// @return 错误
func (f *fakeHookCtl) SetCandidateUsers(taskID string, ids []string) error {
	// 桩故障注入：按方法名返回错误
	if f.failSet == "users" {
		return api.NewError("FAKE-102", "重写候选用户失败", nil)
	}
	f.candidateUsers = append([]string(nil), ids...)
	return nil
}

// SetCandidateGroups 记录候选组重写。
// @param taskID 任务 ID
// @param ids 候选组列表
// @return 错误
func (f *fakeHookCtl) SetCandidateGroups(taskID string, ids []string) error {
	// 桩故障注入：按方法名返回错误
	if f.failSet == "groups" {
		return api.NewError("FAKE-103", "重写候选组失败", nil)
	}
	f.candidateGrps = append([]string(nil), ids...)
	return nil
}

// SetVariable 记录变量写入。
// @param name 变量名
// @param value 变量值
// @return 错误
func (f *fakeHookCtl) SetVariable(name string, value any) error {
	f.variables[name] = value
	return nil
}

// Defer 记录延迟动作。
// @param action 延迟动作
func (f *fakeHookCtl) Defer(action func(ctx context.Context) error) {
	f.deferred = append(f.deferred, action)
}

// newRewriteHookEnv 构造钩子单测环境：真实引擎（模型元数据）+ 内存台账。
// @param t 测试上下文
// @return 引擎
// @return 钩子
// @return 下一步域服务
// @return 台账存储（直接插数用）
func newRewriteHookEnv(t *testing.T) (api.Engine, *ExecutorRewriteHook, *NextActivityService, *MemNextStore) {
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
	store := NewMemNextStore(nil)
	next := NewNextActivityService(store, nil)
	hook := NewExecutorRewriteHook(NewModelMetaService(eng), next)
	return eng, hook, next, store
}

// insertRewriteRecord 直插一条预生成记录（绕过服务校验，支持无执行人的组记录）。
// @param t 测试上下文
// @param store 台账存储
// @param instanceID 实例 ID
// @param activityID 节点 ID
// @param identityID 执行人 Id
// @param groupID 候选组 Id
// @param version 版本号
// @return 记录 Id
func insertRewriteRecord(t *testing.T, store *MemNextStore, instanceID, activityID, identityID, groupID string, version int) string {
	t.Helper()
	record := &NextActivity{
		BaseColumns:       BaseColumns{ID: "rec-" + identityID + groupID + "-" + activityID + "-" + instanceID},
		NextActivityID:    activityID,
		IdentityID:        identityID,
		GroupID:           groupID,
		ProcessInstanceID: instanceID,
		Used:              0,
		Version:           version,
	}
	if err := store.InsertDual(context.Background(), record); err != nil {
		t.Fatalf("插入预生成记录失败: %v", err)
	}
	return record.ID
}

// rewriteTaskSnapshot 构造任务快照。
// @param taskID 任务 ID
// @param instanceID 实例 ID
// @param definitionID 定义 ID
// @param activityID 节点 ID
// @param assignee 办理人
// @param variables 任务可见变量
// @return 任务快照
func rewriteTaskSnapshot(taskID, instanceID, definitionID, activityID, assignee string, variables map[string]any) *api.Task {
	return &api.Task{
		ID:                  taskID,
		ProcessInstanceID:   instanceID,
		ProcessDefinitionID: definitionID,
		TaskDefinitionKey:   activityID,
		Assignee:            assignee,
		Variables:           variables,
	}
}

// TestRewriteHookGateSkipped 验证门控变量缺失或非法时钩子不执行任何改写（负向）。
// @param t 测试上下文
func TestRewriteHookGateSkipped(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-1", "next", "u2", "", 1)
	ctl := &fakeHookCtl{}
	// operationType 缺失
	task := rewriteTaskSnapshot("t1", "pi-1", definitionID, "next", "u9", nil)
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("门控缺失时钩子应静默通过: %v", err)
	}
	// operationType 非法值
	task.Variables = map[string]any{VarOperationType: "back"}
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("门控非法值时钩子应静默通过: %v", err)
	}
	// 断言：无改写调用，记录未被消耗
	if ctl.assignee != nil || ctl.candidateUsers != nil || ctl.candidateGrps != nil {
		t.Fatalf("门控未命中不应改写任务: %+v", ctl)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-1", "next")
	if len(rows) != 1 || rows[0].Used != 0 {
		t.Fatalf("门控未命中不应消耗记录: %+v", rows)
	}
	// agreeComplete 命中门控（正向对照）
	task.Variables = map[string]any{VarOperationType: OperationTypeAgreeComplete}
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("门控命中应执行重写: %v", err)
	}
	if ctl.assignee == nil || *ctl.assignee != "u2" {
		t.Fatalf("门控命中应改写办理人为 u2: %v", ctl.assignee)
	}
}

// TestRewritePlainSingleAssignee 验证非会签单条记录回填办理人并消耗记录。
// @param t 测试上下文
func TestRewritePlainSingleAssignee(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-2", "next", "u2", "", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t2", "pi-2", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("钩子执行失败: %v", err)
	}
	// 办理人回填为记录执行人
	if ctl.assigneeTaskID != "t2" || ctl.assignee == nil || *ctl.assignee != "u2" {
		t.Fatalf("应回填办理人 u2: taskID=%s assignee=%v", ctl.assigneeTaskID, ctl.assignee)
	}
	// 记录消耗：used=1 且 taskId 回写
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-2", "next")
	if len(rows) != 0 {
		t.Fatalf("记录应被消耗: %+v", rows)
	}
	used, _ := store.ListUsedByInstance(ctx, "pi-2")
	if len(used) != 1 || used[0].Used != 1 || used[0].TaskID != "t2" {
		t.Fatalf("记录应回写 used=1 与 taskId: %+v", used)
	}
}

// TestRewritePlainMultiCandidates 验证非会签多条记录整体重写候选用户并清空默认办理人。
// @param t 测试上下文
func TestRewritePlainMultiCandidates(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-3", "next", "u2", "", 1)
	insertRewriteRecord(t, store, "pi-3", "next", "u3", "", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t3", "pi-3", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeCompleteToTarget})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("钩子执行失败: %v", err)
	}
	// 多人语义：候选用户整体重写 + 清空默认办理人
	if len(ctl.candidateUsers) != 2 || ctl.candidateUsers[0] != "u2" || ctl.candidateUsers[1] != "u3" {
		t.Fatalf("应回填候选用户 [u2 u3]: %v", ctl.candidateUsers)
	}
	if ctl.assignee == nil || *ctl.assignee != "" {
		t.Fatalf("多人语义应清空默认办理人: %v", ctl.assignee)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-3", "next")
	if len(rows) != 0 {
		t.Fatalf("两条记录都应被消耗: %+v", rows)
	}
}

// TestRewritePlainGroupRecords 验证仅带候选组的记录回填候选组。
// @param t 测试上下文
func TestRewritePlainGroupRecords(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-4", "next", "", "g1", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t4", "pi-4", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("钩子执行失败: %v", err)
	}
	if len(ctl.candidateGrps) != 1 || ctl.candidateGrps[0] != "g1" {
		t.Fatalf("应回填候选组 [g1]: %v", ctl.candidateGrps)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-4", "next")
	if len(rows) != 0 {
		t.Fatalf("记录应被消耗: %+v", rows)
	}
}

// TestRewritePlainNoRecordsFallback 验证无未消耗记录时兜底保留模型默认执行人。
// @param t 测试上下文
func TestRewritePlainNoRecordsFallback(t *testing.T) {
	eng, hook, _, _ := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t5", "pi-5", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(context.Background(), task, ctl); err != nil {
		t.Fatalf("无记录时钩子应静默通过: %v", err)
	}
	// 断言：模型默认执行人 u9 未被改写
	if ctl.assignee != nil || ctl.candidateUsers != nil || ctl.candidateGrps != nil {
		t.Fatalf("无记录应保留模型默认执行人: %+v", ctl)
	}
}

// TestRewritePlainDirtyRecordsFallback 验证记录执行人与组全空（脏数据）时兜底不消费。
// @param t 测试上下文
func TestRewritePlainDirtyRecordsFallback(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-6", "next", "", "", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t6", "pi-6", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("脏记录时钩子应静默通过: %v", err)
	}
	if ctl.assignee != nil {
		t.Fatalf("脏记录不应改写执行人: %v", ctl.assignee)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-6", "next")
	if len(rows) != 1 || rows[0].Used != 0 {
		t.Fatalf("脏记录不应被消费: %+v", rows)
	}
}

// TestRewriteCountersignPersonTwoInstances 验证并行会签 2 实例各消费 1 条且办理人各归位。
// @param t 测试上下文
func TestRewriteCountersignPersonTwoInstances(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctx := context.Background()
	// 预生成 2 条会签执行人记录（版本升序）
	insertRewriteRecord(t, store, "pi-7", "cs", "u1", "", 1)
	insertRewriteRecord(t, store, "pi-7", "cs", "u2", "", 1)
	// 实例 1：办理人 u1（元素变量驱动），应匹配同身份记录并消耗
	ctl1 := &fakeHookCtl{}
	task1 := rewriteTaskSnapshot("t7-1", "pi-7", definitionID, "cs", "u1",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task1, ctl1); err != nil {
		t.Fatalf("实例 1 钩子失败: %v", err)
	}
	// 实例 2：办理人 u2，此时 u1 的记录已消耗，只能匹配 u2 记录
	ctl2 := &fakeHookCtl{}
	task2 := rewriteTaskSnapshot("t7-2", "pi-7", definitionID, "cs", "u2",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task2, ctl2); err != nil {
		t.Fatalf("实例 2 钩子失败: %v", err)
	}
	// 各实例候选用户归位（逐人 SetCandidateUsers）
	if len(ctl1.candidateUsers) != 1 || ctl1.candidateUsers[0] != "u1" {
		t.Fatalf("实例 1 应回填候选人 u1: %v", ctl1.candidateUsers)
	}
	if len(ctl2.candidateUsers) != 1 || ctl2.candidateUsers[0] != "u2" {
		t.Fatalf("实例 2 应回填候选人 u2: %v", ctl2.candidateUsers)
	}
	// 每实例恰好消耗一条，taskId 各自回写
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-7", "cs")
	if len(rows) != 0 {
		t.Fatalf("2 实例应消费全部 2 条记录: %+v", rows)
	}
	used, _ := store.ListUsedByInstance(ctx, "pi-7")
	if len(used) != 2 {
		t.Fatalf("消耗后应有 2 条已用记录: %+v", used)
	}
	taskIDs := map[string]bool{}
	for _, record := range used {
		taskIDs[record.TaskID] = true
	}
	if !taskIDs["t7-1"] || !taskIDs["t7-2"] {
		t.Fatalf("消耗记录应分别回写两个实例任务 ID: %+v", used)
	}
}

// TestRewriteCountersignPersonNoAssigneeOrder 验证实例无办理人时按序消费第一条记录。
// @param t 测试上下文
func TestRewriteCountersignPersonNoAssigneeOrder(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-8", "cs", "u1", "", 1)
	insertRewriteRecord(t, store, "pi-8", "cs", "u2", "", 1)
	// 实例未解析出办理人：按序消费 u1 记录
	ctl1 := &fakeHookCtl{}
	task1 := rewriteTaskSnapshot("t8-1", "pi-8", definitionID, "cs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task1, ctl1); err != nil {
		t.Fatalf("实例 1 钩子失败: %v", err)
	}
	if len(ctl1.candidateUsers) != 1 || ctl1.candidateUsers[0] != "u1" {
		t.Fatalf("无办理人实例应按序回填 u1: %v", ctl1.candidateUsers)
	}
	// 实例 2 继续按序消费 u2 记录
	ctl2 := &fakeHookCtl{}
	task2 := rewriteTaskSnapshot("t8-2", "pi-8", definitionID, "cs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task2, ctl2); err != nil {
		t.Fatalf("实例 2 钩子失败: %v", err)
	}
	if len(ctl2.candidateUsers) != 1 || ctl2.candidateUsers[0] != "u2" {
		t.Fatalf("实例 2 应按序回填 u2: %v", ctl2.candidateUsers)
	}
}

// TestRewriteCountersignGroup 验证组会签按任务第一候选组匹配记录并回填。
// @param t 测试上下文
func TestRewriteCountersignGroup(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "group-cs.bpmn20.xml", rewriteFixtureGroupCountersign)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-9", "gs", "u5", "g1", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t9", "pi-9", definitionID, "gs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	task.CandidateGroups = []string{"g1"}
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("组会签钩子失败: %v", err)
	}
	if len(ctl.candidateGrps) != 1 || ctl.candidateGrps[0] != "g1" {
		t.Fatalf("应回填候选组 g1: %v", ctl.candidateGrps)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-9", "gs")
	if len(rows) != 0 {
		t.Fatalf("组记录应被消耗: %+v", rows)
	}
	used, _ := store.ListUsedByInstance(ctx, "pi-9")
	if len(used) != 1 || used[0].TaskID != "t9" {
		t.Fatalf("组记录应回写 taskId: %+v", used)
	}
}

// TestRewriteCountersignNoRecordsFallback 验证会签节点无未消耗记录时兜底保留模型默认。
// @param t 测试上下文
func TestRewriteCountersignNoRecordsFallback(t *testing.T) {
	eng, hook, _, _ := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t10", "pi-10", definitionID, "cs", "u1",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(context.Background(), task, ctl); err != nil {
		t.Fatalf("无记录时钩子应静默通过: %v", err)
	}
	if ctl.candidateUsers != nil || ctl.assignee != nil {
		t.Fatalf("会签无记录应保留模型默认: %+v", ctl)
	}
}

// TestRewriteCountersignAssigneeMissKeepsModel 验证实例已有办理人但记录未覆盖时不误消费他人记录。
// @param t 测试上下文
func TestRewriteCountersignAssigneeMissKeepsModel(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-11", "cs", "u1", "", 1)
	ctl := &fakeHookCtl{}
	// 实例办理人 u9 不在任何记录中：不应消费 u1 的记录
	task := rewriteTaskSnapshot("t11", "pi-11", definitionID, "cs", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("钩子应静默通过: %v", err)
	}
	if ctl.candidateUsers != nil {
		t.Fatalf("办理人未命中不应回填他人: %v", ctl.candidateUsers)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-11", "cs")
	if len(rows) != 1 || rows[0].Used != 0 {
		t.Fatalf("记录不应被误消费: %+v", rows)
	}
}

// hookFailingNextStore 是 ListUnusedByInstanceAndActivity 故障注入桩（覆盖钩子错误回滚路径）。
type hookFailingNextStore struct {
	MemNextStore // 复用内存实现，仅覆盖查询方法
}

// ListUnusedByInstanceAndActivity 注入查询失败。
// @param ctx 操作上下文
// @param processInstanceID 实例 Id
// @param nextActivityID 节点 Id
// @return 记录列表
// @return 错误
func (f *hookFailingNextStore) ListUnusedByInstanceAndActivity(ctx context.Context, processInstanceID, nextActivityID string) ([]*NextActivity, error) {
	return nil, api.NewError("FAKE-104", "查询预生成记录失败", nil)
}

// TestRewriteHookErrorRollsBackCommand 验证钩子查询失败使办结命令整体回滚（任务消失）。
// @param t 测试上下文
func TestRewriteHookErrorRollsBackCommand(t *testing.T) {
	eng, hook, next, _ := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, nil, "plain.bpmn20.xml", rewriteFixturePlain)
	// 换用故障台账构造钩子并注册：查询失败必须让命令回滚
	broken := NewExecutorRewriteHook(NewModelMetaService(eng), NewNextActivityService(&hookFailingNextStore{MemNextStore: *NewMemNextStore(nil)}, nil))
	broken.Register(eng)
	_ = next
	_ = hook
	ctx := context.Background()
	// 预热故障钩子的元数据缓存（Warmup 走模型服务不受故障台账影响），使命中门控后能到达台账查询
	if err := broken.Warmup(ctx, definitionID); err != nil {
		t.Fatalf("预热失败: %v", err)
	}
	// 发起流程（首任务 apply，发起命令内门控未命中不触发钩子）
	instance, err := eng.RuntimeService().StartProcessInstance(ctx, api.StartRequest{
		ProcessDefinitionKey: "rewriteProc",
		BusinessKey:          "biz-rollback",
		TenantID:             "",
		UserID:               "u1",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "u1"}},
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	// 办结首任务并携带门控变量：下一任务创建触发钩子 → 查询失败 → 整体回滚
	err = eng.TaskService().Complete(ctx, firstTaskIDOf(t, eng, instance.ID), api.CompleteTaskRequest{
		UserID:    "u1",
		Variables: []api.Variable{{Name: VarOperationType, Value: OperationTypeAgreeComplete}},
	})
	if err == nil {
		t.Fatalf("钩子失败应使命令回滚")
	}
	// 回滚断言：apply 任务仍在（办结被回滚），next 任务未创建
	page, listErr := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if listErr != nil {
		t.Fatalf("查询任务失败: %v", listErr)
	}
	if page.Total != 1 || page.Data[0].TaskDefinitionKey != "apply" {
		t.Fatalf("命令应整体回滚（apply 仍在、next 未创建）: total=%d", page.Total)
	}
}

// firstTaskIDOf 查询实例的第一个任务 ID。
// @param t 测试上下文
// @param eng 引擎
// @param instanceID 实例 ID
// @return 任务 ID
func firstTaskIDOf(t *testing.T, eng api.Engine, instanceID string) string {
	t.Helper()
	page, err := eng.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instanceID})
	if err != nil || page.Total == 0 {
		t.Fatalf("查询实例任务失败: %v", err)
	}
	return page.Data[0].ID
}

// TestRewriteHookNilTask 验证空任务快照的防御分支。
// @param t 测试上下文
func TestRewriteHookNilTask(t *testing.T) {
	_, hook, _, _ := newRewriteHookEnv(t)
	if err := hook.Hook(context.Background(), nil, &fakeHookCtl{}); err != nil {
		t.Fatalf("空任务应静默通过: %v", err)
	}
}

// TestRewriteHookCtlErrorPropagation 验证 ctl 改写失败错误向上传播（命令回滚由引擎保证）。
// @param t 测试上下文
func TestRewriteHookCtlErrorPropagation(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-12", "next", "u2", "", 1)
	ctl := &fakeHookCtl{failSet: "assignee"}
	task := rewriteTaskSnapshot("t12", "pi-12", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err == nil {
		t.Fatalf("ctl 改写失败应向上传播错误")
	}
	// 改写失败发生在消耗之前：记录保持未消耗
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-12", "next")
	if len(rows) != 1 || rows[0].Used != 0 {
		t.Fatalf("改写失败时记录不应被消耗: %+v", rows)
	}
}

// TestRewriteHookMarkUsedErrorPropagation 验证消耗回写失败错误向上传播。
// @param t 测试上下文
func TestRewriteHookMarkUsedErrorPropagation(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-13", "next", "u2", "", 1)
	ctl := &fakeHookCtl{failSet: "users"}
	// 多条记录触发候选用户重写路径，注入重写失败
	insertRewriteRecord(t, store, "pi-13", "next", "u3", "", 1)
	task := rewriteTaskSnapshot("t13", "pi-13", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err == nil {
		t.Fatalf("候选用户重写失败应向上传播错误")
	}
	_ = store
}

// TestRewriteHookWarmupAll 验证全量预热：已部署定义的节点元数据全部进入只读缓存。
// @param t 测试上下文
func TestRewriteHookWarmupAll(t *testing.T) {
	eng, hook, _, _ := newRewriteHookEnv(t)
	deployRewriteResource(t, eng, nil, "plain.bpmn20.xml", rewriteFixturePlain)
	definitions, err := eng.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil || len(definitions) == 0 {
		t.Fatalf("查询定义失败: %v", err)
	}
	// 全量预热前缓存未命中
	if hook.cachedUserTaskMeta(definitions[0].ID, "next") != nil {
		t.Fatalf("预热前缓存不应命中")
	}
	if err := hook.WarmupAll(context.Background()); err != nil {
		t.Fatalf("全量预热失败: %v", err)
	}
	// 全量预热后节点元数据命中且含会签/执行者摘要
	meta := hook.cachedUserTaskMeta(definitions[0].ID, "next")
	if meta == nil || meta.Assignee != "u9" {
		t.Fatalf("全量预热后应命中节点元数据: %+v", meta)
	}
	// 未部署定义的节点保持未命中（降级兜底路径）
	if hook.cachedUserTaskMeta(definitions[0].ID, "missing") != nil {
		t.Fatalf("未知节点不应命中")
	}
}

// rewriteFixtureCountersignDefault 是缺省执行者类型的会签语料（无 executorType/executorList 扩展）。
const rewriteFixtureCountersignDefault = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="defaultCSProc" isExecutable="true">
    <startEvent id="dcsStart"/>
    <sequenceFlow id="dcsf0" sourceRef="dcsStart" targetRef="dcs"/>
    <userTask id="dcs" name="缺省会签" flowable:assignee="${signer}"
              executorList='[{"identityId":"u1","executorType":"person"}]'>
      <multiInstanceLoopCharacteristics isSequential="false"
          flowable:collection="${executorList}" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="dcsf1" sourceRef="dcs" targetRef="dcsEnd"/>
    <endEvent id="dcsEnd"/>
  </process>
</definitions>`

// failingMetaRepo 是 RepositoryService 故障注入桩（预热失败路径覆盖）。
type failingMetaRepo struct {
	api.RepositoryService      // 仅覆写被预热路径调用的方法，其余不触达
	failModel             bool // GetBpmnModel 失败开关
	failList              bool // ListProcessDefinitions 失败开关
}

// GetBpmnModel 按开关返回模型查询失败。
// @param ctx 上下文
// @param definitionID 定义 ID
// @return BPMN 模型
// @return 错误
func (f *failingMetaRepo) GetBpmnModel(ctx context.Context, definitionID string) (*model.BpmnModel, error) {
	// 故障注入：模拟模型查询失败
	if f.failModel {
		return nil, api.NewError("FAKE-201", "模型查询失败", nil)
	}
	return &model.BpmnModel{}, nil
}

// ListProcessDefinitions 按开关返回定义列表查询失败。
// @param ctx 上下文
// @return 定义列表
// @return 错误
func (f *failingMetaRepo) ListProcessDefinitions(ctx context.Context) ([]api.ProcessDefinition, error) {
	// 故障注入：模拟定义列表查询失败
	if f.failList {
		return nil, api.NewError("FAKE-202", "定义列表查询失败", nil)
	}
	// 正常路径返回单定义，供 WarmupAll 循环触达单定义预热
	return []api.ProcessDefinition{{ID: "def-1"}}, nil
}

// failingMetaEngine 是仅提供故障 RepositoryService 的引擎桩（预热失败路径覆盖）。
type failingMetaEngine struct {
	api.Engine // 其余服务不触达
	repo       *failingMetaRepo
}

// RepositoryService 返回故障仓库服务。
// @return 仓库服务
func (f *failingMetaEngine) RepositoryService() api.RepositoryService {
	return f.repo
}

// TestRewriteWarmupErrorPaths 验证预热失败路径（模型/定义列表查询错误向上传播）。
// @param t 测试上下文
func TestRewriteWarmupErrorPaths(t *testing.T) {
	repo := &failingMetaRepo{failModel: true}
	hook := NewExecutorRewriteHook(NewModelMetaService(&failingMetaEngine{repo: repo}), NewNextActivityService(NewMemNextStore(nil), nil))
	// Warmup：模型查询失败
	if err := hook.Warmup(context.Background(), "def-1"); err == nil {
		t.Fatalf("模型查询失败时预热应报错")
	}
	// WarmupAll：定义列表查询失败
	repo.failModel = false
	repo.failList = true
	if err := hook.WarmupAll(context.Background()); err == nil {
		t.Fatalf("定义列表查询失败时全量预热应报错")
	}
	// WarmupAll：单定义预热失败
	repo.failList = false
	repo.failModel = true
	if err := hook.WarmupAll(context.Background()); err == nil {
		t.Fatalf("单定义预热失败时全量预热应报错")
	}
}

// TestRewriteHookCacheMissDefersWarmup 验证缓存未命中时降级兜底并登记延迟预热。
// @param t 测试上下文
func TestRewriteHookCacheMissDefersWarmup(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, nil, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-20", "next", "u2", "", 1)
	ctl := &fakeHookCtl{}
	// 未预热直接触发门控命中的钩子：降级保留模型默认并登记延迟预热
	task := rewriteTaskSnapshot("t20", "pi-20", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("缓存未命中应降级静默通过: %v", err)
	}
	if ctl.assignee != nil {
		t.Fatalf("缓存未命中不应改写执行人: %v", ctl.assignee)
	}
	if len(ctl.deferred) != 1 {
		t.Fatalf("缓存未命中应登记延迟预热: %d", len(ctl.deferred))
	}
	// 模拟命令提交后执行延迟动作：预热完成，缓存命中
	if err := ctl.deferred[0](ctx); err != nil {
		t.Fatalf("延迟预热失败: %v", err)
	}
	if hook.cachedUserTaskMeta(definitionID, "next") == nil {
		t.Fatalf("延迟预热后缓存应命中")
	}
}

// TestRewritePlainCtlErrorsAllBranches 验证非会签各改写分支的 ctl 错误传播。
// @param t 测试上下文
func TestRewritePlainCtlErrorsAllBranches(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "plain.bpmn20.xml", rewriteFixturePlain)
	ctx := context.Background()
	// 多人分支：清空默认办理人（SetAssignee("")）失败
	insertRewriteRecord(t, store, "pi-21", "next", "u2", "", 1)
	insertRewriteRecord(t, store, "pi-21", "next", "u3", "", 1)
	ctl := &fakeHookCtl{failSet: "assignee"}
	task := rewriteTaskSnapshot("t21", "pi-21", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err == nil {
		t.Fatalf("多人分支清空办理人失败应向上传播")
	}
	// 组回填分支：记录附带候选组时 SetCandidateGroups 失败
	insertRewriteRecord(t, store, "pi-21", "next", "u2", "g1", 1)
	ctl2 := &fakeHookCtl{failSet: "groups"}
	task2 := rewriteTaskSnapshot("t22", "pi-21", definitionID, "next", "u9",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task2, ctl2); err == nil {
		t.Fatalf("组回填失败应向上传播")
	}
	_ = task2
}

// TestRewriteCountersignOrderingComparator 验证会签消费顺序三级比较（版本/创建时间/Id）。
// @param t 测试上下文
func TestRewriteCountersignOrderingComparator(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctx := context.Background()
	// 记录带显式创建时间与不同版本：覆盖"版本优先"与"创建时间次之"比较分支
	insertRewriteRecordAt(t, store, "pi-23", "cs", "uB", "", 2, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	insertRewriteRecordAt(t, store, "pi-23", "cs", "uA", "", 1, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	// 实例无办理人：按序消费版本 1 的 uA 记录
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t23-1", "pi-23", definitionID, "cs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("实例 1 钩子失败: %v", err)
	}
	if len(ctl.candidateUsers) != 1 || ctl.candidateUsers[0] != "uA" {
		t.Fatalf("版本 1 记录应先被消费: %v", ctl.candidateUsers)
	}
	// 固定时钟（同版本同创建时间）：Id 终局排序分支
	fakeStore := NewMemNextStore(clock.NewFake(time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)))
	hook2 := NewExecutorRewriteHook(NewModelMetaService(eng), NewNextActivityService(fakeStore, nil))
	if err := hook2.Warmup(ctx, definitionID); err != nil {
		t.Fatalf("预热失败: %v", err)
	}
	insertRewriteRecordAt(t, fakeStore, "pi-24", "cs", "u2", "", 1, time.Time{})
	insertRewriteRecordAt(t, fakeStore, "pi-24", "cs", "u1", "", 1, time.Time{})
	ctl2 := &fakeHookCtl{}
	task2 := rewriteTaskSnapshot("t24", "pi-24", definitionID, "cs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook2.Hook(ctx, task2, ctl2); err != nil {
		t.Fatalf("固定时钟钩子失败: %v", err)
	}
	// 同版本同创建时间按 Id 升序：rec-u1 < rec-u2
	if len(ctl2.candidateUsers) != 1 || ctl2.candidateUsers[0] != "u1" {
		t.Fatalf("同批次应按 Id 升序消费 u1: %v", ctl2.candidateUsers)
	}
}

// insertRewriteRecordAt 直插一条带显式创建时间的预生成记录。
// @param t 测试上下文
// @param store 台账存储
// @param instanceID 实例 ID
// @param activityID 节点 ID
// @param identityID 执行人 Id
// @param groupID 候选组 Id
// @param version 版本号
// @param createTime 创建时间
// @return 记录 Id
func insertRewriteRecordAt(t *testing.T, store *MemNextStore, instanceID, activityID, identityID, groupID string, version int, createTime time.Time) string {
	t.Helper()
	record := &NextActivity{
		BaseColumns:       BaseColumns{ID: "rec-" + identityID + groupID + "-" + activityID + "-" + instanceID, CreateTime: createTime},
		NextActivityID:    activityID,
		IdentityID:        identityID,
		GroupID:           groupID,
		ProcessInstanceID: instanceID,
		Used:              0,
		Version:           version,
	}
	if err := store.InsertDual(context.Background(), record); err != nil {
		t.Fatalf("插入预生成记录失败: %v", err)
	}
	return record.ID
}

// TestRewriteCountersignDefaultExecutorType 验证缺省执行者类型回退 person（首项类型为空）。
// @param t 测试上下文
func TestRewriteCountersignDefaultExecutorType(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "default-cs.bpmn20.xml", rewriteFixtureCountersignDefault)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-25", "dcs", "u1", "", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t25", "pi-25", definitionID, "dcs", "u1",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("钩子失败: %v", err)
	}
	// 缺省按 person 消费：匹配同身份记录并回填候选人
	if len(ctl.candidateUsers) != 1 || ctl.candidateUsers[0] != "u1" {
		t.Fatalf("缺省 person 类型应回填候选人 u1: %v", ctl.candidateUsers)
	}
}

// TestRewriteCountersignPersonCtlError 验证会签按人分支候选回写失败错误传播。
// @param t 测试上下文
func TestRewriteCountersignPersonCtlError(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctx := context.Background()
	insertRewriteRecord(t, store, "pi-26", "cs", "u1", "", 1)
	ctl := &fakeHookCtl{failSet: "users"}
	task := rewriteTaskSnapshot("t26", "pi-26", definitionID, "cs", "u1",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err == nil {
		t.Fatalf("会签候选人回写失败应向上传播")
	}
	// 改写失败发生在消耗之前：记录保持未消耗
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-26", "cs")
	if len(rows) != 1 || rows[0].Used != 0 {
		t.Fatalf("改写失败时记录不应被消耗: %+v", rows)
	}
}

// TestRewriteCountersignGroupFallbackAndError 验证组会签兜底消费与改写失败传播。
// @param t 测试上下文
func TestRewriteCountersignGroupFallbackAndError(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "group-cs.bpmn20.xml", rewriteFixtureGroupCountersign)
	ctx := context.Background()
	// 兜底：任务未解析出候选组时按序消费第一条带组记录
	insertRewriteRecord(t, store, "pi-27", "gs", "u5", "g2", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t27", "pi-27", definitionID, "gs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("组兜底钩子失败: %v", err)
	}
	if len(ctl.candidateGrps) != 1 || ctl.candidateGrps[0] != "g2" {
		t.Fatalf("无候选组任务应兜底消费记录组 g2: %v", ctl.candidateGrps)
	}
	// 改写失败：SetCandidateGroups 错误向上传播
	insertRewriteRecord(t, store, "pi-28", "gs", "u6", "g3", 1)
	ctl2 := &fakeHookCtl{failSet: "groups"}
	task2 := rewriteTaskSnapshot("t28", "pi-28", definitionID, "gs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task2, ctl2); err == nil {
		t.Fatalf("组会签改写失败应向上传播")
	}
}

// TestRewriteCountersignGroupDirtyRecordsFallback 验证组会签记录全无候选组时兜底不消费。
// @param t 测试上下文
func TestRewriteCountersignGroupDirtyRecordsFallback(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "group-cs.bpmn20.xml", rewriteFixtureGroupCountersign)
	ctx := context.Background()
	// 记录无候选组（脏记录）：组兜底无可用目标，保留模型默认不消费
	insertRewriteRecord(t, store, "pi-29", "gs", "u5", "", 1)
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t29", "pi-29", definitionID, "gs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("组脏记录钩子应静默通过: %v", err)
	}
	if ctl.candidateGrps != nil {
		t.Fatalf("组脏记录不应回填候选组: %v", ctl.candidateGrps)
	}
	rows, _ := store.ListUnusedByInstanceAndActivity(ctx, "pi-29", "gs")
	if len(rows) != 1 || rows[0].Used != 0 {
		t.Fatalf("组脏记录不应被消费: %+v", rows)
	}
}

// TestRewriteCountersignSameVersionTimeOrder 验证同版本记录按创建时间升序消费。
// @param t 测试上下文
func TestRewriteCountersignSameVersionTimeOrder(t *testing.T) {
	eng, hook, _, store := newRewriteHookEnv(t)
	definitionID := deployRewriteResource(t, eng, hook, "countersign.bpmn20.xml", rewriteFixtureCountersign)
	ctx := context.Background()
	// 同版本、不同创建时间：先消费创建时间早的记录
	insertRewriteRecordAt(t, store, "pi-30", "cs", "uL", "", 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	insertRewriteRecordAt(t, store, "pi-30", "cs", "uE", "", 1, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	ctl := &fakeHookCtl{}
	task := rewriteTaskSnapshot("t30", "pi-30", definitionID, "cs", "",
		map[string]any{VarOperationType: OperationTypeAgreeComplete})
	if err := hook.Hook(ctx, task, ctl); err != nil {
		t.Fatalf("钩子失败: %v", err)
	}
	// 创建时间 1 月的 uL 先于 3 月的 uE 被消费
	if len(ctl.candidateUsers) != 1 || ctl.candidateUsers[0] != "uL" {
		t.Fatalf("同版本应按创建时间升序消费 uL: %v", ctl.candidateUsers)
	}
}
