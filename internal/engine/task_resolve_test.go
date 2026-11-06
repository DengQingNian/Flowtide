package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// resolveBPMN 是单用户任务的最小流程模型（apply 节点处理人 11）。
const resolveBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="resolve" isExecutable="true"><startEvent id="start"/><sequenceFlow id="one" sourceRef="start" targetRef="apply"/><userTask id="apply" flowable:assignee="11"/><sequenceFlow id="two" sourceRef="apply" targetRef="end"/><endEvent id="end"/></process></definitions>`

// newResolveTestEngine 构造内存引擎并部署单用户任务模型。
// @param t 测试上下文
// @return 引擎
// @return 租户请求上下文
func newResolveTestEngine(t *testing.T) (*Engine, context.Context) {
	t.Helper()
	ctx := tenant.With(context.Background(), "flow")
	eng, err := New(Config{TenantID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RepositoryService().Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"resolve.bpmn20.xml": []byte(resolveBPMN)}}); err != nil {
		t.Fatal(err)
	}
	return eng, ctx
}

// startResolveTask 发起实例并返回 apply 节点的任务 ID。
// @param t 测试上下文
// @param eng 引擎
// @param ctx 请求上下文
// @return 任务 ID
// @return 实例 ID
func startResolveTask(t *testing.T, eng *Engine, ctx context.Context) (string, string) {
	t.Helper()
	instance, err := eng.RuntimeService().StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "resolve"})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks.Data) != 1 {
		t.Fatalf("实例应有 1 个待办，实际 %d", len(tasks.Data))
	}
	return tasks.Data[0].ID, instance.ID
}

// TestResolveWithVariablesAppliesLocalVariables 验证归还委派时同步写入任务局部变量。
// 场景：任务委派给 22 后按 Java resolveTask(taskId, variables) 语义归还并带变量；
// 预期：局部变量可读、委派状态 RESOLVED、处理人还原为所有者 11。
// @param t 测试上下文
func TestResolveWithVariablesAppliesLocalVariables(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	taskID, _ := startResolveTask(t, eng, ctx)
	if err := eng.TaskService().Delegate(ctx, taskID, "22"); err != nil {
		t.Fatal(err)
	}
	variables := []api.Variable{{Name: "score", Value: 42}, {Name: "remark", Value: "已核实"}}
	if err := eng.TaskService().ResolveWithVariables(ctx, taskID, "", variables); err != nil {
		t.Fatal(err)
	}
	score, err := eng.TaskService().GetTaskVariable(ctx, taskID, "score")
	if err != nil {
		t.Fatalf("局部变量应可读：%v", err)
	}
	if score != 42 {
		t.Fatalf("score 应为 42，实际 %v", score)
	}
	remark, err := eng.TaskService().GetTaskVariable(ctx, taskID, "remark")
	if err != nil || remark != "已核实" {
		t.Fatalf("remark 应为 已核实，实际 %v err=%v", remark, err)
	}
	task, err := eng.TaskService().GetTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.DelegationState != "RESOLVED" {
		t.Fatalf("委派状态应为 RESOLVED，实际 %s", task.DelegationState)
	}
	if task.Assignee != "11" || task.Owner != "11" {
		t.Fatalf("处理人应还原为所有者 11，实际 assignee=%s owner=%s", task.Assignee, task.Owner)
	}
}

// TestResolveWithVariablesRejectsNonOwner 验证非所有者归还被拒绝。
// 场景：userId 显式给 33（既非所有者）；
// 预期：返回 ENGINE-0813 权限错误，任务保持 PENDING。
// @param t 测试上下文
func TestResolveWithVariablesRejectsNonOwner(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	taskID, _ := startResolveTask(t, eng, ctx)
	if err := eng.TaskService().Delegate(ctx, taskID, "22"); err != nil {
		t.Fatal(err)
	}
	err := eng.TaskService().ResolveWithVariables(ctx, taskID, "33", []api.Variable{{Name: "score", Value: 1}})
	if err == nil || !strings.Contains(err.Error(), "只有任务所有者可以处理委派") {
		t.Fatalf("应拒绝非所有者归还，实际 err=%v", err)
	}
	task, err2 := eng.TaskService().GetTask(ctx, taskID)
	if err2 != nil {
		t.Fatal(err2)
	}
	if task.DelegationState != "PENDING" {
		t.Fatalf("归还失败后委派状态应保持 PENDING，实际 %s", task.DelegationState)
	}
	if _, err := eng.TaskService().GetTaskVariable(ctx, taskID, "score"); err == nil {
		t.Fatal("归还失败时不应写入局部变量")
	}
}

// TestResolveWithVariablesRejectsNotPending 验证未委派任务不能归还。
// 场景：任务未委派直接调 ResolveWithVariables；
// 预期：返回 ENGINE-0811 冲突错误。
// @param t 测试上下文
func TestResolveWithVariablesRejectsNotPending(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	taskID, _ := startResolveTask(t, eng, ctx)
	err := eng.TaskService().ResolveWithVariables(ctx, taskID, "", []api.Variable{{Name: "score", Value: 1}})
	if err == nil || !strings.Contains(err.Error(), "任务未处于委派状态") {
		t.Fatalf("未委派任务应拒绝归还，实际 err=%v", err)
	}
}

// TestResolveWithVariablesTaskNotFound 验证任务不存在时返回明确错误。
// 场景：taskID 不存在；
// 预期：返回 ENGINE-0805 任务不存在。
// @param t 测试上下文
func TestResolveWithVariablesTaskNotFound(t *testing.T) {
	eng, ctx := newResolveTestEngine(t)
	err := eng.TaskService().ResolveWithVariables(ctx, "missing", "", nil)
	if err == nil || !strings.Contains(err.Error(), "任务不存在") {
		t.Fatalf("应返回任务不存在，实际 err=%v", err)
	}
}
