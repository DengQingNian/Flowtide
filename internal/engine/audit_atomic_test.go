package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// TestAuditNestedRollback 验证失败命令隔离嵌套对象、切片及历史变量。
// @param t 测试上下文
// @return 无
func TestAuditNestedRollback(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	value := map[string]any{"obj": map[string]any{"v": "原值"}, "list": []any{[]byte{1, 2, 3}}}
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "data", Value: value}}})
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]any{"obj": map[string]any{"v": "原值"}, "list": []any{[]byte{1, 2, 3}}}
	_, err = app.execute(context.Background(), func(cc *commandContext) (any, error) {
		root, _ := app.rootExecution(app.state.Instances[instance.ID])
		current := root.Variables["data"].(map[string]any)
		current["obj"].(map[string]any)["v"] = "修改值"
		current["list"].([]any)[0].([]byte)[0] = 9
		return nil, errors.New("委托失败")
	})
	if err == nil {
		t.Fatal("失败命令应报错")
	}
	got, err := app.GetVariable(context.Background(), instance.ID, "data")
	if err != nil || !reflect.DeepEqual(got, before) {
		t.Fatalf("回滚未恢复嵌套变量: %+v %v", got, err)
	}
}

// TestAuditVariableOwnership 验证设置和读取变量均不泄露可变引用。
// @param t 测试上下文
// @return 无
func TestAuditVariableOwnership(t *testing.T) {
	app := newTestEngine(t)
	deployXML(t, app, simpleProcess)
	value := map[string]any{"items": []int{1, 2}}
	instance, err := app.StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "data", Value: value}}})
	if err != nil {
		t.Fatal(err)
	}
	value["items"].([]int)[0] = 8
	got, _ := app.GetVariable(context.Background(), instance.ID, "data")
	if got.(map[string]any)["items"].([]int)[0] != 1 {
		t.Fatal("设置变量共享调用方引用")
	}
	got.(map[string]any)["items"].([]int)[0] = 9
	again, _ := app.GetVariable(context.Background(), instance.ID, "data")
	if again.(map[string]any)["items"].([]int)[0] != 1 {
		t.Fatal("读取变量泄露内部引用")
	}
}

// TestAuditRejectDualStorage 验证双权威存储配置在构造阶段被拒绝。
// @param t 测试上下文
// @return 无
func TestAuditRejectDualStorage(t *testing.T) {
	repository, _ := flowablerow.NewRepository(&persistDB{}, "mysql")
	for _, cfg := range []Config{{StateStore: statestore.NewMemory(), RowRepository: repository}, {StatePath: t.TempDir() + "/state.json", RowRepository: repository}, {LoadRows: true}} {
		if app, err := New(cfg); err == nil {
			_ = app.Close(context.Background())
			t.Fatal("冲突或缺失存储配置应拒绝")
		}
	}
}

// TestAuditRestoreAtomic 验证后续导入失败不会留下前半份恢复数据。
// @param t 测试上下文
// @return 无
func TestAuditRestoreAtomic(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	rows, _ := source.SnapshotRows(context.Background())
	broken, _ := flowablerow.NewRow("ACT_RU_TASK")
	broken.Values = map[string]any{"ID_": "broken", "EXECUTION_ID_": "missing"}
	app := newTestEngine(t)
	if err := app.RestoreRows(context.Background(), append(rows, *broken)); err == nil {
		t.Fatal("无效执行引用应报错")
	}
	definitions, _ := app.ListProcessDefinitions(context.Background())
	if len(definitions) != 0 {
		t.Fatal("失败恢复留下了流程定义")
	}
}

// TestAuditSnapshotVersion 验证旧版快照兼容以及未知版本拒绝。
// @param t 测试上下文
// @return 无
func TestAuditSnapshotVersion(t *testing.T) {
	for _, data := range []string{`{}`, `{"version":1}`, `{"version":2}`} {
		store := statestore.NewMemory()
		if err := store.Save(context.Background(), []byte(data)); err != nil {
			t.Fatal(err)
		}
		_, err := New(Config{StateStore: store})
		if (err != nil) != (data == `{"version":2}`) {
			t.Fatalf("快照版本处理错误: %s %v", data, err)
		}
	}
}

// TestAuditCopyContainers 验证类型保持、时间指针、循环映射与空值复制。
// @param t 测试上下文
// @return 无
func TestAuditCopyContainers(t *testing.T) {
	number := 1
	source := map[string]any{"pointer": &number, "array": [2][]byte{{1}, {2}}, "nil": nil, "slice": []any(nil)}
	source["self"] = source
	copied := copyValue(source).(map[string]any)
	*copied["pointer"].(*int) = 9
	copied["array"].([2][]byte)[0][0] = 8
	copied["self"].(map[string]any)["new"] = "独立"
	if number != 1 || source["array"].([2][]byte)[0][0] != 1 || source["new"] != nil {
		t.Fatal("深拷贝共享底层数据")
	}
	if copied["new"] != "独立" {
		t.Fatal("循环映射关系丢失")
	}
}

// nonTransactionalRows 仅支持单条写入，不能用于自动命令持久化。
type nonTransactionalRows struct{}

// ExecContext 模拟没有事务能力的数据库。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 执行结果
// @return 执行错误
func (nonTransactionalRows) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	return persistResult{}, nil
}

// TestAuditRejectNonTransactionalRows 验证引擎拒绝没有事务能力的权威行仓储。
// @param t 测试上下文
// @return 无
func TestAuditRejectNonTransactionalRows(t *testing.T) {
	repository, _ := flowablerow.NewRepository(nonTransactionalRows{}, "mysql")
	if _, err := New(Config{RowRepository: repository}); err == nil {
		t.Fatal("命令持久化必须具备事务能力")
	}
}

// TestAuditSnapshotFieldInventory 验证全部业务状态字段都有显式快照映射。
// @param t 测试上下文
// @return 无
func TestAuditSnapshotFieldInventory(t *testing.T) {
	stateType := reflect.TypeOf(state{})
	snapshotType := reflect.TypeOf(snapshot{})
	for index := 0; index < stateType.NumField(); index++ {
		field := stateType.Field(index)
		if _, exists := snapshotType.FieldByName(field.Name); !exists {
			t.Errorf("状态字段缺少快照映射: %s", field.Name)
		}
	}
}
