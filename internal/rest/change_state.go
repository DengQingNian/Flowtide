package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// changeStateRequest 是活动迁移请求。
type changeStateRequest struct {
	StartActivityIDs []string `json:"startActivityIds"` // 目标活动 ID
}

// handleExecutionChangeState 迁移执行到新活动。
// @param w 响应
// @param r 请求
func (s *Server) handleExecutionChangeState(w http.ResponseWriter, r *http.Request) {
	var payload changeStateRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if len(payload.StartActivityIDs) == 0 {
		writeError(w, api.NewError("ENGINE-0662", "目标活动不能为空", api.ErrBadRequest))
		return
	}
	if err := s.engine.RuntimeService().ChangeActivityState(r.Context(), r.PathValue("id"), payload.StartActivityIDs); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleInstanceChangeState 迁移实例根执行到新活动。
// @param w 响应
// @param r 请求
func (s *Server) handleInstanceChangeState(w http.ResponseWriter, r *http.Request) {
	var payload changeStateRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if len(payload.StartActivityIDs) == 0 {
		writeError(w, api.NewError("ENGINE-0662", "目标活动不能为空", api.ErrBadRequest))
		return
	}
	if err := s.engine.RuntimeService().ChangeProcessInstanceState(r.Context(), r.PathValue("id"), payload.StartActivityIDs); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
