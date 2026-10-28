package diagram

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
)

// pngScale 是超采样倍数。
const pngScale = 2

// PNG 泳池/泳道样式常量。
const (
	pngContainerBand  = 30.0 // 名称带宽度
	pngContainerScale = 1.5  // 泳池/泳道文字缩放
	pngContainerGap   = 12.0 // 名称与边框的间距
)

// 泳池/泳道颜色。
var (
	pngPoolFill      = color.RGBA{R: 248, G: 250, B: 252, A: 255} // 泳池填充色
	pngPoolBorder    = color.RGBA{R: 148, G: 163, B: 184, A: 255} // 泳池边框色
	pngLaneFill      = color.RGBA{R: 255, G: 255, B: 255, A: 255} // 泳道填充色
	pngLaneBorder    = color.RGBA{R: 203, G: 213, B: 225, A: 255} // 泳道边框色
	pngLaneSeparator = color.RGBA{R: 148, G: 163, B: 184, A: 255} // 泳道分隔线颜色
	pngContainerText = color.RGBA{R: 51, G: 65, B: 85, A: 255}    // 泳池/泳道文字颜色
)

// pngRenderer 是 PNG 渲染器。
type pngRenderer struct {
	img *image.RGBA // 画布
}

// RenderPNG 渲染流程图为 PNG。
// @param d 流程图
// @param highlights 需要高亮的活动 ID
// @return PNG 内容
// @return 渲染错误
func RenderPNG(d *Diagram, highlights map[string]bool) ([]byte, error) {
	renderer := &pngRenderer{img: image.NewRGBA(image.Rect(0, 0, d.Width*pngScale, d.Height*pngScale))}
	renderer.fillBackground()
	// 先绘制泳池与泳道，保证节点与连线位于其上层
	for _, pool := range d.Pools {
		renderer.drawPool(pool)
	}
	for _, lane := range d.Lanes {
		renderer.drawLane(lane)
	}
	for _, edge := range d.Edges {
		renderer.drawEdge(edge)
	}
	for _, shape := range d.Shapes {
		renderer.drawShape(shape, highlights[shape.ElementID])
	}
	output := image.NewRGBA(image.Rect(0, 0, d.Width, d.Height))
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			var red, green, blue, alpha uint32
			for dy := 0; dy < pngScale; dy++ {
				for dx := 0; dx < pngScale; dx++ {
					pixel := renderer.img.RGBAAt(x*pngScale+dx, y*pngScale+dy)
					red += uint32(pixel.R)
					green += uint32(pixel.G)
					blue += uint32(pixel.B)
					alpha += uint32(pixel.A)
				}
			}
			total := uint32(pngScale * pngScale)
			output.SetRGBA(x, y, color.RGBA{R: uint8(red / total), G: uint8(green / total), B: uint8(blue / total), A: uint8(alpha / total)})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, output); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// fillBackground 填充白色背景。
func (r *pngRenderer) fillBackground() {
	for index := 0; index < len(r.img.Pix); index += 4 {
		r.img.Pix[index] = 255
		r.img.Pix[index+1] = 255
		r.img.Pix[index+2] = 255
		r.img.Pix[index+3] = 255
	}
}

// setPixel 设置像素。
// @param x 横坐标
// @param y 纵坐标
// @param c 颜色
func (r *pngRenderer) setPixel(x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= r.img.Bounds().Dx() || y >= r.img.Bounds().Dy() {
		return
	}
	r.img.SetRGBA(x, y, c)
}

// line 绘制线段。
// @param x1 起点横坐标
// @param y1 起点纵坐标
// @param x2 终点横坐标
// @param y2 终点纵坐标
// @param thickness 线宽（最终像素）
// @param c 颜色
func (r *pngRenderer) line(x1, y1, x2, y2, thickness float64, c color.RGBA) {
	x1 *= pngScale
	y1 *= pngScale
	x2 *= pngScale
	y2 *= pngScale
	thickness *= pngScale
	steps := int(math.Max(math.Abs(x2-x1), math.Abs(y2-y1)))
	if steps == 0 {
		steps = 1
	}
	half := int(math.Ceil(thickness / 2))
	for step := 0; step <= steps; step++ {
		x := int(math.Round(x1 + (x2-x1)*float64(step)/float64(steps)))
		y := int(math.Round(y1 + (y2-y1)*float64(step)/float64(steps)))
		for dy := -half; dy <= half; dy++ {
			for dx := -half; dx <= half; dx++ {
				r.setPixel(x+dx, y+dy, c)
			}
		}
	}
}

// dashedLine 绘制虚线。
// @param x1 起点横坐标
// @param y1 起点纵坐标
// @param x2 终点横坐标
// @param y2 终点纵坐标
// @param c 颜色
func (r *pngRenderer) dashedLine(x1, y1, x2, y2 float64, c color.RGBA) {
	length := math.Hypot(x2-x1, y2-y1)
	if length == 0 {
		return
	}
	step := 6.0
	for offset := 0.0; offset < length; offset += step * 2 {
		start := offset / length
		end := math.Min((offset+step)/length, 1)
		r.line(x1+(x2-x1)*start, y1+(y2-y1)*start, x1+(x2-x1)*end, y1+(y2-y1)*end, 1, c)
	}
}

// fillRect 填充矩形。
// @param x 横坐标
// @param y 纵坐标
// @param width 宽度
// @param height 高度
// @param c 颜色
func (r *pngRenderer) fillRect(x, y, width, height float64, c color.RGBA) {
	for py := int(y * pngScale); py < int((y+height)*pngScale); py++ {
		for px := int(x * pngScale); px < int((x+width)*pngScale); px++ {
			r.setPixel(px, py, c)
		}
	}
}

// strokeRect 绘制矩形边框。
// @param x 横坐标
// @param y 纵坐标
// @param width 宽度
// @param height 高度
// @param c 颜色
func (r *pngRenderer) strokeRect(x, y, width, height float64, c color.RGBA) {
	r.line(x, y, x+width, y, 1, c)
	r.line(x+width, y, x+width, y+height, 1, c)
	r.line(x+width, y+height, x, y+height, 1, c)
	r.line(x, y+height, x, y, 1, c)
}

// circle 绘制圆形（填充 + 边框）。
// @param cx 圆心横坐标
// @param cy 圆心纵坐标
// @param radius 半径
// @param fill 填充色
// @param stroke 边框色
// @param border 边框宽度
func (r *pngRenderer) circle(cx, cy, radius float64, fill, stroke color.RGBA, border float64) {
	centerX, centerY := cx*pngScale, cy*pngScale
	scaledRadius := radius * pngScale
	inner := scaledRadius - border*pngScale
	for py := int(centerY - scaledRadius); py <= int(centerY+scaledRadius); py++ {
		for px := int(centerX - scaledRadius); px <= int(centerX+scaledRadius); px++ {
			distance := math.Hypot(float64(px)-centerX, float64(py)-centerY)
			if distance > scaledRadius {
				continue
			}
			if distance >= inner {
				r.setPixel(px, py, stroke)
			} else {
				r.setPixel(px, py, fill)
			}
		}
	}
}

// polygon 填充多边形并描边。
// @param points 顶点
// @param fill 填充色
// @param stroke 边框色
func (r *pngRenderer) polygon(points []Point, fill, stroke color.RGBA) {
	if len(points) < 3 {
		return
	}
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, point := range points {
		minX = math.Min(minX, point.X)
		maxX = math.Max(maxX, point.X)
		minY = math.Min(minY, point.Y)
		maxY = math.Max(maxY, point.Y)
	}
	for py := int(minY * pngScale); py <= int(maxY*pngScale); py++ {
		for px := int(minX * pngScale); px <= int(maxX*pngScale); px++ {
			x := float64(px) / pngScale
			y := float64(py) / pngScale
			if insidePolygon(x, y, points) {
				r.setPixel(px, py, fill)
			}
		}
	}
	for index := range points {
		next := points[(index+1)%len(points)]
		r.line(points[index].X, points[index].Y, next.X, next.Y, 1.5, stroke)
	}
}

// insidePolygon 判断点是否在多边形内。
// @param x 横坐标
// @param y 纵坐标
// @param points 顶点
// @return 是否在内部
func insidePolygon(x, y float64, points []Point) bool {
	inside := false
	j := len(points) - 1
	for i := 0; i < len(points); i++ {
		if (points[i].Y > y) != (points[j].Y > y) &&
			x < (points[j].X-points[i].X)*(y-points[i].Y)/(points[j].Y-points[i].Y)+points[i].X {
			inside = !inside
		}
		j = i
	}
	return inside
}

// drawPool 绘制泳池。
// @param pool 泳池
func (r *pngRenderer) drawPool(pool Pool) {
	b := pool.Bounds
	r.fillRect(b.X, b.Y, b.W, b.H, pngPoolFill)
	r.strokeRect(b.X, b.Y, b.W, b.H, pngPoolBorder)
	// 水平泳池的名称带在左侧，垂直泳池的名称带在顶部
	if pool.IsHorizontal {
		band := math.Min(pngContainerBand, b.W/3)
		r.line(b.X+band, b.Y, b.X+band, b.Y+b.H, 1, pngPoolBorder)
		maxChars := int((b.H - pngContainerGap) / (7 * pngContainerScale))
		if name := fitChars(pool.Name, maxChars); name != "" {
			r.verticalText(b.X+band/2, b.Y+b.H/2, name, pngContainerScale, pngContainerText)
		}
		return
	}
	band := math.Min(pngContainerBand, b.H/3)
	r.line(b.X, b.Y+band, b.X+b.W, b.Y+band, 1, pngPoolBorder)
	maxChars := int((b.W - pngContainerGap) / (5 * pngContainerScale))
	if name := fitChars(pool.Name, maxChars); name != "" {
		r.textScaled(b.X+b.W/2, b.Y+band/2, name, pngContainerScale, pngContainerText)
	}
}

// drawLane 绘制泳道。
// @param lane 泳道
func (r *pngRenderer) drawLane(lane Lane) {
	b := lane.Bounds
	r.fillRect(b.X, b.Y, b.W, b.H, pngLaneFill)
	r.strokeRect(b.X, b.Y, b.W, b.H, pngLaneBorder)
	// 泳道分隔线沿泳道起始边绘制
	if lane.IsHorizontal {
		r.line(b.X, b.Y, b.X+b.W, b.Y, 1.5, pngLaneSeparator)
	} else {
		r.line(b.X, b.Y, b.X, b.Y+b.H, 1.5, pngLaneSeparator)
	}
	maxChars := int((b.W - pngContainerGap) / (5 * pngContainerScale))
	name := fitChars(lane.Name, maxChars)
	if name == "" {
		return
	}
	if lane.IsHorizontal {
		r.textAt(b.X+6, b.Y+b.H/2, name, pngContainerScale, pngContainerText)
		return
	}
	r.textScaled(b.X+b.W/2, b.Y+math.Min(pngContainerBand, b.H/3)/2, name, pngContainerScale, pngContainerText)
}

// drawShape 绘制图形节点。
// @param shape 图形
// @param highlighted 是否高亮
func (r *pngRenderer) drawShape(shape Shape, highlighted bool) {
	border := color.RGBA{R: 17, G: 17, B: 17, A: 255}
	fill := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if highlighted {
		border = color.RGBA{R: 37, G: 99, B: 235, A: 255}
		fill = color.RGBA{R: 219, G: 234, B: 254, A: 255}
	}
	b := shape.Bounds
	switch {
	case strings.Contains(shape.Type, "Event"):
		radius := math.Min(b.W, b.H) / 2
		r.circle(b.X+b.W/2, b.Y+b.H/2, radius, fill, border, 1.5)
		if shape.Type == "endEvent" || shape.Type == "intermediateThrowEvent" || shape.Type == "boundaryEvent" {
			r.circle(b.X+b.W/2, b.Y+b.H/2, math.Max(radius-6, 2), fill, border, 1)
		}
	case strings.Contains(shape.Type, "Gateway"):
		r.polygon([]Point{
			{X: b.X + b.W/2, Y: b.Y},
			{X: b.X + b.W, Y: b.Y + b.H/2},
			{X: b.X + b.W/2, Y: b.Y + b.H},
			{X: b.X, Y: b.Y + b.H/2},
		}, fill, border)
	case shape.Type == "textAnnotation":
		r.line(b.X, b.Y, b.X, b.Y+b.H, 1, border)
		r.line(b.X, b.Y+b.H, b.X+b.W, b.Y+b.H, 1, border)
	case shape.Type == "dataObjectReference" || shape.Type == "dataStoreReference":
		r.polygon([]Point{
			{X: b.X + 8, Y: b.Y},
			{X: b.X + b.W, Y: b.Y},
			{X: b.X + b.W, Y: b.Y + b.H},
			{X: b.X, Y: b.Y + b.H},
		}, fill, border)
	default:
		r.fillRect(b.X, b.Y, b.W, b.H, fill)
		r.strokeRect(b.X, b.Y, b.W, b.H, border)
	}
	r.drawMarker(shape, border)
	r.drawLabel(shape, border)
}

// drawMarker 绘制图形内部标记。
// @param shape 图形
// @param c 描边颜色
func (r *pngRenderer) drawMarker(shape Shape, c color.RGBA) {
	if shape.Marker == "" {
		return
	}
	box := markerBox(shape)
	if box.W < 4 || box.H < 4 {
		return
	}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	x, y, w, h := box.X, box.Y, box.W, box.H
	cx, cy := x+w/2, y+h/2
	switch shape.Marker {
	case markerTimer:
		radius := math.Min(w, h) / 2
		r.circle(cx, cy, radius, white, c, 1)
		r.line(cx, cy, cx, cy-radius*0.6, 1, c)
		r.line(cx, cy, cx+radius*0.5, cy, 1, c)
	case markerMessage, markerReceive:
		ex, ey, ew, eh := x, y+h*0.15, w, h*0.7
		r.strokeRect(ex, ey, ew, eh, c)
		r.line(ex, ey, ex+ew/2, ey+eh*0.6, 1, c)
		r.line(ex+ew/2, ey+eh*0.6, ex+ew, ey, 1, c)
	case markerSend:
		ex, ey, ew, eh := x, y+h*0.15, w, h*0.7
		r.fillRect(ex, ey, ew, eh, c)
		r.line(ex, ey, ex+ew/2, ey+eh*0.6, 1, white)
		r.line(ex+ew/2, ey+eh*0.6, ex+ew, ey, 1, white)
	case markerSignal:
		r.polygon([]Point{
			{X: cx, Y: y},
			{X: x + w, Y: y + h},
			{X: x, Y: y + h},
		}, white, c)
	case markerTerminate:
		// 终止事件使用实心圆
		radius := math.Min(w, h) / 2
		r.circle(cx, cy, radius, c, c, 1)
	case markerCancel:
		// 取消事件使用加粗叉号
		r.line(x+w*0.15, y+h*0.15, x+w*0.85, y+h*0.85, 2.5, c)
		r.line(x+w*0.85, y+h*0.15, x+w*0.15, y+h*0.85, 2.5, c)
	case markerError:
		// 错误事件使用闪电多边形
		r.polygon([]Point{
			{X: x + w*0.6, Y: y},
			{X: x + w*0.15, Y: y + h*0.55},
			{X: x + w*0.42, Y: y + h*0.55},
			{X: x + w*0.3, Y: y + h},
			{X: x + w*0.85, Y: y + h*0.42},
			{X: x + w*0.55, Y: y + h*0.42},
		}, white, c)
	case markerCompensate:
		// 补偿事件使用两个指向左侧的三角形
		r.polygon([]Point{
			{X: x + w*0.45, Y: y},
			{X: x + w*0.05, Y: cy},
			{X: x + w*0.45, Y: y + h},
		}, white, c)
		r.polygon([]Point{
			{X: x + w*0.95, Y: y},
			{X: x + w*0.55, Y: cy},
			{X: x + w*0.95, Y: y + h},
		}, white, c)
	case markerLink:
		// 链接事件使用指向右侧的箭头
		r.line(x, cy, x+w, cy, 1, c)
		r.line(x+w*0.65, cy-h*0.3, x+w, cy, 1, c)
		r.line(x+w*0.65, cy+h*0.3, x+w, cy, 1, c)
	case markerUser:
		r.circle(cx, y+h*0.28, h*0.18, white, c, 1)
		r.line(x+w*0.15, y+h, cx, y+h*0.5, 1, c)
		r.line(cx, y+h*0.5, x+w*0.85, y+h, 1, c)
	case markerService:
		radius := math.Min(w, h) * 0.28
		r.circle(cx, cy, radius, white, c, 1)
		for index := 0; index < 6; index++ {
			angle := math.Pi * 2 * float64(index) / 6
			r.line(cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				cx+math.Cos(angle)*radius*1.55, cy+math.Sin(angle)*radius*1.55, 1, c)
		}
	case markerScript:
		r.strokeRect(x, y, w, h, c)
		for index := 0; index < 3; index++ {
			lineY := y + h*(0.3+0.2*float64(index))
			r.line(x+w*0.2, lineY, x+w*0.8, lineY, 1, c)
		}
	}
}

// drawLabel 绘制图形标签。
// @param shape 图形
// @param c 颜色
func (r *pngRenderer) drawLabel(shape Shape, c color.RGBA) {
	if shape.Name == "" {
		return
	}
	centerX, centerY := shapeLabelPoint(shape)
	r.text(centerX, centerY, shape.Name, c)
}

// text 绘制文本（仅 ASCII）。
// @param centerX 中心横坐标
// @param centerY 中心纵坐标
// @param value 文本
// @param c 颜色
func (r *pngRenderer) text(centerX, centerY float64, value string, c color.RGBA) {
	r.textScaled(centerX, centerY, value, 3, c)
}

// textScaled 按缩放倍数绘制居中的文本（仅 ASCII）。
// @param centerX 中心横坐标
// @param centerY 中心纵坐标
// @param value 文本
// @param scale 缩放倍数
// @param c 颜色
func (r *pngRenderer) textScaled(centerX, centerY float64, value string, scale float64, c color.RGBA) {
	width := float64(len([]rune(strings.ToUpper(value)))) * 5 * scale
	r.textAt(centerX-width/2, centerY, value, scale, c)
}

// textAt 从左侧起点绘制文本（仅 ASCII）。
// @param startX 左侧起点横坐标
// @param centerY 中心纵坐标
// @param value 文本
// @param scale 缩放倍数
// @param c 颜色
func (r *pngRenderer) textAt(startX, centerY float64, value string, scale float64, c color.RGBA) {
	runes := []rune(strings.ToUpper(value))
	startY := centerY - 3*scale
	for index, char := range runes {
		glyph, ok := fontGlyphs[char]
		if !ok {
			continue
		}
		offsetX := startX + float64(index)*5*scale
		for _, segment := range glyph {
			r.line(offsetX+float64(segment[0])*scale, startY+float64(segment[1])*scale,
				offsetX+float64(segment[2])*scale, startY+float64(segment[3])*scale, 1, c)
		}
	}
}

// verticalText 绘制竖排文本（用于水平泳池的名称带）。
// @param centerX 中心横坐标
// @param centerY 中心纵坐标
// @param value 文本
// @param scale 缩放倍数
// @param c 颜色
func (r *pngRenderer) verticalText(centerX, centerY float64, value string, scale float64, c color.RGBA) {
	runes := []rune(strings.ToUpper(value))
	if len(runes) == 0 {
		return
	}
	advance := 7 * scale
	startY := centerY - float64(len(runes))*advance/2 + advance/2
	for index, char := range runes {
		r.textScaled(centerX, startY+float64(index)*advance, string(char), scale, c)
	}
}

// drawEdge 绘制连线。
// @param edge 连线
func (r *pngRenderer) drawEdge(edge Edge) {
	if len(edge.Waypoints) < 2 {
		return
	}
	c := color.RGBA{R: 85, G: 85, B: 85, A: 255}
	for index := 0; index < len(edge.Waypoints)-1; index++ {
		start := edge.Waypoints[index]
		end := edge.Waypoints[index+1]
		if edge.Type == "association" {
			r.dashedLine(start.X, start.Y, end.X, end.Y, c)
		} else {
			r.line(start.X, start.Y, end.X, end.Y, 1.5, c)
		}
	}
	r.drawArrow(edge.Waypoints[len(edge.Waypoints)-2], edge.Waypoints[len(edge.Waypoints)-1], c)
	if edge.Name != "" {
		center := edge.Waypoints[len(edge.Waypoints)/2]
		r.text(center.X, center.Y-8, edge.Name, c)
	}
}

// drawArrow 绘制箭头。
// @param from 起点
// @param to 终点
// @param c 颜色
func (r *pngRenderer) drawArrow(from, to Point, c color.RGBA) {
	angle := math.Atan2(to.Y-from.Y, to.X-from.X)
	length := 10.0
	left := Point{X: to.X - length*math.Cos(angle-math.Pi/6), Y: to.Y - length*math.Sin(angle-math.Pi/6)}
	right := Point{X: to.X - length*math.Cos(angle+math.Pi/6), Y: to.Y - length*math.Sin(angle+math.Pi/6)}
	r.line(to.X, to.Y, left.X, left.Y, 1.5, c)
	r.line(to.X, to.Y, right.X, right.Y, 1.5, c)
}
