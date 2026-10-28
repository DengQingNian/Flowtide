package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestBatchMigrateLifecycle 验证批次创建、成功/失败分片与查询。
// @param t 测试上下文
func TestBatchMigrateLifecycle(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, migrationV1)
	v1, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, app, migrationV2)
	v2, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	first, _ := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	second, _ := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})

	success, err := app.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{first.ID, second.ID}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批次迁移失败: %v", err)
	}
	if success.Total != 2 || success.Completed != 2 || success.Status != BatchStatusCompleted || success.EndTime == nil {
		t.Fatalf("成功批次字段错误: %+v", success)
	}
	if success.TenantID != v2.TenantID || success.Type != BatchTypeProcessInstanceMigration {
		t.Fatalf("批次元数据错误: %+v", success)
	}

	third, _ := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	partial, err := app.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{third.ID, "missing"}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("部分失败批次迁移失败: %v", err)
	}
	if partial.Total != 2 || partial.Completed != 1 || partial.Status != BatchStatusFailed {
		t.Fatalf("部分失败批次字段错误: %+v", partial)
	}

	batches, err := app.ListBatches(context.Background())
	if err != nil || len(batches) != 2 {
		t.Fatalf("批次列表错误: %v %+v", err, batches)
	}
	if batches[0].ID == batches[1].ID {
		t.Fatalf("批次列表重复: %+v", batches)
	}

	got, err := app.GetBatch(context.Background(), partial.ID)
	if err != nil || got.ID != partial.ID || got.Completed != 1 {
		t.Fatalf("查询批次错误: %v %+v", err, got)
	}
	parts, err := app.ListBatchParts(context.Background(), partial.ID)
	if err != nil || len(parts) != 2 {
		t.Fatalf("分片列表错误: %v %+v", err, parts)
	}
	failed := 0
	for _, part := range parts {
		if part.Status == BatchPartStatusFailed {
			failed++
			if part.ErrorMessage == "" || part.EndTime == nil {
				t.Fatalf("失败分片信息缺失: %+v", part)
			}
		}
	}
	if failed != 1 {
		t.Fatalf("失败分片数量错误: %+v", parts)
	}
	one, err := app.GetBatchPart(context.Background(), parts[0].ID)
	if err != nil || one.ID != parts[0].ID || one.BatchID != partial.ID {
		t.Fatalf("查询分片错误: %v %+v", err, one)
	}
}

// TestBatchErrors 验证批次参数与不存在资源错误。
// @param t 测试上下文
func TestBatchErrors(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, migrationV1)
	v1, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, app, migrationV2)
	v2, _ := app.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, _ := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})

	if _, err := app.BatchMigrateProcessInstances(context.Background(), v2.ID, nil, nil); err == nil {
		t.Fatalf("空实例列表应报错")
	}
	if _, err := app.BatchMigrateProcessInstances(context.Background(), "missing", []string{instance.ID}, nil); err == nil {
		t.Fatalf("缺失目标定义应报错")
	}
	// 不同租户请求不应看到目标定义
	otherTenant := tenant.With(context.Background(), "other")
	if _, err := app.BatchMigrateProcessInstances(otherTenant, v2.ID, []string{instance.ID}, nil); err == nil {
		t.Fatalf("跨租户应报错")
	}
	if _, err := app.GetBatch(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失批次应报错")
	}
	if _, err := app.ListBatchParts(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失批次分片应报错")
	}
	if _, err := app.GetBatchPart(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失分片应报错")
	}

	batch, err := app.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{instance.ID}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批次迁移失败: %v", err)
	}
	// 直接调用内部更新/汇总，覆盖异常分支
	if err := app.updateBatchPart(context.Background(), "missing", "missing", BatchPartStatusFailed, ""); err == nil {
		t.Fatalf("缺失批次更新应报错")
	}
	if err := app.updateBatchPart(context.Background(), batch.ID, "missing", BatchPartStatusFailed, ""); err == nil {
		t.Fatalf("缺失分片更新应报错")
	}
	if _, err := app.finishBatch(context.Background(), "missing", 0); err == nil {
		t.Fatalf("缺失批次汇总应报错")
	}
}

// TestBatchSnapshotRoundTrip 验证批次快照持久化与恢复。
// @param t 测试上下文
func TestBatchSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "engine.json")
	first, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, first, migrationV1)
	v1, _ := first.GetProcessDefinitionByKey(context.Background(), "migrate")
	deployXML(t, first, migrationV2)
	v2, _ := first.GetProcessDefinitionByKey(context.Background(), "migrate")
	instance, err := first.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1.ID})
	if err != nil {
		t.Fatalf("启动实例失败: %v", err)
	}
	batch, err := first.BatchMigrateProcessInstances(context.Background(), v2.ID,
		[]string{instance.ID, "missing"}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批次迁移失败: %v", err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭引擎失败: %v", err)
	}

	second, err := New(Config{HistoryLevel: HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("恢复引擎失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	got, err := second.GetBatch(context.Background(), batch.ID)
	if err != nil || got.ID != batch.ID || got.Status != BatchStatusFailed || got.EndTime == nil {
		t.Fatalf("恢复批次错误: %v %+v", err, got)
	}
	parts, err := second.ListBatchParts(context.Background(), batch.ID)
	if err != nil || len(parts) != 2 {
		t.Fatalf("恢复分片失败: %v %+v", err, parts)
	}
	for _, part := range parts {
		if part.Status == BatchPartStatusFailed && part.ErrorMessage == "" {
			t.Fatalf("恢复失败分片缺少原因: %+v", part)
		}
	}
}
