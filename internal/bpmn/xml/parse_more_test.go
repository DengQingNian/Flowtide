package xml

import (
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_AllElementKinds 验证各类元素解析。
// @param t 测试上下文
func TestParse_AllElementKinds(t *testing.T) {
	data := `<?xml version="1.0"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:activiti="http://activiti.org/bpmn"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="kinds">
    <documentation>流程说明</documentation>
    <startEvent id="t1"><timerEventDefinition><timeDate>2026-12-01T00:00:00Z</timeDate></timerEventDefinition></startEvent>
    <startEvent id="t2"><timerEventDefinition><timeCycle>R3/PT1H</timeCycle></timerEventDefinition></startEvent>
    <serviceTask id="s1" flowable:expression="${calc}" flowable:resultVariable="r"/>
    <serviceTask id="s2" flowable:class="com.example.Handler"/>
    <scriptTask id="sc" scriptFormat="starlark"><script>1+1</script></scriptTask>
    <manualTask id="m"/>
    <endEvent id="e1"><terminateEventDefinition/></endEvent>
    <userTask id="u" activiti:assignee="bob" activiti:dueDate="2026-12-01" activiti:priority="3"/>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="s1"/>
    <sequenceFlow id="f2" sourceRef="t2" targetRef="s2"/>
    <sequenceFlow id="f3" sourceRef="s1" targetRef="sc"/>
    <sequenceFlow id="f4" sourceRef="s2" targetRef="m"/>
    <sequenceFlow id="f5" sourceRef="sc" targetRef="e1"/>
    <sequenceFlow id="f6" sourceRef="m" targetRef="u"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	process := parsed.Processes[0]
	if process.Executable {
		t.Fatalf("isExecutable 缺省应为 false")
	}
	if process.Documentation != "流程说明" {
		t.Fatalf("流程文档错误")
	}
	t1 := process.Elements["t1"].(*model.StartEvent)
	if t1.Timer == nil || t1.Timer.TimeDate == "" {
		t.Fatalf("timeDate 解析错误")
	}
	t2 := process.Elements["t2"].(*model.StartEvent)
	if t2.Timer == nil || t2.Timer.TimeCycle != "R3/PT1H" {
		t.Fatalf("timeCycle 解析错误")
	}
	s1 := process.Elements["s1"].(*model.ServiceTask)
	if s1.ImplementationType != "expression" || s1.ResultVariable != "r" {
		t.Fatalf("表达式服务任务错误: %+v", s1)
	}
	s2 := process.Elements["s2"].(*model.ServiceTask)
	if s2.ImplementationType != "class" {
		t.Fatalf("类服务任务错误")
	}
	script := process.Elements["sc"].(*model.ScriptTask)
	if script.ScriptFormat != "starlark" || script.Script != "1+1" {
		t.Fatalf("脚本任务错误: %+v", script)
	}
	if _, ok := process.Elements["m"].(*model.ManualTask); !ok {
		t.Fatalf("手动任务错误")
	}
	end := process.Elements["e1"].(*model.EndEvent)
	if !end.Terminate {
		t.Fatalf("终止结束事件错误")
	}
	user := process.Elements["u"].(*model.UserTask)
	if user.Assignee != "bob" || user.DueDate != "2026-12-01" || user.Priority != "3" {
		t.Fatalf("Activiti 扩展属性错误: %+v", user)
	}
	if len(process.StartEvents()) != 2 {
		t.Fatalf("开始事件数量错误")
	}
}

// TestParse_BusinessRuleTask 验证业务规则任务属性解析。
// @param t 测试上下文
func TestParse_BusinessRuleTask(t *testing.T) {
	data := `<?xml version="1.0"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:activiti="http://activiti.org/bpmn"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="rules" isExecutable="true">
    <startEvent id="s"/>
    <businessRuleTask id="flowableRule" name="Flowable Rule"
                      flowable:decisionRef="decisionA"
                      flowable:resultVariable="result"
                      flowable:asyncBefore="true"/>
    <businessRuleTask id="plainRule" decisionRef="decisionB" resultVariable="out"/>
    <businessRuleTask id="activitiRule" activiti:decisionRef="decisionC" activiti:resultVariable="out2"/>
    <endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析业务规则任务失败: %v", err)
	}
	process := parsed.Processes[0]
	flowableRule, ok := process.Elements["flowableRule"].(*model.BusinessRuleTask)
	if !ok {
		t.Fatalf("flowable 业务规则任务未解析: %+v", process.Elements["flowableRule"])
	}
	if flowableRule.DecisionRef != "decisionA" || flowableRule.ResultVariable != "result" {
		t.Fatalf("flowable 业务规则任务属性错误: %+v", flowableRule)
	}
	if !flowableRule.IsAsyncBefore() || flowableRule.ElementType() != "businessRuleTask" {
		t.Fatalf("业务规则任务公共属性错误: %+v", flowableRule)
	}
	plainRule := process.Elements["plainRule"].(*model.BusinessRuleTask)
	if plainRule.DecisionRef != "decisionB" || plainRule.ResultVariable != "out" {
		t.Fatalf("无前缀业务规则任务属性错误: %+v", plainRule)
	}
	activitiRule := process.Elements["activitiRule"].(*model.BusinessRuleTask)
	if activitiRule.DecisionRef != "decisionC" || activitiRule.ResultVariable != "out2" {
		t.Fatalf("activiti 业务规则任务属性错误: %+v", activitiRule)
	}
}

// TestParse_SequenceFlowMissingID 验证顺序流缺少 ID。
// @param t 测试上下文
func TestParse_SequenceFlowMissingID(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><sequenceFlow sourceRef="s" targetRef="s"/></process></definitions>`
	if _, err := Parse([]byte(data)); err == nil || !strings.Contains(err.Error(), "sequenceFlow 缺少 id") {
		t.Fatalf("期望顺序流缺少 ID 错误: %v", err)
	}
}

// TestParse_UnknownElementIgnored 验证未知元素被忽略。
// @param t 测试上下文
func TestParse_UnknownElementIgnored(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><dataObject id="d"/><endEvent id="e"/><sequenceFlow id="f" sourceRef="s" targetRef="e"/></process></definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if _, ok := parsed.Processes[0].Elements["d"]; ok {
		t.Fatalf("未知元素不应进入模型")
	}
}
