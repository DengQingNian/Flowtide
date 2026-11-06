package engine

import (
	"context"

	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// newMITestEngine 构造内存引擎并部署指定模型。
// @param t 测试上下文
// @param key 部署资源名及流程 Key
// @param bpmn 模型 XML
// @return 引擎
// @return 租户请求上下文
func newMITestEngine(t *testing.T, key, bpmn string) (*Engine, context.Context) {
	t.Helper()
	ctx := tenant.With(context.Background(), "flow")
	eng, err := New(Config{TenantID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RepositoryService().Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{key + ".bpmn20.xml": []byte(bpmn)}}); err != nil {
		t.Fatal(err)
	}
	return eng, ctx
}

// miBPMN 是含 3 人并行会签（完成条件比例 0.8）的最小流程模型。
const miBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="mi" isExecutable="true"><startEvent id="start"/><sequenceFlow id="one" sourceRef="start" targetRef="review"/><userTask id="review" flowable:assignee="${member}"><multiInstanceLoopCharacteristics isSequential="false" flowable:elementVariable="member"><loopCardinality>3</loopCardinality><completionCondition>${nrOfCompletedInstances/nrOfInstances &gt;= 0.8}</completionCondition></multiInstanceLoopCharacteristics></userTask><sequenceFlow id="two" sourceRef="review" targetRef="end"/><endEvent id="end"/></process></definitions>`

// plainBPMN 是普通单处理人用户任务模型（无多实例）。
const plainBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="plain" isExecutable="true"><startEvent id="start"/><sequenceFlow id="one" sourceRef="start" targetRef="apply"/><userTask id="apply" flowable:assignee="11"/><sequenceFlow id="two" sourceRef="apply" targetRef="end"/><endEvent id="end"/></process></definitions>`

// miTasks 发起会签实例并返回三个子任务 Id。
// @param t 测试上下文
// @param eng 引擎
// @param ctx 请求上下文
// @return 按创建顺序排列的子任务 ID
func miTasks(t *testing.T, eng *Engine, ctx context.Context) []string {
	t.Helper()
	instance, err := eng.RuntimeService().StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "mi"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 3 {
		t.Fatalf("会签应产生 3 个子任务，实际 %d", len(page.Data))
	}
	ids := make([]string, 0, len(page.Data))
	for _, task := range page.Data {
		ids = append(ids, task.ID)
	}
	return ids
}

// TestIsMultiInstanceLastPersonProportion 验证会签比例口径（含 Java 整数除法特性）。
// 场景：3 人会签、完成条件 (nrOfCompletedInstances/nrOfInstances)>=0.8；
// 预期：0/1 票完成后 (n+1)/3 整数除为 0 → false；2 票完成后 (2+1)/3 整数除为 1 → true
// （0.8 保证引擎在 2/3=0.67 时不会提前结束会签，第 3 人仍是待办成员）。
// @param t 测试上下文
func TestIsMultiInstanceLastPersonProportion(t *testing.T) {
	eng, ctx := newMITestEngine(t, "mi", miBPMN)
	ids := miTasks(t, eng, ctx)
	// 未办结任何一票：completed=0 → (0+1)/3=0 → false
	last, err := eng.TaskService().IsMultiInstanceLastPerson(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if last {
		t.Fatal("0 票完成时不应判定为最后一名")
	}
	// 完成第 1 票：completed=1 → (1+1)/3=0（整数除法）→ false
	if err := eng.TaskService().Complete(ctx, ids[0], api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	last, err = eng.TaskService().IsMultiInstanceLastPerson(ctx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	if last {
		t.Fatal("1/3 完成时按整数除法不应判定为最后一名")
	}
	// 完成第 2 票：completed=2 → (2+1)/3=1 → 1>=0.8 → true
	if err := eng.TaskService().Complete(ctx, ids[1], api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	last, err = eng.TaskService().IsMultiInstanceLastPerson(ctx, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if !last {
		t.Fatal("2/3 完成时 (2+1)/3=1>=0.8 应判定为最后一名")
	}
}

// TestIsMultiInstanceLastPersonPlainTask 验证非会签用户任务恒为 true。
// 场景：普通单处理人用户任务；
// 预期：返回 true（非会签节点当前处理人即最后一个处理人员）。
// @param t 测试上下文
func TestIsMultiInstanceLastPersonPlainTask(t *testing.T) {
	eng, ctx := newMITestEngine(t, "plain", plainBPMN)
	instance, err := eng.RuntimeService().StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "plain"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := eng.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("应有 1 个待办，实际 %d err=%v", len(page.Data), err)
	}
	last, err := eng.TaskService().IsMultiInstanceLastPerson(ctx, page.Data[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !last {
		t.Fatal("非会签用户任务应返回 true")
	}
}

// TestIsMultiInstanceLastPersonTaskMissing 验证任务不存在时返回 false（对位 Java task==null 口径）。
// @param t 测试上下文
func TestIsMultiInstanceLastPersonTaskMissing(t *testing.T) {
	eng, ctx := newMITestEngine(t, "plain", plainBPMN)
	last, err := eng.TaskService().IsMultiInstanceLastPerson(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if last {
		t.Fatal("任务不存在应返回 false")
	}
}

// TestIsMultiInstanceLastPersonNoCondition 验证无完成条件的会签返回 false。
// 场景：多实例用户任务但 completionCondition 为空；
// 预期：返回 false（Java fallthrough 口径）。
// @param t 测试上下文
func TestIsMultiInstanceLastPersonNoCondition(t *testing.T) {
	bpmn := strings.Replace(miBPMN, `<completionCondition>${nrOfCompletedInstances/nrOfInstances &gt;= 0.8}</completionCondition>`, "", 1)
	eng, ctx := newMITestEngine(t, "mi", bpmn)
	ids := miTasks(t, eng, ctx)
	last, err := eng.TaskService().IsMultiInstanceLastPerson(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if last {
		t.Fatal("无完成条件的会签应返回 false")
	}
}
