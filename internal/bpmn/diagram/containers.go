package diagram

import (
	"strings"

	"github.com/flowable-go/flowable/bpmn/model"
	bpmnxml "github.com/flowable-go/flowable/internal/bpmn/xml"
)

// Pool 表示 BPMN 中的泳池（Participant）。
type Pool struct {
	ElementID    string // 元素 ID
	Name         string // 名称
	Bounds       Bounds // 图形区域
	IsHorizontal bool   // 是否水平摆放
	ProcessRef   string // 关联的流程 ID
}

// Lane 表示 BPMN 中的泳道。
type Lane struct {
	ElementID    string   // 元素 ID
	Name         string   // 名称
	Bounds       Bounds   // 图形区域
	IsHorizontal bool     // 是否水平摆放
	PoolID       string   // 所属泳池 ID，无法确定时为空
	FlowNodeRefs []string // 泳道内节点 ID
}

// 容器类型常量。
const (
	containerParticipant = "participant" // 泳池
	containerLane        = "lane"        // 泳道
)

// containerMeta 记录泳池/泳道在 BPMN 定义中的元信息。
type containerMeta struct {
	kind         string   // 容器类型
	name         string   // 名称
	processRef   string   // 泳池关联的流程 ID
	parentID     string   // 泳道所属的流程、泳池或父泳道 ID
	flowNodeRefs []string // 泳道内节点 ID
}

// collectContainers 收集 BPMN 定义中的泳池与泳道元信息。
// @param root XML 根节点
// @return 以元素 ID 为键的元信息
func collectContainers(root *bpmnxml.Node) map[string]*containerMeta {
	metas := make(map[string]*containerMeta)
	for _, node := range root.Children {
		switch node.Name.Local {
		case "collaboration":
			for _, participant := range node.ChildrenNamed("participant") {
				id := participant.Attr("id")
				if id == "" {
					continue
				}
				metas[id] = &containerMeta{
					kind:       containerParticipant,
					name:       participant.Attr("name"),
					processRef: participant.Attr("processRef"),
				}
				// 泳池自身也可以直接包含泳道
				for _, laneSet := range participant.ChildrenNamed("laneSet") {
					collectLaneNodes(laneSet, id, metas)
				}
			}
		case "process":
			processID := node.Attr("id")
			for _, laneSet := range node.ChildrenNamed("laneSet") {
				collectLaneNodes(laneSet, processID, metas)
			}
		}
	}
	linkLanesToPool(metas)
	return metas
}

// collectLaneNodes 递归收集泳道及其子泳道。
// @param parent 泳道集合或父泳道节点
// @param parentID 父容器 ID
// @param metas 元信息集合
func collectLaneNodes(parent *bpmnxml.Node, parentID string, metas map[string]*containerMeta) {
	for _, lane := range parent.ChildrenNamed("lane") {
		id := lane.Attr("id")
		if id == "" {
			continue
		}
		meta := &containerMeta{
			kind:     containerLane,
			name:     lane.Attr("name"),
			parentID: parentID,
		}
		for _, ref := range lane.ChildrenNamed("flowNodeRef") {
			if value := strings.TrimSpace(ref.TextContent()); value != "" {
				meta.flowNodeRefs = append(meta.flowNodeRefs, value)
			}
		}
		metas[id] = meta
		// 支持嵌套子泳道
		for _, childSet := range lane.ChildrenNamed("childLaneSet") {
			collectLaneNodes(childSet, id, metas)
		}
	}
}

// linkLanesToPool 通过父容器链将泳道关联到所属泳池。
// @param metas 元信息集合
func linkLanesToPool(metas map[string]*containerMeta) {
	for _, meta := range metas {
		if meta.kind != containerLane {
			continue
		}
		meta.parentID = resolvePoolID(metas, meta.parentID)
	}
}

// resolvePoolID 沿父容器链查找泳道所属的泳池。
// @param metas 元信息集合
// @param parentID 父容器 ID
// @return 泳池 ID，无法确定时为空
func resolvePoolID(metas map[string]*containerMeta, parentID string) string {
	current := parentID
	// 限制深度，避免异常数据造成死循环
	for depth := 0; depth < 8 && current != ""; depth++ {
		parent, ok := metas[current]
		if !ok {
			// 到达流程 ID 时通过 processRef 反查泳池
			return matchPoolByProcess(metas, current)
		}
		if parent.kind == containerParticipant {
			return current
		}
		current = parent.parentID
	}
	return ""
}

// matchPoolByProcess 查找引用了指定流程的泳池 ID。
// @param metas 元信息集合
// @param processID 流程 ID
// @return 泳池 ID，未找到时为空
func matchPoolByProcess(metas map[string]*containerMeta, processID string) string {
	matched := ""
	for id, meta := range metas {
		// 多个泳池引用同一流程时取最小 ID，保证结果稳定
		if meta.kind == containerParticipant && meta.processRef == processID && (matched == "" || id < matched) {
			matched = id
		}
	}
	return matched
}

// parsePool 解析泳池图形。
// @param node BPMNShape 节点
// @param meta 容器元信息
// @param bpmn BPMN 模型
// @return 泳池
// @return 是否泳池图形
func parsePool(node *bpmnxml.Node, meta *containerMeta, bpmn *model.BpmnModel) (*Pool, bool) {
	if meta == nil || meta.kind != containerParticipant {
		return nil, false
	}
	bounds, ok := parseBounds(node)
	if !ok {
		return nil, false
	}
	elementID := node.Attr("bpmnElement")
	return &Pool{
		ElementID:    elementID,
		Name:         resolveContainerName(bpmn, meta, elementID),
		Bounds:       bounds,
		IsHorizontal: parseHorizontal(node, bounds),
		ProcessRef:   meta.processRef,
	}, true
}

// parseLane 解析泳道图形。
// @param node BPMNShape 节点
// @param meta 容器元信息
// @param bpmn BPMN 模型
// @return 泳道
// @return 是否泳道图形
func parseLane(node *bpmnxml.Node, meta *containerMeta, bpmn *model.BpmnModel) (*Lane, bool) {
	if meta == nil || meta.kind != containerLane {
		return nil, false
	}
	bounds, ok := parseBounds(node)
	if !ok {
		return nil, false
	}
	elementID := node.Attr("bpmnElement")
	return &Lane{
		ElementID:    elementID,
		Name:         resolveContainerName(bpmn, meta, elementID),
		Bounds:       bounds,
		IsHorizontal: parseHorizontal(node, bounds),
		PoolID:       meta.parentID,
		FlowNodeRefs: append([]string(nil), meta.flowNodeRefs...),
	}, true
}

// resolveContainerName 解析泳池/泳道名称。
// @param bpmn BPMN 模型
// @param meta 容器元信息
// @param elementID 元素 ID
// @return 名称，依次尝试模型、XML 定义与元素 ID
func resolveContainerName(bpmn *model.BpmnModel, meta *containerMeta, elementID string) string {
	if bpmn != nil {
		if name, _ := elementInfo(bpmn, elementID); name != "" {
			return name
		}
	}
	if meta != nil && meta.name != "" {
		return meta.name
	}
	return elementID
}

// parseHorizontal 解析 isHorizontal 属性。
// @param node BPMNShape 节点
// @param bounds 图形区域
// @return 是否水平摆放，缺省时按宽高推断
func parseHorizontal(node *bpmnxml.Node, bounds Bounds) bool {
	raw := strings.TrimSpace(node.Attr("isHorizontal"))
	switch {
	// 未显式给出时按区域形状推断
	case raw == "":
		return bounds.W >= bounds.H
	case strings.EqualFold(raw, "true") || raw == "1":
		return true
	default:
		return false
	}
}
