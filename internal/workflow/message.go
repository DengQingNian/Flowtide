package workflow

import (
	"context"
	"log/slog"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// MessageExtendService 是待办扩展域服务（待办中心唯一数据源）。
// 行为规格对位 Java WorkflowTaskMessageExtendServiceImpl：任务级落库按（实例/任务/处理人）
// 幂等去重（saveMessageExtendInfo 的 count==0 语义）；办结流转时先删待办、
// 由 HistoryExtendService 承接已办记录。
type MessageExtendService struct {
	store MessageExtendStore // 存储端口
	clock clock.Clock        // 时间源
	newID func() string      // ID 生成函数
	// delegateHook 是待办新建委托钩子：新建待办落库后同步触发一次
	// 委托判定（对位 Java saveMessageExtendInfo 尾部对 delegateTaskMessageList 的自动委托段）。
	// 失败仅记告警日志、不阻断待办创建；幂等重建（同任务已有待办直接返回既有行）不触发。
	delegateHook func(ctx context.Context, msg *MessageExtend) (*MessageExtend, error)
}

// NewMessageExtendService 创建待办扩展域服务。
// @param store 存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewMessageExtendService(store MessageExtendStore, c clock.Clock) *MessageExtendService {
	if c == nil {
		c = clock.Real{}
	}
	return &MessageExtendService{store: store, clock: c, newID: DefaultIDGen()}
}

// InstallDelegateHook 注入待办新建委托钩子（由装配层在 DelegateRuleService 构建后调用，
// 挂接 DelegateRuleService.onTodoCreate；重复注入覆盖旧钩子，nil 卸载）。
// 钩子契约：仅对新建待办触发一次；钩子内如重建待办（改派）必须保证重建行已带
// ConsignerID 并在钩子入口跳过该类待办（对位 Java「不支持第三人再将工作委托」），
// 否则会因重建再次触发钩子造成递归。
// @param hook 钩子函数：入参为已落库的待办，返回改派后的待办（未命中原样返回）与错误
func (s *MessageExtendService) InstallDelegateHook(hook func(ctx context.Context, msg *MessageExtend) (*MessageExtend, error)) {
	s.delegateHook = hook
}

// CreateForTask 为任务创建一条待办扩展（含业务字段与 extend 透传）。
// 按（任务）唯一键幂等：已存在时直接返回既有记录（Java count==0 才插入语义，
// DDL 上 task_id 唯一）。
// @param ctx 操作上下文
// @param req 待办扩展创建请求（必填 task_id/process_instance_id/owner_id/tenant_id）
// @return 落库后的待办扩展
// @return 错误
func (s *MessageExtendService) CreateForTask(ctx context.Context, req *MessageCreateRequest) (*MessageExtend, error) {
	// 与 Java saveTaskMessage 一致的最小必填校验（标题/租户/处理人）
	if req == nil || req.TaskID == "" || req.ProcessInstanceID == "" || req.OwnerID == "" || req.TenantID == "" {
		return nil, apiError("WORKFLOW-1301", "待办扩展缺少任务/实例/处理人/租户")
	}
	// 幂等：同任务已有待办时返回既有记录，不重复插入
	if existing, found, err := s.store.GetByTaskID(ctx, req.TaskID); err != nil {
		return nil, err
	} else if found {
		return existing, nil
	}
	entity := *req
	// 签收时间缺省取当前时间（Java 取任务创建时间，调用方传入时尊重原值）
	if entity.TaskReceiptTime == nil {
		now := s.clock.Now()
		entity.TaskReceiptTime = &now
	}
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, firstNonEmpty(entity.SenderID, entity.OwnerID), entity.TenantID)
	if err := s.store.Save(ctx, &entity); err != nil {
		return nil, err
	}
	// 待办新建委托钩子（对位 Java saveMessageExtendInfo 自动委托段）：
	// 仅新建行触发（幂等重建已在上方 return）；失败记告警不阻断待办创建——
	// 与 Java 异步委托段 try-catch 后仅日志的口径一致，补偿走 TriggerDelegateScan 运维端点；
	// 失败时回滚钩子对实体字段的就地改写，保证调用方拿到的待办与存储行一致
	if s.delegateHook != nil {
		snapshot := entity
		if _, hookErr := s.delegateHook(ctx, &entity); hookErr != nil {
			entity = snapshot
			slog.Warn("workflow-message: 待办委托钩子执行失败（不阻断待办创建，可经 delegate/scheduled/trigger 补偿）",
				"taskId", entity.TaskID, "owner", entity.OwnerID, "error", hookErr)
		}
	}
	return &entity, nil
}

// DeleteByTaskID 按任务删除待办扩展（办结流转时调用）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (s *MessageExtendService) DeleteByTaskID(ctx context.Context, taskID string) error {
	return s.store.DeleteByTaskID(ctx, taskID)
}

// DeleteByTaskIDAndOwner 按任务与处理人删除待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @return 错误
func (s *MessageExtendService) DeleteByTaskIDAndOwner(ctx context.Context, taskID, ownerID string) error {
	return s.store.DeleteByTaskIDAndOwner(ctx, taskID, ownerID)
}

// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除待办扩展（退回/撤回清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param stepCodes 步骤代号集合
// @return 错误
func (s *MessageExtendService) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	return s.store.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, stepCodes)
}

// DeleteByInstance 按实例删除全部待办扩展（删除实例时级联清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *MessageExtendService) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	return s.store.DeleteByInstance(ctx, processInstanceID)
}

// DeleteByBusinessID 按业务单据与租户删除待办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (s *MessageExtendService) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	return s.store.DeleteByBusinessID(ctx, businessID, tenantID)
}

// UpdateRead 标记待办已读并回写阅读时间（时间为零值时取当前时间）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param readTime 阅读时间
// @return 错误
func (s *MessageExtendService) UpdateRead(ctx context.Context, taskID, ownerID string, readTime time.Time) error {
	// 零值时间统一取当前时间，避免落库空值
	if readTime.IsZero() {
		readTime = s.clock.Now()
	}
	return s.store.UpdateRead(ctx, taskID, ownerID, readTime)
}

// UpdateDelay 标记待办已延时并回写延时时间点（时间为零值时取当前时间）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param delayTime 延时时间点
// @return 错误
func (s *MessageExtendService) UpdateDelay(ctx context.Context, taskID string, delayTime time.Time) error {
	// 零值时间统一取当前时间
	if delayTime.IsZero() {
		delayTime = s.clock.Now()
	}
	return s.store.UpdateDelay(ctx, taskID, delayTime)
}

// SetPlanFinishTime 设置计划办结时间（nil 表示清空；催办/自动办结调度使用）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param planFinishTime 计划办结时间
// @return 错误
func (s *MessageExtendService) SetPlanFinishTime(ctx context.Context, taskID string, planFinishTime *time.Time) error {
	return s.store.SetPlanFinishTime(ctx, taskID, planFinishTime)
}

// ListByOwner 按处理人分页查询待办（租户过滤，待办列表主查询）。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 分页结果
// @return 错误
func (s *MessageExtendService) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) (PageResult[*MessageExtend], error) {
	data, total, err := s.store.ListByOwner(ctx, ownerID, tenantID, page)
	if err != nil {
		return PageResult[*MessageExtend]{}, err
	}
	return PageResult[*MessageExtend]{Data: data, Total: total, Start: page.Start, Size: page.Size}, nil
}

// ListByBusinessID 按业务单据与租户查询待办扩展（表单状态聚合数据源）。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 列表
// @return 错误
func (s *MessageExtendService) ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*MessageExtend, error) {
	return s.store.ListByBusinessID(ctx, businessID, tenantID)
}

// ListByInstance 查询实例下全部待办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *MessageExtendService) ListByInstance(ctx context.Context, processInstanceID string) ([]*MessageExtend, error) {
	return s.store.ListByInstance(ctx, processInstanceID)
}

// GetByTaskID 按任务查询待办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展
// @return 是否存在
// @return 错误
func (s *MessageExtendService) GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error) {
	return s.store.GetByTaskID(ctx, taskID)
}

// CopyAsDelegate 复制待办为委托镜像（P4 委托域接入，镜像字段 entrust_* 随 P4 落地）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param entrustID 受托人 Id
// @param entrustName 受托人姓名
// @return 始终返回未实现错误
// @return WORKFLOW-0501 错误
func (s *MessageExtendService) CopyAsDelegate(ctx context.Context, taskID, entrustID, entrustName string) (*MessageExtend, error) {
	// P4 预留：委托镜像表 workflow_delegate_task 与镜像字段 entrust_* 随 P4 委托域接入
	_, _ = taskID, entrustID
	_ = entrustName
	return nil, errNotImplemented()
}
