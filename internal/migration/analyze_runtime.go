package migration

import (
	"encoding/json"
	"math"
	"strconv"

	"github.com/flowable-go/flowable/internal/persistence/flowable"
)

// checkExecution 校验简单执行树的根、父链、定义和复杂状态。
// @param row 执行行
// @return 无
func (s *analysisState) checkExecution(row flowable.Row) {
	instance := analysisString(row, "PROC_INST_ID_")
	root := s.index["ACT_RU_EXECUTION"][instance]
	if instance == "" || root.Values == nil || analysisString(root, "PROC_INST_ID_") != instance || s.tenant(root) != s.tenant(row) {
		s.add(row, "EXECUTION_ROOT", "执行缺少有效同租户流程实例根。")
	}
	definition := s.index["ACT_RE_PROCDEF"][analysisString(row, "PROC_DEF_ID_")]
	if definition.Values == nil || s.tenant(definition) != s.tenant(row) || root.Values != nil && analysisString(root, "PROC_DEF_ID_") != analysisString(row, "PROC_DEF_ID_") {
		s.add(row, "EXECUTION_DEFINITION", "执行引用的流程定义缺失或属于其他租户。")
	}
	activity := analysisString(row, "ACT_ID_")
	if process := s.processes[analysisString(row, "PROC_DEF_ID_")]; process != nil && activity != "" {
		element := process.Elements[activity]
		if element == nil || element.ElementType() != "userTask" {
			s.add(row, "EXECUTION_ACTIVITY", "当前执行活动不存在或不在简单用户任务等待分析范围。")
		}
	}
	// 沿父链校验归属并记录访问集合，避免损坏来源中的循环无限遍历。
	seen := map[string]bool{analysisID(row): true}
	parentID := analysisString(row, "PARENT_ID_")
	for parentID != "" {
		parent := s.index["ACT_RU_EXECUTION"][parentID]
		if seen[parentID] || parent.Values == nil || analysisString(parent, "PROC_INST_ID_") != instance || s.tenant(parent) != s.tenant(row) || analysisString(parent, "PROC_DEF_ID_") != analysisString(row, "PROC_DEF_ID_") {
			s.add(row, "EXECUTION_PARENT", "执行父链缺失、循环或跨实例及租户。 ")
			break
		}
		seen[parentID] = true
		parentID = analysisString(parent, "PARENT_ID_")
	}
	if analysisID(row) != instance && analysisString(row, "PARENT_ID_") == "" {
		s.add(row, "EXECUTION_PARENT", "非根执行缺少父执行。")
	}
	if analysisTrue(row.Values["IS_MI_ROOT_"]) || analysisTrue(row.Values["IS_CONCURRENT_"]) || analysisTrue(row.Values["IS_EVENT_SCOPE_"]) || analysisString(row, "SUPER_EXEC_") != "" || analysisString(row, "SCOPE_TYPE_") != "" {
		s.add(row, "EXECUTION_COMPLEX", "多实例、并发、事件作用域或调用关系尚未完成存量转换验证。")
	}
}

// checkTask 校验任务及其执行、定义和流程实例归属。
// @param row 任务行
// @return 无
func (s *analysisState) checkTask(row flowable.Row) {
	executionID := analysisString(row, "EXECUTION_ID_")
	instance := analysisString(row, "PROC_INST_ID_")
	execution := s.index["ACT_RU_EXECUTION"][executionID]
	// 独立任务可以没有流程归属，但不能保留不完整的流程引用。
	if executionID != "" || instance != "" {
		if execution.Values == nil || instance != analysisString(execution, "PROC_INST_ID_") || s.tenant(row) != s.tenant(execution) || analysisString(row, "PROC_DEF_ID_") != analysisString(execution, "PROC_DEF_ID_") {
			s.add(row, "TASK_OWNER", "任务引用执行、实例、定义或租户归属不一致。")
		}
		key := analysisString(row, "TASK_DEF_KEY_")
		process := s.processes[analysisString(row, "PROC_DEF_ID_")]
		if key == "" || key != analysisString(execution, "ACT_ID_") || process == nil || process.Elements[key] == nil || process.Elements[key].ElementType() != "userTask" {
			s.add(row, "TASK_ACTIVITY", "任务活动键与执行活动或流程模型中的用户任务不一致。")
		}
	}
	if analysisString(row, "SCOPE_TYPE_") != "" || analysisString(row, "SCOPE_ID_") != "" {
		s.add(row, "TASK_SCOPE", "任务包含尚未支持的外部作用域。")
	}
}

// checkVariable 校验变量类型、值可解码性、字节引用与所有权。
// @param row 变量行
// @return 无
func (s *analysisState) checkVariable(row flowable.Row) {
	executionID, taskID, instance := analysisString(row, "EXECUTION_ID_"), analysisString(row, "TASK_ID_"), analysisString(row, "PROC_INST_ID_")
	execution, task := s.index["ACT_RU_EXECUTION"][executionID], s.index["ACT_RU_TASK"][taskID]
	validOwner := (executionID != "" && execution.Values != nil) || (taskID != "" && task.Values != nil)
	// 所有已填写引用都必须存在并相互吻合，不能用一个有效引用掩盖另一孤儿引用。
	if executionID != "" && (execution.Values == nil || instance != analysisString(execution, "PROC_INST_ID_")) {
		validOwner = false
	}
	if taskID != "" && (task.Values == nil || instance != analysisString(task, "PROC_INST_ID_") || executionID != "" && executionID != analysisString(task, "EXECUTION_ID_")) {
		validOwner = false
	}
	if execution.Values != nil && task.Values != nil && s.tenant(execution) != s.tenant(task) {
		validOwner = false
	}
	if !validOwner {
		s.add(row, "VARIABLE_OWNER", "变量缺少有效所有者或执行、任务及流程实例归属冲突。")
	}
	if analysisString(row, "SCOPE_TYPE_") != "" || analysisString(row, "SCOPE_ID_") != "" {
		s.add(row, "VARIABLE_SCOPE", "变量包含尚未支持的外部作用域。")
	}
	kind := analysisString(row, "TYPE_")
	switch kind {
	case "null", "string", "longString", "boolean", "short", "integer", "long", "double", "date", "json", "longJson", "bytes":
	default:
		s.add(row, "VARIABLE_TYPE", "变量类型不在显式支持清单内，禁止隐式降级或 Java 反序列化。")
		return
	}
	integer, intOK := analysisInteger(row.Values["LONG_"])
	number, numOK := analysisNumber(row.Values["DOUBLE_"])
	text := analysisString(row, "TEXT_")
	var textPointer *string
	if row.Values["TEXT_"] != nil {
		textPointer = &text
	}
	var data []byte
	reference := analysisString(row, "BYTEARRAY_ID_")
	if reference != "" {
		array := s.index["ACT_GE_BYTEARRAY"][reference]
		var exists bool
		data, exists = array.Values["BYTES_"].([]byte)
		if !exists || data == nil {
			s.add(row, "VARIABLE_DATA", "变量引用的外置字节数据缺失或格式错误。")
			return
		}
	}
	if !intOK || !numOK {
		s.add(row, "VARIABLE_DATA", "变量数值列无法按有符号整数或浮点类型解码。")
		return
	}
	if _, err := flowable.VariableValue(kind, integer, number, textPointer, data); err != nil {
		s.add(row, "VARIABLE_DATA", "变量内容无法按声明类型解码。")
		return
	}
	if kind == "json" && !json.Valid([]byte(text)) || kind == "longJson" && !json.Valid(data) {
		s.add(row, "VARIABLE_DATA", "JSON 变量包含不完整内容或多余尾部数据。")
	}
}

// analysisInteger 精确解析数据库整数列并拒绝溢出及非整数表示。
// @param value 整数列
// @return 可空整数
// @return 是否合法
func analysisInteger(value any) (*int64, bool) {
	if value == nil {
		return nil, true
	}
	var number int64
	// 驱动可能返回整数或文本，禁止通过浮点转换丢失精度。
	switch v := value.(type) {
	case int:
		number = int64(v)
	case int64:
		number = v
	case bool:
		if v {
			number = 1
		}
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, false
		}
		number = parsed
	case []byte:
		return analysisInteger(string(v))
	default:
		return nil, false
	}
	return &number, true
}

// analysisNumber 解析数据库浮点列。
// @param value 浮点列
// @return 可空浮点
// @return 是否合法
func analysisNumber(value any) (*float64, bool) {
	if value == nil {
		return nil, true
	}
	var number float64
	// 兼容两种驱动的数值表示，并统一在返回前排除非有限值。
	switch v := value.(type) {
	case float64:
		number = v
	case int64:
		number = float64(v)
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, false
		}
		number = parsed
	case []byte:
		return analysisNumber(string(v))
	default:
		return nil, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return nil, false
	}
	return &number, true
}

// analysisTrue 识别数据库整数及布尔标记。
// @param value 标记列
// @return 是否为真
func analysisTrue(value any) bool {
	number, ok := analysisInteger(value)
	return ok && number != nil && *number != 0
}
