package rest

// 本文件承载 /workflow/** 门面收尾能力端点（facade_extra.go 对接）：
// 审阅人绑定（/extend）、会签权重（/weight）、签收与委派处理（claim/resolve）、
// 批量挂起激活（suspend|activate processes）、实例分页（instance/all/list）、
// 身份树（identity/tree）与操作按钮清单（operationButtonList）。
// 响应协议与公共辅助见 workflow_handlers.go。

import (
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/workflow"
)

// workflowWeightSavePayload 是 /weight/save 请求体（configs 为节点权重配置列表）。
type workflowWeightSavePayload struct {
	ProcessDefinitionKey string                       `json:"processDefinitionKey"` // 流程定义 Key
	ActivityID           string                       `json:"activityId"`           // 节点 ID
	TenantID             string                       `json:"tenantId"`             // 租户（空不隔离）
	Configs              []workflow.CountersignWeight `json:"configs"`              // 权重配置列表
}

// ─── /extend/**：审阅人绑定（活动扩展）────────────────────────────

// handleWorkflowExtendSave 保存审阅人绑定（POST /workflow/extend/save）。
// 参数：body/query activityId/processDefinitionKey/ownerType/ownerValue（前两者必填）、
// activityName、stepId、tenantId。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowExtendSave(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	ext := &workflow.ActExtend{}
	if err := params.decode(ext); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量，租户回退请求租户
	ext.ActivityID = firstNonEmpty(params.get("activityId"), ext.ActivityID)
	ext.ActivityName = firstNonEmpty(params.get("activityName"), ext.ActivityName)
	ext.ProcessDefinitionKey = firstNonEmpty(params.get("processDefinitionKey"), ext.ProcessDefinitionKey)
	ext.OwnerType = firstNonEmpty(params.get("ownerType"), ext.OwnerType)
	ext.OwnerValue = firstNonEmpty(params.get("ownerValue"), ext.OwnerValue)
	ext.StepID = firstNonEmpty(params.get("stepId"), ext.StepID)
	ext.TenantID = firstNonEmpty(params.get("tenantId"), ext.TenantID, requestTenant(r))
	result, err := s.workflow.SaveActExtend(r.Context(), ext)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowExtendList 按定义 Key（可加节点）查询审阅人绑定
// （GET /workflow/extend/getActByProcessDefKeyAndActId）。
// 参数：query processDefinitionKey（必填）、activityId（可空表示全部）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowExtendList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	definitionKey, err := params.getRequired("processDefinitionKey")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ListActExtends(r.Context(), definitionKey, params.get("activityId"))
	writeWorkflowResult(w, result, err)
}

// ─── /weight/**：节点会签权重 ───────────────────────────────────

// handleWorkflowWeightSave 保存节点会签权重配置（POST /workflow/weight/save）。
// 参数：body/query processDefinitionKey/activityId（必填）、tenantId、
// body configs[{identityId,agreeWeight,rejectWeight}]。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowWeightSave(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	payload := &workflowWeightSavePayload{}
	if err := params.decode(payload); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐定位键（configs 数组仅 body 表达）
	payload.ProcessDefinitionKey = firstNonEmpty(params.get("processDefinitionKey"), payload.ProcessDefinitionKey)
	payload.ActivityID = firstNonEmpty(params.get("activityId"), payload.ActivityID)
	payload.TenantID = firstNonEmpty(params.get("tenantId"), payload.TenantID, requestTenant(r))
	result, err := s.workflow.SaveCountersignWeights(r.Context(), payload.ProcessDefinitionKey, payload.ActivityID, payload.TenantID, payload.Configs)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowWeightList 查询节点会签权重配置
// （GET /workflow/weight/getWeightByProcessDefKeyAndActId）。
// 参数：query processDefinitionKey/activityId（必填）、tenantId。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowWeightList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	definitionKey, err := params.getRequired("processDefinitionKey")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	activityID, err := params.getRequired("activityId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ListCountersignWeights(r.Context(), definitionKey, activityID, workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// ─── /process/**：批量挂起激活与签收/委派处理 ─────────────────────

// handleWorkflowBatchSuspend 批量挂起流程实例（POST /workflow/process/suspend/processes）。
// 参数：body processInstanceIds[]（JSON 数组或 query 逗号分隔，必填）、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowBatchSuspend(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceIDs := params.getSlice("processInstanceIds")
	if len(instanceIDs) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 processInstanceIds", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.SuspendProcessInstances(r.Context(), instanceIDs, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowBatchActivate 批量恢复挂起的流程实例（POST /workflow/process/activate/processes）。
// 参数：body processInstanceIds[]（JSON 数组或 query 逗号分隔，必填）、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowBatchActivate(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceIDs := params.getSlice("processInstanceIds")
	if len(instanceIDs) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 processInstanceIds", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.ActivateProcessInstances(r.Context(), instanceIDs, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowResolveTask 处理委派任务（POST /workflow/process/resolve/task）。
// 参数：query/body taskId（必填）、userId（回退认证身份）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowResolveTask(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ResolveTask(r.Context(), taskID, firstNonEmpty(params.get("userId"), requestUserID(r)))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowClaimTask 签收任务（POST /workflow/process/claim/task）。
// 参数：query/body taskId（必填）、userId（回退认证身份）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowClaimTask(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.ClaimTask(r.Context(), taskID, firstNonEmpty(params.get("userId"), requestUserID(r)))
	writeWorkflowResult(w, result, err)
}

// ─── /process/**：实例分页 ──────────────────────────────────────

// handleWorkflowInstancePageList 分页查询运行中实例（GET /workflow/process/instance/all/list）。
// 参数：query start/size（缺省 0/10，上限 1000）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowInstancePageList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.GetProcessInstancePageList(r.Context(), params.pageQuery())
	writeWorkflowResult(w, result, err)
}

// ─── /identity/**：身份库树 ─────────────────────────────────────

// handleWorkflowIdentityTree 查询内置身份库树（GET /workflow/identity/tree，无参）。
// 组 → 组成员用户两级，未分组用户挂"未分组"根节点。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowIdentityTree(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.GetIdentityTree(r.Context())
	writeWorkflowResult(w, result, err)
}

// ─── /task/**：操作按钮清单 ─────────────────────────────────────

// handleWorkflowOperationButtons 按实例状态聚合可用操作按钮
// （GET /workflow/task/operationButtonList/{processInstanceId}）。
// 参数：路径 processInstanceId（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowOperationButtons(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := r.PathValue("processInstanceId")
	if instanceID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 processInstanceId", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.GetOperationButtons(r.Context(), instanceID)
	writeWorkflowResult(w, result, err)
}
