// 端到端测试夹具：生成带用户任务与 BPMN DI 的最小流程。
export interface BuildBpmnOptions {
  /** 流程 Key */
  key: string
  /** 流程名称 */
  name: string
  /** 用户任务名称 */
  taskName: string
  /** 办理人 */
  assignee?: string
}

// 生成可用于部署和设计器导入的 BPMN XML。
// @param options 流程参数
// @return BPMN XML
export function buildBpmn(options: BuildBpmnOptions): string {
  const assignee = options.assignee ? ` flowable:assignee="${options.assignee}"` : ''
  return `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" xmlns:di="http://www.omg.org/spec/DD/20100524/DI" xmlns:flowable="http://flowable.org/bpmn" id="Definitions_${options.key}" targetNamespace="http://flowtide.local/bpmn">
  <bpmn:process id="${options.key}" name="${options.name}" isExecutable="true">
    <bpmn:startEvent id="start" name="开始" />
    <bpmn:sequenceFlow id="f1" sourceRef="start" targetRef="approve" />
    <bpmn:userTask id="approve" name="${options.taskName}"${assignee} />
    <bpmn:sequenceFlow id="f2" sourceRef="approve" targetRef="end" />
    <bpmn:endEvent id="end" name="结束" />
  </bpmn:process>
  <bpmndi:BPMNDiagram id="BPMNDiagram_1">
    <bpmndi:BPMNPlane id="BPMNPlane_1" bpmnElement="${options.key}">
      <bpmndi:BPMNShape id="start_di" bpmnElement="start"><dc:Bounds x="160" y="100" width="36" height="36" /></bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="approve_di" bpmnElement="approve"><dc:Bounds x="260" y="78" width="100" height="80" /></bpmndi:BPMNShape>
      <bpmndi:BPMNShape id="end_di" bpmnElement="end"><dc:Bounds x="430" y="100" width="36" height="36" /></bpmndi:BPMNShape>
      <bpmndi:BPMNEdge id="f1_di" bpmnElement="f1"><di:waypoint x="196" y="118" /><di:waypoint x="260" y="118" /></bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge id="f2_di" bpmnElement="f2"><di:waypoint x="360" y="118" /><di:waypoint x="430" y="118" /></bpmndi:BPMNEdge>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</bpmn:definitions>`
}
