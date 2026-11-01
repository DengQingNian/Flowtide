package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
	"github.com/DengQingNian/Flowtide/internal/persistence/sqlstate"
	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// storageSetup 是启动参数解析后的存储初始化结果。
type storageSetup struct {
	Store      statestore.Store        // 快照存储；行模式为空
	Repository *flowablerow.Repository // 按表行仓储；快照模式为空
	LoadRows   bool                    // 启动时是否从行仓储恢复
	WFStore    *pwf.Store              // workflow 行仓储入口（/workflow/** SQL 模式；nil 时 workflow 走内存）
	once       sync.Once               // 保证资源释放只执行一次
	closeFn    func() error            // 释放独占锁并关闭连接池
	closeErr   error                   // 首次释放结果
}

// parseStorageMode 校验存储类型及与状态文件的互斥关系。
// @param storage 存储类型，空值表示沿用内存或文件模式
// @param statePath 状态文件路径
// @return 是否为 MySQL 行仓储模式
// @return 配置错误
func parseStorageMode(storage, statePath string) (bool, error) {
	switch storage {
	case "":
		return false, nil
	case "mysql":
		// 快照模式保持原有行为，但仍不允许与状态文件同时配置。
		if statePath != "" {
			return false, errors.New("-storage mysql 与 -state 不能同时使用")
		}
		return false, nil
	case "mysql-rows", "postgres-rows":
		// 行仓储与状态文件同样只能选择一个权威来源。
		if statePath != "" {
			return false, fmt.Errorf("-storage %s 与 -state 不能同时使用", storage)
		}
		return true, nil
	default:
		return false, fmt.Errorf("不支持的存储类型: %s", storage)
	}
}

// newStorage 根据启动参数初始化快照存储或按表行仓储。
// @param ctx 初始化上下文
// @param storage 存储类型，空值保留原有内存或文件模式
// @param statePath 原有状态文件路径，与 MySQL 模式互斥
// @param envPath MySQL 配置文件路径
// @return 存储初始化结果
// @return 初始化错误
func newStorage(ctx context.Context, storage, statePath, envPath string) (*storageSetup, error) {
	rowMode, err := parseStorageMode(storage, statePath)
	if err != nil {
		return nil, err
	}
	// 未显式选择数据库时保留原有内存或文件模式，不读取配置文件。
	if storage == "" {
		return &storageSetup{}, nil
	}
	if storage == "postgres-rows" {
		return newPostgresStorage(ctx, envPath)
	}
	cfg, err := mysqlstore.LoadConfig(envPath)
	if err != nil {
		return nil, err
	}
	db, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := mysqlstore.EnsureSchema(ctx, db, false); err != nil {
		_ = db.Close()
		return nil, err
	}
	if rowMode {
		return newRowStorage(ctx, db, cfg.Database)
	}
	store, release, err := sqlstate.NewExclusiveMySQL(ctx, db, cfg.Database)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &storageSetup{Store: store, closeFn: func() error {
		releaseErr := release()
		closeErr := db.Close()
		if releaseErr != nil {
			return releaseErr
		}
		return closeErr
	}}, nil
}

// newPostgresStorage 校验 PostgreSQL Schema 并初始化绑定会话独占锁的行仓储。
// @param ctx 初始化上下文
// @param envPath 配置文件路径
// @return 存储配置
// @return 配置、Schema、锁或仓储错误
func newPostgresStorage(ctx context.Context, envPath string) (*storageSetup, error) {
	dsn, err := pgstore.LoadDSN(envPath)
	if err != nil {
		return nil, err
	}
	db, err := pgstore.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pgstore.EnsureSchema(ctx, db, false); err != nil {
		_ = db.Close()
		return nil, err
	}
	return newPostgresRowStorage(ctx, db)
}

// newPostgresRowStorage 在已校验数据库上取独占锁并拒绝混用快照数据。
// @param ctx 初始化上下文
// @param db 已校验连接池
// @return 行仓储初始化结果
// @return 初始化错误
func newPostgresRowStorage(ctx context.Context, db *sql.DB) (*storageSetup, error) {
	locked, err := pgstore.NewLockedRows(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	cleanup := func() error { return errors.Join(locked.Release(), db.Close()) }
	rows, err := locked.QueryContext(ctx, "SELECT COUNT(*) FROM ACT_GE_BYTEARRAY WHERE NAME_ = $1", "flowtide-state")
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	var count int
	if !rows.Next() {
		err = errors.New("查询 PostgreSQL 快照标记失败")
	} else {
		err = rows.Scan(&count)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	if count != 0 {
		return nil, errors.Join(errors.New("PostgreSQL 包含快照状态，不能启用行仓储"), cleanup())
	}
	repository, err := flowablerow.NewRepository(locked, "postgres")
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	// workflow 行仓储与引擎行仓储共用连接池（closeFn 统一释放）
	wfStore, wfErr := pwf.NewStore(db, "postgres")
	if wfErr != nil {
		return nil, errors.Join(wfErr, cleanup())
	}
	return &storageSetup{Repository: repository, LoadRows: true, WFStore: wfStore, closeFn: cleanup}, nil
}

// newRowStorage 在已校验 Schema 的连接池上建立行仓储及其独占锁。
// @param ctx 初始化上下文
// @param db 已校验的数据库连接池
// @param database 数据库名称，用于生成与快照模式一致的独占锁
// @return 行仓储初始化结果
// @return 初始化错误
func newRowStorage(ctx context.Context, db *sql.DB, database string) (*storageSetup, error) {
	locked, err := mysqlstore.NewLockedRows(ctx, db, database)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	// 先销毁持锁连接再关闭连接池，避免把带锁会话归还池中。
	cleanup := func() error {
		releaseErr := locked.Release()
		closeErr := db.Close()
		if releaseErr != nil {
			return releaseErr
		}
		return closeErr
	}
	// 持锁后再检查快照标记，防止检查与快照实例启动交错。
	if err := mysqlstore.CheckNoSnapshot(ctx, db); err != nil {
		return nil, errors.Join(err, cleanup())
	}
	repository, err := flowablerow.NewRepository(locked, "mysql")
	if err != nil {
		return nil, errors.Join(err, cleanup())
	}
	// workflow 行仓储与引擎行仓储共用连接池（closeFn 统一释放）
	wfStore, wfErr := pwf.NewStore(db, "mysql")
	if wfErr != nil {
		return nil, errors.Join(wfErr, cleanup())
	}
	return &storageSetup{Repository: repository, LoadRows: true, WFStore: wfStore, closeFn: cleanup}, nil
}

// Close 释放独占锁并关闭连接池，重复调用返回首次结果。
// @return 释放或关闭错误
func (s *storageSetup) Close() error {
	if s == nil || s.closeFn == nil {
		return nil
	}
	s.once.Do(func() { s.closeErr = s.closeFn() })
	return s.closeErr
}
