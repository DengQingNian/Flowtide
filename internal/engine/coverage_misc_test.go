package engine

import (
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// TestServiceGetters 验证服务获取方法。
// @param t 测试上下文
func TestServiceGetters(t *testing.T) {
	engine := newTestEngine(t)
	if engine.FormService() == nil {
		t.Fatalf("表单服务不能为空")
	}
	if engine.IdentityService() == nil {
		t.Fatalf("身份服务不能为空")
	}
}

// TestRowHelpers 验证行辅助函数。
// @param t 测试上下文
func TestRowHelpers(t *testing.T) {
	if rowID(flowablerow.Row{}) != "" {
		t.Fatalf("无 ID 行应返回空串")
	}
	if _, err := runtimeRow("BAD", []string{"ID_"}); err == nil {
		t.Fatalf("非法表应失败")
	}
	if _, err := runtimeRow("ACT_RU_JOB", []string{"BAD_"}); err == nil {
		t.Fatalf("非法列应失败")
	}
}

// TestSortJobInfos 验证作业排序同到期时间分支。
// @param t 测试上下文
func TestSortJobInfos(t *testing.T) {
	due := time.Now()
	jobs := []api.JobInfo{{ID: "b", DueDate: due}, {ID: "a", DueDate: due}}
	sortJobInfos(jobs)
	if jobs[0].ID != "a" {
		t.Fatalf("同到期时间应按 ID 排序")
	}
}

// TestDeleteStartArtifacts 验证启动工件清理。
// @param t 测试上下文
func TestDeleteStartArtifacts(t *testing.T) {
	engine := newTestEngine(t)
	engine.state.Subscriptions["s"] = &subscriptionState{ID: "s", ProcessDefinitionID: "d"}
	engine.state.Jobs["j"] = &jobState{ID: "j", ProcessDefinitionID: "d"}
	deleteStartArtifacts(engine.state, "d")
	if len(engine.state.Subscriptions) != 0 || len(engine.state.Jobs) != 0 {
		t.Fatalf("启动工件未清理")
	}
}
