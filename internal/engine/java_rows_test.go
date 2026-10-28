package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// TestJavaShapeRootMetadata 验证子执行先排序时实例元数据仍从流程根恢复。
// @param t 测试上下文
// @return 无
func TestJavaShapeRootMetadata(t *testing.T) {
	rows, instanceID, started := javaShapeRows(t)
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	instance, err := target.GetProcessInstance(context.Background(), instanceID)
	if err != nil || instance.BusinessKey != "java-order" || !instance.StartTime.Equal(started) {
		t.Fatalf("实例没有使用根执行元数据: %+v %v", instance, err)
	}
}

// TestJavaShapeCompletionHistory 验证无 Go 控制扩展的根子执行可结束并闭合原活动历史。
// @param t 测试上下文
// @return 无
func TestJavaShapeCompletionHistory(t *testing.T) {
	rows, instanceID, _ := javaShapeRows(t)
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	tasks := listTasks(t, target)
	if err := target.Complete(context.Background(), tasks[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	instance, err := target.GetProcessInstance(context.Background(), instanceID)
	if err != nil || !instance.Ended {
		t.Fatalf("Java 根子执行完成后实例未结束: %+v %v", instance, err)
	}
	activities, err := target.ListHistoricActivityInstances(context.Background(), instanceID)
	if err != nil {
		t.Fatal(err)
	}
	// 原有等待活动必须原地结束，不得留下一条永久未闭合的记录。
	for _, activity := range activities {
		if activity.EndTime == nil {
			t.Fatalf("导入的活动历史没有闭合: %+v", activity)
		}
	}
}

// TestJavaShapeHistoricVariableUpdate 验证原始历史变量更新沿用原记录标识且不新增重复变量。
// @param t 测试上下文
// @return 无
func TestJavaShapeHistoricVariableUpdate(t *testing.T) {
	rows, instanceID, _ := javaShapeRows(t)
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	before, err := target.ListHistoricVariableInstances(context.Background(), instanceID)
	if err != nil || len(before) != 1 {
		t.Fatalf("原始变量历史不完整: %+v %v", before, err)
	}
	if err := target.SetExecutionVariable(context.Background(), instanceID, "initiator", "bob"); err != nil {
		t.Fatal(err)
	}
	after, err := target.ListHistoricVariableInstances(context.Background(), instanceID)
	if err != nil || len(after) != 1 || after[0].ID != before[0].ID || after[0].Value != "bob" {
		t.Fatalf("更新变量产生重复历史或丢失标识: %+v %v", after, err)
	}
}

// TestJavaShapeRejectsComplexTree 验证无控制扩展的复杂执行树不能被推测为简单串行实例。
// @param t 测试上下文
// @return 无
func TestJavaShapeRejectsComplexTree(t *testing.T) {
	for _, change := range []string{"多叶执行", "子作用域", "有活动的父执行"} {
		t.Run(change, func(t *testing.T) {
			rows, _, _ := javaShapeRows(t)
			// 模拟缺少转换控制的原始树，导入必须作为一条命令整体失败。
			for i := range rows {
				if rows[i].Table != "ACT_RU_EXECUTION" {
					continue
				}
				if rows[i].Values["ID_"] == "000-java-child" {
					if change == "多叶执行" {
						copy := flowablerow.Row{Table: rows[i].Table, Values: make(map[string]any)}
						for key, value := range rows[i].Values {
							copy.Values[key] = value
						}
						copy.Values["ID_"] = "001-java-child"
						rows = append(rows, copy)
					} else if change == "子作用域" {
						rows[i].Values["IS_SCOPE_"] = true
					}
				} else if change == "有活动的父执行" {
					rows[i].Values["ACT_ID_"] = "approve"
				}
			}
			target := newTestEngine(t)
			if err := target.RestoreRows(context.Background(), rows); err == nil {
				t.Fatal("复杂原始执行树应拒绝")
			}
			if len(listTasks(t, target)) != 0 {
				t.Fatal("失败导入残留任务")
			}
		})
	}
}

// TestJavaShapeRejectsAmbiguousHistory 验证同一执行的多个未结束活动历史会阻断恢复。
// @param t 测试上下文
// @return 无
func TestJavaShapeRejectsAmbiguousHistory(t *testing.T) {
	rows, _, _ := javaShapeRows(t)
	// 复制当前等待记录，不能随机选择其中一条当作续跑关联。
	for _, row := range rows {
		if row.Table == "ACT_HI_ACTINST" && row.Values["END_TIME_"] == nil {
			copy := flowablerow.Row{Table: row.Table, Values: make(map[string]any)}
			for key, value := range row.Values {
				copy.Values[key] = value
			}
			copy.Values["ID_"] = "duplicate-active-history"
			rows = append(rows, copy)
			break
		}
	}
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err == nil {
		t.Fatal("歧义活动历史应拒绝")
	}
}

// TestJavaShapeTaskLocalHistory 验证流程任务局部变量更新保留原历史记录标识和值。
// @param t 测试上下文
// @return 无
func TestJavaShapeTaskLocalHistory(t *testing.T) {
	rows, instanceID, _ := javaShapeRows(t)
	target := newTestEngine(t)
	if err := target.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	task := listTasks(t, target)[0]
	if err := target.SetTaskVariable(context.Background(), task.ID, "local-note", "初值"); err != nil {
		t.Fatal(err)
	}
	before, err := target.ListHistoricVariableInstances(context.Background(), instanceID)
	if err != nil || len(before) != 2 {
		t.Fatalf("流程任务局部变量未记录历史: %+v %v", before, err)
	}
	localHistoryID := ""
	for _, variable := range before {
		if variable.Name == "local-note" {
			localHistoryID = variable.ID
		}
	}
	rows, err = target.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restarted := newTestEngine(t)
	if err := restarted.RestoreRows(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	if err := restarted.SetTaskVariable(context.Background(), task.ID, "local-note", "新值"); err != nil {
		t.Fatal(err)
	}
	after, err := restarted.ListHistoricVariableInstances(context.Background(), instanceID)
	if err != nil || len(after) != 2 {
		t.Fatalf("恢复后局部变量历史重复: %+v %v", after, err)
	}
	// 任务归属与原主键均须保留，不能变成执行全局变量。
	for _, variable := range after {
		if variable.Name == "local-note" && (variable.TaskID != task.ID || variable.ID != localHistoryID || variable.Value != "新值") {
			t.Fatalf("任务局部历史归属或内容丢失: %+v", variable)
		}
	}
}

// javaShapeRows 根据源码布局建立无控制扩展的根子执行回归行，不能作为 Java 真实生成证据。
// @param t 测试上下文
// @return 回归行集合
// @return 实例标识
// @return 根执行开始时间
func javaShapeRows(t *testing.T) ([]flowablerow.Row, string, time.Time) {
	t.Helper()
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	instance, err := source.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", BusinessKey: "java-order", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := source.SnapshotRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result := make([]flowablerow.Row, 0, len(rows)+1)
	// Java 只在流程根保存业务键和开始时间，叶执行的标识故意排在根之前。
	for _, row := range rows {
		if row.Table == "ACT_GE_BYTEARRAY" && strings.HasPrefix(row.Values["NAME_"].(string), "flowable-go-control/") {
			continue
		}
		switch row.Table {
		case "ACT_RU_EXECUTION":
			child := flowablerow.Row{Table: row.Table, Values: make(map[string]any, len(row.Values))}
			for key, value := range row.Values {
				child.Values[key] = value
			}
			child.Values["ID_"] = "000-java-child"
			child.Values["PARENT_ID_"] = instance.ID
			child.Values["BUSINESS_KEY_"] = nil
			child.Values["START_TIME_"] = nil
			child.Values["IS_SCOPE_"] = false
			row.Values["ACT_ID_"] = nil
			row.Values["IS_ACTIVE_"] = false
			result = append(result, child)
		case "ACT_RU_TASK", "ACT_HI_ACTINST":
			row.Values["EXECUTION_ID_"] = "000-java-child"
		}
		result = append(result, row)
	}
	return result, instance.ID, instance.StartTime
}
