// business_callback_hook_test.go 验证业务回调钩子（TASK_CREATED）的接线语义。
// 016 号审计：CallbackClient 此前在生产代码零调用点，本文件锁定钩子的组包、
// 延迟发送（锁外）、未预热降级与单定义预热行为。
package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// newCallbackHookEnv 构造注册了回调地址的钩子与报文采集服务。
// @param t 测试上下文
// @return 钩子
// @return 已接收报文（按顺序）
// @return 关闭函数
func newCallbackHookEnv(t *testing.T) (*BusinessCallbackHook, func() []map[string]any) {
	t.Helper()
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
	t.Cleanup(server.Close)

	store := NewMemoryBusinessSystemStore()
	if _, err := NewBusinessSystemService(store).Register(context.Background(),
		&BusinessSystem{SystemSymbol: "oa", TaskCallAddr: server.URL + "/task"}); err != nil {
		t.Fatalf("注册业务系统失败: %v", err)
	}
	hook := NewBusinessCallbackHook(NewModelMetaService(nil), newCallbackFixture(t, store, &fakeSleeper{}))
	// 直接注入预热缓存，绕开引擎元数据读取（本文件只验证钩子自身语义）
	hook.cache["pd-1"] = map[string]*UserTaskMeta{
		"approve": {ActivityID: "approve", BusinessCallbackRaw: "true", BusinessCallbackParamsRaw: "name:张三"},
		"audit":   {ActivityID: "audit"},
	}
	return hook, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), bodies...)
	}
}

// newCallbackHookTask 构造任务快照。
// @param key 节点 ID
// @return 任务快照
func newCallbackHookTask(key string) *api.Task {
	return &api.Task{
		ID: "task-1", Name: "审批", ProcessInstanceID: "pi-1", ProcessDefinitionID: "pd-1",
		TaskDefinitionKey: key, TenantID: "oa",
		Variables: map[string]any{VarBusinessKey: "biz-1"},
	}
}

// TestBusinessCallbackHook_SendsAfterCommit 验证声明回调属性的节点在命令后延迟发送。
// @param t 测试上下文
func TestBusinessCallbackHook_SendsAfterCommit(t *testing.T) {
	hook, sent := newCallbackHookEnv(t)
	ctl := &fakeHookCtl{variables: map[string]any{}}

	if err := hook.Hook(context.Background(), newCallbackHookTask("approve"), ctl); err != nil {
		t.Fatalf("钩子不应返回错误: %v", err)
	}
	// 钩子内不得直接发 HTTP：只能在 ctl.Defer 登记一个动作
	if len(sent()) != 0 {
		t.Fatalf("锁内不得发起 HTTP 回调")
	}
	if len(ctl.deferred) != 1 {
		t.Fatalf("应登记 1 个延迟动作，实际 %d", len(ctl.deferred))
	}
	if err := ctl.deferred[0](context.Background()); err != nil {
		t.Fatalf("延迟回调失败: %v", err)
	}
	bodies := sent()
	if len(bodies) != 1 {
		t.Fatalf("应发送 1 次回调，实际 %d", len(bodies))
	}
	if bodies[0]["taskId"] != "task-1" || bodies[0]["businessKey"] != "biz-1" || bodies[0]["tenantId"] != "oa" {
		t.Fatalf("报文字段不符: %+v", bodies[0])
	}
	if bodies[0]["params"] != "name:张三" {
		t.Fatalf("params 应取节点扩展属性: %+v", bodies[0])
	}
}

// TestBusinessCallbackHook_SkipsNodeWithoutAttribute 验证未声明回调属性的节点不发送。
// @param t 测试上下文
func TestBusinessCallbackHook_SkipsNodeWithoutAttribute(t *testing.T) {
	hook, sent := newCallbackHookEnv(t)
	ctl := &fakeHookCtl{variables: map[string]any{}}
	if err := hook.Hook(context.Background(), newCallbackHookTask("audit"), ctl); err != nil {
		t.Fatalf("钩子不应返回错误: %v", err)
	}
	if len(ctl.deferred) != 0 {
		t.Fatalf("未声明回调属性的节点不应登记延迟动作")
	}
	for _, action := range ctl.deferred {
		_ = action(context.Background())
	}
	if len(sent()) != 0 {
		t.Fatalf("不应发送回调")
	}
}

// TestBusinessCallbackHook_WarmupFallbackOnMiss 验证未预热时降级为延迟预热且不发送。
// @param t 测试上下文
func TestBusinessCallbackHook_WarmupFallbackOnMiss(t *testing.T) {
	hook, sent := newCallbackHookEnv(t)
	ctl := &fakeHookCtl{variables: map[string]any{}}
	task := newCallbackHookTask("approve")
	task.ProcessDefinitionID = "unknown-def"

	if err := hook.Hook(context.Background(), task, ctl); err != nil {
		t.Fatalf("钩子不应返回错误: %v", err)
	}
	if len(ctl.deferred) != 1 {
		t.Fatalf("未预热应登记 1 个延迟预热动作，实际 %d", len(ctl.deferred))
	}
	if len(sent()) != 0 {
		t.Fatalf("未预热不应发送回调")
	}
}

// TestBusinessCallbackHook_NilClientIsNoop 验证未装配回调 client 时为空操作。
// @param t 测试上下文
func TestBusinessCallbackHook_NilClientIsNoop(t *testing.T) {
	hook := NewBusinessCallbackHook(NewModelMetaService(nil), nil)
	ctl := &fakeHookCtl{variables: map[string]any{}}
	if err := hook.Hook(context.Background(), newCallbackHookTask("approve"), ctl); err != nil {
		t.Fatalf("空 client 不应报错: %v", err)
	}
	if len(ctl.deferred) != 0 {
		t.Fatalf("空 client 不应登记延迟动作")
	}
	// Register 对空 client 与空引擎均为安全空操作
	hook.Register(nil)
	hook.Register(nil)
}

// TestUserTaskMeta_BusinessCallbackAccessors 验证回调属性访问器。
// @param t 测试上下文
func TestUserTaskMeta_BusinessCallbackAccessors(t *testing.T) {
	var nilMeta *UserTaskMeta
	if nilMeta.BusinessCallbackEnabled() || nilMeta.BusinessCallbackParams() != "" {
		t.Fatalf("nil 元数据应安全返回零值")
	}
	enabled := &UserTaskMeta{BusinessCallbackRaw: " TRUE ", BusinessCallbackParamsRaw: "a:1"}
	if !enabled.BusinessCallbackEnabled() {
		t.Fatalf("TRUE（含空白）应判定为启用")
	}
	if enabled.BusinessCallbackParams() != "a:1" {
		t.Fatalf("params 应原样返回")
	}
	disabled := &UserTaskMeta{BusinessCallbackRaw: "false"}
	if disabled.BusinessCallbackEnabled() {
		t.Fatalf("false 不应判定为启用")
	}
}
