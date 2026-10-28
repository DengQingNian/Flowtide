package migration

import (
	"reflect"
	"time"

	"github.com/flowable-go/flowable/internal/persistence/flowable"
)

// 可选 audit 表只在实际存在时检查，最低必需表仍使用原有十四张表。
var auditAnalysisTables = []string{"ACT_RU_ACTINST", "ACT_RU_IDENTITYLINK", "ACT_HI_PROCINST", "ACT_HI_ACTINST", "ACT_HI_TASKINST", "ACT_HI_VARINST", "ACT_HI_IDENTITYLINK", "ACT_HI_COMMENT"}

// checkAuditRow 按 Java audit 列映射检查历史与身份链接。
// @param row 来源实体
// @return 无
func (s *analysisState) checkAuditRow(row flowable.Row) {
	// 各历史类型保持独立生命周期，已经完成的实体不强制引用运行时行。
	switch row.Table {
	case "ACT_RU_ACTINST":
		s.checkRuntimeActivity(row)
	case "ACT_HI_PROCINST":
		s.checkHistoricProcess(row)
	case "ACT_HI_ACTINST":
		s.checkHistoricActivity(row)
	case "ACT_HI_TASKINST":
		s.checkHistoricTask(row)
	case "ACT_HI_VARINST":
		s.checkHistoricVariable(row)
	case "ACT_RU_IDENTITYLINK", "ACT_HI_IDENTITYLINK":
		s.checkIdentityLink(row)
	case "ACT_HI_COMMENT":
		s.checkHistoricComment(row)
	}
}

// auditRelatedRows 收集历史和身份实体的全部已存在归属行，供租户及冲突检查。
// @param row 待检查实体
// @return 关联实体
func (s *analysisState) auditRelatedRows(row flowable.Row) []flowable.Row {
	result := []flowable.Row{}
	instance, task, definition, execution := analysisString(row, "PROC_INST_ID_"), analysisString(row, "TASK_ID_"), analysisString(row, "PROC_DEF_ID_"), analysisString(row, "EXECUTION_ID_")
	// 运行时和历史关联同时保留，不能用一个有效归属掩盖另一个跨租户引用。
	for _, reference := range []struct{ table, id string }{{"ACT_HI_PROCINST", instance}, {"ACT_RU_EXECUTION", instance}, {"ACT_HI_TASKINST", task}, {"ACT_RU_TASK", task}, {"ACT_RE_PROCDEF", definition}, {"ACT_RU_EXECUTION", execution}} {
		if owner := s.index[reference.table][reference.id]; owner.Values != nil {
			result = append(result, owner)
		}
	}
	return result
}

// auditInScope 判断关联历史及身份实体是否涉及目标租户，无法归属时全局保守检查。
// @param row 待检查实体
// @return 是否属于已分析 audit 类型
// @return 是否纳入范围
func (s *analysisState) auditInScope(row flowable.Row) (bool, bool) {
	analyzed := false
	for _, table := range auditAnalysisTables {
		if row.Table == table {
			analyzed = true
			break
		}
	}
	if !analyzed {
		return false, false
	}
	related := s.auditRelatedRows(row)
	// 直接租户或任一关联命中都必须检查，以发现反向跨租户引用。
	if value, present := row.Values["TENANT_ID_"]; present && analysisText(value) == *s.options.TenantID {
		return true, true
	}
	for _, owner := range related {
		if analysisString(owner, "TENANT_ID_") == *s.options.TenantID {
			return true, true
		}
	}
	if _, present := row.Values["TENANT_ID_"]; !present && len(related) == 0 {
		return true, true
	}
	return true, false
}

// auditTenant 从关联历史流程、任务、定义或执行获取无直接租户列的实体归属。
// @param row 待检查实体
// @return 推导租户
func (s *analysisState) auditTenant(row flowable.Row) string {
	for _, owner := range s.auditRelatedRows(row) {
		if value, present := owner.Values["TENANT_ID_"]; present {
			return analysisText(value)
		}
	}
	return ""
}

// checkHistoricProcess 检查历史实例主键、定义、生命周期及不支持的跨引擎关系。
// @param row 历史流程行
// @return 无
func (s *analysisState) checkHistoricProcess(row flowable.Row) {
	instance := analysisString(row, "PROC_INST_ID_")
	if instance == "" || analysisID(row) != instance {
		s.add(row, "HISTORY_OWNER", "历史流程主键与流程实例标识缺失或不一致。")
	}
	definition := s.index["ACT_RE_PROCDEF"][analysisString(row, "PROC_DEF_ID_")]
	if definition.Values == nil || s.tenant(definition) != s.tenant(row) {
		s.add(row, "HISTORY_DEFINITION", "历史流程定义缺失或租户不一致。")
	}
	s.checkHistoryTime(row, "START_TIME_", "END_TIME_")
	s.checkHistoryRuntime(row, "ACT_RU_EXECUTION", instance)
	// 调用、回调和外部引用无法由基础历史导入器完整表达，保守阻断。
	for _, column := range []string{"SUPER_PROCESS_INSTANCE_ID_", "CALLBACK_ID_", "CALLBACK_TYPE_", "REFERENCE_ID_", "REFERENCE_TYPE_", "PROPAGATED_STAGE_INST_ID_"} {
		if analysisString(row, column) != "" {
			s.add(row, "HISTORY_SCOPE", "历史流程包含尚未支持的调用、回调或外部作用域。")
			break
		}
	}
}

// historicOwner 校验历史实体的流程、任务和现存执行引用，允许完成后执行已被删除。
// @param row 历史活动、任务或变量行
// @param standalone 是否允许无流程的独立任务或任务变量
// @return 所属历史流程或运行时根
func (s *analysisState) historicOwner(row flowable.Row, standalone bool) flowable.Row {
	instance := analysisString(row, "PROC_INST_ID_")
	owner := s.index["ACT_HI_PROCINST"][instance]
	if owner.Values == nil {
		owner = s.index["ACT_RU_EXECUTION"][instance]
	}
	valid := owner.Values != nil && analysisString(owner, "PROC_INST_ID_") == instance
	if instance == "" && standalone {
		valid = true
	}
	if tenant, present := row.Values["TENANT_ID_"]; present && owner.Values != nil && analysisText(tenant) != s.tenant(owner) {
		valid = false
	}
	// 定义列存在时必须与所属实例相同，不能只验证定义自身存在。
	definition := analysisString(row, "PROC_DEF_ID_")
	if (definition != "" || row.Table == "ACT_HI_ACTINST" || row.Table == "ACT_RU_ACTINST" || row.Table == "ACT_HI_TASKINST" && instance != "") && (definition == "" || definition != analysisString(owner, "PROC_DEF_ID_") || s.index["ACT_RE_PROCDEF"][definition].Values == nil) {
		s.add(row, "HISTORY_DEFINITION", "历史实体定义缺失或与流程实例定义不一致。")
	}
	if execution := s.index["ACT_RU_EXECUTION"][analysisString(row, "EXECUTION_ID_")]; execution.Values != nil && (analysisString(execution, "PROC_INST_ID_") != instance || owner.Values != nil && s.tenant(execution) != s.tenant(owner)) {
		valid = false
	}
	taskID := analysisString(row, "TASK_ID_")
	if taskID != "" {
		task := s.index["ACT_HI_TASKINST"][taskID]
		if task.Values == nil {
			task = s.index["ACT_RU_TASK"][taskID]
		}
		if task.Values == nil || analysisString(task, "PROC_INST_ID_") != instance || owner.Values != nil && s.tenant(task) != s.tenant(owner) {
			valid = false
		}
	}
	if !valid {
		s.add(row, "HISTORY_OWNER", "历史实体的流程、任务、执行或租户归属缺失或冲突。")
	}
	return owner
}

// checkHistoricActivity 检查活动模型节点及活动中的执行、任务关联。
// @param row 历史活动行
// @return 无
func (s *analysisState) checkHistoricActivity(row flowable.Row) {
	owner := s.historicOwner(row, false)
	process := s.processes[analysisString(owner, "PROC_DEF_ID_")]
	activity := analysisString(row, "ACT_ID_")
	if process == nil || process.Elements[activity] == nil || process.Elements[activity].ElementType() != analysisString(row, "ACT_TYPE_") {
		s.add(row, "HISTORY_ACTIVITY", "历史活动标识或类型与流程模型不一致。")
	}
	s.checkHistoryTime(row, "START_TIME_", "END_TIME_")
	if row.Values["END_TIME_"] == nil {
		s.checkHistoryRuntime(row, "ACT_RU_EXECUTION", analysisString(row, "EXECUTION_ID_"))
	}
	if analysisString(row, "CALL_PROC_INST_ID_") != "" {
		s.add(row, "HISTORY_SCOPE", "历史活动包含尚未支持的调用流程关系。")
	}
}

// checkHistoricTask 检查历史任务的模型、作用域、时间及尚未结束的运行时关联。
// @param row 历史任务行
// @return 无
func (s *analysisState) checkHistoricTask(row flowable.Row) {
	owner := s.historicOwner(row, true)
	// 独立历史任务没有流程活动键；流程任务必须对应同一模型中的用户任务。
	if analysisString(row, "PROC_INST_ID_") != "" {
		process := s.processes[analysisString(owner, "PROC_DEF_ID_")]
		key := analysisString(row, "TASK_DEF_KEY_")
		if process == nil || process.Elements[key] == nil || process.Elements[key].ElementType() != "userTask" {
			s.add(row, "HISTORY_ACTIVITY", "历史任务活动键与流程模型中的用户任务不一致。")
		}
	}
	s.checkAuditScope(row, "HISTORY_SCOPE")
	s.checkHistoryTime(row, "START_TIME_", "END_TIME_")
	s.checkHistoryRuntime(row, "ACT_RU_TASK", analysisID(row))
}

// checkHistoricVariable 检查历史变量归属及 VAR_TYPE_ 指定的值编码。
// @param row 历史变量行
// @return 无
func (s *analysisState) checkHistoricVariable(row flowable.Row) {
	s.historicOwner(row, analysisString(row, "TASK_ID_") != "")
	s.checkAuditScope(row, "HISTORY_SCOPE")
	s.checkHistoryTime(row, "CREATE_TIME_", "LAST_UPDATED_TIME_")
	s.checkVariableData(row, "VAR_TYPE_")
}

// checkHistoryRuntime 检查未结束审计实体的运行时对应项，并拒绝已结束实例及任务仍活跃。
// @param row 历史实体
// @param table 运行时表
// @param id 运行时主键
// @return 无
func (s *analysisState) checkHistoryRuntime(row flowable.Row, table, id string) {
	runtime := s.index[table][id]
	ended := row.Values["END_TIME_"] != nil
	// 已完成活动可以复用尚存执行，流程与任务则必须同步结束并移除运行时记录。
	if ended {
		if runtime.Values != nil {
			s.add(row, "HISTORY_RUNTIME", "已结束历史流程或任务仍有对应运行时实体。")
		}
		return
	}
	if runtime.Values == nil || analysisString(runtime, "PROC_INST_ID_") != analysisString(row, "PROC_INST_ID_") || s.tenant(runtime) != s.tenant(row) || analysisString(runtime, "PROC_DEF_ID_") != analysisString(row, "PROC_DEF_ID_") {
		s.add(row, "HISTORY_RUNTIME", "未结束历史实体缺少一致的运行时对应项。")
		return
	}
	if table == "ACT_RU_TASK" && (analysisString(runtime, "EXECUTION_ID_") != analysisString(row, "EXECUTION_ID_") || analysisString(runtime, "TASK_DEF_KEY_") != analysisString(row, "TASK_DEF_KEY_")) {
		s.add(row, "HISTORY_RUNTIME", "未结束历史任务的执行或活动键与运行时任务不一致。")
	}
	if (row.Table == "ACT_HI_ACTINST" || row.Table == "ACT_RU_ACTINST") && analysisString(runtime, "ACT_ID_") != analysisString(row, "ACT_ID_") {
		s.add(row, "HISTORY_RUNTIME", "未结束审计活动与运行时执行当前活动不一致。")
	}
}

// checkHistoryTime 检查必需起始时间及可选结束时间的格式和先后。
// @param row 历史实体
// @param startColumn 必需起始列
// @param endColumn 可选结束或更新时间列
// @return 无
func (s *analysisState) checkHistoryTime(row flowable.Row, startColumn, endColumn string) {
	start, startOK := analysisTime(row.Values[startColumn])
	if !startOK || start.IsZero() {
		s.add(row, "HISTORY_TIME", "历史实体起始时间缺失或格式错误。")
		return
	}
	// 可选结束列存在时必须可解析，并且不能早于已确认的起始时间。
	if row.Values[endColumn] != nil {
		end, ok := analysisTime(row.Values[endColumn])
		if !ok || end.Before(start) {
			s.add(row, "HISTORY_TIME", "历史实体结束或更新时间格式错误或早于起始时间。")
		}
	}
}

// analysisTime 兼容扫描器驱动时间及 Java 黄金样本的时间表示。
// @param value 时间列
// @return 解析时间
// @return 是否合法
func analysisTime(value any) (time.Time, bool) {
	if typed, ok := value.(time.Time); ok {
		return typed, true
	}
	// 与 ACT 解码器保持一致的文本格式，禁止宽松解析后丢失日期精度。
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, analysisText(value)); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// checkAuditScope 阻断基础 BPMN 归属无法表达的外部作用域列。
// @param row 审计或身份链接行
// @param code 问题码
// @return 无
func (s *analysisState) checkAuditScope(row flowable.Row, code string) {
	for _, column := range []string{"SCOPE_ID_", "SUB_SCOPE_ID_", "SCOPE_TYPE_", "SCOPE_DEFINITION_ID_", "PROPAGATED_STAGE_INST_ID_"} {
		if analysisString(row, column) != "" {
			s.add(row, code, "实体包含尚未支持的外部作用域。")
			return
		}
	}
}

// checkIdentityLink 检查 Java 标准身份链接类型、唯一主体及全部归属引用。
// @param row 运行时或历史身份链接行
// @return 无
func (s *analysisState) checkIdentityLink(row flowable.Row) {
	user, group := analysisString(row, "USER_ID_"), analysisString(row, "GROUP_ID_")
	if (user == "") == (group == "") {
		s.add(row, "IDENTITY_SUBJECT", "身份链接必须且只能指定一个用户或组主体。")
	}
	switch analysisString(row, "TYPE_") {
	case "assignee", "candidate", "owner", "starter", "participant", "reactivator":
	default:
		s.add(row, "IDENTITY_TYPE", "身份链接类型不在已验证的 Java 标准类型清单内。")
	}
	s.checkAuditScope(row, "IDENTITY_SCOPE")
	historical := row.Table == "ACT_HI_IDENTITYLINK"
	instance, taskID, definitionID := analysisString(row, "PROC_INST_ID_"), analysisString(row, "TASK_ID_"), analysisString(row, "PROC_DEF_ID_")
	valid := false
	owners := []flowable.Row{}
	// 每个非空引用都必须能解析，不能靠其他有效引用掩盖孤儿或跨租户数据。
	for _, reference := range []struct{ id, table string }{{instance, "ACT_RU_EXECUTION"}, {taskID, "ACT_RU_TASK"}, {definitionID, "ACT_RE_PROCDEF"}} {
		if reference.id == "" {
			continue
		}
		table := reference.table
		if historical {
			if table == "ACT_RU_EXECUTION" {
				table = "ACT_HI_PROCINST"
			}
			if table == "ACT_RU_TASK" {
				table = "ACT_HI_TASKINST"
			}
		}
		owner := s.index[table][reference.id]
		if owner.Values == nil {
			s.add(row, "IDENTITY_OWNER", "身份链接的流程、任务或定义归属缺失。")
			return
		}
		owners = append(owners, owner)
		valid = true
	}
	for _, owner := range owners {
		if s.tenant(owner) != s.tenant(row) || instance != "" && owner.Table != "ACT_RE_PROCDEF" && analysisString(owner, "PROC_INST_ID_") != instance || definitionID != "" && owner.Table != "ACT_RE_PROCDEF" && analysisString(owner, "PROC_DEF_ID_") != definitionID {
			valid = false
		}
	}
	if !valid {
		s.add(row, "IDENTITY_OWNER", "身份链接缺少有效归属或不同引用之间存在实例、定义及租户冲突。")
	}
	if historical {
		created, ok := analysisTime(row.Values["CREATE_TIME_"])
		if !ok || created.IsZero() {
			s.add(row, "HISTORY_TIME", "历史身份链接创建时间缺失或格式错误。")
		}
	}
}

// checkHistoricComment 检查评论与事件的类型、时间及任务或流程归属。
// @param row 历史评论行
// @return 无
func (s *analysisState) checkHistoricComment(row flowable.Row) {
	commentType := analysisString(row, "TYPE_")
	// Java 使用 comment 表示人工评论、event 表示任务事件，其他类型无法保证语义。
	switch commentType {
	case "comment", "event":
	default:
		s.add(row, "COMMENT_TYPE", "历史评论类型不在已验证的 Java 清单内。")
	}
	if _, ok := analysisTime(row.Values["TIME_"]); !ok {
		s.add(row, "HISTORY_TIME", "历史评论创建时间缺失或格式错误。")
	}
	instance, taskID := analysisString(row, "PROC_INST_ID_"), analysisString(row, "TASK_ID_")
	// 评论必须至少归属任务或流程之一，不能留下无法定位的孤儿事件。
	if instance == "" && taskID == "" {
		s.add(row, "COMMENT_OWNER", "历史评论缺少任务或流程归属。")
		return
	}
	valid := true
	if taskID != "" {
		task := s.index["ACT_HI_TASKINST"][taskID]
		if task.Values == nil {
			task = s.index["ACT_RU_TASK"][taskID]
		}
		if task.Values == nil || instance != "" && analysisString(task, "PROC_INST_ID_") != instance {
			valid = false
		}
	}
	if instance != "" {
		owner := s.index["ACT_HI_PROCINST"][instance]
		if owner.Values == nil {
			owner = s.index["ACT_RU_EXECUTION"][instance]
		}
		if owner.Values == nil {
			valid = false
		}
	}
	if !valid {
		s.add(row, "COMMENT_OWNER", "历史评论的任务或流程归属缺失或与实例冲突。")
	}
}

// auditTable 判断表是否属于已分析 audit 扩展。
// @param table 表名
// @return 是否 audit 表
func auditTable(table string) bool {
	// 可选表清单用于租户推导和校验分发，保持同一能力边界。
	for _, name := range auditAnalysisTables {
		if name == table {
			return true
		}
	}
	return false
}

// checkRuntimeActivity 只接受存在完整同标识历史副本的运行时活动，防止排除重复容器时丢失独有业务信息。
// @param row Java 运行时活动行
// @return 无
func (s *analysisState) checkRuntimeActivity(row flowable.Row) {
	s.checkHistoricActivity(row)
	history := s.index["ACT_HI_ACTINST"][analysisID(row)]
	if history.Values == nil {
		s.add(row, "RUNTIME_ACTIVITY_HISTORY", "运行时活动缺少相同标识的完整历史副本。")
		return
	}
	// 修订号仅属于各表存储状态，其余任一业务列或扩展列都必须具有等价副本。
	columns := make(map[string]bool)
	for column := range row.Values {
		columns[column] = true
	}
	for column := range history.Values {
		columns[column] = true
	}
	for column := range columns {
		if column != "REV_" && !equivalentActivityValue(row.Values[column], history.Values[column]) {
			s.add(row, "RUNTIME_ACTIVITY_HISTORY", "运行时活动与历史副本存在不同业务列，禁止丢弃原始信息。")
			return
		}
	}
}

// equivalentActivityValue 比较驱动及黄金样本中的同一活动列值，空文本统一为空值。
// @param left 运行时列值
// @param right 历史列值
// @return 是否等价
func equivalentActivityValue(left, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}
	// Java 构造历史对象时允许空租户及文本由空值归一为空串。
	if (left == nil || analysisText(left) != "" || reflect.TypeOf(left) == reflect.TypeOf("")) && (right == nil || analysisText(right) != "" || reflect.TypeOf(right) == reflect.TypeOf("")) {
		return analysisText(left) == analysisText(right)
	}
	leftTime, leftOK := analysisTime(left)
	rightTime, rightOK := analysisTime(right)
	return leftOK && rightOK && leftTime.Equal(rightTime)
}
