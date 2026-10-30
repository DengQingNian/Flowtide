package migration

import (
	"bytes"
	"strings"

	bpmnxml "github.com/DengQingNian/Flowtide/internal/bpmn/xml"
	"github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// checkDefinition 检查定义部署、资源及 BPMN 中需要 Go 映射的 Java 扩展。
// @param row 流程定义行
// @return 无
func (s *analysisState) checkDefinition(row flowable.Row) {
	deployment := analysisString(row, "DEPLOYMENT_ID_")
	owner := s.index["ACT_RE_DEPLOYMENT"][deployment]
	if owner.Values == nil || s.tenant(owner) != s.tenant(row) {
		s.add(row, "DEFINITION_DEPLOYMENT", "流程定义部署缺失或租户归属不一致。")
		return
	}
	resource := analysisString(row, "RESOURCE_NAME_")
	// 资源按部署及名称联合定位，不能用其他部署同名资源替代。
	resources := s.resources[deployment+"\x00"+resource]
	if len(resources) != 1 {
		s.add(row, "DEFINITION_RESOURCE", "流程定义引用的 BPMN 资源缺失或名称重复。")
		return
	}
	content, _ := resources[0].Values["BYTES_"].([]byte)
	if len(content) == 0 {
		s.add(row, "DEFINITION_RESOURCE", "流程定义引用的 BPMN 资源缺失或为空。")
		return
	}
	if len(content) > bpmnxml.MaxBPMNSize {
		s.add(row, "BPMN_INVALID", "BPMN 资源超过大小限制。")
		return
	}
	root, err := bpmnxml.Decode(bytes.NewReader(content), bpmnxml.MaxBPMNSize)
	if err != nil {
		s.add(row, "BPMN_INVALID", "BPMN 资源无法安全解码。")
		return
	}
	s.checkBPMNNode(row, root)
	parsed, err := bpmnxml.Parse(content)
	if err != nil {
		s.add(row, "BPMN_INVALID", "BPMN 资源无法由 Go 模型解析器解析。")
		return
	}
	for _, process := range parsed.Processes {
		if process.ID == analysisString(row, "KEY_") {
			s.processes[analysisID(row)] = process
			return
		}
	}
	s.add(row, "DEFINITION_PROCESS", "流程定义键未匹配资源中的流程标识。")
}

// checkBPMNNode 遍历资源节点检查 Java 委托、监听器、脚本和复杂结构。
// @param row 所属流程定义行
// @param node 当前 XML 节点
// @return 无
func (s *analysisState) checkBPMNNode(row flowable.Row, node *bpmnxml.Node) {
	local := node.Name.Local
	mapped := local == "taskListener" || local == "executionListener" || local == "serviceTask"
	// 只检查属性存在性，报告不得输出类名、表达式及脚本正文。
	for _, attr := range node.Attrs {
		if (attr.Name.Local == "class" || attr.Name.Local == "delegateExpression") && strings.TrimSpace(attr.Value) != "" {
			mapped = true
		}
	}
	if mapped {
		s.add(row, "BPMN_MAPPING", "BPMN 包含 Java 类、委托表达式或监听器，必须提供已验证的 Go 映射。")
	}
	if local == "scriptTask" {
		s.add(row, "BPMN_SCRIPT", "BPMN 脚本任务必须验证语言子集并显式转换，禁止直接执行 Java 脚本。")
	}
	switch local {
	case "multiInstanceLoopCharacteristics", "subProcess", "transaction", "callActivity", "adHocSubProcess", "boundaryEvent", "eventBasedGateway", "inclusiveGateway", "parallelGateway":
		s.add(row, "BPMN_COMPLEX", "BPMN 包含尚未完成 Java 存量作用域及路径转换验证的复杂结构。")
	}
	for _, child := range node.Children {
		s.checkBPMNNode(row, child)
	}
}
