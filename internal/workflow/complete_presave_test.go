// complete_presave_test.go 验证办结失败时的预存下一步补偿清理（016 号审计 B3）。
//
// 背景：预存 workflow_next_activity（used=0）后若引擎办结命令失败，记录会残留，
// 下次同节点任务创建时被当作执行人消费导致错派。本文件锁定补偿语义：
// 失败路径清理本次预存，成功路径保留预存与历史双写。
package workflow

import (
	"testing"

	"github.com/DengQingNian/Flowtide/api"
)

// countUnusedNext 统计实例下指定节点的未消耗记录数。
// @param t 测试上下文
// @param fx 测试环境
// @param instanceID 实例 Id
// @param activityID 节点 Id
// @return 记录数
func countUnusedNext(t *testing.T, fx *completeFixture, instanceID, activityID string) int {
	t.Helper()
	rows, err := fx.next.ListUnusedByInstanceAndActivity(ctxB, instanceID, activityID)
	if err != nil {
		t.Fatalf("查询未消耗记录失败: %v", err)
	}
	return len(rows)
}

// presaveCompleteRequest 构造带下一步预存的办结请求。
// @param taskID 任务 Id
// @return 办结请求
func presaveCompleteRequest(taskID string) *api.CompleteTaskVo {
	return &api.CompleteTaskVo{
		BaseProcessVo: api.BaseProcessVo{TaskID: taskID, UserCode: "alice", Message: "同意"},
		Request: &api.NextActivityRequest{
			TaskID: taskID,
			NextActivityList: []*api.NextActivityVo{{
				ActivityID:  "handle",
				ExecutorIDs: []string{"alice"},
			}},
		},
	}
}

// TestComplete_PresaveRetainedOnSuccess 验证成功办结时预存记录与历史双写均保留。
// fixture 未注册执行人重写钩子，因此预存记录不会被消费，可用于对比失败路径的清理。
// @param t 测试上下文
func TestComplete_PresaveRetainedOnSuccess(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, task := fx.start(t, "flowComplete", "biz-comp-2", "alice")
	if _, err := fx.complete.Complete(ctxB, presaveCompleteRequest(task.ID)); err != nil {
		t.Fatalf("办结应成功: %v", err)
	}
	// 成功路径保留预存记录（等待新任务创建时由钩子消费）
	if kept := countUnusedNext(t, fx, instance.ID, "handle"); kept != 1 {
		t.Fatalf("成功办结应保留 1 条预存记录，实际 %d", kept)
	}
	// 历史双写同样保留
	his, err := fx.next.ListHisByInstance(ctxB, instance.ID)
	if err != nil {
		t.Fatalf("查询历史记录失败: %v", err)
	}
	if len(his) == 0 {
		t.Fatalf("成功办结不应清理历史双写记录")
	}
}

// TestComplete_PresaveCompensatedOnEngineFailure 验证引擎办结失败后预存记录被清理。
// 构造：首次办结成功后对同一任务重复办结，预存先写入而引擎命令必然失败。
// @param t 测试上下文
func TestComplete_PresaveCompensatedOnEngineFailure(t *testing.T) {
	fx := newCompleteFixture(t)
	instance, task := fx.start(t, "flowComplete", "biz-comp-1", "alice")

	if _, err := fx.complete.Complete(ctxB, presaveCompleteRequest(task.ID)); err != nil {
		t.Fatalf("首次办结应成功: %v", err)
	}
	before := countUnusedNext(t, fx, instance.ID, "handle")
	if before != 1 {
		t.Fatalf("首次办结后应保留 1 条预存记录，实际 %d", before)
	}

	// 对已完成任务重复办结：预存先写入（会叠加），引擎命令失败后补偿应恢复到 before
	if _, err := fx.complete.Complete(ctxB, presaveCompleteRequest(task.ID)); err == nil {
		t.Fatalf("对已完成任务重复办结应失败")
	}
	after := countUnusedNext(t, fx, instance.ID, "handle")
	if after != before {
		t.Fatalf("办结失败后应补偿清理本次预存：失败前 %d，失败后 %d", before, after)
	}
}
