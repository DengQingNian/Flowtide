package xml

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
)

// 支持的命名空间常量。
const (
	// NSBPMN 是 BPMN 2.0 模型命名空间。
	NSBPMN = "http://www.omg.org/spec/BPMN/20100524/MODEL"
	// NSFlowable 是 Flowable 扩展命名空间。
	NSFlowable = "http://flowable.org/bpmn"
	// NSActiviti 是 Activiti 兼容命名空间。
	NSActiviti = "http://activiti.org/bpmn"
	// MaxBPMNSize 是单个 BPMN 文件的最大字节数。
	MaxBPMNSize = 16 << 20
)

// Parse 解析 BPMN XML 为模型。
// @param data XML 内容
// @return BPMN 模型
// @return 解析错误
func Parse(data []byte) (*model.BpmnModel, error) {
	if len(data) == 0 {
		return nil, api.NewError("BPMN-1000", "BPMN 内容为空", nil)
	}
	root, err := Decode(bytes.NewReader(data), MaxBPMNSize)
	if err != nil {
		return nil, api.NewError("BPMN-1000", "BPMN XML 解析失败", err)
	}
	if root.Name.Local != "definitions" {
		return nil, api.NewError("BPMN-1000", "根元素必须是 definitions", nil)
	}
	result := &model.BpmnModel{}
	for _, messageNode := range root.ChildrenNamed("message") {
		result.Messages = append(result.Messages, &model.Message{ID: messageNode.Attr("id"), Name: messageNode.Attr("name")})
	}
	for _, signalNode := range root.ChildrenNamed("signal") {
		result.Signals = append(result.Signals, &model.Signal{ID: signalNode.Attr("id"), Name: signalNode.Attr("name")})
	}
	for _, errorNode := range root.ChildrenNamed("error") {
		result.Errors = append(result.Errors, &model.Error{ID: errorNode.Attr("id"), ErrorCode: errorNode.Attr("errorCode")})
	}
	for _, processNode := range root.ChildrenNamed("process") {
		process, parseErr := parseProcess(processNode)
		if parseErr != nil {
			return nil, parseErr
		}
		result.Processes = append(result.Processes, process)
	}
	if len(result.Processes) == 0 {
		return nil, api.NewError("BPMN-1001", "未找到 process 元素", nil)
	}
	return result, nil
}

// parseProcess 解析单个流程。
// @param node 流程节点
// @return 流程模型
// @return 解析错误
func parseProcess(node *Node) (*model.Process, error) {
	id := node.Attr("id")
	if id == "" {
		return nil, api.NewError("BPMN-1001", "process 缺少 id", nil)
	}
	process := &model.Process{
		ID:         id,
		Name:       node.Attr("name"),
		Executable: strings.EqualFold(node.Attr("isExecutable"), "true"),
		Elements:   make(map[string]model.FlowElement),
	}
	if doc, ok := node.Child("documentation"); ok {
		process.Documentation = doc.TextContent()
	}
	for _, child := range node.Children {
		element, err := parseElement(child)
		if err != nil {
			return nil, err
		}
		if element != nil {
			if _, exists := process.Elements[element.GetID()]; exists {
				return nil, api.NewError("BPMN-1002", fmt.Sprintf("元素 ID 重复: %s", element.GetID()), nil)
			}
			process.AddElement(element)
		}
	}
	if err := linkSequenceFlows(process); err != nil {
		return nil, err
	}
	if err := attachBoundaryEvents(process.Elements, process.Order); err != nil {
		return nil, err
	}
	if err := attachCompensationHandlers(process.Elements, process.Order); err != nil {
		return nil, err
	}
	if len(process.StartEvents()) == 0 {
		return nil, api.NewError("BPMN-1003", fmt.Sprintf("流程 %s 缺少开始事件", process.ID), nil)
	}
	return process, nil
}

// parseElement 解析单个 BPMN 元素。
// @param node XML 节点
// @return 流程元素，非流程元素返回 nil
// @return 解析错误
func parseElement(node *Node) (model.FlowElement, error) {
	switch node.Name.Local {
	case "startEvent":
		event := &model.StartEvent{}
		parseFlowNodeBase(node, &event.FlowNodeBase)
		event.FormKey = attrAny(node, "formKey")
		event.Initiator = attrAny(node, "initiator")
		event.MessageRef, event.SignalRef, event.Timer = parseEventDefinition(node)
		event.Interrupting = !strings.EqualFold(node.Attr("isInterrupting"), "false")
		event.FormProperties = parseFormProperties(node)
		return event, nil
	case "endEvent":
		event := &model.EndEvent{
			Terminate: hasChild(node, "terminateEventDefinition"),
			Cancel:    hasChild(node, "cancelEventDefinition"),
			ErrorRef:  parseErrorRef(node),
		}
		parseFlowNodeBase(node, &event.FlowNodeBase)
		return event, nil
	case "userTask":
		task := &model.UserTask{}
		parseFlowNodeBase(node, &task.FlowNodeBase)
		task.Assignee = attrAny(node, "assignee")
		task.CandidateUsers = splitList(attrAny(node, "candidateUsers"))
		task.CandidateGroups = splitList(attrAny(node, "candidateGroups"))
		task.DueDate = attrAny(node, "dueDate")
		task.Priority = attrAny(node, "priority")
		task.FormKey = attrAny(node, "formKey")
		task.Category = attrAny(node, "category")
		task.LoopCharacteristics = parseMultiInstance(node)
		task.FormProperties = parseFormProperties(node)
		return task, nil
	case "serviceTask":
		task := &model.ServiceTask{}
		parseFlowNodeBase(node, &task.FlowNodeBase)
		task.DelegateExpression = attrAny(node, "delegateExpression")
		task.Expression = attrAny(node, "expression")
		task.ClassName = attrAny(node, "class")
		task.ResultVariable = attrAny(node, "resultVariable")
		task.LoopCharacteristics = parseMultiInstance(node)
		switch {
		case task.DelegateExpression != "":
			task.ImplementationType = "delegateExpression"
		case task.Expression != "":
			task.ImplementationType = "expression"
		case task.ClassName != "":
			task.ImplementationType = "class"
		}
		return task, nil
	case "scriptTask":
		task := &model.ScriptTask{ScriptFormat: node.Attr("scriptFormat")}
		parseFlowNodeBase(node, &task.FlowNodeBase)
		if script, ok := node.Child("script"); ok {
			task.Script = script.TextContent()
		}
		task.ResultVariable = attrAny(node, "resultVariable")
		return task, nil
	case "manualTask":
		task := &model.ManualTask{}
		parseFlowNodeBase(node, &task.FlowNodeBase)
		return task, nil
	case "exclusiveGateway":
		gateway := &model.ExclusiveGateway{DefaultFlow: node.Attr("default")}
		parseFlowNodeBase(node, &gateway.FlowNodeBase)
		return gateway, nil
	case "eventBasedGateway":
		gateway := &model.EventGateway{}
		parseFlowNodeBase(node, &gateway.FlowNodeBase)
		return gateway, nil
	case "inclusiveGateway":
		gateway := &model.InclusiveGateway{DefaultFlow: node.Attr("default")}
		parseFlowNodeBase(node, &gateway.FlowNodeBase)
		return gateway, nil
	case "parallelGateway":
		gateway := &model.ParallelGateway{}
		parseFlowNodeBase(node, &gateway.FlowNodeBase)
		return gateway, nil
	case "receiveTask":
		task := &model.ReceiveTask{MessageRef: attrAny(node, "messageRef"), SignalRef: attrAny(node, "signalRef")}
		parseFlowNodeBase(node, &task.FlowNodeBase)
		return task, nil
	case "sendTask":
		task := &model.SendTask{MessageRef: attrAny(node, "messageRef"), SignalRef: attrAny(node, "signalRef")}
		parseFlowNodeBase(node, &task.FlowNodeBase)
		return task, nil
	case "intermediateThrowEvent":
		event := &model.IntermediateThrowEvent{Compensate: hasChild(node, "compensateEventDefinition")}
		parseFlowNodeBase(node, &event.FlowNodeBase)
		return event, nil
	case "intermediateCatchEvent":
		event := &model.IntermediateCatchEvent{}
		parseFlowNodeBase(node, &event.FlowNodeBase)
		event.MessageRef, event.SignalRef, event.Timer = parseEventDefinition(node)
		return event, nil
	case "callActivity":
		activity := &model.CallActivity{
			CalledElement:    attrAny(node, "calledElement"),
			InheritVariables: strings.EqualFold(attrAny(node, "inheritVariables"), "true"),
			BusinessKey:      attrAny(node, "businessKey"),
			NameExpression:   attrAny(node, "processInstanceName"),
		}
		parseFlowNodeBase(node, &activity.FlowNodeBase)
		activity.InParameters, activity.OutParameters = parseIOParameters(node)
		return activity, nil
	case "boundaryEvent":
		messageRef, signalRef, timer := parseEventDefinition(node)
		event := &model.BoundaryEvent{
			AttachedToRef:  node.Attr("attachedToRef"),
			CancelActivity: !strings.EqualFold(node.Attr("cancelActivity"), "false"),
			Compensate:     hasChild(node, "compensateEventDefinition"),
			Timer:          timer,
			MessageRef:     messageRef,
			SignalRef:      signalRef,
			ErrorRef:       parseErrorRef(node),
			Cancel:         hasChild(node, "cancelEventDefinition"),
		}
		parseFlowNodeBase(node, &event.FlowNodeBase)
		if event.AttachedToRef == "" {
			return nil, api.NewError("BPMN-1006", "边界事件缺少 attachedToRef", nil)
		}
		return event, nil
	case "subProcess":
		return parseSubProcess(node, false)
	case "transaction":
		return parseSubProcess(node, true)
	case "association":
		association := &model.Association{SourceRef: node.Attr("sourceRef"), TargetRef: node.Attr("targetRef")}
		association.ID = node.Attr("id")
		association.Name = node.Attr("name")
		return association, nil
	case "sequenceFlow":
		flow := &model.SequenceFlow{
			SourceRef: node.Attr("sourceRef"),
			TargetRef: node.Attr("targetRef"),
		}
		flow.ID = node.Attr("id")
		flow.Name = node.Attr("name")
		if flow.ID == "" {
			return nil, api.NewError("BPMN-1004", "sequenceFlow 缺少 id", nil)
		}
		if condition, ok := node.Child("conditionExpression"); ok {
			flow.Condition = condition.TextContent()
		}
		return flow, nil
	default:
		return nil, nil
	}
}

// parseFlowNodeBase 解析流程节点公共字段。
// @param node XML 节点
// @param base 目标基础结构
func parseFlowNodeBase(node *Node, base *model.FlowNodeBase) {
	base.ID = node.Attr("id")
	base.Name = node.Attr("name")
	async := strings.EqualFold(attrAny(node, "async"), "true")
	base.AsyncBefore = async || strings.EqualFold(attrAny(node, "asyncBefore"), "true")
	base.AsyncAfter = strings.EqualFold(attrAny(node, "asyncAfter"), "true")
	base.Exclusive = !strings.EqualFold(attrAny(node, "exclusive"), "false")
	base.ForCompensation = strings.EqualFold(attrAny(node, "isForCompensation"), "true")
	if doc, ok := node.Child("documentation"); ok {
		base.Documentation = doc.TextContent()
	}
}

// parseFormProperties 解析表单字段。
// @param node 活动节点
// @return 表单字段列表
func parseFormProperties(node *Node) []*model.FormProperty {
	extension, ok := node.Child("extensionElements")
	if !ok {
		return nil
	}
	properties := make([]*model.FormProperty, 0)
	for _, child := range extension.Children {
		if child.Name.Local != "formProperty" {
			continue
		}
		property := &model.FormProperty{
			ID:       child.Attr("id"),
			Name:     child.Attr("name"),
			Type:     child.Attr("type"),
			Required: strings.EqualFold(child.Attr("required"), "true"),
			Readable: !strings.EqualFold(child.Attr("readable"), "false"),
			Writable: !strings.EqualFold(child.Attr("writable"), "false"),
			Value:    child.Attr("value"),
		}
		if property.Type == "" {
			property.Type = "string"
		}
		for _, valueNode := range child.ChildrenNamed("value") {
			property.EnumValues = append(property.EnumValues, &model.FormValue{ID: valueNode.Attr("id"), Name: valueNode.Attr("name")})
		}
		properties = append(properties, property)
	}
	return properties
}

// parseMultiInstance 解析多实例配置。
// @param node 活动节点
// @return 多实例配置，不存在时为 nil
func parseMultiInstance(node *Node) *model.MultiInstanceLoopCharacteristics {
	loopNode, ok := node.Child("multiInstanceLoopCharacteristics")
	if !ok {
		return nil
	}
	loop := &model.MultiInstanceLoopCharacteristics{
		Sequential:      strings.EqualFold(loopNode.Attr("isSequential"), "true"),
		Collection:      attrAny(loopNode, "collection"),
		ElementVariable: attrAny(loopNode, "elementVariable"),
	}
	if child, exists := loopNode.Child("loopCardinality"); exists {
		loop.LoopCardinality = child.TextContent()
	}
	if child, exists := loopNode.Child("completionCondition"); exists {
		loop.CompletionCondition = child.TextContent()
	}
	return loop
}

// linkSequenceFlows 解析流程顺序流并建立节点连接。
// @param process 流程
// @return 连接错误
func linkSequenceFlows(process *model.Process) error {
	return linkFlows(process.Elements, process.Order)
}

// linkFlows 解析指定容器的顺序流并建立连接。
// @param elements 元素表
// @param order 元素顺序
// @return 连接错误
func linkFlows(elements map[string]model.FlowElement, order []string) error {
	for _, id := range order {
		flow, ok := elements[id].(*model.SequenceFlow)
		if !ok {
			continue
		}
		source, ok := lookupFlowNode(elements, flow.SourceRef)
		if !ok {
			return api.NewError("BPMN-1005", fmt.Sprintf("顺序流 %s 的源节点不存在: %s", flow.ID, flow.SourceRef), nil)
		}
		target, ok := lookupFlowNode(elements, flow.TargetRef)
		if !ok {
			return api.NewError("BPMN-1005", fmt.Sprintf("顺序流 %s 的目标节点不存在: %s", flow.ID, flow.TargetRef), nil)
		}
		source.AddOutgoing(flow)
		target.AddIncoming(flow)
	}
	// 标记默认流，供排他网关与条件分支使用
	for _, id := range order {
		switch gateway := elements[id].(type) {
		case *model.ExclusiveGateway:
			if gateway.DefaultFlow != "" {
				if flow, exists := elements[gateway.DefaultFlow].(*model.SequenceFlow); exists {
					flow.IsDefault = true
				}
			}
		case *model.InclusiveGateway:
			if gateway.DefaultFlow != "" {
				if flow, exists := elements[gateway.DefaultFlow].(*model.SequenceFlow); exists {
					flow.IsDefault = true
				}
			}
		}
	}
	return nil
}

// lookupFlowNode 在元素表中查找流程节点。
// @param elements 元素表
// @param id 元素 ID
// @return 流程节点
// @return 是否存在
func lookupFlowNode(elements map[string]model.FlowElement, id string) (model.FlowNode, bool) {
	element, ok := elements[id]
	if !ok {
		return nil, false
	}
	node, ok := element.(model.FlowNode)
	return node, ok
}

// parseErrorRef 解析错误引用。
// @param node 事件节点
// @return 错误引用
func parseErrorRef(node *Node) string {
	if child, ok := node.Child("errorEventDefinition"); ok {
		return attrAny(child, "errorRef")
	}
	return ""
}

// parseEventDefinition 解析事件定义。
// @param node 事件节点
// @return 消息引用
// @return 信号引用
// @return 定时器定义
func parseEventDefinition(node *Node) (string, string, *model.TimerDefinition) {
	var messageRef string
	var signalRef string
	var timer *model.TimerDefinition
	if child, ok := node.Child("messageEventDefinition"); ok {
		messageRef = attrAny(child, "messageRef")
	}
	if child, ok := node.Child("signalEventDefinition"); ok {
		signalRef = attrAny(child, "signalRef")
	}
	if child, ok := node.Child("timerEventDefinition"); ok {
		timer = &model.TimerDefinition{}
		if value, exists := child.Child("timeDate"); exists {
			timer.TimeDate = value.TextContent()
		}
		if value, exists := child.Child("timeDuration"); exists {
			timer.TimeDuration = value.TextContent()
		}
		if value, exists := child.Child("timeCycle"); exists {
			timer.TimeCycle = value.TextContent()
		}
	}
	return messageRef, signalRef, timer
}

// parseIOParameters 解析调用活动入参与出参。
// @param node 调用活动节点
// @return 入参列表
// @return 出参列表
func parseIOParameters(node *Node) ([]*model.IOParameter, []*model.IOParameter) {
	var inputs []*model.IOParameter
	var outputs []*model.IOParameter
	for _, child := range node.Children {
		if child.Name.Local != "extensionElements" {
			continue
		}
		for _, parameter := range child.Children {
			if parameter.Name.Local != "in" && parameter.Name.Local != "out" {
				continue
			}
			item := &model.IOParameter{
				Source:           attrAny(parameter, "source"),
				Target:           attrAny(parameter, "target"),
				SourceExpression: attrAny(parameter, "sourceExpression"),
			}
			if parameter.Name.Local == "in" {
				inputs = append(inputs, item)
			} else {
				outputs = append(outputs, item)
			}
		}
	}
	return inputs, outputs
}

// attachBoundaryEvents 将边界事件挂到依附活动。
// @param elements 元素表
// @param order 元素顺序
// @return 挂载错误
func attachBoundaryEvents(elements map[string]model.FlowElement, order []string) error {
	for _, id := range order {
		event, ok := elements[id].(*model.BoundaryEvent)
		if !ok {
			continue
		}
		node, ok := lookupFlowNode(elements, event.AttachedToRef)
		if !ok {
			return api.NewError("BPMN-1007", fmt.Sprintf("边界事件 %s 的依附活动不存在: %s", event.ID, event.AttachedToRef), nil)
		}
		node.AddBoundaryEvent(event)
	}
	return nil
}

// attachCompensationHandlers 绑定补偿边界与处理器。
// @param elements 元素表
// @param order 元素顺序
// @return 绑定错误
func attachCompensationHandlers(elements map[string]model.FlowElement, order []string) error {
	for _, id := range order {
		boundary, ok := elements[id].(*model.BoundaryEvent)
		if !ok || !boundary.Compensate {
			continue
		}
		for _, associationID := range order {
			association, exists := elements[associationID].(*model.Association)
			if !exists || association.SourceRef != boundary.ID {
				continue
			}
			if _, handlerExists := elements[association.TargetRef]; !handlerExists {
				return api.NewError("BPMN-1008", fmt.Sprintf("补偿处理器不存在: %s", association.TargetRef), nil)
			}
			boundary.CompensationHandlerID = association.TargetRef
			break
		}
	}
	return nil
}

// parseSubProcess 解析嵌入式子流程。
// @param node XML 节点
// @return 子流程
// @return 解析错误
func parseSubProcess(node *Node, transaction bool) (model.FlowElement, error) {
	sub := &model.SubProcess{
		TriggeredByEvent: strings.EqualFold(node.Attr("triggeredByEvent"), "true"),
		Transaction:      transaction,
		Elements:         make(map[string]model.FlowElement),
	}
	parseFlowNodeBase(node, &sub.FlowNodeBase)
	for _, child := range node.Children {
		element, err := parseElement(child)
		if err != nil {
			return nil, err
		}
		if element == nil {
			continue
		}
		if _, exists := sub.Elements[element.GetID()]; exists {
			return nil, api.NewError("BPMN-1002", fmt.Sprintf("子流程元素 ID 重复: %s", element.GetID()), nil)
		}
		sub.AddElement(element)
	}
	if err := linkFlows(sub.Elements, sub.Order); err != nil {
		return nil, err
	}
	if err := attachBoundaryEvents(sub.Elements, sub.Order); err != nil {
		return nil, err
	}
	if err := attachCompensationHandlers(sub.Elements, sub.Order); err != nil {
		return nil, err
	}
	if len(sub.StartEvents()) == 0 {
		return nil, api.NewError("BPMN-1003", fmt.Sprintf("子流程 %s 缺少开始事件", sub.ID), nil)
	}
	return sub, nil
}

// attrAny 依次尝试 Flowable、Activiti 与无命名空间属性。
// @param node XML 节点
// @param local 本地名
// @return 属性值
func attrAny(node *Node, local string) string {
	for _, space := range []string{NSFlowable, NSActiviti, ""} {
		if value := node.AttrNS(space, local); value != "" {
			return value
		}
	}
	return node.Attr(local)
}

// splitList 按逗号拆分并去除空白与空项。
// @param raw 原始字符串
// @return 字符串列表
func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// hasChild 判断是否存在指定本地名的子节点。
// @param node XML 节点
// @param local 本地名
// @return 是否存在
func hasChild(node *Node, local string) bool {
	_, ok := node.Child(local)
	return ok
}
