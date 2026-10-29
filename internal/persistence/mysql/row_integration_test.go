package mysql_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
	"github.com/flowable-go/flowable/internal/persistence/sqlstate"
)

// TestIntegrationMySQLRows 验证真实外键下部署、模型源码、属性、变量及任务续跑。
// @param t 测试上下文
// @return 无
func TestIntegrationMySQLRows(t *testing.T) {
	ctx, _, db := newIntegrationDB(t)
	if err := mysqlstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="row_test" isExecutable="true"><startEvent id="start"/><sequenceFlow id="a" sourceRef="start" targetRef="task"/><userTask id="task" flowable:candidateUsers="alice"/><sequenceFlow id="b" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployment, err := first.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"test.bpmn20.xml": []byte(xml)}})
	if err != nil {
		t.Fatal(err)
	}
	model, err := first.CreateModel(ctx, api.CreateModelRequest{Name: "模型", Key: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetModelSource(ctx, model.ID, []byte("模型源码")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"一", "二", "三"} {
		if _, err := first.UpdateModel(ctx, model.ID, api.ModelUpdate{Name: &name}); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.CreateProperty(ctx, "audit.property", "旧值"); err != nil {
		t.Fatal(err)
	}
	if err := first.UpdateProperty(ctx, "audit.property", "新值"); err != nil {
		t.Fatal(err)
	}
	binary := []byte{1, 2, 3}
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "row_test", UserID: "starter", Variables: []api.Variable{{Name: "binary", Value: binary}, {Name: "long", Value: strings.Repeat("长文本", 2000)}}})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("验收时区", 8*3600))
	standalone, err := first.CreateTask(ctx, api.CreateTaskRequest{Name: "独立任务", DueDate: &due})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Claim(ctx, standalone.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	claimed, err := first.GetTask(ctx, standalone.ID)
	if err != nil || claimed.ClaimTime == nil {
		t.Fatalf("独立任务签收元数据缺失: %v", err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	if task, err := second.GetTask(ctx, standalone.ID); err != nil || task.ClaimTime == nil || !task.ClaimTime.Equal(claimed.ClaimTime.Truncate(time.Millisecond)) {
		t.Fatalf("MySQL 标准任务行丢失签收时间: %+v %v", task, err)
	} else if task.CreateTime.Sub(claimed.CreateTime).Abs() > time.Millisecond {
		t.Fatalf("MySQL 任务创建时间重启后漂移: 原值=%s 恢复=%s", claimed.CreateTime, task.CreateTime)
	} else if task.DueDate == nil || !task.DueDate.Equal(due) {
		t.Fatalf("MySQL 到期时间重启后漂移: %+v", task.DueDate)
	}
	restored, err := second.GetModelSource(ctx, model.ID)
	if err != nil || !bytes.Equal(restored, []byte("模型源码")) {
		t.Fatalf("源码恢复失败: %v", err)
	}
	got, err := second.GetVariable(ctx, instance.ID, "binary")
	if err != nil || !reflect.DeepEqual(got, binary) {
		t.Fatalf("字节变量恢复失败: %T %v", got, err)
	}
	name := "恢复后修改"
	if _, err := second.UpdateModel(ctx, model.ID, api.ModelUpdate{Name: &name}); err != nil {
		t.Fatal(err)
	}
	tasks, err := second.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("任务恢复失败: %+v %v", tasks, err)
	}
	if tasks.Data[0].ExecutionID == "" || tasks.Data[0].ExecutionID == instance.ID {
		t.Fatalf("MySQL 恢复后任务仍绑定实例根: %+v", tasks.Data[0])
	}
	var rootActivity, rootParent, childBusiness sql.NullString
	var rootScope bool
	if err := db.QueryRowContext(ctx, "SELECT ACT_ID_, PARENT_ID_, IS_SCOPE_ FROM ACT_RU_EXECUTION WHERE ID_=?", instance.ID).Scan(&rootActivity, &rootParent, &rootScope); err != nil || rootActivity.Valid || rootParent.Valid || !rootScope {
		t.Fatalf("MySQL 实例根标准行不符合空活动作用域: 活动=%+v 父级=%+v 作用域=%v 错误=%v", rootActivity, rootParent, rootScope, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT BUSINESS_KEY_ FROM ACT_RU_EXECUTION WHERE ID_=?", tasks.Data[0].ExecutionID).Scan(&childBusiness); err != nil || childBusiness.Valid {
		t.Fatalf("MySQL 子执行重复存储根业务键: %+v %v", childBusiness, err)
	}
	if !reflect.DeepEqual(tasks.Data[0].CandidateUsers, []string{"alice"}) {
		t.Fatal("候选人恢复失败")
	}
	if err := second.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := second.DeleteTask(ctx, standalone.ID, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := second.DeleteModel(ctx, model.ID); err != nil {
		t.Fatal(err)
	}
	if err := second.DeleteProperty(ctx, "audit.property"); err != nil {
		t.Fatal(err)
	}
	if err := second.DeleteDeployment(ctx, deployment.ID, true); err != nil {
		t.Fatal(err)
	}
	var properties int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_GE_PROPERTY WHERE NAME_ IN ('schema.version','flowable-go.schema.checksum')").Scan(&properties); err != nil || properties != 2 {
		t.Fatalf("系统属性被修改或删除: %d %v", properties, err)
	}
	t.Log("真实外键、连续更新、属性及字节变量恢复、任务完成和删除验证通过")
}

// TestIntegrationMySQLRowsServiceMode 验证行仓储服务模式的独占锁、重启恢复与快照拒绝。
// @param t 测试上下文
// @return 无
func TestIntegrationMySQLRowsServiceMode(t *testing.T) {
	ctx, cfg, db := newIntegrationDB(t)
	if err := mysqlstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	if err := mysqlstore.CheckNoSnapshot(ctx, db); err != nil {
		t.Fatal(err)
	}
	locked, err := mysqlstore.NewLockedRows(ctx, db, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	// 同一数据库的第二个行模式实例必须立即失败，保证单执行者边界。
	if _, err := mysqlstore.NewLockedRows(ctx, db, cfg.Database); err == nil {
		t.Fatal("第二个行模式连接不应获取独占锁")
	}
	repository, err := flowablerow.NewRepository(locked, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:flowable="http://flowable.org/bpmn"><process id="row_service" isExecutable="true"><startEvent id="start"/><sequenceFlow id="a" sourceRef="start" targetRef="review"/><userTask id="review" name="审核"/><sequenceFlow id="b" sourceRef="review" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployment, err := first.Deploy(ctx, api.DeploymentRequest{Name: "行仓储服务模式", Resources: map[string][]byte{"row-service.bpmn20.xml": []byte(xml)}})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "row_service", UserID: "alice", Variables: []api.Variable{{Name: "金额", Value: 123}}})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := first.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("启动后任务=%+v 错误=%v", tasks, err)
	}
	if err := first.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 关闭顺序与 main 一致：先关闭引擎，再释放独占锁并关闭连接池。
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restoredLock, err := mysqlstore.NewLockedRows(ctx, reopened, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	restoredRepository, err := flowablerow.NewRepository(restoredLock, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.New(engine.Config{RowRepository: restoredRepository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	if restoredDeployment, err := second.GetDeployment(ctx, deployment.ID); err != nil || restoredDeployment.Name != "行仓储服务模式" {
		t.Fatalf("部署恢复失败: %+v 错误=%v", restoredDeployment, err)
	}
	if restoredXML, err := second.GetProcessDefinitionXML(ctx, instance.ProcessDefinitionID); err != nil || len(restoredXML) == 0 {
		t.Fatalf("流程定义恢复失败: %v", err)
	}
	// 已结束实例只保留历史行，行模式重启后以历史状态作为完成依据。
	if history, err := second.ListHistoricProcessInstances(ctx); err != nil || len(history) != 1 || !history[0].Ended {
		t.Fatalf("历史流程恢复失败: %+v 错误=%v", history, err)
	}
	if historicTasks, err := second.ListHistoricTaskInstances(ctx, instance.ID); err != nil || len(historicTasks) != 1 {
		t.Fatalf("历史任务恢复失败: %+v 错误=%v", historicTasks, err)
	}
	if variables, err := second.ListHistoricVariableInstances(ctx, instance.ID); err != nil {
		t.Fatalf("历史变量恢复失败: %v", err)
	} else if len(variables) != 1 || variables[0].Name != "金额" || fmt.Sprint(variables[0].Value) != "123" {
		t.Fatalf("历史变量恢复失败: %+v", variables)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restoredLock.Release(); err != nil {
		t.Fatal(err)
	}
	// 模拟数据库曾被快照模式使用，行模式必须拒绝启动而不是静默混用。
	if _, err := reopened.ExecContext(ctx, "INSERT INTO ACT_GE_BYTEARRAY (ID_, REV_, NAME_, BYTES_, GENERATED_) VALUES (?, ?, ?, ?, ?)", "flowable-go-state", 1, "flowable-go-state", []byte("旧快照"), false); err != nil {
		t.Fatal(err)
	}
	if err := mysqlstore.CheckNoSnapshot(ctx, reopened); err == nil {
		t.Fatal("存在快照状态时必须拒绝行模式")
	}
	mixingLock, err := mysqlstore.NewLockedRows(ctx, reopened, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err := mysqlstore.CheckNoSnapshot(ctx, reopened); err == nil {
		t.Fatal("持锁后仍须拒绝快照数据")
	}
	if err := mixingLock.Release(); err != nil {
		t.Fatal(err)
	}
	t.Log("行仓储独占锁、完成状态重启恢复及快照混用拒绝验证通过")
}

// TestIntegrationMySQLExclusive 验证两个真实连接不能同时独占同一数据库快照。
// @param t 测试上下文
// @return 无
func TestIntegrationMySQLExclusive(t *testing.T) {
	ctx, cfg, db := newIntegrationDB(t)
	first, release, err := sqlstate.NewExclusiveMySQL(ctx, db, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.CheckOwner(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sqlstate.NewExclusiveMySQL(ctx, db, cfg.Database); err == nil {
		t.Fatal("重复服务实例必须拒绝")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	second, releaseSecond, err := sqlstate.NewExclusiveMySQL(ctx, db, cfg.Database)
	if err != nil {
		t.Fatal("前一个服务释放后应可接管")
	}
	if err := second.CheckOwner(ctx); err != nil {
		t.Fatal(err)
	}
	if err := releaseSecond(); err != nil {
		t.Fatal(err)
	}
}
