package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const taskListenerXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="listenerProcess" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task" flowable:assignee="alice">
      <extensionElements>
        <flowable:taskListener event="create" class="onCreate"/>
        <flowable:taskListener event="complete" delegateExpression="onComplete"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestTaskListeners 验证任务监听器创建与完成事件。
// @param t 测试上下文
func TestTaskListeners(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskListenerXML)
	created, completed := 0, 0
	engine.RegisterTaskListener("onCreate", func(ctx context.Context, task *api.Task) error {
		created++
		if task.Name != "Task" || task.Assignee != "alice" {
			t.Fatalf("创建监听器任务上下文错误: %+v", task)
		}
		return nil
	})
	engine.RegisterTaskListener("onComplete", func(ctx context.Context, task *api.Task) error {
		completed++
		return nil
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if created != 1 {
		t.Fatalf("创建监听器未执行: %d", created)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("任务缺失: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if completed != 1 {
		t.Fatalf("完成监听器未执行: %d", completed)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("流程应结束")
	}
}

// TestTaskListenerErrors 验证监听器缺失与失败回滚。
// @param t 测试上下文
func TestTaskListenerErrors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskListenerXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerProcess"}); err == nil {
		t.Fatalf("未注册监听器应失败")
	}
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("失败后应回滚实例: %+v", instances)
	}
	engine.RegisterTaskListener("onCreate", func(ctx context.Context, task *api.Task) error { return nil })
	engine.RegisterTaskListener("onComplete", func(ctx context.Context, task *api.Task) error {
		return errors.New("listener failed")
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("任务缺失: %+v", tasks)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err == nil {
		t.Fatalf("监听器失败应返回错误")
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("监听器失败应回滚任务删除")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("监听器失败不应结束流程")
	}
}

// TestTaskListenerExpressionUnsupported 验证表达式监听器被拒绝。
// @param t 测试上下文
func TestTaskListenerExpressionUnsupported(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="listenerExpr" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="task"/><userTask id="task" name="T"><extensionElements><flowable:taskListener event="create" expression="${notify}"/></extensionElements></userTask><sequenceFlow id="f2" sourceRef="task" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerExpr"}); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("表达式监听器应 400: %v", err)
	}
}
