package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// handleInjectActivity 向流程实例注入动态活动。
// @param w 响应
// @param r 请求
func (s *Server) handleInjectActivity(w http.ResponseWriter, r *http.Request) {
	var payload api.InjectRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	task, err := s.engine.RuntimeService().InjectUserTask(r.Context(), r.PathValue("id"), payload)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
