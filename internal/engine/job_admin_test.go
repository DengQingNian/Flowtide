package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
)

const timerJobXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="timerJob" isExecutable="true">
    <startEvent id="s"><timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition></startEvent>
    <sequenceFlow id="f1" sourceRef="s" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// TestDeleteJob 验证定时器作业不能从可执行桶删除。
// @param t 测试上下文
func TestDeleteJob(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, timerJobXML)
	ctx := context.Background()
	jobs, err := engine.ListJobs(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("定时作业未创建: %v %+v", err, jobs)
	}
	jobID := jobs[0].ID
	// 定时器作业属于独立桶，跨桶删除必须 404
	if err := engine.DeleteJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("定时器作业从可执行桶删除应 404: %v", err)
	}
	if err := engine.DeleteTimerJob(ctx, jobID); err != nil {
		t.Fatalf("删除定时器作业失败: %v", err)
	}
	if jobs, _ := engine.ListJobs(ctx); len(jobs) != 0 {
		t.Fatalf("删除后作业应为空: %+v", jobs)
	}
	if _, err := engine.GetJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后查询应 404: %v", err)
	}
	if err := engine.DeleteTimerJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
}

// TestDeleteExecutableJob 验证可执行作业删除。
// @param t 测试上下文
func TestDeleteExecutableJob(t *testing.T) {
	engine := newTestEngine(t)
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error { return nil })
	deployXML(t, engine, asyncServiceXML("asyncDelete", `flowable:asyncBefore="true"`))
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "asyncDelete"}); err != nil {
		t.Fatalf("启动实例失败: %v", err)
	}
	jobs, err := engine.ListJobs(context.Background())
	if err != nil || len(jobs) != 1 {
		t.Fatalf("可执行作业未创建: %v %+v", err, jobs)
	}
	if err := engine.DeleteJob(context.Background(), jobs[0].ID); err != nil {
		t.Fatalf("删除可执行作业失败: %v", err)
	}
	if err := engine.DeleteJob(context.Background(), jobs[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
}
