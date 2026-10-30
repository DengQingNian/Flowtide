package engine

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// InjectUserTask 在流程实例中注入一个动态用户任务。
// 动态任务独立于主流程执行，完成后不影响原有令牌。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param req 注入请求
// @return 任务
// @return 注入错误
func (e *Engine) InjectUserTask(ctx context.Context, processInstanceID string, req api.InjectRequest) (api.Task, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Ended {
			return nil, api.NewError("ENGINE-0670", "流程实例已结束", api.ErrConflict)
		}
		if instance.Suspended {
			return nil, api.NewError("ENGINE-0671", "流程实例已挂起", api.ErrConflict)
		}
		if req.InjectionType != "" && !strings.EqualFold(req.InjectionType, "task") {
			return nil, api.NewError("ENGINE-0672", "暂不支持注入类型: "+req.InjectionType, api.ErrBadRequest)
		}
		if strings.TrimSpace(req.Name) == "" {
			return nil, api.NewError("ENGINE-0673", "任务名称不能为空", api.ErrBadRequest)
		}
		if req.TaskID != "" {
			reference, exists := e.findTask(req.TaskID)
			if !exists || reference.Task.ProcessInstanceID != processInstanceID {
				return nil, api.NewError("ENGINE-0404", "参考任务不存在", api.ErrNotFound)
			}
		}
		taskID := req.ID
		if taskID == "" {
			taskID = e.newID()
		}
		if _, exists := e.state.Tasks[taskID]; exists {
			return nil, api.NewError("ENGINE-0674", "任务 ID 已存在", api.ErrConflict)
		}
		task := &taskState{
			ExecutionID:    "",
			ActivityID:     taskID,
			LocalVariables: make(map[string]any),
			Task: api.Task{
				ID:                  taskID,
				Name:                req.Name,
				ProcessInstanceID:   instance.ID,
				ProcessDefinitionID: instance.ProcessDefinitionID,
				TaskDefinitionKey:   taskID,
				Assignee:            req.Assignee,
				Priority:            50,
				CreateTime:          cc.now,
			},
		}
		task.HistoricTaskID = e.recordTaskStart(cc, task)
		e.state.Tasks[task.Task.ID] = task
		e.recordTaskIdentityLinks(cc, task)
		e.recordTaskLog(cc, task, "create", task.Task.Assignee, "injected")
		return task.Task, nil
	})
	if err != nil {
		return api.Task{}, err
	}
	return result.(api.Task), nil
}
