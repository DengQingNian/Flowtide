package workflow

import (
	"context"
	"testing"
	"time"

	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// TestMemCompleteToActivityStore 验证直送记录内存存储：校验/保存/查询/克隆隔离。
// @param t 测试上下文
func TestMemCompleteToActivityStore(t *testing.T) {
	store := NewMemCompleteToActivityStore(nil)
	ctx := context.Background()

	// 校验：缺少目标/任务/实例拒绝
	if err := store.Save(ctx, nil); err == nil {
		t.Fatalf("nil 记录应拒绝")
	}
	if err := store.Save(ctx, &CompleteToActivity{TaskID: "t1", ProcessInstanceID: "pi1"}); err == nil {
		t.Fatalf("缺目标应拒绝")
	}
	if err := store.Save(ctx, &CompleteToActivity{TargetActivityID: "approve", ProcessInstanceID: "pi1"}); err == nil {
		t.Fatalf("缺任务应拒绝")
	}
	if err := store.Save(ctx, &CompleteToActivity{TargetActivityID: "approve", TaskID: "t1"}); err == nil {
		t.Fatalf("缺实例应拒绝")
	}

	// 保存：ID/审计列自动填充
	record := &CompleteToActivity{
		BaseColumns:       BaseColumns{TenantID: "dev", CreateTime: time.Time{}},
		SourceActivityID:  "apply",
		TargetActivityID:  "approve",
		TaskID:            "t1",
		ProcessInstanceID: "pi1",
	}
	if err := store.Save(ctx, record); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if record.ID == "" || record.CreateTime.IsZero() {
		t.Fatalf("审计列应自动填充: %+v", record)
	}
	// 第二条（同实例不同任务；显式更晚的创建时间保证升序断言确定）
	if err := store.Save(ctx, &CompleteToActivity{
		BaseColumns:       BaseColumns{CreateTime: record.CreateTime.Add(time.Minute)},
		SourceActivityID:  "handle",
		TargetActivityID:  "review",
		TaskID:            "t2",
		ProcessInstanceID: "pi1",
	}); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 按实例查询（按创建时间升序）
	byInstance, err := store.ListByProcessInstance(ctx, "pi1")
	if err != nil || len(byInstance) != 2 {
		t.Fatalf("按实例查询错误: %d（%v）", len(byInstance), err)
	}
	if byInstance[0].TaskID != "t1" || byInstance[1].TaskID != "t2" {
		t.Fatalf("升序排序错误: %s, %s", byInstance[0].TaskID, byInstance[1].TaskID)
	}
	// 按任务查询
	byTask, err := store.ListByTask(ctx, "t2")
	if err != nil || len(byTask) != 1 || byTask[0].TargetActivityID != "review" {
		t.Fatalf("按任务查询错误: %+v（%v）", byTask, err)
	}
	// 未命中实例返回空
	empty, err := store.ListByProcessInstance(ctx, "ghost")
	if err != nil || len(empty) != 0 {
		t.Fatalf("未命中应为空: %d（%v）", len(empty), err)
	}
	// 乱序创建时间：升序排序的冒泡交换分支
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	for i, offset := range []int{2, 0, 1} {
		if err := store.Save(ctx, &CompleteToActivity{
			BaseColumns:       BaseColumns{ID: "sort-" + string(rune('a'+i)), CreateTime: base.Add(time.Duration(offset) * time.Minute)},
			TargetActivityID:  "t" + string(rune('a'+i)),
			TaskID:            "tsort",
			ProcessInstanceID: "pisort",
		}); err != nil {
			t.Fatalf("保存排序样本失败: %v", err)
		}
	}
	sorted, _ := store.ListByTask(ctx, "tsort")
	if len(sorted) != 3 || sorted[0].TargetActivityID != "tb" || sorted[1].TargetActivityID != "tc" || sorted[2].TargetActivityID != "ta" {
		t.Fatalf("乱序记录应按创建时间升序: %+v", sorted)
	}
	// nil 记录克隆防御
	if cloned := cloneCompleteToActivity(nil); cloned != nil {
		t.Fatalf("nil 克隆应返回 nil")
	}
	// 克隆隔离：修改返回值不影响存储
	byTask[0].TargetActivityID = "hacked"
	again, _ := store.ListByTask(ctx, "t2")
	if again[0].TargetActivityID != "review" {
		t.Fatalf("行克隆隔离失败")
	}
}

// TestCompleteToActivityRowConversion 验证 SQL 适配的实体↔行转换（纯函数，无 DB）。
// @param t 测试上下文
func TestCompleteToActivityRowConversion(t *testing.T) {
	created := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	entity := &CompleteToActivity{
		BaseColumns: BaseColumns{
			ID: "row-1", Creator: "u1", CreatorName: "张三",
			CreateTime: created, Updater: "u1", UpdateTime: created, TenantID: "dev",
		},
		FlowID:            "flow-9",
		SourceActivityID:  "apply",
		TargetActivityID:  "approve",
		TaskID:            "t9",
		ProcessInstanceID: "pi9",
	}
	row := completeToActivityToRow(entity)
	// 行列完整（表定义列名核对）
	for _, column := range []string{"id", "flow_id", "source_flow_element_id", "target_flow_element_id", "task_id", "process_instance_id", "creator", "create_time", "tenant_id"} {
		if _, ok := row[column]; !ok {
			t.Fatalf("缺少列 %s", column)
		}
	}
	// 行转实体往返
	back := completeToActivitiesFromRows([]pwf.Row{row})
	if len(back) != 1 {
		t.Fatalf("转换结果缺失")
	}
	restored := back[0]
	if restored.ID != "row-1" || restored.FlowID != "flow-9" || restored.SourceActivityID != "apply" ||
		restored.TargetActivityID != "approve" || restored.TaskID != "t9" || restored.ProcessInstanceID != "pi9" ||
		restored.TenantID != "dev" || restored.CreatorName != "张三" || !restored.CreateTime.Equal(created) {
		t.Fatalf("往返转换字段错误: %+v", restored)
	}
	// 空行列表转换
	if out := completeToActivitiesFromRows(nil); len(out) != 0 {
		t.Fatalf("空行应转空列表")
	}
}

// TestNewSQLCompleteToActivityStore 验证 SQL 适配器构造（组合 pwf 仓储）。
// @param t 测试上下文
func TestNewSQLCompleteToActivityStore(t *testing.T) {
	// 无 DB 场景仅验证装配（Save 走集成开关覆盖）
	store := NewSQLCompleteToActivityStore(nil)
	if store == nil || store.repos == nil {
		t.Fatalf("SQL 适配器装配失败")
	}
}
