package rest

import (
	"net/http"
	"strconv"

	"github.com/flowable-go/flowable/api"
)

// handleListTaskEvents 返回任务事件列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListTaskEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.engine.TaskService().ListTaskEvents(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// handleGetTaskEvent 返回单个任务事件。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTaskEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r.PathValue("eventId"))
	if err != nil {
		writeError(w, err)
		return
	}
	event, err := s.engine.TaskService().GetTaskEvent(r.Context(), r.PathValue("id"), eventID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, event)
}

// handleDeleteTaskEvent 删除任务事件。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteTaskEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseEventID(r.PathValue("eventId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.TaskService().DeleteTaskEvent(r.Context(), r.PathValue("id"), eventID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseEventID 解析路径中的事件 ID。
// @param raw 原始字符串
// @return 事件 ID
// @return 解析错误
func parseEventID(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, api.NewError("REST-0132", "事件 ID 必须为整数", api.ErrBadRequest)
	}
	return value, nil
}

// handleHistoricTaskForm 返回历史任务表单。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricTaskForm(w http.ResponseWriter, r *http.Request) {
	form, err := s.engine.HistoryService().GetHistoricTaskFormData(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, form)
}

// handleGetHistoricProcessComment 返回单条历史实例评论。
// @param w 响应
// @param r 请求
func (s *Server) handleGetHistoricProcessComment(w http.ResponseWriter, r *http.Request) {
	if _, err := s.historicProcessForRequest(r); err != nil {
		writeError(w, err)
		return
	}
	comment, err := s.engine.TaskService().GetComment(r.Context(), r.PathValue("commentId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if comment.ProcessInstanceID != r.PathValue("id") {
		writeError(w, api.NewError("ENGINE-0404", "评论不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, comment)
}
