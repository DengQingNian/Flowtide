package workflow

import (
	"context"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 直送域错误码（WORKFLOW-25xx 段，对位 Java completeToTargetActivity:4550 的
// BusinessException 文案）。
const (
	errCodeDirectVoBlank     = "WORKFLOW-2501" // 直送请求为空
	errCodeDirectTaskIDBlank = "WORKFLOW-2502" // 参数 taskId 为空
	errCodeDirectTargetBlank = "WORKFLOW-2503" // 参数 targetActivityId 为空
	errCodeDirectUserBlank   = "WORKFLOW-2504" // 参数 userCode 为空
	errCodeDirectTaskMissing = "WORKFLOW-2505" // 任务不存在
	errCodeDirectSuspended   = "WORKFLOW-2506" // 流程已暂停，请联系管理员激活
	errCodeDirectNoExecutor  = "WORKFLOW-2507" // 目标步骤未设置执行者
)

// DirectService 是直送（提交流程至某节点）域服务（P2-B）。
//
// 与 complete.go 自由选路兜底的语义衔接（不可改其文件，注释说明演进关系）：
// CompleteService.Complete 在常规办结后按"实际落位节点 != 指定目标"做引擎迁移兜底
// （applyFreeSelectFallback），并在迁移发生时补记直送轨迹；本服务是"以直送为主语义"
// 的引擎级门面——办结动作经 WorkflowExtService().CompleteToTarget 在单条引擎命令内
// 直接迁往目标（Java completeToTargetActivity 伪造连线的等价能力），不再依赖
// "先按模型出线办结、后纠偏迁移"的两段式路径。两入口并存：常规办结 + 自由选路
// 参数继续走 Complete（兜底仍生效），显式直送场景走本门面。
type DirectService struct {
	engine     api.Engine              // 引擎门面（七服务）
	meta       *ModelMetaService       // 模型元数据（目标节点执行者配置校验）
	next       *NextActivityService    // 下一步预生成-消耗记账
	comments   *CommentService         // 审批意见
	operations *OperationRecordService // 操作记录
	messages   *MessageExtendService   // 待办扩展
	histories  *HistoryExtendService   // 已办扩展
	documents  *DocumentService        // 业务表单状态机
	completeTo CompleteToActivityStore // 直送记录（workflow_complete_to_activity）
	users      UserQuery               // 身份查询（姓名展示），nil 时保留身份 Id
	clock      clock.Clock             // 时间源
}

// DirectOption 是直送服务选项函数。
type DirectOption func(*DirectService)

// WithDirectUsers 注入身份查询（办理人姓名展示）。
// @param users 身份查询
// @return 选项
func WithDirectUsers(users UserQuery) DirectOption {
	return func(s *DirectService) { s.users = users }
}

// NewDirectService 构造直送域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param next 下一步预生成域服务
// @param comments 审批意见域服务
// @param operations 操作记录域服务
// @param messages 待办扩展域服务
// @param histories 已办扩展域服务
// @param documents 业务表单状态域服务
// @param completeTo 直送记录存储端口
// @param c 时间源，nil 时回退真实时钟
// @param options 可选配置（身份查询）
// @return 直送域服务
func NewDirectService(engine api.Engine, meta *ModelMetaService, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService, completeTo CompleteToActivityStore, c clock.Clock, options ...DirectOption) *DirectService {
	if c == nil {
		c = clock.Real{}
	}
	s := &DirectService{
		engine: engine, meta: meta, next: next, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents,
		completeTo: completeTo, clock: c,
	}
	for _, option := range options {
		option(s)
	}
	return s
}

// CompleteToTargetActivity 直送：办结当前任务并把流程提交到指定目标节点
// （对位 Java completeToTargetActivity:4550）：
// 目标节点预存（saveCompleteToTargetNextActivity 对位）→ 入参/任务/挂起校验 →
// TZ 类意见 → 引擎 WorkflowExtService().CompleteToTarget（operationType=completeToTarget
// 门控触发执行人重写）→ 落 workflow_complete_to_activity（flowId 空、source=当前节点）→
// 操作记录(isAgree=1) → 删当前节点预存 → 删当前待办 → 已办完成时间 →
// 新待办生成（目标节点新任务）→ 表单状态机刷新。
// 意见类型核实结论：Java 直送场景 type 缺省取 CommentTypeEnum.TZ（跳转），
// 显式传入 vo.Type 时用 vo.Type（L4632）。
// @param ctx 操作上下文
// @param vo 办结请求（TaskID/TargetActivityID/UserCode 必填；Request 首条为目标执行人预存）
// @return 错误
func (s *DirectService) CompleteToTargetActivity(ctx context.Context, vo *api.CompleteTaskVo) error {
	// 1. 入参校验（Java completeToTargetActivity 逐项 isBlank 校验；请求先校验再预存，
	// 与 Java"先预存后校验"的顺序差异不影响校验覆盖面）
	if vo == nil {
		return apiError(errCodeDirectVoBlank, "直送请求为空")
	}
	if strings.TrimSpace(vo.TaskID) == "" {
		return apiError(errCodeDirectTaskIDBlank, "参数 taskId 为空")
	}
	if strings.TrimSpace(vo.TargetActivityID) == "" {
		return apiError(errCodeDirectTargetBlank, "参数 targetActivityId 为空")
	}
	if strings.TrimSpace(vo.UserCode) == "" {
		return apiError(errCodeDirectUserBlank, "参数 userCode 为空")
	}
	// 2. 任务定位
	task, err := s.engine.TaskService().GetTask(ctx, vo.TaskID)
	if err != nil {
		return api.NewError(errCodeDirectTaskMissing, "任务不存在", err)
	}
	// 3. 暂停拒绝（Java isSuspended → "流程已暂停，请联系管理员激活"）
	if err := s.ensureInstanceActive(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	// 4. 目标节点执行人预存（Java saveCompleteToTargetNextActivity：仅取 request 首条）
	if err := s.saveCompleteToTargetExecutors(ctx, vo, task); err != nil {
		return err
	}
	// 5. 意见类型：vo.Type 缺省 TZ（Java L4632 权威语义）
	commentType := firstNonEmpty(vo.Type, CommentTypeTZ)
	// 意见（引擎评论 API 绑定运行态任务，须在直送命令前写入；空意见跳过——
	// Java 直送不校验 message，Go 引擎拒绝空评论）
	if strings.TrimSpace(vo.Message) != "" {
		if _, err := s.comments.Add(ctx, CommentRequest{
			TaskID:            vo.TaskID,
			ProcessInstanceID: task.ProcessInstanceID,
			UserID:            vo.UserCode,
			Message:           vo.Message,
			Type:              commentType,
		}); err != nil {
			return err
		}
	}
	// 6. 设置任务办理人（Java taskService.setAssignee 对位，避免引擎处理人不匹配校验拒绝）
	assignee := vo.UserCode
	if _, err := s.engine.TaskService().UpdateTask(ctx, vo.TaskID, api.TaskUpdate{Assignee: &assignee, UserID: vo.UserCode}); err != nil {
		return err
	}
	// 7. 引擎级直送（单条命令内办结 + 迁移；operationType=completeToTarget 由引擎内部写入，
	// 触发执行人重写钩子按预存记录回填目标节点执行人）
	if err := s.engine.WorkflowExtService().CompleteToTarget(ctx, vo.TaskID, vo.TargetActivityID, copyVariables(vo.Variables)); err != nil {
		return err
	}
	// 8. 直送轨迹落库（Java workflowCompleteToActivityService.save；
	// flowId 在 Java 为伪造连线的随机 ID，Go 引擎级迁移不产生新连线，保留空值）
	if s.completeTo != nil {
		record := &CompleteToActivity{
			BaseColumns:       BaseColumns{TenantID: task.TenantID},
			SourceActivityID:  task.TaskDefinitionKey,
			TargetActivityID:  vo.TargetActivityID,
			TaskID:            vo.TaskID,
			ProcessInstanceID: task.ProcessInstanceID,
		}
		if err := s.completeTo.Save(ctx, record); err != nil {
			return err
		}
	}
	// 9. 操作记录（isAgree=1 同意，Java addOperationRecord 第 4 参）
	agree := 1
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            vo.TaskID,
		ProcessInstanceID: task.ProcessInstanceID,
		OwnerID:           vo.UserCode,
		IsAgree:           &agree,
		Message:           vo.Message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	// 10. 删除当前节点的下一步预存记录（Java deleteCurrentWorkflowNextActivity；历史表保留追溯）
	if err := s.deleteCurrentNextActivity(ctx, task); err != nil {
		return err
	}
	// 11. 删除当前任务待办（Java deleteTaskMessageExtendByTaskId）
	if err := s.messages.DeleteByTaskID(ctx, vo.TaskID); err != nil {
		return err
	}
	// 12. 已办完成时间（Java updateByTaskIdAndOwnerId；时间为零值时服务内取当前时间）
	if err := s.histories.UpdateFinishTime(ctx, vo.TaskID, vo.UserCode, time.Time{}); err != nil {
		return err
	}
	// 13. 新增待办（Java addTaskMessageExtendByProcessInstanceId：按实例重扫引擎任务，
	// 目标节点新任务在此生成待办）
	if _, err := s.generateTodosAfterDirect(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	// 14. 表单状态机刷新（Java 直送链路未调用 updateWorkflowDocument，Go 补齐账本一致：
	// 实例已结束时置终态，否则按待办聚合刷新）
	if s.isInstanceFinished(ctx, task.ProcessInstanceID) {
		return s.documents.Finish(ctx, task.ProcessInstanceID)
	}
	return s.documents.UpdateByProcessInstanceOrTask(ctx, task.ProcessInstanceID, vo.TaskID)
}

// saveCompleteToTargetExecutors 预存目标节点的直送执行人（Java saveCompleteToTargetNextActivity 对位）。
// 仅消费 request 首条（Java nextActivityList.get(0)）；目标为保留代号 end 时跳过；
// 目标节点模型未配置执行者时拒绝（Java "【x】步骤未设置执行者"）。
// @param ctx 操作上下文
// @param vo 直送请求
// @param task 当前任务
// @return 错误
func (s *DirectService) saveCompleteToTargetExecutors(ctx context.Context, vo *api.CompleteTaskVo, task api.Task) error {
	// Java：request 缺失/列表为空 → "参数 nextActivityList 为空"
	if vo.Request == nil || len(vo.Request.NextActivityList) == 0 || vo.Request.NextActivityList[0] == nil {
		return apiError(errCodeDirectNoExecutor, "参数 nextActivityList 为空")
	}
	target := vo.Request.NextActivityList[0]
	// 目标为保留代号 end 时由调用方跳过预存（与发起/办结链路的 end 约定一致）
	if IsEndStep(target.ActivityID) {
		return nil
	}
	// 目标节点模型执行者配置校验（Java getUserTaskJsonInfo 判 assignee/candidateUsers/candidateGroups）
	meta, err := s.meta.UserTaskMeta(ctx, task.ProcessDefinitionID, target.ActivityID)
	if err != nil {
		return err
	}
	// Go 扩展执行者形态（executorList 扩展属性）与 Java 三形态一并视为"已配置"
	if meta.Assignee == "" && len(meta.CandidateUsers) == 0 && len(meta.CandidateGroups) == 0 && strings.TrimSpace(meta.ExecutorListRaw) == "" {
		return apiError(errCodeDirectNoExecutor, "【"+target.ActivityID+"】步骤未设置执行者")
	}
	// 执行人缺失：SaveAgreeNext 拒绝（Java "保存下一步骤信息异常：执行人为空"同语义）
	sendID := firstNonEmpty(vo.Request.SendIdentityID, vo.UserCode)
	return s.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
		ProcessInstanceID: task.ProcessInstanceID,
		ActivityID:        target.ActivityID,
		IdentityIDs:       target.ExecutorIDs,
		PreTaskID:         vo.TaskID,
		SendIdentityID:    sendID,
		SendUserName:      firstNonEmpty(vo.Request.SendUserName, s.displayName(ctx, sendID)),
		TenantID:          task.TenantID,
		Creator:           vo.UserCode,
	})
}

// deleteCurrentNextActivity 删除当前任务在当前节点上的预存记录（运行态）。
// 对位 Java deleteCurrentWorkflowNextActivity（instance+node+task 过滤）；complete.go
// 同名逻辑的复刻（不可改其文件，直送门面独立持有服务依赖）。
// @param ctx 操作上下文
// @param task 已直送的任务
// @return 错误
func (s *DirectService) deleteCurrentNextActivity(ctx context.Context, task api.Task) error {
	records, err := s.next.ListByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, record := range records {
		// 仅删当前节点的记录（其他节点记录属其他分支，不清理）
		if record.NextActivityID != task.TaskDefinitionKey {
			continue
		}
		if err := s.next.DeleteForResolve(ctx, task.ProcessInstanceID, task.TaskDefinitionKey, task.ID, record.IdentityID); err != nil {
			return err
		}
	}
	return nil
}

// ensureInstanceActive 校验实例未挂起（Java isSuspended）。
// 实例已结束/查询失败时交由引擎校验兜底，不在链路前置拒绝。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *DirectService) ensureInstanceActive(ctx context.Context, processInstanceID string) error {
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	// 运行表查不到（已结束）：后续引擎校验兜底
	if err != nil {
		return nil
	}
	if instance.Suspended {
		return apiError(errCodeDirectSuspended, "流程已暂停，请联系管理员激活")
	}
	return nil
}

// generateTodosAfterDirect 按实例重新生成待办与已办镜像（Java addTaskMessageExtendByProcessInstanceId
// 对位；与退回域 generateTodosAfterBack 同构，直送后目标节点新任务在此生成待办）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 新建待办的任务 Id 列表（含已有待办的任务）
// @return 错误
func (s *DirectService) generateTodosAfterDirect(ctx context.Context, processInstanceID string) ([]string, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: backHistoryPageSize})
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
		message, err := s.buildDirectMessageExtend(ctx, processInstanceID, current)
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

// buildDirectMessageExtend 组装新任务的待办扩展（实例/定义/预存发送人信息联查补全）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param task 新任务
// @return 待办扩展
// @return 错误
func (s *DirectService) buildDirectMessageExtend(ctx context.Context, processInstanceID string, task api.Task) (*MessageExtend, error) {
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
	// 发送人信息取该节点第一条未消耗预存记录（直送预存记录可追溯）
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

// displayName 查询身份展示名（身份查询缺失或查询失败时回退身份 Id）。
// @param ctx 操作上下文
// @param userID 身份 Id
// @return 展示名
func (s *DirectService) displayName(ctx context.Context, userID string) string {
	if s.users == nil || userID == "" {
		return userID
	}
	if user, err := s.users.GetUser(ctx, userID); err == nil {
		if name := userDisplayName(user); name != "" {
			return name
		}
	}
	return userID
}

// isInstanceFinished 判断实例是否已结束（运行表查不到或实例 Ended）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 是否已结束
func (s *DirectService) isInstanceFinished(ctx context.Context, processInstanceID string) bool {
	// 无法定位实例：按未结束处理，不虚报终态
	if processInstanceID == "" {
		return false
	}
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	// 运行表查不到：已结束（Java 同判据）
	if err != nil {
		return true
	}
	return instance.Ended
}
