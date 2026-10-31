package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/expr"
)

// simulate 错误码段（29xx 段为本文件专用，避免与他人冲突）。
const (
	errCodeSimTaskMissing   = "WORKFLOW-2901" // 推演入口任务/定义不存在
	errCodeSimDefMissing    = "WORKFLOW-2902" // 按 Key 推演时定义不存在
	errCodeSimCondEval      = "WORKFLOW-2903" // 连线条件求值失败（仅日志语境，实际剪枝按 false 处理）
	errCodeSimReviewMissing = "WORKFLOW-2910" // 审阅任务不存在
	errCodeSimReviewBlank   = "WORKFLOW-2911" // 审阅参数缺失
)

// SimEndActivityID 是"办结"伪节点的保留代号（对位 Java next() 的 end 内置节点：
// his_step 添加步骤时禁止用户使用该代号）。
const SimEndActivityID = "end"

// SimEndActivityName 是"办结"伪节点的展示名。
const SimEndActivityName = "办结"

// NextUserTaskView 是下一步推演视图（对位 Java NextUserTaskVo 的数据面）。
type NextUserTaskView struct {
	ActivityID     string   // 节点 ID（保留代号 end 表示办结）
	ActivityName   string   // 节点名称
	ExecutorIDs    []string // 执行人身份 Id 列表
	ExecutorNames  []string // 执行人姓名列表（与 ExecutorIDs 按下标对应，查不到回退 Id）
	DynamicRoleIDs []string // 动态角色 Id 列表（需业务系统二次解析的角色，对应 Java dynamicRoles）
	Sort           int      // 步骤排序号（his_step.sort，缺失为 0）
	ReturnStep     bool     // 是否退回步骤（模式 1/3/4 的退回目标、模式 5 历史步骤均为 true）
}

// SimulateService 是下一步推演器（P5.3，对位 Java ProcessesUtil#getNextUserTaskAndUserList:3073 /
// getNextUserTaskAndUserListByKey:3375 / FlowUtils#getNextUserTasks:39 静态图遍历）。
// 推演是纯静态图遍历：不考虑会签已完成/并行已走分支等运行时事实，
// 唯一运行时校正 = his_next_activity 历史执行人优先（与 Java 行为一致）。
type SimulateService struct {
	engine    api.Engine            // 引擎门面（七服务）
	meta      *ModelMetaService     // 模型元数据（节点执行人配置读取）
	next      *NextActivityService  // his_next_activity 历史执行人来源，nil 跳过该来源
	histories *HistoryExtendService // 已办扩展（预留给历史链路扩展，nil 安全）
	messages  *MessageExtendService // 待办扩展（退回步骤 sender_step 信息来源），nil 跳过退回模式
	identity  api.IdentityService   // 身份服务（组成员展开/姓名补全），nil 时保留 Id 原值
	steps     StepStore             // his_step 步骤规则（sort/returnApprovalStep），nil 跳过退回模式与排序
	clock     clock.Clock           // 时间源（预留，推演本身无写入）
}

// SimulateOption 是推演服务选项函数。
type SimulateOption func(*SimulateService)

// WithSimulateSteps 注入 his_step 步骤规则存储（sort/returnApprovalStep 读取）。
// 未注入时跳过退回模式分支，输出按静态遍历顺序（对位 Java 无规则时的行为）。
// @param steps 步骤规则存储端口
// @return 选项
func WithSimulateSteps(steps StepStore) SimulateOption {
	return func(s *SimulateService) { s.steps = steps }
}

// NewSimulateService 构造下一步推演域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param next 下一步预生成域服务（his_next_activity 历史执行人来源，可 nil）
// @param histories 已办扩展域服务（预留，可 nil）
// @param messages 待办扩展域服务（退回步骤 sender 信息来源，可 nil）
// @param identity 身份服务（组成员展开/姓名补全，可 nil）
// @param c 时间源，nil 时回退真实时钟
// @param opts 选项（步骤规则注入等）
// @return 推演域服务
func NewSimulateService(engine api.Engine, meta *ModelMetaService, next *NextActivityService, histories *HistoryExtendService, messages *MessageExtendService, identity api.IdentityService, c clock.Clock, opts ...SimulateOption) *SimulateService {
	if c == nil {
		c = clock.Real{}
	}
	s := &SimulateService{
		engine: engine, meta: meta, next: next, histories: histories,
		messages: messages, identity: identity, clock: c,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SimulateByTask 按当前任务推演下一步用户任务及执行人（对位 getNextUserTaskAndUserList:3073）。
// 流程：三源变量合并 → isReturnStep 判定（五种退回模式）→ 静态图遍历 → 执行人解析 → 排序输出。
// @param ctx 操作上下文
// @param taskID 当前任务 Id（空或任务不存在返回空列表）
// @param extraVars 前端入参变量（连线条件过滤用，优先级最高）
// @return 推演视图列表（含保留代号 end 的"办结"项，与 Java getNextUserTasks 一致）
// @return 错误
func (s *SimulateService) SimulateByTask(ctx context.Context, taskID string, extraVars map[string]any) ([]NextUserTaskView, error) {
	// 与 Java 一致：任务 Id 为空直接返回空列表
	if taskID == "" || s.engine == nil {
		return []NextUserTaskView{}, nil
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		// Java 对不存在任务返回空列表（R.ok(empty)），保持行为一致
		return []NextUserTaskView{}, nil
	}
	// 实例优先取运行态；已结束实例回退历史（Java 运行态查询为空会 NPE，此处更稳健）
	instance, instanceErr := s.engine.RuntimeService().GetProcessInstance(ctx, task.ProcessInstanceID)
	if instanceErr != nil {
		historic, hisErr := s.engine.HistoryService().GetHistoricProcessInstance(ctx, task.ProcessInstanceID)
		if hisErr != nil {
			return []NextUserTaskView{}, nil
		}
		instance = api.ProcessInstance{
			ID:                   historic.ID,
			ProcessDefinitionID:  historic.ProcessDefinitionID,
			StartUserID:          historic.StartUserID,
			TenantID:             historic.TenantID,
			ProcessDefinitionKey: historic.ProcessDefinitionKey,
		}
	}
	// 1. 三源变量合并：实例变量 + 任务局部 + extraVars → entityMap（含 initiator）
	entityMap, err := s.buildEntityMap(ctx, instance.ID, taskID, instance.StartUserID, extraVars)
	if err != nil {
		return nil, err
	}
	// 定义与部署（his_step 按部署定位）
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, api.NewError(errCodeSimDefMissing, "流程定义不存在", err)
	}
	// 2. isReturnStep 判定与五种退回模式
	returnApprovalStep, backStep, hasBack := s.detectReturnMode(ctx, definition.DeploymentID, task, instance)
	switch returnApprovalStep {
	case "1":
		// 模式 1：只给退回步骤
		if !hasBack {
			return []NextUserTaskView{}, nil
		}
		return []NextUserTaskView{backStep}, nil
	case "5":
		// 模式 5：历史步骤列表（preTaskId 链）
		return s.historyStepViews(ctx, instance.ID, task, definition.DeploymentID)
	}
	// 3. 静态图遍历（getNextUserTasks 语义，含办结伪节点）
	process, cands, err := s.traverseFromTask(ctx, task, entityMap)
	if err != nil {
		return nil, err
	}
	views, err := s.resolveCandidates(ctx, process, task.ProcessDefinitionID, definition.DeploymentID, instance.ID, instance.StartUserID, cands, entityMap, true)
	if err != nil {
		return nil, err
	}
	// 4/5. 退回模式 3/4 的列表合成与排序
	return s.composeReturnModes(returnApprovalStep, backStep, hasBack, views), nil
}

// SimulateByKey 未发起实例按流程定义推演（对位 getNextUserTaskAndUserListByKey:3375，
// 从开始事件遍历、不包含办结伪节点）。
// @param ctx 操作上下文
// @param tenantID 租户（透传给调用方上下文校验，推演本身按定义遍历）
// @param processDefinitionKey 流程定义 Key（取最新版本）
// @param businessKey 业务单据 Id（预留参数，仅透传语义）
// @param vars 表单/条件变量（连线求值与 ${xxx} 执行人取值）
// @return 推演视图列表（不含 end 伪节点，与 Java getNextUserTasksWithoutEndNode 一致）
// @return 错误
func (s *SimulateService) SimulateByKey(ctx context.Context, tenantID, processDefinitionKey, businessKey string, vars map[string]any) ([]NextUserTaskView, error) {
	_ = tenantID
	_ = businessKey
	if strings.TrimSpace(processDefinitionKey) == "" {
		return nil, api.NewError(errCodeSimReviewBlank, "参数 processDefinitionKey 为空", nil)
	}
	if s.engine == nil {
		return []NextUserTaskView{}, nil
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinitionByKey(ctx, processDefinitionKey)
	if err != nil {
		return nil, api.NewError(errCodeSimDefMissing, "流程定义不存在", err)
	}
	// 变量源仅有入参（未发起实例无流程变量），initiator 从入参透传
	entityMap := map[string]any{}
	if raw, ok := entityMapValue(vars[VarEntity]); ok {
		entityMap[VarEntity] = raw
		// 表单 Map 顶层键并入，与 Java variables.putAll(map) 一致
		for key, value := range raw {
			entityMap[key] = value
		}
	}
	for key, value := range vars {
		entityMap[key] = value
	}
	bpmn, err := s.engine.RepositoryService().GetBpmnModel(ctx, definition.ID)
	if err != nil {
		return nil, err
	}
	if len(bpmn.Processes) == 0 {
		return nil, api.NewError(errCodeSimDefMissing, "流程定义不包含流程", nil)
	}
	process := bpmn.Processes[0]
	// 从开始事件遍历（getNextUserTasksWithoutEndNode 对位，不含 end）
	cands := simCollectFromStarts(process, entityMap, false)
	return s.resolveCandidates(ctx, process, definition.ID, definition.DeploymentID, "", entityMapValueString(entityMap, VarInitiator), cands, entityMap, false)
}

// buildEntityMap 合并三源变量为连线条件求值集合（对位 Java entityMap 组装）：
// 实例变量 + 实体表单 Map（entity 键与其顶层键）+ 任务局部变量 + extraVars，内置 initiator。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param taskID 任务 Id
// @param startUserId 发起人身份 Id
// @param extraVars 前端入参
// @return 合并后的变量集合
// @return 错误
func (s *SimulateService) buildEntityMap(ctx context.Context, processInstanceID, taskID, startUserId string, extraVars map[string]any) (map[string]any, error) {
	entityMap := map[string]any{}
	// 源 1：流程实例变量（含 entity 表单 Map）
	if instanceVars, err := s.engine.RuntimeService().GetVariables(ctx, processInstanceID); err == nil {
		for key, value := range instanceVars {
			entityMap[key] = value
		}
	}
	// 源 2：任务局部变量（任务级覆盖实例级）
	if taskVars, err := s.engine.TaskService().GetTaskVariables(ctx, taskID); err == nil {
		for key, value := range taskVars {
			entityMap[key] = value
		}
	}
	// entity 表单 Map：键 "entity" 保留整包，顶层键并入求值集合（Java variables.putAll(map) 语义）
	if raw, ok := entityMapValue(entityMap[VarEntity]); ok {
		entityMap[VarEntity] = raw
		for key, value := range raw {
			entityMap[key] = value
		}
	}
	// 源 3：前端入参（优先级最高）
	for key, value := range extraVars {
		entityMap[key] = value
	}
	// 内置发起人参数（Java 未传 initiator 时注入 startUserId）
	if _, ok := entityMap[VarInitiator]; !ok {
		entityMap[VarInitiator] = startUserId
	}
	return entityMap, nil
}

// entityMapValue 提取实体表单 Map（非 map 值返回否）。
// @param value 原始值
// @return 表单 Map
// @return 是否为 Map
func entityMapValue(value any) (map[string]any, bool) {
	if m, ok := value.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

// entityMapValueString 读取变量集合中的字符串值（nil → 空串）。
// @param vars 变量集合
// @param key 变量名
// @return 字符串值
func entityMapValueString(vars map[string]any, key string) string {
	if vars == nil {
		return ""
	}
	return expr.ToString(vars[key])
}

// detectReturnMode 判定当前步骤是否退回步骤并解析退回审批模式
// （对位 Java isReturnStep SQL：当前步骤 sort < 发送步骤 sort 即退回位）。
// 步骤规则/待办扩展未注入或信息缺失时返回空模式（跳过退回分支）。
// @param ctx 操作上下文
// @param deploymentID 部署 Id
// @param task 当前任务
// @param instance 流程实例
// @return 退回审批模式原始值（"" 表示非退回步骤或无法判定）
// @return 退回步骤视图（sender 信息来源待办扩展，模式 1/3/4 使用）
// @return 是否存在可用退回步骤视图
func (s *SimulateService) detectReturnMode(ctx context.Context, deploymentID string, task api.Task, instance api.ProcessInstance) (string, NextUserTaskView, bool) {
	empty := NextUserTaskView{}
	// 步骤规则或待办扩展未注入：跳过退回模式分支（P3-A 接口位约定，注释保留行为差异）
	if s.steps == nil || s.messages == nil {
		return "", empty, false
	}
	current, found, err := s.steps.GetByDeploymentAndActivity(ctx, deploymentID, task.TaskDefinitionKey)
	if err != nil || !found {
		return "", empty, false
	}
	msg, found, err := s.messages.GetByTaskID(ctx, task.ID)
	if err != nil || !found || msg.SenderStepCode == "" {
		return "", empty, false
	}
	sender, found, err := s.steps.GetByDeploymentAndActivity(ctx, deploymentID, msg.SenderStepCode)
	if err != nil || !found {
		return "", empty, false
	}
	// 退回位判定：当前步骤排序号 < 发送步骤排序号（sort1 < sort2）
	if current.Sort >= sender.Sort {
		return "", empty, false
	}
	// 退回步骤视图：节点取待办扩展 sender_step，人员取 his_next_activity 上次设置
	backStep := NextUserTaskView{
		ActivityID:   msg.SenderStepCode,
		ActivityName: msg.SenderStepName,
		Sort:         sender.Sort,
		ReturnStep:   true,
	}
	if s.next != nil {
		if ids, err := s.historyExecutorsOf(ctx, instance.ID, msg.SenderStepCode); err == nil && len(ids) > 0 {
			backStep.ExecutorIDs = ids
			backStep.ExecutorNames = s.executorNames(ctx, ids)
		}
	}
	return current.ReturnApprovalStep, backStep, backStep.ActivityID != ""
}

// historyExecutorsOf 读取实例下指定节点的历史执行人（his_next_activity 去重保序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityID 节点 Id
// @return 身份 Id 列表（来源未注入或无记录返回空）
// @return 错误
func (s *SimulateService) historyExecutorsOf(ctx context.Context, processInstanceID, activityID string) ([]string, error) {
	if s.next == nil {
		return nil, nil
	}
	records, err := s.next.ListHisByInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, record := range records {
		// 仅取该节点记录，按版本序去重保序（Java getHisNextActivityIdentityIds 语义）
		if record.NextActivityID != activityID || seen[record.IdentityID] {
			continue
		}
		seen[record.IdentityID] = true
		ids = append(ids, record.IdentityID)
	}
	return ids, nil
}

// historyStepViews 组装模式 5 的历史步骤列表（对位 getHisStepList:3283 + getBackNodesByHisTaskId:2803）：
// 沿 his_next_activity 的 pre_task_id 链上溯，收集链上节点（去重、排除当前步骤），
// 且仅保留流转过程（历史任务）中出现过的节点，人员取各历史任务的办理人。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param task 当前任务
// @param deploymentID 部署 Id
// @return 历史步骤视图列表
// @return 错误
func (s *SimulateService) historyStepViews(ctx context.Context, processInstanceID string, task api.Task, deploymentID string) ([]NextUserTaskView, error) {
	views := make([]NextUserTaskView, 0)
	if s.next == nil {
		return views, nil
	}
	// 流转过程：实例全部历史任务的节点代号集合（Java getCirculationProcess 比对语义）
	circulated := map[string]bool{}
	if hisTasks, err := s.engine.HistoryService().ListHistoricTaskInstances(ctx, processInstanceID); err == nil {
		for _, hisTask := range hisTasks {
			circulated[hisTask.TaskDefinitionKey] = true
		}
	}
	// 各节点历史办理人：按节点代号聚合历史任务办理人
	assignees := map[string][]string{}
	if hisTasks, err := s.engine.HistoryService().ListHistoricTaskInstances(ctx, processInstanceID); err == nil {
		for _, hisTask := range hisTasks {
			if hisTask.Assignee == "" {
				continue
			}
			assignees[hisTask.TaskDefinitionKey] = append(assignees[hisTask.TaskDefinitionKey], hisTask.Assignee)
		}
	}
	// preTaskId 链上溯：当前任务 → 记录 preTaskId → 历史任务 → 该任务自身的记录链
	seen := map[string]bool{task.TaskDefinitionKey: true}
	cursor := task.ID
	// 链深度按历史任务数封顶，防止脏数据成环
	for depth := 0; depth < 128; depth++ {
		records, err := s.next.ListHisByTaskID(ctx, cursor)
		if err != nil || len(records) == 0 {
			break
		}
		preTaskID := records[0].PreTaskID
		if preTaskID == "" {
			break
		}
		hisTask, err := s.engine.HistoryService().GetHistoricTaskInstance(ctx, preTaskID)
		if err != nil {
			break
		}
		// 排除当前步骤，且只保留流转过程中出现过的节点（Java taskKeyList 比对）
		if hisTask.TaskDefinitionKey != task.TaskDefinitionKey && circulated[hisTask.TaskDefinitionKey] && !seen[hisTask.TaskDefinitionKey] {
			seen[hisTask.TaskDefinitionKey] = true
			view := NextUserTaskView{
				ActivityID:   hisTask.TaskDefinitionKey,
				ActivityName: hisTask.Name,
				ReturnStep:   true,
			}
			if ids := distinctNonEmpty(assignees[hisTask.TaskDefinitionKey]); len(ids) > 0 {
				view.ExecutorIDs = ids
				view.ExecutorNames = s.executorNames(ctx, ids)
			}
			// 步骤排序号（his_step 缺失为 0）
			if s.steps != nil {
				if rule, found, err := s.steps.GetByDeploymentAndActivity(ctx, deploymentID, hisTask.TaskDefinitionKey); err == nil && found {
					view.Sort = rule.Sort
				}
			}
			views = append(views, view)
		}
		cursor = preTaskID
	}
	return views, nil
}

// composeReturnModes 按退回审批模式合成最终列表（对位 Java 模式 2/3/4 与默认排序）。
// @param returnApprovalStep 退回审批模式原始值（"" 为非退回步骤）
// @param backStep 退回步骤视图
// @param hasBack 是否存在可用退回步骤视图
// @param views 静态遍历推演结果（已按 sort + 节点 ID 排序）
// @return 最终推演列表
func (s *SimulateService) composeReturnModes(returnApprovalStep string, backStep NextUserTaskView, hasBack bool, views []NextUserTaskView) []NextUserTaskView {
	switch returnApprovalStep {
	case "3":
		// 模式 3 退回优先：退回步骤在前，下一步骤在后（去掉与退回步骤同名项，不重排）
		result := make([]NextUserTaskView, 0, len(views)+1)
		if hasBack {
			result = append(result, backStep)
		}
		for _, view := range views {
			if hasBack && view.ActivityID == backStep.ActivityID {
				continue
			}
			result = append(result, view)
		}
		return result
	case "4":
		// 模式 4 下一步优先：下一步骤在前（去掉同名项），退回步骤在后
		result := make([]NextUserTaskView, 0, len(views)+1)
		for _, view := range views {
			if hasBack && view.ActivityID == backStep.ActivityID {
				continue
			}
			result = append(result, view)
		}
		if hasBack {
			result = append(result, backStep)
		}
		return result
	default:
		// 默认/模式 2：仅下一步骤，按 sort + 节点 ID 排序
		return views
	}
}

// traverseFromTask 从当前任务节点执行静态图遍历（FlowUtils.getNextUserTasks 对位，含 end 伪节点）。
// @param ctx 操作上下文
// @param task 当前任务
// @param entityMap 连线条件求值集合
// @return 流程模型（供执行人解析复用）
// @return 候选节点列表
// @return 错误
func (s *SimulateService) traverseFromTask(ctx context.Context, task api.Task, entityMap map[string]any) (*model.Process, []simCandidate, error) {
	bpmn, err := s.engine.RepositoryService().GetBpmnModel(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, nil, err
	}
	if len(bpmn.Processes) == 0 {
		return nil, nil, api.NewError(errCodeSimDefMissing, "流程定义不包含流程", nil)
	}
	process := bpmn.Processes[0]
	cands := simCollectNext(process, task.TaskDefinitionKey, entityMap, true)
	return process, cands, nil
}

// simCandidate 是静态图遍历产生的候选节点。
type simCandidate struct {
	activityID   string // 节点 Id（保留代号 end 为伪节点）
	activityName string // 节点名称
	isEnd        bool   // 是否"办结"伪节点
	isCall       bool   // 是否调用活动边界（不深入子定义）
	multiInst    bool   // 是否会签（多实例）节点
}

// simScope 是遍历中的容器视图（主流程或子流程），统一元素访问。
type simScope struct {
	elements map[string]model.FlowElement // 元素表
	order    []string                     // 文档顺序
	isSub    bool                         // 是否子流程容器
	id       string                       // 容器元素 Id（主流程为流程 Id）
}

// simRootScope 构造主流程容器视图。
// @param process 流程
// @return 容器视图
func simRootScope(process *model.Process) *simScope {
	return &simScope{elements: process.Elements, order: process.Order, id: process.ID}
}

// simSubScope 构造子流程容器视图。
// @param sub 子流程
// @return 容器视图
func simSubScope(sub *model.SubProcess) *simScope {
	return &simScope{elements: sub.Elements, order: sub.Order, isSub: true, id: sub.ID}
}

// lookup 在当前容器查找元素，未命中回退主流程（与 Java getFlowElementById 的递归查找等价）。
// @param root 主流程
// @param id 元素 Id
// @return 元素
// @return 所在容器
// @return 是否找到
func (sc *simScope) lookup(root *model.Process, id string) (model.FlowElement, *simScope, bool) {
	if element, ok := sc.elements[id]; ok {
		return element, sc, true
	}
	// 回退主流程容器（子流程内出线一般指向同容器元素，此处兜底跨容器引用）
	if !sc.isSub {
		return nil, sc, false
	}
	if element, ok := root.Elements[id]; ok {
		return element, simRootScope(root), true
	}
	return nil, sc, false
}

// simCollectFromStarts 从开始事件遍历（getNextUserTasksWithoutEndNode 的按 Key 推演入口）。
// @param process 流程
// @param vars 连线条件求值集合
// @param includeEnd 是否包含办结伪节点
// @return 候选节点列表
func simCollectFromStarts(process *model.Process, vars map[string]any, includeEnd bool) []simCandidate {
	cands := make([]simCandidate, 0, 4)
	visited := map[string]bool{}
	for _, start := range process.StartEvents() {
		simWalk(process, simRootScope(process), start.ID, vars, includeEnd, visited, &cands)
	}
	return cands
}

// simCollectNext 从指定节点遍历静态图收集下一步候选（FlowUtils.next 对位）：
// 停在用户任务与调用活动；穿网关（排他/并行/包容）、服务/接收任务、子流程边界；
// 主流程结束事件产出"办结"伪节点；子流程结束事件上溯到子流程出线继续。
// @param process 流程
// @param startID 起始节点 Id
// @param vars 连线条件求值集合
// @param includeEnd 是否包含办结伪节点
// @return 候选节点列表
func simCollectNext(process *model.Process, startID string, vars map[string]any, includeEnd bool) []simCandidate {
	cands := make([]simCandidate, 0, 4)
	simWalk(process, simRootScope(process), startID, vars, includeEnd, map[string]bool{}, &cands)
	return cands
}

// simWalk 静态图遍历的深度优先递归实现。
// @param process 主流程
// @param scope 当前容器（主流程或子流程）
// @param nodeID 当前节点 Id
// @param vars 连线条件求值集合
// @param includeEnd 是否包含办结伪节点
// @param visited 已访问集合（防环）
// @param cands 候选收集目标
func simWalk(process *model.Process, scope *simScope, nodeID string, vars map[string]any, includeEnd bool, visited map[string]bool, cands *[]simCandidate) {
	// 防止环形图导致的无限递归（Java 无防环，此处增强）
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true
	element, container, ok := scope.lookup(process, nodeID)
	if !ok {
		return
	}
	switch typed := element.(type) {
	case *model.UserTask:
		// 用户任务：收集并停止（Java next() 对 UserTask 不继续递归）
		*cands = append(*cands, simCandidate{
			activityID:   typed.ID,
			activityName: typed.Name,
			multiInst:    typed.LoopCharacteristics != nil,
		})
		return
	case *model.CallActivity:
		// 调用活动：标记为边界节点，不深入子定义（行为差异见报告：Java 会继续穿出线）
		*cands = append(*cands, simCandidate{activityID: typed.ID, activityName: typed.Name, isCall: true})
		return
	case *model.EndEvent:
		if container.isSub {
			// 子流程结束事件：上溯到子流程元素，沿其出线继续（Java getSubProcess 语义）
			simWalk(process, simRootScope(process), container.id, vars, includeEnd, visited, cands)
			return
		}
		// 主流程结束事件：产出"办结"伪节点（Java next() 语义），结束事件无出线自然终止
		if includeEnd {
			*cands = append(*cands, simCandidate{activityID: SimEndActivityID, activityName: SimEndActivityName, isEnd: true})
		}
		return
	case *model.SubProcess:
		// 子流程元素作为遍历入口（仅上溯场景到达此处）：沿其出线继续
		simTakeOutgoing(process, container, typed, vars, includeEnd, visited, cands)
		return
	}
	node, isNode := element.(model.FlowNode)
	if !isNode {
		return
	}
	simTakeOutgoing(process, container, node, vars, includeEnd, visited, cands)
}

// simTakeOutgoing 按节点类型选择出线并继续遍历：
// 并行网关全部出线；排他/包容网关条件剪枝 + 默认流兜底；其余节点按条件逐条判定。
// @param process 主流程
// @param container 当前容器
// @param node 当前节点
// @param vars 连线条件求值集合
// @param includeEnd 是否包含办结伪节点
// @param visited 已访问集合
// @param cands 候选收集目标
func simTakeOutgoing(process *model.Process, container *simScope, node model.FlowNode, vars map[string]any, includeEnd bool, visited map[string]bool, cands *[]simCandidate) {
	flows := node.GetOutgoing()
	if len(flows) == 0 {
		return
	}
	switch gateway := node.(type) {
	case *model.ParallelGateway:
		// 并行网关：各分支递归，忽略条件
		for _, flow := range flows {
			simFollow(process, container, flow, vars, includeEnd, visited, cands)
		}
		return
	case *model.ExclusiveGateway:
		// 排他网关：条件为真的出线；全假时取默认流；无默认流取无条件出线
		simFollowGateway(process, container, gateway.DefaultFlow, flows, vars, includeEnd, visited, cands)
		return
	case *model.InclusiveGateway:
		// 包容网关：全部条件为真的出线；全真为零时与排他同规则兜底
		simFollowGateway(process, container, gateway.DefaultFlow, flows, vars, includeEnd, visited, cands)
		return
	}
	// 其余节点（开始/服务/接收/中间事件等）：无条件或条件为真的出线逐条递归
	for _, flow := range flows {
		// 条件剪枝：空白条件直通，条件表达式在 entityMap 上求值为 true 才放行
		if !simConditionTrue(flow, vars) {
			continue
		}
		simFollow(process, container, flow, vars, includeEnd, visited, cands)
	}
}

// simFollowGateway 网关出线选择：先取条件为真的出线，全假时回退默认流，再回退无条件出线。
// @param process 主流程
// @param container 当前容器
// @param defaultFlowID 默认出线 Id（空表示未设置）
// @param flows 出线列表
// @param vars 连线条件求值集合
// @param includeEnd 是否包含办结伪节点
// @param visited 已访问集合
// @param cands 候选收集目标
func simFollowGateway(process *model.Process, container *simScope, defaultFlowID string, flows []*model.SequenceFlow, vars map[string]any, includeEnd bool, visited map[string]bool, cands *[]simCandidate) {
	matched := make([]*model.SequenceFlow, 0, 1)
	fallbacks := make([]*model.SequenceFlow, 0, 1)
	var defaultFlow *model.SequenceFlow
	for _, flow := range flows {
		// 默认流记录兜底位（不参与条件判定）
		if (defaultFlowID != "" && flow.ID == defaultFlowID) || flow.IsDefault {
			defaultFlow = flow
			continue
		}
		if flow.Condition == "" {
			fallbacks = append(fallbacks, flow)
			continue
		}
		// 条件求值剪枝
		if simConditionTrue(flow, vars) {
			matched = append(matched, flow)
		}
	}
	if len(matched) > 0 {
		for _, flow := range matched {
			simFollow(process, container, flow, vars, includeEnd, visited, cands)
		}
		return
	}
	// 默认流兜底（BPMN 运行时语义），无默认流再取无条件出线
	if defaultFlow != nil {
		simFollow(process, container, defaultFlow, vars, includeEnd, visited, cands)
		return
	}
	for _, flow := range fallbacks {
		simFollow(process, container, flow, vars, includeEnd, visited, cands)
	}
}

// simFollow 沿一条出线取目标节点并继续遍历（子流程目标进入其首个开始事件）。
// @param process 主流程
// @param container 当前容器
// @param flow 出线
// @param vars 连线条件求值集合
// @param includeEnd 是否包含办结伪节点
// @param visited 已访问集合
// @param cands 候选收集目标
func simFollow(process *model.Process, container *simScope, flow *model.SequenceFlow, vars map[string]any, includeEnd bool, visited map[string]bool, cands *[]simCandidate) {
	element, scope, ok := container.lookup(process, flow.TargetRef)
	if !ok {
		return
	}
	// 子流程边界：进入其首个开始事件（Java getFlowElementById 返回子流程开始节点）
	if sub, isSub := element.(*model.SubProcess); isSub {
		for _, start := range simSubScope(sub).startEvents() {
			simWalk(process, scope, start.ID, vars, includeEnd, visited, cands)
		}
		return
	}
	simWalk(process, scope, flow.TargetRef, vars, includeEnd, visited, cands)
}

// startEvents 返回容器视图内的开始事件列表（子流程容器用）。
// @param sc 容器视图
// @return 开始事件列表
func (sc *simScope) startEvents() []*model.StartEvent {
	events := make([]*model.StartEvent, 0, 1)
	for _, id := range sc.order {
		if event, ok := sc.elements[id].(*model.StartEvent); ok {
			events = append(events, event)
		}
	}
	return events
}

// simConditionTrue 判定出线条件是否放行（FlowUtils.result 语义适配）：
// 空白条件直通；${...}/#{...}/裸表达式经 expr 在变量集合上求值；
// 求值失败或非布尔结果按 Java Boolean.valueOf(String.valueOf(...)) 宽松转换（"true" 为真）。
// @param flow 出线
// @param vars 变量集合
// @return 是否放行
func simConditionTrue(flow *model.SequenceFlow, vars map[string]any) bool {
	// 空白条件直通（Java StringUtils.isBlank(expression) 分支）
	if strings.TrimSpace(flow.Condition) == "" {
		return true
	}
	value, err := expr.Eval(flow.Condition, vars)
	if err != nil {
		// 求值失败剪枝（Java 抛 BusinessException 中断整条出线遍历，Go 降级为该线不通，见报告差异）
		return false
	}
	// 非布尔结果宽松转换：Boolean.valueOf(String.valueOf(result)) 语义
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return strings.EqualFold(strings.TrimSpace(expr.ToString(value)), "true")
}

// resolveCandidates 将候选节点解析为推演视图（执行人解析 + 排序 + 姓名补全）。
// @param ctx 操作上下文
// @param process 流程模型
// @param definitionID 流程定义 Id
// @param deploymentID 部署 Id（his_step 排序定位）
// @param processInstanceID 流程实例 Id（空表示未发起，跳过历史执行人来源）
// @param startUserId 发起人身份 Id（${initiator} 解析）
// @param cands 候选节点
// @param entityMap 变量集合（${xxx} 执行人取值）
// @param sorted 是否按 sort + 节点 ID 排序（模式 3/4 由调用方自行合成，传 false）
// @return 推演视图列表
// @return 错误
func (s *SimulateService) resolveCandidates(ctx context.Context, process *model.Process, definitionID, deploymentID, processInstanceID, startUserId string, cands []simCandidate, entityMap map[string]any, sorted bool) ([]NextUserTaskView, error) {
	views := make([]NextUserTaskView, 0, len(cands))
	for _, cand := range cands {
		view := NextUserTaskView{ActivityID: cand.activityID, ActivityName: cand.activityName}
		// 办结伪节点：无执行人解析，排序号取 0（Java hisStepEntity 为 null 语义）
		if cand.isEnd {
			views = append(views, view)
			continue
		}
		// 来源 1（运行时校正）：his_next_activity 该实例该节点历史执行人优先
		hisIDs, err := s.historyExecutorsOf(ctx, processInstanceID, cand.activityID)
		if err != nil {
			return nil, err
		}
		if len(hisIDs) > 0 {
			view.ExecutorIDs = hisIDs
			view.ExecutorNames = s.executorNames(ctx, hisIDs)
		} else {
			// 来源 2：模型元数据解析（assignee 表达式/candidateUsers/candidateGroups/executorList）
			ids, dynamicRoles := s.resolveModelExecutors(ctx, process, definitionID, cand, entityMap, startUserId)
			view.ExecutorIDs = ids
			view.DynamicRoleIDs = dynamicRoles
			view.ExecutorNames = s.executorNames(ctx, ids)
		}
		// 步骤排序号（his_step 缺失为 0）
		if s.steps != nil {
			if rule, found, err := s.steps.GetByDeploymentAndActivity(ctx, deploymentID, cand.activityID); err == nil && found {
				view.Sort = rule.Sort
			}
		}
		views = append(views, view)
	}
	if sorted {
		// 按排序号升序、同号按节点 Id（Java 按 sort 排序 + 稳定序增强）
		sort.SliceStable(views, func(i, j int) bool {
			if views[i].Sort != views[j].Sort {
				return views[i].Sort < views[j].Sort
			}
			return views[i].ActivityID < views[j].ActivityID
		})
	}
	return views, nil
}

// resolveModelExecutors 按模型配置解析候选节点执行人（对位 Java 执行人解析链）。
// 优先级：${executor}（executorType+executorList）→ ${initiator} → 其他 ${xxx} 取变量 →
// 字面 assignee → candidateUsers 直用 → candidateGroups 展开成员。
// @param ctx 操作上下文
// @param process 流程模型
// @param definitionID 流程定义 Id
// @param cand 候选节点
// @param entityMap 变量集合
// @param startUserId 发起人身份 Id
// @return 执行人身份 Id 列表
// @return 动态角色 Id 列表（需业务系统二次解析）
func (s *SimulateService) resolveModelExecutors(ctx context.Context, process *model.Process, definitionID string, cand simCandidate, entityMap map[string]any, startUserId string) ([]string, []string) {
	meta := simMetaOf(process, cand)
	if meta == nil {
		return nil, nil
	}
	// ${executor} 表达式（子流程/会签）：走 executorType+executorList 配置展开
	if meta.Assignee == "${executor}" || (len(meta.CandidateGroups) > 0 && meta.CandidateGroups[0] == "${executor}") {
		// 节点自身无配置时回退所在子流程扩展（Java getSubProcessUserTaskEntities 语义）
		configMeta := meta
		if configMeta.ExecutorListRaw == "" && configMeta.ExecutorType == "" {
			configMeta = simParentMetaOf(process, cand.activityID, configMeta)
		}
		return s.expandExecutorConfigs(ctx, configMeta, entityMap, startUserId)
	}
	// assignee 表达式分支
	if assignee := strings.TrimSpace(meta.Assignee); assignee != "" {
		if strings.HasPrefix(assignee, "${") && strings.HasSuffix(assignee, "}") {
			// ${initiator} → 发起人
			if assignee == "${"+VarInitiator+"}" {
				if startUserId == "" {
					return nil, nil
				}
				return []string{startUserId}, nil
			}
			// 其他 ${xxx}：从表单/变量集合取值（entity. 前缀剥除，Java 同名语义）
			key := strings.TrimSuffix(strings.TrimPrefix(assignee, "${"), "}")
			// 带容器前缀（entity.xxx）时剥除，取业务字段名
			if index := strings.Index(key, "."); index >= 0 {
				key = key[index+1:]
			}
			// 变量缺失按无执行人处理（Java cast 失败同样产出空）
			if value := entityMapValueString(entityMap, key); value != "" {
				return []string{value}, nil
			}
			return nil, nil
		}
		// 字面身份 Id 直用（Java accountIdentityService.getById 的 Go 等价透传）
		return []string{assignee}, nil
	}
	// candidateUsers 直用
	if len(meta.CandidateUsers) > 0 {
		return distinctNonEmpty(meta.CandidateUsers), nil
	}
	// candidateGroups 展开成员（org 直取成员；展开失败视为动态角色收集标记）
	if len(meta.CandidateGroups) > 0 {
		return s.expandGroups(ctx, meta.CandidateGroups)
	}
	return nil, nil
}

// expandExecutorConfigs 展开 executorType+executorList 配置为身份列表
// （person 直用 / org 展开成员 / role 展开成员或收集动态角色标记）。
// @param ctx 操作上下文
// @param meta 节点元数据（含 ExecutorType 与 ExecutorListRaw）
// @param entityMap 变量集合（预留：动态角色路径取值）
// @param startUserId 发起人身份 Id（预留：发起人角色）
// @return 执行人身份 Id 列表
// @return 动态角色 Id 列表
func (s *SimulateService) expandExecutorConfigs(ctx context.Context, meta *UserTaskMeta, entityMap map[string]any, startUserId string) ([]string, []string) {
	_ = entityMap
	_ = startUserId
	configs := ParseExecutorConfigs(meta.ExecutorListRaw)
	ids := make([]string, 0, len(configs))
	dynamic := make([]string, 0, 2)
	for _, config := range configs {
		// 条目类型缺失回退节点级 executorType，再回退 person（与 countersign 一致）
		executorType := config.ExecutorType
		if executorType == "" {
			executorType = meta.ExecutorType
		}
		switch executorType {
		case "org":
			// 组织：展开成员；失败保留组织 Id 原值（countersign.expandGroupMembers 兜底语义）
			ids = append(ids, s.expandGroupOrKeep(ctx, config.OrgID)...)
		case "role":
			// 角色：展开成员；无法展开的角色收集为动态角色标记（Java dynamicRoles 语义适配）
			if members, ok := s.groupMembers(ctx, config.RoleID); ok && len(members) > 0 {
				ids = append(ids, members...)
			} else if config.RoleID != "" {
				dynamic = append(dynamic, config.RoleID)
			}
		default:
			// person 类型直用身份 Id
			ids = append(ids, config.IdentityID)
		}
	}
	return distinctNonEmpty(ids), distinctNonEmpty(dynamic)
}

// expandGroups 展开候选组列表为成员身份列表。
// 组织类组直取成员；查询失败或无成员的组按动态角色收集（Java getIdentityIdsByGroupIds 的
// org→role 回退 + 动态角色收集语义适配）。
// @param ctx 操作上下文
// @param groupIDs 组 Id 列表
// @return 成员身份 Id 列表
// @return 动态角色 Id 列表
func (s *SimulateService) expandGroups(ctx context.Context, groupIDs []string) ([]string, []string) {
	ids := make([]string, 0, 4)
	dynamic := make([]string, 0, 2)
	for _, groupID := range groupIDs {
		// ${executor} 已在上游分支处理，此处跳过表达式形态
		if strings.HasPrefix(groupID, "${") {
			continue
		}
		if members, ok := s.groupMembers(ctx, groupID); ok && len(members) > 0 {
			ids = append(ids, members...)
			continue
		}
		// 无法静态展开的组 → 动态角色标记，供前端二次确认
		dynamic = append(dynamic, groupID)
	}
	return distinctNonEmpty(ids), distinctNonEmpty(dynamic)
}

// groupMembers 查询组成员身份列表。
// @param ctx 操作上下文
// @param groupID 组 Id
// @return 成员身份 Id 列表
// @return 是否查询成功（身份服务未注入或查询失败为否）
func (s *SimulateService) groupMembers(ctx context.Context, groupID string) ([]string, bool) {
	if s.identity == nil || groupID == "" {
		return nil, false
	}
	users, err := s.identity.ListMembers(ctx, groupID)
	if err != nil {
		return nil, false
	}
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	return ids, true
}

// expandGroupOrKeep 展开组成员，失败时保留组 Id 原值（与 countersign.expandGroupMembers 一致）。
// @param ctx 操作上下文
// @param groupID 组 Id
// @return 身份 Id 列表
func (s *SimulateService) expandGroupOrKeep(ctx context.Context, groupID string) []string {
	if groupID == "" {
		return nil
	}
	if members, ok := s.groupMembers(ctx, groupID); ok && len(members) > 0 {
		return members
	}
	return []string{groupID}
}

// executorNames 补全执行人姓名（查不到回退 Id 本身）。
// @param ctx 操作上下文
// @param ids 身份 Id 列表
// @return 姓名列表（与 ids 按下标对应）
func (s *SimulateService) executorNames(ctx context.Context, ids []string) []string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		name := id
		if s.identity != nil {
			if user, err := s.identity.GetUser(ctx, id); err == nil && user.DisplayName != "" {
				name = user.DisplayName
			}
		}
		names = append(names, name)
	}
	return names
}

// simMetaOf 提取候选节点的用户任务元数据（用户任务取模型；调用活动由扩展属性合成）。
// @param process 流程模型
// @param cand 候选节点
// @return 元数据（无法提取返回 nil）
func simMetaOf(process *model.Process, cand simCandidate) *UserTaskMeta {
	element, ok := findFlowElement(process, cand.activityID)
	if !ok {
		return nil
	}
	if task, isTask := element.(*model.UserTask); isTask {
		return userTaskMetaOf(task)
	}
	// 调用活动边界：扩展属性合成执行人配置（executorType/executorList）
	if call, isCall := element.(*model.CallActivity); isCall {
		extensions := extensionsOfNode(call)
		meta := &UserTaskMeta{ActivityID: call.ID, ActivityName: call.Name}
		if extensions != nil {
			meta.ExecutorType = extensions.Attr("executorType")
			meta.ExecutorListRaw = extensions.Attr("executorList")
		}
		return meta
	}
	return nil
}

// simParentMetaOf 查找候选节点所在子流程的执行人配置
// （Java getSubProcessUserTaskEntities：子流程会签属性挂在子流程多实例配置上）。
// @param process 流程模型
// @param activityID 候选节点 Id
// @param fallback 节点自身元数据（找不到子流程时原样返回）
// @return 子流程元数据或原元数据
func simParentMetaOf(process *model.Process, activityID string, fallback *UserTaskMeta) *UserTaskMeta {
	sub, ok := simContainerOf(process, activityID)
	if !ok {
		return fallback
	}
	extensions := extensionsOfNode(sub)
	meta := &UserTaskMeta{ActivityID: sub.ID, ActivityName: sub.Name}
	if extensions != nil {
		meta.ExecutorType = extensions.Attr("executorType")
		meta.ExecutorListRaw = extensions.Attr("executorList")
	}
	// 子流程也未配置时维持节点自身配置
	if meta.ExecutorListRaw == "" && meta.ExecutorType == "" {
		return fallback
	}
	return meta
}

// simContainerOf 查找元素所在的直接子流程容器（仅一层子流程归属判定）。
// @param process 流程模型
// @param activityID 元素 Id
// @return 子流程
// @return 是否找到
func simContainerOf(process *model.Process, activityID string) (*model.SubProcess, bool) {
	for _, id := range process.Order {
		if sub, ok := process.Elements[id].(*model.SubProcess); ok {
			if _, inside := sub.Elements[activityID]; inside {
				return sub, true
			}
			// 嵌套子流程递归下探
			if found := simContainerInSub(sub, activityID); found != nil {
				return found, true
			}
		}
	}
	return nil, false
}

// simContainerInSub 在子流程（含嵌套）中查找元素所在容器。
// @param sub 子流程
// @param activityID 元素 Id
// @return 子流程
func simContainerInSub(sub *model.SubProcess, activityID string) *model.SubProcess {
	for _, id := range sub.Order {
		if nested, ok := sub.Elements[id].(*model.SubProcess); ok {
			if _, inside := nested.Elements[activityID]; inside {
				return nested
			}
			if found := simContainerInSub(nested, activityID); found != nil {
				return found
			}
		}
	}
	return nil
}

// SimulateViewString 推演视图的调试摘要（节点[执行人]）。
// @param view 推演视图
// @return 摘要文本
func SimulateViewString(view NextUserTaskView) string {
	return fmt.Sprintf("%s(%s)", view.ActivityID, strings.Join(view.ExecutorIDs, ","))
}
