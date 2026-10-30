package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// newFaultBackService 构造注入故障桩的退回服务（存储经故障装饰器包裹）。
// @param t 测试上下文
// @param f 真实测试环境
// @param eng 故障引擎
// @param steps 故障步骤存储
// @param next 故障下一步存储
// @param messages 故障待办存储
// @param histories 故障已办存储
// @param operations 故障操作记录存储
// @param models 故障模型主数据存储
// @return 退回服务
func newFaultBackService(f *backFixture, eng api.Engine, steps, next, messages, histories, operations, models any) *BackService {
	stepStore, _ := steps.(StepStore)
	nextStore, _ := next.(NextActivityStore)
	messageStore, _ := messages.(MessageExtendStore)
	historyStore, _ := histories.(HistoryExtendStore)
	operationStore, _ := operations.(OperationRecordStore)
	modelStore, _ := models.(ModelStore)
	return NewBackService(eng, NewModelMetaService(eng), NewNextActivityService(nextStore, f.fakeClock),
		fxComments(f), fxOperations(f, operationStore), NewMessageExtendService(messageStore, f.fakeClock),
		NewHistoryExtendService(historyStore, f.fakeClock), fxDocuments(f), NewStepRuleService(stepStore, eng),
		engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, f.fakeClock,
		WithBackModels(modelStore), WithBackUsers(errUsers{}))
}

// fxComments 构造意见服务（复用环境依赖）。
// @param f 测试环境
// @return 意见服务
func fxComments(f *backFixture) *CommentService {
	return NewCommentService(f.engine.TaskService(), f.engine.HistoryService(), f.engine.IdentityService())
}

// fxOperations 构造操作记录服务（操作存储可替换为故障装饰器）。
// @param f 测试环境
// @param store 操作记录存储
// @return 操作记录服务
func fxOperations(f *backFixture, store OperationRecordStore) *OperationRecordService {
	return NewOperationRecordService(store, f.engine.HistoryService(), engineInstanceQuery{runtime: f.engine.RuntimeService(), history: f.engine.HistoryService()}, f.engine.RepositoryService(), f.engine.IdentityService(), f.fakeClock)
}

// fxDocuments 构造表单状态服务（复用环境存储）。
// @param f 测试环境
// @return 表单状态服务
func fxDocuments(f *backFixture) *DocumentService {
	return NewDocumentService(f.stores.Document, f.stores.Message, f.stores.Operation, engineInstanceQuery{runtime: f.engine.RuntimeService(), history: f.engine.HistoryService()}, f.engine.HistoryService(), f.fakeClock)
}

// TestBackNodesFaults 验证可退节点计算各依赖故障的错误上抛分支。
// @param t 测试上下文
func TestBackNodesFaults(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	baseStores := NewMemStores(fx.fakeClock)
	cases := []struct {
		name  string
		eng   api.Engine
		steps StepStore
		next  NextActivityStore
		msgs  MessageExtendStore
		hist  HistoryExtendStore
		ops   OperationRecordStore
		model ModelStore
	}{
		{"任务查询故障", &faultEngine{Engine: fx.engine, failGetTask: true}, NewMemStepStore(), baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"定义查询故障", &faultEngine{Engine: fx.engine, failRepo: true}, NewMemStepStore(), baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"步骤规则故障", &faultEngine{Engine: fx.engine}, &faultStepStore{StepStore: fx.steps, failGet: true}, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"模型查询故障", &faultEngine{Engine: fx.engine, failBpmn: true}, NewMemStepStore(), baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"空模型故障", &faultEngine{Engine: fx.engine, emptyModel: true}, NewMemStepStore(), baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"历史活动故障", &faultEngine{Engine: fx.engine, failHistAct: true}, NewMemStepStore(), baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"退回排序故障", &faultEngine{Engine: fx.engine}, NewMemStepStore(), baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, &faultModelStore{ModelStore: fx.models, failQuery: true}},
		{"父链查询故障", &faultEngine{Engine: fx.engine}, NewMemStepStore(), &faultNextStore{NextActivityStore: baseStores.Next, failHisTask: true}, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"白名单故障", &faultEngine{Engine: fx.engine}, &faultStepStore{StepStore: fx.steps, failGet: true}, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"排序索引故障", &faultEngine{Engine: fx.engine}, &faultStepStore{StepStore: fx.steps, failList: true}, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
	}
	for _, c := range cases {
		svc := newFaultBackService(fx, c.eng, c.steps, c.next, c.msgs, c.hist, c.ops, c.model)
		if _, err := svc.BackNodes(ctx, s2.ID); err == nil {
			t.Fatalf("%s 应上抛错误", c.name)
		}
	}
}

// TestBackNodesFaultGetTaskMissing 验证任务 NotFound 与普通错误的分支区分：
// NotFound 返回空列表，普通错误上抛（与故障注入的 failGetTask 用例互补）。
// @param t 测试上下文
func TestBackNodesFaultGetTaskMissing(t *testing.T) {
	fx := newBackFixture(t)
	eng := &faultEngine{Engine: fx.engine, taskMissing: true}
	svc := newFaultBackService(fx, eng, NewMemStepStore(), NewMemStores(fx.fakeClock).Next, NewMemStores(fx.fakeClock).Message, NewMemStores(fx.fakeClock).History, NewMemStores(fx.fakeClock).Operation, fx.models)
	if views, err := svc.BackNodes(context.Background(), "any-task"); err != nil || len(views) != 0 {
		t.Fatalf("任务 NotFound 应返回空列表: %v %v", views, err)
	}
}

// TestBackToStepFaults 验证任意退回收尾链各依赖故障的错误上抛分支。
// 每个用例使用独立流程实例（引擎状态共用，退回会推进实例）。
// @param t 测试上下文
func TestBackToStepFaults(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	deploymentID := deploymentOf(t, fx, "flowComplete")
	fx.steps.Put(&StepRule{DeploymentID: deploymentID, StepCode: "handle", AllowReturn: "1", Sort: 2})
	baseStores := NewMemStores(fx.fakeClock)
	cases := []struct {
		name  string
		eng   api.Engine
		steps StepStore
		next  NextActivityStore
		msgs  MessageExtendStore
		hist  HistoryExtendStore
		ops   OperationRecordStore
		model ModelStore
	}{
		{"引擎退回故障", &faultEngine{Engine: fx.engine, extErr: assertError("退回故障")}, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"模型查询故障", &faultEngine{Engine: fx.engine, failBpmn: true}, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"操作记录故障", &faultEngine{Engine: fx.engine}, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, &faultOperationStore{OperationRecordStore: baseStores.Operation, failSave: true}, fx.models},
		{"预存删除故障", &faultEngine{Engine: fx.engine}, fx.steps, &faultNextStore{NextActivityStore: baseStores.Next, failDelete: true}, baseStores.Message, baseStores.History, baseStores.Operation, fx.models},
		{"待办删除故障", &faultEngine{Engine: fx.engine}, fx.steps, baseStores.Next, &faultMessageStore{MessageExtendStore: baseStores.Message, failDeleteActs: true}, baseStores.History, baseStores.Operation, fx.models},
		{"当前待办删除故障", &faultEngine{Engine: fx.engine}, fx.steps, baseStores.Next, &faultMessageStore{MessageExtendStore: baseStores.Message, failDeleteTask: true}, baseStores.History, baseStores.Operation, fx.models},
		{"已办回写故障", &faultEngine{Engine: fx.engine}, fx.steps, baseStores.Next, baseStores.Message, &faultHistoryStore{HistoryExtendStore: baseStores.History, failFinish: true}, baseStores.Operation, fx.models},
		{"待办重建故障", &faultEngine{Engine: fx.engine}, fx.steps, baseStores.Next, &faultMessageStore{MessageExtendStore: baseStores.Message, failCreate: true}, baseStores.History, baseStores.Operation, fx.models},
		{"已办镜像故障", &faultEngine{Engine: fx.engine}, fx.steps, baseStores.Next, baseStores.Message, &faultHistoryStore{HistoryExtendStore: baseStores.History, failCreate: true}, baseStores.Operation, fx.models},
	}
	for _, c := range cases {
		// 每个用例独立实例：推进到 handle 后执行退回
		instance, apply := startBack(t, fx, "flowComplete", "alice")
		completeBack(t, fx, apply.ID, "alice")
		handle := fx.firstBackTask(t, instance.ID)
		svc := newFaultBackService(fx, c.eng, c.steps, c.next, c.msgs, c.hist, c.ops, c.model)
		if err := svc.BackToStep(ctx, BackTaskVo{TaskID: handle.ID, UserCode: "alice", Message: "退回"}, "apply"); err == nil {
			t.Fatalf("%s 应上抛错误", c.name)
		}
	}
}

// TestStepRuleServiceFaultStore 验证步骤规则服务在存储故障下的错误上抛
// （首次读取成功、第二次读取失败的分支：AllowReturn 通过后 BackStepCodes 失败）。
// @param t 测试上下文
func TestStepRuleServiceFaultStore(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	deploymentID := deploymentOf(t, fx, "flowComplete")
	faulted := &faultStepStore{StepStore: fx.steps, failGetAfter: 1}
	svc := NewStepRuleService(faulted, nil)
	// 第一次读取成功：AllowReturn 正常返回
	if allowed, err := svc.AllowReturn(ctx, deploymentID, "apply"); err != nil || !allowed {
		t.Fatalf("首次读取应成功: %v %v", allowed, err)
	}
	// 第二次读取失败：BackStepCodes/SortOf/StepNameOf/ReturnPercentOf 依次上抛
	if _, err := svc.BackStepCodes(ctx, deploymentID, "apply"); err == nil {
		t.Fatalf("白名单读取故障应上抛")
	}
	if _, err := svc.SortOf(ctx, deploymentID, "apply"); err == nil {
		t.Fatalf("排序号读取故障应上抛")
	}
	if _, err := svc.StepNameOf(ctx, deploymentID, "apply"); err == nil {
		t.Fatalf("步骤名读取故障应上抛")
	}
	if _, _, err := svc.ReturnPercentOf(ctx, deploymentID, "apply"); err == nil {
		t.Fatalf("比例读取故障应上抛")
	}
}

// TestBackToPreviousFaults 验证退回上一节点的错误分支：任务查询故障与模型查询故障。
// @param t 测试上下文
func TestBackToPreviousFaults(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, task := startBack(t, fx, "backFlow3", "alice")
	completeBack(t, fx, task.ID, "alice")
	s2 := fx.firstBackTask(t, instance.ID)
	baseStores := NewMemStores(fx.fakeClock)
	// 引擎退回故障
	eng := &faultEngine{Engine: fx.engine, extErr: assertError("退回故障")}
	svc := newFaultBackService(fx, eng, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if err := svc.BackToPrevious(ctx, BackTaskVo{TaskID: s2.ID, UserCode: "alice", Message: "退回"}); err == nil {
		t.Fatalf("引擎退回故障应上抛")
	}
	// 模型查询故障（退回命令前的模型读取）
	eng2 := &faultEngine{Engine: fx.engine, failBpmn: true}
	svc2 := newFaultBackService(fx, eng2, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	if err := svc2.BackToPrevious(ctx, BackTaskVo{TaskID: s2.ID, UserCode: "alice", Message: "退回"}); err == nil {
		t.Fatalf("模型查询故障应上抛")
	}
}

// TestBackServiceFaultUsers 验证身份查询故障时办理人回退身份 Id。
// @param t 测试上下文
func TestBackServiceFaultUsers(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	svc := newFaultBackService(fx, fx.engine, fx.steps, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation, fx.models)
	// GetUser 故障：回退身份 Id
	if name := svc.displayName(ctx, "alice"); name != "alice" {
		t.Fatalf("身份查询故障应回退身份 Id: %s", name)
	}
}

// TestDirectServiceFaults 验证直送链路各依赖故障的错误上抛分支。
// 每个用例使用独立流程实例（引擎状态共用，直送会推进实例）。
// @param t 测试上下文
func TestDirectServiceFaults(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	baseStores := NewMemStores(fx.fakeClock)
	build := func(eng api.Engine, next, msgs, hist, ops any) *DirectService {
		nextStore, _ := next.(NextActivityStore)
		messageStore, _ := msgs.(MessageExtendStore)
		historyStore, _ := hist.(HistoryExtendStore)
		operationStore, _ := ops.(OperationRecordStore)
		return NewDirectService(eng, NewModelMetaService(eng), NewNextActivityService(nextStore, fx.fakeClock),
			fxComments(fx), fxOperations(fx, operationStore), NewMessageExtendService(messageStore, fx.fakeClock),
			NewHistoryExtendService(historyStore, fx.fakeClock), fxDocuments(fx), fx.completeTo, fx.fakeClock, WithDirectUsers(errUsers{}))
	}
	cases := []struct {
		name string
		eng  api.Engine
		next NextActivityStore
		msgs MessageExtendStore
		hist HistoryExtendStore
		ops  OperationRecordStore
	}{
		{"引擎直送故障", &faultEngine{Engine: fx.engine, extErr: assertError("直送故障")}, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation},
		{"模型查询故障", &faultEngine{Engine: fx.engine, failBpmn: true}, baseStores.Next, baseStores.Message, baseStores.History, baseStores.Operation},
		{"操作记录故障", &faultEngine{Engine: fx.engine}, baseStores.Next, baseStores.Message, baseStores.History, &faultOperationStore{OperationRecordStore: baseStores.Operation, failSave: true}},
		{"当前预存查询故障", &faultEngine{Engine: fx.engine}, &faultNextStore{NextActivityStore: baseStores.Next, failListByTask: true}, baseStores.Message, baseStores.History, baseStores.Operation},
		{"当前预存清理故障", &faultEngine{Engine: fx.engine}, &faultNextStore{NextActivityStore: baseStores.Next, failResolve: true}, baseStores.Message, baseStores.History, baseStores.Operation},
		{"待办删除故障", &faultEngine{Engine: fx.engine}, baseStores.Next, &faultMessageStore{MessageExtendStore: baseStores.Message, failDeleteTask: true}, baseStores.History, baseStores.Operation},
		{"已办回写故障", &faultEngine{Engine: fx.engine}, baseStores.Next, baseStores.Message, &faultHistoryStore{HistoryExtendStore: baseStores.History, failFinish: true}, baseStores.Operation},
		{"待办创建故障", &faultEngine{Engine: fx.engine}, baseStores.Next, &faultMessageStore{MessageExtendStore: baseStores.Message, failCreate: true}, baseStores.History, baseStores.Operation},
		{"已办镜像故障", &faultEngine{Engine: fx.engine}, baseStores.Next, baseStores.Message, &faultHistoryStore{HistoryExtendStore: baseStores.History, failCreate: true}, baseStores.Operation},
	}
	for _, c := range cases {
		// 每个用例独立实例：推进到 handle 后执行直送
		instance, apply := startBack(t, fx, "flowComplete", "alice")
		completeBack(t, fx, apply.ID, "alice")
		handle := fx.firstBackTask(t, instance.ID)
		// 播种当前任务预存记录（播种到用例自己的存储装饰器，直送收尾清理分支可触及）
		caseNext, _ := c.next.(NextActivityStore)
		if err := NewNextActivityService(caseNext, fx.fakeClock).InsertDual(ctx, &NextActivity{
			BaseColumns:       BaseColumns{TenantID: "dev"},
			NextActivityID:    "handle",
			IdentityID:        "alice",
			ProcessInstanceID: instance.ID,
			Used:              1,
			TaskID:            handle.ID,
			Version:           1,
		}, "alice"); err != nil {
			t.Fatal(err)
		}
		vo := &api.CompleteTaskVo{
			BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice", Message: "直送"},
			TargetActivityID: "apply",
			Request: &api.NextActivityRequest{
				TaskID:           handle.ID,
				NextActivityList: []*api.NextActivityVo{{ActivityID: "apply", ExecutorIDs: []string{"bob"}}},
			},
		}
		svc := build(c.eng, c.next, c.msgs, c.hist, c.ops)
		if err := svc.CompleteToTargetActivity(ctx, vo); err == nil {
			t.Fatalf("%s 应上抛错误", c.name)
		}
	}
}

// TestDirectFaultBpmnBeforeEngine 验证直送在模型读取故障时不推进流程（故障在引擎命令前触发）。
// @param t 测试上下文
func TestDirectFaultBpmnBeforeEngine(t *testing.T) {
	fx := newBackFixture(t)
	ctx := context.Background()
	instance, apply := startBack(t, fx, "flowComplete", "alice")
	completeBack(t, fx, apply.ID, "alice")
	handle := fx.firstBackTask(t, instance.ID)
	eng := &faultEngine{Engine: fx.engine, failBpmn: true}
	baseStores := NewMemStores(fx.fakeClock)
	svc := NewDirectService(eng, NewModelMetaService(eng), NewNextActivityService(baseStores.Next, fx.fakeClock),
		fxComments(fx), fxOperations(fx, baseStores.Operation), NewMessageExtendService(baseStores.Message, fx.fakeClock),
		NewHistoryExtendService(baseStores.History, fx.fakeClock), fxDocuments(fx), fx.completeTo, fx.fakeClock)
	// 目标节点执行者校验经模型查询 → failBpmn 上抛
	err := svc.CompleteToTargetActivity(ctx, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice"},
		TargetActivityID: "apply",
		Request: &api.NextActivityRequest{
			TaskID:           handle.ID,
			NextActivityList: []*api.NextActivityVo{{ActivityID: "apply", ExecutorIDs: []string{"bob"}}},
		},
	})
	if err == nil {
		t.Fatalf("模型查询故障应上抛")
	}
	// 实例未推进（任务仍在 handle）
	still := fx.firstBackTask(t, instance.ID)
	if still.TaskDefinitionKey != "handle" {
		t.Fatalf("故障不应推进流程: %+v", still)
	}
}

// TestRowIntOfTypes 验证通用行值转整型的类型分支全集。
// @param t 测试上下文
func TestRowIntOfTypes(t *testing.T) {
	if rowIntOf(nil) != 0 {
		t.Fatalf("nil 应为 0")
	}
	if rowIntOf(7) != 7 || rowIntOf(int32(8)) != 8 || rowIntOf(int64(9)) != 9 || rowIntOf(float64(10)) != 10 {
		t.Fatalf("数值类型转换失败")
	}
	if rowIntOf([]byte("11")) != 11 || rowIntOf("12") != 12 {
		t.Fatalf("字符串类型转换失败")
	}
	if rowIntOf([]byte("bad")) != 0 || rowIntOf("bad") != 0 || rowIntOf(struct{}{}) != 0 {
		t.Fatalf("非法输入应回退 0")
	}
	_ = time.Now
}
