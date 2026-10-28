package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	"github.com/flowable-go/flowable/internal/expr"
)

// conditionEventType 是条件事件订阅类型。
const conditionEventType = "condition"

// EvaluateConditions 显式求值流程实例的全部条件事件订阅。
// 实例不存在返回 404，实例已结束或已挂起返回 409。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 求值错误
func (e *Engine) EvaluateConditions(ctx context.Context, processInstanceID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		if strings.TrimSpace(processInstanceID) == "" {
			return nil, api.NewError("ENGINE-0635", "流程实例 ID 不能为空", api.ErrBadRequest)
		}
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Ended {
			return nil, api.NewError("ENGINE-0636", "流程实例已结束，无法求值条件", api.ErrConflict)
		}
		if instance.Suspended {
			return nil, api.NewError("ENGINE-0637", "流程实例已挂起，无法求值条件", api.ErrConflict)
		}
		if err := e.evaluateConditionalSubscriptions(cc, processInstanceID); err != nil {
			return nil, err
		}
		return nil, cc.agenda.run(cc)
	})
	return err
}

// evaluateConditionalSubscriptions 求值实例的全部条件事件订阅。
// 条件为真时触发订阅；表达式错误直接返回，不静默忽略。
// @param cc 命令上下文
// @param processInstanceID 流程实例 ID
// @return 求值错误
func (e *Engine) evaluateConditionalSubscriptions(cc *commandContext, processInstanceID string) error {
	if strings.TrimSpace(processInstanceID) == "" {
		return nil
	}
	for _, subscription := range e.conditionalSubscriptionsFor(processInstanceID) {
		// 前序订阅触发时可能已经清理当前订阅，需要重新检查
		if _, exists := e.state.Subscriptions[subscription.ID]; !exists {
			continue
		}
		execution, ok := e.findExecution(subscription.ExecutionID)
		if !ok || execution.Ended {
			delete(e.state.Subscriptions, subscription.ID)
			continue
		}
		instance, ok := e.findInstance(execution.ProcessInstanceID)
		if !ok || instance.Ended {
			delete(e.state.Subscriptions, subscription.ID)
			continue
		}
		// 挂起实例不触发条件事件
		if instance.Suspended {
			continue
		}
		matched, err := expr.EvalBool(strings.TrimSpace(subscription.Condition), e.conditionalVariables(execution))
		if err != nil {
			return api.NewError("ENGINE-0631", "条件表达式求值失败: "+subscription.Condition, err)
		}
		if !matched {
			continue
		}
		if err := e.triggerConditionalSubscription(cc, subscription); err != nil {
			return err
		}
	}
	return nil
}

// conditionalSubscriptionsFor 返回实例的条件订阅，按创建时间排序。
// @param processInstanceID 流程实例 ID
// @return 条件订阅列表
func (e *Engine) conditionalSubscriptionsFor(processInstanceID string) []*subscriptionState {
	out := make([]*subscriptionState, 0)
	for _, subscription := range e.state.Subscriptions {
		if subscription.EventType == conditionEventType && subscription.ProcessInstanceID == processInstanceID {
			out = append(out, subscription)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// conditionalVariables 返回条件求值可见变量。
// 在普通执行变量之上叠加当前执行所属任务的局部变量，
// 使任务变量在条件事件中具有执行可见性。
// @param execution 执行
// @return 变量快照
func (e *Engine) conditionalVariables(execution *executionState) map[string]any {
	values := e.variablesOf(execution)
	for _, task := range e.state.Tasks {
		if task.ExecutionID != execution.ID {
			continue
		}
		for name, value := range task.LocalVariables {
			values[name] = value
		}
	}
	return values
}

// registerConditionalBoundarySubscriptions 为用户任务注册边界条件订阅。
// 仅处理配置了条件表达式的边界事件；非中断边界按中断语义处理。
// @param cc 命令上下文
// @param execution 用户任务执行
// @param task 用户任务节点
// @return 注册错误
func (e *Engine) registerConditionalBoundarySubscriptions(cc *commandContext, execution *executionState, task *model.UserTask) error {
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	for _, boundary := range task.GetBoundaryEvents() {
		condition := strings.TrimSpace(boundary.Condition)
		if condition == "" {
			continue
		}
		subscription := &subscriptionState{
			ID:                  e.newID(),
			ProcessDefinitionID: definition.Info.ID,
			ProcessInstanceID:   execution.ProcessInstanceID,
			ExecutionID:         execution.ID,
			EventType:           conditionEventType,
			ActivityID:          task.ID,
			BoundaryEventID:     boundary.ID,
			Interrupting:        true,
			Condition:           condition,
			TenantID:            definition.Info.TenantID,
			CreatedAt:           cc.now,
		}
		e.state.Subscriptions[subscription.ID] = subscription
	}
	return nil
}

// registerConditionalCatchSubscription 为中间捕获条件事件注册订阅。
// @param cc 命令上下文
// @param execution 执行
// @param definition 流程定义
// @param event 中间捕获事件
// @return 注册错误
func (e *Engine) registerConditionalCatchSubscription(cc *commandContext, execution *executionState, definition *definitionState, event *model.IntermediateCatchEvent) error {
	condition := strings.TrimSpace(event.Condition)
	if condition == "" {
		return nil
	}
	subscription := &subscriptionState{
		ID:                  e.newID(),
		ProcessDefinitionID: definition.Info.ID,
		ProcessInstanceID:   execution.ProcessInstanceID,
		ExecutionID:         execution.ID,
		EventType:           conditionEventType,
		ActivityID:          event.ID,
		Condition:           condition,
		TenantID:            definition.Info.TenantID,
		CreatedAt:           cc.now,
	}
	e.state.Subscriptions[subscription.ID] = subscription
	return nil
}

// triggerConditionalSubscription 触发单个条件订阅。
// @param cc 命令上下文
// @param subscription 条件订阅
// @return 触发错误
func (e *Engine) triggerConditionalSubscription(cc *commandContext, subscription *subscriptionState) error {
	if subscription.BoundaryEventID != "" {
		// 安全子集只支持中断边界条件，统一强制中断依附活动
		return e.triggerBoundaryEvent(cc, subscription, true)
	}
	return e.triggerCatchConditionalSubscription(cc, subscription)
}

// triggerCatchConditionalSubscription 触发中间捕获条件事件并沿出线继续。
// @param cc 命令上下文
// @param subscription 条件订阅
// @return 触发错误
func (e *Engine) triggerCatchConditionalSubscription(cc *commandContext, subscription *subscriptionState) error {
	delete(e.state.Subscriptions, subscription.ID)
	execution, ok := e.findExecution(subscription.ExecutionID)
	if !ok || execution.Ended {
		return nil
	}
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok || instance.Ended || instance.Suspended {
		return nil
	}
	e.recordActivityEnd(cc, execution.CurrentHistActivityID)
	execution.CurrentHistActivityID = ""
	execution.Waiting = false
	cc.agenda.schedule(&takeOutgoingOperation{executionID: execution.ID, activityID: subscription.ActivityID})
	return nil
}
