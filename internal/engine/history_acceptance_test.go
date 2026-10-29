package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestHistoryLevelAcceptance 验证六级历史的流程、活动、任务、变量与明细写入矩阵。
// @param t 测试上下文
// @return 无
func TestHistoryLevelAcceptance(t *testing.T) {
	for _, level := range []string{HistoryNone, HistoryInstance, HistoryTask, HistoryActivity, HistoryAudit, HistoryFull} {
		t.Run(level, func(t *testing.T) {
			app, err := New(Config{HistoryLevel: level})
			if err != nil {
				t.Fatal(err)
			}
			deployXML(t, app, simpleProcess)
			instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := app.SetVariable(context.Background(), instance.ID, "initiator", "bob"); err != nil {
				t.Fatal(err)
			}
			processes, _ := app.ListHistoricProcessInstances(context.Background())
			activities, _ := app.ListHistoricActivityInstances(context.Background(), instance.ID)
			tasks, _ := app.ListHistoricTaskInstances(context.Background(), instance.ID)
			variables, _ := app.ListHistoricVariableInstances(context.Background(), instance.ID)
			details, _ := app.ListHistoricDetails(context.Background(), instance.ID)
			// 各级别独立断言，防止提高测试默认历史级别掩盖生产默认行为。
			if (len(processes) > 0) != (level != HistoryNone) || (len(activities) > 0) != app.historyAtLeast(HistoryActivity) || (len(tasks) > 0) != app.taskHistoryEnabled() || (len(variables) > 0) != app.historyAtLeast(HistoryActivity) {
				t.Fatalf("历史级别矩阵错误: 流程=%d 活动=%d 任务=%d 变量=%d", len(processes), len(activities), len(tasks), len(variables))
			}
			wantDetails := 0
			if level == HistoryFull {
				wantDetails = 2
			}
			if len(details) != wantDetails {
				t.Fatalf("变量更新明细=%d，期望=%d", len(details), wantDetails)
			}
		})
	}
}

// TestHistoricStandaloneTenantAcceptance 验证独立任务历史变量、明细查询及删除均隔离租户。
// @param t 测试上下文
// @return 无
func TestHistoricStandaloneTenantAcceptance(t *testing.T) {
	app, err := New(Config{HistoryLevel: HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	owner := tenant.With(context.Background(), "owner")
	foreign := tenant.With(context.Background(), "foreign")
	task, err := app.CreateTask(owner, api.CreateTaskRequest{Name: "租户任务"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.SetTaskVariable(owner, task.ID, "payload", "私有值"); err != nil {
		t.Fatal(err)
	}
	variables, err := app.ListHistoricVariableInstances(owner, "")
	if err != nil || len(variables) != 1 {
		t.Fatalf("历史变量: %+v %v", variables, err)
	}
	details, err := app.ListHistoricDetails(owner, "")
	if err != nil || len(details) != 1 {
		t.Fatalf("历史明细: %+v %v", details, err)
	}
	if _, err := app.GetHistoricTaskVariableByName(foreign, task.ID, "payload"); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("跨租户历史变量查询: %v", err)
	}
	if _, err := app.GetHistoricDetail(foreign, details[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("跨租户历史明细查询: %v", err)
	}
	if err := app.DeleteHistoricVariableInstance(foreign, variables[0].ID); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("跨租户历史变量删除: %v", err)
	}
	if _, err := app.GetHistoricVariableInstance(owner, variables[0].ID); err != nil {
		t.Fatalf("越权请求破坏了历史变量: %v", err)
	}
}

// TestHistoricOnlyLookupAcceptance 验证仅导入历史行后仍能按名称读取变量和明细。
// @param t 测试上下文
// @return 无
func TestHistoricOnlyLookupAcceptance(t *testing.T) {
	source, err := New(Config{HistoryLevel: HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	deployXML(t, source, simpleProcess)
	ctx := context.Background()
	instance, err := source.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
	if err != nil {
		t.Fatal(err)
	}
	task := listTasks(t, source)[0]
	if err := source.SetTaskVariable(ctx, task.ID, "local", "保留"); err != nil {
		t.Fatal(err)
	}
	if err := source.Complete(ctx, task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	rows, err := source.ExportHistoricRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target := newTestEngine(t)
	if err := target.ImportHistoricRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if variable, err := target.GetHistoricVariableByName(ctx, instance.ID, "initiator"); err != nil || variable.Value != "alice" {
		t.Errorf("已归档实例变量: %+v %v", variable, err)
	}
	if variable, err := target.GetHistoricTaskVariableByName(ctx, task.ID, "local"); err != nil || variable.Value != "保留" {
		t.Errorf("已归档任务变量: %+v %v", variable, err)
	}
	details, err := target.ListHistoricDetails(ctx, instance.ID)
	if err != nil || len(details) == 0 {
		t.Fatalf("归档明细: %+v %v", details, err)
	}
	if _, err := target.GetHistoricDetail(ctx, details[0].ID); err != nil {
		t.Errorf("归档明细读取: %v", err)
	}
}
