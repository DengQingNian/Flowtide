// terminate.go 实现终止/删除/挂起/激活/重激活门面服务（P2 波2-B）。
// 对位 Java ProcessesUtil：
//	doStopProcessByTaskId:675 / doStopProcessByProcessInstanceId:726（软终止收尾）、
//	deleteProcessInstanceById:5216（实例删除收尾）、deleteProcessByBusinessId:5970（业务键批量删除）、
//	suspendProcessInstanceByProcessInstanceId:1300 / activateProcessInstanceByProcessInstanceId:1316
//	（LCZT/LCJH 意见 + 操作记录，next_activity 不动）。
// 引擎级软终止（迁至结束事件）与历史实例复活由 api.WorkflowExtService 承载。
package workflow

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 终止/删除/挂起激活/重激活域错误码（WORKFLOW-27xx 段，
// 对位 Java doStopProcess* / deleteProcessInstanceById / deleteProcessByBusinessId 的 BusinessException 文案）。
const (
	errCodeTermTaskIDBlank       = "WORKFLOW-2701" // 参数 taskId 为空
	errCodeTermIdentityBlank     = "WORKFLOW-2702" // 参数 identityId 为空
	errCodeTermTaskMissing       = "WORKFLOW-2703" // 任务不存在
	errCodeTermPiIDBlank         = "WORKFLOW-2704" // 参数 processInstanceId 为空
	errCodeTermInstanceMissing   = "WORKFLOW-2705" // 流程实例不存在
	errCodeTermSuspended         = "WORKFLOW-2706" // 流程实例已暂停，无法终止流程
	errCodeTermBusinessBlank     = "WORKFLOW-2707" // 参数 businessId 为空
	errCodeTermHistoricMissing   = "WORKFLOW-2708" // 历史流程实例不存在
	errCodeTermDeleteReasonBlank = "WORKFLOW-2709" // 参数 reason 为空
)

// 终止/删除/挂起激活/重激活的固定文案（对位 Java addComment/addOperationRecord 第 5 参）。
const (
	termCommentMessage      = "流程终止" // 软终止意见/操作记录文案
	deleteCommentMessage    = "流程删除" // 实例删除操作记录文案
	deleteDefaultReason     = "删除流程实例" // 运行级联删除的引擎删除原因（Java DeleteFlowableProcessInstanceCmd 第 2 参）
	suspendCommentMessage   = "流程暂停" // 挂起意见/操作记录文案
	activateCommentMessage  = "流程激活" // 激活意见/操作记录文案
	reactivateCommentMsg    = "步骤激活" // 重激活意见/操作记录文案（BZJH）
	instanceListPageSize    = 500       // 实例/任务列表批量查询上限（与 completeListSize 同口径）
)

// TerminateService 是终止/删除/挂起/激活/重激活域服务（P2 波2-B）。
// 引擎级软终止（StopProcessToEvent）与历史实例复活（ReactivateProcessInstance）
// 已由 api.WorkflowExtService 承载，本服务负责校验、挂起门控与收尾账务。
type TerminateService struct {
	engine        api.Engine              // 引擎门面（七服务）
	next          *NextActivityService    // 下一步预生成-消耗域服务（双表清理）
	comments      *CommentService         // 审批意见服务（LCZZ/LCZT/LCJH/BZJH 意见）
	operations    *OperationRecordService // 操作记录服务（LCZZ/LCSC/LCZT/LCJH/BZJH 留痕）
	messages      *MessageExtendService   // 待办扩展服务（待办清理/重生成）
	histories     *HistoryExtendService   // 已办扩展服务（已办清理/重生成）
	documents     *DocumentService        // 业务表单状态服务（终止/重建/刷新）
	documentStore DocumentStore           // 单据存储端口（MarkDeleted 逻辑删除用，未注入时跳过逻辑删除）
	clock         clock.Clock             // 时间源
}

// TerminateOption 是终止域服务的可选注入函数。
type TerminateOption func(*TerminateService)

// WithTerminateDocumentStore 注入单据存储端口（实例删除/业务键删除的 MarkDeleted 逻辑删除用）。
// 未注入时删除链路跳过 document 逻辑删除（DocumentService 未透出该能力）。
// @param store 单据存储端口
// @return 选项
func WithTerminateDocumentStore(store DocumentStore) TerminateOption {
	return func(s *TerminateService) {
		// 空注入视为未注入，保持跳过语义
		if store != nil {
			s.documentStore = store
		}
	}
}

// NewTerminateService 创建终止/删除/挂起激活/重激活域服务。
// @param engine 引擎门面（七服务）
// @param next 下一步预生成域服务
// @param comments 审批意见服务
// @param operations 操作记录服务
// @param messages 待办扩展服务
// @param histories 已办扩展服务
// @param documents 业务表单状态服务
// @param c 时间源，nil 时回退真实时钟
// @param options 可选注入（单据存储端口等）
// @return 终止域服务
func NewTerminateService(engine api.Engine, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService, c clock.Clock, options ...TerminateOption) *TerminateService {
	if c == nil {
		c = clock.Real{}
	}
	service := &TerminateService{
		engine: engine, next: next, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents, clock: c,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// StopByTask 按任务终止流程（对位 Java doStopProcessByTaskId:675）。
// 校验链后委托 StopByInstance 复用软终止收尾链。
// @param ctx 操作上下文
// @param taskID 运行任务 Id
// @param operator 操作人身份 Id
// @return 错误
func (s *TerminateService) StopByTask(ctx context.Context, taskID, operator string) error {
	// 入参校验（对位 Java 逐参 isBlank 校验）
	if strings.TrimSpace(taskID) == "" {
		return apiError(errCodeTermTaskIDBlank, "参数 taskId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return apiError(errCodeTermTaskMissing, "任务不存在")
	}
	// 挂起任务所属实例拒绝终止（Java task.isSuspended → "流程实例已暂停，无法终止流程"）
	if task.Suspended {
		return apiError(errCodeTermSuspended, "流程实例已暂停，无法终止流程")
	}
	return s.StopByInstance(ctx, task.ProcessInstanceID, operator, termCommentMessage)
}

// StopByInstance 按实例软终止流程（对位 Java doStopProcessByProcessInstanceId:726）：
// 引擎 StopProcessToEvent（全部活动执行迁至首个正常结束事件，保留 end 事件历史）→
// LCZZ 意见 + 操作记录 → 删 next_activity 双表 → 待办/已办清理 → document 置已终止。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id
// @param message 终止意见文案，空时取默认"流程终止"
// @return 错误
func (s *TerminateService) StopByInstance(ctx context.Context, processInstanceID, operator, message string) error {
	// 入参校验
	if strings.TrimSpace(processInstanceID) == "" {
		return apiError(errCodeTermPiIDBlank, "参数 processInstanceId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err != nil {
		return apiError(errCodeTermInstanceMissing, "流程实例不存在")
	}
	// 挂起实例拒绝终止（Java processInstance.isSuspended）
	if instance.Suspended {
		return apiError(errCodeTermSuspended, "流程实例已暂停，无法终止流程")
	}
	// 引擎软终止：全部活动执行迁至首个正常结束事件走完（G10，对位 Java
	// moveExecutionsToSingleActivityId(executionIds, endId)）
	if err := s.engine.WorkflowExtService().StopProcessToEvent(ctx, processInstanceID); err != nil {
		return err
	}
	// 收尾文案：空时取 Java 固定文案
	text := firstNonEmpty(strings.TrimSpace(message), termCommentMessage)
	// LCZZ 意见（流程级，Java addComment(identityId, piId, LCZZ, "流程终止")）
	if _, err := s.comments.Add(ctx, CommentRequest{
		ProcessInstanceID: processInstanceID,
		UserID:            operator,
		Message:           text,
		Type:              CommentTypeLCZZ,
	}); err != nil {
		return err
	}
	// LCZZ 操作记录（Java addOperationRecord(null, piId, identityId, null, ...)：
	// 任务为空、isAgree 不适用）
	if _, err := s.operations.Save(ctx, OperationRequest{
		ProcessInstanceID: processInstanceID,
		OwnerID:           operator,
		Message:           text,
		Type:              CommentTypeLCZZ,
	}); err != nil {
		return err
	}
	// 删除下一步骤信息及历史信息（Java deleteNextActivityByProcessInstanceId 双表）
	if err := s.deleteNextActivityAll(ctx, processInstanceID); err != nil {
		return err
	}
	// 删除待办消息及已办消息（Java deleteTaskMessageExtendByProcessInstanceId / deleteTaskHistoryExtendByProcessInstanceId）
	if err := s.messages.DeleteByInstance(ctx, processInstanceID); err != nil {
		return err
	}
	if err := s.histories.DeleteByInstance(ctx, processInstanceID); err != nil {
		return err
	}
	// 表单状态置"已终止"（Java 侧状态机经最后一条 LCZZ 操作记录派生，Go 显式落位）
	return s.documents.Terminate(ctx, processInstanceID)
}

// DeleteInstance 删除流程实例（对位 Java deleteProcessInstanceById:5216）：
// LCSC 操作记录 → 运行实例引擎级联删除（DeleteProcessInstance+原因）或
// 已结束实例历史删除（DeleteHistoricProcessInstance）→ next_activity 双表/待办/已办/
// document（MarkDeleted）级联清理。
// 与 Java 的对照差异：操作记录与直送记录无删除端口（且 Java 亦保留操作记录作审计轨迹），
// 本服务保留 LCSC 及此前记录不删（差异已在注释与报告说明）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id
// @param reason 删除原因，空时取默认"删除流程实例"
// @return 错误
func (s *TerminateService) DeleteInstance(ctx context.Context, processInstanceID, operator, reason string) error {
	// 入参校验
	if strings.TrimSpace(processInstanceID) == "" {
		return apiError(errCodeTermPiIDBlank, "参数 processInstanceId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	// 删除原因缺省取 Java 固定值
	text := firstNonEmpty(strings.TrimSpace(reason), deleteDefaultReason)
	// LCSC 操作记录先行（Java 顺序：addOperationRecord(LCSC) → 删实例），
	// 运行/历史实例此时均可联查补全业务键与租户
	if _, err := s.operations.Save(ctx, OperationRequest{
		ProcessInstanceID: processInstanceID,
		OwnerID:           operator,
		Message:           deleteCommentMessage,
		Type:              CommentTypeLCSC,
	}); err != nil {
		return err
	}
	// 解析业务键与租户（document MarkDeleted 定位键，须在实例删除前解析）
	businessKey, tenantID := s.resolveBusinessScope(ctx, processInstanceID)
	// 运行实例：引擎级联删除（历史记 completed + 删除原因）；已结束实例：历史删除
	if _, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID); err == nil {
		if err := s.engine.RuntimeService().DeleteProcessInstance(ctx, processInstanceID, text); err != nil {
			return err
		}
	} else if err := s.engine.HistoryService().DeleteHistoricProcessInstance(ctx, processInstanceID); err != nil {
		// 运行表查不到且历史删除失败（含历史也不存在）：对位 Java 历史分支
		return err
	}
	// 级联清理：next_activity 双表 → 待办 → 已办 → document（MarkDeleted）
	if err := s.deleteNextActivityAll(ctx, processInstanceID); err != nil {
		return err
	}
	if err := s.messages.DeleteByInstance(ctx, processInstanceID); err != nil {
		return err
	}
	if err := s.histories.DeleteByInstance(ctx, processInstanceID); err != nil {
		return err
	}
	// document 逻辑删除（Java 在 deleteProcessByBusinessId 层删除 document，
	// 按实例删除时同步落位，避免悬挂单据；未注入存储端口时跳过）
	if businessKey != "" && s.documentStore != nil {
		return s.documentStore.MarkDeletedByBusinessID(ctx, businessKey, tenantID)
	}
	return nil
}

// DeleteByBusinessID 按业务单据 Id 批量删除流程（对位 Java deleteProcessByBusinessId:5970）：
// 历史实例（含运行中实例的历史行）逐个 DeleteInstance，再按业务键清扫待办/已办/document。
// @param ctx 操作上下文
// @param tenantID 租户（业务系统代号），空表示不过滤租户
// @param businessKey 业务单据 Id
// @param operator 操作人身份 Id
// @return 错误
func (s *TerminateService) DeleteByBusinessID(ctx context.Context, tenantID, businessKey, operator string) error {
	// 入参校验（对位 Java isBlank 校验）
	if strings.TrimSpace(businessKey) == "" {
		return apiError(errCodeTermBusinessBlank, "参数 businessId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	// 历史实例按业务键查询（Java createHistoricProcessInstanceQuery().processInstanceBusinessKey；
	// Go 引擎对运行中实例同样保留历史行，故该查询同时覆盖运行与已结束实例）
	page, err := s.engine.HistoryService().QueryHistoricProcessInstances(ctx, api.HistoricProcessInstanceFilter{
		BusinessKey: businessKey,
		TenantID:    tenantID,
		Size:        instanceListPageSize,
	})
	if err != nil {
		return err
	}
	// 逐实例删除（每个实例各自落 LCSC 记录与级联清理）
	for _, historic := range page.Data {
		if err := s.DeleteInstance(ctx, historic.ID, operator, deleteDefaultReason); err != nil {
			return err
		}
	}
	// 按业务键清扫扩展数据（Java deleteTaskMessageExtendByBusinessIdId 等；
	// 未注入单据存储端口时跳过 document 逻辑删除）
	if err := s.messages.DeleteByBusinessID(ctx, businessKey, tenantID); err != nil {
		return err
	}
	if err := s.histories.DeleteByBusinessID(ctx, businessKey, tenantID); err != nil {
		return err
	}
	if s.documentStore != nil {
		return s.documentStore.MarkDeletedByBusinessID(ctx, businessKey, tenantID)
	}
	return nil
}

// Suspend 挂起流程实例（对位 Java suspendProcessInstanceByProcessInstanceId:1300）：
// 引擎挂起 + LCZT 意见 + 操作记录；next_activity 不动（Java 同语义，已核实）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id
// @return 错误
func (s *TerminateService) Suspend(ctx context.Context, processInstanceID, operator string) error {
	// 入参校验
	if strings.TrimSpace(processInstanceID) == "" {
		return apiError(errCodeTermPiIDBlank, "参数 processInstanceId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	if _, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID); err != nil {
		return apiError(errCodeTermInstanceMissing, "流程实例不存在")
	}
	// 引擎挂起（挂起后办结/退回/撤回/直送等链路由各自校验拒绝）
	if err := s.engine.RuntimeService().SuspendProcessInstance(ctx, processInstanceID); err != nil {
		return err
	}
	return s.writeStateTailChain(ctx, processInstanceID, operator, CommentTypeLCZT, suspendCommentMessage)
}

// Activate 激活流程实例（对位 Java activateProcessInstanceByProcessInstanceId:1316）：
// 引擎激活 + LCJH 意见 + 操作记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id
// @return 错误
func (s *TerminateService) Activate(ctx context.Context, processInstanceID, operator string) error {
	// 入参校验
	if strings.TrimSpace(processInstanceID) == "" {
		return apiError(errCodeTermPiIDBlank, "参数 processInstanceId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	if _, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID); err != nil {
		return apiError(errCodeTermInstanceMissing, "流程实例不存在")
	}
	// 引擎激活
	if err := s.engine.RuntimeService().ActivateProcessInstance(ctx, processInstanceID); err != nil {
		return err
	}
	return s.writeStateTailChain(ctx, processInstanceID, operator, CommentTypeLCJH, activateCommentMessage)
}

// Reactivate 重激活历史实例（对位 Java ReactivateProcessInstanceCmd）：
// 引擎按历史实例重建运行实例（历史变量按创建时间升序覆盖、发起人重写为操作人）→
// BZJH 意见 + 操作记录 → document 重建（Save upsert）→ 新待办/已办镜像生成 → 表单状态刷新。
// @param ctx 操作上下文
// @param historicProcessInstanceID 历史实例 Id
// @param targetActivityID 落位活动 Id，空串表示回开始事件后首个用户任务
// @param operator 操作人身份 Id（重写发起人）
// @return 错误
func (s *TerminateService) Reactivate(ctx context.Context, historicProcessInstanceID, targetActivityID, operator string) error {
	// 入参校验（历史实例与操作人必填；落位活动允许空 = 回首）
	if strings.TrimSpace(historicProcessInstanceID) == "" {
		return apiError(errCodeTermPiIDBlank, "参数 processInstanceId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeTermIdentityBlank, "参数 identityId 为空")
	}
	// 历史实例定位（document 重建与错误文案均需要；引擎复活前历史行保持原样）
	historic, err := s.engine.HistoryService().GetHistoricProcessInstance(ctx, historicProcessInstanceID)
	if err != nil {
		return apiError(errCodeTermHistoricMissing, "历史流程实例不存在")
	}
	// 引擎复活：历史行 endTime/deleteReason 复位、变量重建、落位活动调度
	if err := s.engine.WorkflowExtService().ReactivateProcessInstance(ctx, historicProcessInstanceID, targetActivityID, operator); err != nil {
		return err
	}
	// BZJH 意见（流程级）
	if _, err := s.comments.Add(ctx, CommentRequest{
		ProcessInstanceID: historicProcessInstanceID,
		UserID:            operator,
		Message:           reactivateCommentMsg,
		Type:              CommentTypeBZJH,
	}); err != nil {
		return err
	}
	// BZJH 操作记录
	if _, err := s.operations.Save(ctx, OperationRequest{
		ProcessInstanceID: historicProcessInstanceID,
		OwnerID:           operator,
		Message:           reactivateCommentMsg,
		Type:              CommentTypeBZJH,
	}); err != nil {
		return err
	}
	// document 重建：既有未删除行整行覆盖，已删除/缺失则重建（Save upsert 语义）
	if err := s.rebuildDocument(ctx, historic, operator); err != nil {
		return err
	}
	// 新落位任务补待办与已办镜像（历史镜像已随复活前的终态清理或不存在）
	if err := s.regenerateTodos(ctx, historicProcessInstanceID, operator); err != nil {
		return err
	}
	// 变量重写后表单状态机刷新
	return s.documents.UpdateByProcessInstanceOrTask(ctx, historicProcessInstanceID, "")
}

// writeStateTailChain 挂起/激活的收尾链：流程级意见 + 操作记录（LCZT/LCJH）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id
// @param commentType 意见类型（LCZT/LCJH）
// @param message 意见文案
// @return 错误
func (s *TerminateService) writeStateTailChain(ctx context.Context, processInstanceID, operator, commentType, message string) error {
	// 流程级意见（Java addComment(identityId, piId, 类型, 文案)）
	if _, err := s.comments.Add(ctx, CommentRequest{
		ProcessInstanceID: processInstanceID,
		UserID:            operator,
		Message:           message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	// 操作记录（Java addOperationRecord(null, piId, identityId, null, 文案, 类型)：
	// 任务为空、isAgree 不适用）
	_, err := s.operations.Save(ctx, OperationRequest{
		ProcessInstanceID: processInstanceID,
		OwnerID:           operator,
		Message:           message,
		Type:              commentType,
	})
	return err
}

// deleteNextActivityAll 清理实例的下一步预生成双表（对位 Java
// workflowNextActivityService + workflowHisNextActivityService 的 deleteNextActivityByProcessInstanceId）。
// 存储端口无"历史表按实例全删"方法，先删运行态全量，再按历史行节点集合走双表删除达成同等净效果。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *TerminateService) deleteNextActivityAll(ctx context.Context, processInstanceID string) error {
	// 运行态全量删除
	if err := s.next.DeleteByProcessInstance(ctx, processInstanceID); err != nil {
		return err
	}
	// 历史表清理：按历史行归并目标节点集合后双表删除（运行态已空，删除为幂等）
	hisRecords, err := s.next.ListHisByInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	activityIDs := make([]string, 0, len(hisRecords))
	seen := make(map[string]bool, len(hisRecords))
	for _, record := range hisRecords {
		// 去重：同节点多执行人多版本记录只保留一个节点键
		if record.NextActivityID == "" || seen[record.NextActivityID] {
			continue
		}
		seen[record.NextActivityID] = true
		activityIDs = append(activityIDs, record.NextActivityID)
	}
	// 无历史行时无需双表删除
	if len(activityIDs) == 0 {
		return nil
	}
	return s.next.DeleteByProcessInstanceAndActivityIDs(ctx, processInstanceID, activityIDs)
}

// resolveBusinessScope 解析实例的业务键与租户（运行优先、历史回退；document MarkDeleted 定位键）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 业务键
// @return 租户
func (s *TerminateService) resolveBusinessScope(ctx context.Context, processInstanceID string) (string, string) {
	// 运行实例优先
	if instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID); err == nil {
		return instance.BusinessKey, instance.TenantID
	}
	// 已结束实例回退历史实例
	if historic, err := s.engine.HistoryService().GetHistoricProcessInstance(ctx, processInstanceID); err == nil {
		return historic.BusinessKey, historic.TenantID
	}
	return "", ""
}

// rebuildDocument 重建业务表单状态（重激活收尾）：按历史实例 upsert document，
// FlowStartTime 沿用历史实例开始时间，分类取流程定义。
// @param ctx 操作上下文
// @param historic 历史实例（引擎复活后 EndTime 已复位）
// @param operator 操作人身份 Id（创建人列）
// @return 错误
func (s *TerminateService) rebuildDocument(ctx context.Context, historic api.HistoricProcessInstance, operator string) error {
	// 业务键缺失（无业务键实例）不落 document，与状态机 resolveBusinessScope 语义一致
	if historic.BusinessKey == "" {
		return nil
	}
	// 定义联查补流程分类（查不到给空，不阻断复活）
	categoryID, categoryName := "", ""
	if definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, historic.ProcessDefinitionID); err == nil {
		categoryID = definition.Key
		categoryName = definition.Name
	}
	startTime := historic.StartTime
	_, err := s.documents.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns:    BaseColumns{Creator: operator, TenantID: historic.TenantID},
		BusinessID:     historic.BusinessKey,
		DocumentID:     historic.BusinessKey,
		FlowTitle:      historic.Name,
		FlowInstanceID: historic.ID,
		FlowCategoryID: categoryID,
		FlowCategory:   categoryName,
		// 复活后状态回到流转中（后续表单刷新按操作记录重算）
		FlowStatus:    FlowStatusRunning,
		FlowStartTime: &startTime,
	}})
	return err
}

// regenerateTodos 为复活实例的当前运行任务补待办与已办镜像（重激活收尾）。
// 逐任务幂等：已有待办或无办理人的任务跳过（与办结链 generateTodosForNewTasks 同口径）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id（审计列创建人）
// @return 错误
func (s *TerminateService) regenerateTodos(ctx context.Context, processInstanceID, operator string) error {
	// 实例与定义联查数据源（循环外取一次）
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err != nil {
		return err
	}
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: instanceListPageSize})
	if err != nil {
		return err
	}
	for i := range page.Data {
		task := page.Data[i]
		// 幂等：已有待办的任务跳过
		if _, found, err := s.messages.GetByTaskID(ctx, task.ID); err != nil {
			return err
		} else if found {
			continue
		}
		// 无办理人（组任务等执行人未落定）暂不生成，待执行人重写钩子补齐
		if task.Assignee == "" {
			continue
		}
		// 组装待办扩展（实例/定义/任务三元补全）
		message := &MessageExtend{
			BaseColumns:       BaseColumns{TenantID: instance.TenantID, Creator: operator},
			TaskID:            task.ID,
			ProcessInstanceID: processInstanceID,
			Title:             instance.Name,
			BusinessID:        instance.BusinessKey,
			TaskStepName:      task.Name,
			TaskStepCode:      task.TaskDefinitionKey,
			OwnerID:           task.Assignee,
			TaskReceiptTime:   &task.CreateTime,
		}
		// 定义联查补流程分类（查不到给空，不阻断待办生成）
		if definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID); err == nil {
			message.FlowCategoryCode = definition.Key
			message.FlowCategoryName = definition.Name
		}
		// 起草人 = 实例发起人
		message.DocumentOwnerID = instance.StartUserID
		if _, err := s.messages.CreateForTask(ctx, message); err != nil {
			return err
		}
		// 已办镜像（待办整行复制语义，与发起/办结链一致）
		if _, err := s.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
			return err
		}
	}
	return nil
}
