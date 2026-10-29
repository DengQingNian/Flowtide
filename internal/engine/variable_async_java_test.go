package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
)

// TestJavaAsyncVariableReference 对照固定 Java 引擎实际排队与执行结果，不从 Go 实现推导期望。
// @param t 测试上下文
// @return 无
func TestJavaAsyncVariableReference(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compatibility/java/002-async-variables.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Baseline                 string         `json:"baseline"`
		ConfigurationClassSHA256 string         `json:"configurationClassSha256"`
		Payload                  map[string]any `json:"payload"`
		Cases                    []struct {
			Method         string         `json:"method"`
			Before         map[string]any `json:"before"`
			AfterEnqueue   map[string]any `json:"afterEnqueue"`
			AfterExecution map[string]any `json:"afterExecution"`
			QueuedJobs     int            `json:"queuedJobs"`
			RemainingJobs  int            `json:"remainingJobs"`
			DetailsBefore  int            `json:"detailsBefore"`
			DetailsAfter   int            `json:"detailsAfter"`
		} `json:"cases"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || fixture.ConfigurationClassSHA256 != "e0746e8d8da5f741743d220a6667b90ce18cd0034130121c468940c078c05f3f" || len(fixture.Cases) != 2 {
		t.Fatal("异步 Java 样本来源与既有固定配置类不一致")
	}
	for _, scenario := range fixture.Cases {
		t.Run(scenario.Method, func(t *testing.T) {
			app, err := New(Config{HistoryLevel: HistoryFull})
			if err != nil {
				t.Fatal(err)
			}
			deployXML(t, app, simpleProcess)
			before := make([]api.Variable, 0, len(scenario.Before))
			for name, value := range scenario.Before {
				before = append(before, api.Variable{Name: name, Value: asyncJavaValue(t, value)})
			}
			ctx := context.Background()
			instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Variables: before})
			if err != nil {
				t.Fatal(err)
			}
			variables := make([]api.Variable, 0, len(fixture.Payload))
			for name, value := range fixture.Payload {
				variables = append(variables, api.Variable{Name: name, Value: asyncJavaValue(t, value)})
			}
			request := api.AsyncVariableRequest{Variables: variables}
			if scenario.Method == "setVariablesAsync" {
				request.ProcessInstanceID = instance.ID
			} else if scenario.Method == "setVariablesLocalAsync" {
				request.ExecutionID = instance.ID
			} else {
				t.Fatal("未知 Java 排队方法")
			}
			if err := app.SetVariablesAsync(ctx, request); err != nil {
				t.Fatal(err)
			}
			assertAsyncJavaValues(t, app, instance.ID, scenario.AfterEnqueue)
			if jobs, err := app.ListJobs(ctx); err != nil || len(jobs) != scenario.QueuedJobs {
				t.Fatalf("排队数量与 Java 不同: %+v %v", jobs, err)
			}
			if details, err := app.ListHistoricDetails(ctx, instance.ID); err != nil || len(details) != scenario.DetailsBefore {
				t.Fatalf("排队阶段 full 明细与 Java 不同: %d %v", len(details), err)
			}
			if count, err := app.RunDueJobs(ctx); err != nil || count != scenario.QueuedJobs {
				t.Fatalf("作业执行数量与 Java 不同: %d %v", count, err)
			}
			assertAsyncJavaValues(t, app, instance.ID, scenario.AfterExecution)
			if jobs, err := app.ListJobs(ctx); err != nil || len(jobs) != scenario.RemainingJobs {
				t.Fatalf("剩余作业与 Java 不同: %+v %v", jobs, err)
			}
			if details, err := app.ListHistoricDetails(ctx, instance.ID); err != nil || len(details) != scenario.DetailsAfter {
				t.Fatalf("执行阶段 full 明细与 Java 不同: %d %v", len(details), err)
			}
		})
	}
}

// asyncJavaValue 无损恢复 Java 黄金样本的长整数、日期和字节标记。
// @param t 测试上下文
// @param value 已使用 UseNumber 解码的 JSON 值
// @return 实际变量值
func asyncJavaValue(t *testing.T, value any) any {
	t.Helper()
	switch value := value.(type) {
	case json.Number:
		integer, err := value.Int64()
		if err != nil {
			t.Fatal(err)
		}
		return integer
	case map[string]any:
		if encoded, ok := value["$bytes"].(string); ok {
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			return decoded
		}
		if text, ok := value["$date"].(string); ok {
			date, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				t.Fatal(err)
			}
			return date
		}
		return value
	default:
		return value
	}
}

// assertAsyncJavaValues 比较实际变量与 Java 样本，规范化只改变字节和日期表示。
// @param t 测试上下文
// @param app 被测引擎
// @param instanceID 实例标识
// @param expected Java 公共服务实际结果
// @return 无
func assertAsyncJavaValues(t *testing.T, app *Engine, instanceID string, expected map[string]any) {
	t.Helper()
	values, err := app.GetVariables(context.Background(), instanceID)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range values {
		switch value := value.(type) {
		case []byte:
			values[name] = map[string]string{"$bytes": base64.StdEncoding.EncodeToString(value)}
		case time.Time:
			values[name] = map[string]string{"$date": value.UTC().Format(time.RFC3339Nano)}
		}
	}
	actualJSON, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actualJSON, expectedJSON) {
		t.Fatalf("异步变量业务结果与 Java 不同: %s，期望 %s", actualJSON, expectedJSON)
	}
}
