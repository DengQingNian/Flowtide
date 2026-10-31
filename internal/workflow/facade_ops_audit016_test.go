// facade_ops_audit016_test.go 覆盖门面装配层的端口空实现与防御分支（016 号审计）。
//
// 背景：facade_ops.go 的 nop 端口与 requireCapable 防御分支在实际链路中不会触发，
// 是覆盖率缺口的主要来源。本文件以表驱动方式直接调用这些端口实现。
package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// TestNopPorts 覆盖装配层空实现的恒定返回值。
// @param t 测试上下文
func TestNopPorts(t *testing.T) {
	ctx := context.Background()
	if modelID, err := (nopCategoryLookup{}).ResolveModelID(ctx, "tenant", "flow"); modelID != "" || err != nil {
		t.Fatalf("分类解析空实现应返回空串与 nil: %q %v", modelID, err)
	}
	if stepID, err := (nopStepResolver{}).ResolveStepID(ctx, "step-1"); stepID != "" || err != nil {
		t.Fatalf("步骤解析空实现应返回空串与 nil: %q %v", stepID, err)
	}
}

// TestFacadeGuards_NilEngine 验证门面方法在能力未装配时的防御分支。
// 这些分支返回 WORKFLOW-0501，是 501 桩协议的基础。
// @param t 测试上下文
func TestFacadeGuards_NilEngine(t *testing.T) {
	ctx := context.Background()
	// 无引擎装配的门面：能力子服务全为 nil，各入口应返回装配错误而非 panic
	svc := &Service{comp: &components{}}
	cases := []struct {
		name string
		call func() (R, error)
	}{
		{"DoDeleteProcessInstance", func() (R, error) { return svc.DoDeleteProcessInstance(ctx, "pi-1", "u1", "reason") }},
		{"DoSuspendProcessInstance", func() (R, error) { return svc.DoSuspendProcessInstance(ctx, "pi-1", "u1") }},
		{"DoActivateProcessInstance", func() (R, error) { return svc.DoActivateProcessInstance(ctx, "pi-1", "u1") }},
		{"GetBackNodes", func() (R, error) { return svc.GetBackNodes(ctx, "task-1") }},
		{"CompleteSign", func() (R, error) { return svc.CompleteSign(ctx, "task-1", 1) }},
		{"DoReactivateProcess", func() (R, error) { return svc.DoReactivateProcess(ctx, "pi-1", "act-1", "u1") }},
		{"DoStopTaskByTaskID", func() (R, error) { return svc.DoStopTaskByTaskID(ctx, "task-1", "u1") }},
		{"DoDeleteProcessByBusinessID", func() (R, error) { return svc.DoDeleteProcessByBusinessID(ctx, "sys", "biz-1", "u1") }},
		{"DoTaskOwnerAdjustment", func() (R, error) { return svc.DoTaskOwnerAdjustment(ctx, "pi-1", "act-1", "u1", "u2", "u1") }},
		{"AddDelegateTask", func() (R, error) { return svc.AddDelegateTask(ctx, nil) }},
		{"AddRemind", func() (R, error) { return svc.AddRemind(ctx, RemindSaveRequest{}) }},
		{"ListTaskExceptions", func() (R, error) { return svc.ListTaskExceptions(ctx, "pi-1") }},
		{"AddExecution", func() (R, error) { return svc.AddExecution(ctx, AddSignRequest{}) }},
		{"DelExecution", func() (R, error) { return svc.DelExecution(ctx, DelSignRequest{}) }},
		{"BackToStepTask", func() (R, error) { return svc.BackToStepTask(ctx, nil) }},
		{"BackTask", func() (R, error) { return svc.BackTask(ctx, nil) }},
		{"CompleteToTargetActivity", func() (R, error) { return svc.CompleteToTargetActivity(ctx, nil) }},
		{"GetNextUserTaskAndUserList", func() (R, error) { return svc.GetNextUserTaskAndUserList(ctx, "task-1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("门面防御分支不应 panic: %v", recovered)
				}
			}()
			_, err := tc.call()
			if err == nil {
				t.Fatalf("能力未装配时应返回错误")
			}
			var apiErr *api.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("应返回 api.Error，实际 %T", err)
			}
		})
	}
}

// TestNewScheduler_NilDependencies 验证调度器在依赖缺失时的安全构造。
// @param t 测试上下文
func TestNewScheduler_NilDependencies(t *testing.T) {
	scheduler := NewScheduler(SchedulerConfig{Options: Options{}, TodoSource: emptyTodoSource{}})
	if scheduler == nil {
		t.Fatalf("调度器应可空依赖构造")
	}
	// 未启动时停止应为空操作
	scheduler.Stop()
}

// TestEmptyTodoSource 覆盖 SQL 模式待办占位源的全部方法。
// @param t 测试上下文
func TestEmptyTodoSource(t *testing.T) {
	ctx := context.Background()
	source := emptyTodoSource{}
	rows, err := source.ListAll(ctx)
	if len(rows) != 0 || err != nil {
		t.Fatalf("空源列表应为空且无错误: %d %v", len(rows), err)
	}
	if _, ok, err := source.GetByTaskID(ctx, "task-1"); ok || err != nil {
		t.Fatalf("空源按任务查询应未命中且无错误")
	}
}

// TestAssembleCapabilities_NilInputs 验证能力装配对空输入的容错。
// @param t 测试上下文
func TestAssembleCapabilities_NilInputs(t *testing.T) {
	fakeClock := clock.NewFake(completeFixtureTime)
	// 空组件与空引擎：不应 panic
	assembleCapabilities(nil, nil, capabilityStores{}, Options{}, fakeClock)
	assembleCapabilities(&components{}, nil, capabilityStores{}, Options{}, fakeClock)
}
