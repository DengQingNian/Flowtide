package workflow

// facade_mgmt_test.go 覆盖 P5 集成门面：模型/分类 CRUD、设计时步骤、模板、部署链路（部署+快照）、
// 审阅影子任务、流程图高亮。复用 opsFixtureEnv 的引擎与身份预置。

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestOpsModelCategoryFacade 模型/分类管理门面 CRUD 与树投影。
// @param t 测试上下文
func TestOpsModelCategoryFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 新增根分类与子模型
	rootR, err := env.service.AddModelCategory(ctx, &ModelCategory{ModelName: "人事分类", ModelCode: "cat-hr", TenantID: "bizA"})
	if err != nil {
		t.Fatalf("新增分类失败: %v", err)
	}
	root := rootR.Data.(*ModelCategory)
	modelR, err := env.service.AddModelCategory(ctx, &ModelCategory{
		ModelName: "请假模型", ModelCode: "opsProc", TenantID: "bizA", ParentID: root.ID,
	})
	if err != nil {
		t.Fatalf("新增模型失败: %v", err)
	}
	model := modelR.Data.(*ModelCategory)
	// 代号唯一
	if _, err := env.service.AddModelCategory(ctx, &ModelCategory{ModelName: "重复", ModelCode: "opsProc", TenantID: "bizA"}); err == nil {
		t.Fatalf("重复代号应报错")
	}
	// 列表与树
	listed, err := env.service.ListModelCategory(ctx, "bizA")
	if err != nil || len(listed.Data.([]*ModelCategory)) != 2 {
		t.Fatalf("列表不符: %+v err=%v", listed.Data, err)
	}
	tree, err := env.service.GetModelCategoryTree(ctx, "bizA")
	if err != nil {
		t.Fatalf("树查询失败: %v", err)
	}
	nodes := tree.Data.([]*ModelTreeNode)
	if len(nodes) != 1 || len(nodes[0].Children) != 1 {
		t.Fatalf("树投影不符: %+v", nodes)
	}
	// 按主键/代号查询
	if _, err := env.service.GetModelCategoryByID(ctx, model.ID); err != nil {
		t.Fatalf("按 Id 查询失败: %v", err)
	}
	byCode, err := env.service.GetModelCategoryByCode(ctx, "bizA", "opsProc")
	if err != nil || byCode.Data == nil {
		t.Fatalf("按代号查询失败: %+v err=%v", byCode.Data, err)
	}
	// 更新与删除
	model.ModelName = "请假模型V2"
	if _, err := env.service.EditModelCategory(ctx, model); err != nil {
		t.Fatalf("更新模型失败: %v", err)
	}
	if _, err := env.service.DeleteModelCategory(ctx, []string{model.ID, root.ID}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
}

// TestOpsStepDesignAndTemplateFacade 设计时步骤与模板的门面 CRUD。
// @param t 测试上下文
func TestOpsStepDesignAndTemplateFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 步骤配置
	stepR, err := env.service.AddStepDesign(ctx, &StepDesign{
		BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "申请", StepCode: "apply", ModelID: "M-1", Sort: 1,
	})
	if err != nil {
		t.Fatalf("新增步骤失败: %v", err)
	}
	step := stepR.Data.(*StepDesign)
	step.Name = "申请（改）"
	if _, err := env.service.EditStepDesign(ctx, step); err != nil {
		t.Fatalf("更新步骤失败: %v", err)
	}
	listed, err := env.service.ListStepDesignByModel(ctx, "M-1")
	if err != nil || len(listed.Data.([]*StepDesign)) != 1 {
		t.Fatalf("步骤列表不符: %+v err=%v", listed.Data, err)
	}
	if _, err := env.service.DeleteStepDesign(ctx, []string{step.ID}); err != nil {
		t.Fatalf("删除步骤失败: %v", err)
	}

	// 模板
	tplR, err := env.service.AddTemplateDef(ctx, &TemplateDef{
		BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "默认模板",
	})
	if err != nil {
		t.Fatalf("新增模板失败: %v", err)
	}
	tpl := tplR.Data.(*TemplateDef)
	tpl.Name = "默认模板V2"
	if _, err := env.service.EditTemplateDef(ctx, tpl); err != nil {
		t.Fatalf("更新模板失败: %v", err)
	}
	tplList, err := env.service.ListTemplateDefs(ctx, "bizA")
	if err != nil || len(tplList.Data.([]*TemplateDef)) != 1 {
		t.Fatalf("模板列表不符: %+v err=%v", tplList.Data, err)
	}
	if _, err := env.service.DeleteTemplateDef(ctx, []string{tpl.ID}); err != nil {
		t.Fatalf("删除模板失败: %v", err)
	}
}

// TestOpsDeployModelFacade 部署链路门面：引擎模型绑定 → 部署 → 步骤快照 → 最近部署时间回写。
// @param t 测试上下文
func TestOpsDeployModelFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")

	// 引擎侧建模型并写入编辑源（与 opsProc 同图）
	model, err := env.engine.RepositoryService().CreateModel(ctx, api.CreateModelRequest{
		Name: "opsProc", Key: "opsProc", TenantID: "bizA",
	})
	if err != nil {
		t.Fatalf("创建引擎模型失败: %v", err)
	}
	if err := env.engine.RepositoryService().SetModelSource(ctx, model.ID, []byte(opsFixture)); err != nil {
		t.Fatalf("写入模型源失败: %v", err)
	}
	// 业务侧模型绑定引擎模型
	savedR, err := env.service.AddModelCategory(ctx, &ModelCategory{
		ModelName: "请假模型", ModelCode: "opsProc", TenantID: "bizA", EngineModelID: model.ID,
	})
	if err != nil {
		t.Fatalf("新增业务模型失败: %v", err)
	}
	saved := savedR.Data.(*ModelCategory)
	// 设计时步骤（apply/handle/approve 各一条，含规则字段）
	for _, d := range []*StepDesign{
		{BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "申请", StepCode: "apply", ModelID: saved.ID, Sort: 1},
		{BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "办理", StepCode: "handle", ModelID: saved.ID, Sort: 2, AllowReturn: "1", ReturnApprovalStep: "3"},
		{BaseColumns: BaseColumns{TenantID: "bizA"}, Name: "批准", StepCode: "approve", ModelID: saved.ID, Sort: 3},
	} {
		if _, err := env.service.AddStepDesign(ctx, d); err != nil {
			t.Fatalf("新增设计步骤 %s 失败: %v", d.StepCode, err)
		}
	}
	// 部署
	deployed, err := env.service.DeployModel(ctx, "bizA", saved.ID)
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	data := deployed.Data.(map[string]any)
	if data["deploymentId"] == "" || data["definitionId"] == "" {
		t.Fatalf("部署结果缺部署/定义 Id: %+v", data)
	}
	// 快照可查：3 条规则且排序正确
	his, err := env.service.ListHisSteps(ctx, data["deploymentId"].(string))
	if err != nil {
		t.Fatalf("快照查询失败: %v", err)
	}
	rules := his.Data.([]*StepRule)
	if len(rules) != 3 || rules[0].StepCode != "apply" || rules[1].ReturnApprovalStep != "3" {
		t.Fatalf("快照规则不符: %+v", rules)
	}
	// 最近部署时间已回写
	refreshed, err := env.service.GetModelCategoryByID(ctx, saved.ID)
	if err != nil {
		t.Fatalf("回查模型失败: %v", err)
	}
	if refreshed.Data.(*ModelCategory).LastDeployTime == nil {
		t.Fatalf("最近部署时间未回写")
	}
}

// TestOpsReviewFacade 审阅影子任务门面：绑定 → 同步 → 办结 → 查询/计数。
// @param t 测试上下文
func TestOpsReviewFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-RV")

	// 审阅人绑定：apply 节点绑定 u2（person）
	task, err := env.engine.TaskService().GetTask(ctx, applyTaskID)
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if err := env.service.comp.extendsStore.Save(ctx, &ActExtend{
		BaseColumns: BaseColumns{TenantID: "bizA"}, ActivityID: "apply", ActivityName: "申请",
		ProcessDefinitionKey: "opsProc", OwnerType: ActOwnerTypePerson, OwnerValue: "u2",
	}); err != nil {
		t.Fatalf("保存审阅绑定失败: %v", err)
	}
	// 同步影子任务
	synced, err := env.service.SyncReviewForTask(ctx, &task, "u1", "张三")
	if err != nil {
		t.Fatalf("同步影子任务失败: %v", err)
	}
	created := synced.Data.([]*ReviewTask)
	if len(created) != 1 || created[0].Reviewer != "u2" {
		t.Fatalf("影子任务生成不符: %+v", created)
	}
	// 幂等：重复同步不新增
	synced2, err := env.service.SyncReviewForTask(ctx, &task, "u1", "张三")
	if err != nil || len(synced2.Data.([]*ReviewTask)) != 0 {
		t.Fatalf("重复同步应幂等: %+v err=%v", synced2.Data, err)
	}
	// 未读计数
	count, err := env.service.CountUnreadReviews(ctx, "u2", "bizA")
	if err != nil || count.Data.(int64) != 1 {
		t.Fatalf("未读计数不符: %+v err=%v", count.Data, err)
	}
	// 办结
	done, err := env.service.CompleteReviewTask(ctx, created[0].ID, "已阅", "u2")
	if err != nil {
		t.Fatalf("办结影子任务失败: %v", err)
	}
	if done.Data.(*ReviewTask).Opinion != "已阅" {
		t.Fatalf("意见未回写: %+v", done.Data)
	}
	// 列表
	page, err := env.service.ListReviewTasksByReviewer(ctx, "u2", "bizA", PageQuery{})
	if err != nil {
		t.Fatalf("按审阅人查询失败: %v", err)
	}
	if page.Data.(PageResult[*ReviewTask]).Total != 1 {
		t.Fatalf("分页总数不符: %+v", page.Data)
	}
	byInst, err := env.service.ListReviewTasksByInstance(ctx, instanceID)
	if err != nil || len(byInst.Data.([]*ReviewTask)) != 1 {
		t.Fatalf("按实例查询不符: %+v err=%v", byInst.Data, err)
	}
}

// TestOpsHighlightFacade 流程图高亮门面：走两步后断言高亮集合与流转一致。
// @param t 测试上下文
func TestOpsHighlightFacade(t *testing.T) {
	env := newOpsService(t)
	ctx := tenant.With(context.Background(), "bizA")
	instanceID, applyTaskID := env.start(t, "opsProc", "BIZ-OPS-HL")

	// 走一步：apply → handle
	if _, err := env.service.Complete(ctx, &CompleteTaskVo{
		BaseProcessVo: BaseProcessVo{TaskID: applyTaskID, UserCode: "u1", Message: "同意", ProcessInstanceID: instanceID, Type: CommentTypeSP},
	}); err != nil {
		t.Fatalf("办结 apply 失败: %v", err)
	}
	handleTaskID := env.currentTask(t, instanceID)

	// 按任务查高亮：apply 已走、handle 在途/待办
	hl, err := env.service.GetFlowHighlightByTask(ctx, handleTaskID)
	if err != nil {
		t.Fatalf("高亮查询失败: %v", err)
	}
	data := hl.Data.(*HighlightData)
	assertContains(t, "已走节点", data.CompletedActivityIDs, "apply")
	assertContains(t, "待办节点", data.TodoActivityIDs, "handle")

	// 按实例查高亮一致
	hl2, err := env.service.GetFlowHighlight(ctx, instanceID)
	if err != nil {
		t.Fatalf("实例高亮查询失败: %v", err)
	}
	if hl2.Data.(*HighlightData).CompletedActivityIDs[0] != "apply" {
		t.Fatalf("实例高亮已走节点不符: %+v", hl2.Data)
	}
}

// assertContains 断言字符串集合包含目标值。
// @param t 测试上下文
// @param name 集合名称（错误信息用）
// @param got 实际集合
// @param want 期望包含值
func assertContains(t *testing.T, name string, got []string, want string) {
	t.Helper()
	for _, item := range got {
		if item == want {
			return
		}
	}
	t.Fatalf("%s 应包含 %s: %v", name, want, got)
}

// 编译期防呆：确保时间字段在测试窗口内可比（引用 time 包避免误删）。
var _ = time.Now
