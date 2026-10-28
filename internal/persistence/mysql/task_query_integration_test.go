package mysql_test

import (
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
)

// TestIntegrationMySQLTaskQueryPushdown 验证行模式下任务查询下推的真实 SQL 路径。
// @param t 测试上下文
func TestIntegrationMySQLTaskQueryPushdown(t *testing.T) {
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
  <process id="task_push" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="approve"/>
    <userTask id="approve" name="Approve" flowable:assignee="${assignee}"/>
    <sequenceFlow id="f2" sourceRef="approve" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Name: "task-query", Resources: map[string][]byte{"task-query.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	first, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "task_push", Variables: []api.Variable{{Name: "assignee", Value: "alice"}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "task_push", Variables: []api.Variable{{Name: "assignee", Value: "bob"}}})
	if err != nil {
		t.Fatal(err)
	}
	// 等值过滤必须命中真实 ACT_RU_TASK 行
	page, err := app.ListTasks(ctx, api.TaskFilter{Assignee: "alice", Size: 10})
	if err != nil || page.Total != 1 || len(page.Data) != 1 || page.Data[0].ProcessInstanceID != first.ID {
		t.Fatalf("负责人过滤错误: %v %+v", err, page)
	}
	// 按流程定义 Key 过滤需要 JOIN ACT_RE_PROCDEF
	page, err = app.ListTasks(ctx, api.TaskFilter{ProcessDefinitionKey: "task_push", Size: 10})
	if err != nil || page.Total != 2 || len(page.Data) != 2 {
		t.Fatalf("定义 Key 过滤错误: %v %+v", err, page)
	}
	// 分页总数与切片必须来自同一 SQL 查询集
	page, err = app.ListTasks(ctx, api.TaskFilter{ProcessDefinitionKey: "task_push", Start: 1, Size: 1})
	if err != nil || page.Total != 2 || len(page.Data) != 1 {
		t.Fatalf("分页错误: %v %+v", err, page)
	}
	if page.Data[0].ProcessInstanceID == "" {
		t.Fatalf("分页任务缺少实例: %+v", page.Data)
	}
	// 候选人过滤通过 ACT_RU_IDENTITYLINK 子查询验证
	candidateXML := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn">
  <process id="task_candidate" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="review"/>
    <userTask id="review" name="Review" flowable:candidateUsers="carol"/>
    <sequenceFlow id="f2" sourceRef="review" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`
	if _, err := app.Deploy(ctx, api.DeploymentRequest{Name: "task-candidate", Resources: map[string][]byte{"task-candidate.bpmn20.xml": []byte(candidateXML)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "task_candidate"}); err != nil {
		t.Fatal(err)
	}
	page, err = app.ListTasks(ctx, api.TaskFilter{CandidateUser: "carol", Size: 10})
	if err != nil || page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("候选人过滤错误: %v %+v", err, page)
	}
	if page.Data[0].Assignee != "" || page.Data[0].ProcessInstanceID == second.ID {
		t.Fatalf("候选人任务错误: %+v", page.Data[0])
	}
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
