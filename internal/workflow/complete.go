package workflow

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// completeListSize 是办结链路查询引擎任务列表的批量上限（单实例任务数量级远低于该值）。
const completeListSize = 500

// varScopeType 是前后加签作用域变量名（父任务局部变量，P3 加签服务写入）。
const varScopeType = "scopeType"

// CompleteResult 是办结结果：新落位的待办任务 ID 与流程是否已结束。
type CompleteResult struct {
	NextTaskIDs []string // 办结后生成的待办任务 Id（委派分支为归还后的待办任务）
	Finished    bool     // 流程是否已结束（运行表查不到或实例已结束）
}

// DelegateTaskRequest 是任务委派请求（对位 Java DelegateTaskVo：taskId/owner/delegateId/message）。
type DelegateTaskRequest struct {
	TaskID     string // 任务 Id（必填）
	Owner      string // 任务拥有者身份 Id（必填，委派后保留所有者）
	DelegateID string // 被委托人身份 Id（必填，受托人）
	Message    string // 委派意见（必填，域服务意见留痕约束）
}

// TransferTaskRequest 是任务转办请求（对位 Java TransferTaskVo：taskId/owner/receiver/message）。
type TransferTaskRequest struct {
	TaskID   string // 任务 Id（必填）
	Owner    string // 原处理人身份 Id（必填，写入任务所有者）
	Receiver string // 接收人身份 Id（必填，新处理人）
	Message  string // 转办意见（必填）
}

// ResolveTaskRequest 是归还（处理委派）请求（对位 Java ProcessesUtil#resolveTask:1904）。
type ResolveTaskRequest struct {
	TaskID   string // 任务 Id（必填）
	UserCode string // 操作人身份 Id（空时按任务所有者留痕）
	Message  string // 归还意见（必填）
	Type     string // 意见类型代码，空时默认 SP
}

// CompleteService 是任务办结域服务（P1-E）。
// 行为规格对位 Java ProcessesUtil#complete:3897 / generalComplete:3979 / completeToTargetActivity:4550 /
// delegateTask:1793 / transferTask:1848 / resolveTask:1904：
// 校验链 → 下一步预存 → 权重会签累计 → 引擎办结（自由选路经引擎级迁移兜底）→
// 委派 PENDING 分支（克隆子任务+归还+还原下一步）→ 加签父任务联动 → 收尾链（意见/操作记录/待办/已办/表单状态）。
type CompleteService struct {
	engine     api.Engine              // 引擎门面（七服务）
	meta       *ModelMetaService       // 模型元数据（权重会签配置读取）
	next       *NextActivityService    // 下一步预生成-消耗记账
	comments   *CommentService         // 审批意见
	operations *OperationRecordService // 操作记录
	messages   *MessageExtendService   // 待办扩展
	histories  *HistoryExtendService   // 已办扩展
	documents  *DocumentService        // 业务表单状态机
	completeTo CompleteToActivityStore // 直送记录（自由选路兜底轨迹）
	users      UserQuery               // 身份查询（补姓名展示）
	clock      clock.Clock             // 时间源
}

// NewCompleteService 创建任务办结域服务。
// @param engine 引擎门面（七服务）
// @param meta 模型元数据服务
// @param next 下一步预生成域服务
// @param comments 审批意见域服务
// @param operations 操作记录域服务
// @param messages 待办扩展域服务
// @param histories 已办扩展域服务
// @param documents 业务表单状态域服务
// @param completeTo 直送记录存储端口（自由选路兜底轨迹）
// @param users 身份查询（engine.IdentityService()），nil 时姓名展示留空
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewCompleteService(engine api.Engine, meta *ModelMetaService, next *NextActivityService, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, documents *DocumentService, completeTo CompleteToActivityStore, users UserQuery, c clock.Clock) *CompleteService {
	if c == nil {
		c = clock.Real{}
	}
	return &CompleteService{
		engine: engine, meta: meta, next: next, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents,
		completeTo: completeTo, users: users, clock: c,
	}
}

// Complete 办结任务（对位 Java generalComplete 主链路）。
// 校验链（任务存在/操作人与意见非空/实例未挂起）→ 预存下一步（end 与委派任务跳过）→
// 权重会签累计 → 引擎办结（operationType 注入触发执行人重写门控）→ 自由选路兜底迁移 →
// 委派 PENDING 分支 → 加签父任务联动 → 收尾链 → 返回新待办与结束标记。
// @param ctx 操作上下文
// @param vo 办结请求
// @return 办结结果（新待办任务 Id 与流程是否结束）
// @return 错误
func (s *CompleteService) Complete(ctx context.Context, vo *api.CompleteTaskVo) (CompleteResult, error) {
	// 1. 入参校验链（对位 Java generalComplete 的逐项 isBlank 校验）
	if err := s.validateCompleteVo(vo); err != nil {
		return CompleteResult{}, err
	}
	// 2. 任务存在性
	task, err := s.engine.TaskService().GetTask(ctx, vo.TaskID)
	if err != nil {
		return CompleteResult{}, api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	// 加签克隆子任务无流程实例归属：从父任务补齐（意见/操作记录/状态机依赖实例定位）
	vo.ProcessInstanceID = task.ProcessInstanceID
	if vo.ProcessInstanceID == "" && task.ParentTaskID != "" {
		if parent, err := s.engine.TaskService().GetTask(ctx, task.ParentTaskID); err == nil {
			vo.ProcessInstanceID = parent.ProcessInstanceID
		}
	}
	// 3. 实例挂起校验（Java isSuspended → "流程已暂停，请联系管理员激活"）
	if err := s.ensureInstanceActive(ctx, vo.ProcessInstanceID); err != nil {
		return CompleteResult{}, err
	}
	// 4. 预存下一步（Java saveAgreeNextActivity：PENDING 委派任务与 end 目标跳过）
	nextTarget := firstNextActivityID(vo)
	if task.DelegationState != "PENDING" && nextTarget != "" && !IsEndStep(nextTarget) {
		if err := s.saveAgreeNextForComplete(ctx, vo, task); err != nil {
			return CompleteResult{}, err
		}
	}
	// 5. 审批意见（Java 顺序在办结后；引擎评论 API 绑定运行态任务，故前置到办结命令前写入；
	// 无法定位实例的孤儿任务跳过意见留痕，不阻断办结）
	commentType := firstNonEmpty(vo.Type, CommentTypeSP)
	if vo.ProcessInstanceID != "" {
		if _, err := s.comments.Add(ctx, CommentRequest{
			TaskID:            vo.TaskID,
			ProcessInstanceID: vo.ProcessInstanceID,
			UserID:            vo.UserCode,
			Message:           vo.Message,
			Type:              commentType,
		}); err != nil {
			return CompleteResult{}, err
		}
	}
	// 6. 分支处理
	ledgerTaskID := vo.TaskID
	delegated := false
	if task.DelegationState == "PENDING" {
		// 委派分支：克隆子任务+完成子任务+归还+还原下一步（Java 2.委派处理）
		subTaskID, err := s.completeDelegationTask(ctx, vo, task)
		if err != nil {
			return CompleteResult{}, err
		}
		// Java 委派分支的待办/已办清理挂在子任务上（原任务待办保留给 owner）
		ledgerTaskID = subTaskID
		delegated = true
	} else {
		// 常规分支：权重会签累计 → 改执行人 → 引擎办结
		variables := copyVariables(vo.Variables)
		if err := s.accumulateCountersignWeight(ctx, vo, task, variables); err != nil {
			return CompleteResult{}, err
		}
		// Java 3.1 修改执行人（相当于签收，避免引擎处理人不匹配校验拒绝）
		assignee := vo.UserCode
		if _, err := s.engine.TaskService().UpdateTask(ctx, vo.TaskID, api.TaskUpdate{Assignee: &assignee, UserID: vo.UserCode}); err != nil {
			return CompleteResult{}, err
		}
		// operationType 注入：执行人重写钩子门控（目标非空=直送语义）
		if vo.TargetActivityID != "" {
			variables[VarOperationType] = OperationTypeCompleteToTarget
		} else {
			variables[VarOperationType] = OperationTypeAgreeComplete
		}
		// Java 3.2 执行任务（变量并入 complete 请求，含 totalWeight 与 operationType）
		if err := s.engine.TaskService().Complete(ctx, vo.TaskID, api.CompleteTaskRequest{UserID: vo.UserCode, Variables: mapToVariables(variables)}); err != nil {
			return CompleteResult{}, err
		}
		// Java 4. 处理加签父任务（子任务全部完成后归还/办结父任务）
		if err := s.handleParentTaskAfterComplete(ctx, task); err != nil {
			return CompleteResult{}, err
		}
	}
	// 6. 自由选路兜底：实际推进节点与指定目标不一致时经引擎迁移到目标（Java 改内存出线的等价兜底，P2.4 引擎级化）
	migrated := false
	if vo.TargetActivityID != "" && vo.TargetActivityID != StepCodeEnd && task.DelegationState != "PENDING" {
		var err error
		migrated, err = s.applyFreeSelectFallback(ctx, vo)
		if err != nil {
			return CompleteResult{}, err
		}
		// 直送轨迹落库（Java workflowCompleteToActivityService.save）
		if migrated && s.completeTo != nil {
			record := &CompleteToActivity{
				BaseColumns:       BaseColumns{TenantID: task.TenantID},
				SourceActivityID:  task.TaskDefinitionKey,
				TargetActivityID:  vo.TargetActivityID,
				TaskID:            vo.TaskID,
				ProcessInstanceID: vo.ProcessInstanceID,
			}
			if err := s.completeTo.Save(ctx, record); err != nil {
				return CompleteResult{}, err
			}
		}
	}
	// 7. 收尾链（Java 5：操作记录 → 删待办 → 已办完成时间 → 新待办 → 表单状态机）
	nextTaskIDs, err := s.writeCompleteTailChain(ctx, vo, task, ledgerTaskID, migrated, delegated)
	if err != nil {
		return CompleteResult{}, err
	}
	return CompleteResult{
		NextTaskIDs: nextTaskIDs,
		Finished:    s.isInstanceFinished(ctx, vo.ProcessInstanceID),
	}, nil
}

// validateCompleteVo 校验办结入参（对位 Java generalComplete 逐项 isBlank 校验：
// taskId/userCode/message 必填；message 为空 Java 直接拒绝，Go 同语义）。
// @param vo 办结请求
// @return 错误
func (s *CompleteService) validateCompleteVo(vo *api.CompleteTaskVo) error {
	// 请求体缺失
	if vo == nil {
		return apiError("WORKFLOW-2001", "办结请求为空")
	}
	if strings.TrimSpace(vo.TaskID) == "" {
		return apiError("WORKFLOW-2002", "参数 taskId 为空")
	}
	if strings.TrimSpace(vo.UserCode) == "" {
		return apiError("WORKFLOW-2003", "参数 userCode 为空")
	}
	// Java StringUtils.isBlank(message) → "参数 message 为空"
	if strings.TrimSpace(vo.Message) == "" {
		return apiError("WORKFLOW-2005", "参数 message 为空")
	}
	return nil
}

// ensureInstanceActive 校验实例未挂起（Java baseProcessService.isSuspended）。
// 实例已结束/查询失败时交由引擎任务校验兜底，不在链路前置拒绝。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 错误
func (s *CompleteService) ensureInstanceActive(ctx context.Context, processInstanceID string) error {
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	// 运行表查不到（已结束）：后续引擎校验兜底
	if err != nil {
		return nil
	}
	// 挂起实例拒绝办结
	if instance.Suspended {
		return apiError("WORKFLOW-2006", "流程已暂停，请联系管理员激活")
	}
	return nil
}

// firstNextActivityID 返回预存请求的首个目标节点 ID（request 缺省视为 end 方向，协议允许省略）。
// @param vo 办结请求
// @return 首个目标节点 ID，无预存信息返回空串
func firstNextActivityID(vo *api.CompleteTaskVo) string {
	// request 缺省或列表为空按 end 方向处理（end 目标可省略 request）
	if vo.Request == nil || len(vo.Request.NextActivityList) == 0 || vo.Request.NextActivityList[0] == nil {
		return ""
	}
	return vo.Request.NextActivityList[0].ActivityID
}

// saveAgreeNextForComplete 预存"同意"方向下一步（对位 Java saveAgreeNextActivity:4898：
// 逐条保存 nextActivityList，end 目标跳过）。
// @param ctx 操作上下文
// @param vo 办结请求
// @param task 当前任务
// @return 错误
func (s *CompleteService) saveAgreeNextForComplete(ctx context.Context, vo *api.CompleteTaskVo, task api.Task) error {
	// 发送人缺省取办结操作人（Java request.sendIdentityId 必填，协议上同义）
	sendID := firstNonEmpty(vo.Request.SendIdentityID, vo.UserCode)
	sendName := firstNonEmpty(vo.Request.SendUserName, s.userDisplayName(ctx, sendID))
	for _, item := range vo.Request.NextActivityList {
		// 空条目与 end 保留代号不落库（Java 保存前置校验 + generalComplete end 跳过）
		if item == nil || item.ActivityID == "" || IsEndStep(item.ActivityID) {
			continue
		}
		if err := s.next.SaveAgreeNext(ctx, NextActivitySaveRequest{
			ProcessInstanceID: vo.ProcessInstanceID,
			ActivityID:        item.ActivityID,
			IdentityIDs:       item.ExecutorIDs,
			PreTaskID:         vo.TaskID,
			SendIdentityID:    sendID,
			SendUserName:      sendName,
			TenantID:          task.TenantID,
			Creator:           vo.UserCode,
		}); err != nil {
			return err
		}
	}
	return nil
}

// accumulateCountersignWeight 权重会签累计（对位 Java countersignByWeightParameterSet:4247）：
// 节点 weightType=custom 时按会签结论 0/1 取模型 weightList 中该办理人的
// rejectWeight/agreeWeight 累计流程变量 totalWeight，并入 complete 请求变量。
// @param ctx 操作上下文
// @param vo 办结请求（CountersignResult：0 否决 / 1 通过）
// @param task 当前任务
// @param variables 待并入 complete 请求的变量集合（原位写入 totalWeight）
// @return 错误
func (s *CompleteService) accumulateCountersignWeight(ctx context.Context, vo *api.CompleteTaskVo, task api.Task, variables map[string]any) error {
	// 非权重会签节点直接跳过（weightType=custom 才触发累计）
	meta, err := s.meta.UserTaskMeta(ctx, task.ProcessDefinitionID, task.TaskDefinitionKey)
	if err != nil || meta == nil || meta.WeightType != "custom" {
		// 模型读取失败按非权重节点处理，避免阻断常规办结
		return nil
	}
	// 会签结论合法性：0 否决 / 1 通过。
	// 016 号审计修复：Java CompleteTaskVo 该字段为 Integer，null 时抛"未知会签任务执行结果"；
	// Grid 裸 int 无法区分"未传"与"显式 0"，此前缺省会静默记为否决并扣 rejectWeight。
	// 现以 CountersignResultSet 标记区分：未显式设置一律报错；显式值仍须为 0/1。
	if !vo.CountersignResultSet {
		return apiError("WORKFLOW-2010", "未知会签任务执行结果：缺少 countersignResult")
	}
	if vo.CountersignResult != 0 && vo.CountersignResult != 1 {
		return apiError("WORKFLOW-2010", "未知会签任务执行结果")
	}
	// 按身份定位个人权重（Java getCountersignWeight 遍历 weightList 匹配 id；
	// 指针区分"未配置"与零值，对位 Java map.get(...) == null 语义）
	agreeWeight, rejectWeight := findPersonalWeight(parseWeightConfigs(meta.WeightListRaw), vo.UserCode)
	// Java：个人权重缺失 → "个人会签权重未设置"
	if agreeWeight == nil && rejectWeight == nil {
		return apiError("WORKFLOW-2011", "个人会签权重未设置")
	}
	// 现值累计：变量缺失按 0 起算（Java Double.valueOf(getVariable) 的等价容错）
	total := weightToFloatOf(s.getVariableQuietly(ctx, vo.ProcessInstanceID, VarTotalWeight))
	if vo.CountersignResult == 0 {
		// 否决：累计减去拒绝权重
		if rejectWeight == nil {
			return apiError("WORKFLOW-2012", "会签拒绝权重未设置")
		}
		total -= *rejectWeight
	} else {
		// 通过：累计加上通过权重
		if agreeWeight == nil {
			return apiError("WORKFLOW-2013", "会签通过权重未设置")
		}
		total += *agreeWeight
	}
	variables[VarTotalWeight] = total
	return nil
}

// weightConfig 是权重会签的个人权重配置（指针区分"未配置"与零值，对位 Java JSONObject.get 可空语义）。
type weightConfig struct {
	IdentityID   string   // 身份 Id
	AgreeWeight  *float64 // 通过权重，未配置为 nil
	RejectWeight *float64 // 拒绝权重，未配置为 nil
}

// parseWeightConfigs 解析 weightList 原始串为指针化权重配置（JSON 数组形态）。
// @param raw 原始串
// @return 权重配置列表，空串或解析失败返回 nil
func parseWeightConfigs(raw string) []weightConfig {
	trimmed := strings.TrimSpace(raw)
	// 仅支持 JSON 数组形态（与设计器写入格式一致），其他形态视为未配置
	if trimmed == "" || !strings.HasPrefix(trimmed, "[") {
		return nil
	}
	var rawConfigs []struct {
		// ID 是 Java 设计器写入的权重条目身份键（ProcessesUtil.getCountersignWeight:4305 读 "id"）
		ID string `json:"id"`
		// IdentityID 是 Go 侧历史格式键，保留兼容
		IdentityID   string   `json:"identityId"`
		AgreeWeight  *float64 `json:"agreeWeight"`
		RejectWeight *float64 `json:"rejectWeight"`
	}
	if err := json.Unmarshal([]byte(trimmed), &rawConfigs); err != nil {
		return nil
	}
	configs := make([]weightConfig, 0, len(rawConfigs))
	for _, item := range rawConfigs {
		// 016 号审计修复：Java 读 "id"，Go 此前只读 "identityId"，按 Java 格式配置的
		// 权重会永远匹配失败并抛 WORKFLOW-2011。两键并存时以 id 为准（Java 权威）。
		identityID := item.ID
		if identityID == "" {
			identityID = item.IdentityID
		}
		configs = append(configs, weightConfig{IdentityID: identityID, AgreeWeight: item.AgreeWeight, RejectWeight: item.RejectWeight})
	}
	return configs
}

// findPersonalWeight 从权重配置列表中定位指定身份的通过/拒绝权重。
// @param configs 权重配置列表
// @param identityID 身份 Id
// @return 通过权重指针，未配置为 nil
// @return 拒绝权重指针，未配置为 nil
func findPersonalWeight(configs []weightConfig, identityID string) (*float64, *float64) {
	for i := range configs {
		// Java 按身份匹配后取该条配置的 agreeWeight/rejectWeight
		if configs[i].IdentityID == identityID {
			return configs[i].AgreeWeight, configs[i].RejectWeight
		}
	}
	return nil, nil
}

// getVariableQuietly 读取流程变量，读取失败（变量不存在等）返回 nil。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param name 变量名
// @return 变量值
func (s *CompleteService) getVariableQuietly(ctx context.Context, processInstanceID, name string) any {
	value, err := s.engine.RuntimeService().GetVariable(ctx, processInstanceID, name)
	// 变量不存在/实例已结束时按缺失处理
	if err != nil {
		return nil
	}
	return value
}

// weightToFloatOf 任意数值表示转 float64（引擎变量可能是多种数值类型）。
// @param value 变量值
// @return 浮点值，无法解析返回 0
func weightToFloatOf(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		return parseWeightString(typed)
	default:
		return 0
	}
}

// parseWeightString 解析字符串数值（引擎变量历史值可能以字符串落库）。
// @param raw 原始串
// @return 浮点值，解析失败返回 0
func parseWeightString(raw string) float64 {
	// 空串与非法串统一回退 0
	parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return parsed
}

// mapToVariables 变量映射转引擎变量列表。
// @param vars 变量映射
// @return 引擎变量列表
func mapToVariables(vars map[string]any) []api.Variable {
	out := make([]api.Variable, 0, len(vars))
	for name, value := range vars {
		out = append(out, api.Variable{Name: name, Value: value})
	}
	return out
}

// copyVariables 复制办结请求变量映射（避免污染调用方入参）。
// @param src 原映射
// @return 副本映射
func copyVariables(src map[string]any) map[string]any {
	out := make(map[string]any, len(src)+2)
	for name, value := range src {
		out[name] = value
	}
	return out
}

// completeDelegationTask 委派 PENDING 分支（对位 Java generalComplete 2.委派处理）：
// 克隆子任务 → 完成子任务（引擎对无执行任务仅记历史并移除）→ 归还（owner 还原 + RESOLVED）→
// SaveResolveNext 把下一步还给 owner → 删除委派完成节点既有预存记录。
// @param ctx 操作上下文
// @param vo 办结请求
// @param task 当前任务（PENDING 委派状态）
// @return 子任务 Id（意见/操作记录挂靠对象）
// @return 错误
func (s *CompleteService) completeDelegationTask(ctx context.Context, vo *api.CompleteTaskVo, task api.Task) (string, error) {
	// 1. 克隆子任务（Java createSubTask:4351：assignee=操作人，parentTaskId=当前任务）
	sub, err := s.engine.TaskService().CreateTask(ctx, api.CreateTaskRequest{
		Name:         task.Name,
		Description:  task.Description,
		Assignee:     vo.UserCode,
		Priority:     task.Priority,
		Category:     task.Category,
		FormKey:      task.FormKey,
		ParentTaskID: task.ID,
		TenantID:     task.TenantID,
	})
	if err != nil {
		return "", err
	}
	// 2. 完成子任务（独立任务不驱动流程，仅产生被委派人的办理历史）
	if err := s.engine.TaskService().Complete(ctx, sub.ID, api.CompleteTaskRequest{UserID: vo.UserCode}); err != nil {
		return "", err
	}
	// 3. 归还委派（引擎 Resolve：assignee=owner + DelegationState=RESOLVED；
	// Java resolveTask(taskId, variables) 无操作人校验，引擎非空操作人必须为所有者，故传空跳过校验）
	if err := s.engine.TaskService().Resolve(ctx, task.ID, ""); err != nil {
		return "", err
	}
	// 4. 还原下一步给 owner（Java saveResolveNextActivity：used=1、taskId=当前任务、组继承既有记录）
	if err := s.next.SaveResolveNext(ctx, ResolveNextSaveRequest{
		ProcessInstanceID: vo.ProcessInstanceID,
		ActivityID:        task.TaskDefinitionKey,
		IdentityIDs:       []string{task.Owner},
		TaskID:            task.ID,
		SendIdentityID:    vo.UserCode,
		SendUserName:      s.userDisplayName(ctx, vo.UserCode),
		TenantID:          task.TenantID,
		Creator:           vo.UserCode,
	}); err != nil {
		return "", err
	}
	// 5. 删除委派完成节点的既有预存记录（Java deleteResolveWorkflowNextActivity 四条件）
	if err := s.next.DeleteForResolve(ctx, vo.ProcessInstanceID, task.TaskDefinitionKey, task.ID, vo.UserCode); err != nil {
		return "", err
	}
	return sub.ID, nil
}

// handleParentTaskAfterComplete 加签父任务联动（对位 Java generalComplete 4.处理加签父任务）：
// 父任务存在且子任务全部完成时归还父任务（resolve）；scopeType=after（后加签）时随之办结父任务。
// @param ctx 操作上下文
// @param task 已完成的子任务
// @return 错误
func (s *CompleteService) handleParentTaskAfterComplete(ctx context.Context, task api.Task) error {
	// 无父任务（非加签场景）直接跳过
	if task.ParentTaskID == "" {
		return nil
	}
	subs, err := s.engine.TaskService().ListSubTasks(ctx, task.ParentTaskID)
	// 父任务已不存在（查询失败）：跳过联动，不阻断办结
	if err != nil {
		return nil
	}
	// 仍有未完成子任务时父任务保持等待（Java subTaskCount==0 才处理）
	if len(subs) > 0 {
		return nil
	}
	parent, err := s.engine.TaskService().GetTask(ctx, task.ParentTaskID)
	// 父任务已不存在（流程已推进）：跳过联动
	if err != nil {
		return nil
	}
	// 归还父任务（Java resolveTask(parentTaskId)）
	if err := s.engine.TaskService().Resolve(ctx, parent.ID, ""); err != nil {
		return err
	}
	// 后加签：父任务随之办结（Java AFTER_ADDSIGN 分支）
	if s.isAfterAddSignScope(parent) {
		return s.engine.TaskService().Complete(ctx, parent.ID, api.CompleteTaskRequest{UserID: parent.Owner})
	}
	return nil
}

// isAfterAddSignScope 判断父任务是否后加签作用域（scopeType 变量 = after）。
// @param parent 父任务
// @return 是否后加签
func (s *CompleteService) isAfterAddSignScope(parent api.Task) bool {
	// 父任务可见变量中的 scopeType（P3 加签服务写入父任务局部变量）
	scope, ok := parent.Variables[varScopeType]
	if !ok {
		return false
	}
	scopeText, ok := scope.(string)
	return ok && scopeText == AddSignScopeAfter
}

// applyFreeSelectFallback 自由选路兜底（Java 改内存出线的等价实现，P2.4 引擎级化）：
// 办结后比对实际推进的任务节点与指定目标，不一致的分支经引擎 ChangeActivityState
// 迁移到目标节点（迁移会清掉误入分支任务）。
// @param ctx 操作上下文
// @param vo 办结请求（TargetActivityID 已知非空且非 end）
// @return 是否发生迁移
// @return 错误
func (s *CompleteService) applyFreeSelectFallback(ctx context.Context, vo *api.CompleteTaskVo) (bool, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: vo.ProcessInstanceID, Size: completeListSize})
	if err != nil {
		return false, err
	}
	migrated := false
	for _, current := range page.Data {
		// 已落位目标节点的分支无需迁移
		if current.TaskDefinitionKey == vo.TargetActivityID {
			continue
		}
		if err := s.engine.RuntimeService().ChangeActivityState(ctx, current.ExecutionID, []string{vo.TargetActivityID}); err != nil {
			// 并行多分支时前一迁移可能已取消本分支：任务已不存在视为已处理
			if _, getErr := s.engine.TaskService().GetTask(ctx, current.ID); getErr != nil {
				migrated = true
				continue
			}
			return migrated, err
		}
		migrated = true
	}
	return migrated, nil
}

// writeCompleteTailChain 办结收尾链（对位 Java generalComplete 5）：
// 操作记录(isAgree=1) → 删当前节点预存记录 → 删待办 → 已办完成时间 → 新待办 → 表单状态机。
// 意见已在办结命令前写入（引擎评论 API 绑定运行态任务）。
// @param ctx 操作上下文
// @param vo 办结请求
// @param task 已办结的任务
// @param ledgerTaskID 待办/已办清理挂靠任务（委派分支为克隆子任务，其余为办结任务本身）
// @param migrated 是否发生自由选路迁移（迁移后强制生成新待办）
// @param delegated 是否委派分支（委派分支的预存清理已在分支内按身份完成，不再删当前节点记录）
// @return 新待办任务 Id 列表
// @return 错误
func (s *CompleteService) writeCompleteTailChain(ctx context.Context, vo *api.CompleteTaskVo, task api.Task, ledgerTaskID string, migrated, delegated bool) ([]string, error) {
	commentType := firstNonEmpty(vo.Type, CommentTypeSP)
	// 操作记录（isAgree=1 同意，Java addOperationRecord 第 4 参；经历史任务补全，兼容已完成任务；
	// 无法定位实例的孤儿任务跳过留痕）
	if vo.ProcessInstanceID != "" {
		agree := 1
		if _, err := s.operations.Save(ctx, OperationRequest{
			TaskID:            vo.TaskID,
			ProcessInstanceID: vo.ProcessInstanceID,
			OwnerID:           vo.UserCode,
			IsAgree:           &agree,
			Message:           vo.Message,
			Type:              commentType,
		}); err != nil {
			return nil, err
		}
	}
	// 删除当前节点的下一步预存记录（Java deleteCurrentWorkflowNextActivity：instance+node+task，历史表保留追溯；
	// 委派分支按 deleteResolveWorkflowNextActivity 身份维度清理，跳过避免误删刚还原给 owner 的记录）
	if !delegated {
		if err := s.deleteCurrentNextActivity(ctx, vo.ProcessInstanceID, task.TaskDefinitionKey, vo.TaskID); err != nil {
			return nil, err
		}
	}
	// 删除待办（委派分支删的是子任务待办，通常不存在；原任务待办保留给 owner）
	if err := s.messages.DeleteByTaskID(ctx, ledgerTaskID); err != nil {
		return nil, err
	}
	// 已办完成时间（Java updateByTaskIdAndOwnerId；时间为零值时服务内取当前时间）
	if err := s.histories.UpdateFinishTime(ctx, ledgerTaskID, vo.UserCode, time.Time{}); err != nil {
		return nil, err
	}
	// 非结束方向生成新待办（Java addTaskMessageExtendByProcessInstanceId 的 end 门控；
	// 迁移兜底后即使预存目标为 end 也需为落位任务补待办；无实例孤儿任务跳过）
	nextTarget := firstNextActivityID(vo)
	shouldGenerateTodos := vo.ProcessInstanceID != "" && (migrated || (nextTarget != "" && !IsEndStep(nextTarget)))
	var nextTaskIDs []string
	// 新待办生成后再刷新表单状态机（聚合列需包含新待办，Java 同序）
	if shouldGenerateTodos {
		ids, err := s.generateTodosForNewTasks(ctx, vo)
		if err != nil {
			return nil, err
		}
		nextTaskIDs = ids
	}
	// 表单状态机刷新（Java 每次办结后 updateWorkflowDocumentByProcessInstanceIdOrTaskId）；
	// 实例已结束时直接置终态：Go 引擎运行表保留已结束实例（Ended 标记），
	// DocumentService 状态机"运行表查不到"判据不触发，此处等价补位
	if vo.ProcessInstanceID != "" {
		if s.isInstanceFinished(ctx, vo.ProcessInstanceID) {
			return nextTaskIDs, s.documents.Finish(ctx, vo.ProcessInstanceID)
		}
		return nextTaskIDs, s.documents.UpdateByProcessInstanceOrTask(ctx, vo.ProcessInstanceID, vo.TaskID)
	}
	return nextTaskIDs, nil
}

// deleteCurrentNextActivity 删除当前任务在指定节点上的预存记录（运行态）。
// 组合 ListByTaskID + DeleteForResolve 达成 Java deleteCurrentWorkflowNextActivity
// 的（实例+节点+任务）过滤；历史表保留追溯。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @param activityID 节点 ID（办结任务的 taskDefinitionKey）
// @param taskID 办结任务 Id
// @return 错误
func (s *CompleteService) deleteCurrentNextActivity(ctx context.Context, processInstanceID, activityID, taskID string) error {
	records, err := s.next.ListByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	for _, record := range records {
		// 仅删当前节点的记录（其他节点记录属其他分支，不清理）
		if record.NextActivityID != activityID {
			continue
		}
		if err := s.next.DeleteForResolve(ctx, processInstanceID, activityID, taskID, record.IdentityID); err != nil {
			return err
		}
	}
	return nil
}

// generateTodosForNewTasks 为办结后的引擎任务生成待办与已办镜像（对位 Java
// addTaskMessageExtendByProcessInstanceId → saveMessageExtendInfo：count==0 才插入，
// 已办镜像同步生成、办结时间由 UpdateFinishTime 回写）。
// @param ctx 操作上下文
// @param vo 办结请求
// @return 新建待办的任务 Id 列表（含已有待办的任务）
// @return 错误
func (s *CompleteService) generateTodosForNewTasks(ctx context.Context, vo *api.CompleteTaskVo) ([]string, error) {
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: vo.ProcessInstanceID, Size: completeListSize})
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
		message, err := s.buildMessageExtend(ctx, vo, current)
		if err != nil {
			return nil, err
		}
		if _, err := s.messages.CreateForTask(ctx, message); err != nil {
			return nil, err
		}
		// 已办镜像（Java saveMessageExtendInfo 同步插入历史行）
		history := &HistoryExtend{MessageExtend: *message}
		if _, err := s.histories.CreateForTask(ctx, history); err != nil {
			return nil, err
		}
		ids = append(ids, current.ID)
	}
	return ids, nil
}

// buildMessageExtend 组装新任务的待办扩展（实例/定义/身份/预存发送人信息联查补全）。
// @param ctx 操作上下文
// @param vo 办结请求
// @param task 新任务
// @return 待办扩展
// @return 错误
func (s *CompleteService) buildMessageExtend(ctx context.Context, vo *api.CompleteTaskVo, task api.Task) (*MessageExtend, error) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, vo.ProcessInstanceID)
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
		ProcessInstanceID: vo.ProcessInstanceID,
		Title:             instance.Name,
		BusinessID:        instance.BusinessKey,
		TaskStepName:      task.Name,
		TaskStepCode:      task.TaskDefinitionKey,
		FlowCategoryCode:  definition.Key,
		FlowCategoryName:  definition.Name,
		OwnerID:           task.Assignee,
		OwnerName:         s.userDisplayName(ctx, task.Assignee),
		TaskReceiptTime:   &task.CreateTime,
	}
	// 发送人信息取该节点第一条未消耗预存记录（执行人重写钩子未接线时记录仍可追溯）
	if record, found, err := s.next.GetFirstUnusedByActivity(ctx, vo.ProcessInstanceID, task.TaskDefinitionKey); err == nil && found {
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
	message.DocumentOwnerName = s.userDisplayName(ctx, instance.StartUserID)
	// 处理人组信息（引擎组无层级路径，多组逗号聚合）
	if s.users != nil {
		if groups, err := s.users.ListUserGroups(ctx, task.Assignee); err == nil && len(groups) > 0 {
			names := make([]string, 0, len(groups))
			groupIDs := make([]string, 0, len(groups))
			for _, group := range groups {
				names = append(names, group.Name)
				groupIDs = append(groupIDs, group.ID)
			}
			message.OwnerGroup = joinNonEmpty(names)
			message.OwnerGroupID = joinNonEmpty(groupIDs)
			message.OwnerGroupPaths = message.OwnerGroup
			message.OwnerGroupPathIDs = message.OwnerGroupID
		}
	}
	return message, nil
}

// collectNextTaskIDs 已并入 writeCompleteTailChain 返回值（避免重复任务列表查询）。
// 保留占位说明：新待办 Id 以收尾链生成结果为准。

// isInstanceFinished 判断实例是否已结束（运行表查不到或实例 Ended）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 是否已结束
func (s *CompleteService) isInstanceFinished(ctx context.Context, processInstanceID string) bool {
	// 无法定位实例（孤儿任务）：按未结束处理，不虚报终态
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

// userDisplayName 查询用户展示名（身份查询不可用或查不到时返回空串）。
// @param ctx 操作上下文
// @param userID 用户 Id
// @return 展示名
func (s *CompleteService) userDisplayName(ctx context.Context, userID string) string {
	// 身份查询未注入时留空（嵌入模式可省身份库）
	if s.users == nil || userID == "" {
		return ""
	}
	if user, err := s.users.GetUser(ctx, userID); err == nil {
		return userDisplayName(user)
	}
	return ""
}

// ─── 委派 / 转办 / 归还 ─────────────────────────────────────────────

// DelegateTask 委派任务（对位 Java delegateTask:1793）：
// 显式置 owner → 引擎 Delegate（PENDING）→ WP 意见 → 操作记录 →
// 删除委派节点预存记录（排除自环）→ 待办 consigner 字段更新。
// @param ctx 操作上下文
// @param req 委派请求
// @return 错误
func (s *CompleteService) DelegateTask(ctx context.Context, req *DelegateTaskRequest) error {
	if req == nil {
		return apiError("WORKFLOW-2021", "委派请求为空")
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return apiError("WORKFLOW-2022", "参数 taskId 为空")
	}
	if strings.TrimSpace(req.Owner) == "" {
		return apiError("WORKFLOW-2022", "参数 owner 为空")
	}
	if strings.TrimSpace(req.DelegateID) == "" {
		return apiError("WORKFLOW-2022", "参数 delegateId 为空")
	}
	if strings.TrimSpace(req.Message) == "" {
		return apiError("WORKFLOW-2022", "参数 message 为空")
	}
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	if err := s.ensureInstanceActive(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	// Java：owner == delegateId → "您已是任务拥有者，无需委派任务"
	if req.Owner == req.DelegateID {
		return apiError("WORKFLOW-2023", "您已是任务拥有者，无需委派任务")
	}
	// 显式置 owner（Java setAssignee(owner) 后 delegate；引擎 Delegate 首次委派才继承，
	// 这里先写 Owner 保证委派人与请求一致）
	owner := req.Owner
	if _, err := s.engine.TaskService().UpdateTask(ctx, req.TaskID, api.TaskUpdate{Owner: &owner, UserID: req.Owner}); err != nil {
		return err
	}
	// 引擎委派：assignee=被委托人 + DelegationState=PENDING
	if err := s.engine.TaskService().Delegate(ctx, req.TaskID, req.DelegateID); err != nil {
		return err
	}
	// 收尾模板：WP 意见 + 操作记录 + 删预存 + 待办更新
	if err := s.writeAssignTailChain(ctx, task, req.Owner, req.Message, CommentTypeWP); err != nil {
		return err
	}
	// 待办 consigner 字段更新：受托人=被委托人，待办转由被委托人办理
	return s.refreshTodoOwner(ctx, req.TaskID, req.DelegateID, req.DelegateID)
}

// TransferTask 转办任务（对位 Java transferTask:1848）：
// setOwner + setAssignee → ZB 意见 → 操作记录 → 删预存记录（排除自环）→ 待办更新给接收人。
// @param ctx 操作上下文
// @param req 转办请求
// @return 错误
func (s *CompleteService) TransferTask(ctx context.Context, req *TransferTaskRequest) error {
	if req == nil {
		return apiError("WORKFLOW-2024", "转办请求为空")
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return apiError("WORKFLOW-2025", "参数 taskId 为空")
	}
	if strings.TrimSpace(req.Owner) == "" {
		return apiError("WORKFLOW-2025", "参数 owner 为空")
	}
	if strings.TrimSpace(req.Receiver) == "" {
		return apiError("WORKFLOW-2025", "参数 receiver 为空")
	}
	if strings.TrimSpace(req.Message) == "" {
		return apiError("WORKFLOW-2025", "参数 message 为空")
	}
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	if err := s.ensureInstanceActive(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	// Java：owner == receiver → "您已是任务拥有者，无需转办任务"
	if req.Owner == req.Receiver {
		return apiError("WORKFLOW-2026", "您已是任务拥有者，无需转办任务")
	}
	// Java setOwner + setAssignee（ZB 转办不改变委派状态）
	owner := req.Owner
	receiver := req.Receiver
	if _, err := s.engine.TaskService().UpdateTask(ctx, req.TaskID, api.TaskUpdate{Owner: &owner, Assignee: &receiver, UserID: req.Owner}); err != nil {
		return err
	}
	// 收尾模板：ZB 意见 + 操作记录 + 删预存 + 待办更新
	if err := s.writeAssignTailChain(ctx, task, req.Owner, req.Message, CommentTypeZB); err != nil {
		return err
	}
	// 待办更新：处理人改为接收人，无受托人语义
	return s.refreshTodoOwner(ctx, req.TaskID, req.Receiver, "")
}

// ResolveTask 归还（处理委派，对位 Java resolveTask:1904）：
// 引擎 Resolve（owner 还原 + DelegationState=RESOLVED）→ 意见 → 操作记录 → 待办还给 owner。
// @param ctx 操作上下文
// @param req 归还请求
// @return 错误
func (s *CompleteService) ResolveTask(ctx context.Context, req *ResolveTaskRequest) error {
	if req == nil || strings.TrimSpace(req.TaskID) == "" {
		return apiError("WORKFLOW-2027", "参数 taskId 为空")
	}
	if strings.TrimSpace(req.Message) == "" {
		return apiError("WORKFLOW-2027", "参数 message 为空")
	}
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	if err := s.ensureInstanceActive(ctx, task.ProcessInstanceID); err != nil {
		return err
	}
	// Java resolveTask 无操作人校验；引擎非空操作人必须为所有者，故传空跳过
	if err := s.engine.TaskService().Resolve(ctx, req.TaskID, ""); err != nil {
		return err
	}
	// 留痕操作人：请求操作人缺省取任务所有者
	actor := firstNonEmpty(req.UserCode, task.Owner)
	commentType := firstNonEmpty(req.Type, CommentTypeSP)
	if _, err := s.comments.Add(ctx, CommentRequest{
		TaskID:            req.TaskID,
		ProcessInstanceID: task.ProcessInstanceID,
		UserID:            actor,
		Message:           req.Message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	agree := 1
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            req.TaskID,
		ProcessInstanceID: task.ProcessInstanceID,
		OwnerID:           actor,
		IsAgree:           &agree,
		Message:           req.Message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	// 待办还给还原后的 owner（引擎 Resolve 后 assignee=owner）
	owner := firstNonEmpty(task.Owner, task.Assignee)
	return s.refreshTodoOwner(ctx, req.TaskID, owner, "")
}

// writeAssignTailChain 委派/转办的收尾模板（对位 Java delegateTask/transferTask 收尾段）：
// 意见 → 操作记录 → 删除转办/委派节点预存记录（排除自环）。
// @param ctx 操作上下文
// @param task 当前任务
// @param operator 操作人（owner）
// @param message 意见内容
// @param commentType 意见类型（WP/ZB）
// @return 错误
func (s *CompleteService) writeAssignTailChain(ctx context.Context, task api.Task, operator, message, commentType string) error {
	// 意见（WP 委派 / ZB 转办）
	if _, err := s.comments.Add(ctx, CommentRequest{
		TaskID:            task.ID,
		ProcessInstanceID: task.ProcessInstanceID,
		UserID:            operator,
		Message:           message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	// 操作记录（isAgree=1，Java addOperationRecord 第 4 参）
	agree := 1
	if _, err := s.operations.Save(ctx, OperationRequest{
		TaskID:            task.ID,
		ProcessInstanceID: task.ProcessInstanceID,
		OwnerID:           operator,
		IsAgree:           &agree,
		Message:           message,
		Type:              commentType,
	}); err != nil {
		return err
	}
	// 删除转办/委派节点的预存记录（Java deleteTransferWorkflowNextActivity：排除 pre_task_id=taskId 的自环）
	return s.deleteTransferNextActivity(ctx, task)
}

// deleteTransferNextActivity 删除转办/委派节点上当前任务的预存记录（排除自环）。
// @param ctx 操作上下文
// @param task 当前任务
// @return 错误
func (s *CompleteService) deleteTransferNextActivity(ctx context.Context, task api.Task) error {
	records, err := s.next.ListByTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, record := range records {
		// Java deleteTransferWorkflowNextActivity：instance+node+task 且 pre_task_id != task（自环保留）
		if record.NextActivityID != task.TaskDefinitionKey || record.PreTaskID == task.ID {
			continue
		}
		if err := s.next.DeleteForResolve(ctx, task.ProcessInstanceID, record.NextActivityID, task.ID, record.IdentityID); err != nil {
			return err
		}
	}
	return nil
}

// refreshTodoOwner 重建待办的处理人与受托人字段（存储端口无列级更新，
// 以删后重建实现 Java 删待办+重生成 的净效果）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @param ownerID 新处理人 Id，空保持原值
// @param consignerID 受托人 Id（委派场景为被委托人），空串清空
// @return 错误
func (s *CompleteService) refreshTodoOwner(ctx context.Context, taskID, ownerID, consignerID string) error {
	existing, found, err := s.messages.GetByTaskID(ctx, taskID)
	// 无待办（如组任务未生成）时无事可做
	if err != nil || !found {
		return err
	}
	updated := *existing
	// 处理人变更：待办转给新处理人
	if ownerID != "" {
		updated.OwnerID = ownerID
		updated.OwnerName = s.userDisplayName(ctx, ownerID)
	}
	// 受托人字段（委派场景记录被委托人）
	updated.ConsignerID = consignerID
	updated.Consigner = s.userDisplayName(ctx, consignerID)
	if err := s.messages.DeleteByTaskID(ctx, taskID); err != nil {
		return err
	}
	_, err = s.messages.CreateForTask(ctx, &updated)
	return err
}
