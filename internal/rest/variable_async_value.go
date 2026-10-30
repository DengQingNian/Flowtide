package rest

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// readAsyncVariables 按路由读取单变量对象或变量数组，保留整数精度。
// @param r 请求
// @return 变量列表
// @return 格式或类型错误
func readAsyncVariables(r *http.Request) ([]api.Variable, error) {
	if variable, ok, err := readMultipartVariable(r, r.PathValue("name")); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if values := r.MultipartForm.Value["scope"]; len(values) > 0 {
			variable.Scope = values[0]
		}
		return []api.Variable{variable}, nil
	}
	single := r.PathValue("name") != ""
	var raw json.RawMessage
	if err := decodeSingleJSON(r, &raw, false, false); err != nil {
		code := "REST-ASYNC-LIST"
		if single {
			code = "REST-ASYNC-SINGLE"
		}
		return nil, api.NewError(code, "异步变量请求体非法", api.ErrBadRequest)
	}
	var parts []json.RawMessage
	if single {
		if len(raw) == 0 || raw[0] != '{' {
			return nil, api.NewError("REST-ASYNC-SINGLE", "单变量请求体必须是对象", api.ErrBadRequest)
		}
		parts = []json.RawMessage{raw}
	} else {
		if err := json.Unmarshal(raw, &parts); err != nil {
			return nil, api.NewError("REST-ASYNC-LIST", "变量集合请求体必须是数组", api.ErrBadRequest)
		}
		if len(parts) == 0 {
			return nil, api.NewError("REST-ASYNC-EMPTY", "变量集合不能为空", api.ErrBadRequest)
		}
	}
	variables := make([]api.Variable, 0, len(parts))
	for _, part := range parts {
		var rawVariable struct {
			Name  string          `json:"name"`
			Type  string          `json:"type"`
			Scope string          `json:"scope"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(part, &rawVariable); err != nil || strings.TrimSpace(rawVariable.Name) == "" {
			return nil, api.NewError("REST-0108", "变量名称或对象非法", api.ErrBadRequest)
		}
		value, err := decodeAsyncVariableValue(rawVariable.Value, rawVariable.Type)
		if err != nil {
			return nil, err
		}
		variables = append(variables, api.Variable{Name: rawVariable.Name, Type: rawVariable.Type, Scope: rawVariable.Scope, Value: value})
	}
	if len(variables) == 0 {
		return nil, api.NewError("REST-0118", "变量列表不能为空", api.ErrBadRequest)
	}
	return variables, nil
}

// decodeAsyncVariableValue 将 REST 标量转换为可持久化类型，JSON 对象内的数字保持精确文本。
// @param raw 变量值 JSON
// @param kind 官方变量类型，空值自动推断
// @return 精确变量值
// @return 编码、范围或类型错误
func decodeAsyncVariableValue(raw []byte, kind string) (any, error) {
	bad := func() (any, error) {
		return nil, api.NewError("REST-0108", "变量类型或值非法", api.ErrBadRequest)
	}
	// 空值也必须先校验声明类型，未知转换器不能借助 null 绕过请求校验。
	switch kind {
	case "", "short", "integer", "long", "double", "string", "boolean", "date", "json":
	default:
		return bad()
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return bad()
	}
	if kind == "" {
		if number, ok := value.(json.Number); ok {
			if strings.ContainsAny(number.String(), ".eE") {
				floating, err := number.Float64()
				if err != nil || math.IsInf(floating, 0) {
					return bad()
				}
				return floating, nil
			}
			integer, err := number.Int64()
			if err != nil {
				return bad()
			}
			return integer, nil
		}
		return value, nil
	}
	// 显式类型不能从不相干的 JSON 值隐式转换，特别是大整数不能先转浮点。
	switch kind {
	case "short", "integer", "long":
		number, ok := value.(json.Number)
		if !ok {
			return bad()
		}
		integer, err := number.Int64()
		if err != nil {
			return bad()
		}
		if kind == "short" {
			if integer < math.MinInt16 || integer > math.MaxInt16 {
				return bad()
			}
			return int16(integer), nil
		}
		if kind == "integer" {
			if integer < math.MinInt32 || integer > math.MaxInt32 {
				return bad()
			}
			return int32(integer), nil
		}
		return integer, nil
	case "double":
		number, ok := value.(json.Number)
		if !ok {
			return bad()
		}
		floating, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsInf(floating, 0) {
			return bad()
		}
		return floating, nil
	case "string":
		if text, ok := value.(string); ok {
			return text, nil
		}
	case "boolean":
		if boolean, ok := value.(bool); ok {
			return boolean, nil
		}
	case "date":
		if text, ok := value.(string); ok {
			parsed, err := parseTaskTime(text)
			if err == nil {
				return parsed, nil
			}
		}
	case "json":
		return value, nil
	}
	return bad()
}
