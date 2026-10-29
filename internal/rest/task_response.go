package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// taskRESTResponse 按原版任务 DTO 输出显式空值、资源引用及小写委派状态。
// @param r 当前请求，用于生成同源资源地址
// @param task 引擎任务
// @return 原版任务响应对象
func taskRESTResponse(r *http.Request, task api.Task) map[string]any {
	response := map[string]any{
		"id": task.ID, "url": taskResourceURL(r, "runtime/tasks", task.ID), "name": task.Name,
		"priority": task.Priority, "createTime": task.CreateTime.UTC().Truncate(time.Millisecond), "dueDate": taskRESTTime(task.DueDate),
		"claimTime": taskRESTTime(task.ClaimTime), "suspended": task.Suspended, "tenantId": task.TenantID,
		"variables": []any{}, "scopeDefinitionId": nil, "scopeId": nil, "subScopeId": nil,
		"scopeType": nil, "propagatedStageInstanceId": nil,
	}
	for name, value := range map[string]string{
		"assignee": task.Assignee, "owner": task.Owner, "description": task.Description,
		"delegationState": strings.ToLower(task.DelegationState), "category": task.Category,
		"formKey": task.FormKey, "taskDefinitionKey": task.TaskDefinitionKey,
	} {
		response[name] = nil
		if value != "" {
			response[name] = value
		}
	}
	// 空引用同时输出空标识和空 URL，不将其错误地拼接为集合地址。
	for _, reference := range []struct{ field, path, id string }{
		{"parentTask", "runtime/tasks", task.ParentTaskID}, {"execution", "runtime/executions", task.ExecutionID},
		{"processInstance", "runtime/process-instances", task.ProcessInstanceID}, {"processDefinition", "repository/process-definitions", task.ProcessDefinitionID},
	} {
		response[reference.field+"Id"], response[reference.field+"Url"] = nil, nil
		if reference.id != "" {
			response[reference.field+"Id"] = reference.id
			response[reference.field+"Url"] = taskResourceURL(r, reference.path, reference.id)
		}
	}
	return response
}

// taskRESTTime 按原版 Java 日期契约输出 UTC 毫秒，并保持空值及调用方对象不变。
// @param value 可空时间
// @return 独立的 UTC 毫秒时间或空值
func taskRESTTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	converted := value.UTC().Truncate(time.Millisecond)
	return &converted
}

// taskResourceURL 生成当前请求同源的服务资源地址，不信任未配置的转发主机头。
// @param r 当前请求
// @param path 服务内资源路径
// @param id 资源标识
// @return 绝对资源地址
func taskResourceURL(r *http.Request, path, id string) string {
	scheme := r.URL.Scheme
	// 服务端请求通常不带协议，使用实际 TLS 连接决定同源地址。
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + r.Host + "/service/" + path + "/" + url.PathEscape(id)
}

// taskRESTResponses 转换任务列表并保持原有顺序，空列表返回数组。
// @param r 当前请求
// @param tasks 引擎任务列表
// @return 原版任务对象数组
func taskRESTResponses(r *http.Request, tasks []api.Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskRESTResponse(r, task))
	}
	return out
}

// writeTaskError 将任务资源定位和签收冲突映射为原版错误正文，其余错误沿用公共处理。
// @param w 响应
// @param r 当前请求
// @param err 引擎错误
// @return 无
func writeTaskError(w http.ResponseWriter, r *http.Request, err error) {
	var detail *api.Error
	// 只转换有明确原版证据的资源错误，不能把其他冲突伪装成签收冲突。
	if errors.As(err, &detail) {
		switch detail.Code {
		case "ENGINE-0805":
			writeJSON(w, 404, map[string]string{"message": "Not found", "exception": fmt.Sprintf("Could not find a task with id '%s'.", r.PathValue("id"))})
			return
		case "ENGINE-0807":
			writeJSON(w, 409, map[string]string{"message": "Task was already claimed", "exception": fmt.Sprintf("Task '%s' is already claimed by someone else.", r.PathValue("id"))})
			return
		}
	}
	writeError(w, err)
}

// taskQueryFromRequest 合并任务 GET 过滤与查询端点分页参数，请求体中的分页值优先。
// @param r 当前请求
// @param body 查询请求体，可为空
// @return 合并后的查询参数
// @return 参数类型错误
func taskQueryFromRequest(r *http.Request, body queryBody) (queryBody, error) {
	if body == nil {
		body = queryBody{}
	}
	for key, values := range r.URL.Query() {
		if len(values) == 0 {
			continue
		}
		if r.Method == http.MethodPost {
			// 原版 POST 查询只从 URL 补充缺失的分页项，不读取 URL 过滤条件。
			if key != "start" && key != "size" && key != "sort" && key != "order" {
				continue
			}
			if raw, exists := body[key]; exists && string(raw) != "null" {
				continue
			}
		}
		var value any = values[0]
		// 公共查询过滤器要求布尔和整数保留类型，不能把所有 URL 参数当字符串。
		switch key {
		case "start", "size", "priority", "minimumPriority", "maximumPriority":
			parsed, err := strconv.Atoi(values[0])
			if err != nil {
				return nil, api.NewError("REST-0121", "任务查询整数非法", api.ErrBadRequest)
			}
			value = parsed
		case "unassigned", "finished", "excludeSubTasks", "withoutTenantId":
			parsed, err := strconv.ParseBool(values[0])
			if err != nil {
				return nil, api.NewError("REST-0124", "任务查询布尔非法", api.ErrBadRequest)
			}
			value = parsed
		}
		body[key], _ = json.Marshal(value)
	}
	return body, nil
}

// serveTaskQuery 共用任务 GET 与 POST 查询的过滤、排序、分页及原版 DTO 输出。
// @param w 响应
// @param r 当前请求
// @param body 合并后的查询参数
// @return 无
func (s *Server) serveTaskQuery(w http.ResponseWriter, r *http.Request, body queryBody) {
	lookup, err := s.taskQueryIndex(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]api.Task, 0)
	// 引擎单页最多一千条，过滤和总量计算前必须读完，不能静默截断后续匹配项。
	for start := 0; ; start += 1000 {
		page, err := s.engine.TaskService().ListTasks(r.Context(), api.TaskFilter{Start: start, Size: 1000,
			Assignee: body.str("assignee"), Owner: body.str("owner"), CandidateUser: body.str("candidateUser"), DelegationState: strings.ToUpper(body.str("delegationState"))})
		if err != nil {
			writeError(w, err)
			return
		}
		items = append(items, page.Data...)
		if len(page.Data) == 0 || int64(start+len(page.Data)) >= page.Total {
			break
		}
	}
	// 候选用户已由引擎按身份组关系过滤，公共字段过滤器不能再次仅按直接候选人缩小结果。
	delete(body, "candidateUser")
	if unassigned, err := body.boolField("unassigned"); err != nil {
		writeError(w, err)
		return
	} else if unassigned != nil {
		// 固定原版控制器只检查字段非空，false 同样选择未签收任务，与 OpenAPI 描述有差异。
		body["unassigned"] = json.RawMessage("true")
	}
	items, err = filterQueryTasks(items, body, requestTenant(r), lookup)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = applyQueryVariableFilter(r.Context(), items, body, s.taskVariables)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, field, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if field == "" {
		field = "id"
	}
	if requested, err := body.intField("size"); err != nil {
		writeError(w, err)
		return
	} else if requested != nil && *requested >= 0 {
		size = *requested
	}
	if err := sortQueryItems(items, field, order, taskSortKey); err != nil {
		writeError(w, err)
		return
	}
	total := int64(len(items))
	data := []api.Task{}
	if start < len(items) {
		end := len(items)
		// 先比较剩余条数再相加，避免极大 size 与非零 start 相加溢出。
		if size < len(items)-start {
			end = start + size
		}
		data = items[start:end]
	}
	writeJSON(w, http.StatusOK, api.Page[map[string]any]{Data: taskRESTResponses(r, data), Total: total, Start: start, Size: len(data), Sort: field, Order: order})
}
