package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	"github.com/flowable-go/flowable/internal/persistence/schema"
	"github.com/flowable-go/flowable/internal/persistence/sqlstate"
	_ "github.com/lib/pq"
)

// TestIntegrationPostgres 验证真实 PostgreSQL 原子快照、行事务、版本竞争与续跑。
// @param t 测试上下文
// @return 无
func TestIntegrationPostgres(t *testing.T) {
	if os.Getenv("FLOWABLE_POSTGRES_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_POSTGRES_INTEGRATION=1 启用 PostgreSQL 验收")
	}
	endpoint, err := url.Parse(os.Getenv("FLOWABLE_POSTGRES_DSN"))
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
	name := fmt.Sprintf("flowable_go_it_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	endpoint.Path = "/" + name
	db, err := sql.Open("postgres", endpoint.String())
	if err != nil {
		t.Fatal("打开临时 PostgreSQL 库失败")
	}
	defer db.Close()
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Log(version)
	ddl, err := schema.Load("postgres")
	if err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(ddl, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(statement) != "" {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	store, _ := sqlstate.New(sqlstate.SQLDB{DB: db}, "postgres")
	if err := store.Save(ctx, []byte("旧快照")); err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := store.Save(canceled, []byte("错误快照")); err == nil {
		t.Fatal("取消写入应失败")
	}
	if data, err := store.Load(ctx); err != nil || !bytes.Equal(data, []byte("旧快照")) {
		t.Fatalf("旧快照丢失: %v", err)
	}
	// 快照与行模式分开验收；行仓储不接管独立的快照记录。
	if _, err := db.ExecContext(ctx, "DELETE FROM ACT_GE_BYTEARRAY"); err != nil {
		t.Fatal(err)
	}
	repository, _ := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "postgres")
	first, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="pg" isExecutable="true"><startEvent id="s"/><sequenceFlow id="a" sourceRef="s" targetRef="task"/><userTask id="task"/><sequenceFlow id="b" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	deployment, err := first.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"test.bpmn20.xml": []byte(xml)}})
	if err != nil {
		t.Fatal(err)
	}
	model, err := first.CreateModel(ctx, api.CreateModelRequest{Name: "初始", Key: "model"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"一", "二", "三"} {
		if _, err := first.UpdateModel(ctx, model.ID, api.ModelUpdate{Name: &name}); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.SetModelSource(ctx, model.ID, []byte("源码")); err != nil {
		t.Fatal(err)
	}
	if err := first.CreateProperty(ctx, "audit.property", "旧"); err != nil {
		t.Fatal(err)
	}
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "pg", Variables: []api.Variable{{Name: "binary", Value: []byte{1, 2, 3}}}})
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
	stale, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	nameUpdate := "新版本"
	if _, err := second.UpdateModel(ctx, model.ID, api.ModelUpdate{Name: &nameUpdate}); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.UpdateModel(ctx, model.ID, api.ModelUpdate{Name: &nameUpdate}); err == nil {
		t.Fatal("旧引擎更新必须发生乐观锁冲突")
	}
	if err := stale.DeleteModel(ctx, model.ID); err == nil {
		t.Fatal("旧引擎删除必须发生乐观锁冲突")
	}
	if err := second.UpdateProperty(ctx, "audit.property", "新"); err != nil {
		t.Fatal(err)
	}
	tasks, err := second.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatal("恢复任务失败")
	}
	if err := second.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
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
	t.Log("PostgreSQL 原子快照、修订号竞争、字节变量、恢复续跑和删除通过")
}
