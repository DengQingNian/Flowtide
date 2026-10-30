package engine

import (
	"context"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// UpdateProcessInstance 在同一命令事务内修改实例元数据、签收身份链接及历史。
// @param ctx 请求上下文
// @param id 实例标识
// @param update 可选属性或签收请求
// @return 更新后的实例
// @return 定位、冲突或持久化错误
func (e *Engine) UpdateProcessInstance(ctx context.Context, id string, update api.ProcessInstanceUpdate) (api.ProcessInstance, error) {
	if err := ctx.Err(); err != nil {
		return api.ProcessInstance{}, err
	}
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		instance, ok := e.findInstance(id)
		if !ok || instance.Ended {
			return nil, api.NewError("ENGINE-1130", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, id); err != nil {
			return nil, err
		}
		// 所有冲突先校验，再修改整组属性；保存失败仍由命令事务回滚。
		if update.ClaimUserID != nil && *update.ClaimUserID != "" {
			for _, link := range e.state.InstanceLinks {
				if link.ProcessInstanceID == id && link.Type == "assignee" {
					return nil, api.NewError("ENGINE-1131", "流程实例已签收", api.ErrConflict)
				}
			}
			if instance.ClaimedBy != "" {
				return nil, api.NewError("ENGINE-1131", "流程实例已签收", api.ErrConflict)
			}
		}
		if update.Name != nil {
			instance.Name = *update.Name
		}
		if update.BusinessKey != nil {
			instance.BusinessKey = *update.BusinessKey
		}
		if update.BusinessStatus != nil {
			instance.BusinessStatus = *update.BusinessStatus
		}
		if update.DueDate != nil {
			due := update.DueDate.UTC().Truncate(time.Millisecond)
			instance.DueDate = &due
		}
		if update.ClaimUserID != nil {
			for key, link := range e.state.InstanceLinks {
				if link.ProcessInstanceID == id && link.Type == "assignee" {
					delete(e.state.InstanceLinks, key)
					delete(e.state.HistoricLinks, key)
				}
			}
			instance.ClaimedBy = *update.ClaimUserID
			instance.ClaimTime = nil
			if instance.ClaimedBy != "" {
				claimed := cc.now.UTC().Truncate(time.Millisecond)
				instance.ClaimTime = &claimed
				link := &api.IdentityLink{ID: e.newID(), ProcessInstanceID: id, UserID: instance.ClaimedBy, Type: "assignee", CreateTime: claimed}
				e.state.InstanceLinks[link.ID] = link
				e.recordInstanceHistoricLink(cc, link)
			}
		}
		if historic, ok := e.state.HistoricProcesses[id]; ok {
			historic.Name = instance.Name
			historic.BusinessKey = instance.BusinessKey
			historic.BusinessStatus = instance.BusinessStatus
			historic.DueDate = cloneTime(instance.DueDate)
			historic.ClaimTime = cloneTime(instance.ClaimTime)
			historic.ClaimedBy = instance.ClaimedBy
		}
		return e.toProcessInstance(instance), nil
	})
	if err != nil {
		return api.ProcessInstance{}, err
	}
	return result.(api.ProcessInstance), nil
}

// messageStartDefinition 在当前租户的最新定义中定位唯一消息开始事件，不误投递到运行时等待订阅。
// @param ctx 请求上下文
// @param name 消息名
// @return 流程定义
// @return 匹配的开始事件
// @return 未找到或歧义错误
func (e *Engine) messageStartDefinition(ctx context.Context, name string) (*definitionState, *model.StartEvent, error) {
	var selected *definitionState
	var event *model.StartEvent
	for _, definition := range e.state.Definitions {
		if e.allowTenant(ctx, definition.Info.TenantID) != nil {
			continue
		}
		latest, ok := e.findTenantDefinition(ctx, definition.Info.Key)
		if !ok || latest.Info.ID != definition.Info.ID {
			continue
		}
		process := processOfDefinition(definition)
		if process == nil {
			continue
		}
		// 仅匹配顶层消息开始，信号及事件子流程不能作为本请求的目标。
		for _, start := range process.StartEvents() {
			kind, value := e.eventTypeAndName(definition.Model, start.MessageRef, start.SignalRef)
			if kind != "message" || value != name {
				continue
			}
			if selected != nil {
				return nil, nil, api.NewError("ENGINE-1133", "消息开始事件不唯一", api.ErrConflict)
			}
			selected, event = definition, start
		}
	}
	if selected == nil {
		return nil, nil, api.NewError("ENGINE-1132", "消息开始事件不存在", api.ErrNotFound)
	}
	return selected, event, nil
}
