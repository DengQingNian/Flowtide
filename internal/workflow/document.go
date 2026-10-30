package workflow

import (
	"context"
	"time"

	"github.com/DengQingNian/Flowtide/internal/clock"
)

// DocumentService 是业务表单状态（workflow_document）域服务。
// 行为规格对位 Java WorkflowDocumentServiceImpl#updateWorkflowDocumentByProcessInstanceIdOrTaskId:96 的状态机：
//  1. 最后一条操作记录为 LCZZ → 已终止（清空待办数据 + 终止时间）；
//  2. 实例已结束（运行表查不到且历史实例有结束时间）→ 已完成（清空待办数据 + 完成时间）；
//  3. 其余 → 操作记录数 <=1 为"未提交"、>1 为"流转中"；
//     待办聚合：document_url/flow_task_id/flow_action_id/flow_action/flow_current_step_name/
//     flow_current_persons 取待办扩展去重逗号串；
//     待办步骤代号命中签收类（Sign/Sign1/Sign2/EngineerSign）→ flow_status=已完成；
//     办结任务 Key == Draft → 记流程开始时间。
type DocumentService struct {
	store     DocumentStore        // 单据存储端口
	msg       MessageExtendStore   // 待办扩展端口（聚合待办人）
	ops       OperationRecordStore // 操作记录端口（状态判据）
	instances InstanceQuery        // 实例查询（业务键/租户/是否结束）
	tasks     TaskHistoryQuery     // 历史任务查询（补任务 Key，Draft 判定）
	clock     clock.Clock          // 时间源
	newID     func() string        // ID 生成函数
}

// NewDocumentService 创建业务表单状态域服务。
// @param store 单据存储端口
// @param msg 待办扩展端口
// @param ops 操作记录端口
// @param instances 实例查询（engine.RuntimeService()/HistoryService()）
// @param tasks 历史任务查询（engine.HistoryService()）
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewDocumentService(store DocumentStore, msg MessageExtendStore, ops OperationRecordStore, instances InstanceQuery, tasks TaskHistoryQuery, c clock.Clock) *DocumentService {
	if c == nil {
		c = clock.Real{}
	}
	return &DocumentService{store: store, msg: msg, ops: ops, instances: instances, tasks: tasks, clock: c, newID: DefaultIDGen()}
}

// Save 按业务单据+租户 upsert 表单状态（flow_status 缺省"未提交"；TaskKey 为 Draft 时记流程开始时间）。
// @param ctx 操作上下文
// @param req 保存请求（Document 必填 business_id + tenant_id）
// @return 落库后的单据
// @return 错误
func (s *DocumentService) Save(ctx context.Context, req DocumentSaveRequest) (*Document, error) {
	// 业务单据与租户是 upsert 定位键
	if req.Document == nil || req.Document.BusinessID == "" || req.Document.TenantID == "" {
		return nil, apiError("WORKFLOW-1501", "表单状态缺少业务单据/租户")
	}
	entity := *req.Document
	// Java saveWorkflowDocument 初始状态固定"未提交"，调用方显式传入时尊重原值
	if entity.FlowStatus == "" {
		entity.FlowStatus = FlowStatusNotSubmitted
	}
	// Draft 步骤提交时记录流程开始时间（Java Draft 语义）
	if IsDraftStep(req.TaskKey) && entity.FlowStartTime == nil {
		now := s.clock.Now()
		entity.FlowStartTime = &now
	}
	fillBase(&entity.BaseColumns, s.newID, s.clock.Now, entity.Creator, entity.TenantID)
	if err := s.store.Save(ctx, &entity); err != nil {
		return nil, err
	}
	return &entity, nil
}

// GetByBusinessID 按业务单据与租户查询表单状态。
// @param ctx 操作上下文
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 单据
// @return 是否存在
// @return 错误
func (s *DocumentService) GetByBusinessID(ctx context.Context, businessID, tenantID string) (*Document, bool, error) {
	return s.store.GetByBusinessID(ctx, businessID, tenantID)
}

// GetByInstance 按流程实例查询表单状态。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 单据
// @return 是否存在
// @return 错误
func (s *DocumentService) GetByInstance(ctx context.Context, processInstanceID string) (*Document, bool, error) {
	return s.store.GetByInstance(ctx, processInstanceID)
}

// UpdateByProcessInstanceOrTask 每次办结后刷新表单状态（状态机主入口；
// procInstID 与 taskID 至少一个非空，taskID 可反查实例）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param taskID 任务 Id
// @return 错误
func (s *DocumentService) UpdateByProcessInstanceOrTask(ctx context.Context, processInstanceID, taskID string) error {
	// 实例 ID 缺省时经历史任务反查（Java taskID 分支）
	if processInstanceID == "" {
		task, err := s.tasks.GetHistoricTaskInstance(ctx, taskID)
		if err != nil {
			// 无法定位实例时静默跳过（Java businessKey 为空直接返回 R.ok）
			return nil
		}
		processInstanceID = task.ProcessInstanceID
	}
	businessKey, tenantID := s.resolveBusinessScope(ctx, processInstanceID)
	// 与 Java 一致：定位不到业务键时不做任何更新
	if businessKey == "" {
		return nil
	}
	status, err := s.computeStatus(ctx, processInstanceID)
	if err != nil {
		return err
	}
	// 终态：清空待办聚合数据并记录完成/终止时间（Java cleanWorkflowDataByProcessInstanceId）
	if status == FlowStatusFinished || status == FlowStatusTerminated {
		return s.cleanInstance(ctx, processInstanceID, status)
	}
	return s.refreshPending(ctx, processInstanceID, businessKey, tenantID, taskID, status)
}

// Terminate 终止状态标记辅助：置"已终止"并记录终止时间、清空待办聚合数据。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *DocumentService) Terminate(ctx context.Context, processInstanceID string) error {
	return s.cleanInstance(ctx, processInstanceID, FlowStatusTerminated)
}

// Finish 完成状态标记辅助：置"已完成"并记录完成时间、清空待办聚合数据。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *DocumentService) Finish(ctx context.Context, processInstanceID string) error {
	return s.cleanInstance(ctx, processInstanceID, FlowStatusFinished)
}

// CleanByProcessInstance 清空实例的待办聚合数据（状态按状态机重算，终态记录对应时间）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *DocumentService) CleanByProcessInstance(ctx context.Context, processInstanceID string) error {
	status, err := s.computeStatus(ctx, processInstanceID)
	if err != nil {
		return err
	}
	return s.cleanInstance(ctx, processInstanceID, status)
}

// computeStatus 按状态机推导当前流程状态（对位 Java getWorkflowStatusByBusinessId）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 状态值（FlowStatus* 常量）
// @return 错误
func (s *DocumentService) computeStatus(ctx context.Context, processInstanceID string) (string, error) {
	// 判据一：最后一条操作记录为 LCZZ（流程终止）→ 已终止（Java 最优先判定）
	lastType, err := s.ops.LastTypeByInstance(ctx, processInstanceID)
	if err != nil {
		return "", err
	}
	if lastType == CommentTypeLCZZ {
		return FlowStatusTerminated, nil
	}
	// 判据二：运行表查不到且历史实例已结束 → 已完成
	if _, err := s.instances.GetProcessInstance(ctx, processInstanceID); err != nil {
		historic, histErr := s.instances.GetHistoricProcessInstance(ctx, processInstanceID)
		// 实例完全不存在（防御分支，Java"未知状态"）
		if histErr != nil {
			return FlowStatusUnknown, nil
		}
		if historic.EndTime != nil {
			return FlowStatusFinished, nil
		}
	}
	// 判据三：操作记录数 <=1 为"未提交"（仅发起记录），否则"流转中"（Java getWorkflowStatusByProcessInstanceId）
	count, err := s.ops.CountByInstance(ctx, processInstanceID)
	if err != nil {
		return "", err
	}
	// Java count == 2 ? "未提交" : "流转中"（getWorkflowStatusByProcessInstanceId 返回 2 表示 count<=1）
	if count <= 1 {
		return FlowStatusNotSubmitted, nil
	}
	return FlowStatusRunning, nil
}

// refreshPending 流转中刷新：聚合待办扩展为逗号串并回写（含签收/草稿/Draft 特判）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param businessKey 业务单据 Id
// @param tenantID 租户
// @param taskID 办结任务 Id（Draft 判定来源，可为空）
// @param status 当前状态
// @return 错误
func (s *DocumentService) refreshPending(ctx context.Context, processInstanceID, businessKey, tenantID, taskID, status string) error {
	messages, err := s.msg.ListByBusinessID(ctx, businessKey, tenantID)
	if err != nil {
		return err
	}
	changes := map[string]any{"flow_status": status}
	// 有待办时聚合去重逗号串（Java distinct + String.join 语义）；无待办保持原列不动
	if len(messages) > 0 {
		urls := make([]string, 0, len(messages))
		taskIDs := make([]string, 0, len(messages))
		stepCodes := make([]string, 0, len(messages))
		stepNames := make([]string, 0, len(messages))
		persons := make([]string, 0, len(messages))
		for _, msg := range messages {
			urls = appendUnique(urls, msg.TargetURL)
			taskIDs = appendUnique(taskIDs, msg.TaskID)
			stepCodes = appendUnique(stepCodes, msg.TaskStepCode)
			stepNames = appendUnique(stepNames, msg.TaskStepName)
			persons = appendUnique(persons, msg.OwnerName)
		}
		changes["document_url"] = joinNonEmpty(urls)
		changes["flow_task_id"] = joinNonEmpty(taskIDs)
		changes["flow_action_id"] = joinNonEmpty(stepCodes)
		changes["flow_action"] = joinNonEmpty(stepNames)
		// Java：flowCurrentStepNames = flowActions（与步骤名称聚合同值）
		changes["flow_current_step_name"] = joinNonEmpty(stepNames)
		changes["flow_current_persons"] = joinNonEmpty(persons)
	}
	// 办结任务为 Draft（草稿提交）时记录流程开始时间
	if taskID != "" {
		if task, err := s.tasks.GetHistoricTaskInstance(ctx, taskID); err == nil && IsDraftStep(task.TaskDefinitionKey) {
			changes["flow_start_time"] = s.clock.Now()
		}
	}
	// 待办步骤代号命中签收类 → 状态直接置"已完成"（Java signStepCodeList 语义，不清空聚合列）
	if containsAny(stepCodesOf(messages), signStepCodes) {
		changes["flow_status"] = FlowStatusFinished
	}
	return s.store.UpdateByInstance(ctx, processInstanceID, changes)
}

// signStepCodes 是签收类步骤代号全集（与 types.go 保留代号一致，Java signStepCodeList）。
var signStepCodes = []string{StepCodeSign, StepCodeSign1, StepCodeSign2, StepCodeEngineerSign}

// stepCodesOf 提取待办扩展的步骤代号列表。
// @param messages 待办扩展列表
// @return 步骤代号列表
func stepCodesOf(messages []*MessageExtend) []string {
	codes := make([]string, 0, len(messages))
	for _, msg := range messages {
		codes = append(codes, msg.TaskStepCode)
	}
	return codes
}

// containsAny 判断列表中是否存在集合内元素。
// @param values 值列表
// @param targets 目标集合
// @return 是否存在
func containsAny(values []string, targets []string) bool {
	set := make(map[string]bool, len(targets))
	for _, target := range targets {
		set[target] = true
	}
	for _, value := range values {
		if set[value] {
			return true
		}
	}
	return false
}

// appendUnique 追加去重（保持首次出现顺序，Java stream().distinct() 语义），空串跳过。
// @param values 原列表
// @param value 新值
// @return 新列表
func appendUnique(values []string, value string) []string {
	// 空串不参与聚合，避免逗号串出现空段
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// joinNonEmpty 逗号拼接列表（全空返回空串，避免出现纯逗号串）。
// @param values 值列表
// @return 逗号串
func joinNonEmpty(values []string) string {
	result := ""
	for _, value := range values {
		// 跳过空段，保证聚合串无空段
		if value == "" {
			continue
		}
		if result != "" {
			result += ","
		}
		result += value
	}
	return result
}

// resolveBusinessScope 解析实例的业务键与租户（运行优先、历史回退）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 业务键
// @return 租户
func (s *DocumentService) resolveBusinessScope(ctx context.Context, processInstanceID string) (string, string) {
	// 运行实例优先；已结束实例回退历史实例（Java 同序）
	if instance, err := s.instances.GetProcessInstance(ctx, processInstanceID); err == nil {
		return instance.BusinessKey, instance.TenantID
	}
	if historic, err := s.instances.GetHistoricProcessInstance(ctx, processInstanceID); err == nil {
		return historic.BusinessKey, historic.TenantID
	}
	return "", ""
}

// cleanInstance 清空实例的待办聚合数据并记录状态与时间（对位 Java cleanWorkflowDataByProcessInstanceId）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param status 终态状态值
// @return 错误
func (s *DocumentService) cleanInstance(ctx context.Context, processInstanceID, status string) error {
	now := s.clock.Now()
	changes := map[string]any{
		"flow_status":            status,
		"document_url":           nil,
		"flow_task_id":           nil,
		"flow_action_id":         nil,
		"flow_action":            nil,
		"flow_current_step_name": nil,
		"flow_current_persons":   nil,
	}
	// 完成态记完成时间，终止态记终止时间（Java <if> 分支语义）
	if status == FlowStatusFinished {
		changes["flow_finish_time"] = now
	}
	if status == FlowStatusTerminated {
		changes["flow_termination_time"] = now
	}
	return s.store.UpdateByInstance(ctx, processInstanceID, changes)
}

// MarkFinishTime 显式回写完成时间（补偿场景使用）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param finishTime 完成时间
// @return 错误
func (s *DocumentService) MarkFinishTime(ctx context.Context, processInstanceID string, finishTime time.Time) error {
	return s.store.UpdateByInstance(ctx, processInstanceID, map[string]any{"flow_finish_time": finishTime})
}
