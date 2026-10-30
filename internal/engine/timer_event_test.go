package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

const timerStartXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="timerStart" isExecutable="true">
    <startEvent id="s"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestTimerStartEvent 验证定时启动事件。
// @param t 测试上下文
func TestTimerStartEvent(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, timerStartXML)
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("定时启动前不应创建实例")
	}
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 0 {
		t.Fatalf("未到期不应执行: %v %d", err, executed)
	}
	fake.Advance(time.Minute)
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("定时启动失败: %v %d", err, executed)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("定时启动应创建任务")
	}
}

const timerCatchXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="timerCatch" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="wait"/>
    <intermediateCatchEvent id="wait"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></intermediateCatchEvent>
    <sequenceFlow id="f2" sourceRef="wait" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f3" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestTimerCatchEvent 验证中间定时捕获。
// @param t 测试上下文
func TestTimerCatchEvent(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, timerCatchXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "timerCatch"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("定时捕获前不应有任务")
	}
	fake.Advance(time.Minute)
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("定时捕获执行失败: %v %d", err, executed)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("定时捕获后应创建任务")
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("定时捕获流程应结束")
	}
}

// TestTimerStart_Invalid 验证非法定时启动。
// @param t 测试上下文
func TestTimerStart_Invalid(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="badTimerStart" isExecutable="true"><startEvent id="s"><timerEventDefinition><timeDuration>bad</timeDuration></timerEventDefinition></startEvent><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{Resources: map[string][]byte{"bad.bpmn20.xml": []byte(xml)}}); err == nil {
		t.Fatalf("非法定时启动应部署失败")
	}
}
