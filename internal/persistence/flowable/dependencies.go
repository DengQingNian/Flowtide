package flowable

// foreignKeys 声明当前支持表的可空外键列与目标表。
var foreignKeys = map[string]map[string]string{
	"ACT_GE_BYTEARRAY":      {"DEPLOYMENT_ID_": "ACT_RE_DEPLOYMENT"},
	"ACT_RE_MODEL":          {"DEPLOYMENT_ID_": "ACT_RE_DEPLOYMENT", "EDITOR_SOURCE_VALUE_ID_": "ACT_GE_BYTEARRAY", "EDITOR_SOURCE_EXTRA_VALUE_ID_": "ACT_GE_BYTEARRAY"},
	"ACT_RE_PROCDEF":        {"DEPLOYMENT_ID_": "ACT_RE_DEPLOYMENT"},
	"ACT_RU_EXECUTION":      {"PROC_INST_ID_": "ACT_RU_EXECUTION", "PARENT_ID_": "ACT_RU_EXECUTION", "SUPER_EXEC_": "ACT_RU_EXECUTION", "PROC_DEF_ID_": "ACT_RE_PROCDEF"},
	"ACT_RU_TASK":           {"EXECUTION_ID_": "ACT_RU_EXECUTION", "PROC_INST_ID_": "ACT_RU_EXECUTION", "PROC_DEF_ID_": "ACT_RE_PROCDEF"},
	"ACT_RU_IDENTITYLINK":   {"TASK_ID_": "ACT_RU_TASK", "PROC_DEF_ID_": "ACT_RE_PROCDEF", "PROC_INST_ID_": "ACT_RU_EXECUTION"},
	"ACT_RU_VARIABLE":       {"EXECUTION_ID_": "ACT_RU_EXECUTION", "PROC_INST_ID_": "ACT_RU_EXECUTION", "BYTEARRAY_ID_": "ACT_GE_BYTEARRAY"},
	"ACT_RU_JOB":            {"EXECUTION_ID_": "ACT_RU_EXECUTION", "PROCESS_INSTANCE_ID_": "ACT_RU_EXECUTION", "PROC_DEF_ID_": "ACT_RE_PROCDEF"},
	"ACT_RU_DEADLETTER_JOB": {"EXECUTION_ID_": "ACT_RU_EXECUTION", "PROCESS_INSTANCE_ID_": "ACT_RU_EXECUTION", "PROC_DEF_ID_": "ACT_RE_PROCDEF"},
	"ACT_RU_EVENT_SUBSCR":   {"EXECUTION_ID_": "ACT_RU_EXECUTION"},
	"ACT_HI_VARINST":        {"BYTEARRAY_ID_": "ACT_GE_BYTEARRAY"},
	"ACT_HI_DETAIL":         {"BYTEARRAY_ID_": "ACT_GE_BYTEARRAY"},
}

// orderedRows 按行级外键依赖稳定排序，允许根执行对自身引用。
// @param rows 待排序行
// @return 父行先于子行的列表
func orderedRows(rows []Row) []Row {
	index := make(map[string]int, len(rows))
	for i, row := range rows {
		id, _ := rowIDValue(row)
		index[row.Table+"\x00"+id] = i
	}
	out := make([]Row, 0, len(rows))
	visited := make([]bool, len(rows))
	active := make([]bool, len(rows))
	// 深度优先遍历只处理本批次内的依赖，已有数据库父行无需重复写入。
	var visit func(int)
	visit = func(i int) {
		if visited[i] || active[i] {
			return
		}
		active[i] = true
		for _, column := range rows[i].Columns() {
			table, foreign := foreignKeys[rows[i].Table][column]
			id, ok := rows[i].Values[column].(string)
			if !foreign || !ok || id == "" {
				continue
			}
			if parent, exists := index[table+"\x00"+id]; exists && parent != i {
				visit(parent)
			}
		}
		active[i] = false
		visited[i] = true
		out = append(out, rows[i])
	}
	for i := range rows {
		visit(i)
	}
	return out
}

// orderOperations 先插入引用目标，再更新引用，最后逆序删除旧行。
// @param operations 待执行操作
// @return 有序操作
func orderOperations(operations []Operation) []Operation {
	groups := map[string][]Row{}
	index := map[string]Operation{}
	for _, op := range operations {
		id, _ := rowIDValue(op.Row)
		index[op.Kind+op.Row.Table+"\x00"+id] = op
		groups[op.Kind] = append(groups[op.Kind], op.Row)
	}
	out := make([]Operation, 0, len(operations))
	// 更新必须位于删除之前，例如模型解除源码引用后才能删除旧字节行。
	for _, kind := range []string{OperationInsert, OperationUpdate, OperationDelete} {
		rows := orderedRows(groups[kind])
		for n := range rows {
			i := n
			if kind == OperationDelete {
				i = len(rows) - 1 - n
			}
			id, _ := rowIDValue(rows[i])
			out = append(out, index[kind+rows[i].Table+"\x00"+id])
		}
	}
	return out
}

// withRevision 复制行并使用已提交行的修订号比较业务字段。
// @param row 新行
// @param previous 上次提交行
// @return 对齐修订号后的独立行
func withRevision(row, previous Row) Row {
	values := make(map[string]any, len(row.Values))
	for column, value := range row.Values {
		values[column] = value
	}
	row.Values = values
	if _, ok := revisionValue(previous); ok {
		row.Values["REV_"] = previous.Values["REV_"]
	}
	return row
}

// CommittedRows 计算成功提交后下一次差量所需的修订号基线。
// @param previous 上次提交行
// @param current 当前业务行
// @return 已推进修订号的行集合
func CommittedRows(previous, current []Row) []Row {
	index := make(map[string]Row, len(previous))
	for _, row := range previous {
		id, _ := rowIDValue(row)
		index[row.Table+"\x00"+id] = row
	}
	out := make([]Row, 0, len(current))
	for _, row := range current {
		id, _ := rowIDValue(row)
		old := index[row.Table+"\x00"+id]
		normalized := withRevision(row, old)
		out = append(out, normalized)
	}
	positions := make(map[string]int, len(out))
	for i, row := range out {
		id, _ := rowIDValue(row)
		positions[row.Table+"\x00"+id] = i
	}
	// 直接采用差量生成的更新版本；使用索引避免按历史总量二次遍历。
	for _, op := range DiffOperations(previous, out) {
		if op.Kind != OperationUpdate {
			continue
		}
		id, _ := rowIDValue(op.Row)
		out[positions[op.Row.Table+"\x00"+id]] = op.Row
	}
	return out
}

// PreserveSchemaRows 保留差量持久化不应删除的数据库系统属性。
// @param previous 已加载行
// @param current 当前业务行
// @return 包含原系统属性的行集合
func PreserveSchemaRows(previous, current []Row) []Row {
	out := make([]Row, 0, len(previous)+len(current))
	for _, row := range current {
		name, _ := row.Values["NAME_"].(string)
		if row.Table != "ACT_GE_PROPERTY" || !isSchemaProperty(name) {
			out = append(out, row)
		}
	}
	for _, row := range previous {
		name, _ := row.Values["NAME_"].(string)
		if row.Table == "ACT_GE_PROPERTY" && isSchemaProperty(name) {
			out = append(out, row)
		}
	}
	return out
}
