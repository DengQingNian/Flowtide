package rest

import (
	"net/http"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// handleWorkflowInjectAfter 将旧请求字段转换为实例派生命令，成功时返回空响应。
// @param w 响应写入器
// @param r 带可信租户上下文的HTTP请求
// @param params 旧JSON及查询参数读取器
// @return 无，错误遵循workflow响应协议
func (s *Server) handleWorkflowInjectAfter(w http.ResponseWriter, r *http.Request, params *workflowParams) {
	instanceID := firstNonEmpty(params.get("processInstaceId"), params.get("processInstanceId"))
	req := api.InsertUserTaskRequest{
		AfterActivityID:     params.get("userTaskId"),
		ProcessDefinitionID: params.get("processDefinitionId"),
		Name:                params.get("name"),
		Assignee:            firstNonEmpty(params.get("assginee"), params.get("assignee")),
	}
	// 旧DTO的五个字段均为必填；兼容旧拼写，租户只取请求上下文。
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(req.ProcessDefinitionID) == "" || strings.TrimSpace(req.AfterActivityID) == "" || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Assignee) == "" {
		writeWorkflowParamError(w, api.NewError("REST-0126", "实例、定义、原节点、新节点名称和办理人不能为空", api.ErrBadRequest))
		return
	}
	if _, err := s.engine.RuntimeService().InjectUserTaskAfter(r.Context(), instanceID, req); err != nil {
		writeWorkflowResult(w, api.R{}, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
