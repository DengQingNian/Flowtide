package postgres

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDSN 验证开发环境配置与进程覆盖，密码交由 URL 编码处理。
// @param t 测试上下文
// @return 无
func TestLoadDSN(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("db_pg_host=127.0.0.1\ndb_pg_port=5432\ndb_pg_user=flowable\ndb_pg_pwd='p@ss:/?'\ndb_pg_db=flowable\ndb_pg_schame=public\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"FLOWABLE_POSTGRES_DSN", "db_pg_host", "db_pg_port", "db_pg_user", "db_pg_pwd", "db_pg_db", "db_pg_schema", "db_pg_schame"} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
	dsn, err := LoadDSN(path)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := endpoint.User.Password()
	if password != "p@ss:/?" || endpoint.Host != "127.0.0.1:5432" || endpoint.Query().Get("search_path") != "public" {
		t.Fatal("PostgreSQL 配置编码或旧字段兼容错误")
	}
	t.Setenv("FLOWABLE_POSTGRES_DSN", "postgres://test:secret@localhost:5433/test?sslmode=disable")
	if got, err := LoadDSN(path); err != nil || got != os.Getenv("FLOWABLE_POSTGRES_DSN") {
		t.Fatal("进程 DSN 未优先")
	}
}

// TestLoadDSNValidation 验证必填项、端口、Schema 与连接失败不泄露凭据。
// @param t 测试上下文
// @return 无
func TestLoadDSNValidation(t *testing.T) {
	for _, value := range []string{"invalid", "mysql://user:secret@host/db", "postgres://host/", "postgres://user:secret@/db"} {
		t.Setenv("FLOWABLE_POSTGRES_DSN", value)
		if _, err := LoadDSN("missing"); err == nil {
			t.Fatal("无效 PostgreSQL DSN 未拒绝")
		}
	}
	t.Setenv("FLOWABLE_POSTGRES_DSN", "")
	for _, port := range []string{"0", "65536", "bad"} {
		t.Setenv("db_pg_host", "localhost")
		t.Setenv("db_pg_user", "user")
		t.Setenv("db_pg_db", "db")
		t.Setenv("db_pg_port", port)
		if _, err := LoadDSN("missing"); err == nil {
			t.Fatal("无效端口未拒绝")
		}
	}
	t.Setenv("db_pg_port", "5432")
	t.Setenv("db_pg_schema", "bad;schema")
	if _, err := LoadDSN("missing"); err == nil {
		t.Fatal("无效 Schema 未拒绝")
	}
	if _, err := Open(context.Background(), "invalid"); err == nil {
		t.Fatal("无效连接未拒绝")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, "postgres://user:secret@127.0.0.1:1/db?sslmode=disable"); err == nil {
		t.Fatal("取消连接未失败")
	}
}
