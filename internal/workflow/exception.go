// exception.go 提供任务异常记录域服务（P4.6，对位 Java ProcessesExceptionAspect 与
// workflow_task_exception 表）：
// 四类异常判定（BusinessException / FlowableException / RuntimeException / NotTaskMessageException，
// 按 Java ProcessesExceptionAspect:109-125 分类语义）、消息截断 1000、处理/忽略流转、
// 进程内原子计数（对位 Java Redis incr）与回调失败记录适配器（P1-C CallbackClient 的
// ExceptionRecorder 端口实现）。
package workflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// 异常类型常量（对位 Java WorkflowTaskExceptionEntity.EXCEPTION_TYPE_*，docs 002 §3.12）。
const (
	// ExceptionTypeBusiness 业务异常（Java BusinessException 等价）。
	ExceptionTypeBusiness = "BusinessException"
	// ExceptionTypeFlowable 工作流异常（Java FlowableException 等价）。
	ExceptionTypeFlowable = "FlowableException"
	// ExceptionTypeRuntime 其他运行时异常（Java RuntimeException 等价兜底）。
	ExceptionTypeRuntime = "RuntimeException"
	// ExceptionTypeNotTaskMessage 未生成待办异常（僵尸流程监控专用）。
	ExceptionTypeNotTaskMessage = "NotTaskMessageException"
)

// 异常处理状态常量（Go 侧新增 handle_status 列，docs 003 §1 索引）。
const (
	// ExceptionUnhandled 未处理。
	ExceptionUnhandled = 0
	// ExceptionHandled 已处理（处理与忽略共用已处理终态）。
	ExceptionHandled = 1
)

// exceptionMessageLimit 是异常消息的落库截断长度（Java :131 message.length() > 1000 → 截 997 + "..."）。
const exceptionMessageLimit = 1000

// TaskException 是 workflow_task_exception（任务异常）的领域实体。
type TaskException struct {
	BaseColumns              // 公共审计列
	TaskID              string // 任务 Id
	ProcessInstanceID   string // 流程实例 Id
	ProcessInstanceName string // 流程实例名称
	ClassName           string // 异常发生类名
	MethodName          string // 异常发生方法名
	Message             string // 异常消息（>1000 截断）
	Params              string // 方法入参 JSON 串
	ExceptionType       string // 异常类型（四类常量）
	ExceptionClass      string // 异常类全名
	ProcessDefinitionID   string // 流程定义 Id
	ProcessDefinitionKey  string // 流程定义 Key
	ProcessDefinitionName string // 流程定义名称
	StartUserID          string // 发起人
	HandleStatus         int    // 处理状态：0 未处理、1 已处理
}

// ─── 存储端口 ─────────────────────────────────────────────

// TaskExceptionStore 是任务异常的存储端口。
type TaskExceptionStore interface {
	// Save 插入一条异常记录（id 由调用方生成）。
	// @param ctx 操作上下文
	// @param e 异常记录
	// @return 错误
	Save(ctx context.Context, e *TaskException) error
	// Get 按主键查询异常记录。
	// @param ctx 操作上下文
	// @param id 异常记录 Id
	// @return 异常记录
	// @return 是否存在
	// @return 错误
	Get(ctx context.Context, id string) (*TaskException, bool, error)
	// ListUnhandled 查询未处理异常（handle_status=0，管理台重发与僵尸监控数据源）。
	// @param ctx 操作上下文
	// @return 记录列表
	// @return 错误
	ListUnhandled(ctx context.Context) ([]*TaskException, error)
	// ListByInstance 查询实例的异常记录。
	// @param ctx 操作上下文
	// @param processInstanceID 流程实例 Id
	// @return 记录列表
	// @return 错误
	ListByInstance(ctx context.Context, processInstanceID string) ([]*TaskException, error)
	// ListByTask 查询任务的异常记录。
	// @param ctx 操作上下文
	// @param taskID 任务 Id
	// @return 记录列表
	// @return 错误
	ListByTask(ctx context.Context, taskID string) ([]*TaskException, error)
	// ListByTypeAndInstances 查询指定实例集合下指定类型的异常记录（僵尸监控去重判据）。
	// @param ctx 操作上下文
	// @param exceptionType 异常类型
	// @param instanceIDs 实例 Id 集合
	// @return 记录列表
	// @return 错误
	ListByTypeAndInstances(ctx context.Context, exceptionType string, instanceIDs []string) ([]*TaskException, error)
	// MarkHandled 批量更新处理状态（处理/忽略 → handle_status）。
	// @param ctx 操作上下文
	// @param ids 异常记录 Id 列表
	// @param status 处理状态（ExceptionUnhandled/ExceptionHandled）
	// @return 错误
	MarkHandled(ctx context.Context, ids []string, status int) error
}

// ─── 内存实现 ─────────────────────────────────────────────

// MemTaskExceptionStore 是任务异常端口的内存实现（线程安全 map + 行克隆存取）。
type MemTaskExceptionStore struct {
	mu    sync.RWMutex            // 读写锁
	rows  map[string]*TaskException // id → 记录
	clock clock.Clock             // 时间源
	newID func() string           // ID 生成函数
}

// NewMemTaskExceptionStore 创建内存任务异常存储。
// @param c 时间源，nil 时回退真实时钟
// @return 内存存储
func NewMemTaskExceptionStore(c clock.Clock) *MemTaskExceptionStore {
	if c == nil {
		c = clock.Real{}
	}
	return &MemTaskExceptionStore{rows: make(map[string]*TaskException), clock: c, newID: DefaultIDGen()}
}

// Save 插入一条异常记录（ID 缺失自动生成，审计列按当前时间填充）。
// @param ctx 操作上下文
// @param e 异常记录
// @return 错误
func (m *MemTaskExceptionStore) Save(ctx context.Context, e *TaskException) error {
	// 异常类型与消息是记录的最小语义信息
	if e == nil || e.ExceptionType == "" || e.Message == "" {
		return apiError("WORKFLOW-1901", "异常记录缺少类型/消息")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	fillBase(&e.BaseColumns, m.newID, m.clock.Now, "", e.TenantID)
	m.rows[e.ID] = cloneTaskException(e)
	return nil
}

// Get 按主键查询异常记录。
// @param ctx 操作上下文
// @param id 异常记录 Id
// @return 异常记录
// @return 是否存在
// @return 错误
func (m *MemTaskExceptionStore) Get(ctx context.Context, id string) (*TaskException, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row, ok := m.rows[id]
	if !ok {
		return nil, false, nil
	}
	return cloneTaskException(row), true, nil
}

// listLocked 按谓词过滤并按创建时间升序返回（调用方须持锁）。
// @param match 过滤谓词
// @return 记录列表
func (m *MemTaskExceptionStore) listLocked(match func(*TaskException) bool) []*TaskException {
	out := make([]*TaskException, 0, 4)
	for _, row := range m.rows {
		if match(row) {
			out = append(out, cloneTaskException(row))
		}
	}
	sortTaskExceptions(out)
	return out
}

// ListUnhandled 查询未处理异常。
// @param ctx 操作上下文
// @return 记录列表
// @return 错误
func (m *MemTaskExceptionStore) ListUnhandled(ctx context.Context) ([]*TaskException, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(e *TaskException) bool { return e.HandleStatus == ExceptionUnhandled }), nil
}

// ListByInstance 查询实例的异常记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (m *MemTaskExceptionStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*TaskException, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(e *TaskException) bool { return e.ProcessInstanceID == processInstanceID }), nil
}

// ListByTask 查询任务的异常记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (m *MemTaskExceptionStore) ListByTask(ctx context.Context, taskID string) ([]*TaskException, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(e *TaskException) bool { return e.TaskID == taskID }), nil
}

// ListByTypeAndInstances 查询指定实例集合下指定类型的异常记录。
// @param ctx 操作上下文
// @param exceptionType 异常类型
// @param instanceIDs 实例 Id 集合
// @return 记录列表
// @return 错误
func (m *MemTaskExceptionStore) ListByTypeAndInstances(ctx context.Context, exceptionType string, instanceIDs []string) ([]*TaskException, error) {
	// 实例集合建索引后线性扫描
	marked := make(map[string]bool, len(instanceIDs))
	for _, id := range instanceIDs {
		marked[id] = true
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked(func(e *TaskException) bool {
		return e.ExceptionType == exceptionType && marked[e.ProcessInstanceID]
	}), nil
}

// MarkHandled 批量更新处理状态。
// @param ctx 操作上下文
// @param ids 异常记录 Id 列表
// @param status 处理状态
// @return 错误
func (m *MemTaskExceptionStore) MarkHandled(ctx context.Context, ids []string, status int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if row, ok := m.rows[id]; ok {
			row.HandleStatus = status
			row.UpdateTime = m.clock.Now()
		}
	}
	return nil
}

// 编译期断言：内存实现满足端口。
var _ TaskExceptionStore = (*MemTaskExceptionStore)(nil)

// cloneTaskException 复制异常记录。
// @param e 原记录
// @return 副本
func cloneTaskException(e *TaskException) *TaskException {
	if e == nil {
		return nil
	}
	copied := *e
	return &copied
}

// sortTaskExceptions 按创建时间升序排序记录。
// @param rows 记录列表
func sortTaskExceptions(rows []*TaskException) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].CreateTime.Before(rows[j-1].CreateTime); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

// ─── SQL 适配 ─────────────────────────────────────────────

// SQLTaskExceptionStore 是任务异常端口的 SQL 适配器（组合 pwf 仓储）。
type SQLTaskExceptionStore struct {
	repos *pwf.Repositories // workflow 行仓储聚合
}

// NewSQLTaskExceptionStore 创建任务异常 SQL 适配器。
// @param store workflow 仓储入口
// @return SQL 适配器
func NewSQLTaskExceptionStore(store *pwf.Store) *SQLTaskExceptionStore {
	return &SQLTaskExceptionStore{repos: pwf.NewRepositories(store)}
}

// Save 插入一条异常记录。
// @param ctx 操作上下文
// @param e 异常记录
// @return 错误
func (s *SQLTaskExceptionStore) Save(ctx context.Context, e *TaskException) error {
	return s.repos.TaskExceptions.Insert(ctx, taskExceptionToRow(e))
}

// Get 按主键查询异常记录。
// @param ctx 操作上下文
// @param id 异常记录 Id
// @return 异常记录
// @return 是否存在
// @return 错误
func (s *SQLTaskExceptionStore) Get(ctx context.Context, id string) (*TaskException, bool, error) {
	row, found, err := s.repos.TaskExceptions.GetByPK(ctx, id)
	if err != nil || !found {
		return nil, false, err
	}
	return taskExceptionFromRow(row), true, nil
}

// ListUnhandled 查询未处理异常。
// @param ctx 操作上下文
// @return 记录列表
// @return 错误
func (s *SQLTaskExceptionStore) ListUnhandled(ctx context.Context) ([]*TaskException, error) {
	rows, err := s.repos.TaskExceptions.ListUnhandled(ctx)
	return taskExceptionsFromRows(rows), err
}

// ListByInstance 查询实例的异常记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *SQLTaskExceptionStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*TaskException, error) {
	rows, err := s.repos.TaskExceptions.ListByProcessInstance(ctx, processInstanceID)
	return taskExceptionsFromRows(rows), err
}

// ListByTask 查询任务的异常记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *SQLTaskExceptionStore) ListByTask(ctx context.Context, taskID string) ([]*TaskException, error) {
	rows, err := s.repos.TaskExceptions.ListByTask(ctx, taskID)
	return taskExceptionsFromRows(rows), err
}

// ListByTypeAndInstances 查询指定实例集合下指定类型的异常记录
// （行仓储无组合查询，逐实例取指定类型记录后聚合）。
// @param ctx 操作上下文
// @param exceptionType 异常类型
// @param instanceIDs 实例 Id 集合
// @return 记录列表
// @return 错误
func (s *SQLTaskExceptionStore) ListByTypeAndInstances(ctx context.Context, exceptionType string, instanceIDs []string) ([]*TaskException, error) {
	out := make([]*TaskException, 0, 4)
	for _, instanceID := range instanceIDs {
		rows, err := s.repos.TaskExceptions.ListByProcessInstance(ctx, instanceID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			// 类型过滤在内存补齐（实例维度已下推）
			if rowString(row, "exception_type") == exceptionType {
				out = append(out, taskExceptionFromRow(row))
			}
		}
	}
	return out, nil
}

// MarkHandled 批量更新处理状态。
// @param ctx 操作上下文
// @param ids 异常记录 Id 列表
// @param status 处理状态
// @return 错误
func (s *SQLTaskExceptionStore) MarkHandled(ctx context.Context, ids []string, status int) error {
	for _, id := range ids {
		// 已处理走仓储专用方法（handle_status=1），其余经通用更新
		if status == ExceptionHandled {
			if _, err := s.repos.TaskExceptions.MarkHandled(ctx, id); err != nil {
				return err
			}
			continue
		}
		if _, err := s.repos.TaskExceptions.UpdateByPK(ctx, id, pwf.Row{"handle_status": status}); err != nil {
			return err
		}
	}
	return nil
}

// 编译期断言：SQL 适配满足端口。
var _ TaskExceptionStore = (*SQLTaskExceptionStore)(nil)

// taskExceptionToRow 实体转通用行。
// @param e 异常记录
// @return 通用行
func taskExceptionToRow(e *TaskException) pwf.Row {
	row := pwf.Row{
		"id":                      e.ID,
		"task_id":                 e.TaskID,
		"process_instance_id":     e.ProcessInstanceID,
		"process_instance_name":   e.ProcessInstanceName,
		"class_name":              e.ClassName,
		"method_name":             e.MethodName,
		"message":                 e.Message,
		"params":                  e.Params,
		"exception_type":          e.ExceptionType,
		"exception_class":         e.ExceptionClass,
		"process_definition_id":   e.ProcessDefinitionID,
		"process_definition_key":  e.ProcessDefinitionKey,
		"process_definition_name": e.ProcessDefinitionName,
		"start_user_id":           e.StartUserID,
		"handle_status":           e.HandleStatus,
	}
	appendBaseRow(row, e.BaseColumns)
	return row
}

// taskExceptionFromRow 通用行转实体。
// @param row 通用行
// @return 异常记录
func taskExceptionFromRow(row pwf.Row) *TaskException {
	return &TaskException{
		BaseColumns:           baseFromRow(row),
		TaskID:                rowString(row, "task_id"),
		ProcessInstanceID:     rowString(row, "process_instance_id"),
		ProcessInstanceName:   rowString(row, "process_instance_name"),
		ClassName:             rowString(row, "class_name"),
		MethodName:            rowString(row, "method_name"),
		Message:               rowString(row, "message"),
		Params:                rowString(row, "params"),
		ExceptionType:         rowString(row, "exception_type"),
		ExceptionClass:        rowString(row, "exception_class"),
		ProcessDefinitionID:   rowString(row, "process_definition_id"),
		ProcessDefinitionKey:  rowString(row, "process_definition_key"),
		ProcessDefinitionName: rowString(row, "process_definition_name"),
		StartUserID:           rowString(row, "start_user_id"),
		HandleStatus:          rowInt(row, "handle_status"),
	}
}

// taskExceptionsFromRows 行列表转实体列表。
// @param rows 通用行列表
// @return 记录列表
func taskExceptionsFromRows(rows []pwf.Row) []*TaskException {
	out := make([]*TaskException, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskExceptionFromRow(row))
	}
	return out
}

// ─── 域服务 ─────────────────────────────────────────────

// ExceptionRequest 是异常记录请求（对位 Java ProcessesExceptionAspect 切面入参投影）。
type ExceptionRequest struct {
	TaskID                string // 任务 Id
	ProcessInstanceID     string // 流程实例 Id
	ProcessInstanceName   string // 流程实例名称
	ClassName             string // 发生异常的类名
	MethodName            string // 发生异常的方法名
	Message               string // 异常消息（>1000 截断）
	Params                string // 入参 JSON 串
	ExceptionType         string // 异常类型（显式指定优先于 Err 推断）
	ExceptionClass        string // 异常类名
	ProcessDefinitionID   string // 流程定义 Id
	ProcessDefinitionKey  string // 流程定义 Key
	ProcessDefinitionName string // 流程定义名称
	StartUserID           string // 发起人
	TenantID              string // 租户
	Err                   error  // 原始错误（ExceptionType 为空时按四类判定推断）
}

// ExceptionService 是异常记录域服务：四类判定、消息截断、处理流转与原子计数。
type ExceptionService struct {
	store TaskExceptionStore // 存储端口
	clock clock.Clock        // 时间源
	newID func() string      // ID 生成函数
	count atomic.Int64       // 进程内异常总计数（对位 Java Redis incr）
}

// NewExceptionService 创建异常记录域服务。
// @param store 存储端口
// @param c 时间源，nil 时回退真实时钟
// @return 域服务
func NewExceptionService(store TaskExceptionStore, c clock.Clock) *ExceptionService {
	if c == nil {
		c = clock.Real{}
	}
	return &ExceptionService{store: store, clock: c, newID: DefaultIDGen()}
}

// Record 记录一条异常（四类判定 → 消息截断 → 落库 → 计数 +1，对位 Java ProcessesExceptionAspect
// doAfterThrowingAdvice:61）。判定优先级：显式 ExceptionType > Err 推断（NotTaskMessage 除外，
// 该类型由僵尸监控显式指定）。
// @param ctx 操作上下文
// @param req 异常记录请求
// @return 落库后的异常记录
// @return 错误
func (s *ExceptionService) Record(ctx context.Context, req *ExceptionRequest) (*TaskException, error) {
	if req == nil {
		return nil, apiError("WORKFLOW-1902", "异常记录请求为空")
	}
	exceptionType := req.ExceptionType
	// 显式类型缺失时按错误推断四类（Java instanceof 链语义）
	if exceptionType == "" {
		exceptionType = ClassifyException(req.Err)
	}
	// 异常消息截断（Java :131：>1000 截 997 + "..."；空消息回退异常类名）
	message := truncateExceptionMessage(req.Message, req.Err)
	entity := &TaskException{
		BaseColumns:           BaseColumns{TenantID: req.TenantID},
		TaskID:                req.TaskID,
		ProcessInstanceID:     req.ProcessInstanceID,
		ProcessInstanceName:   req.ProcessInstanceName,
		ClassName:             req.ClassName,
		MethodName:            req.MethodName,
		Message:               message,
		Params:                req.Params,
		ExceptionType:         exceptionType,
		ExceptionClass:        req.ExceptionClass,
		ProcessDefinitionID:   req.ProcessDefinitionID,
		ProcessDefinitionKey:  req.ProcessDefinitionKey,
		ProcessDefinitionName: req.ProcessDefinitionName,
		StartUserID:           req.StartUserID,
		HandleStatus:          ExceptionUnhandled,
	}
	if err := s.store.Save(ctx, entity); err != nil {
		return nil, err
	}
	// 落库成功后计数 +1（Java Redis incr 在 save 成功分支）
	s.count.Add(1)
	return entity, nil
}

// ClassifyException 按错误值判定异常类型（对位 Java ProcessesExceptionAspect:109-125 的
// instanceof 链）：api.Error 的请求参数类错误视作业务异常；错误链含 flowable 标记或引擎
// 内部错误码视作工作流异常；其余为运行时异常。
// @param err 原始错误
// @return 异常类型常量
func ClassifyException(err error) string {
	return classifyException(err)
}

// classifyException 是 ClassifyException 的实现。
// @param err 原始错误
// @return 异常类型常量
func classifyException(err error) string {
	// 空错误按运行时异常兜底（Java 非 Flowable/Business → RuntimeException）
	if err == nil {
		return ExceptionTypeRuntime
	}
	// 业务异常：api.Error 且状态为请求参数非法（Java BusinessException 的等价语义）
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		// 业务校验类错误 → BusinessException
		if errors.Is(err, api.ErrBadRequest) || errors.Is(apiErr.Cause, api.ErrBadRequest) {
			return ExceptionTypeBusiness
		}
		// 引擎域错误码（ENGINE-*/WORKFLOW-* 之外的非参数错误）→ FlowableException
		if isEngineError(apiErr) {
			return ExceptionTypeFlowable
		}
		return ExceptionTypeBusiness
	}
	// 工作流异常：错误链文本含 flowable 标记（Java ex instanceof FlowableException 等价）
	if strings.Contains(strings.ToLower(err.Error()), "flowable") {
		return ExceptionTypeFlowable
	}
	return ExceptionTypeRuntime
}

// isEngineError 判断是否引擎域错误码（ENGINE- 前缀，工作流异常等价类）。
// @param e api 错误
// @return 是否引擎错误
func isEngineError(e *api.Error) bool {
	return strings.HasPrefix(e.Code, "ENGINE-")
}

// truncateExceptionMessage 截断异常消息（Java :131 语义：>1000 截 997 补 "..."，
// 空消息回退异常类名，再为空回退固定占位）。
// @param message 原始消息
// @param err 原始错误
// @return 截断后的消息
func truncateExceptionMessage(message string, err error) string {
	// 消息为空时回退异常类名（Java ex.getClass().getName()）
	if strings.TrimSpace(message) == "" {
		if err != nil {
			message = err.Error()
		}
	}
	// 仍为空回退固定占位，保证落库非空（存储校验要求）
	if strings.TrimSpace(message) == "" {
		return ExceptionTypeRuntime
	}
	// 超长截断补省略号
	if len(message) > exceptionMessageLimit {
		return message[:exceptionMessageLimit-3] + "..."
	}
	return message
}

// ListUnhandled 查询未处理异常。
// @param ctx 操作上下文
// @return 记录列表
// @return 错误
func (s *ExceptionService) ListUnhandled(ctx context.Context) ([]*TaskException, error) {
	return s.store.ListUnhandled(ctx)
}

// ListByInstance 查询实例的异常记录。
// @param ctx 操作上下文
// @param processInstanceID 流程实例 Id
// @return 记录列表
// @return 错误
func (s *ExceptionService) ListByInstance(ctx context.Context, processInstanceID string) ([]*TaskException, error) {
	return s.store.ListByInstance(ctx, processInstanceID)
}

// ListByTask 查询任务的异常记录。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return 记录列表
// @return 错误
func (s *ExceptionService) ListByTask(ctx context.Context, taskID string) ([]*TaskException, error) {
	return s.store.ListByTask(ctx, taskID)
}

// Handle 批量处理异常（处理与忽略均为 handle_status=1 的终态，动作区分由调用方操作留痕；
// 存储端口保留 status 参数供管理台复位等扩展场景）。
// @param ctx 操作上下文
// @param ids 异常记录 Id 列表
// @param handled true 处理、false 忽略（两者同置已处理终态）
// @return 错误
func (s *ExceptionService) Handle(ctx context.Context, ids []string, handled bool) error {
	// 空列表无事可做
	if len(ids) == 0 {
		return nil
	}
	// 处理与忽略同为终态（handle_status 仅 0/1 两态）
	return s.store.MarkHandled(ctx, ids, ExceptionHandled)
}

// Count 返回进程内异常累计计数（对位 Java Redis incr，重启清零）。
// @return 计数值
func (s *ExceptionService) Count() int64 {
	return s.count.Load()
}

// RecordCallbackFailure 记录一次回调失败（实现 callback.go 的 ExceptionRecorder 端口：
// P1-C CallbackClient 重试耗尽后落 workflow_task_exception）。
// @param ctx 操作上下文
// @param exception 回调异常
// @return 记录错误
func (s *ExceptionService) RecordCallbackFailure(ctx context.Context, exception *CallbackException) error {
	// 回调异常为空直接跳过
	if exception == nil {
		return nil
	}
	message := ""
	if exception.Cause != nil {
		message = exception.Cause.Error()
	}
	_, err := s.Record(ctx, &ExceptionRequest{
		MethodName:    "postWithRetry",
		Message:       message,
		Params:        exception.Payload,
		ExceptionType: ExceptionTypeRuntime,
		ExceptionClass: "CallbackException",
		TenantID:      exception.TenantID,
	})
	return err
}

// CallbackExceptionRecorder 是异常服务到回调异常记录器端口的适配器
// （P1-C CallbackClient 的 ExceptionRecorder 实现，装配：WithCallbackRecorder(NewCallbackExceptionRecorder(svc))）。
type CallbackExceptionRecorder struct {
	service *ExceptionService // 异常域服务
}

// NewCallbackExceptionRecorder 创建回调异常记录适配器。
// @param service 异常域服务
// @return 记录适配器
func NewCallbackExceptionRecorder(service *ExceptionService) *CallbackExceptionRecorder {
	return &CallbackExceptionRecorder{service: service}
}

// RecordCallbackException 记录一次回调失败（委托异常服务落库）。
// @param ctx 操作上下文
// @param exception 回调异常
// @return 记录错误
func (r *CallbackExceptionRecorder) RecordCallbackException(ctx context.Context, exception *CallbackException) error {
	// 服务缺失时静默跳过（适配器不放大回调失败）
	if r.service == nil {
		return nil
	}
	return r.service.RecordCallbackFailure(ctx, exception)
}

// 编译期断言：适配器满足回调异常记录器端口。
var _ ExceptionRecorder = (*CallbackExceptionRecorder)(nil)

// RecordZombieInstance 记录"流程未生成待办"异常（NotTaskMessageException，对位 Java
// ExceptionMonitorSchedule#doSaveflowTaskException:104；调度器 ZombieScanTick 使用）。
// @param ctx 操作上下文
// @param instance 流程实例
// @return 已记录的异常（实例已有同类记录时返回 nil 表示跳过）
// @return 错误
func (s *ExceptionService) RecordZombieInstance(ctx context.Context, instance api.ProcessInstance) (*TaskException, error) {
	return s.recordZombieInstance(ctx, instance)
}

// recordZombieInstance 是 RecordZombieInstance 的实现。
// @param ctx 操作上下文
// @param instance 流程实例
// @return 已记录的异常（重复记录返回 nil）
// @return 错误
func (s *ExceptionService) recordZombieInstance(ctx context.Context, instance api.ProcessInstance) (*TaskException, error) {
	existing, err := s.store.ListByTypeAndInstances(ctx, ExceptionTypeNotTaskMessage, []string{instance.ID})
	if err != nil {
		return nil, err
	}
	// 已有同类记录时不重复落库（Java anyMatch 跳过语义）
	for _, item := range existing {
		if item.ProcessInstanceID == instance.ID {
			return nil, nil
		}
	}
	return s.Record(ctx, &ExceptionRequest{
		ProcessInstanceID:     instance.ID,
		ProcessInstanceName:   instance.Name,
		Message:               "流程未生成待办",
		ExceptionType:         ExceptionTypeNotTaskMessage,
		ExceptionClass:        ExceptionTypeNotTaskMessage,
		ProcessDefinitionID:   instance.ProcessDefinitionID,
		ProcessDefinitionKey:  instance.ProcessDefinitionKey,
		ProcessDefinitionName: instance.ProcessDefinitionKey,
		StartUserID:           instance.StartUserID,
		TenantID:              instance.TenantID,
	})
}
