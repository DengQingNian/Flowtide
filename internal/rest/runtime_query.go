package rest

import (
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// serveInstanceQuery 共用实例 GET 与 POST 的过滤、实际分页和完整 DTO 输出。
// @param w 响应
// @param r 请求
// @param body 合并后的参数
// @return 无
func (s *Server) serveInstanceQuery(w http.ResponseWriter, r *http.Request, body queryBody) {
	items, err := s.engine.RuntimeService().ListProcessInstances(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterQueryProcessInstances(items, body, requestTenant(r))
	if err == nil {
		items, err = applyQueryVariableFilter(r.Context(), items, body, s.processInstanceVariables)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, field, order, err := runtimePagination(body, false)
	if err == nil {
		err = sortQueryItems(items, field, order, processInstanceSortKey)
	}
	if err != nil {
		writeRuntimeQueryError(w, err)
		return
	}
	include, err := body.boolField("includeProcessVariables")
	if err != nil {
		writeError(w, err)
		return
	}
	data := make([]map[string]any, 0)
	// 定义说明与变量包含仅对当前页补充，默认列表不能泄露内部合并变量。
	for _, instance := range runtimePage(items, start, size) {
		definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), instance.ProcessDefinitionID)
		if err != nil {
			writeError(w, err)
			return
		}
		var values map[string]any
		if include != nil && *include {
			values, err = s.engine.RuntimeService().GetVariables(r.Context(), instance.ID)
			if err != nil {
				writeError(w, err)
				return
			}
		}
		data = append(data, instanceRESTResponse(r, instance, &definition, values))
	}
	writeJSON(w, http.StatusOK, api.Page[map[string]any]{Data: data, Total: int64(len(items)), Start: start, Size: len(data), Sort: field, Order: order})
}

// serveExecutionQuery 共用执行 GET 与 POST 查询，并排除核心接口保留的已结束执行。
// @param w 响应
// @param r 请求
// @param body 合并后的参数
// @return 无
func (s *Server) serveExecutionQuery(w http.ResponseWriter, r *http.Request, body queryBody) {
	all, err := s.engine.RuntimeService().ListExecutions(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]api.ExecutionInfo, 0, len(all))
	for _, item := range all {
		if !item.Ended {
			items = append(items, item)
		}
	}
	items, err = filterQueryExecutions(items, body)
	if err == nil {
		items, err = applyQueryVariableFilter(r.Context(), items, body, s.executionVariables)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, field, order, err := runtimePagination(body, true)
	if err == nil {
		err = sortQueryItems(items, field, order, executionSortKey)
	}
	if err != nil {
		writeRuntimeQueryError(w, err)
		return
	}
	data := make([]map[string]any, 0)
	for _, execution := range runtimePage(items, start, size) {
		data = append(data, executionRESTResponse(r, execution))
	}
	writeJSON(w, http.StatusOK, api.Page[map[string]any]{Data: data, Total: int64(len(items)), Start: start, Size: len(data), Sort: field, Order: order})
}
