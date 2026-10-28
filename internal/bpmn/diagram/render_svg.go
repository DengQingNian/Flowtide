package diagram

import (
	"fmt"
	"html"
	"strings"
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
	for _, edge := range d.Edges {
		builder.WriteString(renderSVGEdge(edge))
	}
	for _, shape := range d.Shapes {
		builder.WriteString(renderSVGShape(shape, highlights[shape.ElementID]))
	}
	builder.WriteString("</svg>\n")
	return []byte(builder.String())
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
	if shape.Name != "" {
		labelX, labelY := b.X+b.W/2, b.Y+b.H/2
		if shape.Label != nil && shape.Label.W > 0 {
			labelX = shape.Label.X + shape.Label.W/2
			labelY = shape.Label.Y + shape.Label.H/2
		}
		fmt.Fprintf(&builder, "<text x=\"%.1f\" y=\"%.1f\" font-family=\"sans-serif\" font-size=\"12\" fill=\"#111111\" text-anchor=\"middle\" dominant-baseline=\"middle\">%s</text>\n", labelX, labelY, html.EscapeString(shape.Name))
	}
	return builder.String()
}
