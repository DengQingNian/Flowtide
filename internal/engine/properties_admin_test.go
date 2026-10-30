package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestPropertiesAdmin 验证自定义属性 CRUD。
// @param t 测试上下文
func TestPropertiesAdmin(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	properties, err := engine.Properties(ctx)
	if err != nil || properties["schema.version"] == "" {
		t.Fatalf("初始属性错误: %v %+v", err, properties)
	}
	if err := engine.CreateProperty(ctx, "feature.x", "1"); err != nil {
		t.Fatalf("创建属性失败: %v", err)
	}
	properties, _ = engine.Properties(ctx)
	if properties["feature.x"] != "1" {
		t.Fatalf("自定义属性缺失: %+v", properties)
	}
	if err := engine.CreateProperty(ctx, "feature.x", "2"); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("重复创建应 409: %v", err)
	}
	if err := engine.CreateProperty(ctx, "schema.version", "x"); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("内置属性创建应 409: %v", err)
	}
	if err := engine.CreateProperty(ctx, " ", "x"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("空属性名应 400: %v", err)
	}
	if err := engine.UpdateProperty(ctx, "feature.x", "2"); err != nil {
		t.Fatalf("更新属性失败: %v", err)
	}
	if properties, _ := engine.Properties(ctx); properties["feature.x"] != "2" {
		t.Fatalf("属性更新未生效: %+v", properties)
	}
	if err := engine.UpdateProperty(ctx, "schema.version", "x"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("内置属性更新应 400: %v", err)
	}
	if err := engine.UpdateProperty(ctx, "missing", "x"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失属性更新应 404: %v", err)
	}
	if err := engine.DeleteProperty(ctx, "schema.version"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("内置属性删除应 400: %v", err)
	}
	if err := engine.DeleteProperty(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失属性删除应 404: %v", err)
	}

	rows, err := engine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("导出行失败: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.Table == "ACT_GE_PROPERTY" && row.Values["NAME_"] == "feature.x" {
			found = true
			if row.Values["VALUE_"] != "2" {
				t.Fatalf("属性行值错误: %+v", row.Values)
			}
		}
	}
	if !found {
		t.Fatalf("未导出属性行")
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if properties, _ := restored.Properties(ctx); properties["feature.x"] != "2" {
		t.Fatalf("恢复属性错误: %+v", properties)
	}
	if snapshotProperties := restored.toSnapshot().Properties; snapshotProperties["feature.x"] != "2" {
		t.Fatalf("快照属性错误: %+v", snapshotProperties)
	}
	cloned := cloneState(engine.state)
	if cloned.Properties["feature.x"] != "2" {
		t.Fatalf("克隆属性错误: %+v", cloned.Properties)
	}
	if err := engine.DeleteProperty(ctx, "feature.x"); err != nil {
		t.Fatalf("删除属性失败: %v", err)
	}
	if properties, _ := engine.Properties(ctx); properties["feature.x"] != "" {
		t.Fatalf("删除后属性仍存在: %+v", properties)
	}
}
