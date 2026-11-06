// simulate_delegate_cover_test.go 补充推演/委托/催办域服务与门面中尚未覆盖的语句：
// 纯模型容器定位、静态图遍历边界、执行人解析、退回模式判定、推演入口故障分支，
// 委托存储克隆/排序/校验分支，催办存储标记/排序与送达重试分支。
package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// simCovNow 是本文件全部用例的固定时钟基准（禁用真实时间）。
var simCovNow = time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

// simCovTask 构造纯模型的用户任务元素。
// @param id 节点 Id
// @param name 节点名称
// @param assignee 处理人配置
// @return 用户任务元素
func simCovTask(id, name, assignee string) *model.UserTask {
	return &model.UserTask{
		FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: id, Name: name}},
		Assignee:     assignee,
	}
}

// simCovNestedModel 构造三层嵌套子流程纯模型语料（外层无扩展、中层有执行人配置、内层空扩展）。
// 层级：outer > middle > inner > deep；另有 outerOnly 直接挂在外层、mid 直接挂在中层。
// @return 流程模型
func simCovNestedModel() *model.Process {
	rootTask := simCovTask("rootTask", "根任务", "alice")
	outerTask := simCovTask("outerOnly", "外层任务", "bob")
	midTask := simCovTask("mid", "中层任务", "carl")
	deepTask := simCovTask("deep", "深层任务", "dave")
	inner := &model.SubProcess{
		FlowNodeBase: model.FlowNodeBase{
			BaseElement: model.BaseElement{ID: "inner", Name: "内层子流程"},
			Extensions:  &model.ElementExtensions{Attributes: map[string]string{}},
		},
		Elements: map[string]model.FlowElement{"deep": deepTask},
		Order:    []string{"deep"},
	}
	middle := &model.SubProcess{
		FlowNodeBase: model.FlowNodeBase{
			BaseElement: model.BaseElement{ID: "middle", Name: "中层子流程"},
			Extensions: &model.ElementExtensions{Attributes: map[string]string{
				"executorType": "org",
				"executorList": `[{"orgId":"grpA"}]`,
			}},
		},
		Elements: map[string]model.FlowElement{"mid": midTask, "inner": inner},
		Order:    []string{"mid", "inner"},
	}
	outer := &model.SubProcess{
		FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "outer", Name: "外层子流程"}},
		Elements:     map[string]model.FlowElement{"outerOnly": outerTask, "middle": middle},
		Order:        []string{"outerOnly", "middle"},
	}
	return &model.Process{
		ID: "covNested",
		Elements: map[string]model.FlowElement{
			"rootTask": rootTask,
			"outer":    outer,
		},
		Order: []string{"rootTask", "outer"},
	}
}

// TestSimCovContainerAndLookupPure 覆盖容器定位/跨容器回退/子流程执行人配置回退与视图摘要。
// @param t 测试上下文
func TestSimCovContainerAndLookupPure(t *testing.T) {
	process := simCovNestedModel()
	// 主流程容器命中
	rootScope := simRootScope(process)
	if element, scope, ok := rootScope.lookup(process, "rootTask"); !ok || scope != rootScope || element.GetID() != "rootTask" {
		t.Fatalf("主流程容器命中不符: %v %v", element, ok)
	}
	// 主流程容器未命中（非子流程容器不回退）
	if _, _, ok := rootScope.lookup(process, "deep"); ok {
		t.Fatal("主流程容器不应命中子流程元素")
	}
	// 子流程容器未命中回退主流程容器
	subScope := simSubScope(process.Elements["outer"].(*model.SubProcess))
	if element, scope, ok := subScope.lookup(process, "rootTask"); !ok || scope.isSub || element.GetID() != "rootTask" {
		t.Fatalf("子流程回退主流程不符: %v %v", element, ok)
	}
	// 子流程与主流程都未命中
	if _, _, ok := subScope.lookup(process, "ghost"); ok {
		t.Fatal("子流程与主流程都未命中时应返回否")
	}
	// 递归查找未命中（返回 false 分支）
	if _, _, ok := simFindElement(process, "ghost"); ok {
		t.Fatal("递归查找未命中应返回否")
	}
	// 直接子流程归属
	if sub, ok := simContainerOf(process, "mid"); !ok || sub.ID != "middle" {
		t.Fatalf("直接子流程归属不符: %v %v", sub, ok)
	}
	// 嵌套子流程归属
	if sub, ok := simContainerOf(process, "deep"); !ok || sub.ID != "inner" {
		t.Fatalf("嵌套子流程归属不符: %v %v", sub, ok)
	}
	// 无子流程归属
	if sub, ok := simContainerOf(process, "ghost"); ok || sub != nil {
		t.Fatalf("无子流程归属应返回否: %v", sub)
	}
	// 嵌套查找失败
	if found := simContainerInSub(process.Elements["outer"].(*model.SubProcess), "ghost"); found != nil {
		t.Fatalf("嵌套查找失败应返回 nil: %v", found)
	}
	// 子流程执行人配置回退：无容器
	fallback := &UserTaskMeta{ActivityID: "deep", ExecutorListRaw: "[]"}
	if got := simParentMetaOf(process, "ghost", fallback); got != fallback {
		t.Fatalf("无容器应回退节点配置: %+v", got)
	}
	// 子流程无扩展属性时回退节点配置
	if got := simParentMetaOf(process, "outerOnly", fallback); got != fallback {
		t.Fatalf("子流程无扩展应回退节点配置: %+v", got)
	}
	// 子流程有扩展属性时取子流程配置
	meta := simParentMetaOf(process, "mid", fallback)
	if meta == fallback || meta.ActivityID != "middle" || meta.ExecutorType != "org" {
		t.Fatalf("子流程配置不符: %+v", meta)
	}
	// 子流程扩展存在但无配置时回退节点配置
	if got := simParentMetaOf(process, "deep", fallback); got != fallback {
		t.Fatalf("空扩展子流程应回退节点配置: %+v", got)
	}
	// 视图摘要
	if got := SimulateViewString(NextUserTaskView{ActivityID: "a", ExecutorIDs: []string{"x", "y"}}); got != "a(x,y)" {
		t.Fatalf("视图摘要不符: %s", got)
	}
}

// simCovWalkModel 构造遍历分支纯模型语料：start→first→exclusive→second，另有包容网关与孤立连线。
// @return 流程模型
func simCovWalkModel() *model.Process {
	start := &model.StartEvent{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "start", Name: "开始"}}}
	first := simCovTask("first", "首任务", "${initiator}")
	second := simCovTask("second", "次任务", "alice")
	exclusive := &model.ExclusiveGateway{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "exclusive", Name: "排他网关"}}}
	inclusive := &model.InclusiveGateway{FlowNodeBase: model.FlowNodeBase{BaseElement: model.BaseElement{ID: "inclusive", Name: "包容网关"}}}
	flowOnly := &model.SequenceFlow{BaseElement: model.BaseElement{ID: "flowOnly", Name: "孤立连线"}}
	start.AddOutgoing(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "sf1"}, SourceRef: "start", TargetRef: "first", Condition: "${pass == true}"})
	first.AddOutgoing(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "sf2"}, SourceRef: "first", TargetRef: "exclusive"})
	exclusive.AddOutgoing(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "sf3"}, SourceRef: "exclusive", TargetRef: "second", Condition: "${pass == true}"})
	exclusive.AddOutgoing(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "sf4"}, SourceRef: "exclusive", TargetRef: "second"})
	inclusive.AddOutgoing(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "sf5"}, SourceRef: "inclusive", TargetRef: "second"})
	return &model.Process{
		ID: "covWalk",
		Elements: map[string]model.FlowElement{
			"start":     start,
			"first":     first,
			"second":    second,
			"exclusive": exclusive,
			"inclusive": inclusive,
			"flowOnly":  flowOnly,
		},
		Order: []string{"start", "first", "exclusive", "second", "inclusive"},
	}
}

// TestSimCovWalkPureBranches 覆盖静态图遍历的缺失节点/已访问/非流节点/空出线/包容网关/条件剪枝分支。
// @param t 测试上下文
func TestSimCovWalkPureBranches(t *testing.T) {
	process := simCovWalkModel()
	rootScope := simRootScope(process)
	// 起点元素不存在
	if cands := simCollectNext(process, "ghost", nil, true); len(cands) != 0 {
		t.Fatalf("起点不存在应返回空候选: %v", cands)
	}
	// 起点为开始事件（走 simWalk 而非出线直取）
	cands := simCollectNext(process, "start", map[string]any{"pass": true}, true)
	if len(cands) != 1 || cands[0].activityID != "first" {
		t.Fatalf("开始事件遍历不符: %v", cands)
	}
	// 已访问节点直接返回
	visited := map[string]bool{"first": true}
	empty := make([]simCandidate, 0, 1)
	simWalk(process, rootScope, "first", nil, true, visited, &empty)
	if len(empty) != 0 {
		t.Fatalf("已访问节点不应重复收集: %v", empty)
	}
	// 节点不存在
	simWalk(process, rootScope, "ghost", nil, true, map[string]bool{}, &empty)
	// 非流节点（连线元素）直接返回
	simWalk(process, rootScope, "flowOnly", nil, true, map[string]bool{}, &empty)
	if len(empty) != 0 {
		t.Fatalf("非流节点不应收集候选: %v", empty)
	}
	// 无出线节点直接返回
	simTakeOutgoing(process, rootScope, simCovTask("lonely", "孤立任务", ""), nil, true, map[string]bool{}, &empty)
	// 包容网关：走网关出线选择
	inclusive := process.Elements["inclusive"].(*model.InclusiveGateway)
	cands = nil
	simTakeOutgoing(process, rootScope, inclusive, nil, true, map[string]bool{}, &cands)
	if len(cands) != 1 || cands[0].activityID != "second" {
		t.Fatalf("包容网关出线不符: %v", cands)
	}
	// 排他网关：条件全假时回退无条件出线
	exclusive := process.Elements["exclusive"].(*model.ExclusiveGateway)
	cands = nil
	simTakeOutgoing(process, rootScope, exclusive, map[string]any{"pass": false}, true, map[string]bool{}, &cands)
	if len(cands) != 1 || cands[0].activityID != "second" {
		t.Fatalf("排他网关兜底出线不符: %v", cands)
	}
	// 普通节点条件为假时剪枝
	start := process.Elements["start"].(*model.StartEvent)
	cands = nil
	simTakeOutgoing(process, rootScope, start, map[string]any{"pass": false}, true, map[string]bool{}, &cands)
	if len(cands) != 0 {
		t.Fatalf("条件为假应剪枝: %v", cands)
	}
	// 条件求值失败剪枝
	if simConditionTrue(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "bad"}, Condition: "${missing.name}"}, nil) {
		t.Fatal("条件求值失败应返回否")
	}
	// 非布尔结果宽松转换
	if !simConditionTrue(&model.SequenceFlow{BaseElement: model.BaseElement{ID: "str"}, Condition: "${flag}"}, map[string]any{"flag": "TRUE"}) {
		t.Fatal("非布尔结果 TRUE 应视为真")
	}
}

// simCovIdentity 是身份服务桩：按组返回成员并支持注入查询错误。
type simCovIdentity struct {
	api.IdentityService                       // 未覆盖方法透传（本文件不使用）
	members             map[string][]api.User // 组 Id → 成员
	errGroups           map[string]bool       // 查询失败的组
}

// ListMembers 查询组成员（可注入失败）。
// @param ctx 上下文
// @param groupID 组 Id
// @return 成员列表
// @return 错误
func (s *simCovIdentity) ListMembers(ctx context.Context, groupID string) ([]api.User, error) {
	if s.errGroups[groupID] {
		return nil, apiError("SIM-IDENTITY-001", "组成员查询失败")
	}
	return s.members[groupID], nil
}

// GetUser 查询用户（固定返回带展示名的用户）。
// @param ctx 上下文
// @param userID 用户 Id
// @return 用户
// @return 错误
func (s *simCovIdentity) GetUser(ctx context.Context, userID string) (api.User, error) {
	return api.User{ID: userID, DisplayName: "姓名-" + userID}, nil
}

// simCovExecModel 构造执行人解析纯模型语料（发起人/候选用户/候选组/子流程配置/调用活动/非流节点）。
// @return 流程模型
func simCovExecModel() *model.Process {
	initTask := simCovTask("initTask", "发起人任务", "${initiator}")
	usersTask := &model.UserTask{
		FlowNodeBase:   model.FlowNodeBase{BaseElement: model.BaseElement{ID: "usersTask", Name: "候选用户任务"}},
		CandidateUsers: []string{"alice", "bob"},
	}
	groupsTask := &model.UserTask{
		FlowNodeBase:    model.FlowNodeBase{BaseElement: model.BaseElement{ID: "groupsTask", Name: "候选组任务"}},
		CandidateGroups: []string{"${expr}", "grpA", "dynRole"},
	}
	execSub := &model.SubProcess{
		FlowNodeBase: model.FlowNodeBase{
			BaseElement: model.BaseElement{ID: "execSub", Name: "执行人子流程"},
			Extensions: &model.ElementExtensions{Attributes: map[string]string{
				"executorType": "org",
				"executorList": `[{"orgId":"grpA"}]`,
			}},
		},
		Elements: map[string]model.FlowElement{"execTask": simCovTask("execTask", "子流程任务", "${executor}")},
		Order:    []string{"execTask"},
	}
	callTask := &model.CallActivity{
		FlowNodeBase: model.FlowNodeBase{
			BaseElement: model.BaseElement{ID: "callTask", Name: "调用活动"},
			Extensions: &model.ElementExtensions{Attributes: map[string]string{
				"executorType": "person",
				"executorList": `[{"identityId":"carl"}]`,
			}},
		},
	}
	flowOnly := &model.SequenceFlow{BaseElement: model.BaseElement{ID: "flowOnly", Name: "孤立连线"}}
	return &model.Process{
		ID: "covExec",
		Elements: map[string]model.FlowElement{
			"initTask":   initTask,
			"usersTask":  usersTask,
			"groupsTask": groupsTask,
			"execSub":    execSub,
			"callTask":   callTask,
			"flowOnly":   flowOnly,
		},
		Order: []string{"initTask", "usersTask", "groupsTask", "execSub", "callTask"},
	}
}

// TestSimCovResolveExecutorPure 覆盖执行人解析链的元数据缺失/发起人为空/候选用户/候选组/子流程配置分支。
// @param t 测试上下文
func TestSimCovResolveExecutorPure(t *testing.T) {
	process := simCovExecModel()
	ctx := context.Background()
	identity := &simCovIdentity{
		members:   map[string][]api.User{"grpA": {{ID: "alice"}, {ID: "bob"}}},
		errGroups: map[string]bool{"errRole": true},
	}
	svc := NewSimulateService(nil, nil, nil, nil, nil, identity, clock.NewFake(simCovNow))
	// 节点元数据缺失
	if ids, dynamic := svc.resolveModelExecutors(ctx, process, "def", simCandidate{activityID: "ghost"}, nil, "alice"); ids != nil || dynamic != nil {
		t.Fatalf("元数据缺失应返回空: %v %v", ids, dynamic)
	}
	// 发起人表达式且发起人为空
	if ids, _ := svc.resolveModelExecutors(ctx, process, "def", simCandidate{activityID: "initTask"}, nil, ""); ids != nil {
		t.Fatalf("发起人为空应返回空: %v", ids)
	}
	// 普通变量表达式且变量缺失
	varProcess := &model.Process{Elements: map[string]model.FlowElement{"varTask": simCovTask("varTask", "变量任务", "${owner}")}}
	if ids, _ := svc.resolveModelExecutors(ctx, varProcess, "def", simCandidate{activityID: "varTask"}, nil, "alice"); ids != nil {
		t.Fatalf("变量缺失应返回空: %v", ids)
	}
	// 候选用户直用
	ids, _ := svc.resolveModelExecutors(ctx, process, "def", simCandidate{activityID: "usersTask"}, nil, "alice")
	if len(ids) != 2 || ids[0] != "alice" || ids[1] != "bob" {
		t.Fatalf("候选用户解析不符: %v", ids)
	}
	// 候选组：表达式跳过、有成员展开、无成员收集动态角色
	ids, dynamic := svc.resolveModelExecutors(ctx, process, "def", simCandidate{activityID: "groupsTask"}, nil, "alice")
	if len(ids) != 2 || len(dynamic) != 1 || dynamic[0] != "dynRole" {
		t.Fatalf("候选组解析不符: %v %v", ids, dynamic)
	}
	// 执行人表达式回退子流程执行人配置
	ids, _ = svc.resolveModelExecutors(ctx, process, "def", simCandidate{activityID: "execTask"}, nil, "alice")
	if len(ids) != 2 || ids[0] != "alice" || ids[1] != "bob" {
		t.Fatalf("子流程执行人配置不符: %v", ids)
	}
	// 调用活动扩展属性合成元数据（无处理人配置时返回空）
	if ids, dynamic := svc.resolveModelExecutors(ctx, process, "def", simCandidate{activityID: "callTask"}, nil, "alice"); ids != nil || dynamic != nil {
		t.Fatalf("调用活动无处理人应返回空: %v %v", ids, dynamic)
	}
	// 非用户任务/调用活动元素元数据为 nil
	if meta := simMetaOf(process, simCandidate{activityID: "flowOnly"}); meta != nil {
		t.Fatalf("非任务元素元数据应为 nil: %+v", meta)
	}
	// 节点不存在元数据为 nil
	if meta := simMetaOf(process, simCandidate{activityID: "ghost"}); meta != nil {
		t.Fatalf("不存在节点元数据应为 nil: %+v", meta)
	}
	// 组成员查询：空组 Id
	if _, ok := svc.groupMembers(ctx, ""); ok {
		t.Fatal("空组 Id 查询应返回否")
	}
	// 组成员查询：身份服务未注入
	noIdentity := NewSimulateService(nil, nil, nil, nil, nil, nil, nil)
	if _, ok := noIdentity.groupMembers(ctx, "grpA"); ok {
		t.Fatal("身份服务未注入应返回否")
	}
	// 组成员查询：注入错误
	if _, ok := svc.groupMembers(ctx, "errRole"); ok {
		t.Fatal("组成员查询错误应返回否")
	}
	// 组织展开：空组 Id 返回 nil
	if ids := svc.expandGroupOrKeep(ctx, ""); ids != nil {
		t.Fatalf("空组 Id 展开应为 nil: %v", ids)
	}
	// 组织展开：无成员时保留组 Id
	if ids := svc.expandGroupOrKeep(ctx, "dynRole"); len(ids) != 1 || ids[0] != "dynRole" {
		t.Fatalf("无成员组织应保留 Id: %v", ids)
	}
	// 组织展开：查询失败时保留组 Id
	if ids := svc.expandGroupOrKeep(ctx, "errRole"); len(ids) != 1 || ids[0] != "errRole" {
		t.Fatalf("查询失败组织应保留 Id: %v", ids)
	}
}

// simCovNextStore 是下一步预生成存储桩：可注入历史记录查询失败。
type simCovNextStore struct {
	NextActivityStore      // 其余方法透传
	failHisInstance   bool // ListHisByInstance 失败
}

// ListHisByInstance 查询实例历史记录（可注入失败）。
// @param ctx 上下文
// @param processInstanceID 实例 Id
// @return 记录列表
// @return 错误
func (s *simCovNextStore) ListHisByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error) {
	if s.failHisInstance {
		return nil, apiError("SIM-NEXT-001", "历史记录查询失败")
	}
	return s.NextActivityStore.ListHisByInstance(ctx, processInstanceID)
}

// TestSimCovResolveCandidatesBranches 覆盖候选解析的历史执行人优先/步骤排序/按节点 Id 排序/历史查询失败分支。
// @param t 测试上下文
func TestSimCovResolveCandidatesBranches(t *testing.T) {
	fx := newSimFixture(t)
	ctx := context.Background()
	instance, task := fx.start("simParallel", "alice", nil)
	// 步骤规则给出不同排序号：tb(1) 应排在 ta(2) 之前
	fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "ta", Sort: 2})
	fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "tb", Sort: 1})
	// 为 ta 播种历史执行人（历史来源优先于模型配置）
	fx.seedNext(instance.ID, "ta", "bob", "task-x", "")
	views, err := fx.sim.SimulateByTask(ctx, task.ID, nil)
	if err != nil {
		t.Fatalf("推演失败: %v", err)
	}
	assertSameStrings(t, viewIDs(views), []string{"tb", "ta"})
	if views[0].Sort != 1 || views[1].Sort != 2 {
		t.Fatalf("步骤排序号不符: %+v", views)
	}
	if len(views[1].ExecutorIDs) != 1 || views[1].ExecutorIDs[0] != "bob" || views[1].ExecutorNames[0] != "李四" {
		t.Fatalf("历史执行人优先不符: %+v", views[1])
	}
	// 历史记录查询失败上抛
	failing := &simCovNextStore{NextActivityStore: fx.stores.Next, failHisInstance: true}
	svc := NewSimulateService(fx.engine, fx.meta, NewNextActivityService(failing, fx.fake), fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake, WithSimulateSteps(fx.steps))
	if _, err := svc.SimulateByTask(ctx, task.ID, nil); err == nil {
		t.Fatal("历史记录查询失败应上抛错误")
	}
	// 历史执行人来源未注入（next 为 nil）
	noNext := NewSimulateService(fx.engine, fx.meta, nil, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if ids, err := noNext.historyExecutorsOf(ctx, instance.ID, "ta"); err != nil || ids != nil {
		t.Fatalf("未注入历史来源应返回空: %v %v", ids, err)
	}
	if views, err := noNext.historyStepViews(ctx, instance.ID, task, fx.deploymentID); err != nil || len(views) != 0 {
		t.Fatalf("未注入历史来源的历史步骤应为空: %v %v", views, err)
	}
}

// simCovRuntime 是运行时服务桩：可注入实例查询失败（其余方法透传）。
type simCovRuntime struct {
	api.RuntimeService
	failInstance bool // GetProcessInstance 失败
}

// GetProcessInstance 查询运行实例（可注入失败）。
// @param ctx 上下文
// @param id 实例 Id
// @return 流程实例
// @return 错误
func (r *simCovRuntime) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	if r.failInstance {
		return api.ProcessInstance{}, apiError("SIM-RUNTIME-001", "运行实例不存在")
	}
	return r.RuntimeService.GetProcessInstance(ctx, id)
}

// simCovHistory 是历史服务桩：可注入实例查询失败并返回固定历史实例。
type simCovHistory struct {
	api.HistoryService
	failInstance bool                        // GetHistoricProcessInstance 失败
	historic     api.HistoricProcessInstance // 固定历史实例
}

// GetHistoricProcessInstance 查询历史实例（可注入失败/固定返回）。
// @param ctx 上下文
// @param processInstanceID 实例 Id
// @return 历史实例
// @return 错误
func (h *simCovHistory) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	if h.failInstance {
		return api.HistoricProcessInstance{}, apiError("SIM-HISTORY-001", "历史实例不存在")
	}
	return h.historic, nil
}

// simCovEngine 是引擎门面桩：替换运行时/历史服务（其余服务透传）。
type simCovEngine struct {
	api.Engine
	runtime api.RuntimeService
	history api.HistoryService
}

// RuntimeService 返回替换后的运行时服务。
// @return 运行时服务
func (e *simCovEngine) RuntimeService() api.RuntimeService { return e.runtime }

// HistoryService 返回替换后的历史服务。
// @return 历史服务
func (e *simCovEngine) HistoryService() api.HistoryService { return e.history }

// TestSimCovSimulateByTaskFaults 覆盖按任务推演的入口/定义/模型/历史实例故障分支。
// @param t 测试上下文
func TestSimCovSimulateByTaskFaults(t *testing.T) {
	fx := newSimFixture(t)
	ctx := context.Background()
	instance, task := fx.start("simLinear", "alice", nil)
	// 任务 Id 为空 / 引擎未注入
	noEngine := NewSimulateService(nil, nil, nil, nil, nil, nil, clock.NewFake(simCovNow))
	if views, err := noEngine.SimulateByTask(ctx, "", nil); err != nil || len(views) != 0 {
		t.Fatalf("空任务应返回空: %v %v", views, err)
	}
	if views, err := noEngine.SimulateByTask(ctx, task.ID, nil); err != nil || len(views) != 0 {
		t.Fatalf("引擎未注入应返回空: %v %v", views, err)
	}
	// 流程定义查询失败
	repoFault := NewSimulateService(&faultEngine{Engine: fx.engine, failRepo: true}, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if _, err := repoFault.SimulateByTask(ctx, task.ID, nil); err == nil {
		t.Fatal("流程定义查询失败应上抛错误")
	}
	// 模型查询失败
	bpmnFault := NewSimulateService(&faultEngine{Engine: fx.engine, failBpmn: true}, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if _, err := bpmnFault.SimulateByTask(ctx, task.ID, nil); err == nil {
		t.Fatal("模型查询失败应上抛错误")
	}
	// 模型不含流程
	emptyFault := NewSimulateService(&faultEngine{Engine: fx.engine, emptyModel: true}, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if _, err := emptyFault.SimulateByTask(ctx, task.ID, nil); err == nil {
		t.Fatal("模型不含流程应上抛错误")
	}
	// 运行实例缺失回退历史实例
	engineStub := &simCovEngine{
		Engine:  fx.engine,
		runtime: &simCovRuntime{RuntimeService: fx.engine.RuntimeService(), failInstance: true},
		history: &simCovHistory{HistoryService: fx.engine.HistoryService(), historic: api.HistoricProcessInstance{
			ID:                   instance.ID,
			ProcessDefinitionID:  task.ProcessDefinitionID,
			ProcessDefinitionKey: "simLinear",
			StartUserID:          "alice",
			TenantID:             "dev",
		}},
	}
	fallback := NewSimulateService(engineStub, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake, WithSimulateSteps(fx.steps))
	views, err := fallback.SimulateByTask(ctx, task.ID, nil)
	if err != nil || len(views) != 1 || views[0].ActivityID != "handle" {
		t.Fatalf("历史实例回退推演不符: %v %v", views, err)
	}
	// 运行实例与历史实例都缺失
	engineStub.history = &simCovHistory{HistoryService: fx.engine.HistoryService(), failInstance: true}
	missing := NewSimulateService(engineStub, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if views, err := missing.SimulateByTask(ctx, task.ID, nil); err != nil || len(views) != 0 {
		t.Fatalf("实例缺失应返回空: %v %v", views, err)
	}
}

// TestSimCovSimulateByKeyFaults 覆盖按 Key 推演的引擎缺失/模型查询失败/空模型分支。
// @param t 测试上下文
func TestSimCovSimulateByKeyFaults(t *testing.T) {
	fx := newSimFixture(t)
	ctx := context.Background()
	// 引擎未注入
	noEngine := NewSimulateService(nil, nil, nil, nil, nil, nil, clock.NewFake(simCovNow))
	if views, err := noEngine.SimulateByKey(ctx, "dev", "simLinear", "BIZ-1", nil); err != nil || len(views) != 0 {
		t.Fatalf("引擎未注入应返回空: %v %v", views, err)
	}
	// 模型查询失败
	bpmnFault := NewSimulateService(&faultEngine{Engine: fx.engine, failBpmn: true}, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if _, err := bpmnFault.SimulateByKey(ctx, "dev", "simLinear", "BIZ-1", nil); err == nil {
		t.Fatal("模型查询失败应上抛错误")
	}
	// 模型不含流程
	emptyFault := NewSimulateService(&faultEngine{Engine: fx.engine, emptyModel: true}, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	if _, err := emptyFault.SimulateByKey(ctx, "dev", "simLinear", "BIZ-1", nil); err == nil {
		t.Fatal("模型不含流程应上抛错误")
	}
}

// TestSimCovDetectReturnModeBranches 覆盖退回模式判定的待办缺失/发送步骤缺失/未处于退回位分支。
// @param t 测试上下文
func TestSimCovDetectReturnModeBranches(t *testing.T) {
	ctx := context.Background()
	t.Run("待办缺失", func(t *testing.T) {
		fx := newSimFixture(t)
		_, task := fx.start("simLinear", "alice", nil)
		fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "apply", Sort: 2, ReturnApprovalStep: "3"})
		views, err := fx.sim.SimulateByTask(ctx, task.ID, nil)
		if err != nil || len(views) == 0 {
			t.Fatalf("待办缺失应继续默认推演: %v %v", views, err)
		}
	})
	t.Run("发送步骤缺失", func(t *testing.T) {
		fx := newSimFixture(t)
		instance, task := fx.start("simLinear", "alice", nil)
		fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "apply", Sort: 2, ReturnApprovalStep: "3"})
		fx.seedTaskMessage(instance.ID, task, "alice", "ghostStep", "幽灵步骤")
		views, err := fx.sim.SimulateByTask(ctx, task.ID, nil)
		if err != nil || len(views) == 0 {
			t.Fatalf("发送步骤缺失应继续默认推演: %v %v", views, err)
		}
	})
	t.Run("未处于退回位", func(t *testing.T) {
		fx := newSimFixture(t)
		instance, task := fx.start("simLinear", "alice", nil)
		fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "apply", Sort: 5, ReturnApprovalStep: "3"})
		fx.steps.Put(&StepRule{DeploymentID: fx.deploymentID, StepCode: "sendStep", Sort: 2})
		fx.seedTaskMessage(instance.ID, task, "alice", "sendStep", "发送步骤")
		views, err := fx.sim.SimulateByTask(ctx, task.ID, nil)
		if err != nil || len(views) == 0 {
			t.Fatalf("当前步骤不早于发送步骤应按非退回处理: %v %v", views, err)
		}
	})
	t.Run("步骤规则未注入", func(t *testing.T) {
		fx := newSimFixture(t)
		_, task := fx.start("simLinear", "alice", nil)
		views, err := fx.simNoSteps.SimulateByTask(ctx, task.ID, nil)
		if err != nil || len(views) == 0 {
			t.Fatalf("步骤规则未注入应继续默认推演: %v %v", views, err)
		}
	})
}

// TestSimCovComposeReturnModes 覆盖退回模式 3/4 与退回步骤同名项剔除分支。
// @param t 测试上下文
func TestSimCovComposeReturnModes(t *testing.T) {
	svc := NewSimulateService(nil, nil, nil, nil, nil, nil, clock.NewFake(simCovNow))
	back := NextUserTaskView{ActivityID: "apply", ActivityName: "申请", ReturnStep: true}
	views := []NextUserTaskView{{ActivityID: "handle", ActivityName: "办理"}, {ActivityID: "apply", ActivityName: "申请"}}
	mode3 := svc.composeReturnModes("3", back, true, views)
	assertSameStrings(t, viewIDs(mode3), []string{"apply", "handle"})
	mode4 := svc.composeReturnModes("4", back, true, views)
	assertSameStrings(t, viewIDs(mode4), []string{"handle", "apply"})
	if len(svc.composeReturnModes("", back, true, views)) != 2 {
		t.Fatal("默认模式应原样返回")
	}
}

// TestSimCovDelegateStoreBranches 覆盖委托规则/镜像存储的禁用过滤、克隆空值、排序交换分支。
// @param t 测试上下文
func TestSimCovDelegateStoreBranches(t *testing.T) {
	ctx := context.Background()
	fake := clock.NewFake(simCovNow)
	store := NewMemTaskDelegateStore(fake)
	disabled := &DelegateRule{
		BaseColumns: BaseColumns{ID: "rule-disabled", CreateTime: simCovNow.Add(2 * time.Hour)},
		ConsignType: ConsignTypeAll, OwnerID: "alice", SourceCode: "dev",
		Enable: DelegateDisabled, Consigners: []DelegateConsigner{{ConsignerID: "bob"}},
	}
	enabled := &DelegateRule{
		BaseColumns: BaseColumns{ID: "rule-enabled", CreateTime: simCovNow.Add(time.Hour)},
		ConsignType: ConsignTypeAll, OwnerID: "alice", SourceCode: "dev",
		Enable: DelegateEnabled, Consigners: []DelegateConsigner{{ConsignerID: "bob"}},
	}
	for _, rule := range []*DelegateRule{disabled, enabled} {
		if err := store.Save(ctx, rule); err != nil {
			t.Fatalf("保存委托规则失败: %v", err)
		}
	}
	// 禁用规则在反查时被跳过
	rules, err := store.ListByConsigner(ctx, "bob", "dev")
	if err != nil || len(rules) != 1 || rules[0].ID != "rule-enabled" {
		t.Fatalf("禁用规则不应命中: %v %v", rules, err)
	}
	// 排序交换（直接驱动插入排序）
	sorted := []*DelegateRule{disabled, enabled}
	sortDelegateRules(sorted)
	if sorted[0].ID != "rule-enabled" || sorted[1].ID != "rule-disabled" {
		t.Fatalf("委托规则排序不符: %v", []string{sorted[0].ID, sorted[1].ID})
	}
	// 克隆空值
	if cloneDelegateRule(nil) != nil {
		t.Fatal("克隆空委托规则应返回 nil")
	}
	if cloneDelegateMirror(nil) != nil {
		t.Fatal("克隆空镜像应返回 nil")
	}
	// 镜像存储排序交换
	mirrors := NewMemDelegateTaskMirrorStore(fake)
	late := &DelegateTaskMirror{MessageExtend: MessageExtend{BaseColumns: BaseColumns{ID: "m-late", CreateTime: simCovNow.Add(2 * time.Hour), TenantID: "dev"}, TaskID: "t1"}, EntrustID: "bob"}
	early := &DelegateTaskMirror{MessageExtend: MessageExtend{BaseColumns: BaseColumns{ID: "m-early", CreateTime: simCovNow.Add(time.Hour), TenantID: "dev"}, TaskID: "t1"}, EntrustID: "bob"}
	for _, mirror := range []*DelegateTaskMirror{late, early} {
		if err := mirrors.Save(ctx, mirror); err != nil {
			t.Fatalf("保存委托镜像失败: %v", err)
		}
	}
	rows, err := mirrors.ListByTaskID(ctx, "t1")
	if err != nil || len(rows) != 2 || rows[0].ID != "m-early" {
		t.Fatalf("镜像排序不符: %v %v", rows, err)
	}
	order := []*DelegateTaskMirror{late, early}
	sortDelegateMirrors(order)
	if order[0].ID != "m-early" || order[1].ID != "m-late" {
		t.Fatalf("镜像排序交换不符: %v", []string{order[0].ID, order[1].ID})
	}
}

// TestSimCovDelegateServiceBranches 覆盖委托规则的字段覆盖、范围缺省、对象必填、引擎缺失与姓名回退分支。
// @param t 测试上下文
func TestSimCovDelegateServiceBranches(t *testing.T) {
	ctx := context.Background()
	fx, svc, store, mirrors := newDelegateFixture(t)
	// 更新：非空字段覆盖 + 有效规则保存成功
	rule := baseRule()
	if err := svc.Add(ctx, rule); err != nil {
		t.Fatalf("新增委托规则失败: %v", err)
	}
	if err := svc.Update(ctx, &DelegateRule{ID: rule.ID, OwnerName: "张三", Type: "协议", Remark: "备注"}); err != nil {
		t.Fatalf("更新委托规则失败: %v", err)
	}
	saved, found, err := store.Get(ctx, rule.ID)
	if err != nil || !found || saved.OwnerName != "张三" || saved.Type != "协议" || saved.Remark != "备注" {
		t.Fatalf("更新字段覆盖不符: %+v %v %v", saved, found, err)
	}
	// 范围缺省回退全部委托
	allRule := baseRule()
	allRule.ConsignType = ""
	allRule.ConsignObjs = nil
	if err := svc.Add(ctx, allRule); err != nil {
		t.Fatalf("缺省范围新增失败: %v", err)
	}
	if allRule.ConsignType != ConsignTypeAll {
		t.Fatalf("缺省范围应回退全部委托: %s", allRule.ConsignType)
	}
	// 项目范围缺少对象
	projectRule := baseRule()
	projectRule.ConsignObjs = nil
	if err := svc.Add(ctx, projectRule); err == nil {
		t.Fatal("项目范围缺少对象应报错")
	}
	// 流程分类范围缺少对象
	categoryRule := baseRule()
	categoryRule.ConsignType = ConsignTypeSingleFlowCategory
	categoryRule.ConsignObjs = nil
	if err := svc.Add(ctx, categoryRule); err == nil {
		t.Fatal("流程分类范围缺少对象应报错")
	}
	// 引擎未注入时定位任务报错
	noEngine := &DelegateRuleService{}
	if _, err := noEngine.engineTask(ctx, "task-x"); err == nil {
		t.Fatal("引擎未注入应报错")
	}
	// 处理人姓名查不到时回退 Id
	names := svc.ownerDisplayNames(ctx, []string{"alice", "ghost-user"})
	if len(names) != 2 || names[0] != "张三" || names[1] != "ghost-user" {
		t.Fatalf("处理人姓名回退不符: %v", names)
	}
	// 时钟未注入时回退真实时钟
	withRealClock := NewDelegateRuleService(store, mirrors, fx.messages, fx.next, fx.operations, fx.engine, fx.engine.IdentityService(), nil, nil)
	if withRealClock.clock == nil {
		t.Fatal("时钟未注入应回退真实时钟")
	}
}

// simCovFacade 构造委托/催办门面测试环境（假时钟 + 内存存储，不部署流程）。
// @param t 测试上下文
// @return 门面服务
func simCovFacade(t *testing.T) *Service {
	t.Helper()
	fake := clock.NewFake(simCovNow)
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
	return NewWorkflowService(eng, WithClock(fake))
}

// TestSimCovFacadeDelegateAndRemind 覆盖门面的委托规则增删改查/启用/注销与催办配置增删改查分支。
// @param t 测试上下文
func TestSimCovFacadeDelegateAndRemind(t *testing.T) {
	service := simCovFacade(t)
	ctx := context.Background()
	rule := &DelegateRule{
		ConsignType: ConsignTypeAll,
		OwnerID:     "u1",
		SourceCode:  "bizA",
		VaildFrom:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		VaildTo:     time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
		Consigners:  []DelegateConsigner{{ConsignerID: "u2"}},
	}
	if _, err := service.AddDelegateTask(ctx, rule); err != nil {
		t.Fatalf("门面新增委托失败: %v", err)
	}
	if rule.ID == "" {
		t.Fatal("门面新增委托应回填 Id")
	}
	list, err := service.ListDelegateTask(ctx, "u1", "bizA")
	if err != nil {
		t.Fatalf("门面委托列表失败: %v", err)
	}
	if rules, ok := list.Data.([]*DelegateRule); !ok || len(rules) != 1 {
		t.Fatalf("门面委托列表不符: %+v", list.Data)
	}
	if _, err := service.EditDelegateTask(ctx, &DelegateRule{ID: rule.ID, Remark: "门面更新"}); err != nil {
		t.Fatalf("门面更新委托失败: %v", err)
	}
	if _, err := service.EnableDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("门面启用委托失败: %v", err)
	}
	if _, err := service.DisableDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("门面停用委托失败: %v", err)
	}
	if _, err := service.RetireDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("门面注销委托失败: %v", err)
	}
	// 不存在规则的更新应报错
	if _, err := service.EditDelegateTask(ctx, &DelegateRule{ID: "ghost"}); err == nil {
		t.Fatal("更新不存在委托应报错")
	}
	// 非法规则新增应报错
	if _, err := service.AddDelegateTask(ctx, &DelegateRule{OwnerID: ""}); err == nil {
		t.Fatal("非法委托新增应报错")
	}
	if _, err := service.DeleteDelegateTask(ctx, rule.ID); err != nil {
		t.Fatalf("门面删除委托失败: %v", err)
	}
	// 催办配置：新增（默认当前处理人对象）
	added, err := service.AddRemind(ctx, RemindSaveRequest{Remind: &Remind{PID: "model-1", StepIDs: "S01", TenantID: "dev"}})
	if err != nil {
		t.Fatalf("门面新增催办失败: %v", err)
	}
	remind, ok := added.Data.(*Remind)
	if !ok || remind.ID == "" {
		t.Fatalf("门面新增催办返回不符: %+v", added.Data)
	}
	if _, err := service.GetRemind(ctx, remind.ID); err != nil {
		t.Fatalf("门面查询催办失败: %v", err)
	}
	// 未命中返回 data 为空
	missing, err := service.GetRemind(ctx, "ghost")
	if err != nil || missing.Data != nil {
		t.Fatalf("未命中催办应返回空数据: %v %v", missing.Data, err)
	}
	// 分页查询
	if _, err := service.GetRemindPageList(ctx, "model-1", PageQuery{Start: 0, Size: 10}); err != nil {
		t.Fatalf("门面分页查询催办失败: %v", err)
	}
	// 更新（重建对象列表）
	if _, err := service.EditRemind(ctx, RemindSaveRequest{
		Remind:  &Remind{ID: remind.ID, StepIDs: "S01,S02"},
		Targets: []*RemindTarget{{ExecutorID: "alice", TargetType: RemindTargetTypeUser, Type: RemindTypeAhead}},
	}); err != nil {
		t.Fatalf("门面更新催办失败: %v", err)
	}
	if _, err := service.DeleteRemind(ctx, remind.ID); err != nil {
		t.Fatalf("门面删除催办失败: %v", err)
	}
}

// TestSimCovRemindStoreBranches 覆盖催办规则空实现、配置保存/发送标记、克隆空值与对象排序分支。
// @param t 测试上下文
func TestSimCovRemindStoreBranches(t *testing.T) {
	ctx := context.Background()
	fake := clock.NewFake(simCovNow)
	// 步骤规则空实现：无自动办结规则
	if rule, err := (NopScheduleRuleProvider{}).AutoCompleteRuleOf(ctx, "S01"); err != nil {
		t.Fatalf("空实现自动办结规则不符: %+v %v", rule, err)
	}
	store := NewMemRemindStore(fake)
	// Id 缺失自动生成
	remind := &Remind{PID: "model-1", TenantID: "dev"}
	if err := store.Save(ctx, remind); err != nil || remind.ID == "" {
		t.Fatalf("保存催办配置失败: %v", err)
	}
	// 标记已发送（命中与未命中都无错误）
	if err := store.MarkSent(ctx, remind.ID); err != nil {
		t.Fatalf("标记已发送失败: %v", err)
	}
	if err := store.MarkSent(ctx, "ghost"); err != nil {
		t.Fatalf("标记不存在配置应无错误: %v", err)
	}
	saved, found, err := store.Get(ctx, remind.ID)
	if err != nil || !found || saved.Sent != 1 {
		t.Fatalf("已发送标记不符: %+v %v %v", saved, found, err)
	}
	// 克隆空值
	if cloneRemind(nil) != nil {
		t.Fatal("克隆空催办配置应返回 nil")
	}
	if cloneRemindTarget(nil) != nil {
		t.Fatal("克隆空催办对象应返回 nil")
	}
	// 对象排序交换
	targets := NewMemRemindTargetStore(fake)
	late := &RemindTarget{ID: "t-late", CreateTime: simCovNow.Add(2 * time.Hour), TenantID: "dev", PID: remind.ID, ExecutorID: "alice", Enable: 1}
	early := &RemindTarget{ID: "t-early", CreateTime: simCovNow.Add(time.Hour), TenantID: "dev", PID: remind.ID, ExecutorID: "bob", Enable: 1}
	for _, target := range []*RemindTarget{late, early} {
		if err := targets.Save(ctx, target); err != nil {
			t.Fatalf("保存催办对象失败: %v", err)
		}
	}
	rows, err := targets.ListByRemind(ctx, remind.ID)
	if err != nil || len(rows) != 2 || rows[0].ID != "t-early" {
		t.Fatalf("催办对象排序不符: %v %v", rows, err)
	}
	order := []*RemindTarget{late, early}
	sortRemindTargets(order)
	if order[0].ID != "t-early" || order[1].ID != "t-late" {
		t.Fatalf("催办对象排序交换不符: %v", []string{order[0].ID, order[1].ID})
	}
	// 通用行浮点读取：缺失列返回 0
	if got := rowFloat(pwf.Row{}, "ahead"); got != 0 {
		t.Fatalf("缺失列浮点值应为 0: %v", got)
	}
	// 时钟未注入回退
	if NewMemRemindStore(nil) == nil {
		t.Fatal("催办存储构造不应返回 nil")
	}
}

// TestSimCovRemindServiceBranches 覆盖催办分页边界、时限缺失、白名单解析、发起人缺失与角色展开分支。
// @param t 测试上下文
func TestSimCovRemindServiceBranches(t *testing.T) {
	ctx := context.Background()
	fake := clock.NewFake(simCovNow)
	reminds := NewMemRemindStore(fake)
	targets := NewMemRemindTargetStore(fake)
	messages := NewMemMessageStore(fake)
	svc := NewRemindService(reminds, targets, NewMemTodoSource(messages), nil, nil, nil, nil, nil, fake)
	for i := 0; i < 2; i++ {
		if _, err := svc.Add(ctx, RemindSaveRequest{Remind: &Remind{PID: "model-1", StepIDs: "S01", TenantID: "dev"}}); err != nil {
			t.Fatalf("新增催办配置失败: %v", err)
		}
	}
	// 负起始位置归零
	page, err := svc.List(ctx, "model-1", PageQuery{Start: -1, Size: 1})
	if err != nil || page.Start != 0 || len(page.Data) != 1 {
		t.Fatalf("负起始分页不符: %+v %v", page, err)
	}
	// 尾页截断
	page, err = svc.List(ctx, "model-1", PageQuery{Start: 1, Size: 99})
	if err != nil || page.Start != 1 || len(page.Data) != 1 {
		t.Fatalf("尾页截断不符: %+v %v", page, err)
	}
	// 时限缺失：接收时间为空
	if due := svc.dueDateOf(ctx, &MessageExtend{}); due != nil {
		t.Fatalf("接收时间为空应返回 nil: %v", due)
	}
	// 白名单：白名单为空
	if svc.inWhitelist(ctx, &Remind{}, &MessageExtend{TaskStepCode: "S01"}) {
		t.Fatal("空白名单不应命中")
	}
	// 白名单：直接按代号匹配
	if !svc.inWhitelist(ctx, &Remind{StepIDs: "S01,S02"}, &MessageExtend{TaskStepCode: "S01"}) {
		t.Fatal("白名单应按代号命中")
	}
	// 白名单：解析端口返回设计时步骤 Id
	resolver := &simCovResolver{stepID: "STEP-9"}
	resolved := NewRemindService(reminds, targets, NewMemTodoSource(messages), nil, nil, resolver, nil, nil, fake)
	if !resolved.inWhitelist(ctx, &Remind{StepIDs: "STEP-9"}, &MessageExtend{TaskStepCode: "S09"}) {
		t.Fatal("白名单应按解析后步骤 Id 命中")
	}
	// 白名单：解析端口未解析到
	resolver.stepID = ""
	if resolved.inWhitelist(ctx, &Remind{StepIDs: "STEP-9"}, &MessageExtend{TaskStepCode: "S09"}) {
		t.Fatal("解析失败不应命中白名单")
	}
	// 发起人：待办为空 / 实例为空
	if got := svc.initiatorOf(ctx, nil); got != "" {
		t.Fatalf("待办为空应返回空发起人: %s", got)
	}
	if got := svc.initiatorOf(ctx, &MessageExtend{}); got != "" {
		t.Fatalf("实例为空应返回空发起人: %s", got)
	}
	// 角色展开：成员端口未注入
	if ids := svc.expandRoleMembers(ctx, "role-1"); ids != nil {
		t.Fatalf("成员端口未注入应返回 nil: %v", ids)
	}
}

// simCovResolver 是步骤代号解析桩。
type simCovResolver struct {
	StepResolver        // 未覆盖方法透传
	stepID       string // 解析结果
}

// ResolveStepID 返回固定步骤 Id（空串表示未解析到）。
// @param ctx 上下文
// @param stepCode 步骤代号
// @return 步骤 Id
// @return 错误
func (r *simCovResolver) ResolveStepID(ctx context.Context, stepCode string) (string, error) {
	return r.stepID, nil
}

// simCovNotifier 是催办送达器桩：可注入失败并记录调用次数。
type simCovNotifier struct {
	fail  bool // 是否返回失败
	count int  // 调用次数
}

// Notify 记录一次送达（可注入失败）。
// @param ctx 上下文
// @param notification 通知报文
// @return 错误
func (n *simCovNotifier) Notify(ctx context.Context, notification *RemindNotification) error {
	n.count++
	if n.fail {
		return apiError("SIM-NOTIFY-001", "送达失败")
	}
	return nil
}

// simCovRemindTargetStore 是催办对象存储桩：可注入发送标记/重试计数失败。
type simCovRemindTargetStore struct {
	RemindTargetStore      // 其余方法透传
	failMarkSent      bool // MarkSent 失败
	failRetry         bool // UpdateRetryCount 失败
}

// MarkSent 标记已发送（可注入失败）。
// @param ctx 上下文
// @param id 对象 Id
// @return 错误
func (s *simCovRemindTargetStore) MarkSent(ctx context.Context, id string) error {
	if s.failMarkSent {
		return apiError("SIM-TARGET-001", "标记已发送失败")
	}
	return s.RemindTargetStore.MarkSent(ctx, id)
}

// UpdateRetryCount 更新重试计数（可注入失败）。
// @param ctx 上下文
// @param id 对象 Id
// @param count 新计数
// @return 错误
func (s *simCovRemindTargetStore) UpdateRetryCount(ctx context.Context, id string, count int) error {
	if s.failRetry {
		return apiError("SIM-TARGET-002", "更新重试计数失败")
	}
	return s.RemindTargetStore.UpdateRetryCount(ctx, id, count)
}

// TestSimCovRemindDeliverBranches 覆盖催办送达的通知失败/发送标记失败/重试计数失败与重试自增分支。
// @param t 测试上下文
func TestSimCovRemindDeliverBranches(t *testing.T) {
	ctx := context.Background()
	fake := clock.NewFake(simCovNow)
	reminds := NewMemRemindStore(fake)
	targetStore := &simCovRemindTargetStore{RemindTargetStore: NewMemRemindTargetStore(fake)}
	svc := NewRemindService(reminds, targetStore, NewMemTodoSource(NewMemMessageStore(fake)), nil, nil, nil, nil, nil, fake)
	newHit := func() *RemindHit {
		return &RemindHit{
			Remind: &Remind{ID: "remind-1"},
			Target: &RemindTarget{
				ID:         "target-1",
				ExecutorID: "alice",
				Enable:     1,
				Sent:       0,
				Retry:      1,
				RetryTimes: 2,
			},
			Todo:    &MessageExtend{Title: "待办标题"},
			Kind:    RemindKindManual,
			Content: "催办内容",
		}
	}
	// 正常送达：送达 1 人，并置已发送 + 重试计数自增
	hit := newHit()
	notifier := &simCovNotifier{}
	delivered, err := svc.Deliver(ctx, hit, notifier)
	if err != nil || delivered != 1 || notifier.count != 1 {
		t.Fatalf("正常送达不符: %d %v %d", delivered, err, notifier.count)
	}
	if hit.Target.Sent != 1 || hit.Target.RetryCount != 1 {
		t.Fatalf("送达标记不符: %+v", hit.Target)
	}
	// 送达器失败：返回已送达数与错误
	failing := &simCovNotifier{fail: true}
	if delivered, err := svc.Deliver(ctx, newHit(), failing); err == nil || delivered != 0 {
		t.Fatalf("送达失败应上抛错误: %d %v", delivered, err)
	}
	// 发送标记失败
	targetStore.failMarkSent = true
	if _, err := svc.Deliver(ctx, newHit(), &simCovNotifier{}); err == nil {
		t.Fatal("发送标记失败应上抛错误")
	}
	targetStore.failMarkSent = false
	// 重试计数失败（已发送对象直接走重试分支）
	retryHit := newHit()
	retryHit.Target.Sent = 1
	targetStore.failRetry = true
	if _, err := svc.Deliver(ctx, retryHit, &simCovNotifier{}); err == nil {
		t.Fatal("重试计数失败应上抛错误")
	}
}

// TestSimCovSimulateMiscBranches 覆盖变量合并内置发起人注入/空变量取值/出线目标缺失分支。
// @param t 测试上下文
func TestSimCovSimulateMiscBranches(t *testing.T) {
	fx := newSimFixture(t)
	ctx := context.Background()
	svc := NewSimulateService(fx.engine, fx.meta, fx.next, fx.histories, fx.messages, fx.engine.IdentityService(), fx.fake)
	// 实例与任务变量都缺失时注入内置发起人
	entityMap, err := svc.buildEntityMap(ctx, "ghost-instance", "ghost-task", "alice", nil)
	if err != nil || entityMap[VarInitiator] != "alice" {
		t.Fatalf("内置发起人注入不符: %+v %v", entityMap, err)
	}
	// 变量集合为空时字符串取值为空
	if got := entityMapValueString(nil, "owner"); got != "" {
		t.Fatalf("空变量集合取值应为空: %s", got)
	}
	// 出线目标节点不存在时遍历终止
	process := simCovWalkModel()
	cands := make([]simCandidate, 0, 1)
	simFollow(process, simRootScope(process), &model.SequenceFlow{BaseElement: model.BaseElement{ID: "lost"}, TargetRef: "ghost"}, nil, true, map[string]bool{}, &cands)
	if len(cands) != 0 {
		t.Fatalf("出线目标缺失不应收集候选: %v", cands)
	}
}

// TestSimCovHistoryStepViewsBranches 覆盖历史步骤上溯的办理人为空/链条为空/链上任务查询失败分支。
// @param t 测试上下文
func TestSimCovHistoryStepViewsBranches(t *testing.T) {
	ctx := context.Background()
	t.Run("办理人为空", func(t *testing.T) {
		fx := newSimFixture(t)
		instance, task := fx.start("simGroup", "alice", nil)
		if views, err := fx.sim.historyStepViews(ctx, instance.ID, task, fx.deploymentID); err != nil || len(views) != 0 {
			t.Fatalf("办理人为空的历史步骤应为空: %v %v", views, err)
		}
	})
	t.Run("上溯链条为空", func(t *testing.T) {
		fx := newSimFixture(t)
		instance, task := fx.start("simLinear", "alice", nil)
		fx.seedNext(instance.ID, "apply", "alice", task.ID, "")
		if views, err := fx.sim.historyStepViews(ctx, instance.ID, task, fx.deploymentID); err != nil || len(views) != 0 {
			t.Fatalf("链条为空的历史步骤应为空: %v %v", views, err)
		}
	})
	t.Run("链上任务查询失败", func(t *testing.T) {
		fx := newSimFixture(t)
		instance, task := fx.start("simLinear", "alice", nil)
		fx.seedNext(instance.ID, "apply", "alice", task.ID, "ghost-task")
		if views, err := fx.sim.historyStepViews(ctx, instance.ID, task, fx.deploymentID); err != nil || len(views) != 0 {
			t.Fatalf("链上任务缺失的历史步骤应为空: %v %v", views, err)
		}
	})
}
