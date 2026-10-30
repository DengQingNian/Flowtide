package engine

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// asyncVariablePayload 以类型编码保存变量，避免重启后大整数、日期及字节退化为 JSON 通用值。
type asyncVariablePayload struct {
	Version   int                      `json:"version"`          // 载荷版本
	Global    bool                     `json:"global,omitempty"` // 是否从目标执行向父作用域解析变量所有权
	Variables map[string]snapshotValue `json:"variables"`        // 精确变量编码
}

// SetVariablesAsync 在单次命令事务中验证整批变量并创建持久作业，不提前写入变量。
// @param ctx 请求上下文
// @param request 目标作用域、变量及创建更新约束
// @return 验证或持久化错误
func (e *Engine) SetVariablesAsync(ctx context.Context, request api.AsyncVariableRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := e.execute(ctx, func(cc *commandContext) (any, error) {
		// 等待命令锁期间也可能取消，必须在修改状态前再次检查。
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if (request.ProcessInstanceID == "") == (request.ExecutionID == "") || len(request.Variables) == 0 {
			return nil, api.NewError("ENGINE-1120", "异步变量必须指定唯一目标和非空变量列表", api.ErrBadRequest)
		}
		id := request.ExecutionID
		if request.ProcessInstanceID != "" {
			if err := e.allowInstance(ctx, request.ProcessInstanceID); err != nil {
				return nil, err
			}
			instance, _ := e.findInstance(request.ProcessInstanceID)
			id = instance.RootExecutionID
		}
		execution, ok := e.findExecution(id)
		if !ok || execution.Ended {
			return nil, api.NewError("ENGINE-0405", "执行不存在", api.ErrNotFound)
		}
		if err := e.allowInstance(ctx, execution.ProcessInstanceID); err != nil {
			return nil, err
		}
		payload, err := e.prepareAsyncVariables(execution, request)
		if err != nil {
			return nil, err
		}
		// 全批验证和序列化完成后才创建作业，后续持久化失败由命令事务统一回滚。
		job := &jobState{ID: e.newID(), Type: "variable-update", ProcessInstanceID: execution.ProcessInstanceID, ProcessDefinitionID: execution.DefinitionID, ExecutionID: execution.ID, ActivityID: execution.ActivityID, Retries: 3, DueDate: cc.now, CreatedAt: cc.now, VariablePayload: payload}
		e.state.Jobs[job.ID] = job
		return nil, nil
	})
	return err
}

// prepareAsyncVariables 校验整批范围与存在性，并生成不共享调用方内存的精确载荷。
// @param execution 目标执行
// @param request 异步请求
// @return 版本化载荷
// @return 参数、冲突或编码错误
func (e *Engine) prepareAsyncVariables(execution *executionState, request api.AsyncVariableRequest) ([]byte, error) {
	payload := asyncVariablePayload{Version: 1, Variables: make(map[string]snapshotValue, len(request.Variables))}
	scope := ""
	for _, variable := range request.Variables {
		current := variable.Scope
		if current == "" || request.ProcessInstanceID != "" {
			current = "local"
		}
		if (current != "local" && current != "global") || (scope != "" && scope != current) || strings.TrimSpace(variable.Name) == "" {
			return nil, api.NewError("ENGINE-1120", "变量名称或统一作用域非法", api.ErrBadRequest)
		}
		scope = current
		if _, duplicate := payload.Variables[variable.Name]; duplicate {
			return nil, api.NewError("ENGINE-1120", "异步变量名称重复", api.ErrBadRequest)
		}
		visible := execution.Variables
		if current == "global" {
			parent, ok := e.findExecution(execution.ParentID)
			if !ok {
				return nil, api.NewError("ENGINE-1120", "根执行不存在可写的父作用域", api.ErrBadRequest)
			}
			visible = e.variablesOf(parent)
			payload.Global = true
		}
		_, exists := visible[variable.Name]
		if request.CreateOnly && exists {
			return nil, api.NewError("ENGINE-1121", "变量已存在，请使用更新方法", &api.VariableConflictError{Name: variable.Name, ExecutionID: execution.ID})
		}
		if request.RequireExisting && !exists {
			return nil, api.NewError("ENGINE-1104", "变量不存在", api.ErrNotFound)
		}
		encoded, err := encodeSnapshotValue(variable.Value)
		if err != nil {
			return nil, api.NewError("ENGINE-1120", "异步变量无法序列化", api.ErrBadRequest)
		}
		payload.Variables[variable.Name] = encoded
	}
	return json.Marshal(payload)
}

// applyAsyncVariables 在作业命令中恢复并原子写入变量，失败由现有作业重试和死信机制接管。
// @param cc 命令上下文
// @param job 变量更新作业
// @return 解码、作用域或条件事件错误
func (e *Engine) applyAsyncVariables(cc *commandContext, job *jobState) error {
	execution, ok := e.findExecution(job.ExecutionID)
	if !ok || execution.Ended {
		return nil
	}
	var payload asyncVariablePayload
	if err := json.Unmarshal(job.VariablePayload, &payload); err != nil || payload.Version != 1 || len(payload.Variables) == 0 {
		return api.NewError("ENGINE-1122", "异步变量载荷损坏", api.ErrBadRequest)
	}
	// 先完整解码后改值，禁止某个变量编码损坏时留下前半批更新。
	values := make(map[string]any, len(payload.Variables))
	for name, encoded := range payload.Variables {
		if strings.TrimSpace(name) == "" {
			return api.NewError("ENGINE-1122", "异步变量名称损坏", api.ErrBadRequest)
		}
		value, err := decodeSnapshotValue(encoded, 0)
		if err != nil {
			return err
		}
		values[name] = value
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if payload.Global {
			parent, ok := e.findExecution(execution.ParentID)
			if !ok {
				return api.NewError("ENGINE-1122", "异步变量父作用域已不存在", api.ErrNotFound)
			}
			e.setVariable(parent, name, values[name])
		} else {
			e.setVariableLocal(execution, name, values[name])
		}
	}
	return e.evaluateConditionalSubscriptions(cc, execution.ProcessInstanceID)
}

// asyncVariableRows 将待执行及死信载荷存入独立字节行，不受 HANDLER_CFG_ 文本列容量限制。
// @param rows 运行时导出行
// @return 带外置异步变量载荷的行
func (e *Engine) asyncVariableRows(rows []flowablerow.Row) []flowablerow.Row {
	out := make([]flowablerow.Row, 0, len(rows))
	for _, row := range rows {
		var job *jobState
		id, _ := row.Values["ID_"].(string)
		if row.Table == "ACT_RU_JOB" {
			job = e.state.Jobs[id]
		} else if row.Table == "ACT_RU_DEADLETTER_JOB" {
			job = e.state.DeadLetters[id]
		}
		if job != nil && job.Type == "variable-update" {
			ref := byteArrayID("async-variable", job.ID)
			data, _ := runtimeRow("ACT_GE_BYTEARRAY", []string{"ID_", "REV_", "NAME_", "DEPLOYMENT_ID_", "BYTES_"})
			_ = data.Set("ID_", ref)
			_ = data.Set("REV_", 1)
			_ = data.Set("NAME_", "flowtide-job-variables")
			_ = data.Set("DEPLOYMENT_ID_", nil)
			_ = data.Set("BYTES_", append([]byte(nil), job.VariablePayload...))
			_ = row.Set("CUSTOM_VALUES_ID_", ref)
			row.InsertColumns = append(row.InsertColumns, "CUSTOM_VALUES_ID_")
			out = append(out, *data)
		}
		out = append(out, row)
	}
	return out
}
