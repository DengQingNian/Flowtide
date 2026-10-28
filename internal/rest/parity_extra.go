package rest

import (
	"encoding/json"
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// handleUpdateGroup 更新组。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	var group api.Group
	if err := decodeJSON(r, &group); err != nil {
		writeError(w, err)
		return
	}
	if group.ID != "" && group.ID != r.PathValue("id") {
		writeError(w, api.NewError("REST-0135", "请求体组 ID 与路径不一致", api.ErrBadRequest))
		return
	}
	group.ID = r.PathValue("id")
	updated, err := s.engine.IdentityService().UpdateGroup(r.Context(), group)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleBulkUpdateTasks 批量更新任务。
// @param w 响应
// @param r 请求
func (s *Server) handleBulkUpdateTasks(w http.ResponseWriter, r *http.Request) {
	var payload map[string]json.RawMessage
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	rawIDs, ok := payload["taskIds"]
	if !ok || string(rawIDs) == "null" {
		writeError(w, api.NewError("REST-0136", "taskIds 不能为空", api.ErrBadRequest))
		return
	}
	var taskIDs []string
	if err := json.Unmarshal(rawIDs, &taskIDs); err != nil {
		writeError(w, api.NewError("REST-0136", "taskIds 必须为字符串数组", api.ErrBadRequest))
		return
	}
	if len(taskIDs) == 0 {
		writeError(w, api.NewError("REST-0136", "taskIds 不能为空", api.ErrBadRequest))
		return
	}
	delete(payload, "taskIds")
	update, err := parseTaskUpdate(payload, requestUserID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	tasks := make([]api.Task, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		task, err := s.engine.TaskService().UpdateTask(r.Context(), taskID, update)
		if err != nil {
			writeError(w, err)
			return
		}
		tasks = append(tasks, task)
	}
	writePage(w, tasks, 0, len(tasks))
}

// handleUpdateExecutionVariables 批量更新执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateExecutionVariables(w http.ResponseWriter, r *http.Request) {
	variables, err := readVariableBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, variable := range variables {
		if err := s.engine.RuntimeService().SetExecutionVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, variables)
}
