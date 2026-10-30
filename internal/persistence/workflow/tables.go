package workflow

import "fmt"

// tableDef 描述一张 workflow 表的列元数据，列名与 DDL（schema/sql/*/002-workflow-1.0.sql）保持一致。
type tableDef struct {
	Name    string   // 表名（小写，与 DDL 一致）
	Columns []string // 全部列（首列恒为 id）
}

// has 判断表是否包含指定列。
// @param column 列名
// @return 是否包含
func (t tableDef) has(column string) bool {
	for _, name := range t.Columns {
		if name == column {
			return true
		}
	}
	return false
}

// orderedColumns 按表定义顺序返回行数据中实际出现的列，保证 SQL 参数顺序确定。
// @param data 行数据
// @return 排序后的列名列表
func (t tableDef) orderedColumns(data Row) []string {
	columns := make([]string, 0, len(data))
	for _, name := range t.Columns {
		if _, ok := data[name]; ok {
			columns = append(columns, name)
		}
	}
	return columns
}

// commonTailColumns 公共审计列（对应 003 文档 §2，所有表均包含）。
var commonTailColumns = []string{"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// concatColumns 拼接列组为新切片，避免多表共享底层数组。
// @param groups 列组
// @return 拼接后的新切片
func concatColumns(groups ...[]string) []string {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	columns := make([]string, 0, total)
	for _, group := range groups {
		columns = append(columns, group...)
	}
	return columns
}

// extendColumns 生成 26 个通用扩展列（extend_s1..s8/i1..i3/f1..f3/d1..d3）。
// @return 扩展列列表
func extendColumns() []string {
	columns := make([]string, 0, 17)
	// s 串型 8 个、i 整型 3 个、f 浮点 3 个、d 时间 3 个，顺序与 Java 实体一致
	for i := 1; i <= 8; i++ {
		columns = append(columns, fmt.Sprintf("extend_s%d", i))
	}
	for i := 1; i <= 3; i++ {
		columns = append(columns, fmt.Sprintf("extend_i%d", i))
	}
	for i := 1; i <= 3; i++ {
		columns = append(columns, fmt.Sprintf("extend_f%d", i))
	}
	for i := 1; i <= 3; i++ {
		columns = append(columns, fmt.Sprintf("extend_d%d", i))
	}
	return columns
}

// messageExtendBaseColumns 待办/已办/委托镜像共用的业务扩展列（不含公共审计列）。
var messageExtendBaseColumns = []string{
	"id", "task_id", "process_instance_id", "title", "message", "business_id", "target_url",
	"task_step_name", "task_step_code", "flow_category_code", "flow_category_name",
	"owner_id", "owner_name", "owner_group", "owner_group_id", "owner_group_paths", "owner_group_path_ids",
	"document_owner_id", "document_owner_name", "document_group_id", "document_group", "document_group_paths", "document_group_path_ids",
	"sender_id", "sender", "sender_step_name", "sender_step_code",
	"consigner_id", "consigner", "is_read", "read_time", "plan_finish_time", "task_receipt_time", "task_start_time",
	"is_delay", "delay_time", "message_group_id",
}

// 表定义注册表，与 DDL 文件逐一对应。
var (
	tableBusinessSystems = tableDef{Name: "workflow_business_system", Columns: concatColumns(
		[]string{"id", "system_symbol", "system_name", "task_call_addr", "execution_call_addr", "secret"}, commonTailColumns)}

	tableModels = tableDef{Name: "workflow_model", Columns: concatColumns(
		[]string{"id", "resource_id", "model_name", "model_code", "tips", "target", "roles", "menu_icon", "remark",
			"sort", "parent_id", "enable", "return_sort", "is_leaf", "process_definition_key", "visible", "model_id", "last_deploy_time"}, commonTailColumns)}

	tableSteps = tableDef{Name: "workflow_step", Columns: concatColumns(
		[]string{"id", "name", "step_code", "description", "value", "account_name", "source", "model_id", "resource_id",
			"sort", "agree", "reject", "remark", "fix", "url", "visible", "task_type", "edit_info", "operate_auth",
			"allow_return", "back_step_codes", "back_step_names", "auto_complete", "auto_complete_start_time", "auto_complete_end_time",
			"time_limit", "time_limit_unit", "remind_time_limit", "remind_time_limit_unit",
			"channel_type_name", "channel_type_name_cn", "return_approval_step", "countersign_percent_return", "return_percent"}, commonTailColumns)}

	tableHisSteps = tableDef{Name: "workflow_his_step", Columns: concatColumns(
		[]string{"id", "step_id", "step_name", "step_code", "account_name", "model_id", "sort",
			"agree", "reject", "remark", "url", "visible", "task_type", "status",
			"step_creator", "step_create_time", "step_updater", "step_update_time",
			"deployment_id", "edit_info", "operate_auth", "process_instance_id",
			"description", "value", "fix", "resource_id", "source",
			"allow_return", "back_step_codes", "back_step_names", "auto_complete", "auto_complete_start_time", "auto_complete_end_time",
			"time_limit", "time_limit_unit", "remind_time_limit", "remind_time_limit_unit",
			"channel_type_name", "channel_type_name_cn", "return_approval_step", "countersign_percent_return", "return_percent"}, commonTailColumns)}

	tableTemplates = tableDef{Name: "workflow_template", Columns: concatColumns(
		[]string{"id", "name", "layout", "flow_xml", "is_visible", "is_default", "source", "category_id"}, commonTailColumns)}

	tableActExtends = tableDef{Name: "workflow_act_extend", Columns: concatColumns(
		[]string{"id", "activity_id", "activity_name", "process_definition_key", "owner_type", "owner_value", "step_id"}, commonTailColumns)}

	tableCountersignWeights = tableDef{Name: "workflow_countersign_weight", Columns: concatColumns(
		[]string{"id", "activity_id", "process_definition_key", "agree_weight", "reject_weight", "identity_id"}, commonTailColumns)}

	tableNextActivities = tableDef{Name: "workflow_next_activity", Columns: concatColumns(
		[]string{"id", "next_activity_id", "group_id", "identity_id", "send_identity_id", "send_user_name",
			"process_instance_id", "agree_weight", "reject_weight", "used", "pre_task_id", "task_id", "version"}, commonTailColumns)}

	tableHisNextActivities = tableDef{Name: "workflow_his_next_activity", Columns: concatColumns(
		[]string{"id", "next_activity_id", "group_id", "identity_id", "send_identity_id", "send_user_name",
			"process_instance_id", "agree_weight", "reject_weight", "used", "pre_task_id", "task_id", "version"}, commonTailColumns)}

	tableMessageExtends = tableDef{Name: "workflow_task_message_extend", Columns: concatColumns(
		messageExtendBaseColumns, extendColumns(), commonTailColumns)}

	tableHistoryExtends = tableDef{Name: "workflow_task_history_extend", Columns: concatColumns(
		messageExtendBaseColumns, []string{"task_finish_time"}, extendColumns(), commonTailColumns)}

	tableDelegateTasks = tableDef{Name: "workflow_delegate_task", Columns: concatColumns(
		messageExtendBaseColumns, []string{"entrust_id", "entrust_name"}, extendColumns(), commonTailColumns)}

	tableDocuments = tableDef{Name: "workflow_document", Columns: concatColumns(
		[]string{"id", "business_id", "create_group_path_ids", "create_group_paths", "create_group_id", "create_group",
			"updater_name", "deleted", "document_url", "document_id", "document_table", "flow_title",
			"flow_task_id", "flow_instance_id", "flow_category_id", "flow_category", "flow_action_id", "flow_action",
			"flow_status", "flow_current_step_name", "flow_current_persons",
			"flow_start_time", "flow_finish_time", "flow_termination_time"},
		extendColumns(), commonTailColumns)}

	tableOperationRecords = tableDef{Name: "workflow_operation_record", Columns: concatColumns(
		[]string{"id", "business_id", "task_id", "process_instance_id", "message", "is_agree",
			"task_step_code", "task_step_name", "flow_category_code", "flow_category_name",
			"owner_id", "owner_name", "owner_group", "owner_group_id", "owner_group_paths", "owner_group_path_ids",
			"deal_time", "task_receipt_time", "task_start_time", "task_finish_time", "type"}, commonTailColumns)}

	tableReviewTasks = tableDef{Name: "workflow_review_task", Columns: concatColumns(
		[]string{"id", "start_time", "task_id", "task_name", "process_instance_id", "process_definition_id",
			"task_key", "business_key", "process_name", "process_definition_key", "process_definition_name",
			"sender", "sender_name", "reviewer", "review_time", "remark", "opinion", "status"}, commonTailColumns)}

	tableTaskExceptions = tableDef{Name: "workflow_task_exception", Columns: concatColumns(
		[]string{"id", "task_id", "process_instance_id", "process_instance_name", "class_name", "method_name",
			"message", "params", "exception_type", "exception_class",
			"process_definition_id", "process_definition_key", "process_definition_name", "start_user_id", "handle_status"}, commonTailColumns)}

	tableCompleteToActivities = tableDef{Name: "workflow_complete_to_activity", Columns: concatColumns(
		[]string{"id", "flow_id", "source_flow_element_id", "target_flow_element_id", "task_id", "process_instance_id"}, commonTailColumns)}

	tableTaskDelegates = tableDef{Name: "workflow_task_delegate", Columns: concatColumns(
		[]string{"id", "consign_type", "owner_id", "owner_name", "source", "source_code", "source_name", "type",
			"condition", "vaild_from", "vaild_to", "enable", "retire", "remark"}, commonTailColumns)}

	tableTaskConsigners = tableDef{Name: "workflow_task_consigners", Columns: concatColumns(
		[]string{"id", "delegate_id", "consigner_id", "consigner_name"}, commonTailColumns)}

	tableTaskConsignObjs = tableDef{Name: "workflow_task_consign_obj", Columns: concatColumns(
		[]string{"id", "delegate_id", "owner_type", "owner_id", "owner_name"}, commonTailColumns)}

	tableReminds = tableDef{Name: "workflow_remind", Columns: concatColumns(
		[]string{"id", "pid", "step_ids", "enable_pre", "enable_post", "sent", "deleted"}, commonTailColumns)}

	tableRemindTargets = tableDef{Name: "workflow_remind_target", Columns: concatColumns(
		[]string{"id", "pid", "executor_id", "enable", "name", "target_type", "type", "retry",
			"retry_duration", "retry_times", "retry_count", "retry_time_unit", "ahead", "delay",
			"by_plan_complete_time", "removable", "sent", "deleted"}, commonTailColumns)}
)
