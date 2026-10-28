package diagram

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// SVG 泳池/泳道样式常量。
const (
	svgPoolFill        = "#f8fafc" // 泳池填充色
	svgPoolBorder      = "#94a3b8" // 泳池边框色
	svgLaneFill        = "#ffffff" // 泳道填充色
	svgLaneBorder      = "#cbd5e1" // 泳道边框色
	svgLaneSeparator   = "#94a3b8" // 泳道分隔线颜色
	svgContainerText   = "#334155" // 泳池/泳道文字颜色
	svgContainerFont   = 11.0      // 泳池/泳道字号
	svgContainerBand   = 30.0      // 泳池/泳道名称带宽度
	svgContainerMargin = 12.0      // 名称与边框的间距
)

// RenderSVG 渲染流程图为 SVG。
// @param d 流程图
// @param highlights 需要高亮的活动 ID
// @return SVG 内容
func RenderSVG(d *Diagram, highlights map[string]bool) []byte {
	var builder strings.Builder
	fmt.Fprintf(&builder, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	fmt.Fprintf(&builder, "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n", d.Width, d.Height, d.Width, d.Height)
	fmt.Fprintf(&builder, "<defs><marker id=\"arrow\" viewBox=\"0 0 10 10\" refX=\"9\" refY=\"5\" markerWidth=\"8\" markerHeight=\"8\" orient=\"auto-start-reverse\"><path d=\"M 0 0 L 10 5 L 0 10 z\" fill=\"#555555\"/></marker></defs>\n")
	fmt.Fprintf(&builder, "<rect x=\"0\" y=\"0\" width=\"%d\" height=\"%d\" fill=\"#ffffff\"/>\n", d.Width, d.Height)
	// 先绘制泳池与泳道，保证节点与连线位于其上层
	for _, pool := range d.Pools {
		builder.WriteString(renderSVGPool(pool))
	}
	for _, lane := range d.Lanes {
		builder.WriteString(renderSVGLane(lane))
	}
	for _, edge := range d.Edges {
		builder.WriteString(renderSVGEdge(edge))
	}
	for _, shape := range d.Shapes {
		builder.WriteString(renderSVGShape(shape, highlights[shape.ElementID]))
	}
	builder.WriteString("</svg>\n")
	return []byte(builder.String())
}

// renderSVGPool 渲染泳池。
// @param pool 泳池
// @return SVG 片段
func renderSVGPool(pool Pool) string {
	b := pool.Bounds
	var builder strings.Builder
	fmt.Fprintf(&builder, "<rect class=\"pool\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"1.5\"/>\n",
		b.X, b.Y, b.W, b.H, svgPoolFill, svgPoolBorder)
	// 水平泳池的名称带在左侧，垂直泳池的名称带在顶部
	if pool.IsHorizontal {
		band := math.Min(svgContainerBand, b.W/3)
		fmt.Fprintf(&builder, "<line class=\"pool-separator\" x1=\"%.1f\" y1=\"%.1f\" x2=\"%.1f\" y2=\"%.1f\" stroke=\"%s\" stroke-width=\"1\"/>\n",
			b.X+band, b.Y, b.X+band, b.Y+b.H, svgPoolBorder)
		if name := fitText(pool.Name, b.H-svgContainerMargin, svgContainerFont); name != "" {
			cx, cy := b.X+band/2, b.Y+b.H/2
			fmt.Fprintf(&builder, "<text class=\"pool-name\" x=\"%.1f\" y=\"%.1f\" font-family=\"sans-serif\" font-size=\"%.1f\" fill=\"%s\" text-anchor=\"middle\" dominant-baseline=\"middle\" transform=\"rotate(-90 %.1f %.1f)\">%s</text>\n",
				cx, cy, svgContainerFont, svgContainerText, cx, cy, html.EscapeString(name))
		}
	} else {
		band := math.Min(svgContainerBand, b.H/3)
		fmt.Fprintf(&builder, "<line class=\"pool-separator\" x1=\"%.1f\" y1=\"%.1f\" x2=\"%.1f\" y2=\"%.1f\" stroke=\"%s\" stroke-width=\"1\"/>\n",
			b.X, b.Y+band, b.X+b.W, b.Y+band, svgPoolBorder)
		if name := fitText(pool.Name, b.W-svgContainerMargin, svgContainerFont); name != "" {
			fmt.Fprintf(&builder, "<text class=\"pool-name\" x=\"%.1f\" y=\"%.1f\" font-family=\"sans-serif\" font-size=\"%.1f\" fill=\"%s\" text-anchor=\"middle\" dominant-baseline=\"middle\">%s</text>\n",
				b.X+b.W/2, b.Y+band/2, svgContainerFont, svgContainerText, html.EscapeString(name))
		}
	}
	return builder.String()
}

// renderSVGLane 渲染泳道。
// @param lane 泳道
// @return SVG 片段
func renderSVGLane(lane Lane) string {
	b := lane.Bounds
	var builder strings.Builder
	fmt.Fprintf(&builder, "<rect class=\"lane\" x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"1\"/>\n",
		b.X, b.Y, b.W, b.H, svgLaneFill, svgLaneBorder)
	// 泳道分隔线沿泳道起始边绘制
	if lane.IsHorizontal {
		fmt.Fprintf(&builder, "<line class=\"lane-separator\" x1=\"%.1f\" y1=\"%.1f\" x2=\"%.1f\" y2=\"%.1f\" stroke=\"%s\" stroke-width=\"1.5\"/>\n",
			b.X, b.Y, b.X+b.W, b.Y, svgLaneSeparator)
	} else {
		fmt.Fprintf(&builder, "<line class=\"lane-separator\" x1=\"%.1f\" y1=\"%.1f\" x2=\"%.1f\" y2=\"%.1f\" stroke=\"%s\" stroke-width=\"1.5\"/>\n",
			b.X, b.Y, b.X, b.Y+b.H, svgLaneSeparator)
	}
	if name := fitText(lane.Name, b.W-svgContainerMargin, svgContainerFont); name != "" {
		if lane.IsHorizontal {
			fmt.Fprintf(&builder, "<text class=\"lane-name\" x=\"%.1f\" y=\"%.1f\" font-family=\"sans-serif\" font-size=\"%.1f\" fill=\"%s\" text-anchor=\"start\" dominant-baseline=\"middle\">%s</text>\n",
				b.X+6, b.Y+b.H/2, svgContainerFont, svgContainerText, html.EscapeString(name))
		} else {
			fmt.Fprintf(&builder, "<text class=\"lane-name\" x=\"%.1f\" y=\"%.1f\" font-family=\"sans-serif\" font-size=\"%.1f\" fill=\"%s\" text-anchor=\"middle\" dominant-baseline=\"middle\">%s</text>\n",
				b.X+b.W/2, b.Y+math.Min(svgContainerBand, b.H/3)/2, svgContainerFont, svgContainerText, html.EscapeString(name))
		}
	}
	return builder.String()
}

// renderSVGEdge 渲染连线。
// @param edge 连线
// @return SVG 片段
func renderSVGEdge(edge Edge) string {
	if len(edge.Waypoints) < 2 {
		return ""
	}
	points := make([]string, 0, len(edge.Waypoints))
	for _, point := range edge.Waypoints {
		points = append(points, fmt.Sprintf("%.1f,%.1f", point.X, point.Y))
	}
	var builder strings.Builder
	dash := ""
	if edge.Type == "association" {
		dash = " stroke-dasharray=\"5,5\""
	}
	fmt.Fprintf(&builder, "<polyline points=\"%s\" fill=\"none\" stroke=\"#555555\" stroke-width=\"1.5\" marker-end=\"url(#arrow)\"%s/>\n", strings.Join(points, " "), dash)
	if edge.Name != "" {
		center := edge.Waypoints[len(edge.Waypoints)/2]
		fmt.Fprintf(&builder, "<text x=\"%.1f\" y=\"%.1f\" font-size=\"11\" fill=\"#333333\" text-anchor=\"middle\">%s</text>\n", center.X, center.Y-6, html.EscapeString(edge.Name))
	}
	return builder.String()
}

// renderSVGShape 渲染图形节点。
// @param shape 图形
// @param highlighted 是否高亮
// @return SVG 片段
func renderSVGShape(shape Shape, highlighted bool) string {
	var builder strings.Builder
	stroke := "#111111"
	fill := "#ffffff"
	strokeWidth := 1.5
	if highlighted {
		stroke = "#2563eb"
		fill = "#dbeafe"
		strokeWidth = 2.5
	}
	b := shape.Bounds
	switch {
	case strings.Contains(shape.Type, "Event"):
		radius := b.W / 2
		if b.H/2 < radius {
			radius = b.H / 2
		}
		fmt.Fprintf(&builder, "<circle cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.1f\"/>\n", b.X+radius, b.Y+radius, radius, fill, stroke, strokeWidth)
		if shape.Type == "endEvent" || shape.Type == "intermediateThrowEvent" || shape.Type == "boundaryEvent" {
			fmt.Fprintf(&builder, "<circle cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\" fill=\"none\" stroke=\"%s\" stroke-width=\"1\"/>\n", b.X+radius, b.Y+radius, radius-3, stroke)
		}
	case strings.Contains(shape.Type, "Gateway"):
		fmt.Fprintf(&builder, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.1f\"/>\n",
			b.X+b.W/2, b.Y, b.X+b.W, b.Y+b.H/2, b.X+b.W/2, b.Y+b.H, b.X, b.Y+b.H/2, fill, stroke, strokeWidth)
	case shape.Type == "callActivity":
		fmt.Fprintf(&builder, "<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" rx=\"4\" fill=\"%s\" stroke=\"%s\" stroke-width=\"3\"/>\n", b.X, b.Y, b.W, b.H, fill, stroke)
	case shape.Type == "textAnnotation":
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f L %.1f %.1f\" fill=\"none\" stroke=\"%s\" stroke-width=\"1\"/>\n", b.X, b.Y, b.X, b.Y+b.H, b.X+b.W, b.Y+b.H, stroke)
	case shape.Type == "dataObjectReference" || shape.Type == "dataStoreReference":
		fmt.Fprintf(&builder, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f\" fill=\"%s\" stroke=\"%s\" stroke-width=\"1\"/>\n",
			b.X+8, b.Y, b.X+b.W, b.Y, b.X+b.W, b.Y+b.H, b.X, b.Y+b.H, fill, stroke)
	case shape.Type == "subProcess" || shape.Type == "transaction":
		fmt.Fprintf(&builder, "<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" rx=\"4\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.1f\"/>\n", b.X, b.Y, b.W, b.H, fill, stroke, strokeWidth)
	default:
		fmt.Fprintf(&builder, "<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" rx=\"8\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%.1f\"/>\n", b.X, b.Y, b.W, b.H, fill, stroke, strokeWidth)
	}
	builder.WriteString(renderSVGMarker(shape, stroke))
	if shape.Name != "" {
		labelX, labelY := shapeLabelPoint(shape)
		fmt.Fprintf(&builder, "<text x=\"%.1f\" y=\"%.1f\" font-family=\"sans-serif\" font-size=\"12\" fill=\"#111111\" text-anchor=\"middle\" dominant-baseline=\"middle\">%s</text>\n", labelX, labelY, html.EscapeString(shape.Name))
	}
	return builder.String()
}

// renderSVGMarker 渲染图形内部标记。
// @param shape 图形
// @param stroke 描边颜色
// @return SVG 片段
func renderSVGMarker(shape Shape, stroke string) string {
	if shape.Marker == "" {
		return ""
	}
	box := markerBox(shape)
	if box.W < 4 || box.H < 4 {
		return ""
	}
	x, y, w, h := box.X, box.Y, box.W, box.H
	cx, cy := x+w/2, y+h/2
	var builder strings.Builder
	fmt.Fprintf(&builder, "<g class=\"marker marker-%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"1.2\" stroke-linecap=\"round\" stroke-linejoin=\"round\">\n", shape.Marker, stroke)
	switch shape.Marker {
	case markerTimer:
		radius := math.Min(w, h) / 2
		fmt.Fprintf(&builder, "<circle cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\"/>\n", cx, cy, radius)
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f M %.1f %.1f L %.1f %.1f\"/>\n",
			cx, cy, cx, cy-radius*0.6, cx, cy, cx+radius*0.5, cy)
	case markerMessage, markerReceive:
		ex, ey, ew, eh := x, y+h*0.15, w, h*0.7
		fmt.Fprintf(&builder, "<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" rx=\"1\"/>\n", ex, ey, ew, eh)
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f L %.1f %.1f\"/>\n", ex, ey, ex+ew/2, ey+eh*0.6, ex+ew, ey)
	case markerSend:
		ex, ey, ew, eh := x, y+h*0.15, w, h*0.7
		fmt.Fprintf(&builder, "<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" rx=\"1\" fill=\"%s\"/>\n", ex, ey, ew, eh, stroke)
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f L %.1f %.1f\" stroke=\"#ffffff\"/>\n", ex, ey, ex+ew/2, ey+eh*0.6, ex+ew, ey)
	case markerSignal:
		fmt.Fprintf(&builder, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f\"/>\n", cx, y, x+w, y+h, x, y+h)
	case markerTerminate:
		// 终止事件使用实心圆，覆盖分组默认的空心填充
		radius := math.Min(w, h) / 2
		fmt.Fprintf(&builder, "<circle cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\" fill=\"%s\"/>\n", cx, cy, radius, stroke)
	case markerCancel:
		// 取消事件使用加粗叉号
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f M %.1f %.1f L %.1f %.1f\" stroke-width=\"2.5\"/>\n",
			x+w*0.15, y+h*0.15, x+w*0.85, y+h*0.85,
			x+w*0.85, y+h*0.15, x+w*0.15, y+h*0.85)
	case markerError:
		// 错误事件使用闪电多边形
		fmt.Fprintf(&builder, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f\"/>\n",
			x+w*0.6, y, x+w*0.15, y+h*0.55, x+w*0.42, y+h*0.55,
			x+w*0.3, y+h, x+w*0.85, y+h*0.42, x+w*0.55, y+h*0.42)
	case markerCompensate:
		// 补偿事件使用两个指向左侧的三角形
		fmt.Fprintf(&builder, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f\"/>\n", x+w*0.45, y, x+w*0.05, cy, x+w*0.45, y+h)
		fmt.Fprintf(&builder, "<polygon points=\"%.1f,%.1f %.1f,%.1f %.1f,%.1f\"/>\n", x+w*0.95, y, x+w*0.55, cy, x+w*0.95, y+h)
	case markerLink:
		// 链接事件使用指向右侧的箭头
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f M %.1f %.1f L %.1f %.1f L %.1f %.1f\"/>\n",
			x, cy, x+w, cy,
			x+w*0.65, cy-h*0.3, x+w, cy, x+w*0.65, cy+h*0.3)
	case markerUser:
		fmt.Fprintf(&builder, "<circle cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\"/>\n", cx, y+h*0.28, h*0.18)
		fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f Q %.1f %.1f %.1f %.1f\"/>\n", x+w*0.15, y+h, cx, y+h*0.42, x+w*0.85, y+h)
	case markerService:
		radius := math.Min(w, h) * 0.28
		fmt.Fprintf(&builder, "<circle cx=\"%.1f\" cy=\"%.1f\" r=\"%.1f\"/>\n", cx, cy, radius)
		for index := 0; index < 6; index++ {
			angle := math.Pi * 2 * float64(index) / 6
			fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f\"/>\n",
				cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				cx+math.Cos(angle)*radius*1.55, cy+math.Sin(angle)*radius*1.55)
		}
	case markerScript:
		fmt.Fprintf(&builder, "<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" rx=\"1\"/>\n", x, y, w, h)
		for index := 0; index < 3; index++ {
			lineY := y + h*(0.3+0.2*float64(index))
			fmt.Fprintf(&builder, "<path d=\"M %.1f %.1f L %.1f %.1f\"/>\n", x+w*0.2, lineY, x+w*0.8, lineY)
		}
	}
	builder.WriteString("</g>\n")
	return builder.String()
}
