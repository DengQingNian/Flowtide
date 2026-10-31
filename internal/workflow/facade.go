package workflow

import (
	"context"
	"database/sql"
	"errors"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// ROk 构造成功返回（api 包构造函数的本地短名）。
// @param data 业务数据
// @return 成功 R
func ROk(data any) R { return api.ROk(data) }

// ROkMsg 构造带消息的成功返回。
// @param msg 成功消息
// @param data 业务数据
// @return 成功 R
func ROkMsg(msg string, data any) R { return api.ROkMsg(msg, data) }

// engineInstanceQuery 将引擎运行时与历史服务组合为 InstanceQuery 窄接口
// （运行优先、历史回退），供操作记录/表单状态等服务在实例结束后仍能取到归属信息。
type engineInstanceQuery struct {
	runtime api.RuntimeService // 运行时服务
	history api.HistoryService // 历史服务
}

// GetProcessInstance 查询运行中实例，已结束时回退历史实例语义由调用方处理。
// @param ctx 上下文
// @param id 实例 ID
// @return 实例
// @return 查询错误
func (a engineInstanceQuery) GetProcessInstance(ctx context.Context, id string) (api.ProcessInstance, error) {
	return a.runtime.GetProcessInstance(ctx, id)
}

// GetHistoricProcessInstance 查询历史实例（含已结束）。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @return 历史实例
// @return 查询错误
func (a engineInstanceQuery) GetHistoricProcessInstance(ctx context.Context, processInstanceID string) (api.HistoricProcessInstance, error) {
	return a.history.GetHistoricProcessInstance(ctx, processInstanceID)
}

// components 聚合门面内部子服务（域服务 + 独立能力），由 assembleComponents 装配。
type components struct {
	next       NextActivityStore       // 下一步预生成-消耗存储
	message    MessageExtendStore      // 待办扩展存储
	history    HistoryExtendStore      // 已办扩展存储
	operation  OperationRecordStore    // 操作记录存储
	document   DocumentStore           // 业务表单状态存储
	completeTo CompleteToActivityStore // 直送记录存储
	meta       *ModelMetaService       // 模型元数据
	nextSvc    *NextActivityService    // 预生成-消耗服务
	comments   *CommentService         // 意见服务
	operations *OperationRecordService // 操作记录服务
	messages   *MessageExtendService   // 待办扩展服务
	histories  *HistoryExtendService   // 已办扩展服务
	documents  *DocumentService        // 表单状态服务
	start      *StartService           // 发起服务
	complete   *CompleteService        // 办结服务
	query      *QueryService           // 查询服务
	rewrite    *ExecutorRewriteHook    // 执行人重写钩子
	// ── P2/P3/P4/P5 能力子服务（assembleCapabilities 装配）──
	stepsStore StepStore            // 步骤规则存储（his_step 快照，nil 则规则分支跳过）
	steps      *StepRuleService     // 步骤规则服务
	back       *BackService         // 退回服务（任意退回/上一节点）
	direct     *DirectService       // 直送服务
	reset      *ResetService        // 重置服务
	rollback   *RollbackService     // 撤回服务
	terminate  *TerminateService    // 软终止/挂起/重激活/删除服务
	addSign    *AddSignService      // 加签减签服务
	counter    *CountersignService  // 会签服务
	delegate   *DelegateRuleService // 委托规则服务
	remind     *RemindService       // 催办服务
	exception  *ExceptionService    // 任务异常服务
	simulate   *SimulateService     // 下一步推演器
	scheduler  *Scheduler           // 调度器（自动办结/超时提醒/催办扫描/僵尸监控）
	modelMgmt  *ModelService        // 模型/分类管理（P5.A）
	stepDesign *StepDesignService   // 设计时步骤管理（P5.A）
	template   *TemplateService     // 设计器模板管理（P5.A）
	snapshot   *DeploySnapshotService // 部署快照服务（P5.A）
	review     *ReviewTaskService   // 审阅影子任务服务（P5.B）
	highlight  *HighlightService    // 流程图高亮数据服务（P5.B）
	extendsStore ActExtendStore     // 审阅人绑定存储直引（宿主/测试播种绑定用）
}

// assembleComponents 依据存储实现装配全部门面子服务。
// @param engine 引擎门面
// @param next 下一步存储
// @param message 待办存储
// @param history 已办存储
// @param operation 操作记录存储
// @param document 表单存储
// @param completeTo 直送记录存储
// @param c 时钟
// @return 子服务聚合
func assembleComponents(engine api.Engine, next NextActivityStore, message MessageExtendStore, history HistoryExtendStore, operation OperationRecordStore, document DocumentStore, completeTo CompleteToActivityStore, c clock.Clock) *components {
	// 引擎缺省（协议兼容场景）仅装配存储与空子服务：501 桩方法不触碰子服务，
	// 已实现方法在 nil 引擎下不可用（由门面调用方保证引擎存在）
	if engine == nil {
		return &components{next: next, message: message, history: history, operation: operation, document: document, completeTo: completeTo}
	}
	meta := NewModelMetaService(engine)
	nextSvc := NewNextActivityService(next, c)
	comments := NewCommentService(engine.TaskService(), engine.HistoryService(), engine.IdentityService())
	instanceQuery := engineInstanceQuery{runtime: engine.RuntimeService(), history: engine.HistoryService()}
	operations := NewOperationRecordService(operation, engine.HistoryService(), instanceQuery, engine.RepositoryService(), engine.IdentityService(), c)
	messages := NewMessageExtendService(message, c)
	histories := NewHistoryExtendService(history, c)
	documents := NewDocumentService(document, message, operation, instanceQuery, engine.HistoryService(), c)
	start := NewStartService(engine, meta, nextSvc, comments, operations, messages, histories, documents)
	complete := NewCompleteService(engine, meta, nextSvc, comments, operations, messages, histories, documents, completeTo, engine.IdentityService(), c)
	query := NewQueryService(engine, messages, histories, operations, documents)
	rewrite := NewExecutorRewriteHook(meta, nextSvc)
	return &components{
		next: next, message: message, history: history, operation: operation, document: document, completeTo: completeTo,
		meta: meta, nextSvc: nextSvc, comments: comments, operations: operations,
		messages: messages, histories: histories, documents: documents,
		start: start, complete: complete, query: query, rewrite: rewrite,
	}
}

// assembleMemoryComponents 装配内存存储（无 DB 嵌入模式与单测默认）。
// @param engine 引擎门面
// @param opts 服务选项（调度开关透传，nil 值字段用缺省）
// @param c 时钟
// @return 子服务聚合
func assembleMemoryComponents(engine api.Engine, opts Options, c clock.Clock) *components {
	stores := NewMemStores(c)
	completeTo := NewMemCompleteToActivityStore(c)
	comp := assembleComponents(engine, stores.Next, stores.Message, stores.History, stores.Operation, stores.Document, completeTo, c)
	// P2-C/P3/P4/P5 能力装配：内存存储族 + 引擎适配
	// hisSteps 同时满足 StepStore 语义（超集），同一实例兼供推演规则读取与快照写入
	hisSteps := NewMemHisStepStore()
	assembleCapabilities(comp, engine, capabilityStores{
		steps:         hisSteps,
		delegate:      NewMemTaskDelegateStore(c),
		mirrors:       NewMemDelegateTaskMirrorStore(c),
		exceptions:    NewMemTaskExceptionStore(c),
		reminds:       NewMemRemindStore(c),
		remindTargets: NewMemRemindTargetStore(c),
		todos:         NewMemTodoSource(stores.Message),
		models:        NewMemModelMgmtStore(c),
		designs:       NewMemStepDesignStore(c),
		templates:     NewMemTemplateStore(c),
		hisSteps:      hisSteps,
		reviews:       NewMemReviewTaskStore(c),
		extends:       NewMemActExtendStore(),
	}, opts, c)
	return comp
}

// assembleSQLComponents 装配 SQL 存储（真实库模式）。
// @param engine 引擎门面
// @param store workflow 仓储入口（internal/persistence/workflow）
// @param db 独立数据库连接（可为 nil，届时使用 store 自带连接）
// @param opts 服务选项（调度开关透传）
// @param c 时钟
// @return 子服务聚合
// @return 错误
func assembleSQLComponents(engine api.Engine, store *pwf.Store, db *sql.DB, opts Options, c clock.Clock) (*components, error) {
	if store == nil {
		return nil, api.NewError("WORKFLOW-0600", "SQL 存储入口不能为空", nil)
	}
	var set *SQLStoreSet
	if db != nil {
		set = NewSQLStoresWithDB(store, db)
	} else {
		set = NewSQLStores(store)
	}
	completeTo := NewSQLCompleteToActivityStore(store)
	comp := assembleComponents(engine, set.Next, set.Message, set.History, set.Operation, set.Document, completeTo, c)
	// P2-C/P3/P4/P5 能力装配：SQL 适配器族（待办数据源暂缺裸 SQL 实现，传空源）
	mgmt := NewSQLModelMgmtStoresWithDB(store, db)
	reviewStores := NewSQLReviewStoresWithDB(store, db)
	assembleCapabilities(comp, engine, capabilityStores{
		steps:         mgmt.HisStep,
		delegate:      NewSQLTaskDelegateStore(store),
		mirrors:       NewSQLDelegateTaskMirrorStore(store),
		exceptions:    NewSQLTaskExceptionStore(store),
		reminds:       NewSQLRemindStore(store, c),
		remindTargets: NewSQLRemindTargetStore(store),
		todos:         emptyTodoSource{},
		models:        mgmt.Model,
		designs:       mgmt.Step,
		templates:     mgmt.Template,
		hisSteps:      mgmt.HisStep,
		reviews:       reviewStores.Tasks,
		extends:       reviewStores.Extends,
	}, opts, c)
	return comp, nil
}

// RegisterExecutorRewriteHook 注册执行人重写全局钩子（TASK_CREATED）。
// 部署新定义后应调用 RewriteWarmupAll 预热元数据缓存（锁外）。
// @param ctx 上下文（用于注册期预热，可取消）
// @return 预热错误（注册本身不失败）
func (s *Service) RegisterExecutorRewriteHook(ctx context.Context) error {
	s.comp.rewrite.Register(s.engine)
	// 启动期全量预热已部署定义的模型元数据，避免首单触发 Defer 预热延迟
	return s.comp.rewrite.WarmupAll(ctx)
}

// startProcessInternal 发起流程的门面内部实现（两个发起入口共用）。
// @param ctx 上下文
// @param vo 发起请求
// @return 统一返回
// @return 错误
func (s *Service) startProcessInternal(ctx context.Context, vo *StartProcessInstanceVo) (R, error) {
	if vo == nil {
		return R{}, api.NewError("WORKFLOW-0400", "发起请求不能为空", api.ErrBadRequest)
	}
	result, err := s.comp.start.Start(ctx, vo)
	if err != nil {
		return R{}, err
	}
	return ROk(map[string]any{
		"processInstanceId":   result.ProcessInstanceID,
		"processDefinitionId": result.ProcessDefinitionID,
		"taskId":              result.TaskID,
		"taskName":            result.TaskName,
		"taskKey":             result.TaskKey,
	}), nil
}

// saveAgreeNextInternal 预存"同意"方向下一步的门面内部实现。
// 请求含多节点列表，逐节点预存；目标为保留代号 end 的节点跳过（协议约定）。
// @param ctx 上下文
// @param req 预存请求
// @return 统一返回
// @return 错误
func (s *Service) saveAgreeNextInternal(ctx context.Context, req *NextActivityRequest) (R, error) {
	if req == nil || req.TaskID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "预存请求与任务 ID 不能为空", api.ErrBadRequest)
	}
	if len(req.NextActivityList) == 0 {
		return R{}, api.NewError("WORKFLOW-0400", "下一步节点列表不能为空", api.ErrBadRequest)
	}
	task, err := s.engine.TaskService().GetTask(ctx, req.TaskID)
	if err != nil {
		return R{}, err
	}
	for _, node := range req.NextActivityList {
		if node == nil || node.ActivityID == "" {
			return R{}, api.NewError("WORKFLOW-0400", "下一步节点 ID 不能为空", api.ErrBadRequest)
		}
		// 保留代号 end 表示流程结束，跳过预存（与 Java saveAgreeNextActivity 一致）
		if IsEndStep(node.ActivityID) {
			continue
		}
		// 处理人为空时同样跳过（无执行者的节点无需预生成，执行人重写将走模型兜底）
		if len(node.ExecutorIDs) == 0 {
			continue
		}
		err := s.comp.nextSvc.SaveAgreeNext(ctx, NextActivitySaveRequest{
			ProcessInstanceID: task.ProcessInstanceID,
			ActivityID:        node.ActivityID,
			IdentityIDs:       node.ExecutorIDs,
			PreTaskID:         req.TaskID,
			SendIdentityID:    req.SendIdentityID,
			SendUserName:      req.SendUserName,
			TenantID:          task.TenantID,
			Creator:           req.SendIdentityID,
		})
		if err != nil {
			return R{}, err
		}
	}
	return ROkMsg("下一步信息已保存", nil), nil
}

// isProcessDeployedInternal 判断定义是否已部署（按租户上下文解析最新定义）。
// @param ctx 上下文
// @param tenantID 租户（业务系统代号）
// @param processDefinitionKey 定义 Key
// @return 统一返回（data 为 bool）
// @return 错误
func (s *Service) isProcessDeployedInternal(ctx context.Context, tenantID, processDefinitionKey string) (R, error) {
	if processDefinitionKey == "" {
		return R{}, api.NewError("WORKFLOW-0400", "流程定义 Key 不能为空", api.ErrBadRequest)
	}
	// 引擎按上下文租户解析定义，统一注入请求租户
	deployed := true
	if _, err := s.engine.RepositoryService().GetProcessDefinitionByKey(tenant.With(ctx, tenantID), processDefinitionKey); err != nil {
		// 未部署（不存在）与查询失败区分：仅"不存在"视为未部署
		if errors.Is(err, api.ErrNotFound) {
			deployed = false
		} else {
			return R{}, err
		}
	}
	return ROk(deployed), nil
}

// isExistRunningProcessInternal 判断业务键下是否存在运行中实例。
// @param ctx 上下文
// @param tenantID 租户
// @param processDefinitionKey 定义 Key
// @param businessKey 业务单据 ID
// @return 统一返回（data 为 bool）
// @return 错误
func (s *Service) isExistRunningProcessInternal(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error) {
	if businessKey == "" {
		return R{}, api.NewError("WORKFLOW-0400", "业务单据 ID 不能为空", api.ErrBadRequest)
	}
	page, err := s.engine.RuntimeService().QueryProcessInstances(tenant.With(ctx, tenantID), api.ProcessInstanceFilter{
		ProcessDefinitionKey: processDefinitionKey,
		BusinessKey:          businessKey,
		Size:                 1,
	})
	if err != nil {
		return R{}, err
	}
	return ROk(len(page.Data) > 0), nil
}

// workflowStatusInternal 查询业务单据的流程状态（表单状态机中文值）。
// @param ctx 上下文
// @param tenantID 租户
// @param processDefinitionKey 定义 Key（未用，保留协议位）
// @param businessKey 业务单据 ID
// @return 统一返回（data 为状态对象）
// @return 错误
func (s *Service) workflowStatusInternal(ctx context.Context, tenantID, processDefinitionKey, businessKey string) (R, error) {
	document, found, err := s.comp.documents.GetByBusinessID(ctx, businessKey, tenantID)
	if err != nil {
		return R{}, err
	}
	if !found {
		return R{}, api.NewError("WORKFLOW-0404", "业务单据未关联流程: "+businessKey, api.ErrNotFound)
	}
	return ROk(map[string]any{
		"flowInstanceId":      document.FlowInstanceID,
		"flowStatus":          document.FlowStatus,
		"flowCurrentStepName": document.FlowCurrentStepName,
		"flowCurrentPersons":  document.FlowCurrentPersons,
		"flowStartTime":       document.FlowStartTime,
		"flowFinishTime":      document.FlowFinishTime,
		"flowTerminationTime": document.FlowTerminationTime,
	}), nil
}

// updateDocumentInternal 按实例或任务回写表单状态（刷新状态机）。
// @param ctx 上下文
// @param processInstanceID 实例 ID
// @param taskID 任务 ID
// @return 统一返回
// @return 错误
func (s *Service) updateDocumentInternal(ctx context.Context, processInstanceID, taskID string) (R, error) {
	if processInstanceID == "" && taskID == "" {
		return R{}, api.NewError("WORKFLOW-0400", "实例 ID 与任务 ID 至少一个非空", api.ErrBadRequest)
	}
	if err := s.comp.documents.UpdateByProcessInstanceOrTask(ctx, processInstanceID, taskID); err != nil {
		return R{}, err
	}
	return ROkMsg("表单状态已更新", nil), nil
}
