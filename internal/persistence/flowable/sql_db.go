package flowable

import (
	"context"
	"database/sql"
)

// SQLDB 为真实数据库提供查询和命令级事务适配。
type SQLDB struct {
	DB *sql.DB // 数据库连接池
}

// ExecContext 执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (d SQLDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return d.DB.ExecContext(ctx, query, args...)
}

// QueryContext 查询行。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 查询错误
func (d SQLDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return d.DB.QueryContext(ctx, query, args...)
}

// BeginTx 开始一条命令的事务。
// @param ctx 上下文
// @return 事务
// @return 创建错误
func (d SQLDB) BeginTx(ctx context.Context) (Transaction, error) {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlTransaction{tx}, nil
}

// sqlTransaction 将真实事务适配为行仓储事务。
type sqlTransaction struct{ *sql.Tx }

// ExecContext 在事务内执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (t sqlTransaction) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return t.Tx.ExecContext(ctx, query, args...)
}
