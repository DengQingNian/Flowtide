package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestModelLifecycle 验证模型创建、更新、源码与删除。
// @param t 测试上下文
func TestModelLifecycle(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.TenantID = "t1"
	ctx := context.Background()
	model, err := engine.CreateModel(ctx, api.CreateModelRequest{
		Name:     "Order Model",
		Key:      "orderModel",
		Category: "sales",
		MetaInfo: "{}",
		TenantID: "t1",
	})
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}
	if model.ID == "" || model.Version != 1 || model.CreateTime.IsZero() || model.TenantID != "t1" {
		t.Fatalf("模型默认值错误: %+v", model)
	}
	loaded, err := engine.GetModel(ctx, model.ID)
	if err != nil || loaded.Name != "Order Model" {
		t.Fatalf("查询模型失败: %v %+v", err, loaded)
	}
	if _, err := engine.GetModel(tenant.With(ctx, "t2"), model.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户查询应 404: %v", err)
	}

	name, category, version, deployment := "Order v2", "", 2, "d1"
	updated, err := engine.UpdateModel(ctx, model.ID, api.ModelUpdate{
		Name: &name, Category: &category, Version: &version, DeploymentID: &deployment,
	})
	if err != nil || updated.Name != name || updated.Category != "" || updated.Version != 2 || updated.DeploymentID != "d1" {
		t.Fatalf("更新模型失败: %v %+v", err, updated)
	}

	source := []byte("<xml/>")
	if err := engine.SetModelSource(ctx, model.ID, source); err != nil {
		t.Fatalf("设置源码失败: %v", err)
	}
	got, err := engine.GetModelSource(ctx, model.ID)
	if err != nil || string(got) != "<xml/>" {
		t.Fatalf("读取源码失败: %v %s", err, got)
	}
	extra := []byte("{}")
	if err := engine.SetModelSourceExtra(ctx, model.ID, extra); err != nil {
		t.Fatalf("设置扩展源码失败: %v", err)
	}
	if got, err := engine.GetModelSourceExtra(ctx, model.ID); err != nil || string(got) != "{}" {
		t.Fatalf("读取扩展源码失败: %v %s", err, got)
	}
	if err := engine.DeleteModel(ctx, model.ID); err != nil {
		t.Fatalf("删除模型失败: %v", err)
	}
	if _, err := engine.GetModel(ctx, model.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后查询应 404: %v", err)
	}
}

// TestModelValidationAndSources 验证模型校验与缺失源码。
// @param t 测试上下文
func TestModelValidationAndSources(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	if _, err := engine.CreateModel(ctx, api.CreateModelRequest{Name: "x"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("缺少 Key 应 400: %v", err)
	}
	if _, err := engine.CreateModel(tenant.With(ctx, "t2"), api.CreateModelRequest{Name: "x", Key: "k", TenantID: "t1"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户创建应 404: %v", err)
	}
	model, err := engine.CreateModel(ctx, api.CreateModelRequest{Name: "x", Key: "k"})
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}
	if _, err := engine.GetModelSource(ctx, model.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失源码应 404: %v", err)
	}
	if _, err := engine.GetModelSourceExtra(ctx, model.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失扩展源码应 404: %v", err)
	}
	if _, err := engine.GetModel(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失模型应 404: %v", err)
	}
	if _, err := engine.UpdateModel(ctx, "missing", api.ModelUpdate{}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("更新缺失模型应 404: %v", err)
	}
	if err := engine.DeleteModel(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除缺失模型应 404: %v", err)
	}
	if err := engine.SetModelSource(ctx, "missing", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失模型设置源码应 404: %v", err)
	}
	if _, err := engine.GetModelSource(tenant.With(ctx, "t2"), model.ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户源码应 404: %v", err)
	}
}

// TestModelListAndPersistence 验证模型列表与行导入导出。
// @param t 测试上下文
func TestModelListAndPersistence(t *testing.T) {
	engine := newTestEngine(t)
	ctx := context.Background()
	first, err := engine.CreateModel(tenant.With(ctx, "t1"), api.CreateModelRequest{Name: "A", Key: "a", TenantID: "t1"})
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}
	second, err := engine.CreateModel(ctx, api.CreateModelRequest{Name: "B", Key: "b"})
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}
	if err := engine.SetModelSource(tenant.With(ctx, "t1"), first.ID, []byte("<a/>")); err != nil {
		t.Fatalf("设置源码失败: %v", err)
	}
	if err := engine.SetModelSourceExtra(tenant.With(ctx, "t1"), first.ID, []byte("<extra/>")); err != nil {
		t.Fatalf("设置扩展源码失败: %v", err)
	}
	models, err := engine.ListModels(ctx)
	if err != nil || len(models) != 1 {
		t.Fatalf("模型列表错误: %v %+v", err, models)
	}
	if filtered, err := engine.ListModels(tenant.With(ctx, "t1")); err != nil || len(filtered) != 1 || filtered[0].ID != first.ID {
		t.Fatalf("租户过滤错误: %v %+v", err, filtered)
	}

	rows, err := engine.ExportRows(ctx)
	if err != nil {
		t.Fatalf("导出模型行失败: %v", err)
	}
	modelRows := 0
	sourceRows := 0
	for _, row := range rows {
		if row.Table == "ACT_RE_MODEL" {
			modelRows++
			if row.Values["ID_"] == first.ID && row.Values["EDITOR_SOURCE_VALUE_ID_"] == "" {
				t.Fatalf("源码 ID 未写入: %+v", row.Values)
			}
		}
		if row.Table == "ACT_GE_BYTEARRAY" && row.Values["DEPLOYMENT_ID_"] == nil {
			sourceRows++
		}
	}
	if modelRows != 2 || sourceRows != 2 {
		t.Fatalf("模型行数量错误: %d %d", modelRows, sourceRows)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatalf("恢复模型行失败: %v", err)
	}
	restoredModel, err := restored.GetModel(tenant.With(ctx, "t1"), first.ID)
	if err != nil || restoredModel.Name != first.Name {
		t.Fatalf("恢复模型错误: %v %+v", err, restoredModel)
	}
	source, err := restored.GetModelSource(tenant.With(ctx, "t1"), first.ID)
	if err != nil || string(source) != "<a/>" {
		t.Fatalf("恢复源码错误: %v %s", err, source)
	}
	extra, err := restored.GetModelSourceExtra(tenant.With(ctx, "t1"), first.ID)
	if err != nil || string(extra) != "<extra/>" {
		t.Fatalf("恢复扩展源码错误: %v %s", err, extra)
	}
	if _, err := restored.GetModel(ctx, second.ID); err != nil {
		t.Fatalf("恢复第二个模型失败: %v", err)
	}
}
