package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// quotedColumns 需要按方言转义的保留字列名。
var quotedColumns = map[string]bool{"condition": true}

// placeholderPattern 匹配 PostgreSQL 相对占位符 $n。
var placeholderPattern = regexp.MustCompile(`\$(\d+)`)

// quoteColumn 按方言为保留字列加引号（如 condition：MySQL 反引号、PostgreSQL 双引号）。
// @param column 列名
// @return 转义后的列名
func (s *Store) quoteColumn(column string) string {
	// 仅 condition 在双库中与保留字/关键字冲突，需要转义
	if quotedColumns[column] {
		if s.dialect == DialectMySQL {
			return "`" + column + "`"
		}
		return `"` + column + `"`
	}
	return column
}

// quoteColumns 按方言转义列名列表并以逗号拼接。
// @param columns 列名列表
// @return 转义后的列名列表文本
func (s *Store) quoteColumns(columns []string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = s.quoteColumn(column)
	}
	return strings.Join(parts, ", ")
}

// shiftPlaceholders 将 SQL 片段中的相对占位符整体平移 offset 位（仅 PostgreSQL 生效）。
// 用于条件更新：SET 占位符占用 1..n，WHERE 子句的相对占位符统一后移。
// @param fragment 含 $n 相对占位符的 SQL 片段
// @param offset 平移位数
// @return 平移后的 SQL 片段
func (s *Store) shiftPlaceholders(fragment string, offset int) string {
	// MySQL 的 "?" 无序号概念，无需平移
	if s.dialect == DialectMySQL || offset <= 0 {
		return fragment
	}
	return placeholderPattern.ReplaceAllStringFunc(fragment, func(match string) string {
		index, err := strconv.Atoi(match[1:])
		// 正则保证 match 形如 "$数字"，转换失败时保持原样
		if err != nil {
			return match
		}
		return "$" + strconv.Itoa(index+offset)
	})
}

// normalizeValue 将驱动返回值统一为通用 Go 类型。
// @param value 驱动值
// @return 通用值
func normalizeValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}

// scanRow 扫描结果集当前行为通用行。
// @param rows 结果集
// @return 行数据（列名→值）
// @return 扫描错误
func scanRow(rows *sql.Rows) (Row, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, api.NewError("WF-0011", "读取 workflow 结果集列失败", err)
	}
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, api.NewError("WF-0012", "扫描 workflow 结果行失败", err)
	}
	row := make(Row, len(columns))
	for i, column := range columns {
		row[column] = normalizeValue(values[i])
	}
	return row, nil
}

// queryOne 执行查询并返回第一行；无结果时返回 (nil, false, nil)。
// @param ctx 操作上下文
// @param query 查询 SQL
// @param args 参数
// @return 行数据
// @return 是否存在
// @return 查询错误
func (s *Store) queryOne(ctx context.Context, query string, args []any) (Row, bool, error) {
	rows, err := s.executorFrom(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, api.NewError("WF-0010", "查询 workflow 表失败", err)
	}
	defer func() { _ = rows.Close() }()
	// 空结果集视为未找到，由调用方决定语义
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, false, api.NewError("WF-0013", "遍历 workflow 结果集失败", err)
		}
		return nil, false, nil
	}
	row, err := scanRow(rows)
	if err != nil {
		return nil, false, err
	}
	return row, true, nil
}

// queryList 执行查询并返回全部行。
// @param ctx 操作上下文
// @param query 查询 SQL
// @param args 参数
// @return 行列表
// @return 查询错误
func (s *Store) queryList(ctx context.Context, query string, args []any) ([]Row, error) {
	rows, err := s.executorFrom(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, api.NewError("WF-0010", "查询 workflow 表失败", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]Row, 0, 8)
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, api.NewError("WF-0013", "遍历 workflow 结果集失败", err)
	}
	return result, nil
}

// baseRepo 提供单表通用 CRUD，由各表仓储内嵌复用。
type baseRepo struct {
	store *Store   // 仓储入口
	def   tableDef // 表元数据
}

// prepare 校验行列合法性并填充公共时间列（inserting 时补 create_time，两者都补 update_time）。
// @param row 原始行数据
// @param inserting 是否为插入
// @return 处理后的行数据
// @return 校验错误
func (b baseRepo) prepare(row Row, inserting bool) (Row, error) {
	if len(row) == 0 {
		return nil, api.NewError("WF-0006", "workflow 行数据不能为空", api.ErrBadRequest)
	}
	data := make(Row, len(row)+2)
	for key, value := range row {
		// 列名直接拼入 SQL，必须严格校验在表定义内，防止注入
		if !b.def.has(key) {
			return nil, api.NewError("WF-0007", fmt.Sprintf("表 %s 不存在列 %s", b.def.Name, key), api.ErrBadRequest)
		}
		data[key] = value
	}
	now := b.store.Now()
	// 公共时间列由仓储统一填充；调用方显式提供时尊重原值
	if _, ok := data["create_time"]; !ok && inserting {
		data["create_time"] = now
	}
	if _, ok := data["update_time"]; !ok {
		data["update_time"] = now
	}
	return data, nil
}

// Insert 插入一行；id 必须由调用方生成传入（internal/idgen），本层不生成。
// @param ctx 操作上下文
// @param row 行数据
// @return 插入错误
func (b baseRepo) Insert(ctx context.Context, row Row) error {
	data, err := b.prepare(row, true)
	if err != nil {
		return err
	}
	// id 由调用方生成传入，缺失说明调用方违反契约
	if id, _ := data["id"].(string); strings.TrimSpace(id) == "" {
		return api.NewError("WF-0008", "插入 "+b.def.Name+" 必须由调用方传入非空 id（internal/idgen 生成）", api.ErrBadRequest)
	}
	columns := b.def.orderedColumns(data)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", b.def.Name, b.store.quoteColumns(columns), b.store.phList(len(columns)))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		args = append(args, data[column])
	}
	if _, err := b.store.executorFrom(ctx).ExecContext(ctx, query, args...); err != nil {
		return api.NewError("WF-0009", "插入 workflow 行失败: "+b.def.Name, err)
	}
	return nil
}

// UpdateByPK 按主键更新指定列；update_time 缺省时自动填充，id 列不可修改。
// @param ctx 操作上下文
// @param id 主键
// @param changes 变更列
// @return 影响行数
// @return 更新错误
func (b baseRepo) UpdateByPK(ctx context.Context, id string, changes Row) (int64, error) {
	if strings.TrimSpace(id) == "" {
		return 0, api.NewError("WF-0014", "更新 "+b.def.Name+" 必须提供主键 id", api.ErrBadRequest)
	}
	// WHERE 相对占位符从 1 开始，由 updateWhere 统一平移到 SET 之后
	return b.updateWhere(ctx, changes, "id = "+b.store.ph(1), []any{id})
}

// updateWhere 按条件更新指定列；where 中的占位符为相对序号（$1..），内部平移到 SET 列之后。
// @param ctx 操作上下文
// @param changes 变更列
// @param where 条件模板（相对占位符）
// @param whereArgs 条件参数
// @return 影响行数
// @return 更新错误
func (b baseRepo) updateWhere(ctx context.Context, changes Row, where string, whereArgs []any) (int64, error) {
	data, err := b.prepare(changes, false)
	if err != nil {
		return 0, err
	}
	// 主键不可通过更新修改
	delete(data, "id")
	if len(data) == 0 {
		return 0, api.NewError("WF-0015", "更新 "+b.def.Name+" 缺少变更列", api.ErrBadRequest)
	}
	columns := b.def.orderedColumns(data)
	set := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns)+len(whereArgs))
	for i, column := range columns {
		set = append(set, b.store.quoteColumn(column)+" = "+b.store.ph(i+1))
		args = append(args, data[column])
	}
	query := "UPDATE " + b.def.Name + " SET " + strings.Join(set, ", ") + " WHERE " + b.store.shiftPlaceholders(where, len(columns))
	args = append(args, whereArgs...)
	result, err := b.store.executorFrom(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("WF-0016", "更新 workflow 行失败: "+b.def.Name, err)
	}
	return result.RowsAffected()
}

// DeleteByPK 按主键删除一行。
// @param ctx 操作上下文
// @param id 主键
// @return 影响行数
// @return 删除错误
func (b baseRepo) DeleteByPK(ctx context.Context, id string) (int64, error) {
	if strings.TrimSpace(id) == "" {
		return 0, api.NewError("WF-0017", "删除 "+b.def.Name+" 必须提供主键 id", api.ErrBadRequest)
	}
	return b.deleteWhere(ctx, "id = "+b.store.ph(1), []any{id})
}

// deleteWhere 按条件删除行。
// @param ctx 操作上下文
// @param where 条件（占位符从 1 开始）
// @param args 条件参数
// @return 影响行数
// @return 删除错误
func (b baseRepo) deleteWhere(ctx context.Context, where string, args []any) (int64, error) {
	query := "DELETE FROM " + b.def.Name + " WHERE " + where
	result, err := b.store.executorFrom(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("WF-0018", "删除 workflow 行失败: "+b.def.Name, err)
	}
	return result.RowsAffected()
}

// GetByPK 按主键查询单行。
// @param ctx 操作上下文
// @param id 主键
// @return 行数据（列名→值）
// @return 是否存在
// @return 查询错误
func (b baseRepo) GetByPK(ctx context.Context, id string) (Row, bool, error) {
	if strings.TrimSpace(id) == "" {
		return nil, false, api.NewError("WF-0019", "查询 "+b.def.Name+" 必须提供主键 id", api.ErrBadRequest)
	}
	query := "SELECT " + b.store.quoteColumns(b.def.Columns) + " FROM " + b.def.Name + " WHERE id = " + b.store.ph(1)
	return b.store.queryOne(ctx, query, []any{id})
}

// getFirst 按条件查询第一行（可选排序）。
// @param ctx 操作上下文
// @param where 条件（占位符从 1 开始）
// @param args 条件参数
// @param order 排序列，空串表示不排序
// @return 行数据
// @return 是否存在
// @return 查询错误
func (b baseRepo) getFirst(ctx context.Context, where string, args []any, order string) (Row, bool, error) {
	query := "SELECT " + b.store.quoteColumns(b.def.Columns) + " FROM " + b.def.Name + " WHERE " + where
	if order != "" {
		query += " ORDER BY " + order
	}
	query += " LIMIT 1"
	return b.store.queryOne(ctx, query, args)
}

// list 按条件查询多行（可选排序）。
// @param ctx 操作上下文
// @param where 条件（占位符从 1 开始；恒真条件用 "1 = 1"）
// @param args 条件参数
// @param order 排序列，空串表示不排序
// @return 行列表
// @return 查询错误
func (b baseRepo) list(ctx context.Context, where string, args []any, order string) ([]Row, error) {
	query := "SELECT " + b.store.quoteColumns(b.def.Columns) + " FROM " + b.def.Name + " WHERE " + where
	if order != "" {
		query += " ORDER BY " + order
	}
	return b.store.queryList(ctx, query, args)
}

// buildInClause 生成 "col IN (...)" 条件与对应参数。
// @param column 列名（占位符从 start 序号起）
// @param start 占位符起始序号
// @param values 值列表
// @return 条件文本
// @return 参数列表
func (s *Store) buildInClause(column string, start int, values []string) (string, []any) {
	args := make([]any, 0, len(values))
	placeholders := make([]string, 0, len(values))
	for i, value := range values {
		placeholders = append(placeholders, s.ph(start+i))
		args = append(args, value)
	}
	return s.quoteColumn(column) + " IN (" + strings.Join(placeholders, ", ") + ")", args
}
