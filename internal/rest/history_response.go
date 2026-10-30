package rest

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// historicNullable 将未设置的文本表示为原版显式空值。
// @param value 属性值
// @return 字符串或空值
func historicNullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// historicNullableTime 将未设置的时间表示为原版显式空值。
// @param value 时间
// @return UTC 毫秒时间或空值
func historicNullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Truncate(time.Millisecond)
}

// historicMillis 将零持续时间表示为原版空值。
// @param value 持续毫秒
// @return 毫秒数或空值
func historicMillis(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// historicProcessRESTResponse 生成原版历史流程实例完整 DTO。
// @param r 当前请求
// @param item 历史实例
// @param definition 可空定义快照
// @param variables 历史变量集合，空切片输出空数组
// @return 原版历史实例对象
func historicProcessRESTResponse(r *http.Request, item api.HistoricProcessInstance, definition *api.ProcessDefinition, variables []map[string]any) map[string]any {
	if variables == nil {
		variables = []map[string]any{}
	}
	response := map[string]any{
		"id": item.ID, "url": taskResourceURL(r, "history/historic-process-instances", item.ID),
		"businessKey": historicNullable(item.BusinessKey), "businessStatus": historicNullable(item.BusinessStatus),
		"callbackId": historicNullable(item.CallbackID), "callbackType": historicNullable(item.CallbackType),
		"claimTime": historicNullableTime(item.ClaimTime), "claimedBy": historicNullable(item.ClaimedBy),
		"deleteReason": historicNullable(item.DeleteReason), "dueDate": historicNullableTime(item.DueDate),
		"durationInMillis": historicMillis(item.DurationMillis),
		"endActivityId":    historicNullable(item.EndActivityID), "endTime": historicNullableTime(item.EndTime),
		"endUserId": historicNullable(item.EndUserID), "name": historicNullable(item.Name),
		"processDefinitionDescription": nil, "processDefinitionId": item.ProcessDefinitionID,
		"processDefinitionName":     nil,
		"processDefinitionUrl":      taskResourceURL(r, "repository/process-definitions", item.ProcessDefinitionID),
		"propagatedStageInstanceId": historicNullable(item.PropagatedStageInstanceID),
		"referenceId":               historicNullable(item.ReferenceID), "referenceType": historicNullable(item.ReferenceType),
		"startActivityId": historicNullable(item.StartActivityID),
		"startTime":       item.StartTime.UTC().Truncate(time.Millisecond), "startUserId": historicNullable(item.StartUserID),
		"state": historicNullable(item.State), "superProcessInstanceId": historicNullable(item.SuperProcessInstanceID),
		"tenantId": item.TenantID, "variables": variables,
	}
	if definition != nil {
		response["processDefinitionName"] = historicNullable(definition.Name)
		response["processDefinitionDescription"] = historicNullable(definition.Description)
	}
	return response
}

// historicTaskRESTResponse 生成原版历史任务完整 DTO。
// @param r 当前请求
// @param item 历史任务
// @param variables 历史变量集合，空切片输出空数组
// @return 原版历史任务对象
func historicTaskRESTResponse(r *http.Request, item api.HistoricTaskInstance, variables []map[string]any) map[string]any {
	if variables == nil {
		variables = []map[string]any{}
	}
	return map[string]any{
		"id": item.ID, "url": taskResourceURL(r, "history/historic-task-instances", item.ID),
		"assignee": historicNullable(item.Assignee), "category": historicNullable(item.Category),
		"claimTime": historicNullableTime(item.ClaimTime), "deleteReason": historicNullable(item.DeleteReason),
		"description": historicNullable(item.Description), "dueDate": historicNullableTime(item.DueDate),
		"durationInMillis": historicMillis(item.DurationMillis), "endTime": historicNullableTime(item.EndTime),
		"executionId": historicNullable(item.ExecutionID), "formKey": historicNullable(item.FormKey),
		"name": historicNullable(item.Name), "owner": historicNullable(item.Owner),
		"parentTaskId": historicNullable(item.ParentTaskID), "priority": item.Priority,
		"processDefinitionId":       historicNullable(item.ProcessDefinitionID),
		"processDefinitionUrl":      taskResourceURL(r, "repository/process-definitions", item.ProcessDefinitionID),
		"processInstanceId":         historicNullable(item.ProcessInstanceID),
		"processInstanceUrl":        historicProcessURL(r, item.ProcessInstanceID),
		"propagatedStageInstanceId": historicNullable(item.PropagatedStageInstanceID),
		"scopeDefinitionId":         historicNullable(item.ScopeDefinitionID), "scopeId": historicNullable(item.ScopeID),
		"scopeType": historicNullable(item.ScopeType), "startTime": item.StartTime.UTC().Truncate(time.Millisecond),
		"subScopeId": historicNullable(item.SubScopeID), "taskDefinitionKey": historicNullable(item.TaskDefinitionKey),
		"tenantId": item.TenantID, "variables": variables, "workTimeInMillis": nil,
	}
}

// historicActivityRESTResponse 生成原版历史活动完整 DTO。
// @param r 当前请求
// @param item 历史活动
// @return 原版历史活动对象
func historicActivityRESTResponse(r *http.Request, item api.HistoricActivityInstance) map[string]any {
	return map[string]any{
		"id": item.ID, "activityId": item.ActivityID, "activityName": historicNullable(item.ActivityName),
		"activityType": item.ActivityType, "assignee": historicNullable(item.Assignee),
		"calledProcessInstanceId": historicNullable(item.CallProcessInstanceID),
		"durationInMillis":        activityDuration(item), "endTime": historicNullableTime(item.EndTime),
		"executionId":          historicNullable(item.ExecutionID),
		"processDefinitionId":  historicNullable(item.ProcessDefinitionID),
		"processDefinitionUrl": taskResourceURL(r, "repository/process-definitions", item.ProcessDefinitionID),
		"processInstanceId":    item.ProcessInstanceID,
		"processInstanceUrl":   historicProcessURL(r, item.ProcessInstanceID),
		"startTime":            item.StartTime.UTC().Truncate(time.Millisecond), "taskId": historicNullable(item.TaskID),
		"tenantId": item.TenantID,
	}
}

// historicDetailRESTResponse 生成原版历史明细完整 DTO。
// @param r 当前请求
// @param item 历史明细
// @return 原版历史明细对象
func historicDetailRESTResponse(r *http.Request, item api.HistoricDetail) map[string]any {
	return map[string]any{
		"id": item.ID, "activityInstanceId": nil,
		"detailType": historicNullable(item.Type), "executionId": historicNullable(item.ExecutionID),
		"processInstanceId":  historicNullable(item.ProcessInstanceID),
		"processInstanceUrl": historicProcessURL(r, item.ProcessInstanceID),
		"propertyId":         nil, "propertyValue": nil, "revision": item.Revision,
		"taskId": historicNullable(item.TaskID), "taskUrl": historicTaskURL(r, item.TaskID),
		"time":     item.Time.UTC().Truncate(time.Millisecond),
		"variable": historicVariableValue(r, item.Name, nil, item.VariableType, item.Value, "history/historic-detail", item.ID),
	}
}

// historicVariableRESTResponse 生成原版历史变量完整 DTO。
// @param r 当前请求
// @param item 历史变量
// @return 原版历史变量对象
func historicVariableRESTResponse(r *http.Request, item api.HistoricVariableInstance) map[string]any {
	return map[string]any{
		"id": item.ID, "executionId": historicNullable(item.ExecutionID),
		"processInstanceId":  item.ProcessInstanceID,
		"processInstanceUrl": historicProcessURL(r, item.ProcessInstanceID),
		"taskId":             historicNullable(item.TaskID),
		"variable":           historicVariableValue(r, item.Name, variableScope(item.TaskID), item.Type, item.Value, "history/historic-variable-instances", item.ID),
	}
}

// historicCommentRESTResponse 生成原版历史评论完整 DTO。
// @param r 当前请求
// @param item 评论
// @return 原版评论对象
func historicCommentRESTResponse(r *http.Request, item api.Comment) map[string]any {
	// 原版评论地址挂在评论子资源上。
	commentURL := ""
	if item.ProcessInstanceID != "" {
		commentURL = taskResourceURL(r, "history/historic-process-instances", item.ProcessInstanceID) + "/comments/" + item.ID
	}
	return map[string]any{
		"id": item.ID, "author": historicNullable(item.UserID), "message": historicNullable(item.Message),
		"processInstanceId":  historicNullable(item.ProcessInstanceID),
		"processInstanceUrl": commentURL,
		"taskId":             historicNullable(item.TaskID), "taskUrl": historicTaskURL(r, item.TaskID),
		"time": item.Time.UTC().Truncate(time.Millisecond),
	}
}

// historicLinkRESTResponse 生成原版历史身份链接完整 DTO。
// @param r 当前请求
// @param item 身份链接
// @return 原版身份链接对象
func historicLinkRESTResponse(r *http.Request, item api.IdentityLink) map[string]any {
	return map[string]any{
		"groupId": historicNullable(item.GroupID), "type": item.Type, "userId": historicNullable(item.UserID),
		"processInstanceId":  historicNullable(item.ProcessInstanceID),
		"processInstanceUrl": historicProcessURL(r, item.ProcessInstanceID),
		"taskId":             historicNullable(item.TaskID), "taskUrl": historicTaskURL(r, item.TaskID),
	}
}

// historicTaskLogRESTResponse 生成原版任务日志完整 DTO。
// @param item 任务日志
// @return 原版任务日志对象
func historicTaskLogRESTResponse(item api.HistoricTaskLog) map[string]any {
	var data any
	if item.Data != "" {
		data = item.Data
	}
	return map[string]any{
		"data": data, "executionId": historicNullable(item.ExecutionID), "logNumber": item.ID,
		"processDefinitionId": historicNullable(item.ProcessDefinitionID),
		"processInstanceId":   historicNullable(item.ProcessInstanceID),
		"scopeDefinitionId":   nil, "scopeId": nil, "scopeType": nil, "subScopeId": nil,
		"taskId": historicNullable(item.TaskID), "tenantId": item.TenantID,
		"timeStamp": item.Time.UTC().Truncate(time.Millisecond), "type": item.Type, "userId": historicNullable(item.UserID),
	}
}

// historicProcessURL 生成历史流程实例资源地址，空实例返回空值。
// @param r 当前请求
// @param instanceID 实例标识
// @return 资源地址或空值
func historicProcessURL(r *http.Request, instanceID string) any {
	if instanceID == "" {
		return nil
	}
	return taskResourceURL(r, "history/historic-process-instances", instanceID)
}

// historicTaskURL 生成历史任务资源地址，空任务返回空值。
// @param r 当前请求
// @param taskID 任务标识
// @return 资源地址或空值
func historicTaskURL(r *http.Request, taskID string) any {
	if taskID == "" {
		return nil
	}
	return taskResourceURL(r, "history/historic-task-instances", taskID)
}

// historicRESTValue 将历史变量值转换为原版 JSON 表示；二进制值在 DTO 中为空，经数据端点读取。
// @param value 变量值
// @return 可序列化值
func historicRESTValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case time.Time:
		return typed.UTC().Truncate(time.Millisecond)
	case []byte:
		return nil
	default:
		return value
	}
}

// historicValueType 根据实际持久化值推断原版类型名。
// @param value 变量值

// historicVariableMap 生成带作用域的历史变量对象。
// @param r 当前请求
// @param item 历史变量
// @param scope 作用域文本，空值输出 null

// historicVariableScopedMaps 按作用域将历史变量切片转换为原版响应对象，名称排序保证稳定输出。
// @param r 当前请求
// @param items 历史变量
// @param scope 作用域文本，空值输出 null

// historicProcessResponseMap 组装单个历史实例响应，补查定义名称与说明。
// @param s 服务
// @param r 当前请求
// @param item 历史实例
// @return 原版响应对象
// @return 定义查询错误
func historicProcessResponseMap(s *Server, r *http.Request, item api.HistoricProcessInstance) (map[string]any, error) {
	definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), item.ProcessDefinitionID)
	if err != nil {
		definition = api.ProcessDefinition{}
	}
	return historicProcessRESTResponse(r, item, &definition, nil), nil
}

// historicTaskRESTList 把历史任务切片转换为原版响应对象列表。
// @param r 当前请求
// @param s 服务
// @param items 历史任务
// @return 响应对象切片
// @return 查询错误
func historicTaskRESTList(r *http.Request, s *Server, items []api.HistoricTaskInstance) ([]map[string]any, error) {
	return historicTaskMaps(s, r, items, false)
}

// historicProcessMaps 将历史实例切片转换为原版响应对象。
// @param s 服务
// @param r 当前请求
// @param items 历史实例
// @param includeVariables 是否填充变量集合
// @return 响应对象切片
// @return 查询错误
func historicProcessMaps(s *Server, r *http.Request, items []api.HistoricProcessInstance, includeVariables bool) ([]map[string]any, error) {
	// 原版历史实例集合的 variables 始终为空数组，变量经专用端点读取。
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), item.ProcessDefinitionID)
		if err != nil {
			definition = api.ProcessDefinition{}
		}
		out = append(out, historicProcessRESTResponse(r, item, &definition, nil))
	}
	return out, nil
}

// historicTaskMaps 将历史任务切片转换为原版响应对象。
// @param s 服务
// @param r 当前请求
// @param items 历史任务
// @param includeVariables 是否填充变量集合
// @return 响应对象切片
// @return 查询错误
func historicTaskMaps(s *Server, r *http.Request, items []api.HistoricTaskInstance, includeVariables bool) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		vars := []map[string]any{}
		if includeVariables {
			variables, err := s.engine.HistoryService().ListHistoricVariableInstances(r.Context(), item.ProcessInstanceID)
			if err != nil {
				return nil, err
			}
			locals := make([]api.HistoricVariableInstance, 0)
			globals := make([]api.HistoricVariableInstance, 0)
			for _, variable := range variables {
				if variable.TaskID == item.ID {
					locals = append(locals, variable)
				} else if variable.TaskID == "" {
					globals = append(globals, variable)
				}
			}
			vars = append(historicVariableValueMaps(r, locals, "local"), historicVariableValueMaps(r, globals, "global")...)
		}
		out = append(out, historicTaskRESTResponse(r, item, vars))
	}
	return out, nil
}

// historicVariableMaps 将历史变量切片转换为原版响应对象。
// @param r 当前请求
// @param items 历史变量
// @return 响应对象切片
func historicVariableMaps(r *http.Request, items []api.HistoricVariableInstance) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, historicVariableRESTResponse(r, item))
	}
	return out
}

// historicDetailMaps 将历史明细切片转换为原版响应对象。
// @param r 当前请求
// @param items 历史明细
// @return 响应对象切片
func historicDetailMaps(r *http.Request, items []api.HistoricDetail) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, historicDetailRESTResponse(r, item))
	}
	return out
}

// historicActivityMaps 将历史活动切片转换为原版响应对象。
// @param r 当前请求
// @param items 历史活动
// @return 响应对象切片
func historicActivityMaps(r *http.Request, items []api.HistoricActivityInstance) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, historicActivityRESTResponse(r, item))
	}
	return out
}

// historicCommentMaps 将评论切片转换为原版响应对象。
// @param r 当前请求
// @param items 评论
// @return 响应对象切片
func historicCommentMaps(r *http.Request, items []api.Comment) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, historicCommentRESTResponse(r, item))
	}
	return out
}

// historicLinkMaps 将身份链接切片转换为原版响应对象。
// @param r 当前请求
// @param items 身份链接
// @return 响应对象切片
func historicLinkMaps(r *http.Request, items []api.IdentityLink) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, historicLinkRESTResponse(r, item))
	}
	return out
}

// historicTaskLogMaps 将任务日志切片转换为原版响应对象，并把内部事件名映射为原版名称。
// @param items 任务日志
// @return 响应对象切片
func historicTaskLogMaps(items []api.HistoricTaskLog) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		// 内部 create/complete 事件名与原版 USER_TASK_* 词汇对齐。
		switch item.Type {
		case "create":
			item.Type = "USER_TASK_CREATED"
		case "complete":
			item.Type = "USER_TASK_COMPLETED"
		}
		out = append(out, historicTaskLogRESTResponse(item))
	}
	return out
}

// writeHistoricPage 输出带原版排序字段的分页外壳。
// @param w 响应
// @param r 当前请求
// @param data 响应对象切片
// @param start 起始位置
// @param size 每页数量
// @param defaultSort 默认排序字段
func writeHistoricPage(w http.ResponseWriter, r *http.Request, data any, start, size int, defaultSort string) {
	items := reflectValueSlice(data)
	// 原版按响应排序字段与方向对数据排序后分页。
	sortHistoricMaps(items, r.URL.Query().Get("sort"), r.URL.Query().Get("order"), defaultSort, "asc")
	total := len(items)
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	if end < start {
		end = start
	}
	shown := end - start
	sortField := r.URL.Query().Get("sort")
	if sortField == "" {
		sortField = defaultSort
	}
	order := r.URL.Query().Get("order")
	if order == "" {
		order = "asc"
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items[start:end], "total": total, "sort": sortField, "order": order, "start": start, "size": shown})
}

// sortHistoricMaps 按原版排序字段对响应对象排序。
// @param items 响应对象列表
// @param sortField 请求排序字段
// @param order 请求排序方向
// @param defaultSort 默认排序字段
// @param defaultOrder 默认排序方向
func sortHistoricMaps(items []any, sortField, order, defaultSort, defaultOrder string) {
	reverse := order
	if reverse == "" {
		reverse = defaultOrder
	}
	if reverse == "" {
		reverse = "asc"
	}
	field := sortField
	if field == "" {
		field = defaultSort
	}
	key := func(item any) string {
		object, ok := item.(map[string]any)
		if !ok {
			return ""
		}
		switch field {
		case "processInstanceId":
			return fmt.Sprint(object["processInstanceId"])
		case "taskInstanceId":
			return fmt.Sprint(object["id"])
		case "startTime":
			return fmt.Sprint(object["startTime"])
		case "variableName":
			if variable, ok := object["variable"].(map[string]any); ok {
				return fmt.Sprint(variable["name"])
			}
			return ""
		case "logNumber":
			if number, ok := object["logNumber"].(int64); ok {
				return fmt.Sprintf("%020d", number)
			}
			return ""
		}
		return fmt.Sprint(object[field])
	}
	sort.SliceStable(items, func(i, j int) bool { return key(items[i]) < key(items[j]) })
	if reverse == "desc" {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
}

// reflectValueSlice 将切片接口还原为元素列表。
// @param data 切片接口
// @return 元素列表
func reflectValueSlice(data any) []any {
	value := reflect.ValueOf(data)
	if value.Kind() != reflect.Slice {
		return nil
	}
	out := make([]any, value.Len())
	for i := range out {
		out[i] = value.Index(i).Interface()
	}
	return out
}

// writeHistoricQueryPage 输出带请求排序回显与默认排序的历史查询分页外壳。
// @param w 响应
// @param r 当前请求
// @param data 响应对象切片
// @param start 起始位置
// @param size 每页数量
// @param bodySort 查询体排序字段
// @param bodyOrder 查询体排序方向
// @param defaultSort 默认排序字段
func writeHistoricQueryPage(w http.ResponseWriter, r *http.Request, data any, start, size int, bodySort, bodyOrder, defaultSort string) {
	sortField := r.URL.Query().Get("sort")
	if sortField == "" {
		sortField = bodySort
	}
	if sortField == "" {
		sortField = defaultSort
	}
	order := r.URL.Query().Get("order")
	if order == "" {
		order = bodyOrder
	}
	if order == "" {
		order = "asc"
	}
	items := reflectValueSlice(data)
	// 原版按响应排序字段与方向对数据排序后分页。
	sortHistoricMaps(items, sortField, order, defaultSort, "asc")
	total := len(items)
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	if end < start {
		end = start
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items[start:end], "total": total, "sort": sortField, "order": order, "start": start, "size": end - start})
}

// activityDuration 活动已结束时输出实际毫秒，进行中输出空值。
// @param item 历史活动
// @return 毫秒或空值
func activityDuration(item api.HistoricActivityInstance) any {
	if item.EndTime == nil {
		return nil
	}
	return item.DurationMillis
}

// variableScope 历史变量的原版作用域：流程变量为 global，任务局部变量为空。
// @param taskID 任务标识
// @return 作用域
func variableScope(taskID string) any {
	if taskID == "" {
		return "global"
	}
	return "local"
}

// historicVariableValueMaps 按作用域将历史变量切片转换为原版裸变量对象，名称排序保证稳定输出。
// @param r 当前请求
// @param items 历史变量
// @param scope 作用域文本，空值输出 null
// @return 裸变量对象数组
func historicVariableValueMaps(r *http.Request, items []api.HistoricVariableInstance, scope string) []map[string]any {
	sorted := append([]api.HistoricVariableInstance{}, items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	out := make([]map[string]any, 0, len(sorted))
	for _, item := range sorted {
		scopeValue := any(nil)
		if scope != "" {
			scopeValue = scope
		}
		out = append(out, map[string]any{
			"name": item.Name, "scope": scopeValue, "type": historicNullable(item.Type),
			"value": historicRESTValue(item.Value),
		})
	}
	return out
}

// historicTaskLinkMaps 生成任务历史链接响应；原版任务侧不携带流程实例信息。
// @param r 当前请求
// @param items 身份链接
// @return 响应对象切片
func historicTaskLinkMaps(r *http.Request, items []api.IdentityLink) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		response := historicLinkRESTResponse(r, item)
		response["processInstanceId"] = nil
		response["processInstanceUrl"] = nil
		out = append(out, response)
	}
	return out
}

// historicVariableValue 生成原版变量值对象；二进制值以数据端点地址提供。
// @param r 当前请求
// @param name 变量名
// @param scope 作用域
// @param valueType 变量类型
// @param value 变量值
// @param dataPath 数据端点路径
// @param id 资源标识
// @return 变量值对象
func historicVariableValue(r *http.Request, name string, scope any, valueType string, value any, dataPath, id string) map[string]any {
	// 原版仅二进制值携带数据端点地址，其余值直接输出。
	if _, binary := value.([]byte); binary {
		return map[string]any{
			"name": name, "scope": scope, "type": historicNullable(valueType),
			"value": nil, "valueUrl": taskResourceURL(r, dataPath, id) + "/data",
		}
	}
	return map[string]any{
		"name": name, "scope": scope, "type": historicNullable(valueType),
		"value": historicRESTValue(value),
	}
}
