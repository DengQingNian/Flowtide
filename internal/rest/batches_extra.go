package rest

import (
	"net/http"
	"strconv"

	"github.com/flowable-go/flowable/api"
)

// handleListBatches 返回批处理作业列表。
// 支持 type、status、tenantId 过滤以及 page/size 分页。
// @param w 响应
// @param r 请求
func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := s.engine.ManagementService().ListBatches(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	query := r.URL.Query()
	tenantFilter := query.Get("tenantId")
	if tenantFilter == "" {
		tenantFilter = r.Header.Get("X-Tenant-Id")
	}
	batches = filterBatches(batches, query.Get("type"), query.Get("status"), tenantFilter)
	start, size := batchPageParams(r)
	writeJSON(w, http.StatusOK, paginateBatches(batches, start, size))
}

// handleGetBatch 返回单个批处理作业。
// @param w 响应
// @param r 请求
func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	batch, err := s.engine.ManagementService().GetBatch(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

// handleListBatchParts 返回批次下的分片列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListBatchParts(w http.ResponseWriter, r *http.Request) {
	parts, err := s.engine.ManagementService().ListBatchParts(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := batchPageParams(r)
	writeJSON(w, http.StatusOK, paginateBatchParts(parts, start, size))
}

// handleGetBatchPart 返回单个批处理分片。
// @param w 响应
// @param r 请求
func (s *Server) handleGetBatchPart(w http.ResponseWriter, r *http.Request) {
	part, err := s.engine.ManagementService().GetBatchPart(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, part)
}

// handleBatchSummary 返回批次统计信息。
// @param w 响应
// @param r 请求
func (s *Server) handleBatchSummary(w http.ResponseWriter, r *http.Request) {
	batch, err := s.engine.ManagementService().GetBatch(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	parts, err := s.engine.ManagementService().ListBatchParts(r.Context(), batch.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	completed, failed, queued := 0, 0, 0
	for _, part := range parts {
		switch part.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		case "queued":
			queued++
		}
	}
	inProgress := batch.Total - completed - failed
	if inProgress < 0 {
		inProgress = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         batch.ID,
		"batchId":    batch.ID,
		"type":       batch.Type,
		"status":     batch.Status,
		"total":      batch.Total,
		"completed":  completed,
		"failed":     failed,
		"inProgress": inProgress,
		"queued":     queued,
		"startTime":  batch.StartTime,
		"endTime":    batch.EndTime,
		"tenantId":   batch.TenantID,
	})
}

// filterBatches 按类型、状态、租户过滤批次。
// @param items 批次列表
// @param batchType 批次类型，空串表示不过滤
// @param status 批次状态，空串表示不过滤
// @param tenant 租户，空串表示不过滤
// @return 过滤后的批次列表
func filterBatches(items []api.Batch, batchType, status, tenant string) []api.Batch {
	if batchType == "" && status == "" && tenant == "" {
		return items
	}
	out := make([]api.Batch, 0, len(items))
	for _, item := range items {
		// 多个条件同时存在时要求全部匹配
		if batchType != "" && item.Type != batchType {
			continue
		}
		if status != "" && item.Status != status {
			continue
		}
		if tenant != "" && item.TenantID != tenant {
			continue
		}
		out = append(out, item)
	}
	return out
}

// batchPageParams 解析批次分页参数。
// 支持 Flowable 的 page/size，也兼容 start/size 写法。
// @param r 请求
// @return 偏移
// @return 每页数量
func batchPageParams(r *http.Request) (int, int) {
	query := r.URL.Query()
	size, _ := strconv.Atoi(query.Get("size"))
	if size <= 0 {
		size = 10
	}
	if size > 1000 {
		size = 1000
	}
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 0 {
		page = 0
	}
	start := page * size
	// 显式 start 优先，兼容既有 REST 分页习惯
	if raw := query.Get("start"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			start = parsed
		}
	}
	return start, size
}

// paginateBatches 分页批次列表。
// @param items 批次列表
// @param start 偏移
// @param size 每页数量
// @return 分页结果
func paginateBatches(items []api.Batch, start, size int) api.Page[api.Batch] {
	total := int64(len(items))
	if start >= len(items) {
		return api.Page[api.Batch]{Data: []api.Batch{}, Total: total, Start: start, Size: size}
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return api.Page[api.Batch]{Data: items[start:end], Total: total, Start: start, Size: size}
}

// paginateBatchParts 分页批次分片。
// @param items 分片列表
// @param start 偏移
// @param size 每页数量
// @return 分页结果
func paginateBatchParts(items []api.BatchPart, start, size int) api.Page[api.BatchPart] {
	total := int64(len(items))
	if start >= len(items) {
		return api.Page[api.BatchPart]{Data: []api.BatchPart{}, Total: total, Start: start, Size: size}
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return api.Page[api.BatchPart]{Data: items[start:end], Total: total, Start: start, Size: size}
}
