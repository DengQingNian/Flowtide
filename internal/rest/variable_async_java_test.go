package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

// TestJavaAsyncRESTReference 逐字段比较原版 Java 控制器的状态码、响应体及排队执行副作用。
// @param t 测试上下文
// @return 无
func TestJavaAsyncRESTReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/003-async-variable-rest.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline string `json:"baseline"`
		Cases    []struct {
			Name             string         `json:"name"`
			Method           string         `json:"method"`
			Path             string         `json:"path"`
			RequestBody      string         `json:"requestBody"`
			Status           int            `json:"status"`
			ResponseBody     string         `json:"responseBody"`
			InitialVariables map[string]any `json:"initialVariables"`
			AfterEnqueue     map[string]any `json:"afterEnqueue"`
			AfterExecution   map[string]any `json:"afterExecution"`
			QueuedJobs       int            `json:"queuedJobs"`
		} `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || len(fixture.Cases) != 14 {
		t.Fatal("Java REST 异步样本基线或方法数量错误")
	}
	for _, scenario := range fixture.Cases {
		t.Run(scenario.Name, func(t *testing.T) {
			app, err := engine.New(engine.Config{})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := app.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"rest.bpmn20.xml": []byte(restProcess)}}); err != nil {
				t.Fatal(err)
			}
			variables := make([]api.Variable, 0, len(scenario.InitialVariables))
			for name, value := range scenario.InitialVariables {
				if number, ok := value.(json.Number); ok {
					integer, err := number.Int64()
					if err != nil {
						t.Fatal(err)
					}
					value = integer
				}
				variables = append(variables, api.Variable{Name: name, Value: value})
			}
			instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "rest", Variables: variables})
			if err != nil {
				t.Fatal(err)
			}
			path := strings.ReplaceAll(scenario.Path, "{instanceId}", instance.ID)
			r := httptest.NewRequest(scenario.Method, path, strings.NewReader(scenario.RequestBody))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			New(app).Handler().ServeHTTP(w, r)
			if w.Code != scenario.Status {
				t.Errorf("状态码与 Java 不同: %d，期望 %d", w.Code, scenario.Status)
			}
			body := strings.ReplaceAll(strings.TrimSpace(w.Body.String()), instance.ID, "{instanceId}")
			if scenario.ResponseBody == "" {
				if body != "" {
					t.Errorf("Java 空响应被改为: %s", body)
				}
			} else {
				assertAsyncJSON(t, []byte(body), []byte(scenario.ResponseBody))
			}
			values, err := app.GetVariables(ctx, instance.ID)
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(values)
			expected, _ := json.Marshal(scenario.AfterEnqueue)
			assertAsyncJSON(t, actual, expected)
			if jobs, err := app.ListJobs(ctx); err != nil || len(jobs) != scenario.QueuedJobs {
				t.Fatalf("排队副作用与 Java 不同: %+v %v", jobs, err)
			}
			if count, err := app.RunDueJobs(ctx); err != nil || count != scenario.QueuedJobs {
				t.Fatalf("执行副作用与 Java 不同: %d %v", count, err)
			}
			values, err = app.GetVariables(ctx, instance.ID)
			if err != nil {
				t.Fatal(err)
			}
			actual, _ = json.Marshal(values)
			expected, _ = json.Marshal(scenario.AfterExecution)
			assertAsyncJSON(t, actual, expected)
		})
	}
}

// assertAsyncJSON 对 JSON 对象逐字段比较，允许对象键顺序不同但不忽略字段和值。
// @param t 测试上下文
// @param actual 实际响应或业务状态 JSON
// @param expected Java 实际响应或业务状态 JSON
// @return 无
func assertAsyncJSON(t *testing.T, actual, expected []byte) {
	t.Helper()
	canonical := func(data []byte) []byte {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		result, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	left, right := canonical(actual), canonical(expected)
	if !bytes.Equal(left, right) {
		t.Errorf("JSON 与原版 Java 不同: %s，期望 %s", left, right)
	}
}
