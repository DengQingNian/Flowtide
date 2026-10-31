// reset.go 实现流程重置门面服务（P2 波2-B）。
// 对位 Java ProcessesUtil#doResetProcess:3766：
// 校验（实例运行中/存在活动任务）→ 引擎 WorkflowExtService().ResetProcess
// （全部活动执行软回首：迁至开始事件首个出线目标）→
// 收尾：LCCZ 意见 + 操作记录 → 删 next_activity 双表 + 待办 + 已办（Java 明确删除已办）→
// saveFirstTaskUser 重建首步执行人（在本包实现解析辅助，不可改 start.go）→
// 首任务待办/已办镜像重建 → document 刷新。
package workflow

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// 重置域错误码（WORKFLOW-28xx 段，对位 Java doResetProcess 的 BusinessException 文案）。
const (
	errCodeResetPiIDBlank       = "WORKFLOW-2801" // 参数 processInstanceId 为空
	errCodeResetUserBlank       = "WORKFLOW-2802" // 参数 userCode 为空
	errCodeResetInstanceMissing = "WORKFLOW-2803" // 流程不存在
	errCodeResetNoTask          = "WORKFLOW-2804" // 任务不存在
	errCodeResetNoExecutor      = "WORKFLOW-2805" // 步骤未设置执行者
	errCodeResetNoFirstTask     = "WORKFLOW-2806" // 重置后未找到首任务
)

// resetCommentMessage 是重置意见固定文案（Java doResetProcess 的意见/操作记录文案来源）。
const resetCommentMessage = "流程重置"

// ResetService 是流程重置域服务（P2 波2-B）。
// 引擎级重置（清场全部活动任务并把执行迁回首节点）已由
// api.WorkflowExtService().ResetProcess 承载，本服务负责校验、首步执行人重建与收尾账务。
type ResetService struct {
	engine     api.Engine              // 引擎门面（七服务）
	meta       *ModelMetaService       // 模型元数据服务（首任务执行者配置读取）
	next       *NextActivityService    // 下一步预生成-消耗域服务（双表清理 + 首步预生成）
	comments   *CommentService         // 审批意见服务（LCCZ 意见）
	operations *OperationRecordService // 操作记录服务（LCCZ 留痕）
	messages   *MessageExtendService   // 待办扩展服务（待办清理与重建）
	histories  *HistoryExtendService   // 已办扩展服务（已办清理与重建）
	documents  *DocumentService        // 业务表单状态服务（状态机刷新）
	clock      clock.Clock             // 时间源
}

// NewResetService 创建流程重置域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param next 下一步预生成域服务
// @param comments 审批意见服务
// @param operations 操作记录服务
// @param messages 待办扩展服务
// @param histories 已办扩展服务
// @param documents 业务表单状态服务
// @param c 时间源，nil 时回退真实时钟
// @return 重置域服务
func NewResetService(engine api.Engine, meta *ModelMetaService, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService, c clock.Clock) *ResetService {
	if c == nil {
		c = clock.Real{}
	}
	return &ResetService{
		engine: engine, meta: meta, next: next, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents, clock: c,
	}
}

// Reset 重置流程实例（对位 Java doResetProcess:3766）。
// 链路：入参校验 → 实例运行中校验 → 活动任务存在校验 → 引擎软回首 →
// 收尾（LCCZ 意见/操作记录 → next_activity 双表清理 → 待办与已办清理 →
// 首步执行人重建（used=1 预生成）→ 首任务待办/已办镜像重建 → document 刷新）。
// 与 Java 的对照差异：
//  1. Java 按任务循环逐个 BackUserTaskCmd 迁移并逐任务写意见/记录；Go 引擎
//     ResetProcess 单命令完成全部执行迁移，故意见/操作记录收敛为一条（任务列取重置前首任务）。
//  2. Java 按推演 actIds 清理 next_activity/待办/已办；Go 语义为"重置后全量预存与
//     扩展台账全部过期"，故按实例全量清理（净效果等价，覆盖并行多分支场景）。
//  3. Java 重置后不重建首任务待办（历史缺口）；Go 补建待办与已办镜像，
//     保证重置后待办中心可用（与发起链落位一致）。
//
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param operator 操作人身份 Id
// @return 错误
func (s *ResetService) Reset(ctx context.Context, processInstanceID, operator string) error {
	// 入参校验（对位 Java 逐参 isBlank 校验）
	if strings.TrimSpace(processInstanceID) == "" {
		return apiError(errCodeResetPiIDBlank, "参数 processInstanceId 为空")
	}
	if strings.TrimSpace(operator) == "" {
		return apiError(errCodeResetUserBlank, "参数 userCode 为空")
	}
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err != nil {
		return apiError(errCodeResetInstanceMissing, "流程不存在")
	}
	// 活动任务存在校验（Java taskList 为空 → "任务不存在"；已结束实例通常无任务，
	// 引擎 ResetProcess 的 Ended 校验先行拒绝）
	beforeTasks, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: instanceListPageSize})
	if err != nil {
		return err
	}
	if len(beforeTasks.Data) == 0 {
		return apiError(errCodeResetNoTask, "任务不存在")
	}
	// 引擎重置：清场全部活动任务并把全部执行迁回首节点（软回首，挂起实例由引擎拒绝）
	if err := s.engine.WorkflowExtService().ResetProcess(ctx, processInstanceID, operator); err != nil {
		return err
	}
	// 收尾链（意见/记录在引擎成功后写，保证失败不留痕）
	return s.writeResetTailChain(ctx, instance, beforeTasks.Data[0], operator)
}

// writeResetTailChain 重置收尾链：LCCZ 意见与操作记录 → 双表/待办/已办清理 →
// 首步执行人重建 → 首任务待办镜像重建 → document 刷新。
// @param ctx 操作上下文
// @param instance 流程实例（重置前快照）
// @param firstBeforeTask 重置前的首个活动任务（操作记录任务列定位）
// @param operator 操作人身份 Id
// @return 错误
func (s *ResetService) writeResetTailChain(ctx context.Context, instance api.ProcessInstance, firstBeforeTask api.Task, operator string) error {
	// LCCZ 意见（流程级；Java 按任务逐个写任务级意见，Go 单命令重置收敛为流程级一条）
	if _, err := s.comments.Add(ctx, CommentRequest{
		ProcessInstanceID: instance.ID,
		UserID:            operator,
		Message:           resetCommentMessage,
		Type:              CommentTypeLCCZ,
	}); err != nil {
		return err
	}
	// LCCZ 操作记录（Java isAgree=0 不同意方向；任务列取重置前首任务，
	// 其历史行经引擎清场保留办结态，可联查补步骤信息）
	reject := 0
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            firstBeforeTask.ID,
		ProcessInstanceID: instance.ID,
		OwnerID:           operator,
		IsAgree:           &reject,
		Message:           resetCommentMessage,
		Type:              CommentTypeLCCZ,
	}); err != nil {
		return err
	}
	// 删 next_activity 双表（重置后既有预存记录全部过期，按实例全量清理）
	if err := deleteNextActivityAll(ctx, s.next, instance.ID); err != nil {
		return err
	}
	// 删待办与已办（Java deleteTaskMessageByProcessInstanceIdAndActIds /
	// deleteTaskHistoryMessageByProcessInstanceIdAndActIds 的实例级等价；已办 Java 明确删除）
	if err := s.messages.DeleteByInstance(ctx, instance.ID); err != nil {
		return err
	}
	if err := s.histories.DeleteByInstance(ctx, instance.ID); err != nil {
		return err
	}
	// 首步执行人重建 + 首任务待办/已办镜像重建
	if err := s.rebuildFirstStep(ctx, instance, operator); err != nil {
		return err
	}
	// 表单状态机刷新（重置后操作记录数 >1，状态回"流转中"）
	return s.documents.UpdateByProcessInstanceOrTask(ctx, instance.ID, "")
}

// rebuildFirstStep 重建首步执行人预生成与首任务扩展数据（对位 Java
// saveFirstTaskUserNextActivity:1142，复用 StartService 的解析优先级，在本包实现辅助）：
// 定位重置后首任务 → 模型元数据读首任务执行者（executorList → assignee → candidateUsers →
// candidateGroups）→ next.SaveFirstTaskUser（used=1 + taskId）→ 首任务待办/已办镜像重建。
// @param ctx 操作上下文
// @param instance 流程实例
// @param operator 操作人身份 Id
// @return 错误
func (s *ResetService) rebuildFirstStep(ctx context.Context, instance api.ProcessInstance, operator string) error {
	// 定位重置后的首任务（引擎回首后落位开始事件首个出线目标的用户任务）
	task, err := s.firstTaskOf(ctx, instance.ID)
	if err != nil {
		return err
	}
	// 模型元数据读取首任务执行者配置（Java getUserTaskJsonInfo 等价）
	meta, err := s.meta.UserTaskMeta(ctx, task.ProcessDefinitionID, task.TaskDefinitionKey)
	if err != nil {
		return err
	}
	// 解析执行人（优先级链与发起链一致）
	identities, groupIDs, err := s.resolveFirstStepExecutors(ctx, meta, instance)
	if err != nil {
		return err
	}
	// 首步预生成：used=1、taskId=重置后首任务（版本号由服务内推算）
	if err := s.next.SaveFirstTaskUser(ctx, FirstStepRequest{
		ProcessInstanceID: instance.ID,
		ActivityID:        task.TaskDefinitionKey,
		IdentityIDs:       identities,
		GroupIDs:          groupIDs,
		SendIdentityID:    operator,
		SendUserName:      s.userDisplayName(ctx, operator),
		TaskID:            task.ID,
		TenantID:          instance.TenantID,
		Creator:           operator,
	}); err != nil {
		return err
	}
	// 首任务待办/已办镜像重建（Java 重置后不重建的历史缺口，Go 补齐）
	_, err = createInstanceTaskTodo(ctx, s.engine, s.messages, s.histories, instance, task, operator)
	return err
}

// firstTaskOf 定位实例当前的首个用户任务（重置后应恰有一个落位任务）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 首个任务
// @return 错误（无任务返回 errCodeResetNoFirstTask）
func (s *ResetService) firstTaskOf(ctx context.Context, processInstanceID string) (api.Task, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: 1})
	if err != nil {
		return api.Task{}, err
	}
	// 重置后必然存在落位任务；查不到说明模型回首后无用户任务（防御分支）
	if page.Total == 0 || len(page.Data) == 0 {
		return api.Task{}, apiError(errCodeResetNoFirstTask, "重置后未找到首任务，请检查流程定义")
	}
	return page.Data[0], nil
}

// resolveFirstStepExecutors 解析首步执行人（对位 StartService 同名逻辑的本地重建，
// 优先级链一致：executorList → assignee → candidateUsers → candidateGroups）。
// @param ctx 操作上下文
// @param meta 首任务模型元数据
// @param instance 流程实例（${initiator} 解析为发起人）
// @return 执行人 Id 列表
// @return 候选组 Id 列表（与执行人等长配对，无组为空）
// @return 错误（步骤未设置执行者）
func (s *ResetService) resolveFirstStepExecutors(ctx context.Context, meta *UserTaskMeta, instance api.ProcessInstance) ([]string, []string, error) {
	// 表达式解析变量：实例变量 + initiator 兜底为发起人（Java VARIABLES_KEY_INITIATOR 分支）
	variables, err := s.engine.RuntimeService().GetVariables(ctx, instance.ID)
	if err != nil {
		// 变量读取失败按空表处理，发起人兜底仍可用
		variables = map[string]any{}
	}
	// ${initiator} 依赖的变量缺失时以实例发起人兜底
	if _, ok := variables[VarInitiator]; !ok {
		if variables == nil {
			variables = map[string]any{}
		}
		variables[VarInitiator] = instance.StartUserID
	}
	// 优先级 0：executorList 模型扩展属性（org/role 组展开成员，person 直取身份）
	configs := ParseExecutorConfigs(meta.ExecutorListRaw)
	if len(configs) > 0 {
		identities, groupIDs := s.expandExecutorConfigs(ctx, configs)
		if len(identities) > 0 {
			return identities, groupIDs, nil
		}
	}
	// 优先级 1：assignee 单人表达式
	if assignee := strings.TrimSpace(meta.Assignee); assignee != "" {
		if id := resolveExecutorExpression(assignee, variables); id != "" {
			return []string{id}, nil, nil
		}
	}
	// 优先级 2：candidateUsers 多人（逐项表达式解析）
	if len(meta.CandidateUsers) > 0 {
		identities := make([]string, 0, len(meta.CandidateUsers))
		for _, raw := range meta.CandidateUsers {
			// 逐项解析：${变量} 取变量值，字面量原样作为身份 Id
			if id := resolveExecutorExpression(raw, variables); id != "" {
				identities = append(identities, id)
			}
		}
		if len(identities) > 0 {
			return identities, nil, nil
		}
	}
	// 优先级 3：candidateGroups 组 → 引擎身份库展开组内用户（组员与所属组配对）
	if len(meta.CandidateGroups) > 0 {
		identities, groupIDs := s.expandGroups(ctx, meta.CandidateGroups)
		// 与发起链一致：组内无有效成员时报"执行人身份不存在"
		if len(identities) == 0 {
			return nil, nil, apiError(errCodeStartNoIdentity, "保存第一个用户任务下一步骤信息异常：执行人身份不存在")
		}
		return identities, groupIDs, nil
	}
	// 全空：按 Java 语义报错阻断（【taskKey】步骤未设置执行者）
	return nil, nil, apiError(errCodeResetNoExecutor, "【"+meta.ActivityID+"】步骤未设置执行者")
}

// expandExecutorConfigs 展开 executorList 配置为执行人（person 取身份；org/role 按组展开成员）。
// 展开为空的组不阻断其余配置（整体空判由调用方兜底）。
// @param ctx 操作上下文
// @param configs 执行者配置列表
// @return 执行人 Id 列表
// @return 候选组 Id 列表（与执行人等长配对）
func (s *ResetService) expandExecutorConfigs(ctx context.Context, configs []ExecutorConfig) ([]string, []string) {
	identities := make([]string, 0, len(configs))
	groupIDs := make([]string, 0, len(configs))
	for _, config := range configs {
		switch config.ExecutorType {
		// org/role 视为组维度：经引擎身份库展开组内用户
		case "org", "role":
			groupID := firstNonEmpty(config.OrgID, config.RoleID)
			// 组 Id 缺失的配置项跳过
			if groupID == "" {
				continue
			}
			members, err := s.engine.IdentityService().ListMembers(ctx, groupID)
			// 组查不到时不阻断其余配置（展开为空的组由整体空判兜底）
			if err != nil {
				continue
			}
			for _, member := range members {
				identities = append(identities, member.ID)
				groupIDs = append(groupIDs, groupID)
			}
		default:
			// person（含缺省）：直接取身份 Id
			if config.IdentityID != "" {
				identities = append(identities, config.IdentityID)
				groupIDs = append(groupIDs, "")
			}
		}
	}
	return identities, groupIDs
}

// expandGroups 展开候选组为组内用户列表（组员与所属组等长配对）。
// @param ctx 操作上下文
// @param groupIDs 候选组 Id 列表
// @return 执行人 Id 列表
// @return 与执行人等长的候选组 Id 列表
func (s *ResetService) expandGroups(ctx context.Context, groupIDs []string) ([]string, []string) {
	identities := make([]string, 0, len(groupIDs))
	paired := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		// 引擎身份库按组查成员（组查不到时跳过该组）
		members, err := s.engine.IdentityService().ListMembers(ctx, groupID)
		if err != nil {
			continue
		}
		for _, member := range members {
			identities = append(identities, member.ID)
			paired = append(paired, groupID)
		}
	}
	return identities, paired
}

// userDisplayName 查询用户展示名（查不到给空，不阻断主流程）。
// @param ctx 操作上下文
// @param userID 用户 Id
// @return 展示名
func (s *ResetService) userDisplayName(ctx context.Context, userID string) string {
	// 身份查不到时返回空串（发送人姓名列为可空展示字段）
	if user, err := s.engine.IdentityService().GetUser(ctx, userID); err == nil {
		return userDisplayName(user)
	}
	return ""
}
