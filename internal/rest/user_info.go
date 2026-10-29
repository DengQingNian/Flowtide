package rest

import (
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/flowable-go/flowable/api"
)

// userInfoRequest 是用户信息请求。
type userInfoRequest struct {
	Key   string `json:"key"`   // 键
	Value string `json:"value"` // 值
}

// userInfoResponse 是用户信息响应。
type userInfoResponse struct {
	Key   string  `json:"key"`             // 键
	Value *string `json:"value,omitempty"` // 可空值，键列表不输出值
	URL   string  `json:"url,omitempty"`   // 资源地址
}

// handleListUserInfo 返回用户信息列表。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleListUserInfo(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	info, err := s.engine.IdentityService().GetUserInfoEntries(r.Context(), userID)
	if err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	// 原版键列表包含空值键，只返回键与资源地址。
	keys := make([]string, 0, len(info))
	for key := range info {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]userInfoResponse, 0, len(keys))
	for _, key := range keys {
		out = append(out, userInfoResponse{Key: key, URL: taskResourceURL(r, "identity/users", userID) + "/info/" + key})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateUserInfo 创建用户信息。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleCreateUserInfo(w http.ResponseWriter, r *http.Request) {
	var payload userInfoRequest
	body, err := readIdentityPatch(r, &payload)
	if err != nil {
		writeError(w, err)
		return
	}
	userID := r.PathValue("id")
	if _, err := s.engine.IdentityService().GetUser(r.Context(), userID); err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	// 先按原版顺序校验键和值，空字符串与缺省值不能混同。
	if raw, ok := body["key"]; !ok || string(raw) == "null" {
		writeRuntimeFailure(w, 400, "The key cannot be null.")
		return
	}
	if raw, ok := body["value"]; !ok || string(raw) == "null" {
		writeRuntimeFailure(w, 400, "The value cannot be null.")
		return
	}
	if _, err := s.engine.IdentityService().GetUserInfoValue(r.Context(), userID, payload.Key); err == nil {
		writeRuntimeFailure(w, 409, fmt.Sprintf("User info with key '%s' already exists for this user.", payload.Key))
		return
	} else if !isNotFound(err) {
		writeError(w, err)
		return
	}
	if err := s.engine.IdentityService().SetUserInfo(r.Context(), userID, payload.Key, payload.Value); err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	writeJSON(w, http.StatusCreated, userInfoResponse{Key: payload.Key, Value: &payload.Value, URL: taskResourceURL(r, "identity/users", userID) + "/info/" + payload.Key})
}

// handleGetUserInfo 返回单个用户信息。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, key := r.PathValue("id"), r.PathValue("key")
	value, err := s.engine.IdentityService().GetUserInfoValue(r.Context(), userID, key)
	if err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	writeJSON(w, http.StatusOK, userInfoResponse{Key: key, Value: &value, URL: taskResourceURL(r, "identity/users", userID) + "/info/" + key})
}

// handleUpdateUserInfo 更新用户信息。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleUpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	var payload userInfoRequest
	body, err := readIdentityPatch(r, &payload)
	if err != nil {
		writeError(w, err)
		return
	}
	userID, key := r.PathValue("id"), r.PathValue("key")
	if _, err := s.engine.IdentityService().GetUserInfoValue(r.Context(), userID, key); err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	// 值必须提供；请求体中的可选键只用于一致性检查。
	if raw, ok := body["value"]; !ok || string(raw) == "null" {
		writeRuntimeFailure(w, 400, "The value cannot be null.")
		return
	}
	if raw, ok := body["key"]; ok && string(raw) != "null" && payload.Key != key {
		writeRuntimeFailure(w, 400, "Key provided in request body does not match the key in the resource URL.")
		return
	}
	if err := s.engine.IdentityService().SetUserInfo(r.Context(), userID, key, payload.Value); err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	writeJSON(w, http.StatusOK, userInfoResponse{Key: key, Value: &payload.Value, URL: taskResourceURL(r, "identity/users", userID) + "/info/" + key})
}

// handleDeleteUserInfo 删除用户信息。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleDeleteUserInfo(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().DeleteUserInfo(r.Context(), r.PathValue("id"), r.PathValue("key")); err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetUserPicture 返回用户头像。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleGetUserPicture(w http.ResponseWriter, r *http.Request) {
	data, mimeType, err := s.engine.IdentityService().GetUserPicture(r.Context(), r.PathValue("id"))
	if err != nil {
		writeIdentityError(w, r, err, false)
		return
	}
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	w.Header().Set("Content-Type", mimeType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleSetUserPicture 设置用户头像。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleSetUserPicture(w http.ResponseWriter, r *http.Request) {
	data, ok, err := readMultipartSource(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeError(w, api.NewError("REST-0129", "缺少上传文件", api.ErrBadRequest))
		return
	}
	mimeType := ""
	if r.MultipartForm != nil {
		// 原版使用文件部分媒体类型，额外 mimeType 表单字段不覆盖该值。
		for _, files := range r.MultipartForm.File {
			if len(files) > 0 {
				mimeType = files[0].Header.Get("Content-Type")
				break
			}
		}
	}
	if err := s.engine.IdentityService().SetUserPicture(r.Context(), r.PathValue("id"), data, mimeType); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// isNotFound 判断错误是否为未找到。
// @param err 错误
// @return 是否未找到
func isNotFound(err error) bool {
	return errors.Is(err, api.ErrNotFound)
}
