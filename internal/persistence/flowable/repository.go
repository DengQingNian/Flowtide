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

// Transaction 是数据库事务。
type Transaction interface {
	DB
	// Commit 提交事务。
	// @return 提交错误
	Commit() error
	// Rollback 回滚事务。
	// @return 回滚错误
	Rollback() error
}

// TransactionalDB 是支持事务的数据库。
type TransactionalDB interface {
	DB
	// BeginTx 开始事务。
	// @param ctx 上下文
	// @return 事务
	// @return 错误
	BeginTx(ctx context.Context) (Transaction, error)
}

// 操作类型。
const (
	OperationInsert = "insert" // 插入
	OperationUpdate = "update" // 更新
	OperationDelete = "delete" // 删除
)

// Operation 是行操作。
type Operation struct {
	Kind        string // 类型
	Row         Row    // 行
	OldRevision int64  // 更新时旧版本
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

// Apply 在事务中执行一组行操作。
// @param ctx 上下文
// @param operations 操作列表
// @return 执行错误
func (r *Repository) Apply(ctx context.Context, operations []Operation) error {
	run := func(repository *Repository) error {
		for _, operation := range operations {
			var err error
			switch operation.Kind {
			case OperationInsert:
				_, err = repository.Insert(ctx, operation.Row)
			case OperationUpdate:
				_, err = repository.Update(ctx, operation.Row, operation.OldRevision)
			case OperationDelete:
				_, err = repository.Delete(ctx, operation.Row)
			default:
				return api.NewError("ROW-0014", "不支持的操作: "+operation.Kind, api.ErrBadRequest)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
	transactional, ok := r.db.(TransactionalDB)
	if !ok {
		return run(r)
	}
	transaction, err := transactional.BeginTx(ctx)
	if err != nil {
		return api.NewError("ROW-0015", "开始事务失败", err)
	}
	repository := &Repository{db: transaction, dialect: r.dialect}
	if err := run(repository); err != nil {
		_ = transaction.Rollback()
		return err
	}
	if err := transaction.Commit(); err != nil {
		_ = transaction.Rollback()
		return api.NewError("ROW-0016", "提交事务失败", err)
	}
	return nil
}

// SnapshotOperations 为整份快照生成"先删后插"操作。
// @param rows 行列表
// @return 操作列表
func SnapshotOperations(rows []Row) []Operation {
	operations := make([]Operation, 0, len(rows)*2)
	for _, row := range rows {
		operations = append(operations, Operation{Kind: OperationDelete, Row: row})
		operations = append(operations, Operation{Kind: OperationInsert, Row: row})
	}
	return operations
}

// ReplaceAll 在事务中替换整份快照。
// @param ctx 上下文
// @param rows 行列表
// @return 替换错误
func (r *Repository) ReplaceAll(ctx context.Context, rows []Row) error {
	return r.Apply(ctx, SnapshotOperations(rows))
}
