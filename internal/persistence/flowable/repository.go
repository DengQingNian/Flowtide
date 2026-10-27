package flowable

import (
	"context"

	"github.com/flowable-go/flowable/api"
)

// Result 是 SQL 执行结果。
type Result interface {
	// RowsAffected 返回影响行数。
	// @return 影响行数
	// @return 错误
	RowsAffected() (int64, error)
}

// DB 是行写入接口。
type DB interface {
	// ExecContext 执行 SQL。
	// @param ctx 上下文
	// @param query SQL
	// @param args 参数
	// @return 结果
	// @return 错误
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
}

// Repository 提供 ACT 行的插入、更新与删除。
type Repository struct {
	db      DB     // 数据库
	dialect string // 方言
}

// NewRepository 创建行仓储。
// @param db 数据库
// @param dialect 方言
// @return 仓储
// @return 创建错误
func NewRepository(db DB, dialect string) (*Repository, error) {
	if db == nil {
		return nil, api.NewError("ROW-0008", "数据库不能为空", api.ErrBadRequest)
	}
	if err := validateDialect(dialect); err != nil {
		return nil, err
	}
	return &Repository{db: db, dialect: dialect}, nil
}

// Insert 插入一行。
// @param ctx 上下文
// @param row 行
// @return 影响行数
// @return 插入错误
func (r *Repository) Insert(ctx context.Context, row Row) (int64, error) {
	query, args, err := row.InsertSQL(r.dialect)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0009", "插入行失败", err)
	}
	return result.RowsAffected()
}

// Update 更新一行并校验 REV_。
// @param ctx 上下文
// @param row 行
// @param oldRevision 旧版本
// @return 影响行数
// @return 更新错误
func (r *Repository) Update(ctx context.Context, row Row, oldRevision int64) (int64, error) {
	query, args, err := row.UpdateSQL(r.dialect, oldRevision)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0010", "更新行失败", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, api.NewError("ROW-0011", "读取更新行数失败", err)
	}
	if affected == 0 {
		return 0, api.NewError("ROW-0012", "乐观锁冲突", api.ErrConflict)
	}
	return affected, nil
}

// Delete 删除一行。
// @param ctx 上下文
// @param row 行
// @return 影响行数
// @return 删除错误
func (r *Repository) Delete(ctx context.Context, row Row) (int64, error) {
	query, args, err := row.DeleteSQL(r.dialect)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0013", "删除行失败", err)
	}
	return result.RowsAffected()
}
