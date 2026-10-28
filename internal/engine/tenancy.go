package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// allowTenant 校验请求租户与资源租户。
// @param ctx 上下文
// @param resourceTenant 资源租户
// @return 校验错误
func (e *Engine) allowTenant(ctx context.Context, resourceTenant string) error {
	requested := e.tenantOf(tenant.From(ctx))
	if requested == resourceTenant {
		return nil
	}
	// 不泄露其他租户资源是否存在
	return api.NewError("ENGINE-0404", "资源不存在", api.ErrNotFound)
}

// allowInstance 校验流程实例租户。
// @param ctx 上下文
// @param instanceID 实例 ID
// @return 校验错误
func (e *Engine) allowInstance(ctx context.Context, instanceID string) error {
	instance, ok := e.findInstance(instanceID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	return e.allowTenant(ctx, instance.TenantID)
}

// allowTask 校验任务租户。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 校验错误
func (e *Engine) allowTask(ctx context.Context, taskID string) error {
	task, ok := e.findTask(taskID)
	if !ok {
		return api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	// 独立任务没有流程实例，直接校验任务租户
	if task.Task.ProcessInstanceID == "" {
		return e.allowTenant(ctx, task.Task.TenantID)
	}
	return e.allowInstance(ctx, task.Task.ProcessInstanceID)
}

// allowDefinition 校验流程定义租户。
// @param ctx 上下文
// @param definitionID 定义 ID
// @return 校验错误
func (e *Engine) allowDefinition(ctx context.Context, definitionID string) error {
	definition, ok := e.findDefinition(definitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	return e.allowTenant(ctx, definition.Info.TenantID)
}

// requestTenantID 根据当前作用域补全创建请求的租户并校验归属。
// @param ctx 请求上下文
// @param requested 请求指定租户
// @return 生效租户
// @return 校验错误
func (e *Engine) requestTenantID(ctx context.Context, requested string) (string, error) {
	if requested == "" {
		requested = e.tenantOf(tenant.From(ctx))
	}
	return requested, e.allowTenant(ctx, requested)
}

// findTenantDefinition 在当前租户内查找最新定义。
// @param ctx 请求上下文
// @param key 定义键
// @return 最新定义
// @return 是否找到
func (e *Engine) findTenantDefinition(ctx context.Context, key string) (*definitionState, bool) {
	var latest *definitionState
	for _, definition := range e.state.Definitions {
		// 不同租户的同名定义必须分别选择，不能先选全局最新版本再鉴权。
		if definition.Info.Key != key || e.allowTenant(ctx, definition.Info.TenantID) != nil {
			continue
		}
		if latest == nil || latest.Info.Version < definition.Info.Version {
			latest = definition
		}
	}
	return latest, latest != nil
}

// allowHistoricResource 校验可由历史实例或历史任务定位的资源归属。
// @param ctx 上下文
// @param instanceID 实例 ID，可为空
// @param taskID 任务 ID，可为空
// @return 校验错误
func (e *Engine) allowHistoricResource(ctx context.Context, instanceID, taskID string) error {
	// 运行时实体可能已清理，按历史任务、历史实例、运行时实体依次解析归属。
	if task, ok := e.state.HistoricTasks[taskID]; ok {
		return e.allowTenant(ctx, task.TenantID)
	}
	if instance, ok := e.state.HistoricProcesses[instanceID]; ok {
		return e.allowTenant(ctx, instance.TenantID)
	}
	if instanceID != "" {
		return e.allowInstance(ctx, instanceID)
	}
	if taskID != "" {
		return e.allowTask(ctx, taskID)
	}
	return e.allowTenant(ctx, "")
}

// allowJob 校验作业关联实例或流程定义的租户。
// @param ctx 上下文
// @param job 作业
// @return 校验错误
func (e *Engine) allowJob(ctx context.Context, job *jobState) error {
	if job.ProcessInstanceID != "" {
		return e.allowInstance(ctx, job.ProcessInstanceID)
	}
	if job.ProcessDefinitionID != "" {
		return e.allowDefinition(ctx, job.ProcessDefinitionID)
	}
	return e.allowTenant(ctx, "")
}
