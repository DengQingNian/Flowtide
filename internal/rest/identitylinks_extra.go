package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// handleGetTaskIdentityLink 返回任务的单个身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTaskIdentityLink(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.TaskService().ListIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	link, ok := findIdentityLink(links, r.PathValue("family"), r.PathValue("identityId"), firstNonEmpty(r.URL.Query().Get("type"), r.PathValue("type")))
	if !ok {
		writeError(w, api.NewError("ENGINE-0404", "身份链接不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// handleGetInstanceIdentityLink 返回实例的单个身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleGetInstanceIdentityLink(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.RuntimeService().ListInstanceIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	link, ok := findIdentityLink(links, r.PathValue("family"), r.PathValue("identityId"), firstNonEmpty(r.URL.Query().Get("type"), r.PathValue("type")))
	if !ok {
		writeError(w, api.NewError("ENGINE-0404", "身份链接不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// findIdentityLink 按族、身份与类型查找链接。
// @param links 链接列表
// @param family 族：users/groups
// @param identityID 用户或组 ID
// @param linkType 链接类型，空表示任意
// @return 链接
// @return 是否找到
func findIdentityLink(links []api.IdentityLink, family, identityID, linkType string) (api.IdentityLink, bool) {
	for _, link := range links {
		if linkType != "" && link.Type != linkType {
			continue
		}
		switch family {
		case "users":
			if link.UserID == identityID {
				return link, true
			}
		case "groups":
			if link.GroupID == identityID {
				return link, true
			}
		}
	}
	return api.IdentityLink{}, false
}
