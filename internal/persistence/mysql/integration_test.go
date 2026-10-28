package mysql_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
	"github.com/flowable-go/flowable/internal/persistence/schema"
	"github.com/flowable-go/flowable/internal/persistence/sqlstate"
)

// newIntegrationDB 创建独立测试库并注册连接与数据库清理。
// @param t 测试上下文
// @return 有限时长的上下文
// @return 临时库连接配置
// @return 临时库连接池
func newIntegrationDB(t *testing.T) (context.Context, mysqlstore.Config, *sql.DB) {
	t.Helper()
	if os.Getenv("FLOWABLE_MYSQL_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_MYSQL_INTEGRATION=1 启用真实 MySQL 测试")
	}
	envPath := os.Getenv("FLOWABLE_MYSQL_ENV")
	if envPath == "" {
		envPath = filepath.Join("..", "..", "..", ".env")
	}
	cfg, err := mysqlstore.LoadConfig(envPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	admin, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	var version string
	if err := admin.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("真实 MySQL 版本：%s", version)
	// 仅创建与清理本次测试生成的临时库，开发库现有表和数据保持完整。
	name := fmt.Sprintf("flowable_go_it_%d", time.Now().UnixNano())
	t.Logf("隔离测试库：%s", name)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4"); err != nil {
		t.Fatalf("创建隔离测试库失败，需要 CREATE/DROP DATABASE 权限: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+name); err != nil {
			t.Errorf("清理隔离测试库失败: %v", err)
		}
	})
	cfg.Database = name
	db, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, cfg, db
}

// TestIntegrationMySQLSchema 验证完整 DDL、重复初始化及启动校验。
// @param t 测试上下文
// @return 无
func TestIntegrationMySQLSchema(t *testing.T) {
	ctx, _, db := newIntegrationDB(t)
	if err := mysqlstore.EnsureSchema(ctx, db, false); err == nil {
		t.Fatal("未初始化应失败")
	}
	if err := mysqlstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	if err := mysqlstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatalf("重复初始化失败: %v", err)
	}
	if err := mysqlstore.EnsureSchema(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	t.Log("完整 Schema 与重复初始化验证通过")
}

// TestIntegrationMySQLSnapshot 独立验证真实数据库的快照原子性与重启恢复。
// @param t 测试上下文
// @return 无
func TestIntegrationMySQLSnapshot(t *testing.T) {
	ctx, cfg, db := newIntegrationDB(t)
	ddl, err := schema.Load("mysql")
	if err != nil {
		t.Fatal(err)
	}
	// 快照恢复只依赖二进制表，与全量 Schema 测试分开运行以定位环境和业务问题。
	start := strings.Index(ddl, "create table ACT_GE_BYTEARRAY (")
	end := strings.Index(ddl[start:], ";")
	if _, err := db.ExecContext(ctx, ddl[start:start+end]); err != nil {
		t.Fatal(err)
	}
	store, err := sqlstate.New(sqlstate.SQLDB{DB: db}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" targetNamespace="测试">
 <process id="mysql_restart" isExecutable="true">
  <startEvent id="start"/><sequenceFlow id="s1" sourceRef="start" targetRef="review"/>
  <userTask id="review" name="审核"/><sequenceFlow id="s2" sourceRef="review" targetRef="end"/>
  <endEvent id="end"/>
 </process>
</definitions>`
	if _, err := first.Deploy(ctx, api.DeploymentRequest{Name: "MySQL恢复验证", Resources: map[string][]byte{"restart.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	model, err := first.CreateModel(ctx, api.CreateModelRequest{Name: "MySQL模型", Key: "mysql-model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetModelSource(ctx, model.ID, []byte("模型编辑源")); err != nil {
		t.Fatal(err)
	}
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "mysql_restart", UserID: "alice", Variables: []api.Variable{{Name: "备注", Value: "中文与表情😀"}, {Name: "金额", Value: 123}}})
	if err != nil {
		t.Fatal(err)
	}
	// 已取消的写入必须保留数据库中的旧快照。
	before, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := store.Save(canceled, []byte("无效新状态")); err == nil {
		t.Fatal("取消写入应失败")
	}
	after, err := store.Load(ctx)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("失败写入破坏旧快照")
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 新建独立连接池，验证恢复不依赖旧进程中的连接和状态。
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restoredStore, err := sqlstate.New(sqlstate.SQLDB{DB: reopened}, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.New(engine.Config{StateStore: restoredStore, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	if restoredModel, err := second.GetModel(ctx, model.ID); err != nil || restoredModel.Name != "MySQL模型" {
		t.Fatalf("模型恢复失败: %v", err)
	}
	if source, err := second.GetModelSource(ctx, model.ID); err != nil || string(source) != "模型编辑源" {
		t.Fatalf("模型编辑源恢复失败: %v", err)
	}
	if links, err := second.ListInstanceIdentityLinks(ctx, instance.ID); err != nil || len(links) != 1 || links[0].UserID != "alice" {
		t.Fatalf("实例身份链接恢复失败: %v", err)
	}
	variables, err := second.GetVariables(ctx, instance.ID)
	if err != nil || variables["备注"] != "中文与表情😀" || variables["金额"] != int(123) {
		t.Fatalf("变量恢复失败: %v", err)
	}
	tasks, err := second.ListTasks(ctx, api.TaskFilter{})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("任务恢复失败: %v", err)
	}
	if err := second.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	third, err := engine.New(engine.Config{StateStore: restoredStore, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close(ctx)
	ended, err := third.GetProcessInstance(ctx, instance.ID)
	if err != nil || !ended.Ended {
		t.Fatalf("结束状态恢复失败: %v", err)
	}
	history, err := third.ListHistoricProcessInstances(ctx)
	if err != nil || len(history) != 1 || !history[0].Ended {
		t.Fatalf("历史恢复失败: %v", err)
	}
	t.Log("原子快照、失败写入保护、变量/任务恢复和历史恢复验证通过")
}
