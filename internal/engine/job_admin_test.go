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

// TestDeleteJob 验证可执行作业删除。
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
	if err := engine.DeleteJob(ctx, jobID); err != nil {
		t.Fatalf("删除作业失败: %v", err)
	}
	if jobs, _ := engine.ListJobs(ctx); len(jobs) != 0 {
		t.Fatalf("删除后作业应为空: %+v", jobs)
	}
	if _, err := engine.GetJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("删除后查询应 404: %v", err)
	}
	if err := engine.DeleteJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
}
