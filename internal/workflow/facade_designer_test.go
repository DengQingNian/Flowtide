package workflow

// facade_designer_test.go 覆盖设计器 XML 存取门面：建模/读改源/历史定义回填/复制/
// 模型步骤联查/步骤详情/引擎委托/权重清空。复用 opsFixtureEnv。

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// travelFixture 是设计器链路专用语料（process id 与模型代号 travelProc 一致）。
const travelFixture = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="travelProc" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="apply"/>
    <userTask id="apply" name="申请" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f1" sourceRef="apply" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestOpsDesignerModelLifecycle 设计器建模→读源→改源→部署→历史回填全链路。
// @param t 测试上下文
func TestOpsDesignerModelLifecycle(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 建模（空骨架）
	created, err := env.service.CreateProcessModel(ctx, "bizA", "差旅模型", "travelProc", "")
	if err != nil {
		t.Fatalf("建模失败: %v", err)
	}
	createdData := created.Data.(CreatedProcessModel)
	if createdData.EngineModelID == "" || createdData.BusinessID == "" {
		t.Fatalf("建模结果缺引擎/业务绑定: %+v", createdData)
	}
	// 代号唯一
	if _, err := env.service.CreateProcessModel(ctx, "bizA", "重复", "travelProc", ""); err == nil {
		t.Fatalf("重复代号应报错")
	}
	// 保存 XML 源（process id 与模型代号一致，部署后按 Key 可解析）
	if _, err := env.service.SaveModelXML(ctx, createdData.BusinessID, travelFixture); err != nil {
		t.Fatalf("保存 XML 失败: %v", err)
	}
	// 读源一致
	read, err := env.service.GetModelXML(ctx, createdData.BusinessID)
	if err != nil {
		t.Fatalf("读取模型源失败: %v", err)
	}
	if got := read.Data.(map[string]any)["xml"].(string); len(got) == 0 {
		t.Fatalf("模型源不应为空")
	}
	// 部署链路（XML 已就绪）
	deployed, err := env.service.DeployModel(ctx, "bizA", createdData.BusinessID)
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	definitionID := deployed.Data.(map[string]any)["definitionId"].(string)
	// 历史定义 XML 可读
	his, err := env.service.GetDefinitionXML(ctx, definitionID)
	if err != nil {
		t.Fatalf("历史定义 XML 读取失败: %v", err)
	}
	if his.Data.(map[string]any)["definitionKey"].(string) != "travelProc" {
		t.Fatalf("历史定义 Key 不符: %+v", his.Data)
	}
	// 历史回填：新版本 XML 覆盖模型源（travelProc 的定义即 opsFixture 同构，内容一致亦成立）
	if _, err := env.service.UpdateModelFromDefinition(ctx, "bizA", definitionID); err != nil {
		t.Fatalf("历史回填失败: %v", err)
	}
	// 参数缺失拒绝
	if _, err := env.service.GetModelXML(ctx, ""); err == nil {
		t.Fatalf("缺 modelId 应报错")
	}
	if _, err := env.service.GetDefinitionXML(ctx, ""); err == nil {
		t.Fatalf("缺 definitionId 应报错")
	}
}

// TestOpsCopyProcessAndStepDetail 复制流程与模型步骤联查、步骤详情。
// @param t 测试上下文
func TestOpsCopyProcessAndStepDetail(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 源模型（绑定引擎模型 + XML）
	source, err := env.service.CreateProcessModel(ctx, "bizA", "源模型", "srcProc", opsFixture)
	if err != nil {
		t.Fatalf("建源模型失败: %v", err)
	}
	sourceData := source.Data.(CreatedProcessModel)
	// 目标模型（空骨架，未绑引擎模型）
	target, err := env.service.CreateProcessModel(ctx, "bizA", "目标模型", "dstProc", "")
	if err != nil {
		t.Fatalf("建目标模型失败: %v", err)
	}
	targetData := target.Data.(CreatedProcessModel)

	// 复制：目标自动补引擎模型绑定
	copied, err := env.service.CopyProcess(ctx, sourceData.BusinessID, []string{targetData.BusinessID, "", sourceData.BusinessID})
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}
	if copied.Data.(map[string]any)["copied"].(int) != 1 {
		t.Fatalf("复制数不符（空 Id 与自复制应跳过）: %+v", copied.Data)
	}
	// 目标源与源一致（非空即认定复制成功，内容一致性经 GetModelXML 比对片段）
	dst, err := env.service.GetModelXML(ctx, targetData.BusinessID)
	if err != nil {
		t.Fatalf("读取目标源失败: %v", err)
	}
	src, err := env.service.GetModelXML(ctx, sourceData.BusinessID)
	if err != nil {
		t.Fatalf("读取源失败: %v", err)
	}
	if dst.Data.(map[string]any)["xml"] != src.Data.(map[string]any)["xml"] {
		t.Fatalf("复制后 XML 应一致")
	}

	// 步骤详情：新增后按 Id 查
	stepR, err := env.service.AddStepDesign(ctx, &StepDesign{
		BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "申请", StepCode: "apply", ModelID: sourceData.BusinessID, Sort: 1,
	})
	if err != nil {
		t.Fatalf("新增步骤失败: %v", err)
	}
	step := stepR.Data.(*StepDesign)
	detail, err := env.service.GetStepDesign(ctx, step.ID)
	if err != nil || detail.Data.(*StepDesign).StepCode != "apply" {
		t.Fatalf("步骤详情不符: %+v err=%v", detail.Data, err)
	}
	// 联查：模型列表附步骤
	page, err := env.service.GetModelAndStepPage(ctx, "bizA")
	if err != nil {
		t.Fatalf("联查失败: %v", err)
	}
	if len(page.Data.([]map[string]any)) < 2 {
		t.Fatalf("联查应含源与目标两行: %+v", page.Data)
	}
}

// TestOpsDelegateTaskAndWeightDelete 引擎委派与权重清空。
// @param t 测试上下文
func TestOpsDelegateTaskAndWeightDelete(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	_, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-DG")

	// 权重：保存 → 清空 → 查询为空
	if _, err := env.service.SaveCountersignWeights(ctx, "opsProc", "apply", "bizA", []CountersignWeight{{IdentityID: "u1", AgreeWeight: 1}}); err != nil {
		t.Fatalf("保存权重失败: %v", err)
	}
	if _, err := env.service.DeleteCountersignWeights(ctx, "opsProc", "apply", "bizA"); err != nil {
		t.Fatalf("清空权重失败: %v", err)
	}
	weights, err := env.service.ListCountersignWeights(ctx, "opsProc", "apply", "bizA")
	if err != nil || len(weights.Data.([]CountersignWeight)) != 0 {
		t.Fatalf("清空后权重应为空: %+v err=%v", weights.Data, err)
	}
	// 引擎委派：u1 委派 u2
	if _, err := env.service.DelegateTask(ctx, applyTaskID, "u2"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	// 归还链路（复用 ResolveTask）后可由 u1 正常办结
	if _, err := env.service.ResolveTask(ctx, applyTaskID, "u1"); err != nil {
		t.Fatalf("委派处理失败: %v", err)
	}
}
