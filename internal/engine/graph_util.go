package engine

import (
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 特殊网关的 _begin/_end 成对后缀约定，与 Java FlowConstant 保持逐字一致。
const (
	specialGatewayBeginSuffix = "_begin" // 并行块开始网关后缀
	specialGatewayEndSuffix   = "_end"   // 并行块汇聚网关后缀
)

// IsReachable 判断从 from 节点沿顺序流（含子流程嵌套）是否可达 to 节点（G2）。
// 对位 Java FlowUtils.isReachable(process, sourceElement, targetElement)；
// 退回场景调用方传入 (target, source)，即"目标能否走到当前节点"。
// @param process 流程
// @param from 起点
// @param to 终点
// @return 是否可达
func IsReachable(process *model.Process, from, to model.FlowNode) bool {
	return isReachableNode(process, from, to, map[string]bool{})
}

// isReachableNode IsReachable 的深度优先递归实现。
// @param process 流程
// @param source 当前节点
// @param target 目标节点
// @param visited 已访问节点集合，防环
// @return 是否可达
func isReachableNode(process *model.Process, source, target model.FlowNode, visited map[string]bool) bool {
	// 事件子流程的开始事件只是等待触发的执行，不算真实路径起点
	if _, isStart := source.(*model.StartEvent); isStart && isInEventSubProcess(process, source.GetID()) {
		return false
	}
	// 无出线：可能是流程或子流程的终点；子流程终点向上提升到子流程继续判定
	if len(source.GetOutgoing()) == 0 {
		visited[source.GetID()] = true
		parent := containerOfElement(process, source.GetID())
		if parent == nil {
			return false
		}
		// 子流程的结束节点，若目标节点仍在该子流程中，说明无法到达
		if _, exists := parent.Elements[target.GetID()]; exists {
			return false
		}
		source = parent
	}
	if source.GetID() == target.GetID() {
		return true
	}
	visited[source.GetID()] = true
	// 当前节点是子流程且目标在子流程内部，直接可达
	if sub, isSub := source.(*model.SubProcess); isSub {
		if _, exists := sub.Elements[target.GetID()]; exists {
			return true
		}
	}
	for _, flow := range source.GetOutgoing() {
		element, ok := findElement(process, flow.TargetRef)
		if !ok {
			continue
		}
		next, isNode := element.(model.FlowNode)
		if !isNode || visited[next.GetID()] {
			continue
		}
		if isReachableNode(process, next, target, visited) {
			return true
		}
	}
	return false
}

// isInEventSubProcess 判断元素是否位于事件子流程内。
// @param process 流程
// @param elementID 元素 ID
// @return 是否在事件子流程内
func isInEventSubProcess(process *model.Process, elementID string) bool {
	current := containerOfElement(process, elementID)
	for current != nil {
		if current.TriggeredByEvent {
			return true
		}
		current = containerOfElement(process, current.ID)
	}
	return false
}

// containerOfElement 返回直接包含指定元素的子流程；元素直属主流程时为 nil。
// @param process 流程
// @param elementID 元素 ID
// @return 直接容器子流程
func containerOfElement(process *model.Process, elementID string) *model.SubProcess {
	// 主流程直属元素没有子流程容器
	if _, ok := process.Elements[elementID]; ok {
		return nil
	}
	for _, id := range process.Order {
		sub, ok := process.Elements[id].(*model.SubProcess)
		if !ok {
			continue
		}
		if found := containerOfElementInSub(sub, elementID); found != nil {
			return found
		}
	}
	return nil
}

// containerOfElementInSub 在子流程树中查找直接包含元素的子流程。
// @param sub 子流程
// @param elementID 元素 ID
// @return 直接容器子流程
func containerOfElementInSub(sub *model.SubProcess, elementID string) *model.SubProcess {
	if _, ok := sub.Elements[elementID]; ok {
		return sub
	}
	for _, id := range sub.Order {
		inner, ok := sub.Elements[id].(*model.SubProcess)
		if !ok {
			continue
		}
		if found := containerOfElementInSub(inner, elementID); found != nil {
			return found
		}
	}
	return nil
}

// SpecialGatewayBlock 描述一个 _begin/_end 特殊网关块。
type SpecialGatewayBlock struct {
	GatewayID string          // 网关基础 ID（剥去 _begin 后缀）
	Elements  map[string]bool // 块内元素 ID 集合（含 _begin 与 _end）
}

// SpecialGatewayElements 识别流程及嵌套子流程中的 _begin/_end 特殊网关块（G3）。
// 对位 Java FlowUtils.getSpecialGatewayElements；返回按包含元素数降序排列（外层块在前）。
// @param process 流程
// @return 网关块列表，外层在前
func SpecialGatewayElements(process *model.Process) []SpecialGatewayBlock {
	blocks := map[string]map[string]bool{}
	collectSpecialGatewayElements(process.Elements, process.Order, blocks)
	ids := make([]string, 0, len(blocks))
	for id := range blocks {
		ids = append(ids, id)
	}
	// 外层到里层排序：包含元素多的块在外层
	sort.Slice(ids, func(i, j int) bool {
		if len(blocks[ids[i]]) != len(blocks[ids[j]]) {
			return len(blocks[ids[i]]) > len(blocks[ids[j]])
		}
		return ids[i] < ids[j]
	})
	out := make([]SpecialGatewayBlock, 0, len(ids))
	for _, id := range ids {
		out = append(out, SpecialGatewayBlock{GatewayID: id, Elements: blocks[id]})
	}
	return out
}

// collectSpecialGatewayElements 遍历容器元素递归收集特殊网关块。
// @param elements 容器元素表
// @param order 容器元素顺序
// @param blocks 收集结果
func collectSpecialGatewayElements(elements map[string]model.FlowElement, order []string, blocks map[string]map[string]bool) {
	for _, id := range order {
		element, ok := elements[id]
		if !ok {
			continue
		}
		isBeginGateway := false
		switch element.(type) {
		case *model.ParallelGateway, *model.InclusiveGateway:
			isBeginGateway = strings.HasSuffix(id, specialGatewayBeginSuffix)
		}
		if isBeginGateway {
			gatewayID := id[:len(id)-len(specialGatewayBeginSuffix)]
			set, exists := blocks[gatewayID]
			if !exists {
				set = map[string]bool{}
				blocks[gatewayID] = set
			}
			// 找不到成对 _end 的脏数据直接跳过，避免整块误收
			if _, hasEnd := elements[gatewayID+specialGatewayEndSuffix]; !hasEnd {
				delete(blocks, gatewayID)
				continue
			}
			if node, isNode := element.(model.FlowNode); isNode {
				findElementsBetweenSpecialGateway(elements, node, gatewayID+specialGatewayEndSuffix, set)
			}
			continue
		}
		if sub, isSub := element.(*model.SubProcess); isSub {
			collectSpecialGatewayElements(sub.Elements, sub.Order, blocks)
		}
	}
}

// findElementsBetweenSpecialGateway 从 _begin 网关沿出线深度优先收集块内元素（含 _end）。
// @param elements 容器元素表
// @param node 当前节点
// @param endID 汇聚网关 ID
// @param collected 收集集合
func findElementsBetweenSpecialGateway(elements map[string]model.FlowElement, node model.FlowNode, endID string, collected map[string]bool) {
	collected[node.GetID()] = true
	for _, flow := range node.GetOutgoing() {
		// 与 Java 一致：无论目标是否为 _end，先记入集合再短路
		collected[endID] = true
		if flow.TargetRef == endID {
			continue
		}
		target, ok := elements[flow.TargetRef]
		if !ok {
			continue
		}
		if next, isNode := target.(model.FlowNode); isNode {
			findElementsBetweenSpecialGateway(elements, next, endID, collected)
		}
	}
}

// parentProcessIDs 返回元素所在容器链（主流程在前），对位 Java getParentProcessIds。
// @param process 流程
// @param elementID 元素 ID
// @return 容器链，第 0 层为主流程 ID
func parentProcessIDs(process *model.Process, elementID string) []string {
	chain := []string{process.ID}
	seen := map[string]bool{process.ID: true}
	current := containerOfElement(process, elementID)
	for current != nil && !seen[current.ID] {
		chain = append(chain, current.ID)
		seen[current.ID] = true
		current = containerOfElement(process, current.ID)
	}
	return chain
}

// diffLevel 求两条容器链的首个分叉层级，对位 Java FlowUtils.getDiffLevel。
// @param sourceList 源容器链
// @param targetList 目标容器链
// @return 分叉下标；等长且全同返回 -1；一方是另一方前缀时返回浅方长度
func diffLevel(sourceList, targetList []string) int {
	// 都在第 0 层：相等即同层，否则在第 0 层分叉
	if len(sourceList) == 1 && len(targetList) == 1 {
		if sourceList[0] != targetList[0] {
			return 0
		}
		return -1
	}
	minSize := len(sourceList)
	if len(targetList) < minSize {
		minSize = len(targetList)
	}
	level := -2
	for i := 0; i < minSize; i++ {
		if sourceList[i] != targetList[i] {
			level = i
			break
		}
	}
	// 前缀全部相同：等长为同层，否则分叉点在浅方自身长度处
	if level == -2 {
		if len(sourceList) == len(targetList) {
			return -1
		}
		return minSize
	}
	return level
}

// getSourceAndTargetRealActivityID 计算退回时实际应操作的源/目标节点 ID（子流程内外映射）。
// 对位 Java FlowUtils.getSourceAndTargetRealActivityId：跨子流程时提升到分叉容器层。
// @param process 流程
// @param source 当前节点
// @param target 目标节点
// @return 实际源节点 ID
// @return 实际目标节点 ID
func getSourceAndTargetRealActivityID(process *model.Process, source, target model.FlowNode) (string, string) {
	sourceReal := source.GetID()
	targetReal := target.GetID()
	sourceParents := parentProcessIDs(process, source.GetID())
	targetParents := parentProcessIDs(process, target.GetID())
	level := diffLevel(sourceParents, targetParents)
	// 同层时保持原节点；跨层时提升到分叉容器
	if level != -1 {
		if len(sourceParents) != level {
			sourceReal = sourceParents[level]
		}
		if len(targetParents) != level {
			targetReal = targetParents[level]
		}
	}
	return sourceReal, targetReal
}
