// Package postgres 提供 PostgreSQL 配置、连接和独占行仓储。
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/lib/pq"
)

// LoadDSN 读取 PostgreSQL URI 或开发配置，进程变量优先且不修改进程环境。
// @param path 环境文件路径，不存在时只使用进程环境
// @return 已验证且正确转义的连接 URI，包含凭据，禁止记录
// @return 不含凭据的中文错误
func LoadDSN(path string) (string, error) {
	values, err := godotenv.Read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("读取 PostgreSQL 配置失败")
	}
	get := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return values[key]
	}
	if dsn := get("FLOWABLE_POSTGRES_DSN"); dsn != "" {
		return validateDSN(dsn)
	}
	host, user, database := strings.TrimSpace(get("db_pg_host")), get("db_pg_user"), get("db_pg_db")
	if host == "" || user == "" || database == "" {
		return "", errors.New("PostgreSQL 配置缺少主机、用户或数据库")
	}
	port := 5432
	if value := get("db_pg_port"); value != "" {
		port, err = strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return "", errors.New("db_pg_port 必须介于 1 和 65535")
		}
	}
	// 兼容现有开发文件中的 schame 拼写；正确字段优先。
	schemaName := get("db_pg_schema")
	if schemaName == "" {
		schemaName = get("db_pg_schame")
	}
	if schemaName == "" {
		schemaName = "public"
	}
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9-]*$`).MatchString(schemaName) {
		return "", errors.New("PostgreSQL Schema 必须以字母或下划线开头，且只含字母、数字、下划线与连字符")
	}
	// search_path 中带连字符等特殊字符的 Schema 名必须加双引号，否则 libpq 解析为标识符时报错。
	// 仅含合法标识符字符时不加引号，保持与既有部署一致的连接串形态。
	searchPath := schemaName
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9]*$`).MatchString(schemaName) {
		searchPath = `"` + schemaName + `"`
	}
	endpoint := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + database, User: url.UserPassword(user, get("db_pg_pwd"))}
	query := url.Values{"search_path": {searchPath}, "connect_timeout": {"5"}}
	if mode := get("db_pg_sslmode"); mode != "" {
		query.Set("sslmode", mode)
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

// validateDSN 校验 PostgreSQL URI，并兼容已有 libpq 键值连接串。
// @param dsn 连接 URI 或键值串
// @return 原始连接串
// @return 不含凭据的参数错误
func validateDSN(dsn string) (string, error) {
	if strings.Contains(dsn, "=") && !strings.Contains(dsn, "://") {
		if _, err := pq.NewConnector(dsn); err != nil {
			return "", errors.New("PostgreSQL 键值连接配置非法")
		}
		return dsn, nil
	}
	endpoint, err := url.Parse(dsn)
	if err != nil || (endpoint.Scheme != "postgres" && endpoint.Scheme != "postgresql") || endpoint.Hostname() == "" || endpoint.User == nil || endpoint.User.Username() == "" || strings.Trim(endpoint.Path, "/") == "" {
		return "", errors.New("PostgreSQL 连接必须是包含主机、用户和数据库的 URI")
	}
	return dsn, nil
}

// Open 创建并验证 PostgreSQL 连接池，失败时关闭且不暴露连接凭据。
// @param ctx 初始化上下文
// @param dsn 连接 URI
// @return 已连接的数据库池
// @return 配置或连接错误
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if _, err := validateDSN(dsn); err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, errors.New("创建 PostgreSQL 连接池失败")
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("连接 PostgreSQL 失败，请检查配置与网络")
	}
	return db, nil
}
