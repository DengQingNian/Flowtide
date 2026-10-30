package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestAddSign_NilClockFallback 验证时钟未注入时回退真实时钟（构造分支覆盖）。
// @param t 测试上下文
func TestAddSign_NilClockFallback(t *testing.T) {
	fx := newCompleteFixture(t)
	svc := NewAddSignService(fx.engine, fx.comments, fx.operations, fx.messages, fx.histories, nil)
	if svc.clock == nil {
		t.Fatal("时钟应回退非 nil")
	}
}

// TestAddSign_HelperUnits 验证展示名/业务键/集合读取的降级分支。
// @param t 测试上下文
func TestAddSign_HelperUnits(t *testing.T) {
	fx := newAddsignFixture(t)
	ctx := context.Background()
	// 业务键：空实例 / 查询失败
	if got := fx.addSign.businessKeyOf(ctx, ""); got != "" {
		t.Fatalf("空实例应返回空: %q", got)
	}
	if got := fx.addSign.businessKeyOf(ctx, "ghost"); got != "" {
		t.Fatalf("查询失败应返回空: %q", got)
	}
	// 展示名：未注入 / 查询失败
	if got := fx.addSign.userDisplayName(ctx, ""); got != "" {
		t.Fatalf("空用户应返回空: %q", got)
	}
	if got := fx.addSign.userDisplayName(ctx, "ghost"); got != "" {
		t.Fatalf("查询失败应返回空: %q", got)
	}
	// 集合变量读取：实例不存在（GetVariable 错误分支）
	if got := toStringSliceOf(fx.engine.RuntimeService(), ctx, "ghost"); len(got) != 0 {
		t.Fatalf("读取失败应返回空切片: %v", got)
	}
	// 作用域读取：未标记父任务按前加签处理
	if got := addSignScopeOf(api.Task{}); got != AddSignScopeBefore {
		t.Fatalf("未标记应按前加签: %s", got)
	}
	if got := addSignScopeOf(api.Task{Variables: map[string]any{varScopeType: AddSignScopeAfter}}); got != AddSignScopeAfter {
		t.Fatalf("after 标记应识别: %s", got)
	}
}

// TestAddSign_GhostInstanceMember 验证对不存在实例的成员维护错误透传（覆盖读取/写入错误分支）。
// @param t 测试上下文
func TestAddSign_GhostInstanceMember(t *testing.T) {
	fx := newAddsignFixture(t)
	err := fx.addSign.AddMultiInstanceMember(context.Background(), "ghost-instance", "csTask", "carol")
	if err == nil || !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("实例不存在应报错: %v", err)
	}
}

// TestAddSign_RemoveMemberPortError 验证减成员端口错误短路变量维护。
// @param t 测试上下文
func TestAddSign_RemoveMemberPortError(t *testing.T) {
	cs := newCountersignFixture(t)
	fx := NewAddSignService(cs.engine, nil, nil, nil, nil, csClock())
	port := &fakeMemberPort{err: assertError("引擎端口失败")}
	fx.WithMultiInstancePort(port)
	err := fx.RemoveMultiInstanceMember(context.Background(), "inst", "csTask", "carol", "exec-1", false)
	if err == nil || !strings.Contains(err.Error(), "引擎端口失败") {
		t.Fatalf("端口错误应上抛: %v", err)
	}
}

// TestAddSign_DelSignNotPending 验证父任务非委派态时减签恢复报错（引擎 Resolve 前置）。
// @param t 测试上下文
func TestAddSign_DelSignNotPending(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-ds5")
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"bob"}, ScopeType: AddSignScopeBefore, Message: "前加签",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	// 手工归还父任务（脱离 PENDING）
	if err := fx.engine.TaskService().Resolve(context.Background(), result.ParentTaskID, ""); err != nil {
		t.Fatalf("手工归还失败: %v", err)
	}
	// 减签恢复时 Resolve 再次执行 → 引擎报"任务未处于委派状态"
	_, err = fx.addSign.DelSign(context.Background(), DelSignRequest{
		ParentTaskID: result.ParentTaskID, SubTaskID: result.SubTaskIDs[0], Operator: "alice",
	})
	if err == nil || !strings.Contains(err.Error(), "委派状态") {
		t.Fatalf("非委派态恢复应报错: %v", err)
	}
}
