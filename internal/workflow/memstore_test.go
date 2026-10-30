package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// TestMemStoresAggregate 验证内存存储聚合构造（nil 时钟回退真实时钟）。
// @param t 测试上下文
func TestMemStoresAggregate(t *testing.T) {
	stores := NewMemStores(nil)
	if stores.Next == nil || stores.Message == nil || stores.History == nil || stores.Operation == nil || stores.Document == nil {
		t.Fatal("聚合应完整")
	}
	// 各存储独立时钟回退
	if stores.Next.Now().IsZero() || stores.Message.clock.Now().IsZero() {
		t.Fatal("nil 时钟应回退真实时钟")
	}
	// 各构造函数 nil 时钟分支
	if NewMemNextStore(nil) == nil || NewMemMessageStore(nil) == nil || NewMemHistoryStore(nil) == nil || NewMemOperationStore(nil) == nil || NewMemDocumentStore(nil) == nil {
		t.Fatal("nil 时钟构造应回退")
	}
}

// TestCloneHelpers 验证克隆工具的 nil 透传与深拷贝。
// @param t 测试上下文
func TestCloneHelpers(t *testing.T) {
	// nil 透传
	if cloneNext(nil) != nil || cloneMessage(nil) != nil || cloneHistory(nil) != nil || cloneOperation(nil) != nil || cloneDocument(nil) != nil {
		t.Fatal("nil 应透传")
	}
	if cloneFloat(nil) != nil || cloneTime(nil) != nil || cloneI64(nil) != nil || cloneInt(nil) != nil {
		t.Fatal("指针 nil 应透传")
	}
	// 深拷贝：修改副本不影响原值
	value := 1.5
	original := &NextActivity{AgreeWeight: &value}
	copied := cloneNext(original)
	*copied.AgreeWeight = 9.9
	// 原值不受影响
	if *original.AgreeWeight != 1.5 {
		t.Fatalf("深拷贝失败: %v", *original.AgreeWeight)
	}
	// 时间/扩展列深拷贝
	stamped := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	ext := ExtendFields{ExtendI1: int64Ptr(1), ExtendF1: floatPtr(2), ExtendD1: &stamped}
	extCopy := cloneExt(ext)
	*extCopy.ExtendI1 = 99
	// 原扩展列不受影响
	if *ext.ExtendI1 != 1 {
		t.Fatal("扩展列深拷贝失败")
	}
	msg := &MessageExtend{ReadTime: &stamped, PlanFinishTime: &stamped, TaskReceiptTime: &stamped, TaskStartTime: &stamped, DelayTime: &stamped}
	msgCopy := cloneMessage(msg)
	*msgCopy.ReadTime = stamped.Add(time.Hour)
	// 原时间不受影响
	if msgCopy.ReadTime.Equal(*msg.ReadTime) {
		t.Fatal("待办时间深拷贝失败")
	}
	hist := &HistoryExtend{MessageExtend: *msg, TaskFinishTime: &stamped}
	if cloneHistory(hist).TaskFinishTime == nil {
		t.Fatal("已办克隆失败")
	}
	op := &OperationRecord{IsAgree: intPtr(1), DealTime: &stamped, TaskReceiptTime: &stamped, TaskStartTime: &stamped, TaskFinishTime: &stamped}
	if cloneOperation(op).IsAgree == nil {
		t.Fatal("操作记录克隆失败")
	}
	doc := &Document{FlowStartTime: &stamped, FlowFinishTime: &stamped, FlowTerminationTime: &stamped, ExtendFields: ext}
	if cloneDocument(doc).FlowStartTime == nil {
		t.Fatal("单据克隆失败")
	}
}

// TestPaginate 验证内存分页边界。
// @param t 测试上下文
func TestPaginate(t *testing.T) {
	rows := []int{1, 2, 3, 4, 5}
	// size<=0 返回 start 起的全量尾部
	data, total := paginate(rows, 2, 0)
	if total != 5 || len(data) != 3 || data[0] != 3 {
		t.Fatalf("不分页语义错误: %d %d", total, len(data))
	}
	// 越界 start 返回空页
	data, total = paginate(rows, 9, 2)
	if total != 5 || len(data) != 0 {
		t.Fatalf("越界分页错误: %d %d", total, len(data))
	}
	// 尾页截断
	data, total = paginate(rows, 4, 3)
	if total != 5 || len(data) != 1 || data[0] != 5 {
		t.Fatalf("尾页分页错误: %d %d", total, len(data))
	}
	// 负 start 归零
	data, _ = paginate(rows, -1, 2)
	if len(data) != 2 || data[0] != 1 {
		t.Fatalf("负偏移应归零: %v", data)
	}
}

// TestApplyDocumentChanges 验证单据变更映射全列覆盖。
// @param t 测试上下文
func TestApplyDocumentChanges(t *testing.T) {
	doc := &Document{FlowStatus: FlowStatusRunning}
	applyDocumentChanges(doc, map[string]any{
		"flow_status":            FlowStatusFinished,
		"document_url":           nil,
		"flow_task_id":           "t1",
		"flow_action_id":         "a1",
		"flow_action":            "审批",
		"flow_current_step_name": "审批",
		"flow_current_persons":   "张三",
		"flow_start_time":        time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local),
		"flow_finish_time":       time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local),
		"flow_termination_time":  (*time.Time)(nil),
		"updater_name":           "更新人",
		"flow_title":             "标题",
		"flow_category_id":       "leave",
		"flow_category":          "请假",
		"unknown_column":         "ignored",
	})
	// 命中列生效
	if doc.FlowStatus != FlowStatusFinished || doc.FlowTaskID != "t1" || doc.FlowCurrentPersons != "张三" {
		t.Fatalf("变更应用错误: %+v", doc)
	}
	// nil 字符串置空
	if doc.DocumentURL != "" {
		t.Fatal("nil 应置空字符串列")
	}
	// nil 时间指针置空
	if doc.FlowTerminationTime != nil {
		t.Fatal("nil 应置空时间列")
	}
	// 时间值生效
	if doc.FlowStartTime == nil || doc.FlowFinishTime == nil {
		t.Fatal("时间列未生效")
	}
	if doc.UpdaterName != "更新人" || doc.FlowTitle != "标题" || doc.FlowCategoryID != "leave" {
		t.Fatalf("名称列未生效: %+v", doc)
	}
	// 指针时间透传
	stamped := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	applyDocumentChanges(doc, map[string]any{"flow_start_time": &stamped})
	if !doc.FlowStartTime.Equal(stamped) {
		t.Fatal("指针时间应透传")
	}
	// 非法类型忽略
	applyDocumentChanges(doc, map[string]any{"flow_start_time": 42, "flow_status": 7})
	if doc.FlowStatus != FlowStatusFinished {
		t.Fatalf("非法类型不应变更字符串列: %s", doc.FlowStatus)
	}
}

// TestMemDocumentStorePaths 验证单据内存存储的查询与逻辑删除路径。
// @param t 测试上下文
func TestMemDocumentStorePaths(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	store := NewMemDocumentStore(fake)
	ctx := context.Background()
	doc := &Document{BaseColumns: BaseColumns{ID: "d1", TenantID: "sys"}, BusinessID: "biz-1", FlowInstanceID: "pi-1"}
	if err := store.Save(ctx, doc); err != nil {
		t.Fatal(err)
	}
	// upsert 保留原主键
	doc2 := &Document{BaseColumns: BaseColumns{ID: "d2", TenantID: "sys"}, BusinessID: "biz-1", FlowInstanceID: "pi-1", FlowStatus: FlowStatusRunning}
	if err := store.Save(ctx, doc2); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetByBusinessID(ctx, "biz-1", "sys")
	if err != nil || !found || got.ID != "d1" {
		t.Fatalf("upsert 主键语义错误: %v %v %+v", found, err, got)
	}
	// 更新时间随刷新推进
	fake.Advance(time.Hour)
	if err := store.UpdateByInstance(ctx, "pi-1", map[string]any{"flow_status": FlowStatusRunning}); err != nil {
		t.Fatal(err)
	}
	got, _, _ = store.GetByInstance(ctx, "pi-1")
	if got.UpdateTime.IsZero() || got.UpdateTime.Equal(got.CreateTime) {
		t.Fatalf("更新时间未刷新: %v", got.UpdateTime)
	}
	// 逻辑删除后查询不可见
	if err := store.MarkDeletedByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.GetByBusinessID(ctx, "biz-1", "sys"); found {
		t.Fatal("逻辑删除后不应可见")
	}
	if _, found, _ := store.GetByInstance(ctx, "pi-1"); found {
		t.Fatal("逻辑删除后按实例不应可见")
	}
	// 未删除的其他租户不可见
	other := &Document{BaseColumns: BaseColumns{ID: "d3", TenantID: "other"}, BusinessID: "biz-2", FlowInstanceID: "pi-2"}
	if err := store.Save(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.GetByBusinessID(ctx, "biz-2", "sys"); found {
		t.Fatal("租户不匹配不应可见")
	}
	if _, found, _ := store.GetByInstance(ctx, "pi-none"); found {
		t.Fatal("未知实例不应可见")
	}
}

// failingOps 是带错误注入的操作记录存储桩（覆盖错误传播路径）。
type failingOps struct {
	OperationRecordStore
	failLastType bool
	failCount    bool
}

// LastTypeByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 类型
// @return 错误
func (f *failingOps) LastTypeByInstance(ctx context.Context, processInstanceID string) (string, error) {
	// 错误注入点
	if f.failLastType {
		return "", errors.New("注入错误")
	}
	return f.OperationRecordStore.LastTypeByInstance(ctx, processInstanceID)
}

// CountByInstance 按开关返回错误。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 计数
// @return 错误
func (f *failingOps) CountByInstance(ctx context.Context, processInstanceID string) (int, error) {
	// 错误注入点
	if f.failCount {
		return 0, errors.New("注入错误")
	}
	return f.OperationRecordStore.CountByInstance(ctx, processInstanceID)
}

// TestDocumentErrorPaths 验证状态机错误传播。
// @param t 测试上下文
func TestDocumentErrorPaths(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	stores := NewMemStores(fake)
	// 实例桩带运行实例（保证业务键可解析，进入状态推导）
	instances := &fakeInstances{running: map[string]api.ProcessInstance{
		"pi-1": {ID: "pi-1", BusinessKey: "biz-1", TenantID: "sys"},
	}}
	failing := &failingOps{OperationRecordStore: stores.Operation, failLastType: true}
	service := NewDocumentService(stores.Document, stores.Message, failing, instances, &fakeTaskHistory{}, fake)
	ctx := context.Background()
	// computeStatus 错误透传（更新入口）
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-1", ""); err == nil {
		t.Fatal("状态推导错误应透传")
	}
	// computeStatus 错误透传（清理入口）
	if err := service.CleanByProcessInstance(ctx, "pi-1"); err == nil {
		t.Fatal("状态推导错误应透传")
	}
	// 计数错误透传
	failing.failLastType = false
	failing.failCount = true
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-1", ""); err == nil {
		t.Fatal("计数错误应透传")
	}
	if err := service.CleanByProcessInstance(ctx, "pi-1"); err == nil {
		t.Fatal("计数错误应透传")
	}
	// 未知实例（运行与历史都查不到）→ 静默跳过
	failing.failCount = false
	if err := service.UpdateByProcessInstanceOrTask(ctx, "pi-none", ""); err != nil {
		t.Fatalf("未知实例应静默: %v", err)
	}
}

// TestMemMessageStorePaths 验证待办内存存储的幂等与查询路径。
// @param t 测试上下文
func TestMemMessageStorePaths(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	store := NewMemMessageStore(fake)
	ctx := context.Background()
	msg := &MessageExtend{BaseColumns: BaseColumns{ID: "m1", TenantID: "sys"}, TaskID: "t1", ProcessInstanceID: "pi-1", BusinessID: "biz", OwnerID: "u1"}
	if err := store.Save(ctx, msg); err != nil {
		t.Fatal(err)
	}
	// 幂等查询
	if _, found, _ := store.GetByTaskID(ctx, "t1"); !found {
		t.Fatal("按任务查询失败")
	}
	if _, found, _ := store.GetByTaskID(ctx, "none"); found {
		t.Fatal("未知任务不应命中")
	}
	// 按任务+处理人删除：处理人不匹配时静默
	if err := store.DeleteByTaskIDAndOwner(ctx, "t1", "u2"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.GetByTaskID(ctx, "t1"); !found {
		t.Fatal("处理人不匹配不应删除")
	}
	// 命中处理人删除
	if err := store.DeleteByTaskIDAndOwner(ctx, "t1", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.GetByTaskID(ctx, "t1"); found {
		t.Fatal("命中处理人应删除")
	}
	// UpdateRead/UpdateDelay/SetPlanFinishTime 未知任务静默
	if err := store.UpdateRead(ctx, "none", "u1", fake.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateDelay(ctx, "none", fake.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPlanFinishTime(ctx, "none", nil); err != nil {
		t.Fatal(err)
	}
}

// TestMemNextStorePaths 验证下一步内存存储的边界路径。
// @param t 测试上下文
func TestMemNextStorePaths(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	store := NewMemNextStore(fake)
	ctx := context.Background()
	// FindAssignSource 未命中
	if _, found, err := store.FindAssignSource(ctx, "pi-1", "a", "s", "t"); err != nil || found {
		t.Fatalf("未命中应返回 false: %v %v", found, err)
	}
	// DeleteForResolve/DeleteForRollback/DeleteForBackToStep 对空库静默
	if err := store.DeleteForResolve(ctx, "pi-1", "a", "t", "u"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteForRollback(ctx, "pi-1", "t", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteForBackToStep(ctx, "pi-1", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteByProcessInstance(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", nil); err != nil {
		t.Fatal(err)
	}
	// MarkUsedAndWriteBack 未知 ID 静默
	if err := store.MarkUsedAndWriteBack(ctx, []string{"none"}, "t1"); err != nil {
		t.Fatal(err)
	}
}

// TestMemHistoryOperationPaths 验证已办/操作记录内存存储的边界路径。
// @param t 测试上下文
func TestMemHistoryOperationPaths(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	history := NewMemHistoryStore(fake)
	operations := NewMemOperationStore(fake)
	ctx := context.Background()
	// UpdateFinishTime 未知任务静默
	if err := history.UpdateFinishTime(ctx, "none", "u1", fake.Now()); err != nil {
		t.Fatal(err)
	}
	// DeleteByTaskID 未知任务静默
	if err := history.DeleteByTaskID(ctx, "none"); err != nil {
		t.Fatal(err)
	}
	// 空库操作记录查询
	if rows, err := operations.ListByTask(ctx, "none"); err != nil || len(rows) != 0 {
		t.Fatalf("空库查询失败: %d %v", len(rows), err)
	}
	if _, err := operations.ListSignable(ctx, nil); err != nil {
		t.Fatal(err)
	}
}
