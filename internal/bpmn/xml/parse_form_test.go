package xml

import (
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestParse_FormProperties 验证表单字段解析。
// @param t 测试上下文
func TestParse_FormProperties(t *testing.T) {
	data := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="p"><startEvent id="s">
      <extensionElements><flowable:formProperty id="initiator" name="发起人" required="true"/></extensionElements>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task">
      <extensionElements>
        <flowable:formProperty id="role" name="角色" type="enum" readable="false" writable="false">
          <flowable:value id="admin" name="管理员"/><flowable:value id="user" name="用户"/>
        </flowable:formProperty>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/>
  </process>
</definitions>`
	parsed, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	start := parsed.Processes[0].Elements["s"].(*model.StartEvent)
	if len(start.FormProperties) != 1 || !start.FormProperties[0].Required || start.FormProperties[0].Type != "string" {
		t.Fatalf("启动表单解析错误: %+v", start.FormProperties)
	}
	task := parsed.Processes[0].Elements["task"].(*model.UserTask)
	if len(task.FormProperties) != 1 || task.FormProperties[0].Type != "enum" {
		t.Fatalf("任务表单解析错误: %+v", task.FormProperties)
	}
	if task.FormProperties[0].Readable || task.FormProperties[0].Writable || len(task.FormProperties[0].EnumValues) != 2 {
		t.Fatalf("表单属性错误: %+v", task.FormProperties[0])
	}
}
