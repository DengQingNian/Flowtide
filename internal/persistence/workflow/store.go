// Package workflow 提供 workflow_* 二次封装表的方言感知行仓储（database/sql，不引 ORM）。
// 引擎 ACT_* 表的行仓储见 internal/persistence/flowable，二者相互独立；
// 本包不生成 ID（由调用方经 internal/idgen 生成后传入），仅自动填充 create_time/update_time。
package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// 支持的数据库方言。
const (
	// DialectMySQL 是 MySQL 方言（"?" 占位符）。
	DialectMySQL = "mysql"
	// DialectPostgres 是 PostgreSQL 方言（"$n" 占位符）。
	DialectPostgres = "postgres"
)

// Row 是一行数据的通用载体：键为列名（与 DDL 一致的小写下划线名），值为可入库的 Go 值。
type Row map[string]any

// Clock 是仓储的统一时间源，测试可注入固定时钟。
type Clock interface {
	// Now 返回当前时间。
	// @return 当前时间
	Now() time.Time
}

// ClockFunc 将函数适配为时间源。
type ClockFunc func() time.Time

// Now 返回当前时间。
// @return 当前时间
func (f ClockFunc) Now() time.Time { return f() }

// realClock 使用系统时间。
type realClock struct{}

// Now 返回系统当前时间。
// @return 当前时间
func (realClock) Now() time.Time { return time.Now() }

// Store 是 workflow_* 表仓储入口，持有连接池、方言与时间源。
type Store struct {
	db      *sql.DB // 数据库连接池
	dialect string  // 方言（小写）
	clock   Clock   // 时间源
}

// Option 是 Store 的可选配置。
type Option func(*Store)

// WithClock 注入时间源。
// @param clock 时间源
// @return 配置项
func WithClock(clock Clock) Option {
	return func(store *Store) {
		if clock != nil {
			store.clock = clock
		}
	}
}

// NewStore 创建 workflow 仓储入口。
// @param db 数据库连接池（与引擎共享同一连接池）
// @param dialect 数据库方言（mysql/postgres）
// @param options 可选配置
// @return 仓储入口
// @return 方言非法或 db 为空错误
func NewStore(db *sql.DB, dialect string, options ...Option) (*Store, error) {
	if db == nil {
		return nil, api.NewError("WF-0001", "数据库连接不能为空", api.ErrBadRequest)
	}
	lower := strings.ToLower(strings.TrimSpace(dialect))
	if lower != DialectMySQL && lower != DialectPostgres {
		return nil, api.NewError("WF-0002", fmt.Sprintf("不支持的数据库方言: %s", dialect), api.ErrBadRequest)
	}
	store := &Store{db: db, dialect: lower, clock: realClock{}}
	for _, option := range options {
		option(store)
	}
	return store, nil
}

// Dialect 返回仓储方言。
// @return 方言名
func (s *Store) Dialect() string { return s.dialect }

// Now 返回仓储统一时间源的当前时间。
// @return 当前时间
func (s *Store) Now() time.Time { return s.clock.Now() }

// ph 返回第 n 个占位符（n 从 1 开始）。
// @param n 序号
// @return 占位符文本
func (s *Store) ph(n int) string {
	// MySQL 全部使用 "?"，PostgreSQL 使用 "$n" 位置参数
	if s.dialect == DialectMySQL {
		return "?"
	}
	return "$" + strconv.Itoa(n)
}

// phList 返回 count 个逗号分隔的占位符（如 "$1, $2" 或 "?, ?"）。
// @param count 数量
// @return 占位符列表文本
func (s *Store) phList(count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = s.ph(i + 1)
	}
	return strings.Join(parts, ", ")
}

// txKey 是携带事务的上下文键。
type txKey struct{}

// WithTx 在独立事务中执行 fn，fn 通过 ctx 获得同一事务实现多表原子提交；
// fn 返回错误或 panic 时整体回滚，否则提交。已在事务中时直接复用当前事务（不嵌套开启）。
// @param ctx 操作上下文
// @param fn 事务体逻辑
// @return fn 的错误或事务开启/提交错误
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return api.NewError("WF-0003", "事务函数不能为空", api.ErrBadRequest)
	}
	// 已处于事务中时复用当前事务，由最外层负责提交与回滚
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始 workflow 事务失败: %w", err)
	}
	committed := false
	defer func() {
		// 未提交（错误或 panic）时回滚，保证工作单元原子性
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交 workflow 事务失败: %w", err)
	}
	committed = true
	return nil
}

// executor 是仓储执行器抽象，*sql.DB 与 *sql.Tx 均满足该接口。
type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// executorFrom 解析当前执行器：ctx 携带事务时返回事务，否则返回连接池。
// @param ctx 操作上下文
// @return 执行器
func (s *Store) executorFrom(ctx context.Context) executor {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.db
}
