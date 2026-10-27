package engine

import (
	"context"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// asyncServiceXML 构造异步服务任务流程。
// @param id 流程 ID
// @param attr 异步属性
// @return BPMN XML
func asyncServiceXML(id, attr string) string {
	return `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="` + id + `" isExecutable="true"><startEvent id="s"/><sequenceFlow id="f1" sourceRef="s" targetRef="svc"/><serviceTask id="svc" flowable:delegateExpression="${noop}" ` + attr + `/><sequenceFlow id="f2" sourceRef="svc" targetRef="e"/><endEvent id="e"/></process></definitions>`
}

// TestAsyncBefore_ServiceTask 验证异步进入服务任务。
// @param t 测试上下文
func TestAsyncBefore_ServiceTask(t *testing.T) {
	engine := newTestEngine(t)
	called := false
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error {
		called = true
		execution.SetVariable("done", true)
		return nil
	})
	deployXML(t, engine, asyncServiceXML("asyncBefore", `flowable:asyncBefore="true"`))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncBefore"})
	if err != nil || instance.Ended {
		t.Fatalf("异步前不应立即完成: %v %+v", err, instance)
	}
	if called {
		t.Fatalf("异步前不应立即执行委托")
	}
	executed, err := engine.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("异步作业执行失败: %v %d", err, executed)
	}
	if !called {
		t.Fatalf("异步作业应执行委托")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("异步执行后应结束")
	}
}

// TestAsyncAfter_ServiceTask 验证异步离开服务任务。
// @param t 测试上下文
func TestAsyncAfter_ServiceTask(t *testing.T) {
	engine := newTestEngine(t)
	called := false
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error {
		called = true
		return nil
	})
	deployXML(t, engine, asyncServiceXML("asyncAfter", `flowable:asyncAfter="true"`))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncAfter"})
	if err != nil || instance.Ended {
		t.Fatalf("异步后不应立即结束: %v %+v", err, instance)
	}
	if !called {
		t.Fatalf("异步前委托应执行")
	}
	executed, err := engine.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("异步后作业执行失败: %v %d", err, executed)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("异步后应结束")
	}
}

const asyncUserXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="asyncUser" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task" flowable:asyncBefore="true"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestAsyncBefore_UserTask 验证异步进入用户任务。
// @param t 测试上下文
func TestAsyncBefore_UserTask(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, asyncUserXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncUser"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("异步前不应创建任务")
	}
	executed, err := engine.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("异步作业执行失败: %v %d", err, executed)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("异步后应创建任务")
	}
}

// TestAsync_Persistence 验证异步作业持久化。
// @param t 测试上下文
func TestAsync_Persistence(t *testing.T) {
	path := t.TempDir() + "/state.json"
	first, _ := New(Config{StatePath: path})
	first.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, first, asyncServiceXML("asyncPersist", `flowable:asyncBefore="true"`))
	if _, err := first.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncPersist"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	second, err := New(Config{StatePath: path})
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	second.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	executed, err := second.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("恢复后执行异步作业失败: %v %d", err, executed)
	}
}
