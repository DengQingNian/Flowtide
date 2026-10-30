package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// modelCreatePayload 是模型创建请求。
type modelCreatePayload struct {
	Name     string `json:"name"`     // 名称
	Key      string `json:"key"`      // Key
	Category string `json:"category"` // 分类
	MetaInfo string `json:"metaInfo"` // 元信息
	TenantID string `json:"tenantId"` // 租户
	Version  int    `json:"version"`  // 版本
}

// handleListModels 查询流程模型。
// @param w 响应
// @param r 请求
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.RepositoryService().ListModels(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	query := r.URL.Query()
	version := 0
	if value := query.Get("version"); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			writeError(w, api.NewError("REST-0121", "version 必须为整数", api.ErrBadRequest))
			return
		}
		version = parsed
	}
	filtered := make([]api.Model, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, query.Get("id")) ||
			!matchQueryString(item.Category, query.Get("category")) ||
			!matchQueryLike(item.Category, query.Get("categoryLike")) ||
			!matchQueryString(item.Name, query.Get("name")) ||
			!matchQueryLike(item.Name, query.Get("nameLike")) ||
			!matchQueryString(item.Key, query.Get("key")) ||
			!matchQueryString(item.DeploymentID, query.Get("deploymentId")) ||
			!matchQueryString(item.TenantID, query.Get("tenantId")) ||
			!matchQueryLike(item.TenantID, query.Get("tenantIdLike")) {
			continue
		}
		if excluded := query.Get("categoryNotEquals"); excluded != "" && item.Category == excluded {
			continue
		}
		if version != 0 && item.Version != version {
			continue
		}
		if query.Get("deployed") == "true" && item.DeploymentID == "" {
			continue
		}
		if query.Get("deployed") == "false" && item.DeploymentID != "" {
			continue
		}
		if query.Get("withoutTenantId") == "true" && item.TenantID != "" {
			continue
		}
		filtered = append(filtered, item)
	}
	if query.Get("latestVersion") == "true" {
		filtered = latestModels(filtered)
	}
	start, size := pageParams(r)
	sortField := query.Get("sort")
	order := strings.ToLower(query.Get("order"))
	if order == "" {
		order = "asc"
	}
	if order != "asc" && order != "desc" {
		writeError(w, api.NewError("REST-0123", "排序方向非法: "+order, api.ErrBadRequest))
		return
	}
	if err := sortQueryItems(filtered, sortField, order, modelSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, filtered, start, size, sortField, order)
}

// latestModels 仅保留每个 Key 的最新版本。
// @param items 模型列表
// @return 过滤结果
func latestModels(items []api.Model) []api.Model {
	maxVersions := make(map[string]int)
	for _, item := range items {
		if item.Version > maxVersions[item.Key] {
			maxVersions[item.Key] = item.Version
		}
	}
	out := make([]api.Model, 0, len(items))
	for _, item := range items {
		if item.Version == maxVersions[item.Key] {
			out = append(out, item)
		}
	}
	return out
}

// modelSortKey 返回模型排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func modelSortKey(field string) (func(api.Model) any, bool) {
	switch field {
	case "id":
		return func(item api.Model) any { return item.ID }, true
	case "category":
		return func(item api.Model) any { return item.Category }, true
	case "createTime":
		return func(item api.Model) any { return item.CreateTime }, true
	case "key":
		return func(item api.Model) any { return item.Key }, true
	case "lastUpdateTime":
		return func(item api.Model) any { return item.LastUpdateTime }, true
	case "name":
		return func(item api.Model) any { return item.Name }, true
	case "version":
		return func(item api.Model) any { return item.Version }, true
	case "tenantId":
		return func(item api.Model) any { return item.TenantID }, true
	}
	return nil, false
}

// handleCreateModel 创建流程模型。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	var payload modelCreatePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	model, err := s.engine.RepositoryService().CreateModel(r.Context(), api.CreateModelRequest{
		Name:     payload.Name,
		Key:      payload.Key,
		Category: payload.Category,
		MetaInfo: payload.MetaInfo,
		TenantID: firstNonEmpty(payload.TenantID, requestTenant(r)),
		Version:  payload.Version,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, model)
}

// handleGetModel 返回流程模型。
// @param w 响应
// @param r 请求
func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	model, err := s.engine.RepositoryService().GetModel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model)
}

// handleUpdateModel 更新流程模型。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	var payload map[string]json.RawMessage
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	update := api.ModelUpdate{}
	for key, target := range map[string]**string{
		"name":         &update.Name,
		"key":          &update.Key,
		"category":     &update.Category,
		"metaInfo":     &update.MetaInfo,
		"deploymentId": &update.DeploymentID,
		"tenantId":     &update.TenantID,
	} {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		value, err := decodeNullableString(raw)
		if err != nil {
			writeError(w, err)
			return
		}
		*target = &value
	}
	if raw, ok := payload["version"]; ok && string(raw) != "null" {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			writeError(w, api.NewError("REST-0121", "version 必须为整数", api.ErrBadRequest))
			return
		}
		update.Version = &value
	}
	model, err := s.engine.RepositoryService().UpdateModel(r.Context(), r.PathValue("id"), update)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, model)
}

// handleDeleteModel 删除流程模型。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RepositoryService().DeleteModel(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetModelSource 返回模型编辑源。
// @param w 响应
// @param r 请求
func (s *Server) handleGetModelSource(w http.ResponseWriter, r *http.Request) {
	s.writeModelSource(w, r, false)
}

// handleSetModelSource 设置模型编辑源。
// @param w 响应
// @param r 请求
func (s *Server) handleSetModelSource(w http.ResponseWriter, r *http.Request) {
	s.updateModelSource(w, r, false)
}

// handleGetModelSourceExtra 返回模型扩展源码。
// @param w 响应
// @param r 请求
func (s *Server) handleGetModelSourceExtra(w http.ResponseWriter, r *http.Request) {
	s.writeModelSource(w, r, true)
}

// handleSetModelSourceExtra 设置模型扩展源码。
// @param w 响应
// @param r 请求
func (s *Server) handleSetModelSourceExtra(w http.ResponseWriter, r *http.Request) {
	s.updateModelSource(w, r, true)
}

// writeModelSource 输出模型源。
// @param w 响应
// @param r 请求
// @param extra 是否扩展源
func (s *Server) writeModelSource(w http.ResponseWriter, r *http.Request, extra bool) {
	content, err := s.engine.RepositoryService().GetModelSource(r.Context(), r.PathValue("id"))
	if extra {
		content, err = s.engine.RepositoryService().GetModelSourceExtra(r.Context(), r.PathValue("id"))
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// updateModelSource 更新模型源。
// @param w 响应
// @param r 请求
// @param extra 是否扩展源
func (s *Server) updateModelSource(w http.ResponseWriter, r *http.Request, extra bool) {
	content, ok, err := readMultipartSource(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeError(w, api.NewError("REST-0129", "缺少上传文件", api.ErrBadRequest))
		return
	}
	if extra {
		err = s.engine.RepositoryService().SetModelSourceExtra(r.Context(), r.PathValue("id"), content)
	} else {
		err = s.engine.RepositoryService().SetModelSource(r.Context(), r.PathValue("id"), content)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readMultipartSource 读取 multipart 上传的第一个文件。
// @param r 请求
// @return 文件内容
// @return 是否存在文件
// @return 解析错误
func readMultipartSource(r *http.Request) ([]byte, bool, error) {
	if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
		return nil, false, api.NewError("REST-0130", "multipart 请求非法", err)
	}
	for _, headers := range r.MultipartForm.File {
		if len(headers) == 0 {
			continue
		}
		file, err := headers[0].Open()
		if err != nil {
			return nil, false, api.NewError("REST-0130", "读取上传文件失败", err)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, MaxUploadSize))
		_ = file.Close()
		if readErr != nil {
			return nil, false, api.NewError("REST-0130", "读取上传文件失败", readErr)
		}
		return content, true, nil
	}
	return nil, false, nil
}
