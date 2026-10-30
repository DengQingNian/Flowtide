package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// TestAuditRowTaskRecovery 验证候选人、委派状态和完成监听器在恢复后仍能使用。
// @param t 测试上下文
// @return 无
func TestAuditRowTaskRecovery(t *testing.T) {
	app := newTestEngine(t)
	xml := strings.Replace(taskListenerXML, `flowable:assignee="alice"`, `flowable:assignee="alice" flowable:candidateUsers="bob" flowable:candidateGroups="reviewers"`, 1)
	deployXML(t, app, xml)
	app.RegisterTaskListener("onCreate", func(ctx context.Context, task *api.Task) error { return nil })
	if _, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "listenerProcess"}); err != nil {
		t.Fatal(err)
	}
	task := listTasks(t, app)[0]
	if err := app.Delegate(context.Background(), task.ID, "carol"); err != nil {
		t.Fatal(err)
	}
	rows, err := app.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestEngine(t)
	calls := 0
	restored.RegisterTaskListener("onComplete", func(ctx context.Context, task *api.Task) error { calls++; return nil })
	if err := restored.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	got, err := restored.GetTask(context.Background(), task.ID)
	if err != nil || !reflect.DeepEqual(got.CandidateUsers, []string{"bob"}) || !reflect.DeepEqual(got.CandidateGroups, []string{"reviewers"}) || got.DelegationState != "PENDING" {
		t.Fatalf("任务恢复不完整: %+v %v", got, err)
	}
	if err := restored.Resolve(context.Background(), task.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := restored.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("完成监听器执行次数: %d", calls)
	}
	historic, err := restored.GetHistoricTaskInstance(context.Background(), task.ID)
	if err != nil || historic.EndTime == nil {
		t.Fatalf("任务历史未完成: %+v %v", historic, err)
	}
}

// TestAuditBinaryRows 验证运行时和历史变量的二进制及长文本往返。
// @param t 测试上下文
// @return 无
func TestAuditBinaryRows(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	expected := map[string]any{"binary": []byte{1, 2, 3}, "long": strings.Repeat("长", 5000)}
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "binary", Value: expected["binary"]}, {Name: "long", Value: expected["long"]}}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := app.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	for name, value := range expected {
		got, err := restored.GetVariable(context.Background(), instance.ID, name)
		if err != nil || !reflect.DeepEqual(got, value) {
			t.Fatalf("变量 %s 往返错误: %T %v", name, got, err)
		}
	}
	historic, err := restored.ListHistoricVariableInstances(context.Background(), instance.ID)
	if err != nil || len(historic) != 2 {
		t.Fatalf("历史变量恢复错误: %v", err)
	}
	for _, variable := range historic {
		if !reflect.DeepEqual(variable.Value, expected[variable.Name]) {
			t.Fatalf("历史变量类型错误: %s %T", variable.Name, variable.Value)
		}
	}
}

// TestAuditUnknownVariableRejected 验证不可读 Java 序列化类型明确阻断且恢复不留部分数据。
// @param t 测试上下文
// @return 无
func TestAuditUnknownVariableRejected(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "x", Value: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := source.SnapshotRows(context.Background())
	for i := range rows {
		if rows[i].Table == "ACT_RU_VARIABLE" {
			rows[i].Values["TYPE_"] = "serializable"
		}
	}
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err == nil {
		t.Fatal("未知变量类型必须阻断")
	}
	if _, err := target.GetProcessInstance(context.Background(), instance.ID); err == nil {
		t.Fatal("失败恢复留下实例")
	}
	if _, err := runtimeVariableValue(flowablerow.RuntimeVariable{Type: "serializable"}); err == nil {
		t.Fatal("未知变量不得静默为空")
	}
}

// TestAuditMultiInstanceResume 验证并行和串行多实例恢复后可以完成全部任务与历史。
// @param t 测试上下文
// @return 无
func TestAuditMultiInstanceResume(t *testing.T) {
	for _, test := range []struct{ xml, key string }{{parallelMIXML, "miParallel"}, {sequentialMIXML, "miSequential"}} {
		t.Run(test.key, func(t *testing.T) {
			source := newTestEngine(t)
			deployXML(t, source, test.xml)
			instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: test.key, Variables: []api.Variable{{Name: "approvers", Value: []string{"alice", "bob"}}}})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := source.SnapshotRows(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			target := newTestEngine(t)
			if err := target.RestoreRows(context.Background(), rows); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				tasks := listTasks(t, target)
				if len(tasks) == 0 {
					t.Fatal("多实例任务提前丢失")
				}
				if err := target.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := target.GetProcessInstance(context.Background(), instance.ID)
			if err != nil || !got.Ended {
				t.Fatalf("多实例恢复后未结束: %+v %v", got, err)
			}
			activities, err := target.ListHistoricActivityInstances(context.Background(), instance.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, activity := range activities {
				if activity.EndTime == nil {
					t.Fatalf("活动历史未闭合: %+v", activity)
				}
			}
		})
	}
}
