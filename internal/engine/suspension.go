package engine

import (
	"context"

	"github.com/flowable-go/flowable/api"
)

// SuspendProcessDefinition 挂起流程定义。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param includeInstances 是否同时挂起实例
// @return 挂起错误
func (e *Engine) SuspendProcessDefinition(ctx context.Context, definitionID string, includeInstances bool) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		definition.Info.Suspended = true
		if includeInstances {
			for _, instance := range e.state.Instances {
				if instance.ProcessDefinitionID == definitionID {
					instance.Suspended = true
				}
			}
		}
		return nil, nil
	})
	return err
}

// ActivateProcessDefinition 激活流程定义。
// @param ctx 上下文
// @param definitionID 流程定义 ID
// @param includeInstances 是否同时激活实例
// @return 激活错误
func (e *Engine) ActivateProcessDefinition(ctx context.Context, definitionID string, includeInstances bool) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		definition, ok := e.findDefinition(definitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, definition.Info.TenantID); err != nil {
			return nil, err
		}
		definition.Info.Suspended = false
		if includeInstances {
			for _, instance := range e.state.Instances {
				if instance.ProcessDefinitionID == definitionID {
					instance.Suspended = false
				}
			}
		}
		return nil, nil
	})
	return err
}

// SuspendProcessInstance 挂起流程实例。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 挂起错误
func (e *Engine) SuspendProcessInstance(ctx context.Context, processInstanceID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		instance.Suspended = true
		return nil, nil
	})
	return err
}

// ActivateProcessInstance 激活流程实例。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 激活错误
func (e *Engine) ActivateProcessInstance(ctx context.Context, processInstanceID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		instance.Suspended = false
		return nil, nil
	})
	return err
}
