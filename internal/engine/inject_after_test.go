package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	bpmnxml "github.com/DengQingNian/Flowtide/internal/bpmn/xml"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestInjectUserTaskAfter 验证新节点在原任务办结后才生成，派生定义不改变其他或新发起实例。
// @param t 测试上下文
// @return 无
func TestInjectUserTaskAfter(t *testing.T) {
	e := newTestEngine(t)
	deployXML(t, e, changeStateXML)
	ctx := context.Background()
	first, _ := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "changeState"})
	other, _ := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "changeState"})
	target, err := e.InjectUserTaskAfter(ctx, first.ID, api.InsertUserTaskRequest{AfterActivityID: "taskA", ID: "extra", Name: "复核", Assignee: "bob", ProcessDefinitionID: first.ProcessDefinitionID})
	if err != nil {
		t.Fatal(err)
	}
	if target.DerivedFrom != first.ProcessDefinitionID || target.DerivedFromRoot != first.ProcessDefinitionID || target.DerivedVersion != 1 {
		t.Fatalf("派生信息错误：%+v", target)
	}
	page, _ := e.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: first.ID, Size: 10})
	if len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "taskA" {
		t.Fatalf("不应立即生成新任务：%+v", page.Data)
	}
	if err := e.Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	page, _ = e.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: first.ID, Size: 10})
	if len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != "extra" || page.Data[0].Assignee != "bob" {
		t.Fatalf("原节点后应进入新增任务：%+v", page.Data)
	}
	if err := e.Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{UserID: "bob"}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.GetProcessInstance(ctx, first.ID)
	if !got.Ended {
		t.Fatal("新增节点办结后应沿原出线结束")
	}
	got, _ = e.GetProcessInstance(ctx, other.ID)
	if got.ProcessDefinitionID != first.ProcessDefinitionID {
		t.Fatal("其他实例定义被修改")
	}
	fresh, err := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "changeState"})
	if err != nil || fresh.ProcessDefinitionID != first.ProcessDefinitionID {
		t.Fatalf("新发起实例误用派生定义：%+v %v", fresh, err)
	}
	if _, err := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionID: target.ID}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("不能直接用派生定义发起：%v", err)
	}
	if err := e.MigrateProcessInstance(ctx, api.MigrationRequest{ProcessInstanceID: other.ID, TargetProcessDefinitionID: target.ID}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("其他实例不能迁入派生定义：%v", err)
	}
	second, err := e.InjectUserTaskAfter(ctx, other.ID, api.InsertUserTaskRequest{AfterActivityID: "taskA", Name: "第二个实例复核", Assignee: "bob"})
	if err != nil || second.DerivedVersion != 2 {
		t.Fatalf("同版本实例派生序号冲突：%+v %v", second, err)
	}
	// 派生元数据和XML通过标准ACT行往返后仍能恢复，最新定义选择不受影响。
	rows, err := e.ExportRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestEngine(t)
	if err = restored.ImportRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	runtimeRows, err := e.ExportRuntimeRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = restored.ImportRuntimeRows(ctx, runtimeRows); err != nil {
		t.Fatal(err)
	}
	page, _ = restored.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: other.ID, Size: 10})
	if len(page.Data) != 1 || page.Data[0].ProcessDefinitionID != second.ID {
		t.Fatalf("恢复后未保留运行任务定义：%+v", page.Data)
	}
	if err = restored.Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	page, _ = restored.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: other.ID, Size: 10})
	if len(page.Data) != 1 || page.Data[0].Name != "第二个实例复核" {
		t.Fatalf("恢复后流转错误：%+v", page.Data)
	}
	definition, err := restored.GetProcessDefinition(ctx, target.ID)
	if err != nil || definition.DerivedFrom != target.DerivedFrom {
		t.Fatalf("派生定义恢复失败：%+v %v", definition, err)
	}
	fresh, err = restored.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "changeState"})
	if err != nil || fresh.ProcessDefinitionID != first.ProcessDefinitionID {
		t.Fatalf("恢复后错误选择派生定义：%+v %v", fresh, err)
	}
}

// TestInsertAfterXMLKeepsScopeAndConditions 验证嵌套节点的原条件、命名空间及办理人配置保留。
// @param t 测试上下文
// @return 无
func TestInsertAfterXMLKeepsScopeAndConditions(t *testing.T) {
	const original = `<b:definitions xmlns:b="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><b:process id="main" isExecutable="true"><b:startEvent id="rootStart"/><b:sequenceFlow id="rootIn" sourceRef="rootStart" targetRef="sub"/><b:subProcess id="sub"><b:startEvent id="s"/><b:sequenceFlow id="in" sourceRef="s" targetRef="task"/><b:userTask id="task" name="原节点" custom="保留"><b:incoming>in</b:incoming><b:outgoing>branch</b:outgoing></b:userTask><b:sequenceFlow id="branch" sourceRef="task" targetRef="end" name="判断"><b:conditionExpression xsi:type="b:tFormalExpression">${amount &gt; 1}</b:conditionExpression></b:sequenceFlow><b:endEvent id="end"/></b:subProcess><b:sequenceFlow id="rootOut" sourceRef="sub" targetRef="rootEnd"/><b:endEvent id="rootEnd"/></b:process></b:definitions>`
	req := api.InsertUserTaskRequest{AfterActivityID: "task", ID: "inserted", Name: "加 & 审", Assignee: "next"}
	data, err := insertAfterXML([]byte(original), "main", req, "newFlow")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `xsi:type="b:tFormalExpression"`) || !strings.Contains(string(data), `xmlns:b="http://www.omg.org/spec/BPMN/20100524/MODEL"`) {
		t.Fatalf("限定名前缀丢失：%s", data)
	}
	parsed, err := bpmnxml.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	process := parsed.Processes[0]
	inserted, ok := findElement(process, "inserted")
	if !ok {
		t.Fatal("新增节点未留在子流程")
	}
	task := inserted.(*model.UserTask)
	if task.Assignee != "next" || task.Name != "加 & 审" || task.Extensions.Attr("custom") != "保留" {
		t.Fatalf("节点属性错误：%+v", task)
	}
	element, _ := findElement(process, "branch")
	branch := element.(*model.SequenceFlow)
	if branch.SourceRef != "inserted" || branch.TargetRef != "end" || branch.Condition != "${amount > 1}" {
		t.Fatalf("原条件出线错误：%+v", branch)
	}
	link, _ := findElement(process, "newFlow")
	if flow := link.(*model.SequenceFlow); flow.SourceRef != "task" || flow.TargetRef != "inserted" || flow.Condition != "" {
		t.Fatalf("新连接错误：%+v", flow)
	}
	for _, tc := range []struct {
		xml, key, id string
		want         error
	}{
		{original, "other", "inserted", api.ErrNotFound}, {original, "main", "invalid id", api.ErrBadRequest}, {original, "main", "newFlow", api.ErrBadRequest}, {"<broken>", "main", "inserted", nil},
	} {
		req.ID = tc.id
		if _, err := insertAfterXML([]byte(tc.xml), tc.key, req, "newFlow"); err == nil || tc.want != nil && !errors.Is(err, tc.want) {
			t.Fatalf("XML非法输入错误：%v", err)
		}
	}
}

// TestInjectUserTaskAfterRollback 验证持久化失败不会残留派生定义或迁移原实例引用。
// @param t 测试上下文
// @return 无
func TestInjectUserTaskAfterRollback(t *testing.T) {
	e := newTestEngine(t)
	deployXML(t, e, changeStateXML)
	ctx := context.Background()
	instance, err := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "changeState"})
	if err != nil {
		t.Fatal(err)
	}
	e.cfg.StateStore = asyncSaveFailureStore{}
	_, err = e.InjectUserTaskAfter(ctx, instance.ID, api.InsertUserTaskRequest{AfterActivityID: "taskA", Name: "复核", Assignee: "bob"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("未透传持久化失败：%v", err)
	}
	if len(e.state.Definitions) != 1 || len(e.state.Deployments) != 1 || e.state.Instances[instance.ID].ProcessDefinitionID != instance.ProcessDefinitionID {
		t.Fatal("注入失败留下部分写入")
	}
}

// TestInjectUserTaskAfterUsesDefaultFlow 验证移到新节点后的默认出线只在条件出线未命中时使用。
// @param t 测试上下文
// @return 无
func TestInjectUserTaskAfterUsesDefaultFlow(t *testing.T) {
	const xml = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:f="http://flowable.org/bpmn"><process id="conditional" isExecutable="true"><startEvent id="start"/><sequenceFlow id="in" sourceRef="start" targetRef="approve"/><userTask id="approve" default="fallback" f:assignee="a"/><sequenceFlow id="matched" sourceRef="approve" targetRef="yes"><conditionExpression>${accepted}</conditionExpression></sequenceFlow><sequenceFlow id="fallback" sourceRef="approve" targetRef="no"/><userTask id="yes"/><userTask id="no"/></process></definitions>`
	e := newTestEngine(t)
	deployXML(t, e, xml)
	for _, tc := range []struct {
		accepted bool
		want     string
	}{{true, "yes"}, {false, "no"}} {
		ctx := t.Context()
		instance, err := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "conditional", Variables: []api.Variable{{Name: "accepted", Value: tc.accepted}}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.InjectUserTaskAfter(ctx, instance.ID, api.InsertUserTaskRequest{AfterActivityID: "approve", ID: "extra", Name: "复核", Assignee: "b"})
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range []struct{ key, user string }{{"approve", "a"}, {"extra", "b"}} {
			page, err := e.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
			if err != nil || len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != step.key {
				t.Fatalf("顺序节点错误：%+v %v", page, err)
			}
			if err := e.Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{UserID: step.user}); err != nil {
				t.Fatal(err)
			}
		}
		page, err := e.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID, Size: 10})
		if err != nil || len(page.Data) != 1 || page.Data[0].TaskDefinitionKey != tc.want {
			t.Fatalf("accepted=%v 应仅选择 %s：%+v %v", tc.accepted, tc.want, page, err)
		}
	}
}

// TestInjectUserTaskAfterValidation 验证非法节点、租户、参数和挂起实例不产生部分派生数据。
// @param t 测试上下文
// @return 无
func TestInjectUserTaskAfterValidation(t *testing.T) {
	e := newTestEngine(t)
	deployXML(t, e, changeStateXML)
	ctx := context.Background()
	instance, _ := e.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "changeState"})
	valid := api.InsertUserTaskRequest{AfterActivityID: "taskA", ID: "extra", Name: "复核", Assignee: "bob"}
	for _, tc := range []struct {
		name, id string
		req      api.InsertUserTaskRequest
		want     error
	}{
		{"实例不存在", "missing", valid, api.ErrNotFound},
		{"节点不存在", instance.ID, api.InsertUserTaskRequest{AfterActivityID: "missing", Name: "复核", Assignee: "bob"}, api.ErrNotFound},
		{"不是用户任务", instance.ID, api.InsertUserTaskRequest{AfterActivityID: "e", Name: "复核", Assignee: "bob"}, api.ErrBadRequest},
		{"名称空白", instance.ID, api.InsertUserTaskRequest{AfterActivityID: "taskA", Name: " "}, api.ErrBadRequest},
		{"节点ID重复", instance.ID, api.InsertUserTaskRequest{AfterActivityID: "taskA", ID: "taskA", Name: "复核", Assignee: "bob"}, api.ErrConflict},
		{"定义不匹配", instance.ID, api.InsertUserTaskRequest{AfterActivityID: "taskA", Name: "复核", Assignee: "bob", ProcessDefinitionID: "wrong"}, api.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.InjectUserTaskAfter(ctx, tc.id, tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("预期%v，实际%v", tc.want, err)
			}
			if len(e.state.Definitions) != 1 || len(e.state.Deployments) != 1 {
				t.Fatal("校验失败残留派生数据")
			}
		})
	}
	if _, err := e.InjectUserTaskAfter(tenant.With(ctx, "other"), instance.ID, valid); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("跨租户应拒绝：%v", err)
	}
	if err := e.SuspendProcessInstance(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.InjectUserTaskAfter(ctx, instance.ID, valid); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("挂起实例应拒绝：%v", err)
	}
}
