package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// handleListInstanceIdentityLinks 返回流程实例身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleListInstanceIdentityLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.RuntimeService().ListInstanceIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// handleAddInstanceIdentityLink 添加流程实例身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleAddInstanceIdentityLink(w http.ResponseWriter, r *http.Request) {
	var payload identityLinkPayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	link, err := s.engine.RuntimeService().AddInstanceIdentityLink(r.Context(), r.PathValue("id"), api.IdentityLink{
		Type:    payload.Type,
		UserID:  payload.UserID,
		GroupID: payload.GroupID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// handleDeleteInstanceIdentityLink 删除流程实例身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteInstanceIdentityLink(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().DeleteInstanceIdentityLink(
		r.Context(), r.PathValue("id"), r.PathValue("family"), r.PathValue("identityId"), r.PathValue("type"),
	); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
