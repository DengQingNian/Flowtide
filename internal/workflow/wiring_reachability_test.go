// wiring_reachability_test.go 是装配可达性守卫（016 号审计新增）。
//
// 背景：本次审计发现多处"能力已实现 + 单测通过，但生产装配层从未接线"的缺陷
// （会签规则端口、会签监听器、业务回调 client、异常记录器）。此类缺陷单测无法发现，
// 因为测试自行构造了完整依赖。本文件以装配结果为准做静态断言，防止回归。
package workflow

import (
	"testing"

	"github.com/DengQingNian/Flowtide/internal/engine"
)

// newWiredService 构造内存模式完整装配的服务（与生产启动路径同构）。
// @param t 测试上下文
// @return 服务
func newWiredService(t *testing.T) *Service {
	t.Helper()
	eng, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close(t.Context()) })
	return NewWorkflowService(eng)
}

// TestAssembly_CountersignPortsWired 验证会签服务拿到规则端口与 next_activity 来源。
// 016 号审计：此前生产只注入权重存储，比例退回恒 disabled、预存覆盖失效。
// @param t 测试上下文
func TestAssembly_CountersignPortsWired(t *testing.T) {
	svc := newWiredService(t)
	if svc.comp == nil || svc.comp.counter == nil {
		t.Fatalf("会签服务未装配")
	}
	if svc.comp.counter.rules == nil {
		t.Fatalf("会签规则端口未注入：比例退回将恒为未启用")
	}
	if svc.comp.counter.next == nil {
		t.Fatalf("会签 next_activity 来源未注入：预存执行者覆盖失效")
	}
	// 成员查询按 Java 语义故意不注入（集合配置阶段不展开 org/role）
	if svc.comp.counter.members != nil {
		t.Fatalf("成员查询不应注入：Java 在集合配置阶段保留组 ID")
	}
}

// TestAssembly_BusinessCallbackWired 验证业务回调 client 与钩子已装配。
// 016 号审计：CallbackClient 此前在生产代码零调用点。
// @param t 测试上下文
func TestAssembly_BusinessCallbackWired(t *testing.T) {
	svc := newWiredService(t)
	if svc.comp.businesses == nil {
		t.Fatalf("业务系统注册存储未装配")
	}
	if svc.comp.bizCallback == nil {
		t.Fatalf("业务回调钩子未装配：D5 回调能力不生效")
	}
	if svc.comp.bizCallback.client == nil {
		t.Fatalf("回调 client 未装配")
	}
	// 异常记录器必须接到 ExceptionService，否则回调失败不落 workflow_task_exception
	if svc.comp.bizCallback.client.recorder == nil {
		t.Fatalf("回调异常记录器未装配")
	}
	if _, ok := svc.comp.bizCallback.client.recorder.(NopExceptionRecorder); ok {
		t.Fatalf("回调异常记录器仍是 Nop：回调失败不会落异常表")
	}
}

// TestAssembly_DeployRewarmEntries 验证部署后预热入口均可用（不 panic）。
// @param t 测试上下文
func TestAssembly_DeployRewarmEntries(t *testing.T) {
	svc := newWiredService(t)
	ctx := t.Context()
	// 空定义 Id 应安全跳过
	if err := svc.RewarmCountersignDefinition(ctx, ""); err != nil {
		t.Fatalf("空定义预热应静默跳过: %v", err)
	}
	if err := svc.RewarmExecutorRewriteDefinition(ctx, ""); err != nil {
		t.Fatalf("空定义预热应静默跳过: %v", err)
	}
	if err := svc.RewarmBusinessCallbackDefinition(ctx, ""); err != nil {
		t.Fatalf("空定义预热应静默跳过: %v", err)
	}
	// 启动期注册入口应可执行
	if err := svc.RegisterBusinessCallbackHook(ctx); err != nil {
		t.Fatalf("业务回调钩子注册失败: %v", err)
	}
	if err := svc.RegisterCountersignRuntime(ctx); err != nil {
		t.Fatalf("会签运行时注册失败: %v", err)
	}
}
