package postgres

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
	tables, err := schema.Tables("postgres")
	if err != nil {
		return err
	}
	count, err := countTables(ctx, db, tables)
	if err != nil {
		return fmt.Errorf("检查 PostgreSQL 引擎表失败: %w", err)
	}
	// 已有部分表意味着迁移中断或外部 Schema，禁止补建后误判为可用。
	if count != 0 && count != len(tables) {
		return fmt.Errorf("PostgreSQL Schema 不完整：已有 %d/%d 张表，请人工检查迁移状态", count, len(tables))
	}
	checksum, err := schema.Checksum("postgres")
	if err != nil {
		return err
	}
	if count == len(tables) {
		return checkVersion(ctx, db, checksum)
	}
	if !initialize {
		return errors.New("PostgreSQL Schema 尚未初始化，请执行 flowtide-migrate -dialect postgres -apply")
	}
	ddl, err := schema.Load("postgres")
	if err != nil {
		return err
	}
	// PostgreSQL 建表与完成标记在同一事务提交，任何失败均回滚。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 PostgreSQL Schema 事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for i, statement := range splitStatements(ddl) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("初始化 PostgreSQL Schema 第 %d 条语句失败: %w", i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ACT_GE_PROPERTY (NAME_, VALUE_, REV_) VALUES ($1, $2, 1)", checksumKey, checksum); err != nil {
		return fmt.Errorf("记录 PostgreSQL Schema 完成标记失败: %w", err)
	}
	return tx.Commit()
}

// ensureWorkflowSchema 校验或补跑 workflow 二次封装表（002 脚本），校验和独立于引擎脚本。
// @param ctx 操作上下文
// @param db 数据库连接池
// @param initialize 是否允许初始化
// @return 初始化或校验错误
func ensureWorkflowSchema(ctx context.Context, db *sql.DB, initialize bool) error {
	tables, err := schema.FileTables("postgres", schema.WorkflowFile)
	if err != nil {
		return err
	}
	count, err := countTables(ctx, db, tables)
	if err != nil {
		return fmt.Errorf("检查 PostgreSQL workflow 表失败: %w", err)
	}
	// 部分表说明 002 脚本执行中断，禁止在残缺结构上继续。
	if count != 0 && count != len(tables) {
		return fmt.Errorf("PostgreSQL workflow Schema 不完整：已有 %d/%d 张表，请人工检查迁移状态", count, len(tables))
	}
	checksum, err := schema.ChecksumFile("postgres", schema.WorkflowFile)
	if err != nil {
		return err
	}
	if count == len(tables) {
		return checkMarker(ctx, db, workflowChecksumKey, checksum)
	}
	if !initialize {
		return errors.New("PostgreSQL workflow 表尚未初始化，请执行 flowtide-migrate -dialect postgres -apply 补齐 002 脚本")
	}
	ddl, err := schema.LoadFile("postgres", schema.WorkflowFile)
	if err != nil {
		return err
	}
	// PostgreSQL workflow 建表与完成标记在同一事务提交，任何失败均回滚。
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 PostgreSQL workflow Schema 事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for i, statement := range splitStatements(ddl) {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("初始化 PostgreSQL workflow Schema 第 %d 条语句失败: %w", i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ACT_GE_PROPERTY (NAME_, VALUE_, REV_) VALUES ($1, $2, 1)", workflowChecksumKey, checksum); err != nil {
		return fmt.Errorf("记录 PostgreSQL workflow Schema 完成标记失败: %w", err)
	}
	return tx.Commit()
}

// countTables 统计当前 Schema 中已存在的指定表数量。
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
	placeholders := make([]string, len(tables))
	for i, table := range tables {
		args[i] = table
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	query := "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND UPPER(table_name) IN (" + strings.Join(placeholders, ",") + ")"
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
	if err := db.QueryRowContext(ctx, "SELECT VALUE_ FROM ACT_GE_PROPERTY WHERE NAME_ = $1", key).Scan(&value); err != nil {
		return fmt.Errorf("读取 PostgreSQL Schema 标识 %s 失败，拒绝自动接管: %w", key, err)
	}
	if value != expected {
		return fmt.Errorf("PostgreSQL Schema 标识 %s 不匹配，请检查版本与迁移状态", key)
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
