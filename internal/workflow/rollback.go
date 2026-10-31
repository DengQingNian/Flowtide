// rollback.go 实现撤回门面服务（P2 波2-B）。
// 对位 Java ProcessesUtil#doRollbackTask:4373：
// 校验（历史任务已办结、实例运行中）→ 引擎 WorkflowExtService().RollbackTask
// （引擎内含撤回工厂判定与 ROLLBACK_* 变量写入，下游执行人重写钩子消费该变量回填办理人）→
// 收尾：CH 意见 + 操作记录(type=CH) + DeleteForRollback（清下一步预存，
// 对位 Java deleteRollbackWorkflowNextActivity）+ 待办遗留清理 + 表单状态刷新。
package workflow

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 撤回域错误码（WORKFLOW-26xx 段，对位 Java doRollbackTask 的 BusinessException 文案）。
const (
	errCodeRollbackTaskIDBlank   = "WORKFLOW-2601" // 参数 taskId 为空
	errCodeRollbackUserBlank     = "WORKFLOW-2602" // 参数 userCode 为空
	errCodeRollbackTaskMissing   = "WORKFLOW-2603" // 撤回任务不存在
	errCodeRollbackUnfinished    = "WORKFLOW-2604" // 任务尚未办结，无法撤回
	errCodeRollbackInstanceEnded = "WORKFLOW-2605" // 流程已结束，无法撤回
	errCodeRollbackSuspended     = "WORKFLOW-2606" // 流程已暂停，请联系管理员激活
	errCodeRollbackPiMismatch    = "WORKFLOW-2607" // 撤回任务不属于该流程实例
)

// rollbackCommentMessage 是撤回意见固定文案（Java addComment 第 5 参）。
const rollbackCommentMessage = "任务撤回"

// RollbackService 是撤回域服务（P2 波2-B）。
// 引擎级撤回（工厂判定/后继任务与历史清理/ROLLBACK_* 变量）已由
// api.WorkflowExtService().RollbackTask 承载，本服务只做前置校验与收尾账务。
type RollbackService struct {
	engine     api.Engine              // 引擎门面（七服务）
	next       *NextActivityService    // 下一步预生成-消耗域服务（撤回清理）
	comments   *CommentService         // 审批意见服务（CH 意见）
	operations *OperationRecordService // 操作记录服务（CH 留痕）
	messages   *MessageExtendService   // 待办扩展服务（待办遗留清理）
	histories  *HistoryExtendService   // 已办扩展服务（预留收尾扩展点）
	documents  *DocumentService        // 业务表单状态服务（状态机刷新）
	clock      clock.Clock             // 时间源
}

// NewRollbackService 创建撤回域服务。
// @param engine 引擎门面（七服务）
// @param next 下一步预生成域服务
// @param comments 审批意见服务
// @param operations 操作记录服务
// @param messages 待办扩展服务
// @param histories 已办扩展服务
// @param documents 业务表单状态服务
// @param c 时间源，nil 时回退真实时钟
// @return 撤回域服务
func NewRollbackService(engine api.Engine, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService, c clock.Clock) *RollbackService {
	if c == nil {
		c = clock.Real{}
	}
	return &RollbackService{
		engine: engine, next: next, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents, clock: c,
	}
}

// Rollback 撤回任务（对位 Java doRollbackTask:4373）。
// 链路：入参校验 → 历史任务定位与已办结校验 → 实例运行中/未挂起校验 →
// 引擎撤回（工厂判定 + ROLLBACK_* 变量 + 后继任务/历史清理 + 重做执行重建）→
// 收尾（CH 意见/CH 操作记录/DeleteForRollback/待办遗留清理/表单状态刷新）。
// 与 Java 的对照差异（均为 Go 侧规格决策）：
//  1. CH 意见为流程级意见：引擎评论 API 绑定运行态任务，被撤回任务已办结
//     （引擎侧无对应运行任务），Java 直插 ACT_HI_COMMENT 的任务级路径不可达；
//     类型权威台账在操作记录 type=CH（意见服务既有约定）。
//  2. 收尾新增待办遗留清理：引擎撤回会删除后继运行任务，其待办扩展成为悬挂行
//     （Java 无任务删除监听，悬挂行残留；Go 侧按"删除原任务关联待办遗留"规格清理）。
//     重做任务的新待办由执行人重写钩子与办结链生成，本服务不重复生成。
//  3. processInstanceID 与历史任务归属不一致时拒绝（Java 静默覆盖为历史任务归属，
//     Go 侧显式暴露调用方传参错误）。
//
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id（协议入参，与历史任务归属不一致时报错）
// @param taskID 待撤回任务 Id（历史任务 Id，必须已办结）
// @param operator 操作人身份 Id（撤回后任务的办理人，对位 Java userCode/assignee）
// @return 错误
func (s *RollbackService) Rollback(ctx context.Context, processInstanceID, taskID, operator string) error {
	// 入参校验（对位 Java 逐参 isBlank 校验）
	if strings.TrimSpace(taskID) == "" {
		return apiError(errCodeRollbackTaskIDBlank, "参数 taskId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeRollbackUserBlank, "参数 userCode 为空")
	}
	// 历史任务定位（Java historyService.createHistoricTaskInstanceQuery().taskId）
	hisTask, err := s.engine.HistoryService().GetHistoricTaskInstance(ctx, taskID)
	if err != nil {
		return apiError(errCodeRollbackTaskMissing, "撤回任务不存在")
	}
	// 归属一致性校验：入参实例 Id 与历史任务归属不一致时拒绝（Java 静默覆盖，Go 显式报错）
	if processInstanceID != "" && processInstanceID != hisTask.ProcessInstanceID {
		return apiError(errCodeRollbackPiMismatch, "撤回任务不属于该流程实例")
	}
	// 实例运行中校验：运行表查不到即已结束（Java isSuspended 在前、Go 运行态校验等价合并）
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, hisTask.ProcessInstanceID)
	if err != nil {
		return apiError(errCodeRollbackInstanceEnded, "该流程已完成，无法进行任务撤回")
	}
	// 挂起实例拒绝撤回（Java isSuspended → "流程已暂停，请联系管理员激活"；
	// Java 挂起校验先于命令内 endTime 校验，Go 同序）
	if instance.Suspended {
		return apiError(errCodeRollbackSuspended, "流程已暂停，请联系管理员激活")
	}
	// 已办结校验（Java RollbackCmd 内 endTime==null 拒绝的等价前置；文案对位）
	if hisTask.EndTime == nil {
		return apiError(errCodeRollbackUnfinished, "任务正在执行，无法撤回")
	}
	// 撤回前快照运行任务 Id（待办遗留清理定位用）
	beforeTaskIDs, err := s.listInstanceTaskIDs(ctx, hisTask.ProcessInstanceID)
	if err != nil {
		return err
	}
	// 引擎撤回（工厂判定 + ROLLBACK_* 变量 + 后继任务与历史清理 + 重做执行）
	if err := s.engine.WorkflowExtService().RollbackTask(ctx, taskID, operator); err != nil {
		return err
	}
	// 收尾链（Java 顺序：意见 → 引擎 → deleteRollbackWorkflowNextActivity；
	// 意见与操作记录在引擎成功后写，保证失败不留痕）
	return s.writeRollbackTailChain(ctx, hisTask, operator, beforeTaskIDs)
}

// writeRollbackTailChain 撤回收尾链：
// CH 意见 → CH 操作记录 → DeleteForRollback → 待办遗留清理 → 表单状态刷新。
// @param ctx 操作上下文
// @param hisTask 被撤回的历史任务
// @param operator 操作人身份 Id
// @param beforeTaskIDs 撤回前快照的运行任务 Id 集合
// @return 错误
func (s *RollbackService) writeRollbackTailChain(ctx context.Context, hisTask api.HistoricTaskInstance, operator string, beforeTaskIDs []string) error {
	// CH 意见（流程级，Java 任务级直插的等价形态，见 Rollback 注释差异 1）
	if _, err := s.comments.Add(ctx, CommentRequest{
		ProcessInstanceID: hisTask.ProcessInstanceID,
		UserID:            operator,
		Message:           rollbackCommentMessage,
		Type:              CommentTypeCH,
	}); err != nil {
		return err
	}
	// CH 操作记录（任务定位携带历史任务 Id；历史任务行已被引擎撤回删除时
	// 服务内静默保留入参，不阻断留痕）
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            hisTask.ID,
		ProcessInstanceID: hisTask.ProcessInstanceID,
		OwnerID:           operator,
		Message:           rollbackCommentMessage,
		Type:              CommentTypeCH,
	}); err != nil {
		return err
	}
	// 删除撤回节点后下一步骤信息（Java deleteRollbackWorkflowNextActivity：
	// 实例 + pre_task_id=被撤回任务 + 目标节点之外，仅运行态；保留目标节点记录供重做消耗）
	if err := s.next.DeleteForRollback(ctx, hisTask.ProcessInstanceID, hisTask.ID, hisTask.TaskDefinitionKey); err != nil {
		return err
	}
	// 待办遗留清理：撤回后已不存在的运行任务，其待办扩展为悬挂行（差异 2）
	if err := s.cleanupStaleTodos(ctx, hisTask, beforeTaskIDs); err != nil {
		return err
	}
	// 表单状态机刷新（Java 无显式调用，状态机按操作记录派生；Go 显式刷新对齐办结链）
	return s.documents.UpdateByProcessInstanceOrTask(ctx, hisTask.ProcessInstanceID, "")
}

// listInstanceTaskIDs 快照实例当前全部运行任务 Id。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 任务 Id 列表
// @return 错误
func (s *RollbackService) listInstanceTaskIDs(ctx context.Context, processInstanceID string) ([]string, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: completeListSize})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page.Data))
	for _, task := range page.Data {
		ids = append(ids, task.ID)
	}
	return ids, nil
}

// cleanupStaleTodos 删除已不存在的运行任务的待办扩展（撤回遗留清理）。
// 清理范围：被撤回任务自身（任务已办结，运行表必无此任务）+ 快照中有而撤回后已消失的
// 后继任务（引擎撤回删除）；新建任务（含重做任务）天然不在范围内，不受影响。
// @param ctx 操作上下文
// @param hisTask 被撤回的历史任务
// @param beforeTaskIDs 撤回前快照的运行任务 Id 集合
// @return 错误
func (s *RollbackService) cleanupStaleTodos(ctx context.Context, hisTask api.HistoricTaskInstance, beforeTaskIDs []string) error {
	// 被撤回任务自身的待办遗留（正常链路已在办结时删除，此处兜底幂等清理）
	if err := s.messages.DeleteByTaskID(ctx, hisTask.ID); err != nil {
		return err
	}
	// 逐个快照任务检查：已被撤回删除的后继任务（运行表查不到）删除其待办
	for _, taskID := range beforeTaskIDs {
		if _, err := s.engine.TaskService().GetTask(ctx, taskID); err == nil {
			// 任务仍在运行：待办仍有效，跳过
			continue
		}
		// 待办不存在时删除为幂等空操作（存储端口语义）
		if err := s.messages.DeleteByTaskID(ctx, taskID); err != nil {
			return err
		}
	}
	return nil
}
