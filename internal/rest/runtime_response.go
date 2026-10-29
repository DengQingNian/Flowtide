package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/flowable-go/flowable/api"
)

// runtimeNullableString 将未设置的运行时属性表示为原版显式空值。
// @param value 属性值
// @return 字符串或空值
func runtimeNullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// instanceRESTResponse 生成完整实例 DTO；动作响应不补查定义名称及说明。
// @param r 当前请求
// @param instance 实例快照
// @param definition 可空定义快照
// @param variables 需要返回的变量，空映射输出空数组
// @return 原版实例对象
func instanceRESTResponse(r *http.Request, instance api.ProcessInstance, definition *api.ProcessDefinition, variables map[string]any) map[string]any {
	response := map[string]any{
		"id": instance.ID, "url": taskResourceURL(r, "runtime/process-instances", instance.ID),
		"processDefinitionId":   instance.ProcessDefinitionID,
		"processDefinitionUrl":  taskResourceURL(r, "repository/process-definitions", instance.ProcessDefinitionID),
		"processDefinitionName": nil, "processDefinitionDescription": nil,
		"startTime": instance.StartTime.UTC().Truncate(time.Millisecond), "dueDate": taskRESTTime(instance.DueDate),
		"claimTime": taskRESTTime(instance.ClaimTime), "ended": instance.Ended, "completed": instance.Ended,
		"suspended": instance.Suspended, "tenantId": instance.TenantID,
		"variables": runtimeRESTVariables(r, instance.ID, variables),
	}
	for field, value := range map[string]string{
		"activityId": instance.ActivityID, "businessKey": instance.BusinessKey, "businessStatus": instance.BusinessStatus,
		"name": instance.Name, "startUserId": instance.StartUserID, "claimedBy": instance.ClaimedBy,
		"callbackId": instance.CallbackID, "callbackType": instance.CallbackType,
		"referenceId": instance.ReferenceID, "referenceType": instance.ReferenceType,
		"propagatedStageInstanceId": instance.PropagatedStageInstanceID, "superProcessInstanceId": instance.SuperProcessInstanceID,
	} {
		response[field] = runtimeNullableString(value)
	}
	if definition != nil {
		response["processDefinitionName"] = runtimeNullableString(definition.Name)
		response["processDefinitionDescription"] = runtimeNullableString(definition.Description)
	}
	return response
}

// runtimeRESTVariables 将变量集合转换为带类型的本地作用域对象，日期采用 UTC 毫秒。
// @param r 当前请求
// @param instanceID 实例标识
// @param values 变量集合
// @return 变量数组；名称排序仅提供稳定输出，不构成变量集合的顺序契约
func runtimeRESTVariables(r *http.Request, instanceID string, values map[string]any) []map[string]any {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]map[string]any, 0, len(names))
	for _, name := range names {
		value, kind := values[name], any(nil)
		// 依据实际持久化类型选择转换器，不能把长整数经过浮点数再输出。
		switch typed := value.(type) {
		case nil:
		case string:
			kind = "string"
		case bool:
			kind = "boolean"
		case int16:
			kind = "short"
		case int32:
			kind = "integer"
		case int, int64:
			kind = "long"
		case float32, float64:
			kind = "double"
		case time.Time:
			kind, value = "date", typed.UTC().Truncate(time.Millisecond)
		case []byte:
			kind, value = "binary", nil
		default:
			kind = "json"
		}
		variable := map[string]any{"name": name, "scope": "local", "type": kind, "value": value}
		if kind == "binary" {
			variable["valueUrl"] = taskResourceURL(r, "runtime/process-instances", instanceID) + "/variables/" + name + "/data"
		}
		result = append(result, variable)
	}
	return result
}

// executionRESTResponse 生成原版执行对象，未设置的父执行及上级执行保留空引用。
// @param r 当前请求
// @param execution 执行快照
// @return 原版执行对象
func executionRESTResponse(r *http.Request, execution api.ExecutionInfo) map[string]any {
	response := map[string]any{
		"id": execution.ID, "url": taskResourceURL(r, "runtime/executions", execution.ID),
		"activityId": runtimeNullableString(execution.ActivityID), "suspended": execution.Suspended, "tenantId": execution.TenantID,
	}
	for _, reference := range []struct{ field, path, id string }{
		{"parent", "runtime/executions", execution.ParentID}, {"superExecution", "runtime/executions", execution.SuperExecutionID},
		{"processInstance", "runtime/process-instances", execution.ProcessInstanceID},
	} {
		response[reference.field+"Id"], response[reference.field+"Url"] = nil, nil
		if reference.id != "" {
			response[reference.field+"Id"] = reference.id
			response[reference.field+"Url"] = taskResourceURL(r, reference.path, reference.id)
		}
	}
	return response
}

// writeRuntimeFailure 输出有原版运行结果依据的状态码和错误正文。
// @param w 响应
// @param status HTTP 状态
// @param exception 原版异常说明
// @return 无
func writeRuntimeFailure(w http.ResponseWriter, status int, exception string) {
	writeJSON(w, status, map[string]string{"message": map[int]string{400: "Bad request", 404: "Not found", 409: "Conflict", 500: "Internal server error"}[status], "exception": exception})
}

// writeRuntimeError 转换资源不存在及实例签收错误，其余错误保留公共语义。
// @param w 响应
// @param r 请求
// @param err 引擎错误
// @param execution 是否执行资源
// @return 无
func writeRuntimeError(w http.ResponseWriter, r *http.Request, err error, execution bool) {
	if errors.Is(err, api.ErrNotFound) {
		resource := "a process instance"
		if execution {
			resource = "an execution"
		}
		writeRuntimeFailure(w, 404, fmt.Sprintf("Could not find %s with id '%s'.", resource, r.PathValue("id")))
		return
	}
	var detail *api.Error
	if errors.As(err, &detail) && detail.Code == "ENGINE-1131" {
		// 固定原版对实例重复签收抛出通用异常，HTTP 为 500；核心接口仍保留冲突哨兵。
		writeRuntimeFailure(w, 500, fmt.Sprintf("Process instance '%s' is already claimed.", r.PathValue("id")))
		return
	}
	writeError(w, err)
}

// runtimeQueryFromRequest 合并分页并将原版 POST 实例字段名转换为内部过滤名称。
// @param r 当前请求
// @param body 查询体
// @param execution 是否执行查询
// @return 合并后的查询
// @return 参数错误
func runtimeQueryFromRequest(r *http.Request, body queryBody, execution bool) (queryBody, error) {
	if r.Method == http.MethodPost && !execution {
		// 原版实例 POST 使用 processBusinessKey 等字段；GET 的简写不是其请求体别名。
		for official, internal := range map[string]string{"processInstanceId": "id", "processInstanceName": "name", "processInstanceNameLike": "nameLike", "processBusinessKey": "businessKey", "processBusinessKeyLike": "businessKeyLike"} {
			delete(body, internal)
			if value, ok := body[official]; ok {
				body[internal] = value
			}
		}
	}
	body, err := taskQueryFromRequest(r, body)
	if err != nil {
		return nil, err
	}
	if r.Method == http.MethodGet {
		for _, key := range []string{"suspended", "includeProcessVariables", "active", "ended", "waiting"} {
			if text := r.URL.Query().Get(key); text != "" {
				body[key] = json.RawMessage(text)
			}
		}
	}
	return body, nil
}

// runtimePagination 解析原版运行时分页与排序白名单，允许零数量及超过一千条的请求。
// @param body 查询参数
// @param execution 是否执行查询
// @return 起点
// @return 数量
// @return 排序字段
// @return 排序方向
// @return 参数错误
func runtimePagination(body queryBody, execution bool) (int, int, string, string, error) {
	start, size, field, order, err := body.pagination()
	if err != nil {
		return 0, 0, "", "", err
	}
	// 通用分页有一千条上限，原版运行时允许更大数量，零数量也必须保留。
	if requested, err := body.intField("size"); err != nil {
		return 0, 0, "", "", err
	} else if requested != nil && *requested >= 0 {
		size = *requested
	}
	allowed := map[string]bool{"id": true, "processDefinitionId": true, "processDefinitionKey": true, "startTime": true, "tenantId": true, "businessKey": true}
	defaultField := "id"
	if execution {
		defaultField = "processInstanceId"
		allowed = map[string]bool{"processDefinitionId": true, "processDefinitionKey": true, "processInstanceId": true, "tenantId": true}
	}
	if field == "" {
		field = defaultField
	}
	if !allowed[field] {
		return 0, 0, "", "", api.NewError("REST-RUNTIME-SORT", fmt.Sprintf("Value for param 'sort' is not valid, '%s' is not a valid property", field), api.ErrBadRequest)
	}
	return start, size, field, order, nil
}

// writeRuntimeQueryError 仅对原版排序白名单错误输出其明确异常说明。
// @param w 响应
// @param err 参数或引擎错误
// @return 无
func writeRuntimeQueryError(w http.ResponseWriter, err error) {
	var detail *api.Error
	if errors.As(err, &detail) && detail.Code == "REST-RUNTIME-SORT" {
		writeRuntimeFailure(w, 400, detail.Message)
		return
	}
	writeError(w, err)
}

// runtimePage 安全截取运行时结果并返回实际条数，避免极大分页值溢出。
// @param items 已过滤排序结果
// @param start 起点
// @param size 请求数量
// @return 当前页
func runtimePage[T any](items []T, start, size int) []T {
	if start >= len(items) {
		return []T{}
	}
	end := len(items)
	if size < len(items)-start {
		end = start + size
	}
	return items[start:end]
}
