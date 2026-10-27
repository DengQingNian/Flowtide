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
	repositoryRows, err := e.ExportRows(ctx)
	if err != nil {
		return err
	}
	runtimeRows, err := e.ExportRuntimeRows(ctx)
	if err != nil {
		return err
	}
	historicRows, err := e.ExportHistoricRows(ctx)
	if err != nil {
		return err
	}
	rows := make([]flowablerow.Row, 0, len(repositoryRows)+len(runtimeRows)+len(historicRows))
	rows = append(rows, repositoryRows...)
	rows = append(rows, runtimeRows...)
	rows = append(rows, historicRows...)
	return repository.ReplaceAll(ctx, rows)
}
