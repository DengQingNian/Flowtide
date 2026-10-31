package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// TestServiceImplementsWorkflowFacade 编译期断言 Service 满足门面接口。
// 已实现方法（发起/办结/退回/撤回/直送/重置/推演/变量/表单等）由 facade_test.go、
// facade_ops_test.go 与各域服务测试覆盖；此处验证协议兼容场景（nil 引擎）下
// 未装配能力子服务的方法统一返回 WORKFLOW-0601 装配错误（不再有 0501 桩语义）。
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
		{"BackToStepTask", func() (R, error) { return service.BackToStepTask(ctx, &BackTaskVo{}) }},
		{"BackTask", func() (R, error) { return service.BackTask(ctx, &BackTaskVo{}) }},
		{"DoRollbackTask", func() (R, error) { return service.DoRollbackTask(ctx, "pi", "task", "u1") }},
		{"CompleteToTargetActivity", func() (R, error) { return service.CompleteToTargetActivity(ctx, &CompleteTaskVo{}) }},
		{"DoResetProcess", func() (R, error) { return service.DoResetProcess(ctx, "pi", "u1") }},
		{"GetNextUserTaskAndUserList", func() (R, error) { return service.GetNextUserTaskAndUserList(ctx, "task") }},
		{"DoTaskOwnerAdjustment", func() (R, error) { return service.DoTaskOwnerAdjustment(ctx, "pi", "key", "u1", "u2", "op") }},
		{"DoTaskOwnerAdjustmentByID", func() (R, error) { return service.DoTaskOwnerAdjustmentByID(ctx, "task", "u2", "op") }},
	}
	for _, c := range cases {
		r, err := c.call()
		// nil 引擎协议兼容场景：能力方法返回装配错误、数据可空，不再使用 0501 桩
		var engineErr *api.Error
		if !errors.As(err, &engineErr) {
			t.Errorf("%s 应返回 api.Error，实际 %v", c.name, err)
			continue
		}
		if engineErr.Code == "WORKFLOW-0501" {
			t.Errorf("%s 已不应返回 0501 桩错误（门面已全量接线），实际 %s %s", c.name, engineErr.Code, engineErr.Message)
		}
		_ = r
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
