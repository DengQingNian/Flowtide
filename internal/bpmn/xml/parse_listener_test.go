package xml

import (
	"testing"

	"github.com/flowable-go/flowable/bpmn/model"
)

// listenerParseXML 覆盖流程、活动与子流程上的执行监听器解析。
const listenerParseXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn" xmlns:activiti="http://activiti.org/bpmn">
  <process id="listenerParse" isExecutable="true">
    <extensionElements>
      <flowable:executionListener event="start" class="processStart"/>
      <activiti:executionListener event="end" delegateExpression="processEnd"/>
    </extensionElements>
    <startEvent id="s">
      <extensionElements>
        <executionListener class="startDefault"/>
      </extensionElements>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="T">
      <extensionElements>
        <flowable:executionListener event="END" expression="${notify}"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="sub"/>
    <subProcess id="sub">
      <extensionElements>
        <flowable:executionListener event="start" class="subStart"/>
      </extensionElements>
      <startEvent id="ss"/>
      <sequenceFlow id="sf" sourceRef="ss" targetRef="se"/>
      <endEvent id="se"/>
    </subProcess>
    <sequenceFlow id="f3" sourceRef="sub" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestParseExecutionListeners 验证流程、活动与子流程执行监听器的解析。
// @param t 测试上下文
func TestParseExecutionListeners(t *testing.T) {
	parsed, err := Parse([]byte(listenerParseXML))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(parsed.Processes) != 1 {
		t.Fatalf("流程数量错误: %d", len(parsed.Processes))
	}
	process := parsed.Processes[0]
	if len(process.ExecutionListeners) != 2 {
		t.Fatalf("流程级监听器数量错误: %+v", process.ExecutionListeners)
	}
	if process.ExecutionListeners[0].Event != "start" || process.ExecutionListeners[0].ClassName != "processStart" {
		t.Fatalf("流程开始监听器解析错误: %+v", process.ExecutionListeners[0])
	}
	if process.ExecutionListeners[1].Event != "end" || process.ExecutionListeners[1].DelegateExpression != "processEnd" {
		t.Fatalf("流程结束监听器解析错误: %+v", process.ExecutionListeners[1])
	}
	start, ok := process.Elements["s"].(*model.StartEvent)
	if !ok {
		t.Fatalf("开始事件缺失")
	}
	if len(start.GetExecutionListeners()) != 1 || start.GetExecutionListeners()[0].Event != "start" {
		t.Fatalf("缺省事件应解析为 start: %+v", start.GetExecutionListeners())
	}
	task, ok := process.Elements["task"].(*model.UserTask)
	if !ok {
		t.Fatalf("用户任务缺失")
	}
	if len(task.GetExecutionListeners()) != 1 {
		t.Fatalf("用户任务监听器数量错误: %+v", task.GetExecutionListeners())
	}
	if task.GetExecutionListeners()[0].Event != "end" || task.GetExecutionListeners()[0].Expression != "${notify}" {
		t.Fatalf("用户任务监听器解析错误: %+v", task.GetExecutionListeners()[0])
	}
	sub, ok := process.Elements["sub"].(*model.SubProcess)
	if !ok {
		t.Fatalf("子流程缺失")
	}
	if len(sub.GetExecutionListeners()) != 1 || sub.GetExecutionListeners()[0].ClassName != "subStart" {
		t.Fatalf("子流程监听器解析错误: %+v", sub.GetExecutionListeners())
	}
}

// TestParseExecutionListenersAbsent 验证无扩展元素时不产生监听器。
// @param t 测试上下文
func TestParseExecutionListenersAbsent(t *testing.T) {
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	process := parsed.Processes[0]
	if process.ExecutionListeners != nil {
		t.Fatalf("流程不应有监听器: %+v", process.ExecutionListeners)
	}
	node, ok := process.GetFlowNode("s")
	if !ok {
		t.Fatalf("开始事件缺失")
	}
	provider, ok := node.(interface {
		GetExecutionListeners() []model.ExecutionListenerDefinition
	})
	if !ok {
		t.Fatalf("节点未暴露执行监听器")
	}
	if listeners := provider.GetExecutionListeners(); len(listeners) != 0 {
		t.Fatalf("节点不应有监听器: %+v", listeners)
	}
}
