package migration

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/persistence/flowable"
)

// TestAnalyzeSimpleSource 验证简单等待实例的统计、基线和只读性。
// @param t 测试上下文
// @return 无
func TestAnalyzeSimpleSource(t *testing.T) {
	source := analysisFixture()
	before, _ := json.Marshal(source.Rows)
	report := Analyze(source, Options{})
	after, _ := json.Marshal(source.Rows)
	if report.Blocked || len(report.Issues) != 0 {
		t.Fatalf("简单来源不应阻断：%+v", report.Issues)
	}
	if string(before) != string(after) {
		t.Fatal("分析修改了来源")
	}
	want := []TenantSummary{{TenantID: "a", Instances: 1, Executions: 2, Tasks: 1, Variables: 1}}
	if !reflect.DeepEqual(report.Tenants, want) || report.Baseline == "" || len(report.Limitations) == 0 {
		t.Fatalf("统计或边界缺失：%+v", report)
	}
}

// TestAnalyzeRejectsSourceRisks 验证源库版本、未知表和未分析数据的阻断。
// @param t 测试上下文
// @return 无
func TestAnalyzeRejectsSourceRisks(t *testing.T) {
	tests := []struct {
		name   string
		change func(*SourceSnapshot)
		code   string
	}{
		{"版本", func(s *SourceSnapshot) { s.SchemaVersion = "7.0" }, "SCHEMA_VERSION"},
		{"缺表", func(s *SourceSnapshot) { s.Tables = s.Tables[1:] }, "MISSING_TABLE"},
		{"未知表", func(s *SourceSnapshot) { s.Tables = append(s.Tables, TableSummary{Name: "ACT_CUSTOM", Count: 1}) }, "UNSUPPORTED_TABLE"},
		{"空未知表", func(s *SourceSnapshot) { s.Tables = append(s.Tables, TableSummary{Name: "ACT_CUSTOM", Count: 0}) }, ""},
		{"未分析历史", func(s *SourceSnapshot) {
			s.Rows = append(s.Rows, analysisRow("ACT_HI_DETAIL", "h", map[string]any{"TENANT_ID_": "a"}))
			s.Tables = append(s.Tables, TableSummary{Name: "ACT_HI_DETAIL", Count: 1})
		}, "UNSUPPORTED_TABLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := analysisFixture()
			test.change(&s)
			r := Analyze(s, Options{})
			if test.code == "" {
				if r.Blocked {
					t.Fatalf("空表不阻断：%+v", r.Issues)
				}
				return
			}
			assertAnalysisIssue(t, r, test.code)
		})
	}
}

// TestAnalyzeVariables 验证逐变量类型、字节引用和归属预检且不泄露内容。
// @param t 测试上下文
// @return 无
func TestAnalyzeVariables(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		code   string
	}{
		{"序列化", map[string]any{"TYPE_": "serializable", "TEXT_": "secret-payload"}, "VARIABLE_TYPE"},
		{"缺失字节", map[string]any{"TYPE_": "bytes", "BYTEARRAY_ID_": "missing"}, "VARIABLE_DATA"},
		{"字节格式", map[string]any{"TYPE_": "longString", "BYTEARRAY_ID_": "missing"}, "VARIABLE_DATA"},
		{"溢出整数", map[string]any{"TYPE_": "long", "LONG_": "9223372036854775808"}, "VARIABLE_DATA"},
		{"无归属", map[string]any{"EXECUTION_ID_": "", "PROC_INST_ID_": ""}, "VARIABLE_OWNER"},
		{"错误实例", map[string]any{"PROC_INST_ID_": "other"}, "VARIABLE_OWNER"},
		{"错误任务", map[string]any{"TASK_ID_": "missing"}, "VARIABLE_OWNER"},
		{"作用域", map[string]any{"SCOPE_TYPE_": "cmmn"}, "VARIABLE_SCOPE"},
		{"错误浮点", map[string]any{"TYPE_": "double", "DOUBLE_": "invalid"}, "VARIABLE_DATA"},
		{"错误json", map[string]any{"TYPE_": "json", "TEXT_": "secret-payload"}, "VARIABLE_DATA"},
		{"json尾部", map[string]any{"TYPE_": "json", "TEXT_": "{} trailing"}, "VARIABLE_DATA"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := analysisFixture()
			v := &s.Rows[len(s.Rows)-1]
			for k, value := range test.values {
				v.Values[k] = value
			}
			r := Analyze(s, Options{})
			assertAnalysisIssue(t, r, test.code)
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "secret-payload") {
				t.Fatal("泄露变量数据")
			}
		})
	}
}

// TestAnalyzeRuntime 验证执行树、任务和 Java 作业的逐实体阻断。
// @param t 测试上下文
// @return 无
func TestAnalyzeRuntime(t *testing.T) {
	tests := []struct {
		name, table, id, code string
		values                map[string]any
	}{
		{"父缺失", "ACT_RU_EXECUTION", "e", "EXECUTION_PARENT", map[string]any{"PARENT_ID_": "missing"}},
		{"多实例", "ACT_RU_EXECUTION", "e", "EXECUTION_COMPLEX", map[string]any{"IS_MI_ROOT_": int64(1)}},
		{"并发", "ACT_RU_EXECUTION", "e", "EXECUTION_COMPLEX", map[string]any{"IS_CONCURRENT_": true}},
		{"调用", "ACT_RU_EXECUTION", "p", "EXECUTION_COMPLEX", map[string]any{"SUPER_EXEC_": "super"}},
		{"环", "ACT_RU_EXECUTION", "e", "EXECUTION_PARENT", map[string]any{"PARENT_ID_": "e"}},
		{"根缺失", "ACT_RU_EXECUTION", "e", "EXECUTION_ROOT", map[string]any{"PROC_INST_ID_": "missing"}},
		{"定义缺失", "ACT_RU_EXECUTION", "e", "EXECUTION_DEFINITION", map[string]any{"PROC_DEF_ID_": "missing"}},
		{"任务执行缺失", "ACT_RU_TASK", "t", "TASK_OWNER", map[string]any{"EXECUTION_ID_": "missing"}},
		{"任务跨租户", "ACT_RU_TASK", "t", "TASK_OWNER", map[string]any{"TENANT_ID_": "b"}},
		{"任务作用域", "ACT_RU_TASK", "t", "TASK_SCOPE", map[string]any{"SCOPE_TYPE_": "cmmn"}},
		{"作业", "ACT_RU_TIMER_JOB", "j", "JAVA_JOB", map[string]any{"TENANT_ID_": "a", "PROCESS_INSTANCE_ID_": "p"}},
		{"订阅", "ACT_RU_EVENT_SUBSCR", "s", "JAVA_SUBSCRIPTION", map[string]any{"TENANT_ID_": "a", "PROC_INST_ID_": "p"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := analysisFixture()
			found := false
			for i := range s.Rows {
				if s.Rows[i].Table == test.table && s.Rows[i].Values["ID_"] == test.id {
					for k, v := range test.values {
						s.Rows[i].Values[k] = v
					}
					found = true
				}
			}
			if !found {
				s.Rows = append(s.Rows, analysisRow(test.table, test.id, test.values))
				s.Tables = append(s.Tables, TableSummary{Name: test.table, Count: 1})
			}
			assertAnalysisIssue(t, Analyze(s, Options{}), test.code)
		})
	}
}

// TestAnalyzeResources 验证部署资源完整性和 Java 扩展映射检查。
// @param t 测试上下文
// @return 无
func TestAnalyzeResources(t *testing.T) {
	tests := []struct{ name, xml, code string }{
		{"Java类", `<serviceTask id="x" flowable:class="com.example.Secret"/>`, "BPMN_MAPPING"},
		{"委托", `<serviceTask id="x" flowable:delegateExpression="${bean}"/>`, "BPMN_MAPPING"},
		{"任务监听器", `<userTask id="x"><extensionElements><flowable:taskListener event="complete" expression="${bean}"/></extensionElements></userTask>`, "BPMN_MAPPING"},
		{"执行监听器", `<extensionElements><flowable:executionListener event="start" class="Foo"/></extensionElements>`, "BPMN_MAPPING"},
		{"脚本", `<scriptTask id="x" scriptFormat="groovy"><script>secret-payload</script></scriptTask>`, "BPMN_SCRIPT"},
		{"多实例模型", `<userTask id="x"><multiInstanceLoopCharacteristics/></userTask>`, "BPMN_COMPLEX"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := analysisFixture()
			s.Rows[2].Values["BYTES_"] = []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="process">` + test.xml + `</process></definitions>`)
			r := Analyze(s, Options{})
			assertAnalysisIssue(t, r, test.code)
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "secret-payload") || strings.Contains(string(data), "com.example.Secret") {
				t.Fatal("报告包含源码")
			}
		})
	}
	s := analysisFixture()
	s.Rows[2].Values["BYTES_"] = []byte("broken")
	assertAnalysisIssue(t, Analyze(s, Options{}), "BPMN_INVALID")
	s = analysisFixture()
	s.Rows[2].Values["NAME_"] = "missing"
	assertAnalysisIssue(t, Analyze(s, Options{}), "DEFINITION_RESOURCE")
	s = analysisFixture()
	s.Rows[1].Values["DEPLOYMENT_ID_"] = "missing"
	assertAnalysisIssue(t, Analyze(s, Options{}), "DEFINITION_DEPLOYMENT")
}

// TestAnalyzeTenantScope 验证指定租户和默认租户严格过滤及全局风险不被隐藏。
// @param t 测试上下文
// @return 无
func TestAnalyzeTenantScope(t *testing.T) {
	s := analysisFixture()
	tenant := "a"
	r := Analyze(s, Options{TenantID: &tenant})
	if r.Blocked || len(r.Tenants) != 1 || r.Scope == "" {
		t.Fatalf("指定租户异常：%+v", r)
	}
	tenant = ""
	r = Analyze(s, Options{TenantID: &tenant})
	if len(r.Tenants) != 0 || r.Blocked {
		t.Fatalf("默认租户不应包含 a：%+v", r)
	}
	s.Tables = append(s.Tables, TableSummary{Name: "ACT_CUSTOM", Count: 1})
	assertAnalysisIssue(t, Analyze(s, Options{TenantID: &tenant}), "UNSUPPORTED_TABLE")
}

// analysisFixture 创建简单 Java 用户任务等待实例的预检样本。
// @return 来源快照
func analysisFixture() SourceSnapshot {
	rows := []flowable.Row{
		analysisRow("ACT_RE_DEPLOYMENT", "d", map[string]any{"TENANT_ID_": "a"}),
		analysisRow("ACT_RE_PROCDEF", "def", map[string]any{"TENANT_ID_": "a", "DEPLOYMENT_ID_": "d", "RESOURCE_NAME_": "p.bpmn20.xml", "KEY_": "process"}),
		analysisRow("ACT_GE_BYTEARRAY", "b", map[string]any{"DEPLOYMENT_ID_": "d", "NAME_": "p.bpmn20.xml", "BYTES_": []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="process"><startEvent id="start"/><sequenceFlow id="f" sourceRef="start" targetRef="wait"/><userTask id="wait"/></process></definitions>`)}),
		analysisRow("ACT_RU_EXECUTION", "p", map[string]any{"TENANT_ID_": "a", "PROC_INST_ID_": "p", "PROC_DEF_ID_": "def"}),
		analysisRow("ACT_RU_EXECUTION", "e", map[string]any{"TENANT_ID_": "a", "PROC_INST_ID_": "p", "PROC_DEF_ID_": "def", "PARENT_ID_": "p", "ACT_ID_": "wait"}),
		analysisRow("ACT_RU_TASK", "t", map[string]any{"TENANT_ID_": "a", "PROC_INST_ID_": "p", "PROC_DEF_ID_": "def", "EXECUTION_ID_": "e", "TASK_DEF_KEY_": "wait"}),
		analysisRow("ACT_RU_VARIABLE", "v", map[string]any{"PROC_INST_ID_": "p", "EXECUTION_ID_": "p", "TYPE_": "string", "TEXT_": "secret-payload"}),
	}
	tables := []TableSummary{}
	for _, name := range []string{"ACT_GE_PROPERTY", "ACT_RE_DEPLOYMENT", "ACT_RE_PROCDEF", "ACT_GE_BYTEARRAY", "ACT_RU_EXECUTION", "ACT_RU_TASK", "ACT_RU_VARIABLE", "ACT_RU_EVENT_SUBSCR", "ACT_RU_JOB", "ACT_RU_TIMER_JOB", "ACT_RU_SUSPENDED_JOB", "ACT_RU_DEADLETTER_JOB", "ACT_RU_EXTERNAL_JOB", "ACT_RU_HISTORY_JOB"} {
		count := int64(0)
		for _, row := range rows {
			if row.Table == name {
				count++
			}
		}
		tables = append(tables, TableSummary{Name: name, Count: count})
	}
	return SourceSnapshot{Dialect: "mysql", SchemaVersion: "8.1.0.3", Tables: tables, Rows: rows}
}

// analysisRow 创建允许缺少空列的预检行。
// @param table 表名
// @param id 主键
// @param values 列值
// @return 数据行
func analysisRow(table, id string, values map[string]any) flowable.Row {
	values["ID_"] = id
	return flowable.Row{Table: table, Values: values}
}

// assertAnalysisIssue 断言报告含指定阻断问题。
// @param t 测试上下文
// @param report 预检报告
// @param code 问题码
// @return 无
func assertAnalysisIssue(t *testing.T, report Report, code string) {
	t.Helper()
	for _, issue := range report.Issues {
		if issue.Code == code && issue.Blocking && report.Blocked {
			return
		}
	}
	t.Fatalf("缺少问题 %s：%+v", code, report.Issues)
}

// TestAnalyzeModelReferences 验证定义、执行和任务引用匹配且拒绝错误模型节点。
// @param t 测试上下文
// @return 无
func TestAnalyzeModelReferences(t *testing.T) {
	tests := []struct {
		name, table, id, code string
		values                map[string]any
	}{
		{"定义Key缺失", "ACT_RE_PROCDEF", "def", "DEFINITION_PROCESS", map[string]any{"KEY_": "missing"}},
		{"执行定义不同", "ACT_RU_EXECUTION", "e", "EXECUTION_DEFINITION", map[string]any{"PROC_DEF_ID_": "def2"}},
		{"活动缺失", "ACT_RU_EXECUTION", "e", "EXECUTION_ACTIVITY", map[string]any{"ACT_ID_": "missing"}},
		{"任务key不同", "ACT_RU_TASK", "t", "TASK_ACTIVITY", map[string]any{"TASK_DEF_KEY_": "other"}},
		{"任务指向开始事件", "ACT_RU_TASK", "t", "TASK_ACTIVITY", map[string]any{"TASK_DEF_KEY_": "start"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := analysisFixture()
			for i := range s.Rows {
				if s.Rows[i].Table == test.table && analysisID(s.Rows[i]) == test.id {
					for k, v := range test.values {
						s.Rows[i].Values[k] = v
					}
				}
			}
			if test.name == "执行定义不同" {
				s.Rows = append(s.Rows, analysisRow("ACT_RE_PROCDEF", "def2", map[string]any{"TENANT_ID_": "a", "DEPLOYMENT_ID_": "d", "RESOURCE_NAME_": "p.bpmn20.xml", "KEY_": "process"}))
			}
			assertAnalysisIssue(t, Analyze(s, Options{}), test.code)
		})
	}
}

// TestAnalyzeNumericColumns 验证有效类型和非有限浮点拒绝。
// @param t 测试上下文
// @return 无
func TestAnalyzeNumericColumns(t *testing.T) {
	for _, value := range []any{int(7), int64(7), "7", []byte("7"), true} {
		number, ok := analysisInteger(value)
		if !ok || number == nil {
			t.Fatalf("整数未接受：%T", value)
		}
	}
	for _, value := range []any{float64(7), int64(7), "7.5", []byte("7.5")} {
		number, ok := analysisNumber(value)
		if !ok || number == nil {
			t.Fatalf("浮点未接受：%T", value)
		}
	}
	for _, value := range []any{"NaN", "+Inf", "-Inf", true, struct{}{}} {
		s := analysisFixture()
		row := &s.Rows[len(s.Rows)-1]
		row.Values["TYPE_"] = "double"
		row.Values["DOUBLE_"] = value
		assertAnalysisIssue(t, Analyze(s, Options{}), "VARIABLE_DATA")
	}
	if number, ok := analysisInteger(float64(7)); ok || number != nil {
		t.Fatal("整数列不应接受浮点")
	}
}

// TestAnalyzeRequestedTenant 验证空范围也保留租户选择并复制值避免外部修改报告。
// @param t 测试上下文
// @return 无
func TestAnalyzeRequestedTenant(t *testing.T) {
	tenant := "absent"
	report := Analyze(analysisFixture(), Options{TenantID: &tenant})
	tenant = "changed"
	if report.RequestedTenantID == nil || *report.RequestedTenantID != "absent" {
		t.Fatalf("租户范围丢失：%+v", report)
	}
	tenant = ""
	report = Analyze(analysisFixture(), Options{TenantID: &tenant})
	if report.Scope != "默认租户" {
		t.Fatalf("默认租户范围不明确：%s", report.Scope)
	}
}
