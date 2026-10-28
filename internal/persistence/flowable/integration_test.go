package flowable

import (
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"os"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/internal/persistence/schema"
	"github.com/flowable-go/flowable/internal/persistence/sqlstate"
)

// TestIntegrationSQL 是可选的真实数据库集成测试。
// 需要 FLOWABLE_TEST_DRIVER 与 FLOWABLE_TEST_DSN，且调用方已注册驱动。
// @param t 测试上下文
func TestIntegrationSQL(t *testing.T) {
	driverName := os.Getenv("FLOWABLE_TEST_DRIVER")
	dsn := os.Getenv("FLOWABLE_TEST_DSN")
	if driverName == "" && dsn == "" {
		t.Skip("未配置 FLOWABLE_TEST_DRIVER/FLOWABLE_TEST_DSN")
	}
	if driverName == "" || dsn == "" {
		t.Fatal("显式数据库测试配置不完整")
	}
	dialect := os.Getenv("FLOWABLE_TEST_DIALECT")
	if dialect == "" {
		dialect = "postgres"
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("数据库不可达: %v", err)
	}
	ddl, err := schema.Load(dialect)
	if err != nil {
		t.Fatalf("读取 Schema 失败: %v", err)
	}
	for _, statement := range splitSQL(ddl) {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("执行 DDL 失败: %v\n%s", err, statement)
		}
	}

	store, err := sqlstate.New(sqlstate.SQLDB{DB: db}, dialect)
	if err != nil {
		t.Fatalf("创建状态存储失败: %v", err)
	}
	if err := store.Save(context.Background(), []byte("integration-state")); err != nil {
		t.Fatalf("保存状态失败: %v", err)
	}
	data, err := store.Load(context.Background())
	if err != nil || string(data) != "integration-state" {
		t.Fatalf("读取状态失败: %v %s", err, data)
	}
}

// splitSQL 按分号拆分 SQL，忽略注释行。
// @param script SQL 脚本
// @return 语句列表
func splitSQL(script string) []string {
	lines := strings.Split(script, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	parts := strings.Split(strings.Join(cleaned, "\n"), ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		if statement := strings.TrimSpace(part); statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}

// TestSplitSQL 验证 SQL 拆分。
// @param t 测试上下文
func TestSplitSQL(t *testing.T) {
	statements := splitSQL(`-- comment
CREATE TABLE a (x int);

CREATE INDEX i ON a(x);`)
	if len(statements) != 2 || !strings.Contains(statements[0], "CREATE TABLE") || !strings.Contains(statements[1], "CREATE INDEX") {
		t.Fatalf("SQL 拆分错误: %+v", statements)
	}
}
