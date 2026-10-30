package diagram

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	bpmnxml "github.com/DengQingNian/Flowtide/internal/bpmn/xml"
)

const diagramXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
             xmlns:omgdc="http://www.omg.org/spec/DD/20100524/DC"
             xmlns:omgdi="http://www.omg.org/spec/DD/20100524/DI">
  <process id="p" name="Process" isExecutable="true">
    <startEvent id="start" name="Start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <userTask id="task" name="Approve Order"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="end"/>
    <endEvent id="end" name="End"/>
  </process>
  <bpmndi:BPMNDiagram id="d1">
    <bpmndi:BPMNPlane bpmnElement="p">
      <bpmndi:BPMNShape bpmnElement="start" id="s1">
        <omgdc:Bounds x="100" y="100" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="task" id="s2">
        <omgdc:Bounds x="200" y="90" width="120" height="60"/>
        <bpmndi:BPMNLabel><omgdc:Bounds x="220" y="110" width="80" height="20"/></bpmndi:BPMNLabel>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="end" id="s3">
        <omgdc:Bounds x="400" y="100" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNEdge bpmnElement="f1" id="e1">
        <omgdi:waypoint x="136" y="118"/>
        <omgdi:waypoint x="200" y="118"/>
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge bpmnElement="f2" id="e2">
        <omgdi:waypoint x="320" y="120"/>
        <omgdi:waypoint x="400" y="118"/>
      </bpmndi:BPMNEdge>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</definitions>`

// parseFixture 解析测试流程图。
// @param t 测试上下文
// @return 流程图
func parseFixture(t *testing.T) *Diagram {
	t.Helper()
	model, err := bpmnxml.Parse([]byte(diagramXML))
	if err != nil {
		t.Fatalf("解析 BPMN 失败: %v", err)
	}
	parsed, err := Parse(model, []byte(diagramXML))
	if err != nil {
		t.Fatalf("解析图形失败: %v", err)
	}
	return parsed
}

// TestParse 验证 DI 解析。
// @param t 测试上下文
func TestParse(t *testing.T) {
	parsed := parseFixture(t)
	if len(parsed.Shapes) != 3 || len(parsed.Edges) != 2 {
		t.Fatalf("图形数量错误: %d %d", len(parsed.Shapes), len(parsed.Edges))
	}
	if parsed.Width <= 400 || parsed.Height <= 100 {
		t.Fatalf("画布尺寸错误: %d %d", parsed.Width, parsed.Height)
	}
	var task *Shape
	for index := range parsed.Shapes {
		if parsed.Shapes[index].ElementID == "task" {
			task = &parsed.Shapes[index]
		}
	}
	if task == nil || task.Name != "Approve Order" || task.Type != "userTask" || task.Label == nil {
		t.Fatalf("任务图形错误: %+v", task)
	}
	if len(parsed.Edges[0].Waypoints) != 2 || parsed.Edges[0].Type != "sequenceFlow" {
		t.Fatalf("连线解析错误: %+v", parsed.Edges[0])
	}
}

// TestParseErrors 验证图形解析错误。
// @param t 测试上下文
func TestParseErrors(t *testing.T) {
	model, _ := bpmnxml.Parse([]byte(diagramXML))
	if _, err := Parse(model, []byte("not xml")); err == nil {
		t.Fatalf("非法 XML 应报错")
	}
	noDI := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p" isExecutable="true"><startEvent id="s"/><endEvent id="e"/></process></definitions>`
	noDIModel, _ := bpmnxml.Parse([]byte(noDI))
	if _, err := Parse(noDIModel, []byte(noDI)); err == nil {
		t.Fatalf("缺少 DI 应报错")
	}
	invalidDI := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"><process id="p" isExecutable="true"><startEvent id="s"/></process><bpmndi:BPMNDiagram id="d"><bpmndi:BPMNPlane bpmnElement="p"><bpmndi:BPMNShape bpmnElement="s"><bpmndi:BPMNLabel/></bpmndi:BPMNShape><bpmndi:BPMNEdge bpmnElement="f"><bpmndi:BPMNLabel/></bpmndi:BPMNEdge></bpmndi:BPMNPlane></bpmndi:BPMNDiagram></definitions>`
	invalidModel, _ := bpmnxml.Parse([]byte(invalidDI))
	if _, err := Parse(invalidModel, []byte(invalidDI)); err == nil {
		t.Fatalf("无有效图形应报错")
	}
}

// TestRenderSVG 验证 SVG 渲染与高亮。
// @param t 测试上下文
func TestRenderSVG(t *testing.T) {
	parsed := parseFixture(t)
	content := string(RenderSVG(parsed, map[string]bool{"task": true}))
	if !strings.Contains(content, "<svg") || !strings.Contains(content, "Approve Order") ||
		!strings.Contains(content, "<polyline") || !strings.Contains(content, "#dbeafe") {
		t.Fatalf("SVG 内容错误: %s", content)
	}
}

// TestRenderPNG 验证 PNG 渲染。
// @param t 测试上下文
func TestRenderPNG(t *testing.T) {
	parsed := parseFixture(t)
	content, err := RenderPNG(parsed, nil)
	if err != nil {
		t.Fatalf("渲染 PNG 失败: %v", err)
	}
	if !bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Fatalf("PNG 头错误")
	}
	image, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("解码 PNG 失败: %v", err)
	}
	if image.Bounds().Dx() != parsed.Width || image.Bounds().Dy() != parsed.Height {
		t.Fatalf("PNG 尺寸错误: %v", image.Bounds())
	}
	highlighted, err := RenderPNG(parsed, map[string]bool{"task": true})
	if err != nil || bytes.Equal(content, highlighted) {
		t.Fatalf("高亮 PNG 应不同: %v", err)
	}
}

// TestRenderAllShapeTypes 验证各图形类型渲染。
// @param t 测试上下文
func TestRenderAllShapeTypes(t *testing.T) {
	types := []string{"startEvent", "endEvent", "intermediateThrowEvent", "boundaryEvent",
		"exclusiveGateway", "userTask", "callActivity", "subProcess", "transaction",
		"textAnnotation", "dataObjectReference", "dataStoreReference", "unknown"}
	parsed := &Diagram{Width: 400, Height: 240, Shapes: make([]Shape, 0), Edges: make([]Edge, 0)}
	for index, kind := range types {
		parsed.Shapes = append(parsed.Shapes, Shape{
			ElementID: kind, Type: kind, Name: "Task 12-a/b.c (x)",
			Bounds: Bounds{X: float64(10 + (index%4)*90), Y: float64(10 + (index/4)*70), W: 80, H: 50},
		})
	}
	parsed.Edges = append(parsed.Edges, Edge{
		ElementID: "flow", Type: "sequenceFlow", Name: "go",
		Waypoints: []Point{{X: 10, Y: 200}, {X: 100, Y: 200}},
	})
	parsed.Edges = append(parsed.Edges, Edge{
		ElementID: "assoc", Type: "association",
		Waypoints: []Point{{X: 20, Y: 210}, {X: 120, Y: 210}},
	})
	svg := string(RenderSVG(parsed, nil))
	if !strings.Contains(svg, "Task 12-a/b.c (x)") || !strings.Contains(svg, "stroke-dasharray") {
		t.Fatalf("多类型 SVG 渲染错误")
	}
	if _, err := RenderPNG(parsed, nil); err != nil {
		t.Fatalf("多类型 PNG 渲染失败: %v", err)
	}
	if _, ok := fontGlyphs['Z']; !ok {
		t.Fatalf("字体缺少字母")
	}
}
