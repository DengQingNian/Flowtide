package migration

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/persistence/schema"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestConvertState 验证离线转换保留变量类型、任务与源数据，生成的状态可独立续跑。
// @param t 测试上下文
// @return 无
func TestConvertState(t *testing.T) {
	source := conversionFixture(t, "a")
	before := make([]map[string]any, len(source.Rows))
	for i, row := range source.Rows {
		before[i] = make(map[string]any, len(row.Values))
		for key, value := range row.Values {
			before[i][key] = value
		}
	}
	data, report, err := Convert(context.Background(), source, Options{})
	if err != nil || report.Blocked || len(data) == 0 {
		t.Fatalf("离线转换失败：%+v %v", report.Issues, err)
	}
	// 产物使用服务端同一状态存储入口读取，不能依靠转换进程内残留状态。
	store := statestore.NewMemory()
	if err := store.Save(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	workflow, err := engine.New(engine.Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenant.With(context.Background(), "a")
	instances, err := workflow.ListProcessInstances(ctx)
	if err != nil || len(instances) != 1 {
		t.Fatalf("转换后实例缺失：%+v %v", instances, err)
	}
	variables, err := workflow.GetVariables(ctx, instances[0].ID)
	want := map[string]any{"amount": int64(9007199254740993), "binary": []byte{0, 1, 255}, "when": time.UnixMilli(1700000000123).UTC()}
	if err != nil || !reflect.DeepEqual(variables, want) {
		t.Fatalf("变量类型或精度丢失：%#v %v", variables, err)
	}
	tasks, err := workflow.ListTasks(ctx, api.TaskFilter{})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("转换后任务缺失：%+v %v", tasks, err)
	}
	if err := workflow.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	remaining, err := workflow.ListProcessInstances(ctx)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("转换后实例无法结束：%+v %v", remaining, err)
	}
	for i, row := range source.Rows {
		if !reflect.DeepEqual(row.Values, before[i]) {
			t.Fatalf("转换修改了源行：%s", row.Table)
		}
	}
}

// TestConvertTenant 验证显式租户只进入对应实例，默认租户空值不会扩大范围。
// @param t 测试上下文
// @return 无
func TestConvertTenant(t *testing.T) {
	source := conversionFixture(t, "a")
	other := conversionFixture(t, "b")
	source.Rows = append(source.Rows, other.Rows...)
	selected := "a"
	data, report, err := Convert(context.Background(), source, Options{TenantID: &selected})
	if err != nil || report.Blocked {
		t.Fatalf("租户转换失败：%+v %v", report.Issues, err)
	}
	store := statestore.NewMemory()
	_ = store.Save(context.Background(), data)
	workflow, err := engine.New(engine.Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	// 同一产物不能包含其它租户的实例、定义或变量。
	for _, id := range []string{"a", "b", ""} {
		instances, err := workflow.ListProcessInstances(tenant.With(context.Background(), id))
		want := 0
		if id == selected {
			want = 1
		}
		if err != nil || len(instances) != want {
			t.Fatalf("租户范围错误：%q %+v %v", id, instances, err)
		}
	}
}

// TestConvertFailures 验证阻断、解码失败和取消均不生成状态产物。
// @param t 测试上下文
// @return 无
func TestConvertFailures(t *testing.T) {
	source := conversionFixture(t, "a")
	source.SchemaVersion = "unknown"
	data, report, err := Convert(context.Background(), source, Options{})
	if len(data) != 0 || !report.Blocked || err != nil {
		t.Fatalf("预检阻断不能生成状态：%d %+v %v", len(data), report, err)
	}
	source = conversionFixture(t, "a")
	for i := range source.Rows {
		if source.Rows[i].Table == "ACT_RE_DEPLOYMENT" {
			delete(source.Rows[i].Values, "DEPLOY_TIME_")
		}
	}
	data, _, err = Convert(context.Background(), source, Options{})
	if len(data) != 0 || err == nil {
		t.Fatalf("导入错误不能生成状态：%d %v", len(data), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, _, err = Convert(ctx, SourceSnapshot{}, Options{})
	if len(data) != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("取消应直接返回：%d %v", len(data), err)
	}
}

// conversionFixture 以现有引擎生成完整列的合成转换夹具，不冒充 Java 实际写库数据。
// @param t 测试上下文
// @param tenantID 夹具租户
// @return 不含 Go 控制扩展的源快照
func conversionFixture(t *testing.T, tenantID string) SourceSnapshot {
	t.Helper()
	workflow, err := engine.New(engine.Config{HistoryLevel: engine.HistoryNone, TenantID: tenantID})
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenant.With(context.Background(), tenantID)
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="convert" isExecutable="true"><startEvent id="start"/><sequenceFlow id="a" sourceRef="start" targetRef="wait"/><userTask id="wait"/><sequenceFlow id="b" sourceRef="wait" targetRef="end"/><endEvent id="end"/></process></definitions>`
	if _, err := workflow.Deploy(ctx, api.DeploymentRequest{TenantID: tenantID, Resources: map[string][]byte{"convert.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "convert", TenantID: tenantID, Variables: []api.Variable{{Name: "amount", Value: int64(9007199254740993)}, {Name: "binary", Value: []byte{0, 1, 255}}, {Name: "when", Value: time.UnixMilli(1700000000123).UTC()}}}); err != nil {
		t.Fatal(err)
	}
	rows, err := workflow.SnapshotRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	source := SourceSnapshot{Dialect: "mysql", SchemaVersion: schema.Version}
	counts := make(map[string]int64)
	// 去除只有 Go 自己才能生成的续跑扩展，使测试覆盖标准行恢复入口。
	for _, row := range rows {
		if row.Table == "ACT_GE_BYTEARRAY" && strings.HasPrefix(analysisString(row, "NAME_"), "flowable-go-control/") {
			continue
		}
		source.Rows = append(source.Rows, row)
		counts[row.Table]++
	}
	for _, name := range requiredAnalysisTables {
		source.Tables = append(source.Tables, TableSummary{Name: name, Count: counts[name]})
		delete(counts, name)
	}
	for name, count := range counts {
		source.Tables = append(source.Tables, TableSummary{Name: name, Count: count})
	}
	return source
}
