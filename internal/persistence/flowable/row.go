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
		Columns: []string{"ID_", "REV_", "PROC_INST_ID_", "BUSINESS_KEY_", "PARENT_ID_", "PROC_DEF_ID_", "SUPER_EXEC_", "ROOT_PROC_INST_ID_", "ACT_ID_", "IS_ACTIVE_", "IS_CONCURRENT_", "IS_SCOPE_", "IS_EVENT_SCOPE_", "IS_MI_ROOT_", "SUSPENSION_STATE_", "CACHED_ENT_STATE_", "TENANT_ID_", "NAME_", "START_ACT_ID_", "START_TIME_", "START_USER_ID_", "LOCK_TIME_", "LOCK_OWNER_", "IS_COUNT_ENABLED_", "EVT_SUBSCR_COUNT_", "TASK_COUNT_", "JOB_COUNT_", "TIMER_JOB_COUNT_", "SUSP_JOB_COUNT_", "DEADLETTER_JOB_COUNT_", "EXTERNAL_WORKER_JOB_COUNT_", "VAR_COUNT_", "ID_LINK_COUNT_", "CALLBACK_ID_", "CALLBACK_TYPE_", "REFERENCE_ID_", "REFERENCE_TYPE_", "PROPAGATED_STAGE_INST_ID_", "BUSINESS_STATUS_", "DUE_DATE_"},
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
	id, ok := r.Values["ID_"]
	if !ok {
		return "", nil, api.NewError("ROW-0003", "行缺少 ID_", api.ErrBadRequest)
	}
	if _, exists := r.Values["REV_"]; !exists {
		return "", nil, api.NewError("ROW-0006", "行缺少 REV_", api.ErrBadRequest)
	}
	columns := r.insertColumns()
	assignments := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns)+2)
	for index, column := range columns {
		if column == "ID_" {
			continue
		}
		value := r.Values[column]
		if column == "REV_" {
			value = oldRevision + 1
		}
		assignments = append(assignments, fmt.Sprintf("%s = %s", column, placeholder(dialect, index+1)))
		args = append(args, value)
	}
	if len(assignments) == 0 {
		return "", nil, api.NewError("ROW-0007", "没有可更新的列", api.ErrBadRequest)
	}
	idPlaceholder := placeholder(dialect, len(assignments)+1)
	revisionPlaceholder := placeholder(dialect, len(assignments)+2)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE ID_ = %s AND REV_ = %s", r.Table, strings.Join(assignments, ", "), idPlaceholder, revisionPlaceholder)
	args = append(args, id, oldRevision)
	return query, args, nil
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
