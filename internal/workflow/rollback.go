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
//     重做任务的新待办由收尾链 generateTodosAfterRollback 重建（其待办镜像已在
//     办结时消耗，执行人重写钩子只回填办理人不建待办）。
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
	// 待办重建：撤回恢复的重做任务其待办镜像已在办结时消耗，需重建
	// （与 BackService.generateTodosAfterBack 同构；执行人重写钩子只回填办理人不建待办）
	if _, err := s.generateTodosAfterRollback(ctx, hisTask.ProcessInstanceID); err != nil {
		return err
	}
	// 表单状态机刷新（Java 无显式调用，状态机按操作记录派生；Go 显式刷新对齐办结链）
	return s.documents.UpdateByProcessInstanceOrTask(ctx, hisTask.ProcessInstanceID, "")
}

// generateTodosAfterRollback 为撤回后实例的当前运行任务重建待办镜像
// （对位 BackService.generateTodosAfterBack 同构语义：幂等跳过已有待办、
// 无处理人任务等待办补齐链路、待办与已办镜像双写）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 生成待办的任务 Id 列表（含已存在待办的任务）
// @return 错误
func (s *RollbackService) generateTodosAfterRollback(ctx context.Context, processInstanceID string) ([]string, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: completeListSize})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(page.Data))
	for i := range page.Data {
		current := page.Data[i]
		// 幂等：同任务已有待办时跳过（Java count==0 才插入）
		if _, found, err := s.messages.GetByTaskID(ctx, current.ID); err != nil {
			return nil, err
		} else if found {
			ids = append(ids, current.ID)
			continue
		}
		// 无处理人（组任务等执行人未落定场景）暂不生成待办，待执行人重写钩子补齐
		if current.Assignee == "" {
			continue
		}
		message, err := s.buildRollbackMessageExtend(ctx, processInstanceID, current)
		if err != nil {
			return nil, err
		}
		if _, err := s.messages.CreateForTask(ctx, message); err != nil {
			return nil, err
		}
		// 已办镜像（Java saveMessageExtendInfo 同步插入历史行）
		if _, err := s.histories.CreateForTask(ctx, &HistoryExtend{MessageExtend: *message}); err != nil {
			return nil, err
		}
		ids = append(ids, current.ID)
	}
	return ids, nil
}

// buildRollbackMessageExtend 组装撤回重做任务的待办扩展（实例/定义/预存发送人信息联查补全，
// 与 BackService.buildBackMessageExtend 同构）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param task 重做任务
// @return 待办扩展
// @return 错误
func (s *RollbackService) buildRollbackMessageExtend(ctx context.Context, processInstanceID string, task api.Task) (*MessageExtend, error) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err != nil {
		return nil, err
	}
	definition, err := s.engine.RepositoryService().GetProcessDefinition(ctx, task.ProcessDefinitionID)
	if err != nil {
		return nil, err
	}
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: instance.TenantID},
		TaskID:            task.ID,
		ProcessInstanceID: processInstanceID,
		Title:             instance.Name,
		BusinessID:        instance.BusinessKey,
		TaskStepName:      task.Name,
		TaskStepCode:      task.TaskDefinitionKey,
		FlowCategoryCode:  definition.Key,
		FlowCategoryName:  definition.Name,
		OwnerID:           task.Assignee,
		OwnerName:         s.displayName(ctx, task.Assignee),
		TaskReceiptTime:   &task.CreateTime,
	}
	// 发送人信息取该节点第一条未消耗预存记录（撤回预存记录/执行人重写未消耗记录均可追溯）
	if record, found, err := s.next.GetFirstUnusedByActivity(ctx, processInstanceID, task.TaskDefinitionKey); err == nil && found {
		message.SenderID = record.SendIdentityID
		message.Sender = record.SendUserName
		// 发送步骤取来源任务名（Java hisTask.getName/TaskDefinitionKey）
		if record.PreTaskID != "" {
			if his, err := s.engine.HistoryService().GetHistoricTaskInstance(ctx, record.PreTaskID); err == nil {
				message.SenderStepName = his.Name
				message.SenderStepCode = his.TaskDefinitionKey
			}
		}
	}
	// 起草人 = 实例发起人
	message.DocumentOwnerID = instance.StartUserID
	message.DocumentOwnerName = s.displayName(ctx, instance.StartUserID)
	return message, nil
}

// displayName 查询用户展示名（查不到回退 Id 本身）。
// @param ctx 操作上下文
// @param userID 身份 Id
// @return 展示名
func (s *RollbackService) displayName(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	if user, err := s.engine.IdentityService().GetUser(ctx, userID); err == nil {
		if name := userDisplayName(user); name != "" {
			return name
		}
	}
	return userID
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
