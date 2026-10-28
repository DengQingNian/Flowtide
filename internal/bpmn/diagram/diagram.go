// Package diagram 解析 BPMN DI 并渲染流程图为 SVG/PNG。
package diagram

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/bpmn/model"
	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
)

// Point 是二维坐标。
type Point struct {
	X float64 `json:"x"` // 横坐标
	Y float64 `json:"y"` // 纵坐标
}

// Bounds 是矩形区域。
type Bounds struct {
	X float64 `json:"x"`      // 左上角横坐标
	Y float64 `json:"y"`      // 左上角纵坐标
	W float64 `json:"width"`  // 宽度
	H float64 `json:"height"` // 高度
}

// Shape 是 BPMN 图形节点。
type Shape struct {
	ElementID string  // 元素 ID
	Type      string  // 元素类型
	Name      string  // 元素名称
	Marker    string  // 内部标记类型：user/service/script/send/receive/timer/message/signal/terminate/cancel/error/compensate/link
	Bounds    Bounds  // 图形区域
	Label     *Bounds // 标签区域
}

// Edge 是 BPMN 顺序流。
type Edge struct {
	ElementID string  // 元素 ID
	Type      string  // 元素类型
	Name      string  // 元素名称
	Waypoints []Point // 折线点
	Label     *Bounds // 标签区域
}

// Diagram 是解析后的流程图。
type Diagram struct {
	Width  int     // 画布宽度
	Height int     // 画布高度
	Pools  []Pool  // 泳池，按文档顺序
	Lanes  []Lane  // 泳道，按文档顺序
	Shapes []Shape // 图形节点
	Edges  []Edge  // 连线
}

// defaultLimit 是 XML 解析的最大字节数。
const defaultLimit = 64 << 20

// Parse 解析 BPMN XML 中的图形信息。
// @param bpmn 解析后的 BPMN 模型，用于补全名称与类型
// @param data 原始 BPMN XML
// @return 流程图
// @return 解析错误
func Parse(bpmn *model.BpmnModel, data []byte) (*Diagram, error) {
	root, err := bpmnxml.Decode(bytes.NewReader(data), defaultLimit)
	if err != nil {
		return nil, api.NewError("ENGINE-1901", "解析 BPMN 图形失败", err)
	}
	containers := collectContainers(root)
	diagram := &Diagram{
		Pools:  make([]Pool, 0),
		Lanes:  make([]Lane, 0),
		Shapes: make([]Shape, 0),
		Edges:  make([]Edge, 0),
	}
	for _, diagramNode := range root.ChildrenNamed("BPMNDiagram") {
		for _, plane := range diagramNode.ChildrenNamed("BPMNPlane") {
			for _, child := range plane.Children {
				switch child.Name.Local {
				case "BPMNShape":
					elementID := child.Attr("bpmnElement")
					meta := containers[elementID]
					// 泳池/泳道单独存放，避免污染节点列表
					if pool, ok := parsePool(child, meta, bpmn); ok {
						diagram.Pools = append(diagram.Pools, *pool)
						continue
					}
					if lane, ok := parseLane(child, meta, bpmn); ok {
						diagram.Lanes = append(diagram.Lanes, *lane)
						continue
					}
					shape := parseShape(child, bpmn)
					if shape != nil {
						diagram.Shapes = append(diagram.Shapes, *shape)
					}
				case "BPMNEdge":
					edge := parseEdge(child, bpmn)
					if edge != nil {
						diagram.Edges = append(diagram.Edges, *edge)
					}
				}
			}
		}
		if len(diagram.Shapes) > 0 {
			break
		}
	}
	if len(diagram.Shapes) == 0 {
		return nil, api.NewError("ENGINE-1901", "流程定义缺少图形信息", api.ErrBadRequest)
	}
	diagram.computeSize()
	return diagram, nil
}

// parseShape 解析 BPMNShape。
// @param node 节点
// @param bpmn BPMN 模型
// @return 图形
func parseShape(node *bpmnxml.Node, bpmn *model.BpmnModel) *Shape {
	elementID := node.Attr("bpmnElement")
	if elementID == "" {
		return nil
	}
	bounds, ok := parseBounds(node)
	if !ok {
		return nil
	}
	shape := &Shape{ElementID: elementID, Bounds: bounds, Label: parseLabel(node)}
	if bpmn != nil {
		shape.Name, shape.Type = elementInfo(bpmn, elementID)
		shape.Marker = elementMarker(bpmn, elementID)
	}
	return shape
}

// parseEdge 解析 BPMNEdge。
// @param node 节点
// @param bpmn BPMN 模型
// @return 连线
func parseEdge(node *bpmnxml.Node, bpmn *model.BpmnModel) *Edge {
	elementID := node.Attr("bpmnElement")
	if elementID == "" {
		return nil
	}
	edge := &Edge{ElementID: elementID, Waypoints: make([]Point, 0), Label: parseLabel(node)}
	if bpmn != nil {
		edge.Name, edge.Type = elementInfo(bpmn, elementID)
	}
	for _, waypoint := range node.ChildrenNamed("waypoint") {
		edge.Waypoints = append(edge.Waypoints, Point{
			X: parseFloat(waypoint.Attr("x")),
			Y: parseFloat(waypoint.Attr("y")),
		})
	}
	if len(edge.Waypoints) < 2 {
		return nil
	}
	return edge
}

// parseBounds 解析 Bounds 子节点。
// @param node 节点
// @return 图形区域
// @return 是否存在
func parseBounds(node *bpmnxml.Node) (Bounds, bool) {
	boundsNode, ok := node.Child("Bounds")
	if !ok {
		return Bounds{}, false
	}
	return Bounds{
		X: parseFloat(boundsNode.Attr("x")),
		Y: parseFloat(boundsNode.Attr("y")),
		W: parseFloat(boundsNode.Attr("width")),
		H: parseFloat(boundsNode.Attr("height")),
	}, true
}

// parseLabel 解析 BPMNLabel 中的标签区域。
// @param node 节点
// @return 标签区域，不存在时为 nil
func parseLabel(node *bpmnxml.Node) *Bounds {
	label, ok := node.Child("BPMNLabel")
	if !ok {
		return nil
	}
	labelBounds, ok := label.Child("Bounds")
	if !ok {
		return nil
	}
	return &Bounds{
		X: parseFloat(labelBounds.Attr("x")),
		Y: parseFloat(labelBounds.Attr("y")),
		W: parseFloat(labelBounds.Attr("width")),
		H: parseFloat(labelBounds.Attr("height")),
	}
}

// elementInfo 返回元素名称与类型。
// @param bpmn BPMN 模型
// @param elementID 元素 ID
// @return 名称
// @return 类型
func elementInfo(bpmn *model.BpmnModel, elementID string) (string, string) {
	element := findElement(bpmn, elementID)
	if element == nil {
		return "", ""
	}
	return element.GetName(), element.ElementType()
}

// parseFloat 解析浮点数。
// @param raw 原始字符串
// @return 浮点数，非法时返回 0
func parseFloat(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return value
}

// computeSize 计算画布尺寸。
func (d *Diagram) computeSize() {
	maxX, maxY := 0.0, 0.0
	for _, bounds := range d.boundsList() {
		if bounds.X+bounds.W > maxX {
			maxX = bounds.X + bounds.W
		}
		if bounds.Y+bounds.H > maxY {
			maxY = bounds.Y + bounds.H
		}
	}
	// 留出边距
	d.Width = int(maxX) + 20
	d.Height = int(maxY) + 20
	if d.Width < 100 {
		d.Width = 100
	}
	if d.Height < 100 {
		d.Height = 100
	}
}

// boundsList 返回参与画布尺寸计算的矩形。
// @return 矩形列表
func (d *Diagram) boundsList() []Bounds {
	bounds := make([]Bounds, 0, len(d.Pools)+len(d.Lanes)+len(d.Shapes)*2)
	for _, pool := range d.Pools {
		bounds = append(bounds, pool.Bounds)
	}
	for _, lane := range d.Lanes {
		bounds = append(bounds, lane.Bounds)
	}
	for _, shape := range d.Shapes {
		bounds = append(bounds, shape.Bounds)
		if shape.Label != nil {
			bounds = append(bounds, *shape.Label)
		}
	}
	for _, edge := range d.Edges {
		for _, point := range edge.Waypoints {
			bounds = append(bounds, Bounds{X: point.X, Y: point.Y})
		}
	}
	return bounds
}
