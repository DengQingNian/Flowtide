package rest

import (
	"github.com/flowable-go/flowable/api"
	"net/http"
)

// serveIdentityUsers 按原版查询、排序和实际分页返回公开用户对象。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) serveIdentityUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.engine.IdentityService().ListUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var memberIDs map[string]bool
	if group := r.URL.Query().Get("memberOfGroup"); group != "" {
		members, err := s.engine.IdentityService().ListMembers(r.Context(), group)
		if err != nil {
			writeError(w, err)
			return
		}
		memberIDs = map[string]bool{}
		for _, member := range members {
			memberIDs[member.ID] = true
		}
	}
	items := make([]api.User, 0, len(users))
	// 成员集合与公开字符串条件取交集，过滤后才计算分页总数。
	for _, user := range users {
		if memberIDs != nil && !memberIDs[user.ID] {
			continue
		}
		if identityQueryMatches(r, map[string]string{"id": user.ID, "firstName": user.FirstName, "lastName": user.LastName, "displayName": user.DisplayName, "email": user.Email, "tenantId": user.TenantID}) {
			items = append(items, user)
		}
	}
	start, size, field, order, err := identityPagination(r, false)
	if err == nil {
		err = sortQueryItems(items, field, order, identityUserSortKey)
	}
	if err != nil {
		writeRuntimeQueryError(w, err)
		return
	}
	data := make([]map[string]any, 0)
	for _, user := range runtimePage(items, start, size) {
		response, err := s.identityUserResponse(r, user)
		if err != nil {
			writeError(w, err)
			return
		}
		data = append(data, response)
	}
	writeJSON(w, http.StatusOK, api.Page[map[string]any]{Data: data, Total: int64(len(items)), Start: start, Size: len(data), Sort: field, Order: order})
}

// serveIdentityGroups 按原版组属性及成员条件过滤后返回实际分页。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) serveIdentityGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.engine.IdentityService().ListGroups(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	var memberGroups map[string]bool
	if user := r.URL.Query().Get("member"); user != "" {
		memberships, err := s.engine.IdentityService().ListUserGroups(r.Context(), user)
		if err != nil {
			writeError(w, err)
			return
		}
		memberGroups = map[string]bool{}
		for _, group := range memberships {
			memberGroups[group.ID] = true
		}
	}
	items := make([]api.Group, 0, len(groups))
	// 组的成员筛选使用实际成员关系，不从用户公开 DTO 推断。
	for _, group := range groups {
		if memberGroups != nil && !memberGroups[group.ID] {
			continue
		}
		if identityQueryMatches(r, map[string]string{"id": group.ID, "name": group.Name, "type": group.Type}) {
			items = append(items, group)
		}
	}
	start, size, field, order, err := identityPagination(r, true)
	if err == nil {
		err = sortQueryItems(items, field, order, identityGroupSortKey)
	}
	if err != nil {
		writeRuntimeQueryError(w, err)
		return
	}
	data := make([]map[string]any, 0)
	for _, group := range runtimePage(items, start, size) {
		data = append(data, identityGroupResponse(r, group))
	}
	writeJSON(w, http.StatusOK, api.Page[map[string]any]{Data: data, Total: int64(len(items)), Start: start, Size: len(data), Sort: field, Order: order})
}
