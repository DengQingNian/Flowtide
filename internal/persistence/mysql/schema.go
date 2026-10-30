package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
)

const (
	checksumKey         = "flowtide.schema.checksum"          // 引擎 Schema 完成标记
	workflowChecksumKey = "flowtide.workflow.schema.checksum" // workflow Schema 完成标记
)

// EnsureSchema 校验已有 Schema（引擎表与 workflow 表分别判断），或在显式允许时初始化缺失部分。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param initialize 是否允许初始化
// @return 初始化或校验错误
func EnsureSchema(ctx context.Context, db *sql.DB, initialize bool) error {
	// 引擎表（001）就绪是 workflow 表（002）的前提：完成标记写在 ACT_GE_PROPERTY 中
	if err := ensureEngineSchema(ctx, db, initialize); err != nil {
		return err
	}
	return ensureWorkflowSchema(ctx, db, initialize)
}

// ensureEngineSchema 校验或初始化 Flowable 引擎表（001 脚本），逻辑与既有升级路径保持一致。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param initialize 是否允许初始化
// @return 初始化或校验错误
func ensureEngineSchema(ctx context.Context, db *sql.DB, initialize bool) error {
	tables, err := schema.Tables("mysql")
	if err != nil {
		return err
	}
	count, err := countTables(ctx, db, tables)
	if err != nil {
		return fmt.Errorf("检查 MySQL 引擎表失败: %w", err)
	}
	// 已有部分表意味着迁移中断或外部 Schema，禁止补建后误判为可用。
	if count != 0 && count != len(tables) {
		return fmt.Errorf("MySQL Schema 不完整：已有 %d/%d 张表，请人工检查迁移状态", count, len(tables))
	}
	checksum, err := schema.Checksum("mysql")
	if err != nil {
		return err
	}
	if count == len(tables) {
		return checkVersion(ctx, db, checksum)
	}
	if !initialize {
		return errors.New("MySQL Schema 尚未初始化，请执行 flowtide-migrate -dialect mysql -apply")
	}
	ddl, err := schema.Load("mysql")
	if err != nil {
		return err
	}
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

// ensureWorkflowSchema 校验或补跑 workflow 二次封装表（002 脚本），校验和独立于引擎脚本。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param initialize 是否允许初始化
// @return 初始化或校验错误
func ensureWorkflowSchema(ctx context.Context, db *sql.DB, initialize bool) error {
	tables, err := schema.FileTables("mysql", schema.WorkflowFile)
	if err != nil {
		return err
	}
	count, err := countTables(ctx, db, tables)
	if err != nil {
		return fmt.Errorf("检查 MySQL workflow 表失败: %w", err)
	}
	// 部分表说明 002 脚本执行中断，禁止在残缺结构上继续。
	if count != 0 && count != len(tables) {
		return fmt.Errorf("MySQL workflow Schema 不完整：已有 %d/%d 张表，请人工检查迁移状态", count, len(tables))
	}
	checksum, err := schema.ChecksumFile("mysql", schema.WorkflowFile)
	if err != nil {
		return err
	}
	if count == len(tables) {
		return checkMarker(ctx, db, workflowChecksumKey, checksum)
	}
	if !initialize {
		return errors.New("MySQL workflow 表尚未初始化，请执行 flowtide-migrate -dialect mysql -apply 补齐 002 脚本")
	}
	ddl, err := schema.LoadFile("mysql", schema.WorkflowFile)
	if err != nil {
		return err
	}
	// MySQL DDL 隐式提交；最后写入完成标记，失败后再次启动会拒绝接管部分 workflow Schema。
	for i, statement := range splitStatements(ddl) {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("初始化 MySQL workflow Schema 第 %d 条语句失败: %w", i+1, err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ACT_GE_PROPERTY (NAME_, VALUE_, REV_) VALUES (?, ?, 1)", workflowChecksumKey, checksum); err != nil {
		return fmt.Errorf("记录 MySQL workflow Schema 完成标记失败: %w", err)
	}
	return nil
}

// countTables 统计数据库中已存在的指定表数量。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param tables 表名列表（大写）
// @return 已存在的表数量
// @return 查询错误
func countTables(ctx context.Context, db *sql.DB, tables []string) (int, error) {
	if len(tables) == 0 {
		return 0, nil
	}
	args := make([]any, len(tables))
	for i, table := range tables {
		args[i] = table
	}
	query := "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND UPPER(table_name) IN (" + strings.TrimSuffix(strings.Repeat("?,", len(tables)), ",") + ")"
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// checkVersion 校验版本与本工具写入的完整初始化标记。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param checksum 引擎脚本校验和
// @return 校验错误
func checkVersion(ctx context.Context, db *sql.DB, checksum string) error {
	// 先校验引擎版本与引擎标记，再由 workflow 部分校验独立标记
	if err := checkMarker(ctx, db, "schema.version", schema.Version); err != nil {
		return err
	}
	return checkMarker(ctx, db, checksumKey, checksum)
}

// checkMarker 校验 ACT_GE_PROPERTY 中的单个初始化标记。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param key 标记名
// @param expected 期望值
// @return 校验错误
func checkMarker(ctx context.Context, db *sql.DB, key, expected string) error {
	var value string
	if err := db.QueryRowContext(ctx, "SELECT VALUE_ FROM ACT_GE_PROPERTY WHERE NAME_ = ?", key).Scan(&value); err != nil {
		return fmt.Errorf("读取 MySQL Schema 标识 %s 失败，拒绝自动接管: %w", key, err)
	}
	if value != expected {
		return fmt.Errorf("MySQL Schema 标识 %s 不匹配，请检查版本与迁移状态", key)
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
