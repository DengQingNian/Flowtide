package workflow

import (
	"context"
	"database/sql"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// Options 是 workflow 服务的配置选项（调度开关与注入点，对位 Java application-dev.yml 业务开关，默认全 false）。
// 行仓储（internal/persistence/workflow 适配器）与回调配置在 P1 阶段接入本结构。
type Options struct {
	// NoHandleAutoComplete 调度开关：超时无处理自动办结。
	NoHandleAutoComplete bool
	// TaskTimeoutRemind 调度开关：任务超时提醒。
	TaskTimeoutRemind bool
	// SendWorkflowRemind 调度开关：发送流程提醒。
	SendWorkflowRemind bool
	// SendTodoRemind 调度开关：发送待办提醒。
	SendTodoRemind bool
	// AdjacentActivitySameOwnerAutoComplete 调度开关：相邻节点同人自动办结。
	AdjacentActivitySameOwnerAutoComplete bool
	// Clock 时钟注入（调度与测试用），nil 时使用 clock.Real。
	Clock clock.Clock

	// sqlStore SQL 存储入口（WithSQLStores 注入；nil 时使用内存存储）。
	sqlStore *pwf.Store
	// sqlDB 独立数据库连接（可选，nil 时使用 sqlStore 自带连接）。
	sqlDB *sql.DB
}

// Option 是 workflow 服务选项函数。
type Option func(*Options)

// WithClock 注入时钟。
// @param c 时钟实现
// @return 选项
func WithClock(c clock.Clock) Option {
	return func(o *Options) {
		if c != nil {
			o.Clock = c
		}
	}
}

// WithNoHandleAutoComplete 设置超时无处理自动办结开关。
// @param enabled 是否启用
// @return 选项
func WithNoHandleAutoComplete(enabled bool) Option {
	return func(o *Options) { o.NoHandleAutoComplete = enabled }
}

// WithTaskTimeoutRemind 设置任务超时提醒开关。
// @param enabled 是否启用
// @return 选项
func WithTaskTimeoutRemind(enabled bool) Option {
	return func(o *Options) { o.TaskTimeoutRemind = enabled }
}

// WithSendWorkflowRemind 设置发送流程提醒开关。
// @param enabled 是否启用
// @return 选项
func WithSendWorkflowRemind(enabled bool) Option {
	return func(o *Options) { o.SendWorkflowRemind = enabled }
}

// WithSendTodoRemind 设置发送待办提醒开关。
// @param enabled 是否启用
// @return 选项
func WithSendTodoRemind(enabled bool) Option {
	return func(o *Options) { o.SendTodoRemind = enabled }
}

// WithAdjacentActivitySameOwnerAutoComplete 设置相邻同人自动办结开关。
// @param enabled 是否启用
// @return 选项
func WithAdjacentActivitySameOwnerAutoComplete(enabled bool) Option {
	return func(o *Options) { o.AdjacentActivitySameOwnerAutoComplete = enabled }
}

// WithSQLStores 切换 workflow 业务表存储为真实库行仓储。
// @param store workflow 仓储入口（internal/persistence/workflow.New）
// @param db 独立数据库连接，可为 nil（使用 store 自带连接）
// @return 选项
func WithSQLStores(store *pwf.Store, db *sql.DB) Option {
	return func(o *Options) {
		if store != nil {
			o.sqlStore = store
			o.sqlDB = db
		}
	}
}

// Service 是 WorkflowFacade 的门面实现。
// 内部子服务（发起/办结/查询/意见/操作记录/待办/已办/表单/预生成）由
// assembleComponents 统一装配，存储默认内存实现（嵌入式无 DB 模式），
// 可经 WithSQLStores 切换为真实库行仓储。
type Service struct {
	engine api.Engine  // 引擎门面（七服务），通过 api 接口访问引擎
	opts   Options     // 服务配置
	comp   *components // 子服务聚合
}

// 编译期断言：Service 必须满足 api.WorkflowFacade 门面契约。
var _ api.WorkflowFacade = (*Service)(nil)

// NewWorkflowService 创建 workflow 门面服务。
// @param engine 引擎门面（七服务）
// @param options 可选配置（调度开关、时钟注入、存储实现）
// @return 服务实例
func NewWorkflowService(engine api.Engine, options ...Option) *Service {
	opts := Options{}
	for _, option := range options {
		option(&opts)
	}
	// 未注入时钟时回退真实时钟，保证调度器无需判空
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	var comp *components
	if opts.sqlStore != nil {
		// 真实库模式：workflow_* 行仓储走 internal/persistence/workflow
		comp, _ = assembleSQLComponents(engine, opts.sqlStore, opts.sqlDB, opts, opts.Clock)
	}
	if comp == nil {
		comp = assembleMemoryComponents(engine, opts, opts.Clock)
	}
	return &Service{engine: engine, opts: opts, comp: comp}
}

// Engine 返回持有的引擎门面。
// @return 引擎门面
func (s *Service) Engine() api.Engine {
	return s.engine
}

// Options 返回服务配置副本。
// @return 配置副本
func (s *Service) Options() Options {
	return s.opts
}

// errNotImplemented 构造"接口未实现"错误（阶段 0 桩统一返回）。
// @return WORKFLOW-0501 错误
func errNotImplemented() error {
	return api.NewError("WORKFLOW-0501", "接口未实现", nil)
}

// stub 返回未实现桩结果。
// @return 空 R
// @return WORKFLOW-0501 错误
func stub() (R, error) {
	return R{}, errNotImplemented()
}

// StartProcess 发起流程实例（注入协议变量并生成首步待办）。
// @param ctx 上下文
// @param vo 发起请求
// @return 统一返回（data 含 processInstanceId/taskId 等）
// @return 错误
func (s *Service) StartProcess(ctx context.Context, vo *StartProcessInstanceVo) (R, error) {
	return s.startProcessInternal(ctx, vo)
}

// StartProcessAndFirstStep 发起流程并预存首步执行人（与 StartProcess 同实现：
// 首步预生成在发起链路内必做，两个入口保留以兼容 Java 门面方法集）。
// @param ctx 上下文
// @param vo 发起请求
// @return 统一返回
// @return 错误
func (s *Service) StartProcessAndFirstStep(ctx context.Context, vo *StartProcessInstanceVo) (R, error) {
	return s.startProcessInternal(ctx, vo)
}

// Complete 办结任务（预存下一步校验、权重累计、委派分支、收尾链）。
// @param ctx 上下文
// @param vo 办结请求
// @return 统一返回（data 含 nextTaskIds/finished）
// @return 错误
func (s *Service) Complete(ctx context.Context, vo *CompleteTaskVo) (R, error) {
	if vo == nil {
		return R{}, api.NewError("WORKFLOW-0400", "办结请求不能为空", api.ErrBadRequest)
	}
	result, err := s.comp.complete.Complete(ctx, vo)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("任务审批成功", map[string]any{
		"nextTaskIds": result.NextTaskIDs,
		"finished":    result.Finished,
	}), nil
}

// GetTodoTaskListByIdentityID 按身份 Id 分页查询待办任务。
// @param ctx 上下文
// @param identityID 身份 Id
// @param query 分页参数
// @return 统一返回（data 为分页待办视图）
// @return 错误
func (s *Service) GetTodoTaskListByIdentityID(ctx context.Context, identityID string, query PageQuery) (R, error) {
	page, err := s.comp.query.TodoListByIdentityID(ctx, identityID, query)
	if err != nil {
		return R{}, err
	}
	return ROk(page), nil
}

// GetDoneTaskListByIdentityID 按身份 Id 分页查询已办任务。
// @param ctx 上下文
// @param identityID 身份 Id
// @param query 分页参数
// @return 统一返回（data 为分页已办视图）
// @return 错误
func (s *Service) GetDoneTaskListByIdentityID(ctx context.Context, identityID string, query PageQuery) (R, error) {
	page, err := s.comp.query.DoneListByIdentityID(ctx, identityID, query)
	if err != nil {
		return R{}, err
	}
	return ROk(page), nil
}

// GetNextUserTaskAndUserList 推演下一步节点与候选处理人（P5 实现）。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回
// @return 未实现错误
// SaveAgreeNextActivity 预存"同意"方向下一步节点与处理人（多节点逐个预存，
// 目标为保留代号 end 或无执行人的节点跳过）。
// @param ctx 上下文
// @param req 预存请求
// @return 统一返回
// @return 错误
func (s *Service) SaveAgreeNextActivity(ctx context.Context, req *NextActivityRequest) (R, error) {
	return s.saveAgreeNextInternal(ctx, req)
}

// IsProcessDeployed 判断流程定义是否已部署（按租户解析最新定义）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @return 统一返回（data 为 bool）
// @return 错误
func (s *Service) IsProcessDeployed(ctx context.Context, tenantID, processDefinitionKey string) (R, error) {
	return s.isProcessDeployedInternal(ctx, tenantID, processDefinitionKey)
}

// IsExistRunningProcess 判断业务键下是否存在运行中实例。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @param businessKey 业务单据 ID
// @return 统一返回（data 为 bool）
// @return 错误
func (s *Service) IsExistRunningProcess(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error) {
	return s.isExistRunningProcessInternal(ctx, tenantID, processDefinitionKey, businessKey)
}

// GetWorkflowStatusByBusinessID 查询业务单据流程状态（表单状态机中文值）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @param businessKey 业务单据 ID
// @return 统一返回（data 为状态对象）
// @return 错误
func (s *Service) GetWorkflowStatusByBusinessID(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error) {
	return s.workflowStatusInternal(ctx, tenantID, processDefinitionKey, businessKey)
}

// UpdateProcessGlobalVariable 更新流程实例全局变量（逐个变量设置）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param variables 变量名到值
// @return 统一返回
// @return 错误
func (s *Service) UpdateProcessGlobalVariable(ctx context.Context, processInstanceID string, variables map[string]any) (R, error) {
	if processInstanceID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "流程实例 ID 不能为空", api.ErrBadRequest)
	}
	for name, value := range variables {
		if err := s.engine.RuntimeService().SetVariable(ctx, processInstanceID, name, value); err != nil {
			return R{}, err
		}
	}
	return ROkMsg("变量已更新", nil), nil
}

// GetProcessGlobalVariableByProcessInstanceID 按实例查询全局变量。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param names 变量名列表，nil 表示全部
// @return 统一返回（data 为变量映射）
// @return 错误
func (s *Service) GetProcessGlobalVariableByProcessInstanceID(ctx context.Context, processInstanceID string, names []string) (R, error) {
	values, err := s.engine.RuntimeService().GetVariables(ctx, processInstanceID)
	if err != nil {
		return R{}, err
	}
	return ROk(filterVariables(values, names)), nil
}

// GetProcessGlobalVariableByBusinessID 按业务键查询运行实例的全局变量。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @param names 变量名列表，nil 表示全部
// @return 统一返回（data 为变量映射）
// @return 错误
func (s *Service) GetProcessGlobalVariableByBusinessID(ctx context.Context, tenantID, businessKey string, names []string) (R, error) {
	if businessKey == "" {
		return R{}, api.NewError("WORKFLOW-0400", "业务单据 ID 不能为空", api.ErrBadRequest)
	}
	page, err := s.engine.RuntimeService().QueryProcessInstances(tenant.With(ctx, tenantID), api.ProcessInstanceFilter{
		BusinessKey: businessKey,
		Size:        1,
	})
	if err != nil {
		return R{}, err
	}
	if len(page.Data) == 0 {
		return R{}, api.NewError("WORKFLOW-0404", "业务单据无运行中实例: "+businessKey, api.ErrNotFound)
	}
	values, err := s.engine.RuntimeService().GetVariables(ctx, page.Data[0].ID)
	if err != nil {
		return R{}, err
	}
	return ROk(filterVariables(values, names)), nil
}

// UpdateWorkflowDocumentByProcessInstanceIDOrTaskID 按实例或任务回写表单状态
// （刷新状态机；document 覆盖参数当前由状态机统一计算，保留协议位）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID（与 taskID 至少一个非空）
// @param taskID 任务 ID
// @param document 回写数据（协议兼容参数）
// @return 统一返回
// @return 错误
func (s *Service) UpdateWorkflowDocumentByProcessInstanceIDOrTaskID(ctx context.Context, processInstanceID, taskID string, document map[string]any) (R, error) {
	return s.updateDocumentInternal(ctx, processInstanceID, taskID)
}

// GetTaskMessageListByBusinessID 按业务单据查询任务消息（待办）列表。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @return 统一返回（data 为消息列表）
// @return 错误
func (s *Service) GetTaskMessageListByBusinessID(ctx context.Context, tenantID, businessKey string) (R, error) {
	list, err := s.comp.query.TaskMessageListByBusinessID(ctx, tenantID, businessKey)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// GetTaskHistoryListByProcessInstanceID 按实例查询任务历史（已办）列表。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 统一返回（data 为历史列表）
// @return 错误
func (s *Service) GetTaskHistoryListByProcessInstanceID(ctx context.Context, processInstanceID string) (R, error) {
	list, err := s.comp.query.TaskHistoryListByInstance(ctx, processInstanceID)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// GetTaskHistoryListByBusinessID 按业务单据查询任务历史列表。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @return 统一返回（data 为历史列表）
// @return 错误
func (s *Service) GetTaskHistoryListByBusinessID(ctx context.Context, tenantID, businessKey string) (R, error) {
	list, err := s.comp.query.TaskHistoryListByBusinessID(ctx, tenantID, businessKey)
	if err != nil {
		return R{}, err
	}
	return ROk(list), nil
}

// GetTaskHistoryListByTaskID 按任务查询历史扩展记录。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回（data 为历史记录）
// @return 错误
func (s *Service) GetTaskHistoryListByTaskID(ctx context.Context, taskID string) (R, error) {
	record, err := s.comp.query.TaskHistoryListByTaskID(ctx, taskID)
	if err != nil {
		return R{}, err
	}
	return ROk(record), nil
}

// GetTaskInfoByTaskID 按任务 ID 查询任务详情（含 workflow 扩展列）。
// Java 方法名 geTaskInfoByTaskId（拼写保留）。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回（data 为任务详情）
// @return 错误
func (s *Service) GetTaskInfoByTaskID(ctx context.Context, taskID string) (R, error) {
	info, err := s.comp.query.TaskInfoByTaskID(ctx, taskID)
	if err != nil {
		return R{}, err
	}
	return ROk(info), nil
}

// filterVariables 按变量名列表过滤变量映射（nil 名单返回全量拷贝）。
// @param values 原变量映射
// @param names 保留的变量名，nil 表示全部
// @return 过滤后的映射
func filterVariables(values map[string]any, names []string) map[string]any {
	if names == nil {
		copied := make(map[string]any, len(values))
		for name, value := range values {
			copied[name] = value
		}
		return copied
	}
	filtered := make(map[string]any, len(names))
	for _, name := range names {
		if value, ok := values[name]; ok {
			filtered[name] = value
		}
	}
	return filtered
}
