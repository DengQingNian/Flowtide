package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
)

// ExportRuntimeRows 导出执行、任务与变量行。
// @param ctx 上下文
// @return 行列表
// @return 导出错误
func (e *Engine) ExportRuntimeRows(ctx context.Context) ([]flowablerow.Row, error) {
	return e.readRows(e.runtimeRowsLocked)
}

// runtimeRowsLocked 在锁内导出运行时行。
// @return 行列表
// @return 导出错误
func (e *Engine) runtimeRowsLocked() ([]flowablerow.Row, error) {

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
		businessKey, tenant := "", ""
		startTime := time.Time{}
		if instance != nil {
			businessKey = instance.BusinessKey
			tenant = instance.TenantID
			startTime = instance.StartTime
			if instance.Suspended {
				suspension = 2
			}
		}
		_ = row.Set("ID_", execution.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("PROC_INST_ID_", execution.ProcessInstanceID)
		_ = row.Set("BUSINESS_KEY_", businessKey)
		_ = row.Set("TENANT_ID_", tenant)
		_ = row.Set("START_TIME_", startTime)
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
		row, rowErr := runtimeRow("ACT_RU_TASK", []string{"ID_", "REV_", "EXECUTION_ID_", "PROC_INST_ID_", "PROC_DEF_ID_", "NAME_", "DESCRIPTION_", "TASK_DEF_KEY_", "PARENT_TASK_ID_", "OWNER_", "ASSIGNEE_", "PRIORITY_", "CREATE_TIME_", "DUE_DATE_", "CATEGORY_", "SUSPENSION_STATE_", "TENANT_ID_", "FORM_KEY_"})
		if rowErr != nil {
			return nil, rowErr
		}
		suspension := 1
		if instance, ok := e.findInstance(task.Task.ProcessInstanceID); ok && instance.Suspended {
			suspension = 2
		}
		tenant := task.Task.TenantID
		if tenant == "" {
			if definition, ok := e.findDefinition(task.Task.ProcessDefinitionID); ok {
				tenant = definition.Info.TenantID
			}
		}
		_ = row.Set("ID_", task.Task.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("EXECUTION_ID_", task.ExecutionID)
		_ = row.Set("PROC_INST_ID_", task.Task.ProcessInstanceID)
		_ = row.Set("PROC_DEF_ID_", task.Task.ProcessDefinitionID)
		_ = row.Set("NAME_", task.Task.Name)
		_ = row.Set("DESCRIPTION_", task.Task.Description)
		_ = row.Set("TASK_DEF_KEY_", task.Task.TaskDefinitionKey)
		_ = row.Set("PARENT_TASK_ID_", task.Task.ParentTaskID)
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
	jobs := make([]*jobState, 0, len(e.state.Jobs))
	for _, job := range e.state.Jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	for _, job := range jobs {
		row, rowErr := runtimeRow("ACT_RU_JOB", []string{"ID_", "REV_", "TYPE_", "EXCLUSIVE_", "EXECUTION_ID_", "PROCESS_INSTANCE_ID_", "PROC_DEF_ID_", "ELEMENT_ID_", "RETRIES_", "EXCEPTION_MSG_", "DUEDATE_", "HANDLER_TYPE_", "HANDLER_CFG_", "CREATE_TIME_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		jobType := "async"
		if strings.Contains(job.Type, "timer") {
			jobType = "timer"
		}
		tenant := ""
		if job.ProcessInstanceID != "" {
			if instance, ok := e.findInstance(job.ProcessInstanceID); ok {
				tenant = instance.TenantID
			}
		}
		if tenant == "" && job.ProcessDefinitionID != "" {
			if definition, ok := e.findDefinition(job.ProcessDefinitionID); ok {
				tenant = definition.Info.TenantID
			}
		}
		config, _ := json.Marshal(timerHandlerConfig{
			Remaining: job.RepeatRemaining, Infinite: job.RepeatInfinite, IntervalMillis: job.IntervalMillis,
		})
		_ = row.Set("ID_", job.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("TYPE_", jobType)
		_ = row.Set("EXCLUSIVE_", true)
		_ = row.Set("EXECUTION_ID_", job.ExecutionID)
		_ = row.Set("PROCESS_INSTANCE_ID_", job.ProcessInstanceID)
		_ = row.Set("PROC_DEF_ID_", job.ProcessDefinitionID)
		_ = row.Set("ELEMENT_ID_", job.ActivityID)
		_ = row.Set("RETRIES_", job.Retries)
		_ = row.Set("EXCEPTION_MSG_", job.Exception)
		_ = row.Set("DUEDATE_", job.DueDate)
		_ = row.Set("HANDLER_TYPE_", job.Type)
		_ = row.Set("HANDLER_CFG_", string(config))
		_ = row.Set("CREATE_TIME_", job.CreatedAt)
		_ = row.Set("TENANT_ID_", tenant)
		rows = append(rows, *row)
	}

	deadLetters := make([]*jobState, 0, len(e.state.DeadLetters))
	for _, job := range e.state.DeadLetters {
		deadLetters = append(deadLetters, job)
	}
	sort.Slice(deadLetters, func(i, j int) bool { return deadLetters[i].ID < deadLetters[j].ID })
	for _, job := range deadLetters {
		row, rowErr := runtimeRow("ACT_RU_DEADLETTER_JOB", []string{"ID_", "REV_", "TYPE_", "EXCLUSIVE_", "EXECUTION_ID_", "PROCESS_INSTANCE_ID_", "PROC_DEF_ID_", "ELEMENT_ID_", "EXCEPTION_MSG_", "DUEDATE_", "HANDLER_TYPE_", "HANDLER_CFG_", "CREATE_TIME_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		tenant := ""
		if job.ProcessInstanceID != "" {
			if instance, ok := e.findInstance(job.ProcessInstanceID); ok {
				tenant = instance.TenantID
			}
		}
		if tenant == "" && job.ProcessDefinitionID != "" {
			if definition, ok := e.findDefinition(job.ProcessDefinitionID); ok {
				tenant = definition.Info.TenantID
			}
		}
		config, _ := json.Marshal(timerHandlerConfig{
			Remaining: job.RepeatRemaining, Infinite: job.RepeatInfinite, IntervalMillis: job.IntervalMillis,
		})
		_ = row.Set("ID_", job.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("TYPE_", "deadletter")
		_ = row.Set("EXCLUSIVE_", true)
		_ = row.Set("EXECUTION_ID_", job.ExecutionID)
		_ = row.Set("PROCESS_INSTANCE_ID_", job.ProcessInstanceID)
		_ = row.Set("PROC_DEF_ID_", job.ProcessDefinitionID)
		_ = row.Set("ELEMENT_ID_", job.ActivityID)
		_ = row.Set("EXCEPTION_MSG_", job.Exception)
		_ = row.Set("DUEDATE_", job.DueDate)
		_ = row.Set("HANDLER_TYPE_", job.Type)
		_ = row.Set("HANDLER_CFG_", string(config))
		_ = row.Set("CREATE_TIME_", job.CreatedAt)
		_ = row.Set("TENANT_ID_", tenant)
		rows = append(rows, *row)
	}

	subscriptions := make([]*subscriptionState, 0, len(e.state.Subscriptions))
	for _, subscription := range e.state.Subscriptions {
		subscriptions = append(subscriptions, subscription)
	}
	sort.Slice(subscriptions, func(i, j int) bool { return subscriptions[i].ID < subscriptions[j].ID })
	for _, subscription := range subscriptions {
		row, rowErr := runtimeRow("ACT_RU_EVENT_SUBSCR", []string{"ID_", "REV_", "EVENT_TYPE_", "EVENT_NAME_", "EXECUTION_ID_", "PROC_INST_ID_", "ACTIVITY_ID_", "CONFIGURATION_", "CREATED_", "PROC_DEF_ID_", "SUB_SCOPE_ID_", "SCOPE_ID_", "SCOPE_DEFINITION_ID_", "SCOPE_DEFINITION_KEY_", "SCOPE_TYPE_", "TENANT_ID_"})
		if rowErr != nil {
			return nil, rowErr
		}
		scopeType := "catch"
		scopeDefinitionID := ""
		scopeDefinitionKey := ""
		switch {
		case subscription.StartEvent:
			scopeType = "start"
		case subscription.BoundaryEventID != "":
			scopeType = "boundary"
			scopeDefinitionID = subscription.BoundaryEventID
		case subscription.EventSubProcessStartID != "":
			scopeType = "event-subprocess"
			scopeDefinitionID = subscription.EventSubProcessStartID
			scopeDefinitionKey = subscription.EventSubProcessID
		case subscription.GatewayGroup != "":
			scopeType = "gateway"
		}
		_ = row.Set("ID_", subscription.ID)
		_ = row.Set("REV_", 1)
		_ = row.Set("EVENT_TYPE_", subscription.EventType)
		_ = row.Set("EVENT_NAME_", subscription.EventName)
		_ = row.Set("EXECUTION_ID_", subscription.ExecutionID)
		_ = row.Set("PROC_INST_ID_", subscription.ProcessInstanceID)
		_ = row.Set("ACTIVITY_ID_", subscription.ActivityID)
		_ = row.Set("CONFIGURATION_", subscription.EventName)
		_ = row.Set("CREATED_", subscription.CreatedAt)
		_ = row.Set("PROC_DEF_ID_", subscription.ProcessDefinitionID)
		_ = row.Set("SUB_SCOPE_ID_", subscription.GatewayGroup)
		_ = row.Set("SCOPE_ID_", subscription.ScopeExecutionID)
		_ = row.Set("SCOPE_DEFINITION_ID_", scopeDefinitionID)
		_ = row.Set("SCOPE_DEFINITION_KEY_", scopeDefinitionKey)
		_ = row.Set("SCOPE_TYPE_", scopeType)
		_ = row.Set("TENANT_ID_", subscription.TenantID)
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Table == rows[j].Table {
			return rowID(rows[i]) < rowID(rows[j])
		}
		return rows[i].Table < rows[j].Table
	})
	return rows, nil
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

// timerHandlerConfig 是定时作业配置。
type timerHandlerConfig struct {
	Remaining      int   // 剩余次数
	Infinite       bool  // 是否无限
	IntervalMillis int64 // 间隔毫秒
}
