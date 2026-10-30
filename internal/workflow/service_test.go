package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// TestServiceImplementsWorkflowFacade 编译期断言 Service 满足门面接口，并逐一验证
// 阶段 0 全部方法返回 WORKFLOW-0501"接口未实现"错误与空 R。
// @param t 测试上下文
func TestServiceImplementsWorkflowFacade(t *testing.T) {
	var facade api.WorkflowFacade = NewWorkflowService(nil)
	service, ok := facade.(*Service)
	if !ok {
		t.Fatalf("NewWorkflowService 应返回 *Service")
	}
	ctx := context.Background()
	cases := []struct {
		name string // 用例名
		call func() (R, error)
	}{
		{"StartProcess", func() (R, error) { return service.StartProcess(ctx, &StartProcessInstanceVo{}) }},
		{"StartProcessAndFirstStep", func() (R, error) { return service.StartProcessAndFirstStep(ctx, &StartProcessInstanceVo{}) }},
		{"Complete", func() (R, error) { return service.Complete(ctx, &CompleteTaskVo{}) }},
		{"BackToStepTask", func() (R, error) { return service.BackToStepTask(ctx, &BackTaskVo{}) }},
		{"BackTask", func() (R, error) { return service.BackTask(ctx, &BackTaskVo{}) }},
		{"DoRollbackTask", func() (R, error) { return service.DoRollbackTask(ctx, "pi", "task", "u1") }},
		{"CompleteToTargetActivity", func() (R, error) { return service.CompleteToTargetActivity(ctx, &CompleteTaskVo{}) }},
		{"DoResetProcess", func() (R, error) { return service.DoResetProcess(ctx, "pi", "u1") }},
		{"GetTodoTaskListByIdentityID", func() (R, error) { return service.GetTodoTaskListByIdentityID(ctx, "u1", PageQuery{}) }},
		{"GetDoneTaskListByIdentityID", func() (R, error) { return service.GetDoneTaskListByIdentityID(ctx, "u1", PageQuery{}) }},
		{"GetNextUserTaskAndUserList", func() (R, error) { return service.GetNextUserTaskAndUserList(ctx, "task") }},
		{"SaveAgreeNextActivity", func() (R, error) { return service.SaveAgreeNextActivity(ctx, &NextActivityRequest{}) }},
		{"IsProcessDeployed", func() (R, error) { return service.IsProcessDeployed(ctx, "sys", "key") }},
		{"IsExistRunningProcess", func() (R, error) { return service.IsExistRunningProcess(ctx, "sys", "key", "biz") }},
		{"GetWorkflowStatusByBusinessID", func() (R, error) { return service.GetWorkflowStatusByBusinessID(ctx, "sys", "key", "biz") }},
		{"UpdateProcessGlobalVariable", func() (R, error) { return service.UpdateProcessGlobalVariable(ctx, "pi", map[string]any{}) }},
		{"GetProcessGlobalVariableByProcessInstanceID", func() (R, error) { return service.GetProcessGlobalVariableByProcessInstanceID(ctx, "pi", nil) }},
		{"GetProcessGlobalVariableByBusinessID", func() (R, error) { return service.GetProcessGlobalVariableByBusinessID(ctx, "sys", "biz", nil) }},
		{"UpdateWorkflowDocumentByProcessInstanceIDOrTaskID", func() (R, error) {
			return service.UpdateWorkflowDocumentByProcessInstanceIDOrTaskID(ctx, "pi", "task", nil)
		}},
		{"DoTaskOwnerAdjustment", func() (R, error) { return service.DoTaskOwnerAdjustment(ctx, "pi", "key", "u1", "u2", "op") }},
		{"DoTaskOwnerAdjustmentByID", func() (R, error) { return service.DoTaskOwnerAdjustmentByID(ctx, "task", "u2", "op") }},
		{"GetTaskMessageListByBusinessID", func() (R, error) { return service.GetTaskMessageListByBusinessID(ctx, "sys", "biz") }},
		{"GetTaskHistoryListByProcessInstanceID", func() (R, error) { return service.GetTaskHistoryListByProcessInstanceID(ctx, "pi") }},
		{"GetTaskHistoryListByBusinessID", func() (R, error) { return service.GetTaskHistoryListByBusinessID(ctx, "sys", "biz") }},
		{"GetTaskHistoryListByTaskID", func() (R, error) { return service.GetTaskHistoryListByTaskID(ctx, "task") }},
		{"GetTaskInfoByTaskID", func() (R, error) { return service.GetTaskInfoByTaskID(ctx, "task") }},
	}
	if len(cases) != 26 {
		t.Fatalf("门面方法应覆盖 26 个，实际用例 %d 个", len(cases))
	}
	for _, c := range cases {
		r, err := c.call()
		if r != (R{}) {
			t.Errorf("%s 桩应返回空 R，实际 %+v", c.name, r)
		}
		var engineErr *api.Error
		if !errors.As(err, &engineErr) {
			t.Errorf("%s 应返回 api.Error，实际 %v", c.name, err)
			continue
		}
		if engineErr.Code != "WORKFLOW-0501" || engineErr.Message != "接口未实现" {
			t.Errorf("%s 应返回 WORKFLOW-0501 接口未实现，实际 %s %s", c.name, engineErr.Code, engineErr.Message)
		}
	}
}

// TestNewWorkflowServiceDefaults 验证构造函数默认值与选项注入。
// @param t 测试上下文
func TestNewWorkflowServiceDefaults(t *testing.T) {
	service := NewWorkflowService(nil)
	opts := service.Options()
	// 调度开关默认全 false
	if opts.NoHandleAutoComplete || opts.TaskTimeoutRemind || opts.SendWorkflowRemind ||
		opts.SendTodoRemind || opts.AdjacentActivitySameOwnerAutoComplete {
		t.Error("调度开关默认应全为 false")
	}
	// 默认时钟为真实时钟
	if _, isReal := opts.Clock.(clock.Real); !isReal {
		t.Errorf("默认时钟应为 clock.Real，实际 %T", opts.Clock)
	}

	// 选项注入：开关与假时钟
	fake := clock.NewFake(time.Unix(1700000000, 0))
	service = NewWorkflowService(nil,
		WithNoHandleAutoComplete(true),
		WithTaskTimeoutRemind(true),
		WithSendWorkflowRemind(true),
		WithSendTodoRemind(true),
		WithAdjacentActivitySameOwnerAutoComplete(true),
		WithClock(fake),
	)
	opts = service.Options()
	if !opts.NoHandleAutoComplete || !opts.TaskTimeoutRemind || !opts.SendWorkflowRemind ||
		!opts.SendTodoRemind || !opts.AdjacentActivitySameOwnerAutoComplete {
		t.Error("选项注入后调度开关应全为 true")
	}
	if opts.Clock != clock.Clock(fake) {
		t.Errorf("注入时钟应生效，实际 %T", opts.Clock)
	}
}

// TestServiceEngineAccessor 验证引擎门面存取。
// @param t 测试上下文
func TestServiceEngineAccessor(t *testing.T) {
	if NewWorkflowService(nil).Engine() != nil {
		t.Error("nil 引擎应原样返回 nil")
	}
}
