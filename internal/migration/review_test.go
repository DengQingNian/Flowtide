package migration

import (
	"math"
	"testing"
)

// TestReviewTenantCrossReferences 验证租户筛选不能隐藏其他租户指向当前实例的冲突实体。
// @param t 测试上下文
// @return 无
func TestReviewTenantCrossReferences(t *testing.T) {
	tests := []struct {
		name, table, code string
		values            map[string]any
	}{
		{"跨租户任务", "ACT_RU_TASK", "TASK_OWNER", map[string]any{"TENANT_ID_": "b", "EXECUTION_ID_": "e", "PROC_INST_ID_": "p", "PROC_DEF_ID_": "def"}},
		{"跨租户作业", "ACT_RU_TIMER_JOB", "JAVA_JOB", map[string]any{"TENANT_ID_": "b", "PROCESS_INSTANCE_ID_": "p", "EXECUTION_ID_": "e"}},
	}
	// 使用其他租户的实体引用当前租户实例，要求从所有权关联识别并阻断。
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := analysisFixture()
			if test.table == "ACT_RU_TASK" {
				source.Rows[5].Values = test.values
				source.Rows[5].Values["ID_"] = "t"
			} else {
				source.Rows = append(source.Rows, analysisRow(test.table, "j", test.values))
				for i := range source.Tables {
					if source.Tables[i].Name == test.table {
						source.Tables[i].Count++
					}
				}
			}
			tenant := "a"
			report := Analyze(source, Options{TenantID: &tenant})
			assertAnalysisIssue(t, report, test.code)
		})
	}
}

// TestReviewRuntimeActivityIntegrity 验证等待执行和任务定义不能指向 BPMN 模型中不存在的活动。
// @param t 测试上下文
// @return 无
func TestReviewRuntimeActivityIntegrity(t *testing.T) {
	source := analysisFixture()
	source.Rows[4].Values["ACT_ID_"] = "missing-node"
	source.Rows[5].Values["TASK_DEF_KEY_"] = "missing-node"
	report := Analyze(source, Options{})
	if !report.Blocked {
		t.Fatal("模型中不存在的活动不能通过预检")
	}
}

// TestReviewTenantVariableInstanceReference 验证变量的实例引用不能被另一租户的执行归属掩盖。
// @param t 测试上下文
// @return 无
func TestReviewTenantVariableInstanceReference(t *testing.T) {
	source := analysisFixture()
	// 变量指向当前实例，但执行属于其他租户且引用了不存在的其他实例。
	source.Rows = append(source.Rows, analysisRow("ACT_RU_EXECUTION", "foreign", map[string]any{"TENANT_ID_": "b", "PROC_INST_ID_": "other", "PROC_DEF_ID_": "other"}))
	source.Rows[6].Values["EXECUTION_ID_"] = "foreign"
	tenant := "a"
	assertAnalysisIssue(t, Analyze(source, Options{TenantID: &tenant}), "VARIABLE_OWNER")
}

// TestReviewResourceDefinitionKey 验证资源中的流程必须与数据库流程定义键一致。
// @param t 测试上下文
// @return 无
func TestReviewResourceDefinitionKey(t *testing.T) {
	source := analysisFixture()
	source.Rows[1].Values["KEY_"] = "other-process"
	if report := Analyze(source, Options{}); !report.Blocked {
		t.Fatal("资源中不存在数据库所指流程键，不能通过预检")
	}
}

// TestReviewDoubleFinite 验证 PostgreSQL 可表示的非有限浮点数不会在后续 JSON 持久化时失败。
// @param t 测试上下文
// @return 无
func TestReviewDoubleFinite(t *testing.T) {
	// PostgreSQL double precision 支持 NaN 和无穷，而 Go JSON 快照不支持这些值。
	for _, number := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		source := analysisFixture()
		source.Rows[6].Values["TYPE_"] = "double"
		source.Rows[6].Values["DOUBLE_"] = number
		assertAnalysisIssue(t, Analyze(source, Options{}), "VARIABLE_DATA")
	}
}

// TestReviewJavaServiceImplementations 验证内置 Java 服务和 Java Bean 方法表达式不会被解析成功掩盖。
// @param t 测试上下文
// @return 无
func TestReviewJavaServiceImplementations(t *testing.T) {
	// Go 服务执行器不支持 Java mail 类型和 Spring Bean 方法，二者都需要显式映射。
	for _, implementation := range []string{`flowable:type="mail"`, `flowable:expression="${mailer.send()}"`} {
		source := analysisFixture()
		source.Rows[2].Values["BYTES_"] = []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="process"><startEvent id="start"/><sequenceFlow id="s" sourceRef="start" targetRef="wait"/><userTask id="wait"/><sequenceFlow id="f" sourceRef="wait" targetRef="mail"/><serviceTask id="mail" ` + implementation + `/><sequenceFlow id="e" sourceRef="mail" targetRef="end"/><endEvent id="end"/></process></definitions>`)
		if report := Analyze(source, Options{}); !report.Blocked {
			t.Fatalf("Go 无法执行的 Java 实现应阻断: %s", implementation)
		}
	}
}
