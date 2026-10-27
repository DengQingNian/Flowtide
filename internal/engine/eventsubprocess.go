package engine

import (
	"github.com/flowable-go/flowable/bpmn/model"
)

// scheduleEventSubProcesses 为作用域内的事件子流程创建启动订阅。
// @param cc 命令上下文
// @param scope 作用域执行
// @param elements 作用域元素表
// @param order 元素顺序
// @param definition 流程定义
func (e *Engine) scheduleEventSubProcesses(cc *commandContext, scope *executionState, elements map[string]model.FlowElement, order []string, definition *definitionState) {
	for _, id := range order {
		sub, ok := elements[id].(*model.SubProcess)
		if !ok || !sub.TriggeredByEvent {
			continue
		}
		for _, start := range sub.StartEvents() {
			eventType, name := e.eventTypeAndName(definition.Model, start.MessageRef, start.SignalRef)
			if eventType == "" {
				continue
			}
			subscription := &subscriptionState{
				ID:                     e.newID(),
				ProcessDefinitionID:    definition.Info.ID,
				ProcessInstanceID:      scope.ProcessInstanceID,
				ExecutionID:            scope.ID,
				EventType:              eventType,
				EventName:              name,
				ActivityID:             start.ID,
				EventSubProcessID:      sub.ID,
				EventSubProcessStartID: start.ID,
				ScopeExecutionID:       scope.ID,
				Interrupting:           start.Interrupting,
				TenantID:               definition.Info.TenantID,
				CreatedAt:              cc.now,
			}
			e.state.Subscriptions[subscription.ID] = subscription
		}
	}
}

// triggerEventSubProcess 触发事件子流程。
// @param cc 命令上下文
// @param subscription 订阅
// @return 触发错误
func (e *Engine) triggerEventSubProcess(cc *commandContext, subscription *subscriptionState) error {
	scope, ok := e.findExecution(subscription.ScopeExecutionID)
	if !ok || scope.Ended {
		delete(e.state.Subscriptions, subscription.ID)
		return nil
	}
	instance, ok := e.findInstance(scope.ProcessInstanceID)
	if !ok || instance.Ended || instance.Suspended {
		return nil
	}
	if subscription.Interrupting {
		e.cancelScopeContent(cc, scope)
		// 中断型事件子流程只能触发一次，清理同作用域订阅
		for id, other := range e.state.Subscriptions {
			if other.ScopeExecutionID == scope.ID {
				delete(e.state.Subscriptions, id)
			}
		}
	}
	root := &executionState{
		ID:                          e.newID(),
		ProcessInstanceID:           scope.ProcessInstanceID,
		DefinitionID:                scope.DefinitionID,
		ParentID:                    scope.ID,
		ActivityID:                  subscription.EventSubProcessStartID,
		Active:                      true,
		Scope:                       true,
		ScopeRoot:                   true,
		EventSubProcessRoot:         true,
		EventSubProcessInterrupting: subscription.Interrupting,
		Variables:                   make(map[string]any),
	}
	e.state.Executions[root.ID] = root
	cc.agenda.schedule(&continueOperation{executionID: root.ID, activityID: subscription.EventSubProcessStartID})
	return nil
}

// cancelScopeContent 取消作用域内的活动，但保留作用域执行本身。
// @param cc 命令上下文
// @param scope 作用域执行
func (e *Engine) cancelScopeContent(cc *commandContext, scope *executionState) {
	for _, execution := range e.descendantsOf(scope) {
		if execution.ID == scope.ID {
			continue
		}
		for taskID, task := range e.state.Tasks {
			if task.ExecutionID != execution.ID {
				continue
			}
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, taskID)
		}
		e.cancelBoundaryTimers(execution.ID)
		e.removeSubscriptionsForExecution(execution.ID)
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		execution.Ended = true
		execution.Active = false
		execution.Waiting = false
	}
	for taskID, task := range e.state.Tasks {
		if task.ExecutionID != scope.ID {
			continue
		}
		e.recordTaskEnd(cc, task)
		delete(e.state.Tasks, taskID)
	}
	e.cancelBoundaryTimers(scope.ID)
	e.cancelBoundarySubscriptions(scope.ID)
	e.recordActivityEnd(cc, scope.CurrentHistActivityID)
	scope.CurrentHistActivityID = ""
	scope.Waiting = false
	scope.Active = true
}

// endEventSubProcess 处理事件子流程结束。
// @param cc 命令上下文
// @param execution 事件子流程根执行
func (e *Engine) endEventSubProcess(cc *commandContext, execution *executionState) {
	execution.Ended = true
	execution.Active = false
	if !execution.EventSubProcessInterrupting {
		return
	}
	scope, ok := e.findExecution(execution.ParentID)
	if !ok {
		return
	}
	instance, ok := e.findInstance(scope.ProcessInstanceID)
	if !ok {
		return
	}
	if scope.ID == instance.RootExecutionID {
		e.endProcessInstance(cc, instance)
		return
	}
	if scope.ScopeRoot {
		e.endExecution(cc, scope, false)
	}
}
