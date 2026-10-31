package workflow

import (
	"context"
	"sort"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 高亮域错误码（31xx 段为本文件专用）。
const (
	errCodeHighlightParam = "WORKFLOW-3101" // 高亮查询参数缺失或引擎未注入
	errCodeHighlightInst  = "WORKFLOW-3102" // 流程实例不存在（运行态与历史双查失败）
	errCodeHighlightModel = "WORKFLOW-3104" // BPMN 模型读取失败
	errCodeHighlightTask  = "WORKFLOW-3105" // 任务不存在
)

// hlSequenceFlowType 是 BPMN 顺序流的元素类型标记（对齐 bpmn/model SequenceFlow.ElementType()）。
// 历史活动里连线也会入史，但连线归属 TakenFlowIDs 单独表达，节点集合需剔除。
const hlSequenceFlowType = "sequenceFlow"

// hlTaskQuerySize 是待办任务单页查询上限（超出部分不进 TodoActivityIDs；
// 常规实例并行任务数远低于该值，页全量语义足够）。
const hlTaskQuerySize = 500

// HighlightData 是流程图高亮数据（前端按图元 ID 套色）。
// 全部列表去重并按字典序排序，保证同一实例多次查询输出稳定。
type HighlightData struct {
	CompletedActivityIDs []string // 已完成节点集合（历史活动去重，剔除 sequenceFlow 连线）
	ActiveActivityIDs    []string // 运行态活动节点集合（RuntimeService 查询，实例已结束为空）
	TakenFlowIDs         []string // 已走连线集合（按 BPMN 拓扑推断）
	TodoActivityIDs      []string // 当前待办任务节点集合（当前任务 taskDefinitionKey 去重）
}

// HighlightService 是流程图高亮数据服务（P5-B）。
// 数据来源：历史活动（完成节点）+ 运行态活动实例（当前活动）+ 当前待办任务（待办节点）
// + BPMN 模型拓扑（已走连线推断），与推演一致性回归同源（同一内存引擎语料）。
type HighlightService struct {
	engine api.Engine // 引擎门面（运行时/历史/仓库/任务四服务）
}

// NewHighlightService 创建流程图高亮域服务。
// @param engine 引擎门面（七服务）
// @return 高亮域服务
func NewHighlightService(engine api.Engine) *HighlightService {
	return &HighlightService{engine: engine}
}

// FlowHighlight 按流程实例查询高亮数据。
// 已结束实例回退全历史：ActiveActivityIDs/TodoActivityIDs 为空、CompletedActivityIDs 取全部历史节点。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID（必填）
// @return 高亮数据
// @return 错误
func (s *HighlightService) FlowHighlight(ctx context.Context, processInstanceID string) (*HighlightData, error) {
	if s.engine == nil || processInstanceID == "" {
		return nil, apiError(errCodeHighlightParam, "高亮查询缺少实例 Id 或引擎未注入")
	}
	// 定义来源与结束判定：运行态优先；SQL 引擎已结束实例运行态查不到（报错）回退历史
	definitionID, definitionKey, ended, err := s.locateInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	acts, err := s.engine.HistoryService().ListHistoricActivityInstances(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	// appeared 是"出现过于历史"的全集（含进行中节点与连线），供连线拓扑推断
	appeared := make(map[string]bool, len(acts))
	completed := make(map[string]bool, len(acts))
	for _, act := range acts {
		appeared[act.ActivityID] = true
		// 已完成 = 历史上有结束时间的节点；sequenceFlow 连线由 TakenFlowIDs 单独表达
		if act.EndTime != nil && act.ActivityType != hlSequenceFlowType {
			completed[act.ActivityID] = true
		}
	}
	data := &HighlightData{CompletedActivityIDs: hlSortedKeys(completed)}
	// 已走连线：连线两端节点都已出现于历史则视为已走（与实例是否结束无关）
	taken, err := s.takenFlows(ctx, definitionID, definitionKey, appeared)
	if err != nil {
		return nil, err
	}
	data.TakenFlowIDs = taken
	// 已结束实例回退全历史：无运行态活动与待办
	if ended {
		return data, nil
	}
	active, err := s.activeActivities(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	todo, err := s.todoActivities(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	data.ActiveActivityIDs = active
	data.TodoActivityIDs = todo
	return data, nil
}

// FlowHighlightByTask 按任务查询高亮数据（先解析任务归属实例）。
// @param ctx 操作上下文
// @param taskID 任务 ID（必填）
// @return 高亮数据
// @return 错误
func (s *HighlightService) FlowHighlightByTask(ctx context.Context, taskID string) (*HighlightData, error) {
	if s.engine == nil || taskID == "" {
		return nil, apiError(errCodeHighlightParam, "高亮查询缺少任务 Id 或引擎未注入")
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return nil, api.NewError(errCodeHighlightTask, "任务不存在: "+taskID, err)
	}
	return s.FlowHighlight(ctx, task.ProcessInstanceID)
}

// locateInstance 定位实例：返回定义 ID/Key 与是否已结束。
// 运行态查得到时按 Ended 标记判断（内存引擎已结束实例仍可查运行态）；
// 运行态查不到时回退历史实例并视为已结束，双查失败报实例不存在。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 流程定义 ID
// @return 流程定义 Key
// @return 是否已结束
// @return 错误
func (s *HighlightService) locateInstance(ctx context.Context, processInstanceID string) (string, string, bool, error) {
	runtime, runtimeErr := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if runtimeErr == nil {
		return runtime.ProcessDefinitionID, runtime.ProcessDefinitionKey, runtime.Ended, nil
	}
	historic, hisErr := s.engine.HistoryService().GetHistoricProcessInstance(ctx, processInstanceID)
	if hisErr != nil {
		return "", "", false, api.NewError(errCodeHighlightInst, "流程实例不存在（运行态与历史双查失败）: "+processInstanceID, runtimeErr)
	}
	return historic.ProcessDefinitionID, historic.ProcessDefinitionKey, true, nil
}

// activeActivities 查询运行态活动节点集合（RuntimeService.ListActivityInstances，去重排序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 去重排序后的节点 Id 列表
// @return 错误
func (s *HighlightService) activeActivities(ctx context.Context, processInstanceID string) ([]string, error) {
	acts, err := s.engine.RuntimeService().ListActivityInstances(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(acts))
	for _, act := range acts {
		// 只统计未结束的运行态活动，已结束执行不作为"当前活动"高亮
		if act.EndTime == nil {
			set[act.ActivityID] = true
		}
	}
	return hlSortedKeys(set), nil
}

// todoActivities 查询当前待办任务节点集合（TaskService.ListTasks 按 taskDefinitionKey 去重）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 去重排序后的节点 Id 列表
// @return 错误
func (s *HighlightService) todoActivities(ctx context.Context, processInstanceID string) ([]string, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: hlTaskQuerySize})
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(page.Data))
	for _, task := range page.Data {
		set[task.TaskDefinitionKey] = true
	}
	return hlSortedKeys(set), nil
}

// takenFlows 按 BPMN 拓扑推断已走连线：连线两端 activity 都已出现于历史则视为已走。
// @param ctx 操作上下文
// @param definitionID 流程定义 ID
// @param definitionKey 流程定义 Key（定位 BPMN process 用，可空回退首个流程）
// @param appeared 历史上出现过的元素 Id 集合
// @return 已走连线 Id 列表（去重排序）
// @return 错误
func (s *HighlightService) takenFlows(ctx context.Context, definitionID, definitionKey string, appeared map[string]bool) ([]string, error) {
	bpmn, err := s.engine.RepositoryService().GetBpmnModel(ctx, definitionID)
	if err != nil {
		return nil, api.NewError(errCodeHighlightModel, "BPMN 模型读取失败", err)
	}
	// 模型无流程（异常定义）时返回空集，不作为错误阻断高亮
	if len(bpmn.Processes) == 0 {
		return []string{}, nil
	}
	process := bpmn.Processes[0]
	for _, candidate := range bpmn.Processes {
		// 定义 Key 与 BPMN process id 同源（部署解析约定），优先按 Key 匹配
		if definitionKey != "" && candidate.ID == definitionKey {
			process = candidate
			break
		}
	}
	flows := make([]string, 0, 8)
	hlCollectTakenFlows(process.Elements, process.Order, appeared, &flows)
	hlSortStrings(flows)
	return flows, nil
}

// hlCollectTakenFlows 递归收集元素集中已走连线（子流程元素向下递归）。
// @param elements 元素集（按 ID 索引）
// @param order 元素文档顺序
// @param appeared 历史上出现过的元素 Id 集合
// @param out 已走连线 Id 输出
func hlCollectTakenFlows(elements map[string]model.FlowElement, order []string, appeared map[string]bool, out *[]string) {
	for _, id := range order {
		element, ok := elements[id]
		// 顺序与索引不一致（防御）时跳过
		if !ok || element == nil {
			continue
		}
		switch typed := element.(type) {
		case *model.SequenceFlow:
			// 拓扑推断：两端 activity 都已出现于历史则该连线已走
			if appeared[typed.SourceRef] && appeared[typed.TargetRef] {
				*out = append(*out, typed.GetID())
			}
		case *model.SubProcess:
			// 子流程内部的连线同样按两端出现判定
			hlCollectTakenFlows(typed.Elements, typed.Order, appeared, out)
		}
	}
}

// hlSortedKeys 返回集合的字典序键列表（输出稳定，便于断言与前端快照）。
// @param set 键集合
// @return 排序后的键列表
func hlSortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	hlSortStrings(keys)
	return keys
}

// hlSortStrings 原地字典序排序字符串列表。
// @param values 字符串列表
func hlSortStrings(values []string) {
	sort.Strings(values)
}
