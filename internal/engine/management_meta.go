package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// 管理端点使用的默认 Schema 方言，两种方言表结构一致
const managementDialect = "postgres"

// Properties 返回引擎属性。
// 属性映射 ACT_GE_PROPERTY 的常用键，并附带引擎运行信息。
// @param ctx 上下文
// @return 属性映射
// @return 查询错误
func (e *Engine) Properties(ctx context.Context) (map[string]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return nil, api.NewError("ENGINE-0002", "引擎已关闭", api.ErrEngineClosed)
	}
	out := e.builtinPropertiesLocked()
	for name, value := range e.state.Properties {
		out[name] = value
	}
	return out, nil
}

// builtinPropertiesLocked 返回内置引擎属性。
// @return 属性映射
func (e *Engine) builtinPropertiesLocked() map[string]string {
	return map[string]string{
		"common.schema.version": schema.Version,
		"schema.version":        schema.Version,
		"schema.history":        "create(" + schema.Version + ")",
		"next.dbid":             "1",
		"engine.version":        Version,
		"historyLevel":          e.cfg.HistoryLevel,
		"tenantId":              e.cfg.TenantID,
	}
}

// TableCount 返回各 ACT_* 表的行数。
// @param ctx 上下文
// @return 表名到行数
// @return 查询错误
func (e *Engine) TableCount(ctx context.Context) (map[string]int64, error) {
	counts := make(map[string]int64)
	tables, err := schema.Tables(managementDialect)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		counts[table] = 0
	}
	rows, err := e.scopedSnapshotRows(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.Table]++
	}
	return counts, nil
}

// GetTableMetaData 返回表的列元数据。
// @param ctx 上下文
// @param tableName 表名
// @return 列元数据
// @return 查询错误
func (e *Engine) GetTableMetaData(ctx context.Context, tableName string) (api.TableMetaData, error) {
	name := strings.ToUpper(strings.TrimSpace(tableName))
	columns, types, err := schema.TableColumns(managementDialect, name)
	if err != nil {
		return api.TableMetaData{}, err
	}
	return api.TableMetaData{TableName: name, ColumnNames: columns, ColumnTypes: types}, nil
}

// QueryTableData 分页查询表数据。
// 行数据来源于引擎快照，未写入的列以 null 补齐。
// @param ctx 上下文
// @param tableName 表名
// @param start 偏移
// @param size 每页数量
// @param sortColumn 排序列
// @param order 排序方向
// @return 分页数据
// @return 查询错误
func (e *Engine) QueryTableData(ctx context.Context, tableName string, start, size int, sortColumn, order string) (api.Page[map[string]any], error) {
	name := strings.ToUpper(strings.TrimSpace(tableName))
	columns, _, err := schema.TableColumns(managementDialect, name)
	if err != nil {
		return api.Page[map[string]any]{}, err
	}
	sortColumn = strings.ToUpper(strings.TrimSpace(sortColumn))
	if sortColumn != "" && !contains(columns, sortColumn) {
		return api.Page[map[string]any]{}, api.NewError("ENGINE-0916", "排序列不存在: "+sortColumn, api.ErrBadRequest)
	}
	order = strings.ToLower(strings.TrimSpace(order))
	if order == "" {
		order = "asc"
	}
	if order != "asc" && order != "desc" {
		return api.Page[map[string]any]{}, api.NewError("ENGINE-0917", "排序方向非法: "+order, api.ErrBadRequest)
	}
	rows, err := e.scopedSnapshotRows(ctx)
	if err != nil {
		return api.Page[map[string]any]{}, err
	}
	data := make([]map[string]any, 0)
	for _, row := range rows {
		if row.Table != name {
			continue
		}
		item := make(map[string]any, len(columns))
		for _, column := range columns {
			item[column] = nil
		}
		for column, value := range row.Values {
			item[strings.ToUpper(column)] = value
		}
		data = append(data, item)
	}
	sort.SliceStable(data, func(i, j int) bool {
		left := sortValue(data[i], sortColumn)
		right := sortValue(data[j], sortColumn)
		if order == "desc" {
			return compareValues(left, right) > 0
		}
		return compareValues(left, right) < 0
	})
	start, size = normalizePage(start, size)
	total := int64(len(data))
	if start >= len(data) {
		data = []map[string]any{}
	} else {
		end := start + size
		if end > len(data) {
			end = len(data)
		}
		data = data[start:end]
	}
	page := api.Page[map[string]any]{Data: data, Total: total, Start: start, Size: size}
	if sortColumn != "" {
		page.Sort = sortColumn
		page.Order = order
	}
	return page, nil
}

// sortValue 返回排序键，未指定列时使用 ID_。
// @param item 行数据
// @param column 排序列
// @return 排序值
func sortValue(item map[string]any, column string) any {
	if column == "" {
		if value, ok := item["ID_"]; ok {
			return value
		}
		return ""
	}
	return item[column]
}

// compareValues 比较两个排序值。
// 支持数值、时间、布尔与字符串，其余类型按字符串比较。
// @param left 左值
// @param right 右值
// @return 负数/零/正数
func compareValues(left, right any) int {
	// null 排在最前，避免与具体类型比较
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	switch leftValue := left.(type) {
	case int:
		return compareFloat(float64(leftValue), numericValue(right))
	case int64:
		return compareFloat(float64(leftValue), numericValue(right))
	case float64:
		return compareFloat(leftValue, numericValue(right))
	case time.Time:
		if rightTime, ok := right.(time.Time); ok {
			return leftValue.Compare(rightTime)
		}
	case bool:
		if rightBool, ok := right.(bool); ok {
			if leftValue == rightBool {
				return 0
			}
			if !leftValue {
				return -1
			}
			return 1
		}
	case []byte:
		return strings.Compare(string(leftValue), stringValue(right))
	}
	return strings.Compare(stringValue(left), stringValue(right))
}

// numericValue 将值转换为浮点数，非数值返回 0。
// @param value 值
// @return 浮点数
func numericValue(value any) float64 {
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

// compareFloat 比较浮点数。
// @param left 左值
// @param right 右值
// @return 负数/零/正数
func compareFloat(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// stringValue 将值转换为可比较字符串。
// @param value 值
// @return 字符串
func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return ""
	}
}

// scopedSnapshotRows 仅为管理查询导出当前租户可见的行。
// @param ctx 请求上下文
// @return 过滤后的快照行
// @return 导出错误
func (e *Engine) scopedSnapshotRows(ctx context.Context) ([]flowablerow.Row, error) {
	rows, err := e.SnapshotRows(ctx)
	if err != nil {
		return nil, err
	}
	return flowablerow.FilterRowsByTenant(rows, e.tenantOf(tenant.From(ctx))), nil
}
