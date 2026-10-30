package workflow

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 退回域错误码（WORKFLOW-24xx 段，对位 Java generalBack/backTask/getBackNodesByTaskId 的
// BusinessException 文案）。
const (
	errCodeBackTaskIDBlank   = "WORKFLOW-2410" // 参数 taskId 为空
	errCodeBackUserBlank     = "WORKFLOW-2411" // 参数 userCode 为空
	errCodeBackTargetBlank   = "WORKFLOW-2412" // 参数 targetActivityId 为空
	errCodeBackTaskMissing   = "WORKFLOW-2413" // 任务不存在
	errCodeBackNotAllowed    = "WORKFLOW-2414" // 当前步骤不允许退回
	errCodeBackSuspended     = "WORKFLOW-2415" // 流程已暂停，请联系管理员激活
	errCodeBackNodeMissing   = "WORKFLOW-2416" // 当前任务节点不存在于模型
	errCodeBackMultiIncoming = "WORKFLOW-2417" // 上一节点不唯一，不允许退回上一节点
	errCodeBackNoIncoming    = "WORKFLOW-2418" // 当前节点没有入线，无法退回上一节点
	errCodeBackHistoryQuery  = "WORKFLOW-2419" // 历史数据查询失败
)

// backHistoryPageSize 是退回链路查询引擎历史数据的批量上限（单实例历史活动/任务量级
// 远低于该值，与 complete.go completeListSize 同一口径）。
const backHistoryPageSize = 1000

// BackNodeView 是可退回节点视图（对位 Java FlowNodeVo 的退回场景字段子集）。
type BackNodeView struct {
	ActivityID    string   // 节点 ID（BPMN userTask id）
	ActivityName  string   // 节点名称
	TaskIDs       []string // 该节点历史任务 ID 列表（多实例节点为全部实例任务）
	AssigneeNames []string // 办理人列表（多实例节点收集全部历史办理人）
	Sort          int      // 步骤排序号（his_step.sort，未配置为 0，仅作展示排序）
}

// ModelStore 是流程模型主数据（workflow_model）的只读端口，供退回祖先链读取
// returnSort（退回排序方式）。为 nil 时 BackNodes 跳过祖先链过滤（对位 Java
// 未启用模型主数据时的降级形态）。
type ModelStore interface {
	// ReturnSortOf 读取定义 Key 对应模型的退回排序方式。
	// 对位 Java getReturnSortByProcessDefinitionKey：模型缺失/字段为空/为 1 返回 true，
	// 为 0 返回 false（true 时父链逆序输出，即最远祖先在前）。
	// @param ctx 操作上下文
	// @param processDefinitionKey 流程定义 Key
	// @return 退回排序方式
	// @return 查询错误
	ReturnSortOf(ctx context.Context, processDefinitionKey string) (bool, error)
}

// MemModelStore 是 ModelStore 的内存实现（单测与嵌入式无 DB 模式）。
type MemModelStore struct {
	mu    sync.RWMutex    // 读写锁
	flags map[string]bool // 定义 Key → returnSort
}

// NewMemModelStore 创建内存模型主数据存储。
// @return 内存存储
func NewMemModelStore() *MemModelStore {
	return &MemModelStore{flags: make(map[string]bool)}
}

// PutReturnSort 写入/覆盖定义 Key 的退回排序方式（测试播种用）。
// @param processDefinitionKey 流程定义 Key
// @param returnSort 退回排序方式
func (m *MemModelStore) PutReturnSort(processDefinitionKey string, returnSort bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flags[processDefinitionKey] = returnSort
}

// ReturnSortOf 读取定义 Key 的退回排序方式（未播种返回 true，对位 Java 缺省升序）。
// @param ctx 操作上下文
// @param processDefinitionKey 流程定义 Key
// @return 退回排序方式
// @return 查询错误
func (m *MemModelStore) ReturnSortOf(_ context.Context, processDefinitionKey string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if flag, ok := m.flags[processDefinitionKey]; ok {
		return flag, nil
	}
	// Java：模型行缺失/字段为空 → true（缺省排序形态）
	return true, nil
}

// 编译期断言：内存实现满足端口。
var _ ModelStore = (*MemModelStore)(nil)

// BackService 是任意退回域服务（P2-B）。
// 行为规格对位 Java ProcessesUtil#generalBack:2557 / getBackNodesByTaskId:2673 /
// backTask:4502：五道过滤链计算可退节点 → 预存目标执行人 → allowReturn/暂停校验 →
// 引擎级任意退回（BackUserTaskCmd 对位）→ TH 意见/操作记录/预存清理/待办账本收尾。
type BackService struct {
	engine     api.Engine              // 引擎门面（七服务）
	meta       *ModelMetaService       // 模型元数据（BPMN 模型读取）
	next       *NextActivityService    // 下一步预生成-消耗记账
	comments   *CommentService         // 审批意见
	operations *OperationRecordService // 操作记录
	messages   *MessageExtendService   // 待办扩展
	histories  *HistoryExtendService   // 已办扩展
	documents  *DocumentService        // 业务表单状态机
	steps      *StepRuleService        // 步骤规则（his_step 端口）
	instances  InstanceQuery           // 实例查询（挂起校验）
	models     ModelStore              // 模型主数据端口（returnSort），nil 跳过祖先链过滤
	users      UserQuery               // 身份查询（办理人姓名展示），nil 时保留身份 Id
	clock      clock.Clock             // 时间源
}

// BackOption 是退回服务选项函数。
type BackOption func(*BackService)

// WithBackModels 注入模型主数据端口（workflow_model.returnSort 读取）。
// @param models 模型主数据端口
// @return 选项
func WithBackModels(models ModelStore) BackOption {
	return func(s *BackService) { s.models = models }
}

// WithBackUsers 注入身份查询（办理人姓名展示）。
// @param users 身份查询
// @return 选项
func WithBackUsers(users UserQuery) BackOption {
	return func(s *BackService) { s.users = users }
}

// NewBackService 构造任意退回域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param next 下一步预生成域服务
// @param comments 审批意见域服务
// @param operations 操作记录域服务
// @param messages 待办扩展域服务
// @param histories 已办扩展域服务
// @param documents 业务表单状态域服务
// @param steps 步骤规则服务（his_step 端口）
// @param instances 实例查询（挂起校验）
// @param c 时间源，nil 时回退真实时钟
// @param options 可选配置（模型主数据端口、身份查询）
// @return 退回域服务
func NewBackService(engine api.Engine, meta *ModelMetaService, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService, steps *StepRuleService, instances InstanceQuery, c clock.Clock, options ...BackOption) *BackService {
	if c == nil {
		c = clock.Real{}
	}
	s := &BackService{
		engine: engine, meta: meta, next: next, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents,
		steps: steps, instances: instances, clock: c,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// ─── 可退节点计算（getBackNodesByTaskId:2673 对位） ─────────────────

// BackNodes 计算当前任务的可退回节点列表（五道过滤链）：
// ①hisStep allowReturn=="0" → 空列表；②数据源=引擎已完成历史活动实例；
// ③过滤 userTask/非当前节点/按节点聚合；④祖先链（his_next_activity preTaskId 链，
// ModelStore 未注入时跳过）；⑤backStepCodes 白名单；⑥可达性（经引擎 BPMN 模型）。
// 多实例节点收集全部历史办理人；结果按步骤排序号升序。
// @param ctx 操作上下文
// @param taskID 当前任务 ID
// @return 可退回节点列表（任务不存在或步骤禁止退回时为空列表）
// @return 错误
func (s *BackService) BackNodes(ctx context.Context, taskID string) ([]BackNodeView, error) {
	// Java：taskId 为空 → 空列表
	if strings.TrimSpace(taskID) == "" {
		return []BackNodeView{}, nil
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		// Java：任务不存在 → 空列表（不报错）
		if errors.Is(err, api.ErrNotFound) {
			return []BackNodeView{}, nil
		}
		return nil, err
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, err
	}
	// 过滤链①：当前节点 hisStep allowReturn=="0" → 空列表（第一道开关）
	allowed, err := s.steps.AllowReturn(ctx, definition.DeploymentID, task.TaskDefinitionKey)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return []BackNodeView{}, nil
	}
	process, err := s.processOf(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, err
	}
	currentElement, found := findFlowElement(process, task.TaskDefinitionKey)
	if !found {
		return nil, api.NewError(errCodeBackNodeMissing, "当前任务节点不存在于模型", api.ErrNotFound)
	}
	currentNode, isNode := currentElement.(model.FlowNode)
	if !isNode {
		return nil, api.NewError(errCodeBackNodeMissing, "当前任务节点不是流程节点", api.ErrBadRequest)
	}
	// 过滤链②数据源：实例已完成（finished）历史活动
	activities, err := s.finishedActivities(ctx, task.ProcessInstanceID)
	if err != nil {
		return nil, err
	}
	// 过滤链③：userTask + 非当前节点，按节点聚合任务 ID（多实例节点多行合并）
	type candidate struct {
		name    string
		taskIDs []string
	}
	candidates := make(map[string]*candidate)
	for i := range activities {
		activity := activities[i]
		// Java：activityType==userTask 且 activityId != 当前节点
		if activity.ActivityType != "userTask" || activity.ActivityID == task.TaskDefinitionKey {
			continue
		}
		item, ok := candidates[activity.ActivityID]
		if !ok {
			item = &candidate{name: activity.ActivityName}
			candidates[activity.ActivityID] = item
		}
		// 同节点多实例的任务 ID 全量收集
		if activity.TaskID != "" {
			item.taskIDs = append(item.taskIDs, activity.TaskID)
		}
	}
	// 过滤链④：祖先链（ModelStore 未注入时跳过；父链非空时仅保留命中父链的活动）
	if s.models != nil {
		returnSort, err := s.models.ReturnSortOf(ctx, process.ID)
		if err != nil {
			return nil, err
		}
		parentTaskIDs, err := s.parentTaskIDs(ctx, taskID, returnSort)
		if err != nil {
			return nil, err
		}
		// Java：父链非空时仅保留历史任务 ID 命中父链的活动
		if len(parentTaskIDs) > 0 {
			parentSet := make(map[string]bool, len(parentTaskIDs))
			for _, id := range parentTaskIDs {
				parentSet[id] = true
			}
			for id, item := range candidates {
				kept := false
				for _, taskIDOfActivity := range item.taskIDs {
					if parentSet[taskIDOfActivity] {
						kept = true
						break
					}
				}
				if !kept {
					delete(candidates, id)
				}
			}
		}
	}
	// 过滤链⑤：当前节点 backStepCodes 白名单（非空时仅白名单内节点可退）
	backStepCodes, err := s.steps.BackStepCodes(ctx, definition.DeploymentID, task.TaskDefinitionKey)
	if err != nil {
		return nil, err
	}
	whitelist := make(map[string]bool, len(backStepCodes))
	for _, code := range backStepCodes {
		whitelist[code] = true
	}
	// 步骤排序号索引（视图排序用，未配置节点取 0；最终顺序由排序号+节点 ID 决定）
	sortIndex, err := s.stepSortIndex(ctx, definition.DeploymentID)
	if err != nil {
		return nil, err
	}
	views := make([]BackNodeView, 0, len(candidates))
	for activityID, item := range candidates {
		// 过滤链⑤：白名单非空且未命中 → 跳过
		if len(whitelist) > 0 && !whitelist[activityID] {
			continue
		}
		// 过滤链⑥：候选节点沿模型可达当前节点（Java FlowUtils.isReachable(process, toBack, current)）
		element, ok := findFlowElement(process, activityID)
		if !ok {
			continue
		}
		toBackNode, isNode := element.(model.FlowNode)
		if !isNode || !isNodeReachable(process, toBackNode, currentNode) {
			continue
		}
		view := BackNodeView{
			ActivityID:   activityID,
			ActivityName: firstNonEmpty(item.name, nodeDisplayName(toBackNode)),
			TaskIDs:      append([]string(nil), item.taskIDs...),
			Sort:         sortIndex[activityID],
		}
		// 办理人收集：多实例节点按历史任务全量汇总，普通节点即该节点办理人
		view.AssigneeNames = s.assigneeNamesOf(ctx, item.taskIDs)
		views = append(views, view)
	}
	// 结果按步骤排序号升序、同号按节点 ID，保证顺序确定
	sort.Slice(views, func(i, j int) bool {
		if views[i].Sort != views[j].Sort {
			return views[i].Sort < views[j].Sort
		}
		return views[i].ActivityID < views[j].ActivityID
	})
	return views, nil
}

// processOf 查询定义对应的第一个流程模型（经引擎 GetBpmnModel）。
// @param ctx 操作上下文
// @param definitionID 流程定义 ID
// @return 流程模型
// @return 错误
func (s *BackService) processOf(ctx context.Context, definitionID string) (*model.Process, error) {
	bpmn, err := s.engine.RepositoryService().GetBpmnModel(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	if len(bpmn.Processes) == 0 {
		return nil, api.NewError(errCodeBackNodeMissing, "流程定义不包含流程", api.ErrNotFound)
	}
	return bpmn.Processes[0], nil
}

// finishedActivities 查询实例全部已完成历史活动。
// @param ctx 操作上下文
// @param processInstanceID 实例 ID
// @return 历史活动列表
// @return 错误
func (s *BackService) finishedActivities(ctx context.Context, processInstanceID string) ([]api.HistoricActivityInstance, error) {
	finished := true
	page, err := s.engine.HistoryService().QueryHistoricActivityInstances(ctx, api.HistoricActivityInstanceFilter{
		ProcessInstanceID: processInstanceID,
		Finished:          &finished,
		Size:              backHistoryPageSize,
	})
	if err != nil {
		return nil, api.NewError(errCodeBackHistoryQuery, "历史活动查询失败", err)
	}
	return page.Data, nil
}

// parentTaskIDs 沿 his_next_activity 的 preTaskId 链收集祖先任务 ID。
// 对位 Java WorkflowHisNextActivityServiceImpl#getParentTaskIds：每层取该任务
// 消耗记录（task_id=当前任务，非自环）中创建时间最早的一条，其 pre_task_id 即父级；
// returnSort=true 时结果逆序（最远祖先在前，Java 降序分支），false 时最近父级在前。
// @param ctx 操作上下文
// @param taskID 当前任务 ID
// @param returnSort 退回排序方式（true 逆序）
// @return 祖先任务 ID 列表（无祖先返回空列表）
// @return 错误
func (s *BackService) parentTaskIDs(ctx context.Context, taskID string, returnSort bool) ([]string, error) {
	chain := make([]string, 0, 4)
	visited := map[string]bool{taskID: true}
	current := taskID
	for {
		records, err := s.next.ListHisByTaskID(ctx, current)
		if err != nil {
			return nil, err
		}
		// Java：list 为空或首条无 preTaskId → 链终止
		if len(records) == 0 || strings.TrimSpace(records[0].PreTaskID) == "" {
			break
		}
		parent := records[0].PreTaskID
		// 防环：父级已出现在链上（Java 无此防护，数据成环时 Java 栈溢出，Go 显式终止）
		if visited[parent] {
			break
		}
		visited[parent] = true
		chain = append(chain, parent)
		current = parent
	}
	// Java：returnSort=false 按层升序（最近父级在前），true 逆序（最远祖先在前）
	if returnSort {
		for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
			chain[i], chain[j] = chain[j], chain[i]
		}
	}
	return chain, nil
}

// stepSortIndex 构建部署下步骤代号到排序号的索引。
// @param ctx 操作上下文
// @param deploymentID 部署 ID
// @return 代号 → 排序号
// @return 错误
func (s *BackService) stepSortIndex(ctx context.Context, deploymentID string) (map[string]int, error) {
	if s.steps == nil {
		return map[string]int{}, nil
	}
	steps, err := s.steps.store.ListByDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(steps))
	for _, step := range steps {
		index[step.StepCode] = step.Sort
	}
	return index, nil
}

// assigneeNamesOf 汇总一组历史任务的办理人展示名（去重；多实例节点全量收集）。
// @param ctx 操作上下文
// @param taskIDs 历史/运行任务 ID 列表
// @return 办理人展示名列表（身份查询缺失时为身份 Id）
func (s *BackService) assigneeNamesOf(ctx context.Context, taskIDs []string) []string {
	seen := map[string]bool{}
	names := make([]string, 0, len(taskIDs))
	for _, id := range taskIDs {
		historic, err := s.engine.HistoryService().GetHistoricTaskInstance(ctx, id)
		if err != nil {
			// 运行中任务历史行也可能存在，回退运行任务查询
			if task, taskErr := s.engine.TaskService().GetTask(ctx, id); taskErr == nil {
				historic.Assignee = task.Assignee
			} else {
				continue
			}
		}
		// 无办理人（组任务等）不参与展示
		if historic.Assignee == "" || seen[historic.Assignee] {
			continue
		}
		seen[historic.Assignee] = true
		names = append(names, s.displayName(ctx, historic.Assignee))
	}
	return names
}

// displayName 查询身份展示名（身份查询缺失或查询失败时回退身份 Id）。
// @param ctx 操作上下文
// @param userID 身份 Id
// @return 展示名
func (s *BackService) displayName(ctx context.Context, userID string) string {
	if s.users == nil {
		return userID
	}
	if user, err := s.users.GetUser(ctx, userID); err == nil {
		if name := userDisplayName(user); name != "" {
			return name
		}
	}
	return userID
}

// nodeDisplayName 读取模型节点名称（扩展名与模型名都缺失时回退节点 ID）。
// @param node 流程节点
// @return 展示名
func nodeDisplayName(node model.FlowNode) string {
	if node.GetName() != "" {
		return node.GetName()
	}
	return node.GetID()
}

// isNodeReachable 判断 from 节点沿顺序流是否可达 to 节点（等价 engine.IsReachable 的
// 本包纯函数实现——workflow 包不 import internal/engine，基于 api 模型对象遍历）。
// 处理子流程出线提升、事件子流程开始事件、防环。
// @param process 流程模型
// @param from 起点
// @param to 终点
// @return 是否可达
func isNodeReachable(process *model.Process, from, to model.FlowNode) bool {
	return isNodeReachableDFS(process, from, to, map[string]bool{})
}

// isNodeReachableDFS isNodeReachable 的深度优先递归实现。
// @param process 流程模型
// @param source 当前节点
// @param target 目标节点
// @param visited 已访问集合（防环）
// @return 是否可达
func isNodeReachableDFS(process *model.Process, source, target model.FlowNode, visited map[string]bool) bool {
	// 事件子流程的开始事件是等待触发的执行，不构成真实路径起点
	if start, isStart := source.(*model.StartEvent); isStart && inEventSubProcess(process, start.ID) {
		return false
	}
	// 子流程终点无出线：向上提升到容器继续判定
	if len(source.GetOutgoing()) == 0 {
		visited[source.GetID()] = true
		parent := containerOfFlowElement(process, source.GetID())
		if parent == nil {
			return false
		}
		// 目标仍在该子流程内而当前已是其终点：不可达
		if _, exists := parent.Elements[target.GetID()]; exists {
			return false
		}
		source = parent
	}
	if source.GetID() == target.GetID() {
		return true
	}
	visited[source.GetID()] = true
	// 当前节点是子流程且目标在其内部：直接可达
	if sub, isSub := source.(*model.SubProcess); isSub {
		if _, exists := sub.Elements[target.GetID()]; exists {
			return true
		}
	}
	for _, flow := range source.GetOutgoing() {
		if visited[flow.TargetRef] {
			continue
		}
		element, ok := findFlowElement(process, flow.TargetRef)
		if !ok {
			continue
		}
		next, isNode := element.(model.FlowNode)
		if !isNode {
			continue
		}
		if isNodeReachableDFS(process, next, target, visited) {
			return true
		}
	}
	return false
}

// inEventSubProcess 判断元素是否位于事件子流程内。
// @param process 流程模型
// @param elementID 元素 ID
// @return 是否在事件子流程内
func inEventSubProcess(process *model.Process, elementID string) bool {
	current := containerOfFlowElement(process, elementID)
	for current != nil {
		if current.TriggeredByEvent {
			return true
		}
		current = containerOfFlowElement(process, current.ID)
	}
	return false
}

// containerOfFlowElement 返回直接包含指定元素的子流程（直属主流程时为 nil）。
// @param process 流程模型
// @param elementID 元素 ID
// @return 直接容器子流程
func containerOfFlowElement(process *model.Process, elementID string) *model.SubProcess {
	// 主流程直属元素没有容器
	if _, ok := process.Elements[elementID]; ok {
		return nil
	}
	for _, id := range process.Order {
		sub, ok := process.Elements[id].(*model.SubProcess)
		if !ok {
			continue
		}
		if found := containerOfFlowElementInSub(sub, elementID); found != nil {
			return found
		}
	}
	return nil
}

// containerOfFlowElementInSub 在子流程树中查找直接包含元素的子流程。
// @param sub 子流程
// @param elementID 元素 ID
// @return 直接容器子流程
func containerOfFlowElementInSub(sub *model.SubProcess, elementID string) *model.SubProcess {
	if _, ok := sub.Elements[elementID]; ok {
		return sub
	}
	for _, id := range sub.Order {
		inner, ok := sub.Elements[id].(*model.SubProcess)
		if !ok {
			continue
		}
		if found := containerOfFlowElementInSub(inner, elementID); found != nil {
			return found
		}
	}
	return nil
}

// nextActivityIDsAfter 枚举目标节点沿出线可达的全部后继元素 ID（不含目标，防环）。
// 对位 Java FlowUtils.getNextActivitys 的图遍历；子流程容器展开内部元素 ID。
// @param process 流程模型
// @param targetID 目标节点 ID
// @return 后继元素 ID 列表（无序）
func nextActivityIDsAfter(process *model.Process, targetID string) []string {
	collected := map[string]bool{}
	var visit func(id string)
	visit = func(id string) {
		// 已收集元素跳过（环路与汇聚）
		if collected[id] {
			return
		}
		element, ok := findFlowElement(process, id)
		if !ok {
			return
		}
		collected[id] = true
		// 子流程容器：内部元素一并收集（嵌套递归）
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
	delete(collected, targetID)
	ids := make([]string, 0, len(collected))
	for id := range collected {
		ids = append(ids, id)
	}
	return ids
}

// ─── 任意退回（generalBack:2557 对位） ─────────────────────────────

// BackToStep 退回任务到指定步骤（对位 Java generalBack）：
// 目标执行人预存（saveBackToStepNext 对位）→ allowReturn 校验 → 暂停拒绝 →
// 引擎级任意退回 → TH 意见 + 操作记录(isAgree=0) → 预存清理（目标之后节点）→
// 待办清理（目标及之后节点 + 当前任务）→ 已办完成时间 → 按实例重新生成待办。
// Go 协议适配：BackTaskVo 无 request 字段，目标执行人取该节点历史办理人
// （与 BackNodes 的办理人展示同源）；意见为空时跳过意见写入（引擎拒绝空评论）。
// @param ctx 操作上下文
// @param req 退回请求（TaskID/UserCode 必填）
// @param targetActivityID 退回目标节点 ID（必填）
// @return 错误
func (s *BackService) BackToStep(ctx context.Context, req BackTaskVo, targetActivityID string) error {
	// 1. 入参校验（Java generalBack 逐项 isBlank 校验）
	if strings.TrimSpace(req.TaskID) == "" {
		return apiError(errCodeBackTaskIDBlank, "参数 taskId 为空")
	}
	if strings.TrimSpace(req.UserCode) == "" {
		return apiError(errCodeBackUserBlank, "参数 userCode 为空")
	}
	if strings.TrimSpace(targetActivityID) == "" {
		return apiError(errCodeBackTargetBlank, "参数 targetActivityId 为空")
	}
	// 2. 任务定位
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return api.NewError(errCodeBackTaskMissing, "任务不存在", err)
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return err
	}
	// 3. allowReturn 校验（Java：hisStep allowReturn=="0" → "当前步骤不允许退回"）
	allowed, err := s.steps.AllowReturn(ctx, definition.DeploymentID, task.TaskDefinitionKey)
	if err != nil {
		return err
	}
	if !allowed {
		return apiError(errCodeBackNotAllowed, "当前步骤不允许退回")
	}
	// 4. 暂停拒绝（Java isSuspended → "流程已暂停，请联系管理员激活"）
	if err := s.ensureInstanceActive(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	// 5. 目标执行人预存（Java saveBackToStepNextActivity 对位；执行人取目标节点历史办理人）
	if err := s.saveBackToStepExecutors(ctx, task, targetActivityID); err != nil {
		return err
	}
	// 6. TH 意见（引擎评论 API 绑定运行态任务，须在退回命令前写入；空意见跳过）
	commentType := firstNonEmpty(req.Type, CommentTypeTH)
	if strings.TrimSpace(req.Message) != "" {
		if _, err := s.comments.Add(ctx, CommentRequest{
			TaskID:            req.TaskID,
			ProcessInstanceID: task.ProcessInstanceID,
			UserID:            req.UserCode,
			Message:           req.Message,
			Type:              commentType,
		}); err != nil {
			return err
		}
	}
	// 7. 引擎级任意退回（BackUserTaskCmd 对位，含目标及其后继的历史清理）
	landed, err := s.engine.WorkflowExtService().BackUserTaskToActivity(ctx, req.TaskID, targetActivityID)
	if err != nil {
		return err
	}
	// 8. 操作记录（isAgree=0 不同意，Java addOperationRecord 第 4 参）
	notAgree := 0
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            req.TaskID,
		ProcessInstanceID: task.ProcessInstanceID,
		OwnerID:           req.UserCode,
		IsAgree:           &notAgree,
		Message:           req.Message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	// 9. 收尾链：模型后继枚举 → 预存/待办清理 → 已办完成时间 → 重生成待办
	process, err := s.processOf(ctx, task.ProcessDefinitionID)
	if err != nil {
		return err
	}
	successors := nextActivityIDsAfter(process, landed)
	// 9.1 删除目标之后节点的预存记录（Java deleteWorkflowNextActivityByProcessInstanceIdAndActIds，
	// 不含目标节点——目标节点的预存记录就是刚预存的目标执行人）
	if len(successors) > 0 {
		if err := s.next.DeleteByProcessInstanceAndActivityIDs(ctx, task.ProcessInstanceID, successors); err != nil {
			return err
		}
	}
	// 9.2 删除目标及之后节点的待办（Java deleteTaskMessageByProcessInstanceIdAndActIds）+
	// 当前任务待办（Java generalBack 未清理当前任务待办行，Go 补齐：当前任务已被退回命令取消，
	// 残留待办会成为死数据，与直送 deleteTaskMessageExtendByTaskId 同一语义）
	actIDs := append(append([]string(nil), successors...), landed)
	if err := s.messages.DeleteByProcessInstanceAndActivityIDs(ctx, task.ProcessInstanceID, actIDs); err != nil {
		return err
	}
	if err := s.messages.DeleteByTaskID(ctx, req.TaskID); err != nil {
		return err
	}
	// 9.3 已办完成时间（Java updateByTaskIdAndOwnerId；时间为零值时服务内取当前时间）
	if err := s.histories.UpdateFinishTime(ctx, req.TaskID, req.UserCode, time.Time{}); err != nil {
		return err
	}
	// 9.4 按实例重新生成待办（Java addTaskMessageExtendByProcessInstanceId 对位）
	if _, err := s.generateTodosAfterBack(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	return nil
}

// BackToPrevious 退回任务到上一节点（对位 Java backTask:4502 的简化退回，Go 按入线自动定位）：
// 取当前节点唯一入线的 sourceRef 直迁；多入线（网关/汇聚后）报错，仅支持串行流程。
// 收尾做最小账本闭合：TH 意见 + 当前任务待办清理 + 已办完成时间 + 重生成待办
// （Java backTask 仅写意见后直接执行命令，Go 补齐待办账本一致性，避免死待办）。
// @param ctx 操作上下文
// @param req 退回请求（TaskID/UserCode 必填）
// @return 错误
func (s *BackService) BackToPrevious(ctx context.Context, req BackTaskVo) error {
	// 入参校验（Java backTask：taskId/identityId 必填）
	if strings.TrimSpace(req.TaskID) == "" {
		return apiError(errCodeBackTaskIDBlank, "参数 taskId 为空")
	}
	if strings.TrimSpace(req.UserCode) == "" {
		return apiError(errCodeBackUserBlank, "参数 userCode 为空")
	}
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return api.NewError(errCodeBackTaskMissing, "任务不存在", err)
	}
	// 暂停拒绝（与 generalBack 同语义）
	if err := s.ensureInstanceActive(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	process, err := s.processOf(ctx, task.ProcessDefinitionID)
	if err != nil {
		return err
	}
	element, found := findFlowElement(process, task.TaskDefinitionKey)
	if !found {
		return api.NewError(errCodeBackNodeMissing, "当前任务节点不存在于模型", api.ErrNotFound)
	}
	node, isNode := element.(model.FlowNode)
	if !isNode {
		return api.NewError(errCodeBackNodeMissing, "当前任务节点不是流程节点", api.ErrBadRequest)
	}
	// 仅串行防护：多入线（并行汇聚/多分支来源）时无法唯一定位上一节点
	incoming := node.GetIncoming()
	if len(incoming) == 0 {
		return apiError(errCodeBackNoIncoming, "当前节点没有入线，无法退回上一节点")
	}
	if len(incoming) > 1 {
		return apiError(errCodeBackMultiIncoming, "上一节点不唯一，不允许退回上一节点")
	}
	// TH 意见（引擎评论 API 绑定运行态任务，须在退回命令前写入；空意见跳过）
	commentType := firstNonEmpty(req.Type, CommentTypeTH)
	if strings.TrimSpace(req.Message) != "" {
		if _, err := s.comments.Add(ctx, CommentRequest{
			TaskID:            req.TaskID,
			ProcessInstanceID: task.ProcessInstanceID,
			UserID:            req.UserCode,
			Message:           req.Message,
			Type:              commentType,
		}); err != nil {
			return err
		}
	}
	// 引擎级退回到入线来源节点
	landed, err := s.engine.WorkflowExtService().BackUserTaskToActivity(ctx, req.TaskID, incoming[0].SourceRef)
	if err != nil {
		return err
	}
	// 最小账本闭合：当前任务待办清理 + 已办完成时间 + 重生成待办
	if err := s.messages.DeleteByTaskID(ctx, req.TaskID); err != nil {
		return err
	}
	if err := s.histories.UpdateFinishTime(ctx, req.TaskID, req.UserCode, time.Time{}); err != nil {
		return err
	}
	if _, err := s.generateTodosAfterBack(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	_ = landed
	return nil
}

// ensureInstanceActive 校验实例未挂起（Java isSuspended）。
// 实例已结束/查询失败时交由引擎校验兜底，不在链路前置拒绝。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *BackService) ensureInstanceActive(ctx context.Context, processInstanceID string) error {
	if s.instances == nil {
		return nil
	}
	instance, err := s.instances.GetProcessInstance(ctx, processInstanceID)
	// 运行表查不到（已结束）：后续引擎校验兜底
	if err != nil {
		return nil
	}
	if instance.Suspended {
		return apiError(errCodeBackSuspended, "流程已暂停，请联系管理员激活")
	}
	return nil
}

// saveBackToStepExecutors 预存目标节点的退回执行人（Java saveBackToStepNextActivity 对位）。
// Go 协议适配：BackTaskVo 无 request 字段，执行人取目标节点的历史办理人
// （退回语义即"退回给曾处理过该节点的人"）；无历史办理人时跳过预存，
// 目标任务执行人回落模型配置。
// @param ctx 操作上下文
// @param task 当前退回任务
// @param targetActivityID 目标节点 ID
// @return 错误
func (s *BackService) saveBackToStepExecutors(ctx context.Context, task api.Task, targetActivityID string) error {
	finished := true
	page, err := s.engine.HistoryService().QueryHistoricTaskInstances(ctx, api.HistoricTaskInstanceFilter{
		ProcessInstanceID: task.ProcessInstanceID,
		TaskDefinitionKey: targetActivityID,
		Finished:          &finished,
		Size:              backHistoryPageSize,
	})
	if err != nil {
		return api.NewError(errCodeBackHistoryQuery, "历史任务查询失败", err)
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(page.Data))
	for _, historic := range page.Data {
		// 历史办理人去重（多实例节点多行合并）
		if historic.Assignee == "" || seen[historic.Assignee] {
			continue
		}
		seen[historic.Assignee] = true
		ids = append(ids, historic.Assignee)
	}
	// 无历史办理人（目标节点从未被处理）：跳过预存
	if len(ids) == 0 {
		return nil
	}
	return s.next.SaveBackToStepNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: task.ProcessInstanceID,
		ActivityID:        targetActivityID,
		IdentityIDs:       ids,
		PreTaskID:         task.ID,
		// Java request.sendIdentityId 必填；Go BackTaskVo 无该字段，统一回退任务办理人
		SendIdentityID: task.Assignee,
		SendUserName:   s.displayName(ctx, task.Assignee),
		TenantID:       task.TenantID,
		Creator:        task.Assignee,
	})
}

// generateTodosAfterBack 按实例重新生成待办与已办镜像（Java addTaskMessageExtendByProcessInstanceId
// → saveMessageExtendInfo 对位；complete.go generateTodosForNewTasks 的同构实现，
// 不可改其文件故在本包复刻：count==0 才插入、无处理人任务跳过待执行人重写补齐）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 新建待办的任务 Id 列表（含已有待办的任务）
// @return 错误
func (s *BackService) generateTodosAfterBack(ctx context.Context, processInstanceID string) ([]string, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: backHistoryPageSize})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page.Data))
	for i := range page.Data {
		current := page.Data[i]
		// 幂等：同任务已有待办时跳过（Java count==0 才插入）
		if _, found, err := s.messages.GetByTaskID(ctx, current.ID); err != nil {
			return nil, err
		} else if found {
			ids = append(ids, current.ID)
			continue
		}
		// 无处理人（组任务等执行人未落定场景）暂不生成待办，待执行人重写钩子补齐
		if current.Assignee == "" {
			continue
		}
		message, err := s.buildBackMessageExtend(ctx, processInstanceID, current)
		if err != nil {
			return nil, err
		}
		if _, err := s.messages.CreateForTask(ctx, message); err != nil {
			return nil, err
		}
		// 已办镜像（Java saveMessageExtendInfo 同步插入历史行）
		if _, err := s.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
			return nil, err
		}
		ids = append(ids, current.ID)
	}
	return ids, nil
}

// buildBackMessageExtend 组装新任务的待办扩展（实例/定义/预存发送人信息联查补全）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param task 新任务
// @return 待办扩展
// @return 错误
func (s *BackService) buildBackMessageExtend(ctx context.Context, processInstanceID string, task api.Task) (*MessageExtend, error) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, err
	}
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		TaskID:            task.ID,
		ProcessInstanceID: processInstanceID,
		Title:             instance.Name,
		BusinessID:        instance.BusinessKey,
		TaskStepName:      task.Name,
		TaskStepCode:      task.TaskDefinitionKey,
		FlowCategoryCode:  definition.Key,
		FlowCategoryName:  definition.Name,
		OwnerID:           task.Assignee,
		OwnerName:         s.displayName(ctx, task.Assignee),
		TaskReceiptTime:   &task.CreateTime,
	}
	// 发送人信息取该节点第一条未消耗预存记录（退回预存记录/执行人重写未消耗记录均可追溯）
	if record, found, err := s.next.GetFirstUnusedByActivity(ctx, processInstanceID, task.TaskDefinitionKey); err == nil && found {
		message.SenderID = record.SendIdentityID
		message.Sender = record.SendUserName
		// 发送步骤取来源任务名（Java hisTask.getName/TaskDefinitionKey）
		if record.PreTaskID != "" {
			if his, err := s.engine.HistoryService().GetHistoricTaskInstance(ctx, record.PreTaskID); err == nil {
				message.SenderStepName = his.Name
				message.SenderStepCode = his.TaskDefinitionKey
			}
		}
	}
	// 起草人 = 实例发起人
	message.DocumentOwnerID = instance.StartUserID
	message.DocumentOwnerName = s.displayName(ctx, instance.StartUserID)
	return message, nil
}
