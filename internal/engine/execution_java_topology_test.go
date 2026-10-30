package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/tenant"
)

// TestJavaExecutionTopologyReference 对照原版实际执行行，验证新实例根作用域、活动执行及恢复后的完成行为。
// @param t 测试上下文
// @return 无
func TestJavaExecutionTopologyReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/001-user-task-audit.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline  string `json:"baseline"`
		Scenarios []struct {
			Before struct {
				Rows []struct {
					Table  string                     `json:"table"`
					Values map[string]json.RawMessage `json:"values"`
				} `json:"rows"`
			} `json:"before"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Scenarios) == 0 {
		t.Fatal("Java执行树基线缺失")
	}
	type signature struct {
		Activity string
		Parent   bool
		Scope    bool
		Active   bool
	}
	want := []signature{}
	resources := map[string][]byte{}
	for _, row := range fixture.Scenarios[0].Before.Rows {
		if row.Table == "ACT_GE_BYTEARRAY" {
			var name string
			_ = json.Unmarshal(row.Values["NAME_"], &name)
			if strings.HasSuffix(name, ".bpmn20.xml") {
				var encoded struct {
					Bytes string `json:"$bytes"`
				}
				if err := json.Unmarshal(row.Values["BYTES_"], &encoded); err != nil {
					t.Fatal(err)
				}
				xml, err := base64.StdEncoding.DecodeString(encoded.Bytes)
				if err != nil {
					t.Fatal(err)
				}
				resources[name] = xml
			}
		}
		if row.Table == "ACT_RU_EXECUTION" {
			var item signature
			var parent string
			_ = json.Unmarshal(row.Values["ACT_ID_"], &item.Activity)
			_ = json.Unmarshal(row.Values["PARENT_ID_"], &parent)
			_ = json.Unmarshal(row.Values["IS_SCOPE_"], &item.Scope)
			_ = json.Unmarshal(row.Values["IS_ACTIVE_"], &item.Active)
			item.Parent = parent != ""
			want = append(want, item)
		}
	}
	if len(want) != 2 || len(resources) != 1 {
		t.Fatal("Java样本不包含完整根与活动执行证据")
	}
	ctx := tenant.With(context.Background(), "tenant-a")
	app := newTestEngine(t)
	if _, err := app.Deploy(ctx, api.DeploymentRequest{TenantID: "tenant-a", Resources: resources}); err != nil {
		t.Fatal(err)
	}
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "java_user_audit", BusinessKey: "java-business", TenantID: "tenant-a", Variables: []api.Variable{{Name: "global", Value: "根变量"}}})
	if err != nil {
		t.Fatal(err)
	}
	actual := []signature{}
	for _, execution := range app.state.Executions {
		if execution.ProcessInstanceID == instance.ID && !execution.Ended {
			actual = append(actual, signature{Activity: execution.ActivityID, Parent: execution.ParentID != "", Scope: execution.Scope, Active: execution.Active})
		}
	}
	sort.Slice(want, func(i, j int) bool { return want[i].Activity < want[j].Activity })
	sort.Slice(actual, func(i, j int) bool { return actual[i].Activity < actual[j].Activity })
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("新实例执行树与Java不同: 实际=%+v 原版=%+v", actual, want)
	}
	tasks, err := app.ListTasks(ctx, api.TaskFilter{})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("活动任务: %v", err)
	}
	if tasks.Data[0].ExecutionID == instance.ID {
		t.Fatal("任务仍绑定到实例根执行")
	}
	if err := app.SetExecutionVariable(ctx, tasks.Data[0].ExecutionID, "local", "活动变量"); err != nil {
		t.Fatal(err)
	}
	if values, err := app.GetVariables(ctx, instance.ID); err != nil || values["local"] != nil || values["global"] != "根变量" {
		t.Fatalf("活动局部变量污染根作用域: %+v %v", values, err)
	}
	rows, err := app.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 原版根活动和子执行业务键为 SQL NULL，空字符串会改变按空值筛选的实际语义。
	for _, row := range rows {
		if row.Table != "ACT_RU_EXECUTION" {
			continue
		}
		if row.Values["ID_"] == instance.ID && row.Values["ACT_ID_"] != nil {
			t.Fatal("根执行活动没有编码为SQL空值")
		}
		if row.Values["ID_"] != instance.ID && row.Values["BUSINESS_KEY_"] != nil {
			t.Fatal("子执行重复保存了流程根业务键")
		}
	}
	restored := newTestEngine(t)
	if err := restored.RestoreRows(ctx, rows); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		page, err := restored.ListTasks(ctx, api.TaskFilter{})
		if err != nil || len(page.Data) != 1 {
			t.Fatalf("恢复后的任务推进: %v %+v", err, page)
		}
		if err := restored.Complete(ctx, page.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if result, err := restored.GetProcessInstance(ctx, instance.ID); err != nil || !result.Ended {
		t.Fatalf("恢复后根作用域未结束: %+v %v", result, err)
	}
}
