package mysql_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

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
	standalone, err := first.CreateTask(ctx, api.CreateTaskRequest{Name: "独立任务"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
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
