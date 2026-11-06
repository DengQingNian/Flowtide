package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// moveBPMN 是两连用户任务的最小流程模型（apply 处理人 11 → review 处理人 22）。
const moveBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="move" isExecutable="true"><startEvent id="start"/><sequenceFlow id="one" sourceRef="start" targetRef="apply"/><userTask id="apply" flowable:assignee="11"/><sequenceFlow id="two" sourceRef="apply" targetRef="review"/><userTask id="review" flowable:assignee="22"/><sequenceFlow id="three" sourceRef="review" targetRef="end"/><endEvent id="end"/></process></definitions>`

// newMoveTestEngine 构造内存引擎并部署两连用户任务模型，发起实例并返回实例 Id。
// @param t 测试上下文
// @return 引擎
// @return 请求上下文
// @return 实例 ID
func newMoveTestEngine(t *testing.T) (*Engine, context.Context, string) {
	t.Helper()
	ctx := tenant.With(context.Background(), "flow")
	eng, err := New(Config{TenantID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RepositoryService().Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"move.bpmn20.xml": []byte(moveBPMN)}}); err != nil {
		t.Fatal(err)
	}
	instance, err := eng.RuntimeService().StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "move"})
	if err != nil {
		t.Fatal(err)
	}
	return eng, ctx, instance.ID
}

// tasksOfInstance 返回实例当前待办节点集合。
// @param t 测试上下文
// @param eng 引擎
// @param ctx 请求上下文
// @param instanceID 实例 ID
// @return 待办任务定义 Key 集合
func tasksOfInstance(t *testing.T, eng *Engine, ctx context.Context, instanceID string) map[string]bool {
	t.Helper()
	page, err := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instanceID})
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, task := range page.Data {
		keys[task.TaskDefinitionKey] = true
	}
	return keys
}

// TestDoMoveActivitySerial 验证普通节点跳转（串行场景，对位 Java moveActivityIdTo）。
// 场景：实例在 apply 节点，跳转到 review；
// 预期：apply 待办取消、出现 review 待办。
// @param t 测试上下文
func TestDoMoveActivitySerial(t *testing.T) {
	eng, ctx, instanceID := newMoveTestEngine(t)
	if err := eng.RuntimeService().DoMoveActivity(ctx, instanceID, "apply", "review"); err != nil {
		t.Fatal(err)
	}
	keys := tasksOfInstance(t, eng, ctx, instanceID)
	if keys["apply"] || !keys["review"] {
		t.Fatalf("跳转后应只有 review 待办，实际 %v", keys)
	}
}

// TestDoMoveActivitySuspended 验证挂起实例拒绝跳转（对位 Java 流程已暂停）。
// @param t 测试上下文
func TestDoMoveActivitySuspended(t *testing.T) {
	eng, ctx, instanceID := newMoveTestEngine(t)
	if err := eng.RuntimeService().SuspendProcessInstance(ctx, instanceID); err != nil {
		t.Fatal(err)
	}
	err := eng.RuntimeService().DoMoveActivity(ctx, instanceID, "apply", "review")
	if err == nil || !strings.Contains(err.Error(), "流程实例已挂起") {
		t.Fatalf("挂起实例应拒绝跳转，实际 err=%v", err)
	}
}

// TestDoMoveActivityMissingSource 验证来源活动无可迁移执行时拒绝。
// @param t 测试上下文
func TestDoMoveActivityMissingSource(t *testing.T) {
	eng, ctx, instanceID := newMoveTestEngine(t)
	err := eng.RuntimeService().DoMoveActivity(ctx, instanceID, "review", "apply")
	if err == nil || !strings.Contains(err.Error(), "来源活动没有可迁移执行") {
		t.Fatalf("应拒绝无执行来源活动，实际 err=%v", err)
	}
}

// TestDoMoveExecution 验证执行迁移（对位 Java moveExecutionToActivityId，无来源节点语义）。
// 场景：实例在 apply 节点，执行迁移到 review；
// 预期：apply 待办取消、出现 review 待办。
// @param t 测试上下文
func TestDoMoveExecution(t *testing.T) {
	eng, ctx, instanceID := newMoveTestEngine(t)
	if err := eng.RuntimeService().DoMoveExecution(ctx, instanceID, "review"); err != nil {
		t.Fatal(err)
	}
	keys := tasksOfInstance(t, eng, ctx, instanceID)
	if keys["apply"] || !keys["review"] {
		t.Fatalf("迁移后应只有 review 待办，实际 %v", keys)
	}
}
