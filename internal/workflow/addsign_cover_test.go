package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// TestAddSign_BlankAssigneeSkipped 验证被加签人空白项跳过（Java isNotBlank 门控）。
// @param t 测试上下文
func TestAddSign_BlankAssigneeSkipped(t *testing.T) {
	fx := newAddsignFixture(t)
	_, apply := fx.startApply(t, "biz-blank")
	result, err := fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: apply.ID, Operator: "alice", Assignees: []string{"", "  ", "bob"}, ScopeType: AddSignScopeAfter, Message: "含空白项",
	})
	if err != nil {
		t.Fatalf("加签失败: %v", err)
	}
	if len(result.SubTaskIDs) != 1 {
		t.Fatalf("空白项应跳过仅剩 1 个子任务: %v", result.SubTaskIDs)
	}
}

// TestAddSign_SecondLevelParentVanished 验证二层加签时顶层父任务已消失的错误。
// @param t 测试上下文
func TestAddSign_SecondLevelParentVanished(t *testing.T) {
	fx := newAddsignFixture(t)
	// 独立占位任务 + 子任务（模拟二层结构，占位任务随后消失）
	holder, err := fx.engine.TaskService().CreateTask(context.Background(), api.CreateTaskRequest{Name: "占位父", Assignee: "alice", TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建占位失败: %v", err)
	}
	sub, err := fx.engine.TaskService().CreateTask(context.Background(), api.CreateTaskRequest{Name: "子任务", Assignee: "bob", ParentTaskID: holder.ID, TenantID: "dev"})
	if err != nil {
		t.Fatalf("创建子任务失败: %v", err)
	}
	// 引擎直办占位任务使其消失（独立任务完成即移除，子任务保留）
	if err := fx.engine.TaskService().Complete(context.Background(), holder.ID, api.CompleteTaskRequest{UserID: "alice"}); err != nil {
		t.Fatalf("办结占位失败: %v", err)
	}
	_, err = fx.addSign.AddSign(context.Background(), AddSignRequest{
		TaskID: sub.ID, Operator: "bob", Assignees: []string{"carol"}, ScopeType: AddSignScopeBefore, Message: "二层加签",
	})
	if err == nil || !strings.Contains(err.Error(), "父任务不存在") {
		t.Fatalf("顶层父任务消失应报错: %v", err)
	}
}

// TestAddSign_DelSignOperatorMissing 验证减签操作人缺失校验。
// @param t 测试上下文
func TestAddSign_DelSignOperatorMissing(t *testing.T) {
	fx := newAddsignFixture(t)
	_, err := fx.addSign.DelSign(context.Background(), DelSignRequest{ParentTaskID: "p", SubTaskID: "s"})
	if err == nil || !strings.Contains(err.Error(), "operator 为空") {
		t.Fatalf("操作人缺失应报错: %v", err)
	}
}

// TestAddSign_RemoveMemberGhostInstance 验证对不存在实例的减成员错误透传。
// @param t 测试上下文
func TestAddSign_RemoveMemberGhostInstance(t *testing.T) {
	fx := newAddsignFixture(t)
	err := fx.addSign.RemoveMultiInstanceMember(context.Background(), "ghost-instance", "csTask", "carol", "", false)
	if err == nil || !strings.Contains(err.Error(), "流程实例不存在") {
		t.Fatalf("实例不存在应报错: %v", err)
	}
}
