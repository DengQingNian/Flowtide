package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
)

// DeleteHistoricProcessInstance 删除历史流程实例及关联记录。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 删除错误
func (e *Engine) DeleteHistoricProcessInstance(ctx context.Context, processInstanceID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.HistoricProcesses[processInstanceID]; !ok {
			return nil, api.NewError("ENGINE-0404", "历史流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, e.state.HistoricProcesses[processInstanceID].TenantID); err != nil {
			return nil, err
		}
		delete(e.state.HistoricProcesses, processInstanceID)
		for id, activity := range e.state.HistoricActivities {
			if activity.ProcessInstanceID == processInstanceID {
				delete(e.state.HistoricActivities, id)
			}
		}
		for id, task := range e.state.HistoricTasks {
			if task.ProcessInstanceID == processInstanceID {
				delete(e.state.HistoricTasks, id)
			}
		}
		for id, variable := range e.state.HistoricVariables {
			if variable.ProcessInstanceID == processInstanceID {
				delete(e.state.HistoricVariables, id)
			}
		}
		for id, detail := range e.state.HistoricDetails {
			if detail.ProcessInstanceID == processInstanceID {
				delete(e.state.HistoricDetails, id)
			}
		}
		for id, link := range e.state.HistoricLinks {
			if link.ProcessInstanceID == processInstanceID {
				delete(e.state.HistoricLinks, id)
			}
		}
		for id, comment := range e.state.Comments {
			if comment.ProcessInstanceID == processInstanceID {
				delete(e.state.Comments, id)
			}
		}
		for id, attachment := range e.state.Attachments {
			if attachment.ProcessInstanceID == processInstanceID {
				delete(e.state.Attachments, id)
			}
		}
		for id, log := range e.state.TaskLogs {
			if log.ProcessInstanceID == processInstanceID {
				delete(e.state.TaskLogs, id)
			}
		}
		return nil, nil
	})
	return err
}

// DeleteHistoricTaskInstance 删除历史任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 删除错误
func (e *Engine) DeleteHistoricTaskInstance(ctx context.Context, taskID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.HistoricTasks[taskID]; !ok {
			return nil, api.NewError("ENGINE-0404", "历史任务不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, e.state.HistoricTasks[taskID].TenantID); err != nil {
			return nil, err
		}
		delete(e.state.HistoricTasks, taskID)
		for id, log := range e.state.TaskLogs {
			if log.TaskID == taskID {
				delete(e.state.TaskLogs, id)
			}
		}
		for id, link := range e.state.HistoricLinks {
			if link.TaskID == taskID {
				delete(e.state.HistoricLinks, id)
			}
		}
		for id, comment := range e.state.Comments {
			if comment.TaskID == taskID {
				delete(e.state.Comments, id)
			}
		}
		for id, attachment := range e.state.Attachments {
			if attachment.TaskID == taskID {
				delete(e.state.Attachments, id)
			}
		}
		return nil, nil
	})
	return err
}

// DeleteHistoricDetail 删除历史变量明细。
// @param ctx 上下文
// @param detailID 明细 ID
// @return 删除错误
func (e *Engine) DeleteHistoricDetail(ctx context.Context, detailID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.HistoricDetails[detailID]; !ok {
			return nil, api.NewError("ENGINE-0404", "历史明细不存在", api.ErrNotFound)
		}
		if err := e.allowHistoricResource(ctx, e.state.HistoricDetails[detailID].ProcessInstanceID, e.state.HistoricDetails[detailID].TaskID); err != nil {
			return nil, err
		}
		delete(e.state.HistoricDetails, detailID)
		return nil, nil
	})
	return err
}

// DeleteHistoricTaskLog 删除任务日志。
// @param ctx 上下文
// @param logID 日志 ID
// @return 删除错误
func (e *Engine) DeleteHistoricTaskLog(ctx context.Context, logID int64) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.state.TaskLogs[logID]; !ok {
			return nil, api.NewError("ENGINE-0404", "任务日志不存在", api.ErrNotFound)
		}
		if err := e.allowHistoricResource(ctx, e.state.TaskLogs[logID].ProcessInstanceID, e.state.TaskLogs[logID].TaskID); err != nil {
			return nil, err
		}
		delete(e.state.TaskLogs, logID)
		return nil, nil
	})
	return err
}
