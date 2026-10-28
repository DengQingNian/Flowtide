package engine

import (
	"context"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// ExecutionListener 是 BPMN 执行监听器。
// 监听器在命令事务内同步执行，返回错误会使整个命令回滚。
type ExecutionListener func(ctx context.Context, execution *Execution) error

// executionListenerProvider 是暴露执行监听器定义的节点。
type executionListenerProvider interface {
	// GetExecutionListeners 返回节点级执行监听器定义。
	// @return 执行监听器列表
	GetExecutionListeners() []model.ExecutionListenerDefinition
}

// setListenerError 记录监听器错误，保留首个错误。
// @param err 监听器错误
func (cc *commandContext) setListenerError(err error) {
	if err != nil && cc.listenerErr == nil {
		cc.listenerErr = err
	}
}

// newExecution 构建监听器可见的执行上下文。
// @param engine 引擎
// @param state 执行状态
// @return 执行上下文
func newExecution(engine *Engine, state *executionState) *Execution {
	return &Execution{engine: engine, state: state, instance: engine.state.Instances[state.ProcessInstanceID]}
}

// executionListenersOf 返回元素的执行监听器定义。
// @param element 流程元素
// @return 执行监听器列表
func executionListenersOf(element model.FlowElement) []model.ExecutionListenerDefinition {
	provider, ok := element.(executionListenerProvider)
	if !ok {
		return nil
	}
	return provider.GetExecutionListeners()
}

// invokeProcessExecutionListeners 调用流程级执行监听器。
// @param cc 命令上下文
// @param instance 流程实例
// @param event 事件：start/end
// @return 执行错误
func (e *Engine) invokeProcessExecutionListeners(cc *commandContext, instance *instanceState, event string) error {
	definition, ok := e.findDefinition(instance.ProcessDefinitionID)
	if !ok {
		return nil
	}
	process := processOfDefinition(definition)
	if process == nil || len(process.ExecutionListeners) == 0 {
		return nil
	}
	execution, ok := e.rootExecution(instance)
	if !ok {
		return nil
	}
	return e.invokeExecutionListeners(cc, newExecution(e, execution), process.ExecutionListeners, event)
}

// invokeActivityStartExecutionListeners 调用活动开始监听器。
// 监听器错误写入命令上下文，由命令边界统一回滚。
// @param cc 命令上下文
// @param execution 执行
// @param element 活动元素
func (e *Engine) invokeActivityStartExecutionListeners(cc *commandContext, execution *executionState, element model.FlowElement) {
	listeners := executionListenersOf(element)
	if len(listeners) == 0 {
		return
	}
	err := e.invokeExecutionListeners(cc, newExecution(e, execution), listeners, "start")
	cc.setListenerError(err)
}

// invokeActivityEndExecutionListeners 根据历史活动调用活动结束监听器。
// 历史记录缺失（例如历史级别不足）时静默跳过，不产生 panic。
// @param cc 命令上下文
// @param historic 历史活动记录
func (e *Engine) invokeActivityEndExecutionListeners(cc *commandContext, historic *api.HistoricActivityInstance) {
	if historic == nil {
		return
	}
	instance, ok := e.findInstance(historic.ProcessInstanceID)
	if !ok {
		return
	}
	execution, ok := e.rootExecution(instance)
	if !ok {
		return
	}
	definition, ok := e.findDefinition(instance.ProcessDefinitionID)
	if !ok {
		return
	}
	process := processOfDefinition(definition)
	if process == nil {
		return
	}
	element, ok := findElement(process, historic.ActivityID)
	if !ok {
		return
	}
	listeners := executionListenersOf(element)
	if len(listeners) == 0 {
		return
	}
	// 结束监听器看到的执行可能已离开该活动，显式覆盖活动 ID
	contextExecution := newExecution(e, execution)
	contextExecution.activityID = historic.ActivityID
	err := e.invokeExecutionListeners(cc, contextExecution, listeners, "end")
	cc.setListenerError(err)
}

// invokeExecutionListeners 按定义顺序执行匹配事件的监听器。
// @param cc 命令上下文
// @param execution 执行上下文
// @param definitions 监听器定义
// @param event 事件：start/end
// @return 执行错误
func (e *Engine) invokeExecutionListeners(cc *commandContext, execution *Execution, definitions []model.ExecutionListenerDefinition, event string) error {
	for _, definition := range definitions {
		declaredEvent := strings.TrimSpace(definition.Event)
		// 事件缺省按开始事件处理
		if declaredEvent == "" {
			declaredEvent = "start"
		}
		// 仅执行事件匹配的监听器，事件不区分大小写
		if !strings.EqualFold(declaredEvent, event) {
			continue
		}
		if strings.TrimSpace(definition.Expression) != "" {
			return api.NewError("ENGINE-1942", "暂不支持表达式执行监听器: "+definition.Expression, api.ErrBadRequest)
		}
		name := strings.TrimSpace(definition.DelegateExpression)
		if name == "" {
			name = strings.TrimSpace(definition.ClassName)
		}
		if name == "" {
			return api.NewError("ENGINE-1940", "执行监听器缺少实现", api.ErrBadRequest)
		}
		listener, ok := e.executionListeners[name]
		if !ok {
			return api.NewError("ENGINE-1941", "执行监听器未注册: "+name, api.ErrBadRequest)
		}
		if err := listener(cc.ctx, execution); err != nil {
			return api.NewError("ENGINE-1943", "执行监听器执行失败: "+name, err)
		}
	}
	return nil
}
