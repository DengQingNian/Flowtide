package flowable

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// TestVariableStorageTypes 验证字节、日期、长文本和高精度 JSON 类型。
// @param t 测试上下文
// @return 无
func TestVariableStorageTypes(t *testing.T) {
	millis := int64(1234)
	for _, test := range []struct {
		kind string
		data []byte
		want any
	}{{"bytes", []byte{1, 2, 3}, []byte{1, 2, 3}}, {"longString", []byte("长文本"), "长文本"}, {"longJson", []byte(`{"n":9007199254740993}`), map[string]any{"n": json.Number("9007199254740993")}}} {
		value, err := VariableValue(test.kind, nil, nil, nil, test.data)
		if err != nil || !reflect.DeepEqual(value, test.want) {
			t.Fatalf("%s 解码错误: %+v %v", test.kind, value, err)
		}
	}
	if value, err := VariableValue("date", &millis, nil, nil, nil); err != nil || !value.(time.Time).Equal(time.UnixMilli(millis)) {
		t.Fatalf("日期解码错误: %v", err)
	}
	for _, kind := range []string{"bytes", "longString", "longJson", "json", "date", "serializable"} {
		if _, err := VariableValue(kind, nil, nil, nil, nil); err == nil {
			t.Fatalf("%s 缺失数据应阻断", kind)
		}
	}
	row := auditRow(t, "ACT_RU_VARIABLE", map[string]any{"ID_": "v", "BYTEARRAY_ID_": "data"})
	arrays := map[string][]byte{"data": {1, 2, 3}}
	value, err := storedVariableValue(row, "bytes", nil, nil, nil, []map[string][]byte{arrays})
	if err != nil || !reflect.DeepEqual(value, []byte{1, 2, 3}) {
		t.Fatalf("引用读取错误: %v", err)
	}
	for _, source := range [][]map[string][]byte{nil, {{}}} {
		if _, err := storedVariableValue(row, "bytes", nil, nil, nil, source); err == nil {
			t.Fatal("缺失引用必须阻断")
		}
	}
	row.Values["BYTEARRAY_ID_"] = 1
	if _, err := storedVariableValue(row, "bytes", nil, nil, nil, nil); err == nil {
		t.Fatal("无效引用类型必须阻断")
	}
	bytesRow := auditRow(t, "ACT_GE_BYTEARRAY", map[string]any{"ID_": "data", "BYTES_": []byte{1, 2, 3}})
	if arrays, err := variableByteArrays([]Row{bytesRow}); err != nil || !reflect.DeepEqual(arrays["data"], []byte{1, 2, 3}) {
		t.Fatalf("字节集合错误: %v", err)
	}
	bytesRow.Values["BYTES_"] = "无效"
	if _, err := variableByteArrays([]Row{bytesRow}); err == nil {
		t.Fatal("字节列类型错误应拒绝")
	}
	delete(bytesRow.Values, "ID_")
	if _, err := variableByteArrays([]Row{bytesRow}); err == nil {
		t.Fatal("字节行缺少标识应拒绝")
	}
}
