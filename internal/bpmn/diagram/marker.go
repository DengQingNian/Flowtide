package diagram

import (
	"math"
	"strings"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 内部标记类型常量。
const (
	markerUser       = "user"       // 用户任务
	markerService    = "service"    // 服务任务
	markerScript     = "script"     // 脚本任务
	markerSend       = "send"       // 发送任务
	markerReceive    = "receive"    // 接收任务
	markerTimer      = "timer"      // 定时器事件
	markerMessage    = "message"    // 消息事件
	markerSignal     = "signal"     // 信号事件
	markerTerminate  = "terminate"  // 终止结束事件
	markerCancel     = "cancel"     // 取消事件
	markerError      = "error"      // 错误事件
	markerCompensate = "compensate" // 补偿事件
	markerLink       = "link"       // 链接事件
)

// findElement 在 BPMN 模型中按 ID 查找元素。
// @param bpmn BPMN 模型
// @param elementID 元素 ID
// @return 流程元素，未找到时为 nil
func findElement(bpmn *model.BpmnModel, elementID string) model.FlowElement {
	if bpmn == nil {
		return nil
	}
	for _, process := range bpmn.Processes {
		if element, ok := process.Elements[elementID]; ok {
			return element
		}
	}
	return nil
}

// elementMarker 返回元素的内部标记类型。
// @param bpmn BPMN 模型
// @param elementID 元素 ID
// @return 标记类型，无需标记时为空
func elementMarker(bpmn *model.BpmnModel, elementID string) string {
	element := findElement(bpmn, elementID)
	if element == nil {
		return ""
	}
	return markerOf(element)
}

// markerOf 根据流程元素返回内部标记类型。
// @param element 流程元素
// @return 标记类型
func markerOf(element model.FlowElement) string {
	switch typed := element.(type) {
	case *model.UserTask:
		return markerUser
	case *model.ServiceTask:
		return markerService
	case *model.ScriptTask:
		return markerScript
	case *model.SendTask:
		return markerSend
	case *model.ReceiveTask:
		return markerReceive
	case *model.StartEvent:
		return eventMarker(typed.Timer, typed.MessageRef, typed.SignalRef)
	case *model.EndEvent:
		return endEventMarker(typed)
	case *model.BoundaryEvent:
		// 保留原有定时器/消息/信号优先级，其次才是补偿、取消与错误
		if marker := eventMarker(typed.Timer, typed.MessageRef, typed.SignalRef); marker != "" {
			return marker
		}
		return eventExtraMarker(typed.ErrorRef, "", typed.Compensate, typed.Cancel)
	case *model.IntermediateCatchEvent:
		// 保留原有定时器/消息/信号优先级，链接事件作为兜底
		if marker := eventMarker(typed.Timer, typed.MessageRef, typed.SignalRef); marker != "" {
			return marker
		}
		return eventExtraMarker("", typed.LinkName, false, false)
	case *model.IntermediateThrowEvent:
		return eventExtraMarker("", typed.LinkName, typed.Compensate, false)
	}
	return ""
}

// endEventMarker 返回结束事件的标记类型。
// @param event 结束事件
// @return 标记类型
func endEventMarker(event *model.EndEvent) string {
	// 终止语义最强，优先于取消与错误
	if event.Terminate {
		return markerTerminate
	}
	return eventExtraMarker(event.ErrorRef, "", false, event.Cancel)
}

// eventMarker 根据事件定义返回标记类型。
// @param timer 定时器定义
// @param messageRef 消息引用
// @param signalRef 信号引用
// @return 标记类型
func eventMarker(timer *model.TimerDefinition, messageRef, signalRef string) string {
	switch {
	// 定时器优先级最高，其余按消息、信号依次判断
	case timer != nil:
		return markerTimer
	case messageRef != "":
		return markerMessage
	case signalRef != "":
		return markerSignal
	default:
		return ""
	}
}

// eventExtraMarker 根据扩展事件定义返回标记类型。
// @param errorRef 错误引用
// @param linkName 链接事件名称
// @param compensate 是否补偿事件
// @param cancel 是否取消事件
// @return 标记类型
func eventExtraMarker(errorRef, linkName string, compensate, cancel bool) string {
	switch {
	// 错误、取消、补偿、链接依次判断，保证同一事件只显示一个图标
	case errorRef != "":
		return markerError
	case cancel:
		return markerCancel
	case compensate:
		return markerCompensate
	case linkName != "":
		return markerLink
	default:
		return ""
	}
}

// markerBox 计算内部标记的绘制区域。
// @param shape 图形
// @return 标记区域
func markerBox(shape Shape) Bounds {
	b := shape.Bounds
	if b.W <= 0 || b.H <= 0 {
		return Bounds{}
	}
	if strings.Contains(shape.Type, "Event") {
		// 事件标记绘制在圆形中央
		size := math.Min(b.W, b.H) * 0.45
		return Bounds{X: b.X + (b.W-size)/2, Y: b.Y + (b.H-size)/2, W: size, H: size}
	}
	// 任务标记绘制在左上角
	size := math.Min(math.Min(b.W, b.H)*0.4, 16)
	return Bounds{X: b.X + 6, Y: b.Y + 6, W: size, H: size}
}

// shapeLabelPoint 计算图形名称的绘制位置。
// @param shape 图形
// @return 横坐标
// @return 纵坐标
func shapeLabelPoint(shape Shape) (float64, float64) {
	b := shape.Bounds
	if shape.Label != nil && shape.Label.W > 0 {
		return shape.Label.X + shape.Label.W/2, shape.Label.Y + shape.Label.H/2
	}
	// 事件内部已有标记时，将名称移到圆形下方避免重叠
	if shape.Marker != "" && strings.Contains(shape.Type, "Event") {
		return b.X + b.W/2, b.Y + b.H + 12
	}
	return b.X + b.W/2, b.Y + b.H/2
}
