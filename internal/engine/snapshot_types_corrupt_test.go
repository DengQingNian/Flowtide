package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
)

// TestSnapshotTypedValueCorruption 验证损坏的类型、标量范围及容器形状不能隐式恢复。
// @param t 测试上下文
// @return 无
func TestSnapshotTypedValueCorruption(t *testing.T) {
	bad := snapshotValue{Type: "unknown"}
	tests := []snapshotValue{
		bad, {Type: "nil", Text: "多余"}, {Type: "bool", Nil: true}, {Type: "bool", Text: "坏值"},
		{Type: "int8", Text: "128"}, {Type: "uint8", Text: "-1"}, {Type: "float32", Text: "1e100"}, {Type: "float64", Text: "NaN"},
		{Type: "time.Time", Text: "不是时间"}, {Type: "json.Number", Text: "null"}, {Type: "interface {}"},
		{Type: "[]uint8", Text: "?"}, {Type: "[]uint8", Nil: true, Text: "AA=="}, {Type: "[]uint8", Items: []snapshotValue{{Type: "nil"}}},
		{Type: "[]int", Text: "多余"}, {Type: "[]int", Nil: true, Items: []snapshotValue{{Type: "nil"}}},
		{Type: "[]int", Fields: map[string]snapshotValue{"x": {Type: "nil"}}},
		{Type: "[2]int", Items: []snapshotValue{{Type: "int", Text: "1"}}}, {Type: "[0]int", Nil: true},
		{Type: "map[string]int", Items: []snapshotValue{{Type: "int", Text: "1"}}},
		{Type: "map[string]int", Fields: map[string]snapshotValue{"x": bad}},
		{Type: "map[string]int", Fields: map[string]snapshotValue{"x": {Type: "nil"}}},
		{Type: "map[string]int", Fields: map[string]snapshotValue{"x": {Type: "string", Text: "1"}}},
		{Type: "[]int", Items: []snapshotValue{bad}}, {Type: "[]int", Items: []snapshotValue{{Type: "string", Text: "1"}}},
	}
	// 损坏值必须明确失败，不能依靠默认零值、溢出转换或空容器蒙混通过。
	for _, test := range tests {
		if _, err := decodeSnapshotValue(test, 0); err == nil {
			t.Fatalf("损坏编码应拒绝：%+v", test)
		}
	}
	nested := snapshotValue{Type: "nil"}
	for range 130 {
		nested = snapshotValue{Type: "[]interface {}", Items: []snapshotValue{nested}}
	}
	if _, err := decodeSnapshotValue(nested, 0); err == nil {
		t.Fatal("过深变量应拒绝")
	}
	for _, name := range []string{"[]invalid", "map[string]invalid", "[-1]int", "[9999999999999999999999]int", "[1]invalid", "[broken", "[9223372036854775807]int64", strings.Repeat("[]", 130) + "int"} {
		if _, err := snapshotType(name, 0); err == nil {
			t.Fatalf("损坏类型应拒绝：%s", name)
		}
	}
}

// TestSnapshotTypedTableCorruption 验证侧表必须与实体一一对应，恢复失败不能发布部分状态。
// @param t 测试上下文
// @return 无
func TestSnapshotTypedTableCorruption(t *testing.T) {
	tests := []struct {
		name   string
		change func(*snapshot)
	}{
		{"执行为空", func(s *snapshot) { s.Executions[0] = nil }},
		{"任务为空", func(s *snapshot) { s.Tasks[0] = nil }},
		{"历史变量为空", func(s *snapshot) { s.HistoricVariables[0] = nil }},
		{"历史明细为空", func(s *snapshot) { s.HistoricDetails[0] = nil }},
		{"执行侧表未知实体", func(s *snapshot) {
			entry := s.TypedValues.Executions["e"]
			delete(s.TypedValues.Executions, "e")
			s.TypedValues.Executions["foreign"] = entry
		}},
		{"任务侧表未知实体", func(s *snapshot) {
			entry := s.TypedValues.Tasks["t"]
			delete(s.TypedValues.Tasks, "t")
			s.TypedValues.Tasks["foreign"] = entry
		}},
		{"历史变量侧表未知实体", func(s *snapshot) {
			entry := s.TypedValues.HistoricVariables["v"]
			delete(s.TypedValues.HistoricVariables, "v")
			s.TypedValues.HistoricVariables["foreign"] = entry
		}},
		{"历史明细侧表未知实体", func(s *snapshot) {
			entry := s.TypedValues.HistoricDetails["d"]
			delete(s.TypedValues.HistoricDetails, "d")
			s.TypedValues.HistoricDetails["foreign"] = entry
		}},
		{"执行重复", func(s *snapshot) {
			s.Executions = append(s.Executions, s.Executions[0])
			s.TypedValues.Executions["extra"] = s.TypedValues.Executions["e"]
		}},
		{"任务重复", func(s *snapshot) {
			s.Tasks = append(s.Tasks, s.Tasks[0])
			s.TypedValues.Tasks["extra"] = s.TypedValues.Tasks["t"]
		}},
		{"历史变量重复", func(s *snapshot) {
			s.HistoricVariables = append(s.HistoricVariables, s.HistoricVariables[0])
			s.TypedValues.HistoricVariables["extra"] = s.TypedValues.HistoricVariables["v"]
		}},
		{"历史明细重复", func(s *snapshot) {
			s.HistoricDetails = append(s.HistoricDetails, s.HistoricDetails[0])
			s.TypedValues.HistoricDetails["extra"] = s.TypedValues.HistoricDetails["d"]
		}},
		{"执行双来源", func(s *snapshot) { s.Executions[0].Variables = map[string]any{"x": 1} }},
		{"任务双来源", func(s *snapshot) { s.Tasks[0].LocalVariables = map[string]any{"x": 1} }},
		{"历史变量双来源", func(s *snapshot) { s.HistoricVariables[0].Value = 1 }},
		{"历史明细双来源", func(s *snapshot) { s.HistoricDetails[0].Value = 1 }},
		{"执行类型损坏", func(s *snapshot) {
			s.TypedValues.Executions["e"] = snapshotExecutionValues{Variables: snapshotValue{Type: "bad"}}
		}},
		{"集合类型损坏", func(s *snapshot) {
			entry := s.TypedValues.Executions["e"]
			entry.MIValues = snapshotValue{Type: "bad"}
			s.TypedValues.Executions["e"] = entry
		}},
		{"执行容器错误", func(s *snapshot) {
			entry := s.TypedValues.Executions["e"]
			entry.Variables = snapshotValue{Type: "string"}
			s.TypedValues.Executions["e"] = entry
		}},
		{"集合容器错误", func(s *snapshot) {
			entry := s.TypedValues.Executions["e"]
			entry.MIValues = snapshotValue{Type: "string"}
			s.TypedValues.Executions["e"] = entry
		}},
		{"任务类型损坏", func(s *snapshot) { s.TypedValues.Tasks["t"] = snapshotValue{Type: "bad"} }},
		{"任务容器错误", func(s *snapshot) { s.TypedValues.Tasks["t"] = snapshotValue{Type: "string"} }},
		{"历史变量类型损坏", func(s *snapshot) { s.TypedValues.HistoricVariables["v"] = snapshotValue{Type: "bad"} }},
		{"历史明细类型损坏", func(s *snapshot) { s.TypedValues.HistoricDetails["d"] = snapshotValue{Type: "bad"} }},
	}
	// 用完整快照模拟磁盘损坏，并确认原引擎状态不会因部分解码而改变。
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stored, err := snapshotWithTypedValues(snapshotTypeFixture())
			if err != nil {
				t.Fatal(err)
			}
			test.change(stored)
			data, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			store := statestore.NewMemory()
			_ = store.Save(context.Background(), data)
			app := &Engine{cfg: Config{StateStore: store}, state: newState()}
			app.state.Properties["保留"] = "旧状态"
			if err := app.loadState(context.Background()); err == nil {
				t.Fatal("损坏侧表应拒绝")
			}
			if app.state.Properties["保留"] != "旧状态" {
				t.Fatal("失败恢复发布了部分状态")
			}
		})
	}
	for _, data := range []string{`{"tasks":{"t":{"type":"nil"},"t":{"type":"nil"}}}`, `{"unknown":1}`, `{"tasks":`, `{} {}`} {
		var typed snapshotTypedValues
		if err := json.Unmarshal([]byte(data), &typed); err == nil {
			t.Fatal("损坏或重复侧表应拒绝")
		}
	}
	for _, data := range []string{`{"x":`, `[1`, strings.Repeat("[", 258) + "0" + strings.Repeat("]", 258)} {
		if err := uniqueSnapshotJSON(json.NewDecoder(strings.NewReader(data)), 0); err == nil {
			t.Fatal("损坏或过深JSON应拒绝")
		}
	}
}

// snapshotTypeFixture 构造每种变量所属实体各一个的完整快照。
// @return 原始快照
func snapshotTypeFixture() *snapshot {
	return &snapshot{Version: snapshotVersion,
		Executions:        []*executionState{{ID: "e", Variables: map[string]any{"x": int64(9007199254740993)}, MIValues: []any{int(1)}}},
		Tasks:             []*taskState{{Task: api.Task{ID: "t"}, LocalVariables: map[string]any{}}},
		HistoricVariables: []*api.HistoricVariableInstance{{ID: "v", Value: []byte{1}}},
		HistoricDetails:   []*api.HistoricDetail{{ID: "d", Value: time.Unix(1, 0).UTC()}},
	}
}

// TestSnapshotTypedEncodingBoundaries 验证各位置的不支持值、重复实体及过深容器都在写盘前失败。
// @param t 测试上下文
// @return 无
func TestSnapshotTypedEncodingBoundaries(t *testing.T) {
	tests := []func(*snapshot){
		func(s *snapshot) { s.Executions[0] = nil }, func(s *snapshot) { s.Tasks[0] = nil }, func(s *snapshot) { s.HistoricVariables[0] = nil }, func(s *snapshot) { s.HistoricDetails[0] = nil },
		func(s *snapshot) { s.Executions = append(s.Executions, s.Executions[0]) }, func(s *snapshot) { s.Tasks = append(s.Tasks, s.Tasks[0]) }, func(s *snapshot) { s.HistoricVariables = append(s.HistoricVariables, s.HistoricVariables[0]) }, func(s *snapshot) { s.HistoricDetails = append(s.HistoricDetails, s.HistoricDetails[0]) },
		func(s *snapshot) { s.Executions[0].Variables["bad"] = make(chan int) }, func(s *snapshot) { s.Executions[0].MIValues = []any{make(chan int)} },
		func(s *snapshot) { s.Tasks[0].LocalVariables["bad"] = make(chan int) }, func(s *snapshot) { s.HistoricVariables[0].Value = make(chan int) }, func(s *snapshot) { s.HistoricDetails[0].Value = make(chan int) },
	}
	// 覆盖每个独立持久化位置，保证任一失败都不会返回可写出的部分快照。
	for _, change := range tests {
		s := snapshotTypeFixture()
		change(s)
		if out, err := snapshotWithTypedValues(s); err == nil || out != nil {
			t.Fatal("无效实体应阻断整个快照")
		}
	}
	var nested any = int(1)
	for range 130 {
		nested = []any{nested}
	}
	for _, value := range []any{nested, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), []any{make(chan int)}, map[string]any{"bad": make(chan int)}} {
		if _, err := encodeSnapshotValue(value); err == nil {
			t.Fatal("不可持久化的值应拒绝")
		}
	}
	// 容器共享同一无环子对象是合法输入，不能误报循环。
	shared := map[string]any{"value": int64(9007199254740993)}
	value := []any{shared, shared}
	encoded, err := encodeSnapshotValue(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSnapshotValue(encoded, 0)
	if err != nil || !reflect.DeepEqual(decoded, value) {
		t.Fatal("共享子对象恢复失败")
	}
}
