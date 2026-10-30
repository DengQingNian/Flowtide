package engine

import (
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/expr"
)

// startCallActivity 启动被调用流程并让父执行等待。
// @param cc 命令上下文
// @param parent 父执行
// @param call 调用活动
// @return 启动错误
func (e *Engine) startCallActivity(cc *commandContext, parent *executionState, call *model.CallActivity) error {
	called := call.CalledElement
	if called == "" {
		return api.NewError("ENGINE-0610", "调用活动缺少 calledElement", nil)
	}
	if strings.Contains(called, "${") || strings.Contains(called, "#{") {
		value, err := expr.EvalString(called, e.variablesOf(parent))
		if err != nil {
			return api.NewError("ENGINE-0611", "调用活动表达式求值失败", err)
		}
		called = value
	}
	definition, ok := e.findTenantDefinition(cc.ctx, called)
	if !ok {
		return api.NewError("ENGINE-0612", "被调用流程不存在: "+called, api.ErrNotFound)
	}
	if definition.Info.Suspended {
		return api.NewError("ENGINE-0613", "被调用流程已挂起", api.ErrConflict)
	}
	depth := e.callDepth(parent)
	if depth >= 20 {
		return api.NewError("ENGINE-0614", "调用活动嵌套超过 20 层", nil)
	}
	childID := e.newID()
	rootID := childID
	variables := make(map[string]any)
	if call.InheritVariables {
		variables = e.variablesOf(parent)
	}
	// 入参映射覆盖继承变量
	for _, parameter := range call.InParameters {
		value, err := e.ioValue(parent, parameter, e.variablesOf(parent))
		if err != nil {
			return err
		}
		target := parameter.Target
		if target == "" {
			target = parameter.Source
		}
		if target == "" {
			return api.NewError("ENGINE-0615", "调用活动入参缺少 target", api.ErrBadRequest)
		}
		variables[target] = value
	}
	businessKey := ""
	if call.BusinessKey != "" {
		value, err := expr.EvalString(call.BusinessKey, e.variablesOf(parent))
		if err != nil {
			return api.NewError("ENGINE-0616", "调用活动业务键求值失败", err)
		}
		businessKey = value
	}
	name := definition.Info.Name
	if call.NameExpression != "" {
		value, err := expr.EvalString(call.NameExpression, e.variablesOf(parent))
		if err != nil {
			return api.NewError("ENGINE-0617", "调用活动实例名求值失败", err)
		}
		name = value
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0618", "被调用流程无流程元素", nil)
	}
	start := firstStartEvent(process)
	if start == nil {
		return api.NewError("ENGINE-0619", "被调用流程缺少开始事件", nil)
	}
	instance := &instanceState{
		ID:                   childID,
		ProcessDefinitionID:  definition.Info.ID,
		ProcessDefinitionKey: definition.Info.Key,
		BusinessKey:          businessKey,
		Name:                 name,
		TenantID:             parentTenant(e, parent),
		StartTime:            cc.now,
		SuperExecutionID:     parent.ID,
		RootExecutionID:      rootID,
	}
	child := &executionState{
		ID:                rootID,
		ProcessInstanceID: childID,
		DefinitionID:      definition.Info.ID,
		ActivityID:        start.ID,
		Active:            true,
		Scope:             true,
		Variables:         variables,
	}
	e.state.Instances[childID] = instance
	e.state.Executions[rootID] = child
	e.recordProcessStart(cc, instance)
	parent.Waiting = true
	parent.Active = false
	cc.agenda.schedule(&continueOperation{executionID: rootID, activityID: start.ID})
	return nil
}

// resumeParent 在子流程结束后恢复父执行并映射出参。
// @param cc 命令上下文
// @param child 子流程实例
func (e *Engine) resumeParent(cc *commandContext, child *instanceState) {
	parentExecution, ok := e.findExecution(child.SuperExecutionID)
	if !ok || parentExecution.Ended {
		return
	}
	parentInstance, ok := e.findInstance(parentExecution.ProcessInstanceID)
	if !ok || parentInstance.Ended {
		return
	}
	definition, ok := e.findDefinition(parentExecution.DefinitionID)
	if !ok {
		return
	}
	process := processOfDefinition(definition)
	if process == nil {
		return
	}
	element, ok := findElement(process, parentExecution.ActivityID)
	if ok {
		if call, isCall := element.(*model.CallActivity); isCall {
			childRoot, hasRoot := e.rootExecution(child)
			if hasRoot {
				childVariables := e.variablesOf(childRoot)
				for _, parameter := range call.OutParameters {
					value, err := e.ioValue(childRoot, parameter, childVariables)
					if err != nil {
						continue
					}
					target := parameter.Target
					if target == "" {
						target = parameter.Source
					}
					if target != "" {
						e.setVariable(parentExecution, target, value)
					}
				}
			}
		}
	}
	e.cancelBoundaryTimers(parentExecution.ID)
	e.recordActivityEnd(cc, parentExecution.CurrentHistActivityID)
	parentExecution.CurrentHistActivityID = ""
	parentExecution.Waiting = false
	parentExecution.Active = true
	cc.agenda.schedule(&takeOutgoingOperation{executionID: parentExecution.ID, activityID: parentExecution.ActivityID})
}

// ioValue 计算入参/出参值。
// @param execution 执行
// @param parameter 映射
// @param variables 变量表
// @return 值
// @return 错误
func (e *Engine) ioValue(execution *executionState, parameter *model.IOParameter, variables map[string]any) (any, error) {
	if parameter.SourceExpression != "" {
		value, err := expr.Eval(parameter.SourceExpression, variables)
		if err != nil {
			return nil, api.NewError("ENGINE-0620", "调用活动参数表达式求值失败", err)
		}
		return value, nil
	}
	if parameter.Source == "" {
		return nil, nil
	}
	value, _ := e.getVariable(execution, parameter.Source)
	return value, nil
}

// callDepth 计算调用活动嵌套深度。
// @param execution 父执行
// @return 深度
func (e *Engine) callDepth(execution *executionState) int {
	depth := 0
	current := execution
	for current != nil {
		depth++
		instance, ok := e.findInstance(current.ProcessInstanceID)
		if !ok || instance.SuperExecutionID == "" {
			break
		}
		parent, ok := e.findExecution(instance.SuperExecutionID)
		if !ok {
			break
		}
		current = parent
	}
	return depth
}

// parentTenant 返回父实例租户。
// @param engine 引擎
// @param execution 父执行
// @return 租户
func parentTenant(engine *Engine, execution *executionState) string {
	instance, ok := engine.findInstance(execution.ProcessInstanceID)
	if !ok {
		return ""
	}
	return instance.TenantID
}
