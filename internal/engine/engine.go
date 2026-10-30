// Package engine 实现 BPMN 流程引擎的运行时。
package engine

import (
	"context"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
)

// Version 是当前引擎版本。
const Version = "0.1.0"

// 支持的历史级别。
const (
	HistoryNone     = "none"
	HistoryInstance = "instance"
	HistoryTask     = "task"
	HistoryActivity = "activity"
	HistoryAudit    = "audit"
	HistoryFull     = "full"
)

// Config 是引擎配置。
type Config struct {
	HistoryLevel   string                  // 历史级别，默认 audit
	Clock          clock.Clock             // 时钟，默认真实时钟
	IDs            idgen.Generator         // ID 生成器，默认 UUIDv7
	MaxAgendaSteps int                     // 单命令最大议程步数，默认 10000
	TenantID       string                  // 默认租户
	StatePath      string                  // 状态文件路径，为空时仅使用内存
	StateStore     statestore.Store        // 状态存储，优先于 StatePath
	RowRepository  *flowablerow.Repository // 按表行仓储，可选
	LoadRows       bool                    // 启动时从行仓储恢复
	LoadTenant     string                  // 启动恢复租户
	// RecordSequenceFlowHistory 在迁移续跑时按 Java audit 语义记录顺序流历史。
	RecordSequenceFlowHistory bool
}

// Delegate 是服务任务委托。
type Delegate func(ctx context.Context, execution *Execution) error

// Execution 是暴露给委托与执行监听器的执行上下文。
type Execution struct {
	engine     *Engine         // 引擎
	state      *executionState // 执行状态
	instance   *instanceState  // 流程实例
	activityID string          // 活动 ID 覆盖值，用于活动结束监听器
}

// GetVariable 按作用域链读取变量。
// @param name 变量名
// @return 变量值
// @return 是否存在
func (e *Execution) GetVariable(name string) (any, bool) {
	return e.engine.getVariable(e.state, name)
}

// SetVariable 设置变量。
// @param name 变量名
// @param value 变量值
func (e *Execution) SetVariable(name string, value any) {
	e.engine.setVariable(e.state, name, value)
}

// SetVariableLocal 设置当前作用域变量。
// @param name 变量名
// @param value 变量值
func (e *Execution) SetVariableLocal(name string, value any) {
	e.engine.setVariableLocal(e.state, name, value)
}

// ProcessInstanceID 返回流程实例 ID。
// @return 实例 ID
func (e *Execution) ProcessInstanceID() string { return e.state.ProcessInstanceID }

// ActivityID 返回当前活动 ID。
// @return 活动 ID
func (e *Execution) ActivityID() string {
	// 活动结束监听器的执行可能已离开当前节点，优先使用显式覆盖值
	if e.activityID != "" {
		return e.activityID
	}
	return e.state.ActivityID
}

// Engine 是流程引擎实现。
type Engine struct {
	mu                 sync.RWMutex                 // 保护全部状态
	cfg                Config                       // 配置
	state              *state                       // 可变状态
	delegates          map[string]Delegate          // 服务任务委托
	taskListeners      map[string]api.TaskListener  // 任务监听器
	executionListeners map[string]ExecutionListener // 执行监听器
	taskCreatedHooks   []api.TaskHook               // 任务创建全局钩子，按注册顺序执行
	taskCompletedHooks []api.TaskHook               // 任务完成全局钩子，按注册顺序执行
	closed             bool                         // 是否关闭
	rowRepository      *flowablerow.Repository      // 行仓储
	lastRows           []flowablerow.Row            // 上次持久化行
	suppressPersist    bool                         // 导入期间暂停持久化
	rowSource          bool                         // 行仓储是否为唯一状态源
}

// commandContext 是单条命令的上下文。
type commandContext struct {
	engine           *Engine                           // 引擎
	ctx              context.Context                   // 调用方上下文
	now              time.Time                         // 命令内统一时间
	agenda           *agenda                           // 议程
	listenerErr      error                             // 执行监听器错误，命令结束统一回滚
	transactionOrder int                               // 命令内历史活动写入顺序
	deferred         []func(ctx context.Context) error // 命令提交成功后锁外顺序执行的延迟动作
}

// nextTransactionOrder 返回当前命令内的下一个活动事务顺序，与 Java audit 记录一致。
// @return 从一开始递增的顺序号
func (cc *commandContext) nextTransactionOrder() int {
	cc.transactionOrder++
	return cc.transactionOrder
}

// New 创建流程引擎。
// @param cfg 配置
// @return 引擎
// @return 创建错误
func New(cfg Config) (*Engine, error) {
	// 快照与行仓储必须只有一个权威来源，避免任一介质失败后形成分叉。
	if cfg.RowRepository != nil && (cfg.StateStore != nil || cfg.StatePath != "") {
		return nil, api.NewError("ENGINE-0916", "快照存储与行仓储不能同时配置", api.ErrBadRequest)
	}
	if cfg.RowRepository != nil && !cfg.RowRepository.SupportsTransactions() {
		return nil, api.NewError("ENGINE-0923", "权威行仓储必须支持事务", api.ErrBadRequest)
	}
	if cfg.LoadRows && cfg.RowRepository == nil {
		return nil, api.NewError("ENGINE-0916", "加载行状态必须配置行仓储", api.ErrBadRequest)
	}

	if cfg.Clock == nil {
		cfg.Clock = clock.Real{}
	}
	if cfg.IDs == nil {
		cfg.IDs = idgen.NewUUIDv7()
	}
	if cfg.HistoryLevel == "" {
		cfg.HistoryLevel = HistoryAudit
	}
	if cfg.MaxAgendaSteps <= 0 {
		cfg.MaxAgendaSteps = 10000
	}
	if !validHistoryLevel(cfg.HistoryLevel) {
		return nil, api.NewError("ENGINE-0001", "非法历史级别: "+cfg.HistoryLevel, api.ErrBadRequest)
	}
	if cfg.StateStore == nil && cfg.StatePath != "" {
		cfg.StateStore = statestore.NewFile(cfg.StatePath)
	}
	engine := &Engine{
		cfg:                cfg,
		state:              newState(),
		delegates:          make(map[string]Delegate),
		taskListeners:      make(map[string]api.TaskListener),
		executionListeners: make(map[string]ExecutionListener),
		rowRepository:      cfg.RowRepository,
	}
	// 转换进程显式启用 Java 顺序流历史；从已准备状态恢复时以状态内标记为准。
	engine.state.RecordSequenceFlowHistory = cfg.RecordSequenceFlowHistory
	if cfg.LoadRows && cfg.RowRepository != nil {
		engine.rowSource = true
		rows, err := loadRowsForTenant(cfg, context.Background())
		if err != nil {
			return nil, err
		}
		engine.lastRows = copyValue(rows).([]flowablerow.Row)
		if len(rows) > 0 {
			engine.suppressPersist = true
			if err := engine.RestoreRows(context.Background(), rows); err != nil {
				return nil, err
			}
			engine.suppressPersist = false
		}
	} else if err := engine.loadState(context.Background()); err != nil {
		return nil, err
	}
	return engine, nil
}

// RegisterDelegate 注册服务任务委托。
// @param name 委托名
// @param delegate 委托函数
func (e *Engine) RegisterDelegate(name string, delegate Delegate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.delegates[name] = delegate
}

// RegisterTaskListener 注册 BPMN 任务监听器。
// @param name 监听器名（class/delegateExpression）
// @param listener 监听器
func (e *Engine) RegisterTaskListener(name string, listener api.TaskListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.taskListeners[name] = listener
}

// RegisterExecutionListener 注册 BPMN 执行监听器。
// @param name 监听器名（class/delegateExpression）
// @param listener 监听器
func (e *Engine) RegisterExecutionListener(name string, listener ExecutionListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executionListeners[name] = listener
}

// RepositoryService 返回仓库服务。
// @return 仓库服务
func (e *Engine) RepositoryService() api.RepositoryService { return e }

// RuntimeService 返回运行时服务。
// @return 运行时服务
func (e *Engine) RuntimeService() api.RuntimeService { return e }

// TaskService 返回任务服务。
// @return 任务服务
func (e *Engine) TaskService() api.TaskService { return e }

// HistoryService 返回历史服务。
// @return 历史服务
func (e *Engine) HistoryService() api.HistoryService { return e }

// ManagementService 返回管理服务。
// @return 管理服务
func (e *Engine) ManagementService() api.ManagementService { return e }

// Close 关闭引擎。
// @param ctx 关闭上下文
// @return 关闭错误
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.saveState(ctx); err != nil {
		return err
	}
	if err := e.persistRows(ctx); err != nil {
		return err
	}
	e.closed = true
	return nil
}

// EngineInfo 返回引擎信息。
// @param ctx 上下文
// @return 引擎信息
// @return 查询错误
func (e *Engine) EngineInfo(ctx context.Context) (map[string]any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return map[string]any{
		"name":         "flowtide",
		"version":      Version,
		"historyLevel": e.cfg.HistoryLevel,
		"closed":       e.closed,
	}, nil
}

// execute 在锁与事务语义中执行命令，命令成功提交后在锁外顺序执行延迟动作。
// @param ctx 上下文
// @param fn 命令函数
// @return 命令结果
// @return 执行错误
func (e *Engine) execute(ctx context.Context, fn func(*commandContext) (any, error)) (any, error) {
	result, err, deferred := e.executeLocked(ctx, fn)
	// 命令已成功落盘，延迟动作在锁释放后执行，动作内可安全调用引擎公有 API
	e.runDeferredActions(ctx, deferred)
	return result, err
}

// executeLocked 在持锁状态下执行命令本体，并返回命令登记的延迟动作。
// @param ctx 上下文
// @param fn 命令函数
// @return 命令结果
// @return 执行错误
// @return 延迟动作列表，命令失败时为空
func (e *Engine) executeLocked(ctx context.Context, fn func(*commandContext) (any, error)) (any, error, []func(ctx context.Context) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, api.NewError("ENGINE-0002", "引擎已关闭", api.ErrEngineClosed), nil
	}
	if owner, ok := e.cfg.StateStore.(interface{ CheckOwner(context.Context) error }); ok {
		if err := owner.CheckOwner(ctx); err != nil {
			return nil, api.NewError("ENGINE-0922", "存储独占权校验失败", err), nil
		}
	}
	backup := cloneState(e.state)
	cc := &commandContext{
		engine: e,
		ctx:    ctx,
		now:    e.cfg.Clock.Now(),
		agenda: newAgenda(e.cfg.MaxAgendaSteps),
	}
	result, err := fn(cc)
	// 执行监听器失败可能发生在无法直接返回错误的钩子中，统一在命令边界上报
	if err == nil {
		err = cc.listenerErr
	}
	if err != nil {
		// 任一操作失败，回滚到命令开始前的状态
		e.state = backup
		return nil, err, nil
	}
	// 瞬态变量仅在命令内可见，落盘前统一清理
	for _, execution := range e.state.Executions {
		execution.Transient = nil
	}
	// 命令成功后落盘；落盘失败同样回滚，保证内存与磁盘一致
	if err := e.saveState(ctx); err != nil {
		e.state = backup
		return nil, err, nil
	}
	if !e.suppressPersist {
		if err := e.persistRows(ctx); err != nil {
			e.state = backup
			return nil, err, nil
		}
	}
	return copyValue(result), nil, cc.deferred
}

// read 在只读锁中执行查询。
// @param fn 查询函数
// @return 查询结果
// @return 查询错误
func (e *Engine) read(fn func() (any, error)) (any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return nil, api.NewError("ENGINE-0002", "引擎已关闭", api.ErrEngineClosed)
	}
	result, err := fn()
	return copyValue(result), err
}

// findDefinition 按 ID 查找流程定义。
// @param id 流程定义 ID
// @return 定义状态
// @return 是否存在
func (e *Engine) findDefinition(id string) (*definitionState, bool) {
	definition, ok := e.state.Definitions[id]
	return definition, ok
}

// findLatestDefinition 按 Key 查找最新流程定义。
// @param key 流程定义 Key
// @return 定义状态
// @return 是否存在
func (e *Engine) findLatestDefinition(key string) (*definitionState, bool) {
	var latest *definitionState
	for _, definition := range e.state.Definitions {
		if definition.Info.Key != key {
			continue
		}
		if latest == nil || definition.Info.Version > latest.Info.Version {
			latest = definition
		}
	}
	return latest, latest != nil
}

// findInstance 查找流程实例。
// @param id 实例 ID
// @return 实例状态
// @return 是否存在
func (e *Engine) findInstance(id string) (*instanceState, bool) {
	instance, ok := e.state.Instances[id]
	return instance, ok
}

// findExecution 查找执行。
// @param id 执行 ID
// @return 执行状态
// @return 是否存在
func (e *Engine) findExecution(id string) (*executionState, bool) {
	execution, ok := e.state.Executions[id]
	return execution, ok
}

// findTask 查找任务。
// @param id 任务 ID
// @return 任务状态
// @return 是否存在
func (e *Engine) findTask(id string) (*taskState, bool) {
	task, ok := e.state.Tasks[id]
	return task, ok
}

// rootExecution 返回实例的根执行。
// @param instance 实例状态
// @return 根执行
// @return 是否存在
func (e *Engine) rootExecution(instance *instanceState) (*executionState, bool) {
	return e.findExecution(instance.RootExecutionID)
}

// getVariable 沿执行树查找变量。
// @param execution 执行
// @param name 变量名
// @return 变量值
// @return 是否存在
func (e *Engine) getVariable(execution *executionState, name string) (any, bool) {
	current := execution
	for current != nil {
		if value, ok := current.Variables[name]; ok {
			return value, true
		}
		if current.ParentID == "" {
			return nil, false
		}
		parent, ok := e.findExecution(current.ParentID)
		if !ok {
			return nil, false
		}
		current = parent
	}
	return nil, false
}

// setVariable 写入最近的已存在变量作用域，否则写入根执行。
// @param execution 执行
// @param name 变量名
// @param value 变量值
func (e *Engine) setVariable(execution *executionState, name string, value any) {
	current := execution
	for current != nil {
		if _, ok := current.Variables[name]; ok {
			current.Variables[name] = copyValue(value)
			e.recordHistoricVariable(current, name, value)
			return
		}
		if current.ParentID == "" {
			if current.Variables == nil {
				current.Variables = make(map[string]any)
			}
			current.Variables[name] = copyValue(value)
			e.recordHistoricVariable(current, name, value)
			return
		}
		parent, ok := e.findExecution(current.ParentID)
		if !ok {
			break
		}
		current = parent
	}
	if execution.Variables == nil {
		execution.Variables = make(map[string]any)
	}
	execution.Variables[name] = copyValue(value)
	e.recordHistoricVariable(execution, name, value)
}

// setVariableLocal 写入当前作用域变量。
// @param execution 执行
// @param name 变量名
// @param value 变量值
func (e *Engine) setVariableLocal(execution *executionState, name string, value any) {
	if execution.Variables == nil {
		execution.Variables = make(map[string]any)
	}
	execution.Variables[name] = copyValue(value)
	e.recordHistoricVariable(execution, name, value)
}

// variablesOf 返回执行可见变量快照。
// @param execution 执行
// @return 变量快照
func (e *Engine) variablesOf(execution *executionState) map[string]any {
	out := make(map[string]any)
	chain := make([]*executionState, 0)
	current := execution
	for current != nil {
		chain = append(chain, current)
		if current.ParentID == "" {
			break
		}
		parent, ok := e.findExecution(current.ParentID)
		if !ok {
			break
		}
		current = parent
	}
	// 从根向叶子覆盖，叶子优先；瞬态变量在命令内覆盖持久变量
	for i := len(chain) - 1; i >= 0; i-- {
		for key, value := range chain[i].Variables {
			out[key] = value
		}
		for key, value := range chain[i].Transient {
			out[key] = value
		}
	}
	return out
}

// validHistoryLevel 校验历史级别。
// @param level 历史级别
// @return 是否合法
func validHistoryLevel(level string) bool {
	switch level {
	case HistoryNone, HistoryInstance, HistoryTask, HistoryActivity, HistoryAudit, HistoryFull:
		return true
	default:
		return false
	}
}

// historyAtLeast 判断历史级别是否达到指定级别。
// @param level 目标级别
// @return 是否达到
func (e *Engine) historyAtLeast(level string) bool {
	order := map[string]int{
		HistoryNone: 0, HistoryInstance: 1, HistoryTask: 2, HistoryActivity: 3, HistoryAudit: 4, HistoryFull: 5,
	}
	return order[e.cfg.HistoryLevel] >= order[level]
}

// newID 生成新 ID。
// @return ID
func (e *Engine) newID() string { return e.cfg.IDs.NewID() }

// tenantOf 返回请求租户或默认租户。
// @param tenant 请求租户
// @return 实际租户
func (e *Engine) tenantOf(tenant string) string {
	if tenant != "" {
		return tenant
	}
	return e.cfg.TenantID
}

// loadRowsForTenant 按配置租户加载行。
// @param cfg 配置
// @param ctx 上下文
// @return 行列表
// @return 加载错误
func loadRowsForTenant(cfg Config, ctx context.Context) ([]flowablerow.Row, error) {
	if cfg.LoadTenant != "" {
		return cfg.RowRepository.LoadAllByTenant(ctx, cfg.LoadTenant)
	}
	return cfg.RowRepository.LoadAll(ctx)
}
