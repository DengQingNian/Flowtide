package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// ctxB 是测试上下文快捷别名。
var ctxB = context.Background()

// TestComplete_FullChain 验证办结全链路：预存下一步 → 办结 → 新待办落位/旧待办删除/
// 已办完成时间/document 状态流转/SP 意见/操作记录断言，及到 end 的完结态。
// @param t 测试上下文
func TestComplete_FullChain(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-1", "alice")
	fx.seedFirstTodo(t, instance, apply)

	// 第一次办结：apply → handle（预存下一步）
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "同意申请", ProcessInstanceID: instance.ID, Type: CommentTypeSP},
		Request: &api.NextActivityRequest{TaskID: apply.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: "handle", ExecutorIDs: []string{"alice"}},
		}},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 结果断言：未结束且有新待办
	if result.Finished {
		t.Fatalf("流程不应已结束")
	}
	handle := fx.firstTaskOf(t, instance.ID)
	if handle.TaskDefinitionKey != "handle" {
		t.Fatalf("期望推进到 handle，实际 %s", handle.TaskDefinitionKey)
	}
	if len(result.NextTaskIDs) != 1 || result.NextTaskIDs[0] != handle.ID {
		t.Fatalf("期望新待办 %s，实际 %v", handle.ID, result.NextTaskIDs)
	}
	// 旧待办删除、新待办落位（含发送人/步骤信息）
	if _, found, _ := fx.messages.GetByTaskID(ctxB, apply.ID); found {
		t.Fatalf("旧待办应已删除")
	}
	todo, found, err := fx.messages.GetByTaskID(ctxB, handle.ID)
	if err != nil || !found {
		t.Fatalf("新待办应已生成: %v", err)
	}
	if todo.OwnerID != "alice" || todo.OwnerName != "张三" {
		t.Fatalf("待办处理人错误: %+v", todo)
	}
	if todo.Title != "标题-biz-1" || todo.BusinessID != "biz-1" || todo.TaskStepCode != "handle" {
		t.Fatalf("待办业务字段错误: %+v", todo)
	}
	if todo.SenderID != "alice" || todo.Sender != "张三" || todo.SenderStepCode != "apply" {
		t.Fatalf("待办发送人信息错误: %+v", todo)
	}
	if todo.TaskReceiptTime == nil || !todo.TaskReceiptTime.Equal(handle.CreateTime) {
		t.Fatalf("待办签收时间应取任务创建时间: %+v", todo.TaskReceiptTime)
	}
	// 已办镜像：办结任务完成时间回写，新任务镜像未完成
	doneApply, found, _ := fx.histories.GetByTaskID(ctxB, apply.ID)
	if !found || doneApply.TaskFinishTime == nil {
		t.Fatalf("已办完成时间应已回写: %v", doneApply)
	}
	doneHandle, found, _ := fx.histories.GetByTaskID(ctxB, handle.ID)
	if !found || doneHandle.TaskFinishTime != nil {
		t.Fatalf("新任务已办镜像应存在且未完成: %v", doneHandle)
	}
	// SP 意见与操作记录（isAgree=1；同刻时钟下顺序不稳定，按内容/类型检索）
	comments, err := fx.engine.TaskService().ListComments(ctxB, apply.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("期望 2 条意见（发起 TJ + 办结 SP），实际 %d: %v", len(comments), err)
	}
	if !containsMessage(comments, "同意申请") {
		t.Fatalf("办结意见内容错误: %+v", comments)
	}
	records, err := fx.stores.Operation.ListByTask(ctxB, apply.ID)
	if err != nil || len(records) != 2 {
		t.Fatalf("期望 2 条操作记录，实际 %d: %v", len(records), err)
	}
	var spRecord *OperationRecord
	for _, record := range records {
		if record.Type == CommentTypeSP {
			spRecord = record
		}
	}
	if spRecord == nil || spRecord.IsAgree == nil || *spRecord.IsAgree != 1 {
		t.Fatalf("办结操作记录缺失: %+v", records)
	}
	if spRecord.TaskStepCode != "apply" || spRecord.BusinessID != "biz-1" || spRecord.OwnerID != "alice" {
		t.Fatalf("操作记录补全字段错误: %+v", spRecord)
	}
	// 预存记录：apply 节点已清理，handle 节点保留未消耗记录
	if left, _ := fx.next.ListByTaskID(ctxB, apply.ID); len(left) != 0 {
		t.Fatalf("当前节点预存记录应已删除，剩 %d 条", len(left))
	}
	unused, err := fx.next.ListUnusedByInstanceAndActivity(ctxB, instance.ID, "handle")
	if err != nil || len(unused) != 1 {
		t.Fatalf("handle 节点应保留 1 条未消耗预存，实际 %d: %v", len(unused), err)
	}
	// document 状态流转：未提交 → 流转中，待办聚合含新任务
	document, found, _ := fx.documents.GetByInstance(ctxB, instance.ID)
	if !found {
		t.Fatalf("表单状态应存在")
	}
	if document.FlowStatus != FlowStatusRunning {
		t.Fatalf("期望流转中，实际 %s", document.FlowStatus)
	}
	if document.FlowTaskID != handle.ID || document.FlowActionID != "handle" {
		t.Fatalf("待办聚合错误: %s / %s", document.FlowTaskID, document.FlowActionID)
	}
	if document.FlowCurrentPersons != "张三" {
		t.Fatalf("当前待办人聚合错误: %s", document.FlowCurrentPersons)
	}

	// 第二次办结：handle → end，实例结束
	fx.fakeClock.Advance(3600)
	result2, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: handle.ID, UserCode: "alice", Message: "办理完毕"},
		Request: &api.NextActivityRequest{TaskID: handle.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: StepCodeEnd},
		}},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	if !result2.Finished {
		t.Fatalf("流程应已结束")
	}
	if len(result2.NextTaskIDs) != 0 {
		t.Fatalf("结束后不应有新待办: %v", result2.NextTaskIDs)
	}
	if _, found, _ := fx.messages.GetByTaskID(ctxB, handle.ID); found {
		t.Fatalf("办结任务待办应已删除")
	}
	// document 终态：已完成 + 完成时间 + 聚合清空
	document, _, _ = fx.documents.GetByInstance(ctxB, instance.ID)
	if document.FlowStatus != FlowStatusFinished {
		t.Fatalf("期望已完成，实际 %s", document.FlowStatus)
	}
	if document.FlowFinishTime == nil || document.FlowTaskID != "" {
		t.Fatalf("终态聚合错误: %+v", document)
	}
}

// TestComplete_ValidationErrors 验证办结入参校验链（对位 Java generalComplete isBlank 校验）。
// @param t 测试上下文
func TestComplete_ValidationErrors(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-v", "alice")
	fx.seedFirstTodo(t, instance, apply)

	cases := []struct {
		name string // 用例名
		vo   *api.CompleteTaskVo
		want string // 期望错误消息
	}{
		{"请求为空", nil, "办结请求为空"},
		{"任务为空", &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{UserCode: "alice", Message: "x"}}, "参数 taskId 为空"},
		{"操作人为空", &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, Message: "x"}}, "参数 userCode 为空"},
		{"意见为空", &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice"}}, "参数 message 为空"},
		{"任务不存在", &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: "ghost", UserCode: "alice", Message: "x"}}, "任务不存在"},
	}
	for _, tc := range cases {
		if _, err := fx.complete.Complete(ctxB, tc.vo); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: 期望错误含 %q，实际 %v", tc.name, tc.want, err)
		}
	}
}

// TestComplete_SuspendedRejected 验证挂起实例拒绝办结（Java "流程已暂停"）。
// @param t 测试上下文
func TestComplete_SuspendedRejected(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-s", "alice")
	fx.seedFirstTodo(t, instance, apply)
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctxB, instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	_, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "流程已暂停") {
		t.Fatalf("期望流程已暂停错误，实际 %v", err)
	}
}

// TestComplete_FreeSelectFallback 验证自由选路兜底：办结后实际节点与目标不一致时
// 经引擎迁移到目标节点并落直送记录。
// @param t 测试上下文
func TestComplete_FreeSelectFallback(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowFreeSelect", "biz-f", "alice")
	fx.seedFirstTodo(t, instance, apply)

	// 不注入 pass 变量：引擎走默认出线到 review，而目标为 approve → 兜底迁移
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:    api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "直送批准", ProcessInstanceID: instance.ID},
		TargetActivityID: "approve",
		Request: &api.NextActivityRequest{TaskID: apply.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: "approve", ExecutorIDs: []string{"alice"}},
		}},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	if result.Finished {
		t.Fatalf("流程不应已结束")
	}
	// 迁移后任务落位目标节点，误入的 review 分支被清掉
	current := fx.firstTaskOf(t, instance.ID)
	if current.TaskDefinitionKey != "approve" {
		t.Fatalf("期望迁移到 approve，实际 %s", current.TaskDefinitionKey)
	}
	// 直送记录落库
	records, err := fx.completeTo.ListByTask(ctxB, apply.ID)
	if err != nil || len(records) != 1 {
		t.Fatalf("期望 1 条直送记录，实际 %d: %v", len(records), err)
	}
	if records[0].SourceActivityID != "apply" || records[0].TargetActivityID != "approve" || records[0].ProcessInstanceID != instance.ID {
		t.Fatalf("直送记录字段错误: %+v", records[0])
	}
	// operationType 门控变量（直送语义）
	value, err := fx.engine.RuntimeService().GetVariable(ctxB, instance.ID, VarOperationType)
	if err != nil || value != OperationTypeCompleteToTarget {
		t.Fatalf("期望 operationType=completeToTarget，实际 %v（%v）", value, err)
	}
	// 目标节点待办生成
	if _, found, _ := fx.messages.GetByTaskID(ctxB, current.ID); !found {
		t.Fatalf("目标节点待办应已生成")
	}
}

// TestComplete_WeightCountersign 验证权重会签累计与错误分支（countersignByWeightParameterSet 对位）。
// @param t 测试上下文
func TestComplete_WeightCountersign(t *testing.T) {
	fx := newCompleteFixture(t)
	// alice 通过：累计 +2.5
	instance, wtask := fx.start(t, "flowWeight", "biz-w1", "alice")
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:        api.BaseProcessVo{TaskID: wtask.ID, UserCode: "alice", Message: "通过"},
		CountersignResult:    1,
		CountersignResultSet: true,
	}); err != nil {
		t.Fatalf("权重办结失败: %v", err)
	}
	if value, err := fx.engine.RuntimeService().GetVariable(ctxB, instance.ID, VarTotalWeight); err != nil || value != 2.5 {
		t.Fatalf("期望 totalWeight=2.5，实际 %v（%v）", value, err)
	}
	// alice 否决：累计 -1
	instance2, wtask2 := fx.start(t, "flowWeight", "biz-w2", "alice")
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:        api.BaseProcessVo{TaskID: wtask2.ID, UserCode: "alice", Message: "否决"},
		CountersignResult:    0,
		CountersignResultSet: true,
	}); err != nil {
		t.Fatalf("权重办结失败: %v", err)
	}
	if value, _ := fx.engine.RuntimeService().GetVariable(ctxB, instance2.ID, VarTotalWeight); value != -1.0 {
		t.Fatalf("期望 totalWeight=-1，实际 %v", value)
	}
	// bob 通过（仅通过权重）：累计 +1
	_, wtask3 := fx.start(t, "flowWeight", "biz-w3", "bob")
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:        api.BaseProcessVo{TaskID: wtask3.ID, UserCode: "bob", Message: "通过"},
		CountersignResult:    1,
		CountersignResultSet: true,
	}); err != nil {
		t.Fatalf("权重办结失败: %v", err)
	}

	// 错误分支：结论非法 / 个人权重未配置 / 拒绝权重未配置 / 通过权重未配置
	errorCases := []struct {
		initiator   string // 发起人（决定任务处理人）
		resultValue int    // 会签结论
		want        string // 期望错误消息
	}{
		{"alice", 2, "未知会签任务执行结果"},
		{"dave", 1, "个人会签权重未设置"},
		{"bob", 0, "会签拒绝权重未设置"},
		{"carl", 1, "会签通过权重未设置"},
	}
	for _, tc := range errorCases {
		_, broken := fx.start(t, "flowWeight", "biz-err-"+tc.initiator, tc.initiator)
		_, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
			BaseProcessVo:        api.BaseProcessVo{TaskID: broken.ID, UserCode: tc.initiator, Message: "x"},
			CountersignResult:    tc.resultValue,
			CountersignResultSet: true,
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s/%d: 期望错误含 %q，实际 %v", tc.initiator, tc.resultValue, tc.want, err)
		}
		// 校验失败发生在引擎办结前，任务仍在办
		if _, err := fx.engine.TaskService().GetTask(ctxB, broken.ID); err != nil {
			t.Fatalf("校验失败时任务不应被办结: %v", err)
		}
	}
}

// TestComplete_NonWeightNodeSkipsCountersign 验证非权重节点不触发累计（weightType 缺省跳过）。
// @param t 测试上下文
func TestComplete_NonWeightNodeSkipsCountersign(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-nw", "alice")
	fx.seedFirstTodo(t, instance, apply)
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo:        api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "普通办结"},
		CountersignResult:    1,
		CountersignResultSet: true,
	}); err != nil {
		t.Fatalf("普通办结失败: %v", err)
	}
	// 非权重节点不写 totalWeight
	if _, err := fx.engine.RuntimeService().GetVariable(ctxB, instance.ID, VarTotalWeight); err == nil {
		t.Fatalf("非权重节点不应写入 totalWeight")
	}
}

// TestComplete_DelegationPendingBranch 验证委派 PENDING 分支：克隆子任务+归还+还原下一步。
// @param t 测试上下文
func TestComplete_DelegationPendingBranch(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-d", "alice")
	fx.seedFirstTodo(t, instance, apply)
	// 委派给 bob（引擎 Delegate：owner=alice、assignee=bob、PENDING）
	if err := fx.engine.TaskService().Delegate(ctxB, apply.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	// 播种委派侧预存记录（对位 Java saveDelegateNextActivity：identity=被委托人、taskId=当前任务）
	if err := fx.next.InsertDual(ctxB, &NextActivity{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		NextActivityID:    "apply",
		IdentityID:        "bob",
		SendIdentityID:    "bob",
		ProcessInstanceID: instance.ID,
		Used:              1,
		TaskID:            apply.ID,
		Version:           1,
	}, "bob"); err != nil {
		t.Fatalf("播种委派预存失败: %v", err)
	}

	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "bob", Message: "被委派已办理", ProcessInstanceID: instance.ID},
		Request: &api.NextActivityRequest{TaskID: apply.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: "handle", ExecutorIDs: []string{"alice"}},
		}},
	})
	if err != nil {
		t.Fatalf("委派办结失败: %v", err)
	}
	if result.Finished {
		t.Fatalf("流程不应已结束")
	}
	// 原任务归还：owner 还原 + RESOLVED，仍挂在流程上
	current, err := fx.engine.TaskService().GetTask(ctxB, apply.ID)
	if err != nil {
		t.Fatalf("原任务应仍存在: %v", err)
	}
	if current.DelegationState != "RESOLVED" || current.Assignee != "alice" || current.Owner != "alice" {
		t.Fatalf("原任务应归还给 owner: %+v", current)
	}
	// 克隆子任务已完成（历史可查，parentTaskId=原任务、办理人=bob；独立任务无实例归属，需全量检索）
	histories, err := fx.engine.HistoryService().QueryHistoricTaskInstances(ctxB, api.HistoricTaskInstanceFilter{Size: 100})
	if err != nil {
		t.Fatalf("查询历史任务失败: %v", err)
	}
	subFound := false
	for _, history := range histories.Data {
		if history.ParentTaskID == apply.ID {
			subFound = true
			if history.Assignee != "bob" || history.EndTime == nil {
				t.Fatalf("子任务历史错误: %+v", history)
			}
		}
	}
	if !subFound {
		t.Fatalf("应存在克隆子任务的历史记录")
	}
	// 预存记录：bob 委派记录已删；保留首步记录与还原给 owner 的新记录（均为 alice）
	ledger, err := fx.next.ListByTaskID(ctxB, apply.ID)
	if err != nil || len(ledger) != 2 {
		t.Fatalf("应保留 2 条 alice 预存记录，实际 %d（%v）", len(ledger), err)
	}
	restoredFound := false
	for _, record := range ledger {
		if record.IdentityID != "alice" {
			t.Fatalf("不应保留非 owner 记录: %+v", record)
		}
		// 还原记录的发送人 = 被委托操作人（SaveResolveNext 语义）
		if record.SendIdentityID == "bob" {
			restoredFound = true
		}
	}
	if !restoredFound {
		t.Fatalf("应存在还原给 owner 的新记录（发送人=bob）")
	}
	// 操作记录挂在办结人名下（SP；同刻时钟下顺序不稳定，按类型检索）
	records, _ := fx.operations.ListByInstance(ctxB, instance.ID)
	if len(records) != 2 || !containsRecordType(records, CommentTypeSP) {
		t.Fatalf("委派办结操作记录错误: %+v", records)
	}
	for _, record := range records {
		if record.Type == CommentTypeSP && record.OwnerID != "bob" {
			t.Fatalf("委派办结操作记录处理人错误: %+v", record)
		}
	}
	// 原任务待办保留给 owner（未被误删）
	if _, found, _ := fx.messages.GetByTaskID(ctxB, apply.ID); !found {
		t.Fatalf("原任务待办应保留给 owner")
	}
}

// TestComplete_DelegationWithoutSourceRecord 验证委派分支无既有预存记录时跳过还原（Java 语义）。
// @param t 测试上下文
func TestComplete_DelegationWithoutSourceRecord(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-d2", "alice")
	fx.seedFirstTodo(t, instance, apply)
	if err := fx.engine.TaskService().Delegate(ctxB, apply.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "bob", Message: "办理"},
	}); err != nil {
		t.Fatalf("委派办结失败: %v", err)
	}
	// 无既有记录：不落还原记录（发送人=bob 的记录不存在），也不报错
	ledger, _ := fx.next.ListByTaskID(ctxB, apply.ID)
	for _, record := range ledger {
		if record.SendIdentityID == "bob" {
			t.Fatalf("不应产生还原记录: %+v", record)
		}
	}
}

// TestComplete_DelegateTransferResolve 验证委派/转办/归还三方法及收尾模板。
// @param t 测试上下文
func TestComplete_DelegateTransferResolve(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-dt", "alice")
	fx.seedFirstTodo(t, instance, apply)

	// 转办：alice → carl（ZB，先执行避免委派状态耦合）
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Owner: "alice", Receiver: "carl", Message: "转给王五"}); err != nil {
		t.Fatalf("转办失败: %v", err)
	}
	transferred, _ := fx.engine.TaskService().GetTask(ctxB, apply.ID)
	if transferred.Owner != "alice" || transferred.Assignee != "carl" || transferred.DelegationState != "" {
		t.Fatalf("转办后任务状态错误: %+v", transferred)
	}
	todo, found, _ := fx.messages.GetByTaskID(ctxB, apply.ID)
	if !found || todo.OwnerID != "carl" || todo.OwnerName != "王五" || todo.ConsignerID != "" {
		t.Fatalf("转办待办应转给接收人: %+v", todo)
	}
	transferRecords, _ := fx.stores.Operation.ListByTask(ctxB, apply.ID)
	if len(transferRecords) != 2 || !containsRecordType(transferRecords, CommentTypeZB) {
		t.Fatalf("转办操作记录缺失: %+v", transferRecords)
	}

	// 委派：owner=alice → bob（WP）
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob", Message: "请你处理"}); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	delegated, _ := fx.engine.TaskService().GetTask(ctxB, apply.ID)
	if delegated.Owner != "alice" || delegated.Assignee != "bob" || delegated.DelegationState != "PENDING" {
		t.Fatalf("委派后任务状态错误: %+v", delegated)
	}
	todo, _, _ = fx.messages.GetByTaskID(ctxB, apply.ID)
	if todo.OwnerID != "bob" || todo.ConsignerID != "bob" || todo.Consigner != "李四" {
		t.Fatalf("委派待办应转给受托人: %+v", todo)
	}
	comments, _ := fx.engine.TaskService().ListComments(ctxB, apply.ID)
	// 同刻钟时钟下意见顺序不稳定，按内容检索（发起 TJ + 转办 ZB + 委派 WP）
	if len(comments) != 3 || !containsMessage(comments, "请你处理") {
		t.Fatalf("委派意见缺失: %+v", comments)
	}
	wpRecords, _ := fx.stores.Operation.ListByTask(ctxB, apply.ID)
	if len(wpRecords) != 3 || !containsRecordType(wpRecords, CommentTypeWP) {
		t.Fatalf("委派操作记录缺失: %+v", wpRecords)
	}

	// 归还：bob 办理后归还给 owner（SP）
	if err := fx.complete.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: apply.ID, UserCode: "bob", Message: "已处理归还"}); err != nil {
		t.Fatalf("归还失败: %v", err)
	}
	resolved, _ := fx.engine.TaskService().GetTask(ctxB, apply.ID)
	if resolved.DelegationState != "RESOLVED" || resolved.Assignee != "alice" {
		t.Fatalf("归还后任务状态错误: %+v", resolved)
	}
	todo, _, _ = fx.messages.GetByTaskID(ctxB, apply.ID)
	if todo.OwnerID != "alice" || todo.ConsignerID != "" {
		t.Fatalf("归还待办应还给 owner: %+v", todo)
	}
	resolveRecords, _ := fx.stores.Operation.ListByTask(ctxB, apply.ID)
	if len(resolveRecords) != 4 || !containsRecordType(resolveRecords, CommentTypeSP) {
		t.Fatalf("归还操作记录缺失: %+v", resolveRecords)
	}
	// 任务仍在办（三个操作均不推进流程）
	if _, err := fx.engine.TaskService().GetTask(ctxB, apply.ID); err != nil {
		t.Fatalf("操作后任务应仍存在: %v", err)
	}
	_ = instance
}

// containsMessage 判断评论列表中是否包含指定内容的评论。
// @param comments 评论列表
// @param fragment 内容片段
// @return 是否包含
func containsMessage(comments []api.Comment, fragment string) bool {
	for _, comment := range comments {
		if strings.Contains(comment.Message, fragment) {
			return true
		}
	}
	return false
}

// containsRecordType 判断操作记录列表中是否包含指定类型的记录。
// @param records 操作记录列表
// @param recordType 类型代码
// @return 是否包含
func containsRecordType(records []*OperationRecord, recordType string) bool {
	for _, record := range records {
		if record.Type == recordType {
			return true
		}
	}
	return false
}

// TestComplete_AssignValidationErrors 验证委派/转办/归还的入参与状态校验。
// @param t 测试上下文
func TestComplete_AssignValidationErrors(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-av", "alice")
	fx.seedFirstTodo(t, instance, apply)

	// 空请求
	if err := fx.complete.DelegateTask(ctxB, nil); err == nil || !strings.Contains(err.Error(), "委派请求为空") {
		t.Fatalf("期望委派请求为空错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, nil); err == nil || !strings.Contains(err.Error(), "转办请求为空") {
		t.Fatalf("期望转办请求为空错误，实际 %v", err)
	}
	// 必填缺失
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{Owner: "alice", DelegateID: "bob", Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 taskId 为空") {
		t.Fatalf("期望 taskId 校验错误，实际 %v", err)
	}
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, DelegateID: "bob", Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 owner 为空") {
		t.Fatalf("期望 owner 校验错误，实际 %v", err)
	}
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 delegateId 为空") {
		t.Fatalf("期望 delegateId 校验错误，实际 %v", err)
	}
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob"}); err == nil || !strings.Contains(err.Error(), "参数 message 为空") {
		t.Fatalf("期望 message 校验错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Owner: "alice", Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 receiver 为空") {
		t.Fatalf("期望 receiver 校验错误，实际 %v", err)
	}
	if err := fx.complete.ResolveTask(ctxB, &ResolveTaskRequest{Message: "m"}); err == nil || !strings.Contains(err.Error(), "参数 taskId 为空") {
		t.Fatalf("期望 taskId 校验错误，实际 %v", err)
	}
	if err := fx.complete.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: apply.ID}); err == nil || !strings.Contains(err.Error(), "参数 message 为空") {
		t.Fatalf("期望 message 校验错误，实际 %v", err)
	}
	// 任务不存在
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: "ghost", Owner: "a", DelegateID: "b", Message: "m"}); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("期望任务不存在错误，实际 %v", err)
	}
	// 自委派/自转办
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "alice", Message: "m"}); err == nil || !strings.Contains(err.Error(), "无需委派") {
		t.Fatalf("期望自委派错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Owner: "alice", Receiver: "alice", Message: "m"}); err == nil || !strings.Contains(err.Error(), "无需转办") {
		t.Fatalf("期望自转办错误，实际 %v", err)
	}
	// 挂起实例
	if err := fx.engine.RuntimeService().SuspendProcessInstance(ctxB, instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if err := fx.complete.DelegateTask(ctxB, &DelegateTaskRequest{TaskID: apply.ID, Owner: "alice", DelegateID: "bob", Message: "m"}); err == nil || !strings.Contains(err.Error(), "流程已暂停") {
		t.Fatalf("期望挂起错误，实际 %v", err)
	}
	if err := fx.complete.TransferTask(ctxB, &TransferTaskRequest{TaskID: apply.ID, Owner: "alice", Receiver: "carl", Message: "m"}); err == nil || !strings.Contains(err.Error(), "流程已暂停") {
		t.Fatalf("期望挂起错误，实际 %v", err)
	}
	if err := fx.complete.ResolveTask(ctxB, &ResolveTaskRequest{TaskID: apply.ID, Message: "m"}); err == nil || !strings.Contains(err.Error(), "流程已暂停") {
		t.Fatalf("期望挂起错误，实际 %v", err)
	}
}

// TestComplete_ParentTaskLinkage 验证加签父任务联动：全部子任务完成后归还父任务，
// 后加签（scopeType=after）随之办结父任务。
// @param t 测试上下文
func TestComplete_ParentTaskLinkage(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-p", "alice")
	fx.seedFirstTodo(t, instance, apply)
	// 推进到 handle 节点作为父任务
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "同意"},
		Request: &api.NextActivityRequest{TaskID: apply.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: "handle", ExecutorIDs: []string{"alice"}},
		}},
	}); err != nil {
		t.Fatalf("首次办结失败: %v", err)
	}
	parent := fx.firstTaskOf(t, instance.ID)

	// 场景一：仍有其他子任务未完成 → 父任务保持等待
	if err := fx.engine.TaskService().Delegate(ctxB, parent.ID, "helper"); err != nil {
		t.Fatalf("父任务委派失败: %v", err)
	}
	subA, err := fx.engine.TaskService().CreateTask(ctxB, api.CreateTaskRequest{Name: "加签A", Assignee: "helper", ParentTaskID: parent.ID, TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	if _, err := fx.engine.TaskService().CreateTask(ctxB, api.CreateTaskRequest{Name: "加签B", Assignee: "helper", ParentTaskID: parent.ID, TenantID: "dev"}); err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: subA.ID, UserCode: "helper", Message: "加签A办理"},
	}); err != nil {
		t.Fatalf("加签子任务办结失败: %v", err)
	}
	waiting, _ := fx.engine.TaskService().GetTask(ctxB, parent.ID)
	if waiting.DelegationState != "PENDING" {
		t.Fatalf("仍有子任务时父任务应保持 PENDING: %+v", waiting)
	}

	// 场景二：最后一个子任务完成（scopeType 未设置）→ 父任务归还但不办结
	subB := fx.firstSubTaskOf(t, parent.ID)
	if _, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: subB.ID, UserCode: "helper", Message: "加签B办理"},
	}); err != nil {
		t.Fatalf("加签子任务办结失败: %v", err)
	}
	resolved, err := fx.engine.TaskService().GetTask(ctxB, parent.ID)
	if err != nil {
		t.Fatalf("父任务应仍存在: %v", err)
	}
	if resolved.DelegationState != "RESOLVED" || resolved.Assignee != "alice" {
		t.Fatalf("父任务应归还给 owner: %+v", resolved)
	}

	// 场景三：后加签（scopeType=after）→ 父任务归还后随之办结，流程推进到 end
	if err := fx.engine.TaskService().Delegate(ctxB, parent.ID, "helper"); err != nil {
		t.Fatalf("父任务再委派失败: %v", err)
	}
	if err := fx.engine.TaskService().SetTaskVariable(ctxB, parent.ID, varScopeType, AddSignScopeAfter); err != nil {
		t.Fatalf("设置后加签标记失败: %v", err)
	}
	subC, err := fx.engine.TaskService().CreateTask(ctxB, api.CreateTaskRequest{Name: "后加签", Assignee: "helper", ParentTaskID: parent.ID, TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: subC.ID, UserCode: "helper", Message: "后加签办理"},
	})
	if err != nil {
		t.Fatalf("后加签子任务办结失败: %v", err)
	}
	// 父任务办结后流程结束（handle → end）
	if !result.Finished {
		t.Fatalf("后加签父任务办结后流程应结束")
	}
}

// firstSubTaskOf 返回父任务当前首个子任务（断言辅助）。
// @param t 测试上下文
// @param parentID 父任务 Id
// @return 子任务
func (f *completeFixture) firstSubTaskOf(t *testing.T, parentID string) api.Task {
	t.Helper()
	subs, err := f.engine.TaskService().ListSubTasks(ctxB, parentID)
	if err != nil || len(subs) == 0 {
		t.Fatalf("查询子任务失败: %v", err)
	}
	return subs[0]
}

// TestComplete_ParentTaskVanished 验证父任务已不存在时联动静默跳过。
// @param t 测试上下文
func TestComplete_ParentTaskVanished(t *testing.T) {
	fx := newCompleteFixture(t)
	// 独立任务承载后删除（引擎创建子任务要求父任务存在）
	holder, err := fx.engine.TaskService().CreateTask(ctxB, api.CreateTaskRequest{Name: "占位", Assignee: "helper", TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建占位任务失败: %v", err)
	}
	sub, err := fx.engine.TaskService().CreateTask(ctxB, api.CreateTaskRequest{Name: "孤儿加签", Assignee: "helper", ParentTaskID: holder.ID, TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	// 引擎删除独立任务会级联删除子任务，此处改为直接办结占位任务使其消失
	if err := fx.engine.TaskService().Complete(ctxB, holder.ID, api.CompleteTaskRequest{UserID: "helper"}); err != nil {
		t.Fatalf("办结占位任务失败: %v", err)
	}
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: sub.ID, UserCode: "helper", Message: "x"},
	})
	if err != nil {
		t.Fatalf("孤儿加签办结不应报错: %v", err)
	}
	// 无法定位实例：不虚报终态
	if result.Finished {
		t.Fatalf("无实例孤儿任务不应报已结束")
	}
}

// TestComplete_GroupNodeTodoSkipped 验证无处理人节点暂不生成待办（执行人未落定）。
// @param t 测试上下文
func TestComplete_GroupNodeTodoSkipped(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, gtask := fx.start(t, "flowGroup", "biz-g", "alice")
	fx.seedFirstTodo(t, instance, gtask)
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: gtask.ID, UserCode: "alice", Message: "提交"},
		Request: &api.NextActivityRequest{TaskID: gtask.ID, NextActivityList: []*api.NextActivityVo{
			{ActivityID: "gnext", ExecutorIDs: []string{"u1"}},
		}},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// 组任务无 assignee：待办暂不生成
	if len(result.NextTaskIDs) != 0 {
		t.Fatalf("无处理人节点不应生成待办: %v", result.NextTaskIDs)
	}
	next := fx.firstTaskOf(t, instance.ID)
	if _, found, _ := fx.messages.GetByTaskID(ctxB, next.ID); found {
		t.Fatalf("组任务待办不应生成")
	}
}

// TestComplete_EndDirectionSkipsTodos 验证 request 缺省（end 方向）跳过新待办生成。
// @param t 测试上下文
func TestComplete_EndDirectionSkipsTodos(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, apply := fx.start(t, "flowComplete", "biz-e", "alice")
	fx.seedFirstTodo(t, instance, apply)
	result, err := fx.complete.Complete(ctxB, &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: apply.ID, UserCode: "alice", Message: "一次性办结"},
	})
	if err != nil {
		t.Fatalf("办结失败: %v", err)
	}
	// request 缺省按 end 方向：不生成新待办（本流程办结后仍有 handle 任务，但门控按协议跳过）
	if result.Finished {
		t.Fatalf("实例应仍在运行（handle 未办）")
	}
	if len(result.NextTaskIDs) != 0 {
		t.Fatalf("end 方向不应生成新待办: %v", result.NextTaskIDs)
	}
	// document 状态机仍刷新（end 方向跳过待办生成但状态按操作记录数推进为流转中）
	document, found, _ := fx.documents.GetByInstance(ctxB, instance.ID)
	if !found || document.FlowStatus != FlowStatusRunning {
		t.Fatalf("document 应为流转中: %+v", document)
	}
}
