package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// TestRepositoryQueries 验证部署与定义查询。
// @param t 测试上下文
func TestRepositoryQueries(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definitions, err := engine.ListProcessDefinitions(context.Background())
	if err != nil || len(definitions) != 1 {
		t.Fatalf("定义列表错误: %v %d", err, len(definitions))
	}
	definition := definitions[0]
	if definition.Key != "simple" || definition.Version != 1 {
		t.Fatalf("定义属性错误: %+v", definition)
	}
	byID, err := engine.GetProcessDefinition(context.Background(), definition.ID)
	if err != nil || byID.ID != definition.ID {
		t.Fatalf("按 ID 查询失败: %v", err)
	}
	byKey, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil || byKey.ID != definition.ID {
		t.Fatalf("按 Key 查询失败: %v", err)
	}
	bpmnModel, err := engine.GetBpmnModel(context.Background(), definition.ID)
	if err != nil || len(bpmnModel.Processes) != 1 {
		t.Fatalf("模型查询失败: %v", err)
	}
	xmlBytes, err := engine.GetProcessDefinitionXML(context.Background(), definition.ID)
	if err != nil || len(xmlBytes) == 0 {
		t.Fatalf("XML 查询失败: %v", err)
	}
	if _, err := engine.GetProcessDefinition(context.Background(), "missing"); err == nil {
		t.Fatalf("期望查询不存在定义失败")
	}
	if _, err := engine.GetProcessDefinitionByKey(context.Background(), "missing"); err == nil {
		t.Fatalf("期望查询不存在 Key 失败")
	}
	if _, err := engine.GetBpmnModel(context.Background(), "missing"); err == nil {
		t.Fatalf("期望查询不存在模型失败")
	}
	if _, err := engine.GetProcessDefinitionXML(context.Background(), "missing"); err == nil {
		t.Fatalf("期望查询不存在 XML 失败")
	}
}

// TestDeploy_Errors 验证部署错误。
// @param t 测试上下文
func TestDeploy_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{}); err == nil {
		t.Fatalf("期望空资源部署失败")
	}
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{Resources: map[string][]byte{"bad.bpmn20.xml": []byte("<bad>")}}); err == nil {
		t.Fatalf("期望非法 XML 部署失败")
	}
}

// TestStart_Errors 验证启动错误。
// @param t 测试上下文
func TestStart_Errors(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{}); err == nil {
		t.Fatalf("期望缺少 Key 失败")
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "missing"}); err == nil {
		t.Fatalf("期望未知 Key 失败")
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: "missing"}); err == nil {
		t.Fatalf("期望未知 ID 失败")
	}
}

// TestStartByDefinitionID 验证按定义 ID 启动。
// @param t 测试上下文
func TestStartByDefinitionID(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	definition, err := engine.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionID: definition.ID,
		Variables:           []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil || instance.ProcessDefinitionID != definition.ID {
		t.Fatalf("按 ID 启动失败: %v", err)
	}
}

// TestDeleteProcessInstance 验证删除实例。
// @param t 测试上下文
func TestDeleteProcessInstance(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if err := engine.DeleteProcessInstance(context.Background(), instance.ID, "测试删除"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	got, _ := engine.GetProcessInstance(context.Background(), instance.ID)
	if !got.Ended {
		t.Fatalf("删除后实例应结束")
	}
	if len(listTasks(t, engine)) != 0 {
		t.Fatalf("删除后任务应清空")
	}
	if err := engine.DeleteProcessInstance(context.Background(), "missing", ""); err == nil {
		t.Fatalf("期望删除不存在实例失败")
	}
}

// TestDelegateTask 验证任务委派。
// @param t 测试上下文
func TestDelegateTask(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="delegate" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Task" flowable:candidateUsers="alice" xmlns:flowable="http://flowable.org/bpmn"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "delegate"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.Delegate(context.Background(), task.ID, ""); err == nil {
		t.Fatalf("空处理人不应允许委派")
	}
	if err := engine.Claim(context.Background(), task.ID, "alice"); err != nil {
		t.Fatalf("签收失败: %v", err)
	}
	if err := engine.Delegate(context.Background(), task.ID, "bob"); err != nil {
		t.Fatalf("委派失败: %v", err)
	}
	got, _ := engine.GetTask(context.Background(), task.ID)
	if got.Owner != "alice" || got.Assignee != "bob" {
		t.Fatalf("委派状态错误: %+v", got)
	}
}

// TestTask_Filters 验证任务过滤。
// @param t 测试上下文
func TestTask_Filters(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="filters" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Task" flowable:candidateUsers="alice,bob" xmlns:flowable="http://flowable.org/bpmn"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "filters"})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	page, err := engine.ListTasks(context.Background(), api.TaskFilter{CandidateUser: "alice"})
	if err != nil || page.Total != 1 {
		t.Fatalf("候选人过滤失败: %v %+v", err, page)
	}
	page, _ = engine.ListTasks(context.Background(), api.TaskFilter{CandidateUser: "carol"})
	if page.Total != 0 {
		t.Fatalf("未知候选人不应匹配")
	}
	page, _ = engine.ListTasks(context.Background(), api.TaskFilter{ProcessInstanceID: instance.ID})
	if page.Total != 1 {
		t.Fatalf("实例过滤失败")
	}
	page, _ = engine.ListTasks(context.Background(), api.TaskFilter{ProcessDefinitionKey: "filters"})
	if page.Total != 1 {
		t.Fatalf("定义 Key 过滤失败")
	}
	page, _ = engine.ListTasks(context.Background(), api.TaskFilter{ProcessDefinitionKey: "other"})
	if page.Total != 0 {
		t.Fatalf("错误定义 Key 不应匹配")
	}
	page, _ = engine.ListTasks(context.Background(), api.TaskFilter{Start: 10, Size: 5})
	if len(page.Data) != 0 || page.Total != 1 {
		t.Fatalf("分页越界处理错误: %+v", page)
	}
}

// TestHistoryQueries 验证历史查询。
// @param t 测试上下文
func TestHistoryQueries(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	historicProcesses, err := engine.ListHistoricProcessInstances(context.Background())
	if err != nil || len(historicProcesses) != 1 {
		t.Fatalf("历史流程实例错误: %v %d", err, len(historicProcesses))
	}
	historicTasks, err := engine.ListHistoricTaskInstances(context.Background(), instance.ID)
	if err != nil || len(historicTasks) != 1 {
		t.Fatalf("历史任务错误: %v %d", err, len(historicTasks))
	}
	activities, err := engine.ListHistoricActivityInstances(context.Background(), instance.ID)
	// 原版把顺序流也记为历史活动：开始、顺序流、用户任务。
	if err != nil || len(activities) != 3 {
		t.Fatalf("历史活动错误: %v %d", err, len(activities))
	}
}

// TestHistoryNone 验证关闭历史。
// @param t 测试上下文
func TestHistoryNone(t *testing.T) {
	engine, err := New(Config{HistoryLevel: HistoryNone, Clock: clock.NewFake(time.Now())})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if items, _ := engine.ListHistoricProcessInstances(context.Background()); len(items) != 0 {
		t.Fatalf("关闭历史后不应有记录")
	}
}

// TestEngineClose 验证关闭后拒绝操作。
// @param t 测试上下文
func TestEngineClose(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "x"}); err == nil {
		t.Fatalf("关闭后应拒绝启动")
	}
	if _, err := engine.GetProcessInstance(context.Background(), "x"); err == nil {
		t.Fatalf("关闭后应拒绝查询")
	}
}

// TestEngineInvalidHistory 验证非法历史级别。
// @param t 测试上下文
func TestEngineInvalidHistory(t *testing.T) {
	if _, err := New(Config{HistoryLevel: "invalid"}); err == nil {
		t.Fatalf("期望非法历史级别失败")
	}
}

// TestExecutionAccessors 验证委托执行上下文。
// @param t 测试上下文
func TestExecutionAccessors(t *testing.T) {
	engine := newTestEngine(t)
	localExecutionID := ""
	engine.RegisterDelegate("inspect", func(ctx context.Context, execution *Execution) error {
		if execution.ProcessInstanceID() == "" || execution.ActivityID() != "svc" {
			return api.NewError("TEST", "执行上下文错误", nil)
		}
		if _, ok := execution.GetVariable("seed"); !ok {
			return api.NewError("TEST", "未读取到 seed", nil)
		}
		localExecutionID = execution.state.ID
		execution.SetVariableLocal("local", "v")
		execution.SetVariable("global", "g")
		return nil
	})
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="inspect" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="svc"/><serviceTask id="svc" flowable:delegateExpression="${inspect}" xmlns:flowable="http://flowable.org/bpmn"/><sequenceFlow id="f2" sourceRef="svc" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "inspect",
		Variables:            []api.Variable{{Name: "seed", Value: "x"}},
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	variables, _ := engine.GetVariables(context.Background(), instance.ID)
	if variables["local"] != nil || variables["global"] != "g" {
		t.Fatalf("委托变量写入错误: %+v", variables)
	}
	local, err := engine.GetExecutionVariables(context.Background(), localExecutionID)
	if err != nil || local["local"] != "v" {
		t.Fatalf("活动局部变量丢失: %+v %v", local, err)
	}
}

// TestScriptTask_JavaScriptExpression 验证 JS 表达式子集。
// @param t 测试上下文
func TestScriptTask_JavaScriptExpression(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="script" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="script"/><scriptTask id="script" scriptFormat="javascript"><script>1+1</script></scriptTask><sequenceFlow id="f2" sourceRef="script" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "script"}); err != nil {
		t.Fatalf("JS 表达式脚本失败: %v", err)
	}
}

// TestTerminateEndEvent 验证终止结束事件。
// @param t 测试上下文
func TestTerminateEndEvent(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="terminate" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="end"/><endEvent id="end"><terminateEventDefinition/></endEvent></process></definitions>`
	deployXML(t, engine, xml)
	instance, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "terminate"})
	if err != nil || !instance.Ended {
		t.Fatalf("终止结束事件失败: %v %+v", err, instance)
	}
}

// TestErrorBranches 验证部分错误分支。
// @param t 测试上下文
func TestErrorBranches(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.GetVariables(context.Background(), "missing"); err == nil {
		t.Fatalf("期望查询不存在实例变量失败")
	}
	if err := engine.SetVariable(context.Background(), "missing", "x", 1); err == nil {
		t.Fatalf("期望设置不存在实例变量失败")
	}
	if _, err := engine.GetTask(context.Background(), "missing"); err == nil {
		t.Fatalf("期望查询不存在任务失败")
	}
	if err := engine.Claim(context.Background(), "missing", "alice"); err == nil {
		t.Fatalf("期望签收不存在任务失败")
	}
	if err := engine.Complete(context.Background(), "missing", api.CompleteTaskRequest{}); err == nil {
		t.Fatalf("期望完成不存在任务失败")
	}
}

// TestServiceAccessors 验证服务门面。
// @param t 测试上下文
func TestServiceAccessors(t *testing.T) {
	engine := newTestEngine(t)
	if engine.RepositoryService() == nil || engine.RuntimeService() == nil ||
		engine.TaskService() == nil || engine.HistoryService() == nil || engine.ManagementService() == nil {
		t.Fatalf("服务门面存在 nil")
	}
	info, err := engine.EngineInfo(context.Background())
	if err != nil || info["name"] != "flowtide" {
		t.Fatalf("引擎信息错误: %v %+v", err, info)
	}
	if err := engine.Close(context.Background()); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

// TestTaskPriorityAndDueDate 验证优先级与到期时间。
// @param t 测试上下文
func TestTaskPriorityAndDueDate(t *testing.T) {
	engine := newTestEngine(t)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="priority" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="task"/><userTask id="task" name="Task" flowable:priority="10" flowable:dueDate="2026-12-01T00:00:00Z" flowable:formKey="form1" xmlns:flowable="http://flowable.org/bpmn"/><sequenceFlow id="f2" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployXML(t, engine, xml)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "priority"}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if task.Priority != 10 || task.DueDate == nil || task.FormKey != "form1" {
		t.Fatalf("任务属性错误: %+v", task)
	}
}

// TestAgendaOverflow 验证议程步数上限。
// @param t 测试上下文
func TestAgendaOverflow(t *testing.T) {
	engine, err := New(Config{MaxAgendaSteps: 1, Clock: clock.NewFake(time.Now())})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err == nil {
		t.Fatalf("期望议程溢出错误")
	}
}

// TestCompleteTask_WrongUser 验证完成人校验。
// @param t 测试上下文
func TestCompleteTask_WrongUser(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	task := listTasks(t, engine)[0]
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{UserID: "bob"}); err == nil {
		t.Fatalf("期望处理人不匹配失败")
	}
}
