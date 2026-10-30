package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// hookTestXML 是扩展监听器测试语料：节点 start 挂 class="countersignCollectionConfig"。
const hookTestXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="hookTest" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f0" sourceRef="start" targetRef="t1"/>
    <userTask id="t1" name="Hook" flowable:assignee="alice">
      <extensionElements>
        <flowable:executionListener event="start" class="countersignCollectionConfig"/>
      </extensionElements>
    </userTask>
    <sequenceFlow id="f1" sourceRef="t1" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// TestExecutionListenerHookWritesVariables 验证扩展监听器在命令内写变量与延迟动作登记。
// @param t 测试上下文
func TestExecutionListenerHookWritesVariables(t *testing.T) {
	eng := newTestEngine(t)
	deferred := 0
	eng.RegisterExecutionListenerHook("countersignCollectionConfig", func(_ context.Context, execution api.ListenerExecution, ctl api.ListenerControl) error {
		// 读取快照变量并写入会签变量族
		if _, ok := execution.Variables["initiator"]; execution.ProcessInstanceID != "" && !ok {
			t.Fatalf("快照应含实例变量")
		}
		if err := ctl.SetVariable("executorList", []string{"alice"}); err != nil {
			return err
		}
		if err := ctl.SetVariableLocal("localMark", true); err != nil {
			return err
		}
		ctl.Defer(func(context.Context) error { deferred++; return nil })
		return nil
	})
	deployXML(t, eng, hookTestXML)
	instance, err := eng.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "hookTest",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	// 延迟动作在命令返回前（锁外）同步执行
	if deferred != 1 {
		t.Fatalf("延迟动作应在命令提交后执行: %d", deferred)
	}
	// 实例级变量与局部变量均已写入
	if value, err := eng.GetVariable(context.Background(), instance.ID, "executorList"); err != nil || value == nil {
		t.Fatalf("实例级变量应写入: %v %v", value, err)
	}
	tasks := listTasks(t, eng)
	if len(tasks) != 1 {
		t.Fatalf("应有 1 个任务: %d", len(tasks))
	}
	if value, err := eng.TaskService().GetTaskVariable(context.Background(), tasks[0].ID, "localMark"); err != nil || value != true {
		t.Fatalf("局部变量应写入: %v %v", value, err)
	}
}

// TestExecutionListenerHookRollback 验证扩展监听器错误使命令整体回滚。
// @param t 测试上下文
func TestExecutionListenerHookRollback(t *testing.T) {
	eng := newTestEngine(t)
	eng.RegisterExecutionListenerHook("countersignCollectionConfig", func(context.Context, api.ListenerExecution, api.ListenerControl) error {
		return errors.New("监听器失败")
	})
	deployXML(t, eng, hookTestXML)
	_, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookTest"})
	if err == nil || !strings.Contains(err.Error(), "执行监听器执行失败") {
		t.Fatalf("监听器错误应上抛: %v", err)
	}
	// 回滚：实例未创建
	instances, err := eng.ListProcessInstances(context.Background())
	if err != nil || len(instances) != 0 {
		t.Fatalf("命令应回滚无实例: %d %v", len(instances), err)
	}
}

// TestExecutionListenerHookFallback 验证扩展监听器未注册时回退原生监听器表。
// @param t 测试上下文
func TestExecutionListenerHookFallback(t *testing.T) {
	eng := newTestEngine(t)
	called := false
	eng.RegisterExecutionListener("countersignCollectionConfig", func(context.Context, *Execution) error {
		called = true
		return nil
	})
	deployXML(t, eng, hookTestXML)
	if _, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookTest"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !called {
		t.Fatal("未注册扩展监听器时应回退原生监听器")
	}
	// 注销扩展监听器（nil）后仍回退原生监听器
	eng.RegisterExecutionListenerHook("countersignCollectionConfig", nil)
	called = false
	if _, err := eng.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookTest"}); err != nil {
		t.Fatalf("再次启动失败: %v", err)
	}
	if !called {
		t.Fatal("注销后应回退原生监听器")
	}
}

// TestListenerControlMissingExecution 验证控制接口对已消失执行的错误返回。
// @param t 测试上下文
func TestListenerControlMissingExecution(t *testing.T) {
	eng := newTestEngine(t)
	ctl := &listenerControl{engine: eng, state: &executionState{ID: "ghost"}, cc: &commandContext{}}
	if err := ctl.SetVariable("k", 1); err == nil {
		t.Fatal("执行不存在应报错")
	}
	if err := ctl.SetVariableLocal("k", 1); err == nil {
		t.Fatal("执行不存在应报错")
	}
	// 空延迟动作忽略
	ctl.Defer(nil)
	if len(ctl.cc.deferred) != 0 {
		t.Fatal("空动作不应入队")
	}
}
