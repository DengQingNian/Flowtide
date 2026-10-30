package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// mustUserTask 取指定 ID 的用户任务元素。
// @param t 测试对象
// @param process 流程
// @param id 元素 ID
// @return 用户任务
func mustUserTask(t *testing.T, process *model.Process, id string) *model.UserTask {
	t.Helper()
	task, ok := process.Elements[id].(*model.UserTask)
	if !ok {
		t.Fatalf("元素 %s 不是 userTask", id)
	}
	return task
}

// mustSequenceFlow 取指定 ID 的顺序流元素。
// @param t 测试对象
// @param process 流程
// @param id 元素 ID
// @return 顺序流
func mustSequenceFlow(t *testing.T, process *model.Process, id string) *model.SequenceFlow {
	t.Helper()
	flow, ok := process.Elements[id].(*model.SequenceFlow)
	if !ok {
		t.Fatalf("元素 %s 不是 sequenceFlow", id)
	}
	return flow
}

// extensionFixture 是含业务扩展属性与扩展字段的 BPMN 语料。
const extensionFixture = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="t">
  <process id="extProc" isExecutable="true" flowable:direction="nowrap">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task1" flowable:direction="hidden"/>
    <userTask id="task1" name="审批" flowable:assignee="${initiator}"
              stepCode="S01" weightType="custom" weightThreshold="60"
              executorType="person" executorList='[{"identityId":"u1","agreeWeight":1,"rejectWeight":-1}]'>
      <extensionElements>
        <flowable:taskListener event="create" class="businessCallTask"/>
        <flowable:field name="biz"><flowable:string>张三</flowable:string></flowable:field>
        <flowable:field name="age"><flowable:string>1</flowable:string></flowable:field>
      </extensionElements>
      <multiInstanceLoopCharacteristics isSequential="false" flowable:collection="${executorList}">
        <completionCondition>${nrOfCompletedInstances/nrOfInstances &gt;= 1}</completionCondition>
      </multiInstanceLoopCharacteristics>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestParseExtensionsAttributes 验证扩展属性捕获：未知属性进 Attributes，已知属性不重复进。
// @param t 测试对象
func TestParseExtensionsAttributes(t *testing.T) {
	bpmn, err := Parse([]byte(extensionFixture))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	process := bpmn.Processes[0]
	// 流程级扩展属性 flowable:direction 应被捕获
	if got := process.Extensions.Attr("direction"); got != "nowrap" {
		t.Fatalf("流程级 direction 扩展属性错误: %q", got)
	}
	userTask := mustUserTask(t, process, "task1")
	// 已知属性 assignee 落模型字段，不进扩展
	if userTask.Assignee != "${initiator}" {
		t.Fatalf("assignee 解析错误: %q", userTask.Assignee)
	}
	for _, key := range []string{"stepCode", "weightType", "weightThreshold", "executorType"} {
		if userTask.Extensions.Attr(key) == "" {
			t.Fatalf("扩展属性 %s 未捕获", key)
		}
	}
	if got := userTask.Extensions.Attr("stepCode"); got != "S01" {
		t.Fatalf("stepCode 值错误: %q", got)
	}
	// assignee 不应再出现在扩展里
	if _, exists := userTask.Extensions.Attributes["assignee"]; exists {
		t.Fatalf("已知属性 assignee 不应进入扩展")
	}
	// 连线扩展
	flow := mustSequenceFlow(t, process, "f1")
	if got := flow.Extensions.Attr("direction"); got != "hidden" {
		t.Fatalf("连线 direction 扩展属性错误: %q", got)
	}
}

// TestParseExtensionsFields 验证扩展字段捕获。
// @param t 测试对象
func TestParseExtensionsFields(t *testing.T) {
	bpmn, err := Parse([]byte(extensionFixture))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	userTask := mustUserTask(t, bpmn.Processes[0], "task1")
	if got := userTask.Extensions.Field("biz"); got != "张三" {
		t.Fatalf("扩展字段 biz 值错误: %q", got)
	}
	if got := userTask.Extensions.Field("age"); got != "1" {
		t.Fatalf("扩展字段 age 值错误: %q", got)
	}
	// 无扩展元素时不产生空对象
	end := bpmn.Processes[0].Elements["end"]
	if base, ok := end.(interface{ GetID() string }); ok && base.GetID() != "end" {
		t.Fatalf("end 元素缺失")
	}
}

// TestParseMultiInstanceKept 验证扩展属性不影响多实例标准解析。
// @param t 测试对象
func TestParseMultiInstanceKept(t *testing.T) {
	bpmn, err := Parse([]byte(extensionFixture))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	userTask := mustUserTask(t, bpmn.Processes[0], "task1")
	if userTask.LoopCharacteristics == nil {
		t.Fatalf("多实例配置丢失")
	}
	if userTask.LoopCharacteristics.Collection != "${executorList}" {
		t.Fatalf("集合表达式错误: %q", userTask.LoopCharacteristics.Collection)
	}
	if userTask.LoopCharacteristics.Sequential {
		t.Fatalf("应为并行多实例")
	}
	if userTask.LoopCharacteristics.CompletionCondition == "" {
		t.Fatalf("完成条件丢失")
	}
}
