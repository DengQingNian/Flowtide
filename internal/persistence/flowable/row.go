// Package flowable 提供 Flowable ACT_* 表的行映射与 SQL 生成。
package flowable

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// Table 描述一张 ACT_* 表的列。
type Table struct {
	PrimaryKeys []string // 复合主键列，非空时优先于单列主键
	Name        string   // 表名
	Columns     []string // 列顺序
	PrimaryKey  string   // 主键列，缺省为 ID_
}

var tables = map[string]Table{
	"ACT_ID_USER": {
		Name: "ACT_ID_USER", Columns: []string{"ID_", "REV_", "FIRST_", "LAST_", "DISPLAY_NAME_", "EMAIL_", "PWD_", "PICTURE_ID_", "TENANT_ID_"},
	},
	"ACT_ID_GROUP": {
		Name: "ACT_ID_GROUP", Columns: []string{"ID_", "REV_", "NAME_", "TYPE_"},
	},
	"ACT_ID_MEMBERSHIP": {
		Name: "ACT_ID_MEMBERSHIP", Columns: []string{"USER_ID_", "GROUP_ID_"}, PrimaryKeys: []string{"USER_ID_", "GROUP_ID_"},
	},
	"ACT_ID_INFO": {
		Name: "ACT_ID_INFO", Columns: []string{"ID_", "REV_", "USER_ID_", "TYPE_", "KEY_", "VALUE_", "PASSWORD_", "PARENT_ID_"},
	},
	"ACT_ID_BYTEARRAY": {
		Name: "ACT_ID_BYTEARRAY", Columns: []string{"ID_", "REV_", "NAME_", "BYTES_"},
	},
	"ACT_RE_DEPLOYMENT": {
		Name:    "ACT_RE_DEPLOYMENT",
		Columns: []string{"ID_", "NAME_", "CATEGORY_", "KEY_", "TENANT_ID_", "DEPLOY_TIME_", "DERIVED_FROM_", "DERIVED_FROM_ROOT_", "PARENT_DEPLOYMENT_ID_", "ENGINE_VERSION_"},
	},
	"ACT_RE_PROCDEF": {
		Name:    "ACT_RE_PROCDEF",
		Columns: []string{"ID_", "REV_", "CATEGORY_", "NAME_", "KEY_", "VERSION_", "DEPLOYMENT_ID_", "RESOURCE_NAME_", "DGRM_RESOURCE_NAME_", "DESCRIPTION_", "HAS_START_FORM_KEY_", "HAS_GRAPHICAL_NOTATION_", "SUSPENSION_STATE_", "TENANT_ID_", "ENGINE_VERSION_", "DERIVED_FROM_", "DERIVED_FROM_ROOT_", "DERIVED_VERSION_"},
	},
	"ACT_GE_PROPERTY": {
		PrimaryKey: "NAME_",
		Name:       "ACT_GE_PROPERTY",
		Columns:    []string{"NAME_", "VALUE_", "REV_"},
	},
	"ACT_RE_MODEL": {
		Name:    "ACT_RE_MODEL",
		Columns: []string{"ID_", "REV_", "NAME_", "KEY_", "CATEGORY_", "CREATE_TIME_", "LAST_UPDATE_TIME_", "VERSION_", "META_INFO_", "DEPLOYMENT_ID_", "EDITOR_SOURCE_VALUE_ID_", "EDITOR_SOURCE_EXTRA_VALUE_ID_", "TENANT_ID_"},
	},
	"ACT_GE_BYTEARRAY": {
		Name:    "ACT_GE_BYTEARRAY",
		Columns: []string{"ID_", "REV_", "NAME_", "DEPLOYMENT_ID_", "BYTES_", "GENERATED_"},
	},
	"ACT_RU_EXECUTION": {
		Name:    "ACT_RU_EXECUTION",
		Columns: []string{"ID_", "REV_", "PROC_INST_ID_", "BUSINESS_KEY_", "PARENT_ID_", "PROC_DEF_ID_", "SUPER_EXEC_", "ROOT_PROC_INST_ID_", "ACT_ID_", "IS_ACTIVE_", "IS_CONCURRENT_", "IS_SCOPE_", "IS_EVENT_SCOPE_", "IS_MI_ROOT_", "SUSPENSION_STATE_", "CACHED_ENT_STATE_", "TENANT_ID_", "NAME_", "START_ACT_ID_", "START_TIME_", "START_USER_ID_", "LOCK_TIME_", "LOCK_OWNER_", "IS_COUNT_ENABLED_", "EVT_SUBSCR_COUNT_", "TASK_COUNT_", "JOB_COUNT_", "TIMER_JOB_COUNT_", "SUSP_JOB_COUNT_", "DEADLETTER_JOB_COUNT_", "EXTERNAL_WORKER_JOB_COUNT_", "VAR_COUNT_", "ID_LINK_COUNT_", "CALLBACK_ID_", "CALLBACK_TYPE_", "REFERENCE_ID_", "REFERENCE_TYPE_", "PROPAGATED_STAGE_INST_ID_", "BUSINESS_STATUS_", "DUE_DATE_", "CLAIM_TIME_", "CLAIMED_BY_"},
	},
	"ACT_RU_IDENTITYLINK": {
		Name:    "ACT_RU_IDENTITYLINK",
		Columns: []string{"ID_", "REV_", "GROUP_ID_", "TYPE_", "USER_ID_", "TASK_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_"},
	},
	"ACT_RU_TASK": {
		Name:    "ACT_RU_TASK",
		Columns: []string{"ID_", "REV_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "TASK_DEF_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_", "PROPAGATED_STAGE_INST_ID_", "STATE_", "NAME_", "PARENT_TASK_ID_", "DESCRIPTION_", "TASK_DEF_KEY_", "OWNER_", "ASSIGNEE_", "DELEGATION_", "PRIORITY_", "CREATE_TIME_", "IN_PROGRESS_TIME_", "IN_PROGRESS_STARTED_BY_", "CLAIM_TIME_", "CLAIMED_BY_", "SUSPENDED_TIME_", "SUSPENDED_BY_", "IN_PROGRESS_DUE_DATE_", "DUE_DATE_", "CATEGORY_", "SUSPENSION_STATE_", "TENANT_ID_", "FORM_KEY_", "IS_COUNT_ENABLED_", "VAR_COUNT_", "ID_LINK_COUNT_", "SUB_TASK_COUNT_"},
	},
	"ACT_RU_VARIABLE": {
		Name:    "ACT_RU_VARIABLE",
		Columns: []string{"ID_", "REV_", "TYPE_", "NAME_", "EXECUTION_ID_", "PROC_INST_ID_", "TASK_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "BYTEARRAY_ID_", "DOUBLE_", "LONG_", "TEXT_", "TEXT2_", "META_INFO_"},
	},
	"ACT_RU_JOB": {
		Name:    "ACT_RU_JOB",
		Columns: []string{"ID_", "REV_", "CATEGORY_", "TYPE_", "LOCK_EXP_TIME_", "LOCK_OWNER_", "EXCLUSIVE_", "EXECUTION_ID_", "PROCESS_INSTANCE_ID_", "PROC_DEF_ID_", "ELEMENT_ID_", "ELEMENT_NAME_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_", "CORRELATION_ID_", "RETRIES_", "EXCEPTION_STACK_ID_", "EXCEPTION_MSG_", "DUEDATE_", "REPEAT_", "HANDLER_TYPE_", "HANDLER_CFG_", "CUSTOM_VALUES_ID_", "CREATE_TIME_", "TENANT_ID_"},
	},
	"ACT_RU_DEADLETTER_JOB": {
		Name:    "ACT_RU_DEADLETTER_JOB",
		Columns: []string{"ID_", "REV_", "CATEGORY_", "TYPE_", "EXCLUSIVE_", "EXECUTION_ID_", "PROCESS_INSTANCE_ID_", "PROC_DEF_ID_", "ELEMENT_ID_", "ELEMENT_NAME_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_", "CORRELATION_ID_", "EXCEPTION_STACK_ID_", "EXCEPTION_MSG_", "DUEDATE_", "REPEAT_", "HANDLER_TYPE_", "HANDLER_CFG_", "CUSTOM_VALUES_ID_", "CREATE_TIME_", "TENANT_ID_"},
	},
	"ACT_HI_PROCINST": {
		Name:    "ACT_HI_PROCINST",
		Columns: []string{"ID_", "REV_", "PROC_INST_ID_", "BUSINESS_KEY_", "PROC_DEF_ID_", "START_TIME_", "END_TIME_", "DURATION_", "START_USER_ID_", "START_ACT_ID_", "END_ACT_ID_", "SUPER_PROCESS_INSTANCE_ID_", "DELETE_REASON_", "TENANT_ID_", "NAME_", "CALLBACK_ID_", "CALLBACK_TYPE_", "REFERENCE_ID_", "REFERENCE_TYPE_", "PROPAGATED_STAGE_INST_ID_", "BUSINESS_STATUS_", "DUE_DATE_", "CLAIM_TIME_", "CLAIMED_BY_", "END_USER_ID_", "STATE_"},
	},
	"ACT_HI_ACTINST": {
		Name:    "ACT_HI_ACTINST",
		Columns: []string{"ID_", "REV_", "PROC_DEF_ID_", "PROC_INST_ID_", "EXECUTION_ID_", "ACT_ID_", "TASK_ID_", "CALL_PROC_INST_ID_", "ACT_NAME_", "ACT_TYPE_", "ASSIGNEE_", "COMPLETED_BY_", "START_TIME_", "END_TIME_", "TRANSACTION_ORDER_", "DURATION_", "DELETE_REASON_", "TENANT_ID_"},
	},
	"ACT_HI_TASKINST": {
		Name:    "ACT_HI_TASKINST",
		Columns: []string{"ID_", "REV_", "PROC_DEF_ID_", "TASK_DEF_ID_", "TASK_DEF_KEY_", "PROC_INST_ID_", "EXECUTION_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_", "PROPAGATED_STAGE_INST_ID_", "STATE_", "NAME_", "PARENT_TASK_ID_", "DESCRIPTION_", "OWNER_", "ASSIGNEE_", "START_TIME_", "IN_PROGRESS_TIME_", "IN_PROGRESS_STARTED_BY_", "CLAIM_TIME_", "CLAIMED_BY_", "SUSPENDED_TIME_", "SUSPENDED_BY_", "END_TIME_", "COMPLETED_BY_", "DURATION_", "DELETE_REASON_", "PRIORITY_", "IN_PROGRESS_DUE_DATE_", "DUE_DATE_", "FORM_KEY_", "CATEGORY_", "TENANT_ID_", "LAST_UPDATED_TIME_"},
	},
	"ACT_HI_COMMENT": {
		Name:    "ACT_HI_COMMENT",
		Columns: []string{"ID_", "TYPE_", "TIME_", "USER_ID_", "TASK_ID_", "PROC_INST_ID_", "ACTION_", "MESSAGE_", "FULL_MSG_"},
	},
	"ACT_HI_TSK_LOG": {
		Name:    "ACT_HI_TSK_LOG",
		Columns: []string{"ID_", "TYPE_", "TASK_ID_", "TIME_STAMP_", "USER_ID_", "DATA_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_DEFINITION_ID_", "TENANT_ID_"},
	},
	"ACT_HI_DETAIL": {
		Name:    "ACT_HI_DETAIL",
		Columns: []string{"ID_", "TYPE_", "PROC_INST_ID_", "EXECUTION_ID_", "TASK_ID_", "ACT_INST_ID_", "NAME_", "VAR_TYPE_", "REV_", "TIME_", "BYTEARRAY_ID_", "DOUBLE_", "LONG_", "TEXT_", "TEXT2_"},
	},
	"ACT_HI_IDENTITYLINK": {
		Name:    "ACT_HI_IDENTITYLINK",
		Columns: []string{"ID_", "GROUP_ID_", "TYPE_", "USER_ID_", "TASK_ID_", "CREATE_TIME_", "PROC_INST_ID_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_"},
	},
	"ACT_HI_ATTACHMENT": {
		Name:    "ACT_HI_ATTACHMENT",
		Columns: []string{"ID_", "REV_", "USER_ID_", "NAME_", "DESCRIPTION_", "TYPE_", "TASK_ID_", "PROC_INST_ID_", "URL_", "CONTENT_ID_", "TIME_"},
	},
	"ACT_HI_VARINST": {
		Name:    "ACT_HI_VARINST",
		Columns: []string{"ID_", "REV_", "PROC_INST_ID_", "EXECUTION_ID_", "TASK_ID_", "NAME_", "VAR_TYPE_", "SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "BYTEARRAY_ID_", "DOUBLE_", "LONG_", "TEXT_", "TEXT2_", "META_INFO_", "CREATE_TIME_", "LAST_UPDATED_TIME_"},
	},
	"ACT_RU_EVENT_SUBSCR": {
		Name:    "ACT_RU_EVENT_SUBSCR",
		Columns: []string{"ID_", "REV_", "EVENT_TYPE_", "EVENT_NAME_", "EXECUTION_ID_", "PROC_INST_ID_", "ACTIVITY_ID_", "CONFIGURATION_", "CREATED_", "PROC_DEF_ID_", "SUB_SCOPE_ID_", "SCOPE_ID_", "SCOPE_DEFINITION_ID_", "SCOPE_DEFINITION_KEY_", "SCOPE_TYPE_", "LOCK_TIME_", "LOCK_OWNER_", "TENANT_ID_"},
	},
}

// Row 是一行 ACT_* 数据。
type Row struct {
	Table         string         // 表名
	Values        map[string]any // 列值
	InsertColumns []string       // 可选的自定义插入列
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
			if text, ok := value.(string); ok && text == "" {
				if _, foreign := foreignKeys[r.Table][column]; foreign {
					value = nil
				}
			}
			r.Values[column] = value
			return nil
		}
	}
	return api.NewError("ROW-0002", "不支持的列: "+column, api.ErrBadRequest)
}

// SetInsertColumns 设置插入列子集。
// @param columns 列名
// @return 错误
func (r *Row) SetInsertColumns(columns []string) error {
	table, ok := tables[r.Table]
	if !ok {
		return api.NewError("ROW-0001", "不支持的表: "+r.Table, api.ErrBadRequest)
	}
	if len(columns) == 0 {
		return api.NewError("ROW-0005", "插入列不能为空", api.ErrBadRequest)
	}
	for _, column := range columns {
		valid := false
		for _, name := range table.Columns {
			if name == column {
				valid = true
				break
			}
		}
		if !valid {
			return api.NewError("ROW-0002", "不支持的列: "+column, api.ErrBadRequest)
		}
	}
	r.InsertColumns = append([]string(nil), columns...)
	return nil
}

// Columns 返回表的列顺序。
// @return 列名列表
func (r *Row) Columns() []string {
	return append([]string(nil), tables[r.Table].Columns...)
}

// Args 按插入列顺序返回值。
// @return 值列表
func (r *Row) Args() []any {
	columns := r.insertColumns()
	args := make([]any, len(columns))
	for i, column := range columns {
		args[i] = sqlRowValue(r.Values[column])
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
	columns := r.insertColumns()
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = placeholder(dialect, i+1)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", r.Table, strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	return query, r.Args(), nil
}

// UpdateSQL 生成带 REV_ 乐观锁的更新语句。
// @param dialect 数据库方言
// @param oldRevision 旧版本号
// @return SQL
// @return 参数
// @return 错误
func (r *Row) UpdateSQL(dialect string, oldRevision int64) (string, []any, error) {
	if err := validateDialect(dialect); err != nil {
		return "", nil, err
	}
	keys := primaryKeys(r.Table)
	keySet := make(map[string]bool, len(keys))
	for _, key := range keys {
		if value, exists := r.Values[key]; !exists || value == nil {
			return "", nil, api.NewError("ROW-0003", "行缺少主键: "+key, api.ErrBadRequest)
		}
		keySet[key] = true
	}
	versioned := tableHasColumn(r.Table, "REV_")
	if _, exists := r.Values["REV_"]; versioned && !exists {
		return "", nil, api.NewError("ROW-0006", "行缺少 REV_", api.ErrBadRequest)
	}
	assignments := make([]string, 0)
	args := make([]any, 0)
	// 跳过主键后按实际参数数编号，避免 PostgreSQL 缺号或复用参数。
	for _, column := range r.insertColumns() {
		if keySet[column] {
			continue
		}
		value := r.Values[column]
		if column == "REV_" {
			value = oldRevision + 1
		}
		args = append(args, sqlRowValue(value))
		assignments = append(assignments, fmt.Sprintf("%s = %s", column, placeholder(dialect, len(args))))
	}
	if len(assignments) == 0 {
		return "", nil, api.NewError("ROW-0007", "没有可更新的列", api.ErrBadRequest)
	}
	conditions := make([]string, 0, len(keys))
	for _, key := range keys {
		args = append(args, r.Values[key])
		conditions = append(conditions, key+" = "+placeholder(dialect, len(args)))
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", r.Table, strings.Join(assignments, ", "), strings.Join(conditions, " AND "))
	if versioned {
		args = append(args, oldRevision)
		query += " AND REV_ = " + placeholder(dialect, len(args))
	}
	return query, args, nil
}

// sqlRowValue 将有时区的时间参数转为 UTC，防止无时区 SQL 列在重启后产生绝对时间漂移。
// @param value 原始列值
// @return 可绑定的 SQL 参数，非时间值保持原样
func sqlRowValue(value any) any {
	switch timestamp := value.(type) {
	case time.Time:
		return timestamp.UTC()
	case *time.Time:
		// 历史与运行时的可空时间使用相同编码，空指针必须绑定为 SQL NULL。
		if timestamp == nil {
			return nil
		}
		return timestamp.UTC()
	}
	return value
}

// DeleteSQL 生成按真实主键及可用修订号删除的语句。
// @param dialect 数据库方言
// @return SQL
// @return 参数
// @return 错误
func (r *Row) DeleteSQL(dialect string) (string, []any, error) {
	if err := validateDialect(dialect); err != nil {
		return "", nil, err
	}
	keys := primaryKeys(r.Table)
	conditions := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)+1)
	// 复合主键必须包含全部分量，不能用第一列扩大删除范围。
	for _, key := range keys {
		value, exists := r.Values[key]
		if !exists || value == nil {
			return "", nil, api.NewError("ROW-0003", "行缺少主键: "+key, api.ErrBadRequest)
		}
		args = append(args, value)
		conditions = append(conditions, key+" = "+placeholder(dialect, len(args)))
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", r.Table, strings.Join(conditions, " AND "))
	if revision, ok := revisionValue(*r); ok {
		args = append(args, revision)
		query += " AND REV_ = " + placeholder(dialect, len(args))
	}
	return query, args, nil
}

// primaryKeys 返回实际主键的全部列，兼容既有单列主键表。
// @param table 表名
// @return 独立的主键列列表
func primaryKeys(table string) []string {
	if keys := tables[table].PrimaryKeys; len(keys) > 0 {
		return append([]string(nil), keys...)
	}
	return []string{primaryKey(table)}
}

// primaryKey 返回当前支持表的实际主键列。
// @param table 表名
// @return 主键列
func primaryKey(table string) string {
	if key := tables[table].PrimaryKey; key != "" {
		return key
	}
	return "ID_"
}

// tableHasColumn 判断表元数据是否包含指定列。
// @param table 表名
// @param column 列名
// @return 是否存在
func tableHasColumn(table, column string) bool {
	for _, name := range tables[table].Columns {
		if name == column {
			return true
		}
	}
	return false
}

// insertColumns 返回实际插入列。
// @return 列名
func (r *Row) insertColumns() []string {
	if len(r.InsertColumns) > 0 {
		return append([]string(nil), r.InsertColumns...)
	}
	return r.Columns()
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
