package workflow

import (
	"context"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// countEnabled 是父任务"计数启用"扩展变量名（对位 Java TaskEntity.countEnabled，加签父任务标记）。
const varCountEnabled = "countEnabled"

// 协议 DTO 归属 api 包（门面契约引用），此处保留包内别名以维持既有引用不变。
type (
	AddSignRequest = api.AddSignRequest // 前后加签请求（对位 Java AddSignTaskVo）
	DelSignRequest = api.DelSignRequest // 减签请求（对位加签的逆操作语义）
)

// AddSignResult 是加签结果。
type AddSignResult struct {
	ParentTaskID      string   // 父任务 Id（首层加签 = 当前任务 Id）
	SubTaskIDs        []string // 新建被加签人子任务 Id 列表
	PlaceholderTaskID string   // 首层加签时为加签人创建并立即办结的占位子任务 Id
	CommentType       string   // 意见类型代码（QJQ 前加签 / HJQ 后加签）
}

// DelSignResult 是减签结果。
type DelSignResult struct {
	ParentResolved bool   // 剩余子任务为空时父任务是否已恢复（resolve 还原 assignee=owner）
	ParentFinished bool   // 后加签且父任务已随之办结
	CommentType    string // 意见类型代码（QJQ/HJQ，与作用域对应）
}

// MultiInstanceMemberPort 是多实例动态成员的引擎端口（接口位）。
// Java 原生能力为 runtimeService.addMultiInstanceExecution(nodeId, proInstId, {assignee}) /
// deleteMultiInstanceExecution(executionId, completed)；Go 引擎 RuntimeService 暂无等价 API
// （P3-A 结论：引擎侧动态创建/销毁多实例子执行未实现），注入本端口可承接引擎侧能力，
// 未注入时仅做 workflow 侧 executorList/nrOfInstances 变量维护。
type MultiInstanceMemberPort interface {
	// AddMember 向多实例活动动态追加一个实例成员。
	// @param ctx 操作上下文
	// @param processInstanceID 流程实例 ID
	// @param activityID 多实例活动 ID
	// @param assignee 新成员身份 Id
	// @return 错误
	AddMember(ctx context.Context, processInstanceID, activityID, assignee string) error
	// RemoveMember 移除多实例活动的一个实例成员。
	// @param ctx 操作上下文
	// @param executionID 被移除实例的执行 ID
	// @param completed 移除时是否视作已完成（Java deleteMultiInstanceExecution 同名参数）
	// @return 错误
	RemoveMember(ctx context.Context, executionID string, completed bool) error
}

// AddSignService 是加签/减签域服务（P3-A）。
// 对位 Java ProcessesServiceImpl#addSignTask:311 / #createSignSubTasks:352 / ProcessesUtil#createSubTask:4351。
// 子任务全部完成后的父任务联动（resolve/后加签办结）由 complete.go handleParentTaskAfterComplete
// 在办结链路实现，本服务只负责加签侧改造（父任务须保持 PENDING 委派态以匹配该联动的 Resolve 语义）。
type AddSignService struct {
	engine     api.Engine              // 引擎门面（七服务）
	comments   *CommentService         // 审批意见
	operations *OperationRecordService // 操作记录
	messages   *MessageExtendService   // 待办扩展
	histories  *HistoryExtendService   // 已办扩展
	members    MultiInstanceMemberPort // 多实例动态成员引擎端口（可选，nil 仅变量维护）
	users      UserQuery               // 身份查询（补姓名展示）
	clock      clock.Clock             // 时间源
}

// NewAddSignService 构造加签/减签域服务。
// @param engine 引擎门面（七服务）
// @param comments 审批意见域服务
// @param operations 操作记录域服务
// @param messages 待办扩展域服务
// @param histories 已办扩展域服务
// @param c 时间源，nil 时回退真实时钟
// @return 加签/减签域服务
func NewAddSignService(engine api.Engine, comments *CommentService, operations *OperationRecordService, messages *MessageExtendService, histories *HistoryExtendService, c clock.Clock) *AddSignService {
	if c == nil {
		c = clock.Real{}
	}
	return &AddSignService{engine: engine, comments: comments, operations: operations, messages: messages, histories: histories, clock: c}
}

// WithMultiInstancePort 注入多实例动态成员引擎端口。
// @param port 引擎端口
// @return 加签/减签域服务（链式）
func (s *AddSignService) WithMultiInstancePort(port MultiInstanceMemberPort) *AddSignService {
	s.members = port
	return s
}

// WithAddSignUsers 注入身份查询（补待办姓名展示）。
// @param users 身份查询
// @return 加签/减签域服务（链式）
func (s *AddSignService) WithAddSignUsers(users UserQuery) *AddSignService {
	s.users = users
	return s
}

// AddSign 前后加签（对位 Java addSignTask:311）。
// 首层加签（当前任务无父任务）：当前任务改造为父任务（owner=操作人、assignee 置空、
// 扩展变量 countEnabled=true/scopeType，引擎侧再置 PENDING 委派态），为每个被加签人建子任务，
// 并为加签人自己建占位子任务立即办结（对位 Java 占位跑通链）；
// 二层加签（当前任务已是子任务）：新子任务挂到顶层父任务（与既有子任务平级），当前子任务立即办结。
// 收尾：候选人清理（加签人是父任务候选时移除）→ QJQ/HJQ 意见 → 操作记录 → 待办（新子任务生成，
// 父任务待办标记 consigner）。
// @param ctx 操作上下文
// @param req 加签请求
// @return 加签结果
// @return 错误
func (s *AddSignService) AddSign(ctx context.Context, req AddSignRequest) (AddSignResult, error) {
	if err := s.validateAddSignRequest(req); err != nil {
		return AddSignResult{}, err
	}
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return AddSignResult{}, api.NewError("WORKFLOW-2004", "任务不存在", err)
	}
	commentType := s.addSignCommentType(req.ScopeType)
	firstLevel := task.ParentTaskID == ""
	if firstLevel {
		// 1. 当前任务改造为父任务（Java setOwner/setAssignee(null)/countEnabled/scopeType + saveTask）
		if err := s.convertParentTask(ctx, task, req); err != nil {
			return AddSignResult{}, err
		}
	}
	// 2. 创建被加签人子任务（Java createSignSubTasks：parentTaskId 指向顶层父任务）
	parentTaskID := task.ID
	topParent := task
	if !firstLevel {
		parentTaskID = task.ParentTaskID
		// 待办的实例/租户信息取顶层父任务（子任务为独立任务，无实例归属）
		top, err := s.engine.TaskService().GetTask(ctx, parentTaskID)
		if err != nil {
			return AddSignResult{}, api.NewError("WORKFLOW-2004", "父任务不存在", err)
		}
		topParent = top
	}
	subTaskIDs := make([]string, 0, len(req.Assignees))
	for _, assignee := range req.Assignees {
		// 空身份跳过（Java isNotBlank 门控）
		if strings.TrimSpace(assignee) == "" {
			continue
		}
		sub, err := s.engine.TaskService().CreateTask(ctx, api.CreateTaskRequest{
			Name:         task.Name,
			Description:  task.Description,
			Assignee:     assignee,
			Owner:        req.Operator,
			Priority:     task.Priority,
			Category:     task.Category,
			FormKey:      task.FormKey,
			ParentTaskID: parentTaskID,
			TenantID:     topParent.TenantID,
		})
		if err != nil {
			return AddSignResult{}, err
		}
		subTaskIDs = append(subTaskIDs, sub.ID)
		// 新被加签人待办生成
		if err := s.createSubTaskTodo(ctx, topParent, sub, req.Operator); err != nil {
			return AddSignResult{}, err
		}
	}
	// 3. 占位/当前子任务办结（Java：首层为加签人建占位子任务并 complete；二层直接 complete 当前子任务）
	placeholderTaskID := ""
	if firstLevel {
		placeholder, err := s.engine.TaskService().CreateTask(ctx, api.CreateTaskRequest{
			Name:         task.Name,
			Description:  task.Description,
			Assignee:     req.Operator,
			Owner:        req.Operator,
			Priority:     task.Priority,
			Category:     task.Category,
			FormKey:      task.FormKey,
			ParentTaskID: parentTaskID,
			TenantID:     task.TenantID,
		})
		if err != nil {
			return AddSignResult{}, err
		}
		placeholderTaskID = placeholder.ID
	}
	// 引擎直办（独立任务仅记历史并移除，不驱动流程；不经 workflow 办结链）
	closeTaskID := req.TaskID
	if firstLevel {
		closeTaskID = placeholderTaskID
	}
	if err := s.engine.TaskService().Complete(ctx, closeTaskID, api.CompleteTaskRequest{UserID: req.Operator}); err != nil {
		return AddSignResult{}, err
	}
	// 4. 候选人清理（Java：加签人是父任务候选时 deleteCandidateUser）
	if err := s.removeOperatorCandidate(ctx, parentTaskID, req.Operator); err != nil {
		return AddSignResult{}, err
	}
	// 5. QJQ/HJQ 意见（Java addComment；意见挂到带实例归属的顶层父任务，
	// 引擎评论 API 绑定运行态任务，独立子任务无实例不可挂）
	if _, err := s.comments.Add(ctx, CommentRequest{
		TaskID:            topParent.ID,
		ProcessInstanceID: topParent.ProcessInstanceID,
		UserID:            req.Operator,
		Message:           req.Message,
		Type:              commentType,
	}); err != nil {
		return AddSignResult{}, err
	}
	// 6. 操作记录（指令要求补齐；Java addSignTask 未写操作记录，此处按域服务留痕约束补记）
	agree := 1
	if topParent.ProcessInstanceID != "" {
		if _, err := s.operations.Save(ctx, OperationRequest{
			TaskID:            topParent.ID,
			ProcessInstanceID: topParent.ProcessInstanceID,
			OwnerID:           req.Operator,
			IsAgree:           &agree,
			Message:           req.Message,
			Type:              commentType,
		}); err != nil {
			return AddSignResult{}, err
		}
	}
	// 7. 父任务待办标记 consigner（原待办保留给 owner，受托人=加签操作人）
	if firstLevel {
		if err := s.markParentTodoConsigner(ctx, task.ID, req.Operator); err != nil {
			return AddSignResult{}, err
		}
	}
	return AddSignResult{
		ParentTaskID:      parentTaskID,
		SubTaskIDs:        subTaskIDs,
		PlaceholderTaskID: placeholderTaskID,
		CommentType:       commentType,
	}, nil
}

// validateAddSignRequest 校验加签入参。
// @param req 加签请求
// @return 错误
func (s *AddSignService) validateAddSignRequest(req AddSignRequest) error {
	if strings.TrimSpace(req.TaskID) == "" {
		return apiError("WORKFLOW-2201", "参数 taskId 为空")
	}
	if strings.TrimSpace(req.Operator) == "" {
		return apiError("WORKFLOW-2201", "参数 userCode 为空")
	}
	// 至少一名被加签人（Java signPersoneds 空集合时不产生任何子任务，属误用）
	hasAssignee := false
	for _, assignee := range req.Assignees {
		if strings.TrimSpace(assignee) != "" {
			hasAssignee = true
			break
		}
	}
	if !hasAssignee {
		return apiError("WORKFLOW-2202", "被加签人列表为空")
	}
	if strings.TrimSpace(req.Message) == "" {
		return apiError("WORKFLOW-2203", "参数 message 为空")
	}
	// 作用域只允许前后两种（FlowConstant.AFTER_ADDSIGN/BEFORE_ADDSIGN）
	if req.ScopeType != AddSignScopeAfter && req.ScopeType != AddSignScopeBefore {
		return apiError("WORKFLOW-2204", "参数 scopeType 非法（仅支持 after/before）")
	}
	return nil
}

// addSignCommentType 按作用域返回意见类型（Java：flag 后加签 → HJQ，否则 QJQ）。
// @param scopeType 作用域
// @return 意见类型代码
func (s *AddSignService) addSignCommentType(scopeType string) string {
	if scopeType == AddSignScopeAfter {
		return CommentTypeHJQ
	}
	return CommentTypeQJQ
}

// convertParentTask 将当前任务改造为父任务（Java addSignTask 第 1 步）。
// owner=操作人、assignee 置空、扩展变量 countEnabled=true/scopeType；
// 引擎侧置 PENDING 委派态——Go 引擎 Resolve 校验 PENDING（Java Flowable resolveTask 无此校验），
// 办结链路 handleParentTaskAfterComplete 以 Resolve 归还父任务，故加签后保持委派态等价对位。
// @param ctx 操作上下文
// @param task 当前任务
// @param req 加签请求
// @return 错误
func (s *AddSignService) convertParentTask(ctx context.Context, task api.Task, req AddSignRequest) error {
	owner := req.Operator
	assignee := ""
	if _, err := s.engine.TaskService().UpdateTask(ctx, task.ID, api.TaskUpdate{Owner: &owner, Assignee: &assignee, UserID: req.Operator}); err != nil {
		return err
	}
	// 扩展变量 countEnabled=true（对位 TaskEntity.setCountEnabled(true)，父任务局部变量）
	if err := s.engine.TaskService().SetTaskVariable(ctx, task.ID, varCountEnabled, true); err != nil {
		return err
	}
	// 扩展变量 scopeType（complete.go isAfterAddSignScope 读取判定后加签办结）
	if err := s.engine.TaskService().SetTaskVariable(ctx, task.ID, varScopeType, req.ScopeType); err != nil {
		return err
	}
	// PENDING 委派态（引擎 Resolve 前置条件，见函数注释）
	return s.engine.TaskService().Delegate(ctx, task.ID, req.Operator)
}

// removeOperatorCandidate 清理加签人在父任务上的候选身份（Java createSignSubTasks 尾段）。
// @param ctx 操作上下文
// @param parentTaskID 父任务 Id
// @param operator 加签人身份 Id
// @return 错误
func (s *AddSignService) removeOperatorCandidate(ctx context.Context, parentTaskID, operator string) error {
	links, err := s.engine.TaskService().ListIdentityLinks(ctx, parentTaskID)
	if err != nil {
		return err
	}
	for _, link := range links {
		// 仅清理加签人本人的候选用户链接（Java taskCandidateUser 查询后删除）
		if link.Type == "candidate" && link.UserID == operator {
			return s.engine.TaskService().DeleteCandidateUser(ctx, parentTaskID, operator)
		}
	}
	return nil
}

// markParentTodoConsigner 标记父任务待办的受托人（原待办保留给 owner）。
// 存储端口无列级更新，以删后重建实现（与 complete.go refreshTodoOwner 同法）。
// @param ctx 操作上下文
// @param parentTaskID 父任务 Id
// @param operator 加签操作人 Id
// @return 错误
func (s *AddSignService) markParentTodoConsigner(ctx context.Context, parentTaskID, operator string) error {
	existing, found, err := s.messages.GetByTaskID(ctx, parentTaskID)
	// 无待办（组任务未生成等）时无事可做
	if err != nil || !found {
		return err
	}
	updated := *existing
	// 受托人 = 加签操作人
	updated.ConsignerID = operator
	updated.Consigner = s.userDisplayName(ctx, operator)
	if err := s.messages.DeleteByTaskID(ctx, parentTaskID); err != nil {
		return err
	}
	_, err = s.messages.CreateForTask(ctx, &updated)
	return err
}

// createSubTaskTodo 为被加签人子任务生成待办。
// @param ctx 操作上下文
// @param parent 父任务（原任务）
// @param sub 新建子任务
// @param senderID 加签操作人 Id
// @return 错误
func (s *AddSignService) createSubTaskTodo(ctx context.Context, parent, sub api.Task, senderID string) error {
	message := &MessageExtend{
		BaseColumns:       BaseColumns{TenantID: parent.TenantID},
		TaskID:            sub.ID,
		ProcessInstanceID: parent.ProcessInstanceID,
		Title:             parent.Name,
		BusinessID:        s.businessKeyOf(ctx, parent.ProcessInstanceID),
		TaskStepName:      parent.Name,
		TaskStepCode:      parent.TaskDefinitionKey,
		OwnerID:           sub.Assignee,
		OwnerName:         s.userDisplayName(ctx, sub.Assignee),
		SenderID:          senderID,
		Sender:            s.userDisplayName(ctx, senderID),
		ConsignerID:       senderID,
		Consigner:         s.userDisplayName(ctx, senderID),
		TaskReceiptTime:   &sub.CreateTime,
	}
	// 无实例归属的孤儿任务（独立父任务）待办仍生成，实例维度字段留空
	if _, err := s.messages.CreateForTask(ctx, message); err != nil {
		return err
	}
	// 已办镜像同步生成（与 complete.go generateTodosForNewTasks 同构）
	history := &HistoryExtend{MessageExtend: *message}
	_, err := s.histories.CreateForTask(ctx, history)
	return err
}

// businessKeyOf 查询实例业务键（查询失败返回空串，不阻断加签）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 业务键
func (s *AddSignService) businessKeyOf(ctx context.Context, processInstanceID string) string {
	if processInstanceID == "" {
		return ""
	}
	instance, err := s.engine.RuntimeService().GetProcessInstance(ctx, processInstanceID)
	if err != nil {
		return ""
	}
	return instance.BusinessKey
}

// userDisplayName 查询用户展示名（身份查询未注入或查不到时返回空串）。
// @param ctx 操作上下文
// @param userID 用户 Id
// @return 展示名
func (s *AddSignService) userDisplayName(ctx context.Context, userID string) string {
	if s.users == nil || userID == "" {
		return ""
	}
	if user, err := s.users.GetUser(ctx, userID); err == nil {
		return userDisplayName(user)
	}
	return ""
}

// DelSign 减签（删除指定加签子任务；Java 无任务级减签对位源码，按加签逆操作定义）。
// 删除子任务 + QJQ/HJQ 意见 + 删待办/已办；剩余子任务为空时恢复父任务
// （resolve 还原 assignee=owner），后加签作用域下父任务随之办结。
// @param ctx 操作上下文
// @param req 减签请求
// @return 减签结果
// @return 错误
func (s *AddSignService) DelSign(ctx context.Context, req DelSignRequest) (DelSignResult, error) {
	if req.ParentTaskID == "" || req.SubTaskID == "" {
		return DelSignResult{}, apiError("WORKFLOW-2210", "参数 parentTaskId/subTaskId 为空")
	}
	if strings.TrimSpace(req.Operator) == "" {
		return DelSignResult{}, apiError("WORKFLOW-2210", "参数 operator 为空")
	}
	parent, err := s.engine.TaskService().GetTask(ctx, req.ParentTaskID)
	if err != nil {
		return DelSignResult{}, api.NewError("WORKFLOW-2004", "父任务不存在", err)
	}
	sub, err := s.engine.TaskService().GetTask(ctx, req.SubTaskID)
	if err != nil {
		return DelSignResult{}, api.NewError("WORKFLOW-2004", "子任务不存在", err)
	}
	// 子任务必须挂在该父任务下（防误删其他分支任务）
	if sub.ParentTaskID != parent.ID {
		return DelSignResult{}, apiError("WORKFLOW-2211", "子任务不属于该父任务")
	}
	// 意见类型与作用域对应（before→QJQ / after→HJQ；Java 无对位源码，复用加签类型语义）
	scopeType := addSignScopeOf(parent)
	commentType := s.addSignCommentType(scopeType)
	message := req.Message
	if strings.TrimSpace(message) == "" {
		message = "减签：" + sub.Assignee
	}
	// 1. 删除子任务（保留历史供追溯）
	if err := s.engine.TaskService().DeleteTask(ctx, req.SubTaskID, false, "减签"); err != nil {
		return DelSignResult{}, err
	}
	// 2. 减签意见
	if _, err := s.comments.Add(ctx, CommentRequest{
		TaskID:            parent.ID,
		ProcessInstanceID: parent.ProcessInstanceID,
		UserID:            req.Operator,
		Message:           message,
		Type:              commentType,
	}); err != nil {
		return DelSignResult{}, err
	}
	// 3. 删子任务待办与已办（todo 关联 task_id 唯一）
	if err := s.messages.DeleteByTaskID(ctx, req.SubTaskID); err != nil {
		return DelSignResult{}, err
	}
	if err := s.histories.DeleteByTaskID(ctx, req.SubTaskID); err != nil {
		return DelSignResult{}, err
	}
	// 4. 剩余子任务为空时恢复父任务
	result := DelSignResult{CommentType: commentType}
	remaining, err := s.engine.TaskService().ListSubTasks(ctx, parent.ID)
	if err != nil {
		return result, err
	}
	// 仍有子任务在办：父任务保持等待
	if len(remaining) > 0 {
		return result, nil
	}
	// resolve 还原父任务（assignee=owner；父任务非 PENDING 态时由引擎报错兜底）
	if err := s.engine.TaskService().Resolve(ctx, parent.ID, ""); err != nil {
		return result, err
	}
	result.ParentResolved = true
	// 后加签：父任务随之办结（与 complete.go handleParentTaskAfterComplete 语义对称）
	if scopeType == AddSignScopeAfter {
		if err := s.engine.TaskService().Complete(ctx, parent.ID, api.CompleteTaskRequest{UserID: parent.Owner}); err != nil {
			return result, err
		}
		result.ParentFinished = true
	}
	return result, nil
}

// addSignScopeOf 读取父任务的前后加签作用域（父任务局部变量 scopeType）。
// @param parent 父任务
// @return 作用域（未标记时按前加签处理）
func addSignScopeOf(parent api.Task) string {
	if scope, ok := parent.Variables[varScopeType]; ok {
		if scopeText, ok := scope.(string); ok && scopeText == AddSignScopeAfter {
			return AddSignScopeAfter
		}
	}
	return AddSignScopeBefore
}

// ─── 多实例动态加减成员（对位 Java PUtil#addExecution:1772/#delExecution:1782） ─────

// AddMultiInstanceMember 多实例动态加签成员（workflow 侧）：
// 维护 executorList 变量（追加成员）并同步 nrOfInstances；引擎端口注入时同时驱动
// 引擎侧实例创建（Go 引擎暂无原生 API，未注入仅变量维护）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param activityID 多实例活动 ID
// @param assignee 新成员身份 Id
// @return 错误
func (s *AddSignService) AddMultiInstanceMember(ctx context.Context, processInstanceID, activityID, assignee string) error {
	if strings.TrimSpace(processInstanceID) == "" || strings.TrimSpace(activityID) == "" || strings.TrimSpace(assignee) == "" {
		return apiError("WORKFLOW-2220", "参数 processInstanceId/activityId/assignee 为空")
	}
	// 引擎端口注入时先驱动引擎侧（变量维护失败随端口错误一并回滚语义由调用方控制）
	if s.members != nil {
		if err := s.members.AddMember(ctx, processInstanceID, activityID, assignee); err != nil {
			return err
		}
	}
	return s.appendExecutorList(ctx, processInstanceID, assignee)
}

// RemoveMultiInstanceMember 多实例动态减签成员（workflow 侧）：
// 维护 executorList 变量（移除成员）并同步 nrOfInstances；引擎端口注入时同时驱动
// 引擎侧实例销毁（executionID 为被移除实例的执行 ID）。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param activityID 多实例活动 ID
// @param assignee 待移除成员身份 Id
// @param executionID 被移除实例的执行 ID（引擎端口用，可空）
// @param completed 移除时是否视作已完成
// @return 错误
func (s *AddSignService) RemoveMultiInstanceMember(ctx context.Context, processInstanceID, activityID, assignee, executionID string, completed bool) error {
	if strings.TrimSpace(processInstanceID) == "" || strings.TrimSpace(activityID) == "" || strings.TrimSpace(assignee) == "" {
		return apiError("WORKFLOW-2220", "参数 processInstanceId/activityId/assignee 为空")
	}
	// 引擎端口注入时先驱动引擎侧
	if s.members != nil && executionID != "" {
		if err := s.members.RemoveMember(ctx, executionID, completed); err != nil {
			return err
		}
	}
	return s.removeFromExecutorList(ctx, processInstanceID, assignee)
}

// appendExecutorList 追加会签执行者并同步 nrOfInstances。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param assignee 新成员身份 Id
// @return 错误
func (s *AddSignService) appendExecutorList(ctx context.Context, processInstanceID, assignee string) error {
	runtime := s.engine.RuntimeService()
	current := toStringSliceOf(runtime, ctx, processInstanceID)
	// 已存在成员不重复追加（集合语义去重）
	for _, id := range current {
		if id == assignee {
			return nil
		}
	}
	next := append(current, assignee)
	if err := runtime.SetVariable(ctx, processInstanceID, VarExecutorList, next); err != nil {
		return err
	}
	// 循环总次数与集合长度同步（Java nrOfInstances = list.size() 语义）
	return runtime.SetVariable(ctx, processInstanceID, VarNrOfInstances, len(next))
}

// removeFromExecutorList 移除会签执行者并同步 nrOfInstances。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @param assignee 待移除成员身份 Id
// @return 错误
func (s *AddSignService) removeFromExecutorList(ctx context.Context, processInstanceID, assignee string) error {
	runtime := s.engine.RuntimeService()
	current := toStringSliceOf(runtime, ctx, processInstanceID)
	next := make([]string, 0, len(current))
	for _, id := range current {
		// 跳过待移除成员（保留其余顺序）
		if id == assignee {
			continue
		}
		next = append(next, id)
	}
	if err := runtime.SetVariable(ctx, processInstanceID, VarExecutorList, next); err != nil {
		return err
	}
	return runtime.SetVariable(ctx, processInstanceID, VarNrOfInstances, len(next))
}

// toStringSliceOf 读取实例 executorList 变量为字符串切片（缺失返回空切片）。
// @param runtime 运行时服务
// @param ctx 操作上下文
// @param processInstanceID 流程实例 ID
// @return 成员列表
func toStringSliceOf(runtime api.RuntimeService, ctx context.Context, processInstanceID string) []string {
	raw, err := runtime.GetVariable(ctx, processInstanceID, VarExecutorList)
	if err != nil || raw == nil {
		return []string{}
	}
	return toStringSlice(raw)
}
