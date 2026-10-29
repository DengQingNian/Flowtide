package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
	"github.com/flowable-go/flowable/internal/engine"
)

// runtimeJavaCase 保存原版运行时请求、完整响应及执行后的资源和变量。
type runtimeJavaCase struct {
	Name           string                    `json:"name"`           // 场景名
	Method         string                    `json:"method"`         // HTTP 方法
	Path           string                    `json:"path"`           // 路径模板
	RequestBody    string                    `json:"requestBody"`    // 请求模板
	Prepare        string                    `json:"prepare"`        // 初始场景状态
	Status         int                       `json:"status"`         // 原版状态码
	ResponseBody   string                    `json:"responseBody"`   // 原版完整正文
	AfterInstances map[string]string         `json:"afterInstances"` // 存活实例的原版 GET 正文
	AfterVariables map[string]map[string]any `json:"afterVariables"` // 原版变量值
	AfterTaskCount int64                     `json:"afterTaskCount"` // 原版运行任务数
}

// TestJavaRuntimeRESTReference 对照原版实例和执行控制器的完整契约及实际副作用。
// @param t 测试上下文
// @return 无
func TestJavaRuntimeRESTReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/005-runtime-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline    string            `json:"baseline"`
		Clock       time.Time         `json:"clock"`
		StartUserID string            `json:"startUserId"`
		Models      map[string]string `json:"models"`
		Cases       []runtimeJavaCase `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Cases) != 42 {
		t.Fatal("运行时Java基线或场景数错误")
	}
	for _, scenario := range fixture.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			ctx := context.Background()
			app, err := engine.New(engine.Config{Clock: clock.NewFake(fixture.Clock), IDs: &runtimeJavaIDs{}, HistoryLevel: engine.HistoryFull})
			if err != nil {
				t.Fatal(err)
			}
			resources := map[string][]byte{}
			for name, xml := range fixture.Models {
				resources[name] = []byte(xml)
			}
			if _, err := app.Deploy(ctx, api.DeploymentRequest{Resources: resources}); err != nil {
				t.Fatal(err)
			}
			ids := map[string]string{}
			definitions, err := app.ListProcessDefinitions(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, definition := range definitions {
				switch definition.Key {
				case "runtime_reference":
					ids["{definitionId}"] = definition.ID
				case "message_reference":
					ids["{messageDefinitionId}"] = definition.ID
				case "receive_reference":
					ids["{receiveDefinitionId}"] = definition.ID
				}
			}
			for index, seed := range []struct {
				name, key string
				amount    int64
			}{{"实例甲", "alpha", 9007199254740993}, {"实例乙", "beta", 42}} {
				instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "runtime_reference", Name: seed.name, BusinessKey: seed.key, UserID: fixture.StartUserID, Variables: []api.Variable{{Name: "amount", Value: seed.amount}, {Name: "flag", Value: "before"}}})
				if err != nil {
					t.Fatal(err)
				}
				label, childLabel := "{instanceId}", "{executionId}"
				if index == 1 {
					label, childLabel = "{secondId}", "{secondExecutionId}"
				}
				ids[label] = instance.ID
				ids[childLabel] = runtimeJavaChild(t, app, instance.ID)
			}
			handler := New(app, WithBasicAuth(map[string]string{fixture.StartUserID: "reference-secret"})).Handler()
			if scenario.Prepare == "suspended" {
				if err := app.SuspendProcessInstance(ctx, ids["{instanceId}"]); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.Prepare == "claimed" {
				w := runtimeJavaRequest(handler, fixture.StartUserID, "PUT", "/service/runtime/process-instances/"+ids["{instanceId}"], `{"action":"claim","assignee":"alice"}`)
				if w.Code != 200 {
					t.Fatalf("签收初始状态未建立: %d %s", w.Code, w.Body.String())
				}
			}
			if scenario.Prepare == "receive" {
				instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "receive_reference", UserID: fixture.StartUserID})
				if err != nil {
					t.Fatal(err)
				}
				ids["{receiveId}"] = instance.ID
				ids["{receiveExecutionId}"] = runtimeJavaChild(t, app, instance.ID)
			}
			path, body := scenario.Path, scenario.RequestBody
			for label, id := range ids {
				path = strings.ReplaceAll(path, label, id)
				body = strings.ReplaceAll(body, label, id)
			}
			response := runtimeJavaRequest(handler, fixture.StartUserID, scenario.Method, path, body)
			if response.Code != scenario.Status {
				t.Errorf("状态码%d，原版%d", response.Code, scenario.Status)
			}
			instances, err := app.ListProcessInstances(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, instance := range instances {
				known := false
				for _, id := range ids {
					if id == instance.ID {
						known = true
						break
					}
				}
				if !known {
					ids["{createdId}"] = instance.ID
					ids["{createdExecutionId}"] = runtimeJavaChild(t, app, instance.ID)
				}
			}
			actual := runtimeJavaNormalize(response.Body.String(), ids)
			if scenario.ResponseBody == "" {
				if strings.TrimSpace(actual) != "" {
					t.Errorf("原版空正文被改为%s", actual)
				}
			} else {
				assertRuntimeJSON(t, []byte(actual), []byte(scenario.ResponseBody))
			}
			if len(instances) != len(scenario.AfterInstances) {
				t.Fatalf("实例副作用不同: 实际%d 原版%d", len(instances), len(scenario.AfterInstances))
			}
			// 每个存活实例重新经 HTTP 读取完整对象，并独立核对变量，防止只返回预期正文而漏掉状态更新。
			for _, instance := range instances {
				label := ""
				for expected, id := range ids {
					if id == instance.ID {
						label = expected
						break
					}
				}
				expected, ok := scenario.AfterInstances[label]
				if !ok {
					t.Fatalf("未知存活实例%s", label)
				}
				read := runtimeJavaRequest(handler, fixture.StartUserID, "GET", "/service/runtime/process-instances/"+instance.ID, "")
				if read.Code != 200 {
					t.Fatalf("实例读回失败%d", read.Code)
				}
				assertAsyncJSON(t, []byte(runtimeJavaNormalize(read.Body.String(), ids)), []byte(expected))
				variables, err := app.GetVariables(ctx, instance.ID)
				if err != nil {
					t.Fatal(err)
				}
				value, _ := json.Marshal(runtimeJavaValues(variables))
				want, _ := json.Marshal(scenario.AfterVariables[label])
				assertAsyncJSON(t, value, want)
			}
			tasks, err := app.ListTasks(ctx, api.TaskFilter{Size: 1000})
			if err != nil || tasks.Total != scenario.AfterTaskCount {
				t.Fatalf("任务副作用%d，原版%d，错误%v", tasks.Total, scenario.AfterTaskCount, err)
			}
		})
	}
}

// runtimeJavaChild 返回简单参考模型唯一活动子执行，禁止将根误当活动。
// @param t 测试上下文
// @param app 引擎
// @param instanceID 实例标识
// @return 活动执行标识
func runtimeJavaChild(t *testing.T, app *engine.Engine, instanceID string) string {
	t.Helper()
	executions, err := app.ListExecutions(context.Background(), instanceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, execution := range executions {
		if execution.ParentID == instanceID && !execution.Ended {
			return execution.ID
		}
	}
	t.Fatal("参考实例缺少活动子执行")
	return ""
}

// runtimeJavaRequest 以固定认证主体执行参考请求。
// @param handler 正式处理链
// @param user 认证主体
// @param method 方法
// @param path 路径
// @param body 请求正文
// @return 录制的完整响应
func runtimeJavaRequest(handler http.Handler, user, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetBasicAuth(user, "reference-secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// runtimeJavaNormalize 仅替换动态标识及其 URL 编码，保留其他正文和数组顺序。
// @param value 正文
// @param ids 占位符到实际标识
// @return 规范化后的正文
func runtimeJavaNormalize(value string, ids map[string]string) string {
	labels := make([]string, 0, len(ids))
	for label := range ids {
		labels = append(labels, label)
	}
	sort.Slice(labels, func(i, j int) bool { return len(ids[labels[i]]) > len(ids[labels[j]]) })
	for _, label := range labels {
		id := ids[label]
		value = strings.ReplaceAll(value, url.QueryEscape(id), label)
		value = strings.ReplaceAll(value, id, label)
	}
	return value
}

// runtimeJavaValues 将日期表示为原版样本标记，标量和整数保持原类型。
// @param value 实际变量
// @return 可无损 JSON 比较的值
func runtimeJavaValues(value any) any {
	switch value := value.(type) {
	case time.Time:
		return map[string]string{"$date": value.UTC().Format(time.RFC3339Nano)}
	case map[string]any:
		out := map[string]any{}
		for key, item := range value {
			out[key] = runtimeJavaValues(item)
		}
		return out
	default:
		return value
	}
}

// runtimeJavaIDs 与 Java 运行器一样生成单调标识，使默认标识排序能够逐项比较。
type runtimeJavaIDs struct {
	next int // 当前序号
}

// NewID 返回固定宽度的递增标识，不依赖宿主机时钟或随机数。
// @return 二十位标识
func (g *runtimeJavaIDs) NewID() string {
	g.next++
	return fmt.Sprintf("%020d", g.next)
}

// assertRuntimeJSON 比较完整 DTO；仅变量集合按作用域和名称排序，不改变分页或执行数组顺序。
// @param t 测试上下文
// @param actual Go 完整正文
// @param expected Java 完整正文
// @return 无
func assertRuntimeJSON(t *testing.T, actual, expected []byte) {
	t.Helper()
	canonical := func(data []byte) []byte {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		canonicalRuntimeVariables(value)
		canonicalHistoryArrays(value)
		out, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	assertAsyncJSON(t, canonical(actual), canonical(expected))
}

// canonicalRuntimeVariables 只规范实例对象的变量集合；变量值中的 JSON 数组保持原顺序。
// @param value 响应对象或分页对象
// @return 无
func canonicalRuntimeVariables(value any) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	if data, ok := object["data"].([]any); ok {
		for _, item := range data {
			canonicalRuntimeVariables(item)
		}
	}
	if _, instance := object["processDefinitionUrl"]; !instance {
		return
	}
	variables, ok := object["variables"].([]any)
	if !ok {
		return
	}
	// 原版集合由 Java Map 遍历产生，比较集合成员而不依赖哈希桶顺序。
	sort.SliceStable(variables, func(i, j int) bool {
		left, lok := variables[i].(map[string]any)
		right, rok := variables[j].(map[string]any)
		if !lok || !rok {
			return false
		}
		return fmt.Sprint(left["scope"], left["name"]) < fmt.Sprint(right["scope"], right["name"])
	})
}
