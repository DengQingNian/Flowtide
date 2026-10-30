package engine

import (
	"context"
	"time"

	"github.com/DengQingNian/Flowtide/api"
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

// deleteHistoricActivitiesByProcessInstanceAndActivityIDs 选择性删除历史活动行（G7）。
// 对位 Java AbstractRollbackOperateStrategy.deleteHisActInstance：
// 删除"开始时间晚于 createdAfter 且活动 ID 在集合内"的行，以及"当前撤回任务活动 ID 且
// 结束时间晚于 createdAfter"的行，与 Java mapper.delete 语义一致。
// @param processInstanceID 实例 ID
// @param activityIDs 活动 ID 集合
// @param createdAfter 开始时间下界
// @param currentActivityID 当前撤回任务的活动 ID（其结束行一并删除）
// @return 删除行数
func (e *Engine) deleteHistoricActivitiesByProcessInstanceAndActivityIDs(processInstanceID string, activityIDs map[string]bool, createdAfter time.Time, currentActivityID string) int {
	removed := 0
	for id, activity := range e.state.HistoricActivities {
		if activity.ProcessInstanceID != processInstanceID {
			continue
		}
		// 撤回时间点之后经过的线条
		hit := activity.StartTime.After(createdAfter) && activityIDs[activity.ActivityID]
		// 当前任务的连线行（结束时间晚于创建时间）
		if !hit && activity.ActivityID == currentActivityID && activity.EndTime != nil && activity.EndTime.After(createdAfter) {
			hit = true
		}
		if !hit {
			continue
		}
		delete(e.state.HistoricActivities, id)
		removed++
	}
	return removed
}

// deleteHistoricTasksByDefinitionKeys 选择性删除历史任务行（G7）。
// 对位 Java deleteRuntimeTasks 的历史任务删除分支；createdAfter 非空时仅删除创建时间不早于该时刻的任务。
// 同步清理任务关联的历史链接、任务变量、任务日志与评论。
// @param processInstanceID 实例 ID
// @param keys 任务定义 Key 集合
// @param createdAfter 创建时间下界，nil 表示不过滤
// @return 删除行数
func (e *Engine) deleteHistoricTasksByDefinitionKeys(processInstanceID string, keys map[string]bool, createdAfter *time.Time) int {
	removed := 0
	for id, historic := range e.state.HistoricTasks {
		if historic.ProcessInstanceID != processInstanceID || !keys[historic.TaskDefinitionKey] {
			continue
		}
		// 网关策略变体：只删撤回时间点之后创建的任务
		if createdAfter != nil && historic.StartTime.Before(*createdAfter) {
			continue
		}
		e.removeHistoricTaskAssociations(id)
		delete(e.state.HistoricTasks, id)
		removed++
	}
	return removed
}

// removeHistoricTaskAssociations 清理历史任务关联的身份链接、任务变量、任务日志与评论。
// @param taskID 任务 ID
func (e *Engine) removeHistoricTaskAssociations(taskID string) {
	for id, link := range e.state.HistoricLinks {
		if link.TaskID == taskID {
			delete(e.state.HistoricLinks, id)
		}
	}
	for id, variable := range e.state.HistoricVariables {
		if variable.TaskID == taskID {
			delete(e.state.HistoricVariables, id)
		}
	}
	for id, log := range e.state.TaskLogs {
		if log.TaskID == taskID {
			delete(e.state.TaskLogs, id)
		}
	}
	for id, comment := range e.state.Comments {
		if comment.TaskID == taskID {
			delete(e.state.Comments, id)
		}
	}
}
