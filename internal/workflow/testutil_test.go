package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// completeFixtureTime 是办结测试的固定时钟基准。
var completeFixtureTime = time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)

// flowCompleteProcess 是串行两节点办结测试语料：apply → handle → end（处理人均发起人）。
const flowCompleteProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="flowComplete" name="办结测试流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="handle"/>
    <userTask id="handle" name="办理" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="handle" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// flowFreeSelectProcess 是自由选路兜底语料：apply 后经排他网关分叉为 approve（条件）/ review（默认）。
const flowFreeSelectProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="flowFreeSelect" name="自由选路流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="gate"/>
    <exclusiveGateway id="gate" default="f3"/>
    <sequenceFlow id="f2" sourceRef="gate" targetRef="approve">
      <conditionExpression>${pass == true}</conditionExpression>
    </sequenceFlow>
    <sequenceFlow id="f3" sourceRef="gate" targetRef="review"/>
    <userTask id="approve" name="批准" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f4" sourceRef="approve" targetRef="end"/>
    <userTask id="review" name="复核" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f5" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// flowWeightProcess 是权重会签语料：wtask（weightType=custom）→ w2 → end。
// 权重名单：alice 通过 2.5/拒绝 1；bob 仅通过 1；carl 仅拒绝 2（用于未配置分支）。
const flowWeightProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="flowWeight" name="权重会签流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="wtask"/>
    <userTask id="wtask" name="权重任务" flowable:assignee="${initiator}"
              weightType="custom" weightThreshold="60"
              weightList='[{"identityId":"alice","agreeWeight":2.5,"rejectWeight":1},{"identityId":"bob","agreeWeight":1},{"identityId":"carl","rejectWeight":2}]'/>
    <sequenceFlow id="f1" sourceRef="wtask" targetRef="w2"/>
    <userTask id="w2" name="后续任务" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="w2" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// flowGroupProcess 是无处理人节点语料：gtask → gnext（仅候选组，无 assignee）→ end。
const flowGroupProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="flowGroup" name="组任务流程" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="gtask"/>
    <userTask id="gtask" name="组前任务" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="gtask" targetRef="gnext"/>
    <userTask id="gnext" name="组任务" flowable:candidateGroups="grpA"/>
    <sequenceFlow id="f2" sourceRef="gnext" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// engineInstanceQuery 适配器已上移到生产代码 facade.go（测试与门面共用）。

// completeFixture 是办结/查询测试的完整环境（内存引擎 + MemStores + 全套域服务）。
type completeFixture struct {
	engine     api.Engine                    // 内存引擎
	stores     *MemStores                    // 五大域内存存储
	completeTo *MemCompleteToActivityStore   // 直送记录内存存储
	meta       *ModelMetaService             // 模型元数据
	next       *NextActivityService          // 下一步预生成
	comments   *CommentService               // 意见
	operations *OperationRecordService       // 操作记录
	messages   *MessageExtendService         // 待办扩展
	histories  *HistoryExtendService         // 已办扩展
	documents  *DocumentService              // 表单状态机
	complete   *CompleteService              // 办结服务
	query      *QueryService                 // 查询门面
	fakeClock  *clock.Fake                   // 假时钟（推进时间用）
}

// newCompleteFixture 构造办结/查询测试环境（部署全部语料、创建测试用户）。
// @param t 测试上下文
// @return 测试环境
func newCompleteFixture(t *testing.T) *completeFixture {
	t.Helper()
	fake := clock.NewFake(completeFixtureTime)
	eng, err := engine.New(engine.Config{
		HistoryLevel: engine.HistoryAudit,
		Clock:        fake,
		IDs:          idgen.NewUUIDv7(),
		TenantID:     "dev", // 默认租户与部署/发起一致，避免租户校验拒绝
	})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(context.Background()) })
	// 部署全部测试语料（一次部署四个定义；租户与发起请求一致）
	if _, err := eng.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:     "complete-test",
		TenantID: "dev",
		Resources: map[string][]byte{
			"complete.bpmn20.xml":   []byte(flowCompleteProcess),
			"freeselect.bpmn20.xml": []byte(flowFreeSelectProcess),
			"weight.bpmn20.xml":     []byte(flowWeightProcess),
			"group.bpmn20.xml":      []byte(flowGroupProcess),
		},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	// 创建测试用户（补姓名展示断言）
	for _, user := range []api.User{
		{ID: "alice", DisplayName: "张三"},
		{ID: "bob", DisplayName: "李四"},
		{ID: "carl", DisplayName: "王五"},
		{ID: "helper", DisplayName: "加签人"},
	} {
		if _, err := eng.IdentityService().CreateUser(context.Background(), user); err != nil {
			t.Fatalf("创建用户 %s 失败: %v", user.ID, err)
		}
	}
	fx := &completeFixture{engine: eng, fakeClock: fake}
	fx.stores = NewMemStores(fake)
	fx.completeTo = NewMemCompleteToActivityStore(fake)
	fx.meta = NewModelMetaService(eng)
	fx.next = NewNextActivityService(fx.stores.Next, fake)
	fx.comments = NewCommentService(eng.TaskService(), eng.HistoryService(), eng.IdentityService())
	fx.operations = NewOperationRecordService(fx.stores.Operation, eng.HistoryService(), engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.RepositoryService(), eng.IdentityService(), fake)
	fx.messages = NewMessageExtendService(fx.stores.Message, fake)
	fx.histories = NewHistoryExtendService(fx.stores.History, fake)
	fx.documents = NewDocumentService(fx.stores.Document, fx.stores.Message, fx.stores.Operation, engineInstanceQuery{runtime: eng.RuntimeService(), history: eng.HistoryService()}, eng.HistoryService(), fake)
	fx.complete = NewCompleteService(eng, fx.meta, fx.next, fx.comments, fx.operations, fx.messages, fx.histories, fx.documents, fx.completeTo, eng.IdentityService(), fake)
	fx.query = NewQueryService(eng, fx.messages, fx.histories, fx.operations, fx.documents)
	return fx
}

// start 发起流程实例并返回实例与首个任务（测试内直接驱动引擎，等价 P1-D 发起辅助的最小形态）。
// @param t 测试上下文
// @param definitionKey 流程定义 Key
// @param businessKey 业务单据 Id
// @param initiator 发起人
// @return 流程实例
// @return 首个用户任务
func (f *completeFixture) start(t *testing.T, definitionKey, businessKey, initiator string) (api.ProcessInstance, api.Task) {
	t.Helper()
	instance, err := f.engine.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: definitionKey,
		BusinessKey:          businessKey,
		Name:                 "标题-" + businessKey,
		TenantID:             "dev",
		UserID:               initiator,
		Variables:            []api.Variable{{Name: VarInitiator, Value: initiator}},
	})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	tasks, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("查询首任务失败: %v（%d 个）", err, len(tasks.Data))
	}
	return instance, tasks.Data[0]
}

// seedFirstTodo 为首个任务播种发起侧数据（对位 P1-D 发起辅助产物）：
// 待办 + 已办镜像 + 首步已消耗预存记录 + TJ 意见/操作记录 + 初始"未提交"表单状态。
// @param t 测试上下文
// @param instance 流程实例
// @param task 首个任务
func (f *completeFixture) seedFirstTodo(t *testing.T, instance api.ProcessInstance, task api.Task) {
	t.Helper()
	ctx := context.Background()
	// 待办 + 已办镜像
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		TaskID:            task.ID,
		ProcessInstanceID: instance.ID,
		Title:             instance.Name,
		BusinessID:        instance.BusinessKey,
		TaskStepName:      task.Name,
		TaskStepCode:      task.TaskDefinitionKey,
		FlowCategoryCode:  "flowComplete",
		FlowCategoryName:  "办结测试流程",
		OwnerID:           task.Assignee,
		DocumentOwnerID:   instance.StartUserID,
		TaskReceiptTime:   &task.CreateTime,
	}
	message.SenderID = "starter"
	if _, err := f.messages.CreateForTask(ctx, message); err != nil {
		t.Fatalf("播种待办失败: %v", err)
	}
	if _, err := f.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
		t.Fatalf("播种已办镜像失败: %v", err)
	}
	// 首步预存记录（used=1，taskId=首任务）
	if err := f.next.InsertDual(ctx, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		NextActivityID:    task.TaskDefinitionKey,
		IdentityID:        task.Assignee,
		SendIdentityID:    "starter",
		ProcessInstanceID: instance.ID,
		Used:              1,
		TaskID:            task.ID,
		Version:           1,
	}, "starter"); err != nil {
		t.Fatalf("播种首步预存失败: %v", err)
	}
	// 发起 TJ 意见与操作记录
	if _, err := f.comments.Add(ctx, CommentRequest{
		TaskID: task.ID, ProcessInstanceID: instance.ID, UserID: instance.StartUserID, Message: "发起申请", Type: CommentTypeTJ,
	}); err != nil {
		t.Fatalf("播种发起意见失败: %v", err)
	}
	if _, err := f.operations.Save(ctx, OperationRequest{
		TaskID: task.ID, ProcessInstanceID: instance.ID, OwnerID: instance.StartUserID, Message: "发起申请", Type: CommentTypeTJ,
	}); err != nil {
		t.Fatalf("播种发起操作记录失败: %v", err)
	}
	// 初始表单状态（未提交）
	if _, err := f.documents.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: instance.TenantID},
		BusinessID:     instance.BusinessKey,
		FlowInstanceID: instance.ID,
		FlowTitle:      instance.Name,
	}}); err != nil {
		t.Fatalf("播种表单状态失败: %v", err)
	}
}

// firstTaskOf 返回实例当前的首个任务（断言辅助）。
// @param t 测试上下文
// @param instanceID 实例 Id
// @return 任务
func (f *completeFixture) firstTaskOf(t *testing.T, instanceID string) api.Task {
	t.Helper()
	page, err := f.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("实例 %s 无任务", instanceID)
	}
	return page.Data[0]
}
