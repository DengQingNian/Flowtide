package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// ExportRuntimeRows 导出执行、任务与变量行。
// @param ctx 上下文
// @return 行列表
// @return 导出错误
func (e *Engine) ExportRuntimeRows(ctx context.Context) ([]flowablerow.Row, error) {
	result, err := e.read(func() (any, error) {
		rows := make([]flowablerow.Row, 0)
		executions := make([]*executionState, 0, len(e.state.Executions))
		for _, execution := range e.state.Executions {
			if execution.Ended {
				continue
			}
			executions = append(executions, execution)
		}
		sort.Slice(executions, func(i, j int) bool { return executions[i].ID < executions[j].ID })
		for _, execution := range executions {
			row, rowErr := runtimeRow("ACT_RU_EXECUTION", []string{"ID_", "REV_", "PROC_INST_ID_", "BUSINESS_KEY_", "PARENT_ID_", "PROC_DEF_ID_", "ACT_ID_", "IS_ACTIVE_", "IS_CONCURRENT_", "IS_SCOPE_", "IS_EVENT_SCOPE_", "IS_MI_ROOT_", "SUSPENSION_STATE_", "TENANT_ID_", "START_TIME_"})
			if rowErr != nil {
				return nil, rowErr
			}
			instance, _ := e.findInstance(execution.ProcessInstanceID)
			suspension := 1
			if instance != nil && instance.Suspended {
				suspension = 2
			}
			_ = row.Set("ID_", execution.ID)
			_ = row.Set("REV_", 1)
			_ = row.Set("PROC_INST_ID_", execution.ProcessInstanceID)
			if instance != nil {
				_ = row.Set("BUSINESS_KEY_", instance.BusinessKey)
				_ = row.Set("TENANT_ID_", instance.TenantID)
				_ = row.Set("START_TIME_", instance.StartTime)
			}
			_ = row.Set("PARENT_ID_", execution.ParentID)
			_ = row.Set("PROC_DEF_ID_", execution.DefinitionID)
			_ = row.Set("ACT_ID_", execution.ActivityID)
			_ = row.Set("IS_ACTIVE_", execution.Active)
			_ = row.Set("IS_CONCURRENT_", execution.Concurrent)
			_ = row.Set("IS_SCOPE_", execution.Scope)
			_ = row.Set("IS_EVENT_SCOPE_", execution.EventSubProcessRoot)
			_ = row.Set("IS_MI_ROOT_", execution.MultiInstanceRoot)
			_ = row.Set("SUSPENSION_STATE_", suspension)
			rows = append(rows, *row)
		}

		tasks := make([]*taskState, 0, len(e.state.Tasks))
		for _, task := range e.state.Tasks {
			tasks = append(tasks, task)
		}
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].Task.ID < tasks[j].Task.ID })
		for _, task := range tasks {
			row, rowErr := runtimeRow("ACT_RU_TASK", []string{"ID_", "REV_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "NAME_", "DESCRIPTION_", "TASK_DEF_KEY_", "OWNER_", "ASSIGNEE_", "PRIORITY_", "CREATE_TIME_", "DUE_DATE_", "CATEGORY_", "SUSPENSION_STATE_", "TENANT_ID_", "FORM_KEY_"})
			if rowErr != nil {
				return nil, rowErr
			}
			suspension := 1
			if instance, ok := e.findInstance(task.Task.ProcessInstanceID); ok && instance.Suspended {
				suspension = 2
			}
			tenant := ""
			if definition, ok := e.findDefinition(task.Task.ProcessDefinitionID); ok {
				tenant = definition.Info.TenantID
			}
			_ = row.Set("ID_", task.Task.ID)
			_ = row.Set("REV_", 1)
			_ = row.Set("EXECUTION_ID_", task.ExecutionID)
			_ = row.Set("PROC_INST_ID_", task.Task.ProcessInstanceID)
			_ = row.Set("PROC_DEF_ID_", task.Task.ProcessDefinitionID)
			_ = row.Set("NAME_", task.Task.Name)
			_ = row.Set("DESCRIPTION_", task.Task.Description)
			_ = row.Set("TASK_DEF_KEY_", task.Task.TaskDefinitionKey)
			_ = row.Set("OWNER_", task.Task.Owner)
			_ = row.Set("ASSIGNEE_", task.Task.Assignee)
			_ = row.Set("PRIORITY_", task.Task.Priority)
			_ = row.Set("CREATE_TIME_", task.Task.CreateTime)
			if task.Task.DueDate != nil {
				_ = row.Set("DUE_DATE_", *task.Task.DueDate)
			} else {
				_ = row.Set("DUE_DATE_", nil)
			}
			_ = row.Set("CATEGORY_", task.Task.Category)
			_ = row.Set("SUSPENSION_STATE_", suspension)
			_ = row.Set("TENANT_ID_", tenant)
			_ = row.Set("FORM_KEY_", task.Task.FormKey)
			rows = append(rows, *row)
		}

		for _, execution := range executions {
			names := make([]string, 0, len(execution.Variables))
			for name := range execution.Variables {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				row, rowErr := variableRow(execution.ID, execution.ProcessInstanceID, "", name, execution.Variables[name])
				if rowErr != nil {
					return nil, rowErr
				}
				rows = append(rows, *row)
			}
		}
		for _, task := range tasks {
			names := make([]string, 0, len(task.LocalVariables))
			for name := range task.LocalVariables {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				row, rowErr := variableRow(task.ExecutionID, task.Task.ProcessInstanceID, task.Task.ID, name, task.LocalVariables[name])
				if rowErr != nil {
					return nil, rowErr
				}
				rows = append(rows, *row)
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Table == rows[j].Table {
				return rowID(rows[i]) < rowID(rows[j])
			}
			return rows[i].Table < rows[j].Table
		})
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]flowablerow.Row), nil
}

// runtimeRow 创建带插入列的行。
// @param table 表名
// @param columns 插入列
// @return 行
// @return 错误
func runtimeRow(table string, columns []string) (*flowablerow.Row, error) {
	row, err := flowablerow.NewRow(table)
	if err != nil {
		return nil, err
	}
	if err := row.SetInsertColumns(columns); err != nil {
		return nil, err
	}
	return row, nil
}

// variableRow 构造变量行。
// @param executionID 执行 ID
// @param processInstanceID 实例 ID
// @param taskID 任务 ID
// @param name 变量名
// @param value 变量值
// @return 行
// @return 错误
func variableRow(executionID, processInstanceID, taskID, name string, value any) (*flowablerow.Row, error) {
	row, err := runtimeRow("ACT_RU_VARIABLE", []string{"ID_", "REV_", "TYPE_", "NAME_", "EXECUTION_ID_", "PROC_INST_ID_", "TASK_ID_", "LONG_", "DOUBLE_", "TEXT_"})
	if err != nil {
		return nil, err
	}
	varType, longValue, doubleValue, textValue := variableColumns(value)
	_ = row.Set("ID_", variableID(executionID, taskID, name))
	_ = row.Set("REV_", 1)
	_ = row.Set("TYPE_", varType)
	_ = row.Set("NAME_", name)
	_ = row.Set("EXECUTION_ID_", executionID)
	_ = row.Set("PROC_INST_ID_", processInstanceID)
	_ = row.Set("TASK_ID_", taskID)
	_ = row.Set("LONG_", longValue)
	_ = row.Set("DOUBLE_", doubleValue)
	_ = row.Set("TEXT_", textValue)
	return row, nil
}

// variableColumns 将变量映射为 ACT 列。
// @param value 变量值
// @return 类型
// @return LONG_
// @return DOUBLE_
// @return TEXT_
func variableColumns(value any) (string, any, any, any) {
	switch typed := value.(type) {
	case string:
		return "string", nil, nil, typed
	case bool:
		if typed {
			return "boolean", int64(1), nil, nil
		}
		return "boolean", int64(0), nil, nil
	case int:
		return "long", int64(typed), nil, nil
	case int64:
		return "long", typed, nil, nil
	case float64:
		return "double", nil, typed, nil
	case nil:
		return "null", nil, nil, nil
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return "json", nil, nil, fmt.Sprint(typed)
		}
		return "json", nil, nil, string(data)
	}
}

// variableID 生成稳定变量 ID。
// @param executionID 执行 ID
// @param taskID 任务 ID
// @param name 变量名
// @return 变量 ID
func variableID(executionID, taskID, name string) string {
	sum := sha256.Sum256([]byte(executionID + "|" + taskID + "|" + name))
	return "var-" + hex.EncodeToString(sum[:])[:32]
}
