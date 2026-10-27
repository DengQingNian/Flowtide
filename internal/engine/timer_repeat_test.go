package engine

import (
	"context"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"

	"github.com/flowable-go/flowable/internal/clock"
)

const repeatTimerStartXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="repeatStart" isExecutable="true">
    <startEvent id="s"><timerEventDefinition><timeCycle>R3/PT1M</timeCycle></timerEventDefinition></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestTimerStart_Repeat 验证重复定时启动。
// @param t 测试上下文
func TestTimerStart_Repeat(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, repeatTimerStartXML)
	for i := 0; i < 3; i++ {
		fake.Advance(time.Minute)
		if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
			t.Fatalf("第 %d 次重复触发失败: %v %d", i+1, err, executed)
		}
	}
	fake.Advance(time.Minute)
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 0 {
		t.Fatalf("重复耗尽后不应触发: %v %d", err, executed)
	}
	instances, _ := engine.ListProcessInstances(context.Background())
	if len(instances) != 3 {
		t.Fatalf("期望 3 个实例，实际 %d", len(instances))
	}
}

// TestParseTimerCycle 验证重复周期解析。
// @param t 测试上下文
func TestParseTimerCycle(t *testing.T) {
	cycle, err := parseTimerCycle("R3/PT1M")
	if err != nil || cycle.interval != time.Minute || cycle.remaining != 2 || cycle.infinite {
		t.Fatalf("R3 解析错误: %+v %v", cycle, err)
	}
	cycle, err = parseTimerCycle("R/PT5M")
	if err != nil || !cycle.infinite || cycle.interval != 5*time.Minute {
		t.Fatalf("无限重复解析错误: %+v %v", cycle, err)
	}
	for _, value := range []string{"PT1M", "R0/PT1M", "R3"} {
		if _, err := parseTimerCycle(value); err == nil {
			t.Fatalf("%q 应解析失败", value)
		}
	}
}

// TestTimerRepeat_Infinite 验证无限重复。
// @param t 测试上下文
func TestTimerRepeat_Infinite(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="infinite" isExecutable="true"><startEvent id="s"><timerEventDefinition><timeCycle>R/PT1M</timeCycle></timerEventDefinition></startEvent><sequenceFlow id="f1" sourceRef="s" targetRef="e"/><endEvent id="e"/></process></definitions>`
	deployXML(t, engine, xml)
	for i := 0; i < 2; i++ {
		fake.Advance(time.Minute)
		if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
			t.Fatalf("无限重复触发失败: %v %d", err, executed)
		}
	}
	jobs, _ := engine.ListJobs(context.Background())
	if len(jobs) != 1 {
		t.Fatalf("无限重复应保留作业")
	}
}

const repeatBoundaryXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="repeatBoundary" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task"/>
    <userTask id="task" name="Task"/>
    <boundaryEvent id="remind" attachedToRef="task" cancelActivity="false">
      <timerEventDefinition><timeCycle>R2/PT1M</timeCycle></timerEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="task" targetRef="e"/>
    <sequenceFlow id="f3" sourceRef="remind" targetRef="remindEnd"/>
    <endEvent id="e"/>
    <endEvent id="remindEnd"/>
  </process>
</definitions>`

// TestBoundaryTimer_Repeat 验证非中断边界定时器重复。
// @param t 测试上下文
func TestBoundaryTimer_Repeat(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, repeatBoundaryXML)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "repeatBoundary"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	fake.Advance(time.Minute)
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("第一次边界重复失败: %v %d", err, executed)
	}
	jobs, _ := engine.ListJobs(context.Background())
	if len(jobs) != 1 {
		t.Fatalf("第一次后应重新排期")
	}
	fake.Advance(time.Minute)
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("第二次边界重复失败: %v %d", err, executed)
	}
	jobs, _ = engine.ListJobs(context.Background())
	if len(jobs) != 0 {
		t.Fatalf("重复耗尽后不应有作业")
	}
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("非中断边界应保留主任务")
	}
}

// TestTimerCycle_MoreErrors 验证重复周期更多错误。
// @param t 测试上下文
func TestTimerCycle_MoreErrors(t *testing.T) {
	if _, err := parseTimerCycle("RX/PT1M"); err == nil {
		t.Fatalf("非法次数应解析失败")
	}
	if _, err := parseTimerCycle("R3/PT1M/extra"); err == nil {
		t.Fatalf("非法间隔应解析失败")
	}
	engine, _ := New(Config{})
	engine.rescheduleTimerJob(&jobState{ID: "j", IntervalMillis: 0}, time.Now())
	if len(engine.state.Jobs) != 0 {
		t.Fatalf("无间隔不应排期")
	}
}
