package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
)

// TestTimerJobBucket 验证定时器作业的移动、改期与跨桶保护。
// @param t 测试上下文
func TestTimerJobBucket(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, timerJobXML)
	ctx := context.Background()
	jobs, err := engine.ListJobs(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("定时作业未创建: %v %+v", err, jobs)
	}
	jobID := jobs[0].ID
	if err := engine.DeleteJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("定时器作业不应从可执行桶删除: %v", err)
	}
	if err := engine.RescheduleTimerJob(ctx, jobID, "bad-date"); !errors.Is(err, api.ErrBadRequest) {
		t.Fatalf("非法到期时间应 400: %v", err)
	}
	if err := engine.RescheduleTimerJob(ctx, jobID, "2030-01-02T03:04:05Z"); err != nil {
		t.Fatalf("改期失败: %v", err)
	}
	job, err := engine.GetJob(ctx, jobID)
	if err != nil || job.DueDate.UTC().Format(time.RFC3339) != "2030-01-02T03:04:05Z" {
		t.Fatalf("改期后到期时间错误: %v %+v", err, job)
	}
	if err := engine.MoveTimerJobToExecutable(ctx, jobID); err != nil {
		t.Fatalf("移动定时器作业失败: %v", err)
	}
	job, err = engine.GetJob(ctx, jobID)
	if err != nil || !job.Executable {
		t.Fatalf("移动后应标记可执行: %v %+v", err, job)
	}
	if err := engine.DeleteTimerJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("移动后不应再从定时器桶删除: %v", err)
	}
	if err := engine.RescheduleTimerJob(ctx, jobID, "2030-01-02T03:04:05Z"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("移动后不应再改期: %v", err)
	}
	if err := engine.MoveTimerJobToExecutable(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复移动应 404: %v", err)
	}
	if err := engine.DeleteJob(ctx, jobID); err != nil {
		t.Fatalf("删除已移动作业失败: %v", err)
	}
	if err := engine.DeleteJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除应 404: %v", err)
	}
	if err := engine.DeleteTimerJob(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失定时器作业应 404: %v", err)
	}
	if err := engine.RescheduleTimerJob(ctx, "missing", "2030-01-02T03:04:05Z"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失定时器作业改期应 404: %v", err)
	}
	if err := engine.MoveTimerJobToExecutable(ctx, "missing"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("缺失定时器作业移动应 404: %v", err)
	}
}

// TestSuspendedJobBucket 验证挂起作业只能从挂起桶删除。
// @param t 测试上下文
func TestSuspendedJobBucket(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, timerBoundaryXML)
	ctx := context.Background()
	instance, err := engine.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "timerBoundary"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	jobs, err := engine.ListJobs(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("边界定时器未创建: %v %+v", err, jobs)
	}
	jobID := jobs[0].ID
	if err := engine.SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatalf("挂起实例失败: %v", err)
	}
	if err := engine.DeleteTimerJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("挂起作业不应从定时器桶删除: %v", err)
	}
	if err := engine.DeleteJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("挂起作业不应从可执行桶删除: %v", err)
	}
	if err := engine.MoveTimerJobToExecutable(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("挂起作业不应被移动: %v", err)
	}
	if err := engine.DeleteSuspendedJob(ctx, jobID); err != nil {
		t.Fatalf("删除挂起作业失败: %v", err)
	}
	if err := engine.DeleteSuspendedJob(ctx, jobID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除挂起作业应 404: %v", err)
	}
}

// TestDeadLetterJobBucket 验证死信作业只能从死信桶删除。
// @param t 测试上下文
func TestDeadLetterJobBucket(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, err := New(Config{Clock: fake})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	engine.RegisterDelegate("noop", func(ctx context.Context, execution *Execution) error {
		return errors.New("失败")
	})
	deployXML(t, engine, asyncServiceXML("bucket", `flowable:asyncBefore="true"`))
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "bucket"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	jobs, _ := engine.ListJobs(context.Background())
	if len(jobs) != 1 {
		t.Fatalf("可执行作业未创建: %+v", jobs)
	}
	if err := engine.SetJobRetries(context.Background(), jobs[0].ID, 1); err != nil {
		t.Fatalf("设置重试失败: %v", err)
	}
	if _, err := engine.RunDueJobs(context.Background()); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	dead, err := engine.ListDeadLetterJobs(context.Background())
	if err != nil || len(dead) != 1 {
		t.Fatalf("死信作业未创建: %v %+v", err, dead)
	}
	deadID := dead[0].ID
	if err := engine.DeleteJob(context.Background(), deadID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("死信作业不应从可执行桶删除: %v", err)
	}
	if err := engine.DeleteTimerJob(context.Background(), deadID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("死信作业不应从定时器桶删除: %v", err)
	}
	if err := engine.DeleteSuspendedJob(context.Background(), deadID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("死信作业不应从挂起桶删除: %v", err)
	}
	if err := engine.DeleteDeadLetterJob(context.Background(), deadID); err != nil {
		t.Fatalf("删除死信作业失败: %v", err)
	}
	if err := engine.DeleteDeadLetterJob(context.Background(), deadID); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("重复删除死信作业应 404: %v", err)
	}
}
