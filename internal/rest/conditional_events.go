package rest

import "net/http"

// handleEvaluateConditions 显式求值流程实例的全部条件事件。
// 建议路由：POST /service/runtime/process-instances/{id}/evaluate-conditions。
// 成功返回 204，实例不存在返回 404，实例已结束或挂起返回 409。
// @param w 响应
// @param r 请求
func (s *Server) handleEvaluateConditions(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().EvaluateConditions(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
