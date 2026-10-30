package diagram

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// TestEventIconMarkers 验证新增事件图标的判断逻辑与优先级。
// @param t 测试上下文
func TestEventIconMarkers(t *testing.T) {
	cases := []struct {
		name    string
		element model.FlowElement
		want    string
	}{
		{"终止结束事件", &model.EndEvent{Terminate: true}, markerTerminate},
		{"取消结束事件", &model.EndEvent{Cancel: true}, markerCancel},
		{"错误结束事件", &model.EndEvent{ErrorRef: "error-1"}, markerError},
		{"普通结束事件", &model.EndEvent{}, ""},
		{"补偿边界事件", &model.BoundaryEvent{Compensate: true}, markerCompensate},
		{"取消边界事件", &model.BoundaryEvent{Cancel: true}, markerCancel},
		{"错误边界事件", &model.BoundaryEvent{ErrorRef: "error-1"}, markerError},
		{"定时边界事件保持不变", &model.BoundaryEvent{Timer: &model.TimerDefinition{}}, markerTimer},
		{"消息边界事件保持不变", &model.BoundaryEvent{MessageRef: "message-1"}, markerMessage},
		{"信号边界事件保持不变", &model.BoundaryEvent{SignalRef: "signal-1"}, markerSignal},
		{"补偿抛出事件", &model.IntermediateThrowEvent{Compensate: true}, markerCompensate},
		{"链接抛出事件", &model.IntermediateThrowEvent{LinkName: "to-end"}, markerLink},
		{"普通抛出事件", &model.IntermediateThrowEvent{}, ""},
		{"链接捕获事件", &model.IntermediateCatchEvent{LinkName: "to-start"}, markerLink},
		{"定时捕获事件保持不变", &model.IntermediateCatchEvent{Timer: &model.TimerDefinition{}}, markerTimer},
		{"消息捕获事件保持不变", &model.IntermediateCatchEvent{MessageRef: "message-1"}, markerMessage},
		{"信号捕获事件保持不变", &model.IntermediateCatchEvent{SignalRef: "signal-1"}, markerSignal},
		{"普通捕获事件", &model.IntermediateCatchEvent{}, ""},
		{"开始事件保持不变", &model.StartEvent{Timer: &model.TimerDefinition{}}, markerTimer},
	}
	for _, item := range cases {
		if got := markerOf(item.element); got != item.want {
			t.Fatalf("%s 标记错误: %q 期望 %q", item.name, got, item.want)
		}
	}
	// 多定义同时存在时按文档约定的优先级选择图标
	priorities := []struct {
		name    string
		element model.FlowElement
		want    string
	}{
		{"结束事件终止优先", &model.EndEvent{Terminate: true, Cancel: true, ErrorRef: "error-1"}, markerTerminate},
		{"结束事件错误优先于取消", &model.EndEvent{Cancel: true, ErrorRef: "error-1"}, markerError},
		{"边界事件错误优先", &model.BoundaryEvent{ErrorRef: "error-1", Cancel: true, Compensate: true}, markerError},
		{"边界事件取消优先于补偿", &model.BoundaryEvent{Cancel: true, Compensate: true}, markerCancel},
		{"边界事件原有定义优先", &model.BoundaryEvent{Timer: &model.TimerDefinition{}, Compensate: true}, markerTimer},
		{"抛出事件补偿优先于链接", &model.IntermediateThrowEvent{Compensate: true, LinkName: "to-end"}, markerCompensate},
		{"捕获事件原有定义优先于链接", &model.IntermediateCatchEvent{MessageRef: "message-1", LinkName: "to-start"}, markerMessage},
	}
	for _, item := range priorities {
		if got := markerOf(item.element); got != item.want {
			t.Fatalf("%s 标记错误: %q 期望 %q", item.name, got, item.want)
		}
	}
	if got := eventExtraMarker("", "", false, false); got != "" {
		t.Fatalf("无扩展定义不应有标记: %q", got)
	}
}

// eventIconDiagram 构造包含全部新增图标的测试流程图。
// @return 流程图
func eventIconDiagram() *Diagram {
	markers := []string{markerTerminate, markerCancel, markerError, markerCompensate, markerLink}
	shapes := make([]Shape, 0, len(markers))
	for index, marker := range markers {
		shapes = append(shapes, Shape{
			ElementID: marker,
			Type:      "endEvent",
			Name:      marker,
			Marker:    marker,
			Bounds:    Bounds{X: 20 + float64(index)*60, Y: 20, W: 36, H: 36},
		})
	}
	return &Diagram{Width: 320, Height: 100, Shapes: shapes}
}

// markerFragment 返回 SVG 中指定标记分组的内容。
// @param content SVG 内容
// @param marker 标记类型
// @return 分组片段，未找到时为空
func markerFragment(content, marker string) string {
	start := strings.Index(content, `<g class="marker marker-`+marker+`"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(content[start:], "</g>")
	if end < 0 {
		return ""
	}
	return content[start : start+end]
}

// TestEventIconSVG 验证新增图标的 SVG 图元与标签避让。
// @param t 测试上下文
func TestEventIconSVG(t *testing.T) {
	parsed := eventIconDiagram()
	content := string(RenderSVG(parsed, nil))
	fragments := map[string]string{}
	for _, marker := range []string{markerTerminate, markerCancel, markerError, markerCompensate, markerLink} {
		fragment := markerFragment(content, marker)
		if fragment == "" {
			t.Fatalf("SVG 缺少标记 %q: %s", marker, content)
		}
		fragments[marker] = fragment
	}
	// 终止事件为实心圆，取消事件为加粗叉号
	if !strings.Contains(fragments[markerTerminate], "<circle") || !strings.Contains(fragments[markerTerminate], `fill="#111111"`) {
		t.Fatalf("终止图标错误: %s", fragments[markerTerminate])
	}
	if strings.Count(fragments[markerCancel], "M ") != 2 || !strings.Contains(fragments[markerCancel], `stroke-width="2.5"`) {
		t.Fatalf("取消图标错误: %s", fragments[markerCancel])
	}
	// 错误事件为闪电多边形，补偿事件为两个三角形
	if !strings.Contains(fragments[markerError], "<polygon") {
		t.Fatalf("错误图标错误: %s", fragments[markerError])
	}
	if strings.Count(fragments[markerCompensate], "<polygon") != 2 {
		t.Fatalf("补偿图标错误: %s", fragments[markerCompensate])
	}
	// 链接事件为箭头
	if !strings.Contains(fragments[markerLink], "L ") {
		t.Fatalf("链接图标错误: %s", fragments[markerLink])
	}
	// 有图标时标签应移到图形下方
	labelX, labelY := shapeLabelPoint(Shape{Type: "endEvent", Marker: markerError, Bounds: Bounds{X: 20, Y: 20, W: 36, H: 36}})
	if labelX != 38 || labelY != 68 {
		t.Fatalf("图标标签避让错误: %v %v", labelX, labelY)
	}
	// 高亮样式保持原有行为
	highlighted := string(RenderSVG(parsed, map[string]bool{markerLink: true}))
	if !strings.Contains(markerFragment(highlighted, markerLink), `stroke="#2563eb"`) {
		t.Fatalf("高亮未生效: %s", highlighted)
	}
}

// TestEventIconPNG 验证新增图标参与 PNG 渲染。
// @param t 测试上下文
func TestEventIconPNG(t *testing.T) {
	markers := []string{markerTerminate, markerCancel, markerError, markerCompensate, markerLink}
	for index, marker := range markers {
		shape := Shape{
			ElementID: marker,
			Type:      "endEvent",
			Marker:    marker,
			Bounds:    Bounds{X: 32, Y: 32, W: 36, H: 36},
		}
		withMarker := &Diagram{Width: 100, Height: 100, Shapes: []Shape{shape}}
		shape.Marker = ""
		withoutMarker := &Diagram{Width: 100, Height: 100, Shapes: []Shape{shape}}
		content, err := RenderPNG(withMarker, nil)
		if err != nil {
			t.Fatalf("渲染 %s PNG 失败: %v", marker, err)
		}
		plain, err := RenderPNG(withoutMarker, nil)
		if err != nil {
			t.Fatalf("渲染无标记 PNG 失败: %v", err)
		}
		if bytes.Equal(content, plain) {
			t.Fatalf("标记 %s 未参与渲染", marker)
		}
		decoded, err := png.Decode(bytes.NewReader(content))
		if err != nil {
			t.Fatalf("解码 %s PNG 失败: %v", marker, err)
		}
		if decoded.Bounds().Dx() != withMarker.Width || decoded.Bounds().Dy() != withMarker.Height {
			t.Fatalf("PNG 尺寸错误: %v", decoded.Bounds())
		}
		// 终止图标是实心圆，中心像素应为深色
		if marker == markerTerminate {
			red, _, _, _ := decoded.At(50, 50).RGBA()
			if red>>8 > 128 {
				t.Fatalf("终止图标中心未填充: %d", red>>8)
			}
		}
		// 每个标记都应在中心附近产生深色线条
		if index == 0 {
			continue
		}
		dark := false
		for y := 42; y <= 58 && !dark; y++ {
			for x := 42; x <= 58; x++ {
				red, green, blue, _ := decoded.At(x, y).RGBA()
				if red>>8 < 128 && green>>8 < 128 && blue>>8 < 128 {
					dark = true
					break
				}
			}
		}
		if !dark {
			t.Fatalf("标记 %s 未在 PNG 中绘制", marker)
		}
	}
}
