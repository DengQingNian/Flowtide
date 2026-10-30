// callback.go 提供业务回调 client（P6.2 核心提前交付）：
// 报文协议与 Java BusinessCallTaskListener / BusinessCallExecutionListener 逐字兼容，
// 按租户查业务系统注册表取回调地址，地址为空或注册表查无时静默跳过（与 Java 一致），
// 失败按 D5 指数退避重试 3 次，重试仍失败记录日志并可插拔异常记录器（P4 接 workflow_task_exception）。
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// TaskCallback 是任务级业务回调报文（对位 Java BusinessCallTaskListener paramMap，JSON 字段名逐字兼容）。
type TaskCallback struct {
	ProcessDefinitionID string `json:"processDefinitionId"` // 流程定义 ID
	ProcessInstanceID   string `json:"processInstanceId"`   // 流程实例 ID
	BusinessKey         string `json:"businessKey"`         // 业务单据 ID
	TenantID            string `json:"tenantId"`            // 租户（= 系统代号）
	TaskID              string `json:"taskId"`              // 任务 ID（执行级报文无此字段）
	TaskName            string `json:"taskName"`            // 任务名（执行级报文无此字段）
	FlowElementID       string `json:"flowElementId"`       // 流程节点 ID
	FlowElementName     string `json:"flowElementName"`     // 流程节点名
	Params              string `json:"params"`              // BPMN 扩展属性串 "k:v;k:v"（Java 报文即字符串）
}

// ExecutionCallback 是执行级业务回调报文（对位 Java BusinessCallExecutionListener paramMap，无 taskId/taskName）。
type ExecutionCallback struct {
	ProcessDefinitionID string `json:"processDefinitionId"` // 流程定义 ID
	ProcessInstanceID   string `json:"processInstanceId"`   // 流程实例 ID
	BusinessKey         string `json:"businessKey"`         // 业务单据 ID
	TenantID            string `json:"tenantId"`            // 租户（= 系统代号）
	FlowElementID       string `json:"flowElementId"`       // 流程节点 ID
	FlowElementName     string `json:"flowElementName"`     // 流程节点名
	Params              string `json:"params"`              // BPMN 扩展属性串 "k:v;k:v"
}

// 回调类别常量（异常记录与日志区分任务级/执行级）。
const (
	// CallbackKindTask 任务级回调（BusinessCallTaskListener 语义）。
	CallbackKindTask = "task"
	// CallbackKindExecution 执行级回调（BusinessCallExecutionListener 语义）。
	CallbackKindExecution = "execution"
)

// 回调 client 错误码。
const (
	errCodeCallbackPost   = "WORKFLOW-2101" // 回调请求失败（含重试耗尽）
	errCodeCallbackStatus = "WORKFLOW-2102" // 回调返回非 2xx
)

// ParseParams 解析 "k:v;k:v" 格式的 BPMN 扩展属性串。
// 无冒号的分段跳过（Java 版会数组越界，此处容错）；值中再含冒号时保留全部剩余内容（比 Java split[1] 更忠实）。
// @param params 原始串（如 "name:张三;age:1;sex:男"）
// @return 键值映射（空串返回空映射）
func ParseParams(params string) map[string]string {
	out := make(map[string]string)
	trimmed := strings.TrimSpace(params)
	if trimmed == "" {
		return out
	}
	for _, segment := range strings.Split(trimmed, ";") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		// 仅按第一个冒号切分，值内冒号保留
		pair := strings.SplitN(segment, ":", 2)
		// 缺冒号或键为空的分段无法构成键值，跳过
		if len(pair) < 2 || strings.TrimSpace(pair[0]) == "" {
			continue
		}
		out[pair[0]] = pair[1]
	}
	return out
}

// FormatParams 将键值映射序列化为 "k:v;k:v" 格式（按键名升序保证确定性）。
// @param params 键值映射
// @return 序列化串（空映射返回空串）
func FormatParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+":"+params[key])
	}
	return strings.Join(parts, ";")
}

// CallbackKind 是回调类别（task 任务级 / execution 执行级）。
type CallbackKind = string

// CallbackException 是一次回调最终失败的记录（对位 workflow_task_exception 的回调来源字段）。
type CallbackException struct {
	Kind      CallbackKind // 回调类别（task/execution）
	TenantID  string       // 租户（系统代号）
	Addr      string       // 回调目标地址
	Payload   string       // 报文 JSON 原文
	Attempts  int          // 总尝试次数
	Cause     error        // 最终失败原因
	OccurTime time.Time    // 发生时间
}

// ExceptionRecorder 是回调异常记录器端口（D5：重试耗尽后落 workflow_task_exception 供管理台重发）。
// 本阶段默认空实现，P4 接管表仓储后替换。
type ExceptionRecorder interface {
	// RecordCallbackException 记录一次回调失败。
	// @param ctx 操作上下文
	// @param exception 异常记录
	// @return 记录错误
	RecordCallbackException(ctx context.Context, exception *CallbackException) error
}

// NopExceptionRecorder 是异常记录器的空实现（本阶段默认，P4 替换为 workflow_task_exception 仓储）。
type NopExceptionRecorder struct{}

// RecordCallbackException 空实现：仅保证接口可用，不做任何持久化。
// @param ctx 操作上下文
// @param exception 异常记录
// @return 恒为 nil
func (NopExceptionRecorder) RecordCallbackException(ctx context.Context, exception *CallbackException) error {
	return nil
}

// CallbackOptions 是回调 client 的可注入配置。
type CallbackOptions struct {
	HTTP     *http.Client        // HTTP 客户端（默认 10 秒超时）
	Backoff  []time.Duration     // 重试退避间隔（默认 1s/2s/4s，D5 指数退避 3 次）
	Sleeper  func(time.Duration) // 退避睡眠函数（默认 time.Sleep，测试注入假实现消除真实延时）
	Logger   *log.Logger         // 回调日志（默认 stderr）
	Recorder ExceptionRecorder   // 异常记录器（默认空实现）
}

// CallbackOption 是回调 client 选项函数。
type CallbackOption func(*CallbackOptions)

// WithCallbackHTTP 注入自定义 HTTP 客户端。
// @param client HTTP 客户端
// @return 选项
func WithCallbackHTTP(client *http.Client) CallbackOption {
	return func(o *CallbackOptions) {
		if client != nil {
			o.HTTP = client
		}
	}
}

// WithCallbackBackoff 注入重试退避间隔序列。
// @param intervals 间隔序列（长度即重试次数；nil 视为不重试）
// @return 选项
func WithCallbackBackoff(intervals []time.Duration) CallbackOption {
	return func(o *CallbackOptions) { o.Backoff = intervals }
}

// WithCallbackSleeper 注入退避睡眠函数（测试用假实现）。
// @param sleeper 睡眠函数
// @return 选项
func WithCallbackSleeper(sleeper func(time.Duration)) CallbackOption {
	return func(o *CallbackOptions) {
		if sleeper != nil {
			o.Sleeper = sleeper
		}
	}
}

// WithCallbackLogger 注入回调日志。
// @param logger 日志
// @return 选项
func WithCallbackLogger(logger *log.Logger) CallbackOption {
	return func(o *CallbackOptions) {
		if logger != nil {
			o.Logger = logger
		}
	}
}

// WithCallbackRecorder 注入异常记录器。
// @param recorder 异常记录器
// @return 选项
func WithCallbackRecorder(recorder ExceptionRecorder) CallbackOption {
	return func(o *CallbackOptions) {
		if recorder != nil {
			o.Recorder = recorder
		}
	}
}

// CallbackClient 是业务回调 client：查注册表 → 空地址/查无跳过 → POST → 失败指数退避重试 → 记录异常。
type CallbackClient struct {
	store    BusinessSystemStore // 业务系统注册表（按 tenantId 查询回调地址）
	http     *http.Client        // HTTP 客户端
	backoff  []time.Duration     // 重试退避间隔
	sleeper  func(time.Duration) // 退避睡眠函数
	logger   *log.Logger         // 回调日志
	recorder ExceptionRecorder   // 异常记录器
}

// NewCallbackClient 创建业务回调 client。
// @param store 业务系统注册表端口
// @param options 可选配置（HTTP 客户端/退避/睡眠/日志/记录器）
// @return 回调 client
func NewCallbackClient(store BusinessSystemStore, options ...CallbackOption) *CallbackClient {
	opts := CallbackOptions{
		HTTP:     &http.Client{Timeout: 10 * time.Second},
		Backoff:  []time.Duration{time.Second, 2 * time.Second, 4 * time.Second},
		Sleeper:  time.Sleep,
		Logger:   log.New(os.Stderr, "workflow-callback: ", log.LstdFlags),
		Recorder: NopExceptionRecorder{},
	}
	for _, option := range options {
		option(&opts)
	}
	return &CallbackClient{
		store:    store,
		http:     opts.HTTP,
		backoff:  opts.Backoff,
		sleeper:  opts.Sleeper,
		logger:   opts.Logger,
		recorder: opts.Recorder,
	}
}

// SendTaskCallback 发送任务级回调（报文 JSON 字段与 Java 协议逐字一致）。
// @param ctx 操作上下文
// @param callback 回调报文
// @return 注册表查错/重试耗尽错误；查无注册或地址为空时静默返回 nil（与 Java 一致）
func (c *CallbackClient) SendTaskCallback(ctx context.Context, callback *TaskCallback) error {
	if callback == nil {
		return api.NewError(errCodeCallbackPost, "任务回调报文不能为空", api.ErrBadRequest)
	}
	payload, err := json.Marshal(callback)
	if err != nil {
		return api.NewError(errCodeCallbackPost, "序列化任务回调报文失败", err)
	}
	return c.sendCallback(ctx, CallbackKindTask, callback.TenantID, string(payload))
}

// SendExecutionCallback 发送执行级回调（报文无 taskId/taskName，与 Java 协议逐字一致）。
// @param ctx 操作上下文
// @param callback 回调报文
// @return 注册表查错/重试耗尽错误；查无注册或地址为空时静默返回 nil（与 Java 一致）
func (c *CallbackClient) SendExecutionCallback(ctx context.Context, callback *ExecutionCallback) error {
	if callback == nil {
		return api.NewError(errCodeCallbackPost, "执行回调报文不能为空", api.ErrBadRequest)
	}
	payload, err := json.Marshal(callback)
	if err != nil {
		return api.NewError(errCodeCallbackPost, "序列化执行回调报文失败", err)
	}
	return c.sendCallback(ctx, CallbackKindExecution, callback.TenantID, string(payload))
}

// sendCallback 查注册表解析回调地址并发送；查无注册或地址为空静默跳过（与 Java 判空逻辑一致）。
// @param ctx 操作上下文
// @param kind 回调类别（决定取 taskCallAddr 还是 executionCallAddr）
// @param tenantID 租户（系统代号）
// @param payload 报文 JSON
// @return 查询错误或重试耗尽错误；跳过时返回 nil
func (c *CallbackClient) sendCallback(ctx context.Context, kind CallbackKind, tenantID, payload string) error {
	system, found, err := c.store.GetByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	// 与 Java CollectionUtils.isEmpty 分支一致：注册表查无，静默跳过
	if !found {
		return nil
	}
	addr := system.ExecutionCallAddr
	if kind == CallbackKindTask {
		addr = system.TaskCallAddr
	}
	// 与 Java StringUtils.isNotBlank 分支一致：地址为空，静默跳过
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	return c.postWithRetry(ctx, kind, tenantID, addr, payload)
}

// postWithRetry 发送报文并按指数退避重试（D5）：重试耗尽后记录日志与异常记录器。
// @param ctx 操作上下文
// @param kind 回调类别
// @param tenantID 租户
// @param addr 回调地址
// @param payload 报文 JSON
// @return 最终失败错误；成功返回 nil
func (c *CallbackClient) postWithRetry(ctx context.Context, kind CallbackKind, tenantID, addr, payload string) error {
	attempts := len(c.backoff) + 1
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		// 首次尝试立即执行，重试前按退避序列等待（1s/2s/4s）
		if attempt > 0 {
			if err := c.wait(ctx, c.backoff[attempt-1]); err != nil {
				return err
			}
		}
		lastErr = c.post(ctx, addr, payload)
		if lastErr == nil {
			return nil
		}
	}
	// 重试全部失败：记录日志 + 可插拔异常记录器（本阶段空实现，P4 接 workflow_task_exception）
	c.logger.Printf("业务回调最终失败: kind=%s tenantId=%s addr=%s attempts=%d err=%v", kind, tenantID, addr, attempts, lastErr)
	exception := &CallbackException{
		Kind:      kind,
		TenantID:  tenantID,
		Addr:      addr,
		Payload:   payload,
		Attempts:  attempts,
		Cause:     lastErr,
		OccurTime: time.Now(),
	}
	// 记录器失败不影响回调结果语义，仅追加日志
	if recordErr := c.recorder.RecordCallbackException(ctx, exception); recordErr != nil {
		c.logger.Printf("回调异常记录失败: kind=%s tenantId=%s err=%v", kind, tenantID, recordErr)
	}
	return api.NewError(errCodeCallbackPost, fmt.Sprintf("业务回调重试 %d 次后仍失败: %s", attempts, addr), lastErr)
}

// wait 执行退避等待（注入假睡眠时无真实延时，便于测试时序断言）。
// @param ctx 操作上下文
// @param d 等待时长
// @return 等待前后上下文已取消时返回取消错误
func (c *CallbackClient) wait(ctx context.Context, d time.Duration) error {
	// 已取消的上下文不再退避，立即返回
	if err := ctx.Err(); err != nil {
		return err
	}
	c.sleeper(d)
	return ctx.Err()
}

// post 发送单次 HTTP POST（application/json，2xx 视为成功）。
// @param ctx 操作上下文
// @param addr 回调地址
// @param payload 报文 JSON
// @return 网络/非 2xx 状态错误
func (c *CallbackClient) post(ctx context.Context, addr, payload string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, strings.NewReader(payload))
	if err != nil {
		return api.NewError(errCodeCallbackPost, "构造回调请求失败", err)
	}
	// 与 Java HttpUtil.post 一致：JSON 体直投业务系统
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return api.NewError(errCodeCallbackPost, "回调请求发送失败", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// 仅 2xx 视为成功，其余状态码触发重试
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return api.NewError(errCodeCallbackStatus, fmt.Sprintf("回调返回非 2xx 状态: %d", resp.StatusCode), nil)
	}
	return nil
}
