package sqlstate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"
)

// exclusiveMySQLDB 将快照与 MySQL 会话独占锁绑定到同一物理连接。
type exclusiveMySQLDB struct {
	conn *sql.Conn // 持有锁的连接
	name string    // 数据库独占锁名称
}

// NewExclusiveMySQL 获取快照独占连接；其他进程占用时立即拒绝启动。
// @param ctx 初始化上下文
// @param db 数据库连接池
// @param namespace 数据库名称
// @return 快照存储
// @return 幂等释放函数
// @return 获取错误
func NewExclusiveMySQL(ctx context.Context, db *sql.DB, namespace string) (*Store, func() error, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("获取快照独占连接失败: %w", err)
	}
	name := fmt.Sprintf("flowable-go:%x", sha256.Sum256([]byte(namespace)))[:64]
	var granted sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&granted); err != nil || !granted.Valid || granted.Int64 != 1 {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("无法独占数据库快照，可能已有其他服务实例")
	}
	store, _ := New(exclusiveMySQLDB{conn: conn, name: name}, "mysql")
	var once sync.Once
	var releaseErr error
	release := func() error {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, releaseErr = conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", name)
			// 释放失败时销毁物理连接，禁止把带锁会话放回连接池。
			if releaseErr != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			if err := conn.Close(); releaseErr == nil {
				releaseErr = err
			}
		})
		return releaseErr
	}
	return store, release, nil
}

// CheckOwner 确认执行快照写入的物理会话仍持有独占权。
// @param ctx 上下文
// @return 所有权或连接错误
func (d exclusiveMySQLDB) CheckOwner(ctx context.Context) error {
	var owned sql.NullBool
	if err := d.conn.QueryRowContext(ctx, "SELECT IS_USED_LOCK(?) = CONNECTION_ID()", d.name).Scan(&owned); err != nil {
		return fmt.Errorf("快照独占连接失效: %w", err)
	}
	if !owned.Valid || !owned.Bool {
		return fmt.Errorf("快照独占权已丢失")
	}
	return nil
}

// ExecContext 在持锁连接执行写入。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (d exclusiveMySQLDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return d.conn.ExecContext(ctx, query, args...)
}

// QueryContext 在持锁连接读取状态。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 查询结果
// @return 查询错误
func (d exclusiveMySQLDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return d.conn.QueryContext(ctx, query, args...)
}

// CheckOwner 在底层具备独占会话时验证其所有权。
// @param ctx 上下文
// @return 所有权错误
func (s *Store) CheckOwner(ctx context.Context) error {
	if owner, ok := s.db.(interface{ CheckOwner(context.Context) error }); ok {
		return owner.CheckOwner(ctx)
	}
	return nil
}
