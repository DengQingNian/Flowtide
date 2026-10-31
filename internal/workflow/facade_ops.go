package workflow

// 本文件承载 P2-C/P3/P4/P5 能力子服务的门面装配（assembleCapabilities）、
// 引擎适配器与门面新增操作方法。P1.2 基础装配在 facade.go，此处独立成文避免单文件过长。

import (
	"context"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// capabilityStores 是 P2-C/P3/P4/P5 能力装配所需的扩展存储集合
// （内存模式传内存实现族；SQL 模式传 SQL 适配器族）。
type capabilityStores struct {
	steps         StepStore               // 步骤规则存储（his_step 快照，nil 则推演/调度规则分支跳过）
	delegate      TaskDelegateStore       // 委托规则主表存储
	mirrors       DelegateTaskMirrorStore // 委托镜像待办存储
	exceptions    TaskExceptionStore      // 任务异常存储
	reminds       RemindStore             // 催办配置存储
	remindTargets RemindTargetStore       // 催办对象存储
	todos         TodoSource              // 待办数据源（调度扫描/催办；nil 时调度与催办扫描停用）
	models        ModelMgmtStore          // 模型/分类主数据存储（P5.A）
	designs       StepDesignStore         // 设计时步骤配置存储（P5.A）
	templates     TemplateStore           // 设计器模板存储（P5.A）
	hisSteps      HisStepStore            // 步骤部署快照存储（P5.A；需满足 StepStore 语义）
	reviews       ReviewTaskStore         // 审阅影子任务存储（P5.B）
	extends       ActExtendStore          // 审阅人绑定（活动扩展）存储（P5.B）
	weights       CountersignWeightStore  // 会签权重存储（/weight 接口数据，nil 时仅模型 weightList）
	businesses    BusinessSystemStore     // 业务系统注册存储（业务回调按 tenantId 查回调地址，nil 时回调停用）
}

// assembleCapabilities 在基础组件之上装配 P2-C/P3/P4/P5 能力子服务与调度器。
// 引擎为 nil（协议兼容场景）时整体跳过：能力方法在 nil 引擎下不可用。
// @param comp 基础组件聚合（就地扩充）
// @param engine 引擎门面
// @param stores 扩展存储集合
// @param opts 服务选项（调度开关透传）
// @param c 时钟
func assembleCapabilities(comp *components, engine api.Engine, stores capabilityStores, opts Options, c clock.Clock) {
	// 协议兼容场景（无引擎）不装配能力子服务
	if engine == nil || comp == nil {
		return
	}
	// ── 步骤规则（推演退回模式 + 调度规则来源）──
	comp.stepsStore = stores.steps
	if stores.steps != nil {
		comp.steps = NewStepRuleService(stores.steps, engine)
	}
	// ── P2.B/P2-C 退回/直送/重置/撤回/软终止族 ──
	comp.back = NewBackService(engine, comp.meta, comp.nextSvc, comp.comments, comp.operations,
		comp.messages, comp.histories, comp.documents, comp.steps, engineInstanceQuery{runtime: engine.RuntimeService(), history: engine.HistoryService()}, c)
	comp.direct = NewDirectService(engine, comp.meta, comp.nextSvc, comp.comments, comp.operations,
		comp.messages, comp.histories, comp.documents, comp.completeTo, c)
	comp.reset = NewResetService(engine, comp.meta, comp.nextSvc, comp.comments, comp.operations,
		comp.messages, comp.histories, comp.documents, c)
	comp.rollback = NewRollbackService(engine, comp.nextSvc, comp.comments, comp.operations,
		comp.messages, comp.histories, comp.documents, c)
	comp.terminate = NewTerminateService(engine, comp.nextSvc, comp.comments, comp.operations,
		comp.messages, comp.histories, comp.documents, c)
	// ── P3.A 加签减签与会签 ──
	comp.addSign = NewAddSignService(engine, comp.comments, comp.operations, comp.messages, comp.histories, c).
		WithAddSignUsers(engine.IdentityService())
	// 会签选项：权重存储 + next_activity 执行者来源 + 比例退回规则端口。
	// 016 号审计：此前生产仅注入权重，导致比例退回恒 disabled（percentReturnRule 直接返回
	// 未启用）、next_activity 预存覆盖失效（执行者退回模型默认），与文档 011 声称不符。
	// 成员查询（WithCountersignMembers）按 Java ProcessesUtil.getExecutorMap 语义故意不注入：
	// Java 在集合配置阶段不展开 org/role，保留组 ID；展开发生在推演器阶段。
	counterOpts := make([]CountersignOption, 0, 3)
	if stores.weights != nil {
		counterOpts = append(counterOpts, WithCountersignWeights(stores.weights))
	}
	if comp.nextSvc != nil {
		counterOpts = append(counterOpts, WithCountersignNext(comp.nextSvc))
	}
	if comp.steps != nil {
		counterOpts = append(counterOpts, WithCountersignRules(comp.steps))
	}
	comp.counter = NewCountersignService(engine, comp.meta, counterOpts...)
	// ── P5.B 下一步推演器（注入步骤规则 → 退回模式分支可用）──
	simOpts := make([]SimulateOption, 0, 1)
	if stores.steps != nil {
		simOpts = append(simOpts, WithSimulateSteps(stores.steps))
	}
	comp.simulate = NewSimulateService(engine, comp.meta, comp.nextSvc, comp.histories, comp.messages,
		engine.IdentityService(), c, simOpts...)
	// ── P4 委托/异常/催办 ──
	comp.delegate = NewDelegateRuleService(stores.delegate, stores.mirrors, comp.messages, comp.nextSvc,
		comp.operations, engine, engine.IdentityService(), nopCategoryLookup{}, c)
	comp.exception = NewExceptionService(stores.exceptions, c)
	// ── P1-C 业务系统注册与业务回调（016 号审计：此前业务系统存储未装配，
	// CallbackClient 在生产代码零调用点，D5"回调失败→异常表→管理台重发"实际不生效）──
	comp.businesses = stores.businesses
	if stores.businesses != nil {
		callbackOpts := make([]CallbackOption, 0, 1)
		// 回调失败经 ExceptionRecorder 落 workflow_task_exception
		if stores.exceptions != nil {
			callbackOpts = append(callbackOpts, WithCallbackRecorder(NewCallbackExceptionRecorder(comp.exception)))
		}
		comp.bizCallback = NewBusinessCallbackHook(comp.meta, NewCallbackClient(stores.businesses, callbackOpts...))
	}
	// 调度规则/步骤解析/渠道来源接设计时步骤数据（P5-A 落地后替换为真实现）
	comp.remind = NewRemindService(stores.reminds, stores.remindTargets, stores.todos,
		NopScheduleRuleProvider{}, NopChannelProvider{}, nopStepResolver{},
		engine.IdentityService(), runtimeVariableSource{runtime: engine.RuntimeService()}, c)
	// ── P5.A 模型/步骤/模板管理与部署快照 ──
	comp.modelMgmt = NewModelService(stores.models, c)
	comp.stepDesign = NewStepDesignService(stores.designs, c)
	comp.template = NewTemplateService(stores.templates, c)
	if stores.models != nil && stores.designs != nil && stores.hisSteps != nil {
		comp.snapshot = NewDeploySnapshotService(stores.models, stores.designs, stores.hisSteps, c)
	}
	// ── P5.B 审阅影子任务与流程图高亮 ──
	comp.extendsStore = stores.extends
	comp.review = NewReviewTaskService(engine, stores.reviews, stores.extends, c)
	comp.highlight = NewHighlightService(engine)
	// ── 调度器（不自动启动，由宿主调用 StartSchedulers；开关透传服务选项）──
	comp.scheduler = NewScheduler(SchedulerConfig{
		Engine:     engine,
		Options:    opts,
		TodoSource: stores.todos,
		Complete:   comp.complete,
		Next:       comp.nextSvc,
		Back:       backExecutorAdapter{back: comp.back},
		Remind:     comp.remind,
		Exceptions: comp.exception,
	})
}

// ─── 装配适配器 ─────────────────────────────────────────────

// nopCategoryLookup 是分类解析端口的空实现（委托规则限定分类前先取空）。
// P5-A 模型服务落地后由模型/分类服务替换。
type nopCategoryLookup struct{}

// ResolveModelID 解析分类代号为模型 Id（空实现恒返回空串，委托规则按"全部分类"匹配）。
// @param ctx 操作上下文
// @param tenantID 租户
// @param flowCategoryCode 分类代号（=定义 Key）
// @return 模型 Id（恒空串）
// @return 错误（恒 nil）
// @param _ 接口占位参数（实现不使用）
// @param _ 接口占位参数（实现不使用）
// @param _ 接口占位参数（实现不使用）
func (nopCategoryLookup) ResolveModelID(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// nopStepResolver 是步骤代号解析端口的空实现（催办白名单匹配跳过）。
// P5-A 设计时步骤服务落地后替换。
type nopStepResolver struct{}

// ResolveStepID 解析步骤代号为设计时步骤 Id（空实现恒返回空串）。
// @param ctx 操作上下文
// @param stepCode 步骤代号
// @return 步骤 Id（恒空串）
// @return 错误（恒 nil）
// @param _ 接口占位参数（实现不使用）
// @param _ 接口占位参数（实现不使用）
func (nopStepResolver) ResolveStepID(_ context.Context, _ string) (string, error) {
	return "", nil
}

// runtimeVariableSource 是流程变量读取端口的引擎适配（api.RuntimeService 子集透传）。
type runtimeVariableSource struct {
	runtime api.RuntimeService // 运行时服务
}

// GetVariable 读取流程实例变量。
// @param ctx 操作上下文
// @param processInstanceID 实例 Id
// @param name 变量名
// @return 变量值
// @return 查询错误
func (s runtimeVariableSource) GetVariable(ctx context.Context, processInstanceID, name string) (any, error) {
	return s.runtime.GetVariable(ctx, processInstanceID, name)
}

// 编译期断言：引擎适配器满足端口。
var _ InstanceVariableSource = runtimeVariableSource{}

// emptyTodoSource 是待办数据源的空实现（SQL 模式裸 SQL 待办查询未补齐前的占位）。
// 调度扫描与催办扫描在空源下无待办可扫（行为安全，不报错）。
type emptyTodoSource struct{}

// ListAll 返回空待办列表。
// @param ctx 操作上下文
// @return 空列表
// @return 错误（恒 nil）
// @param _ 接口占位参数（实现不使用）
func (emptyTodoSource) ListAll(_ context.Context) ([]*MessageExtend, error) {
	return []*MessageExtend{}, nil
}

// GetByTaskID 按任务查询待办扩展（空实现恒未命中）。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 待办扩展（恒 nil）
// @return 是否存在（恒 false）
// @return 错误（恒 nil）
// @param _ 接口占位参数（实现不使用）
// @param _ 接口占位参数（实现不使用）
func (emptyTodoSource) GetByTaskID(_ context.Context, _ string) (*MessageExtend, bool, error) {
	return nil, false, nil
}

// 编译期断言：空待办源满足端口。
var _ TodoSource = emptyTodoSource{}

// backExecutorAdapter 将退回服务适配为调度器自动办结的退回执行器端口。
type backExecutorAdapter struct {
	back *BackService // 退回服务
}

// BackToStepTask 退回任务到指定步骤（调度器退回型自动办结回调）。
// @param ctx 操作上下文
// @param vo 退回请求（TargetActivityID 必填）
// @return 错误
func (a backExecutorAdapter) BackToStepTask(ctx context.Context, vo *api.BackTaskVo) error {
	if a.back == nil || vo == nil {
		return api.NewError("WORKFLOW-0601", "退回服务未装配或请求为空", nil)
	}
	return a.back.BackToStep(ctx, *vo, vo.TargetActivityID)
}

// 编译期断言：适配器满足调度器退回执行器端口。
var _ AutoCompleteBackExecutor = backExecutorAdapter{}

// ─── P2.B/P2-C 门面方法（替换阶段 0 桩）────────────────────────────

// requireCapable 断言能力子服务已装配（nil 引擎协议场景快速失败）。
// @param comp 组件聚合
// @return 错误（未装配时返回 WORKFLOW-0601）
func requireCapable(comp *components) error {
	if comp == nil || comp.back == nil {
		return api.NewError("WORKFLOW-0601", "workflow 能力子服务未装配（缺少引擎）", nil)
	}
	return nil
}

// GetBackNodes 查询任务的可退回节点列表（前端"退回"按钮的数据源）。
// @param ctx 上下文
// @param taskID 当前任务 ID
// @return 统一返回（data 为可退节点视图列表）
// @return 错误
func (s *Service) GetBackNodes(ctx context.Context, taskID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	nodes, err := s.comp.back.BackNodes(ctx, taskID)
	if err != nil {
		return R{}, err
	}
	return ROk(nodes), nil
}

// BackToStepTask 退回任务到指定步骤（任意节点退回；目标为空时由服务端计算）。
// @param ctx 上下文
// @param vo 退回请求
// @return 统一返回
// @return 错误
func (s *Service) BackToStepTask(ctx context.Context, vo *BackTaskVo) (R, error) {
	if vo == nil {
		return R{}, api.NewError("WORKFLOW-0400", "退回请求不能为空", api.ErrBadRequest)
	}
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	// 目标节点为空时回退上一节点语义（Java backToStepTask 空目标取串行上一节点）
	target := vo.TargetActivityID
	if target == "" {
		if err := s.comp.back.BackToPrevious(ctx, *vo); err != nil {
			return R{}, err
		}
		return ROkMsg("退回成功", nil), nil
	}
	if err := s.comp.back.BackToStep(ctx, *vo, target); err != nil {
		return R{}, err
	}
	return ROkMsg("退回成功", nil), nil
}

// BackTask 退回任务到上一节点（仅串行流程）。
// @param ctx 上下文
// @param vo 退回请求
// @return 统一返回
// @return 错误
func (s *Service) BackTask(ctx context.Context, vo *BackTaskVo) (R, error) {
	if vo == nil {
		return R{}, api.NewError("WORKFLOW-0400", "退回请求不能为空", api.ErrBadRequest)
	}
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.back.BackToPrevious(ctx, *vo); err != nil {
		return R{}, err
	}
	return ROkMsg("退回成功", nil), nil
}

// DoRollbackTask 撤回任务（发起人/特权人撤回已流转任务，策略族执行）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param taskID 待撤回任务 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoRollbackTask(ctx context.Context, processInstanceID, taskID, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.rollback.Rollback(ctx, processInstanceID, taskID, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("撤回成功", nil), nil
}

// CompleteToTargetActivity 直送：办结并提交至指定目标节点。
// @param ctx 上下文
// @param vo 办结请求（TargetActivityID 必填）
// @return 统一返回
// @return 错误
func (s *Service) CompleteToTargetActivity(ctx context.Context, vo *CompleteTaskVo) (R, error) {
	if vo == nil {
		return R{}, api.NewError("WORKFLOW-0400", "直送请求不能为空", api.ErrBadRequest)
	}
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.direct.CompleteToTargetActivity(ctx, vo); err != nil {
		return R{}, err
	}
	return ROkMsg("直送成功", nil), nil
}

// DoResetProcess 重置流程实例（清空运行态回到发起节点）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoResetProcess(ctx context.Context, processInstanceID, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.reset.Reset(ctx, processInstanceID, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("重置成功", nil), nil
}

// ─── P2.C 软终止/挂起/重激活/删除门面 ─────────────────────────────

// DoStopProcessInstance 软终止流程实例（按实例 ID，写终态留痕）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param operator 操作人身份 Id
// @param message 终止意见
// @return 统一返回
// @return 错误
func (s *Service) DoStopProcessInstance(ctx context.Context, processInstanceID, operator, message string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.terminate.StopByInstance(ctx, processInstanceID, operator, message); err != nil {
		return R{}, err
	}
	return ROkMsg("终止成功", nil), nil
}

// DoStopTaskByTaskID 按任务软终止其所在流程实例。
// @param ctx 上下文
// @param taskID 任务 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoStopTaskByTaskID(ctx context.Context, taskID, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.terminate.StopByTask(ctx, taskID, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("终止成功", nil), nil
}

// DoDeleteProcessInstance 删除流程实例（运行态删除 + 历史留痕）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param operator 操作人身份 Id
// @param reason 删除原因
// @return 统一返回
// @return 错误
func (s *Service) DoDeleteProcessInstance(ctx context.Context, processInstanceID, operator, reason string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.terminate.DeleteInstance(ctx, processInstanceID, operator, reason); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// DoDeleteProcessByBusinessID 按业务单据删除其流程实例。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param businessKey 业务单据 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoDeleteProcessByBusinessID(ctx context.Context, tenantID, businessKey, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.terminate.DeleteByBusinessID(ctx, tenantID, businessKey, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// DoSuspendProcessInstance 挂起流程实例。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoSuspendProcessInstance(ctx context.Context, processInstanceID, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.terminate.Suspend(ctx, processInstanceID, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("挂起成功", nil), nil
}

// DoActivateProcessInstance 恢复挂起的流程实例。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoActivateProcessInstance(ctx context.Context, processInstanceID, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.terminate.Activate(ctx, processInstanceID, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("激活成功", nil), nil
}

// DoReactivateProcess 重激活已结束实例到指定节点（重激活生命周期）。
// @param ctx 上下文
// @param historicProcessInstanceID 历史实例 ID
// @param targetActivityID 重激活目标节点 ID
// @param operator 操作人身份 Id
// @return 统一返回（data 含新实例 ID）
// @return 错误
func (s *Service) DoReactivateProcess(ctx context.Context, historicProcessInstanceID, targetActivityID, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	// 重激活为原实例复活（历史行复位、实例 ID 不变），回传原 ID 供前端定位
	if err := s.comp.terminate.Reactivate(ctx, historicProcessInstanceID, targetActivityID, operator); err != nil {
		return R{}, err
	}
	return ROkMsg("重激活成功", map[string]any{"processInstanceId": historicProcessInstanceID}), nil
}

// ─── P3.A 加签减签/会签门面 ─────────────────────────────────

// AddExecution 加签：为当前任务追加办理人（前加签/后加签）。
// @param ctx 上下文
// @param req 加签请求
// @return 统一返回（data 为加签结果）
// @return 错误
func (s *Service) AddExecution(ctx context.Context, req AddSignRequest) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	result, err := s.comp.addSign.AddSign(ctx, req)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("加签成功", result), nil
}

// DelExecution 减签：移除被加签人子任务并按需恢复父任务。
// @param ctx 上下文
// @param req 减签请求
// @return 统一返回（data 为减签结果）
// @return 错误
func (s *Service) DelExecution(ctx context.Context, req DelSignRequest) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	result, err := s.comp.addSign.DelSign(ctx, req)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("减签成功", result), nil
}

// AddMultiInstanceMember 向多实例（会签）活动动态追加一个实例成员。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param activityID 多实例活动 ID
// @param assignee 新成员身份 Id
// @return 统一返回
// @return 错误
func (s *Service) AddMultiInstanceMember(ctx context.Context, processInstanceID, activityID, assignee string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.addSign.AddMultiInstanceMember(ctx, processInstanceID, activityID, assignee); err != nil {
		return R{}, err
	}
	return ROkMsg("加签成功", nil), nil
}

// RemoveMultiInstanceMember 移除多实例（会签）活动的一个实例成员。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param activityID 多实例活动 ID
// @param assignee 成员身份 Id
// @param executionID 被移除实例的执行 ID
// @param completed 移除时是否视作已完成
// @return 统一返回
// @return 错误
func (s *Service) RemoveMultiInstanceMember(ctx context.Context, processInstanceID, activityID, assignee, executionID string, completed bool) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.addSign.RemoveMultiInstanceMember(ctx, processInstanceID, activityID, assignee, executionID, completed); err != nil {
		return R{}, err
	}
	return ROkMsg("减签成功", nil), nil
}

// CompleteSign 会签比例结论：按已办比例判定会签是否通过。
// @param ctx 上下文
// @param taskID 会签任务 ID
// @param countersignResult 会签结论：0 否决 / 1 通过
// @return 统一返回（data 为是否达成比例）
// @return 错误
func (s *Service) CompleteSign(ctx context.Context, taskID string, countersignResult int) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	passed, err := s.comp.counter.CountersignPercentResult(ctx, taskID, countersignResult)
	if err != nil {
		return R{}, err
	}
	return ROk(passed), nil
}

// ─── P5.B 下一步推演门面 ────────────────────────────────────

// GetNextUserTaskAndUserList 推演当前任务之后的节点与候选处理人。
// @param ctx 上下文
// @param taskID 任务 ID
// @return 统一返回（data 为节点与用户视图列表）
// @return 错误
func (s *Service) GetNextUserTaskAndUserList(ctx context.Context, taskID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	views, err := s.comp.simulate.SimulateByTask(ctx, taskID, nil)
	if err != nil {
		return R{}, err
	}
	return ROk(views), nil
}

// GetNextUserTaskAndUserListByKey 未发起实例按流程定义推演（不含办结伪节点）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 流程定义 Key（取最新版本）
// @param businessKey 业务单据 Id（预留透传）
// @param vars 表单/条件变量
// @return 统一返回（data 为节点与用户视图列表）
// @return 错误
func (s *Service) GetNextUserTaskAndUserListByKey(ctx context.Context, tenantID, processDefinitionKey, businessKey string, vars map[string]any) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	views, err := s.comp.simulate.SimulateByKey(ctx, tenantID, processDefinitionKey, businessKey, vars)
	if err != nil {
		return R{}, err
	}
	return ROk(views), nil
}

// ─── P4 委托规则门面 ────────────────────────────────────────

// DoTaskOwnerAdjustment 按节点调整运行中任务的处理人（遍历实例下该节点任务逐个改写）。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @param taskKey 目标节点 Key
// @param fromOwner 原处理人，空表示不限
// @param toOwner 新处理人
// @param operator 操作人身份 Id
// @return 统一返回（data 含调整任务数）
// @return 错误
func (s *Service) DoTaskOwnerAdjustment(ctx context.Context, processInstanceID, taskKey, fromOwner, toOwner, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	// 实例下任务逐个比对节点 Key 与原处理人（空 fromOwner 不限）
	page, err := s.engine.TaskService().ListTasks(ctx, api.TaskFilter{ProcessInstanceID: processInstanceID, Size: 100})
	if err != nil {
		return R{}, err
	}
	adjusted := 0
	for _, task := range page.Data {
		if task.TaskDefinitionKey != taskKey {
			continue
		}
		if fromOwner != "" && task.Assignee != fromOwner {
			continue
		}
		keep := false
		if err := s.comp.delegate.AdjustTaskOwner(ctx, AdjustOwnerRequest{
			TaskID: task.ID, CurrentOwnerID: task.Assignee, UserCode: operator,
			OwnerIDs: []string{toOwner}, KeepCurrentTask: &keep,
		}); err != nil {
			return R{}, err
		}
		adjusted++
	}
	return ROkMsg("调整成功", map[string]any{"adjusted": adjusted}), nil
}

// DoTaskOwnerAdjustmentByID 按任务 ID 调整处理人。
// @param ctx 上下文
// @param taskID 任务 ID
// @param toOwner 新处理人
// @param operator 操作人身份 Id
// @return 统一返回
// @return 错误
func (s *Service) DoTaskOwnerAdjustmentByID(ctx context.Context, taskID, toOwner, operator string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	task, err := s.engine.TaskService().GetTask(ctx, taskID)
	if err != nil {
		return R{}, api.NewError("WORKFLOW-0404", "任务不存在", err)
	}
	keep := false
	if err := s.comp.delegate.AdjustTaskOwner(ctx, AdjustOwnerRequest{
		TaskID: taskID, CurrentOwnerID: task.Assignee, UserCode: operator,
		OwnerIDs: []string{toOwner}, KeepCurrentTask: &keep,
	}); err != nil {
		return R{}, err
	}
	return ROkMsg("调整成功", nil), nil
}

// AddDelegateTask 新增委托规则。
// @param ctx 上下文
// @param rule 委托规则（Create 时 ID 可为空由存储生成）
// @return 统一返回（data 为规则 Id）
// @return 错误
func (s *Service) AddDelegateTask(ctx context.Context, rule *DelegateRule) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.delegate.Add(ctx, rule); err != nil {
		return R{}, err
	}
	return ROkMsg("委托成功", map[string]any{"id": rule.ID}), nil
}

// EditDelegateTask 更新委托规则。
// @param ctx 上下文
// @param rule 委托规则（ID 必填）
// @return 统一返回
// @return 错误
func (s *Service) EditDelegateTask(ctx context.Context, rule *DelegateRule) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.delegate.Update(ctx, rule); err != nil {
		return R{}, err
	}
	return ROkMsg("更新成功", nil), nil
}

// DeleteDelegateTask 删除委托规则。
// @param ctx 上下文
// @param id 规则 ID
// @return 统一返回
// @return 错误
func (s *Service) DeleteDelegateTask(ctx context.Context, id string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.delegate.Remove(ctx, id); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// ListDelegateTask 分页查询委托规则列表。
// @param ctx 上下文
// @param ownerID 委托人身份 Id
// @param tenantID 租户（业务系统代号）
// @return 统一返回（data 为规则列表）
// @return 错误
func (s *Service) ListDelegateTask(ctx context.Context, ownerID, tenantID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	rules, err := s.comp.delegate.List(ctx, ownerID, tenantID)
	if err != nil {
		return R{}, err
	}
	return ROk(rules), nil
}

// EnableDelegateTask 启用委托规则。
// @param ctx 上下文
// @param id 规则 ID
// @return 统一返回
// @return 错误
func (s *Service) EnableDelegateTask(ctx context.Context, id string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.delegate.Enable(ctx, id); err != nil {
		return R{}, err
	}
	return ROkMsg("启用成功", nil), nil
}

// DisableDelegateTask 停用委托规则。
// @param ctx 上下文
// @param id 规则 ID
// @return 统一返回
// @return 错误
func (s *Service) DisableDelegateTask(ctx context.Context, id string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.delegate.Disable(ctx, id); err != nil {
		return R{}, err
	}
	return ROkMsg("停用成功", nil), nil
}

// RetireDelegateTask 注销委托规则（不可再启用）。
// @param ctx 上下文
// @param id 规则 ID
// @return 统一返回
// @return 错误
func (s *Service) RetireDelegateTask(ctx context.Context, id string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.delegate.Retire(ctx, id); err != nil {
		return R{}, err
	}
	return ROkMsg("注销成功", nil), nil
}

// ─── P4 催办/异常门面 ───────────────────────────────────────

// AddRemind 新增催办配置。
// @param ctx 上下文
// @param req 催办保存请求
// @return 统一返回（data 为催办配置）
// @return 错误
func (s *Service) AddRemind(ctx context.Context, req RemindSaveRequest) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	remind, err := s.comp.remind.Add(ctx, req)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("保存成功", remind), nil
}

// EditRemind 更新催办配置。
// @param ctx 上下文
// @param req 催办保存请求（ID 必填）
// @return 统一返回（data 为催办配置）
// @return 错误
func (s *Service) EditRemind(ctx context.Context, req RemindSaveRequest) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	remind, err := s.comp.remind.Update(ctx, req)
	if err != nil {
		return R{}, err
	}
	return ROkMsg("更新成功", remind), nil
}

// DeleteRemind 删除催办配置。
// @param ctx 上下文
// @param id 催办配置 ID
// @return 统一返回
// @return 错误
func (s *Service) DeleteRemind(ctx context.Context, id string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.remind.Remove(ctx, id); err != nil {
		return R{}, err
	}
	return ROkMsg("删除成功", nil), nil
}

// GetRemind 查询单条催办配置。
// @param ctx 上下文
// @param id 催办配置 ID
// @return 统一返回（data 为催办配置，未命中 data 为 null）
// @return 错误
func (s *Service) GetRemind(ctx context.Context, id string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	remind, found, err := s.comp.remind.Get(ctx, id)
	if err != nil {
		return R{}, err
	}
	if !found {
		return ROk(nil), nil
	}
	return ROk(remind), nil
}

// GetRemindPageList 分页查询催办配置（可按流程模型过滤）。
// @param ctx 上下文
// @param modelID 流程模型 ID（空表示全部）
// @param query 分页参数
// @return 统一返回（data 为分页结果）
// @return 错误
func (s *Service) GetRemindPageList(ctx context.Context, modelID string, query PageQuery) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	page, err := s.comp.remind.List(ctx, modelID, query)
	if err != nil {
		return R{}, err
	}
	return ROk(page), nil
}

// ListTaskExceptions 按实例查询未处理任务异常。
// @param ctx 上下文
// @param processInstanceID 流程实例 ID
// @return 统一返回（data 为异常列表）
// @return 错误
func (s *Service) ListTaskExceptions(ctx context.Context, processInstanceID string) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	exceptions, err := s.comp.exception.ListByInstance(ctx, processInstanceID)
	if err != nil {
		return R{}, err
	}
	return ROk(exceptions), nil
}

// HandleTaskExceptions 批量标记异常处理状态。
// @param ctx 上下文
// @param ids 异常 ID 列表
// @param handled true 置为已处理 / false 回退未处理
// @return 统一返回
// @return 错误
func (s *Service) HandleTaskExceptions(ctx context.Context, ids []string, handled bool) (R, error) {
	if err := requireCapable(s.comp); err != nil {
		return R{}, err
	}
	if err := s.comp.exception.Handle(ctx, ids, handled); err != nil {
		return R{}, err
	}
	return ROkMsg("处理成功", nil), nil
}

// ─── 调度器生命周期 ─────────────────────────────────────────

// StartSchedulers 启动后台调度（自动办结/超时提醒/催办扫描/僵尸监控，按配置开关生效）。
// @param ctx 上下文（取消后调度循环以取消态执行 tick）
func (s *Service) StartSchedulers(ctx context.Context) {
	if s.comp != nil && s.comp.scheduler != nil {
		s.comp.scheduler.Start(ctx)
	}
}

// StopSchedulers 优雅停止后台调度。
func (s *Service) StopSchedulers() {
	if s.comp != nil && s.comp.scheduler != nil {
		s.comp.scheduler.Stop()
	}
}
