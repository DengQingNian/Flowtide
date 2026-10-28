package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// historicVariableSortKey 返回历史变量排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func historicVariableSortKey(field string) (func(api.HistoricVariableInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.HistoricVariableInstance) any { return item.ID }, true
	case "name":
		return func(item api.HistoricVariableInstance) any { return item.Name }, true
	case "type":
		return func(item api.HistoricVariableInstance) any { return item.Type }, true
	case "createTime":
		return func(item api.HistoricVariableInstance) any { return item.CreateTime }, true
	case "lastUpdatedTime":
		return func(item api.HistoricVariableInstance) any { return item.LastUpdatedTime }, true
	}
	return nil, false
}

// historicDetailSortKey 返回历史明细排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func historicDetailSortKey(field string) (func(api.HistoricDetail) any, bool) {
	switch field {
	case "id":
		return func(item api.HistoricDetail) any { return item.ID }, true
	case "name":
		return func(item api.HistoricDetail) any { return item.Name }, true
	case "variableType":
		return func(item api.HistoricDetail) any { return item.VariableType }, true
	case "time":
		return func(item api.HistoricDetail) any { return item.Time }, true
	}
	return nil, false
}

// handleQueryHistoricVariables 按请求体查询历史变量。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryHistoricVariables(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lookup, err := s.historicProcessIndex(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.HistoryService().ListHistoricVariableInstances(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	filtered := make([]api.HistoricVariableInstance, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ProcessInstanceID, body.str("processInstanceId")) ||
			!matchQueryString(item.ExecutionID, body.str("executionId")) ||
			!matchQueryString(item.TaskID, body.str("taskId")) ||
			!matchQueryString(item.Name, firstNonEmpty(body.str("variableName"), body.str("name"))) ||
			!matchQueryLike(item.Name, firstNonEmpty(body.str("variableNameLike"), body.str("nameLike"))) ||
			!matchQueryString(item.Type, firstNonEmpty(body.str("variableType"), body.str("type"))) {
			continue
		}
		if tenant := requestTenant(r); tenant != "" && item.ProcessInstanceID != "" && lookup.tenants[item.ProcessInstanceID] != tenant {
			continue
		}
		filtered = append(filtered, item)
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(filtered, sortField, order, historicVariableSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, filtered, start, size, sortField, order)
}

// handleQueryHistoricDetails 按请求体查询历史变量明细。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryHistoricDetails(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lookup, err := s.historicProcessIndex(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.HistoryService().ListHistoricDetails(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	after, err := body.timeField("timeAfter")
	if err != nil {
		writeError(w, err)
		return
	}
	before, err := body.timeField("timeBefore")
	if err != nil {
		writeError(w, err)
		return
	}
	filtered := make([]api.HistoricDetail, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ProcessInstanceID, body.str("processInstanceId")) ||
			!matchQueryString(item.ExecutionID, body.str("executionId")) ||
			!matchQueryString(item.TaskID, body.str("taskId")) ||
			!matchQueryString(item.ActivityID, firstNonEmpty(body.str("activityInstanceId"), body.str("activityId"))) ||
			!matchQueryString(item.Name, firstNonEmpty(body.str("variableName"), body.str("name"))) ||
			!matchQueryLike(item.Name, firstNonEmpty(body.str("variableNameLike"), body.str("nameLike"))) ||
			!matchQueryString(item.VariableType, firstNonEmpty(body.str("variableType"), body.str("type"))) {
			continue
		}
		if tenant := requestTenant(r); tenant != "" && item.ProcessInstanceID != "" && lookup.tenants[item.ProcessInstanceID] != tenant {
			continue
		}
		if !matchQueryTime(item.Time, after, before) {
			continue
		}
		filtered = append(filtered, item)
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(filtered, sortField, order, historicDetailSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, filtered, start, size, sortField, order)
}
