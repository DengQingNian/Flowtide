package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// addsignFixture 是加签测试环境（复用办结测试环境：引擎/五大域服务/办结服务齐备）。
type addsignFixture struct {
	*completeFixture                 // 复用 complete 环境的引擎与服务
	addSign          *AddSignService // 加签/减签服务
}

// newAddsignFixture 构造加签测试环境。
// @param t 测试上下文
// @return 测试环境
func newAddsignFixture(t *testing.T) *addsignFixture {
	t.Helper()
	base := newCompleteFixture(t)
	fx := &addsignFixture{completeFixture: base}
	fx.addSign = NewAddSignService(fx.engine, fx.comments, fx.operations, fx.messages, fx.histories, fx.fakeClock)
	fx.addSign.WithAddSignUsers(fx.engine.IdentityService())
	return fx
}

// startApply 发起 flowComplete 流程并返回实例与首任务（apply 节点）。
// @param t 测试上下文
// @param businessKey 业务键
// @return 实例
// @return 首任务
func (f *addsignFixture) startApply(t *testing.T, businessKey string) (api.ProcessInstance, api.Task) {
	t.Helper()
	return f.start(t, "flowComplete", businessKey, "alice")
}

// subTasksOf 查询父任务的未完成子任务（断言辅助）。
// @param t 测试上下文
// @param parentTaskID 父任务 Id
// @return 子任务列表
func (f *addsignFixture) subTasksOf(t *testing.T, parentTaskID string) []api.Task {
	t.Helper()
	subs, err := f.engine.TaskService().ListSubTasks(context.Background(), parentTaskID)
	if err != nil {
		t.Fatalf("查询子任务失败: %v", err)
	}
	return subs
}

// TestAddSign_After_ParentStructure 验证后加签父子结构与占位子任务（ListSubTasks）。
// @param t 测试上下文
func TestAddSign_After_ParentStructure(t *testing.T) {
	fx := newAddsignFixture(t)
	instance, apply := fx.startApply(t, "biz-as1")
	// 播种父任务待办（模拟发起侧产物，供 consigner 标记断言）
	if _, err := fx.messages.CreateForTask(context.Background(), &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: "dev"},
		TaskID:            apply.ID,
		ProcessInstanceID: instance.ID,
		Title:             "申请",
		OwnerID:           "alice",
	}); err != nil {
		t.Fatalf("播种待办失败: %v", err)
	}
	// 后加签 bob
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeAfter, Message: "请协助处理",
	})
	if err != nil {
		t.Fatalf("后加签失败: %v", err)
	}
	// 父任务 = 原任务：owner=alice、PENDING 委派态（引擎 Resolve 前置）、countEnabled/scopeType 变量
	parent, err := fx.engine.TaskService().GetTask(context.Background(), apply.ID)
	if err != nil {
		t.Fatalf("父任务应存在: %v", err)
	}
	if parent.Owner != "alice" || parent.DelegationState != "PENDING" {
		t.Fatalf("父任务应为 owner=alice + PENDING: %+v", parent)
	}
	if enabled, ok := parent.Variables[varCountEnabled].(bool); !ok || !enabled {
		t.Fatalf("countEnabled 应为 true: %v", parent.Variables[varCountEnabled])
	}
	if scope, ok := parent.Variables[varScopeType].(string); !ok || scope != AddSignScopeAfter {
		t.Fatalf("scopeType 应为 after: %v", parent.Variables[varScopeType])
	}
	// 占位子任务已立即办结：剩余子任务仅 bob 一个
	subs := fx.subTasksOf(t, parent.ID)
	if len(subs) != 1 || subs[0].ID != result.SubTaskIDs[0] || subs[0].Assignee != "bob" {
		t.Fatalf("剩余子任务应仅 bob: %+v", subs)
	}
	if result.CommentType != CommentTypeHJQ {
		t.Fatalf("后加签意见类型应为 HJQ: %s", result.CommentType)
	}
	// 子任务挂靠父任务（独立任务无实例归属，实例信息在办结链路自父任务补齐）
	if subs[0].ParentTaskID != parent.ID {
		t.Fatalf("子任务应挂靠父任务: %+v", subs[0])
	}
	_ = instance
	// 新子任务待办生成（owner=bob、consigner=alice）
	message, found, err := fx.messages.GetByTaskID(context.Background(), subs[0].ID)
	if err != nil || !found {
		t.Fatalf("子任务待办应生成: %v %v", found, err)
	}
	if message.OwnerID != "bob" || message.ConsignerID != "alice" {
		t.Fatalf("待办应 owner=bob/consigner=alice: %+v", message)
	}
	// 原待办标记 consigner（保留给 owner）
	parentTodo, found, err := fx.messages.GetByTaskID(context.Background(), parent.ID)
	if err != nil || !found {
		t.Fatalf("父任务待办应保留: %v %v", found, err)
	}
	if parentTodo.ConsignerID != "alice" || parentTodo.OwnerID != "alice" {
		t.Fatalf("父待办应标记 consigner: %+v", parentTodo)
	}
	// HJQ 意见类型权威台账在操作记录（引擎评论 API 不透出自定义类型）
	records, err := fx.operations.ListByInstance(context.Background(), apply.ProcessInstanceID)
	if err != nil || len(records) == 0 {
		t.Fatalf("操作记录应留痕: %v %v", records, err)
	}
	foundHJQ := false
	for _, record := range records {
		if record.Type == CommentTypeHJQ {
			foundHJQ = true
		}
	}
	if !foundHJQ {
		t.Fatalf("应存在 HJQ 操作记录: %+v", records)
	}
}

// TestAddSign_After_CompletionLinkage 验证后加签办结联动（complete.go 路径：子任务完成 → 父任务 resolve+办结 → 流程推进）。
// @param t 测试上下文
func TestAddSign_After_CompletionLinkage(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-as2")
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeAfter, Message: "后加签",
	})
	if err != nil {
		t.Fatalf("后加签失败: %v", err)
	}
	// 被加签人经 workflow 办结链完成子任务 → 父任务 resolve + 后加签办结 → 推进到 handle
	completed, err := fx.complete.Complete(context.Background(), &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: result.SubTaskIDs[0], UserCode: "bob", Message: "加签办理完成"},
	})
	if err != nil {
		t.Fatalf("子任务办结失败: %v", err)
	}
	if completed.Finished {
		t.Fatalf("后加签联动应推进到 handle 而非结束流程")
	}
	// 流程应推进到 handle（父任务已办结消失）
	page, err := fx.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: apply.ProcessInstanceID, Size: 10})
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "handle" {
		t.Fatalf("流程应推进到 handle: %+v", page.Data)
	}
}

// TestAddSign_Before_CompletionLinkage 验证前加签办结联动（子任务完成 → 父任务归还等原办理人）。
// @param t 测试上下文
func TestAddSign_Before_CompletionLinkage(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-as3")
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeBefore, Message: "前加签",
	})
	if err != nil {
		t.Fatalf("前加签失败: %v", err)
	}
	if result.CommentType != CommentTypeQJQ {
		t.Fatalf("前加签意见类型应为 QJQ: %s", result.CommentType)
	}
	// 子任务办结 → 父任务仅归还（不办结）
	if _, err := fx.complete.Complete(context.Background(), &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: result.SubTaskIDs[0], UserCode: "bob", Message: "前加签办理完成"},
	}); err != nil {
		t.Fatalf("子任务办结失败: %v", err)
	}
	parent, err := fx.engine.TaskService().GetTask(context.Background(), apply.ID)
	if err != nil {
		t.Fatalf("父任务应保留: %v", err)
	}
	// 归还：assignee=owner=alice、RESOLVED
	if parent.Assignee != "alice" || parent.DelegationState != "RESOLVED" {
		t.Fatalf("父任务应归还给 alice: %+v", parent)
	}
	// 流程未推进（仍在 apply 节点等待原办理人）
	if parent.TaskDefinitionKey != "apply" {
		t.Fatalf("前加签父任务应停留在原节点: %+v", parent)
	}
	// QJQ 意见类型权威台账在操作记录
	records, err := fx.operations.ListByInstance(context.Background(), apply.ProcessInstanceID)
	if err != nil {
		t.Fatalf("查询操作记录失败: %v", err)
	}
	foundQJQ := false
	for _, record := range records {
		if record.Type == CommentTypeQJQ {
			foundQJQ = true
		}
	}
	if !foundQJQ {
		t.Fatalf("应存在 QJQ 操作记录: %+v", records)
	}
}

// TestAddSign_SecondLevel 验证加签再加签（二层：新子任务挂顶层父任务、当前子任务立即办结）。
// @param t 测试上下文
func TestAddSign_SecondLevel(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-as4")
	first, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeBefore, Message: "首层加签",
	})
	if err != nil {
		t.Fatalf("首层加签失败: %v", err)
	}
	// bob 在自己的子任务上再加签 carol
	second, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: first.SubTaskIDs[0], Operator: "bob", Assignees: []string{"carol"}, ScopeType: AddSignScopeBefore, Message: "二层加签",
	})
	if err != nil {
		t.Fatalf("二层加签失败: %v", err)
	}
	// 新子任务挂顶层父任务（与 bob 子任务平级）
	if second.ParentTaskID != first.ParentTaskID {
		t.Fatalf("二层子任务应挂顶层父任务: %s", second.ParentTaskID)
	}
	// bob 的子任务已被立即办结：顶层剩余子任务仅 carol
	subs := fx.subTasksOf(t, first.ParentTaskID)
	if len(subs) != 1 || subs[0].Assignee != "carol" || subs[0].ID != second.SubTaskIDs[0] {
		t.Fatalf("二层加签后应仅剩 carol 子任务: %+v", subs)
	}
	// carol 办结 → 父任务归还（前加签）
	if _, err := fx.complete.Complete(context.Background(), &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: second.SubTaskIDs[0], UserCode: "carol", Message: "二层办理完成"},
	}); err != nil {
		t.Fatalf("二层子任务办结失败: %v", err)
	}
	parent, err := fx.engine.TaskService().GetTask(context.Background(), first.ParentTaskID)
	if err != nil {
		t.Fatalf("父任务应保留: %v", err)
	}
	if parent.Assignee != "alice" || parent.DelegationState != "RESOLVED" {
		t.Fatalf("父任务应归还给 alice: %+v", parent)
	}
}

// TestAddSign_CandidateCleanup 验证加签人是父任务候选时清理候选身份。
// @param t 测试上下文
func TestAddSign_CandidateCleanup(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-as5")
	// 将操作人加为候选用户
	if err := fx.engine.TaskService().AddCandidateUser(context.Background(), apply.ID, "alice"); err != nil {
		t.Fatalf("添加候选失败: %v", err)
	}
	links, err := fx.engine.TaskService().ListIdentityLinks(context.Background(), apply.ID)
	if err != nil || len(links) == 0 {
		t.Fatalf("候选身份应存在: %v %v", links, err)
	}
	// 加签后候选身份被清理
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeAfter, Message: "加签",
	}); err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	links, err = fx.engine.TaskService().ListIdentityLinks(context.Background(), apply.ID)
	if err != nil {
		t.Fatalf("查询身份链接失败: %v", err)
	}
	for _, link := range links {
		if link.Type == "candidate" && link.UserID == "alice" {
			t.Fatalf("加签人候选身份应被清理: %+v", links)
		}
	}
}

// TestAddSign_DelSign 验证减签：删除子任务、恢复父任务、待办清理。
// @param t 测试上下文
func TestAddSign_DelSign(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-ds1")
	// 加签两人（后加签）
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob", "carol"}, ScopeType: AddSignScopeAfter, Message: "两人加签",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	// 减掉 bob（剩余 carol：父任务保持等待）
	deleted, err := fx.addSign.DelSign(context.Background(), DelSignRequest{
		ParentTaskID: result.ParentTaskID, SubTaskID: result.SubTaskIDs[0], Operator: "alice",
	})
	if err != nil {
		t.Fatalf("减签失败: %v", err)
	}
	if deleted.ParentResolved {
		t.Fatalf("仍有剩余子任务不应恢复父任务")
	}
	subs := fx.subTasksOf(t, result.ParentTaskID)
	if len(subs) != 1 || subs[0].Assignee != "carol" {
		t.Fatalf("应仅剩 carol 子任务: %+v", subs)
	}
	// bob 待办/已办已清理
	if _, found, err := fx.messages.GetByTaskID(context.Background(), result.SubTaskIDs[0]); err != nil || found {
		t.Fatalf("减签任务待办应删除: %v %v", found, err)
	}
	// carol 办结 → 后加签父任务办结推进
	if _, err := fx.complete.Complete(context.Background(), &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: result.SubTaskIDs[1], UserCode: "carol", Message: "carol 办理"},
	}); err != nil {
		t.Fatalf("子任务办结失败: %v", err)
	}
	page, err := fx.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: apply.ProcessInstanceID, Size: 10})
	if err != nil || len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "handle" {
		t.Fatalf("后加签联动应推进到 handle: %+v %v", page.Data, err)
	}
}

// TestAddSign_DelSignRestoreParent 验证减掉最后一个子任务时恢复父任务（before 归还 / after 办结）。
// @param t 测试上下文
func TestAddSign_DelSignRestoreParent(t *testing.T) {
	fx := newAddsignFixture(t)
	// before：恢复父任务等待原办理人
	_, apply := fx.startApply(t, "biz-ds2")
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeBefore, Message: "前加签",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	restored, err := fx.addSign.DelSign(context.Background(), DelSignRequest{
		ParentTaskID: result.ParentTaskID, SubTaskID: result.SubTaskIDs[0], Operator: "alice",
	})
	if err != nil {
		t.Fatalf("减签失败: %v", err)
	}
	if !restored.ParentResolved || restored.ParentFinished {
		t.Fatalf("前加签减空应仅恢复父任务: %+v", restored)
	}
	parent, err := fx.engine.TaskService().GetTask(context.Background(), result.ParentTaskID)
	if err != nil || parent.Assignee != "alice" {
		t.Fatalf("父任务应恢复 assignee=alice: %+v %v", parent, err)
	}
	// after：父任务随之办结
	instance, apply2 := fx.startApply(t, "biz-ds3")
	result2, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply2.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeAfter, Message: "后加签",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	finished, err := fx.addSign.DelSign(context.Background(), DelSignRequest{
		ParentTaskID: result2.ParentTaskID, SubTaskID: result2.SubTaskIDs[0], Operator: "alice",
	})
	if err != nil {
		t.Fatalf("减签失败: %v", err)
	}
	if !finished.ParentResolved || !finished.ParentFinished {
		t.Fatalf("后加签减空应办结父任务: %+v", finished)
	}
	page, err := fx.engine.TaskService().ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
	if err != nil || len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "handle" {
		t.Fatalf("后加签减空应推进到 handle: %+v %v", page.Data, err)
	}
}

// TestAddSign_DelSignErrors 验证减签校验（任务不存在/子任务不归属）。
// @param t 测试上下文
func TestAddSign_DelSignErrors(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-ds4")
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeAfter, Message: "加签",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	// 父任务不存在
	if _, err := fx.addSign.DelSign(context.Background(), DelSignRequest{ParentTaskID: "ghost", SubTaskID: result.SubTaskIDs[0], Operator: "alice"}); err == nil || !strings.Contains(err.Error(), "父任务不存在") {
		t.Fatalf("父任务不存在应报错: %v", err)
	}
	// 子任务不存在
	if _, err := fx.addSign.DelSign(context.Background(), DelSignRequest{ParentTaskID: result.ParentTaskID, SubTaskID: "ghost", Operator: "alice"}); err == nil || !strings.Contains(err.Error(), "子任务不存在") {
		t.Fatalf("子任务不存在应报错: %v", err)
	}
	// 子任务不归属父任务
	if _, err := fx.addSign.DelSign(context.Background(), DelSignRequest{ParentTaskID: apply.ID, SubTaskID: apply.ID, Operator: "alice"}); err == nil || !strings.Contains(err.Error(), "不属于该父任务") {
		t.Fatalf("子任务不归属应报错: %v", err)
	}
	// 参数缺失
	if _, err := fx.addSign.DelSign(context.Background(), DelSignRequest{SubTaskID: "x", Operator: "a"}); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("参数缺失应报错: %v", err)
	}
}

// TestAddSign_Validation 验证加签入参校验。
// @param t 测试上下文
func TestAddSign_Validation(t *testing.T) {
	fx := newAddsignFixture(t)
	// taskId 缺失
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{Operator: "a", Assignees: []string{"b"}, ScopeType: AddSignScopeAfter, Message: "m"}); err == nil || !strings.Contains(err.Error(), "taskId 为空") {
		t.Fatalf("taskId 缺失应报错: %v", err)
	}
	// 操作人缺失
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{TaskID: "t", Assignees: []string{"b"}, ScopeType: AddSignScopeAfter, Message: "m"}); err == nil || !strings.Contains(err.Error(), "userCode 为空") {
		t.Fatalf("userCode 缺失应报错: %v", err)
	}
	// 被加签人为空
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{TaskID: "t", Operator: "a", ScopeType: AddSignScopeAfter, Message: "m"}); err == nil || !strings.Contains(err.Error(), "被加签人列表为空") {
		t.Fatalf("被加签人为空应报错: %v", err)
	}
	// 意见缺失
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{TaskID: "t", Operator: "a", Assignees: []string{"b"}, ScopeType: AddSignScopeAfter}); err == nil || !strings.Contains(err.Error(), "message 为空") {
		t.Fatalf("message 缺失应报错: %v", err)
	}
	// 作用域非法
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{TaskID: "t", Operator: "a", Assignees: []string{"b"}, ScopeType: "side", Message: "m"}); err == nil || !strings.Contains(err.Error(), "scopeType 非法") {
		t.Fatalf("scopeType 非法应报错: %v", err)
	}
	// 任务不存在
	if _, err := fx.addSign.AddSign(context.Background(), AddSignRequest{TaskID: "ghost", Operator: "a", Assignees: []string{"b"}, ScopeType: AddSignScopeAfter, Message: "m"}); err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("任务不存在应报错: %v", err)
	}
}

// TestAddSign_MultiInstanceMember 验证多实例动态加减成员的 workflow 侧变量维护。
// @param t 测试上下文
func TestAddSign_MultiInstanceMember(t *testing.T) {
	cs := newCountersignFixture(t)
	// 成员维护方法不触达意见/操作记录/待办服务，最小构造（nil 依赖）
	fx := NewAddSignService(cs.engine, nil, nil, nil, nil, csClock())
	instance, _ := cs.startCountersign(t, "csCollection", "carol", nil)
	// 加成员 carol（executorList 追加 + nrOfInstances 同步）
	if err := fx.AddMultiInstanceMember(context.Background(), instance.ID, "csTask", "carol"); err != nil {
		t.Fatalf("动态加签失败: %v", err)
	}
	list, ok := cs.instanceVariable(t, instance.ID, VarExecutorList).([]string)
	if !ok || len(list) != 3 || list[2] != "carol" {
		t.Fatalf("executorList 应追加 carol: %v", list)
	}
	if got := intOf(cs.instanceVariable(t, instance.ID, VarNrOfInstances)); got != 3 {
		t.Fatalf("nrOfInstances 应同步为 3: %v", got)
	}
	// 重复追加幂等
	if err := fx.AddMultiInstanceMember(context.Background(), instance.ID, "csTask", "carol"); err != nil {
		t.Fatalf("重复加签失败: %v", err)
	}
	list, _ = cs.instanceVariable(t, instance.ID, VarExecutorList).([]string)
	if len(list) != 3 {
		t.Fatalf("重复加签应幂等: %v", list)
	}
	// 减成员 carol
	if err := fx.RemoveMultiInstanceMember(context.Background(), instance.ID, "csTask", "carol", "", false); err != nil {
		t.Fatalf("动态减签失败: %v", err)
	}
	list, _ = cs.instanceVariable(t, instance.ID, VarExecutorList).([]string)
	if len(list) != 2 {
		t.Fatalf("减签后应剩 2 人: %v", list)
	}
	if got := intOf(cs.instanceVariable(t, instance.ID, VarNrOfInstances)); got != 2 {
		t.Fatalf("nrOfInstances 应同步为 2: %v", got)
	}
	// 参数缺失
	if err := fx.AddMultiInstanceMember(context.Background(), "", "csTask", "x"); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("参数缺失应报错: %v", err)
	}
	if err := fx.RemoveMultiInstanceMember(context.Background(), instance.ID, "", "x", "", false); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("参数缺失应报错: %v", err)
	}
}

// fakeMemberPort 是多实例成员引擎端口的测试桩（记录调用）。
type fakeMemberPort struct {
	added   []string // 追加的成员
	removed []string // 移除的执行
	err     error    // 注入错误
}

// AddMember 记录追加调用。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param activityID 活动 ID
// @param assignee 新成员
// @return 错误
func (f *fakeMemberPort) AddMember(_ context.Context, processInstanceID, activityID, assignee string) error {
	if f.err != nil {
		return f.err
	}
	f.added = append(f.added, processInstanceID+"/"+activityID+"/"+assignee)
	return nil
}

// RemoveMember 记录移除调用。
// @param ctx 操作上下文
// @param executionID 执行 ID
// @param completed 是否视作完成
// @return 错误
func (f *fakeMemberPort) RemoveMember(_ context.Context, executionID string, completed bool) error {
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, executionID)
	_ = completed
	return nil
}

// TestAddSign_MultiInstancePort 验证引擎端口注入时的调用链（端口错误短路变量维护）。
// @param t 测试上下文
func TestAddSign_MultiInstancePort(t *testing.T) {
	cs := newCountersignFixture(t)
	fx := NewAddSignService(cs.engine, nil, nil, nil, nil, csClock())
	instance, _ := cs.startCountersign(t, "csCollection", "carol", nil)
	port := &fakeMemberPort{}
	fx.WithMultiInstancePort(port)
	// 端口正常：调用链走通 + 变量维护
	if err := fx.AddMultiInstanceMember(context.Background(), instance.ID, "csTask", "carol"); err != nil {
		t.Fatalf("端口加签失败: %v", err)
	}
	if len(port.added) != 1 || !strings.Contains(port.added[0], "csTask") {
		t.Fatalf("端口应被调用: %v", port.added)
	}
	list, _ := cs.instanceVariable(t, instance.ID, VarExecutorList).([]string)
	if len(list) != 3 {
		t.Fatalf("端口加签后变量应维护: %v", list)
	}
	// 端口移除
	if err := fx.RemoveMultiInstanceMember(context.Background(), instance.ID, "csTask", "carol", "exec-1", true); err != nil {
		t.Fatalf("端口减签失败: %v", err)
	}
	if len(port.removed) != 1 || port.removed[0] != "exec-1" {
		t.Fatalf("端口移除应被调用: %v", port.removed)
	}
	// 端口错误：短路变量维护
	port.err = assertError("引擎端口失败")
	if err := fx.AddMultiInstanceMember(context.Background(), instance.ID, "csTask", "dave"); err == nil || !strings.Contains(err.Error(), "引擎端口失败") {
		t.Fatalf("端口错误应上抛: %v", err)
	}
}

// assertError 构造测试用错误。
// @param message 错误信息
// @return 错误
func assertError(message string) error {
	return api.NewError("TEST-0001", message, nil)
}
