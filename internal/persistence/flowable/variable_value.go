package flowable

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// VariableValue 按受支持的显式类型恢复变量，未知类型必须阻断。
// @param kind 变量类型
// @param integer 整数列
// @param number 浮点列
// @param text 文本列
// @param data 外置字节数据
// @return 原始类型的值
// @return 类型或数据错误
func VariableValue(kind string, integer *int64, number *float64, text *string, data []byte) (any, error) {
	// 已知类型逐一解码，禁止将 Java 序列化等未知类型退化为空值。
	switch kind {
	case "null":
		return nil, nil
	case "string":
		if text == nil {
			return "", nil
		}
		return *text, nil
	case "longString":
		if data == nil {
			break
		}
		return string(data), nil
	case "bytes":
		if data == nil {
			break
		}
		return append([]byte{}, data...), nil
	case "boolean":
		if integer == nil {
			return false, nil
		}
		return *integer != 0, nil
	case "short", "integer", "long":
		// Flowable 对超出浮点安全范围的整数会在 LONG_ 中丢失精度，TEXT_ 保留精确十进制文本。
		if text != nil && *text != "" {
			value, err := strconv.ParseInt(*text, 10, 64)
			if err != nil {
				return nil, api.NewError("ROW-0034", "整数变量文本无法精确解析", err)
			}
			return value, nil
		}
		if integer == nil {
			return int64(0), nil
		}
		return *integer, nil
	case "date":
		if integer == nil {
			break
		}
		return time.UnixMilli(*integer).UTC(), nil
	case "double":
		if number == nil {
			return 0.0, nil
		}
		return *number, nil
	case "json", "longJson":
		var raw string
		if kind == "longJson" {
			if data == nil {
				break
			}
			raw = string(data)
		} else {
			if text == nil {
				break
			}
			raw = *text
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, api.NewError("ROW-0031", "JSON 变量解析失败", err)
		}
		return value, nil
	default:
		return nil, api.NewError("ROW-0033", "不支持的变量类型: "+kind, api.ErrBadRequest)
	}
	return nil, api.NewError("ROW-0032", "变量数据缺失: "+kind, api.ErrBadRequest)
}

// variableByteArrays 收集同批次变量引用的字节行并校验内容类型。
// @param rows 全部导入行
// @return 字节行索引
// @return 解码错误
func variableByteArrays(rows []Row) (map[string][]byte, error) {
	arrays := make(map[string][]byte)
	for _, row := range rows {
		if row.Table != "ACT_GE_BYTEARRAY" {
			continue
		}
		id, err := stringValue(row, "ID_")
		if err != nil {
			return nil, err
		}
		data, ok := row.Values["BYTES_"].([]byte)
		if !ok {
			return nil, api.NewError("ROW-0032", "字节行内容类型错误", api.ErrBadRequest)
		}
		arrays[id] = append([]byte{}, data...)
	}
	return arrays, nil
}

// storedVariableValue 解析行内列和外置字节引用。
// @param row 变量行
// @param kind 变量类型
// @param integer 整数列
// @param number 浮点列
// @param text 文本列
// @param arrays 字节数据索引
// @return 变量值
// @return 解码错误
func storedVariableValue(row Row, kind string, integer *int64, number *float64, text *string, arrays []map[string][]byte) (any, error) {
	reference, err := optionalStringValue(row, "BYTEARRAY_ID_")
	if err != nil {
		return nil, err
	}
	var data []byte
	if reference != "" {
		if len(arrays) == 0 {
			return nil, api.NewError("ROW-0032", "缺少字节数据集合", api.ErrBadRequest)
		}
		var exists bool
		data, exists = arrays[0][reference]
		if !exists {
			return nil, api.NewError("ROW-0032", "变量字节数据缺失: "+reference, api.ErrBadRequest)
		}
	}
	return VariableValue(kind, integer, number, text, data)
}

// optionalStringValue 将缺失或可空列统一表示为空字符串。
// @param row 行
// @param column 可选列
// @return 字符串值
// @return 类型错误
func optionalStringValue(row Row, column string) (string, error) {
	value, err := optionalString(row, column)
	if err != nil || value == nil {
		return "", err
	}
	return *value, nil
}
