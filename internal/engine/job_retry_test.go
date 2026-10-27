package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
)

// TestJobRetry_DeadLetter 验证作业重试与死信恢复。
// @param t 测试上下文
func TestJobRetry_DeadLetter(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	fail := true
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error {
		if fail {
			return errors.New("临时失败")
		}
		return nil
	})
	deployXML(t, engine, asyncServiceXML("retry", `flowable:asyncBefore="true"`))
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "retry"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	for i := 0; i < 2; i++ {
		executed, runErr := engine.RunDueJobs(context.Background())
		if runErr != nil || executed != 1 {
			t.Fatalf("第 %d 次重试执行失败: %v %d", i+1, runErr, executed)
		}
		jobs, _ := engine.ListJobs(context.Background())
		if len(jobs) != 1 {
			t.Fatalf("第 %d 次失败后应保留作业", i+1)
		}
		fake.Advance(10 * time.Second)
	}
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("第三次重试失败: %v %d", err, executed)
	}
	jobs, _ := engine.ListJobs(context.Background())
	if len(jobs) != 0 {
		t.Fatalf("重试耗尽后不应有可执行作业")
	}
	dead, _ := engine.ListDeadLetterJobs(context.Background())
	if len(dead) != 1 {
		t.Fatalf("应进入死信: %+v", dead)
	}
	if err := engine.MoveDeadLetterJobToExecutable(context.Background(), dead[0].ID, 2); err != nil {
		t.Fatalf("恢复死信失败: %v", err)
	}
	fail = false
	if executed, err := engine.RunDueJobs(context.Background()); err != nil || executed != 1 {
		t.Fatalf("恢复后执行失败: %v %d", err, executed)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("恢复后应完成流程")
	}
}

// firstDefinitionID 返回第一个流程定义 ID。
// @param t 测试上下文
// @param engine 引擎
// @return 定义 ID
func firstDefinitionID(t *testing.T, engine *Engine) string {
	t.Helper()
	definitions, err := engine.ListProcessDefinitions(context.Background())
	if err != nil || len(definitions) == 0 {
		t.Fatalf("查询定义失败: %v", err)
	}
	return definitions[0].ID
}

// TestJobAdmin_Coverage 验证作业管理边界。
// @param t 测试上下文
func TestJobAdmin_Coverage(t *testing.T) {
	engine, _ := New(Config{})
	if executed, err := engine.runOneJob(context.Background(), "missing"); err != nil || executed != 0 {
		t.Fatalf("缺失作业应返回 0: %v %d", err, executed)
	}
	job := &jobState{ID: "j", Retries: 0}
	engine.handleJobFailure(job, errors.New("x"), time.Now())
	if job.Retries != 2 || engine.state.Jobs["j"] == nil {
		t.Fatalf("默认重试次数错误: %+v", job)
	}
	engine.state.DeadLetters["d"] = &jobState{ID: "d"}
	if err := engine.MoveDeadLetterJobToExecutable(context.Background(), "d", 0); err != nil {
		t.Fatalf("恢复死信失败: %v", err)
	}
	if engine.state.Jobs["d"].Retries != 3 {
		t.Fatalf("默认恢复重试次数错误")
	}
	if err := engine.MoveDeadLetterJobToExecutable(context.Background(), "missing", 1); err == nil {
		t.Fatalf("缺失死信应报错")
	}
	_ = engine.Close(context.Background())
	if _, err := engine.ListJobs(context.Background()); err == nil {
		t.Fatalf("关闭后作业列表应报错")
	}
	if _, err := engine.ListDeadLetterJobs(context.Background()); err == nil {
		t.Fatalf("关闭后死信列表应报错")
	}
	if err := engine.MoveDeadLetterJobToExecutable(context.Background(), "d", 1); err == nil {
		t.Fatalf("关闭后恢复应报错")
	}
	if _, err := engine.RunDueJobs(context.Background()); err == nil {
		t.Fatalf("关闭后执行作业应报错")
	}
}
