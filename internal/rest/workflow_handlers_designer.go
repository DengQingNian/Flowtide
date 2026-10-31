package rest

// 本文件承载 /workflow/** P6 设计器 XML 存取链路端点（facade_designer.go 对接）：
// 建模（add/model、create）、模型 XML 读改（getModelJsonByType、export/xml、save/xml）、
// 历史定义 XML（getHisModelJson、getProcessDefinitionJsonInfo、updateHisModelXml）、
// 复制流程（copyProcess）、模型+步骤联查（findModelAndStepPageList）、步骤详情（step/findById）、
// 引擎委派（delegate/task）与权重清空（weight/delete）。
// 响应协议与公共辅助见 workflow_handlers.go。

import (
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
)

// ─── /process/**：设计器建模与 XML 存取 ─────────────────────────

// handleWorkflowCreateModel 设计器建模（POST /workflow/process/add/model 与 ANY /create）。
// 参数：query/body name/modelCode（必填）、tenantId（回退请求租户）、xml（可空建骨架）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowCreateModel(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	name, err := params.getRequired("name")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	modelCode, err := params.getRequired("modelCode")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.CreateProcessModel(r.Context(), workflowTenant(r, params), name, modelCode, params.get("xml"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowGetModelJSON 读取模型/定义 XML（GET /workflow/process/getModelJsonByType 与 /export/xml）。
// 参数：query type=his 时按 definitionId 读历史定义 XML，否则按 modelId 读模型编辑源。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowGetModelJSON(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	// type=his 分支读历史定义，默认分支读业务模型编辑源
	if params.get("type") == "his" {
		definitionID, err := params.getRequired("definitionId")
		if err != nil {
			writeWorkflowParamError(w, err)
			return
		}
		result, err := s.workflow.GetDefinitionXML(r.Context(), definitionID)
		writeWorkflowResult(w, result, err)
		return
	}
	modelID, err := params.getRequired("modelId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetModelXML(r.Context(), modelID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowGetDefinitionJSON 读取历史流程定义 XML
// （GET /workflow/process/getHisModelJson 与 /workflow/model/getProcessDefinitionJsonInfo）。
// 参数：query definitionId（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowGetDefinitionJSON(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	definitionID, err := params.getRequired("definitionId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetDefinitionXML(r.Context(), definitionID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowSaveModelXML 保存模型 BPMN XML 编辑源（POST /workflow/process/save/xml）。
// 参数：query/body modelId（必填）、xml（必填，空源骨架模型可首存）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowSaveModelXML(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	modelID, err := params.getRequired("modelId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	xml, err := params.getRequired("xml")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.SaveModelXML(r.Context(), modelID, xml)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowUpdateModelFromDefinition 历史定义 XML 回填同 Key 模型编辑源
// （POST /workflow/process/updateHisModelXml）。
// 参数：query/body definitionId（必填）、tenantId（回退请求租户）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowUpdateModelFromDefinition(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	definitionID, err := params.getRequired("definitionId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.UpdateModelFromDefinition(r.Context(), workflowTenant(r, params), definitionID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowCopyProcess 复制源模型 XML 到目标模型列表（POST /workflow/process/copyProcess）。
// 参数：query/body resourceId（源业务模型主键，必填）、modelIdList[]（目标主键数组，必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowCopyProcess(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	sourceModelID, err := params.getRequired("resourceId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	targetModelIDs := params.getSlice("modelIdList")
	if len(targetModelIDs) == 0 {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 modelIdList", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.CopyProcess(r.Context(), sourceModelID, targetModelIDs)
	writeWorkflowResult(w, result, err)
}

// ─── /process/**：引擎委派 ──────────────────────────────────────

// handleWorkflowDelegateTask 引擎委派任务（POST /workflow/process/delegate/task）。
// 参数：query/body taskId（必填）、userId（被委派人，回退认证身份）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDelegateTask(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.DelegateTask(r.Context(), taskID, firstNonEmpty(params.get("userId"), requestUserID(r)))
	writeWorkflowResult(w, result, err)
}

// ─── /weight/**：权重清空 ───────────────────────────────────────

// handleWorkflowWeightDelete 清空节点会签权重（POST /workflow/weight/delete，Java 兼容方法级语义）。
// 参数：query/body processDefinitionKey/activityId（必填）、tenantId（空不隔离）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowWeightDelete(w http.ResponseWriter, r *http.Request, params *workflowParams) {
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
	result, err := s.workflow.DeleteCountersignWeights(r.Context(), definitionKey, activityID, workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// ─── /model/**、/step/**：联查与详情 ────────────────────────────

// handleWorkflowModelAndStepPage 查询租户下模型列表并附带设计时步骤
// （GET /workflow/model/findModelAndStepPageList）。
// 参数：query tenantId（回退请求租户；缺失由服务端判业务失败）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowModelAndStepPage(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	result, err := s.workflow.GetModelAndStepPage(r.Context(), workflowTenant(r, params))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowStepGet 按主键查询设计时步骤（GET /workflow/step/findById）。
// 参数：query id（必填，未命中 data 为 null）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowStepGet(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	id, err := params.getRequired("id")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetStepDesign(r.Context(), id)
	writeWorkflowResult(w, result, err)
}

// ─── /model/**：设计器节点元数据 ────────────────────────────────

// handleWorkflowUserTaskMetas 查询定义上全部用户任务节点元数据
// （GET /workflow/model/getUserTaskJsonInfo）。
// 参数：query definitionId（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowUserTaskMetas(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	definitionID, err := params.getRequired("definitionId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetUserTaskMetas(r.Context(), definitionID)
	writeWorkflowResult(w, result, err)
}
