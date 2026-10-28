package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// propertyRequest 是属性请求体。
type propertyRequest struct {
	Name  string `json:"name"`  // 名称
	Value string `json:"value"` // 值
}

// handleCreateProperty 创建自定义引擎属性。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateProperty(w http.ResponseWriter, r *http.Request) {
	var payload propertyRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.ManagementService().CreateProperty(r.Context(), payload.Name, payload.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": payload.Name, "value": payload.Value})
}

// handleUpdateProperty 更新自定义引擎属性。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateProperty(w http.ResponseWriter, r *http.Request) {
	var payload propertyRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	name := r.PathValue("name")
	if payload.Name != "" && payload.Name != name {
		writeError(w, api.NewError("ENGINE-1805", "请求体属性名与路径不一致", api.ErrBadRequest))
		return
	}
	if err := s.engine.ManagementService().UpdateProperty(r.Context(), name, payload.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "value": payload.Value})
}

// handleDeleteProperty 删除自定义引擎属性。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteProperty(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.ManagementService().DeleteProperty(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
