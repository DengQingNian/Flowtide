package rest

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
	"github.com/flowable-go/flowable/internal/engine"
)

// TestJavaTaskRESTReference 对照原版任务控制器完整响应、空值、分页及动作副作用。
// @param t 测试上下文
// @return 无
func TestJavaTaskRESTReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/004-task-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline string    `json:"baseline"`
		Clock    time.Time `json:"clock"`
		Cases    []struct {
			Name         string                    `json:"name"`
			Method       string                    `json:"method"`
			Path         string                    `json:"path"`
			RequestBody  string                    `json:"requestBody"`
			Status       int                       `json:"status"`
			ResponseBody string                    `json:"responseBody"`
			ReadBackBody string                    `json:"readBackBody"`
			AfterTasks   map[string]map[string]any `json:"afterTasks"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Cases) != 26 {
		t.Fatal("任务样本基线或场景缺失")
	}
	for _, scenario := range fixture.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			ctx := context.Background()
			app, err := engine.New(engine.Config{Clock: clock.NewFake(fixture.Clock), HistoryLevel: engine.HistoryFull})
			if err != nil {
				t.Fatal(err)
			}
			ids := map[string]string{}
			for _, seed := range []struct{ id, name string }{{"0-parent", "审批甲"}, {"1-free", "审批乙"}, {"2-child", "子任务"}} {
				request := api.CreateTaskRequest{Name: seed.name}
				if seed.id == "0-parent" {
					due := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
					request.Assignee = "alice"
					request.Owner = "owner"
					request.Description = "任务说明"
					request.Category = "review"
					request.FormKey = "form"
					request.DueDate = &due
				}
				if seed.id == "2-child" {
					request.ParentTaskID = ids["0-parent"]
				}
				task, err := app.CreateTask(ctx, request)
				if err != nil {
					t.Fatal(err)
				}
				ids[seed.id] = task.ID
			}
			if scenario.Name == "resolve-free" {
				if err := app.Delegate(ctx, ids["1-free"], "bob"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.Name == "resolve" {
				if err := app.Delegate(ctx, ids["0-parent"], "bob"); err != nil {
					t.Fatal(err)
				}
			}
			path := scenario.Path
			for expected, actual := range ids {
				path = strings.ReplaceAll(path, expected, actual)
			}
			handler := New(app).Handler()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(scenario.Method, "http://localhost"+path, strings.NewReader(scenario.RequestBody))
			r.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(w, r)
			if w.Code != scenario.Status {
				t.Errorf("状态码 %d，Java为%d", w.Code, scenario.Status)
			}
			if scenario.Name == "create" && w.Code == 201 {
				var created struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
					t.Fatal(err)
				}
				ids["{createdId}"] = created.ID
			}
			// 只归一化动态标识；完整对象字段、空值、数组顺序及时间字符串均参与比较。
			body := w.Body.String()
			for expected, actual := range ids {
				body = strings.ReplaceAll(body, actual, expected)
			}
			if scenario.ResponseBody == "" {
				if strings.TrimSpace(body) != "" {
					t.Errorf("原版空正文被改为%s", body)
				}
			} else {
				assertAsyncJSON(t, []byte(body), []byte(scenario.ResponseBody))
			}
			if scenario.ReadBackBody != "" {
				read := httptest.NewRecorder()
				handler.ServeHTTP(read, httptest.NewRequest("GET", "http://localhost"+path, nil))
				body = read.Body.String()
				for expected, actual := range ids {
					body = strings.ReplaceAll(body, actual, expected)
				}
				assertAsyncJSON(t, []byte(body), []byte(scenario.ReadBackBody))
			}
			page, err := app.ListTasks(ctx, api.TaskFilter{Size: 1000})
			if err != nil {
				t.Fatal(err)
			}
			after := map[string]map[string]any{}
			for _, task := range page.Data {
				id := task.ID
				for expected, actual := range ids {
					if actual == id {
						id = expected
						break
					}
				}
				state := map[string]any{"name": task.Name, "assignee": task.Assignee, "owner": task.Owner, "delegationState": strings.ToLower(task.DelegationState)}
				for key, value := range state {
					if value == "" {
						state[key] = nil
					}
				}
				after[id] = state
			}
			actual, _ := json.Marshal(after)
			expected, _ := json.Marshal(scenario.AfterTasks)
			assertAsyncJSON(t, actual, expected)
		})
	}
}
