// Package migration 提供既有 Java 数据库的只读迁移预检。
package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/flowable-go/flowable/internal/persistence/flowable"
)

// SourceSnapshot 保存同一只读事务取得的源库结构和行数据；行数据不参与报告序列化。
type SourceSnapshot struct {
	Dialect       string         `json:"dialect"`       // 数据库方言
	SchemaVersion string         `json:"schemaVersion"` // 源库版本
	Tables        []TableSummary `json:"tables"`        // 各表统计
	Rows          []flowable.Row `json:"-"`             // 仅供进程内分析的行
}

// TableSummary 表示一张源表的实际行数。
type TableSummary struct {
	Name  string `json:"name"`  // 规范化的大写表名
	Count int64  `json:"count"` // 源行数
}

var sourceIdentifier = regexp.MustCompile(`(?i)^ACT_[A-Z0-9_]+$`)

// Scan 在可重复读的只读事务内扫描既有库，事务结束始终回滚且不执行任何初始化。
// @param ctx 扫描上下文
// @param db 已连接的源数据库
// @param dialect 数据库方言，仅支持 mysql 和 postgres
// @return 完整快照，失败时为空快照
// @return 连接、查询、读取或回滚错误
func Scan(ctx context.Context, db *sql.DB, dialect string) (snapshot SourceSnapshot, err error) {
	if db == nil {
		return SourceSnapshot{}, errors.New("预检数据库不能为空")
	}
	if dialect != "mysql" && dialect != "postgres" {
		return SourceSnapshot{}, errors.New("预检仅支持 mysql 或 postgres")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("开始只读预检事务失败: %w", err)
	}
	// 无论查询成功与否均结束事务；任何错误都不得返回不完整的预检数据。
	defer func() {
		err = errors.Join(err, tx.Rollback())
		if err != nil {
			snapshot = SourceSnapshot{}
		}
	}()
	names, err := sourceTables(ctx, tx, dialect)
	if err != nil {
		return SourceSnapshot{}, err
	}
	known := make(map[string]bool)
	for _, name := range flowable.Tables() {
		known[name] = true
	}
	for _, name := range []string{"ACT_RU_ACTINST", "ACT_RU_TIMER_JOB", "ACT_RU_SUSPENDED_JOB", "ACT_RU_EXTERNAL_JOB", "ACT_RU_HISTORY_JOB", "ACT_HI_JOB_LOG"} {
		known[name] = true
	}
	snapshot = SourceSnapshot{Dialect: dialect, Tables: make([]TableSummary, 0, len(names)), Rows: make([]flowable.Row, 0)}
	for _, name := range names {
		canonical := strings.ToUpper(name)
		quoted := "`" + name + "`"
		if dialect == "postgres" {
			quoted = `"` + name + `"`
		}
		var count int64
		// 已支持表保留实际列供分析，未知表只统计，避免载入未支持引擎的数据内容。
		if known[canonical] {
			rows, readErr := sourceRows(ctx, tx, canonical, quoted)
			if readErr != nil {
				return SourceSnapshot{}, readErr
			}
			count = int64(len(rows))
			snapshot.Rows = append(snapshot.Rows, rows...)
		} else if readErr := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoted).Scan(&count); readErr != nil {
			return SourceSnapshot{}, fmt.Errorf("统计源表 %s 失败: %w", canonical, readErr)
		}
		snapshot.Tables = append(snapshot.Tables, TableSummary{Name: canonical, Count: count})
	}
	for _, row := range snapshot.Rows {
		if row.Table == "ACT_GE_PROPERTY" && row.Values["NAME_"] == "schema.version" {
			snapshot.SchemaVersion, _ = row.Values["VALUE_"].(string)
		}
	}
	return snapshot, nil
}

// sourceTables 发现当前数据库或模式中的源表并拒绝不安全或规范化后冲突的名称。
// @param ctx 扫描上下文
// @param tx 只读事务
// @param dialect 数据库方言
// @return 数据库实际表名
// @return 查询、扫描、关闭或名称校验错误
func sourceTables(ctx context.Context, tx *sql.Tx, dialect string) (names []string, err error) {
	schema := "DATABASE()"
	if dialect == "postgres" {
		schema = "current_schema()"
	}
	query := "SELECT table_name FROM information_schema.tables WHERE table_schema = " + schema + " AND table_type = 'BASE TABLE' AND LEFT(UPPER(table_name), 4) = 'ACT_' ORDER BY table_name"
	result, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("发现源库表失败: %w", err)
	}
	defer func() { err = errors.Join(err, result.Close()) }()
	seen := make(map[string]bool)
	for result.Next() {
		var name string
		if err := result.Scan(&name); err != nil {
			return nil, fmt.Errorf("读取源表名失败: %w", err)
		}
		normalized := strings.ToUpper(name)
		// 标识符必须来自受限字符集合，且同一规范表名只能对应一张实际表。
		if !sourceIdentifier.MatchString(name) || seen[normalized] {
			return nil, errors.New("源库包含不安全或大小写冲突的 ACT 表名")
		}
		seen[normalized] = true
		names = append(names, name)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("遍历源表清单失败: %w", err)
	}
	return names, nil
}

// sourceRows 读取一张已支持表的实际列，复制驱动字节并统一列名。
// @param ctx 扫描上下文
// @param tx 只读事务
// @param canonical 规范表名
// @param quoted 已校验并引用的实际表名
// @return 读取的行
// @return 查询、列元数据、扫描、遍历或关闭错误
func sourceRows(ctx context.Context, tx *sql.Tx, canonical, quoted string) (rows []flowable.Row, err error) {
	result, err := tx.QueryContext(ctx, "SELECT * FROM "+quoted)
	if err != nil {
		return nil, fmt.Errorf("读取源表 %s 失败: %w", canonical, err)
	}
	defer func() { err = errors.Join(err, result.Close()) }()
	columns, err := result.Columns()
	if err != nil {
		return nil, fmt.Errorf("读取源表列名失败: %w", err)
	}
	for result.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range targets {
			targets[i] = &values[i]
		}
		if err := result.Scan(targets...); err != nil {
			return nil, fmt.Errorf("扫描源表 %s 失败: %w", canonical, err)
		}
		row := flowable.Row{Table: canonical, Values: make(map[string]any, len(columns))}
		for i, column := range columns {
			column = strings.ToUpper(column)
			// 普通文本按字符串归一化，二进制内容保留副本，避免复用驱动缓冲区。
			value := values[i]
			if data, ok := value.([]byte); ok {
				if column == "BYTES_" || column == "FULL_MSG_" {
					value = append([]byte{}, data...)
				} else {
					value = string(data)
				}
			}
			row.Values[column] = value
		}
		rows = append(rows, row)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("遍历源表 %s 失败: %w", canonical, err)
	}
	return rows, nil
}
