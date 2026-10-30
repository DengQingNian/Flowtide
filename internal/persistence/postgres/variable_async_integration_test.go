package postgres_test

import (
	"bytes"
	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	dbstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
	"reflect"
	"testing"
	"time"
)

// TestIntegrationAsyncVariables 验证真实数据库持久排队、重新取锁恢复、精确类型及载荷回收。
// @param t 测试上下文
// @return 无
func TestIntegrationAsyncVariables(t *testing.T) {
	ctx, db := postgresServiceDatabase(t)
	locked, err := dbstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = locked.Release() })
	repository, err := flowablerow.NewRepository(locked, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="asyncvars" isExecutable="true"><startEvent id="s"/><sequenceFlow id="a" sourceRef="s" targetRef="task"/><userTask id="task"/><sequenceFlow id="b" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	if _, err := first.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"async.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "asyncvars", Variables: []api.Variable{{Name: "amount", Value: int64(42)}}})
	if err != nil {
		t.Fatal(err)
	}
	binary := bytes.Repeat([]byte{0, 255, 1}, 5000)
	date := time.UnixMilli(1700000000123).UTC()
	request := api.AsyncVariableRequest{ProcessInstanceID: instance.ID, Variables: []api.Variable{{Name: "amount", Value: int64(9007199254740993)}, {Name: "binary", Value: binary}, {Name: "date", Value: date}}}
	if err := first.SetVariablesAsync(ctx, request); err != nil {
		t.Fatal(err)
	}
	if value, err := first.GetVariable(ctx, instance.ID, "amount"); err != nil || value != int64(42) {
		t.Fatal("真实库异步变量提前生效")
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := locked.Release(); err != nil {
		t.Fatal(err)
	}
	// 新执行者从行表读取作业及独立字节载荷，不能依赖旧引擎内存。
	restartedLock, err := dbstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedLock.Release() })
	restartedRepository, _ := flowablerow.NewRepository(restartedLock, "postgres")
	second, err := engine.New(engine.Config{RowRepository: restartedRepository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	if value, err := second.GetVariable(ctx, instance.ID, "amount"); err != nil || value != int64(42) {
		t.Fatal("重启提前执行了变量作业")
	}
	if count, err := second.RunDueJobs(ctx); err != nil || count != 1 {
		t.Fatalf("恢复后作业执行: %d %v", count, err)
	}
	values, err := second.GetVariables(ctx, instance.ID)
	if err != nil || values["amount"] != int64(9007199254740993) || !reflect.DeepEqual(values["binary"], binary) || !reflect.DeepEqual(values["date"], date) {
		t.Fatalf("真实库异步变量恢复类型错误: %v", err)
	}
	if jobs, err := second.ListJobs(ctx); err != nil || len(jobs) != 0 {
		t.Fatalf("已执行作业未删除: %+v %v", jobs, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_GE_BYTEARRAY WHERE NAME_='flowtide-job-variables'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("已执行载荷未回收: %d %v", count, err)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restartedLock.Release(); err != nil {
		t.Fatal(err)
	}
	t.Log("异步变量未提前生效，重新取锁恢复、大整数、日期、超文本容量字节与载荷回收通过")
}
