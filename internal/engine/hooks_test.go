package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// taskHooksXML 两级用户任务语料：task1(alice) → task2(bob)。
const taskHooksXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="hookProcess" isExecutable="true">
    <startEvent id="s"/>
    <sequenceFlow id="f1" sourceRef="s" targetRef="task1"/>
    <userTask id="task1" name="First" flowable:assignee="alice"/>
    <sequenceFlow id="f2" sourceRef="task1" targetRef="task2"/>
    <userTask id="task2" name="Second" flowable:assignee="bob"/>
    <sequenceFlow id="f3" sourceRef="task2" targetRef="e"/>
    <endEvent id="e"/>
  </process>
</definitions>`

// hookCtxKey 延迟动作上下文校验键。
type hookCtxKey struct{}

// TestTaskHooks_InvokeOrder 验证创建/完成钩子均按注册顺序同步触发，且快照改写无效。
// @param t 测试上下文
func TestTaskHooks_InvokeOrder(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	var createdOrder, completedOrder []string
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		createdOrder = append(createdOrder, "first")
		// 快照改写必须无效，改写需经 ctl
		task.Assignee = "mallory"
		return nil
	})
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		createdOrder = append(createdOrder, "second")
		return nil
	})
	engine.AddTaskCompletedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		completedOrder = append(completedOrder, "first")
		return nil
	})
	engine.AddTaskCompletedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		completedOrder = append(completedOrder, "second")
		return nil
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(createdOrder) != 2 || createdOrder[0] != "first" || createdOrder[1] != "second" {
		t.Fatalf("创建钩子应按注册顺序触发: %v", createdOrder)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "task1" {
		t.Fatalf("任务状态错误: %+v", tasks)
	}
	// 快照改写无效：办理人仍为 alice
	if tasks[0].Assignee != "alice" {
		t.Fatalf("快照改写不应生效: %s", tasks[0].Assignee)
	}
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	if len(completedOrder) != 2 || completedOrder[0] != "first" || completedOrder[1] != "second" {
		t.Fatalf("完成钩子应按注册顺序触发: %v", completedOrder)
	}
}

// TestTaskHook_RollbackOnError 验证钩子失败使命令失败回滚（含钩子自身的改写一并失效）。
// @param t 测试上下文
func TestTaskHook_RollbackOnError(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		// 先写入再失败，证明失败后全部回滚
		if err := ctl.SetVariable("hookVar", "v1"); err != nil {
			return err
		}
		return errors.New("created hook boom")
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"}); err == nil {
		t.Fatalf("创建钩子失败应使命令失败")
	}
	if instances, _ := engine.ListProcessInstances(context.Background()); len(instances) != 0 {
		t.Fatalf("失败后应回滚流程实例: %+v", instances)
	}
	if tasks := listTasks(t, engine); len(tasks) != 0 {
		t.Fatalf("失败后应回滚任务: %+v", tasks)
	}

	// 控制句柄操作不存在的任务返回未找到错误并使命令失败
	engine2 := newTestEngine(t)
	deployXML(t, engine2, taskHooksXML)
	engine2.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		return ctl.SetAssignee("not-exists", "u1")
	})
	_, err := engine2.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"})
	if !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("改写不存在任务应返回未找到: %v", err)
	}
	if tasks := listTasks(t, engine2); len(tasks) != 0 {
		t.Fatalf("失败后应回滚任务: %+v", tasks)
	}
}

// TestTaskHookControl_SetAssigneeAndCandidates 验证执行人重写与候选重写在命令内生效。
// @param t 测试上下文
func TestTaskHookControl_SetAssigneeAndCandidates(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		if task.TaskDefinitionKey != "task1" {
			return nil
		}
		// 同值改写应为无操作
		if err := ctl.SetAssignee(task.ID, "alice"); err != nil {
			return err
		}
		if err := ctl.SetAssignee(task.ID, "rewriter"); err != nil {
			return err
		}
		if err := ctl.SetCandidateUsers(task.ID, []string{"u1", "u2"}); err != nil {
			return err
		}
		return ctl.SetCandidateGroups(task.ID, []string{"g1"})
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 {
		t.Fatalf("任务缺失: %+v", tasks)
	}
	got, err := engine.GetTask(context.Background(), tasks[0].ID)
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	if got.Assignee != "rewriter" {
		t.Fatalf("办理人重写未生效: %s", got.Assignee)
	}
	if len(got.CandidateUsers) != 2 || got.CandidateUsers[0] != "u1" || got.CandidateUsers[1] != "u2" {
		t.Fatalf("候选用户重写未生效: %+v", got.CandidateUsers)
	}
	if len(got.CandidateGroups) != 1 || got.CandidateGroups[0] != "g1" {
		t.Fatalf("候选组重写未生效: %+v", got.CandidateGroups)
	}
}

// TestTaskHookControl_SetVariable 验证创建/完成钩子内设置流程实例变量最终生效。
// @param t 测试上下文
func TestTaskHookControl_SetVariable(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		// 完成命令推进到 task2 时创建钩子会再次触发，变量名需要区分事件
		return ctl.SetVariable("createdVar", "created")
	})
	engine.AddTaskCompletedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		// 完成钩子在历史结束行落库前执行，变量改写应进入提交后的状态
		return ctl.SetVariable("completedVar", "completed")
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	value, err := engine.GetVariable(context.Background(), instance.ID, "createdVar")
	if err != nil || value != "created" {
		t.Fatalf("创建钩子变量未生效: %v %v", value, err)
	}
	tasks := listTasks(t, engine)
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	value, err = engine.GetVariable(context.Background(), instance.ID, "completedVar")
	if err != nil || value != "completed" {
		t.Fatalf("完成钩子变量未生效: %v %v", value, err)
	}
	// 完成命令推进产生的 task2 创建钩子同样生效
	value, err = engine.GetVariable(context.Background(), instance.ID, "createdVar")
	if err != nil || value != "created" {
		t.Fatalf("推进任务的创建钩子变量未生效: %v %v", value, err)
	}
}

// TestTaskHook_DeferRunsAfterCommit 验证延迟动作在命令成功提交后按序执行并携带原上下文。
// @param t 测试上下文
func TestTaskHook_DeferRunsAfterCommit(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	var order []string
	var committed bool
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		ctl.Defer(func(actionCtx context.Context) error {
			// 动作执行时命令已提交：实例已可见
			instances, _ := engine.ListProcessInstances(actionCtx)
			committed = len(instances) == 1
			// 原上下文透传校验
			if actionCtx.Value(hookCtxKey{}) != "outer" {
				return errors.New("延迟动作未收到原上下文")
			}
			order = append(order, "a")
			return nil
		})
		ctl.Defer(func(actionCtx context.Context) error {
			order = append(order, "b")
			return nil
		})
		return nil
	})
	ctx := context.WithValue(context.Background(), hookCtxKey{}, "outer")
	if _, err := engine.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "hookProcess"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if !committed {
		t.Fatalf("延迟动作应在其所在命令提交成功后执行")
	}
	if len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("延迟动作应按登记顺序执行: %v", order)
	}
}

// TestTaskHook_DeferSkippedOnFailure 验证命令失败回滚时不执行延迟动作。
// @param t 测试上下文
func TestTaskHook_DeferSkippedOnFailure(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	deferredRan := false
	engine.AddTaskCompletedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		ctl.Defer(func(actionCtx context.Context) error {
			deferredRan = true
			return nil
		})
		return errors.New("complete hook boom")
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if err := engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err == nil {
		t.Fatalf("完成钩子失败应使命令失败")
	}
	if deferredRan {
		t.Fatalf("失败命令不应执行延迟动作")
	}
	// 回滚后任务仍在，流程未结束
	if len(listTasks(t, engine)) != 1 {
		t.Fatalf("失败后任务应回滚保留")
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if got.Ended {
		t.Fatalf("失败命令不应结束流程")
	}
}

// TestTaskHook_DeferCallsPublicAPI 验证延迟动作内调用引擎公有 API 不死锁且可驱动下一任务。
// @param t 测试上下文
func TestTaskHook_DeferCallsPublicAPI(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	var task2ID string
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		if task.TaskDefinitionKey == "task2" {
			task2ID = task.ID
		}
		return nil
	})
	engine.AddTaskCompletedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		if task.TaskDefinitionKey != "task1" {
			return nil
		}
		// 相邻同人自动办结场景：命令提交后完成下一任务
		ctl.Defer(func(actionCtx context.Context) error {
			return engine.Complete(actionCtx, task2ID, api.CompleteTaskRequest{UserID: "bob"})
		})
		return nil
	})
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	tasks := listTasks(t, engine)
	if len(tasks) != 1 || tasks[0].TaskDefinitionKey != "task1" {
		t.Fatalf("任务状态错误: %+v", tasks)
	}
	// 完成命令带超时守护，死锁时显式失败而不是挂起整个测试
	done := make(chan error, 1)
	go func() {
		done <- engine.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{UserID: "alice"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("完成任务失败: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("完成命令疑似死锁：延迟动作内调用公有 API 未返回")
	}
	// 延迟动作已完成 task2，流程应结束
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("延迟动作完成下一任务后流程应结束")
	}
}

// TestTaskHook_DeferPanicRecovered 验证延迟动作 panic 被恢复且不影响后续动作与命令结果。
// @param t 测试上下文
func TestTaskHook_DeferPanicRecovered(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	secondRan := false
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		ctl.Defer(func(actionCtx context.Context) error {
			panic("deferred boom")
		})
		// 返回错误的动作仅记录日志，不应阻断后续动作
		ctl.Defer(func(actionCtx context.Context) error {
			return errors.New("deferred error")
		})
		ctl.Defer(func(actionCtx context.Context) error {
			secondRan = true
			return nil
		})
		return nil
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"}); err != nil {
		t.Fatalf("延迟动作 panic 不应影响命令结果: %v", err)
	}
	if !secondRan {
		t.Fatalf("panic 后后续延迟动作仍应执行")
	}
}

// TestTaskHookControl_NegativePaths 验证控制句柄对不存在任务与空动作的防御路径。
// @param t 测试上下文
func TestTaskHookControl_NegativePaths(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, taskHooksXML)
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		// 空延迟动作应被忽略，不进入延迟队列
		ctl.Defer(nil)
		if err := ctl.SetCandidateUsers("not-exists", []string{"u1"}); !errors.Is(err, api.ErrNotFound) {
			return err
		}
		if err := ctl.SetCandidateGroups("not-exists", []string{"g1"}); !errors.Is(err, api.ErrNotFound) {
			return err
		}
		return errors.New("negative path boom")
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "hookProcess"}); err == nil {
		t.Fatalf("钩子失败应使命令失败")
	}
	if tasks := listTasks(t, engine); len(tasks) != 0 {
		t.Fatalf("失败后应回滚任务: %+v", tasks)
	}
}

// TestTaskHooks_MultiInstance 验证并行多实例每个实例任务创建都触发创建钩子。
// @param t 测试上下文
func TestTaskHooks_MultiInstance(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, parallelMIXML)
	var assignees []string
	engine.AddTaskCreatedHook(func(ctx context.Context, task *api.Task, ctl api.HookControl) error {
		assignees = append(assignees, task.Assignee)
		return nil
	})
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "miParallel",
		Variables:            []api.Variable{{Name: "approvers", Value: []string{"alice", "bob"}}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if len(assignees) != 2 {
		t.Fatalf("并行多实例应触发 2 次创建钩子，实际 %d: %v", len(assignees), assignees)
	}
	seen := map[string]bool{assignees[0]: true, assignees[1]: true}
	if !seen["alice"] || !seen["bob"] {
		t.Fatalf("钩子快照应覆盖每个实例的处理人: %v", assignees)
	}
}
