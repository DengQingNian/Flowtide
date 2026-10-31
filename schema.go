package flowtide

import (
	"context"
	"database/sql"
	"errors"

	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
)

// EnsureMySQLSchema 只读校验完整引擎及 workflow 结构，仅 initialize=true 时允许显式初始化。
// @param ctx 有界操作上下文
// @param db 调用方拥有的 MySQL 连接
// @param initialize 是否明确允许初始化，已有部分表或未知标记仍拒绝接管
// @return 结构检查或初始化错误
func EnsureMySQLSchema(ctx context.Context, db *sql.DB, initialize bool) error {
	if db == nil {
		return errors.New("结构检查的数据库连接不能为空")
	}
	return mysqlstore.EnsureSchema(ctx, db, initialize)
}
