package rest

// 本文件承载 /workflow/** 已接线端点的公共辅助（参数解析、响应协议）与
// /process/**、/task/**、/dict/** 业务 handler。
// 响应协议（005 §4）：成功与业务失败均为 HTTP 200 + api.R 信封
// （成功 R.Success=true；业务失败 R.Success=false）；
// 参数/系统错误为 HTTP 500 + RErr。未注入 workflow 服务时统一 501（见 workflow_routes.go）。

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/workflow"
)

// workflowBizErrCode 是 workflow 门面业务失败/参数错误的协议状态码（enm.R 仅 "200"/"500" 两态）。
const workflowBizErrCode = "500"

// workflowMaxPageSize 是 workflow 门面列表端点的单页上限（与 pageParams 保持一致）。
const workflowMaxPageSize = 1000

// workflowParams 是 /workflow/** 端点的参数读取器：query 优先、JSON body 兜底。
type workflowParams struct {
	query url.Values     // URL 查询参数
	body  map[string]any // JSON body 对象（空体/非对象体为 nil）
	raw   []byte         // 原始 body 字节（供结构化解码）
}

// newWorkflowParams 构造参数读取器：解析 JSON body（空体容错，非对象体报参数错误）。
// @param r 请求
// @return 参数读取器
// @return body 解析错误
func newWorkflowParams(r *http.Request) (*workflowParams, error) {
	params := &workflowParams{query: r.URL.Query()}
	if r.Body == nil || r.ContentLength == 0 {
		return params, nil
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxUploadSize+1))
	if err != nil {
		return params, api.NewError("REST-0104", "读取请求体失败", err)
	}
	if len(data) > MaxUploadSize {
		return params, api.NewError("REST-0105", "请求体超过大小限制", api.ErrBadRequest)
	}
	params.raw = bytes.TrimSpace(data)
	if len(params.raw) == 0 {
		return params, nil
	}
	// body 统一按 JSON 对象解析：/workflow/** 端点协议均为对象形态
	if err := json.Unmarshal(params.raw, &params.body); err != nil || params.body == nil {
		return params, api.NewError("REST-0109", "请求体必须为 JSON 对象", api.ErrBadRequest)
	}
	return params, nil
}

// get 返回参数值：query 优先、body 兜底（数值/布尔值转十进制串）。
// @param key 参数名
// @return 参数值（缺失为空串）
func (p *workflowParams) get(key string) string {
	if v := p.query.Get(key); v != "" {
		return v
	}
	if p.body == nil {
		return ""
	}
	// body 值可能是 string/float64/bool/json.Number，统一串化
	switch v := p.body[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return ""
	}
}

// getRequired 取必填参数，缺失时报参数错误。
// @param key 参数名
// @return 参数值
// @return 缺失错误（REST-0115）
func (p *workflowParams) getRequired(key string) (string, error) {
	v := p.get(key)
	if v == "" {
		return "", api.NewError("REST-0115", "缺少必填参数 "+key, api.ErrBadRequest)
	}
	return v, nil
}

// getInt 返回整数参数：query 优先、body 兜底，非法/缺失用缺省值。
// @param key 参数名
// @param def 缺省值
// @return 整数值
func (p *workflowParams) getInt(key string, def int) int {
	v := p.get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	// 非法数值回落缺省（协议容错）
	if err != nil {
		return def
	}
	return n
}

// getBool 返回布尔参数：query 优先、body 兜底，缺失用缺省值。
// @param key 参数名
// @param def 缺省值
// @return 布尔值
func (p *workflowParams) getBool(key string, def bool) bool {
	if v := p.query.Get(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		return def
	}
	if p.body == nil {
		return def
	}
	// body 布尔可能以原生 bool、数值或字符串形态出现
	switch v := p.body[key].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// getSlice 返回字符串数组参数：query（逗号分隔）优先、body 数组兜底。
// @param key 参数名
// @return 非空元素列表
func (p *workflowParams) getSlice(key string) []string {
	if v := p.query.Get(key); v != "" {
		return splitWorkflowIDs(v)
	}
	if p.body == nil {
		return nil
	}
	// body 数组元素兼容字符串与数值形态
	switch v := p.body[key].(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			switch s := item.(type) {
			case string:
				if s != "" {
					out = append(out, s)
				}
			case float64:
				out = append(out, strconv.FormatFloat(s, 'f', -1, 64))
			}
		}
		return out
	case string:
		return splitWorkflowIDs(v)
	default:
		return nil
	}
}

// pageQuery 解析分页参数（query 优先、body 兜底；start 缺省 0、size 缺省 10、上限 1000）。
// @return 分页参数
func (p *workflowParams) pageQuery() api.PageQuery {
	start := p.getInt("start", 0)
	size := p.getInt("size", 10)
	// 分页规范化：负偏移归零、非正数量取默认、超限截断
	if start < 0 {
		start = 0
	}
	if size <= 0 {
		size = 10
	}
	if size > workflowMaxPageSize {
		size = workflowMaxPageSize
	}
	return api.PageQuery{Start: start, Size: size}
}

// decode 将 JSON body 解码到目标结构（空体不操作）。
// @param target 目标对象
// @return 解析错误
func (p *workflowParams) decode(target any) error {
	if len(p.raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(p.raw, target); err != nil {
		return api.NewError("REST-0109", "请求体 JSON 非法", api.ErrBadRequest)
	}
	return nil
}

// splitWorkflowIDs 拆分逗号分隔的 Id 串（去空白、去空项）。
// @param raw 逗号分隔串
// @return Id 列表
func splitWorkflowIDs(raw string) []string {
	out := make([]string, 0, 4)
	for _, part := range bytes.Split([]byte(raw), []byte(",")) {
		trimmed := string(bytes.TrimSpace(part))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// workflowTenant 解析请求租户：参数 tenantId 优先，回退认证中间件绑定的请求租户。
// @param r 请求
// @param params 参数读取器
// @return 租户（业务系统代号）
func workflowTenant(r *http.Request, params *workflowParams) string {
	return firstNonEmpty(params.get("tenantId"), requestTenant(r))
}

// workflowOperator 解析操作人：参数优先，回退认证身份（X-User-Id / Basic Auth 用户）。
// @param r 请求
// @param params 参数读取器
// @param key 操作人参数名（operator 或 userCode）
// @return 操作人身份 Id
func workflowOperator(r *http.Request, params *workflowParams, key string) string {
	return firstNonEmpty(params.get(key), requestUserID(r))
}

// workflowErrMessage 提取错误的业务文案（api.Error 取中文说明，其余取原始串）。
// @param err 业务错误
// @return 文案
func workflowErrMessage(err error) string {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return err.Error()
}

// writeWorkflowParamError 输出参数/系统错误：HTTP 500 + R 失败体。
// @param w 响应
// @param err 参数错误
func writeWorkflowParamError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, api.RErr(workflowBizErrCode, workflowErrMessage(err)))
}

// writeWorkflowResult 输出 workflow 门面统一响应：成功与业务失败均为 HTTP 200 + R 信封。
// 业务失败（服务返回 error）转 R{code:"500", ErrorMessage, success:false}。
// @param w 响应
// @param result 成功统一返回
// @param err 业务错误
func writeWorkflowResult(w http.ResponseWriter, result api.R, err error) {
	if err != nil {
		writeJSON(w, http.StatusOK, api.RErr(workflowBizErrCode, workflowErrMessage(err)))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── /process/**：发起/办结/退回/终止/删除/挂起 ──────────────────

// handleWorkflowApplyTask 发起流程并预存首步（POST /workflow/process/apply/task）。
// 参数：body/query processDefinitionKey/title/operator/tenantId/businessKey/remark/variables。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowApplyTask(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	vo := &api.StartProcessInstanceVo{}
	if err := params.decode(vo); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量字段，认证身份兜底操作人与租户
	vo.ProcessDefinitionKey = firstNonEmpty(params.get("processDefinitionKey"), vo.ProcessDefinitionKey)
	vo.Title = firstNonEmpty(params.get("title"), vo.Title)
	vo.Operator = firstNonEmpty(params.get("operator"), vo.Operator, requestUserID(r))
	vo.TenantID = firstNonEmpty(params.get("tenantId"), vo.TenantID, requestTenant(r))
	vo.BusinessKey = firstNonEmpty(params.get("businessKey"), vo.BusinessKey)
	vo.Remark = firstNonEmpty(params.get("remark"), vo.Remark)
	result, err := s.workflow.StartProcessAndFirstStep(r.Context(), vo)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowCompleteTask 办结任务（POST /workflow/process/complete/task）。
// 参数：body/query taskId/userCode/message/processInstanceId/type/variables/request 等。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowCompleteTask(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	vo := &api.CompleteTaskVo{}
	if err := params.decode(vo); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐基础字段，操作人回退认证身份
	vo.TaskID = firstNonEmpty(params.get("taskId"), vo.TaskID)
	vo.UserCode = firstNonEmpty(params.get("userCode"), vo.UserCode, requestUserID(r))
	vo.Message = firstNonEmpty(params.get("message"), vo.Message)
	vo.ProcessInstanceID = firstNonEmpty(params.get("processInstanceId"), vo.ProcessInstanceID)
	vo.Type = firstNonEmpty(params.get("type"), vo.Type)
	result, err := s.workflow.Complete(r.Context(), vo)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowBackProcess 退回任务到指定步骤/上一节点（DELETE /workflow/process/back/process）。
// 参数：query/body taskId（必填）、targetActivityId（空时退回上一节点）、userCode、message、type。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowBackProcess(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	vo := &api.BackTaskVo{
		TaskID:           taskID,
		TargetActivityID: params.get("targetActivityId"),
		UserCode:         workflowOperator(r, params, "userCode"),
		Message:          params.get("message"),
		Type:             params.get("type"),
	}
	result, err := s.workflow.BackToStepTask(r.Context(), vo)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowStopProcess 软终止流程实例（DELETE /workflow/process/end/process 与
// POST /workflow/process/stop/processInstance/{pid}）。
// 参数：路径 pid 或 query/body processInstanceId（必填）、operator、message。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowStopProcess(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := firstNonEmpty(r.PathValue("pid"), params.get("processInstanceId"))
	if instanceID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 processInstanceId", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DoStopProcessInstance(r.Context(), instanceID, workflowOperator(r, params, "operator"), params.get("message"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDeleteByBusiness 按业务单据删除流程实例（DELETE /workflow/process/delete/process）。
// 参数：query/body businessKey（必填）、tenantId、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDeleteByBusiness(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	businessKey, err := params.getRequired("businessKey")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.DoDeleteProcessByBusinessID(r.Context(), workflowTenant(r, params), businessKey, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDeleteInstance 删除流程实例（ANY /workflow/process/instance/all/delete/{id}）。
// 参数：路径 id（必填）、operator、reason。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDeleteInstance(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := r.PathValue("id")
	if instanceID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 id", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DoDeleteProcessInstance(r.Context(), instanceID, workflowOperator(r, params, "operator"), params.get("reason"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowSuspendInstance 挂起流程实例（POST /workflow/process/suspend/processInstance/{pid}）。
// 参数：路径 pid（必填）、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowSuspendInstance(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := r.PathValue("pid")
	if instanceID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 pid", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DoSuspendProcessInstance(r.Context(), instanceID, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowActivateInstance 恢复挂起的流程实例（POST /workflow/process/activate/processInstance/{pid}）。
// 参数：路径 pid（必填）、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowActivateInstance(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := r.PathValue("pid")
	if instanceID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 pid", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.DoActivateProcessInstance(r.Context(), instanceID, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// ─── /process/**：待办/已办/任务详情/历史 ────────────────────────

// handleWorkflowTaskList 按任务 ID 查询任务详情（GET /workflow/process/task/list）。
// 参数：query/body taskId（必填）→ GetTaskInfoByTaskID。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTaskList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetTaskInfoByTaskID(r.Context(), taskID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowTodoList 按身份 Id 分页查询待办（GET /workflow/process/task/all/todoList）。
// 参数：query identityId（必填）、start/size。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTodoList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	identityID, err := params.getRequired("identityId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetTodoTaskListByIdentityID(r.Context(), identityID, params.pageQuery())
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDoneList 按身份 Id 分页查询已办（GET /workflow/process/task/all/doneList）。
// 参数：query identityId（必填）、start/size。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDoneList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	identityID, err := params.getRequired("identityId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.GetDoneTaskListByIdentityID(r.Context(), identityID, params.pageQuery())
	writeWorkflowResult(w, result, err)
}

// handleWorkflowHistoryStepList 按任务查询历史扩展记录（GET /workflow/process/history/step/list/{taskId}）。
// 参数：路径 taskId（必填）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowHistoryStepList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID := r.PathValue("taskId")
	if taskID == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 taskId", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.GetTaskHistoryListByTaskID(r.Context(), taskID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowHistoryTaskList 查询任务历史列表（GET /workflow/process/history/task/list）。
// 按 query 分派：processInstanceId → 按实例；businessKey → 按业务单据；都缺 → 参数错误。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowHistoryTaskList(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := params.get("processInstanceId")
	if instanceID != "" {
		result, err := s.workflow.GetTaskHistoryListByProcessInstanceID(r.Context(), instanceID)
		writeWorkflowResult(w, result, err)
		return
	}
	businessKey := params.get("businessKey")
	if businessKey != "" {
		result, err := s.workflow.GetTaskHistoryListByBusinessID(r.Context(), workflowTenant(r, params), businessKey)
		writeWorkflowResult(w, result, err)
		return
	}
	writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 processInstanceId 或 businessKey", api.ErrBadRequest))
}

// ─── /process/**：处理人调整/会签/加签减签 ───────────────────────

// handleWorkflowTransferTask 按节点调整运行中任务处理人（POST /workflow/process/transfer/task）。
// 参数：query/body processInstanceId/taskKey/toOwner（必填）、fromOwner（可空）、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowTransferTask(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID, err := params.getRequired("processInstanceId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	taskKey, err := params.getRequired("taskKey")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	toOwner, err := params.getRequired("toOwner")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.DoTaskOwnerAdjustment(r.Context(), instanceID, taskKey, params.get("fromOwner"), toOwner, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowCompleteSign 会签比例结论（POST /workflow/process/complete/sign）。
// 参数：query/body taskId（必填）、countersignResult（必填：0 否决 / 1 通过）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowCompleteSign(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	raw := params.get("countersignResult")
	if raw == "" {
		writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 countersignResult", api.ErrBadRequest))
		return
	}
	// 会签结论仅允许 0/1，其余回落否决并在服务层按比例计算
	conclusion, convErr := strconv.Atoi(raw)
	if convErr != nil {
		writeWorkflowParamError(w, api.NewError("REST-0115", "countersignResult 必须为整数", api.ErrBadRequest))
		return
	}
	result, err := s.workflow.CompleteSign(r.Context(), taskID, conclusion)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowAddExecution 加签（POST /workflow/process/add/execution）。
// 参数：body taskId/operator/assignees/scopeType/message；query 标量兜底、assignees 逗号分隔兜底。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowAddExecution(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	req := workflow.AddSignRequest{}
	if err := params.decode(&req); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐标量与加签人列表
	req.TaskID = firstNonEmpty(params.get("taskId"), req.TaskID)
	req.Operator = firstNonEmpty(params.get("operator"), req.Operator, requestUserID(r))
	req.ScopeType = firstNonEmpty(params.get("scopeType"), req.ScopeType)
	req.Message = firstNonEmpty(params.get("message"), req.Message)
	if assignees := params.getSlice("assignees"); len(assignees) > 0 {
		req.Assignees = assignees
	}
	result, err := s.workflow.AddExecution(r.Context(), req)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowDelExecution 减签（DELETE /workflow/process/del/execution）。
// 参数：query/body parentTaskId/subTaskId（必填）、operator、message。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDelExecution(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	req := workflow.DelSignRequest{}
	if err := params.decode(&req); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	req.ParentTaskID = firstNonEmpty(params.get("parentTaskId"), req.ParentTaskID)
	req.SubTaskID = firstNonEmpty(params.get("subTaskId"), req.SubTaskID)
	req.Operator = firstNonEmpty(params.get("operator"), req.Operator, requestUserID(r))
	req.Message = firstNonEmpty(params.get("message"), req.Message)
	result, err := s.workflow.DelExecution(r.Context(), req)
	writeWorkflowResult(w, result, err)
}

// ─── /process/**：部署/模型/高亮 ────────────────────────────────

// handleWorkflowDeploy 部署模型并生成步骤快照（ANY /workflow/process/deplay 与 /deploy）。
// 参数：query/body modelId（必填）、tenantId（回退请求租户）。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDeploy(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	modelID, err := params.getRequired("modelId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.DeployModel(r.Context(), workflowTenant(r, params), modelID)
	writeWorkflowResult(w, result, err)
}

// handleWorkflowHighlightImg 查询流程图高亮数据（DELETE /workflow/process/highlight/img 与 /normal/img，
// DELETE 方法保留 Java 兼容）。
// 参数：query/body processInstanceId 或 taskId 分派，都缺 → 参数错误。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowHighlightImg(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := params.get("processInstanceId")
	if instanceID != "" {
		result, err := s.workflow.GetFlowHighlight(r.Context(), instanceID)
		writeWorkflowResult(w, result, err)
		return
	}
	taskID := params.get("taskId")
	if taskID != "" {
		result, err := s.workflow.GetFlowHighlightByTask(r.Context(), taskID)
		writeWorkflowResult(w, result, err)
		return
	}
	writeWorkflowParamError(w, api.NewError("REST-0115", "缺少必填参数 processInstanceId 或 taskId", api.ErrBadRequest))
}

// ─── /task/**：撤回/直送 ────────────────────────────────────────

// handleWorkflowRollback 撤回已流转任务（POST /workflow/task/rollback）。
// 参数：query/body processInstanceId/taskId（必填）、operator。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowRollback(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID, err := params.getRequired("processInstanceId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	taskID, err := params.getRequired("taskId")
	if err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	result, err := s.workflow.DoRollbackTask(r.Context(), instanceID, taskID, workflowOperator(r, params, "operator"))
	writeWorkflowResult(w, result, err)
}

// handleWorkflowCompleteToTargetActivity 直送目标节点（POST /workflow/task/completeToTargetActivity）。
// 参数：body/query taskId/targetActivityId/userCode/message/processInstanceId/type 等。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowCompleteToTargetActivity(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	vo := &api.CompleteTaskVo{}
	if err := params.decode(vo); err != nil {
		writeWorkflowParamError(w, err)
		return
	}
	// query 优先补齐基础字段（直送目标必填）
	vo.TaskID = firstNonEmpty(params.get("taskId"), vo.TaskID)
	vo.TargetActivityID = firstNonEmpty(params.get("targetActivityId"), vo.TargetActivityID)
	vo.UserCode = firstNonEmpty(params.get("userCode"), vo.UserCode, requestUserID(r))
	vo.Message = firstNonEmpty(params.get("message"), vo.Message)
	vo.ProcessInstanceID = firstNonEmpty(params.get("processInstanceId"), vo.ProcessInstanceID)
	vo.Type = firstNonEmpty(params.get("type"), vo.Type)
	result, err := s.workflow.CompleteToTargetActivity(r.Context(), vo)
	writeWorkflowResult(w, result, err)
}

// ─── /dict/**：内置静态字典（D9）────────────────────────────────

// handleWorkflowDict 按码名返回静态字典（GET /workflow/dict/{code}）。
// 字典服务无状态（internal/workflow.DictService），未知名按业务失败返回。
// @param w 响应
// @param r 请求
// @param params 参数读取器
func (s *Server) handleWorkflowDict(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	dict, err := workflow.NewDictService().GetDict(r.PathValue("code"))
	writeWorkflowResult(w, api.ROk(dict), err)
}
