package workflow

import (
	"context"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/idgen"
)

// ─── 引擎能力窄接口 ─────────────────────────────────────────────
// 域服务按需依赖引擎能力的最小方法集（api.XxxService 的子集），
// 便于单元测试注入轻量桩；生产装配直接传 engine.TaskService() 等七服务。

// TaskCommentEngine 是评论域服务所需的任务评论能力（api.TaskService 子集）。
type TaskCommentEngine interface {
	// AddComment 添加任务评论（ACT_HI_COMMENT 等价存储）。
	AddComment(ctx context.Context, taskID, userID, message string) (api.Comment, error)
	// AddProcessComment 添加流程实例评论（流程级意见，无任务上下文）。
	AddProcessComment(ctx context.Context, processInstanceID, userID, message string) (api.Comment, error)
	// ListInstanceComments 返回流程实例下的全部评论，含任务评论。
	ListInstanceComments(ctx context.Context, processInstanceID string) ([]api.Comment, error)
}

// TaskHistoryQuery 是历史任务查询能力（api.HistoryService 子集）。
type TaskHistoryQuery interface {
	// GetHistoricTaskInstance 查询历史任务。
	GetHistoricTaskInstance(ctx context.Context, taskID string) (api.HistoricTaskInstance, error)
}

// InstanceQuery 是流程实例查询能力（api.RuntimeService / HistoryService 子集）。
type InstanceQuery interface {
	// GetProcessInstance 查询运行中实例（实例已结束时返回不存在错误）。
	GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error)
	// GetHistoricProcessInstance 查询历史实例（含已结束）。
	GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error)
}

// DefinitionQuery 是流程定义查询能力（api.RepositoryService 子集）。
type DefinitionQuery interface {
	// GetProcessDefinition 按 ID 查询流程定义。
	GetProcessDefinition(ctx context.Context, id string) (api.ProcessDefinition, error)
}

// UserQuery 是身份用户查询能力（api.IdentityService 子集）。
type UserQuery interface {
	// GetUser 查询用户。
	GetUser(ctx context.Context, userID string) (api.User, error)
	// ListUserGroups 返回用户所属组。
	ListUserGroups(ctx context.Context, userID string) ([]api.Group, error)
}

// ─── 请求/视图类型 ─────────────────────────────────────────────

// FirstStepRequest 是首步预生成请求（对位 Java ProcessesUtil#saveFirstTaskUserNextActivity:1142）。
// 语义：首步记录 used=1、taskId=首任务 ID（首个任务尚未办结，记录即视为已消耗，
// 执行人重写钩子对该记录无需再次回填）。
type FirstStepRequest struct {
	ProcessInstanceID string   // 流程实例 Id
	ActivityID        string   // 首个用户任务节点 ID（taskDefinitionKey）
	IdentityIDs       []string // 执行人身份 Id 列表（必填，与 Java 执行人校验一致）
	GroupIDs          []string // 执行人候选组 Id 列表：与 IdentityIDs 等长按下标配对，单值则全部共用，否则留空
	SendIdentityID    string   // 发送人身份 Id（=发起人 operator）
	SendUserName      string   // 发送人姓名
	TaskID            string   // 首个任务 Id（写入 task_id，Java 语义必须携带）
	PreTaskID         string   // 来源任务 Id（首步场景 Java 不设置，保留字段给调用方灵活传入）
	TenantID          string   // 租户
	Creator           string   // 创建人身份 Id（=operator）
}

// NextActivitySaveRequest 是"同意/退回"方向的下一步预生成请求
// （对位 Java saveAgreeNextActivity / saveBackToStepNextActivity：used=0、taskId 空、preTaskId=当前任务）。
type NextActivitySaveRequest struct {
	ProcessInstanceID string   // 流程实例 Id
	ActivityID        string   // 目标节点 ID（end 保留代号由调用方跳过，不落库）
	IdentityIDs       []string // 执行人身份 Id 列表（必填）
	GroupIDs          []string // 候选组 Id 列表：与 IdentityIDs 等长按下标配对，单值则全部共用，否则留空
	PreTaskID         string   // 当前任务 Id（写入 pre_task_id）
	SendIdentityID    string   // 发送人身份 Id
	SendUserName      string   // 发送人姓名
	TenantID          string   // 租户
	Creator           string   // 创建人身份 Id（=operator）
}

// ResolveNextSaveRequest 是"委派处理/转办还原"方向的下一步预生成请求
// （对位 Java saveResolveNextActivity：used=1、taskId=当前任务、groupId 继承既有记录）。
type ResolveNextSaveRequest struct {
	ProcessInstanceID string   // 流程实例 Id
	ActivityID        string   // 目标节点 ID
	IdentityIDs       []string // 执行人身份 Id 列表（必填）
	TaskID            string   // 当前任务 Id（写入 task_id 与匹配条件）
	SendIdentityID    string   // 发送人身份 Id（与既有记录匹配条件之一）
	SendUserName      string   // 发送人姓名
	TenantID          string   // 租户
	Creator           string   // 创建人身份 Id（=operator）
}

// CommentRequest 是添加审批意见请求（对位 Java CommentVo → AddHisCommentCmd）。
type CommentRequest struct {
	TaskID            string // 任务 Id；为空表示流程级意见（AddProcessComment 等价）
	ProcessInstanceID string // 流程实例 Id
	UserID            string // 操作人身份 Id
	Message           string // 意见内容（>3900 截断，见 AddHisCommentCmd 语义）
	Type              string // 意见类型 CommentTypeEnum 代码；空时默认 SP（Java 语义）
}

// CommentView 是意见查询视图：引擎评论 + 联查补全的展示字段。
type CommentView struct {
	api.Comment        // 引擎评论原始字段
	TaskName    string // 任务名称（联历史任务，查不到为空）
	TypeName    string // 意见类型中文名（GetEnumMsgByType 映射，非枚举代码为空）
	UserName    string // 操作人姓名（联身份库，查不到为空）
}

// OperationRequest 是写操作记录请求（对位 Java WorkflowOperationRecordRequest）。
// 任务/定义/业务/租户/处理人信息由服务经引擎查询补全（saveWorkflowOperationRecord 语义）。
type OperationRequest struct {
	TaskID            string // 任务 Id（非空时补全任务步骤与签收/办结时间）
	ProcessInstanceID string // 流程实例 Id（必填）
	OwnerID           string // 处理人身份 Id（必填，补全姓名与组织）
	IsAgree           *int   // 是否同意：1/0，nil 不适用
	Message           string // 操作说明（审批意见内容）
	Type              string // 意见类型代码（必填，如 TJ/SP/TH）
}

// MessageCreateRequest 是待办扩展创建请求。
// 直接复用领域实体承载业务字段与 extend 透传（Java saveMessageExtendInfo 的单任务落库部分）。
type MessageCreateRequest = MessageExtend

// DocumentSaveRequest 是业务表单状态保存请求（upsert by businessId+tenant）。
type DocumentSaveRequest struct {
	Document *Document // 单据状态数据（必填 business_id + tenant_id）
	TaskKey  string    // 提交任务步骤代号；为 Draft 时记录流程开始时间（Java Draft 语义）
}

// ─── 仓储端口 ─────────────────────────────────────────────
// 端口定义在本包（消费者侧），SQL 适配器见 store_sql.go（组合 internal/persistence/workflow），
// 内存实现见 memstore.go（单测与嵌入式无 DB 模式）。

// NextActivityStore 是"下一步预生成-消耗"记账的存储端口（运行态 + 历史双写）。
type NextActivityStore interface {
	// InsertDual 双写运行态与历史表（同 ID 同内容）。
	InsertDual(ctx context.Context, act *NextActivity) error
	// LatestVersion 返回实例下指定节点已消耗（used=1）记录的最大版本号，无记录返回 0。
	LatestVersion(ctx context.Context, processInstanceID, nextActivityID string) (int, error)
	// ListUnusedByInstanceAndActivity 查询实例下指定节点未消耗记录。
	ListUnusedByInstanceAndActivity(ctx context.Context, processInstanceID, nextActivityID string) ([]*NextActivity, error)
	// ListUnusedByInstance 查询实例下全部未消耗记录（执行人重写钩子读取）。
	ListUnusedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error)
	// GetFirstUnusedByActivity 按创建时间升序返回实例下指定节点的第一条未消耗记录。
	GetFirstUnusedByActivity(ctx context.Context, processInstanceID, nextActivityID string) (*NextActivity, bool, error)
	// ListByPreTask 查询以上一任务为来源的运行态记录。
	ListByPreTask(ctx context.Context, preTaskID string) ([]*NextActivity, error)
	// ListByTaskID 查询已挂到任务上的运行态记录。
	ListByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error)
	// ListUsedByInstance 查询实例下已回写任务 ID 的记录（轨迹补全）。
	ListUsedByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error)
	// MarkUsedAndWriteBack 置 used=1 并回写任务 ID（运行态与历史表同步，仅未消耗记录生效）。
	MarkUsedAndWriteBack(ctx context.Context, ids []string, taskID string) error
	// ListHisByInstance 查询实例的全部历史记录（轨迹回放，按版本升序）。
	ListHisByInstance(ctx context.Context, processInstanceID string) ([]*NextActivity, error)
	// ListHisByTaskID 查询历史表中任务相关且非自环（pre_task_id != taskID）的记录。
	ListHisByTaskID(ctx context.Context, taskID string) ([]*NextActivity, error)
	// DeleteByProcessInstance 删除实例的全部运行态记录。
	DeleteByProcessInstance(ctx context.Context, processInstanceID string) error
	// DeleteByProcessInstanceAndActivityIDs 删除实例下指定节点集合的运行态与历史记录。
	DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, nextActivityIDs []string) error
	// DeleteForBackToStep 退回清理：删除实例下目标节点（含已消耗）的全部记录。
	DeleteForBackToStep(ctx context.Context, processInstanceID, nextActivityID string) error
	// DeleteForRollback 撤回清理：删除实例下以 taskID 为来源、目标节点之外的记录（仅运行态，Java 语义）。
	DeleteForRollback(ctx context.Context, processInstanceID, preTaskID, keepActivityID string) error
	// DeleteForResolve 处理委派清理：删除实例下指定节点+任务+执行人的记录。
	DeleteForResolve(ctx context.Context, processInstanceID, nextActivityID, taskID, identityID string) error
	// FindAssignSource 按（实例/节点/发送人/任务）定位既有记录（转办/委派/还原继承组用）。
	FindAssignSource(ctx context.Context, processInstanceID, nextActivityID, sendIdentityID, taskID string) (*NextActivity, bool, error)
}

// MessageExtendStore 是待办扩展的存储端口。
type MessageExtendStore interface {
	// Save 插入一条待办扩展（id 由调用方生成）。
	Save(ctx context.Context, m *MessageExtend) error
	// GetByTaskID 按任务 ID 查询待办扩展。
	GetByTaskID(ctx context.Context, taskID string) (*MessageExtend, bool, error)
	// ListByInstance 查询实例下全部待办扩展。
	ListByInstance(ctx context.Context, processInstanceID string) ([]*MessageExtend, error)
	// ListByBusinessID 按业务单据与租户查询待办扩展。
	ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*MessageExtend, error)
	// ListByOwner 按处理人分页查询待办扩展（租户过滤），返回当前页与总数。
	ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*MessageExtend, int64, error)
	// DeleteByTaskID 按任务删除待办扩展。
	DeleteByTaskID(ctx context.Context, taskID string) error
	// DeleteByTaskIDAndOwner 按任务与处理人删除待办扩展。
	DeleteByTaskIDAndOwner(ctx context.Context, taskID, ownerID string) error
	// DeleteByInstance 按实例删除全部待办扩展。
	DeleteByInstance(ctx context.Context, processInstanceID string) error
	// DeleteByBusinessID 按业务单据与租户删除待办扩展。
	DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error
	// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除待办扩展（退回/撤回清理）。
	DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error
	// UpdateRead 标记待办已读并回写阅读时间（按任务+处理人）。
	UpdateRead(ctx context.Context, taskID, ownerID string, readTime time.Time) error
	// UpdateDelay 标记待办已延时并回写延时时间点。
	UpdateDelay(ctx context.Context, taskID string, delayTime time.Time) error
	// SetPlanFinishTime 设置计划办结时间（nil 表示清空）。
	SetPlanFinishTime(ctx context.Context, taskID string, planFinishTime *time.Time) error
}

// HistoryExtendStore 是已办扩展的存储端口。
type HistoryExtendStore interface {
	// Save 插入一条已办扩展（id 由调用方生成）。
	Save(ctx context.Context, h *HistoryExtend) error
	// GetByTaskID 按任务 ID 查询已办扩展。
	GetByTaskID(ctx context.Context, taskID string) (*HistoryExtend, bool, error)
	// ListByInstance 查询实例下全部已办扩展（按办结时间/创建时间排序，聚合轨迹）。
	ListByInstance(ctx context.Context, processInstanceID string) ([]*HistoryExtend, error)
	// ListByBusinessID 按业务单据与租户查询已办扩展。
	ListByBusinessID(ctx context.Context, businessID, tenantID string) ([]*HistoryExtend, error)
	// ListByOwner 按处理人分页查询已办扩展（租户过滤），返回当前页与总数。
	ListByOwner(ctx context.Context, ownerID, tenantID string, page PageQuery) ([]*HistoryExtend, int64, error)
	// UpdateFinishTime 办结回写：置（任务+处理人）记录的办结时间，并删除同任务其他处理人的记录
	//（对位 Java updateByTaskIdAndOwnerId）。
	UpdateFinishTime(ctx context.Context, taskID, ownerID string, finishTime time.Time) error
	// DeleteByTaskID 按任务删除已办扩展。
	DeleteByTaskID(ctx context.Context, taskID string) error
	// DeleteByInstance 按实例删除全部已办扩展。
	DeleteByInstance(ctx context.Context, processInstanceID string) error
	// DeleteByBusinessID 按业务单据与租户删除已办扩展。
	DeleteByBusinessID(ctx context.Context, businessID, tenantID string) error
	// DeleteByProcessInstanceAndActivityIDs 按实例与步骤代号集合删除已办扩展（退回/撤回清理）。
	DeleteByProcessInstanceAndActivityIDs(ctx context.Context, processInstanceID string, stepCodes []string) error
}

// OperationRecordStore 是操作记录（意见留痕）的存储端口。
type OperationRecordStore interface {
	// Save 插入一条操作记录（id 由调用方生成）。
	Save(ctx context.Context, r *OperationRecord) error
	// ListByTask 查询任务的操作记录（按处理时间升序）。
	ListByTask(ctx context.Context, taskID string) ([]*OperationRecord, error)
	// ListByInstance 查询实例的操作记录（按处理时间升序）。
	ListByInstance(ctx context.Context, processInstanceID string) ([]*OperationRecord, error)
	// ListByBusinessID 查询业务单据的操作记录（按处理时间升序）。
	ListByBusinessID(ctx context.Context, businessID string) ([]*OperationRecord, error)
	// ListByBusinessIDs 查询多个业务单据的操作记录（按处理时间升序）。
	ListByBusinessIDs(ctx context.Context, businessIDs []string) ([]*OperationRecord, error)
	// ListSignable 查询"可签名"记录：type ∈ {TJ,SP,TZ} 且步骤代号非空（Java getSignableOperationRecordListByBusinessIds）。
	ListSignable(ctx context.Context, businessIDs []string) ([]*OperationRecord, error)
	// CountByInstance 统计实例的操作记录数（状态机"未提交/流转中"判据）。
	CountByInstance(ctx context.Context, processInstanceID string) (int, error)
	// LastTypeByInstance 返回实例最后一条（按处理时间）操作记录的类型的类型代码，无记录返回空串。
	LastTypeByInstance(ctx context.Context, processInstanceID string) (string, error)
}

// DocumentStore 是业务表单状态（workflow_document）的存储端口。
type DocumentStore interface {
	// Save 按业务单据+租户 upsert：存在则更新非主键/非创建列，不存在则插入。
	Save(ctx context.Context, d *Document) error
	// GetByBusinessID 按业务单据与租户查询未删除单据。
	GetByBusinessID(ctx context.Context, businessID, tenantID string) (*Document, bool, error)
	// GetByInstance 按流程实例查询单据。
	GetByInstance(ctx context.Context, processInstanceID string) (*Document, bool, error)
	// UpdateByInstance 按实例更新指定列（changes 为列名→新值，nil 值表示置 NULL）。
	UpdateByInstance(ctx context.Context, processInstanceID string, changes map[string]any) error
	// MarkDeletedByBusinessID 按业务单据与租户逻辑删除。
	MarkDeletedByBusinessID(ctx context.Context, businessID, tenantID string) error
}

// ─── 公共工具 ─────────────────────────────────────────────

// DomainClock 是域服务统一时间源别名（与 internal/clock.Clock 一致）。
type DomainClock = clock.Clock

// DefaultIDGen 返回默认 ID 生成函数（UUIDv7，时间有序）。
// @return ID 生成函数
func DefaultIDGen() func() string {
	gen := idgen.NewUUIDv7()
	return gen.NewID
}

// fillBase 填充实体公共审计列：ID 为空时生成，Create/UpdateTime 为零值时取当前时间。
// @param base 公共列指针
// @param newID ID 生成函数
// @param now 当前时间函数
// @param creator 创建人
// @param tenantID 租户
func fillBase(base *BaseColumns, newID func() string, now func() time.Time, creator, tenantID string) {
	if base.ID == "" && newID != nil {
		base.ID = newID()
	}
	if base.CreateTime.IsZero() {
		base.CreateTime = now()
	}
	if base.UpdateTime.IsZero() {
		base.UpdateTime = now()
	}
	if creator != "" {
		base.Creator = creator
		base.Updater = creator
	}
	// 租户仅在缺失时补齐，不覆盖实体既有租户
	if base.TenantID == "" {
		base.TenantID = tenantID
	}
}

// apiError 构造领域层参数错误（统一 WORKFLOW-1xxx 段，入参非法）。
// @param code 错误码
// @param message 错误消息
// @return 领域错误
func apiError(code, message string) error {
	return api.NewError(code, message, api.ErrBadRequest)
}

// firstNonEmpty 返回第一个非空字符串。
// @param values 候选值
// @return 非空值，全空返回空串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// pairGroupID 计算第 i 个执行人的候选组 Id：组列表与执行人列表等长按下标配对，
// 组列表单值时全部共用，否则无组。
// @param groupIDs 组 Id 列表
// @param index 执行人下标
// @param total 执行人总数
// @return 组 Id（可为空串）
func pairGroupID(groupIDs []string, index, total int) string {
	// 组与执行人一一对应（Java identityOfGroup 逐人取组的等价形态）
	if len(groupIDs) == total {
		return groupIDs[index]
	}
	// 单组共享：全部执行人同属一个候选组
	if len(groupIDs) == 1 {
		return groupIDs[0]
	}
	return ""
}

// cleanIdentityIDs 过滤空白与字面量 "null" 的执行人 Id（Java save*NextActivity 同名清洗）。
// @param ids 原始执行人 Id 列表
// @return 清洗后的列表
func cleanIdentityIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		// 与 Java 一致：isNotBlank 过滤（含纯空白）+ 剔除字面量 "null"，避免脏执行人落库
		if strings.TrimSpace(id) == "" || id == "null" {
			continue
		}
		out = append(out, id)
	}
	return out
}
