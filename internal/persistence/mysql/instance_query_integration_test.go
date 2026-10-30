package mysql_test

import (
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
)

// TestIntegrationMySQLInstanceQueryPushdown 验证行模式下实例与历史查询下推的真实 SQL 路径。
// @param t 测试上下文
func TestIntegrationMySQLInstanceQueryPushdown(t *testing.T) {
	ctx, _, db := newIntegrationDB(t)
	if err := mysqlstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	app, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="instance_push" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="approve"/>
    <userTask id="approve" name="Approve"/>
    <sequenceFlow id="f2" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Name: "instance-query", Resources: map[string][]byte{"instance-query.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	first, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "instance_push", BusinessKey: "bk-a",
		Variables: []api.Variable{{Name: "amount", Value: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "instance_push", BusinessKey: "bk-b",
		Variables: []api.Variable{{Name: "amount", Value: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	// 业务键等值过滤必须命中真实 ACT_RU_EXECUTION 实例根行
	page, err := app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{BusinessKey: "bk-a", Size: 10})
	if err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != first.ID {
		t.Fatalf("业务键过滤错误: %v %+v", err, page)
	}
	// 按流程定义 Key 过滤需要 JOIN ACT_RE_PROCDEF
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{ProcessDefinitionKey: "instance_push", Size: 10})
	if err != nil || page.Total != 2 || len(page.Data) != 2 {
		t.Fatalf("定义 Key 过滤错误: %v %+v", err, page)
	}
	// 分页总数与切片必须来自同一 SQL 查询集
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{ProcessDefinitionKey: "instance_push", Start: 1, Size: 1})
	if err != nil || page.Total != 2 || len(page.Data) != 1 {
		t.Fatalf("分页错误: %v %+v", err, page)
	}
	// 挂起状态以 SUSPENSION_STATE_ 编码下推
	if err := app.SuspendProcessInstance(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	suspended := true
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{Suspended: &suspended, Size: 10})
	if err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != first.ID {
		t.Fatalf("挂起过滤错误: %v %+v", err, page)
	}
	// 挂起实例的任务不能完成，完成前必须恢复实例（原版同样返回 409）
	if err := app.ActivateProcessInstance(ctx, first.ID); err != nil {
		t.Fatalf("恢复实例失败: %v", err)
	}
	// 时间上界包含两个实例，下界推进到未来后应排除全部运行实例
	before := time.Now().Add(time.Hour).UTC()
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{StartedBefore: &before, Size: 10})
	if err != nil || page.Total != 2 {
		t.Fatalf("开始时间上界过滤错误: %v %+v", err, page)
	}
	after := time.Now().Add(time.Hour).UTC()
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{StartedAfter: &after, Size: 10})
	if err != nil || page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("开始时间下界过滤错误: %v %+v", err, page)
	}
	// 完成两个实例后运行列表为空，历史列表携带结束状态与业务键
	for _, instanceID := range []string{first.ID, second.ID} {
		tasks, err := app.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
		if err != nil || len(tasks.Data) != 1 {
			t.Fatalf("查询实例任务失败: %v %+v", err, tasks)
		}
		if err := app.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{ProcessDefinitionKey: "instance_push", Size: 10})
	if err != nil || page.Total != 0 {
		t.Fatalf("运行实例应为空: %v %+v", err, page)
	}
	finished := true
	historic, err := app.QueryHistoricProcessInstances(ctx, api.HistoricProcessInstanceFilter{Finished: &finished, Size: 10})
	if err != nil || historic.Total != 2 || len(historic.Data) != 2 {
		t.Fatalf("历史结束过滤错误: %v %+v", err, historic)
	}
	historic, err = app.QueryHistoricProcessInstances(ctx, api.HistoricProcessInstanceFilter{BusinessKey: "bk-b", Size: 10})
	if err != nil || historic.Total != 1 || len(historic.Data) != 1 || historic.Data[0].ID != second.ID {
		t.Fatalf("历史业务键过滤错误: %v %+v", err, historic)
	}
	if historic.Data[0].EndTime == nil {
		t.Fatalf("历史实例缺少结束时间: %+v", historic.Data[0])
	}
	// 历史开始与结束时间区间在真实 datetime(3) 精度下过滤
	historic, err = app.QueryHistoricProcessInstances(ctx, api.HistoricProcessInstanceFilter{StartedBefore: &before, FinishedBefore: &before, Size: 10})
	if err != nil || historic.Total != 2 {
		t.Fatalf("历史时间区间过滤错误: %v %+v", err, historic)
	}
	// 历史任务下推：完成后的两个任务全部结束，按结束状态与定义 Key 过滤
	finishedTasks, err := app.QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{Finished: &finished, Size: 10})
	if err != nil || finishedTasks.Total != 2 || len(finishedTasks.Data) != 2 {
		t.Fatalf("历史任务结束过滤错误: %v %+v", err, finishedTasks)
	}
	finishedTasks, err = app.QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{ProcessDefinitionKey: "instance_push", Size: 10})
	if err != nil || finishedTasks.Total != 2 {
		t.Fatalf("历史任务定义 Key 过滤错误: %v %+v", err, finishedTasks)
	}
	finishedTasks, err = app.QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{TaskDefinitionKey: "approve", StartedBefore: &before, Size: 10})
	if err != nil || finishedTasks.Total != 2 {
		t.Fatalf("历史任务定义与时间过滤错误: %v %+v", err, finishedTasks)
	}
	finishedTasks, err = app.QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{Name: "Approve", Size: 10})
	if err != nil || finishedTasks.Total != 2 {
		t.Fatalf("历史任务名称过滤错误: %v %+v", err, finishedTasks)
	}
	// 历史活动下推：每个实例五个活动（开始、顺序流、用户任务、顺序流、结束）
	finishedActivities, err := app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{Finished: &finished, Size: 20})
	if err != nil || finishedActivities.Total != 10 || len(finishedActivities.Data) != 10 {
		t.Fatalf("历史活动结束过滤错误: %v %+v", err, finishedActivities)
	}
	finishedActivities, err = app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{ActivityID: "approve", Size: 20})
	if err != nil || finishedActivities.Total != 2 {
		t.Fatalf("历史活动 ID 过滤错误: %v %+v", err, finishedActivities)
	}
	finishedActivities, err = app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{ActivityType: "userTask", ProcessDefinitionKey: "instance_push", Size: 20})
	if err != nil || finishedActivities.Total != 2 {
		t.Fatalf("历史活动类型与定义过滤错误: %v %+v", err, finishedActivities)
	}
	finishedActivities, err = app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{ProcessInstanceID: first.ID, StartedBefore: &before, Size: 20})
	if err != nil || finishedActivities.Total != 5 {
		t.Fatalf("历史活动实例与时间过滤错误: %v %+v", err, finishedActivities)
	}
	// 历史变量下推：启动写入的流程变量在实例结束后进入 ACT_HI_VARINST
	historicVariables, err := app.QueryHistoricVariableInstances(ctx, api.HistoricVariableInstanceFilter{Name: "amount", VariableType: "long", Size: 10})
	if err != nil || historicVariables.Total != 2 || len(historicVariables.Data) != 2 {
		t.Fatalf("历史变量名称与类型过滤错误: %v %+v", err, historicVariables)
	}
	historicVariables, err = app.QueryHistoricVariableInstances(ctx, api.HistoricVariableInstanceFilter{ProcessInstanceID: first.ID, Size: 10})
	if err != nil || historicVariables.Total != 1 || len(historicVariables.Data) != 1 || historicVariables.Data[0].ProcessInstanceID != first.ID {
		t.Fatalf("历史变量实例过滤错误: %v %+v", err, historicVariables)
	}
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
