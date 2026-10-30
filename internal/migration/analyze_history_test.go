package migration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// TestAnalyzeAuditHistory 验证 Java audit 核心历史和身份链接能独立通过基本关联检查。
// @param t 测试上下文
// @return 无
func TestAnalyzeAuditHistory(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "等待", true: "已结束"}[completed], func(t *testing.T) {
			source := auditAnalysisFixture(completed)
			report := Analyze(source, Options{})
			if report.Blocked {
				t.Fatalf("合法audit记录被阻断：%+v", report.Issues)
			}
			tenant := "a"
			if report := Analyze(source, Options{TenantID: &tenant}); report.Blocked {
				t.Fatalf("租户audit记录被阻断：%+v", report.Issues)
			}
		})
	}
}

// TestAnalyzeHistoricProcessRisks 验证历史流程的定义、租户、生命周期和外部作用域。
// @param t 测试上下文
// @return 无
func TestAnalyzeHistoricProcessRisks(t *testing.T) {
	tests := []struct {
		name, code string
		values     map[string]any
	}{
		{"孤儿定义", "HISTORY_DEFINITION", map[string]any{"PROC_DEF_ID_": "missing"}},
		{"跨租户定义", "HISTORY_DEFINITION", map[string]any{"TENANT_ID_": "b"}},
		{"实例ID冲突", "HISTORY_OWNER", map[string]any{"PROC_INST_ID_": "other"}},
		{"调用关系", "HISTORY_SCOPE", map[string]any{"SUPER_PROCESS_INSTANCE_ID_": "super"}},
		{"缺时间", "HISTORY_TIME", map[string]any{"START_TIME_": nil}},
		{"时间格式", "HISTORY_TIME", map[string]any{"END_TIME_": "invalid"}},
		{"时间倒序", "HISTORY_TIME", map[string]any{"END_TIME_": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := auditAnalysisFixture(false)
			changeAuditRow(&s, "ACT_HI_PROCINST", "p", test.values)
			assertAnalysisIssue(t, Analyze(s, Options{}), test.code)
		})
	}
	s := auditAnalysisFixture(true)
	changeAuditRow(&s, "ACT_HI_PROCINST", "p", map[string]any{"END_TIME_": nil})
	assertAnalysisIssue(t, Analyze(s, Options{}), "HISTORY_RUNTIME")
}

// TestAnalyzeHistoricActivitiesAndTasks 验证历史活动任务的模型、所有权与活跃关联。
// @param t 测试上下文
// @return 无
func TestAnalyzeHistoricActivitiesAndTasks(t *testing.T) {
	tests := []struct {
		name, table, id, code string
		values                map[string]any
	}{
		{"活动孤儿实例", "ACT_HI_ACTINST", "ha", "HISTORY_OWNER", map[string]any{"PROC_INST_ID_": "missing"}},
		{"活动错误定义", "ACT_HI_ACTINST", "ha", "HISTORY_DEFINITION", map[string]any{"PROC_DEF_ID_": "missing"}},
		{"活动跨租户", "ACT_HI_ACTINST", "ha", "HISTORY_OWNER", map[string]any{"TENANT_ID_": "b"}},
		{"活动节点缺失", "ACT_HI_ACTINST", "ha", "HISTORY_ACTIVITY", map[string]any{"ACT_ID_": "missing"}},
		{"活动类型冲突", "ACT_HI_ACTINST", "ha", "HISTORY_ACTIVITY", map[string]any{"ACT_TYPE_": "serviceTask"}},
		{"活动任务缺失", "ACT_HI_ACTINST", "ha", "HISTORY_OWNER", map[string]any{"TASK_ID_": "missing"}},
		{"活动调用", "ACT_HI_ACTINST", "ha", "HISTORY_SCOPE", map[string]any{"CALL_PROC_INST_ID_": "called"}},
		{"任务孤儿实例", "ACT_HI_TASKINST", "t", "HISTORY_OWNER", map[string]any{"PROC_INST_ID_": "missing"}},
		{"任务跨租户", "ACT_HI_TASKINST", "t", "HISTORY_OWNER", map[string]any{"TENANT_ID_": "b"}},
		{"任务错误节点", "ACT_HI_TASKINST", "t", "HISTORY_ACTIVITY", map[string]any{"TASK_DEF_KEY_": "start"}},
		{"任务外部作用域", "ACT_HI_TASKINST", "t", "HISTORY_SCOPE", map[string]any{"SUB_SCOPE_ID_": "cmmn"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := auditAnalysisFixture(false)
			changeAuditRow(&s, test.table, test.id, test.values)
			assertAnalysisIssue(t, Analyze(s, Options{}), test.code)
		})
	}
	s := auditAnalysisFixture(true)
	changeAuditRow(&s, "ACT_HI_TASKINST", "t", map[string]any{"END_TIME_": nil})
	assertAnalysisIssue(t, Analyze(s, Options{}), "HISTORY_RUNTIME")
	s = auditAnalysisFixture(true)
	changeAuditRow(&s, "ACT_HI_ACTINST", "ha", map[string]any{"END_TIME_": nil})
	assertAnalysisIssue(t, Analyze(s, Options{}), "HISTORY_RUNTIME")
}

// TestAnalyzeHistoricVariables 验证 VAR_TYPE_、历史归属和外置数据，不依赖已删除的运行时执行。
// @param t 测试上下文
// @return 无
func TestAnalyzeHistoricVariables(t *testing.T) {
	tests := []struct {
		name, code string
		values     map[string]any
	}{
		{"序列化", "VARIABLE_TYPE", map[string]any{"VAR_TYPE_": "serializable"}},
		{"字节缺失", "VARIABLE_DATA", map[string]any{"VAR_TYPE_": "bytes", "BYTEARRAY_ID_": "missing"}},
		{"错误json", "VARIABLE_DATA", map[string]any{"VAR_TYPE_": "json", "TEXT_": "private-json"}},
		{"孤儿实例", "HISTORY_OWNER", map[string]any{"PROC_INST_ID_": "missing"}},
		{"孤儿任务", "HISTORY_OWNER", map[string]any{"TASK_ID_": "missing"}},
		{"外部作用域", "HISTORY_SCOPE", map[string]any{"SCOPE_TYPE_": "cmmn"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := auditAnalysisFixture(true)
			changeAuditRow(&s, "ACT_HI_VARINST", "hv", test.values)
			report := Analyze(s, Options{})
			assertAnalysisIssue(t, report, test.code)
			raw, _ := json.Marshal(report)
			if strings.Contains(string(raw), "private-json") {
				t.Fatal("泄露历史变量内容")
			}
		})
	}
	s := auditAnalysisFixture(true)
	changeAuditRow(&s, "ACT_HI_VARINST", "hv", map[string]any{"VAR_TYPE_": "bytes", "BYTEARRAY_ID_": "history-bytes"})
	s.Rows = append(s.Rows, analysisRow("ACT_GE_BYTEARRAY", "history-bytes", map[string]any{"BYTES_": []byte{1, 2, 3}}))
	if r := Analyze(s, Options{}); r.Blocked {
		t.Fatalf("合法历史字节值被阻断：%+v", r.Issues)
	}
}

// TestAnalyzeIdentityLinks 验证运行时与历史身份链接的主体、类型、归属和外部作用域。
// @param t 测试上下文
// @return 无
func TestAnalyzeIdentityLinks(t *testing.T) {
	tests := []struct {
		name, code string
		values     map[string]any
	}{
		{"缺主体", "IDENTITY_SUBJECT", map[string]any{"USER_ID_": "", "GROUP_ID_": ""}},
		{"双主体", "IDENTITY_SUBJECT", map[string]any{"USER_ID_": "u", "GROUP_ID_": "g"}},
		{"未知类型", "IDENTITY_TYPE", map[string]any{"TYPE_": "custom"}},
		{"无归属", "IDENTITY_OWNER", map[string]any{"PROC_INST_ID_": "", "TASK_ID_": ""}},
		{"孤儿任务", "IDENTITY_OWNER", map[string]any{"TASK_ID_": "missing"}},
		{"孤儿实例", "IDENTITY_OWNER", map[string]any{"PROC_INST_ID_": "missing"}},
		{"外部作用域", "IDENTITY_SCOPE", map[string]any{"SCOPE_DEFINITION_ID_": "cmmn"}},
	}
	for _, table := range []string{"ACT_RU_IDENTITYLINK", "ACT_HI_IDENTITYLINK"} {
		for _, test := range tests {
			t.Run(table+test.name, func(t *testing.T) {
				s := auditAnalysisFixture(false)
				id := "rl"
				if table == "ACT_HI_IDENTITYLINK" {
					id = "hl"
				}
				changeAuditRow(&s, table, id, test.values)
				assertAnalysisIssue(t, Analyze(s, Options{}), test.code)
			})
		}
	}
	s := auditAnalysisFixture(false)
	changeAuditRow(&s, "ACT_RU_IDENTITYLINK", "rl", map[string]any{"TASK_ID_": "", "PROC_INST_ID_": "", "PROC_DEF_ID_": "def", "TYPE_": "candidate", "USER_ID_": "", "GROUP_ID_": "g"})
	if r := Analyze(s, Options{}); r.Blocked {
		t.Fatalf("定义候选组身份应受支持：%+v", r.Issues)
	}
}

// TestAnalyzeAuditTenantReferences 验证租户过滤不隐藏历史及身份实体的反向冲突与孤儿。
// @param t 测试上下文
// @return 无
func TestAnalyzeAuditTenantReferences(t *testing.T) {
	tests := []struct {
		table, id, code string
		values          map[string]any
	}{
		{"ACT_HI_TASKINST", "t", "HISTORY_OWNER", map[string]any{"TENANT_ID_": "b"}},
		{"ACT_HI_ACTINST", "ha", "HISTORY_OWNER", map[string]any{"TENANT_ID_": "b"}},
		{"ACT_HI_VARINST", "hv", "HISTORY_OWNER", map[string]any{"PROC_INST_ID_": "p", "TASK_ID_": "foreign"}},
		{"ACT_RU_IDENTITYLINK", "rl", "IDENTITY_OWNER", map[string]any{"PROC_INST_ID_": "p", "TASK_ID_": "foreign"}},
		{"ACT_HI_IDENTITYLINK", "hl", "IDENTITY_OWNER", map[string]any{"PROC_INST_ID_": "p", "TASK_ID_": "foreign"}},
	}
	for _, test := range tests {
		t.Run(test.table, func(t *testing.T) {
			s := auditAnalysisFixture(true)
			if test.table == "ACT_RU_IDENTITYLINK" {
				s = auditAnalysisFixture(false)
			}
			s.Rows = append(s.Rows, analysisRow("ACT_HI_TASKINST", "foreign", map[string]any{"TENANT_ID_": "b", "PROC_INST_ID_": "other", "END_TIME_": time.Now()}))
			changeAuditRow(&s, test.table, test.id, test.values)
			tenant := "a"
			assertAnalysisIssue(t, Analyze(s, Options{TenantID: &tenant}), test.code)
		})
	}
	s := auditAnalysisFixture(true)
	changeAuditRow(&s, "ACT_HI_IDENTITYLINK", "hl", map[string]any{"PROC_INST_ID_": "missing", "TASK_ID_": "missing"})
	tenant := "absent"
	assertAnalysisIssue(t, Analyze(s, Options{TenantID: &tenant}), "IDENTITY_OWNER")
}

// TestAnalyzeHistoricComments 验证 Java 事件评论的类型、时间与任务或流程归属。
// @param t 测试上下文
// @return 无
func TestAnalyzeHistoricComments(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	comment := func(values map[string]any) flowable.Row {
		row := analysisRow("ACT_HI_COMMENT", "c", map[string]any{"TYPE_": "event", "ACTION_": "AddUserLink", "TASK_ID_": "t", "TIME_": now})
		for key, value := range values {
			row.Values[key] = value
		}
		return row
	}
	source := auditAnalysisFixture(false)
	source.Rows = append(source.Rows, comment(nil), analysisRow("ACT_HI_COMMENT", "c-instance", map[string]any{"TYPE_": "comment", "PROC_INST_ID_": "p", "TIME_": now}))
	source.Tables = append(source.Tables, TableSummary{Name: "ACT_HI_COMMENT", Count: 2})
	if report := Analyze(source, Options{}); report.Blocked {
		t.Fatalf("合法历史评论被阻断：%+v", report.Issues)
	}
	tests := []struct {
		name, code string
		values     map[string]any
	}{
		{"未知类型", "COMMENT_TYPE", map[string]any{"TYPE_": "custom"}},
		{"孤儿任务", "COMMENT_OWNER", map[string]any{"TASK_ID_": "missing"}},
		{"无归属", "COMMENT_OWNER", map[string]any{"TASK_ID_": ""}},
		{"缺时间", "HISTORY_TIME", map[string]any{"TIME_": nil}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := auditAnalysisFixture(false)
			s.Rows = append(s.Rows, comment(test.values))
			s.Tables = append(s.Tables, TableSummary{Name: "ACT_HI_COMMENT", Count: 1})
			assertAnalysisIssue(t, Analyze(s, Options{}), test.code)
		})
	}
}

// auditAnalysisFixture 按 Java audit 列映射创建活动中或已完成的关联数据。
// @param completed 是否移除运行时行并结束历史
// @return 含基础审计数据的来源快照
func auditAnalysisFixture(completed bool) SourceSnapshot {
	s := analysisFixture()
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	var ended any
	if completed {
		ended = now.Add(time.Minute)
		rows := []flowable.Row{}
		for _, row := range s.Rows {
			if !strings.HasPrefix(row.Table, "ACT_RU_") {
				rows = append(rows, row)
			}
		}
		s.Rows = rows
	}
	history := []flowable.Row{
		analysisRow("ACT_HI_PROCINST", "p", map[string]any{"PROC_INST_ID_": "p", "PROC_DEF_ID_": "def", "TENANT_ID_": "a", "START_TIME_": now, "END_TIME_": ended}),
		analysisRow("ACT_HI_ACTINST", "ha", map[string]any{"PROC_INST_ID_": "p", "PROC_DEF_ID_": "def", "EXECUTION_ID_": "e", "TASK_ID_": "t", "ACT_ID_": "wait", "ACT_TYPE_": "userTask", "TENANT_ID_": "a", "START_TIME_": now, "END_TIME_": ended}),
		analysisRow("ACT_HI_TASKINST", "t", map[string]any{"PROC_INST_ID_": "p", "PROC_DEF_ID_": "def", "EXECUTION_ID_": "e", "TASK_DEF_KEY_": "wait", "TENANT_ID_": "a", "START_TIME_": now, "END_TIME_": ended}),
		analysisRow("ACT_HI_VARINST", "hv", map[string]any{"PROC_INST_ID_": "p", "EXECUTION_ID_": "p", "VAR_TYPE_": "string", "TEXT_": "private-value", "CREATE_TIME_": now, "LAST_UPDATED_TIME_": now}),
		analysisRow("ACT_HI_IDENTITYLINK", "hl", map[string]any{"PROC_INST_ID_": "p", "TASK_ID_": "t", "TYPE_": "candidate", "USER_ID_": "u", "CREATE_TIME_": now}),
	}
	if !completed {
		history = append(history, analysisRow("ACT_RU_IDENTITYLINK", "rl", map[string]any{"PROC_INST_ID_": "p", "TASK_ID_": "t", "TYPE_": "candidate", "USER_ID_": "u"}))
	}
	s.Rows = append(s.Rows, history...)
	counts := make(map[string]int64)
	for _, row := range s.Rows {
		counts[row.Table]++
	}
	for i := range s.Tables {
		s.Tables[i].Count = counts[s.Tables[i].Name]
		delete(counts, s.Tables[i].Name)
	}
	for name, count := range counts {
		s.Tables = append(s.Tables, TableSummary{Name: name, Count: count})
	}
	return s
}

// changeAuditRow 修改测试快照中的指定审计行。
// @param source 待修改快照
// @param table 表名
// @param id 主键
// @param values 覆盖列值
// @return 无
func changeAuditRow(source *SourceSnapshot, table, id string, values map[string]any) {
	for i := range source.Rows {
		if source.Rows[i].Table == table && analysisID(source.Rows[i]) == id {
			for k, v := range values {
				source.Rows[i].Values[k] = v
			}
			return
		}
	}
}

// TestAnalyzeRuntimeActivityCopies 验证运行时活动只在完整历史副本存在时可作为重复容器分析。
// @param t 测试上下文
// @return 无
func TestAnalyzeRuntimeActivityCopies(t *testing.T) {
	source := auditAnalysisFixture(false)
	values := map[string]any{}
	for _, row := range source.Rows {
		if row.Table == "ACT_HI_ACTINST" {
			for k, v := range row.Values {
				values[k] = v
			}
		}
	}
	values["REV_"] = int64(2)
	source.Rows = append(source.Rows, flowable.Row{Table: "ACT_RU_ACTINST", Values: values})
	source.Tables = append(source.Tables, TableSummary{Name: "ACT_RU_ACTINST", Count: 1})
	if report := Analyze(source, Options{}); report.Blocked {
		t.Fatalf("有完整历史副本的运行时活动不应阻断：%+v", report.Issues)
	}
	changeAuditRow(&source, "ACT_RU_ACTINST", "ha", map[string]any{"COMPLETED_BY_": "different"})
	assertAnalysisIssue(t, Analyze(source, Options{}), "RUNTIME_ACTIVITY_HISTORY")
	changeAuditRow(&source, "ACT_RU_ACTINST", "ha", map[string]any{"ID_": "missing"})
	assertAnalysisIssue(t, Analyze(source, Options{}), "RUNTIME_ACTIVITY_HISTORY")
}

// TestAnalyzeOpenHistoryRuntimeAgreement 验证活跃历史活动和任务必须与当前运行时活动完全对齐。
// @param t 测试上下文
// @return 无
func TestAnalyzeOpenHistoryRuntimeAgreement(t *testing.T) {
	source := auditAnalysisFixture(false)
	changeAuditRow(&source, "ACT_HI_ACTINST", "ha", map[string]any{"ACT_ID_": "start", "ACT_TYPE_": "startEvent"})
	assertAnalysisIssue(t, Analyze(source, Options{}), "HISTORY_RUNTIME")
	source = auditAnalysisFixture(false)
	changeAuditRow(&source, "ACT_HI_TASKINST", "t", map[string]any{"EXECUTION_ID_": "p"})
	assertAnalysisIssue(t, Analyze(source, Options{}), "HISTORY_RUNTIME")
}

// TestAnalyzeStandaloneAuditTask 验证独立历史任务及任务变量不强制包含流程实例。
// @param t 测试上下文
// @return 无
func TestAnalyzeStandaloneAuditTask(t *testing.T) {
	source := analysisFixture()
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	source.Rows = append(source.Rows,
		analysisRow("ACT_HI_TASKINST", "standalone", map[string]any{"TENANT_ID_": "a", "START_TIME_": now, "END_TIME_": now.Add(time.Minute)}),
		analysisRow("ACT_HI_VARINST", "standalone-v", map[string]any{"TASK_ID_": "standalone", "VAR_TYPE_": "long", "LONG_": int64(7), "CREATE_TIME_": now}),
		analysisRow("ACT_HI_IDENTITYLINK", "standalone-l", map[string]any{"TASK_ID_": "standalone", "TYPE_": "owner", "USER_ID_": "u", "CREATE_TIME_": now}),
	)
	source.Tables = append(source.Tables, TableSummary{Name: "ACT_HI_TASKINST", Count: 1}, TableSummary{Name: "ACT_HI_VARINST", Count: 1}, TableSummary{Name: "ACT_HI_IDENTITYLINK", Count: 1})
	if report := Analyze(source, Options{}); report.Blocked {
		t.Fatalf("独立历史任务合法归属被阻断：%+v", report.Issues)
	}
}
