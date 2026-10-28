package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
)

// ListTaskEvents 返回任务事件。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 事件列表
// @return 查询错误
func (e *Engine) ListTaskEvents(ctx context.Context, taskID string) ([]api.TaskEvent, error) {
	result, err := e.read(func() (any, error) {
		if err := e.allowTaskEvents(ctx, taskID); err != nil {
			return nil, err
		}
		out := make([]api.TaskEvent, 0)
		for _, log := range e.state.TaskLogs {
			if log.TaskID != taskID {
				continue
			}
			out = append(out, taskEventFromLog(log))
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Timestamp.Equal(out[j].Timestamp) {
				return out[i].ID < out[j].ID
			}
			return out[i].Timestamp.Before(out[j].Timestamp)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.TaskEvent), nil
}

// GetTaskEvent 查询单个任务事件。
// @param ctx 上下文
// @param taskID 任务 ID
// @param eventID 事件 ID
// @return 事件
// @return 查询错误
func (e *Engine) GetTaskEvent(ctx context.Context, taskID string, eventID int64) (api.TaskEvent, error) {
	result, err := e.read(func() (any, error) {
		if err := e.allowTaskEvents(ctx, taskID); err != nil {
			return nil, err
		}
		log, ok := e.state.TaskLogs[eventID]
		if !ok || log.TaskID != taskID {
			return nil, api.NewError("ENGINE-1401", "任务事件不存在", api.ErrNotFound)
		}
		return taskEventFromLog(log), nil
	})
	if err != nil {
		return api.TaskEvent{}, err
	}
	return result.(api.TaskEvent), nil
}

// DeleteTaskEvent 删除任务事件。
// @param ctx 上下文
// @param taskID 任务 ID
// @param eventID 事件 ID
// @return 删除错误
func (e *Engine) DeleteTaskEvent(ctx context.Context, taskID string, eventID int64) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if _, ok := e.findTask(taskID); !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(cc.ctx, taskID); err != nil {
			return nil, err
		}
		log, ok := e.state.TaskLogs[eventID]
		if !ok || log.TaskID != taskID {
			return nil, api.NewError("ENGINE-1401", "任务事件不存在", api.ErrNotFound)
		}
		delete(e.state.TaskLogs, eventID)
		return nil, nil
	})
	return err
}

// allowTaskEvents 校验任务事件访问权限。
// 运行时任务按任务租户校验，已完成任务按历史任务所属实例校验。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 校验错误
func (e *Engine) allowTaskEvents(ctx context.Context, taskID string) error {
	if _, ok := e.findTask(taskID); ok {
		return e.allowTask(ctx, taskID)
	}
	historic, ok := e.state.HistoricTasks[taskID]
	if !ok {
		return api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	if historic.ProcessInstanceID != "" {
		return e.allowInstance(ctx, historic.ProcessInstanceID)
	}
	return e.allowTenant(ctx, historic.TenantID)
}

// taskEventFromLog 将任务日志转换为事件。
// @param log 任务日志
// @return 任务事件
func taskEventFromLog(log *api.HistoricTaskLog) api.TaskEvent {
	return api.TaskEvent{
		ID:                  log.ID,
		Action:              log.Type,
		Timestamp:           log.Time,
		UserID:              log.UserID,
		TaskID:              log.TaskID,
		ExecutionID:         log.ExecutionID,
		ProcessInstanceID:   log.ProcessInstanceID,
		ProcessDefinitionID: log.ProcessDefinitionID,
		TenantID:            log.TenantID,
		Data:                log.Data,
	}
}
