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
	requested := tenant.From(ctx)
	if requested == "" || requested == resourceTenant {
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
