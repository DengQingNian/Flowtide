package engine

import (
	"context"
	"os"
	"testing"

	"github.com/flowable-go/flowable/api"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// TestLegacySingleRootStateRestore 验证真实旧版单根状态仍能加载、转成行状态并完成，不隐式重写执行标识。
// @param t 测试上下文
// @return 无
func TestLegacySingleRootStateRestore(t *testing.T) {
	ctx := context.Background()
	data, err := os.ReadFile("../../testdata/compatibility/go/001-single-root-state.json")
	if err != nil {
		t.Fatal(err)
	}
	store := statestore.NewMemory()
	if err := store.Save(ctx, data); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := app.ListProcessInstances(ctx)
	if err != nil || len(instances) != 1 {
		t.Fatalf("旧版实例缺失: %v", err)
	}
	instance := instances[0]
	executions, err := app.ListExecutions(ctx, instance.ID)
	if err != nil || len(executions) != 1 || executions[0].ID != instance.ID || executions[0].ActivityID != "approve" {
		t.Fatalf("旧版执行标识被隐式改写: %+v %v", executions, err)
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	// 文件状态与重新导出的行状态分别完成，覆盖旧布局的两条续跑路径。
	for _, candidate := range []*Engine{app, restored} {
		values, err := candidate.GetVariables(ctx, instance.ID)
		if err != nil || values["marker"] != "旧版状态" {
			t.Fatalf("旧变量丢失: %+v %v", values, err)
		}
		tasks, err := candidate.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
		if err != nil || len(tasks.Data) != 1 || tasks.Data[0].ExecutionID != instance.ID {
			t.Fatalf("旧任务绑定丢失: %+v %v", tasks, err)
		}
		if err := candidate.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatal(err)
		}
		if result, err := candidate.GetProcessInstance(ctx, instance.ID); err != nil || !result.Ended || result.BusinessKey != "legacy-business" {
			t.Fatalf("旧状态续跑未完成: %+v %v", result, err)
		}
	}
}
