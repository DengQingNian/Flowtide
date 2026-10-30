package rest

import (
	"net/http"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// activityInstanceSortKey 返回活动实例排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func activityInstanceSortKey(field string) (func(api.ActivityInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.ActivityInstance) any { return item.ID }, true
	case "activityId":
		return func(item api.ActivityInstance) any { return item.ActivityID }, true
	case "activityType":
		return func(item api.ActivityInstance) any { return item.ActivityType }, true
	case "activityName":
		return func(item api.ActivityInstance) any { return item.ActivityName }, true
	case "startTime":
		return func(item api.ActivityInstance) any { return item.StartTime }, true
	}
	return nil, false
}

// filterActivityInstances 过滤活动实例。
// @param items 活动实例
// @param body 查询体
// @return 过滤结果
// @return 查询错误
func filterActivityInstances(items []api.ActivityInstance, body queryBody) ([]api.ActivityInstance, error) {
	assignee := firstNonEmpty(body.str("taskAssignee"), body.str("assignee"))
	instanceID := body.str("processInstanceId")
	definitionID := body.str("processDefinitionId")
	activityID := body.str("activityId")
	activityType := body.str("activityType")
	activityName := firstNonEmpty(body.str("activityName"), body.str("name"))
	activityNameLike := body.str("activityNameLike")
	tenantID := body.str("tenantId")
	id := body.str("id")
	out := make([]api.ActivityInstance, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.ProcessInstanceID, instanceID) ||
			!matchQueryString(item.ProcessDefinitionID, definitionID) ||
			!matchQueryString(item.ActivityID, activityID) ||
			!matchQueryString(item.ActivityType, activityType) ||
			!matchQueryString(item.ActivityName, activityName) ||
			!matchQueryLike(item.ActivityName, activityNameLike) ||
			!matchQueryString(item.TenantID, tenantID) ||
			!matchQueryString(item.Assignee, assignee) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// handleListActivityInstances 返回运行中活动实例。
// @param w 响应
// @param r 请求
func (s *Server) handleListActivityInstances(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.RuntimeService().ListActivityInstances(r.Context(), r.URL.Query().Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	query := r.URL.Query()
	filtered := make([]api.ActivityInstance, 0, len(items))
	assignee := firstNonEmpty(query.Get("taskAssignee"), query.Get("assignee"))
	for _, item := range items {
		if !matchQueryString(item.ID, query.Get("id")) ||
			!matchQueryString(item.ProcessDefinitionID, query.Get("processDefinitionId")) ||
			!matchQueryString(item.ActivityID, query.Get("activityId")) ||
			!matchQueryString(item.ActivityType, query.Get("activityType")) ||
			!matchQueryString(item.ActivityName, query.Get("activityName")) ||
			!matchQueryLike(item.ActivityName, query.Get("activityNameLike")) ||
			!matchQueryString(item.Assignee, assignee) {
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
	if err := sortQueryItems(filtered, sortField, order, activityInstanceSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, filtered, start, size, sortField, order)
}

// handleQueryActivityInstances 按请求体查询活动实例。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryActivityInstances(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.RuntimeService().ListActivityInstances(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterActivityInstances(items, body)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, activityInstanceSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleGetExecutionActivities 返回执行活动 ID。
// @param w 响应
// @param r 请求
func (s *Server) handleGetExecutionActivities(w http.ResponseWriter, r *http.Request) {
	activities, err := s.engine.RuntimeService().GetActiveActivityIDs(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, activities)
}
