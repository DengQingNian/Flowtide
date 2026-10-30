package engine

import (
	"context"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// scriptXML 构造脚本任务流程。
// @param format 脚本格式
// @param script 脚本内容
// @param result 结果变量
// @return BPMN XML
func scriptXML(format, script, result string) string {
	return `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="script" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><scriptTask id="task" scriptFormat="` + format + `" flowable:resultVariable="` + result + `"><script>` + script + `</script></scriptTask><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`
}

// TestScriptTask_Expression 验证表达式脚本。
// @param t 测试上下文
func TestScriptTask_Expression(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, scriptXML("expression", "1 + 2", "sum"))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "script"})
	if err != nil || !instance.Ended {
		t.Fatalf("脚本执行失败: %v %+v", err, instance)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["sum"] != 3.0 {
		t.Fatalf("脚本结果错误: %+v", variables)
	}
}

// TestScriptTask_Return 验证 return 与变量。
// @param t 测试上下文
func TestScriptTask_Return(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, scriptXML("juel", "return x * 2;", "result"))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "script",
		Variables:            []api.Variable{{Name: "x", Value: 4}},
	})
	if err != nil {
		t.Fatalf("脚本执行失败: %v", err)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["result"] != 8.0 {
		t.Fatalf("脚本结果错误: %+v", variables)
	}
}

// TestScriptTask_Errors 验证脚本错误。
// @param t 测试上下文
func TestScriptTask_Errors(t *testing.T) {
	cases := []struct {
		format string
		script string
	}{
		{"groovy", "1 + 1"},
		{"unknown", "1 + 1"},
		{"expression", ""},
		{"expression", "1 +"},
	}
	for _, tc := range cases {
		engine := newTestEngine(t)
		deployXML(t, engine, scriptXML(tc.format, tc.script, "result"))
		if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "script"}); err == nil {
			t.Fatalf("格式 %s 脚本 %q 应失败", tc.format, tc.script)
		}
	}
}
