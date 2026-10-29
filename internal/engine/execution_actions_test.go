package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestMessageEventReceivedForExecution 验证定向消息投递。
// @param t 测试上下文
func TestMessageEventReceivedForExecution(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, messageCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "msgCatch"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	executionID := waitingActivityExecutionID(t, engine, instance.ID)
	if err := engine.MessageEventReceivedForExecution(context.Background(), executionID, "other", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("错误消息名应 404: %v", err)
	}
	if err := engine.MessageEventReceivedForExecution(context.Background(), executionID, "approvedMessage", []api.Variable{{Name: "assignee", Value: "bob"}}); err != nil {
		t.Fatalf("定向消息失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].Assignee != "bob" {
		t.Fatalf("定向消息后任务错误: %+v", tasks)
	}
	if err := engine.MessageEventReceivedForExecution(context.Background(), executionID, "approvedMessage", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("订阅消费后应 404: %v", err)
	}
	if err := engine.MessageEventReceivedForExecution(context.Background(), "missing", "approvedMessage", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失执行应 404: %v", err)
	}
}

// TestSignalEventReceivedForExecution 验证定向信号投递。
// @param t 测试上下文
func TestSignalEventReceivedForExecution(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, signalCatchXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "signalCatch"})
	executionID := waitingActivityExecutionID(t, engine, instance.ID)
	if err := engine.SignalEventReceivedForExecution(context.Background(), executionID, "wrong", nil); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("错误信号名应 404: %v", err)
	}
	if err := engine.SignalEventReceivedForExecution(context.Background(), executionID, "broadcast", nil); err != nil {
		t.Fatalf("定向信号失败: %v", err)
	}
	if tasks := listTasks(t, engine); len(tasks) != 1 {
		t.Fatalf("定向信号后任务错误: %+v", tasks)
	}
}
