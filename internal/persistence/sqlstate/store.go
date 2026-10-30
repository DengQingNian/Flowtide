// Package sqlstate 提供基于 ACT_GE_BYTEARRAY 的 SQL 状态存储。
package sqlstate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// Result 是 SQL 执行结果。
type Result interface {
	// RowsAffected 返回影响行数。
	// @return 影响行数
	// @return 错误
	RowsAffected() (int64, error)
}

// Rows 是查询结果集。
type Rows interface {
	// Next 前进到下一行。
	// @return 是否有下一行
	Next() bool
	// Scan 扫描当前行。
	// @param dest 目标
	// @return 扫描错误
	Scan(dest ...any) error
	// Close 关闭结果集。
	// @return 关闭错误
	Close() error
	// Err 返回遍历错误。
	// @return 遍历错误
	Err() error
}

// DB 是 SQL 执行接口，便于替换真实数据库与测试替身。
type DB interface {
	// ExecContext 执行语句。
	// @param ctx 上下文
	// @param query SQL
	// @param args 参数
	// @return 结果
	// @return 错误
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
	// QueryContext 执行查询。
	// @param ctx 上下文
	// @param query SQL
	// @param args 参数
	// @return 结果集
	// @return 错误
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
}

// SQLDB 将 *sql.DB 适配为 DB 接口。
type SQLDB struct {
	DB *sql.DB // 数据库连接
}

// ExecContext 执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (s SQLDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return s.DB.ExecContext(ctx, query, args...)
}

// QueryContext 执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 错误
func (s SQLDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return s.DB.QueryContext(ctx, query, args...)
}

// Store 将引擎状态快照存入 ACT_GE_BYTEARRAY。
type Store struct {
	db      DB     // 数据库
	dialect string // 方言
}

// stateID 是状态记录固定 ID。
const stateID = "flowtide-state"

// New 创建 SQL 状态存储。
// @param db 数据库接口
// @param dialect 数据库方言
// @return 状态存储
// @return 创建错误
func New(db DB, dialect string) (*Store, error) {
	if db == nil {
		return nil, api.NewError("STATE-0001", "数据库不能为空", api.ErrBadRequest)
	}
	normalized := strings.ToLower(dialect)
	if normalized != "mysql" && normalized != "postgres" && normalized != "postgresql" {
		return nil, api.NewError("STATE-0002", "不支持的数据库方言: "+dialect, api.ErrBadRequest)
	}
	if normalized == "postgresql" {
		normalized = "postgres"
	}
	return &Store{db: db, dialect: normalized}, nil
}

// Load 读取状态。
// @param ctx 上下文
// @return 状态字节
// @return 错误
func (s *Store) Load(ctx context.Context) ([]byte, error) {
	if err := s.CheckOwner(ctx); err != nil {
		return nil, err
	}
	query := s.selectQuery()
	rows, err := s.db.QueryContext(ctx, query, stateID)
	if err != nil {
		return nil, api.NewError("STATE-0003", "查询状态失败", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, api.NewError("STATE-0004", "遍历状态失败", err)
		}
		return nil, nil
	}
	var data []byte
	if err := rows.Scan(&data); err != nil {
		return nil, api.NewError("STATE-0005", "读取状态失败", err)
	}
	return data, nil
}

// Save 保存状态。
// @param ctx 上下文
// @param data 状态字节
// @return 错误
func (s *Store) Save(ctx context.Context, data []byte) error {
	if err := s.CheckOwner(ctx); err != nil {
		return err
	}
	// MySQL 使用单条原子更新，避免删除成功后写入失败导致旧快照丢失。
	if s.dialect == "mysql" {
		query := "INSERT INTO ACT_GE_BYTEARRAY (ID_, REV_, NAME_, BYTES_, GENERATED_) VALUES (?, ?, ?, ?, ?) " +
			"ON DUPLICATE KEY UPDATE BYTES_ = ?, REV_ = REV_ + 1"
		if _, err := s.db.ExecContext(ctx, query, stateID, 1, stateID, data, false, data); err != nil {
			return api.NewError("STATE-0006", "写入状态失败", err)
		}
		return nil
	}
	query := "INSERT INTO ACT_GE_BYTEARRAY (ID_, REV_, NAME_, BYTES_, GENERATED_) VALUES ($1, $2, $3, $4, $5) " +
		"ON CONFLICT (ID_) DO UPDATE SET BYTES_ = EXCLUDED.BYTES_, REV_ = ACT_GE_BYTEARRAY.REV_ + 1"
	if _, err := s.db.ExecContext(ctx, query, stateID, 1, stateID, data, false); err != nil {
		return api.NewError("STATE-0006", "写入状态失败", err)
	}
	return nil
}

// selectQuery 返回状态查询 SQL。
// @return SQL
func (s *Store) selectQuery() string {
	return fmt.Sprintf("SELECT BYTES_ FROM ACT_GE_BYTEARRAY WHERE ID_ = %s", s.placeholder(1))
}

// placeholder 返回方言占位符。
// @param index 参数序号
// @return 占位符
func (s *Store) placeholder(index int) string {
	if s.dialect == "postgres" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}
