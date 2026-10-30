package rest

import (
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
)

// handleListDefinitionIdentityLinks 返回流程定义候选发起人。
// @param w 响应
// @param r 请求
func (s *Server) handleListDefinitionIdentityLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.RepositoryService().ListDefinitionIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// handleAddDefinitionIdentityLink 添加候选发起人。
// @param w 响应
// @param r 请求
func (s *Server) handleAddDefinitionIdentityLink(w http.ResponseWriter, r *http.Request) {
	var payload identityLinkPayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	link, err := s.engine.RepositoryService().AddDefinitionIdentityLink(r.Context(), r.PathValue("id"), api.IdentityLink{
		UserID:  payload.UserID,
		GroupID: payload.GroupID,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// handleGetDefinitionIdentityLink 返回单个候选发起人。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDefinitionIdentityLink(w http.ResponseWriter, r *http.Request) {
	family := r.PathValue("family")
	if family != "users" && family != "groups" {
		writeError(w, api.NewError("ENGINE-1303", "参与者类型非法: "+family, api.ErrBadRequest))
		return
	}
	link, err := s.engine.RepositoryService().GetDefinitionIdentityLink(r.Context(), r.PathValue("id"), family, r.PathValue("identityId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// handleDeleteDefinitionIdentityLink 删除候选发起人。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteDefinitionIdentityLink(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RepositoryService().DeleteDefinitionIdentityLink(r.Context(), r.PathValue("id"), r.PathValue("family"), r.PathValue("identityId")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
