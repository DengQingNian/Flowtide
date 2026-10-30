package migration

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestConvertReviewStandaloneHistoricTask 验证独立历史任务的租户、表单及删除原因经过产物重启后完整保留。
// @param t 测试上下文
// @return 无
func TestConvertReviewStandaloneHistoricTask(t *testing.T) {
	source := conversionFixture(t, "a")
	started := time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)
	row := convertReviewRow(t, "ACT_HI_TASKINST", map[string]any{"ID_": "historic-standalone", "NAME_": "历史独立任务", "ASSIGNEE_": "handler", "TENANT_ID_": "a", "START_TIME_": started, "END_TIME_": started.Add(time.Minute), "DURATION_": int64(60000), "FORM_KEY_": "approval-form", "DELETE_REASON_": "approved", "OWNER_": "owner", "DESCRIPTION_": "业务说明", "COMPLETED_BY_": "completer", "PRIORITY_": int64(60), "CATEGORY_": "audit", "DUE_DATE_": started.Add(time.Hour), "CLAIM_TIME_": started.Add(time.Second)})
	source.Rows = append(source.Rows, row)
	source.Tables = append(source.Tables, TableSummary{Name: row.Table, Count: 1})
	data, report, err := Convert(context.Background(), source, Options{})
	if report.Blocked {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	workflow := convertReviewEngine(t, data)
	tasks, err := workflow.ListHistoricTaskInstances(tenant.With(context.Background(), "a"), "")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("独立历史任务丢失租户归属：%+v %v", tasks, err)
	}
	if tasks[0].FormKey != "approval-form" || tasks[0].DeleteReason != "approved" {
		t.Fatalf("历史任务元数据丢失：%+v", tasks[0])
	}
	defaultTasks, err := workflow.ListHistoricTaskInstances(tenant.With(context.Background(), ""), "")
	if err != nil || len(defaultTasks) != 0 {
		t.Fatalf("历史任务暴露给默认租户：%+v %v", defaultTasks, err)
	}
	exported, err := workflow.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 非空 Java 业务列必须保留；无法保留时应在准备状态之前阻断。
	for _, restored := range exported {
		if restored.Table == row.Table && analysisID(restored) == analysisID(row) {
			for _, column := range []string{"FORM_KEY_", "DELETE_REASON_", "OWNER_", "DESCRIPTION_", "COMPLETED_BY_", "PRIORITY_", "CATEGORY_", "DUE_DATE_", "CLAIM_TIME_", "TENANT_ID_"} {
				if !reflect.DeepEqual(restored.Values[column], row.Values[column]) {
					t.Errorf("历史任务列 %s 丢失：%#v != %#v", column, restored.Values[column], row.Values[column])
				}
			}
			return
		}
	}
	t.Fatal("产物中缺少源历史任务行")
}

// TestConvertReviewHistoricActivityColumns 验证活动中的原执行、任务、处理人及顺序信息不会随重复容器排除而丢失。
// @param t 测试上下文
// @return 无
func TestConvertReviewHistoricActivityColumns(t *testing.T) {
	source := conversionFixture(t, "a")
	var task flowable.Row
	for _, row := range source.Rows {
		if row.Table == "ACT_RU_TASK" {
			task = row
			break
		}
	}
	started := task.Values["CREATE_TIME_"]
	values := map[string]any{"ID_": "historic-wait", "PROC_DEF_ID_": task.Values["PROC_DEF_ID_"], "PROC_INST_ID_": task.Values["PROC_INST_ID_"], "EXECUTION_ID_": task.Values["EXECUTION_ID_"], "TASK_ID_": task.Values["ID_"], "ACT_ID_": "wait", "ACT_TYPE_": "userTask", "ACT_NAME_": "审批", "ASSIGNEE_": "handler", "TENANT_ID_": "a", "START_TIME_": started, "TRANSACTION_ORDER_": int64(3)}
	history := convertReviewRow(t, "ACT_HI_ACTINST", values)
	source.Rows = append(source.Rows, history)
	source.Tables = append(source.Tables, TableSummary{Name: history.Table, Count: 1})
	// Java 运行时活动具有相同业务列，转换器只有在预检证明等价后才能排除它。
	runtimeValues := make(map[string]any, len(history.Values))
	for key, value := range history.Values {
		runtimeValues[key] = value
	}
	runtimeValues["REV_"] = int64(2)
	source.Rows = append(source.Rows, flowable.Row{Table: "ACT_RU_ACTINST", Values: runtimeValues})
	source.Tables = append(source.Tables, TableSummary{Name: "ACT_RU_ACTINST", Count: 1})
	data, report, err := Convert(context.Background(), source, Options{})
	if report.Blocked {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	workflow := convertReviewEngine(t, data)
	exported, err := workflow.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range exported {
		if row.Table == history.Table && analysisID(row) == analysisID(history) {
			for _, column := range []string{"PROC_DEF_ID_", "EXECUTION_ID_", "TASK_ID_", "ASSIGNEE_", "TRANSACTION_ORDER_", "TENANT_ID_"} {
				if !reflect.DeepEqual(row.Values[column], history.Values[column]) {
					t.Errorf("历史活动列 %s 丢失：%#v != %#v", column, row.Values[column], history.Values[column])
				}
			}
			return
		}
	}
	t.Fatal("产物中缺少源历史活动行")
}

// convertReviewRow 创建具有全部受支持实际列的数据库读取形态测试行。
// @param t 测试上下文
// @param table 表名
// @param values 非空测试列
// @return 数据行
func convertReviewRow(t *testing.T, table string, values map[string]any) flowable.Row {
	t.Helper()
	row, err := flowable.NewRow(table)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range row.Columns() {
		row.Values[column] = nil
	}
	for column, value := range values {
		row.Values[column] = value
	}
	return *row
}

// convertReviewEngine 从实际离线状态产物新建引擎，排除转换进程内残留对象的影响。
// @param t 测试上下文
// @param data 状态产物
// @return 重新恢复的独立引擎
func convertReviewEngine(t *testing.T, data []byte) *engine.Engine {
	t.Helper()
	store := statestore.NewMemory()
	if err := store.Save(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	workflow, err := engine.New(engine.Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}
