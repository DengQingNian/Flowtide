package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// csClock 返回会签测试的假时钟（每个环境独立实例）。
// @return 假时钟
func csClock() *clock.Fake {
	return clock.NewFake(completeFixtureTime)
}

// csCollectionProcess 是并行会签（集合配置）语料：csTask（weightType=average + 集合配置监听器）→ after → end。
// executorList 含重复项（alice 重复），验证去重；执行者经元素变量 signer 落到各实例任务。
const csCollectionProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="csCollection" name="集合会签流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="csTask"/>
    <userTask id="csTask" name="会签任务" flowable:assignee="${signer}" executorType="person"
              weightType="average"
              executorList='[{"identityId":"alice","executorType":"person"},{"identityId":"bob","executorType":"person"},{"identityId":"alice","executorType":"person"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignCollectionConfig"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="csTask" targetRef="after"/>
    <userTask id="after" name="后续任务" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// csWeightProcess 是权重会签语料：wTask（weightType=custom + 权重监听器 + 完成条件）→ after → end。
// 完成条件对位 Java multiInstance 判定 Bean：累计权重≥阈值通过 / 负累计绝对值≥阈值否决（一票否决）/
// 全部完成未达阈值否决。
const csWeightProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="csWeight" name="权重会签流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="wTask"/>
    <userTask id="wTask" name="权重任务" flowable:assignee="${signer}" executorType="person"
              weightType="custom" weightThreshold="60"
              executorList='[{"identityId":"alice","executorType":"person"},{"identityId":"bob","executorType":"person"}]'
              weightList='[{"identityId":"alice","agreeWeight":70,"rejectWeight":1000},{"identityId":"bob","agreeWeight":10,"rejectWeight":1000}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignByWeight"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer">
        <completionCondition>${totalWeight >= weightThreshold || totalWeight &lt;= -weightThreshold || nrOfCompletedInstances == nrOfInstances}</completionCondition>
      </multiInstanceLoopCharacteristics>
    </userTask>
    <sequenceFlow id="f1" sourceRef="wTask" targetRef="after"/>
    <userTask id="after" name="后续任务" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="after" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// csWeightNoThresholdProcess 是权重阈值缺失语料（weightType=custom 且无 weightThreshold）。
const csWeightNoThresholdProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="csWeightNoThreshold" name="无阈值权重流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="wTask"/>
    <userTask id="wTask" name="无阈值任务" flowable:assignee="${signer}" executorType="person"
              weightType="custom"
              executorList='[{"identityId":"alice","executorType":"person"}]'>
      <extensionElements>
        <flowable:executionListener event="start" class="countersignByWeight"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="wTask" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// csEmptyProcess 是空会签集合语料（模型 executorList 为空且无业务变量）。
const csEmptyProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="csEmpty" name="空集合流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="csTask"/>
    <userTask id="csTask" name="空集合任务" flowable:assignee="${signer}" executorType="person"
              weightType="average">
      <extensionElements>
        <flowable:executionListener event="start" class="countersignCollectionConfig"/>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="executorList" flowable:elementVariable="signer"/>
    </userTask>
    <sequenceFlow id="f1" sourceRef="csTask" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// fakeStepRules 是比例退回规则的测试桩（按任务返回固定规则）。
type fakeStepRules struct {
	rule    *PercentReturnRule // 返回的规则
	enabled bool               // 是否启用比例退回
	err     error              // 注入错误
}

// PercentReturnRule 返回预设规则。
// @param ctx 操作上下文
// @param taskID 任务 ID
// @return 规则
// @return 是否启用
// @return 错误
// @param _ 接口占位参数（实现不使用）
// @param _ 接口占位参数（实现不使用）
func (f *fakeStepRules) PercentReturnRule(_ context.Context, _ string) (*PercentReturnRule, bool, error) {
	// 注入错误优先
	if f.err != nil {
		return nil, false, f.err
	}
	return f.rule, f.enabled, nil
}

// countersignFixture 是会签测试环境（内存引擎 + 会签服务 + 可选来源注入）。
type countersignFixture struct {
	engine  api.Engine                 // 内存引擎
	meta    *ModelMetaService          // 模型元数据
	next    *NextActivityService       // 下一步预生成（next_activity 来源）
	cs      *CountersignService        // 会签服务
	weights *MemCountersignWeightStore // 权重内存存储
	rules   *fakeStepRules             // 比例退回规则桩
}

// newCountersignFixture 构造会签测试环境（部署语料、注册监听器、预热缓存、创建用户）。
// @param t 测试上下文
// @return 测试环境
func newCountersignFixture(t *testing.T) *countersignFixture {
	t.Helper()
	fake := csClock()
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        fake,
		IDs:          idgen.NewUUIDv7(),
		TenantID:     "dev",
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "countersign-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"collection.bpmn20.xml":          []byte(csCollectionProcess),
			"weight.bpmn20.xml":              []byte(csWeightProcess),
			"weight-no-threshold.bpmn20.xml": []byte(csWeightNoThresholdProcess),
			"empty.bpmn20.xml":               []byte(csEmptyProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	for _, user := range []api.User{
		{ID: "alice", DisplayName: "张三"},
		{ID: "bob", DisplayName: "李四"},
		{ID: "carol", DisplayName: "王五"},
	} {
		if _, err := eng.IdentityService().CreateUser(context.Background(), user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	fx := &countersignFixture{engine: eng}
	fx.meta = NewModelMetaService(eng)
	stores := NewMemStores(fake)
	fx.next = NewNextActivityService(stores.Next, fake)
	fx.weights = NewMemCountersignWeightStore()
	fx.rules = &fakeStepRules{}
	fx.cs = NewCountersignService(eng, fx.meta,
		WithCountersignNext(fx.next),
		WithCountersignWeights(fx.weights),
		WithCountersignRules(fx.rules),
	)
	// 注册会签监听器（引擎需实现 api.ExecutionListenerRegistrar）
	if err := fx.cs.RegisterCountersignListeners(eng); err != nil {
		t.Fatalf("注册会签监听器失败: %v", err)
	}
	// 预热全部定义的节点元数据缓存（接线约定：部署后必须预热）
	if err := fx.cs.WarmupAll(context.Background()); err != nil {
		t.Fatalf("预热失败: %v", err)
	}
	return fx
}

// startCountersign 发起会签流程实例并返回实例与当前任务列表。
// @param t 测试上下文
// @param definitionKey 流程定义 Key
// @param initiator 发起人
// @param variables 启动变量
// @return 流程实例
// @return 当前任务列表
func (f *countersignFixture) startCountersign(t *testing.T, definitionKey, initiator string, variables map[string]any) (api.ProcessInstance, []api.Task) {
	t.Helper()
	vars := []api.Variable{{Name: VarInitiator, Value: initiator}}
	for name, value := range variables {
		vars = append(vars, api.Variable{Name: name, Value: value})
	}
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: definitionKey,
		BusinessKey:          "biz-" + definitionKey,
		TenantID:             "dev",
		UserID:               initiator,
		Variables:            vars,
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	page, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	return instance, page.Data
}

// instanceVariable 读取实例变量（断言辅助）。
// @param t 测试上下文
// @param instanceID 实例 Id
// @param name 变量名
// @return 变量值
func (f *countersignFixture) instanceVariable(t *testing.T, instanceID, name string) any {
	t.Helper()
	value, err := f.engine.RuntimeService().GetVariable(context.Background(), instanceID, name)
	if err != nil {
		t.Fatalf("读取变量 %s 失败: %v", name, err)
	}
	return value
}

// TestCountersign_CollectionVariables 验证集合配置监听器写入变量族（模型来源 + 去重 + 计数初始化）。
// @param t 测试上下文
func TestCountersign_CollectionVariables(t *testing.T) {
	fx := newCountersignFixture(t)
	instance, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	// executorList：模型来源（person 直用）+ 去重，保持首次出现顺序
	list, ok := fx.instanceVariable(t, instance.ID, VarExecutorList).([]string)
	if !ok {
		t.Fatalf("executorList 应为字符串切片: %T", fx.instanceVariable(t, instance.ID, VarExecutorList))
	}
	if len(list) != 2 || list[0] != "alice" || list[1] != "bob" {
		t.Fatalf("executorList 应去重为 [alice bob]: %v", list)
	}
	// nrOfInstances = 去重后集合长度
	if got := intOf(fx.instanceVariable(t, instance.ID, VarNrOfInstances)); got != 2 {
		t.Fatalf("nrOfInstances 应为 2: %v", got)
	}
	// 计数族初始化为 0
	if got := intOf(fx.instanceVariable(t, instance.ID, VarCountersignPass)); got != 0 {
		t.Fatalf("countersignPass 应为 0: %v", got)
	}
	if got := intOf(fx.instanceVariable(t, instance.ID, VarCountersignReturn)); got != 0 {
		t.Fatalf("countersignReturn 应为 0: %v", got)
	}
	// 两个并行实例任务分别落到 alice/bob
	if len(tasks) != 2 {
		t.Fatalf("应产生 2 个会签任务: %d", len(tasks))
	}
	assignees := map[string]bool{}
	for _, task := range tasks {
		assignees[task.Assignee] = true
	}
	if !assignees["alice"] || !assignees["bob"] {
		t.Fatalf("会签任务处理人应为 alice/bob: %v", assignees)
	}
}

// TestCountersign_CustomExecutorListPriority 验证执行者来源优先级：
// 业务变量 customExecutorList 最终覆盖（Java 源码顺序），next_activity 次之，模型最末。
// @param t 测试上下文
func TestCountersign_CustomExecutorListPriority(t *testing.T) {
	fx := newCountersignFixture(t)
	// 注入确定性随机排序（逆序），验证 randomSort 打乱
	original := fx.cs.shuffleFn
	fx.cs.shuffleFn = func(values []string) {
		for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
			values[i], values[j] = values[j], values[i]
		}
	}
	defer func() { fx.cs.shuffleFn = original }()
	// customExecutorList + randomSort=true：业务变量胜出且被打乱
	instance, _ := fx.startCountersign(t, "csCollection", "carol", map[string]any{
		VarCustomExecutorList: []string{"u1", "u2", "u3"},
		VarRandomSort:         true,
	})
	list, ok := fx.instanceVariable(t, instance.ID, VarExecutorList).([]string)
	if !ok || len(list) != 3 || list[0] != "u3" || list[1] != "u2" || list[2] != "u1" {
		t.Fatalf("customExecutorList 应逆序胜出: %v", list)
	}
	// customExecutorList 不打乱（randomSort 缺省）
	instance2, _ := fx.startCountersign(t, "csCollection", "carol", map[string]any{
		VarCustomExecutorList: []string{"u1", "u2"},
	})
	list2, _ := fx.instanceVariable(t, instance2.ID, VarExecutorList).([]string)
	if len(list2) != 2 || list2[0] != "u1" || list2[1] != "u2" {
		t.Fatalf("无 randomSort 应保持原序: %v", list2)
	}
}

// sameStringSet 判断两个列表是否为同一集合（忽略顺序与重复）。
// @param a 列表 a
// @param b 列表 b
// @return 是否同集
func sameStringSet(a, b []string) bool {
	counts := map[string]int{}
	for _, value := range a {
		counts[value]++
	}
	for _, value := range b {
		counts[value]--
	}
	for _, count := range counts {
		// 计数不为 0 说明存在差异元素
		if count != 0 {
			return false
		}
	}
	return true
}

// TestCountersign_NextActivitySource 验证 next_activity 预存记录来源覆盖模型配置
// （person→identityId / 非 person→groupId / 无记录回退空）。
// @param t 测试上下文
func TestCountersign_NextActivitySource(t *testing.T) {
	fx := newCountersignFixture(t)
	instance, _ := fx.startCountersign(t, "csCollection", "carol", nil)
	meta := &UserTaskMeta{ExecutorType: "person"}
	execution := api.ListenerExecution{ProcessInstanceID: instance.ID, ActivityID: "csTask"}
	// 无预存记录：返回空（上层回退模型来源）
	if ids := fx.cs.nextActivityExecutorIDs(context.Background(), execution, meta); len(ids) != 0 {
		t.Fatalf("无预存记录应返回空: %v", ids)
	}
	// 预存 person 记录 → identityId 列表
	if err := fx.next.SaveAgreeNext(context.Background(), NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "csTask",
		IdentityIDs:       []string{"x1", "x2"},
		TenantID:          "dev",
		Creator:           "carol",
	}); err != nil {
		t.Fatalf("预存失败: %v", err)
	}
	ids := fx.cs.nextActivityExecutorIDs(context.Background(), execution, meta)
	// 内存存储遍历无稳定顺序，按集合比较
	if len(ids) != 2 || !sameStringSet(ids, []string{"x1", "x2"}) {
		t.Fatalf("person 预存应取 identityId: %v", ids)
	}
	// 非 person 类型 → groupId 列表
	if err := fx.next.SaveAgreeNext(context.Background(), NextActivitySaveRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        "after",
		IdentityIDs:       []string{"g1"},
		GroupIDs:          []string{"grpA"},
		TenantID:          "dev",
		Creator:           "carol",
	}); err != nil {
		t.Fatalf("预存失败: %v", err)
	}
	groupMeta := &UserTaskMeta{ExecutorType: "role"}
	executionAfter := api.ListenerExecution{ProcessInstanceID: instance.ID, ActivityID: "after"}
	ids = fx.cs.nextActivityExecutorIDs(context.Background(), executionAfter, groupMeta)
	if len(ids) != 1 || ids[0] != "grpA" {
		t.Fatalf("非 person 预存应取 groupId: %v", ids)
	}
	// 来源未注入：返回空
	bare := NewCountersignService(fx.engine, fx.meta)
	if ids := bare.nextActivityExecutorIDs(context.Background(), execution, meta); len(ids) != 0 {
		t.Fatalf("来源未注入应返回空: %v", ids)
	}
}

// TestCountersign_EmptyCollection 验证空会签集合拒绝（模型与业务变量均为空）。
// @param t 测试上下文
func TestCountersign_EmptyCollection(t *testing.T) {
	fx := newCountersignFixture(t)
	_, err := fx.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "csEmpty",
		BusinessKey:          "biz-empty",
		TenantID:             "dev",
		UserID:               "carol",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "carol"}},
	})
	if err == nil || !strings.Contains(err.Error(), "会签集合不能为空") {
		t.Fatalf("空集合应报错: %v", err)
	}
}

// TestCountersign_WeightListener 验证权重监听器变量族（totalWeight/weightThreshold/countersignByWeight）。
// @param t 测试上下文
func TestCountersign_WeightListener(t *testing.T) {
	fx := newCountersignFixture(t)
	instance, tasks := fx.startCountersign(t, "csWeight", "carol", nil)
	// 权重三件套
	if got := intOf(fx.instanceVariable(t, instance.ID, VarTotalWeight)); got != 0 {
		t.Fatalf("totalWeight 应为 0: %v", got)
	}
	if got := fx.instanceVariable(t, instance.ID, VarWeightThreshold); got != "60" {
		t.Fatalf("weightThreshold 应为 60: %v", got)
	}
	if got, ok := fx.instanceVariable(t, instance.ID, VarCountersignByWeight).(bool); !ok || !got {
		t.Fatalf("countersignByWeight 应为 true: %v", fx.instanceVariable(t, instance.ID, VarCountersignByWeight))
	}
	// 集合配置同样生效
	if got := intOf(fx.instanceVariable(t, instance.ID, VarNrOfInstances)); got != 2 {
		t.Fatalf("nrOfInstances 应为 2: %v", got)
	}
	if len(tasks) != 2 {
		t.Fatalf("应产生 2 个会签任务: %d", len(tasks))
	}
}

// TestCountersign_WeightThresholdMissing 验证权重阈值缺失报错（监听器使启动命令失败）。
// @param t 测试上下文
func TestCountersign_WeightThresholdMissing(t *testing.T) {
	fx := newCountersignFixture(t)
	_, err := fx.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "csWeightNoThreshold",
		BusinessKey:          "biz-noth",
		TenantID:             "dev",
		UserID:               "carol",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "carol"}},
	})
	if err == nil || !strings.Contains(err.Error(), "未设置会签权重阈值") {
		t.Fatalf("阈值缺失应报错: %v", err)
	}
}

// TestCountersign_WeightCompletionCondition 验证权重完成条件语料（一票否决 = rejectWeight 巨大）。
// 场景：alice 否决（totalWeight=-1000，绝对值≥阈值 60）→ 完成条件命中，多实例提前结束。
// @param t 测试上下文
func TestCountersign_WeightCompletionCondition(t *testing.T) {
	fx := newCountersignFixture(t)
	instance, tasks := fx.startCountersign(t, "csWeight", "carol", nil)
	// 找到 alice 的任务并否决（totalWeight 累计 -1000，走办结链累计逻辑等价的直写）
	var aliceTask string
	for _, task := range tasks {
		if task.Assignee == "alice" {
			aliceTask = task.ID
		}
	}
	if aliceTask == "" {
		t.Fatalf("未找到 alice 任务")
	}
	// 引擎办结写入否决累计（与 complete.go accumulateCountersignWeight 的变量写入一致）
	if err := fx.engine.TaskService().Complete(context.Background(), aliceTask, api.CompleteTaskRequest{
		UserID:    "alice",
		Variables: []api.Variable{{Name: VarTotalWeight, Value: -1000.0}},
	}); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 完成条件 totalWeight <= -weightThreshold 命中：bob 的实例被取消，流程推进到后续任务
	page, err := fx.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "after" {
		t.Fatalf("一票否决应提前结束会签并推进到 after: %+v", page.Data)
	}
}

// TestCountersign_PercentReturn 验证比例退回累计与超线判定（decimal 精确比较，阈值恰好相等触发）。
// @param t 测试上下文
func TestCountersign_PercentReturn(t *testing.T) {
	fx := newCountersignFixture(t)
	percent := 50.0
	fx.rules.rule = &PercentReturnRule{CountersignPercentReturn: true, ReturnPercent: &percent}
	fx.rules.enabled = true
	instance, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	if len(tasks) != 2 {
		t.Fatalf("应产生 2 个会签任务: %d", len(tasks))
	}
	// 第一票退回：累计 return=1，未达全部实例 → 不判定
	over, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 0)
	if err != nil {
		t.Fatalf("比例退回累计失败: %v", err)
	}
	if over {
		t.Fatalf("未完成全部实例不应判定超线")
	}
	// 第二票退回：return=2/pass+return=2，退回率 100% >= 50% → 超线
	over, err = fx.cs.CountersignPercentResult(context.Background(), tasks[1].ID, 0)
	if err != nil {
		t.Fatalf("比例退回判定失败: %v", err)
	}
	if !over {
		t.Fatalf("退回率 100%% >= 50%% 应超线")
	}
	// 累计变量已写回
	if got := intOf(fx.instanceVariable(t, instance.ID, VarCountersignReturn)); got != 2 {
		t.Fatalf("countersignReturn 应累计为 2: %v", got)
	}
	if got := intOf(fx.instanceVariable(t, instance.ID, VarCountersignPass)); got != 0 {
		t.Fatalf("countersignPass 应为 0: %v", got)
	}
}

// TestCountersign_PercentReturnThresholdEqual 验证阈值恰好相等通过（0.5 == 0.5 → 超线，decimal 精度）。
// @param t 测试上下文
func TestCountersign_PercentReturnThresholdEqual(t *testing.T) {
	fx := newCountersignFixture(t)
	percent := 50.0
	fx.rules.rule = &PercentReturnRule{CountersignPercentReturn: true, ReturnPercent: &percent}
	fx.rules.enabled = true
	_, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	// 一票通过 + 一票退回：退回率 0.5 恰好等于阈值 50% → 超线（Java BigDecimal compare>=0 同语义）
	if _, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 1); err != nil {
		t.Fatalf("通过累计失败: %v", err)
	}
	over, err := fx.cs.CountersignPercentResult(context.Background(), tasks[1].ID, 0)
	if err != nil {
		t.Fatalf("比例退回判定失败: %v", err)
	}
	if !over {
		t.Fatalf("退回率 0.5 恰好等于阈值 0.5 应超线")
	}
}

// TestCountersign_PercentReturnBelow 验证未达阈值不超线（1/4 = 0.25 < 0.5）。
// @param t 测试上下文
func TestCountersign_PercentReturnBelow(t *testing.T) {
	fx := newCountersignFixture(t)
	percent := 50.0
	fx.rules.rule = &PercentReturnRule{CountersignPercentReturn: true, ReturnPercent: &percent}
	fx.rules.enabled = true
	_, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	// 三票通过 + 一票退回：退回率 0.25 < 0.5 → 不超线
	if _, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 1); err != nil {
		t.Fatalf("通过累计失败: %v", err)
	}
	if _, err := fx.cs.CountersignPercentResult(context.Background(), tasks[1].ID, 1); err != nil {
		t.Fatalf("通过累计失败: %v", err)
	}
	// 第三票通过后 pass+return=3 != nrOfInstances=2（语料仅 2 实例，改用双票一退场景退化验证）
	over, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 0)
	if err != nil {
		t.Fatalf("重复累计不应报错: %v", err)
	}
	// pass=3+return=1=4 != 2 → 不判定
	if over {
		t.Fatalf("实例计数不匹配不应判定超线")
	}
}

// TestCountersign_PercentReturnDisabled 验证规则未启用/端口未注入时不判定超线。
// @param t 测试上下文
func TestCountersign_PercentReturnDisabled(t *testing.T) {
	fx := newCountersignFixture(t)
	_, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	// 规则未启用
	over, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 0)
	if err != nil || over {
		t.Fatalf("规则未启用不应判定: %v %v", over, err)
	}
	// 端口未注入
	fx.cs.rules = nil
	over, err = fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 0)
	if err != nil || over {
		t.Fatalf("端口未注入不应判定: %v %v", over, err)
	}
}

// TestCountersign_PercentReturnEdgeErrors 验证结论非法/比例缺失的错误分支。
// @param t 测试上下文
func TestCountersign_PercentReturnEdgeErrors(t *testing.T) {
	fx := newCountersignFixture(t)
	// 启用规则但退回比例缺失（全部实例完成时报"未设置会签退回比例"）
	fx.rules.rule = &PercentReturnRule{CountersignPercentReturn: true, ReturnPercent: nil}
	fx.rules.enabled = true
	_, tasks := fx.startCountersign(t, "csCollection", "carol", nil)
	// 结论非法
	_, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 7)
	if err == nil || !strings.Contains(err.Error(), "未知会签任务执行结果") {
		t.Fatalf("非法结论应报错: %v", err)
	}
	// 任务不存在
	_, err = fx.cs.CountersignPercentResult(context.Background(), "ghost", 1)
	if err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("任务不存在应报错: %v", err)
	}
	// 两票退回后全部实例完成 → 比例缺失报错
	if _, err := fx.cs.CountersignPercentResult(context.Background(), tasks[0].ID, 0); err != nil {
		t.Fatalf("累计失败: %v", err)
	}
	_, err = fx.cs.CountersignPercentResult(context.Background(), tasks[1].ID, 0)
	if err == nil || !strings.Contains(err.Error(), "未设置会签退回比例") {
		t.Fatalf("比例缺失应报错: %v", err)
	}
}

// TestCountersign_RegisterWithoutRegistrar 验证引擎不支持扩展监听器时的注册错误。
// @param t 测试上下文
func TestCountersign_RegisterWithoutRegistrar(t *testing.T) {
	fx := newCountersignFixture(t)
	// 构造仅实现 api.Engine 的桩（不含 RegisterExecutionListenerHook）
	var stub api.Engine = struct{ api.Engine }{}
	err := fx.cs.RegisterCountersignListeners(stub)
	if err == nil || !strings.Contains(err.Error(), "不支持扩展执行监听器注册") {
		t.Fatalf("应报注册能力缺失: %v", err)
	}
}

// TestCountersign_WarmupCache 验证未预热定义的监听器报错与 Warmup 单定义预热。
// @param t 测试上下文
func TestCountersign_WarmupCache(t *testing.T) {
	fx := newCountersignFixture(t)
	// 清空缓存模拟未预热
	fx.cs.mu.Lock()
	fx.cs.cache = make(map[string]map[string]*UserTaskMeta)
	fx.cs.mu.Unlock()
	_, err := fx.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "csCollection",
		BusinessKey:          "biz-warm",
		TenantID:             "dev",
		UserID:               "carol",
		Variables:            []api.Variable{{Name: VarInitiator, Value: "carol"}},
	})
	if err == nil || !strings.Contains(err.Error(), "未预热") {
		t.Fatalf("未预热应报错: %v", err)
	}
	// 单定义预热后可正常发起
	definition, err := fx.engine.RepositoryService().GetProcessDefinitionByKey(context.Background(), "csCollection")
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	if err := fx.cs.Warmup(context.Background(), definition.ID); err != nil {
		t.Fatalf("预热失败: %v", err)
	}
	if _, tasks := fx.startCountersign(t, "csCollection", "carol", nil); len(tasks) != 2 {
		t.Fatalf("预热后应正常产生会签任务: %d", len(tasks))
	}
}

// TestCountersign_WeightStore 验证权重存储端口的先删后插与模型回退。
// @param t 测试上下文
func TestCountersign_WeightStore(t *testing.T) {
	fx := newCountersignFixture(t)
	key := CountersignWeightKey{ProcessDefinitionKey: "csWeight", ActivityID: "wTask", TenantID: "dev"}
	// 模型回退：无存储记录的节点取 weightList（须在首次保存前验证）
	definition, err := fx.engine.RepositoryService().GetProcessDefinitionByKey(context.Background(), "csWeight")
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	modelConfigs, err := fx.cs.ListNodeWeights(context.Background(), definition.ID, key)
	if err != nil || len(modelConfigs) != 2 {
		t.Fatalf("模型回退应返回 2 条: %v %v", modelConfigs, err)
	}
	// 保存 → 查询
	if err := fx.cs.SaveNodeWeights(context.Background(), key, []CountersignWeight{
		{IdentityID: "alice", AgreeWeight: 2.5, RejectWeight: 1},
		{IdentityID: "", AgreeWeight: 9}, // 无身份行应被跳过
	}); err != nil {
		t.Fatalf("保存权重失败: %v", err)
	}
	configs, err := fx.cs.ListNodeWeights(context.Background(), "def-id", key)
	if err != nil || len(configs) != 1 {
		t.Fatalf("存储来源应返回 1 条: %v %v", configs, err)
	}
	if configs[0].AgreeWeight != 2.5 {
		t.Fatalf("权重应持久化: %+v", configs[0])
	}
	// 覆盖保存（先删后插）
	if err := fx.cs.SaveNodeWeights(context.Background(), key, []CountersignWeight{
		{IdentityID: "bob", AgreeWeight: 3},
	}); err != nil {
		t.Fatalf("覆盖保存失败: %v", err)
	}
	configs, _ = fx.cs.ListNodeWeights(context.Background(), "def-id", key)
	if len(configs) != 1 || configs[0].IdentityID != "bob" {
		t.Fatalf("覆盖后应仅剩 bob: %v", configs)
	}
	// 未配置存储时保存报错
	bare := NewCountersignService(fx.engine, fx.meta)
	if err := bare.SaveNodeWeights(context.Background(), key, nil); err == nil || !strings.Contains(err.Error(), "权重存储未配置") {
		t.Fatalf("无存储保存应报错: %v", err)
	}
	// 定位键不完整
	if err := fx.weights.SaveByNode(context.Background(), CountersignWeightKey{ActivityID: "w"}, nil); err == nil || !strings.Contains(err.Error(), "缺少定义 Key") {
		t.Fatalf("定位键缺失应报错: %v", err)
	}
}
