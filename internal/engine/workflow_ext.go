package engine

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
)

// 撤回变量键与表达式常量，与 Java RollbackConstants / RollbackCmd 逐字一致。
// 下游全局监听消费 ROLLBACK_* 变量回填办理人，键名不可改动。
const (
	rollbackAssigneePrefixKey = "ROLLBACK_ASSIGNEE_PREFIX_"  // 撤回办理人变量前缀
	rollbackTaskTypePrefixKey = "ROLLBACK_TASK_TYPE_PREFIX_" // 撤回策略类型变量前缀
	rollbackAssigneeExpr      = "executor"                   // 会签单人变量名
	rollbackAssigneeListExpr  = "executorList"               // 会签集合变量名
	// rollbackPlaceholderAssignee 并行会签占位子实例的执行人标记，与 Java 一致
	rollbackPlaceholderAssignee = "已完成任务"
)

// 撤回策略类型值：取 Java 策略类 SimpleName，保证下游监听兼容。
const (
	rollbackTypeNextDefaultUserTask    = "NextDefaultUserTaskRollbackOperateStrategy"
	rollbackTypeNextGateway            = "DefaultTaskNextGatewayRollbackOperateStrategy"
	rollbackTypeNextSubProcess         = "NextSubProcessRollbackOperateStrategy"
	rollbackTypeNextCallActivity       = "NextCallActivityRollbackOperateStrategy"
	rollbackTypeCompletedMultiInstance = "CompletedMultiInstanceTaskAndNextDefaultTaskRollbackOperateStrategy"
)

// WorkflowExtService 返回 workflow 二次封装扩展服务。
// @return 扩展服务
func (e *Engine) WorkflowExtService() api.WorkflowExtService { return e }

// BackUserTaskToActivity 任意退回：把当前任务退回到目标活动，返回实际落位节点 ID。
// 对位 Java BackUserTaskCmd：可达性校验 → 子流程内外映射 + 特殊网关四分支 → 执行收集 → 迁移 → 补汇聚。
// @param ctx 上下文
// @param taskID 当前运行任务 ID
// @param targetActivityID 目标活动 ID
// @return 实际落位节点 ID
// @return 退回错误
func (e *Engine) BackUserTaskToActivity(ctx context.Context, taskID, targetActivityID string) (string, error) {
	result, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return e.backUserTaskToActivity(cc, taskID, targetActivityID)
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

// backUserTaskToActivity 在命令内执行任意退回。
// @param cc 命令上下文
// @param taskID 当前运行任务 ID
// @param targetActivityID 目标活动 ID
// @return 实际落位节点 ID
// @return 退回错误
func (e *Engine) backUserTaskToActivity(cc *commandContext, taskID, targetActivityID string) (string, error) {
	if strings.TrimSpace(targetActivityID) == "" {
		return "", api.NewError("ENGINE-0670", "目标活动不能为空", api.ErrBadRequest)
	}
	task, ok := e.findTask(taskID)
	if !ok {
		return "", api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	if err := e.allowTask(cc.ctx, task.Task.ID); err != nil {
		return "", err
	}
	instance, ok := e.findInstance(task.Task.ProcessInstanceID)
	if !ok {
		return "", api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	if instance.Ended {
		return "", api.NewError("ENGINE-0680", "流程实例已结束，无法退回", api.ErrConflict)
	}
	if instance.Suspended {
		return "", api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
	}
	definition, ok := e.findDefinition(task.Task.ProcessDefinitionID)
	if !ok {
		return "", api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return "", api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	sourceElement, ok := findElement(process, task.Task.TaskDefinitionKey)
	if !ok {
		return "", api.NewError("ENGINE-0681", "当前任务活动不存在", api.ErrNotFound)
	}
	sourceNode, isNode := sourceElement.(model.FlowNode)
	if !isNode {
		return "", api.NewError("ENGINE-0681", "当前任务活动不是流程节点", api.ErrConflict)
	}
	// 只支持从用户任务退回
	if _, isUserTask := sourceElement.(*model.UserTask); !isUserTask {
		return "", api.NewError("ENGINE-0681", "只支持从用户任务退回", api.ErrConflict)
	}
	targetElement, ok := findElement(process, targetActivityID)
	if !ok {
		return "", api.NewError("ENGINE-0404", "目标活动不存在: "+targetActivityID, api.ErrNotFound)
	}
	targetNode, isNode := targetElement.(model.FlowNode)
	if !isNode {
		return "", api.NewError("ENGINE-0682", "目标活动不是流程节点", api.ErrBadRequest)
	}
	// 目标到当前不可达时不允许退回（G2）
	if !IsReachable(process, targetNode, sourceNode) {
		return "", api.NewError("ENGINE-0683", "Cannot back to ["+targetActivityID+"]", api.ErrConflict)
	}
	sourceReal, targetReal := getSourceAndTargetRealActivityID(process, sourceNode, targetNode)
	blocks := SpecialGatewayElements(process)
	sourceIn, targetIn := specialGatewayLists(blocks, sourceReal, targetReal)

	// 四分支：确定需重算的节点集合与需要补汇聚的目标网关
	sourceRealActivityIDs := map[string]bool{}
	targetRealSpecialGateway := ""
	switch {
	case len(targetIn) == 0 && len(sourceIn) == 0:
		// 分支1：双方都不在并行块内，单点退回
		sourceRealActivityIDs[sourceReal] = true
	case len(targetIn) == 0:
		// 分支2：目标在外、当前在并行块内：收拢最外层整块的全部执行
		for id := range findBlock(blocks, sourceIn[0]) {
			sourceRealActivityIDs[id] = true
		}
	case len(sourceIn) == 0:
		// 分支3：目标在并行块内、当前在外：并行流转已结束，不允许退回
		return "", api.NewError("ENGINE-0684", "并行流转已结束，不允许退回到【"+targetNode.GetName()+"】", api.ErrConflict)
	default:
		// 分支4：双方都在并行块内
		level := diffLevel(sourceIn, targetIn)
		if level == -1 {
			// 同块同分支：单点退回
			sourceRealActivityIDs[sourceReal] = true
		} else {
			if len(sourceIn) == level {
				// 当前节点相对目标在块外层：单点退回
				sourceRealActivityIDs[sourceReal] = true
			} else {
				for id := range findBlock(blocks, sourceIn[level]) {
					sourceRealActivityIDs[id] = true
				}
			}
			// 目标相对当前在块内层：退回后需补造目标网关的汇聚执行
			if len(targetIn) != level {
				targetRealSpecialGateway = targetIn[level]
			}
		}
	}

	realExecutions := e.collectRealExecutions(task.Task.ProcessInstanceID, task.ExecutionID, sourceReal, sourceRealActivityIDs)
	if len(realExecutions) == 0 {
		return "", api.NewError("ENGINE-0685", "没有找到可退回的执行", api.ErrConflict)
	}
	if targetRealSpecialGateway != "" {
		// 目标在并行块内：不整体迁移，清场后以源执行为汇聚宿主重建重入结构
		endElement, exists := findElement(process, targetRealSpecialGateway+specialGatewayEndSuffix)
		if !exists {
			return "", api.NewError("ENGINE-0686", "汇聚网关不存在: "+targetRealSpecialGateway, nil)
		}
		endNode, endIsNode := endElement.(model.FlowNode)
		if !endIsNode {
			return "", api.NewError("ENGINE-0686", "汇聚网关不是流程节点", nil)
		}
		e.cleanupExecutions(cc, realExecutions, "back")
		host := realExecutions[0]
		host.Ended = false
		host.Active = false
		host.Waiting = false
		// 汇聚计数 = 退回重入分支 1 + 其余补造分支
		host.PendingBranches = len(endNode.GetIncoming())
		reentry := &executionState{
			ID:                e.newID(),
			ProcessInstanceID: host.ProcessInstanceID,
			DefinitionID:      host.DefinitionID,
			ParentID:          host.ID,
			ActivityID:        targetReal,
			Active:            true,
			Concurrent:        true,
			Variables:         make(map[string]any),
		}
		e.state.Executions[reentry.ID] = reentry
		cc.agenda.schedule(&continueOperation{executionID: reentry.ID, activityID: targetReal})
		if err := e.createJoinBranchExecutions(cc, host, endNode, 1); err != nil {
			return "", err
		}
	} else {
		// 常规迁移：多条执行合并到目标活动（G1）
		if _, err := e.moveExecutionsToSingleActivity(cc, realExecutions, targetReal, "back"); err != nil {
			return "", err
		}
	}
	// G7 历史清理（P2-B 增补）：对位 Java generalBack L2594-2604——退回命令内删除
	// "目标及其后继节点"的运行/历史活动行。Go 运行态活动行由迁移清场覆盖，
	// 此处补删历史活动行；新落位目标的活动行在 agenda.run 中创建，位于删除之后，
	// 不会被误删（蓝图 §5.3 风险 4：清理必须留在同一命令内保证原子回滚）。
	e.deleteBackHistoricActivities(process, targetReal, task.Task.ProcessInstanceID)
	if err := cc.agenda.run(cc); err != nil {
		return "", err
	}
	return targetReal, nil
}

// backSuccessorActivityIDs 枚举目标活动沿出线可达的全部后继元素 ID（不含目标本身，防环）。
// 对位 Java FlowUtils.getNextActivitys 的图遍历：沿 outgoing 深度优先收集；
// 命中子流程容器时展开其内部全部元素 ID（历史活动行以内部元素 ID 落库）。
// @param process 流程模型
// @param targetID 目标活动 ID
// @return 后继元素 ID 集合
func backSuccessorActivityIDs(process *model.Process, targetID string) map[string]bool {
	collected := map[string]bool{}
	var visit func(id string)
	visit = func(id string) {
		// 已收集的元素跳过（环路与汇聚防止重复展开）
		if collected[id] {
			return
		}
		element, ok := findElement(process, id)
		if !ok {
			return
		}
		collected[id] = true
		// 子流程容器：内部元素的历史活动行随容器一并清理（嵌套子流程递归展开）
		if sub, isSub := element.(*model.SubProcess); isSub {
			for innerID := range sub.Elements {
				visit(innerID)
			}
		}
		if node, isNode := element.(model.FlowNode); isNode {
			for _, flow := range node.GetOutgoing() {
				visit(flow.TargetRef)
			}
		}
	}
	visit(targetID)
	// 目标本身由调用方并入删除集合，这里移除，仅保留"目标之后"语义
	delete(collected, targetID)
	return collected
}

// deleteBackHistoricActivities 退回历史清理（G7 活动侧）：
// 删除"目标及其后继节点"的全部历史活动行，对位 Java generalBack 的
// workflowHisActinstMapper.deleteHisActinstsByProcessInstanceIdAndActIds
// （按活动 ID 全量删除、不设时间窗，与 Java 一致）。
// 历史任务行（ACT_HI_TASKINST 等价存储）与 Java 一致保留，不在此清理。
// @param process 流程模型
// @param targetActivityID 实际落位目标活动 ID
// @param processInstanceID 实例 ID
func (e *Engine) deleteBackHistoricActivities(process *model.Process, targetActivityID, processInstanceID string) {
	successors := backSuccessorActivityIDs(process, targetActivityID)
	activityIDs := make(map[string]bool, len(successors)+1)
	for id := range successors {
		activityIDs[id] = true
	}
	// 目标本身并入删除集合（Java actIds.add(distFlowElementId)）
	activityIDs[targetActivityID] = true
	// 零时间下界：删除集合内活动 ID 的全部历史行（Java 无时间过滤语义）
	_ = e.deleteHistoricActivitiesByProcessInstanceAndActivityIDs(processInstanceID, activityIDs, time.Time{}, "")
}

// specialGatewayLists 计算源/目标节点分别落在哪些特殊网关块内（外层在前）。
// @param blocks 网关块列表
// @param sourceID 源节点 ID
// @param targetID 目标节点 ID
// @return 源所在网关 ID 列表
// @return 目标所在网关 ID 列表
func specialGatewayLists(blocks []SpecialGatewayBlock, sourceID, targetID string) ([]string, []string) {
	sourceIn := make([]string, 0)
	targetIn := make([]string, 0)
	for _, block := range blocks {
		if block.Elements[sourceID] {
			sourceIn = append(sourceIn, block.GatewayID)
		}
		if block.Elements[targetID] {
			targetIn = append(targetIn, block.GatewayID)
		}
	}
	return sourceIn, targetIn
}

// findBlock 返回指定网关 ID 的块内元素集合，不存在时返回空集合。
// @param blocks 网关块列表
// @param gatewayID 网关 ID
// @return 块内元素集合
func findBlock(blocks []SpecialGatewayBlock, gatewayID string) map[string]bool {
	for _, block := range blocks {
		if block.GatewayID == gatewayID {
			return block.Elements
		}
	}
	return map[string]bool{}
}

// collectRealExecutions 收集退回需重算的执行（G8 + 多实例 root 归并）。
// 对位 Java BackUserTaskCmd.getRealExecutions：定位真实父执行后取同父下落在节点集合内的全部执行。
// @param processInstanceID 实例 ID
// @param taskExecutionID 当前任务的执行 ID
// @param sourceReal 实际源节点 ID
// @param sourceRealActivityIDs 需重算的节点集合
// @return 执行列表（按 ID 排序）
func (e *Engine) collectRealExecutions(processInstanceID, taskExecutionID, sourceReal string, sourceRealActivityIDs map[string]bool) []*executionState {
	all := make([]*executionState, 0)
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID == processInstanceID && !execution.Ended {
			all = append(all, execution)
		}
	}
	parentIDs := e.getParentExecutionIDsByActivityID(all, sourceReal)
	if len(parentIDs) == 0 {
		return nil
	}
	taskExec, ok := e.findExecution(taskExecutionID)
	if !ok {
		return nil
	}
	// 沿父链上溯找第一个命中父集合的祖先
	realParentID := ""
	current, ok := e.findExecution(taskExec.ParentID)
	for ok && current != nil {
		if parentIDs[current.ID] {
			realParentID = current.ID
			break
		}
		if current.ParentID == "" {
			break
		}
		current, ok = e.findExecution(current.ParentID)
	}
	if realParentID == "" {
		return nil
	}
	out := make([]*executionState, 0)
	for _, execution := range all {
		if execution.ParentID == realParentID && sourceRealActivityIDs[execution.ActivityID] {
			out = append(out, execution)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// RollbackTask 撤回：把已完成的当前任务撤回到待办状态，删除其后继任务与历史。
// 对位 Java RollbackCmd + DefaultRollbackStrategyFactoryBean 策略族：工厂判定后直调内部函数。
// @param ctx 上下文
// @param taskID 已完成任务的 ID（历史任务 ID）
// @param assignee 撤回后任务的办理人，空串取历史任务办理人
// @return 撤回错误
func (e *Engine) RollbackTask(ctx context.Context, taskID, assignee string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.rollbackTask(cc, taskID, assignee)
	})
	return err
}

// rollbackTask 在命令内执行撤回：校验 → 工厂判定 → 策略内部函数族。
// @param cc 命令上下文
// @param taskID 已完成任务的 ID
// @param assignee 撤回后任务的办理人
// @return 撤回错误
func (e *Engine) rollbackTask(cc *commandContext, taskID, assignee string) error {
	if strings.TrimSpace(taskID) == "" {
		return api.NewError("ENGINE-0690", "撤回任务Id不存在", api.ErrBadRequest)
	}
	hisTask, ok := e.state.HistoricTasks[taskID]
	if !ok {
		return api.NewError("ENGINE-0690", "撤回任务不存在", api.ErrNotFound)
	}
	// 正在执行的任务不能撤回
	if hisTask.EndTime == nil {
		return api.NewError("ENGINE-0691", "任务正在执行，无法撤回", api.ErrConflict)
	}
	if assignee == "" {
		assignee = hisTask.Assignee
	}
	if assignee == "" {
		return api.NewError("ENGINE-0692", "撤回任务办理人不存在", api.ErrConflict)
	}
	instance, ok := e.findInstance(hisTask.ProcessInstanceID)
	if !ok || instance.Ended {
		return api.NewError("ENGINE-0693", "该流程已完成，无法进行任务撤回", api.ErrConflict)
	}
	if instance.Suspended {
		return api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
	}
	if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
		return err
	}
	definition, ok := e.findDefinition(hisTask.ProcessDefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	params := &rollbackParams{hisTask: hisTask, process: process}
	// 定位撤回任务节点（先主流程后子流程）
	params.currentTaskElement, params.currentSubProcess = rollbackFindUserTask(process, hisTask.TaskDefinitionKey)
	if params.currentTaskElement == nil {
		return api.NewError("ENGINE-0694", "没有获取撤回任务节点", api.ErrNotFound)
	}
	rollbackCollectNextElements(params)
	loop := params.currentTaskElement.LoopCharacteristics
	// 工厂判定（顺序即 Java createStrategyInstance 的优先级）
	var strategyErr error
	switch {
	case e.rollbackCurrentMultiInstanceUnfinished(params):
		strategyErr = e.rollbackActiveMultiInstance(cc, params, assignee)
	case loop == nil && len(params.gatewayMap) == 0 && len(params.nextUserTaskList) > 0:
		strategyErr = e.rollbackNextDefaultUserTask(cc, params, assignee)
	case loop == nil && len(params.gatewayMap) == 0 && len(params.subProcessMap) > 0:
		strategyErr = e.rollbackNextSubProcess(cc, params, assignee)
	case loop == nil && len(params.gatewayMap) == 0 && len(params.callActivityMap) > 0:
		strategyErr = e.rollbackNextCallActivity(cc, params, assignee)
	case loop == nil && len(params.gatewayMap) > 0:
		strategyErr = e.rollbackNextGateway(cc, params, assignee)
	case loop != nil && len(params.gatewayMap) == 0 && len(params.nextUserTaskList) > 0:
		strategyErr = e.rollbackCompletedMultiInstance(cc, params, assignee)
	default:
		return api.NewError("ENGINE-0695", "当前流程节点不支持撤回", api.ErrConflict)
	}
	if strategyErr != nil {
		return strategyErr
	}
	// 撤回重建的任务与其余调度操作统一在命令内驱动
	return cc.agenda.run(cc)
}

// rollbackParams 是撤回策略共用的参数模板，对位 Java RollbackParamsTemplate。
type rollbackParams struct {
	hisTask            *api.HistoricTaskInstance      // 撤回的历史任务
	process            *model.Process                 // 流程模型
	currentTaskElement *model.UserTask                // 撤回任务节点
	currentSubProcess  *model.SubProcess              // 撤回任务所在子流程，主流程时为 nil
	nextFlowIDList     map[string]bool                // 后继连线 ID + 节点 ID 集合
	nextUserTaskList   []*model.UserTask              // 后继用户任务
	gatewayMap         map[string]model.FlowNode      // 后继网关（多级网关递归展开）
	subProcessMap      map[string]*model.SubProcess   // 后继嵌入式子流程
	callActivityMap    map[string]*model.CallActivity // 后继调用活动
}

// rollbackFindUserTask 按任务 Key 在主流程与子流程中查找撤回任务节点。
// @param process 流程
// @param key 任务定义 Key
// @return 用户任务节点
// @return 所在子流程，主流程时为 nil
func rollbackFindUserTask(process *model.Process, key string) (*model.UserTask, *model.SubProcess) {
	if element, ok := process.Elements[key]; ok {
		if userTask, isTask := element.(*model.UserTask); isTask {
			return userTask, nil
		}
	}
	return rollbackFindUserTaskInContainer(process.Elements, process.Order, key)
}

// rollbackFindUserTaskInContainer 在容器树中递归查找撤回任务节点。
// @param elements 容器元素表
// @param order 容器元素顺序
// @param key 任务定义 Key
// @return 用户任务节点
// @return 直接容器子流程
func rollbackFindUserTaskInContainer(elements map[string]model.FlowElement, order []string, key string) (*model.UserTask, *model.SubProcess) {
	for _, id := range order {
		element, ok := elements[id]
		if !ok {
			continue
		}
		if sub, isSub := element.(*model.SubProcess); isSub {
			if userTask, inner := rollbackFindUserTaskInContainer(sub.Elements, sub.Order, key); userTask != nil {
				// 直接命中的子流程为直接容器，更深层返回内层容器
				if inner != nil {
					return userTask, inner
				}
				return userTask, sub
			}
			continue
		}
		if id == key {
			if userTask, isTask := element.(*model.UserTask); isTask {
				return userTask, nil
			}
		}
	}
	return nil, nil
}

// rollbackCollectNextElements 沿撤回任务出线递归收集后继连线、节点、网关、子流程与调用活动。
// 对位 Java DefaultRollbackStrategyFactoryBean.getNextElementInfo。
// @param params 撤回参数
func rollbackCollectNextElements(params *rollbackParams) {
	params.nextFlowIDList = map[string]bool{}
	params.nextUserTaskList = nil
	params.gatewayMap = map[string]model.FlowNode{}
	params.subProcessMap = map[string]*model.SubProcess{}
	params.callActivityMap = map[string]*model.CallActivity{}
	// 当前任务在子流程内时沿子流程容器展开
	elements := params.process.Elements
	if params.currentSubProcess != nil {
		elements = params.currentSubProcess.Elements
	}
	rollbackCollectOutgoing(params, elements, params.currentTaskElement.GetOutgoing())
}

// rollbackCollectOutgoing 递归收集一组出线的后继元素。
// @param params 撤回参数
// @param elements 容器元素表
// @param flows 出线列表
func rollbackCollectOutgoing(params *rollbackParams, elements map[string]model.FlowElement, flows []*model.SequenceFlow) {
	for _, flow := range flows {
		params.nextFlowIDList[flow.ID] = true
		target, ok := elements[flow.TargetRef]
		if !ok {
			continue
		}
		params.nextFlowIDList[target.GetID()] = true
		switch typed := target.(type) {
		case *model.UserTask:
			params.nextUserTaskList = append(params.nextUserTaskList, typed)
		case *model.ParallelGateway, *model.InclusiveGateway, *model.ExclusiveGateway, *model.EventGateway:
			if node, isNode := target.(model.FlowNode); isNode {
				params.gatewayMap[target.GetID()] = node
				// 多级网关递归展开
				rollbackCollectOutgoing(params, elements, node.GetOutgoing())
			}
		case *model.SubProcess:
			params.subProcessMap[typed.ID] = typed
		case *model.CallActivity:
			params.callActivityMap[typed.ID] = typed
		}
	}
}

// rollbackCurrentMultiInstanceUnfinished 判断撤回任务节点是否为仍有运行中任务的会签节点。
// 对位 Java currentMultiInstanceTaskUnfinished。
// @param params 撤回参数
// @return 是否存在未完成会签任务
func (e *Engine) rollbackCurrentMultiInstanceUnfinished(params *rollbackParams) bool {
	if params.currentTaskElement.LoopCharacteristics == nil {
		return false
	}
	for _, task := range e.state.Tasks {
		if task.Task.ProcessInstanceID == params.hisTask.ProcessInstanceID && task.Task.TaskDefinitionKey == params.hisTask.TaskDefinitionKey {
			return true
		}
	}
	return false
}

// rollbackExistNextFinishedTask 防护：后继节点已出现更晚完成的任务时拒绝撤回。
// 对位 Java AbstractRollbackOperateStrategy.existNextFinishedTask。
// @param params 撤回参数
// @param keys 需检查的任务定义 Key 集合
// @return 检查错误
func (e *Engine) rollbackExistNextFinishedTask(params *rollbackParams, keys map[string]bool) error {
	for _, historic := range e.state.HistoricTasks {
		if historic.ProcessInstanceID != params.hisTask.ProcessInstanceID || historic.EndTime == nil {
			continue
		}
		// 只拦撤回时间点之后完成的任务
		if !historic.EndTime.After(*params.hisTask.EndTime) {
			continue
		}
		if keys[historic.TaskDefinitionKey] {
			return api.NewError("ENGINE-0696", "任务已被处理，无法进行流程节点撤回", api.ErrConflict)
		}
	}
	return nil
}

// rollbackSetAssigneeVars 构造撤回办理人与策略类型变量（键与 Java 逐字一致）。
// @param params 撤回参数
// @param assignee 撤回办理人
// @param strategyType 策略类型值，空串表示不写类型变量
// @return 变量映射
func rollbackSetAssigneeVars(params *rollbackParams, assignee, strategyType string) map[string]any {
	variables := map[string]any{}
	variables[rollbackAssigneePrefixKey+params.hisTask.ProcessInstanceID+params.hisTask.TaskDefinitionKey] = assignee
	if strategyType != "" {
		variables[rollbackTaskTypePrefixKey+params.hisTask.ProcessInstanceID+params.hisTask.TaskDefinitionKey] = strategyType
	}
	return variables
}

// rollbackGetExecutionEntity 定位撤回基准执行（对位 Java getExecutionEntity）：
// 优先历史任务的执行，失活时按后继节点集合兜底取现存执行。
// @param params 撤回参数
// @return 基准执行
// @return 定位错误
func (e *Engine) rollbackGetExecutionEntity(params *rollbackParams) (*executionState, error) {
	if execution, ok := e.findExecution(params.hisTask.ExecutionID); ok && !execution.Ended {
		return execution, nil
	}
	candidates := e.findActiveExecutionsByActivityIDs(params.hisTask.ProcessInstanceID, params.nextFlowIDList)
	if len(candidates) == 0 {
		return nil, api.NewError("ENGINE-0697", "没有找到可撤回的任务", api.ErrConflict)
	}
	return candidates[0], nil
}

// rollbackCreateExecution 创建撤回重做执行（对位 Java createExecution 模板）：
// 基准执行的父作用域下新子执行落到当前任务节点，ROLLBACK_* 变量落为子执行局部变量并调度建任务。
// @param cc 命令上下文
// @param params 撤回参数
// @param variables 撤回变量
// @return 新执行
// @return 创建错误
func (e *Engine) rollbackCreateExecution(cc *commandContext, params *rollbackParams, variables map[string]any) (*executionState, error) {
	execution, err := e.rollbackGetExecutionEntity(params)
	if err != nil {
		return nil, err
	}
	newExecution := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: execution.ProcessInstanceID,
		DefinitionID:      execution.DefinitionID,
		ParentID:          execution.ParentID,
		ActivityID:        params.currentTaskElement.ID,
		Active:            true,
		Variables:         cloneAnyMap(variables),
	}
	// 基准执行为子流程作用域根时（撤回任务位于子流程内），重做执行继承作用域根语义，
	// 保证子流程内重走完成后能正确恢复父执行
	newExecution.ScopeRoot = execution.ScopeRoot
	if newExecution.Variables == nil {
		newExecution.Variables = make(map[string]any)
	}
	e.state.Executions[newExecution.ID] = newExecution
	for name, value := range newExecution.Variables {
		e.recordHistoricVariable(newExecution, name, value)
	}
	cc.agenda.schedule(&continueOperation{executionID: newExecution.ID, activityID: params.currentTaskElement.ID})
	return newExecution, nil
}

// rollbackRemoveRuntimeTask 移除运行中任务及其执行（G9，对位 Java removeRuntimeTaskOperate）：
// 删身份链接、任务变量、任务行，并以静默结束整棵执行子树等价 Java 的执行删除。
// @param cc 命令上下文
// @param task 运行中任务
func (e *Engine) rollbackRemoveRuntimeTask(cc *commandContext, task *taskState) {
	e.removeHistoricTaskAssociations(task.Task.ID)
	e.recordTaskLog(cc, task, "delete", actorFromContext(cc.ctx), "rollback")
	e.recordTaskEnd(cc, task)
	delete(e.state.Tasks, task.Task.ID)
	e.endExecutionSubtreeQuietly(cc, task.ExecutionID)
}

// deleteTaskRow 只删除任务行，不结束其执行（汇聚宿主保活用）。
// @param cc 命令上下文
// @param task 运行中任务
func (e *Engine) deleteTaskRow(cc *commandContext, task *taskState) {
	e.removeHistoricTaskAssociations(task.Task.ID)
	e.recordTaskLog(cc, task, "delete", actorFromContext(cc.ctx), "rollback")
	e.recordTaskEnd(cc, task)
	delete(e.state.Tasks, task.Task.ID)
}

// endExecutionSubtreeQuietly 静默结束执行子树：清任务/定时器/订阅/活动历史并结束后代，不改父作用域计数。
// @param cc 命令上下文
// @param executionID 执行 ID
func (e *Engine) endExecutionSubtreeQuietly(cc *commandContext, executionID string) {
	execution, ok := e.findExecution(executionID)
	if !ok || execution.Ended {
		return
	}
	for _, current := range e.descendantsOf(execution) {
		e.cancelExecutionTasks(cc, current.ID, "rollback")
		e.cancelBoundaryTimers(current.ID)
		e.removeSubscriptionsForExecution(current.ID)
		e.recordActivityEnd(cc, current.CurrentHistActivityID)
		current.CurrentHistActivityID = ""
		current.Ended = true
		current.Active = false
		current.Waiting = false
	}
}

// rollbackDeleteRuntimeTasks 删除后继节点的运行中任务与历史任务（G9 + G7 任务侧）。
// @param cc 命令上下文
// @param params 撤回参数
func (e *Engine) rollbackDeleteRuntimeTasks(cc *commandContext, params *rollbackParams) {
	for _, task := range e.findTasksByActivityKeys(params.hisTask.ProcessInstanceID, params.nextFlowIDList) {
		e.rollbackRemoveRuntimeTask(cc, task)
	}
	e.deleteHistoricTasksByDefinitionKeys(params.hisTask.ProcessInstanceID, params.nextFlowIDList, nil)
	e.pruneHangingForkParents(params.hisTask.ProcessInstanceID)
}

// rollbackDeleteRuntimeTasksAfter 网关策略变体：仅删除撤回时间点之后创建的后继任务。
// 对位 Java DefaultTaskNextGatewayRollbackOperateStrategy.deleteRuntimeTasks 的 taskCreatedAfter 过滤。
// @param cc 命令上下文
// @param params 撤回参数
func (e *Engine) rollbackDeleteRuntimeTasksAfter(cc *commandContext, params *rollbackParams) {
	for _, task := range e.findTasksByActivityKeys(params.hisTask.ProcessInstanceID, params.nextFlowIDList) {
		// 网关策略只撤回撤回时间点之后生成的任务
		if task.Task.CreateTime.After(*params.hisTask.EndTime) || task.Task.CreateTime.Equal(*params.hisTask.EndTime) {
			e.rollbackRemoveRuntimeTask(cc, task)
		}
	}
	e.deleteHistoricTasksByDefinitionKeys(params.hisTask.ProcessInstanceID, params.nextFlowIDList, params.hisTask.EndTime)
	e.pruneHangingForkParents(params.hisTask.ProcessInstanceID)
}

// rollbackDeleteHisTaskAndActs 删除撤回产生的历史活动行与当前历史任务（G7）。
// 对位 Java deleteHisActInstance + removeHisTask。
// @param cc 命令上下文
// @param params 撤回参数
// @param activityIDs 需删除的活动 ID 集合
func (e *Engine) rollbackDeleteHisTaskAndActs(cc *commandContext, params *rollbackParams, activityIDs map[string]bool) {
	// Go 历史任务的创建时间即 StartTime（对位 Java hisTask.getCreateTime()）
	e.deleteHistoricActivitiesByProcessInstanceAndActivityIDs(params.hisTask.ProcessInstanceID, activityIDs, params.hisTask.StartTime, params.hisTask.TaskDefinitionKey)
	e.removeHistoricTaskAssociations(params.hisTask.ID)
	delete(e.state.HistoricTasks, params.hisTask.ID)
}

// removeHisTaskRow 仅移除撤回历史任务行（会签策略用，活动历史保留）。
// @param hisTask 撤回的历史任务
func (e *Engine) removeHisTaskRow(hisTask *api.HistoricTaskInstance) {
	e.removeHistoricTaskAssociations(hisTask.ID)
	delete(e.state.HistoricTasks, hisTask.ID)
}

// pruneHangingForkParents 清理再无活动后代的悬挂并行父执行（对位 Java 删除残留网关 execution）。
// 悬挂判定：等待分支数大于 0、自身不活跃不等待、子树内已无未结束后代。
// @param processInstanceID 实例 ID
func (e *Engine) pruneHangingForkParents(processInstanceID string) {
	// 级联清理：父执行悬挂结束后可能让祖父执行也悬挂
	changed := true
	for changed {
		changed = false
		for _, execution := range e.state.Executions {
			if execution.ProcessInstanceID != processInstanceID || execution.Ended {
				continue
			}
			if execution.PendingBranches <= 0 || execution.Active || execution.Waiting {
				continue
			}
			if e.hasUnendedDescendant(execution) {
				continue
			}
			execution.Ended = true
			changed = true
		}
	}
}

// hasUnendedDescendant 判断执行子树内是否仍有未结束的后代。
// @param root 根执行
// @return 是否存在未结束后代
func (e *Engine) hasUnendedDescendant(root *executionState) bool {
	for _, current := range e.descendantsOf(root) {
		if current.ID != root.ID && !current.Ended {
			return true
		}
	}
	return false
}

// rollbackNextDefaultUserTask 普通节点撤回策略（对位 NextDefaultUserTaskRollbackOperateStrategy）。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @return 撤回错误
func (e *Engine) rollbackNextDefaultUserTask(cc *commandContext, params *rollbackParams, assignee string) error {
	if err := e.rollbackExistNextFinishedTask(params, params.nextFlowIDList); err != nil {
		return err
	}
	variables := rollbackSetAssigneeVars(params, assignee, rollbackTypeNextDefaultUserTask)
	if _, err := e.rollbackCreateExecution(cc, params, variables); err != nil {
		return err
	}
	e.rollbackDeleteRuntimeTasks(cc, params)
	e.rollbackDeleteHisTaskAndActs(cc, params, params.nextFlowIDList)
	return nil
}

// rollbackNextGateway 网关撤回策略（对位 DefaultTaskNextGatewayRollbackOperateStrategy）：
// 重建撤回任务后按网关双态处理（汇聚宿主保活 + 其余分支伪造到达 / 删残留网关执行）。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @return 撤回错误
func (e *Engine) rollbackNextGateway(cc *commandContext, params *rollbackParams, assignee string) error {
	if err := e.rollbackExistNextFinishedTask(params, params.nextFlowIDList); err != nil {
		return err
	}
	base, err := e.rollbackGetExecutionEntity(params)
	if err != nil {
		return err
	}
	variables := rollbackSetAssigneeVars(params, assignee, rollbackTypeNextGateway)
	// 其余入线非空时基准执行保留为汇聚接收者，重做执行须建在其子层
	sqFlows := gatewayJoinFlows(params)
	parentID := base.ParentID
	if len(sqFlows) > 0 {
		parentID = base.ID
	}
	newExecution := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: base.ProcessInstanceID,
		DefinitionID:      base.DefinitionID,
		ParentID:          parentID,
		ActivityID:        params.currentTaskElement.ID,
		Active:            true,
		Variables:         cloneAnyMap(variables),
	}
	// 与模板撤回一致：基准执行为子流程作用域根时继承作用域根语义
	newExecution.ScopeRoot = base.ScopeRoot
	if newExecution.Variables == nil {
		newExecution.Variables = make(map[string]any)
	}
	e.state.Executions[newExecution.ID] = newExecution
	for name, value := range newExecution.Variables {
		e.recordHistoricVariable(newExecution, name, value)
	}
	cc.agenda.schedule(&continueOperation{executionID: newExecution.ID, activityID: params.currentTaskElement.ID})
	if err := e.rollbackProcessGateway(cc, params, base, sqFlows); err != nil {
		return err
	}
	e.rollbackDeleteRuntimeTasksAfter(cc, params)
	e.rollbackDeleteHisTaskAndActs(cc, params, params.nextFlowIDList)
	return nil
}

// gatewayJoinFlows 收集网关的其余入线来源：跳过网关间连线与当前撤回任务的连线。
// 对位 Java processGateway 的 sqFlowMap 收集。
// @param params 撤回参数
// @return 源节点 ID 到入线的映射
func gatewayJoinFlows(params *rollbackParams) map[string]*model.SequenceFlow {
	sqFlows := map[string]*model.SequenceFlow{}
	for _, gateway := range params.gatewayMap {
		for _, flow := range gateway.GetIncoming() {
			// 跳过网关间连线
			if _, isGateway := params.gatewayMap[flow.SourceRef]; isGateway {
				continue
			}
			// 跳过当前撤回历史任务的连线
			if flow.SourceRef == params.hisTask.TaskDefinitionKey {
				continue
			}
			sqFlows[flow.SourceRef] = flow
		}
	}
	return sqFlows
}

// rollbackProcessGateway 网关撤回的双态处理（对位 Java processGateway）。
// 网关后任务已生成：其余入线非空时以基准执行为汇聚宿主补造"其余分支已到齐"的到达，并删除网关后任务；
// 任务未生成：仅删除残留的网关执行。
// @param cc 命令上下文
// @param params 撤回参数
// @param base 撤回基准执行
// @param sqFlows 网关其余入线来源
// @return 处理错误
func (e *Engine) rollbackProcessGateway(cc *commandContext, params *rollbackParams, base *executionState, sqFlows map[string]*model.SequenceFlow) error {
	nextTasks := e.findTasksByActivityKeys(params.hisTask.ProcessInstanceID, params.nextFlowIDList)
	if len(nextTasks) == 0 {
		// 任务未生成：删除残留网关执行
		gatewayIDs := map[string]bool{}
		for id := range params.gatewayMap {
			gatewayIDs[id] = true
		}
		for _, execution := range e.findActiveExecutionsByActivityIDs(params.hisTask.ProcessInstanceID, gatewayIDs) {
			e.endExecutionSubtreeQuietly(cc, execution.ID)
		}
		return nil
	}
	hostMode := len(sqFlows) > 0
	if hostMode {
		// 汇聚宿主：预留重做分支到达 + 其余伪造分支到达
		base.Waiting = false
		base.Active = false
		base.PendingBranches = len(sqFlows) + 1
		for _, flow := range sqFlows {
			child := &executionState{
				ID:                e.newID(),
				ProcessInstanceID: base.ProcessInstanceID,
				DefinitionID:      base.DefinitionID,
				ParentID:          base.ID,
				ActivityID:        flow.TargetRef,
				Active:            true,
				Concurrent:        true,
				Variables:         make(map[string]any),
			}
			e.state.Executions[child.ID] = child
			cc.agenda.schedule(&continueOperation{executionID: child.ID, activityID: flow.TargetRef})
		}
	}
	removedHistoricIDs := map[string]bool{}
	for _, task := range nextTasks {
		if hostMode && task.ExecutionID == base.ID {
			// 宿主自身承载任务：只删任务行，执行留作汇聚接收者
			e.deleteTaskRow(cc, task)
		} else {
			e.rollbackRemoveRuntimeTask(cc, task)
		}
		removedHistoricIDs[task.Task.ID] = true
	}
	for id := range e.state.HistoricTasks {
		if removedHistoricIDs[id] {
			e.removeHistoricTaskAssociations(id)
			delete(e.state.HistoricTasks, id)
		}
	}
	return nil
}

// rollbackNextSubProcess 嵌入式子流程撤回策略（对位 NextSubProcessRollbackOperateStrategy）。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @return 撤回错误
func (e *Engine) rollbackNextSubProcess(cc *commandContext, params *rollbackParams, assignee string) error {
	sub := firstSubProcess(params.subProcessMap)
	subKeys := map[string]bool{}
	collectSubElementIDs(sub, subKeys)
	// 子流程内部已出现完成任务即拒绝撤回
	if err := e.rollbackExistNextFinishedTask(params, subKeys); err != nil {
		return err
	}
	variables := rollbackSetAssigneeVars(params, assignee, rollbackTypeNextSubProcess)
	// 子流程作为后继时基准执行可能已关闭，按后继节点集合兜底
	if _, err := e.rollbackCreateExecution(cc, params, variables); err != nil {
		return err
	}
	// 删除子流程内运行任务
	for _, task := range e.findTasksByActivityKeys(params.hisTask.ProcessInstanceID, subKeys) {
		e.rollbackRemoveRuntimeTask(cc, task)
	}
	// 删除子流程层级执行（级联子节点随子树静默结束）
	subExecutionIDs := map[string]bool{sub.ID: true}
	for _, execution := range e.findActiveExecutionsByActivityIDs(params.hisTask.ProcessInstanceID, subExecutionIDs) {
		e.endExecutionSubtreeQuietly(cc, execution.ID)
	}
	e.deleteHistoricTasksByDefinitionKeys(params.hisTask.ProcessInstanceID, subKeys, nil)
	e.pruneHangingForkParents(params.hisTask.ProcessInstanceID)
	e.rollbackDeleteHisTaskAndActs(cc, params, subKeys)
	return nil
}

// firstSubProcess 取映射中的首个子流程（Java 侧同样只处理单个）。
// @param subProcesses 子流程映射
// @return 子流程，可能为 nil
func firstSubProcess(subProcesses map[string]*model.SubProcess) *model.SubProcess {
	for _, sub := range subProcesses {
		return sub
	}
	return nil
}

// collectSubElementIDs 递归收集子流程内部全部元素 ID。
// @param sub 子流程
// @param out 收集集合
func collectSubElementIDs(sub *model.SubProcess, out map[string]bool) {
	if sub == nil {
		return
	}
	for id, element := range sub.Elements {
		out[id] = true
		if inner, isSub := element.(*model.SubProcess); isSub {
			collectSubElementIDs(inner, out)
		}
	}
}

// rollbackNextCallActivity 调用活动撤回策略（对位 NextCallActivityRollbackOperateStrategy）。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @return 撤回错误
func (e *Engine) rollbackNextCallActivity(cc *commandContext, params *rollbackParams, assignee string) error {
	callActivity := firstCallActivity(params.callActivityMap)
	callExec := e.findCallActivityExecution(params.hisTask.ProcessInstanceID, callActivity)
	if callExec != nil {
		child := e.childInstanceOf(callExec.ID)
		if child != nil {
			// 子流程已产生完成任务即拒绝撤回
			for _, historic := range e.state.HistoricTasks {
				if historic.ProcessInstanceID == child.ID && historic.EndTime != nil {
					return api.NewError("ENGINE-0698", "子流程已经具有完成的任务，流程无法撤回", api.ErrConflict)
				}
			}
		}
	} else {
		// 调用活动未启动时退回默认防护
		if err := e.rollbackExistNextFinishedTask(params, params.nextFlowIDList); err != nil {
			return err
		}
	}
	variables := rollbackSetAssigneeVars(params, assignee, rollbackTypeNextCallActivity)
	if _, err := e.rollbackCreateExecution(cc, params, variables); err != nil {
		return err
	}
	if callExec != nil {
		if child := e.childInstanceOf(callExec.ID); child != nil {
			e.destroyChildProcessInstance(cc, child.ID)
		}
		// 删除父侧调用活动执行
		e.endExecutionSubtreeQuietly(cc, callExec.ID)
	}
	e.pruneHangingForkParents(params.hisTask.ProcessInstanceID)
	e.rollbackDeleteHisTaskAndActs(cc, params, params.nextFlowIDList)
	return nil
}

// firstCallActivity 取映射中的首个调用活动。
// @param callActivities 调用活动映射
// @return 调用活动，可能为 nil
func firstCallActivity(callActivities map[string]*model.CallActivity) *model.CallActivity {
	for _, call := range callActivities {
		return call
	}
	return nil
}

// findCallActivityExecution 查找调用活动在父流程中的执行。
// @param processInstanceID 父实例 ID
// @param callActivity 调用活动
// @return 调用活动执行，未启动时为 nil
func (e *Engine) findCallActivityExecution(processInstanceID string, callActivity *model.CallActivity) *executionState {
	if callActivity == nil {
		return nil
	}
	ids := map[string]bool{callActivity.ID: true}
	candidates := e.findActiveExecutionsByActivityIDs(processInstanceID, ids)
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

// childInstanceOf 按父流程执行 ID 查找调用活动子实例。
// @param superExecutionID 父流程执行 ID
// @return 子实例，不存在时为 nil
func (e *Engine) childInstanceOf(superExecutionID string) *instanceState {
	for _, instance := range e.state.Instances {
		if instance.SuperExecutionID == superExecutionID {
			return instance
		}
	}
	return nil
}

// destroyChildProcessInstance 清理调用活动子实例的运行数据（对位 Java cleanCallActivityProcessInstance）。
// @param cc 命令上下文
// @param childID 子实例 ID
func (e *Engine) destroyChildProcessInstance(cc *commandContext, childID string) {
	if _, ok := e.findInstance(childID); !ok {
		return
	}
	// 运行任务全部移除
	for id, task := range e.state.Tasks {
		if task.Task.ProcessInstanceID != childID {
			continue
		}
		e.removeHistoricTaskAssociations(task.Task.ID)
		e.recordTaskEnd(cc, task)
		delete(e.state.Tasks, id)
	}
	// 历史任务全部移除
	for id := range e.state.HistoricTasks {
		historic := e.state.HistoricTasks[id]
		if historic.ProcessInstanceID == childID {
			e.removeHistoricTaskAssociations(id)
			delete(e.state.HistoricTasks, id)
		}
	}
	// 执行、作业、订阅清空
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID == childID {
			execution.Ended = true
			execution.Active = false
		}
	}
	for id, job := range e.state.Jobs {
		if job.ProcessInstanceID == childID {
			delete(e.state.Jobs, id)
		}
	}
	e.removeSubscriptionsForInstance(childID)
	delete(e.state.Instances, childID)
}

// rollbackActiveMultiInstance 正在执行会签撤回策略（对位 ActiveMultiInstanceTaskRollbackOperateStrategy）。
// 跳过 existNextFinishedTask/setAssignee，按串行/并行分路处理。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @return 撤回错误
func (e *Engine) rollbackActiveMultiInstance(cc *commandContext, params *rollbackParams, assignee string) error {
	loop := params.currentTaskElement.LoopCharacteristics
	currentTasks := make([]*taskState, 0)
	for _, task := range e.state.Tasks {
		if task.Task.ProcessInstanceID == params.hisTask.ProcessInstanceID && task.Task.TaskDefinitionKey == params.hisTask.TaskDefinitionKey {
			currentTasks = append(currentTasks, task)
		}
	}
	// 会签任务已全部完成时不可按进行中策略撤回
	if len(currentTasks) == 0 {
		return api.NewError("ENGINE-0699", "当前会签任务已经完成", api.ErrConflict)
	}
	sort.Slice(currentTasks, func(i, j int) bool { return currentTasks[i].Task.ID < currentTasks[j].Task.ID })
	if loop.Sequential {
		return e.rollbackSequentialMultiInstance(cc, params, assignee, currentTasks)
	}
	return e.rollbackParallelMultiInstance(cc, params, assignee, currentTasks)
}

// rollbackSequentialMultiInstance 串行会签撤回：校验"前一位"后在同一子实例上重建任务。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @param currentTasks 运行中会签任务
// @return 撤回错误
func (e *Engine) rollbackSequentialMultiInstance(cc *commandContext, params *rollbackParams, assignee string, currentTasks []*taskState) error {
	miActivityID := params.currentTaskElement.ID
	// 串行特殊场景：下一位已完成则不可撤回（a->b->c 中 b 完成后 a 不可撤回）
	for _, historic := range e.state.HistoricTasks {
		if historic.ProcessInstanceID != params.hisTask.ProcessInstanceID || historic.TaskDefinitionKey != params.hisTask.TaskDefinitionKey {
			continue
		}
		if historic.EndTime == nil || !historic.EndTime.After(*params.hisTask.EndTime) {
			continue
		}
		return api.NewError("ENGINE-0696", "串行会签撤回，已经具有下一线性完成任务,无法进行任务撤回", api.ErrConflict)
	}
	if execution, ok := e.findExecution(params.hisTask.ExecutionID); ok && !execution.Ended {
		// 未推进到下一位：在原子实例上重建当前任务
		for _, task := range e.tasksOnExecution(execution.ID) {
			e.removeHistoricTaskAssociations(task.Task.ID)
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, task.Task.ID)
		}
		e.setVariableLocal(execution, rollbackAssigneeExpr, assignee)
		loopCounter := e.counter(execution, "loopCounter")
		e.setVariableLocal(execution, "loopCounter", loopCounter-1)
		execution.MIIndex = loopCounter - 1
		// 原位重跑取代了已完成位：多实例根计数与元素值同步前移
		if parent, ok := e.findExecution(execution.ParentID); ok && parent.MultiInstanceRoot {
			completed := e.counter(parent, "nrOfCompletedInstances") - 1
			if completed < 0 {
				completed = 0
			}
			e.setVariableLocal(parent, "nrOfCompletedInstances", completed)
			if loopCounter-1 >= 0 && loopCounter-1 < len(parent.MIValues) {
				parent.MIValues[loopCounter-1] = assignee
			}
		}
		execution.Active = true
		execution.Waiting = false
		execution.Ended = false
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: miActivityID})
		e.removeHisTaskRow(params.hisTask)
		return nil
	}
	// 已推进到下一位：只允许撤回当前执行人的"前一位"（G5）
	currentTask := currentTasks[0]
	currentExec, ok := e.findExecution(currentTask.ExecutionID)
	if !ok {
		return api.NewError("ENGINE-0697", "没有找到可撤回的任务", api.ErrConflict)
	}
	currentLoopCounter := e.counter(currentExec, "loopCounter")
	assigneeList, listExec, err := e.rollbackAssigneeList(params.hisTask.ProcessInstanceID)
	if err != nil {
		return err
	}
	if params.hisTask.Assignee == "" {
		return api.NewError("ENGINE-0692", "没有找到历史任务执行人,无法进行 执行顺序判断", api.ErrConflict)
	}
	index := indexOfAssignee(assigneeList, params.hisTask.Assignee)
	if index == -1 {
		return api.NewError("ENGINE-0692", "执行人不存在于初始参数,无法进行 执行顺序判断", api.ErrConflict)
	}
	// 只允许撤"前一位"
	if index != currentLoopCounter-1 {
		return api.NewError("ENGINE-0692", "任务执行人不是 当前执行人的前位 , 不合法撤回", api.ErrConflict)
	}
	// 改写会签人集合并回写；多实例根计数与元素值同步前移（蓝图风险点 2）
	assigneeList[index] = assignee
	e.setVariable(listExec, rollbackAssigneeListExpr, assigneeList)
	if parent, ok := e.findExecution(currentExec.ParentID); ok && parent.MultiInstanceRoot {
		completed := e.counter(parent, "nrOfCompletedInstances") - 1
		if completed < 0 {
			completed = 0
		}
		e.setVariableLocal(parent, "nrOfCompletedInstances", completed)
		if index < len(parent.MIValues) {
			parent.MIValues[index] = assignee
		}
	}
	e.deleteTaskRow(cc, currentTask)
	e.setVariableLocal(currentExec, "loopCounter", currentLoopCounter-1)
	e.setVariableLocal(currentExec, rollbackAssigneeExpr, assignee)
	currentExec.MIIndex = currentLoopCounter - 1
	currentExec.Active = true
	currentExec.Waiting = false
	currentExec.Ended = false
	cc.agenda.schedule(&continueOperation{executionID: currentExec.ID, activityID: miActivityID})
	e.removeHisTaskRow(params.hisTask)
	return nil
}

// rollbackParallelMultiInstance 并行会签撤回：从历史变量恢复该序号的会签上下文（G5）。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @param currentTasks 运行中会签任务
// @return 撤回错误
func (e *Engine) rollbackParallelMultiInstance(cc *commandContext, params *rollbackParams, assignee string, currentTasks []*taskState) error {
	miActivityID := params.currentTaskElement.ID
	if execution, ok := e.findExecution(params.hisTask.ExecutionID); ok && !execution.Ended {
		// 兜底：并行子实例未结束时直接重建当前任务
		for _, task := range e.tasksOnExecution(execution.ID) {
			e.removeHistoricTaskAssociations(task.Task.ID)
			e.recordTaskEnd(cc, task)
			delete(e.state.Tasks, task.Task.ID)
		}
		e.setVariableLocal(execution, rollbackAssigneeExpr, assignee)
		execution.Active = true
		execution.Waiting = false
		cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: miActivityID})
		e.removeHisTaskRow(params.hisTask)
		return nil
	}
	// 从历史变量恢复该序号上下文；Go 多实例子实例局部变量不入历史，
	// 缺失时按"撤回执行人在会签集合中的下标"兜底推导
	root := e.multiInstanceRootOf(currentTasks)
	if root == nil {
		return api.NewError("ENGINE-0697", "没有找到可撤回的任务", api.ErrConflict)
	}
	assigneeList, _, listErr := e.rollbackAssigneeList(params.hisTask.ProcessInstanceID)
	if listErr != nil {
		return listErr
	}
	hisVars := e.historicVariablesOfExecution(params.hisTask.ExecutionID)
	loopCounter := -1
	if raw, hasLoop := hisVars["loopCounter"]; hasLoop {
		loopCounter = toIntValue(raw)
	} else {
		loopCounter = indexOfAssignee(assigneeList, params.hisTask.Assignee)
	}
	if loopCounter < 0 {
		return api.NewError("ENGINE-0692", "缺少会签任务变量", api.ErrConflict)
	}
	var elementVar any
	if root.MIElementVariable != "" {
		if value, hasElement := hisVars[root.MIElementVariable]; hasElement {
			elementVar = value
		} else {
			elementVar = params.hisTask.Assignee
		}
	}
	// 重入子实例并写撤回人（计数修正由重入函数负责）
	child := e.reenterMultiInstanceChild(cc, root, loopCounter, elementVar)
	child.Variables[rollbackAssigneeExpr] = assignee
	e.removeHisTaskRow(params.hisTask)
	return nil
}

// multiInstanceRootOf 从运行中会签任务定位多实例根执行。
// @param currentTasks 运行中会签任务
// @return 多实例根执行，找不到时为 nil
func (e *Engine) multiInstanceRootOf(currentTasks []*taskState) *executionState {
	for _, task := range currentTasks {
		execution, ok := e.findExecution(task.ExecutionID)
		if !ok {
			continue
		}
		if parent, ok := e.findExecution(execution.ParentID); ok && parent.MultiInstanceRoot {
			return parent
		}
	}
	return nil
}

// rollbackAssigneeList 读取实例上的会签人集合变量。
// @param processInstanceID 实例 ID
// @return 会签人列表副本
// @return 变量归属执行
// @return 读取错误
func (e *Engine) rollbackAssigneeList(processInstanceID string) ([]string, *executionState, error) {
	instance, ok := e.findInstance(processInstanceID)
	if !ok {
		return nil, nil, api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	root, ok := e.rootExecution(instance)
	if !ok {
		return nil, nil, api.NewError("ENGINE-0405", "根执行不存在", nil)
	}
	raw, exists := root.Variables[rollbackAssigneeListExpr]
	if !exists {
		return nil, nil, api.NewError("ENGINE-0699", "没有可用会签参数："+rollbackAssigneeListExpr, api.ErrConflict)
	}
	list := assigneeListOf(raw)
	if list == nil {
		return nil, nil, api.NewError("ENGINE-0699", "没有可用会签参数："+rollbackAssigneeListExpr, api.ErrConflict)
	}
	return list, root, nil
}

// assigneeListOf 将任意值转换为字符串列表副本。
// @param raw 原始值
// @return 字符串列表，非列表时为 nil
func assigneeListOf(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

// indexOfAssignee 求字符串在列表中的下标。
// @param values 字符串列表
// @param target 目标值
// @return 下标，不存在时为 -1
func indexOfAssignee(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

// toIntValue 将任意数值转换为整数。
// @param raw 原始值
// @return 整数
func toIntValue(raw any) int {
	switch typed := raw.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

// tasksOnExecution 列出执行上的运行中任务。
// @param executionID 执行 ID
// @return 任务列表
func (e *Engine) tasksOnExecution(executionID string) []*taskState {
	out := make([]*taskState, 0)
	for _, task := range e.state.Tasks {
		if task.ExecutionID == executionID {
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task.ID < out[j].Task.ID })
	return out
}

// historicVariablesOfExecution 按执行 ID 汇总历史变量（撤回恢复会签上下文用）。
// @param executionID 执行 ID
// @return 变量映射
func (e *Engine) historicVariablesOfExecution(executionID string) map[string]any {
	out := map[string]any{}
	for _, variable := range e.state.HistoricVariables {
		if variable.ExecutionID == executionID && variable.TaskID == "" {
			out[variable.Name] = variable.Value
		}
	}
	return out
}

// rollbackCompletedMultiInstance 已完成会签撤回策略（G6，对位 CompletedMultiInstanceTaskAndNextDefaultTask）：
// 重造会签根作用域与占位子实例，串行校验最后完成位。
// @param cc 命令上下文
// @param params 撤回参数
// @param assignee 撤回办理人
// @return 撤回错误
func (e *Engine) rollbackCompletedMultiInstance(cc *commandContext, params *rollbackParams, assignee string) error {
	loop := params.currentTaskElement.LoopCharacteristics
	if err := e.rollbackExistNextFinishedTask(params, params.nextFlowIDList); err != nil {
		return err
	}
	execution, err := e.rollbackGetExecutionEntity(params)
	if err != nil {
		return err
	}
	assigneeList, listExec, err := e.rollbackAssigneeList(params.hisTask.ProcessInstanceID)
	if err != nil {
		return err
	}
	// Go 多实例子实例局部变量不入历史：历史变量缺失时按撤回执行人下标兜底推导
	hisVars := e.historicVariablesOfExecution(params.hisTask.ExecutionID)
	_, hasExecutor := hisVars[rollbackAssigneeExpr]
	_, hasLoop := hisVars["loopCounter"]
	if !hasExecutor || !hasLoop {
		// 兜底：executor 取历史任务执行人，loopCounter 取其在会签集合中的下标
		index := indexOfAssignee(assigneeList, params.hisTask.Assignee)
		if params.hisTask.Assignee == "" || index < 0 {
			return api.NewError("ENGINE-0692", "缺少会签任务变量", api.ErrConflict)
		}
		if !hasExecutor {
			hisVars[rollbackAssigneeExpr] = params.hisTask.Assignee
		}
		if !hasLoop {
			hisVars["loopCounter"] = index
		}
	}
	hisLoopCounter := toIntValue(hisVars["loopCounter"])
	if loop.Sequential {
		// 串行最终 loopCounter 与执行人都是最后一位：只允许撤回最后完成位
		if assigneeList[len(assigneeList)-1] != params.hisTask.Assignee {
			return api.NewError("ENGINE-0692", "不是串行最后一个节点，无法进行撤回", api.ErrConflict)
		}
		assigneeList[len(assigneeList)-1] = assignee
		e.setVariable(listExec, rollbackAssigneeListExpr, assigneeList)
	}
	root := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: execution.ProcessInstanceID,
		DefinitionID:      execution.DefinitionID,
		ParentID:          execution.ParentID,
		Active:            false,
		Variables:         make(map[string]any),
	}
	e.state.Executions[root.ID] = root
	e.rebuildMultiInstanceScope(cc, root, params.currentTaskElement, toAnySliceOf(assigneeList), hisLoopCounter, assignee)
	for name, value := range rollbackSetAssigneeVars(params, assignee, rollbackTypeCompletedMultiInstance) {
		e.setVariableLocal(root, name, value)
	}
	e.rollbackDeleteRuntimeTasks(cc, params)
	e.rollbackDeleteHisTaskAndActs(cc, params, params.nextFlowIDList)
	return nil
}

// toAnySliceOf 将字符串列表转为 any 切片。
// @param values 字符串列表
// @return any 切片
func toAnySliceOf(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// CompleteToTarget 直送（D2）：办结当前任务并把执行迁移到目标活动继续。
// 对位 Java completeToTargetActivity 的伪造连线语义：任务完成历史保留，目标节点生成新任务，
// operationType=completeToTarget 变量标记直送语义供下游识别。
// @param ctx 上下文
// @param taskID 当前运行任务 ID
// @param targetActivityID 目标活动 ID
// @param variables 直送时写入的流程变量
// @return 直送错误
func (e *Engine) CompleteToTarget(ctx context.Context, taskID, targetActivityID string, variables map[string]any) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.completeToTarget(cc, taskID, targetActivityID, variables)
	})
	return err
}

// completeToTarget 在命令内执行直送。
// @param cc 命令上下文
// @param taskID 当前运行任务 ID
// @param targetActivityID 目标活动 ID
// @param variables 直送时写入的流程变量
// @return 直送错误
func (e *Engine) completeToTarget(cc *commandContext, taskID, targetActivityID string, variables map[string]any) error {
	if strings.TrimSpace(targetActivityID) == "" {
		return api.NewError("ENGINE-0670", "目标活动不能为空", api.ErrBadRequest)
	}
	task, ok := e.findTask(taskID)
	if !ok {
		return api.NewError("ENGINE-0805", "任务不存在", api.ErrNotFound)
	}
	if err := e.allowTask(cc.ctx, task.Task.ID); err != nil {
		return err
	}
	instance, ok := e.findInstance(task.Task.ProcessInstanceID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	if instance.Ended {
		return api.NewError("ENGINE-0680", "流程实例已结束，无法直送", api.ErrConflict)
	}
	if instance.Suspended {
		return api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
	}
	execution, ok := e.findExecution(task.ExecutionID)
	if !ok {
		return api.NewError("ENGINE-0405", "任务所属执行不存在", nil)
	}
	definition, ok := e.findDefinition(execution.DefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	targetElement, ok := findElement(process, targetActivityID)
	if !ok {
		return api.NewError("ENGINE-0404", "目标活动不存在: "+targetActivityID, api.ErrNotFound)
	}
	if _, isNode := targetElement.(model.FlowNode); !isNode {
		return api.NewError("ENGINE-0682", "目标活动不是流程节点", api.ErrBadRequest)
	}
	// 直送变量先落库，下游监听按 operationType 识别直送语义
	if root, ok := e.rootExecution(instance); ok {
		e.setVariable(root, "operationType", "completeToTarget")
		for name, value := range variables {
			e.setVariable(root, name, value)
		}
	}
	// 当前任务按完成语义闭合（保留完成历史，无删除原因）
	if err := e.invokeTaskListeners(cc, task, "complete"); err != nil {
		return err
	}
	if err := e.invokeTaskHooks(cc, task, hookEventCompleted); err != nil {
		return err
	}
	e.recordTaskLog(cc, task, "complete", actorFromContext(cc.ctx), "completeToTarget")
	e.recordTaskEnd(cc, task)
	e.promoteTaskParticipants(cc, task)
	delete(e.state.Tasks, task.Task.ID)
	// 执行整体迁移到目标活动继续（G1，含后代清场）
	if _, err := e.moveExecutionsToSingleActivity(cc, []*executionState{execution}, targetActivityID, "completeToTarget"); err != nil {
		return err
	}
	if err := e.evaluateConditionalSubscriptions(cc, task.Task.ProcessInstanceID); err != nil {
		return err
	}
	return cc.agenda.run(cc)
}

// ResetProcess 重置流程（软）：清场全部活动任务并把全部执行迁回开始事件的首个出线目标。
// 对位 Java doResetProcess；next_activity 等外围数据由 workflow 层清理，引擎只管状态。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param operator 操作人（业务操作记录由 workflow 层落库，引擎不消费）
// @return 重置错误
func (e *Engine) ResetProcess(ctx context.Context, processInstanceID, operator string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.resetProcess(cc, processInstanceID, operator)
	})
	return err
}

// resetProcess 在命令内执行重置。
// @param cc 命令上下文
// @param processInstanceID 实例 ID
// @param operator 操作人
// @return 重置错误
func (e *Engine) resetProcess(cc *commandContext, processInstanceID, operator string) error {
	instance, ok := e.findInstance(processInstanceID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程实例不存在", api.ErrNotFound)
	}
	if err := e.allowTenant(cc.ctx, instance.TenantID); err != nil {
		return err
	}
	if instance.Ended {
		return api.NewError("ENGINE-0680", "流程实例已结束，无法重置", api.ErrConflict)
	}
	if instance.Suspended {
		return api.NewError("ENGINE-0661", "流程实例已挂起", api.ErrConflict)
	}
	definition, ok := e.findDefinition(instance.ProcessDefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0501", "流程定义不包含流程", nil)
	}
	startEvent := firstStartEvent(process)
	if startEvent == nil {
		return api.NewError("ENGINE-0104", "流程缺少开始事件", nil)
	}
	if len(startEvent.GetOutgoing()) == 0 {
		return api.NewError("ENGINE-0687", "开始事件没有出线", nil)
	}
	targetID := startEvent.GetOutgoing()[0].TargetRef
	executions := e.topActivityExecutions(processInstanceID)
	if len(executions) == 0 {
		return api.NewError("ENGINE-0685", "实例没有可重置的活动执行", api.ErrConflict)
	}
	// 重置原因记录在实例删除原因上，操作人由 workflow 层写业务记录
	instance.DeleteReason = "reset"
	if _, err := e.moveExecutionsToSingleActivity(cc, executions, targetID, "reset"); err != nil {
		return err
	}
	return cc.agenda.run(cc)
}

// StopProcessToEvent 软终止（G10）：全部活动执行迁至首个正常结束事件走完，保留 end 事件历史。
// 对位 Java doStopProcessByTaskId 的软终止语义；挂起实例拒绝。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 终止错误
func (e *Engine) StopProcessToEvent(ctx context.Context, processInstanceID string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.moveToEndEvent(cc, processInstanceID)
	})
	return err
}

// ReactivateProcessInstance 历史实例复活：按历史实例重建运行实例并可指定落位活动。
// 对位 Java ReactivateProcessInstanceCmd：历史行 endTime/deleteReason/duration 清空复位，
// 变量按历史变量创建时间升序覆盖重建，initiator 重写为操作人，落位活动为空时回开始事件。
// @param ctx 上下文
// @param historicProcessInstanceID 历史实例 ID
// @param targetActivityID 落位活动 ID，空串表示回开始事件后首个用户任务
// @param operator 操作人（重写发起人）
// @return 复活错误
func (e *Engine) ReactivateProcessInstance(ctx context.Context, historicProcessInstanceID, targetActivityID, operator string) error {
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		return nil, e.reactivateProcessInstance(cc, historicProcessInstanceID, targetActivityID, operator)
	})
	return err
}

// reactivateProcessInstance 在命令内执行历史实例复活。
// @param cc 命令上下文
// @param historicProcessInstanceID 历史实例 ID
// @param targetActivityID 落位活动 ID
// @param operator 操作人
// @return 复活错误
func (e *Engine) reactivateProcessInstance(cc *commandContext, historicProcessInstanceID, targetActivityID, operator string) error {
	historic, ok := e.state.HistoricProcesses[historicProcessInstanceID]
	if !ok {
		return api.NewError("ENGINE-0404", "历史流程实例不存在", api.ErrNotFound)
	}
	if err := e.allowTenant(cc.ctx, historic.TenantID); err != nil {
		return err
	}
	// 运行中实例不允许重复复活
	if instance, exists := e.findInstance(historicProcessInstanceID); exists && !instance.Ended {
		return api.NewError("ENGINE-0680", "流程实例仍在运行，无法复活", api.ErrConflict)
	}
	definition, ok := e.findDefinition(historic.ProcessDefinitionID)
	if !ok {
		return api.NewError("ENGINE-0404", "流程定义不存在", api.ErrNotFound)
	}
	if definition.Info.Suspended {
		return api.NewError("ENGINE-0102", "流程定义已挂起", api.ErrConflict)
	}
	process := processOfDefinition(definition)
	if process == nil {
		return api.NewError("ENGINE-0103", "流程定义不包含流程", nil)
	}
	// 落位活动：空取开始事件（首个用户任务由流程自然推进产生）
	initialActivityID := ""
	if strings.TrimSpace(targetActivityID) != "" {
		element, exists := findElement(process, targetActivityID)
		if !exists {
			return api.NewError("ENGINE-0404", "目标活动不存在: "+targetActivityID, api.ErrNotFound)
		}
		if _, isNode := element.(model.FlowNode); !isNode {
			return api.NewError("ENGINE-0682", "目标活动不是流程节点", api.ErrBadRequest)
		}
		initialActivityID = targetActivityID
	} else {
		start := firstStartEvent(process)
		if start == nil {
			return api.NewError("ENGINE-0104", "流程缺少开始事件", nil)
		}
		initialActivityID = start.ID
	}
	initiator := operator
	if initiator == "" {
		initiator = historic.StartUserID
	}
	// 历史行复活：endTime/deleteReason/duration 清空语义
	historic.EndTime = nil
	historic.DeleteReason = ""
	historic.DurationMillis = 0
	historic.Ended = false
	historic.State = "running"
	historic.EndActivityID = ""
	historic.EndUserID = ""
	instance := &instanceState{
		ID:                   historicProcessInstanceID,
		ProcessDefinitionID:  historic.ProcessDefinitionID,
		ProcessDefinitionKey: historic.ProcessDefinitionKey,
		BusinessKey:          historic.BusinessKey,
		Name:                 historic.Name,
		StartUserID:          initiator,
		TenantID:             historic.TenantID,
		CallbackID:           historic.CallbackID,
		CallbackType:         historic.CallbackType,
		ReferenceID:          historic.ReferenceID,
		ReferenceType:        historic.ReferenceType,
		StartTime:            historic.StartTime,
		RootExecutionID:      historicProcessInstanceID,
	}
	e.state.Instances[historicProcessInstanceID] = instance
	root := &executionState{
		ID:                historicProcessInstanceID,
		ProcessInstanceID: historicProcessInstanceID,
		DefinitionID:      historic.ProcessDefinitionID,
		Active:            true,
		Scope:             true,
		Variables:         make(map[string]any),
		Transient:         make(map[string]any),
		PendingBranches:   1,
	}
	e.state.Executions[root.ID] = root
	// 变量按历史变量创建时间升序覆盖（同一节点以最后执行人为准）
	for _, item := range e.historicVariablesOfInstance(historicProcessInstanceID) {
		root.Variables[item.name] = copyValue(item.value)
	}
	// 重写发起人 starter 历史链接
	if initiator != "" {
		link := &api.IdentityLink{ID: e.newID(), Type: "starter", UserID: initiator, ProcessInstanceID: historicProcessInstanceID, CreateTime: cc.now}
		e.state.HistoricLinks[link.ID] = link
	}
	execution := &executionState{
		ID:                e.newID(),
		ProcessInstanceID: historicProcessInstanceID,
		DefinitionID:      historic.ProcessDefinitionID,
		ParentID:          root.ID,
		ActivityID:        initialActivityID,
		Active:            true,
		Variables:         make(map[string]any),
	}
	e.state.Executions[execution.ID] = execution
	e.scheduleEventSubProcesses(cc, root, process.Elements, process.Order, definition)
	cc.agenda.schedule(&continueOperation{executionID: execution.ID, activityID: initialActivityID})
	if err := cc.agenda.run(cc); err != nil {
		return err
	}
	return nil
}

// historicVariableItem 是历史变量的排序载体。
type historicVariableItem struct {
	name       string    // 变量名
	value      any       // 变量值
	createTime time.Time // 创建时间
	updateTime time.Time // 最后更新时间
}

// historicVariablesOfInstance 按创建时间升序收集实例的历史变量。
// @param processInstanceID 实例 ID
// @return 排序后的变量列表
func (e *Engine) historicVariablesOfInstance(processInstanceID string) []historicVariableItem {
	items := make([]historicVariableItem, 0)
	for _, variable := range e.state.HistoricVariables {
		if variable.ProcessInstanceID != processInstanceID {
			continue
		}
		items = append(items, historicVariableItem{name: variable.Name, value: variable.Value, createTime: variable.CreateTime, updateTime: variable.LastUpdatedTime})
	}
	// 创建时间升序：后创建的变量覆盖先创建的同名变量
	sort.Slice(items, func(i, j int) bool {
		if !items[i].createTime.Equal(items[j].createTime) {
			return items[i].createTime.Before(items[j].createTime)
		}
		if !items[i].updateTime.Equal(items[j].updateTime) {
			return items[i].updateTime.Before(items[j].updateTime)
		}
		return items[i].name < items[j].name
	})
	return items
}

// topActivityExecutions 收集实例中承载活动的最顶层执行（排除根作用域与被祖先覆盖的执行）。
// @param processInstanceID 实例 ID
// @return 顶层执行列表（按 ID 排序）
func (e *Engine) topActivityExecutions(processInstanceID string) []*executionState {
	candidates := make([]*executionState, 0)
	for _, execution := range e.state.Executions {
		if execution.ProcessInstanceID != processInstanceID || execution.Ended {
			continue
		}
		// 根作用域不承载活动
		if execution.ActivityID == "" {
			continue
		}
		candidates = append(candidates, execution)
	}
	// 只保留彼此无祖先关系的最顶层执行
	out := make([]*executionState, 0, len(candidates))
	for _, candidate := range candidates {
		covered := false
		for _, other := range candidates {
			if other.ID == candidate.ID {
				continue
			}
			if e.isAncestor(other, candidate) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, candidate)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// isAncestor 判断祖先执行是否在后代执行的父链上。
// @param ancestor 祖先候选
// @param node 后代节点
// @return 是否为祖先
func (e *Engine) isAncestor(ancestor, node *executionState) bool {
	current := node
	for current != nil && current.ParentID != "" {
		if current.ParentID == ancestor.ID {
			return true
		}
		parent, ok := e.findExecution(current.ParentID)
		if !ok {
			return false
		}
		current = parent
	}
	return false
}
