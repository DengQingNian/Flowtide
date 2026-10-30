// 新模型的最小 BPMN 2.0 文档生成。

// escapeXml 转义 XML 文本。
// @param value 文本
// @return 转义后的文本
function escapeXml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

// buildMinimalBpmn 生成包含开始事件的最小流程定义。
// @param key 流程 Key
// @param name 流程名称
// @return BPMN XML
export function buildMinimalBpmn(key: string, name = ''): string {
  const processId = escapeXml(key || 'process')
  const processName = escapeXml(name || key || '新建流程')
  return `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
  xmlns:dc="http://www.omg.org/spec/DD/20100524/DC"
  xmlns:di="http://www.omg.org/spec/DD/20100524/DI"
  id="Definitions_${processId}"
  targetNamespace="http://flowtide.local/bpmn">
  <bpmn:process id="${processId}" name="${processName}" isExecutable="true">
    <bpmn:startEvent id="StartEvent_1" name="开始" />
  </bpmn:process>
  <bpmndi:BPMNDiagram id="BPMNDiagram_1">
    <bpmndi:BPMNPlane id="BPMNPlane_1" bpmnElement="${processId}">
      <bpmndi:BPMNShape id="StartEvent_1_di" bpmnElement="StartEvent_1">
        <dc:Bounds x="180" y="160" width="36" height="36" />
      </bpmndi:BPMNShape>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</bpmn:definitions>
`
}

// extractProcessId 从 BPMN XML 中提取第一个流程 ID。
// @param xml BPMN XML
// @return 流程 ID，未找到返回空串
export function extractProcessId(xml: string): string {
  const match = /<bpmn:process\b[^>]*\bid="([^"]+)"/.exec(xml)
  return match ? match[1] : ''
}

// hasStartEvent 判断 XML 是否至少包含一个开始事件。
// @param xml BPMN XML
// @return 是否包含
export function hasStartEvent(xml: string): boolean {
  return /<bpmn:startEvent\b/.test(xml)
}
