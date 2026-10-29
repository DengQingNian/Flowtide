package rest

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// taskCreatePayload 是独立任务创建请求体。
type taskCreatePayload struct {
	Name         string `json:"name"`         // 名称
	Description  string `json:"description"`  // 描述
	Assignee     string `json:"assignee"`     // 处理人
	Owner        string `json:"owner"`        // 所有者
	Priority     int    `json:"priority"`     // 优先级
	DueDate      string `json:"dueDate"`      // 到期时间
	Category     string `json:"category"`     // 分类
	FormKey      string `json:"formKey"`      // 表单 Key
	ParentTaskID string `json:"parentTaskId"` // 父任务 ID
	TenantID     string `json:"tenantId"`     // 租户
}

// decodeTaskJSON 解析任务请求体，允许客户端携带未知字段。
// @param r 请求
// @param target 目标对象
// @return 解析错误
func decodeTaskJSON(r *http.Request, target any) error {
	return decodeSingleJSON(r, target, false, false)
}

// parseTaskTime 解析 ISO-8601 时间。
// @param value 时间字符串
// @return 时间
// @return 解析错误
func parseTaskTime(value string) (time.Time, error) {
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, api.NewError("REST-0122", "时间格式非法: "+value, api.ErrBadRequest)
}

// handleCreateTask 创建独立任务。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var payload taskCreatePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	var dueDate *time.Time
	if strings.TrimSpace(payload.DueDate) != "" {
		parsed, err := parseTaskTime(payload.DueDate)
		if err != nil {
			writeError(w, err)
			return
		}
		dueDate = &parsed
	}
	task, err := s.engine.TaskService().CreateTask(r.Context(), api.CreateTaskRequest{
		Name:         payload.Name,
		Description:  payload.Description,
		Assignee:     payload.Assignee,
		Owner:        payload.Owner,
		Priority:     payload.Priority,
		DueDate:      dueDate,
		Category:     payload.Category,
		FormKey:      payload.FormKey,
		ParentTaskID: payload.ParentTaskID,
		TenantID:     firstNonEmpty(payload.TenantID, requestTenant(r)),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, taskRESTResponse(r, task))
}

// handleUpdateTask 更新任务属性。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	var payload map[string]json.RawMessage
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	update, err := parseTaskUpdate(payload, requestUserID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	task, err := s.engine.TaskService().UpdateTask(r.Context(), r.PathValue("id"), update)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskRESTResponse(r, task))
}

// decodeNullableString 解析可空字符串，null 转为空串。
// @param raw 原始 JSON
// @return 字符串
// @return 解析错误
func decodeNullableString(raw json.RawMessage) (string, error) {
	if string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", api.NewError("REST-0126", "字符串字段类型错误", api.ErrBadRequest)
	}
	return value, nil
}

// handleDeleteTask 删除独立任务。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	cascadeHistory := r.URL.Query().Get("cascadeHistory") == "true"
	deleteReason := r.URL.Query().Get("deleteReason")
	if err := s.engine.TaskService().DeleteTask(r.Context(), r.PathValue("id"), cascadeHistory, deleteReason); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseTaskUpdate 解析任务部分更新请求。
// @param payload 原始 JSON 字段
// @param userID 操作人
// @return 更新请求
// @return 解析错误
func parseTaskUpdate(payload map[string]json.RawMessage, userID string) (api.TaskUpdate, error) {
	update := api.TaskUpdate{UserID: userID}
	for key, target := range map[string]**string{
		"name":        &update.Name,
		"description": &update.Description,
		"assignee":    &update.Assignee,
		"owner":       &update.Owner,
		"category":    &update.Category,
		"formKey":     &update.FormKey,
	} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		value, err := decodeNullableString(raw)
		if err != nil {
			return api.TaskUpdate{}, err
		}
		*target = &value
	}
	if raw, ok := payload["priority"]; ok && string(raw) != "null" {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return api.TaskUpdate{}, api.NewError("REST-0121", "priority 必须为整数", api.ErrBadRequest)
		}
		update.Priority = &value
	}
	if raw, ok := payload["dueDate"]; ok {
		if string(raw) == "null" {
			update.ClearDueDate = true
		} else {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return api.TaskUpdate{}, api.NewError("REST-0122", "dueDate 格式非法", api.ErrBadRequest)
			}
			if strings.TrimSpace(value) == "" {
				update.ClearDueDate = true
			} else {
				parsed, err := parseTaskTime(value)
				if err != nil {
					return api.TaskUpdate{}, err
				}
				update.DueDate = &parsed
			}
		}
	}
	return update, nil
}

// handleListSubTasks 返回子任务。
// @param w 响应
// @param r 请求
func (s *Server) handleListSubTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.engine.TaskService().ListSubTasks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskRESTResponses(r, tasks))
}
