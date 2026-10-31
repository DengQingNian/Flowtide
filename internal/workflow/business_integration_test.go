// business_integration_test.go 验证业务系统注册 SQL 适配器在真实 MySQL 上的往返（开关跳过模式）。
// 设置 FLOWABLE_MYSQL_INTEGRATION=1 启用；使用 .env 连接配置并创建隔离临时库，测试后清理。
package workflow

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	_ "github.com/go-sql-driver/mysql"
)

// bizEnvPath 返回仓库根目录 .env 路径（internal/workflow 上两级）。
// @return 配置文件路径
func bizEnvPath() string {
	if custom := os.Getenv("FLOWABLE_ENV"); custom != "" {
		return custom
	}
	return filepath.Join("..", "..", ".env")
}

// bizRunScript 执行 DDL 脚本（去掉整行注释后按分号拆分）。
// @param t 测试上下文
// @param ctx 操作上下文
// @param db 连接池
// @param script 脚本内容
func bizRunScript(t *testing.T, ctx context.Context, db *sql.DB, script string) {
	t.Helper()
	lines := make([]string, 0)
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for i, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("执行 workflow DDL 第 %d 条语句失败: %v", i+1, err)
		}
	}
}

// TestIntegrationBusinessSystemMySQL 验证业务系统注册在真实 MySQL 上的注册/查询/更新/注销往返。
// @param t 测试上下文
func TestIntegrationBusinessSystemMySQL(t *testing.T) {
	if os.Getenv("FLOWABLE_MYSQL_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_MYSQL_INTEGRATION=1 启用真实 MySQL 测试")
	}
	cfg, err := mysqlstore.LoadConfig(bizEnvPath())
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
	// 隔离临时库：只创建/清理本次测试库，不触碰开发库现有数据
	name := "flowable_wf_biz_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Logf("MySQL 隔离测试库：%s", name)
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
	ddl, err := schema.LoadFile("mysql", schema.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	bizRunScript(t, ctx, db, ddl)
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewSQLBusinessSystemStore(db, pwf.DialectMySQL, store)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewBusinessSystemService(adapter)
	// 注册
	registered, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa", SystemName: "OA 系统", TaskCallAddr: "http://oa/task", ExecutionCallAddr: "http://oa/exec", Creator: "admin", CreatorName: "管理员"})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if registered.ID == "" || registered.Secret == "" || registered.TenantID != "oa" {
		t.Fatalf("注册回填不符: %+v", registered)
	}
	// 重复注册冲突
	if _, err := svc.Register(ctx, &BusinessSystem{SystemSymbol: "oa"}); !errors.Is(err, api.ErrConflict) {
		t.Errorf("重复注册应报 conflict，实际 %v", err)
	}
	// 按代号查询（走仓储）与按租户查询（裸 SQL 补齐路径）
	got, err := svc.Get(ctx, "oa")
	if err != nil || got.ID != registered.ID {
		t.Fatalf("按代号查询失败: %v", err)
	}
	byTenant, found, err := adapter.GetByTenant(ctx, "oa")
	if err != nil || !found || byTenant.SystemSymbol != "oa" || byTenant.TaskCallAddr != "http://oa/task" {
		t.Fatalf("按租户查询失败: %v found=%v", err, found)
	}
	// 更新（upsert 覆盖路径）
	if _, err := svc.Update(ctx, &BusinessSystem{SystemSymbol: "oa", SystemName: "OA 二期", Updater: "editor"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	after, err := svc.Get(ctx, "oa")
	if err != nil || after.SystemName != "OA 二期" || after.Updater != "editor" || after.TaskCallAddr != "http://oa/task" {
		t.Fatalf("更新结果不符: %+v %v", after, err)
	}
	// 回调 client 与 SQL 注册表联跑：未注册的租户静默跳过（真实网络零请求）。
	// 已注册的 oa 指向不可达地址，发起真实回调会因 DNS 解析失败而重试 4 次，
	// 与"真实网络零请求"的联跑意图不符，此处只验证查无注册的跳过语义。
	client := NewCallbackClient(adapter)
	if err := client.SendTaskCallback(ctx, &TaskCallback{TenantID: "unregistered-tenant"}); err != nil {
		t.Errorf("未注册租户的回调应静默跳过: %v", err)
	}
	// 列表与注销
	all, err := svc.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("列表失败: %v", err)
	}
	if err := svc.Remove(ctx, "oa"); err != nil {
		t.Fatalf("注销失败: %v", err)
	}
	if _, err := svc.Get(ctx, "oa"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("注销后查询应报 not found，实际 %v", err)
	}
}
