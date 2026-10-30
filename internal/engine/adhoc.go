package engine

import (
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/expr"
)

// Ad-hoc 子流程支持的执行顺序。
const (
	// adHocOrderingParallel 并行激活全部内部活动。
	adHocOrderingParallel = "Parallel"
	// adHocOrderingSequential 按文档顺序逐个激活内部活动。
	adHocOrderingSequential = "Sequential"
)

// enterAdHocSubProcess 进入 Ad-hoc 子流程。
// 与普通子流程不同，Ad-hoc 子流程没有开始事件，进入后直接激活内部活动：
// Parallel 为每个活动创建子执行，Sequential 只激活文档顺序中的第一个活动。
// @param cc 命令上下文
// @param execution 父执行
// @param sub Ad-hoc 子流程
// @return 执行错误
func (e *Engine) enterAdHocSubProcess(cc *commandContext, execution *executionState, sub *model.SubProcess) error {
	ordering, err := normalizeAdHocOrdering(sub.AdHocOrdering)
	if err != nil {
		return err
	}
	activities := adHocActivities(sub)
	// 无内部活动：记录子流程活动结束后直接沿出线继续，不创建作用域
	if len(activities) == 0 {
		e.cancelBoundaryTimers(execution.ID)
		e.cancelBoundarySubscriptions(execution.ID)
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		return e.scheduleTakeOutgoing(cc, execution, sub)
	}
	scope := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: execution.ProcessInstanceID,
		DefinitionID:      execution.DefinitionID,
		ParentID:          execution.ID,
		ActivityID:        sub.ID,
		Active:            false,
		Scope:             true,
		ScopeRoot:         true,
		Waiting:           true,
		Variables:         make(map[string]any),
	}
	e.state.Executions[scope.ID] = scope
	execution.Waiting = true
	execution.Active = false
	if definition, ok := e.findDefinition(execution.DefinitionID); ok {
		e.scheduleEventSubProcesses(cc, scope, sub.Elements, sub.Order, definition)
	}
	// 串行模式先激活第一个活动，其余活动在完成后依次激活
	if ordering == adHocOrderingSequential {
		e.createAdHocChild(cc, scope, activities[0], false)
		return nil
	}
	for _, activity := range activities {
		e.createAdHocChild(cc, scope, activity, true)
	}
	return nil
}

// normalizeAdHocOrdering 校验并规范化 Ad-hoc 执行顺序，空值按规范默认为并行。
// @param raw 原始顺序
// @return 规范化后的顺序
// @return 校验错误
func normalizeAdHocOrdering(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return adHocOrderingParallel, nil
	case strings.EqualFold(trimmed, adHocOrderingParallel):
		return adHocOrderingParallel, nil
	case strings.EqualFold(trimmed, adHocOrderingSequential):
		return adHocOrderingSequential, nil
	default:
		return "", api.NewError("ENGINE-0513", "Ad-hoc 子流程 ordering 非法: "+raw, api.ErrBadRequest)
	}
}

// adHocActivities 返回 Ad-hoc 子流程中需要动态激活的内部活动，按文档顺序。
// @param sub Ad-hoc 子流程
// @return 内部活动列表
func adHocActivities(sub *model.SubProcess) []model.FlowNode {
	if sub == nil {
		return nil
	}
	out := make([]model.FlowNode, 0, len(sub.Order))
	for _, id := range sub.Order {
		element := sub.Elements[id]
		node, ok := element.(model.FlowNode)
		// 开始/结束/边界事件与事件子流程不作为 Ad-hoc 活动
		if !ok || !isAdHocActivity(element) {
			continue
		}
		out = append(out, node)
	}
	return out
}

// isAdHocActivity 判断元素是否可作为 Ad-hoc 子流程的直接活动。
// @param element 流程元素
// @return 是否可作为活动
func isAdHocActivity(element model.FlowElement) bool {
	switch typed := element.(type) {
	case *model.StartEvent, *model.EndEvent, *model.BoundaryEvent:
		return false
	case *model.SubProcess:
		return !typed.TriggeredByEvent
	}
	return true
}

// createAdHocChild 为 Ad-hoc 内部活动创建子执行并调度进入。
// @param cc 命令上下文
// @param scope Ad-hoc 作用域根执行
// @param node 内部活动
// @param concurrent 是否并行子执行
func (e *Engine) createAdHocChild(cc *commandContext, scope *executionState, node model.FlowNode, concurrent bool) {
	child := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: scope.ProcessInstanceID,
		DefinitionID:      scope.DefinitionID,
		ParentID:          scope.ID,
		ActivityID:        node.GetID(),
		Active:            true,
		Concurrent:        concurrent,
		Variables:         make(map[string]any),
	}
	e.state.Executions[child.ID] = child
	cc.agenda.schedule(&continueOperation{executionID: child.ID, activityID: node.GetID()})
}

// adHocScopeOf 判断执行是否为 Ad-hoc 子流程的内部活动，并返回所属作用域与子流程。
// @param execution 执行
// @return Ad-hoc 作用域根执行
// @return Ad-hoc 子流程
// @return 是否命中
func (e *Engine) adHocScopeOf(execution *executionState) (*executionState, *model.SubProcess, bool) {
	// 作用域根、事件子流程根与根执行本身不是内部活动
	if execution == nil || execution.ScopeRoot || execution.EventSubProcessRoot || execution.ParentID == "" {
		return nil, nil, false
	}
	parent, ok := e.findExecution(execution.ParentID)
	if !ok {
		return nil, nil, false
	}
	// 只有 Ad-hoc 作用域根的子执行才属于内部活动，避免把子流程宿主执行误判为内部活动
	if !parent.ScopeRoot {
		return nil, nil, false
	}
	sub, ok := e.subProcessAt(parent)
	if !ok || !sub.AdHoc {
		return nil, nil, false
	}
	return parent, sub, true
}

// subProcessAt 返回执行当前活动对应的子流程定义。
// @param execution 执行
// @return 子流程
// @return 是否存在
func (e *Engine) subProcessAt(execution *executionState) (*model.SubProcess, bool) {
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return nil, false
	}
	process := processOfDefinition(definition)
	if process == nil {
		return nil, false
	}
	element, ok := findElement(process, execution.ActivityID)
	if !ok {
		return nil, false
	}
	sub, ok := element.(*model.SubProcess)
	return sub, ok
}

// finishAdHocActivity 处理 Ad-hoc 内部活动完成。
// 先结束子执行，再按完成条件、并行剩余活动或串行下一活动决定是否离开子流程。
// @param cc 命令上下文
// @param execution 已完成的内部活动执行
// @param scope Ad-hoc 作用域根执行
// @param sub Ad-hoc 子流程
// @return 执行错误
func (e *Engine) finishAdHocActivity(cc *commandContext, execution, scope *executionState, sub *model.SubProcess) error {
	execution.Ended = true
	execution.Active = false
	execution.Waiting = false
	e.cancelBoundaryTimers(execution.ID)
	e.cancelBoundarySubscriptions(execution.ID)
	e.removeSubscriptionsForExecution(execution.ID)
	// 有完成条件时，每个内部活动完成后求值，为真则取消剩余活动并离开
	if strings.TrimSpace(sub.CompletionCondition) != "" {
		matched, err := expr.EvalBool(sub.CompletionCondition, e.variablesOf(scope))
		if err != nil {
			return api.NewError("ENGINE-0514", "Ad-hoc 子流程完成条件求值失败", err)
		}
		if matched {
			e.cancelRemainingAdHocChildren(cc, scope)
			e.endExecution(cc, scope, false)
			return nil
		}
	}
	// 并行模式仍有内部活动未完成时继续等待
	if e.hasRunningAdHocChildren(scope) {
		return nil
	}
	// 串行模式按文档顺序激活下一个尚未执行的活动
	if strings.EqualFold(strings.TrimSpace(sub.AdHocOrdering), adHocOrderingSequential) {
		if next, ok := e.nextAdHocActivity(scope, sub); ok {
			e.createAdHocChild(cc, scope, next, false)
			return nil
		}
	}
	e.endExecution(cc, scope, false)
	return nil
}

// hasRunningAdHocChildren 判断作用域内是否仍有未完成的内部活动执行。
// @param scope Ad-hoc 作用域根执行
// @return 是否仍有运行中的内部执行
func (e *Engine) hasRunningAdHocChildren(scope *executionState) bool {
	for _, execution := range e.state.Executions {
		if execution.ParentID == scope.ID && !execution.Ended {
			return true
		}
	}
	return false
}

// nextAdHocActivity 返回串行模式下尚未激活的下一个内部活动。
// @param scope Ad-hoc 作用域根执行
// @param sub Ad-hoc 子流程
// @return 内部活动
// @return 是否存在
func (e *Engine) nextAdHocActivity(scope *executionState, sub *model.SubProcess) (model.FlowNode, bool) {
	started := make(map[string]bool)
	for _, execution := range e.state.Executions {
		if execution.ParentID == scope.ID {
			started[execution.ActivityID] = true
		}
	}
	for _, id := range sub.Order {
		element := sub.Elements[id]
		node, ok := element.(model.FlowNode)
		if !ok || !isAdHocActivity(element) || started[id] {
			continue
		}
		return node, true
	}
	return nil, false
}

// cancelRemainingAdHocChildren 取消 Ad-hoc 作用域内尚未完成的执行、任务与订阅。
// @param cc 命令上下文
// @param scope Ad-hoc 作用域根执行
func (e *Engine) cancelRemainingAdHocChildren(cc *commandContext, scope *executionState) {
	for _, execution := range e.descendantsOf(scope) {
		if execution.ID == scope.ID || execution.Ended {
			continue
		}
		for taskID, task := range e.state.Tasks {
			if task.ExecutionID != execution.ID {
				continue
			}
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, taskID)
		}
		e.recordActivityEnd(cc, execution.CurrentHistActivityID)
		execution.CurrentHistActivityID = ""
		e.cancelBoundaryTimers(execution.ID)
		e.cancelBoundarySubscriptions(execution.ID)
		e.removeSubscriptionsForExecution(execution.ID)
		execution.Ended = true
		execution.Active = false
		execution.Waiting = false
	}
}
