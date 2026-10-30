package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// variableOperation 表示变量过滤操作符。
type variableOperation string

// 支持的变量过滤操作符，统一使用小写形式比较。
const (
	variableOperationEquals             variableOperation = "equals"             // 等于
	variableOperationNotEquals          variableOperation = "notequals"          // 不等于
	variableOperationLike               variableOperation = "like"               // 模糊匹配
	variableOperationGreaterThan        variableOperation = "greaterthan"        // 大于
	variableOperationGreaterThanOrEqual variableOperation = "greaterthanorequal" // 大于等于
	variableOperationLessThan           variableOperation = "lessthan"           // 小于
	variableOperationLessThanOrEqual    variableOperation = "lessthanorequal"    // 小于等于
)

// variableCondition 表示单个变量过滤条件。
type variableCondition struct {
	Name      string            // 变量名
	Value     any               // 期望值
	Operation variableOperation // 操作符
}

// queryVariableLookup 返回单个查询项对应的变量快照。
// @param ctx 上下文
// @param item 查询项
// @return 变量快照
// @return 查询错误
type queryVariableLookup[T any] func(ctx context.Context, item T) (map[string]any, error)

// parseQueryVariableConditions 解析查询体中的 variables 过滤条件。
// @param body 查询体
// @return 变量条件列表
// @return 解析错误
func parseQueryVariableConditions(body queryBody) ([]variableCondition, error) {
	raw, ok := body["variables"]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, api.NewError("REST-0142", "variables 必须为数组", api.ErrBadRequest)
	}
	conditions := make([]variableCondition, 0, len(entries))
	for _, entry := range entries {
		var payload struct {
			Name      string          `json:"name"`      // 变量名
			Value     json.RawMessage `json:"value"`     // 期望值
			Operation string          `json:"operation"` // 操作符
		}
		if err := json.Unmarshal(entry, &payload); err != nil {
			return nil, api.NewError("REST-0142", "变量过滤条件格式错误", api.ErrBadRequest)
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			return nil, api.NewError("REST-0142", "变量过滤条件缺少 name", api.ErrBadRequest)
		}
		operation, err := parseVariableOperation(payload.Operation)
		if err != nil {
			return nil, err
		}
		value, err := decodeVariableConditionValue(payload.Value)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, variableCondition{Name: name, Value: value, Operation: operation})
	}
	return conditions, nil
}

// parseVariableOperation 解析变量操作符，空值默认为 equals。
// @param raw 原始操作符
// @return 规范化操作符
// @return 解析错误
func parseVariableOperation(raw string) (variableOperation, error) {
	operation := variableOperation(strings.ToLower(strings.TrimSpace(raw)))
	if operation == "" {
		return variableOperationEquals, nil
	}
	switch operation {
	case variableOperationEquals,
		variableOperationNotEquals,
		variableOperationLike,
		variableOperationGreaterThan,
		variableOperationGreaterThanOrEqual,
		variableOperationLessThan,
		variableOperationLessThanOrEqual:
		return operation, nil
	default:
		return "", api.NewError("REST-0143", "变量过滤操作符非法: "+raw, api.ErrBadRequest)
	}
}

// decodeVariableConditionValue 解析变量期望值。
// @param raw 原始 JSON 值
// @return 解析后的值
// @return 解析错误
func decodeVariableConditionValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, api.NewError("REST-0142", "变量过滤条件 value 非法", api.ErrBadRequest)
	}
	return value, nil
}

// applyQueryVariableFilter 解析查询体变量条件并在分页排序前过滤查询项。
// @param ctx 上下文
// @param items 待过滤查询项
// @param body 查询体
// @param lookup 变量快照访问器
// @return 过滤结果
// @return 过滤错误
func applyQueryVariableFilter[T any](ctx context.Context, items []T, body queryBody, lookup queryVariableLookup[T]) ([]T, error) {
	conditions, err := parseQueryVariableConditions(body)
	if err != nil {
		return nil, err
	}
	return filterItemsByVariables(ctx, items, conditions, lookup)
}

// filterItemsByVariables 按变量条件过滤查询项，条件为空时原样返回。
// @param ctx 上下文
// @param items 待过滤查询项
// @param conditions 变量条件
// @param lookup 变量快照访问器
// @return 过滤结果
// @return 过滤错误
func filterItemsByVariables[T any](ctx context.Context, items []T, conditions []variableCondition, lookup queryVariableLookup[T]) ([]T, error) {
	if len(conditions) == 0 {
		return items, nil
	}
	out := make([]T, 0, len(items))
	for _, item := range items {
		variables, err := lookup(ctx, item)
		if err != nil {
			return nil, err
		}
		if matchVariableConditions(conditions, variables) {
			out = append(out, item)
		}
	}
	return out, nil
}

// matchVariableConditions 判断变量快照是否满足全部条件（多个条件为 AND）。
// @param conditions 变量条件
// @param variables 变量快照
// @return 是否满足
func matchVariableConditions(conditions []variableCondition, variables map[string]any) bool {
	for _, condition := range conditions {
		value, exists := variables[condition.Name]
		// 变量不存在时所有操作符均视为不满足，避免 notEquals 误判
		if !exists {
			return false
		}
		if !matchVariableCondition(value, condition) {
			return false
		}
	}
	return true
}

// matchVariableCondition 判断单个变量条件是否满足。
// @param value 变量实际值
// @param condition 变量条件
// @return 是否满足
func matchVariableCondition(value any, condition variableCondition) bool {
	switch condition.Operation {
	case variableOperationEquals:
		return compareVariableValues(value, condition.Value) == 0
	case variableOperationNotEquals:
		return compareVariableValues(value, condition.Value) != 0
	case variableOperationLike:
		if condition.Value == nil {
			return false
		}
		return matchQueryLike(variableStringValue(value), variableStringValue(condition.Value))
	case variableOperationGreaterThan:
		return compareVariableValues(value, condition.Value) > 0
	case variableOperationGreaterThanOrEqual:
		return compareVariableValues(value, condition.Value) >= 0
	case variableOperationLessThan:
		return compareVariableValues(value, condition.Value) < 0
	case variableOperationLessThanOrEqual:
		return compareVariableValues(value, condition.Value) <= 0
	default:
		return false
	}
}

// compareVariableValues 按时间、数值、字符串的优先级比较变量值。
// @param left 左值
// @param right 右值
// @return 负数/零/正数
func compareVariableValues(left, right any) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if leftTime, ok := variableTimeValue(left); ok {
		if rightTime, ok := variableTimeValue(right); ok {
			return compareQueryValues(leftTime, rightTime)
		}
	}
	if leftNumber, ok := variableNumberValue(left); ok {
		if rightNumber, ok := variableNumberValue(right); ok {
			return compareQueryFloat(leftNumber, rightNumber)
		}
	}
	return strings.Compare(variableStringValue(left), variableStringValue(right))
}

// variableTimeValue 将值解析为时间。
// @param value 变量值
// @return 时间
// @return 是否为合法时间
func variableTimeValue(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, true
	case *time.Time:
		if typed == nil {
			return time.Time{}, false
		}
		return *typed, true
	case string:
		return parseVariableTime(typed)
	case []byte:
		return parseVariableTime(string(typed))
	default:
		return time.Time{}, false
	}
}

// parseVariableTime 按常见时间格式解析字符串。
// @param raw 时间字符串
// @return 时间
// @return 是否解析成功
func parseVariableTime(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// variableNumberValue 将值转换为浮点数。
// @param value 变量值
// @return 浮点数
// @return 是否为数值
func variableNumberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// variableStringValue 将变量值转换为字符串。
// @param value 变量值
// @return 字符串
func variableStringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	case *time.Time:
		if typed == nil {
			return ""
		}
		return typed.Format(time.RFC3339Nano)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

// processInstanceVariables 返回运行实例的可见变量。
// @param ctx 上下文
// @param item 流程实例
// @return 变量快照
// @return 查询错误
func (s *Server) processInstanceVariables(ctx context.Context, item api.ProcessInstance) (map[string]any, error) {
	return s.engine.RuntimeService().GetVariables(ctx, item.ID)
}

// taskVariables 返回运行任务的可见变量（执行可见变量与任务局部变量）。
// @param ctx 上下文
// @param item 任务
// @return 变量快照
// @return 查询错误
func (s *Server) taskVariables(ctx context.Context, item api.Task) (map[string]any, error) {
	return s.engine.TaskService().GetTaskVariables(ctx, item.ID)
}

// executionVariables 返回执行可见变量（含执行局部变量）。
// @param ctx 上下文
// @param item 执行信息
// @return 变量快照
// @return 查询错误
func (s *Server) executionVariables(ctx context.Context, item api.ExecutionInfo) (map[string]any, error) {
	return s.engine.RuntimeService().GetExecutionVariables(ctx, item.ID)
}

// cachedQueryVariableLookup 为变量访问器增加按缓存键复用的查询内缓存。
// 缓存仅在单次查询内生效，变量加载失败不写入缓存。
// @param keyOf 查询项到缓存键的映射
// @param load 变量加载函数
// @return 带缓存的变量访问器
func cachedQueryVariableLookup[T any](keyOf func(T) string, load queryVariableLookup[T]) queryVariableLookup[T] {
	cache := make(map[string]map[string]any)
	return func(ctx context.Context, item T) (map[string]any, error) {
		key := keyOf(item)
		// 同一批查询内命中缓存，避免重复访问同一实例的历史变量
		if values, ok := cache[key]; ok {
			return values, nil
		}
		values, err := load(ctx, item)
		if err != nil {
			return nil, err
		}
		cache[key] = values
		return values, nil
	}
}

// historicActivityVariablesLookup 构造历史活动的变量访问器。
// 历史活动只有流程实例 ID，按实例缓存变量快照，同一实例的多个活动复用一次查询。
// @return 变量访问器
func (s *Server) historicActivityVariablesLookup() queryVariableLookup[api.HistoricActivityInstance] {
	return cachedQueryVariableLookup(
		func(item api.HistoricActivityInstance) string { return item.ProcessInstanceID },
		func(ctx context.Context, item api.HistoricActivityInstance) (map[string]any, error) {
			return s.historicVariables(ctx, item.ProcessInstanceID, "")
		},
	)
}

// historicProcessVariables 返回历史流程实例的历史变量。
// @param ctx 上下文
// @param item 历史流程实例
// @return 变量快照
// @return 查询错误
func (s *Server) historicProcessVariables(ctx context.Context, item api.HistoricProcessInstance) (map[string]any, error) {
	return s.historicVariables(ctx, item.ID, "")
}

// historicTaskVariables 返回历史任务可见的历史变量。
// @param ctx 上下文
// @param item 历史任务
// @return 变量快照
// @return 查询错误
func (s *Server) historicTaskVariables(ctx context.Context, item api.HistoricTaskInstance) (map[string]any, error) {
	return s.historicVariables(ctx, item.ProcessInstanceID, item.ID)
}

// historicVariables 按实例查询历史变量并生成可见快照。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param taskID 历史任务 ID，非空时优先采用任务局部变量
// @return 变量快照
// @return 查询错误
func (s *Server) historicVariables(ctx context.Context, processInstanceID, taskID string) (map[string]any, error) {
	if processInstanceID == "" {
		return map[string]any{}, nil
	}
	entries, err := s.engine.HistoryService().ListHistoricVariableInstances(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	return historicVariableSnapshot(entries, taskID), nil
}

// historicVariableSnapshot 将历史变量列表合并为按变量名索引的快照。
// @param entries 历史变量列表
// @param taskID 历史任务 ID，非空时优先采用任务局部变量
// @return 变量快照
func historicVariableSnapshot(entries []api.HistoricVariableInstance, taskID string) map[string]any {
	selected := make(map[string]api.HistoricVariableInstance, len(entries))
	for _, entry := range entries {
		current, ok := selected[entry.Name]
		if !ok || preferHistoricVariable(entry, current, taskID) {
			selected[entry.Name] = entry
		}
	}
	values := make(map[string]any, len(selected))
	for name, entry := range selected {
		values[name] = entry.Value
	}
	return values
}

// preferHistoricVariable 判断候选历史变量是否应覆盖当前变量。
// @param candidate 候选变量
// @param current 当前变量
// @param taskID 历史任务 ID
// @return 是否覆盖
func preferHistoricVariable(candidate, current api.HistoricVariableInstance, taskID string) bool {
	candidatePriority := historicVariablePriority(candidate, taskID)
	currentPriority := historicVariablePriority(current, taskID)
	if candidatePriority != currentPriority {
		return candidatePriority > currentPriority
	}
	if !candidate.LastUpdatedTime.Equal(current.LastUpdatedTime) {
		return candidate.LastUpdatedTime.After(current.LastUpdatedTime)
	}
	if !candidate.CreateTime.Equal(current.CreateTime) {
		return candidate.CreateTime.After(current.CreateTime)
	}
	return candidate.ID > current.ID
}

// historicVariablePriority 返回历史变量的作用域优先级。
// @param entry 历史变量
// @param taskID 历史任务 ID
// @return 优先级，越大越优先
func historicVariablePriority(entry api.HistoricVariableInstance, taskID string) int {
	if taskID != "" {
		if entry.TaskID == taskID {
			return 2
		}
		if entry.TaskID == "" {
			return 1
		}
		return 0
	}
	if entry.TaskID == "" {
		return 1
	}
	return 0
}
