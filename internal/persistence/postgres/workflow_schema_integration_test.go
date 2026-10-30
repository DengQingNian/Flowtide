package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	_ "github.com/lib/pq"
)

// newEnsureSchemaDB 创建隔离测试库并注册清理。
// @param t 测试上下文
// @return 操作上下文
// @return 隔离库连接池
func newEnsureSchemaDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	if os.Getenv("FLOWABLE_POSTGRES_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_POSTGRES_INTEGRATION=1 启用 PostgreSQL 验收")
	}
	dsn := os.Getenv("FLOWABLE_POSTGRES_DSN")
	if dsn == "" {
		var err error
		// 环境变量未提供时退回 .env 开发配置
		if dsn, err = pgstore.LoadDSN("../../../.env"); err != nil {
			t.Skipf("读取 .env 配置失败，跳过: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgstore.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("flowable_go_esi_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("创建隔离测试库失败: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	endpoint, err := url.Parse(dsn)
	if err != nil || endpoint.Host == "" {
		t.Fatalf("连接地址非法: %v", err)
	}
	endpoint.Path = "/" + name
	db, err := pgstore.Open(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

// TestIntegrationPostgresEnsureSchema 验证真实库上引擎（001）与 workflow（002）两段初始化、
// 重复执行幂等与 workflow 表数量。
// @param t 测试上下文
func TestIntegrationPostgresEnsureSchema(t *testing.T) {
	ctx, db := newEnsureSchemaDB(t)
	// 空库且未授权时应拒绝
	if err := pgstore.EnsureSchema(ctx, db, false); err == nil {
		t.Fatal("未初始化应失败")
	}
	// 首次初始化：引擎表 + workflow 表 + 各自完成标记
	if err := pgstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	// 重复初始化与只读校验均应通过
	if err := pgstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatalf("重复初始化失败: %v", err)
	}
	if err := pgstore.EnsureSchema(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	// workflow 表应全部建齐
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND UPPER(table_name) LIKE 'WORKFLOW\\_%' ESCAPE '\\'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 22 {
		t.Fatalf("workflow 表数量错误: %d", count)
	}
	// workflow 完成标记应写入 ACT_GE_PROPERTY
	var value string
	if err := db.QueryRowContext(ctx, "SELECT VALUE_ FROM ACT_GE_PROPERTY WHERE NAME_ = 'flowtide.workflow.schema.checksum'").Scan(&value); err != nil {
		t.Fatal(err)
	}
	checksum, _ := schema.ChecksumFile("postgres", schema.WorkflowFile)
	if value != checksum {
		t.Fatalf("workflow 完成标记不匹配: %s", value)
	}
	t.Log("引擎与 workflow 两段初始化、重复校验与完成标记验证通过")
}
