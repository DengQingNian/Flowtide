package engine

import (
	"context"
	"sort"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// ListEventSubscriptions 返回事件订阅。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID，空串表示全部
// @return 订阅列表
// @return 查询错误
func (e *Engine) ListEventSubscriptions(ctx context.Context, processInstanceID string) ([]api.EventSubscription, error) {
	result, err := e.read(func() (any, error) {
		requested := e.tenantOf(tenant.From(ctx))
		out := make([]api.EventSubscription, 0, len(e.state.Subscriptions))
		for _, subscription := range e.state.Subscriptions {
			if processInstanceID != "" && subscription.ProcessInstanceID != processInstanceID {
				continue
			}
			// 租户不匹配的订阅对当前请求不可见
			if requested != e.subscriptionTenant(subscription) {
				continue
			}
			out = append(out, toEventSubscription(subscription))
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Created.Equal(out[j].Created) {
				return out[i].ID < out[j].ID
			}
			return out[i].Created.Before(out[j].Created)
		})
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]api.EventSubscription), nil
}

// GetEventSubscription 查询事件订阅。
// @param ctx 上下文
// @param subscriptionID 订阅 ID
// @return 订阅
// @return 查询错误
func (e *Engine) GetEventSubscription(ctx context.Context, subscriptionID string) (api.EventSubscription, error) {
	result, err := e.read(func() (any, error) {
		subscription, ok := e.state.Subscriptions[subscriptionID]
		if !ok {
			return nil, api.NewError("ENGINE-0701", "事件订阅不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, e.subscriptionTenant(subscription)); err != nil {
			return nil, err
		}
		return toEventSubscription(subscription), nil
	})
	if err != nil {
		return api.EventSubscription{}, err
	}
	return result.(api.EventSubscription), nil
}

// subscriptionTenant 返回订阅所属租户。
// @param subscription 订阅
// @return 租户
func (e *Engine) subscriptionTenant(subscription *subscriptionState) string {
	if subscription.ProcessInstanceID != "" {
		if instance, ok := e.findInstance(subscription.ProcessInstanceID); ok {
			return instance.TenantID
		}
	}
	return subscription.TenantID
}

// toEventSubscription 将订阅状态转换为 DTO。
// @param subscription 订阅
// @return 订阅 DTO
func toEventSubscription(subscription *subscriptionState) api.EventSubscription {
	return api.EventSubscription{
		ID:                  subscription.ID,
		URL:                 "/service/runtime/event-subscriptions/" + subscription.ID,
		EventType:           subscription.EventType,
		EventName:           subscription.EventName,
		ExecutionID:         subscription.ExecutionID,
		ProcessInstanceID:   subscription.ProcessInstanceID,
		ActivityID:          subscription.ActivityID,
		ProcessDefinitionID: subscription.ProcessDefinitionID,
		TenantID:            subscription.TenantID,
		Created:             subscription.CreatedAt,
	}
}
