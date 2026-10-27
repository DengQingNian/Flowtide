package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const formXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="formProcess" isExecutable="true">
    <startEvent id="start" flowable:formKey="startForm">
      <extensionElements>
        <flowable:formProperty id="initiator" name="发起人" type="string" required="true"/>
      </extensionElements>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Approve" flowable:formKey="taskForm" flowable:assignee="${initiator}">
      <extensionElements>
        <flowable:formProperty id="approved" name="通过" type="boolean" required="true"/>
        <flowable:formProperty id="amount" name="金额" type="long"/>
        <flowable:formProperty id="role" name="角色" type="string" readable="true" writable="false"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestForm_StartAndTask 验证启动表单与任务表单。
// @param t 测试上下文
func TestForm_StartAndTask(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, formXML)
	definition, _ := engine.GetProcessDefinitionByKey(context.Background(), "formProcess")
	startForm, err := engine.GetStartFormData(context.Background(), definition.ID)
	if err != nil || startForm.FormKey != "startForm" || len(startForm.Properties) != 1 {
		t.Fatalf("启动表单错误: %v %+v", err, startForm)
	}
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "formProcess",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	form, err := engine.GetTaskFormData(context.Background(), task.ID)
	if err != nil || form.FormKey != "taskForm" || len(form.Properties) != 3 {
		t.Fatalf("任务表单错误: %v %+v", err, form)
	}
	if form.Properties[0].ID != "approved" || !form.Properties[0].Required {
		t.Fatalf("字段属性错误: %+v", form.Properties[0])
	}
	if err := engine.SubmitTaskFormData(context.Background(), task.ID, map[string]any{}); err == nil {
		t.Fatalf("缺少必填字段应失败")
	}
	if err := engine.SubmitTaskFormData(context.Background(), task.ID, map[string]any{"approved": true, "amount": "10"}); err != nil {
		t.Fatalf("提交表单失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("提交表单后应结束")
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["approved"] != true || variables["amount"] != int64(10) {
		t.Fatalf("表单变量错误: %+v", variables)
	}
}

// TestForm_Errors 验证表单错误。
// @param t 测试上下文
func TestForm_Errors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, formXML)
	if _, err := engine.GetStartFormData(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失定义表单应报错")
	}
	if _, err := engine.GetTaskFormData(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失任务表单应报错")
	}
	if err := engine.SubmitTaskFormData(context.Background(), "missing", nil); err == nil {
		t.Fatalf("缺失任务提交应报错")
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "formProcess",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.SubmitTaskFormData(context.Background(), task.ID, map[string]any{"approved": true, "role": "x"}); err == nil {
		t.Fatalf("只读字段应拒绝提交")
	}
	if err := engine.SubmitTaskFormData(context.Background(), task.ID, map[string]any{"approved": "bad"}); err == nil {
		t.Fatalf("类型错误应失败")
	}
}
