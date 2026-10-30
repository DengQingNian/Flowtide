// callback_test.go 验证业务回调 client：报文字段与 Java 协议逐字一致、重试退避时序、空地址/查无注册跳过、异常记录器。
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// fakeSleeper 记录退避等待序列的可注入睡眠实现。
type fakeSleeper struct {
	mu     sync.Mutex      // 保护 sleeps
	sleeps []time.Duration // 退避等待记录
}

// Sleep 记录等待时长（不真实睡眠）。
// @param d 等待时长
func (f *fakeSleeper) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sleeps = append(f.sleeps, d)
}

// recorded 返回已记录的等待序列副本。
// @return 等待序列
func (f *fakeSleeper) recorded() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration(nil), f.sleeps...)
}

// recordingRecorder 捕获异常记录的可插拔记录器实现。
type recordingRecorder struct {
	mu         sync.Mutex           // 保护 exceptions
	exceptions []*CallbackException // 已记录异常
	err        error                // 注入的记录错误
}

// RecordCallbackException 捕获并保存异常记录。
// @param ctx 操作上下文
// @param exception 异常记录
// @return 注入的错误（可空）
func (r *recordingRecorder) RecordCallbackException(ctx context.Context, exception *CallbackException) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exceptions = append(r.exceptions, exception)
	return r.err
}

// TestParseParams 验证 "k:v;k:v" 报文参数解析。
// @param t 测试上下文
func TestParseParams(t *testing.T) {
	parsed := ParseParams("name:张三;age:1;sex:男")
	if len(parsed) != 3 || parsed["name"] != "张三" || parsed["age"] != "1" || parsed["sex"] != "男" {
		t.Errorf("解析结果不符: %v", parsed)
	}
	// 值中含冒号保留（比 Java split[1] 更忠实）
	if got := ParseParams("url:http://a:1/x")["url"]; got != "http://a:1/x" {
		t.Errorf("值内冒号应保留: %s", got)
	}
	// 无冒号分段跳过、空串返回空映射、空值保留
	if got := ParseParams("novalue;k:;"); len(got) != 1 || got["k"] != "" {
		t.Errorf("异常分段处理不符: %v", got)
	}
	if got := ParseParams(""); len(got) != 0 {
		t.Errorf("空串应返回空映射: %v", got)
	}
}

// TestFormatParams 验证 "k:v;k:v" 序列化（键升序确定性与往返）。
// @param t 测试上下文
func TestFormatParams(t *testing.T) {
	// 键升序
	if got := FormatParams(map[string]string{"sex": "男", "name": "张三"}); got != "name:张三;sex:男" {
		t.Errorf("序列化应为键升序: %s", got)
	}
	if FormatParams(nil) != "" || FormatParams(map[string]string{}) != "" {
		t.Error("空映射应返回空串")
	}
	// 往返一致
	round := ParseParams(FormatParams(map[string]string{"a": "1", "b": "2"}))
	if round["a"] != "1" || round["b"] != "2" {
		t.Errorf("往返结果不符: %v", round)
	}
}

// newCallbackFixture 构造注册好回调地址的 client 与假睡眠。
// @param t 测试上下文
// @param store 注册表
// @param sleeper 假睡眠
// @return 回调 client
func newCallbackFixture(t *testing.T, store BusinessSystemStore, sleeper *fakeSleeper) *CallbackClient {
	t.Helper()
	return NewCallbackClient(store, WithCallbackSleeper(sleeper.Sleep),
		WithCallbackLogger(log.New(&strings.Builder{}, "", 0)))
}

// TestCallbackClientTaskPayload 验证任务级回调报文字段与 Java 协议逐字一致。
// @param t 测试上下文
func TestCallbackClientTaskPayload(t *testing.T) {
	var mu sync.Mutex
	bodies := make([]map[string]any, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewMemoryBusinessSystemStore()
	if _, err := NewBusinessSystemService(store).Register(context.Background(),
		&BusinessSystem{SystemSymbol: "oa", TaskCallAddr: server.URL + "/task", ExecutionCallAddr: server.URL + "/exec"}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	client := newCallbackFixture(t, store, &fakeSleeper{})
	err := client.SendTaskCallback(context.Background(), &TaskCallback{
		ProcessDefinitionID: "pd-1", ProcessInstanceID: "pi-1", BusinessKey: "biz-1", TenantID: "oa",
		TaskID: "task-1", TaskName: "审批", FlowElementID: "approve", FlowElementName: "审批节点",
		Params: "name:张三;age:1",
	})
	if err != nil {
		t.Fatalf("任务回调失败: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("应恰好 1 次请求，实际 %d", len(bodies))
	}
	body := bodies[0]
	// JSON 字段名与 Java paramMap 键逐字对照：processDefinitionId/processInstanceId/businessKey/tenantId/taskId/taskName/flowElementId/flowElementName/params
	expect := map[string]string{
		"processDefinitionId": "pd-1", "processInstanceId": "pi-1", "businessKey": "biz-1",
		"tenantId": "oa", "taskId": "task-1", "taskName": "审批",
		"flowElementId": "approve", "flowElementName": "审批节点", "params": "name:张三;age:1",
	}
	for key, want := range expect {
		if got, _ := body[key].(string); got != want {
			t.Errorf("字段 %s 应为 %s，实际 %v", key, want, body[key])
		}
	}
	if len(body) != len(expect) {
		t.Errorf("报文应恰好 %d 个字段，实际 %d 个: %v", len(expect), len(body), body)
	}
}

// TestCallbackClientExecutionPayload 验证执行级回调无 taskId/taskName 字段。
// @param t 测试上下文
func TestCallbackClientExecutionPayload(t *testing.T) {
	var mu sync.Mutex
	bodies := make([]map[string]any, 0, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewMemoryBusinessSystemStore()
	if _, err := NewBusinessSystemService(store).Register(context.Background(),
		&BusinessSystem{SystemSymbol: "oa", ExecutionCallAddr: server.URL + "/exec"}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	client := newCallbackFixture(t, store, &fakeSleeper{})
	err := client.SendExecutionCallback(context.Background(), &ExecutionCallback{
		ProcessDefinitionID: "pd-1", ProcessInstanceID: "pi-1", BusinessKey: "biz-1", TenantID: "oa",
		FlowElementID: "start", FlowElementName: "开始", Params: "step:1",
	})
	if err != nil {
		t.Fatalf("执行回调失败: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("应恰好 1 次请求，实际 %d", len(bodies))
	}
	body := bodies[0]
	// 执行级报文与 Java 一致：无 taskId/taskName，其余字段同任务级
	if _, ok := body["taskId"]; ok {
		t.Error("执行级报文不应含 taskId 字段")
	}
	if _, ok := body["taskName"]; ok {
		t.Error("执行级报文不应含 taskName 字段")
	}
	if body["flowElementId"] != "start" || body["tenantId"] != "oa" || body["params"] != "step:1" {
		t.Errorf("执行级报文字段不符: %v", body)
	}
}

// TestCallbackClientRetryBackoff 验证重试退避时序与最终成功。
// @param t 测试上下文
func TestCallbackClientRetryBackoff(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()
		// 前两次失败（5xx），第三次成功
		if current < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewMemoryBusinessSystemStore()
	if _, err := NewBusinessSystemService(store).Register(context.Background(),
		&BusinessSystem{SystemSymbol: "oa", TaskCallAddr: server.URL}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	sleeper := &fakeSleeper{}
	client := newCallbackFixture(t, store, sleeper)
	err := client.SendTaskCallback(context.Background(), &TaskCallback{TenantID: "oa", TaskID: "t1"})
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	mu.Lock()
	if attempts != 3 {
		t.Errorf("应共 3 次尝试，实际 %d", attempts)
	}
	mu.Unlock()
	// 退避时序：默认 1s/2s（第 2、3 次尝试前）
	got := sleeper.recorded()
	if len(got) != 2 || got[0] != time.Second || got[1] != 2*time.Second {
		t.Errorf("退避序列应为 [1s 2s]，实际 %v", got)
	}
}

// TestCallbackClientRetryExhausted 验证重试耗尽：次数、退避 1s/2s/4s、异常记录器与返回错误。
// @param t 测试上下文
func TestCallbackClientRetryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := NewMemoryBusinessSystemStore()
	if _, err := NewBusinessSystemService(store).Register(context.Background(),
		&BusinessSystem{SystemSymbol: "oa", TaskCallAddr: server.URL}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	sleeper := &fakeSleeper{}
	recorder := &recordingRecorder{}
	client := NewCallbackClient(store, WithCallbackSleeper(sleeper.Sleep),
		WithCallbackRecorder(recorder), WithCallbackLogger(log.New(&strings.Builder{}, "", 0)))
	err := client.SendTaskCallback(context.Background(), &TaskCallback{TenantID: "oa", BusinessKey: "biz-1"})
	if err == nil {
		t.Fatal("重试耗尽应返回错误")
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.Code != errCodeCallbackPost {
		t.Errorf("应返回 WORKFLOW-2101，实际 %v", err)
	}
	got := sleeper.recorded()
	// D5：指数退避 3 次（1s/2s/4s），总尝试 1+3=4
	if len(got) != 3 || got[0] != time.Second || got[1] != 2*time.Second || got[2] != 4*time.Second {
		t.Errorf("退避序列应为 [1s 2s 4s]，实际 %v", got)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.exceptions) != 1 {
		t.Fatalf("应记录 1 条异常，实际 %d", len(recorder.exceptions))
	}
	exception := recorder.exceptions[0]
	if exception.Kind != CallbackKindTask || exception.TenantID != "oa" || exception.Attempts != 4 || exception.Addr != server.URL || exception.Cause == nil {
		t.Errorf("异常记录不符: %+v", exception)
	}
	if exception.Payload == "" || !strings.Contains(exception.Payload, `"businessKey":"biz-1"`) {
		t.Errorf("异常记录应含报文原文: %s", exception.Payload)
	}
}

// TestCallbackClientSkipRules 验证空地址与查无注册静默跳过（与 Java 判空一致）。
// @param t 测试上下文
func TestCallbackClientSkipRules(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewMemoryBusinessSystemStore()
	ctx := context.Background()
	// 注册：任务地址为空、执行地址有效
	if _, err := NewBusinessSystemService(store).Register(ctx,
		&BusinessSystem{SystemSymbol: "oa", ExecutionCallAddr: server.URL}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	client := newCallbackFixture(t, store, &fakeSleeper{})
	// 任务地址为空 → 静默跳过
	if err := client.SendTaskCallback(ctx, &TaskCallback{TenantID: "oa"}); err != nil {
		t.Errorf("空地址应静默跳过，实际 %v", err)
	}
	if requests != 0 {
		t.Errorf("空地址不应发请求，实际 %d", requests)
	}
	// 查无注册 → 静默跳过
	if err := client.SendTaskCallback(ctx, &TaskCallback{TenantID: "unknown"}); err != nil {
		t.Errorf("查无注册应静默跳过，实际 %v", err)
	}
	if err := client.SendExecutionCallback(ctx, &ExecutionCallback{TenantID: "unknown"}); err != nil {
		t.Errorf("查无注册应静默跳过，实际 %v", err)
	}
	if requests != 0 {
		t.Errorf("跳过场景不应发请求，实际 %d", requests)
	}
	// 执行地址有效 → 发送成功
	if err := client.SendExecutionCallback(ctx, &ExecutionCallback{TenantID: "oa"}); err != nil {
		t.Errorf("执行回调失败: %v", err)
	}
	if requests != 1 {
		t.Errorf("执行回调应发 1 次请求，实际 %d", requests)
	}
	// 空报文
	if err := client.SendTaskCallback(ctx, nil); err == nil {
		t.Error("nil 任务报文应报错")
	}
	if err := client.SendExecutionCallback(ctx, nil); err == nil {
		t.Error("nil 执行报文应报错")
	}
}

// TestCallbackClientOptions 验证注入项：退避序列覆盖、记录器报错不影响结果、上下文取消。
// @param t 测试上下文
func TestCallbackClientOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 恒 500：触发重试路径
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := NewMemoryBusinessSystemStore()
	ctx := context.Background()
	if _, err := NewBusinessSystemService(store).Register(ctx,
		&BusinessSystem{SystemSymbol: "oa", TaskCallAddr: server.URL}); err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	// 自定义退避（2 次重试，间隔 10ms/20ms）+ 记录器报错
	sleeper := &fakeSleeper{}
	recorder := &recordingRecorder{err: errors.New("record failed")}
	client := NewCallbackClient(store, WithCallbackSleeper(sleeper.Sleep), WithCallbackRecorder(recorder),
		WithCallbackBackoff([]time.Duration{10 * time.Millisecond, 20 * time.Millisecond}),
		WithCallbackLogger(log.New(&strings.Builder{}, "", 0)))
	if err := client.SendTaskCallback(ctx, &TaskCallback{TenantID: "oa"}); err == nil {
		t.Error("重试耗尽应返回错误")
	}
	got := sleeper.recorded()
	if len(got) != 2 || got[0] != 10*time.Millisecond || got[1] != 20*time.Millisecond {
		t.Errorf("自定义退避序列应为 [10ms 20ms]，实际 %v", got)
	}
	recorder.mu.Lock()
	count := len(recorder.exceptions)
	recorder.mu.Unlock()
	if count != 1 {
		t.Errorf("记录器报错不应中断记录，应 1 条，实际 %d", count)
	}
	// 不重试：退避序列为空
	noRetry := NewCallbackClient(store, WithCallbackSleeper(sleeper.Sleep), WithCallbackBackoff(nil),
		WithCallbackLogger(log.New(&strings.Builder{}, "", 0)))
	if err := noRetry.SendTaskCallback(ctx, &TaskCallback{TenantID: "oa"}); err == nil {
		t.Error("无重试配置下仍应返回失败")
	}
	// 已取消上下文：不再退避
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	sleeper2 := &fakeSleeper{}
	ctxClient := newCallbackFixture(t, store, sleeper2)
	if err := ctxClient.SendTaskCallback(canceled, &TaskCallback{TenantID: "oa"}); !errors.Is(err, context.Canceled) {
		t.Errorf("已取消上下文应返回取消错误，实际 %v", err)
	}
	if got := sleeper2.recorded(); len(got) != 0 {
		t.Errorf("已取消上下文不应进入退避，实际 %v", got)
	}
}

// TestCallbackClientLookupError 验证注册表查询错误上抛（不静默）。
// @param t 测试上下文
func TestCallbackClientLookupError(t *testing.T) {
	store := errLookupStore{}
	client := newCallbackFixture(t, store, &fakeSleeper{})
	if err := client.SendTaskCallback(context.Background(), &TaskCallback{TenantID: "oa"}); err == nil {
		t.Error("注册表查询错误应上抛")
	}
}

// errLookupStore 恒返回查询错误的存储桩。
type errLookupStore struct{}

// Save 保存（未使用）。
// @param ctx 操作上下文
// @param system 注册信息
// @return 恒为 nil
func (errLookupStore) Save(ctx context.Context, system *BusinessSystem) error { return nil }

// GetBySymbol 按代号查询（未使用）。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 恒 nil
// @return 恒 false
// @return 恒错误
func (errLookupStore) GetBySymbol(ctx context.Context, symbol string) (*BusinessSystem, bool, error) {
	return nil, false, errors.New("lookup failed")
}

// GetByTenant 按租户查询（回调查询入口）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 恒 nil
// @return 恒 false
// @return 恒错误
func (errLookupStore) GetByTenant(ctx context.Context, tenantID string) (*BusinessSystem, bool, error) {
	return nil, false, errors.New("lookup failed")
}

// List 列表（未使用）。
// @param ctx 操作上下文
// @return 恒 nil
// @return 恒 nil
func (errLookupStore) List(ctx context.Context) ([]BusinessSystem, error) { return nil, nil }

// Delete 删除（未使用）。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 恒 nil
func (errLookupStore) Delete(ctx context.Context, symbol string) error { return nil }
