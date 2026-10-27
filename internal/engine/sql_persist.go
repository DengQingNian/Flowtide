package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// PersistSnapshot 将完整引擎快照写入行仓储。
// 写入采用"先删后插"，应在事务中调用。
// @param ctx 上下文
// @param repository 行仓储
// @return 写入错误
func (e *Engine) PersistSnapshot(ctx context.Context, repository *flowablerow.Repository) error {
	if repository == nil {
		return api.NewError("ENGINE-0914", "行仓储不能为空", api.ErrBadRequest)
	}
	rows, err := e.SnapshotRows(ctx)
	if err != nil {
		return err
	}
	return repository.ReplaceAll(ctx, rows)
}

// SnapshotRows 返回完整引擎快照行。
// @param ctx 上下文
// @return 行列表
// @return 导出错误
func (e *Engine) SnapshotRows(ctx context.Context) ([]flowablerow.Row, error) {
	return e.readRows(e.snapshotRowsLocked)
}

// PersistDiff 将当前快照相对旧快照的增量写入行仓储。
// @param ctx 上下文
// @param repository 行仓储
// @param previous 旧快照行
// @return 写入错误
func (e *Engine) PersistDiff(ctx context.Context, repository *flowablerow.Repository, previous []flowablerow.Row) error {
	if repository == nil {
		return api.NewError("ENGINE-0914", "行仓储不能为空", api.ErrBadRequest)
	}
	current, err := e.SnapshotRows(ctx)
	if err != nil {
		return err
	}
	return repository.Apply(ctx, flowablerow.DiffOperations(previous, current))
}

// persistRows 将当前状态差异写入行仓储。
// @param ctx 上下文
// @return 写入错误
func (e *Engine) persistRows(ctx context.Context) error {
	if e.rowRepository == nil {
		return nil
	}
	current, err := e.snapshotRowsLocked()
	if err != nil {
		return err
	}
	operations := flowablerow.DiffOperations(e.lastRows, current)
	if err := e.rowRepository.Apply(ctx, operations); err != nil {
		return api.NewError("ENGINE-0915", "写入行仓储失败", err)
	}
	e.lastRows = current
	return nil
}
