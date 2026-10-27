package engine

import (
	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// registerCompensation 登记活动的补偿处理器。
// @param cc 命令上下文
// @param execution 执行
// @param activityID 已完成活动 ID
func (e *Engine) registerCompensation(cc *commandContext, execution *executionState, activityID string) {
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return
	}
	process := processOfDefinition(definition)
	if process == nil {
		return
	}
	element, exists := findElement(process, activityID)
	if !exists {
		return
	}
	node, isNode := element.(model.FlowNode)
	if !isNode {
		return
	}
	for _, boundary := range node.GetBoundaryEvents() {
		if !boundary.Compensate || boundary.CompensationHandlerID == "" {
			continue
		}
		entry := &compensationState{
			ID:                e.newID(),
			ProcessInstanceID: execution.ProcessInstanceID,
			ActivityID:        activityID,
			HandlerID:         boundary.CompensationHandlerID,
			CreatedAt:         cc.now,
		}
		e.state.Compensations[execution.ProcessInstanceID] = append(e.state.Compensations[execution.ProcessInstanceID], entry)
	}
}

// compensate 逆序执行已登记的补偿处理器。
// @param cc 命令上下文
// @param execution 执行
// @return 补偿错误
func (e *Engine) compensate(cc *commandContext, execution *executionState) error {
	entries := e.state.Compensations[execution.ProcessInstanceID]
	if len(entries) == 0 {
		return nil
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		handlerElement, exists := findElement(process, entries[i].HandlerID)
		if !exists {
			continue
		}
		switch handler := handlerElement.(type) {
		case *model.ServiceTask:
			histID := e.recordActivityStart(cc, execution, handler)
			if err := e.executeServiceTask(cc, execution, handler); err != nil {
				return err
			}
			e.recordActivityEnd(cc, histID)
		case *model.ManualTask:
			histID := e.recordActivityStart(cc, execution, handler)
			e.recordActivityEnd(cc, histID)
		default:
			return api.NewError("ENGINE-0650", "暂不支持的补偿处理器类型: "+handlerElement.ElementType(), nil)
		}
	}
	e.state.Compensations[execution.ProcessInstanceID] = nil
	return nil
}
