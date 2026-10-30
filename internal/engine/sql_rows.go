package engine

import flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"

// readRows 在只读锁中执行行导出。
// @param fn 导出函数
// @return 行列表
// @return 导出错误
func (e *Engine) readRows(fn func() ([]flowablerow.Row, error)) ([]flowablerow.Row, error) {
	result, err := e.read(func() (any, error) {
		return fn()
	})
	if err != nil {
		return nil, err
	}
	return result.([]flowablerow.Row), nil
}

// snapshotRowsLocked 在锁内汇总完整快照行。
// @return 行列表
// @return 导出错误
func (e *Engine) snapshotRowsLocked() ([]flowablerow.Row, error) {
	repositoryRows, err := e.repositoryRowsLocked()
	if err != nil {
		return nil, err
	}
	runtimeRows, err := e.runtimeRowsLocked()
	if err != nil {
		return nil, err
	}
	historicRows, err := e.historicRowsLocked()
	if err != nil {
		return nil, err
	}
	identityRows, err := e.identityRowsLocked()
	if err != nil {
		return nil, err
	}
	rows := make([]flowablerow.Row, 0, len(repositoryRows)+len(runtimeRows)+len(historicRows))
	rows = append(rows, repositoryRows...)
	rows = append(rows, runtimeRows...)
	rows = append(rows, historicRows...)
	rows = append(rows, identityRows...)
	return rows, nil
}
