package flowtide_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide"
	"github.com/DengQingNian/Flowtide/api"
	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
)

// TestIntegrationEmbeddedMySQL 验证公共嵌入入口的真实 SQL 启停、独占锁、门面写入与重启恢复。
// @param t 测试上下文
// @return 无
func TestIntegrationEmbeddedMySQL(t *testing.T) {
	if os.Getenv("FLOWABLE_MYSQL_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_MYSQL_INTEGRATION=1 在独立临时库运行")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cfg, err := mysqlstore.LoadConfig(".env")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("flowtide_embed_it_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name); err != nil {
			t.Error(err)
		}
	}()
	cfg.Database = name
	db, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := flowtide.EnsureMySQLSchema(ctx, db, false); err == nil {
		t.Fatal("空库校验意外通过")
	}
	if err := flowtide.EnsureMySQLSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	config := flowtide.EmbeddedConfig{DB: db, Database: name, TenantID: "oa"}
	app, err := flowtide.NewEmbedded(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close(context.Background())
	if other, err := flowtide.NewEmbedded(ctx, config); err == nil {
		_ = other.Close(ctx)
		t.Fatal("重复写者未被拒绝")
	}
	callCtx := flowtide.WithExecutionContext(ctx, "oa", "101")
	if _, err := app.Engine().IdentityService().CreateUser(callCtx, api.User{ID: "101", TenantID: "oa", FirstName: "申请人"}); err != nil {
		t.Fatal(err)
	}
	bpmn := `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn" targetNamespace="test"><process id="embedded" isExecutable="true"><startEvent id="start"/><sequenceFlow id="f1" sourceRef="start" targetRef="apply"/><userTask id="apply" name="申请" flowable:assignee="${initiator}"/><sequenceFlow id="f2" sourceRef="apply" targetRef="end"/><endEvent id="end"/></process></definitions>`
	if _, err := app.Engine().RepositoryService().Deploy(callCtx, api.DeploymentRequest{Name: "嵌入验收", TenantID: "oa", Resources: map[string][]byte{"embedded.bpmn20.xml": []byte(bpmn)}}); err != nil {
		t.Fatal(err)
	}
	result, err := app.Workflow().StartProcess(callCtx, &api.StartProcessInstanceVo{ProcessDefinitionKey: "embedded", Title: "嵌入SQL验证", Operator: "101", TenantID: "oa", BusinessKey: "B1"})
	if err != nil || !result.Success {
		t.Fatalf("workflow 发起失败：%+v %v", result, err)
	}
	var messages int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM workflow_task_message_extend").Scan(&messages); err != nil || messages == 0 {
		t.Fatalf("workflow 扩展数据未落库：%d %v", messages, err)
	}
	tasks, err := app.Engine().TaskService().ListTasks(callCtx, api.TaskFilter{Assignee: "101"})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("任务不存在：%v %v", tasks, err)
	}
	task := tasks.Data[0]
	derived, err := app.Engine().RuntimeService().InjectUserTaskAfter(callCtx, task.ProcessInstanceID, api.InsertUserTaskRequest{
		AfterActivityID: task.TaskDefinitionKey, ID: "review", Name: "复核", Assignee: "101", ProcessDefinitionID: task.ProcessDefinitionID,
	})
	if err != nil {
		t.Fatalf("插入节点失败：%v", err)
	}
	if err := app.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("SDK 关闭了宿主连接池：%v", err)
	}
	restarted, err := flowtide.NewEmbedded(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close(context.Background())
	if resumed, err := restarted.Engine().TaskService().GetTask(callCtx, task.ID); err != nil || resumed.ProcessDefinitionID != derived.ID {
		t.Fatalf("任务重启恢复失败：%+v %v", resumed, err)
	}
	result, err = restarted.Workflow().Complete(callCtx, &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: task.ID, ProcessInstanceID: task.ProcessInstanceID, UserCode: "101", Message: "同意", Type: "SP"}})
	if err != nil || !result.Success {
		t.Fatalf("workflow 办结失败：%+v %v", result, err)
	}
	tasks, err = restarted.Engine().TaskService().ListTasks(callCtx, api.TaskFilter{ProcessInstanceID: task.ProcessInstanceID})
	if err != nil || len(tasks.Data) != 1 || tasks.Data[0].TaskDefinitionKey != "review" {
		t.Fatalf("重启后原节点办结未流转至插入节点：%+v %v", tasks, err)
	}
	result, err = restarted.Workflow().Complete(callCtx, &api.CompleteTaskVo{BaseProcessVo: api.BaseProcessVo{TaskID: tasks.Data[0].ID, ProcessInstanceID: task.ProcessInstanceID, UserCode: "101", Message: "复核完成", Type: "SP"}})
	if err != nil || !result.Success {
		t.Fatalf("插入节点办结失败：%+v %v", result, err)
	}
	ended, err := restarted.Engine().HistoryService().GetHistoricProcessInstance(callCtx, task.ProcessInstanceID)
	if err != nil || ended.EndTime == nil {
		t.Fatalf("插入节点办结后未结束：%+v %v", ended, err)
	}
	if _, err := restarted.RunDueJobs(callCtx); err != nil {
		t.Fatal(err)
	}
	t.Log("已验证：SQL门面发起/办结、派生节点续办、独占拒绝、宿主连接池保留、停启恢复")
}
