package rest

import (
	"net/http"
)

// handleCreateExecutionVariable 创建执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateExecutionVariable(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusCreated, variables)
}

// handleAsyncInstanceVariables 同步更新实例变量（异步端点兼容实现）。
// @param w 响应
// @param r 请求
func (s *Server) handleAsyncInstanceVariables(w http.ResponseWriter, r *http.Request) {
	variables, err := readVariableBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, variable := range variables {
		if err := s.engine.RuntimeService().SetVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
			writeError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAsyncInstanceVariable 同步更新单个实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleAsyncInstanceVariable(w http.ResponseWriter, r *http.Request) {
	var payload executionVariablePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.RuntimeService().SetVariable(r.Context(), r.PathValue("id"), r.PathValue("name"), payload.Value); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAsyncExecutionVariables 同步更新执行变量（异步端点兼容实现）。
// @param w 响应
// @param r 请求
func (s *Server) handleAsyncExecutionVariables(w http.ResponseWriter, r *http.Request) {
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
	w.WriteHeader(http.StatusNoContent)
}

// handleAsyncExecutionVariable 同步更新单个执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleAsyncExecutionVariable(w http.ResponseWriter, r *http.Request) {
	var payload executionVariablePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.RuntimeService().SetExecutionVariable(r.Context(), r.PathValue("id"), r.PathValue("name"), payload.Value); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
