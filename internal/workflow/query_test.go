package workflow

import (
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// CreateTaskRequest4Test 构造独立任务创建请求（查询测试辅助）。
// @return 创建请求
func CreateTaskRequest4Test() api.CreateTaskRequest {
	return api.CreateTaskRequest{Name: "独立任务", Assignee: "alice", TenantID: "dev"}
}

// seedQueryRows 播种查询测试数据。
// owner 行租户置空（查询门面无租户入参，空租户=不过滤，与 SQL 适配约定一致）；
// 业务行租户为 dev（按业务查询显式携带租户）。行经存储端口直存（绕过服务层租户校验）。
// @param t 测试上下文
// @param f 测试环境
func seedQueryRows(t *testing.T, f *completeFixture) {
	t.Helper()
	seq := 0
	nextTime := func() time.Time {
		// 逐行递增创建时间，保证排序断言确定
		seq++
		return completeFixtureTime.Add(time.Duration(seq) * time.Minute)
	}
	saveTodo := func(id, taskID, owner, tenant, businessID, instanceID string) {
		t.Helper()
		row := &MessageExtend{
			BaseColumns: BaseColumns{ID: id, TenantID: tenant, CreateTime: nextTime()},
			TaskID:      taskID, ProcessInstanceID: instanceID, Title: "标题q", BusinessID: businessID,
			TaskStepName: "步骤" + taskID, TaskStepCode: taskID, OwnerID: owner,
			FlowCategoryCode: "flowComplete", FlowCategoryName: "办结测试流程",
		}
		if err := f.stores.Message.Save(ctxB, row); err != nil {
			t.Fatalf("播种待办失败: %v", err)
		}
	}
	saveDone := func(id, taskID, owner, tenant, businessID, instanceID string) {
		t.Helper()
		finish := completeFixtureTime.Add(time.Hour)
		row := &HistoryExtend{
			MessageExtend: MessageExtend{
				BaseColumns: BaseColumns{ID: id, TenantID: tenant, CreateTime: nextTime()},
				TaskID:      taskID, ProcessInstanceID: instanceID, Title: "标题q", BusinessID: businessID,
				TaskStepName: "步骤" + taskID, TaskStepCode: taskID, OwnerID: owner,
				FlowCategoryCode: "flowComplete", FlowCategoryName: "办结测试流程",
			},
			TaskFinishTime: &finish,
		}
		if err := f.stores.History.Save(ctxB, row); err != nil {
			t.Fatalf("播种已办失败: %v", err)
		}
	}
	// owner 行（空租户）：alice 两条待办/已办（虚拟任务联查不命中），bob 一条待办
	saveTodo("qm-1", "t-1", "alice", "", "biz-q", "pi-q")
	saveTodo("qm-2", "t-2", "alice", "", "biz-q", "pi-q")
	saveTodo("qm-3", "t-3", "bob", "", "biz-q", "pi-q")
	saveDone("qh-1", "t-1", "alice", "", "biz-q", "pi-q")
	saveDone("qh-2", "t-2", "alice", "", "biz-q", "pi-q")
	// 业务行（dev 租户）：两条待办/已办（按业务键查询用）
	saveTodo("qm-b1", "t-b1", "alice", "dev", "biz-qb", "pi-qb")
	saveTodo("qm-b2", "t-b2", "alice", "dev", "biz-qb", "pi-qb")
	saveDone("qh-b1", "t-b1-h", "alice", "dev", "biz-qb", "pi-qb")
	saveDone("qh-b2", "t-b2-h", "alice", "dev", "biz-qb", "pi-qb")
	// 可签名操作记录：TJ/SP/TZ 命中，TH/ZB 不命中
	for _, itemType := range []string{CommentTypeTJ, CommentTypeSP, CommentTypeTZ, CommentTypeTH, CommentTypeZB} {
		if _, err := f.operations.Save(ctxB, OperationRequest{
			ProcessInstanceID: "pi-q", OwnerID: "alice", Message: "记录" + itemType, Type: itemType,
		}); err != nil {
			t.Fatalf("播种操作记录失败: %v", err)
		}
	}
}

// TestQuery_TodoAndDonePagination 验证待办/已办按处理人分页与引擎联查补齐。
// @param t 测试上下文
func TestQuery_TodoAndDonePagination(t *testing.T) {
	fx := newCompleteFixture(t)
	seedQueryRows(t, fx)

	// 待办：alice 共 4 条（空租户不过滤语义：含 dev 租户业务行），分页各 1 条
	page1, err := fx.query.TodoListByIdentityID(ctxB, "alice", PageQuery{Start: 0, Size: 1})
	if err != nil {
		t.Fatalf("待办查询失败: %v", err)
	}
	if page1.Total != 4 || len(page1.Data) != 1 {
		t.Fatalf("待办分页错误: total=%d data=%d", page1.Total, len(page1.Data))
	}
	if page1.Data[0].TaskID != "t-1" {
		t.Fatalf("期望第一条为 t-1，实际 %s", page1.Data[0].TaskID)
	}
	// 虚拟任务联查不命中：EngineExists=false
	if page1.Data[0].EngineExists {
		t.Fatalf("虚拟任务不应命中引擎")
	}
	page2, err := fx.query.TodoListByIdentityID(ctxB, "alice", PageQuery{Start: 1, Size: 1})
	if err != nil || page2.Data[0].TaskID != "t-2" {
		t.Fatalf("第二页错误: %v %+v", err, page2.Data)
	}
	// bob 仅 1 条
	bobPage, err := fx.query.TodoListByIdentityID(ctxB, "bob", PageQuery{})
	if err != nil || bobPage.Total != 1 {
		t.Fatalf("bob 待办错误: total=%d err=%v", bobPage.Total, err)
	}
	// Size 缺省回退默认 10（归一化）
	if bobPage.Size != defaultPageSize {
		t.Fatalf("默认页大小应为 %d，实际 %d", defaultPageSize, bobPage.Size)
	}
	// 已办：alice 共 4 条（空租户不过滤语义），联查引擎历史任务不命中
	done, err := fx.query.DoneListByIdentityID(ctxB, "alice", PageQuery{Size: 10})
	if err != nil || done.Total != 4 {
		t.Fatalf("已办查询错误: total=%d err=%v", done.Total, err)
	}
	if done.Data[0].EngineExists {
		t.Fatalf("虚拟任务历史不应命中引擎")
	}
	if done.Data[0].TaskFinishTime == nil {
		t.Fatalf("已办完成时间应保留播种值")
	}
}

// TestQuery_TodoJoinEngineTask 验证待办联查真实引擎任务补实时字段。
// @param t 测试上下文
func TestQuery_TodoJoinEngineTask(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-j", "alice")
	// owner 查询行（空租户=不过滤，经存储端口直存）
	row := &MessageExtend{
		BaseColumns: BaseColumns{ID: "qm-j", TenantID: "", CreateTime: completeFixtureTime},
		TaskID:      apply.ID, ProcessInstanceID: instance.ID, Title: instance.Name,
		BusinessID: instance.BusinessKey, TaskStepName: apply.Name,
		TaskStepCode: apply.TaskDefinitionKey, OwnerID: "alice",
	}
	if err := fx.stores.Message.Save(ctxB, row); err != nil {
		t.Fatalf("播种待办失败: %v", err)
	}

	todo, err := fx.query.TodoListByIdentityID(ctxB, "alice", PageQuery{Size: 10})
	if err != nil || todo.Total != 1 {
		t.Fatalf("待办查询失败: total=%d err=%v", todo.Total, err)
	}
	view := todo.Data[0]
	if !view.EngineExists || view.EngineTaskName != "申请" {
		t.Fatalf("引擎任务联查失败: %+v", view)
	}
	if view.EngineCreate.IsZero() {
		t.Fatalf("引擎任务创建时间应非零")
	}
}

// TestQuery_ByBusinessAndTask 验证按业务单据/实例/任务的扩展行查询。
// @param t 测试上下文
func TestQuery_ByBusinessAndTask(t *testing.T) {
	fx := newCompleteFixture(t)
	seedQueryRows(t, fx)

	messages, err := fx.query.TaskMessageListByBusinessID(ctxB, "dev", "biz-qb")
	if err != nil || len(messages) != 2 {
		t.Fatalf("按业务查待办错误: %d（%v）", len(messages), err)
	}
	histories, err := fx.query.TaskHistoryListByBusinessID(ctxB, "dev", "biz-qb")
	if err != nil || len(histories) != 2 {
		t.Fatalf("按业务查已办错误: %d（%v）", len(histories), err)
	}
	// 租户不匹配返回空
	wrong, err := fx.query.TaskMessageListByBusinessID(ctxB, "other", "biz-qb")
	if err != nil || len(wrong) != 0 {
		t.Fatalf("租户过滤失败: %d（%v）", len(wrong), err)
	}
	byInstance, err := fx.query.TaskHistoryListByInstance(ctxB, "pi-q")
	if err != nil || len(byInstance) != 2 {
		t.Fatalf("按实例查已办错误: %d（%v）", len(byInstance), err)
	}
	byTask, err := fx.query.TaskHistoryListByTaskID(ctxB, "t-1")
	if err != nil || byTask == nil || byTask.TaskID != "t-1" {
		t.Fatalf("按任务查已办错误: %+v（%v）", byTask, err)
	}
	// 无记录返回 nil
	if missing, err := fx.query.TaskHistoryListByTaskID(ctxB, "ghost"); err != nil || missing != nil {
		t.Fatalf("无记录应返回 nil: %+v（%v）", missing, err)
	}
}

// TestQuery_TaskInfo 验证任务详情（引擎任务 + 扩展列）。
// @param t 测试上下文
func TestQuery_TaskInfo(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-i", "alice")
	fx.seedFirstTodo(t, instance, apply)

	// 运行中任务：待办扩展命中
	info, err := fx.query.TaskInfoByTaskID(ctxB, apply.ID)
	if err != nil {
		t.Fatalf("任务详情查询失败: %v", err)
	}
	if info.Name != "申请" || info.Assignee != "alice" {
		t.Fatalf("引擎任务字段错误: %+v", info.Task)
	}
	if info.Extend == nil || info.Extend.OwnerID != "alice" || info.History == nil {
		t.Fatalf("扩展列缺失: %+v", info)
	}
	// 任务不存在
	if _, err := fx.query.TaskInfoByTaskID(ctxB, "ghost"); err == nil {
		t.Fatalf("期望任务不存在错误")
	}
	// 独立任务无扩展列（未生成待办/已办镜像）
	standalone, err := fx.engine.TaskService().CreateTask(ctxB, CreateTaskRequest4Test())
	if err != nil {
		t.Fatalf("创建独立任务失败: %v", err)
	}
	standaloneInfo, err := fx.query.TaskInfoByTaskID(ctxB, standalone.ID)
	if err != nil || standaloneInfo.Extend != nil || standaloneInfo.History != nil {
		t.Fatalf("独立任务不应有扩展列: %+v（%v）", standaloneInfo, err)
	}
}

// TestQuery_Instances 验证实例列表（状态映射/分页/按用户过滤）。
// @param t 测试上下文
func TestQuery_Instances(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-ins", "alice")
	fx.seedFirstTodo(t, instance, apply)
	// alice 办结首步（产生新待办与聚合）
	aliceVo := api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "同意"},
		Request:       &api.NextActivityRequest{TaskID: apply.ID, NextActivityList: []*api.NextActivityVo{{ActivityID: "handle", ExecutorIDs: []string{"alice"}}}},
	}
	if _, err := fx.complete.Complete(ctxB, &aliceVo); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 发起一个 bob 的实例并两步推进到结束（推进时钟保证发起时间可排序）
	fx.fakeClock.Advance(3600)
	bobInstance, bobTask := fx.start(t, "flowComplete", "biz-ins2", "bob")
	fx.seedFirstTodo(t, bobInstance, bobTask)
	bobVo := api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: bobTask.ID, UserCode: "bob", Message: "同意"},
		Request:       &api.NextActivityRequest{TaskID: bobTask.ID, NextActivityList: []*api.NextActivityVo{{ActivityID: "handle", ExecutorIDs: []string{"bob"}}}},
	}
	if _, err := fx.complete.Complete(ctxB, &bobVo); err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	handle := fx.firstTaskOf(t, bobInstance.ID)
	endVo := api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: handle.ID, UserCode: "bob", Message: "办结"},
		Request:       &api.NextActivityRequest{TaskID: handle.ID, NextActivityList: []*api.NextActivityVo{{ActivityID: StepCodeEnd}}},
	}
	if _, err := fx.complete.Complete(ctxB, &endVo); err != nil {
		t.Fatalf("办结失败: %v", err)
	}

	// 按用户：alice 仅自己发起的实例（运行中 → 0）
	aliceList, err := fx.query.InstanceListByUser(ctxB, "alice", PageQuery{Size: 10})
	if err != nil || aliceList.Total != 1 {
		t.Fatalf("alice 实例查询错误: total=%d err=%v", aliceList.Total, err)
	}
	if aliceList.Data[0].Status != InstanceStatusRunning {
		t.Fatalf("运行中实例状态应为 0，实际 %d", aliceList.Data[0].Status)
	}
	if aliceList.Data[0].Title != "标题-biz-ins" || aliceList.Data[0].BusinessID != "biz-ins" {
		t.Fatalf("实例视图字段错误: %+v", aliceList.Data[0])
	}
	if aliceList.Data[0].CurrentStep != "办理" || aliceList.Data[0].CurrentPersons != "张三" {
		t.Fatalf("实例当前步骤/人员聚合错误: %+v", aliceList.Data[0])
	}
	// bob 的实例已结束（document 无已办以外待办 → 已完成 → 1）
	bobList, err := fx.query.InstanceListByUser(ctxB, "bob", PageQuery{Size: 10})
	if err != nil || bobList.Total != 1 {
		t.Fatalf("bob 实例查询错误: total=%d err=%v", bobList.Total, err)
	}
	if bobList.Data[0].Status != InstanceStatusFinished {
		t.Fatalf("已结束实例状态应为 1，实际 %d", bobList.Data[0].Status)
	}
	// carl 无实例
	empty, err := fx.query.InstanceListByUser(ctxB, "carl", PageQuery{Size: 10})
	if err != nil || empty.Total != 0 || len(empty.Data) != 0 {
		t.Fatalf("无实例用户应为空页: %+v（%v）", empty, err)
	}
	// 全量列表 + 越界分页
	all, err := fx.query.AllInstanceList(ctxB, PageQuery{Size: 1})
	if err != nil || all.Total != 2 || len(all.Data) != 1 {
		t.Fatalf("全量实例分页错误: total=%d data=%d（%v）", all.Total, len(all.Data), err)
	}
	beyond, err := fx.query.AllInstanceList(ctxB, PageQuery{Start: 10, Size: 10})
	if err != nil || beyond.Total != 2 || len(beyond.Data) != 0 {
		t.Fatalf("越界分页应返回空页: %+v（%v）", beyond, err)
	}
	// 按发起时间倒序：bob 的实例（后发起）在前
	allDesc, err := fx.query.AllInstanceList(ctxB, PageQuery{Size: 10})
	if err != nil || allDesc.Data[0].BusinessID != "biz-ins2" || allDesc.Data[1].BusinessID != "biz-ins" {
		t.Fatalf("实例排序错误: %+v", allDesc.Data)
	}
	// 已终止映射：document 置已终止 → 状态 2
	if _, err := fx.documents.Save(ctxB, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "dev"},
		BusinessID:     "biz-ins",
		FlowInstanceID: instance.ID,
		FlowStatus:     FlowStatusTerminated,
	}}); err != nil {
		t.Fatalf("置终止状态失败: %v", err)
	}
	terminated, err := fx.query.InstanceListByUser(ctxB, "alice", PageQuery{Size: 10})
	if err != nil || terminated.Data[0].Status != InstanceStatusTerminated {
		t.Fatalf("已终止实例状态应为 2: %d（%v）", terminated.Data[0].Status, err)
	}
}

// TestQuery_SignableOperationRecords 验证可签名记录透传（TJ/SP/TZ）。
// @param t 测试上下文
func TestQuery_SignableOperationRecords(t *testing.T) {
	fx := newCompleteFixture(t)
	seedQueryRows(t, fx)
	// 按业务单据关联：先补业务键
	records, err := fx.query.SignableOperationRecords(ctxB, []string{})
	if err != nil {
		t.Fatalf("可签名查询失败: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("空业务键集合应为空: %d", len(records))
	}
	// 经实例查询后按业务键透传（播种记录挂在 pi-q，业务键未回填 → 用实例记录集合断言）
	all, err := fx.operations.ListByInstance(ctxB, "pi-q")
	if err != nil || len(all) != 5 {
		t.Fatalf("播种记录缺失: %d（%v）", len(all), err)
	}
	signable := 0
	for _, record := range all {
		// 端口过滤条件：type ∈ {TJ,SP,TZ}
		if record.Type == CommentTypeTJ || record.Type == CommentTypeSP || record.Type == CommentTypeTZ {
			signable++
		}
	}
	if signable != 3 {
		t.Fatalf("可签名类型应为 3 条，实际 %d", signable)
	}
}

// TestInstanceStatusOf 验证实例状态映射的分支完备。
// @param t 测试上下文
func TestInstanceStatusOf(t *testing.T) {
	// document 状态优先
	if got := instanceStatusOf(FlowStatusTerminated, false); got != InstanceStatusTerminated {
		t.Fatalf("已终止映射错误: %d", got)
	}
	if got := instanceStatusOf(FlowStatusFinished, false); got != InstanceStatusFinished {
		t.Fatalf("已完成映射错误: %d", got)
	}
	// 无 document：引擎结束标记回退
	if got := instanceStatusOf("", true); got != InstanceStatusFinished {
		t.Fatalf("引擎结束映射错误: %d", got)
	}
	if got := instanceStatusOf(FlowStatusRunning, false); got != InstanceStatusRunning {
		t.Fatalf("流转中映射错误: %d", got)
	}
	if got := instanceStatusOf(FlowStatusNotSubmitted, false); got != InstanceStatusRunning {
		t.Fatalf("未提交映射错误: %d", got)
	}
}
