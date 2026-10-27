package engine

import (
	"github.com/flowable-go/flowable/bpmn/model"
)

// throwError 抛出 BPMN 错误，沿作用域向上寻找错误边界。
// @param cc 命令上下文
// @param execution 抛出错误的执行
// @param errorRef 错误引用
// @return 执行错误
func (e *Engine) throwError(cc *commandContext, execution *executionState, errorRef string) error {
	instance, ok := e.findInstance(execution.ProcessInstanceID)
	if !ok {
		return nil
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return nil
	}
	process := processOfDefinition(definition)
	if process == nil {
		return nil
	}
	code := errorCode(definition.Model, errorRef)
	scope := e.scopeRoot(execution)
	for scope != nil && scope.ParentID != "" {
		parent, parentOK := e.findExecution(scope.ParentID)
		if !parentOK {
			break
		}
		element, exists := findElement(process, parent.ActivityID)
		if !exists {
			break
		}
		sub, isSub := element.(*model.SubProcess)
		if !isSub {
			break
		}
		for _, boundary := range sub.GetBoundaryEvents() {
			if boundary.ErrorRef == "" {
				continue
			}
			if errorCode(definition.Model, boundary.ErrorRef) != code {
				continue
			}
			e.destroyScope(cc, scope)
			parent.Waiting = false
			parent.Active = true
			e.cancelBoundaryTimers(parent.ID)
			e.cancelBoundarySubscriptions(parent.ID)
			e.recordActivityEnd(cc, parent.CurrentHistActivityID)
			parent.CurrentHistActivityID = ""
			parent.ActivityID = boundary.ID
			cc.agenda.schedule(&continueOperation{executionID: parent.ID, activityID: boundary.ID})
			return nil
		}
		scope = e.scopeRoot(parent)
	}
	// 没有匹配的边界事件：流程实例失败
	instance.DeleteReason = "BPMN error: " + code
	e.recordActivityEnd(cc, execution.CurrentHistActivityID)
	e.endProcessInstance(cc, instance)
	return nil
}

// scopeRoot 返回执行所在的作用域根。
// @param execution 执行
// @return 作用域根执行
func (e *Engine) scopeRoot(execution *executionState) *executionState {
	current := execution
	for current != nil {
		if current.ScopeRoot || current.ParentID == "" {
			return current
		}
		parent, ok := e.findExecution(current.ParentID)
		if !ok {
			return current
		}
		current = parent
	}
	return execution
}

// destroyScope 销毁作用域内的执行、任务、作业与订阅。
// @param cc 命令上下文
// @param scope 作用域根执行
func (e *Engine) destroyScope(cc *commandContext, scope *executionState) {
	descendants := e.descendantsOf(scope)
	for _, execution := range descendants {
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
}

// descendantsOf 返回执行及其全部后代。
// @param root 根执行
// @return 执行列表
func (e *Engine) descendantsOf(root *executionState) []*executionState {
	out := []*executionState{root}
	queue := []*executionState{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, execution := range e.state.Executions {
			if execution.ParentID == current.ID {
				out = append(out, execution)
				queue = append(queue, execution)
			}
		}
	}
	return out
}

// errorCode 解析错误码。
// @param bpmnModel 模型
// @param ref 错误引用
// @return 错误码
func errorCode(bpmnModel *model.BpmnModel, ref string) string {
	for _, definition := range bpmnModel.Errors {
		if definition.ID != ref {
			continue
		}
		if definition.ErrorCode != "" {
			return definition.ErrorCode
		}
		return definition.ID
	}
	return ref
}
