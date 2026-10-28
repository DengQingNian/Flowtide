package rest

import (
	"errors"
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
	Key   string `json:"key"`           // 键
	Value string `json:"value"`         // 值
	URL   string `json:"url,omitempty"` // 资源地址
}

// handleListUserInfo 返回用户信息列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListUserInfo(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	info, err := s.engine.IdentityService().GetUserInfo(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	keys := make([]string, 0, len(info))
	for key := range info {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]userInfoResponse, 0, len(keys))
	for _, key := range keys {
		out = append(out, userInfoResponse{Key: key, Value: info[key], URL: "/service/identity/users/" + userID + "/info/" + key})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateUserInfo 创建用户信息。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateUserInfo(w http.ResponseWriter, r *http.Request) {
	var payload userInfoRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	userID := r.PathValue("id")
	if payload.Key == "" {
		writeError(w, api.NewError("ENGINE-1702", "用户信息键不能为空", api.ErrBadRequest))
		return
	}
	if _, err := s.engine.IdentityService().GetUserInfoValue(r.Context(), userID, payload.Key); err == nil {
		writeError(w, api.NewError("ENGINE-1705", "用户信息已存在", api.ErrConflict))
		return
	} else if !isNotFound(err) {
		writeError(w, err)
		return
	}
	if err := s.engine.IdentityService().SetUserInfo(r.Context(), userID, payload.Key, payload.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, userInfoResponse{Key: payload.Key, Value: payload.Value, URL: "/service/identity/users/" + userID + "/info/" + payload.Key})
}

// handleGetUserInfo 返回单个用户信息。
// @param w 响应
// @param r 请求
func (s *Server) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	key := r.PathValue("key")
	value, err := s.engine.IdentityService().GetUserInfoValue(r.Context(), userID, key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userInfoResponse{Key: key, Value: value, URL: "/service/identity/users/" + userID + "/info/" + key})
}

// handleUpdateUserInfo 更新用户信息。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateUserInfo(w http.ResponseWriter, r *http.Request) {
	var payload userInfoRequest
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	userID := r.PathValue("id")
	key := r.PathValue("key")
	if payload.Key != "" && payload.Key != key {
		writeError(w, api.NewError("ENGINE-1706", "请求体 key 与路径不一致", api.ErrBadRequest))
		return
	}
	if _, err := s.engine.IdentityService().GetUserInfoValue(r.Context(), userID, key); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.IdentityService().SetUserInfo(r.Context(), userID, key, payload.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userInfoResponse{Key: key, Value: payload.Value, URL: "/service/identity/users/" + userID + "/info/" + key})
}

// handleDeleteUserInfo 删除用户信息。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteUserInfo(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().DeleteUserInfo(r.Context(), r.PathValue("id"), r.PathValue("key")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetUserPicture 返回用户头像。
// @param w 响应
// @param r 请求
func (s *Server) handleGetUserPicture(w http.ResponseWriter, r *http.Request) {
	data, mimeType, err := s.engine.IdentityService().GetUserPicture(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
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
		if values := r.MultipartForm.Value["mimeType"]; len(values) > 0 {
			mimeType = values[0]
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
