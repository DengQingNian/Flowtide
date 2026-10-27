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
	repositoryRows, err := e.ExportRows(ctx)
	if err != nil {
		return nil, err
	}
	runtimeRows, err := e.ExportRuntimeRows(ctx)
	if err != nil {
		return nil, err
	}
	historicRows, err := e.ExportHistoricRows(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]flowablerow.Row, 0, len(repositoryRows)+len(runtimeRows)+len(historicRows))
	rows = append(rows, repositoryRows...)
	rows = append(rows, runtimeRows...)
	rows = append(rows, historicRows...)
	return rows, nil
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
