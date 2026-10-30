package workflow

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
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

// Service 是 WorkflowFacade 的骨架实现（阶段 0 仅提供编译期契约与 501 桩）。
// P1 起按职能文件（start.go/complete.go/back.go/rollback.go 等）逐步填充实现。
type Service struct {
	engine api.Engine // 引擎门面（七服务），通过 api 接口访问引擎
	opts   Options    // 服务配置
}

// 编译期断言：Service 必须满足 api.WorkflowFacade 门面契约。
var _ api.WorkflowFacade = (*Service)(nil)

// NewWorkflowService 创建 workflow 门面服务。
// @param engine 引擎门面（七服务）
// @param options 可选配置（调度开关、时钟注入）
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
	return &Service{engine: engine, opts: opts}
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

// StartProcess 发起流程实例（P1 实现）。
// @param ctx 上下文
// @param vo 发起请求
// @return 统一返回
// @return 未实现错误
func (s *Service) StartProcess(ctx context.Context, vo *StartProcessInstanceVo) (R, error) {
	return stub()
}

// StartProcessAndFirstStep 发起流程并预存首步下一步信息（P1 实现）。
// @param ctx 上下文
// @param vo 发起请求
// @return 统一返回
// @return 未实现错误
func (s *Service) StartProcessAndFirstStep(ctx context.Context, vo *StartProcessInstanceVo) (R, error) {
	return stub()
}

// Complete 办结任务（P1 实现）。
// @param ctx 上下文
// @param vo 办结请求
// @return 统一返回
// @return 未实现错误
func (s *Service) Complete(ctx context.Context, vo *CompleteTaskVo) (R, error) {
	return stub()
}

// BackToStepTask 退回任务到指定步骤（P2 实现）。
// @param ctx 上下文
// @param vo 退回请求
// @return 统一返回
// @return 未实现错误
func (s *Service) BackToStepTask(ctx context.Context, vo *BackTaskVo) (R, error) {
	return stub()
}

// BackTask 退回任务到上一节点（P2 实现）。
// @param ctx 上下文
// @param vo 退回请求
// @return 统一返回
// @return 未实现错误
func (s *Service) BackTask(ctx context.Context, vo *BackTaskVo) (R, error) {
	return stub()
}

// DoRollbackTask 撤回任务（P2 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param taskID 待撤回任务 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 未实现错误
func (s *Service) DoRollbackTask(ctx context.Context, processInstanceID, taskID, operator string) (R, error) {
	return stub()
}

// CompleteToTargetActivity 直送指定目标节点（P2 实现）。
// @param ctx 上下文
// @param vo 办结请求（TargetActivityID 必填）
// @return 统一返回
// @return 未实现错误
func (s *Service) CompleteToTargetActivity(ctx context.Context, vo *CompleteTaskVo) (R, error) {
	return stub()
}

// DoResetProcess 重置流程实例（P2 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 未实现错误
func (s *Service) DoResetProcess(ctx context.Context, processInstanceID, operator string) (R, error) {
	return stub()
}

// GetTodoTaskListByIdentityID 查询待办任务（P1 实现）。
// @param ctx 上下文
// @param identityID 身份 Id
// @param query 分页参数
// @return 统一返回
// @return 未实现错误
func (s *Service) GetTodoTaskListByIdentityID(ctx context.Context, identityID string, query PageQuery) (R, error) {
	return stub()
}

// GetDoneTaskListByIdentityID 查询已办任务（P1 实现）。
// @param ctx 上下文
// @param identityID 身份 Id
// @param query 分页参数
// @return 统一返回
// @return 未实现错误
func (s *Service) GetDoneTaskListByIdentityID(ctx context.Context, identityID string, query PageQuery) (R, error) {
	return stub()
}

// GetNextUserTaskAndUserList 推演下一步节点与候选处理人（P5 实现）。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetNextUserTaskAndUserList(ctx context.Context, taskID string) (R, error) {
	return stub()
}

// SaveAgreeNextActivity 预存"同意"方向下一步节点与处理人（P1 实现；
// 其余 save*NextActivity 变体随 P1 规格确认后补充）。
// @param ctx 上下文
// @param req 预存请求
// @return 统一返回
// @return 未实现错误
func (s *Service) SaveAgreeNextActivity(ctx context.Context, req *NextActivityRequest) (R, error) {
	return stub()
}

// IsProcessDeployed 判断流程定义是否已部署（P1 实现）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @return 统一返回
// @return 未实现错误
func (s *Service) IsProcessDeployed(ctx context.Context, tenantID, processDefinitionKey string) (R, error) {
	return stub()
}

// IsExistRunningProcess 判断业务键下是否存在运行中实例（P1 实现）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @param businessKey 业务单据 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) IsExistRunningProcess(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error) {
	return stub()
}

// GetWorkflowStatusByBusinessID 查询业务单据流程状态（P1 实现）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key
// @param businessKey 业务单据 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetWorkflowStatusByBusinessID(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error) {
	return stub()
}

// UpdateProcessGlobalVariable 更新流程实例全局变量（P1 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param variables 变量名到值
// @return 统一返回
// @return 未实现错误
func (s *Service) UpdateProcessGlobalVariable(ctx context.Context, processInstanceID string, variables map[string]any) (R, error) {
	return stub()
}

// GetProcessGlobalVariableByProcessInstanceID 按实例查询全局变量（P1 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param names 变量名列表，nil 表示全部
// @return 统一返回
// @return 未实现错误
func (s *Service) GetProcessGlobalVariableByProcessInstanceID(ctx context.Context, processInstanceID string, names []string) (R, error) {
	return stub()
}

// GetProcessGlobalVariableByBusinessID 按业务键查询全局变量（P1 实现）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @param names 变量名列表，nil 表示全部
// @return 统一返回
// @return 未实现错误
func (s *Service) GetProcessGlobalVariableByBusinessID(ctx context.Context, tenantID, businessKey string, names []string) (R, error) {
	return stub()
}

// UpdateWorkflowDocumentByProcessInstanceIDOrTaskID 回写业务表单状态（P1 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID（与 taskID 至少一个非空）
// @param taskID 任务 ID
// @param document 回写数据
// @return 统一返回
// @return 未实现错误
func (s *Service) UpdateWorkflowDocumentByProcessInstanceIDOrTaskID(ctx context.Context, processInstanceID, taskID string, document map[string]any) (R, error) {
	return stub()
}

// DoTaskOwnerAdjustment 按节点调整处理人（P4 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param taskKey 目标节点 Key
// @param fromOwner 原处理人，空表示不限
// @param toOwner 新处理人
// @param operator 操作人身份 Id
// @return 统一返回
// @return 未实现错误
func (s *Service) DoTaskOwnerAdjustment(ctx context.Context, processInstanceID, taskKey, fromOwner, toOwner, operator string) (R, error) {
	return stub()
}

// DoTaskOwnerAdjustmentByID 按任务 ID 调整处理人（P4 实现）。
// @param ctx 上下文
// @param taskID 任务 ID
// @param toOwner 新处理人
// @param operator 操作人身份 Id
// @return 统一返回
// @return 未实现错误
func (s *Service) DoTaskOwnerAdjustmentByID(ctx context.Context, taskID, toOwner, operator string) (R, error) {
	return stub()
}

// GetTaskMessageListByBusinessID 按业务单据查询任务消息列表（P1 实现）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetTaskMessageListByBusinessID(ctx context.Context, tenantID, businessKey string) (R, error) {
	return stub()
}

// GetTaskHistoryListByProcessInstanceID 按实例查询任务历史列表（P1 实现）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetTaskHistoryListByProcessInstanceID(ctx context.Context, processInstanceID string) (R, error) {
	return stub()
}

// GetTaskHistoryListByBusinessID 按业务单据查询任务历史列表（P1 实现）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetTaskHistoryListByBusinessID(ctx context.Context, tenantID, businessKey string) (R, error) {
	return stub()
}

// GetTaskHistoryListByTaskID 按任务查询历史扩展记录（P1 实现）。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetTaskHistoryListByTaskID(ctx context.Context, taskID string) (R, error) {
	return stub()
}

// GetTaskInfoByTaskID 按任务 ID 查询任务详情（P1 实现）。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回
// @return 未实现错误
func (s *Service) GetTaskInfoByTaskID(ctx context.Context, taskID string) (R, error) {
	return stub()
}
