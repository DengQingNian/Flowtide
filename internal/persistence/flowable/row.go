// Package flowable 提供 Flowable ACT_* 表的行映射与 SQL 生成。
package flowable

import (
	"fmt"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// Table 描述一张 ACT_* 表的列。
type Table struct {
	Name    string   // 表名
	Columns []string // 列顺序
}

var tables = map[string]Table{
	"ACT_RE_DEPLOYMENT": {
		Name:    "ACT_RE_DEPLOYMENT",
		Columns: []string{"ID_", "NAME_", "CATEGORY_", "KEY_", "TENANT_ID_", "DEPLOY_TIME_", "DERIVED_FROM_", "DERIVED_FROM_ROOT_", "PARENT_DEPLOYMENT_ID_", "ENGINE_VERSION_"},
	},
	"ACT_RE_PROCDEF": {
		Name:    "ACT_RE_PROCDEF",
		Columns: []string{"ID_", "REV_", "CATEGORY_", "NAME_", "KEY_", "VERSION_", "DEPLOYMENT_ID_", "RESOURCE_NAME_", "DGRM_RESOURCE_NAME_", "DESCRIPTION_", "HAS_START_FORM_KEY_", "HAS_GRAPHICAL_NOTATION_", "SUSPENSION_STATE_", "TENANT_ID_", "ENGINE_VERSION_", "DERIVED_FROM_", "DERIVED_FROM_ROOT_", "DERIVED_VERSION_"},
	},
	"ACT_GE_BYTEARRAY": {
		Name:    "ACT_GE_BYTEARRAY",
		Columns: []string{"ID_", "REV_", "NAME_", "DEPLOYMENT_ID_", "BYTES_", "GENERATED_"},
	},
}

// Row 是一行 ACT_* 数据。
type Row struct {
	Table  string         // 表名
	Values map[string]any // 列值
}

// NewRow 创建行并校验表名。
// @param table 表名
// @return 行
// @return 错误
func NewRow(table string) (*Row, error) {
	if _, ok := tables[table]; !ok {
		return nil, api.NewError("ROW-0001", "不支持的表: "+table, api.ErrBadRequest)
	}
	return &Row{Table: table, Values: make(map[string]any)}, nil
}

// Set 设置列值并校验列名。
// @param column 列名
// @param value 值
// @return 错误
func (r *Row) Set(column string, value any) error {
	table, ok := tables[r.Table]
	if !ok {
		return api.NewError("ROW-0001", "不支持的表: "+r.Table, api.ErrBadRequest)
	}
	for _, name := range table.Columns {
		if name == column {
			r.Values[column] = value
			return nil
		}
	}
	return api.NewError("ROW-0002", "不支持的列: "+column, api.ErrBadRequest)
}

// Columns 返回表的列顺序。
// @return 列名列表
func (r *Row) Columns() []string {
	return append([]string(nil), tables[r.Table].Columns...)
}

// Args 按列顺序返回值。
// @return 值列表
func (r *Row) Args() []any {
	columns := r.Columns()
	args := make([]any, len(columns))
	for i, column := range columns {
		args[i] = r.Values[column]
	}
	return args
}

// InsertSQL 生成 INSERT 语句。
// @param dialect 数据库方言
// @return SQL
// @return 参数
// @return 错误
func (r *Row) InsertSQL(dialect string) (string, []any, error) {
	if err := validateDialect(dialect); err != nil {
		return "", nil, err
	}
	columns := r.Columns()
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = placeholder(dialect, i+1)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", r.Table, strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	return query, r.Args(), nil
}

// DeleteSQL 生成按 ID_ 删除语句。
// @param dialect 数据库方言
// @return SQL
// @return 参数
// @return 错误
func (r *Row) DeleteSQL(dialect string) (string, []any, error) {
	if err := validateDialect(dialect); err != nil {
		return "", nil, err
	}
	id, ok := r.Values["ID_"]
	if !ok {
		return "", nil, api.NewError("ROW-0003", "行缺少 ID_", api.ErrBadRequest)
	}
	return fmt.Sprintf("DELETE FROM %s WHERE ID_ = %s", r.Table, placeholder(dialect, 1)), []any{id}, nil
}

// Tables 返回支持的表名。
// @return 表名列表
func Tables() []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// validateDialect 校验方言。
// @param dialect 方言
// @return 错误
func validateDialect(dialect string) error {
	if dialect != "mysql" && dialect != "postgres" {
		return api.NewError("ROW-0004", "不支持的方言: "+dialect, api.ErrBadRequest)
	}
	return nil
}

// placeholder 返回方言占位符。
// @param dialect 方言
// @param index 序号
// @return 占位符
func placeholder(dialect string, index int) string {
	if dialect == "postgres" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}
