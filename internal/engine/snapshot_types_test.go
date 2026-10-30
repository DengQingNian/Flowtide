package engine

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	statestore "github.com/DengQingNian/Flowtide/internal/persistence/state"
)

// TestSnapshotTypedRoundTrip 验证所有变量位置与嵌套容器经过状态重启后保持原始类型。
// @param t 测试上下文
// @return 无
func TestSnapshotTypedRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewMemory()
	first, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	values := snapshotTypeSamples()
	first.state.Executions["e"] = &executionState{ID: "e", Variables: values, MIValues: []any{int64(9007199254740993), []byte{1}, json.Number("1.234567890123456789")}}
	first.state.Tasks["t"] = &taskState{Task: api.Task{ID: "t"}, LocalVariables: values}
	for name, value := range values {
		first.state.HistoricVariables[name] = &api.HistoricVariableInstance{ID: name, Name: name, Value: value}
		first.state.HistoricDetails[name] = &api.HistoricDetail{ID: name, Name: name, Value: value}
	}
	if err := first.saveState(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.state.Executions["e"].Variables, values) {
		t.Fatal("保存过程修改了原始变量")
	}
	restored, err := New(Config{StateStore: store})
	if err != nil {
		t.Fatal(err)
	}
	// 分别检查执行、任务、历史变量和明细，避免只覆盖外层业务查询。
	for _, got := range []map[string]any{restored.state.Executions["e"].Variables, restored.state.Tasks["t"].LocalVariables} {
		if !reflect.DeepEqual(got, values) {
			t.Fatalf("变量类型或精度丢失：\n期望 %#v\n实际 %#v", values, got)
		}
	}
	if !reflect.DeepEqual(restored.state.Executions["e"].MIValues, first.state.Executions["e"].MIValues) {
		t.Fatal("多实例集合类型丢失")
	}
	for name, want := range values {
		if !reflect.DeepEqual(restored.state.HistoricVariables[name].Value, want) || !reflect.DeepEqual(restored.state.HistoricDetails[name].Value, want) {
			t.Fatalf("历史值 %s 类型丢失", name)
		}
	}
}

// snapshotTypeSamples 提供包含边界整数、空容器及伪类型标记业务数据的变量。
// @return 原始变量样本
func snapshotTypeSamples() map[string]any {
	return map[string]any{
		"null": nil, "text": "中文", "bool": true,
		"int": int(-10), "int8": int8(-128), "int16": int16(-32768), "int32": int32(-2147483648), "int64": int64(9007199254740993),
		"uint": uint(42), "uint8": uint8(255), "uint16": uint16(65535), "uint32": uint32(4294967295), "uint64": uint64(math.MaxUint64),
		"float32": float32(1.25), "float64": float64(0.1), "number": json.Number("12345678901234567890.123456789"),
		"date": time.Date(2026, 10, 28, 1, 2, 3, 456789123, time.UTC), "bytes": []byte{0, 1, 255}, "nilBytes": []byte(nil),
		"nilMap": map[string]any(nil), "emptyMap": map[string]any{}, "nilSlice": []any(nil), "emptySlice": []any{},
		"strings": []string{"一", "二"}, "integers": map[string]int64{"精确": 9007199254740993}, "array": [2]int16{1, 2},
		"nested": map[string]any{"type": "bytes", "data": "并非内部标记", "items": []any{int8(1), json.Number("9007199254740993"), []byte{3}, map[string]any{"time": time.Unix(12, 34).UTC()}}},
	}
}

// TestSnapshotTypedFailurePreservesOldState 验证不支持类型及循环引用失败时不覆盖已保存状态。
// @param t 测试上下文
// @return 无
func TestSnapshotTypedFailurePreservesOldState(t *testing.T) {
	ctx := context.Background()
	store := statestore.NewMemory()
	app, _ := New(Config{StateStore: store})
	deployXML(t, app, simpleProcess)
	instance, err := app.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "simple", Variables: []api.Variable{{Name: "initiator", Value: "alice"}}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := store.Load(ctx)
	cycle := map[string]any{}
	cycle["self"] = cycle
	// 这些值不能安全表示为当前快照类型，必须先失败再保留旧快照与原内存。
	for _, value := range []any{make(chan int), func() {}, math.NaN(), math.Inf(1), cycle, json.Number("bad")} {
		if err := app.SetVariable(ctx, instance.ID, "invalid", value); err == nil {
			t.Fatalf("值 %T 应拒绝保存", value)
		}
		after, _ := store.Load(ctx)
		if string(before) != string(after) {
			t.Fatal("失败保存覆盖旧快照")
		}
		if _, err := app.GetVariable(ctx, instance.ID, "invalid"); err == nil {
			t.Fatal("失败命令未回滚变量")
		}
	}
}

// TestSnapshotTypedCorruption 验证新版本拒绝缺失类型侧表和未知版本。
// @param t 测试上下文
// @return 无
func TestSnapshotTypedCorruption(t *testing.T) {
	// 旧版本可继续读取，新版本只有完整类型信息才能恢复。
	for _, data := range []string{`{"version":2}`, `{"version":3}`, `{"version":-1}`} {
		store := statestore.NewMemory()
		_ = store.Save(context.Background(), []byte(data))
		if _, err := New(Config{StateStore: store}); err == nil {
			t.Fatalf("损坏版本应失败：%s", data)
		}
	}
}
