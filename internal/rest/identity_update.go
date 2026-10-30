package rest

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DengQingNian/Flowtide/api"
)

// readIdentityPatch 解析单个身份更新对象并保留字段是否出现，显式 null 与缺省不同。
// @param r 请求
// @param target 类型化属性对象
// @return 原始属性映射
// @return 请求格式错误
func readIdentityPatch(r *http.Request, target any) (queryBody, error) {
	var raw json.RawMessage
	if err := decodeTaskJSON(r, &raw); err != nil {
		return nil, err
	}
	// 先验证对象，再解码类型，同时保留字段出现信息。
	body := queryBody{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, api.NewError("REST-0109", "身份更新必须为对象", api.ErrBadRequest)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return nil, api.NewError("REST-0109", "身份属性类型非法", api.ErrBadRequest)
	}
	return body, nil
}

// updateIdentityUser 只合并请求中提供的公开资料，保留未提交的资料与内部身份字段。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) updateIdentityUser(w http.ResponseWriter, r *http.Request) {
	var patch api.User
	body, err := readIdentityPatch(r, &patch)
	if err != nil {
		writeError(w, err)
		return
	}
	existing, err := s.engine.IdentityService().GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	// 明确提交的 null 清空该字段，缺省字段继续使用读取到的原值。
	for key, pair := range map[string]struct {
		to    *string
		value string
	}{"firstName": {&existing.FirstName, patch.FirstName}, "lastName": {&existing.LastName, patch.LastName}, "displayName": {&existing.DisplayName, patch.DisplayName}, "email": {&existing.Email, patch.Email}, "password": {&existing.Password, patch.Password}} {
		if _, provided := body[key]; provided {
			*pair.to = pair.value
		}
	}
	updated, err := s.engine.IdentityService().UpdateUser(r.Context(), existing)
	if err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	s.writeIdentityUser(w, r, http.StatusOK, updated)
}

// updateIdentityGroup 合并组的可选名称和类型，不将空更新变成属性清空。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) updateIdentityGroup(w http.ResponseWriter, r *http.Request) {
	var patch api.Group
	body, err := readIdentityPatch(r, &patch)
	if err != nil {
		writeError(w, err)
		return
	}
	existing, err := s.engine.IdentityService().GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		writeIdentityError(w, r, err, true)
		return
	}
	if raw, provided := body["id"]; provided && string(raw) != "null" && patch.ID != existing.ID {
		writeRuntimeFailure(w, 400, "Key provided in request body does not match the key in the resource URL.")
		return
	}
	// 每个属性独立判断存在性，以便 null 清空与未提供保留原值。
	if _, provided := body["name"]; provided {
		existing.Name = patch.Name
	}
	if _, provided := body["type"]; provided {
		existing.Type = patch.Type
	}
	updated, err := s.engine.IdentityService().UpdateGroup(r.Context(), existing)
	if err != nil {
		writeIdentityError(w, r, err, true)
		return
	}
	writeJSON(w, http.StatusOK, identityGroupResponse(r, updated))
}

// identityMemberExists 在目标组的实际成员集合中检查指定用户。
// @param r 请求
// @param userID 用户标识
// @return 是否为组成员
// @return 查询失败
func (s *Server) identityMemberExists(r *http.Request, userID string) (bool, error) {
	members, err := s.engine.IdentityService().ListMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		return false, err
	}
	// 成员查询不依赖用户资料中的扩展角色。
	for _, member := range members {
		if member.ID == userID {
			return true, nil
		}
	}
	return false, nil
}

// createIdentityMembership 按原版返回成员对象，并在重复添加时返回冲突。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) createIdentityMembership(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID *string `json:"userId"`
	}
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	groupID := r.PathValue("id")
	if _, err := s.engine.IdentityService().GetGroup(r.Context(), groupID); err != nil {
		writeIdentityError(w, r, err, true)
		return
	}
	if payload.UserID == nil {
		writeRuntimeFailure(w, 400, "UserId cannot be null.")
		return
	}
	// 核心成员添加是幂等扩展，原版 HTTP 则要求重复添加返回冲突。
	member, err := s.identityMemberExists(r, *payload.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	if member {
		writeRuntimeFailure(w, 409, fmt.Sprintf("User '%s' is already part of group '%s'.", *payload.UserID, groupID))
		return
	}
	if err := s.engine.IdentityService().AddMembership(r.Context(), *payload.UserID, groupID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"userId": *payload.UserID, "groupId": groupID, "url": taskResourceURL(r, "identity/groups", groupID) + "/members/" + *payload.UserID})
}

// deleteIdentityMembership 拒绝删除不存在的成员关系，成功时返回空正文。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) deleteIdentityMembership(w http.ResponseWriter, r *http.Request) {
	groupID, userID := r.PathValue("id"), r.PathValue("userId")
	if _, err := s.engine.IdentityService().GetGroup(r.Context(), groupID); err != nil {
		writeIdentityError(w, r, err, true)
		return
	}
	// 核心移除是幂等扩展，原版 HTTP 必须先确认关系存在。
	member, err := s.identityMemberExists(r, userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !member {
		writeRuntimeFailure(w, 404, fmt.Sprintf("User '%s' is not part of group '%s'.", userID, groupID))
		return
	}
	if err := s.engine.IdentityService().RemoveMembership(r.Context(), userID, groupID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
