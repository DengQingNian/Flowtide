// Package migration 提供 Java 存量数据库的只读迁移风险预检。
package migration

import (
	"sort"
	"strings"

	"github.com/flowable-go/flowable/bpmn/model"
	"github.com/flowable-go/flowable/internal/persistence/flowable"
	"github.com/flowable-go/flowable/internal/persistence/schema"
)

// Options 指定预检范围，空指针表示全部租户，空字符串表示默认租户。
type Options struct{ TenantID *string }

// Issue 记录不包含业务值和源码的逐实体迁移问题。
type Issue struct {
	Code              string `json:"code"`
	Table             string `json:"table"`
	EntityID          string `json:"entityId"`
	TenantID          string `json:"tenantId"`
	ProcessInstanceID string `json:"processInstanceId"`
	Message           string `json:"message"`
	Blocking          bool   `json:"blocking"`
}

// TenantSummary 统计预检范围内的运行时实体。
type TenantSummary struct {
	TenantID   string `json:"tenantId"`
	Instances  int    `json:"instances"`
	Executions int    `json:"executions"`
	Tasks      int    `json:"tasks"`
	Variables  int    `json:"variables"`
}

// Report 描述来源基线、只读分析范围及阻断问题，不代表续跑验收通过。
type Report struct {
	Baseline          string          `json:"baseline"`
	SchemaVersion     string          `json:"schemaVersion"`
	Dialect           string          `json:"dialect"`
	Blocked           bool            `json:"blocked"`
	Issues            []Issue         `json:"issues"`
	Tenants           []TenantSummary `json:"tenants"`
	Tables            []TableSummary  `json:"tables"`
	Scope             string          `json:"scope"`
	RequestedTenantID *string         `json:"requestedTenantId,omitempty"`
	Limitations       []string        `json:"limitations"`
}

// 核心及 Java 独立作业表必须存在，否则无法判定来源完整性。
var requiredAnalysisTables = []string{"ACT_GE_PROPERTY", "ACT_RE_DEPLOYMENT", "ACT_RE_PROCDEF", "ACT_GE_BYTEARRAY", "ACT_RU_EXECUTION", "ACT_RU_TASK", "ACT_RU_VARIABLE", "ACT_RU_EVENT_SUBSCR", "ACT_RU_JOB", "ACT_RU_TIMER_JOB", "ACT_RU_SUSPENDED_JOB", "ACT_RU_DEADLETTER_JOB", "ACT_RU_EXTERNAL_JOB", "ACT_RU_HISTORY_JOB"}

// analysisState 保存来源索引及当前分析范围，不修改来源数据。
type analysisState struct {
	report    Report
	options   Options
	index     map[string]map[string]flowable.Row
	selected  map[string]map[string]bool
	processes map[string]*model.Process
	resources map[string][]flowable.Row
}

// Analyze 逐实体检查 Java 来源快照并给出保守的迁移阻断报告。
// @param snapshot 一致性只读来源快照
// @param options 租户范围
// @return 不含变量值、二进制及源码的风险报告
func Analyze(snapshot SourceSnapshot, options Options) Report {
	state := analysisState{options: options, index: make(map[string]map[string]flowable.Row), selected: make(map[string]map[string]bool), processes: make(map[string]*model.Process), resources: make(map[string][]flowable.Row)}
	state.report = Report{Baseline: "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e", SchemaVersion: snapshot.SchemaVersion, Dialect: snapshot.Dialect, Issues: []Issue{}, Tenants: []TenantSummary{}, Tables: append([]TableSummary{}, snapshot.Tables...), Scope: "全部租户", Limitations: []string{
		"只读预检不执行转换，不代表 Java 与 Go 续跑、反向转换或回退验收通过。",
		"实例检查限定简单用户任务等待结构；复杂作用域、作业和事件订阅必须另行转换及对照验证。",
		"表数量为全库统计；未知或未分析非空表按全库阻断，租户筛选不能证明这些表可安全迁移。",
		"尚未覆盖的历史、身份、模型及扩展实体在非空时阻断；表达式、业务委托和外部副作用仍需人工对照。",
	}}
	if options.TenantID != nil {
		state.report.Scope = "指定租户"
		tenant := *options.TenantID
		state.report.RequestedTenantID = &tenant
		if tenant == "" {
			state.report.Scope = "默认租户"
		}
	}
	// 为关联校验保留全部行，租户过滤不得提前丢弃孤儿和跨租户冲突。
	for _, row := range snapshot.Rows {
		if state.index[row.Table] == nil {
			state.index[row.Table] = make(map[string]flowable.Row)
		}
		state.index[row.Table][analysisID(row)] = row
		if row.Table == "ACT_GE_BYTEARRAY" {
			key := analysisString(row, "DEPLOYMENT_ID_") + "\x00" + analysisString(row, "NAME_")
			state.resources[key] = append(state.resources[key], row)
		}
	}
	state.checkTables(snapshot)
	state.selectRows(snapshot.Rows)
	for _, row := range snapshot.Rows {
		if row.Table == "ACT_RE_PROCDEF" && state.inScope(row) {
			state.checkDefinition(row)
		}
	}
	counts := make(map[string]*TenantSummary)
	for _, row := range snapshot.Rows {
		if !state.inScope(row) {
			continue
		}
		tenant := state.tenant(row)
		count := counts[tenant]
		if count == nil {
			count = &TenantSummary{TenantID: tenant}
			counts[tenant] = count
		}
		// 统计和校验使用同一关联范围，变量归属由任务及执行共同推导。
		switch row.Table {
		case "ACT_RU_EXECUTION":
			count.Executions++
			if analysisString(row, "ID_") == analysisString(row, "PROC_INST_ID_") {
				count.Instances++
			}
			state.checkExecution(row)
		case "ACT_RU_TASK":
			count.Tasks++
			state.checkTask(row)
		case "ACT_RU_VARIABLE":
			count.Variables++
			state.checkVariable(row)
		case "ACT_RU_EVENT_SUBSCR":
			state.add(row, "JAVA_SUBSCRIPTION", "Java 事件订阅尚无已验收的转换映射，需要逐订阅转换及续跑对照。")
		default:
			if analysisJobTable(row.Table) {
				state.add(row, "JAVA_JOB", "Java 作业处理器及锁、重试、周期语义尚无已验收转换映射。")
			}
		}
	}
	for _, count := range counts {
		if count.Instances+count.Executions+count.Tasks+count.Variables > 0 {
			state.report.Tenants = append(state.report.Tenants, *count)
		}
	}
	sort.Slice(state.report.Tenants, func(i, j int) bool { return state.report.Tenants[i].TenantID < state.report.Tenants[j].TenantID })
	sort.Slice(state.report.Tables, func(i, j int) bool { return state.report.Tables[i].Name < state.report.Tables[j].Name })
	sort.SliceStable(state.report.Issues, func(i, j int) bool {
		a, b := state.report.Issues[i], state.report.Issues[j]
		return a.Table+"\x00"+a.EntityID+"\x00"+a.Code < b.Table+"\x00"+b.EntityID+"\x00"+b.Code
	})
	return state.report
}

// checkTables 检查固定版本、必需表和当前未分析的非空表。
// @param snapshot 来源快照
// @return 无
func (s *analysisState) checkTables(snapshot SourceSnapshot) {
	if snapshot.SchemaVersion != schema.Version {
		s.add(flowable.Row{Table: "ACT_GE_PROPERTY"}, "SCHEMA_VERSION", "来源 Schema 版本缺失或与固定基线不一致。")
	}
	present := make(map[string]bool)
	analyzed := make(map[string]bool)
	for _, name := range requiredAnalysisTables {
		analyzed[name] = true
	}
	// 未知表即使无法读取实体，也必须按全库数量报告，不能被租户筛选隐藏。
	for _, table := range snapshot.Tables {
		present[table.Name] = true
		if table.Count > 0 && !analyzed[table.Name] {
			s.add(flowable.Row{Table: table.Name}, "UNSUPPORTED_TABLE", "该表包含数据，当前分析器尚未覆盖；无法确定租户归属，按全库阻断。")
		}
	}
	for _, name := range requiredAnalysisTables {
		if !present[name] {
			s.add(flowable.Row{Table: name}, "MISSING_TABLE", "来源缺少预检必需表，无法完成完整性检查。")
		}
	}
}

// selectRows 建立明确租户范围并保留无法安全归属的运行时实体。
// @param rows 全量来源行
// @return 无
func (s *analysisState) selectRows(rows []flowable.Row) {
	if s.options.TenantID == nil {
		return
	}
	// 先记录可确认的租户归属，后续仍保留关联冲突与孤儿检查。
	for _, row := range flowable.FilterRowsByTenant(rows, *s.options.TenantID) {
		if s.selected[row.Table] == nil {
			s.selected[row.Table] = make(map[string]bool)
		}
		s.selected[row.Table][analysisID(row)] = true
	}
}

// inScope 判断实体是否属于请求范围，孤儿运行时行保守纳入检查。
// @param row 来源行
// @return 是否应检查
func (s *analysisState) inScope(row flowable.Row) bool {
	if s.options.TenantID == nil || s.selected[row.Table][analysisID(row)] {
		return true
	}
	// 任一关联落在目标范围都必须检查，即使实体直接租户列冲突也不能静默丢弃。
	instance := analysisString(row, "PROC_INST_ID_")
	if instance == "" {
		instance = analysisString(row, "PROCESS_INSTANCE_ID_")
	}
	if s.selected["ACT_RU_EXECUTION"][analysisString(row, "EXECUTION_ID_")] || s.selected["ACT_RU_EXECUTION"][instance] {
		return true
	}
	// 作业表不一定在 Go 映射中，直接租户列可用于预检筛选。
	if analysisJobTable(row.Table) || row.Table == "ACT_RU_EVENT_SUBSCR" {
		return analysisString(row, "TENANT_ID_") == *s.options.TenantID
	}
	if row.Table == "ACT_RU_VARIABLE" {
		execution := s.index["ACT_RU_EXECUTION"][analysisString(row, "EXECUTION_ID_")]
		task := s.index["ACT_RU_TASK"][analysisString(row, "TASK_ID_")]
		if execution.Values == nil && task.Values == nil {
			return true
		}
		return analysisString(execution, "TENANT_ID_") == *s.options.TenantID && execution.Values != nil || analysisString(task, "TENANT_ID_") == *s.options.TenantID && task.Values != nil
	}
	return false
}

// tenant 从直接租户列或关联任务及执行推导实体租户。
// @param row 来源行
// @return 租户标识
func (s *analysisState) tenant(row flowable.Row) string {
	// 无直接租户列的实体按任务和执行回溯，不使用业务值推测归属。
	if value, exists := row.Values["TENANT_ID_"]; exists {
		return analysisText(value)
	}
	if task := s.index["ACT_RU_TASK"][analysisString(row, "TASK_ID_")]; task.Values != nil {
		return analysisString(task, "TENANT_ID_")
	}
	return analysisString(s.index["ACT_RU_EXECUTION"][analysisString(row, "EXECUTION_ID_")], "TENANT_ID_")
}

// add 添加固定说明的阻断问题，不携带来源错误或业务内容。
// @param row 来源实体
// @param code 稳定问题码
// @param message 固定中文说明
// @return 无
func (s *analysisState) add(row flowable.Row, code, message string) {
	instance := analysisString(row, "PROC_INST_ID_")
	if instance == "" {
		instance = analysisString(row, "PROCESS_INSTANCE_ID_")
	}
	s.report.Issues = append(s.report.Issues, Issue{Code: code, Table: row.Table, EntityID: analysisID(row), TenantID: s.tenant(row), ProcessInstanceID: instance, Message: message, Blocking: true})
	s.report.Blocked = true
}

// analysisID 读取来源主键，属性表使用名称主键。
// @param row 来源行
// @return 主键文本
func analysisID(row flowable.Row) string {
	if row.Table == "ACT_GE_PROPERTY" {
		return analysisString(row, "NAME_")
	}
	return analysisString(row, "ID_")
}

// analysisString 将列读取为字符串，不格式化任意业务对象。
// @param row 来源行
// @param column 列名
// @return 文本或空值
func analysisString(row flowable.Row, column string) string { return analysisText(row.Values[column]) }

// analysisText 兼容数据库驱动文本与字节类型。
// @param value 来源值
// @return 文本或空值
func analysisText(value any) string {
	// 只接受驱动文本类型，避免把任意业务对象格式化进报告。
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

// analysisJobTable 判断 Java 独立作业表。
// @param table 表名
// @return 是否作业表
func analysisJobTable(table string) bool {
	return strings.HasPrefix(table, "ACT_RU_") && strings.HasSuffix(table, "JOB")
}
