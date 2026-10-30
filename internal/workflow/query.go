package workflow

import (
	"context"
	"sort"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// queryListSize 是查询门面拉取引擎列表的批量上限（联查/过滤在内存完成）。
const queryListSize = 1000

// 默认分页大小（对位 Java PageUtil 的公共分页语义）。
const defaultPageSize = 10

// 实例列表状态代号（对位调研 005 §1.1 instance/list 的状态字段约定）。
const (
	// InstanceStatusRunning 运行中（含未提交/流转中）。
	InstanceStatusRunning = 0
	// InstanceStatusFinished 已完成（实例已结束且最后操作非终止）。
	InstanceStatusFinished = 1
	// InstanceStatusTerminated 已终止（最后一条操作记录为 LCZZ）。
	InstanceStatusTerminated = 2
)

// TodoTaskView 是待办任务列表视图：待办扩展为主体，联查引擎任务补实时状态。
type TodoTaskView struct {
	*MessageExtend     // 待办扩展全字段（待办中心唯一数据源）
	EngineTaskName string // 引擎任务名称（任务仍在办时非空，已流转为空）
	EngineCreate   time.Time // 引擎任务创建时间
	EngineExists   bool   // 引擎任务是否仍存在（false 表示已办结/已流转）
}

// DoneTaskView 是已办任务列表视图：已办扩展为主体，联查引擎历史任务补办结时间。
type DoneTaskView struct {
	*HistoryExtend        // 已办扩展全字段
	EngineFinishTime *time.Time // 引擎历史任务办结时间（查不到为空）
	EngineExists     bool       // 引擎历史任务是否存在
}

// TaskInfoView 是任务详情视图：引擎任务 + workflow 扩展列。
type TaskInfoView struct {
	api.Task             // 引擎任务全字段
	Extend    *MessageExtend // 待办扩展（存在时非空）
	History   *HistoryExtend // 已办扩展（存在时非空）
}

// ProcessInstanceView 是流程实例列表视图（字段对位调研 005 §1.1 instance/list）。
type ProcessInstanceView struct {
	ProcessInstanceID    string     // 实例 ID
	ProcessDefinitionKey string     // 流程定义 Key
	ProcessDefinitionID  string     // 流程定义 ID
	Title                string     // 实例标题（=实例名称）
	BusinessID           string     // 业务单据 Id（=businessKey）
	StartUserID          string     // 发起人身份 Id
	StartTime            time.Time  // 发起时间
	EndTime              *time.Time // 结束时间，运行中为空
	Status               int        // 状态：0 运行中 / 1 已完成 / 2 已终止
	CurrentStep          string     // 当前待办步骤聚合（document.flow_action）
	CurrentPersons       string     // 当前待办人聚合（document.flow_current_persons）
}

// QueryService 是待办/已办/任务/实例查询门面（P1.8）。
// 数据契约：待办以 workflow_task_message_extend 为唯一数据源、已办以
// workflow_task_history_extend 为唯一数据源，引擎任务/历史任务仅作实时状态联查补齐；
// 实例列表经引擎历史实例查询，状态经 document 状态机映射（0/1/2）。
type QueryService struct {
	engine     api.Engine              // 引擎门面（七服务）
	messages   *MessageExtendService   // 待办扩展域服务
	histories  *HistoryExtendService   // 已办扩展域服务
	operations *OperationRecordService // 操作记录域服务（可签名查询）
	documents  *DocumentService        // 业务表单状态域服务（实例状态映射）
}

// NewQueryService 创建查询门面服务。
// @param engine 引擎门面（七服务）
// @param messages 待办扩展域服务
// @param histories 已办扩展域服务
// @param operations 操作记录域服务
// @param documents 业务表单状态域服务
// @return 查询门面
func NewQueryService(engine api.Engine, messages *MessageExtendService, histories *HistoryExtendService, operations *OperationRecordService, documents *DocumentService) *QueryService {
	return &QueryService{engine: engine, messages: messages, histories: histories, operations: operations, documents: documents}
}

// normalizePageQuery 归一化分页参数（Start 负值归 0，Size 缺省/非法回退默认值）。
// @param query 原始分页参数
// @return 归一化后的分页参数
func normalizePageQuery(query PageQuery) PageQuery {
	// 起始偏移不允许负值
	if query.Start < 0 {
		query.Start = 0
	}
	// 未传或非法按默认页大小
	if query.Size <= 0 {
		query.Size = defaultPageSize
	}
	return query
}

// TodoListByIdentityID 按身份 Id 分页查询待办任务（MessageExtendService.ListByOwner + 引擎任务联查）。
// @param ctx 操作上下文
// @param identityID 身份 Id
// @param query 分页参数
// @return 分页视图
// @return 错误
func (s *QueryService) TodoListByIdentityID(ctx context.Context, identityID string, query PageQuery) (PageResult[*TodoTaskView], error) {
	page := normalizePageQuery(query)
	result, err := s.messages.ListByOwner(ctx, identityID, "", page)
	if err != nil {
		return PageResult[*TodoTaskView]{}, err
	}
	views := make([]*TodoTaskView, 0, len(result.Data))
	for _, message := range result.Data {
		view := &TodoTaskView{MessageExtend: message}
		// 联查引擎任务补实时状态：任务已流转时保留扩展字段并标记不存在
		if task, err := s.engine.TaskService().GetTask(ctx, message.TaskID); err == nil {
			view.EngineExists = true
			view.EngineTaskName = task.Name
			view.EngineCreate = task.CreateTime
		}
		views = append(views, view)
	}
	return PageResult[*TodoTaskView]{Data: views, Total: result.Total, Start: page.Start, Size: page.Size}, nil
}

// DoneListByIdentityID 按身份 Id 分页查询已办任务（HistoryExtendService.ListByOwner + 引擎历史任务联查）。
// @param ctx 操作上下文
// @param identityID 身份 Id
// @param query 分页参数
// @return 分页视图
// @return 错误
func (s *QueryService) DoneListByIdentityID(ctx context.Context, identityID string, query PageQuery) (PageResult[*DoneTaskView], error) {
	page := normalizePageQuery(query)
	result, err := s.histories.ListByOwner(ctx, identityID, "", page)
	if err != nil {
		return PageResult[*DoneTaskView]{}, err
	}
	views := make([]*DoneTaskView, 0, len(result.Data))
	for _, history := range result.Data {
		view := &DoneTaskView{HistoryExtend: history}
		// 联查引擎历史任务补办结时间（扩展行办结时间缺失时回填引擎值）
		if task, err := s.engine.HistoryService().GetHistoricTaskInstance(ctx, history.TaskID); err == nil {
			view.EngineExists = true
			view.EngineFinishTime = task.EndTime
		}
		views = append(views, view)
	}
	return PageResult[*DoneTaskView]{Data: views, Total: result.Total, Start: page.Start, Size: page.Size}, nil
}

// TaskMessageListByBusinessID 按业务单据查询待办扩展列表。
// @param ctx 操作上下文
// @param tenantID 租户（业务系统代号）
// @param businessID 业务单据 Id
// @return 待办扩展列表
// @return 错误
func (s *QueryService) TaskMessageListByBusinessID(ctx context.Context, tenantID, businessID string) ([]*MessageExtend, error) {
	return s.messages.ListByBusinessID(ctx, businessID, tenantID)
}

// TaskHistoryListByBusinessID 按业务单据查询已办扩展列表。
// @param ctx 操作上下文
// @param tenantID 租户（业务系统代号）
// @param businessID 业务单据 Id
// @return 已办扩展列表
// @return 错误
func (s *QueryService) TaskHistoryListByBusinessID(ctx context.Context, tenantID, businessID string) ([]*HistoryExtend, error) {
	return s.histories.ListByBusinessID(ctx, businessID, tenantID)
}

// TaskHistoryListByInstance 按流程实例查询已办扩展列表（轨迹聚合）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 已办扩展列表
// @return 错误
func (s *QueryService) TaskHistoryListByInstance(ctx context.Context, processInstanceID string) ([]*HistoryExtend, error) {
	return s.histories.ListByInstance(ctx, processInstanceID)
}

// TaskHistoryListByTaskID 按任务查询已办扩展记录（无记录返回 nil）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 已办扩展，不存在为 nil
// @return 错误
func (s *QueryService) TaskHistoryListByTaskID(ctx context.Context, taskID string) (*HistoryExtend, error) {
	history, found, err := s.histories.GetByTaskID(ctx, taskID)
	// 无已办记录：返回 nil 由调用方判空
	if err != nil || !found {
		return nil, err
	}
	return history, nil
}

// TaskInfoByTaskID 按任务 ID 查询任务详情（引擎任务 + workflow 扩展列）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 任务详情视图
// @return 错误（任务不存在）
func (s *QueryService) TaskInfoByTaskID(ctx context.Context, taskID string) (*TaskInfoView, error) {
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return nil, api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	view := &TaskInfoView{Task: task}
	// 待办扩展存在时补全
	if message, found, err := s.messages.GetByTaskID(ctx, taskID); err == nil && found {
		view.Extend = message
	}
	// 已办扩展存在时补全
	if history, found, err := s.histories.GetByTaskID(ctx, taskID); err == nil && found {
		view.History = history
	}
	return view, nil
}

// InstanceListByUser 按用户分页查询流程实例（用户发起的实例；参与的实例经待办/已办列表呈现）。
// @param ctx 操作上下文
// @param userID 用户身份 Id
// @param query 分页参数
// @return 分页实例视图
// @return 错误
func (s *QueryService) InstanceListByUser(ctx context.Context, userID string, query PageQuery) (PageResult[*ProcessInstanceView], error) {
	// 按发起人过滤后复用统一装配与分页
	return s.listInstances(ctx, query, func(instance api.HistoricProcessInstance) bool {
		return instance.StartUserID == userID
	})
}

// AllInstanceList 分页查询全部流程实例（含运行中与已结束）。
// @param ctx 操作上下文
// @param query 分页参数
// @return 分页实例视图
// @return 错误
func (s *QueryService) AllInstanceList(ctx context.Context, query PageQuery) (PageResult[*ProcessInstanceView], error) {
	// 全量查询：谓词恒真
	return s.listInstances(ctx, query, func(api.HistoricProcessInstance) bool { return true })
}

// listInstances 装配实例分页视图（历史实例 + document 状态映射 + 内存分页）。
// @param ctx 操作上下文
// @param query 分页参数
// @param match 实例过滤谓词
// @return 分页实例视图
// @return 错误
func (s *QueryService) listInstances(ctx context.Context, query PageQuery, match func(api.HistoricProcessInstance) bool) (PageResult[*ProcessInstanceView], error) {
	page := normalizePageQuery(query)
	instances, err := s.engine.HistoryService().ListHistoricProcessInstances(ctx)
	if err != nil {
		return PageResult[*ProcessInstanceView]{}, err
	}
	views := make([]*ProcessInstanceView, 0, len(instances))
	for i := range instances {
		instance := instances[i]
		// 调用方谓词过滤（按发起人/全量）
		if !match(instance) {
			continue
		}
		views = append(views, s.buildInstanceView(ctx, instance))
	}
	// 按发起时间倒序（最近的在前，与待办/已办列表语义一致）
	sort.SliceStable(views, func(i, j int) bool {
		return views[i].StartTime.After(views[j].StartTime)
	})
	return paginateInstances(views, page), nil
}

// buildInstanceView 装配单个实例视图（状态经 document 状态机映射，缺 document 时按引擎结束标记回退）。
// @param ctx 操作上下文
// @param instance 历史实例
// @return 实例视图
func (s *QueryService) buildInstanceView(ctx context.Context, instance api.HistoricProcessInstance) *ProcessInstanceView {
	view := &ProcessInstanceView{
		ProcessInstanceID:    instance.ID,
		ProcessDefinitionKey: instance.ProcessDefinitionKey,
		ProcessDefinitionID:  instance.ProcessDefinitionID,
		Title:                instance.Name,
		BusinessID:           instance.BusinessKey,
		StartUserID:          instance.StartUserID,
		StartTime:            instance.StartTime,
		EndTime:              instance.EndTime,
	}
	// 状态映射：document 流转状态优先，缺 document 按引擎结束标记回退
	if document, found, err := s.documents.GetByInstance(ctx, instance.ID); err == nil && found {
		view.Status = instanceStatusOf(document.FlowStatus, instance.Ended)
		view.CurrentStep = document.FlowAction
		view.CurrentPersons = document.FlowCurrentPersons
	} else {
		view.Status = instanceStatusOf("", instance.Ended)
	}
	return view
}

// instanceStatusOf 将 document 流转状态映射为实例状态代号（0/1/2）。
// @param flowStatus document.flow_status 中文字面值
// @param ended 引擎实例是否已结束
// @return 状态代号
func instanceStatusOf(flowStatus string, ended bool) int {
	// 终止优先于完成（最后操作 LCZZ 的实例可能已结束也可能被终止删除）
	if flowStatus == FlowStatusTerminated {
		return InstanceStatusTerminated
	}
	if flowStatus == FlowStatusFinished || ended {
		return InstanceStatusFinished
	}
	return InstanceStatusRunning
}

// paginateInstances 内存分页（偏移越界返回空页）。
// @param views 全量视图
// @param page 归一化分页参数
// @return 分页结果
func paginateInstances(views []*ProcessInstanceView, page PageQuery) PageResult[*ProcessInstanceView] {
	total := int64(len(views))
	// 偏移越界返回空页
	if page.Start >= len(views) {
		return PageResult[*ProcessInstanceView]{Data: []*ProcessInstanceView{}, Total: total, Start: page.Start, Size: page.Size}
	}
	end := page.Start + page.Size
	// 末页截断
	if end > len(views) {
		end = len(views)
	}
	return PageResult[*ProcessInstanceView]{Data: views[page.Start:end], Total: total, Start: page.Start, Size: page.Size}
}

// SignableOperationRecords 查询"可签名"操作记录（type ∈ {TJ,SP,TZ} 且步骤代号非空，透传端口）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 记录列表
// @return 错误
func (s *QueryService) SignableOperationRecords(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	return s.operations.ListSignable(ctx, businessIDs)
}
