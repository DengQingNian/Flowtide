package main

import (
	"context"
	"fmt"

	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
	"github.com/flowable-go/flowable/internal/persistence/sqlstate"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// newStateStore 根据启动参数创建状态存储及资源清理函数。
// @param ctx 初始化上下文
// @param storage 存储类型，空值保留原有内存或文件模式
// @param statePath 原有文件路径，与 MySQL 互斥
// @param envPath MySQL 配置文件路径
// @return 状态存储，默认模式返回空值
// @return 数据库关闭函数
// @return 创建错误
func newStateStore(ctx context.Context, storage, statePath, envPath string) (statestore.Store, func() error, error) {
	if storage == "" {
		return nil, func() error { return nil }, nil
	}
	// 显式选择数据库，避免配置文件存在时改变原有部署行为。
	if storage != "mysql" {
		return nil, nil, fmt.Errorf("不支持的存储类型: %s", storage)
	}
	if statePath != "" {
		return nil, nil, fmt.Errorf("-storage mysql 与 -state 不能同时使用")
	}
	cfg, err := mysqlstore.LoadConfig(envPath)
	if err != nil {
		return nil, nil, err
	}
	db, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := mysqlstore.EnsureSchema(ctx, db, false); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	store, err := sqlstate.New(sqlstate.SQLDB{DB: db}, "mysql")
	return store, db.Close, err
}
