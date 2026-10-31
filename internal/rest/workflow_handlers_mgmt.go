package rest

// 本文件承载 /workflow/** 管理类端点 handler：
// /model/**、/category/**（一表两页同语义）、/step/**、/hisstep/**、/template/**、
// /delegate/**、/remind/**、/exception/**。
// 响应协议与公共辅助见 workflow_handlers.go。

import (
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/workflow"
)

// ─── /model/** 与 /category/**：模型/分类管理（workflow_model 一表两页）──

// handleWorkflowModelAdd 新增模型/分类（POST /workflow/model/add 与 /category/add）。
// 参数：body/query modelCode（必填）、modelName、parentId、tenantId 等（ModelCategory 同构字段）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowModelAdd(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	m := &workflow.ModelCategory{}
	if err := params.decode(m); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量，租户回退请求租户
	m.ModelCode = firstNonEmpty(params.get("modelCode"), m.ModelCode)
	m.ModelName = firstNonEmpty(params.get("modelName"), m.ModelName)
	m.ParentID = firstNonEmpty(params.get("parentId"), m.ParentID)
	m.TenantID = firstNonEmpty(params.get("tenantId"), m.TenantID, requestTenant(r))
	result, err := s.workflow.AddModelCategory(r.Context(), m)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowModelEdit 更新模型/分类（POST /workflow/model/edit 与 /category/edit）。
// 参数：body/query id（必填）、modelCode（必填）等（ModelCategory 同构字段）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowModelEdit(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	m := &workflow.ModelCategory{}
	if err := params.decode(m); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	m.ID = firstNonEmpty(params.get("id"), m.ID)
	m.ModelCode = firstNonEmpty(params.get("modelCode"), m.ModelCode)
	m.ModelName = firstNonEmpty(params.get("modelName"), m.ModelName)
	result, err := s.workflow.EditModelCategory(r.Context(), m)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowModelDelete 批量删除模型/分类（ANY /workflow/model/delete/{ids}、
// /category/delete/{ids} 与 /process/model/delete/{id}）。
// 参数：路径 id/ids（逗号分隔）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowModelDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	ids := splitWorkflowIDs(firstNonEmpty(r.PathValue("ids"), r.PathValue("id")))
	if len(ids) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 ids", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DeleteModelCategory(r.Context(), ids)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowModelList 查询租户下模型与分类列表（GET /workflow/model/list、/category/list
// 与 /process/model/list）。
// 参数：query tenantId（回退请求租户）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowModelList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.ListModelCategory(r.Context(), workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowModelTree 查询租户下模型/分类树（GET /workflow/model/tree/list）。
// 参数：query tenantId（回退请求租户）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowModelTree(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.GetModelCategoryTree(r.Context(), workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// ─── /step/**：设计时步骤配置 ───────────────────────────────────

// handleWorkflowStepAdd 新增设计时步骤（POST /workflow/step/add）。
// 参数：body/query modelId/stepCode（必填）、name、tenantId 等（StepDesign 同构字段）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowStepAdd(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	d := &workflow.StepDesign{}
	if err := params.decode(d); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量，租户回退请求租户
	d.ModelID = firstNonEmpty(params.get("modelId"), d.ModelID)
	d.StepCode = firstNonEmpty(params.get("stepCode"), d.StepCode)
	d.Name = firstNonEmpty(params.get("name"), d.Name)
	d.TenantID = firstNonEmpty(params.get("tenantId"), d.TenantID, requestTenant(r))
	result, err := s.workflow.AddStepDesign(r.Context(), d)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowStepEdit 更新设计时步骤（POST /workflow/step/edit）。
// 参数：body/query id（必填）、modelId/stepCode/name 等。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowStepEdit(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	d := &workflow.StepDesign{}
	if err := params.decode(d); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	d.ID = firstNonEmpty(params.get("id"), d.ID)
	d.ModelID = firstNonEmpty(params.get("modelId"), d.ModelID)
	d.StepCode = firstNonEmpty(params.get("stepCode"), d.StepCode)
	d.Name = firstNonEmpty(params.get("name"), d.Name)
	result, err := s.workflow.EditStepDesign(r.Context(), d)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowStepDelete 批量删除设计时步骤（ANY /workflow/step/delete/{ids}）。
// 参数：路径 ids（逗号分隔）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowStepDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	ids := splitWorkflowIDs(firstNonEmpty(r.PathValue("ids"), r.PathValue("id")))
	if len(ids) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 ids", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DeleteStepDesign(r.Context(), ids)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowStepList 按模型查询设计时步骤列表（GET /workflow/step/list）。
// 参数：query modelId（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowStepList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	modelID, err := params.getRequired("modelId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ListStepDesignByModel(r.Context(), modelID)
	writeWorkflowResult(w, result, err)
}

// ─── /hisstep/**：部署步骤快照（只读）────────────────────────────

// handleWorkflowHisStepList 查询部署下的步骤快照（GET /workflow/hisstep/list）。
// 参数：query deploymentId（必填）。增删改维持 501（快照只读）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowHisStepList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	deploymentID, err := params.getRequired("deploymentId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ListHisSteps(r.Context(), deploymentID)
	writeWorkflowResult(w, result, err)
}

// ─── /template/**：设计器模板 ───────────────────────────────────

// handleWorkflowTemplateAdd 新增设计器模板（POST /workflow/template/add）。
// 参数：body/query name（必填）、tenantId 等（TemplateDef 同构字段）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTemplateAdd(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	t := &workflow.TemplateDef{}
	if err := params.decode(t); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量，租户回退请求租户
	t.Name = firstNonEmpty(params.get("name"), t.Name)
	t.TenantID = firstNonEmpty(params.get("tenantId"), t.TenantID, requestTenant(r))
	result, err := s.workflow.AddTemplateDef(r.Context(), t)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowTemplateEdit 更新设计器模板（POST /workflow/template/edit）。
// 参数：body/query id（必填）、name 等。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTemplateEdit(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	t := &workflow.TemplateDef{}
	if err := params.decode(t); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	t.ID = firstNonEmpty(params.get("id"), t.ID)
	t.Name = firstNonEmpty(params.get("name"), t.Name)
	result, err := s.workflow.EditTemplateDef(r.Context(), t)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowTemplateDelete 批量删除设计器模板（ANY /workflow/template/delete/{ids}）。
// 参数：路径 ids（逗号分隔）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTemplateDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	ids := splitWorkflowIDs(firstNonEmpty(r.PathValue("ids"), r.PathValue("id")))
	if len(ids) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 ids", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DeleteTemplateDef(r.Context(), ids)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowTemplateList 查询租户下设计器模板列表（GET /workflow/template/list）。
// 参数：query tenantId（回退请求租户）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTemplateList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.ListTemplateDefs(r.Context(), workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// ─── /delegate/**：委托规则 CRUD ────────────────────────────────

// handleWorkflowDelegateAdd 新增委托规则（POST /workflow/delegate/add）。
// 参数：body DelegateRule 同构 JSON（ownerId/vaildFrom/vaildTo/consigners 必填，服务端校验），
// query ownerId/tenantId 标量兜底，租户回退请求租户。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDelegateAdd(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	rule := &workflow.DelegateRule{}
	if err := params.decode(rule); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量（body 已解码时以 query 覆盖空缺）
	rule.OwnerID = firstNonEmpty(params.get("ownerId"), rule.OwnerID)
	rule.OwnerName = firstNonEmpty(params.get("ownerName"), rule.OwnerName)
	rule.ConsignType = firstNonEmpty(params.get("consignType"), rule.ConsignType)
	rule.SourceCode = firstNonEmpty(params.get("tenantId"), rule.SourceCode, requestTenant(r))
	result, err := s.workflow.AddDelegateTask(r.Context(), rule)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDelegateEdit 更新委托规则（POST /workflow/delegate/edit）。
// 参数：body DelegateRule 同构 JSON（id 必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDelegateEdit(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	rule := &workflow.DelegateRule{}
	if err := params.decode(rule); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	rule.ID = firstNonEmpty(params.get("id"), rule.ID)
	result, err := s.workflow.EditDelegateTask(r.Context(), rule)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDelegateDelete 删除委托规则（ANY /workflow/delegate/delete/{ids}）。
// 参数：路径 ids（协议为单个规则 Id，整段透传）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDelegateDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	id := firstNonEmpty(r.PathValue("ids"), r.PathValue("id"))
	if id == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 ids", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DeleteDelegateTask(r.Context(), id)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDelegateList 分页查询委托规则列表（GET /workflow/delegate/list）。
// 参数：query ownerId（必填）、tenantId（回退请求租户）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDelegateList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	ownerID, err := params.getRequired("ownerId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ListDelegateTask(r.Context(), ownerID, workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// ─── /remind/**：催办配置 ───────────────────────────────────────

// handleWorkflowRemindPageList 分页查询催办配置（GET /workflow/remind/getPageList）。
// 参数：query modelId（可空表示全部）、start/size。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowRemindPageList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.GetRemindPageList(r.Context(), params.get("modelId"), params.pageQuery())
	writeWorkflowResult(w, result, err)
}

// handleWorkflowRemindGet 查询单条催办配置（GET /workflow/remind/getWorkflowRemind/{id}）。
// 参数：路径 id（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowRemindGet(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	id := r.PathValue("id")
	if id == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 id", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.GetRemind(r.Context(), id)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowRemindAdd 新增催办配置（POST /workflow/remind/add）。
// 参数：body {"remind":{...},"targets":[...]}（RemindSaveRequest 同构；remind.pid 必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowRemindAdd(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	req := workflow.RemindSaveRequest{}
	if err := params.decode(&req); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// 主表缺失或模型 Id 为空直接判参数错误（服务端同样校验，此处给出更准的协议错误）
	if req.Remind == nil || req.Remind.PID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少催办配置或模型 Id（remind.pid）", api.ErrBadRequest))
		return
	}
	req.Remind.TenantID = firstNonEmpty(params.get("tenantId"), req.Remind.TenantID, requestTenant(r))
	result, err := s.workflow.AddRemind(r.Context(), req)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowRemindEdit 更新催办配置（POST /workflow/remind/edit）。
// 参数：body {"remind":{...},"targets":[...]}（remind.id 必填，服务端校验存在性）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowRemindEdit(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	req := workflow.RemindSaveRequest{}
	if err := params.decode(&req); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	if req.Remind == nil || req.Remind.ID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少催办配置或配置 Id（remind.id）", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.EditRemind(r.Context(), req)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowRemindDelete 删除催办配置（DELETE /workflow/remind/delete/{id}）。
// 参数：路径 id（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowRemindDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	id := r.PathValue("id")
	if id == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 id", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DeleteRemind(r.Context(), id)
	writeWorkflowResult(w, result, err)
}

// ─── /exception/**：任务异常 ────────────────────────────────────

// handleWorkflowExceptionList 按实例查询未处理任务异常（GET /workflow/exception/list）。
// 参数：query processInstanceId（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowExceptionList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID, err := params.getRequired("processInstanceId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ListTaskExceptions(r.Context(), instanceID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowExceptionDelete 批量标记异常处理状态（ANY /workflow/exception/delete/{ids}）。
// 参数：路径 ids（逗号分隔，必填）、query/body handled（缺省 true 置为已处理）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowExceptionDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	ids := splitWorkflowIDs(firstNonEmpty(r.PathValue("ids"), r.PathValue("id")))
	if len(ids) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 ids", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.HandleTaskExceptions(r.Context(), ids, params.getBool("handled", true))
	writeWorkflowResult(w, result, err)
}
