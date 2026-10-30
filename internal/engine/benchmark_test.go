package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

const benchUserTaskXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn"
             targetNamespace="http://example.com">
  <process id="bench" name="Bench" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="approve"/>
    <userTask id="approve" name="Approve" flowable:assignee="${initiator}"/>
    <sequenceFlow id="f2" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// newBenchEngine 创建一个隔离的内存引擎供基准测试使用。
// @param b 基准上下文
// @return 引擎
func newBenchEngine(b *testing.B) *Engine {
	b.Helper()
	engine, err := New(Config{
		HistoryLevel: HistoryAudit,
		Clock:        clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)),
		IDs:          idgen.NewUUIDv7(),
	})
	if err != nil {
		b.Fatalf("创建引擎失败: %v", err)
	}
	return engine
}

// deployBench 部署基准流程。
// @param b 基准上下文
// @param engine 引擎
func deployBench(b *testing.B, engine *Engine) {
	b.Helper()
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Name:      "bench",
		Resources: map[string][]byte{"bench.bpmn20.xml": []byte(benchUserTaskXML)},
	}); err != nil {
		b.Fatalf("部署失败: %v", err)
	}
}

// startBenchInstance 启动一个停在用户任务的实例并返回任务 ID。
// @param b 基准上下文
// @param engine 引擎
// @return 任务 ID
func startBenchInstance(b *testing.B, engine *Engine) string {
	b.Helper()
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "bench",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		b.Fatalf("启动实例失败: %v", err)
	}
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID, Size: 1})
	if err != nil || len(page.Data) != 1 {
		b.Fatalf("查询任务失败: %v %+v", err, page)
	}
	return page.Data[0].ID
}

// BenchmarkStartAndCompleteUserTask 测量启动实例到完成用户任务的端到端命令开销。
// @param b 基准上下文
func BenchmarkStartAndCompleteUserTask(b *testing.B) {
	engine := newBenchEngine(b)
	deployBench(b, engine)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		instance, err := engine.StartProcessInstance(ctx, api.StartRequest{
			ProcessDefinitionKey: "bench",
			Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
		})
		if err != nil {
			b.Fatalf("启动实例失败: %v", err)
		}
		page, err := engine.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 1})
		if err != nil || len(page.Data) != 1 {
			b.Fatalf("查询任务失败: %v %+v", err, page)
		}
		if err := engine.Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
			b.Fatalf("完成任务失败: %v", err)
		}
	}
}

// BenchmarkCompleteUserTask 在固定任务集上测量完成任务命令开销。
// @param b 基准上下文
func BenchmarkCompleteUserTask(b *testing.B) {
	engine := newBenchEngine(b)
	deployBench(b, engine)
	ctx := context.Background()
	taskIDs := make([]string, b.N)
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		taskIDs[i] = startBenchInstance(b, engine)
	}
	b.StartTimer()
	b.ReportAllocs()
	for _, taskID := range taskIDs {
		if err := engine.Complete(ctx, taskID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
			b.Fatalf("完成任务失败: %v", err)
		}
	}
}

// BenchmarkSnapshotRows1000 测量包含 1000 个运行实例时的全量行导出开销。
// @param b 基准上下文
func BenchmarkSnapshotRows1000(b *testing.B) {
	engine := newBenchEngine(b)
	deployBench(b, engine)
	ctx := context.Background()
	b.StopTimer()
	for i := 0; i < 1000; i++ {
		startBenchInstance(b, engine)
	}
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rows, err := engine.SnapshotRows(ctx)
		if err != nil || len(rows) == 0 {
			b.Fatalf("导出快照失败: %v %d", err, len(rows))
		}
	}
}

// BenchmarkListTasks1000 测量 1000 个运行任务的分页查询开销。
// @param b 基准上下文
func BenchmarkListTasks1000(b *testing.B) {
	engine := newBenchEngine(b)
	deployBench(b, engine)
	ctx := context.Background()
	b.StopTimer()
	for i := 0; i < 1000; i++ {
		startBenchInstance(b, engine)
	}
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := engine.ListTasks(ctx, api.TaskFilter{Size: 100})
		if err != nil || page.Total != 1000 {
			b.Fatalf("查询任务失败: %v %+v", err, page)
		}
	}
}
