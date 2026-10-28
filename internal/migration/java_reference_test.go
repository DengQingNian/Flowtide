package migration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/migration"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"github.com/flowable-go/flowable/internal/tenant"
)

// javaReferenceFixture 保存固定 Java 提交生成的原始行及公共服务期望。
type javaReferenceFixture struct {
	Baseline          string                   `json:"baseline"`
	SchemaVersion     string                   `json:"schemaVersion"`
	Database          string                   `json:"database"`
	Scenarios         []javaReferenceScenario  `json:"scenarios"`
	Completed         migration.SourceSnapshot `json:"completed"`
	CompletedExpected javaReferenceExpected    `json:"completedExpected"`
}

// javaReferenceScenario 表示从一个真实 Java 用户任务断点向前执行一步的对照。
type javaReferenceScenario struct {
	Name           string                   `json:"name"`
	Before         migration.SourceSnapshot `json:"before"`
	BeforeExpected javaReferenceExpected    `json:"beforeExpected"`
	Expected       javaReferenceExpected    `json:"expected"`
}

// javaReferenceExpected 表示 Java 公共服务读取的稳定业务结果。
type javaReferenceExpected struct {
	ProcessInstanceID  string                      `json:"processInstanceId"`
	BusinessKey        string                      `json:"businessKey"`
	TenantID           string                      `json:"tenantId"`
	Ended              bool                        `json:"ended"`
	Tasks              []javaReferenceTask         `json:"tasks"`
	Variables          map[string]any              `json:"variables"`
	HistoricTasks      []javaReferenceHistoricTask `json:"historicTasks"`
	HistoricActivities []javaReferenceActivity     `json:"historicActivities"`
	HistoricVariables  []javaReferenceVariable     `json:"historicVariables"`
}

// javaReferenceTask 保存任务属性、候选人及局部变量期望。
type javaReferenceTask struct {
	ID              string         `json:"id"`
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Assignee        string         `json:"assignee"`
	Owner           string         `json:"owner"`
	ExecutionID     string         `json:"executionId"`
	CandidateUsers  []string       `json:"candidateUsers"`
	CandidateGroups []string       `json:"candidateGroups"`
	LocalVariables  map[string]any `json:"localVariables"`
}

// javaReferenceHistoricTask 保存任务历史的业务状态。
type javaReferenceHistoricTask struct {
	ID       string `json:"id"`
	Key      string `json:"key"`
	Assignee string `json:"assignee"`
	Ended    bool   `json:"ended"`
}

// javaReferenceActivity 保存活动历史的业务状态及原记录标识。
type javaReferenceActivity struct {
	ID           string `json:"id"`
	ActivityID   string `json:"activityId"`
	ActivityType string `json:"activityType"`
	Ended        bool   `json:"ended"`
}

// javaReferenceVariable 保存历史变量的归属、值及原记录标识。
type javaReferenceVariable struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	TaskID      string `json:"taskId"`
	ExecutionID string `json:"executionId"`
	Type        string `json:"type"`
	Value       any    `json:"value"`
}

// TestJavaReferenceUserTaskAudit 验证真实 Java 断点可离线转换并由独立 Go 引擎继续执行。
// @param t 测试上下文
// @return 无
func TestJavaReferenceUserTaskAudit(t *testing.T) {
	fixture := loadJavaReferenceFixture(t)
	if len(fixture.Scenarios) != 2 {
		t.Fatalf("需要 approve 和 review 两个 Java 断点，实际为 %d", len(fixture.Scenarios))
	}
	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			workflow, ctx := restoreJavaReference(t, scenario.Before, scenario.BeforeExpected.TenantID)
			assertJavaReferenceExpected(t, ctx, workflow, scenario.Before, scenario.BeforeExpected)
			assertJavaReferenceTypedValues(t, ctx, workflow, scenario.BeforeExpected.ProcessInstanceID)
			tasks, err := workflow.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: scenario.BeforeExpected.ProcessInstanceID})
			if err != nil || len(tasks.Data) != 1 || tasks.Data[0].TaskDefinitionKey != scenario.Name {
				t.Fatalf("Java 断点的当前任务不唯一或错误：%+v %v", tasks.Data, err)
			}
			request := api.CompleteTaskRequest{Variables: []api.Variable{{Name: "status", Value: "completed"}}}
			if scenario.Name == "approve" {
				request.Variables = []api.Variable{{Name: "approved", Value: true}, {Name: "status", Value: "approved"}}
			}
			if err := workflow.Complete(ctx, tasks.Data[0].ID, request); err != nil {
				t.Fatal(err)
			}
			assertJavaReferenceExpected(t, ctx, workflow, scenario.Before, scenario.Expected)
		})
	}
	t.Run("completed", func(t *testing.T) {
		workflow, ctx := restoreJavaReference(t, fixture.Completed, fixture.CompletedExpected.TenantID)
		assertJavaReferenceExpected(t, ctx, workflow, fixture.Completed, fixture.CompletedExpected)
	})
}

// loadJavaReferenceFixture 无损解码 Java 黄金制品并校验其来源基线。
// @param t 测试上下文
// @return 已解码并恢复列类型的制品
func loadJavaReferenceFixture(t *testing.T) javaReferenceFixture {
	t.Helper()
	data, err := os.ReadFile("../../testdata/compatibility/java/001-user-task-audit.json")
	if err != nil {
		t.Fatalf("读取真实 Java 黄金制品失败：%v", err)
	}
	// SourceSnapshot.Rows 被生产报告明确排除，测试通过独立原始结构读取制品中的实体。
	var raw struct {
		Baseline      string `json:"baseline"`
		SchemaVersion string `json:"schemaVersion"`
		Database      string `json:"database"`
		Scenarios     []struct {
			Name           string                `json:"name"`
			Before         json.RawMessage       `json:"before"`
			BeforeExpected javaReferenceExpected `json:"beforeExpected"`
			Expected       javaReferenceExpected `json:"expected"`
		} `json:"scenarios"`
		Completed         json.RawMessage       `json:"completed"`
		CompletedExpected javaReferenceExpected `json:"completedExpected"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if raw.Baseline != "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e" || raw.SchemaVersion != "8.1.0.3" || raw.Database != "h2" {
		t.Fatal("Java 制品基线、Schema 或来源数据库不匹配")
	}
	fixture := javaReferenceFixture{Baseline: raw.Baseline, SchemaVersion: raw.SchemaVersion, Database: raw.Database, Completed: decodeJavaReferenceSource(t, raw.Completed), CompletedExpected: raw.CompletedExpected}
	for _, scenario := range raw.Scenarios {
		fixture.Scenarios = append(fixture.Scenarios, javaReferenceScenario{Name: scenario.Name, Before: decodeJavaReferenceSource(t, scenario.Before), BeforeExpected: scenario.BeforeExpected, Expected: scenario.Expected})
	}
	return fixture
}

// decodeJavaReferenceSource 恢复 JDBC 原始行中的整数、浮点与字节列。
// @param t 测试上下文
// @param data 原始快照 JSON
// @return 可交给预检与转换器的源快照
func decodeJavaReferenceSource(t *testing.T, data []byte) migration.SourceSnapshot {
	t.Helper()
	var raw struct {
		Dialect       string                   `json:"dialect"`
		SchemaVersion string                   `json:"schemaVersion"`
		Tables        []migration.TableSummary `json:"tables"`
		Rows          []struct {
			Table  string         `json:"table"`
			Values map[string]any `json:"values"`
		} `json:"rows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	source := migration.SourceSnapshot{Dialect: raw.Dialect, SchemaVersion: raw.SchemaVersion, Tables: raw.Tables}
	for _, row := range raw.Rows {
		for column, value := range row.Values {
			// JDBC 的整数不可经 float64；只有实际 DOUBLE_ 列使用浮点解码。
			if number, ok := value.(json.Number); ok {
				if column == "DOUBLE_" {
					floating, err := strconv.ParseFloat(number.String(), 64)
					if err != nil {
						t.Fatalf("Java 浮点列 %s 不能恢复：%v", column, err)
					}
					row.Values[column] = floating
				} else {
					integer, err := number.Int64()
					if err != nil {
						t.Fatalf("Java 整数列 %s 不能无损恢复：%v", column, err)
					}
					row.Values[column] = integer
				}
			} else if marker, ok := value.(map[string]any); ok {
				encoded, exists := marker["$bytes"].(string)
				if !exists || len(marker) != 1 {
					t.Fatalf("Java 列 %s 包含未知类型标记", column)
				}
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Fatal(err)
				}
				row.Values[column] = decoded
			}
		}
		source.Rows = append(source.Rows, flowablerow.Row{Table: row.Table, Values: row.Values})
	}
	return source
}

// restoreJavaReference 预检并转换 Java 原始行，通过内存状态存储创建独立 Go 引擎。
// @param t 测试上下文
// @param source Java 原始快照
// @param tenantID 目标租户
// @return 从转换制品恢复的新引擎
// @return 带租户的调用上下文
func restoreJavaReference(t *testing.T, source migration.SourceSnapshot, tenantID string) (*engine.Engine, context.Context) {
	t.Helper()
	ctx := tenant.With(context.Background(), tenantID)
	report := migration.Analyze(source, migration.Options{})
	if report.Blocked {
		t.Fatalf("真实 Java 样本预检出现阻断：%+v", report.Issues)
	}
	data, report, err := migration.Convert(ctx, source, migration.Options{})
	if err != nil || report.Blocked || len(data) == 0 || report.PreparedState == nil {
		t.Fatalf("真实 Java 样本离线转换失败：%+v %v", report.Issues, err)
	}
	store := statestore.NewMemory()
	if err := store.Save(ctx, data); err != nil {
		t.Fatal(err)
	}
	workflow, err := engine.New(engine.Config{StateStore: store, HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workflow.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return workflow, ctx
}

// assertJavaReferenceTypedValues 验证跨状态恢复后仍保留精确大整数、字节、时间与 JSON 数值类型。
// @param t 测试上下文
// @param ctx 租户上下文
// @param workflow 独立恢复的 Go 引擎
// @param instanceID 实例标识
// @return 无
func assertJavaReferenceTypedValues(t *testing.T, ctx context.Context, workflow *engine.Engine, instanceID string) {
	t.Helper()
	values, err := workflow.GetVariables(ctx, instanceID)
	if err != nil {
		t.Fatal(err)
	}
	if large, ok := values["large"].(int64); !ok || large != 9007199254740993 {
		t.Fatalf("大整数精度或类型丢失：%T %v", values["large"], values["large"])
	}
	if _, ok := values["binary"].([]byte); !ok {
		t.Fatalf("字节类型丢失：%T", values["binary"])
	}
	if _, ok := values["submittedAt"].(time.Time); !ok {
		t.Fatalf("日期类型丢失：%T", values["submittedAt"])
	}
	document, ok := values["document"].(map[string]any)
	if !ok {
		t.Fatalf("JSON 对象类型丢失：%T", values["document"])
	}
	if amount, ok := document["amount"].(json.Number); !ok || amount.String() != "12" {
		t.Fatalf("JSON 数值类型或精度丢失：%T %v", document["amount"], document["amount"])
	}
}

// assertJavaReferenceExpected 对照 Java 公共服务结果并检查原有历史记录标识保持。
// @param t 测试上下文
// @param ctx 租户上下文
// @param workflow 独立恢复的 Go 引擎
// @param source 转换前的原始 Java 快照
// @param want Java 公共服务期望
// @return 无
func assertJavaReferenceExpected(t *testing.T, ctx context.Context, workflow *engine.Engine, source migration.SourceSnapshot, want javaReferenceExpected) {
	t.Helper()
	processes, err := workflow.ListHistoricProcessInstances(ctx)
	if err != nil || len(processes) != 1 {
		t.Fatalf("历史实例数量错误：%+v %v", processes, err)
	}
	process := processes[0]
	if process.ID != want.ProcessInstanceID || process.BusinessKey != want.BusinessKey || process.TenantID != want.TenantID || process.Ended != want.Ended || (process.EndTime != nil) != want.Ended {
		t.Fatalf("历史实例属性与 Java 不一致：%+v，期望 %+v", process, want)
	}
	for _, row := range source.Rows {
		if row.Table == "ACT_HI_PROCINST" && row.Values["ID_"] == process.ID {
			started, err := time.Parse(time.RFC3339Nano, row.Values["START_TIME_"].(string))
			if err != nil || !process.StartTime.Equal(started) {
				t.Fatalf("原始实例开始时间丢失：%v %v", process.StartTime, err)
			}
		}
	}
	tasks, err := workflow.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: want.ProcessInstanceID})
	if err != nil || len(tasks.Data) != len(want.Tasks) {
		t.Fatalf("活跃任务数量与 Java 不一致：%+v %v，期望 %+v", tasks.Data, err, want.Tasks)
	}
	byKey := make(map[string]api.Task)
	for _, task := range tasks.Data {
		byKey[task.TaskDefinitionKey] = task
	}
	for _, expected := range want.Tasks {
		actual, exists := byKey[expected.Key]
		if !exists || actual.Name != expected.Name || actual.Assignee != expected.Assignee || actual.Owner != expected.Owner {
			t.Fatalf("任务属性与 Java 不一致：%+v，期望 %+v", actual, expected)
		}
		assertJavaOriginalID(t, source, "ACT_RU_TASK", expected.ID, actual.ID)
		assertJavaReferenceStrings(t, "候选用户", actual.CandidateUsers, expected.CandidateUsers)
		assertJavaReferenceStrings(t, "候选组", actual.CandidateGroups, expected.CandidateGroups)
		locals := make(map[string]any)
		variables, err := workflow.ListVariableInstances(ctx, api.VariableInstanceFilter{TaskID: actual.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, variable := range variables {
			locals[variable.Name] = variable.Value
		}
		assertJavaReferenceValue(t, "任务局部变量", locals, expected.LocalVariables)
	}
	if !want.Ended {
		instance, err := workflow.GetProcessInstance(ctx, want.ProcessInstanceID)
		if err != nil || instance.Ended || instance.BusinessKey != want.BusinessKey || instance.TenantID != want.TenantID {
			t.Fatalf("运行时实例属性错误：%+v %v", instance, err)
		}
		variables, err := workflow.GetVariables(ctx, want.ProcessInstanceID)
		if err != nil {
			t.Fatal(err)
		}
		assertJavaReferenceValue(t, "流程变量", variables, want.Variables)
	}
	assertJavaReferenceHistory(t, ctx, workflow, source, want)
}

// assertJavaReferenceHistory 对照历史任务、活动和变量，拒绝缺失或重复记录。
// @param t 测试上下文
// @param ctx 租户上下文
// @param workflow 独立恢复的 Go 引擎
// @param source 转换前的原始 Java 快照
// @param want Java 公共服务期望
// @return 无
func assertJavaReferenceHistory(t *testing.T, ctx context.Context, workflow *engine.Engine, source migration.SourceSnapshot, want javaReferenceExpected) {
	t.Helper()
	tasks, err := workflow.ListHistoricTaskInstances(ctx, want.ProcessInstanceID)
	if err != nil || len(tasks) != len(want.HistoricTasks) {
		t.Fatalf("历史任务缺失或重复：%+v %v，期望 %+v", tasks, err, want.HistoricTasks)
	}
	for _, expected := range want.HistoricTasks {
		found := false
		for _, task := range tasks {
			if task.TaskDefinitionKey != expected.Key {
				continue
			}
			found = true
			if task.Assignee != expected.Assignee || (task.EndTime != nil) != expected.Ended {
				t.Fatalf("历史任务状态错误：%+v，期望 %+v", task, expected)
			}
			assertJavaOriginalID(t, source, "ACT_HI_TASKINST", expected.ID, task.ID)
		}
		if !found {
			t.Fatalf("缺少历史任务 %s", expected.Key)
		}
	}
	activities, err := workflow.ListHistoricActivityInstances(ctx, want.ProcessInstanceID)
	if err != nil || len(activities) != len(want.HistoricActivities) {
		t.Fatalf("历史活动缺失或重复：%+v %v，期望 %+v", activities, err, want.HistoricActivities)
	}
	for _, expected := range want.HistoricActivities {
		found := false
		for _, activity := range activities {
			if activity.ActivityID != expected.ActivityID || activity.ActivityType != expected.ActivityType {
				continue
			}
			found = true
			if (activity.EndTime != nil) != expected.Ended {
				t.Fatalf("历史活动结束状态错误：%+v，期望 %+v", activity, expected)
			}
			assertJavaOriginalID(t, source, "ACT_HI_ACTINST", expected.ID, activity.ID)
		}
		if !found {
			t.Fatalf("缺少历史活动 %s/%s", expected.ActivityID, expected.ActivityType)
		}
	}
	variables, err := workflow.ListHistoricVariableInstances(ctx, want.ProcessInstanceID)
	if err != nil || len(variables) != len(want.HistoricVariables) {
		t.Fatalf("历史变量缺失或重复：%+v %v，期望 %+v", variables, err, want.HistoricVariables)
	}
	for _, expected := range want.HistoricVariables {
		found := false
		for _, variable := range variables {
			if variable.Name != expected.Name || variable.TaskID != expected.TaskID {
				continue
			}
			found = true
			assertJavaOriginalID(t, source, "ACT_HI_VARINST", expected.ID, variable.ID)
			if variable.ExecutionID != expected.ExecutionID {
				t.Fatalf("历史变量执行归属错误：%+v，期望 %+v", variable, expected)
			}
			if variable.Type != expected.Type {
				t.Fatalf("历史变量类型错误：%s=%s，期望 %s", variable.Name, variable.Type, expected.Type)
			}
			assertJavaReferenceValue(t, "历史变量 "+expected.Name, variable.Value, expected.Value)
		}
		if !found {
			t.Fatalf("缺少历史变量 %s/%s", expected.TaskID, expected.Name)
		}
	}
}

// assertJavaOriginalID 只对原始快照已存在的记录要求保持 Java 标识。
// @param t 测试上下文
// @param source 原始 Java 快照
// @param table 实体表名
// @param expected Java 期望记录标识
// @param actual 恢复或执行后的记录标识
// @return 无
func assertJavaOriginalID(t *testing.T, source migration.SourceSnapshot, table, expected, actual string) {
	t.Helper()
	for _, row := range source.Rows {
		if row.Table == table && row.Values["ID_"] == expected && actual != expected {
			t.Fatalf("原始 %s 标识 %s 被替换为 %s", table, expected, actual)
		}
	}
}

// assertJavaReferenceStrings 将无序且可空的候选集合按精确成员比较。
// @param t 测试上下文
// @param label 断言说明
// @param actual Go 候选集合
// @param expected Java 候选集合
// @return 无
func assertJavaReferenceStrings(t *testing.T, label string, actual, expected []string) {
	t.Helper()
	a, b := append([]string{}, actual...), append([]string{}, expected...)
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%s 不一致：%v，期望 %v", label, a, b)
	}
}

// assertJavaReferenceValue 使用精确数字和显式日期字节标记比较 Java 与 Go 值。
// @param t 测试上下文
// @param label 断言说明
// @param actual Go 实际值
// @param expected Java 编码期望
// @return 无
func assertJavaReferenceValue(t *testing.T, label string, actual, expected any) {
	t.Helper()
	a, b := canonicalJavaValue(t, actual), canonicalJavaValue(t, expected)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%s 与 Java 不一致：%#v，期望 %#v", label, a, b)
	}
}

// canonicalJavaValue 递归规范化边界表示但保持整数精度及日期字节类型。
// @param t 测试上下文
// @param value 待比较的业务值
// @return 无损规范化值
func canonicalJavaValue(t *testing.T, value any) any {
	t.Helper()
	// Java 显式标记仅在参考制品期望中恢复，业务对象仍逐项保留结构。
	switch typed := value.(type) {
	case map[string]any:
		if encoded, ok := typed["$bytes"].(string); ok && len(typed) == 1 {
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
		if encoded, ok := typed["$date"].(string); ok && len(typed) == 1 {
			date, err := time.Parse(time.RFC3339Nano, encoded)
			if err != nil {
				t.Fatal(err)
			}
			return date.UTC()
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = canonicalJavaValue(t, item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = canonicalJavaValue(t, item)
		}
		return out
	case time.Time:
		return typed.UTC()
	case json.Number:
		return typed
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return json.Number(fmt.Sprint(typed))
	case float64:
		return json.Number(strconv.FormatFloat(typed, 'g', -1, 64))
	default:
		return value
	}
}
