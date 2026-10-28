package rest

import (
	"net/http"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// variableInstanceSortKey 返回变量实例排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func variableInstanceSortKey(field string) (func(api.VariableInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.VariableInstance) any { return item.ID }, true
	case "name":
		return func(item api.VariableInstance) any { return item.Name }, true
	case "type":
		return func(item api.VariableInstance) any { return item.Type }, true
	case "processInstanceId":
		return func(item api.VariableInstance) any { return item.ProcessInstanceID }, true
	case "executionId":
		return func(item api.VariableInstance) any { return item.ExecutionID }, true
	case "taskId":
		return func(item api.VariableInstance) any { return item.TaskID }, true
	}
	return nil, false
}

// handleListVariableInstances 返回运行时变量实例。
// @param w 响应
// @param r 请求
func (s *Server) handleListVariableInstances(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := api.VariableInstanceFilter{
		ProcessInstanceID: query.Get("processInstanceId"),
		ExecutionID:       query.Get("executionId"),
		TaskID:            query.Get("taskId"),
		Name:              firstNonEmpty(query.Get("variableName"), query.Get("name")),
		NameLike:          firstNonEmpty(query.Get("variableNameLike"), query.Get("nameLike")),
	}
	items, err := s.engine.RuntimeService().ListVariableInstances(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
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
	if err := sortQueryItems(items, sortField, order, variableInstanceSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleQueryVariableInstances 按请求体查询变量实例。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryVariableInstances(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	filter := api.VariableInstanceFilter{
		ProcessInstanceID: body.str("processInstanceId"),
		ExecutionID:       body.str("executionId"),
		TaskID:            body.str("taskId"),
		Name:              firstNonEmpty(body.str("variableName"), body.str("name")),
		NameLike:          firstNonEmpty(body.str("variableNameLike"), body.str("nameLike")),
	}
	items, err := s.engine.RuntimeService().ListVariableInstances(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, variableInstanceSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleGetVariableInstanceData 返回变量实例原始值。
// @param w 响应
// @param r 请求
func (s *Server) handleGetVariableInstanceData(w http.ResponseWriter, r *http.Request) {
	item, err := s.engine.RuntimeService().GetVariableInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeVariableData(w, item.Value)
}
