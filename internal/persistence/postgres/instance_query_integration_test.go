package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
	_ "github.com/lib/pq"
)

// TestIntegrationPostgresInstanceQueryPushdown 验证 PostgreSQL 方言下实例与历史查询的真实 SQL 下推。
// 覆盖实例、历史实例、历史任务、历史活动与历史变量五类查询，确认 $n 占位符、JOIN 与时间边界在真实库可用。
// @param t 测试上下文
func TestIntegrationPostgresInstanceQueryPushdown(t *testing.T) {
	if os.Getenv("FLOWABLE_POSTGRES_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_POSTGRES_INTEGRATION=1 启用 PostgreSQL 查询下推验收")
	}
	endpoint, err := url.Parse(integrationDSN(t))
	if err != nil || endpoint.Host == "" {
		t.Fatal("需要有效的 FLOWABLE_POSTGRES_DSN 连接地址")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	admin, err := sql.Open("postgres", endpoint.String())
	if err != nil {
		t.Fatal("打开 PostgreSQL 失败")
	}
	defer admin.Close()
	if err := admin.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	// 只创建并删除测试生成的临时库，不写开发库
	name := fmt.Sprintf("flowable_go_qp_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal("创建 PostgreSQL 临时库失败")
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	endpoint.Path = "/" + name
	// 隔离库中 search_path 指向的 Schema 尚不存在，需先建 Schema 再建业务连接
	prepareIsolatedDatabase(ctx, t, endpoint)
	db, err := sql.Open("postgres", endpoint.String())
	if err != nil {
		t.Fatal("打开临时 PostgreSQL 库失败")
	}
	defer db.Close()
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Log("真实 PostgreSQL 版本：" + version)
	if err := pgstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	app, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="pg_push" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="approve"/>
    <userTask id="approve" name="Approve"/>
    <sequenceFlow id="f2" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Name: "pg-query", Resources: map[string][]byte{"pg-query.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	first, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "pg_push", BusinessKey: "bk-a",
		Variables: []api.Variable{{Name: "amount", Value: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "pg_push", BusinessKey: "bk-b",
		Variables: []api.Variable{{Name: "amount", Value: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	// 等值过滤、JOIN 定义 Key 与分页
	page, err := app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{BusinessKey: "bk-a", Size: 10})
	if err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != first.ID {
		t.Fatalf("业务键过滤错误: %v %+v", err, page)
	}
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{ProcessDefinitionKey: "pg_push", Size: 10})
	if err != nil || page.Total != 2 || len(page.Data) != 2 {
		t.Fatalf("定义 Key 过滤错误: %v %+v", err, page)
	}
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{ProcessDefinitionKey: "pg_push", Start: 1, Size: 1})
	if err != nil || page.Total != 2 || len(page.Data) != 1 {
		t.Fatalf("分页错误: %v %+v", err, page)
	}
	// 挂起状态以 SUSPENSION_STATE_ 编码下推；恢复后才能完成任务
	if err := app.SuspendProcessInstance(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	suspended := true
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{Suspended: &suspended, Size: 10})
	if err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != first.ID {
		t.Fatalf("挂起过滤错误: %v %+v", err, page)
	}
	if err := app.ActivateProcessInstance(ctx, first.ID); err != nil {
		t.Fatalf("恢复实例失败: %v", err)
	}
	// 时间边界：上界含两个实例，下界推进到未来后排除全部运行实例
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
	for _, instanceID := range []string{first.ID, second.ID} {
		tasks, err := app.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instanceID, Size: 10})
		if err != nil || len(tasks.Data) != 1 {
			t.Fatalf("查询实例任务失败: %v %+v", err, tasks)
		}
		if err := app.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	page, err = app.QueryProcessInstances(ctx, api.ProcessInstanceFilter{ProcessDefinitionKey: "pg_push", Size: 10})
	if err != nil || page.Total != 0 {
		t.Fatalf("运行实例应为空: %v %+v", err, page)
	}
	// 历史实例：结束状态、业务键与时间区间
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
	historic, err = app.QueryHistoricProcessInstances(ctx, api.HistoricProcessInstanceFilter{StartedBefore: &before, FinishedBefore: &before, Size: 10})
	if err != nil || historic.Total != 2 {
		t.Fatalf("历史时间区间过滤错误: %v %+v", err, historic)
	}
	// 历史任务：结束状态、定义 Key、任务定义 Key 与时间、名称
	finishedTasks, err := app.QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{Finished: &finished, Size: 10})
	if err != nil || finishedTasks.Total != 2 || len(finishedTasks.Data) != 2 {
		t.Fatalf("历史任务结束过滤错误: %v %+v", err, finishedTasks)
	}
	finishedTasks, err = app.QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{ProcessDefinitionKey: "pg_push", Size: 10})
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
	// 历史活动：每个实例五个活动（开始、顺序流、用户任务、顺序流、结束）
	finishedActivities, err := app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{Finished: &finished, Size: 20})
	if err != nil || finishedActivities.Total != 10 || len(finishedActivities.Data) != 10 {
		t.Fatalf("历史活动结束过滤错误: %v %+v", err, finishedActivities)
	}
	finishedActivities, err = app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{ActivityID: "approve", Size: 20})
	if err != nil || finishedActivities.Total != 2 {
		t.Fatalf("历史活动 ID 过滤错误: %v %+v", err, finishedActivities)
	}
	finishedActivities, err = app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{ActivityType: "userTask", ProcessDefinitionKey: "pg_push", Size: 20})
	if err != nil || finishedActivities.Total != 2 {
		t.Fatalf("历史活动类型与定义过滤错误: %v %+v", err, finishedActivities)
	}
	finishedActivities, err = app.QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{ProcessInstanceID: first.ID, StartedBefore: &before, Size: 20})
	if err != nil || finishedActivities.Total != 5 {
		t.Fatalf("历史活动实例与时间过滤错误: %v %+v", err, finishedActivities)
	}
	// 历史变量：名称与类型、实例过滤
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
