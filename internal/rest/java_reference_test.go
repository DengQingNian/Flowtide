package rest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/migration"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// javaHTTPExpected 保存真实 Java 公共服务的稳定业务字段，用于验收 HTTP 输出及写入副作用。
type javaHTTPExpected struct {
	ProcessInstanceID string `json:"processInstanceId"` // 原始实例标识
	BusinessKey       string `json:"businessKey"`       // 业务键
	TenantID          string `json:"tenantId"`          // 所属租户
	Ended             bool   `json:"ended"`             // 流程是否结束
	Tasks             []struct {
		Key      string `json:"key"`      // 任务定义键
		Name     string `json:"name"`     // 任务名称
		Assignee string `json:"assignee"` // 处理人
	} `json:"tasks"`
	HistoricTasks []struct {
		Key      string `json:"key"`      // 任务定义键
		Assignee string `json:"assignee"` // 处理人
		Ended    bool   `json:"ended"`    // 历史任务是否结束
	} `json:"historicTasks"`
	HistoricVariables []json.RawMessage `json:"historicVariables"` // 用于核对历史变量总数
}

// TestJavaReferenceHTTPPlayback 在真实 Java 等待点恢复后，通过 HTTP 完成任务并核对 Java 公共服务结果。
// @param t 测试上下文
// @return 无
func TestJavaReferenceHTTPPlayback(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/001-user-task-audit.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline  string `json:"baseline"`
		Scenarios []struct {
			Name           string           `json:"name"`
			Before         json.RawMessage  `json:"before"`
			BeforeExpected javaHTTPExpected `json:"beforeExpected"`
			Expected       javaHTTPExpected `json:"expected"`
		} `json:"scenarios"`
		Completed         json.RawMessage  `json:"completed"`
		CompletedExpected javaHTTPExpected `json:"completedExpected"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Scenarios) != 2 {
		t.Fatal("Java 对照基线错误")
	}
	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			s := restoreJavaHTTP(t, scenario.Before)
			assertJavaHTTP(t, s, scenario.BeforeExpected)
			page := javaHTTPRequest(t, s, http.MethodGet, "/service/runtime/tasks?processInstanceId="+scenario.BeforeExpected.ProcessInstanceID, "", http.StatusOK)
			tasks := page["data"].([]any)
			if len(tasks) != 1 {
				t.Fatalf("Java 等待点任务不唯一: %+v", tasks)
			}
			body := `{"action":"complete","variables":[{"name":"status","value":"completed"}]}`
			if scenario.Name == "approve" {
				body = `{"action":"complete","variables":[{"name":"approved","value":true},{"name":"status","value":"approved"}]}`
			}
			javaHTTPRequest(t, s, http.MethodPost, "/service/runtime/tasks/"+tasks[0].(map[string]any)["id"].(string), body, http.StatusNoContent, scenario.BeforeExpected.Tasks[0].Assignee)
			assertJavaHTTP(t, s, scenario.Expected)
		})
	}
	t.Run("completed", func(t *testing.T) {
		assertJavaHTTP(t, restoreJavaHTTP(t, fixture.Completed), fixture.CompletedExpected)
	})
}

// restoreJavaHTTP 将 JDBC 黄金行无损解码并通过正式转换入口创建独立 HTTP 服务。
// @param t 测试上下文
// @param data Java JDBC 快照
// @return 已恢复的 HTTP 处理器
func restoreJavaHTTP(t *testing.T, data []byte) http.Handler {
	t.Helper()
	var raw struct {
		Dialect       string                   `json:"dialect"`
		SchemaVersion string                   `json:"schemaVersion"`
		Tables        []migration.TableSummary `json:"tables"`
		Rows          []flowablerow.Row        `json:"rows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	for _, row := range raw.Rows {
		for column, value := range row.Values {
			// JDBC 整数保持 int64；DOUBLE_ 单独恢复浮点，外置数据按固定字节标记解码。
			switch value := value.(type) {
			case json.Number:
				var err error
				if column == "DOUBLE_" {
					row.Values[column], err = strconv.ParseFloat(value.String(), 64)
				} else {
					row.Values[column], err = value.Int64()
				}
				if err != nil {
					t.Fatal(err)
				}
			case map[string]any:
				encoded, ok := value["$bytes"].(string)
				if !ok || len(value) != 1 {
					t.Fatal("未知 JDBC 字节标记")
				}
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Fatal(err)
				}
				row.Values[column] = decoded
			}
		}
	}
	ctx := context.Background()
	state, report, err := migration.Convert(ctx, migration.SourceSnapshot{Dialect: raw.Dialect, SchemaVersion: raw.SchemaVersion, Tables: raw.Tables, Rows: raw.Rows}, migration.Options{})
	if err != nil || report.Blocked {
		t.Fatalf("Java HTTP 状态转换: %+v %v", report.Issues, err)
	}
	store := statestore.NewMemory()
	if err := store.Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	app, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return New(app, WithBasicAuth(map[string]string{"tester": "secret", "alice": "secret", "carol": "secret"}), WithTenantBindings(map[string]string{"tester": "tenant-a", "alice": "tenant-a", "carol": "tenant-a"})).Handler()
}

// javaHTTPRequest 发起具有可信租户身份的 HTTP 请求并保留精确 JSON 数值。
// @param t 测试上下文
// @param handler 被测处理器
// @param method HTTP 方法
// @param target 路径及查询参数
// @param body JSON 请求体
// @param status 预期响应码
// @param users 可选实际操作人
// @return 响应对象，空响应时为空映射
func javaHTTPRequest(t *testing.T, handler http.Handler, method, target, body string, status int, users ...string) map[string]any {
	t.Helper()
	r := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	user := "tester"
	if len(users) > 0 {
		user = users[0]
	}
	r.SetBasicAuth(user, "secret")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: %d %s", method, target, w.Code, w.Body.String())
	}
	result := map[string]any{}
	if w.Body.Len() == 0 {
		return result
	}
	decoder := json.NewDecoder(w.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

// assertJavaHTTP 核对 HTTP 分页字段、任务业务字段、历史结束状态和变量数量。
// @param t 测试上下文
// @param handler 已恢复的处理器
// @param expected Java 公共服务期望
// @return 无
func assertJavaHTTP(t *testing.T, handler http.Handler, expected javaHTTPExpected) {
	t.Helper()
	page := javaHTTPRequest(t, handler, http.MethodGet, "/service/runtime/tasks?processInstanceId="+expected.ProcessInstanceID, "", 200)
	tasks := page["data"].([]any)
	if len(tasks) != len(expected.Tasks) || page["total"].(json.Number).String() != strconv.Itoa(len(expected.Tasks)) {
		t.Fatalf("HTTP 当前任务与 Java 不同: %+v", page)
	}
	for _, want := range expected.Tasks {
		found := false
		for _, value := range tasks {
			got := value.(map[string]any)
			if got["taskDefinitionKey"] == want.Key {
				found = true
				if got["name"] != want.Name || firstHTTPString(got["assignee"]) != want.Assignee || got["tenantId"] != expected.TenantID {
					t.Fatalf("HTTP 任务字段与 Java 不同: %+v %+v", got, want)
				}
			}
		}
		if !found {
			t.Fatalf("缺失 Java 任务: %s", want.Key)
		}
	}
	history := javaHTTPRequest(t, handler, http.MethodGet, "/service/history/historic-process-instances/"+expected.ProcessInstanceID, "", 200)
	if history["businessKey"] != expected.BusinessKey || history["tenantId"] != expected.TenantID || (history["endTime"] != nil) != expected.Ended {
		t.Fatalf("HTTP 历史实例与 Java 不同: %+v", history)
	}
	page = javaHTTPRequest(t, handler, http.MethodPost, "/service/query/historic-task-instances", `{"processInstanceId":"`+expected.ProcessInstanceID+`","size":1000}`, 200)
	historic := page["data"].([]any)
	if len(historic) != len(expected.HistoricTasks) {
		t.Fatalf("历史任务数量与 Java 不同: %+v", page)
	}
	for _, want := range expected.HistoricTasks {
		found := false
		for _, value := range historic {
			got := value.(map[string]any)
			if got["taskDefinitionKey"] == want.Key {
				found = true
				if firstHTTPString(got["assignee"]) != want.Assignee || (got["endTime"] != nil) != want.Ended {
					t.Fatalf("HTTP 历史任务与 Java 不同: %+v %+v", got, want)
				}
			}
		}
		if !found {
			t.Fatalf("缺失 Java 历史任务: %s", want.Key)
		}
	}
	page = javaHTTPRequest(t, handler, http.MethodPost, "/service/query/historic-variable-instances", `{"processInstanceId":"`+expected.ProcessInstanceID+`","size":1000}`, 200)
	if len(page["data"].([]any)) != len(expected.HistoricVariables) {
		t.Fatalf("HTTP 历史变量数量与 Java 不同: %+v", page)
	}
}

// firstHTTPString 将可选 JSON 字符串转换为空串或实际文本。
// @param value JSON 值
// @return 字符串
func firstHTTPString(value any) string {
	text, _ := value.(string)
	return text
}
