package workflow

// 本文件承载 P5 集成门面方法：模型/分类管理、设计时步骤、设计器模板、部署快照（部署链路）、
// 审阅影子任务、流程图高亮数据。装配在 facade_ops.go 的 assembleCapabilities。

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
)

// requireMgmt 断言 P5 管理子服务已装配（nil 引擎协议场景快速失败）。
// @param comp 组件聚合
// @return 错误（未装配时返回 WORKFLOW-0601）
func requireMgmt(comp *components) error {
	if comp == nil || comp.modelMgmt == nil {
		return api.NewError("WORKFLOW-0601", "workflow 管理子服务未装配（缺少引擎）", nil)
	}
	return nil
}

// ─── P5.A 模型/分类管理门面 ─────────────────────────────────

// AddModelCategory 新增模型/分类（一表两用：模型与分类）。
// @param ctx 上下文
// @param m 实体（ModelName/ModelCode/租户必填）
// @return 统一返回（data 为落库实体）
// @return 错误
func (s *Service) AddModelCategory(ctx context.Context, m *ModelCategory) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	saved, err := s.comp.modelMgmt.Add(ctx, m)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", saved), nil
}

// EditModelCategory 更新模型/分类。
// @param ctx 上下文
// @param m 实体（ID 必填）
// @return 统一返回（data 为更新后实体）
// @return 错误
func (s *Service) EditModelCategory(ctx context.Context, m *ModelCategory) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	saved, err := s.comp.modelMgmt.Edit(ctx, m)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("更新成功", saved), nil
}

// DeleteModelCategory 批量删除模型/分类（物理删除，启停经 Edit 调整）。
// @param ctx 上下文
// @param ids 主键集合
// @return 统一返回
// @return 错误
func (s *Service) DeleteModelCategory(ctx context.Context, ids []string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.modelMgmt.Delete(ctx, ids...); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// ListModelCategory 查询租户下模型与分类列表（sort 升序）。
// @param ctx 上下文
// @param tenantID 租户
// @return 统一返回（data 为列表）
// @return 错误
func (s *Service) ListModelCategory(ctx context.Context, tenantID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	list, err := s.comp.modelMgmt.List(ctx, tenantID)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// GetModelCategoryTree 查询租户下模型/分类树（分类层级投影）。
// @param ctx 上下文
// @param tenantID 租户
// @return 统一返回（data 为树节点列表）
// @return 错误
func (s *Service) GetModelCategoryTree(ctx context.Context, tenantID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	tree, err := s.comp.modelMgmt.Tree(ctx, tenantID)
	if err != nil {
		return R{}, err
	}
	return ROk(tree), nil
}

// GetModelCategoryByID 按主键查询模型/分类。
// @param ctx 上下文
// @param id 主键
// @return 统一返回（data 为实体，未命中 data 为 null）
// @return 错误
func (s *Service) GetModelCategoryByID(ctx context.Context, id string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	m, found, err := s.comp.modelMgmt.GetByID(ctx, id)
	if err != nil {
		return R{}, err
	}
	if !found {
		return ROk(nil), nil
	}
	return ROk(m), nil
}

// GetModelCategoryByCode 按租户内代号查询模型/分类。
// @param ctx 上下文
// @param tenantID 租户
// @param modelCode 模型代号（=定义 Key 或分类代号）
// @return 统一返回（data 为实体，未命中 data 为 null）
// @return 错误
func (s *Service) GetModelCategoryByCode(ctx context.Context, tenantID, modelCode string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	m, found, err := s.comp.modelMgmt.GetByCode(ctx, tenantID, modelCode)
	if err != nil {
		return R{}, err
	}
	if !found {
		return ROk(nil), nil
	}
	return ROk(m), nil
}

// ─── P5.A 设计时步骤/模板管理门面 ───────────────────────────

// AddStepDesign 新增设计时步骤配置。
// @param ctx 上下文
// @param d 实体（Name/StepCode/ModelID 必填）
// @return 统一返回（data 为落库实体）
// @return 错误
func (s *Service) AddStepDesign(ctx context.Context, d *StepDesign) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	saved, err := s.comp.stepDesign.Add(ctx, d)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", saved), nil
}

// EditStepDesign 更新设计时步骤配置。
// @param ctx 上下文
// @param d 实体（ID 必填）
// @return 统一返回（data 为更新后实体）
// @return 错误
func (s *Service) EditStepDesign(ctx context.Context, d *StepDesign) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	saved, err := s.comp.stepDesign.Edit(ctx, d)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("更新成功", saved), nil
}

// DeleteStepDesign 批量删除设计时步骤配置。
// @param ctx 上下文
// @param ids 主键集合
// @return 统一返回
// @return 错误
func (s *Service) DeleteStepDesign(ctx context.Context, ids []string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.stepDesign.Delete(ctx, ids...); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// ListStepDesignByModel 按模型查询设计时步骤列表。
// @param ctx 上下文
// @param modelID 模型主键
// @return 统一返回（data 为步骤列表）
// @return 错误
func (s *Service) ListStepDesignByModel(ctx context.Context, modelID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	list, err := s.comp.stepDesign.ListByModel(ctx, modelID)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// AddTemplateDef 新增设计器模板。
// @param ctx 上下文
// @param t 实体（Name 必填）
// @return 统一返回（data 为落库实体）
// @return 错误
func (s *Service) AddTemplateDef(ctx context.Context, t *TemplateDef) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	saved, err := s.comp.template.Add(ctx, t)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", saved), nil
}

// EditTemplateDef 更新设计器模板。
// @param ctx 上下文
// @param t 实体（ID 必填）
// @return 统一返回（data 为更新后实体）
// @return 错误
func (s *Service) EditTemplateDef(ctx context.Context, t *TemplateDef) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	saved, err := s.comp.template.Edit(ctx, t)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("更新成功", saved), nil
}

// DeleteTemplateDef 批量删除设计器模板。
// @param ctx 上下文
// @param ids 主键集合
// @return 统一返回
// @return 错误
func (s *Service) DeleteTemplateDef(ctx context.Context, ids []string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.template.Delete(ctx, ids...); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// ListTemplateDefs 查询租户下设计器模板列表。
// @param ctx 上下文
// @param tenantID 租户
// @return 统一返回（data 为模板列表）
// @return 错误
func (s *Service) ListTemplateDefs(ctx context.Context, tenantID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	list, err := s.comp.template.List(ctx, tenantID)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// ─── P5.A 部署链路（部署 + 步骤快照）────────────────────────

// DeployModel 部署模型并生成步骤快照：引擎部署绑定的 BPMN XML →
// 设计时步骤转运行期 his_step 快照 → 回写最近部署时间。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param modelID 模型主键（须已绑定引擎模型 EngineModelID）
// @return 统一返回（data 含 deploymentId/definitionId/snapshotCount）
// @return 错误
func (s *Service) DeployModel(ctx context.Context, tenantID, modelID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	if s.comp.snapshot == nil {
		return R{}, api.NewError("WORKFLOW-0601", "部署快照服务未装配", nil)
	}
	m, found, err := s.comp.modelMgmt.GetByID(ctx, modelID)
	if err != nil {
		return R{}, err
	}
	if !found {
		return R{}, api.NewError("WORKFLOW-1404", "流程模型不存在", api.ErrNotFound)
	}
	if m.EngineModelID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "模型未绑定引擎侧模型，无法部署", api.ErrBadRequest)
	}
	// 引擎模型编辑源 → 引擎部署
	source, err := s.engine.RepositoryService().GetModelSource(ctx, m.EngineModelID)
	if err != nil {
		return R{}, err
	}
	deployment, err := s.engine.RepositoryService().Deploy(ctx, api.DeploymentRequest{
		Name:     m.ModelName,
		TenantID: tenantID,
		Resources: map[string][]byte{
			m.ModelCode + ".bpmn20.xml": source,
		},
	})
	if err != nil {
		return R{}, err
	}
	// 最新定义定位（快照按部署+节点落行）
	definition, err := s.engine.RepositoryService().GetProcessDefinitionByKey(ctx, m.ModelCode)
	if err != nil {
		return R{}, err
	}
	// 设计时步骤 → his_step 快照（幂等：同部署整表替换 + 回写最近部署时间）
	rules, err := s.comp.snapshot.SnapshotForDeploy(ctx, tenantID, m.ModelCode, deployment.ID, definition.ID)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("部署成功", map[string]any{
		"deploymentId":  deployment.ID,
		"definitionId":  definition.ID,
		"snapshotCount": len(rules),
	}), nil
}

// ListHisSteps 查询部署下的步骤快照（运行期规则字典只读）。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @return 统一返回（data 为快照规则列表）
// @return 错误
func (s *Service) ListHisSteps(ctx context.Context, deploymentID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	rules, err := s.comp.stepsStore.ListByDeployment(ctx, deploymentID)
	if err != nil {
		return R{}, err
	}
	return ROk(rules), nil
}

// ─── P5.B 审阅影子任务门面 ─────────────────────────────────

// SyncReviewForTask 为任务同步审阅影子任务（按 act_extend 审阅人绑定，幂等）。
// @param ctx 上下文
// @param task 引擎任务
// @param senderID 发送人身份 Id
// @param senderName 发送人姓名
// @return 统一返回（data 为生成的影子任务列表）
// @return 错误
func (s *Service) SyncReviewForTask(ctx context.Context, task *api.Task, senderID, senderName string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	created, err := s.comp.review.SyncForTask(ctx, task, senderID, senderName)
	if err != nil {
		return R{}, err
	}
	return ROk(created), nil
}

// CompleteReviewTask 办结审阅影子任务（回写意见与审阅时间）。
// @param ctx 上下文
// @param reviewTaskID 影子任务 ID
// @param opinion 审阅意见
// @param operator 操作人身份 Id
// @return 统一返回（data 为更新后影子任务）
// @return 错误
func (s *Service) CompleteReviewTask(ctx context.Context, reviewTaskID, opinion, operator string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	updated, err := s.comp.review.Complete(ctx, reviewTaskID, opinion, operator)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("审阅成功", updated), nil
}

// GetReviewTask 查询单条审阅影子任务。
// @param ctx 上下文
// @param reviewTaskID 影子任务 ID
// @return 统一返回（data 为影子任务，未命中 data 为 null）
// @return 错误
func (s *Service) GetReviewTask(ctx context.Context, reviewTaskID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	task, found, err := s.comp.review.Get(ctx, reviewTaskID)
	if err != nil {
		return R{}, err
	}
	if !found {
		return ROk(nil), nil
	}
	return ROk(task), nil
}

// ListReviewTasksByReviewer 按审阅人分页查询影子任务。
// @param ctx 上下文
// @param reviewer 审阅人身份 Id
// @param tenantID 租户
// @param query 分页参数
// @return 统一返回（data 为分页结果）
// @return 错误
func (s *Service) ListReviewTasksByReviewer(ctx context.Context, reviewer, tenantID string, query PageQuery) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	page, err := s.comp.review.ListByReviewer(ctx, reviewer, tenantID, query)
	if err != nil {
		return R{}, err
	}
	return ROk(page), nil
}

// ListReviewTasksByInstance 按实例查询审阅影子任务。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 统一返回（data 为影子任务列表）
// @return 错误
func (s *Service) ListReviewTasksByInstance(ctx context.Context, processInstanceID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	list, err := s.comp.review.ListByInstance(ctx, processInstanceID)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// CountUnreadReviews 统计审阅人未读影子任务数。
// @param ctx 上下文
// @param reviewer 审阅人身份 Id
// @param tenantID 租户
// @return 统一返回（data 为未读数）
// @return 错误
func (s *Service) CountUnreadReviews(ctx context.Context, reviewer, tenantID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	count, err := s.comp.review.CountUnreadByReviewer(ctx, reviewer, tenantID)
	if err != nil {
		return R{}, err
	}
	return ROk(count), nil
}

// ─── P5.B 流程图高亮门面 ────────────────────────────────────

// GetFlowHighlight 查询流程图高亮数据（已走节点/在途节点/已走连线/待办节点）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 统一返回（data 为高亮数据）
// @return 错误
func (s *Service) GetFlowHighlight(ctx context.Context, processInstanceID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	data, err := s.comp.highlight.FlowHighlight(ctx, processInstanceID)
	if err != nil {
		return R{}, err
	}
	return ROk(data), nil
}

// GetFlowHighlightByTask 按任务查询其所在实例的流程图高亮数据。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回（data 为高亮数据）
// @return 错误
func (s *Service) GetFlowHighlightByTask(ctx context.Context, taskID string) (R, error) {
	if err := requireMgmt(s.comp); err != nil {
		return R{}, err
	}
	data, err := s.comp.highlight.FlowHighlightByTask(ctx, taskID)
	if err != nil {
		return R{}, err
	}
	return ROk(data), nil
}
