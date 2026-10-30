package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// CreateTask 创建独立任务。
// @param ctx 上下文
// @param req 创建请求
// @return 任务
// @return 创建错误
func (e *Engine) CreateTask(ctx context.Context, req api.CreateTaskRequest) (api.Task, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			return nil, api.NewError("ENGINE-0820", "任务名称不能为空", api.ErrBadRequest)
		}
		tenant, tenantErr := e.requestTenantID(cc.ctx, req.TenantID)
		if tenantErr != nil {
			return nil, tenantErr
		}
		// 子任务优先继承父任务租户
		if req.ParentTaskID != "" {
			parent, ok := e.findTask(req.ParentTaskID)
			if !ok {
				return nil, api.NewError("ENGINE-0805", "父任务不存在", api.ErrNotFound)
			}
			if err := e.allowTask(cc.ctx, parent.Task.ID); err != nil {
				return nil, err
			}
			if tenant == "" {
				tenant = parent.Task.TenantID
			}
			if tenant != parent.Task.TenantID {
				return nil, api.NewError("ENGINE-0821", "子任务租户与父任务不一致", api.ErrConflict)
			}
		}
		if err := e.allowTenant(cc.ctx, tenant); err != nil {
			return nil, err
		}
		priority := req.Priority
		if priority == 0 {
			priority = 50
		}
		dueDate := cloneTime(req.DueDate)
		task := &taskState{
			LocalVariables: make(map[string]any),
			Task: api.Task{
				ID:           e.newID(),
				Name:         name,
				Description:  req.Description,
				Assignee:     req.Assignee,
				Owner:        req.Owner,
				Priority:     priority,
				FormKey:      req.FormKey,
				Category:     req.Category,
				ParentTaskID: req.ParentTaskID,
				TenantID:     tenant,
				CreateTime:   cc.now,
				DueDate:      dueDate,
			},
		}
		task.HistoricTaskID = e.recordTaskStart(cc, task)
		e.state.Tasks[task.Task.ID] = task
		e.recordTaskLog(cc, task, "create", actorFromContext(cc.ctx), "")
		return task.Task, nil
	})
	if err != nil {
		return api.Task{}, err
	}
	return result.(api.Task), nil
}

// UpdateTask 更新任务属性。
// @param ctx 上下文
// @param taskID 任务 ID
// @param update 更新内容
// @return 更新后的任务
// @return 更新错误
func (e *Engine) UpdateTask(ctx context.Context, taskID string, update api.TaskUpdate) (api.Task, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(cc.ctx, taskID); err != nil {
			return nil, err
		}
		if update.Name != nil {
			task.Task.Name = *update.Name
		}
		if update.Description != nil {
			task.Task.Description = *update.Description
		}
		previousAssignee := task.Task.Assignee
		if update.Assignee != nil {
			task.Task.Assignee = *update.Assignee
			if *update.Assignee == "" {
				task.Task.ClaimTime = nil
			}
		}
		if update.Owner != nil {
			task.Task.Owner = *update.Owner
		}
		if update.Priority != nil {
			task.Task.Priority = *update.Priority
		}
		if update.Category != nil {
			task.Task.Category = *update.Category
		}
		if update.FormKey != nil {
			task.Task.FormKey = *update.FormKey
		}
		// 显式 null 清空到期时间，否则仅在提供时更新
		if update.ClearDueDate {
			task.Task.DueDate = nil
		} else if update.DueDate != nil {
			task.Task.DueDate = cloneTime(update.DueDate)
		}
		if historic, exists := e.state.HistoricTasks[task.HistoricTaskID]; exists {
			historic.Name = task.Task.Name
		}
		e.syncTaskAssigneeHistory(task)
		e.recordTaskIdentityLinks(cc, task)
		// 原版把处理人变化记录为独立事件并升级流程参与者。
		if update.Assignee != nil && *update.Assignee != previousAssignee {
			if *update.Assignee != "" {
				e.promoteProcessParticipant(cc, task.Task.ProcessInstanceID, *update.Assignee, cc.now)
			}
			e.recordTaskLog(cc, task, "USER_TASK_ASSIGNEE_CHANGED", actorFromContext(cc.ctx), assigneeChangedLogData(previousAssignee, *update.Assignee))
		} else {
			e.recordTaskLog(cc, task, "update", update.UserID, "")
		}
		return e.toTaskDTO(task), nil
	})
	if err != nil {
		return api.Task{}, err
	}
	return result.(api.Task), nil
}

// DeleteTask 删除独立任务及子任务。
// @param ctx 上下文
// @param taskID 任务 ID
// @param cascadeHistory 是否级联删除历史
// @param deleteReason 删除原因
// @return 删除错误
func (e *Engine) DeleteTask(ctx context.Context, taskID string, cascadeHistory bool, deleteReason string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		task, ok := e.findTask(taskID)
		if !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		// 与 Flowable REST 一致：流程内任务不允许直接删除
		if task.ExecutionID != "" {
			return nil, api.NewError("ENGINE-0822", "不能删除流程内任务", api.ErrForbidden)
		}
		if err := e.allowTask(cc.ctx, taskID); err != nil {
			return nil, err
		}
		e.deleteStandaloneTask(cc, task, cascadeHistory, deleteReason)
		return nil, nil
	})
	return err
}

// deleteStandaloneTask 递归删除独立任务及其子任务。
// @param cc 命令上下文
// @param task 任务
// @param cascadeHistory 是否级联删除历史
// @param deleteReason 删除原因
func (e *Engine) deleteStandaloneTask(cc *commandContext, task *taskState, cascadeHistory bool, deleteReason string) {
	children := make([]*taskState, 0)
	for _, child := range e.state.Tasks {
		if child.Task.ParentTaskID == task.Task.ID {
			children = append(children, child)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Task.ID < children[j].Task.ID })
	for _, child := range children {
		e.deleteStandaloneTask(cc, child, cascadeHistory, deleteReason)
	}
	e.recordTaskLog(cc, task, "delete", "", deleteReason)
	e.recordTaskEnd(cc, task)
	if historic, ok := e.state.HistoricTasks[task.HistoricTaskID]; ok {
		historic.DeleteReason = deleteReason
	}
	if cascadeHistory {
		e.removeHistoricTask(task.Task.ID)
	}
	delete(e.state.Tasks, task.Task.ID)
}

// removeHistoricTask 删除历史任务及关联记录。
// @param taskID 任务 ID
func (e *Engine) removeHistoricTask(taskID string) {
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
}

// ListSubTasks 返回子任务。
// @param ctx 上下文
// @param taskID 父任务 ID
// @return 子任务列表
// @return 查询错误
func (e *Engine) ListSubTasks(ctx context.Context, taskID string) ([]api.Task, error) {
	result, err := e.read(func() (any, error) {
		if _, ok := e.findTask(taskID); !ok {
			return nil, api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
		}
		if err := e.allowTask(ctx, taskID); err != nil {
			return nil, err
		}
		out := make([]api.Task, 0)
		for _, task := range e.state.Tasks {
			if task.Task.ParentTaskID != taskID {
				continue
			}
			out = append(out, e.toTaskDTO(task))
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].CreateTime.Equal(out[j].CreateTime) {
				return out[i].ID < out[j].ID
			}
			return out[i].CreateTime.Before(out[j].CreateTime)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.Task), nil
}
