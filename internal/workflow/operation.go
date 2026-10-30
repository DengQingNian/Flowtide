package workflow

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// OperationRecordService 是操作记录（意见留痕）域服务。
// 行为规格对位 Java WorkflowOperationRecordServiceImpl#saveWorkflowOperationRecord:52：
// 请求仅携带任务/实例/处理人/意见，任务步骤、流程定义、业务键、租户、处理人姓名与组织
// 信息由服务经引擎查询补全；可签名记录只取 TJ/SP/TZ 三类。
type OperationRecordService struct {
	store       OperationRecordStore // 存储端口
	history     TaskHistoryQuery     // 历史任务查询（补任务步骤与签收/办结时间）
	instances   InstanceQuery        // 实例查询（补定义/业务键/租户）
	definitions DefinitionQuery     // 定义查询（补流程分类）
	users       UserQuery            // 身份查询（补处理人姓名与组）
	clock       clock.Clock          // 时间源
	newID       func() string        // ID 生成函数
}

// NewOperationRecordService 创建操作记录域服务。
// @param store 存储端口
// @param history 历史任务查询（engine.HistoryService()）
// @param instances 实例查询（engine.RuntimeService()/HistoryService()）
// @param definitions 定义查询（engine.RepositoryService()）
// @param users 身份查询（engine.IdentityService()）
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewOperationRecordService(store OperationRecordStore, history TaskHistoryQuery, instances InstanceQuery, definitions DefinitionQuery, users UserQuery, c clock.Clock) *OperationRecordService {
	if c == nil {
		c = clock.Real{}
	}
	return &OperationRecordService{store: store, history: history, instances: instances, definitions: definitions, users: users, clock: c, newID: DefaultIDGen()}
}

// Save 写入一条操作记录（按 Java saveWorkflowOperationRecord 语义补全可空字段）。
// @param ctx 操作上下文
// @param req 操作请求（ProcessInstanceID/OwnerID/Type 必填）
// @return 落库后的记录
// @return 错误
func (s *OperationRecordService) Save(ctx context.Context, req OperationRequest) (*OperationRecord, error) {
	// 与 Java 一致的最小入参：实例 + 处理人 + 类型（类型驱动状态机与可签名查询）
	if req.ProcessInstanceID == "" || req.OwnerID == "" || req.Type == "" {
		return nil, apiError("WORKFLOW-1201", "操作记录缺少实例/处理人/类型")
	}
	record := &OperationRecord{
		BaseColumns:       BaseColumns{},
		TaskID:            req.TaskID,
		ProcessInstanceID: req.ProcessInstanceID,
		Message:           req.Message,
		IsAgree:           req.IsAgree,
		OwnerID:           req.OwnerID,
		Type:              req.Type,
	}
	s.fillTaskInfo(ctx, req.TaskID, record)
	s.fillInstanceInfo(ctx, req.ProcessInstanceID, record)
	s.fillOwnerInfo(ctx, req.OwnerID, record)
	now := s.clock.Now()
	record.DealTime = &now
	// 审计列：创建人 = 处理人；租户已由实例信息填充时尊重实例值
	fillBase(&record.BaseColumns, s.newID, s.clock.Now, req.OwnerID, record.TenantID)
	if err := s.store.Save(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

// fillTaskInfo 经历史任务补全任务步骤与签收/办结时间（Java taskId 非空分支）。
// @param ctx 操作上下文
// @param taskID 任务 Id，空跳过
// @param record 待补全记录
func (s *OperationRecordService) fillTaskInfo(ctx context.Context, taskID string, record *OperationRecord) {
	// 任务为空（流程级操作）时跳过
	if taskID == "" {
		return
	}
	task, err := s.history.GetHistoricTaskInstance(ctx, taskID)
	if err != nil {
		// 任务查不到时保留入参，不阻断留痕（Java 此处直接取值会空指针，Go 侧容错）
		return
	}
	record.TaskID = task.ID
	record.ProcessInstanceID = task.ProcessInstanceID
	record.TaskStepCode = task.TaskDefinitionKey
	record.TaskStepName = task.Name
	started := task.StartTime
	record.TaskReceiptTime = &started
	finished := s.clock.Now()
	record.TaskFinishTime = &finished
}

// fillInstanceInfo 经运行/历史实例补全业务键、租户与流程分类（Java 运行优先、历史回退）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param record 待补全记录
func (s *OperationRecordService) fillInstanceInfo(ctx context.Context, processInstanceID string, record *OperationRecord) {
	definitionID := ""
	// 运行实例优先；实例已结束时回退历史实例（与 Java 查询顺序一致）
	if instance, err := s.instances.GetProcessInstance(ctx, processInstanceID); err == nil {
		record.BusinessID = instance.BusinessKey
		record.TenantID = instance.TenantID
		definitionID = instance.ProcessDefinitionID
	} else if historic, err := s.instances.GetHistoricProcessInstance(ctx, processInstanceID); err == nil {
		record.BusinessID = historic.BusinessKey
		record.TenantID = historic.TenantID
		definitionID = historic.ProcessDefinitionID
	}
	// 定义查不到时流程分类留空
	if definitionID == "" {
		return
	}
	if definition, err := s.definitions.GetProcessDefinition(ctx, definitionID); err == nil {
		record.FlowCategoryCode = definition.Key
		record.FlowCategoryName = definition.Name
	}
}

// fillOwnerInfo 经身份库补全处理人姓名与所属组（Java 账号+组织的等价形态；
// 引擎身份库无组织路径概念，多组以逗号聚合）。
// @param ctx 操作上下文
// @param ownerID 处理人 Id
// @param record 待补全记录
func (s *OperationRecordService) fillOwnerInfo(ctx context.Context, ownerID string, record *OperationRecord) {
	if user, err := s.users.GetUser(ctx, ownerID); err == nil {
		record.OwnerName = userDisplayName(user)
	}
	// 组信息查不到时留空，不阻断留痕
	groups, err := s.users.ListUserGroups(ctx, ownerID)
	if err != nil || len(groups) == 0 {
		return
	}
	names := make([]string, 0, len(groups))
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
		ids = append(ids, group.ID)
	}
	record.OwnerGroup = strings.Join(names, ",")
	record.OwnerGroupID = strings.Join(ids, ",")
	// 引擎组无层级路径，路径列与简名列同值（协议保持非空即可）
	record.OwnerGroupPaths = record.OwnerGroup
	record.OwnerGroupPathIDs = record.OwnerGroupID
}

// ListByInstance 查询实例的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *OperationRecordService) ListByInstance(ctx context.Context, processInstanceID string) ([]*OperationRecord, error) {
	return s.store.ListByInstance(ctx, processInstanceID)
}

// ListByBusinessIDs 查询多个业务单据的操作记录（按处理时间升序）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 记录列表
// @return 错误
func (s *OperationRecordService) ListByBusinessIDs(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	return s.store.ListByBusinessIDs(ctx, businessIDs)
}

// ListSignable 查询"可签名"记录（type ∈ {TJ,SP,TZ} 且步骤代号非空）。
// @param ctx 操作上下文
// @param businessIDs 业务单据 Id 集合
// @return 记录列表
// @return 错误
func (s *OperationRecordService) ListSignable(ctx context.Context, businessIDs []string) ([]*OperationRecord, error) {
	return s.store.ListSignable(ctx, businessIDs)
}
