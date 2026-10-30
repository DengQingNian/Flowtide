package workflow

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// newDocumentFixture 构造业务表单状态域服务测试夹具（含引擎桩与预置数据）。
// @param t 测试上下文
// @return 域服务
// @return 内存存储聚合
// @return 固定时钟
// @return 实例桩（用例内可调整状态）
func newDocumentFixture(t *testing.T) (*DocumentService, *MemStores, *clock.Fake, *fakeInstances) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local))
	stores := NewMemStores(fake)
	instances := &fakeInstances{
		running: map[string]api.ProcessInstance{
			"pi-1": {ID: "pi-1", BusinessKey: "biz-1", TenantID: "sys"},
		},
		history: map[string]api.HistoricProcessInstance{
			"pi-2": {ID: "pi-2", BusinessKey: "biz-2", TenantID: "sys", EndTime: clockTime(fake.Now())},
		},
	}
	tasks := &fakeTaskHistory{tasks: map[string]api.HistoricTaskInstance{
		"task-draft": {ID: "task-draft", Name: "草稿", ProcessInstanceID: "pi-1", TaskDefinitionKey: "Draft"},
		"task-1":     {ID: "task-1", Name: "审批", ProcessInstanceID: "pi-1", TaskDefinitionKey: "step-approve"},
	}}
	service := NewDocumentService(stores.Document, stores.Message, stores.Operation, instances, tasks, fake)
	return service, stores, fake, instances
}

// clockTime 构造时间指针。
// @param t 时间
// @return 指针
func clockTime(t time.Time) *time.Time { return &t }

// seedPending 为实例预置待办与操作记录（驱动状态机）。
// @param ctx 操作上下文
// @param stores 存储聚合
// @param procInstID 流程实例 Id
// @param stepCodes 待办步骤代号列表
// @param opCount 操作记录条数
// @return 错误
func seedPending(ctx context.Context, stores *MemStores, procInstID, businessKey string, stepCodes []string, opCount int) error {
	for i, code := range stepCodes {
		msg := &MessageExtend{
			BaseColumns:       BaseColumns{ID: "msg-" + code + "-" + string(rune('0'+i)), TenantID: "sys"},
			TaskID:            "task-" + code,
			ProcessInstanceID: procInstID,
			BusinessID:        businessKey,
			TaskStepCode:      code,
			TaskStepName:      "步骤-" + code,
			OwnerID:           "u" + string(rune('0'+i)),
			OwnerName:         "待办人" + string(rune('0'+i)),
			TargetURL:         "/form/" + code,
		}
		if err := stores.Message.Save(ctx, msg); err != nil {
			return err
		}
	}
	for i := 0; i < opCount; i++ {
		// 逐条推进时间源，保证排序键单调（内存存储按 CreateTime/DealTime 排序）
		stamped := stores.Operation.Now()
		record := &OperationRecord{
			BaseColumns:       BaseColumns{ID: "op-" + procInstID + "-" + string(rune('0'+i)), TenantID: "sys", CreateTime: stamped, UpdateTime: stamped},
			ProcessInstanceID: procInstID,
			BusinessID:        businessKey,
			OwnerID:           "u9",
			Type:              CommentTypeTJ,
			TaskStepCode:      "step-x",
			DealTime:          &stamped,
		}
		if err := stores.Operation.Save(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

// TestDocumentSave 验证保存：默认未提交、Draft 记开始时间、upsert 幂等。
// @param t 测试上下文
func TestDocumentSave(t *testing.T) {
	service, stores, fake, _ := newDocumentFixture(t)
	ctx := context.Background()
	doc, err := service.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "sys", Creator: "u0"},
		BusinessID:     "biz-1",
		FlowInstanceID: "pi-1",
		FlowTitle:      "请假申请",
	}, TaskKey: "step-approve"})
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	// 默认状态"未提交"
	if doc.FlowStatus != FlowStatusNotSubmitted {
		t.Fatalf("默认状态应为未提交: %s", doc.FlowStatus)
	}
	if doc.ID == "" || doc.Creator != "u0" {
		t.Fatalf("审计列未填充: %+v", doc.BaseColumns)
	}
	// Draft 提交记录开始时间
	draft, err := service.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "sys"},
		BusinessID:     "biz-1",
		FlowInstanceID: "pi-1",
	}, TaskKey: "Draft"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.FlowStartTime == nil || !draft.FlowStartTime.Equal(fake.Now()) {
		t.Fatalf("Draft 应记录开始时间: %v", draft.FlowStartTime)
	}
	// upsert：不新增行
	if err := stores.Document.MarkDeletedByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatal(err)
	}
	// 缺参校验
	if _, err := service.Save(ctx, DocumentSaveRequest{Document: &Document{BusinessID: "biz-2"}}); err == nil {
		t.Fatal("缺租户应报错")
	}
	if _, err := service.Save(ctx, DocumentSaveRequest{}); err == nil {
		t.Fatal("空单据应报错")
	}
}

// TestDocumentUpdateStateMachine 验证状态机四分支与待办聚合。
// @param t 测试上下文
func TestDocumentUpdateStateMachine(t *testing.T) {
	service, stores, fake, instances := newDocumentFixture(t)
	ctx := context.Background()
	// 建单据
	if _, err := service.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "sys"},
		BusinessID:     "biz-1",
		FlowInstanceID: "pi-1",
		FlowStatus:     FlowStatusNotSubmitted,
	}}); err != nil {
		t.Fatal(err)
	}
	// 分支一：仅 1 条操作记录 → 未提交（不聚合报错前先预置待办）
	if err := seedPending(ctx, stores, "pi-1", "biz-1", []string{"step-a"}, 1); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-1", ""); err != nil {
		t.Fatalf("未提交分支失败: %v", err)
	}
	doc, found, err := service.GetByInstance(ctx, "pi-1")
	if err != nil || !found {
		t.Fatalf("单据查询失败: %v %v", found, err)
	}
	if doc.FlowStatus != FlowStatusNotSubmitted {
		t.Fatalf("状态应为未提交: %s", doc.FlowStatus)
	}
	if doc.FlowCurrentPersons != "待办人0" || doc.FlowTaskID != "task-step-a" || doc.FlowActionID != "step-a" {
		t.Fatalf("待办聚合错误: %+v", doc)
	}
	if doc.DocumentURL != "/form/step-a" || doc.FlowCurrentStepName != "步骤-step-a" {
		t.Fatalf("待办聚合 URL/步骤名错误: %+v", doc)
	}
	// 分支二：操作记录 >1 → 流转中；Draft 办结任务记录开始时间
	if err := seedPending(ctx, stores, "pi-1", "biz-1", []string{"step-a", "step-b"}, 2); err != nil {
		t.Fatal(err)
	}
	fake.Advance(time.Hour)
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-1", "task-draft"); err != nil {
		t.Fatalf("流转中分支失败: %v", err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-1")
	if doc.FlowStatus != FlowStatusRunning {
		t.Fatalf("状态应为流转中: %s", doc.FlowStatus)
	}
	if doc.FlowStartTime == nil || !doc.FlowStartTime.Equal(fake.Now()) {
		t.Fatalf("Draft 应记录开始时间: %v", doc.FlowStartTime)
	}
	// 聚合去重：两个步骤 → persons/codes/names 各两项（内存存储同 CreateTime 时顺序不定，按集合比较）
	if got := splitAndSort(doc.FlowActionID); len(got) != 2 || got[0] != "step-a" || got[1] != "step-b" {
		t.Fatalf("步骤代号去重聚合错误: %q", doc.FlowActionID)
	}
	if got := splitAndSort(doc.FlowCurrentPersons); len(got) != 2 || got[0] != "待办人0" || got[1] != "待办人1" {
		t.Fatalf("待办人去重聚合错误: %q", doc.FlowCurrentPersons)
	}
	// 分支三：待办命中签收代号 → 已完成（不清空聚合列）
	if err := seedPending(ctx, stores, "pi-1", "biz-1", []string{"Sign"}, 2); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-1", ""); err != nil {
		t.Fatalf("签收分支失败: %v", err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-1")
	if doc.FlowStatus != FlowStatusFinished {
		t.Fatalf("签收后状态应为已完成: %s", doc.FlowStatus)
	}
	if doc.FlowCurrentPersons == "" {
		t.Fatal("签收分支不清空聚合列")
	}
	// 分支四：终止（最后操作记录 LCZZ）→ 已终止 + 终止时间 + 清空待办
	// 推进时钟，保证 LCZZ 记录的处理时间严格晚于既有记录（排序键唯一）
	fake.Advance(time.Second)
	stampedLCZZ := stores.Operation.Now()
	terminated := &OperationRecord{
		BaseColumns:       BaseColumns{ID: "op-lczz", TenantID: "sys", CreateTime: stampedLCZZ, UpdateTime: stampedLCZZ},
		ProcessInstanceID: "pi-1",
		BusinessID:        "biz-1",
		OwnerID:           "u9",
		Type:              CommentTypeLCZZ,
		DealTime:          &stampedLCZZ,
	}
	if err := stores.Operation.Save(ctx, terminated); err != nil {
		t.Fatal(err)
	}
	fake.Advance(time.Hour)
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-1", ""); err != nil {
		t.Fatalf("终止分支失败: %v", err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-1")
	if doc.FlowStatus != FlowStatusTerminated {
		t.Fatalf("状态应为已终止: %s", doc.FlowStatus)
	}
	if doc.FlowTerminationTime == nil {
		t.Fatal("终止时间未记录")
	}
	if doc.FlowCurrentPersons != "" || doc.DocumentURL != "" || doc.FlowTaskID != "" {
		t.Fatalf("终止分支应清空待办聚合: %+v", doc)
	}
	// 分支五：实例已结束（运行表查不到、历史有结束时间）→ 已完成 + 完成时间
	if _, err := service.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "sys"},
		BusinessID:     "biz-2",
		FlowInstanceID: "pi-2",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := seedPending(ctx, stores, "pi-2", "biz-2", []string{"step-c"}, 2); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-2", ""); err != nil {
		t.Fatalf("已完成分支失败: %v", err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-2")
	if doc.FlowStatus != FlowStatusFinished || doc.FlowFinishTime == nil {
		t.Fatalf("已完成分支错误: %s %v", doc.FlowStatus, doc.FlowFinishTime)
	}
	// 未知实例：静默跳过
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-none", ""); err != nil {
		t.Fatalf("未知实例应静默: %v", err)
	}
	// taskID 反查实例（Draft 任务挂 pi-1）
	if err := service.UpdateByProcessInstanceOrTask(ctx, "", "task-1"); err != nil {
		t.Fatalf("按任务回写失败: %v", err)
	}
	_ = instances
	// 无法定位任务时静默跳过
	if err := service.UpdateByProcessInstanceOrTask(ctx, "", "task-ghost"); err != nil {
		t.Fatalf("未知任务应静默: %v", err)
	}
}

// TestDocumentHelpers 验证 Terminate/Finish/CleanByProcessInstance 辅助方法。
// @param t 测试上下文
func TestDocumentHelpers(t *testing.T) {
	service, stores, _, _ := newDocumentFixture(t)
	ctx := context.Background()
	if _, err := service.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{TenantID: "sys"},
		BusinessID:     "biz-1",
		FlowInstanceID: "pi-1",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := seedPending(ctx, stores, "pi-1", "biz-1", []string{"step-a"}, 2); err != nil {
		t.Fatal(err)
	}
	// Terminate：置已终止 + 终止时间 + 清空
	if err := service.Terminate(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	doc, _, _ := service.GetByInstance(ctx, "pi-1")
	if doc.FlowStatus != FlowStatusTerminated || doc.FlowTerminationTime == nil || doc.FlowCurrentPersons != "" {
		t.Fatalf("Terminate 语义错误: %+v", doc)
	}
	// Finish：置已完成 + 完成时间 + 清空
	if err := service.Finish(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-1")
	if doc.FlowStatus != FlowStatusFinished || doc.FlowFinishTime == nil {
		t.Fatalf("Finish 语义错误: %+v", doc)
	}
	// CleanByProcessInstance：按状态机重算（2 条操作记录 → 流转中，不清列）
	if err := service.CleanByProcessInstance(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-1")
	if doc.FlowStatus != FlowStatusRunning {
		t.Fatalf("Clean 后状态应为流转中: %s", doc.FlowStatus)
	}
	// MarkFinishTime 补偿回写
	stamped := time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local)
	if err := service.MarkFinishTime(ctx, "pi-1", stamped); err != nil {
		t.Fatal(err)
	}
	doc, _, _ = service.GetByInstance(ctx, "pi-1")
	if doc.FlowFinishTime == nil || !doc.FlowFinishTime.Equal(stamped) {
		t.Fatalf("补偿回写失败: %v", doc.FlowFinishTime)
	}
	// 按业务单据查询
	if _, found, err := service.GetByBusinessID(ctx, "biz-1", "sys"); err != nil || !found {
		t.Fatalf("按业务单据查询失败: %v %v", found, err)
	}
}

// splitAndSort 拆分逗号串并排序（聚合顺序无关断言用）。
// @param joined 逗号串
// @return 排序后的片段列表
func splitAndSort(joined string) []string {
	if joined == "" {
		return nil
	}
	parts := strings.Split(joined, ",")
	sort.Strings(parts)
	return parts
}

// TestDocumentJoinHelpers 验证聚合工具函数边界。
// @param t 测试上下文
func TestDocumentJoinHelpers(t *testing.T) {
	// appendUnique 去重与空串跳过
	values := appendUnique(nil, "a")
	values = appendUnique(values, "a")
	values = appendUnique(values, "")
	values = appendUnique(values, "b")
	if len(values) != 2 || values[0] != "a" || values[1] != "b" {
		t.Fatalf("appendUnique 语义错误: %v", values)
	}
	// joinNonEmpty 全空返回空串
	if got := joinNonEmpty([]string{"", ""}); got != "" {
		t.Fatalf("joinNonEmpty 全空应返回空串: %q", got)
	}
	if got := joinNonEmpty([]string{"a", "", "b"}); got != "a,b" {
		t.Fatalf("joinNonEmpty 拼接错误: %q", got)
	}
	// containsAny
	if !containsAny([]string{"x", "Sign"}, signStepCodes) {
		t.Fatal("containsAny 应命中")
	}
	if containsAny([]string{"x"}, signStepCodes) {
		t.Fatal("containsAny 不应命中")
	}
	// stepCodesOf
	if codes := stepCodesOf([]*MessageExtend{{TaskStepCode: "a"}, {TaskStepCode: "b"}}); len(codes) != 2 {
		t.Fatalf("stepCodesOf 错误: %v", codes)
	}
}
