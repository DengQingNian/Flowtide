// integration_dsn_external_test.go 为外部测试包（postgres_test）提供 PostgreSQL 集成测试基础设施（016 号审计）。
//
// 提供两项能力：
//  1. DSN 解析：环境变量 FLOWABLE_POSTGRES_DSN 优先，未设置时退回仓库根 .env；
//  2. 隔离库引导：DSN 携带 search_path 时，新库中该 Schema 尚不存在会导致建连失败，
//     因此先以无 search_path 的连接建 Schema，再建立带 search_path 的业务连接。
package postgres_test

import (
	"context"
	"database/sql"
	"net/url"
	"os"

	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
)

// integrationEnvPath 返回仓库根目录的 .env 路径（可用 FLOWABLE_ENV 覆盖）。
// @return 配置文件路径
func integrationEnvPath() string {
	if custom := os.Getenv("FLOWABLE_ENV"); custom != "" {
		return custom
	}
	return "../../../.env"
}

// integrationDSN 返回 PostgreSQL 验收 DSN：环境变量优先，未设置时退回 .env。
// @param t 测试上下文
// @return 连接 URI
func integrationDSN(t interface{ Fatalf(string, ...any) }) string {
	if dsn := os.Getenv("FLOWABLE_POSTGRES_DSN"); dsn != "" {
		return dsn
	}
	dsn, err := pgstore.LoadDSN(integrationEnvPath())
	if err != nil {
		t.Fatalf("需要有效的 FLOWABLE_POSTGRES_DSN 或可读的 .env 配置: %v", err)
	}
	return dsn
}

// prepareIsolatedDatabase 在隔离库中预建 search_path 指向的 Schema。
// 连接 DSN 携带 search_path 时，Schema 不存在会让建连本身失败，故需先用
// 去掉 search_path 的连接创建 Schema。
// @param ctx 操作上下文
// @param t 测试上下文
// @param endpoint 已指向隔离库的连接 URI
func prepareIsolatedDatabase(ctx context.Context, t interface{ Fatalf(string, ...any) }, endpoint *url.URL) {
	searchPath := endpoint.Query().Get("search_path")
	// 未配置或不指向自定义 Schema 时无需预建
	if searchPath == "" || searchPath == "public" {
		return
	}
	clone := *endpoint
	query := clone.Query()
	query.Del("search_path")
	clone.RawQuery = query.Encode()
	bootstrap, err := sql.Open("postgres", clone.String())
	if err != nil {
		t.Fatalf("连接隔离测试库失败: %v", err)
	}
	defer func() { _ = bootstrap.Close() }()
	// Schema 名可能带引号（含连字符时由 LoadDSN 加引号），此处按原样作为标识符使用
	if _, err := bootstrap.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+searchPath); err != nil {
		t.Fatalf("创建测试 Schema 失败: %v", err)
	}
}
