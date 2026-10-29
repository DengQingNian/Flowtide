package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestAsyncVariableValueTypes 验证 REST 精确标量转换、范围拒绝与 JSON 数值保留。
// @param t 测试上下文
// @return 无
func TestAsyncVariableValueTypes(t *testing.T) {
	for _, tc := range []struct {
		raw, kind string
		want      any
		bad       bool
	}{
		{`null`, "long", nil, false}, {``, "", nil, false},
		{`1`, "", int64(1), false}, {`1.25`, "", float64(1.25), false}, {`1e1000`, "", nil, true}, {`9223372036854775808`, "", nil, true},
		{`9007199254740993`, "long", int64(9007199254740993), false}, {`32767`, "short", int16(32767), false}, {`32768`, "short", nil, true},
		{`2147483647`, "integer", int32(2147483647), false}, {`2147483648`, "integer", nil, true}, {`1.5`, "long", nil, true}, {`"1"`, "long", nil, true},
		{`1.5`, "double", float64(1.5), false}, {`true`, "double", nil, true}, {`1e1000`, "double", nil, true},
		{`"中文"`, "string", "中文", false}, {`1`, "string", nil, true}, {`true`, "boolean", true, false}, {`"true"`, "boolean", nil, true},
		{`"2026-10-29T00:00:00Z"`, "date", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), false}, {`"bad"`, "date", nil, true}, {`1`, "date", nil, true},
		{`{"n":9007199254740993}`, "json", map[string]any{"n": json.Number("9007199254740993")}, false},
		{`true`, "unknown", nil, true}, {`null`, "unknown", nil, true}, {`{`, "", nil, true},
	} {
		value, err := decodeAsyncVariableValue([]byte(tc.raw), tc.kind)
		if (err != nil) != tc.bad || (!tc.bad && !reflect.DeepEqual(value, tc.want)) {
			t.Errorf("值=%s 类型=%s: %T %v %v", tc.raw, tc.kind, value, value, err)
		}
	}
}

// TestReadAsyncVariableForms 验证单变量对象、集合数组及原版 Java 拒绝的畸形边界。
// @param t 测试上下文
// @return 无
func TestReadAsyncVariableForms(t *testing.T) {
	for _, body := range []string{`{"name":"x","value":1}`, `[{"name":"x","value":1}]`} {
		request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
		if strings.HasPrefix(body, "{") {
			request.SetPathValue("name", "x")
		}
		variables, err := readAsyncVariables(request)
		if err != nil || len(variables) != 1 || variables[0].Name != "x" || variables[0].Value != int64(1) {
			t.Fatalf("变量请求体=%s: %+v %v", body, variables, err)
		}
	}
	for _, body := range []string{`[]`, `{}`, `{"name":"x","value":1}`, `{"x":1}`, `1`, `[null]`, `[{"value":1}]`, `[{"name":"x","type":"long","value":"bad"}]`, `{"x":9223372036854775808}`} {
		if _, err := readAsyncVariables(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))); err == nil {
			t.Errorf("无效变量未拒绝: %s", body)
		}
	}
}

// TestAsyncVariableMultipart 验证 multipart 只排队字节值，作业执行后才可读取。
// @param t 测试上下文
// @return 无
func TestAsyncVariableMultipart(t *testing.T) {
	server, app := newTestServer(t)
	deployREST(t, server)
	id := startQueryInstance(t, server.URL, "rest", "binary", "异步字节")
	response := multipartVariable(t, http.MethodPost, server.URL+"/service/runtime/process-instances/"+id+"/variables-async", "blob", []byte{0, 255, 1})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("异步上传状态=%d", response.StatusCode)
	}
	_ = response.Body.Close()
	if values, err := app.GetVariables(context.Background(), id); err != nil || values["blob"] != nil {
		t.Fatal("异步上传提前生效")
	}
	if count, err := app.RunDueJobs(context.Background()); err != nil || count != 1 {
		t.Fatalf("异步上传作业: %d %v", count, err)
	}
	if value, err := app.GetVariable(context.Background(), id, "blob"); err != nil || !reflect.DeepEqual(value, []byte{0, 255, 1}) {
		t.Fatalf("异步上传内容: %v %v", value, err)
	}
}
