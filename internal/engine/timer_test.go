package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

const timerBoundaryXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="timerBoundary" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Wait"/>
    <boundaryEvent id="timeout" attachedToRef="task" cancelActivity="true">
      <timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="normalEnd"/>
    <sequenceFlow id="f3" sourceRef="timeout" targetRef="timeoutEnd"/>
    <endEvent id="normalEnd"/>
    <endEvent id="timeoutEnd"/>
  </process>
</definitions>`

// TestTimer_BoundaryInterrupting 验证中断边界定时器。
// @param t 测试上下文
func TestTimer_BoundaryInterrupting(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, err := New(Config{Clock: fake, HistoryLevel: HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, engine, timerBoundaryXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "timerBoundary"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("期望任务等待")
	}
	executed, err := engine.RunDueJobs(context.Background())
	if err != nil || executed != 0 {
		t.Fatalf("未到期不应执行: %v %d", err, executed)
	}
	fake.Advance(time.Minute)
	executed, err = engine.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("到期应执行: %v %d", err, executed)
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("中断边界应删除任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("边界路径应结束实例")
	}
}

const nonInterruptingTimerXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="timerNon" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Wait"/>
    <boundaryEvent id="remind" attachedToRef="task" cancelActivity="false">
      <timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="end"/>
    <sequenceFlow id="f3" sourceRef="remind" targetRef="remindEnd"/>
    <endEvent id="end"/>
    <endEvent id="remindEnd"/>
  </process>
</definitions>`

// TestTimer_BoundaryNonInterrupting 验证非中断边界定时器。
// @param t 测试上下文
func TestTimer_BoundaryNonInterrupting(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, nonInterruptingTimerXML)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "timerNon"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	fake.Advance(time.Minute)
	if _, err := engine.RunDueJobs(context.Background()); err != nil {
		t.Fatalf("定时器执行失败: %v", err)
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("非中断边界应保留任务")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("原任务未完成，实例不应结束")
	}
}

// TestParseISODuration 验证 ISO 持续时间解析。
// @param t 测试上下文
func TestParseISODuration(t *testing.T) {
	cases := []struct {
		value string
		want  time.Duration
	}{
		{"PT1M", time.Minute},
		{"P1D", 24 * time.Hour},
		{"P1DT2H30M", 26*time.Hour + 30*time.Minute},
		{"PT0.5S", 500 * time.Millisecond},
	}
	for _, tc := range cases {
		got, err := parseISODuration(tc.value)
		if err != nil || got != tc.want {
			t.Fatalf("%s 期望 %v，实际 %v %v", tc.value, tc.want, got, err)
		}
	}
	for _, value := range []string{"", "1M", "PT", "P0D", "P1Y", "PT1X"} {
		if _, err := parseISODuration(value); err == nil {
			t.Fatalf("%q 应解析失败", value)
		}
	}
}

// TestTimerDueDate 验证到期时间计算。
// @param t 测试上下文
func TestTimerDueDate(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	due, ok, err := timerDueDate(now, &model.TimerDefinition{TimeDate: "2026-10-28T00:01:00Z"})
	if err != nil || !ok || !due.Equal(now.Add(time.Minute)) {
		t.Fatalf("timeDate 计算错误: %v %v %v", due, ok, err)
	}
	due, ok, err = timerDueDate(now, &model.TimerDefinition{TimeCycle: "R3/PT5M"})
	if err != nil || !ok || !due.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("timeCycle 计算错误: %v %v %v", due, ok, err)
	}
	if _, ok, err := timerDueDate(now, &model.TimerDefinition{}); err != nil || ok {
		t.Fatalf("空定时器应返回不存在")
	}
	if _, _, err := timerDueDate(now, &model.TimerDefinition{TimeDuration: "bad"}); err == nil {
		t.Fatalf("非法持续时间应报错")
	}
}

// TestTimer_Persistence 验证定时器作业持久化。
// @param t 测试上下文
func TestTimer_Persistence(t *testing.T) {
	path := t.TempDir() + "/state.json"
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	first, _ := New(Config{Clock: fake, StatePath: path})
	deployXML(t, first, timerBoundaryXML)
	if _, err := first.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "timerBoundary"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	second, err := New(Config{Clock: fake, StatePath: path})
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	fake.Advance(time.Minute)
	executed, err := second.RunDueJobs(context.Background())
	if err != nil || executed != 1 {
		t.Fatalf("恢复后执行定时器失败: %v %d", err, executed)
	}
}
