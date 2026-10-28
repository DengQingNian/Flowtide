package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// queryBody 是 Flowable POST 查询请求体。
// 保留原始字段，未知字段按兼容约定忽略。
type queryBody map[string]json.RawMessage

// readQueryBody 读取查询请求体，空体视为无过滤条件。
// @param r 请求
// @return 查询体
// @return 解析错误
func readQueryBody(r *http.Request) (queryBody, error) {
	body := make(queryBody)
	decoder := json.NewDecoder(io.LimitReader(r.Body, MaxUploadSize))
	if err := decoder.Decode(&body); err != nil {
		if err == io.EOF {
			return body, nil
		}
		return nil, api.NewError("REST-0120", "查询请求体 JSON 非法", api.ErrBadRequest)
	}
	return body, nil
}

// str 返回字符串字段，缺失或类型不符时返回空串。
// @param key 字段名
// @return 字段值
func (b queryBody) str(key string) string {
	raw, ok := b[key]
	if !ok {
		return ""
	}
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

// boolField 解析布尔字段。
// @param key 字段名
// @return 布尔指针，缺失时为 nil
// @return 解析错误
func (b queryBody) boolField(key string) (*bool, error) {
	raw, ok := b[key]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, api.NewError("REST-0124", "布尔字段类型错误: "+key, api.ErrBadRequest)
	}
	return &value, nil
}

// intField 解析整数字段。
// @param key 字段名
// @return 整数指针，缺失时为 nil
// @return 解析错误
func (b queryBody) intField(key string) (*int, error) {
	raw, ok := b[key]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, api.NewError("REST-0121", "整数字段类型错误: "+key, api.ErrBadRequest)
	}
	return &value, nil
}

// stringsField 解析字符串数组字段，兼容逗号分隔字符串。
// @param key 字段名
// @return 字符串列表
// @return 解析错误
func (b queryBody) stringsField(key string) ([]string, error) {
	raw, ok := b[key]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, api.NewError("REST-0125", "字符串数组字段类型错误: "+key, api.ErrBadRequest)
	}
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, nil
}

// timeField 解析 ISO-8601 时间字段。
// @param key 字段名
// @return 时间指针，缺失时为 nil
// @return 解析错误
func (b queryBody) timeField(key string) (*time.Time, error) {
	value := b.str(key)
	if value == "" {
		return nil, nil
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, api.NewError("REST-0122", "时间字段格式错误: "+key, api.ErrBadRequest)
}

// pagination 解析查询分页与排序参数。
// @return 偏移
// @return 每页数量
// @return 排序字段
// @return 排序方向
// @return 解析错误
func (b queryBody) pagination() (int, int, string, string, error) {
	start, size := 0, 10
	if raw, ok := b["start"]; ok && string(raw) != "null" {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, 0, "", "", api.NewError("REST-0121", "start 必须为整数", api.ErrBadRequest)
		}
		if value > 0 {
			start = value
		}
	}
	if raw, ok := b["size"]; ok && string(raw) != "null" {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, 0, "", "", api.NewError("REST-0121", "size 必须为整数", api.ErrBadRequest)
		}
		if value > 0 {
			size = value
		}
	}
	if size > 1000 {
		size = 1000
	}
	sortField := b.str("sort")
	order := strings.ToLower(b.str("order"))
	if order == "" {
		order = "asc"
	}
	if order != "asc" && order != "desc" {
		return 0, 0, "", "", api.NewError("REST-0123", "排序方向非法: "+order, api.ErrBadRequest)
	}
	return start, size, sortField, order, nil
}

// writeQueryPage 输出查询分页结果。
// @param w 响应
// @param items 全量结果
// @param start 偏移
// @param size 每页数量
// @param sortField 排序字段
// @param order 排序方向
func writeQueryPage[T any](w http.ResponseWriter, items []T, start, size int, sortField, order string) {
	if sortField == "" {
		order = ""
	}
	total := int64(len(items))
	data := items
	if start >= len(items) {
		data = []T{}
	} else {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		data = items[start:end]
	}
	writeJSON(w, http.StatusOK, api.Page[T]{Data: data, Total: total, Start: start, Size: size, Sort: sortField, Order: order})
}

// sortQueryItems 按字段访问器排序查询结果。
// @param items 结果
// @param field 排序字段
// @param order 排序方向
// @param key 字段访问器
// @return 排序错误
func sortQueryItems[T any](items []T, field, order string, key func(string) (func(T) any, bool)) error {
	if field == "" {
		return nil
	}
	value, ok := key(field)
	if !ok {
		return api.NewError("REST-0123", "非法排序字段: "+field, api.ErrBadRequest)
	}
	sort.SliceStable(items, func(i, j int) bool {
		cmp := compareQueryValues(value(items[i]), value(items[j]))
		if order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return nil
}

// compareQueryValues 比较两个查询排序值。
// @param left 左值
// @param right 右值
// @return 负数/零/正数
func compareQueryValues(left, right any) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	switch value := left.(type) {
	case string:
		return strings.Compare(value, queryStringValue(right))
	case int:
		return compareQueryFloat(float64(value), queryNumeric(right))
	case int64:
		return compareQueryFloat(float64(value), queryNumeric(right))
	case float64:
		return compareQueryFloat(value, queryNumeric(right))
	case bool:
		if other, ok := right.(bool); ok {
			if value == other {
				return 0
			}
			if !value {
				return -1
			}
			return 1
		}
	case time.Time:
		if other, ok := right.(time.Time); ok {
			if value.Before(other) {
				return -1
			}
			if value.After(other) {
				return 1
			}
			return 0
		}
	case *time.Time:
		return compareQueryTimePointers(value, right)
	case []byte:
		return strings.Compare(string(value), queryStringValue(right))
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}

// compareQueryTimePointers 比较时间指针。
// @param left 左值
// @param right 右值
// @return 负数/零/正数
func compareQueryTimePointers(left *time.Time, right any) int {
	if left == nil {
		return -1
	}
	switch other := right.(type) {
	case time.Time:
		return compareQueryValues(*left, other)
	case *time.Time:
		if other == nil {
			return 1
		}
		return compareQueryValues(*left, *other)
	}
	return 1
}

// queryNumeric 将值转换为浮点数。
// @param value 值
// @return 浮点数
func queryNumeric(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

// compareQueryFloat 比较浮点数。
// @param left 左值
// @param right 右值
// @return 负数/零/正数
func compareQueryFloat(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// queryStringValue 将值转换为字符串。
// @param value 值
// @return 字符串
func queryStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(value)
	}
}

// matchQueryString 判断精确匹配，expected 为空表示不过滤。
// @param value 实际值
// @param expected 期望值
// @return 是否匹配
func matchQueryString(value, expected string) bool {
	return expected == "" || value == expected
}

// matchQueryAny 判断值是否属于集合，集合为空表示不过滤。
// @param value 实际值
// @param expected 期望集合
// @return 是否匹配
func matchQueryAny(value string, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, item := range expected {
		if item == value {
			return true
		}
	}
	return false
}

// matchQueryLike 判断模糊匹配，忽略大小写与首尾百分号。
// @param value 实际值
// @param pattern 匹配模式
// @return 是否匹配
func matchQueryLike(value, pattern string) bool {
	if pattern == "" {
		return true
	}
	normalized := strings.Trim(pattern, "%")
	return strings.Contains(strings.ToLower(value), strings.ToLower(normalized))
}

// matchQueryTime 判断时间范围。
// @param value 实际时间
// @param after 下界
// @param before 上界
// @return 是否匹配
func matchQueryTime(value time.Time, after, before *time.Time) bool {
	if after != nil && value.Before(*after) {
		return false
	}
	if before != nil && value.After(*before) {
		return false
	}
	return true
}

// matchQueryTimePointer 判断可空时间范围，边界存在时空值不匹配。
// @param value 实际时间
// @param after 下界
// @param before 上界
// @return 是否匹配
func matchQueryTimePointer(value *time.Time, after, before *time.Time) bool {
	if value == nil {
		return after == nil && before == nil
	}
	return matchQueryTime(*value, after, before)
}

// filterQueryProcessInstances 过滤运行中流程实例。
// @param items 实例列表
// @param body 查询体
// @param tenant 请求租户
// @return 过滤结果
// @return 查询错误
func filterQueryProcessInstances(items []api.ProcessInstance, body queryBody, tenant string) ([]api.ProcessInstance, error) {
	after, err := body.timeField("startedAfter")
	if err != nil {
		return nil, err
	}
	before, err := body.timeField("startedBefore")
	if err != nil {
		return nil, err
	}
	finished, err := body.boolField("finished")
	if err != nil {
		return nil, err
	}
	suspended, err := body.boolField("suspended")
	if err != nil {
		return nil, err
	}
	keyIn, err := body.stringsField("processDefinitionKeyIn")
	if err != nil {
		return nil, err
	}
	filterTenant := body.str("tenantId")
	if filterTenant == "" {
		filterTenant = tenant
	}
	id := body.str("id")
	definitionID := body.str("processDefinitionId")
	definitionKey := body.str("processDefinitionKey")
	businessKey := body.str("businessKey")
	name := body.str("name")
	nameLike := body.str("nameLike")
	businessKeyLike := body.str("businessKeyLike")
	out := make([]api.ProcessInstance, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.ProcessDefinitionID, definitionID) ||
			!matchQueryString(item.ProcessDefinitionKey, definitionKey) ||
			!matchQueryAny(item.ProcessDefinitionKey, keyIn) ||
			!matchQueryString(item.BusinessKey, businessKey) ||
			!matchQueryString(item.Name, name) ||
			!matchQueryLike(item.Name, nameLike) ||
			!matchQueryLike(item.BusinessKey, businessKeyLike) {
			continue
		}
		if filterTenant != "" && item.TenantID != filterTenant {
			continue
		}
		if !matchQueryTime(item.StartTime, after, before) {
			continue
		}
		if finished != nil && item.Ended != *finished {
			continue
		}
		if suspended != nil && item.Suspended != *suspended {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// filterQueryExecutions 过滤执行实例。
// @param items 执行列表
// @param body 查询体
// @return 过滤结果
// @return 查询错误
func filterQueryExecutions(items []api.ExecutionInfo, body queryBody) ([]api.ExecutionInfo, error) {
	ended, err := body.boolField("ended")
	if err != nil {
		return nil, err
	}
	active, err := body.boolField("active")
	if err != nil {
		return nil, err
	}
	waiting, err := body.boolField("waiting")
	if err != nil {
		return nil, err
	}
	id := body.str("id")
	instanceID := body.str("processInstanceId")
	definitionID := body.str("processDefinitionId")
	activityID := body.str("activityId")
	out := make([]api.ExecutionInfo, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.ProcessInstanceID, instanceID) ||
			!matchQueryString(item.ProcessDefinitionID, definitionID) ||
			!matchQueryString(item.ActivityID, activityID) {
			continue
		}
		if ended != nil && item.Ended != *ended {
			continue
		}
		if active != nil && item.Active != *active {
			continue
		}
		if waiting != nil && item.Waiting != *waiting {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// taskQueryLookup 提供任务查询所需的实例与定义索引。
type taskQueryLookup struct {
	tenants        map[string]string // 实例到租户
	businessKeys   map[string]string // 实例到业务键
	definitionKeys map[string]string // 定义 ID 到 Key
}

// taskQueryIndex 构建任务查询索引。
// @param ctx 上下文
// @param s 服务
// @return 索引
// @return 查询错误
func (s *Server) taskQueryIndex(ctx context.Context) (*taskQueryLookup, error) {
	lookup := &taskQueryLookup{
		tenants:        make(map[string]string),
		businessKeys:   make(map[string]string),
		definitionKeys: make(map[string]string),
	}
	instances, err := s.engine.RuntimeService().ListProcessInstances(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		lookup.tenants[instance.ID] = instance.TenantID
		lookup.businessKeys[instance.ID] = instance.BusinessKey
	}
	definitions, err := s.engine.RepositoryService().ListProcessDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	for _, definition := range definitions {
		lookup.definitionKeys[definition.ID] = definition.Key
	}
	return lookup, nil
}

// filterQueryTasks 过滤运行中任务。
// @param items 任务列表
// @param body 查询体
// @param tenant 请求租户
// @param lookup 实例与定义索引
// @return 过滤结果
// @return 查询错误
func filterQueryTasks(items []api.Task, body queryBody, tenant string, lookup *taskQueryLookup) ([]api.Task, error) {
	unassigned, err := body.boolField("unassigned")
	if err != nil {
		return nil, err
	}
	finished, err := body.boolField("finished")
	if err != nil {
		return nil, err
	}
	priority, err := body.intField("priority")
	if err != nil {
		return nil, err
	}
	minPriority, err := body.intField("minimumPriority")
	if err != nil {
		return nil, err
	}
	maxPriority, err := body.intField("maximumPriority")
	if err != nil {
		return nil, err
	}
	createdAfter, err := body.timeField("createdAfter")
	if err != nil {
		return nil, err
	}
	createdBefore, err := body.timeField("createdBefore")
	if err != nil {
		return nil, err
	}
	dueAfter, err := body.timeField("dueAfter")
	if err != nil {
		return nil, err
	}
	dueBefore, err := body.timeField("dueBefore")
	if err != nil {
		return nil, err
	}
	candidateGroups, err := body.stringsField("candidateGroupIn")
	if err != nil {
		return nil, err
	}
	filterTenant := body.str("tenantId")
	if filterTenant == "" {
		filterTenant = tenant
	}
	id := firstNonEmpty(body.str("id"), body.str("taskId"))
	name := body.str("name")
	nameLike := body.str("nameLike")
	description := body.str("description")
	descriptionLike := body.str("descriptionLike")
	assignee := body.str("assignee")
	assigneeLike := body.str("assigneeLike")
	owner := body.str("owner")
	ownerLike := body.str("ownerLike")
	candidateUser := body.str("candidateUser")
	candidateGroup := body.str("candidateGroup")
	involvedUser := body.str("involvedUser")
	definitionID := body.str("processDefinitionId")
	definitionKey := body.str("processDefinitionKey")
	instanceID := body.str("processInstanceId")
	businessKey := body.str("processInstanceBusinessKey")
	activityID := body.str("taskDefinitionKey")
	out := make([]api.Task, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.Name, name) ||
			!matchQueryLike(item.Name, nameLike) ||
			!matchQueryString(item.Description, description) ||
			!matchQueryLike(item.Description, descriptionLike) ||
			!matchQueryString(item.Assignee, assignee) ||
			!matchQueryLike(item.Assignee, assigneeLike) ||
			!matchQueryString(item.Owner, owner) ||
			!matchQueryLike(item.Owner, ownerLike) ||
			!matchQueryString(item.ProcessDefinitionID, definitionID) ||
			!matchQueryString(item.ProcessInstanceID, instanceID) ||
			!matchQueryString(item.TaskDefinitionKey, activityID) {
			continue
		}
		if definitionKey != "" && lookup.definitionKeys[item.ProcessDefinitionID] != definitionKey {
			continue
		}
		if businessKey != "" && lookup.businessKeys[item.ProcessInstanceID] != businessKey {
			continue
		}
		if filterTenant != "" && lookup.tenants[item.ProcessInstanceID] != filterTenant {
			continue
		}
		if unassigned != nil && (item.Assignee == "") != *unassigned {
			continue
		}
		if finished != nil && *finished {
			continue
		}
		if priority != nil && item.Priority != *priority {
			continue
		}
		if minPriority != nil && item.Priority < *minPriority {
			continue
		}
		if maxPriority != nil && item.Priority > *maxPriority {
			continue
		}
		if !matchQueryTime(item.CreateTime, createdAfter, createdBefore) {
			continue
		}
		if !matchQueryTimePointer(item.DueDate, dueAfter, dueBefore) {
			continue
		}
		if candidateUser != "" && (item.Assignee != "" || !containsString(item.CandidateUsers, candidateUser)) {
			continue
		}
		if candidateGroup != "" && !containsString(item.CandidateGroups, candidateGroup) {
			continue
		}
		if len(candidateGroups) > 0 && !containsAnyString(item.CandidateGroups, candidateGroups) {
			continue
		}
		if involvedUser != "" && item.Assignee != involvedUser && item.Owner != involvedUser &&
			!containsString(item.CandidateUsers, involvedUser) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// containsString 判断字符串是否在列表中。
// @param values 列表
// @param target 目标
// @return 是否存在
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// containsAnyString 判断列表是否包含任意目标值。
// @param values 列表
// @param targets 目标列表
// @return 是否包含
func containsAnyString(values, targets []string) bool {
	for _, target := range targets {
		if containsString(values, target) {
			return true
		}
	}
	return false
}

// filterQueryHistoricProcesses 过滤历史流程实例。
// @param items 历史实例
// @param body 查询体
// @param tenant 请求租户
// @return 过滤结果
// @return 查询错误
func filterQueryHistoricProcesses(items []api.HistoricProcessInstance, body queryBody, tenant string) ([]api.HistoricProcessInstance, error) {
	finished, err := body.boolField("finished")
	if err != nil {
		return nil, err
	}
	after, err := body.timeField("startedAfter")
	if err != nil {
		return nil, err
	}
	before, err := body.timeField("startedBefore")
	if err != nil {
		return nil, err
	}
	endedAfter, err := body.timeField("endedAfter")
	if err != nil {
		return nil, err
	}
	endedBefore, err := body.timeField("endedBefore")
	if err != nil {
		return nil, err
	}
	keyIn, err := body.stringsField("processDefinitionKeyIn")
	if err != nil {
		return nil, err
	}
	filterTenant := body.str("tenantId")
	if filterTenant == "" {
		filterTenant = tenant
	}
	id := body.str("id")
	definitionID := body.str("processDefinitionId")
	definitionKey := body.str("processDefinitionKey")
	businessKey := body.str("businessKey")
	businessKeyLike := body.str("businessKeyLike")
	name := body.str("name")
	nameLike := body.str("nameLike")
	deleteReason := body.str("deleteReason")
	out := make([]api.HistoricProcessInstance, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.ProcessDefinitionID, definitionID) ||
			!matchQueryString(item.ProcessDefinitionKey, definitionKey) ||
			!matchQueryAny(item.ProcessDefinitionKey, keyIn) ||
			!matchQueryString(item.BusinessKey, businessKey) ||
			!matchQueryLike(item.BusinessKey, businessKeyLike) ||
			!matchQueryString(item.Name, name) ||
			!matchQueryLike(item.Name, nameLike) ||
			!matchQueryString(item.DeleteReason, deleteReason) {
			continue
		}
		if filterTenant != "" && item.TenantID != filterTenant {
			continue
		}
		if finished != nil && item.Ended != *finished {
			continue
		}
		if !matchQueryTime(item.StartTime, after, before) {
			continue
		}
		if !matchQueryTimePointer(item.EndTime, endedAfter, endedBefore) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// historicProcessLookup 提供历史任务/活动查询索引。
type historicProcessLookup struct {
	tenants        map[string]string // 实例到租户
	definitionKeys map[string]string // 实例到流程定义 Key
}

// historicProcessIndex 构建历史查询索引。
// @param ctx 上下文
// @param s 服务
// @return 索引
// @return 查询错误
func (s *Server) historicProcessIndex(ctx context.Context) (*historicProcessLookup, error) {
	lookup := &historicProcessLookup{
		tenants:        make(map[string]string),
		definitionKeys: make(map[string]string),
	}
	instances, err := s.engine.HistoryService().ListHistoricProcessInstances(ctx)
	if err != nil {
		return nil, err
	}
	for _, instance := range instances {
		lookup.tenants[instance.ID] = instance.TenantID
		lookup.definitionKeys[instance.ID] = instance.ProcessDefinitionKey
	}
	return lookup, nil
}

// filterQueryHistoricTasks 过滤历史任务。
// @param items 历史任务
// @param body 查询体
// @param tenant 请求租户
// @param lookup 历史实例索引
// @return 过滤结果
// @return 查询错误
func filterQueryHistoricTasks(items []api.HistoricTaskInstance, body queryBody, tenant string, lookup *historicProcessLookup) ([]api.HistoricTaskInstance, error) {
	finished, err := body.boolField("finished")
	if err != nil {
		return nil, err
	}
	after, err := body.timeField("startedAfter")
	if err != nil {
		return nil, err
	}
	before, err := body.timeField("startedBefore")
	if err != nil {
		return nil, err
	}
	endedAfter, err := body.timeField("endedAfter")
	if err != nil {
		return nil, err
	}
	endedBefore, err := body.timeField("endedBefore")
	if err != nil {
		return nil, err
	}
	filterTenant := body.str("tenantId")
	if filterTenant == "" {
		filterTenant = tenant
	}
	id := firstNonEmpty(body.str("id"), body.str("taskId"))
	name := firstNonEmpty(body.str("name"), body.str("taskName"))
	nameLike := firstNonEmpty(body.str("nameLike"), body.str("taskNameLike"))
	assignee := firstNonEmpty(body.str("assignee"), body.str("taskAssignee"))
	assigneeLike := firstNonEmpty(body.str("assigneeLike"), body.str("taskAssigneeLike"))
	activityID := body.str("taskDefinitionKey")
	activityLike := body.str("taskDefinitionKeyLike")
	instanceID := body.str("processInstanceId")
	definitionKey := body.str("processDefinitionKey")
	out := make([]api.HistoricTaskInstance, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.Name, name) ||
			!matchQueryLike(item.Name, nameLike) ||
			!matchQueryString(item.Assignee, assignee) ||
			!matchQueryLike(item.Assignee, assigneeLike) ||
			!matchQueryString(item.TaskDefinitionKey, activityID) ||
			!matchQueryLike(item.TaskDefinitionKey, activityLike) ||
			!matchQueryString(item.ProcessInstanceID, instanceID) {
			continue
		}
		if filterTenant != "" && lookup.tenants[item.ProcessInstanceID] != filterTenant {
			continue
		}
		if definitionKey != "" && lookup.definitionKeys[item.ProcessInstanceID] != definitionKey {
			continue
		}
		if finished != nil && (item.EndTime != nil) != *finished {
			continue
		}
		if !matchQueryTime(item.StartTime, after, before) {
			continue
		}
		if !matchQueryTimePointer(item.EndTime, endedAfter, endedBefore) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// filterQueryHistoricActivities 过滤历史活动。
// @param items 历史活动
// @param body 查询体
// @param tenant 请求租户
// @param lookup 历史实例索引
// @return 过滤结果
// @return 查询错误
func filterQueryHistoricActivities(items []api.HistoricActivityInstance, body queryBody, tenant string, lookup *historicProcessLookup) ([]api.HistoricActivityInstance, error) {
	finished, err := body.boolField("finished")
	if err != nil {
		return nil, err
	}
	after, err := body.timeField("startedAfter")
	if err != nil {
		return nil, err
	}
	before, err := body.timeField("startedBefore")
	if err != nil {
		return nil, err
	}
	endedAfter, err := body.timeField("endedAfter")
	if err != nil {
		return nil, err
	}
	endedBefore, err := body.timeField("endedBefore")
	if err != nil {
		return nil, err
	}
	filterTenant := body.str("tenantId")
	if filterTenant == "" {
		filterTenant = tenant
	}
	id := body.str("id")
	instanceID := body.str("processInstanceId")
	activityID := body.str("activityId")
	activityType := body.str("activityType")
	activityName := firstNonEmpty(body.str("activityName"), body.str("name"))
	activityNameLike := body.str("activityNameLike")
	definitionKey := body.str("processDefinitionKey")
	out := make([]api.HistoricActivityInstance, 0, len(items))
	for _, item := range items {
		if !matchQueryString(item.ID, id) ||
			!matchQueryString(item.ProcessInstanceID, instanceID) ||
			!matchQueryString(item.ActivityID, activityID) ||
			!matchQueryString(item.ActivityType, activityType) ||
			!matchQueryString(item.ActivityName, activityName) ||
			!matchQueryLike(item.ActivityName, activityNameLike) {
			continue
		}
		if filterTenant != "" && lookup.tenants[item.ProcessInstanceID] != filterTenant {
			continue
		}
		if definitionKey != "" && lookup.definitionKeys[item.ProcessInstanceID] != definitionKey {
			continue
		}
		if finished != nil && (item.EndTime != nil) != *finished {
			continue
		}
		if !matchQueryTime(item.StartTime, after, before) {
			continue
		}
		if !matchQueryTimePointer(item.EndTime, endedAfter, endedBefore) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// processInstanceSortKey 返回流程实例排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func processInstanceSortKey(field string) (func(api.ProcessInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.ProcessInstance) any { return item.ID }, true
	case "processDefinitionId":
		return func(item api.ProcessInstance) any { return item.ProcessDefinitionID }, true
	case "processDefinitionKey":
		return func(item api.ProcessInstance) any { return item.ProcessDefinitionKey }, true
	case "businessKey":
		return func(item api.ProcessInstance) any { return item.BusinessKey }, true
	case "name":
		return func(item api.ProcessInstance) any { return item.Name }, true
	case "startTime":
		return func(item api.ProcessInstance) any { return item.StartTime }, true
	case "endTime":
		return func(item api.ProcessInstance) any { return item.EndTime }, true
	}
	return nil, false
}

// executionSortKey 返回执行排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func executionSortKey(field string) (func(api.ExecutionInfo) any, bool) {
	switch field {
	case "id":
		return func(item api.ExecutionInfo) any { return item.ID }, true
	case "processInstanceId":
		return func(item api.ExecutionInfo) any { return item.ProcessInstanceID }, true
	case "processDefinitionId":
		return func(item api.ExecutionInfo) any { return item.ProcessDefinitionID }, true
	case "activityId":
		return func(item api.ExecutionInfo) any { return item.ActivityID }, true
	}
	return nil, false
}

// taskSortKey 返回任务排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func taskSortKey(field string) (func(api.Task) any, bool) {
	switch field {
	case "id":
		return func(item api.Task) any { return item.ID }, true
	case "name":
		return func(item api.Task) any { return item.Name }, true
	case "description":
		return func(item api.Task) any { return item.Description }, true
	case "assignee":
		return func(item api.Task) any { return item.Assignee }, true
	case "owner":
		return func(item api.Task) any { return item.Owner }, true
	case "priority":
		return func(item api.Task) any { return item.Priority }, true
	case "createTime":
		return func(item api.Task) any { return item.CreateTime }, true
	case "dueDate":
		return func(item api.Task) any { return item.DueDate }, true
	}
	return nil, false
}

// historicProcessSortKey 返回历史流程实例排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func historicProcessSortKey(field string) (func(api.HistoricProcessInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.HistoricProcessInstance) any { return item.ID }, true
	case "processDefinitionKey":
		return func(item api.HistoricProcessInstance) any { return item.ProcessDefinitionKey }, true
	case "businessKey":
		return func(item api.HistoricProcessInstance) any { return item.BusinessKey }, true
	case "name":
		return func(item api.HistoricProcessInstance) any { return item.Name }, true
	case "startTime":
		return func(item api.HistoricProcessInstance) any { return item.StartTime }, true
	case "endTime":
		return func(item api.HistoricProcessInstance) any { return item.EndTime }, true
	}
	return nil, false
}

// historicTaskSortKey 返回历史任务排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func historicTaskSortKey(field string) (func(api.HistoricTaskInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.HistoricTaskInstance) any { return item.ID }, true
	case "name":
		return func(item api.HistoricTaskInstance) any { return item.Name }, true
	case "assignee":
		return func(item api.HistoricTaskInstance) any { return item.Assignee }, true
	case "startTime":
		return func(item api.HistoricTaskInstance) any { return item.StartTime }, true
	case "endTime":
		return func(item api.HistoricTaskInstance) any { return item.EndTime }, true
	}
	return nil, false
}

// historicActivitySortKey 返回历史活动排序字段访问器。
// @param field 字段名
// @return 访问器
// @return 是否支持
func historicActivitySortKey(field string) (func(api.HistoricActivityInstance) any, bool) {
	switch field {
	case "id":
		return func(item api.HistoricActivityInstance) any { return item.ID }, true
	case "activityId":
		return func(item api.HistoricActivityInstance) any { return item.ActivityID }, true
	case "activityType":
		return func(item api.HistoricActivityInstance) any { return item.ActivityType }, true
	case "activityName":
		return func(item api.HistoricActivityInstance) any { return item.ActivityName }, true
	case "startTime":
		return func(item api.HistoricActivityInstance) any { return item.StartTime }, true
	case "endTime":
		return func(item api.HistoricActivityInstance) any { return item.EndTime }, true
	}
	return nil, false
}

// handleQueryProcessInstances 查询运行中流程实例。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryProcessInstances(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.RuntimeService().ListProcessInstances(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterQueryProcessInstances(items, body, requestTenant(r))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, processInstanceSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleQueryExecutions 查询执行实例。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryExecutions(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.RuntimeService().ListExecutions(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterQueryExecutions(items, body)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, executionSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleQueryTasks 查询运行中任务。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryTasks(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	lookup, err := s.taskQueryIndex(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.TaskService().ListTasks(r.Context(), api.TaskFilter{Size: 1000})
	if err != nil {
		writeError(w, err)
		return
	}
	items.Data, err = filterQueryTasks(items.Data, body, requestTenant(r), lookup)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items.Data, sortField, order, taskSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items.Data, start, size, sortField, order)
}

// handleQueryHistoricProcesses 查询历史流程实例。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryHistoricProcesses(w http.ResponseWriter, r *http.Request) {
	body, err := readQueryBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.engine.HistoryService().ListHistoricProcessInstances(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterQueryHistoricProcesses(items, body, requestTenant(r))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, historicProcessSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleQueryHistoricTasks 查询历史任务。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryHistoricTasks(w http.ResponseWriter, r *http.Request) {
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
	items, err := s.engine.HistoryService().ListHistoricTaskInstances(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterQueryHistoricTasks(items, body, requestTenant(r), lookup)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, historicTaskSortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}

// handleQueryHistoricActivities 查询历史活动。
// @param w 响应
// @param r 请求
func (s *Server) handleQueryHistoricActivities(w http.ResponseWriter, r *http.Request) {
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
	items, err := s.engine.HistoryService().ListHistoricActivityInstances(r.Context(), body.str("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	items, err = filterQueryHistoricActivities(items, body, requestTenant(r), lookup)
	if err != nil {
		writeError(w, err)
		return
	}
	start, size, sortField, order, err := body.pagination()
	if err != nil {
		writeError(w, err)
		return
	}
	if err := sortQueryItems(items, sortField, order, historicActivitySortKey); err != nil {
		writeError(w, err)
		return
	}
	writeQueryPage(w, items, start, size, sortField, order)
}
