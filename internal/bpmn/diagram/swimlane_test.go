package diagram

import (
	"bytes"
	"encoding/xml"
	"image/png"
	"strings"
	"testing"

	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
)

const swimlaneXML = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI"
             xmlns:omgdc="http://www.omg.org/spec/DD/20100524/DC"
             xmlns:omgdi="http://www.omg.org/spec/DD/20100524/DI">
  <message id="m" name="Message"/>
  <signal id="s" name="Signal"/>
  <process id="p" name="Order Process" isExecutable="true">
    <laneSet id="ls">
      <lane id="lane1" name="Sales"><flowNodeRef>start</flowNodeRef></lane>
      <lane id="lane2" name="Ops">
        <childLaneSet id="cls"><lane id="lane3" name="Ops Sub"/></childLaneSet>
      </lane>
    </laneSet>
    <startEvent id="start" name="Start">
      <timerEventDefinition><timeDuration>PT1H</timeDuration></timerEventDefinition>
    </startEvent>
    <sequenceFlow id="f1" sourceRef="start" targetRef="user"/>
    <userTask id="user" name="Approve"/>
    <boundaryEvent id="boundary" name="Timeout" attachedToRef="user">
      <timerEventDefinition><timeDuration>PT5M</timeDuration></timerEventDefinition>
    </boundaryEvent>
    <sequenceFlow id="f2" sourceRef="user" targetRef="service"/>
    <serviceTask id="service" name="Charge" delegateExpression="chargeDelegate"/>
    <sequenceFlow id="f3" sourceRef="service" targetRef="script"/>
    <scriptTask id="script" name="Log" scriptFormat="groovy"><script>1</script></scriptTask>
    <sequenceFlow id="f4" sourceRef="script" targetRef="send"/>
    <sendTask id="send" name="Send"/>
    <sequenceFlow id="f5" sourceRef="send" targetRef="receive"/>
    <receiveTask id="receive" name="Receive"/>
    <sequenceFlow id="f6" sourceRef="receive" targetRef="message"/>
    <intermediateCatchEvent id="message" name="Wait Message">
      <messageEventDefinition messageRef="m"/>
    </intermediateCatchEvent>
    <sequenceFlow id="f7" sourceRef="message" targetRef="signal"/>
    <intermediateCatchEvent id="signal" name="Wait Signal">
      <signalEventDefinition signalRef="s"/>
    </intermediateCatchEvent>
    <sequenceFlow id="f8" sourceRef="signal" targetRef="end"/>
    <endEvent id="end" name="End"/>
  </process>
  <collaboration id="c">
    <participant id="pool" name="Order Pool" processRef="p">
      <laneSet id="pls">
        <lane id="participantLane" name="Participant Lane"/>
        <lane id="noNameLane"/>
      </laneSet>
    </participant>
    <participant id="pool2" processRef="p"/>
  </collaboration>
  <bpmndi:BPMNDiagram id="d1">
    <bpmndi:BPMNPlane bpmnElement="c">
      <bpmndi:BPMNShape bpmnElement="pool" isHorizontal="true" id="poolShape">
        <omgdc:Bounds x="160" y="80" width="620" height="320"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="pool2" id="pool2Shape">
        <omgdc:Bounds x="840" y="80" width="120" height="320"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="lane1" isHorizontal="true" id="lane1Shape">
        <omgdc:Bounds x="190" y="80" width="590" height="80"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="lane2" id="lane2Shape">
        <omgdc:Bounds x="190" y="160" width="590" height="80"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="lane3" isHorizontal="true" id="lane3Shape">
        <omgdc:Bounds x="190" y="240" width="590" height="80"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="participantLane" isHorizontal="true" id="participantLaneShape">
        <omgdc:Bounds x="870" y="80" width="90" height="160"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="noNameLane" id="noNameLaneShape">
        <omgdc:Bounds x="870" y="240" width="90" height="160"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="start" id="s1">
        <omgdc:Bounds x="220" y="100" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="boundary" id="s2">
        <omgdc:Bounds x="320" y="90" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="user" id="s3">
        <omgdc:Bounds x="260" y="180" width="120" height="60"/>
        <bpmndi:BPMNLabel><omgdc:Bounds x="280" y="200" width="80" height="20"/></bpmndi:BPMNLabel>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="service" id="s4">
        <omgdc:Bounds x="260" y="260" width="120" height="60"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="script" id="s5">
        <omgdc:Bounds x="420" y="180" width="120" height="60"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="send" id="s6">
        <omgdc:Bounds x="420" y="260" width="120" height="60"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="receive" id="s7">
        <omgdc:Bounds x="560" y="180" width="120" height="60"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="message" id="s8">
        <omgdc:Bounds x="560" y="260" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="signal" id="s9">
        <omgdc:Bounds x="640" y="260" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNShape bpmnElement="end" id="s10">
        <omgdc:Bounds x="700" y="180" width="36" height="36"/>
      </bpmndi:BPMNShape>
      <bpmndi:BPMNEdge bpmnElement="f1" id="e1">
        <omgdi:waypoint x="256" y="118"/><omgdi:waypoint x="260" y="200"/>
      </bpmndi:BPMNEdge>
      <bpmndi:BPMNEdge bpmnElement="f2" id="e2">
        <omgdi:waypoint x="380" y="210"/><omgdi:waypoint x="260" y="280"/>
      </bpmndi:BPMNEdge>
    </bpmndi:BPMNPlane>
  </bpmndi:BPMNDiagram>
</definitions>`

// parseSwimlaneFixture 解析带泳道的测试流程图。
// @param t 测试上下文
// @return 流程图
func parseSwimlaneFixture(t *testing.T) *Diagram {
	t.Helper()
	bpmn, err := bpmnxml.Parse([]byte(swimlaneXML))
	if err != nil {
		t.Fatalf("解析 BPMN 失败: %v", err)
	}
	parsed, err := Parse(bpmn, []byte(swimlaneXML))
	if err != nil {
		t.Fatalf("解析图形失败: %v", err)
	}
	return parsed
}

// TestParseSwimlanes 验证泳池与泳道解析。
// @param t 测试上下文
func TestParseSwimlanes(t *testing.T) {
	parsed := parseSwimlaneFixture(t)
	if len(parsed.Pools) != 2 {
		t.Fatalf("泳池数量错误: %d", len(parsed.Pools))
	}
	if len(parsed.Lanes) != 5 {
		t.Fatalf("泳道数量错误: %d", len(parsed.Lanes))
	}
	pool := parsed.Pools[0]
	if pool.ElementID != "pool" || pool.Name != "Order Pool" || !pool.IsHorizontal || pool.ProcessRef != "p" {
		t.Fatalf("泳池解析错误: %+v", pool)
	}
	pool2 := parsed.Pools[1]
	// 没有名称时回退到元素 ID，没有 isHorizontal 时按宽高推断
	if pool2.Name != "pool2" || pool2.IsHorizontal || pool2.ProcessRef != "p" {
		t.Fatalf("无名称泳池解析错误: %+v", pool2)
	}
	lanes := make(map[string]Lane, len(parsed.Lanes))
	for _, lane := range parsed.Lanes {
		lanes[lane.ElementID] = lane
	}
	if lane := lanes["lane1"]; lane.Name != "Sales" || !lane.IsHorizontal || lane.PoolID != "pool" ||
		len(lane.FlowNodeRefs) != 1 || lane.FlowNodeRefs[0] != "start" {
		t.Fatalf("泳道 lane1 解析错误: %+v", lane)
	}
	if lane := lanes["lane2"]; lane.Name != "Ops" || !lane.IsHorizontal || lane.PoolID != "pool" {
		t.Fatalf("泳道 lane2 解析错误: %+v", lane)
	}
	// 嵌套泳道仍应归属到泳池
	if lane := lanes["lane3"]; lane.Name != "Ops Sub" || lane.PoolID != "pool" {
		t.Fatalf("嵌套泳道解析错误: %+v", lane)
	}
	if lane := lanes["participantLane"]; lane.Name != "Participant Lane" || !lane.IsHorizontal || lane.PoolID != "pool" {
		t.Fatalf("泳池内泳道解析错误: %+v", lane)
	}
	if lane := lanes["noNameLane"]; lane.Name != "noNameLane" || lane.IsHorizontal || lane.PoolID != "pool" {
		t.Fatalf("无名称泳道解析错误: %+v", lane)
	}
	for _, shape := range parsed.Shapes {
		if shape.ElementID == "pool" || shape.ElementID == "lane1" {
			t.Fatalf("泳池/泳道不应出现在节点列表: %+v", shape)
		}
	}
	if len(parsed.Shapes) != 10 {
		t.Fatalf("节点数量错误: %d", len(parsed.Shapes))
	}
	if parsed.Width < 960 || parsed.Height < 400 {
		t.Fatalf("画布尺寸未包含泳池: %d %d", parsed.Width, parsed.Height)
	}
}

// TestParseMarkers 验证内部标记解析。
// @param t 测试上下文
func TestParseMarkers(t *testing.T) {
	parsed := parseSwimlaneFixture(t)
	expected := map[string]string{
		"start":    markerTimer,
		"boundary": markerTimer,
		"user":     markerUser,
		"service":  markerService,
		"script":   markerScript,
		"send":     markerSend,
		"receive":  markerReceive,
		"message":  markerMessage,
		"signal":   markerSignal,
		"end":      "",
	}
	for _, shape := range parsed.Shapes {
		if shape.Marker != expected[shape.ElementID] {
			t.Fatalf("标记类型错误: %s=%q 期望 %q", shape.ElementID, shape.Marker, expected[shape.ElementID])
		}
	}
}

// TestRenderSwimlaneSVG 验证 SVG 泳道、名称与标记渲染。
// @param t 测试上下文
func TestRenderSwimlaneSVG(t *testing.T) {
	parsed := parseSwimlaneFixture(t)
	content := string(RenderSVG(parsed, map[string]bool{"user": true}))
	for _, want := range []string{
		`class="pool"`, `class="pool-name"`, `class="pool-separator"`,
		`class="lane"`, `class="lane-name"`, `class="lane-separator"`,
		"Order Pool", "Sales", "Ops Sub", "Particip",
		`marker-user`, `marker-service`, `marker-script`,
		`marker-send`, `marker-receive`, `marker-timer`,
		`marker-message`, `marker-signal`, "#dbeafe",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("SVG 缺少内容 %q", want)
		}
	}
	poolIndex := strings.Index(content, `class="pool"`)
	laneIndex := strings.Index(content, `class="lane"`)
	edgeIndex := strings.Index(content, "<polyline")
	if !(poolIndex < laneIndex && laneIndex < edgeIndex) {
		t.Fatalf("渲染顺序错误: %d %d %d", poolIndex, laneIndex, edgeIndex)
	}
}

// TestRenderSwimlanePNG 验证 PNG 泳道渲染。
// @param t 测试上下文
func TestRenderSwimlanePNG(t *testing.T) {
	parsed := parseSwimlaneFixture(t)
	content, err := RenderPNG(parsed, nil)
	if err != nil {
		t.Fatalf("渲染 PNG 失败: %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("解码 PNG 失败: %v", err)
	}
	if decoded.Bounds().Dx() != parsed.Width || decoded.Bounds().Dy() != parsed.Height {
		t.Fatalf("PNG 尺寸错误: %v", decoded.Bounds())
	}
	// 取消泳道后图像应不同，证明泳道确实被绘制
	withoutLanes := &Diagram{Width: parsed.Width, Height: parsed.Height, Shapes: parsed.Shapes, Edges: parsed.Edges}
	plain, err := RenderPNG(withoutLanes, nil)
	if err != nil || bytes.Equal(plain, content) {
		t.Fatalf("泳道未参与渲染: %v", err)
	}
}

// TestRenderContainerEdges 验证无声明的池、泳道与过小标记。
// @param t 测试上下文
func TestRenderContainerEdges(t *testing.T) {
	parsed := &Diagram{
		Width:  200,
		Height: 160,
		Pools:  []Pool{{ElementID: "p", Bounds: Bounds{X: 10, Y: 10, W: 180, H: 140}}},
		Lanes:  []Lane{{ElementID: "l", Bounds: Bounds{X: 20, Y: 20, W: 160, H: 120}}},
		Shapes: []Shape{{
			ElementID: "tiny", Type: "userTask", Marker: markerUser,
			Bounds: Bounds{X: 40, Y: 40, W: 2, H: 2},
		}},
	}
	svg := string(RenderSVG(parsed, nil))
	if strings.Contains(svg, `class="pool-name"`) || strings.Contains(svg, `class="lane-name"`) {
		t.Fatalf("空名称不应输出文本: %s", svg)
	}
	if !strings.Contains(svg, `class="pool"`) || !strings.Contains(svg, `class="lane"`) {
		t.Fatalf("泳池/泳道矩形缺失: %s", svg)
	}
	if _, err := RenderPNG(parsed, nil); err != nil {
		t.Fatalf("渲染 PNG 失败: %v", err)
	}
	if _, err := Parse(nil, []byte(swimlaneXML)); err != nil {
		t.Fatalf("模型为空时仍应解析图形: %v", err)
	}
}

// TestParseContainerHelpers 验证容器解析辅助函数。
// @param t 测试上下文
func TestParseContainerHelpers(t *testing.T) {
	shapeNode := decodeNode(t, `<BPMNShape bpmnElement="x"/>`)
	if _, ok := parsePool(shapeNode, nil, nil); ok {
		t.Fatalf("缺少元信息不应解析为泳池")
	}
	if _, ok := parsePool(shapeNode, &containerMeta{kind: containerLane}, nil); ok {
		t.Fatalf("泳道元信息不应解析为泳池")
	}
	if _, ok := parseLane(shapeNode, nil, nil); ok {
		t.Fatalf("缺少元信息不应解析为泳道")
	}
	if _, ok := parseLane(shapeNode, &containerMeta{kind: containerParticipant}, nil); ok {
		t.Fatalf("泳池元信息不应解析为泳道")
	}
	if _, ok := parsePool(shapeNode, &containerMeta{kind: containerParticipant}, nil); ok {
		t.Fatalf("缺少 Bounds 不应解析为泳池")
	}
	if _, ok := parseLane(shapeNode, &containerMeta{kind: containerLane}, nil); ok {
		t.Fatalf("缺少 Bounds 不应解析为泳道")
	}

	bounds := Bounds{W: 100, H: 50}
	meta := &containerMeta{kind: containerParticipant, name: "Meta Pool"}
	poolShape := decodeNode(t, `<BPMNShape bpmnElement="x" isHorizontal="false"><Bounds x="1" y="2" width="10" height="20"/></BPMNShape>`)
	pool, ok := parsePool(poolShape, meta, nil)
	if !ok || pool.Name != "Meta Pool" || pool.IsHorizontal {
		t.Fatalf("泳池解析错误: %+v", pool)
	}
	lane, ok := parseLane(poolShape, &containerMeta{kind: containerLane, name: "Meta Lane", flowNodeRefs: []string{"a"}}, nil)
	if !ok || lane.Name != "Meta Lane" || len(lane.FlowNodeRefs) != 1 {
		t.Fatalf("泳道解析错误: %+v", lane)
	}
	if parseHorizontal(poolShape, bounds) {
		t.Fatalf("isHorizontal=false 解析错误")
	}
	if name := resolveContainerName(nil, nil, "fallback"); name != "fallback" {
		t.Fatalf("名称回退错误: %s", name)
	}
	if name := resolveContainerName(nil, meta, "x"); name != "Meta Pool" {
		t.Fatalf("名称解析错误: %s", name)
	}
}

// TestParseHorizontal 验证 isHorizontal 各分支。
// @param t 测试上下文
func TestParseHorizontal(t *testing.T) {
	cases := []struct {
		attr string
		w, h float64
		want bool
	}{
		{"false", 100, 20, false},
		{"TRUE", 20, 100, true},
		{"1", 20, 100, true},
		{"", 100, 20, true},
		{"", 20, 100, false},
	}
	for _, item := range cases {
		attrs := ""
		if item.attr != "" {
			attrs = ` isHorizontal="` + item.attr + `"`
		}
		node := decodeNode(t, "<BPMNShape"+attrs+"/>")
		if got := parseHorizontal(node, Bounds{W: item.w, H: item.h}); got != item.want {
			t.Fatalf("isHorizontal=%q 宽高=%v/%v 期望 %v 实际 %v", item.attr, item.w, item.h, item.want, got)
		}
	}
}

// TestCollectContainersEdges 验证泳池/泳道元信息收集的边界情况。
// @param t 测试上下文
func TestCollectContainersEdges(t *testing.T) {
	root := decodeNode(t, `<definitions>
  <process>
    <laneSet>
      <lane name="无 ID"/>
      <lane id="a" name="A"><flowNodeRef> </flowNodeRef></lane>
    </laneSet>
  </process>
  <collaboration>
    <participant name="无 ID"/>
    <participant id="orphan" processRef="missing"/>
  </collaboration>
</definitions>`)
	metas := collectContainers(root)
	if _, ok := metas["a"]; !ok {
		t.Fatalf("泳道 a 未收集")
	}
	if len(metas["a"].flowNodeRefs) != 0 {
		t.Fatalf("空白节点引用不应收集")
	}
	if metas["a"].parentID != "" {
		t.Fatalf("无流程 ID 时不应关联父容器: %q", metas["a"].parentID)
	}
	if _, ok := metas["orphan"]; !ok {
		t.Fatalf("泳池 orphan 未收集")
	}

	// 直接构造元信息覆盖断链、环与无父容器的情况
	manual := map[string]*containerMeta{
		"disconnected": {kind: containerLane, parentID: ""},
		"missing":      {kind: containerLane, parentID: "ghost"},
		"a":            {kind: containerLane, parentID: "b"},
		"b":            {kind: containerLane, parentID: "a"},
	}
	linkLanesToPool(manual)
	if manual["missing"].parentID != "" {
		t.Fatalf("找不到父泳池时应清空关联: %q", manual["missing"].parentID)
	}
	if matchPoolByProcess(manual, "none") != "" {
		t.Fatalf("无匹配泳池时应返回空")
	}
}

// TestMarkerHelpers 验证标记辅助函数。
// @param t 测试上下文
func TestMarkerHelpers(t *testing.T) {
	if findElement(nil, "x") != nil {
		t.Fatalf("空模型不应找到元素")
	}
	if markerOf(nil) != "" {
		t.Fatalf("空元素不应有标记")
	}
	if eventMarker(nil, "", "") != "" {
		t.Fatalf("无事件定义不应有标记")
	}
	box := markerBox(Shape{Type: "userTask", Bounds: Bounds{X: 10, Y: 20, W: 100, H: 60}})
	if box.W <= 0 || box.X != 16 || box.Y != 26 {
		t.Fatalf("任务标记区域错误: %+v", box)
	}
	eventBox := markerBox(Shape{Type: "startEvent", Bounds: Bounds{X: 0, Y: 0, W: 40, H: 40}})
	if eventBox.X != 11 || eventBox.W != 18 {
		t.Fatalf("事件标记区域错误: %+v", eventBox)
	}
	if empty := markerBox(Shape{Bounds: Bounds{}}); empty != (Bounds{}) {
		t.Fatalf("零尺寸不应有标记区域: %+v", empty)
	}
	// 缺少 Bounds 的节点应被忽略
	node := decodeNode(t, `<BPMNShape bpmnElement="x"/>`)
	if shape := parseShape(node, nil); shape != nil {
		t.Fatalf("缺少 Bounds 不应生成图形")
	}
	// 缺少 bpmnElement 的节点应被忽略
	emptyNode := decodeNode(t, `<BPMNShape><Bounds x="0" y="0" width="1" height="1"/></BPMNShape>`)
	if shape := parseShape(emptyNode, nil); shape != nil {
		t.Fatalf("缺少 bpmnElement 不应生成图形")
	}
	edgeNode := decodeNode(t, `<BPMNEdge bpmnElement="f"><waypoint x="0" y="0"/></BPMNEdge>`)
	if edge := parseEdge(edgeNode, nil); edge != nil {
		t.Fatalf("不足两个折点不应生成连线")
	}
	if edge := parseEdge(decodeNode(t, `<BPMNEdge/>`), nil); edge != nil {
		t.Fatalf("缺少 bpmnElement 不应生成连线")
	}
}

// TestFitText 验证文本截断。
// @param t 测试上下文
func TestFitText(t *testing.T) {
	if got := fitText("", 100, 11); got != "" {
		t.Fatalf("空文本错误: %q", got)
	}
	if got := fitText("abc", 0, 11); got != "" {
		t.Fatalf("零宽度错误: %q", got)
	}
	if got := fitText("abc", 100, 0); got != "" {
		t.Fatalf("零字号错误: %q", got)
	}
	if got := fitText("abc", 100, 11); got != "abc" {
		t.Fatalf("短文本不应截断: %q", got)
	}
	if got := fitText("abcdefghij", 40, 11); got != "ab..." {
		t.Fatalf("截断结果错误: %q", got)
	}
	if got := fitText("abcdefghij", 10, 11); got != "..." {
		t.Fatalf("极窄宽度应只保留省略号: %q", got)
	}
	if got := fitChars("", 5); got != "" {
		t.Fatalf("空文本错误: %q", got)
	}
	if got := fitChars("abc", 0); got != "" {
		t.Fatalf("零字符数错误: %q", got)
	}
	if got := fitChars("abc", 5); got != "abc" {
		t.Fatalf("短文本不应截断: %q", got)
	}
	if got := fitChars("abc", 2); got != "ab" {
		t.Fatalf("少量字符截断错误: %q", got)
	}
	if got := fitChars("abcdef", 5); got != "ab..." {
		t.Fatalf("字符截断结果错误: %q", got)
	}
}

// decodeNode 解码测试用单节点 XML。
// @param t 测试上下文
// @param data XML 内容
// @return 根节点
func decodeNode(t *testing.T, data string) *bpmnxml.Node {
	t.Helper()
	root, err := bpmnxml.Decode(strings.NewReader(data), 1<<20)
	if err != nil {
		t.Fatalf("解码 XML 失败: %v", err)
	}
	return root
}

// TestNodeAttrs 验证测试辅助函数与 XML 属性兼容。
// @param t 测试上下文
func TestNodeAttrs(t *testing.T) {
	node := &bpmnxml.Node{Attrs: []xml.Attr{{Name: xml.Name{Local: "isHorizontal"}, Value: "false"}}}
	if parseHorizontal(node, Bounds{W: 100}) {
		t.Fatalf("显式 false 解析错误")
	}
}
