package workflow

import (
	"context"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// HistoryExtendService 是已办扩展域服务（ACT_HI_TASKINST 1:1）。
// 行为规格对位 Java WorkflowTaskHistoryExtendServiceImpl：已办记录由待办扩展整行复制生成；
// 办结回写按（任务+处理人）置办结时间并删除同任务其他处理人的记录（updateByTaskIdAndOwnerId）。
type HistoryExtendService struct {
	store HistoryExtendStore // 存储端口
	clock clock.Clock        // 时间源
	newID func() string      // ID 生成函数
}

// NewHistoryExtendService 创建已办扩展域服务。
// @param store 存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewHistoryExtendService(store HistoryExtendStore, c clock.Clock) *HistoryExtendService {
	if c == nil {
		c = clock.Real{}
	}
	return &HistoryExtendService{store: store, clock: c, newID: DefaultIDGen()}
}

// CreateForTask 为任务创建一条已办扩展（Java 由待办扩展复制生成，此处直接承接完整字段；
// 办结时间初始为空，由 UpdateFinishTime 回写）。
// @param ctx 操作上下文
// @param req 已办扩展数据（必填 task_id/owner_id/tenant_id）
// @return 落库后的已办扩展
// @return 错误
func (s *HistoryExtendService) CreateForTask(ctx context.Context, req *HistoryExtend) (*HistoryExtend, error) {
	// 与 Java saveMessageExtendInfo 的已办镜像一致：任务/处理人/租户必填
	if req == nil || req.TaskID == "" || req.OwnerID == "" || req.TenantID == "" {
		return nil, apiError("WORKFLOW-1401", "已办扩展缺少任务/处理人/租户")
	}
	entity := *req
	// 已办初始无办结时间，调用方显式传入时尊重原值
	if entity.TaskFinishTime == nil {
		entity.TaskFinishTime = nil
	}
	// 签收时间缺省取当前时间（待办复制场景应显式传入任务创建时间）
	if entity.TaskReceiptTime == nil {
		now := s.clock.Now()
		entity.TaskReceiptTime = &now
	}
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, firstNonEmpty(entity.SenderID, entity.OwnerID), entity.TenantID)
	if err := s.store.Save(ctx, &entity); err != nil {
		return nil, err
	}
	return &entity, nil
}

// UpdateFinishTime 办结回写：置（任务+处理人）记录的办结时间并删除同任务其他处理人的记录
// （时间为零值时取当前时间，对位 Java updateByTaskIdAndOwnerId）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 处理人 Id
// @param finishTime 办结时间
// @return 错误
func (s *HistoryExtendService) UpdateFinishTime(ctx context.Context, taskID, ownerID string, finishTime time.Time) error {
	// 任务/处理人是回写定位键
	if taskID == "" || ownerID == "" {
		return apiError("WORKFLOW-1402", "办结回写缺少任务/处理人")
	}
	// 零值时间统一取当前时间（Java new Date() 语义）
	if finishTime.IsZero() {
		finishTime = s.clock.Now()
	}
	return s.store.UpdateFinishTime(ctx, taskID, ownerID, finishTime)
}

// ListByInstance 查询实例下全部已办扩展（按办结时间/创建时间排序聚合轨迹）。
// TODO-P5：his_step 快照排序字段（sort）接入后改为按步骤排序号聚合。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 列表
// @return 错误
func (s *HistoryExtendService) ListByInstance(ctx context.Context, processInstanceID string) ([]*HistoryExtend, error) {
	return s.store.ListByInstance(ctx, processInstanceID)
}

// ListByBusinessID 按业务单据与租户查询已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 列表
// @return 错误
func (s *HistoryExtendService) ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*HistoryExtend, error) {
	return s.store.ListByBusinessID(ctx, businessID, tenantID)
}

// ListByOwner 按处理人分页查询已办（租户过滤，已办列表主查询）。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param tenantID 租户
// @param page 分页参数
// @return 分页结果
// @return 错误
func (s *HistoryExtendService) ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) (PageResult[*HistoryExtend], error) {
	data, total, err := s.store.ListByOwner(ctx, ownerID, tenantID, page)
	if err != nil {
		return PageResult[*HistoryExtend]{}, err
	}
	return PageResult[*HistoryExtend]{Data: data, Total: total, Start: page.Start, Size: page.Size}, nil
}

// GetByTaskID 按任务查询已办扩展。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 已办扩展
// @return 是否存在
// @return 错误
func (s *HistoryExtendService) GetByTaskID(ctx context.Context, taskID string) (*HistoryExtend, bool, error) {
	return s.store.GetByTaskID(ctx, taskID)
}

// DeleteByTaskID 按任务删除已办扩展（办结流转/删除实例时级联）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 错误
func (s *HistoryExtendService) DeleteByTaskID(ctx context.Context, taskID string) error {
	return s.store.DeleteByTaskID(ctx, taskID)
}

// DeleteByInstance 按实例删除全部已办扩展。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *HistoryExtendService) DeleteByInstance(ctx context.Context, processInstanceID string) error {
	return s.store.DeleteByInstance(ctx, processInstanceID)
}

// DeleteByBusinessID 按业务单据与租户删除已办扩展。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 错误
func (s *HistoryExtendService) DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error {
	return s.store.DeleteByBusinessID(ctx, businessID, tenantID)
}

// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除已办扩展（退回/撤回清理）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param stepCodes 步骤代号集合
// @return 错误
func (s *HistoryExtendService) DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error {
	return s.store.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, stepCodes)
}
