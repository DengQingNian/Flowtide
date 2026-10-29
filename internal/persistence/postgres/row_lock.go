package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/flowable-go/flowable/internal/persistence/flowable"
)

// rowLockKey 为同一数据库内的服务执行者保留固定会话锁，不用于集群仲裁。
const rowLockKey int64 = 0x466c6f7761626c65

// rowOwnerSQL 校验当前物理会话仍持有完整的单键 advisory lock。
const rowOwnerSQL = "SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND pid = pg_backend_pid() AND classid = ($1::bigint >> 32)::oid AND objid = ($1::bigint & 4294967295)::oid AND objsubid = 1 AND granted)"

// ErrRowLockLost 表示当前行仓储失去数据库独占权。
var ErrRowLockLost = errors.New("PostgreSQL 行仓储独占权已丢失")

// LockedRows 将读写及事务绑定到持锁的同一物理连接；断线后禁止透明重连写入。
type LockedRows struct {
	conn *sql.Conn // 持锁且执行所有操作的专用连接
	once sync.Once // 释放幂等
	err  error     // 首次释放结果
}

// NewLockedRows 获取数据库会话独占锁，其他服务正在运行时立即失败。
// @param ctx 初始化上下文
// @param db 数据库连接池
// @return 独占行数据库
// @return 获取错误
func NewLockedRows(ctx context.Context, db *sql.DB) (*LockedRows, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL 连接池不能为空")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取 PostgreSQL 独占连接失败: %w", err)
	}
	var granted bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", rowLockKey).Scan(&granted); err != nil || !granted {
		_ = conn.Close()
		return nil, errors.New("无法独占 PostgreSQL 数据库，可能已有服务实例")
	}
	return &LockedRows{conn: conn}, nil
}

// CheckOwner 校验当前连接的独占锁归属。
// @param ctx 上下文
// @return 连接或归属错误
func (l *LockedRows) CheckOwner(ctx context.Context) error {
	return checkOwner(ctx, l.conn)
}

// rowOwnerQuery 为连接与事务提供共同的归属查询接口。
type rowOwnerQuery interface {
	// QueryRowContext 执行单行归属查询。
	// @param ctx 上下文
	// @param query 查询语句
	// @param args 绑定参数
	// @return 单行结果
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// checkOwner 使用原会话或原事务校验独占锁，绝不借用其他池连接。
// @param ctx 上下文
// @param query 当前连接或事务
// @return 校验错误
func checkOwner(ctx context.Context, query rowOwnerQuery) error {
	var owned bool
	if err := query.QueryRowContext(ctx, rowOwnerSQL, rowLockKey).Scan(&owned); err != nil {
		return fmt.Errorf("PostgreSQL 独占连接失效: %w", err)
	}
	if !owned {
		return ErrRowLockLost
	}
	return nil
}

// ExecContext 在持锁连接执行写入。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (l *LockedRows) ExecContext(ctx context.Context, query string, args ...any) (flowable.Result, error) {
	if err := l.CheckOwner(ctx); err != nil {
		return nil, err
	}
	return l.conn.ExecContext(ctx, query, args...)
}

// QueryContext 在持锁连接执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 行结果
// @return 查询错误
func (l *LockedRows) QueryContext(ctx context.Context, query string, args ...any) (flowable.Rows, error) {
	if err := l.CheckOwner(ctx); err != nil {
		return nil, err
	}
	return l.conn.QueryContext(ctx, query, args...)
}

// BeginTx 在持锁物理连接开始事务。
// @param ctx 上下文
// @return 事务
// @return 开始错误
func (l *LockedRows) BeginTx(ctx context.Context) (flowable.Transaction, error) {
	if err := l.CheckOwner(ctx); err != nil {
		return nil, err
	}
	tx, err := l.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &lockedTransaction{tx: tx}, nil
}

// Release 释放锁后销毁专用物理连接，避免未释放锁或异常会话回池。
// @return 首次释放错误
func (l *LockedRows) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released bool
		if err := l.conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", rowLockKey).Scan(&released); err != nil {
			l.err = fmt.Errorf("释放 PostgreSQL 独占锁失败: %w", err)
		} else if !released {
			l.err = ErrRowLockLost
		}
		// 即使释放查询失败也销毁物理连接，由数据库终止会话并释放会话锁。
		_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = l.conn.Close()
	})
	return l.err
}

// lockedTransaction 绑定持锁连接上的事务，提交前在事务内核验归属。
type lockedTransaction struct{ tx *sql.Tx }

// ExecContext 执行事务写入。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (t *lockedTransaction) ExecContext(ctx context.Context, query string, args ...any) (flowable.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

// Commit 校验锁归属后提交，校验失败时回滚。
// @return 提交错误
func (t *lockedTransaction) Commit() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := checkOwner(ctx, t.tx); err != nil {
		_ = t.tx.Rollback()
		return err
	}
	return t.tx.Commit()
}

// Rollback 回滚事务，无需仍持有独占锁。
// @return 回滚错误
func (t *lockedTransaction) Rollback() error { return t.tx.Rollback() }
