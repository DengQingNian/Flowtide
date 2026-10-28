package engine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"time"
)

// UnmarshalJSON 拒绝类型侧表中的重复键及未知字段，避免重复实体被静默覆盖。
// @param data 类型侧表的 JSON
// @return 数据格式错误
func (s *snapshotTypedValues) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueSnapshotJSON(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("快照类型侧表包含多余内容")
	}
	type plain snapshotTypedValues
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(s))
}

// uniqueSnapshotJSON 遍历 JSON 对象并拒绝任意层级的重复键。
// @param decoder JSON 读取器
// @param depth 当前层级
// @return 读取或重复键错误
func uniqueSnapshotJSON(decoder *json.Decoder, depth int) error {
	if depth > 256 {
		return errors.New("快照类型侧表嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("快照类型侧表不是有效 JSON")
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	// 每个对象独立记录键集合，不允许后一个同名实体覆盖前一个实体。
	seen := make(map[string]bool)
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("读取快照类型侧表键失败")
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("快照类型侧表包含重复键")
			}
			seen[name] = true
		}
		if err := uniqueSnapshotJSON(decoder, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("快照类型侧表容器不完整")
	}
	return nil
}

// restoreSnapshotTypedValues 校验类型侧表与实体一一对应后恢复原始业务类型。
// @param stored 待恢复快照
// @return 侧表、实体引用或值错误
func restoreSnapshotTypedValues(stored *snapshot) error {
	typed := stored.TypedValues
	if typed == nil || len(typed.Executions) != len(stored.Executions) || len(typed.Tasks) != len(stored.Tasks) || len(typed.HistoricVariables) != len(stored.HistoricVariables) || len(typed.HistoricDetails) != len(stored.HistoricDetails) {
		return errors.New("快照类型侧表缺失或实体数量不一致")
	}
	seen := make(map[string]bool)
	// 所有原始 any 字段在新格式中必须为空，防止同一变量存在两份冲突的权威数据。
	for _, execution := range stored.Executions {
		if execution == nil || seen[execution.ID] || execution.Variables != nil || execution.MIValues != nil {
			return errors.New("快照执行实体无效或重复")
		}
		seen[execution.ID] = true
		entry, exists := typed.Executions[execution.ID]
		if !exists {
			return errors.New("快照执行缺少类型数据")
		}
		variables, err := decodeSnapshotValue(entry.Variables, 0)
		if err != nil {
			return err
		}
		items, err := decodeSnapshotValue(entry.MIValues, 0)
		if err != nil {
			return err
		}
		var valid bool
		execution.Variables, valid = variables.(map[string]any)
		if !valid {
			return errors.New("快照执行变量容器类型错误")
		}
		execution.MIValues, valid = items.([]any)
		if !valid {
			return errors.New("快照多实例集合类型错误")
		}
	}
	seen = make(map[string]bool)
	for _, task := range stored.Tasks {
		if task == nil || seen[task.Task.ID] || task.LocalVariables != nil {
			return errors.New("快照任务实体无效或重复")
		}
		seen[task.Task.ID] = true
		entry, exists := typed.Tasks[task.Task.ID]
		if !exists {
			return errors.New("快照任务缺少类型数据")
		}
		value, err := decodeSnapshotValue(entry, 0)
		if err != nil {
			return err
		}
		var valid bool
		task.LocalVariables, valid = value.(map[string]any)
		if !valid {
			return errors.New("快照任务变量容器类型错误")
		}
	}
	seen = make(map[string]bool)
	for _, historic := range stored.HistoricVariables {
		if historic == nil || seen[historic.ID] || historic.Value != nil {
			return errors.New("快照历史变量无效或重复")
		}
		seen[historic.ID] = true
		entry, exists := typed.HistoricVariables[historic.ID]
		if !exists {
			return errors.New("快照历史变量缺少类型数据")
		}
		value, err := decodeSnapshotValue(entry, 0)
		if err != nil {
			return err
		}
		historic.Value = value
	}
	seen = make(map[string]bool)
	for _, detail := range stored.HistoricDetails {
		if detail == nil || seen[detail.ID] || detail.Value != nil {
			return errors.New("快照历史明细无效或重复")
		}
		seen[detail.ID] = true
		entry, exists := typed.HistoricDetails[detail.ID]
		if !exists {
			return errors.New("快照历史明细缺少类型数据")
		}
		value, err := decodeSnapshotValue(entry, 0)
		if err != nil {
			return err
		}
		detail.Value = value
	}
	return nil
}

// decodeSnapshotValue 恢复明确类型值并验证不匹配的标记及容器元素。
// @param encoded 类型编码
// @param depth 当前嵌套深度
// @return 原始 Go 类型的值
// @return 类型或内容错误
func decodeSnapshotValue(encoded snapshotValue, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("快照变量嵌套过深")
	}
	if encoded.Type == "nil" {
		if encoded.Text != "" || encoded.Nil || len(encoded.Items) > 0 || len(encoded.Fields) > 0 {
			return nil, errors.New("空值类型携带冲突数据")
		}
		return nil, nil
	}
	typ, err := snapshotType(encoded.Type, 0)
	if err != nil {
		return nil, err
	}
	if typ.Kind() != reflect.Map && typ.Kind() != reflect.Array && typ.Kind() != reflect.Slice {
		if encoded.Nil || len(encoded.Items) > 0 || len(encoded.Fields) > 0 {
			return nil, errors.New("标量类型携带容器标记")
		}
		return decodeSnapshotScalar(encoded, typ)
	}
	if encoded.Type == "[]uint8" {
		if len(encoded.Items) > 0 || len(encoded.Fields) > 0 || encoded.Nil && encoded.Text != "" {
			return nil, errors.New("二进制类型携带冲突数据")
		}
		if encoded.Nil {
			return []byte(nil), nil
		}
		data, err := base64.StdEncoding.DecodeString(encoded.Text)
		if err != nil {
			return nil, errors.New("快照二进制内容无效")
		}
		return data, nil
	}
	if encoded.Text != "" || encoded.Nil && (len(encoded.Items) > 0 || len(encoded.Fields) > 0 || typ.Kind() == reflect.Array) {
		return nil, errors.New("容器类型携带冲突数据")
	}
	if encoded.Nil {
		return reflect.Zero(typ).Interface(), nil
	}
	var result reflect.Value
	// 先检查集合形状与长度，再分配目标容器，禁止静默丢弃不匹配的数据。
	switch typ.Kind() {
	case reflect.Map:
		if len(encoded.Items) > 0 {
			return nil, errors.New("映射类型携带顺序元素")
		}
		result = reflect.MakeMapWithSize(typ, len(encoded.Fields))
		for key, child := range encoded.Fields {
			value, err := decodeSnapshotValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			item, err := snapshotAssignable(value, typ.Elem())
			if err != nil {
				return nil, err
			}
			result.SetMapIndex(reflect.ValueOf(key), item)
		}
	case reflect.Array, reflect.Slice:
		if len(encoded.Fields) > 0 || typ.Kind() == reflect.Array && typ.Len() != len(encoded.Items) {
			return nil, errors.New("快照集合形状或数组长度不一致")
		}
		if typ.Kind() == reflect.Array {
			result = reflect.New(typ).Elem()
		} else {
			result = reflect.MakeSlice(typ, len(encoded.Items), len(encoded.Items))
		}
		for i, child := range encoded.Items {
			value, err := decodeSnapshotValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			item, err := snapshotAssignable(value, typ.Elem())
			if err != nil {
				return nil, err
			}
			result.Index(i).Set(item)
		}
	}
	return result.Interface(), nil
}

// snapshotAssignable 检查解码值能否原样存入声明的容器元素类型。
// @param value 解码值
// @param target 元素类型
// @return 可赋值的反射值
// @return 类型不一致错误
func snapshotAssignable(value any, target reflect.Type) (reflect.Value, error) {
	if value == nil {
		if target.Kind() == reflect.Interface {
			return reflect.Zero(target), nil
		}
		return reflect.Value{}, errors.New("空值与快照容器元素类型不一致")
	}
	result := reflect.ValueOf(value)
	if !result.Type().AssignableTo(target) {
		return reflect.Value{}, errors.New("快照容器元素类型不一致")
	}
	return result, nil
}

// decodeSnapshotScalar 按精确位宽解析标量，不经过浮点中转。
// @param encoded 编码标量
// @param typ 目标类型
// @return 原始标量
// @return 内容错误
func decodeSnapshotScalar(encoded snapshotValue, typ reflect.Type) (any, error) {
	// 命名标准类型使用显式分支，其他标量只接受内建类型。
	switch encoded.Type {
	case "time.Time":
		value, err := time.Parse(time.RFC3339Nano, encoded.Text)
		if err != nil {
			return nil, errors.New("快照时间格式无效")
		}
		return value, nil
	case "json.Number":
		if !validSnapshotNumber(encoded.Text) {
			return nil, errors.New("快照 JSON 数字无效")
		}
		return json.Number(encoded.Text), nil
	}
	result := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.String:
		result.SetString(encoded.Text)
	case reflect.Bool:
		value, err := strconv.ParseBool(encoded.Text)
		if err != nil {
			return nil, errors.New("快照布尔值无效")
		}
		result.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(encoded.Text, 10, typ.Bits())
		if err != nil {
			return nil, errors.New("快照整数无效或溢出")
		}
		result.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(encoded.Text, 10, typ.Bits())
		if err != nil {
			return nil, errors.New("快照无符号整数无效或溢出")
		}
		result.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(encoded.Text, typ.Bits())
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("快照浮点数无效")
		}
		result.SetFloat(value)
	default:
		return nil, errors.New("快照标量类型无效")
	}
	return result.Interface(), nil
}
