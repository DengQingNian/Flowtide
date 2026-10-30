package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/bpmn/model"
	"github.com/DengQingNian/Flowtide/internal/clock"
)

// TestDeployment_CascadeCleansJobsAndSubs 验证级联清理作业与订阅。
// @param t 测试上下文
func TestDeployment_CascadeCleansJobsAndSubs(t *testing.T) {
	fake := clock.NewFake(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC))
	engine, _ := New(Config{Clock: fake})
	deployXML(t, engine, timerStartXML)
	deployment := mustDeployment(t, engine)
	if len(engine.state.Jobs) == 0 {
		t.Fatalf("定时启动应创建作业")
	}
	if err := engine.DeleteDeployment(context.Background(), deployment.ID, true); err != nil {
		t.Fatalf("删除部署失败: %v", err)
	}
	if len(engine.state.Jobs) != 0 || len(engine.state.Subscriptions) != 0 {
		t.Fatalf("级联删除应清理作业与订阅")
	}
}

// TestDeleteInstance_CleansSubscription 验证删除实例清理订阅。
// @param t 测试上下文
func TestDeleteInstance_CleansSubscription(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, messageCatchXML)
	instance, _ := engine.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "msgCatch"})
	if len(engine.state.Subscriptions) == 0 {
		t.Fatalf("消息捕获应创建订阅")
	}
	if err := engine.DeleteProcessInstance(context.Background(), instance.ID, "test"); err != nil {
		t.Fatalf("删除实例失败: %v", err)
	}
	if len(engine.state.Subscriptions) != 0 {
		t.Fatalf("删除实例应清理订阅")
	}
}

// TestErrorCodeFallbacks 验证错误码回退。
// @param t 测试上下文
func TestErrorCodeFallbacks(t *testing.T) {
	bpmnModel := &model.BpmnModel{Errors: []*model.Error{{ID: "e1"}}}
	if got := errorCode(bpmnModel, "e1"); got != "e1" {
		t.Fatalf("错误码应回退到 ID: %s", got)
	}
	if got := errorCode(bpmnModel, "missing"); got != "missing" {
		t.Fatalf("缺失错误引用应原样返回: %s", got)
	}
}

// TestScopeRootBroken 验证断链作用域根。
// @param t 测试上下文
func TestScopeRootBroken(t *testing.T) {
	engine, _ := New(Config{})
	execution := &executionState{ID: "e", ParentID: "missing"}
	if got := engine.scopeRoot(execution); got != execution {
		t.Fatalf("断链作用域根应返回自身")
	}
}

// TestDestroyScopeEmpty 验证空作用域销毁。
// @param t 测试上下文
func TestDestroyScopeEmpty(t *testing.T) {
	engine, _ := New(Config{})
	engine.state.Instances["i"] = &instanceState{ID: "i", RootExecutionID: "root"}
	engine.state.Executions["root"] = &executionState{ID: "root", ProcessInstanceID: "i"}
	cc := &commandContext{engine: engine, ctx: context.Background(), now: time.Now()}
	engine.destroyScope(cc, engine.state.Executions["root"])
	if !engine.state.Executions["root"].Ended {
		t.Fatalf("作用域应结束")
	}
}
