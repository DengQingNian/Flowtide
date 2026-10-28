package engine

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/flowable-go/flowable/api"
)

// snapshotTypedValues 在业务对象之外记录变量类型，避免业务 JSON 与内部标记混淆。
type snapshotTypedValues struct {
	Executions        map[string]snapshotExecutionValues `json:"executions"`        // 执行局部变量及多实例集合
	Tasks             map[string]snapshotValue           `json:"tasks"`             // 任务局部变量
	HistoricVariables map[string]snapshotValue           `json:"historicVariables"` // 历史变量值
	HistoricDetails   map[string]snapshotValue           `json:"historicDetails"`   // 历史明细值
}

// snapshotExecutionValues 保存执行中两类可变业务值。
type snapshotExecutionValues struct {
	Variables snapshotValue `json:"variables"` // 执行变量
	MIValues  snapshotValue `json:"miValues"`  // 多实例元素
}

// snapshotValue 是只在类型侧表内使用的递归编码。
type snapshotValue struct {
	Type   string                   `json:"type"`             // 受限 Go 类型描述
	Text   string                   `json:"text,omitempty"`   // 标量的精确文本
	Nil    bool                     `json:"nil,omitempty"`    // 可空容器标记
	Items  []snapshotValue          `json:"items,omitempty"`  // 有序集合
	Fields map[string]snapshotValue `json:"fields,omitempty"` // 字符串键映射
}

// snapshotReference 标识递归栈中的可变容器，拒绝循环但允许共享子对象。
type snapshotReference struct {
	kind    reflect.Type
	pointer uintptr
	length  int
}

var snapshotScalarTypes = map[string]reflect.Type{
	"bool": reflect.TypeOf(false), "string": reflect.TypeOf(""),
	"int": reflect.TypeOf(int(0)), "int8": reflect.TypeOf(int8(0)), "int16": reflect.TypeOf(int16(0)), "int32": reflect.TypeOf(int32(0)), "int64": reflect.TypeOf(int64(0)),
	"uint": reflect.TypeOf(uint(0)), "uint8": reflect.TypeOf(uint8(0)), "uint16": reflect.TypeOf(uint16(0)), "uint32": reflect.TypeOf(uint32(0)), "uint64": reflect.TypeOf(uint64(0)),
	"float32": reflect.TypeOf(float32(0)), "float64": reflect.TypeOf(float64(0)),
	"interface {}": reflect.TypeOf((*any)(nil)).Elem(), "time.Time": reflect.TypeOf(time.Time{}), "json.Number": reflect.TypeOf(json.Number("")),
}

// snapshotWithTypedValues 复制携带变量的快照对象并将变量提取至明确类型侧表。
// @param source 原始快照，其内容不会被修改
// @return 可以编码的新版本快照
// @return 不支持的类型、循环或无效实体错误
func snapshotWithTypedValues(source *snapshot) (*snapshot, error) {
	out := *source
	out.TypedValues = &snapshotTypedValues{Executions: map[string]snapshotExecutionValues{}, Tasks: map[string]snapshotValue{}, HistoricVariables: map[string]snapshotValue{}, HistoricDetails: map[string]snapshotValue{}}
	out.Executions = append([]*executionState(nil), source.Executions...)
	out.Tasks = append([]*taskState(nil), source.Tasks...)
	out.HistoricVariables = append([]*api.HistoricVariableInstance(nil), source.HistoricVariables...)
	out.HistoricDetails = append([]*api.HistoricDetail(nil), source.HistoricDetails...)
	// 只复制包含业务值的实体，禁止把去掉变量的对象写回运行中的状态。
	for i, execution := range source.Executions {
		if execution == nil {
			return nil, errors.New("快照执行不能为空")
		}
		if _, exists := out.TypedValues.Executions[execution.ID]; exists {
			return nil, errors.New("快照执行标识重复")
		}
		variables, err := encodeSnapshotValue(execution.Variables)
		if err != nil {
			return nil, err
		}
		items, err := encodeSnapshotValue(execution.MIValues)
		if err != nil {
			return nil, err
		}
		out.TypedValues.Executions[execution.ID] = snapshotExecutionValues{Variables: variables, MIValues: items}
		copied := *execution
		copied.Variables, copied.MIValues = nil, nil
		out.Executions[i] = &copied
	}
	for i, task := range source.Tasks {
		if task == nil {
			return nil, errors.New("快照任务不能为空")
		}
		if _, exists := out.TypedValues.Tasks[task.Task.ID]; exists {
			return nil, errors.New("快照任务标识重复")
		}
		value, err := encodeSnapshotValue(task.LocalVariables)
		if err != nil {
			return nil, err
		}
		out.TypedValues.Tasks[task.Task.ID] = value
		copied := *task
		copied.LocalVariables = nil
		out.Tasks[i] = &copied
	}
	for i, historic := range source.HistoricVariables {
		if historic == nil {
			return nil, errors.New("历史变量不能为空")
		}
		if _, exists := out.TypedValues.HistoricVariables[historic.ID]; exists {
			return nil, errors.New("历史变量标识重复")
		}
		value, err := encodeSnapshotValue(historic.Value)
		if err != nil {
			return nil, err
		}
		out.TypedValues.HistoricVariables[historic.ID] = value
		copied := *historic
		copied.Value = nil
		out.HistoricVariables[i] = &copied
	}
	for i, detail := range source.HistoricDetails {
		if detail == nil {
			return nil, errors.New("历史明细不能为空")
		}
		if _, exists := out.TypedValues.HistoricDetails[detail.ID]; exists {
			return nil, errors.New("历史明细标识重复")
		}
		value, err := encodeSnapshotValue(detail.Value)
		if err != nil {
			return nil, err
		}
		out.TypedValues.HistoricDetails[detail.ID] = value
		copied := *detail
		copied.Value = nil
		out.HistoricDetails[i] = &copied
	}
	return &out, nil
}

// encodeSnapshotValue 将变量转换为明确类型编码。
// @param value 原始变量
// @return 类型编码
// @return 不支持类型或值错误
func encodeSnapshotValue(value any) (snapshotValue, error) {
	return encodeSnapshotReflect(reflect.ValueOf(value), make(map[snapshotReference]bool), 0)
}

// encodeSnapshotReflect 递归编码内建值与字符串键容器，检测循环及过深结构。
// @param value 原始反射值
// @param active 当前递归栈中的容器
// @param depth 当前深度
// @return 类型编码
// @return 值错误
func encodeSnapshotReflect(value reflect.Value, active map[snapshotReference]bool, depth int) (snapshotValue, error) {
	if depth > 128 {
		return snapshotValue{}, errors.New("变量嵌套超过快照限制")
	}
	if !value.IsValid() {
		return snapshotValue{Type: "nil"}, nil
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return snapshotValue{Type: "nil"}, nil
		}
		return encodeSnapshotReflect(value.Elem(), active, depth+1)
	}
	typeName := value.Type().String()
	if _, err := snapshotType(typeName, 0); err != nil {
		return snapshotValue{}, err
	}
	out := snapshotValue{Type: typeName}
	// 时间、原始数字及二进制在 JSON 中有特殊表示，必须显式保留原类型。
	switch typed := value.Interface().(type) {
	case time.Time:
		out.Text = typed.Format(time.RFC3339Nano)
		if _, err := time.Parse(time.RFC3339Nano, out.Text); err != nil {
			return snapshotValue{}, errors.New("时间超出快照范围")
		}
		return out, nil
	case json.Number:
		if !validSnapshotNumber(string(typed)) {
			return snapshotValue{}, errors.New("JSON 数字无效")
		}
		out.Text = string(typed)
		return out, nil
	case []byte:
		out.Nil, out.Text = typed == nil, base64.StdEncoding.EncodeToString(typed)
		return out, nil
	}
	// 可变容器只记录当前路径，允许多个变量共享同一个无环子对象。
	if value.Kind() == reflect.Map || value.Kind() == reflect.Slice {
		if value.IsNil() {
			out.Nil = true
			return out, nil
		}
		key := snapshotReference{kind: value.Type(), pointer: value.Pointer(), length: value.Len()}
		if active[key] {
			return snapshotValue{}, errors.New("变量包含循环容器")
		}
		active[key] = true
		defer delete(active, key)
	}
	switch value.Kind() {
	case reflect.String:
		out.Text = value.String()
	case reflect.Bool:
		out.Text = strconv.FormatBool(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		out.Text = strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		out.Text = strconv.FormatUint(value.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(value.Float()) || math.IsInf(value.Float(), 0) {
			return snapshotValue{}, errors.New("快照不支持非有限浮点数")
		}
		out.Text = strconv.FormatFloat(value.Float(), 'g', -1, value.Type().Bits())
	case reflect.Map:
		out.Fields = make(map[string]snapshotValue, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			child, err := encodeSnapshotReflect(iter.Value(), active, depth+1)
			if err != nil {
				return snapshotValue{}, err
			}
			out.Fields[iter.Key().String()] = child
		}
	case reflect.Slice, reflect.Array:
		out.Items = make([]snapshotValue, value.Len())
		for i := range out.Items {
			child, err := encodeSnapshotReflect(value.Index(i), active, depth+1)
			if err != nil {
				return snapshotValue{}, err
			}
			out.Items[i] = child
		}
	default:
		return snapshotValue{}, errors.New("变量类型不受快照支持")
	}
	return out, nil
}

// validSnapshotNumber 检查保留原始文本的 JSON 数字语法。
// @param text 数字文本
// @return 是否合法
func validSnapshotNumber(text string) bool {
	return text != "" && (text[0] == '-' || text[0] >= '0' && text[0] <= '9') && json.Valid([]byte(text))
}

// snapshotType 将受限类型描述恢复为反射类型，拒绝任意用户类型及非法数组长度。
// @param name 类型描述
// @param depth 当前类型嵌套深度
// @return 内建类型
// @return 描述错误
func snapshotType(name string, depth int) (result reflect.Type, err error) {
	if depth > 128 {
		return nil, errors.New("快照类型嵌套过深")
	}
	if scalar := snapshotScalarTypes[name]; scalar != nil {
		return scalar, nil
	}
	// 反射构造超出地址空间的数组会恐慌，将损坏类型描述转换为读取错误。
	defer func() {
		if recover() != nil {
			result, err = nil, errors.New("快照容器类型超出范围")
		}
	}()
	if strings.HasPrefix(name, "[]") {
		element, err := snapshotType(name[2:], depth+1)
		if err != nil {
			return nil, err
		}
		return reflect.SliceOf(element), nil
	}
	if strings.HasPrefix(name, "map[string]") {
		element, err := snapshotType(strings.TrimPrefix(name, "map[string]"), depth+1)
		if err != nil {
			return nil, err
		}
		return reflect.MapOf(reflect.TypeOf(""), element), nil
	}
	if strings.HasPrefix(name, "[") {
		end := strings.IndexByte(name, ']')
		if end > 1 {
			length, err := strconv.Atoi(name[1:end])
			if err != nil || length < 0 {
				return nil, errors.New("快照数组长度无效")
			}
			element, err := snapshotType(name[end+1:], depth+1)
			if err != nil {
				return nil, err
			}
			return reflect.ArrayOf(length, element), nil
		}
	}
	return nil, errors.New("快照包含不支持的变量类型")
}
