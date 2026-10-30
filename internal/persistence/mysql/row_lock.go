package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	"github.com/DengQingNian/Flowtide/internal/persistence/sqlstate"
)

// snapshotStateName 是快照模式在 ACT_GE_BYTEARRAY 中的固定 NAME_。
const snapshotStateName = "flowtide-state"

// lockOperationTimeout 是校验或释放独占锁的超时时间。
const lockOperationTimeout = 5 * time.Second

// ErrRowLockLost 表示行仓储独占权已不属于当前连接。
var ErrRowLockLost = errors.New("行仓储独占权已丢失，拒绝继续读写")

// LockedRows 将行仓储读写绑定到数据库独占锁，并在每次操作前校验锁归属。
type LockedRows struct {
	db   *sql.DB   // 执行行读写的连接池
	conn *sql.Conn // 持有独占锁的专用连接
	name string    // 独占锁名称
	once sync.Once // 保证释放只执行一次
	err  error     // 释放错误
}

// 编译期确认包装满足行仓储所需的全部数据库接口。
var (
	_ flowable.DB              = (*LockedRows)(nil)
	_ flowable.QueryDB         = (*LockedRows)(nil)
	_ flowable.TransactionalDB = (*LockedRows)(nil)
	_ flowable.Transaction     = (*lockedTransaction)(nil)
)

// NewLockedRows 获取行仓储数据库独占锁；其他实例占用时立即拒绝启动。
// @param ctx 初始化上下文
// @param db 数据库连接池
// @param namespace 数据库名称，用于生成与快照模式一致的锁名
// @return 绑定了独占锁的行数据库
// @return 获取错误
func NewLockedRows(ctx context.Context, db *sql.DB, namespace string) (*LockedRows, error) {
	// 连接池缺失时无法借出持锁连接。
	if db == nil {
		return nil, errors.New("行仓储数据库连接池不能为空")
	}
	conn, err := db.Conn(ctx)
	// 取连接失败通常意味着数据库不可用或上下文已取消。
	if err != nil {
		return nil, fmt.Errorf("获取行仓储独占连接失败: %w", err)
	}
	name := sqlstate.LockName(namespace)
	var granted sql.NullInt64
	// 非阻塞取锁，已有快照或行模式实例运行时直接失败。
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&granted); err != nil || !granted.Valid || granted.Int64 != 1 {
		_ = conn.Close()
		return nil, errors.New("无法独占数据库，可能已有快照或行模式服务实例")
	}
	return &LockedRows{db: db, conn: conn, name: name}, nil
}

// CheckOwner 确认执行行读写的会话仍持有独占权。
// @param ctx 上下文
// @return 所有权或连接错误
func (l *LockedRows) CheckOwner(ctx context.Context) error {
	var owned sql.NullBool
	if err := l.conn.QueryRowContext(ctx, "SELECT IS_USED_LOCK(?) = CONNECTION_ID()", l.name).Scan(&owned); err != nil {
		return fmt.Errorf("行仓储独占连接失效: %w", err)
	}
	// 锁归属其他连接或已被释放时禁止继续读写，避免双写。
	if !owned.Valid || !owned.Bool {
		return ErrRowLockLost
	}
	return nil
}

// ExecContext 校验独占权后执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (l *LockedRows) ExecContext(ctx context.Context, query string, args ...any) (flowable.Result, error) {
	if err := l.CheckOwner(ctx); err != nil {
		return nil, err
	}
	return l.db.ExecContext(ctx, query, args...)
}

// QueryContext 校验独占权后执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 查询结果
// @return 查询错误
func (l *LockedRows) QueryContext(ctx context.Context, query string, args ...any) (flowable.Rows, error) {
	if err := l.CheckOwner(ctx); err != nil {
		return nil, err
	}
	return l.db.QueryContext(ctx, query, args...)
}

// BeginTx 校验独占权后开始事务。
// @param ctx 上下文
// @return 事务
// @return 创建错误
func (l *LockedRows) BeginTx(ctx context.Context) (flowable.Transaction, error) {
	if err := l.CheckOwner(ctx); err != nil {
		return nil, err
	}
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &lockedTransaction{tx: tx, lock: l}, nil
}

// Release 释放独占锁并关闭持锁连接，重复调用返回同一结果。
// @return 释放或关闭错误
func (l *LockedRows) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), lockOperationTimeout)
		defer cancel()
		var released sql.NullInt64
		err := l.conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", l.name).Scan(&released)
		// 释放失败或锁已不归本连接时销毁物理连接，禁止带锁会话回到连接池。
		if err != nil {
			l.err = fmt.Errorf("释放行仓储独占锁失败: %w", err)
			_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
		} else if !released.Valid || released.Int64 != 1 {
			l.err = fmt.Errorf("释放行仓储独占锁失败: %w", ErrRowLockLost)
			_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		if closeErr := l.conn.Close(); l.err == nil && closeErr != nil {
			l.err = fmt.Errorf("关闭行仓储独占连接失败: %w", closeErr)
		}
	})
	return l.err
}

// lockedTransaction 是校验独占权后提交的数据库事务。
type lockedTransaction struct {
	tx   *sql.Tx     // 底层事务
	lock *LockedRows // 独占锁
}

// ExecContext 在事务内执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (t *lockedTransaction) ExecContext(ctx context.Context, query string, args ...any) (flowable.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

// Commit 校验独占权后提交事务；锁丢失时回滚并返回错误。
// @return 提交错误
func (t *lockedTransaction) Commit() error {
	ctx, cancel := context.WithTimeout(context.Background(), lockOperationTimeout)
	defer cancel()
	// 锁已丢失时回滚，禁止把来源不明的写入提交到数据库。
	if err := t.lock.CheckOwner(ctx); err != nil {
		_ = t.tx.Rollback()
		return err
	}
	return t.tx.Commit()
}

// Rollback 回滚事务，不依赖独占锁是否仍然有效。
// @return 回滚错误
func (t *lockedTransaction) Rollback() error {
	return t.tx.Rollback()
}

// CheckNoSnapshot 拒绝在包含快照状态的数据上启用行仓储。
// @param ctx 上下文
// @param db 数据库连接池
// @return 检查错误
func CheckNoSnapshot(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_GE_BYTEARRAY WHERE NAME_ = ?", snapshotStateName).Scan(&count); err != nil {
		return fmt.Errorf("检查快照状态失败: %w", err)
	}
	// 快照与行仓储不能共享同一权威来源，发现快照记录时拒绝启动。
	if count != 0 {
		return errors.New("数据库包含 flowtide-state 快照数据，-storage mysql-rows 不能与快照模式混用")
	}
	return nil
}
