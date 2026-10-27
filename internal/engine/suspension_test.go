package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
)

// TestSuspendProcessDefinition 验证定义挂起与激活。
// @param t 测试上下文
func TestSuspendProcessDefinition(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definition, _ := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err := engine.SuspendProcessDefinition(context.Background(), definition.ID, false); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple"}); err == nil {
		t.Fatalf("挂起后应拒绝启动")
	}
	if err := engine.ActivateProcessDefinition(context.Background(), definition.ID, false); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("激活后启动失败: %v", err)
	}
	if err := engine.SuspendProcessDefinition(context.Background(), "missing", false); err == nil {
		t.Fatalf("缺失定义应报错")
	}
	if err := engine.ActivateProcessDefinition(context.Background(), "missing", false); err == nil {
		t.Fatalf("缺失定义应报错")
	}
}

// TestSuspendProcessInstance 验证实例挂起与任务限制。
// @param t 测试上下文
func TestSuspendProcessInstance(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	task := listTasks(t, engine)[0]
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起实例失败: %v", err)
	}
	if err := engine.Claim(context.Background(), task.ID, "alice"); err == nil {
		t.Fatalf("挂起实例任务不应允许签收")
	}
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{UserID: "alice"}); err == nil {
		t.Fatalf("挂起实例任务不应允许完成")
	}
	if err := engine.ActivateProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("激活实例失败: %v", err)
	}
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("激活后完成失败: %v", err)
	}
	if err := engine.SuspendProcessInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失实例应报错")
	}
	if err := engine.ActivateProcessInstance(context.Background(), "missing"); err == nil {
		t.Fatalf("缺失实例应报错")
	}
}

// TestSuspendDefinitionWithInstances 验证同时挂起实例。
// @param t 测试上下文
func TestSuspendDefinitionWithInstances(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	definition, _ := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err := engine.SuspendProcessDefinition(context.Background(), definition.ID, true); err != nil {
		t.Fatalf("挂起定义失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Suspended {
		t.Fatalf("实例应被挂起")
	}
}

// TestSuspendedTimerJob 验证挂起实例不执行定时器。
// @param t 测试上下文
func TestSuspendedTimerJob(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, timerBoundaryXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "timerBoundary"})
	if err := engine.SuspendProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	fake.Advance(time.Minute)
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 0 {
		t.Fatalf("挂起实例定时器不应执行: %v %d", err, executed)
	}
	if err := engine.ActivateProcessInstance(context.Background(), instance.ID); err != nil {
		t.Fatalf("激活失败: %v", err)
	}
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("激活后定时器应执行: %v %d", err, executed)
	}
}
