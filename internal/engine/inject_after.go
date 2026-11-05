package engine

import (
	"bytes"
	"context"
	"encoding/xml"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	bpmnxml "github.com/DengQingNian/Flowtide/internal/bpmn/xml"
)

// InjectUserTaskAfter 为单个实例派生定义，在指定用户节点后增加节点并迁移该实例的定义引用。
// @param ctx 上下文
// @param processInstanceID 运行实例ID
// @param req 新节点属性和可选预期定义
// @return 新派生定义，不会立即生成运行任务
// @return 校验或持久化错误，失败时整个命令回滚
func (e *Engine) InjectUserTaskAfter(ctx context.Context, processInstanceID string, req api.InsertUserTaskRequest) (api.ProcessDefinition, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		instance, ok := e.findInstance(processInstanceID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
		}
		if err := e.allowTenant(ctx, instance.TenantID); err != nil {
			return nil, err
		}
		if instance.Ended || instance.Suspended {
			return nil, api.NewError("ENGINE-0675", "已结束或挂起的实例不能插入节点", api.ErrConflict)
		}
		source, ok := e.findDefinition(instance.ProcessDefinitionID)
		if !ok {
			return nil, api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
		}
		if req.ProcessDefinitionID != "" && req.ProcessDefinitionID != source.Info.ID {
			return nil, api.NewError("ENGINE-0676", "预期定义与实例当前定义不一致", api.ErrConflict)
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Assignee) == "" || strings.TrimSpace(req.AfterActivityID) == "" {
			return nil, api.NewError("ENGINE-0677", "节点、名称和办理人不能为空", api.ErrBadRequest)
		}
		if req.ID == "" {
			req.ID = "UserTaskAdd" + strings.ReplaceAll(e.newID(), "-", "")
		}
		data, err := insertAfterXML(source.XML, source.Info.Key, req, "seq_"+e.newID())
		if err != nil {
			return nil, err
		}
		parsed, err := bpmnxml.Parse(data)
		if err != nil {
			return nil, err
		}
		// XML与状态均为新对象，避免命令失败时污染共享的源定义模型。
		info := source.Info
		info.ID = e.newID()
		info.DeploymentID = e.newID()
		info.DerivedFrom = source.Info.ID
		info.DerivedFromRoot = source.Info.DerivedFromRoot
		if info.DerivedFromRoot == "" {
			info.DerivedFromRoot = source.Info.ID
		}
		// 同一普通定义的多个实例可各自派生，按数据库唯一键范围分配派生序号。
		for _, existing := range e.state.Definitions {
			other := existing.Info
			if other.Key == info.Key && other.Version == info.Version && other.TenantID == info.TenantID {
				info.DerivedVersion = max(info.DerivedVersion, other.DerivedVersion)
			}
		}
		info.DerivedVersion++
		e.state.Definitions[info.ID] = &definitionState{Info: info, Model: parsed, XML: data}
		e.state.Deployments[info.DeploymentID] = &deploymentState{Info: api.Deployment{ID: info.DeploymentID, Name: info.Name, TenantID: info.TenantID, DeployTime: cc.now}, Resources: map[string][]byte{info.ResourceName: data}}
		e.switchInstanceDefinition(instance, info.ID)
		return info, nil
	})
	if err != nil {
		return api.ProcessDefinition{}, err
	}
	return result.(api.ProcessDefinition), nil
}

// switchInstanceDefinition 在同一命令内更新实例及关联执行、任务、作业、订阅和当前历史引用。
// @param instance 目标实例
// @param definitionID 实例专用定义ID
// @return 无
func (e *Engine) switchInstanceDefinition(instance *instanceState, definitionID string) {
	instance.ProcessDefinitionID = definitionID
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID == instance.ID {
			execution.DefinitionID = definitionID
		}
	}
	for _, task := range e.state.Tasks {
		if task.Task.ProcessInstanceID == instance.ID {
			task.Task.ProcessDefinitionID = definitionID
		}
	}
	// 历史查询按派生定义汇总同一实例，已完成与运行中的记录均切换引用。
	for _, history := range e.state.HistoricTasks {
		if history.ProcessInstanceID == instance.ID {
			history.ProcessDefinitionID = definitionID
		}
	}
	for _, activity := range e.state.HistoricActivities {
		if activity.ProcessInstanceID == instance.ID {
			activity.ProcessDefinitionID = definitionID
		}
	}
	if history := e.state.HistoricProcesses[instance.ID]; history != nil {
		history.ProcessDefinitionID = definitionID
	}
	for _, job := range e.state.Jobs {
		if job.ProcessInstanceID == instance.ID {
			job.ProcessDefinitionID = definitionID
		}
	}
	for _, job := range e.state.DeadLetters {
		if job.ProcessInstanceID == instance.ID {
			job.ProcessDefinitionID = definitionID
		}
	}
	for _, subscription := range e.state.Subscriptions {
		if subscription.ProcessInstanceID == instance.ID {
			subscription.ProcessDefinitionID = definitionID
		}
	}
}

// setXMLAttribute 设置或移除指定本地名属性，原命名空间保持不变。
// @param node XML节点
// @param local 属性本地名
// @param value 新值，空串表示删除
// @param space 新增属性时的命名空间
// @return 无
func setXMLAttribute(node *bpmnxml.Node, local, value, space string) {
	for index, attr := range node.Attrs {
		if attr.Name.Local == local {
			if value == "" {
				node.Attrs = append(node.Attrs[:index:index], node.Attrs[index+1:]...)
			} else {
				node.Attrs[index].Value = value
			}
			return
		}
	}
	if value != "" {
		node.Attrs = append(node.Attrs, xml.Attr{Name: xml.Name{Space: space, Local: local}, Value: value})
	}
}

// qualifiedXMLName 将原始前缀写入限定名，避免编码器改变BPMN文档内的前缀绑定。
// @param name Space保存原始前缀的名称
// @return 可直接写入XML的限定名
func qualifiedXMLName(name xml.Name) xml.Name {
	if name.Space == "" {
		return name
	}
	return xml.Name{Local: name.Space + ":" + name.Local}
}

// encodeXMLNode 保留元素、属性和声明的原前缀后写出通用节点树。
// @param encoder XML编码器
// @param node 当前节点
// @return 编码错误
func encodeXMLNode(encoder *xml.Encoder, node *bpmnxml.Node) error {
	start := xml.StartElement{Name: qualifiedXMLName(node.Name)}
	for _, attr := range node.Attrs {
		attr.Name = qualifiedXMLName(attr.Name)
		start.Attr = append(start.Attr, attr)
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	if node.Text != "" {
		if err := encoder.EncodeToken(xml.CharData(node.Text)); err != nil {
			return err
		}
	}
	for _, child := range node.Children {
		if err := encodeXMLNode(encoder, child); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(start.End())
}

// findXMLScope 找到包含指定活动的流程或子流程作用域。
// @param root 当前XML节点
// @param activityID 活动ID
// @return 直接包含活动的作用域与活动，未找到返回nil
func findXMLScope(root *bpmnxml.Node, activityID string) (*bpmnxml.Node, *bpmnxml.Node) {
	for _, child := range root.Children {
		if child.Attr("id") == activityID {
			return root, child
		}
	}
	for _, child := range root.Children {
		if child.Name.Local == "subProcess" || child.Name.Local == "transaction" || child.Name.Local == "adHocSubProcess" {
			if scope, activity := findXMLScope(child, activityID); scope != nil {
				return scope, activity
			}
		}
	}
	return nil, nil
}

// insertAfterXML 构造实例专用BPMN，移动原条件出线到新节点并保留扩展属性及监听器。
// @param data 原始BPMN
// @param processID 流程Key
// @param req 新节点属性
// @param flowID 原节点到新节点的无条件连线ID
// @return 新BPMN
// @return 源节点不存在、类型非法、ID冲突或编码错误
func insertAfterXML(data []byte, processID string, req api.InsertUserTaskRequest, flowID string) ([]byte, error) {
	root, err := bpmnxml.DecodeWithPrefixes(bytes.NewReader(data), int64(len(data))+1)
	if err != nil {
		return nil, err
	}
	var process *bpmnxml.Node
	for _, child := range root.Children {
		if child.Name.Local == "process" && child.Attr("id") == processID {
			process = child
			break
		}
	}
	if process == nil {
		return nil, api.NewError("ENGINE-0404", "源流程不存在", api.ErrNotFound)
	}
	scope, source := findXMLScope(process, req.AfterActivityID)
	if source == nil {
		return nil, api.NewError("ENGINE-0404", "源节点不存在", api.ErrNotFound)
	}
	if source.Name.Local != "userTask" {
		return nil, api.NewError("ENGINE-0678", "只能在用户任务之后插入", api.ErrBadRequest)
	}
	// 新ID必须在整个XML文档内唯一，不能与其他流程或图形元素冲突。
	var conflicts func(*bpmnxml.Node) bool
	conflicts = func(node *bpmnxml.Node) bool {
		if id := node.Attr("id"); id == req.ID || id == flowID {
			return true
		}
		for _, child := range node.Children {
			if conflicts(child) {
				return true
			}
		}
		return false
	}
	if conflicts(root) {
		return nil, api.NewError("ENGINE-0679", "新节点或连线ID已存在", api.ErrConflict)
	}
	if req.ID == flowID || strings.ContainsAny(req.ID, " \t\r\n") {
		return nil, api.NewError("ENGINE-0680", "新节点ID无效", api.ErrBadRequest)
	}
	// source来自独立解码树；clone仅修改属性和子节点切片，不会修改共享子树。
	clone := *source
	clone.Attrs = append([]xml.Attr(nil), source.Attrs...)
	clone.Children = nil
	for _, child := range source.Children {
		if child.Name.Local != "incoming" {
			clone.Children = append(clone.Children, child)
		}
	}
	setXMLAttribute(&clone, "id", req.ID, "")
	setXMLAttribute(&clone, "name", req.Name, "")
	if clone.Attr("assignee") == "" {
		// 未声明办理人时用新节点局部命名空间，避免依赖源文件是否声明flowable前缀。
		setXMLAttribute(&clone, "flowtideInsert", "http://flowable.org/bpmn", "xmlns")
	}
	setXMLAttribute(&clone, "assignee", req.Assignee, "flowtideInsert")
	setXMLAttribute(source, "default", "", "")
	oldChildren := source.Children
	source.Children = nil
	for _, child := range oldChildren {
		if child.Name.Local != "outgoing" {
			source.Children = append(source.Children, child)
		}
	}
	source.Children = append(source.Children, &bpmnxml.Node{Name: xml.Name{Space: source.Name.Space, Local: "outgoing"}, Text: flowID})
	clone.Children = append(clone.Children, &bpmnxml.Node{Name: xml.Name{Space: source.Name.Space, Local: "incoming"}, Text: flowID})
	for _, child := range scope.Children {
		if child.Name.Local == "sequenceFlow" && child.Attr("sourceRef") == req.AfterActivityID {
			setXMLAttribute(child, "sourceRef", req.ID, "")
		}
	}
	flow := &bpmnxml.Node{Name: xml.Name{Space: source.Name.Space, Local: "sequenceFlow"}, Attrs: []xml.Attr{{Name: xml.Name{Local: "id"}, Value: flowID}, {Name: xml.Name{Local: "sourceRef"}, Value: req.AfterActivityID}, {Name: xml.Name{Local: "targetRef"}, Value: req.ID}}}
	scope.Children = append(scope.Children, &clone, flow)
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	if err := encodeXMLNode(encoder, root); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
