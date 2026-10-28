package rest

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// handleGetComment 返回单条评论。
// @param w 响应
// @param r 请求
func (s *Server) handleGetComment(w http.ResponseWriter, r *http.Request) {
	comment, err := s.engine.TaskService().GetComment(r.Context(), r.PathValue("commentId"))
	if err != nil {
		writeError(w, err)
		return
	}
	// 评论必须属于路径中的任务
	if comment.TaskID != r.PathValue("id") {
		writeError(w, api.NewError("ENGINE-0404", "评论不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, comment)
}

// handleGetAttachment 返回单个附件。
// @param w 响应
// @param r 请求
func (s *Server) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	attachment, err := s.engine.TaskService().GetAttachment(r.Context(), r.PathValue("attachmentId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if attachment.TaskID != r.PathValue("id") {
		writeError(w, api.NewError("ENGINE-0404", "附件不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, attachment)
}

// handleListIdentityLinksFamily 按用户/组返回任务参与者。
// @param w 响应
// @param r 请求
func (s *Server) handleListIdentityLinksFamily(w http.ResponseWriter, r *http.Request) {
	family := r.PathValue("family")
	if family != "users" && family != "groups" {
		writeError(w, api.NewError("REST-0127", "参与者类型非法: "+family, api.ErrBadRequest))
		return
	}
	links, err := s.engine.TaskService().ListIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	filtered := make([]api.IdentityLink, 0, len(links))
	for _, link := range links {
		if family == "users" && link.UserID != "" {
			filtered = append(filtered, link)
		}
		if family == "groups" && link.GroupID != "" {
			filtered = append(filtered, link)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

// eventSubscriptionSortKey 返回事件订阅排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func eventSubscriptionSortKey(field string) (func(api.EventSubscription) any, bool) {
	switch field {
	case "id":
		return func(item api.EventSubscription) any { return item.ID }, true
	case "created":
		return func(item api.EventSubscription) any { return item.Created }, true
	case "executionId":
		return func(item api.EventSubscription) any { return item.ExecutionID }, true
	case "processInstanceId":
		return func(item api.EventSubscription) any { return item.ProcessInstanceID }, true
	case "processDefinitionId":
		return func(item api.EventSubscription) any { return item.ProcessDefinitionID }, true
	case "tenantId":
		return func(item api.EventSubscription) any { return item.TenantID }, true
	}
	return nil, false
}

// handleListEventSubscriptions 查询事件订阅。
// @param w 响应
// @param r 请求
func (s *Server) handleListEventSubscriptions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	items, err := s.engine.RuntimeService().ListEventSubscriptions(r.Context(), query.Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	createdAfter, createdBefore, err := parseEventTimeRange(query.Get("createdAfter"), query.Get("createdBefore"))
	if err != nil {
		writeError(w, err)
		return
	}
	withoutInstance := query.Get("withoutProcessInstanceId") == "true"
	withoutDefinition := query.Get("withoutProcessDefinitionId") == "true"
	withoutTenant := query.Get("withoutTenantId") == "true"
	filtered := make([]api.EventSubscription, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, query.Get("id")) ||
			!matchQueryString(item.EventType, query.Get("eventType")) ||
			!matchQueryString(item.EventName, query.Get("eventName")) ||
			!matchQueryString(item.ActivityID, query.Get("activityId")) ||
			!matchQueryString(item.ExecutionID, query.Get("executionId")) ||
			!matchQueryString(item.ProcessInstanceID, query.Get("processInstanceId")) ||
			!matchQueryString(item.ProcessDefinitionID, query.Get("processDefinitionId")) {
			continue
		}
		if withoutInstance && item.ProcessInstanceID != "" {
			continue
		}
		if withoutDefinition && item.ProcessDefinitionID != "" {
			continue
		}
		if withoutTenant && item.TenantID != "" {
			continue
		}
		if !matchQueryTime(item.Created, createdAfter, createdBefore) {
			continue
		}
		filtered = append(filtered, item)
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
	if err := sortQueryItems(filtered, sortField, order, eventSubscriptionSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, filtered, start, size, sortField, order)
}

// parseEventTimeRange 解析订阅创建时间范围。
// @param after 下界字符串
// @param before 上界字符串
// @return 下界
// @return 上界
// @return 解析错误
func parseEventTimeRange(after, before string) (*time.Time, *time.Time, error) {
	var afterTime, beforeTime *time.Time
	if after != "" {
		parsed, err := parseTaskTime(after)
		if err != nil {
			return nil, nil, err
		}
		afterTime = &parsed
	}
	if before != "" {
		parsed, err := parseTaskTime(before)
		if err != nil {
			return nil, nil, err
		}
		beforeTime = &parsed
	}
	return afterTime, beforeTime, nil
}

// handleGetEventSubscription 返回单条事件订阅。
// @param w 响应
// @param r 请求
func (s *Server) handleGetEventSubscription(w http.ResponseWriter, r *http.Request) {
	subscription, err := s.engine.RuntimeService().GetEventSubscription(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, subscription)
}

// bulkDeletePayload 是批量删除请求体。
type bulkDeletePayload struct {
	ProcessInstanceIDs []string `json:"processInstanceIds"` // 实例 ID 列表
	DeleteReason       string   `json:"deleteReason"`       // 删除原因
}

// handleBulkDeleteInstances 批量删除运行中实例。
// @param w 响应
// @param r 请求
func (s *Server) handleBulkDeleteInstances(w http.ResponseWriter, r *http.Request) {
	var payload bulkDeletePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if len(payload.ProcessInstanceIDs) == 0 {
		writeError(w, api.NewError("REST-0128", "processInstanceIds 不能为空", api.ErrBadRequest))
		return
	}
	for _, instanceID := range payload.ProcessInstanceIDs {
		if err := s.engine.RuntimeService().DeleteProcessInstance(r.Context(), instanceID, payload.DeleteReason); err != nil {
			writeError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBulkDeleteHistoricInstances 批量删除历史实例。
// @param w 响应
// @param r 请求
func (s *Server) handleBulkDeleteHistoricInstances(w http.ResponseWriter, r *http.Request) {
	var payload bulkDeletePayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if len(payload.ProcessInstanceIDs) == 0 {
		writeError(w, api.NewError("REST-0128", "processInstanceIds 不能为空", api.ErrBadRequest))
		return
	}
	for _, instanceID := range payload.ProcessInstanceIDs {
		if err := s.engine.HistoryService().DeleteHistoricProcessInstance(r.Context(), instanceID); err != nil {
			writeError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDefinitionResourceData 下载流程定义原始资源。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionResourceData(w http.ResponseWriter, r *http.Request) {
	definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	content, err := s.engine.RepositoryService().GetProcessDefinitionXML(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	filename := definition.ResourceName
	if filename == "" {
		filename = definition.Key + ".bpmn20.xml"
	}
	filename = strings.ReplaceAll(filename, "\x22", "_")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
