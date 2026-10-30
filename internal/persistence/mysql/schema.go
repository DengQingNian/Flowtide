package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

const checksumKey = "flowtide.schema.checksum" // 完整初始化标记

// EnsureSchema 校验已有 Schema，或在显式允许时初始化尚无引擎表的数据库。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param initialize 是否允许初始化
// @return 初始化或校验错误
func EnsureSchema(ctx context.Context, db *sql.DB, initialize bool) error {
	tables, _ := schema.Tables("mysql")
	args := make([]any, len(tables))
	for i, table := range tables {
		args[i] = table
	}
	query := "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND UPPER(table_name) IN (" + strings.TrimSuffix(strings.Repeat("?,", len(tables)), ",") + ")"
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return fmt.Errorf("检查 MySQL 引擎表失败: %w", err)
	}
	// 已有部分表意味着迁移中断或外部 Schema，禁止补建后误判为可用。
	if count != 0 && count != len(tables) {
		return fmt.Errorf("MySQL Schema 不完整：已有 %d/%d 张表，请人工检查迁移状态", count, len(tables))
	}
	checksum, _ := schema.Checksum("mysql")
	if count == len(tables) {
		return checkVersion(ctx, db, checksum)
	}
	if !initialize {
		return errors.New("MySQL Schema 尚未初始化，请执行 flowtide-migrate -dialect mysql -apply")
	}
	ddl, _ := schema.Load("mysql")
	// MySQL DDL 隐式提交；最后写入完成标记，失败后再次启动会拒绝接管部分 Schema。
	for i, statement := range splitStatements(ddl) {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("初始化 MySQL Schema 第 %d 条语句失败: %w", i+1, err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ACT_GE_PROPERTY (NAME_, VALUE_, REV_) VALUES (?, ?, 1)", checksumKey, checksum); err != nil {
		return fmt.Errorf("记录 MySQL Schema 完成标记失败: %w", err)
	}
	return nil
}

// checkVersion 校验版本与本工具写入的完整初始化标记。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param checksum 内置脚本校验和
// @return 校验错误
func checkVersion(ctx context.Context, db *sql.DB, checksum string) error {
	for _, expected := range []struct{ key, value string }{{"schema.version", schema.Version}, {checksumKey, checksum}} {
		var value string
		if err := db.QueryRowContext(ctx, "SELECT VALUE_ FROM ACT_GE_PROPERTY WHERE NAME_ = ?", expected.key).Scan(&value); err != nil {
			return fmt.Errorf("读取 MySQL Schema 标识 %s 失败，拒绝自动接管: %w", expected.key, err)
		}
		if value != expected.value {
			return fmt.Errorf("MySQL Schema 标识 %s 不匹配，请检查版本与迁移状态", expected.key)
		}
	}
	return nil
}

// splitStatements 拆分受控的内置 DDL；不用于存储过程或任意用户 SQL。
// @param script 内置建表脚本
// @return 去掉整行注释与空项的语句列表
func splitStatements(script string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	statements := make([]string, 0)
	// 当前脚本没有字符串中的分号或存储过程，可按分号拆分。
	for _, part := range strings.Split(strings.Join(lines, "\n"), ";") {
		if statement := strings.TrimSpace(part); statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}
