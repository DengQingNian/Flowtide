package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// executionVariablePayload 是执行变量写入请求。
type executionVariablePayload struct {
	Value any    `json:"value"` // 变量值
	Type  string `json:"type"`  // 变量类型
}

// handleGetExecutionVariable 返回单个执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleGetExecutionVariable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	value, err := s.engine.RuntimeService().GetExecutionVariable(r.Context(), r.PathValue("id"), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Variable{Name: name, Value: value})
}

// handleSetExecutionVariable 设置执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleSetExecutionVariable(w http.ResponseWriter, r *http.Request) {
	if variable, ok, err := readMultipartVariable(r, r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	} else if ok {
		if err := s.engine.RuntimeService().SetExecutionVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, variable)
		return
	}
	var payload executionVariablePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	name := r.PathValue("name")
	if err := s.engine.RuntimeService().SetExecutionVariable(r.Context(), r.PathValue("id"), name, payload.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Variable{Name: name, Type: payload.Type, Value: payload.Value})
}

// handleDeleteExecutionVariable 删除执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteExecutionVariable(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().DeleteExecutionVariable(r.Context(), r.PathValue("id"), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteAllExecutionVariables 清空执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteAllExecutionVariables(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().DeleteAllExecutionVariables(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetExecutionVariableData 返回执行变量原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetExecutionVariableData(w http.ResponseWriter, r *http.Request) {
	value, err := s.engine.RuntimeService().GetExecutionVariable(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, value)
}

// handleUpdateInstanceVariables 批量创建或更新实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateInstanceVariables(w http.ResponseWriter, r *http.Request) {
	variables, err := readVariableBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	updated := make([]api.Variable, 0, len(variables))
	for _, variable := range variables {
		if err := s.engine.RuntimeService().SetVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
			writeError(w, err)
			return
		}
		updated = append(updated, api.Variable{Name: variable.Name, Type: variable.Type, Value: variable.Value})
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteAllInstanceVariables 清空实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteAllInstanceVariables(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().DeleteAllInstanceVariables(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetInstanceVariableData 返回实例变量原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetInstanceVariableData(w http.ResponseWriter, r *http.Request) {
	value, err := s.engine.RuntimeService().GetVariable(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, value)
}

// handleDeleteAllTaskVariables 清空任务局部变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteAllTaskVariables(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.TaskService().DeleteAllTaskVariables(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetTaskVariableData 返回任务变量原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTaskVariableData(w http.ResponseWriter, r *http.Request) {
	value, err := s.engine.TaskService().GetTaskVariable(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, value)
}

// handleGetHistoricVariableData 返回历史变量原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetHistoricVariableData(w http.ResponseWriter, r *http.Request) {
	variable, err := s.engine.HistoryService().GetHistoricVariableInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, variable.Value)
}

// handleGetHistoricProcessVariableData 返回实例历史变量原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetHistoricProcessVariableData(w http.ResponseWriter, r *http.Request) {
	variable, err := s.engine.HistoryService().GetHistoricVariableByName(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			writeRuntimeFailure(w, 404, fmt.Sprintf("Historic process instance '%s' variable value for %s could not be found.", r.PathValue("id"), r.PathValue("name")))
			return
		}
		writeError(w, err)
		return
	}
	writeVariableData(w, variable.Value)
}

// handleGetHistoricTaskVariableData 返回历史任务变量原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetHistoricTaskVariableData(w http.ResponseWriter, r *http.Request) {
	variable, err := s.engine.HistoryService().GetHistoricTaskVariableByName(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, variable.Value)
}

// handleGetHistoricDetailData 返回历史明细原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetHistoricDetailData(w http.ResponseWriter, r *http.Request) {
	detail, err := s.engine.HistoryService().GetHistoricDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, detail.Value)
}

// handleCreateHistoricProcessComment 为历史实例添加评论。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateHistoricProcessComment(w http.ResponseWriter, r *http.Request) {
	var payload commentPayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.historicProcessForRequest(r); err != nil {
		writeError(w, err)
		return
	}
	comment, err := s.engine.TaskService().AddProcessComment(r.Context(), r.PathValue("id"), firstNonEmpty(payload.UserID, requestUserID(r)), payload.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, historicCommentRESTResponse(r, comment))
}

// handleDeleteHistoricProcessComment 删除历史实例评论。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteHistoricProcessComment(w http.ResponseWriter, r *http.Request) {
	comment, err := s.engine.TaskService().GetComment(r.Context(), r.PathValue("commentId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if comment.ProcessInstanceID != r.PathValue("id") {
		writeError(w, api.NewError("ENGINE-0404", "评论不存在", api.ErrNotFound))
		return
	}
	if err := s.engine.TaskService().DeleteComment(r.Context(), comment.ID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readVariableBody 读取变量数组或对象。
// @param r 请求
// @return 变量列表
// @return 解析错误
func readVariableBody(r *http.Request) ([]api.Variable, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxUploadSize))
	if err != nil {
		return nil, api.NewError("REST-0118", "读取请求体失败", err)
	}
	if len(data) == 0 {
		return nil, api.NewError("REST-0118", "缺少变量请求体", api.ErrBadRequest)
	}
	variables, err := parseVariables(json.RawMessage(data))
	if err != nil {
		return nil, err
	}
	if len(variables) == 0 {
		return nil, api.NewError("REST-0118", "变量列表不能为空", api.ErrBadRequest)
	}
	return variables, nil
}

// readMultipartVariable 解析 multipart 二进制变量请求。
// 非 multipart 返回 ok=false，由调用方继续解析 JSON。
// @param r 请求
// @param name 路径变量名，可为空
// @return 变量
// @return 是否 multipart
// @return 解析错误
func readMultipartVariable(r *http.Request, name string) (api.Variable, bool, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return api.Variable{}, false, nil
	}
	data, ok, err := readMultipartSource(r)
	if err != nil {
		return api.Variable{}, true, err
	}
	if !ok || len(data) == 0 {
		return api.Variable{}, true, api.NewError("REST-0144", "缺少上传文件", api.ErrBadRequest)
	}
	if name == "" && r.MultipartForm != nil {
		if values := r.MultipartForm.Value["name"]; len(values) > 0 {
			name = values[0]
		}
	}
	if strings.TrimSpace(name) == "" {
		return api.Variable{}, true, api.NewError("REST-0145", "变量名不能为空", api.ErrBadRequest)
	}
	return api.Variable{Name: name, Type: "binary", Value: data}, true, nil
}

// writeVariableData 输出变量原始内容。
// @param w 响应
// @param value 变量值
func writeVariableData(w http.ResponseWriter, value any) {
	var data []byte
	switch typed := value.(type) {
	case []byte:
		data = typed
	case string:
		data = []byte(typed)
	default:
		writeError(w, api.NewError("REST-0131", "变量不是文本或二进制", api.ErrBadRequest))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
