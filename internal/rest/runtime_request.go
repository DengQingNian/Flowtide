package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// validateRuntimeStart 校验启动目标互斥及租户限制，输出原版请求错误。
// @param w 响应
// @param payload 启动请求
// @return 是否可以执行启动
func validateRuntimeStart(w http.ResponseWriter, payload startPayload) bool {
	count := 0
	for _, selector := range []string{payload.ProcessDefinitionID, payload.ProcessDefinitionKey, payload.Message} {
		if selector != "" {
			count++
		}
	}
	if count == 0 {
		writeRuntimeFailure(w, 400, "Either processDefinitionId, processDefinitionKey or message is required.")
		return false
	}
	if count > 1 {
		writeRuntimeFailure(w, 400, "Only one of processDefinitionId, processDefinitionKey or message should be set.")
		return false
	}
	if payload.ProcessDefinitionID != "" && payload.TenantID != "" {
		writeRuntimeFailure(w, 400, "TenantId can only be used with either processDefinitionKey or message.")
		return false
	}
	return true
}

// writeRuntimeStartError 将启动时的定义定位失败映射为原版请求错误。
// @param w 响应
// @param payload 启动请求
// @param err 引擎错误
// @return 无
func writeRuntimeStartError(w http.ResponseWriter, payload startPayload, err error) {
	if errors.Is(err, api.ErrNotFound) && payload.ProcessDefinitionKey != "" {
		writeRuntimeFailure(w, 400, fmt.Sprintf("No process definition found for key '%s'", payload.ProcessDefinitionKey))
		return
	}
	writeError(w, err)
}

// parseRuntimeVariables 解析运行时变量并保留声明类型，兼容既有对象形式扩展。
// @param raw 原始变量数组或对象
// @return 精确类型的变量列表
// @return 格式或类型错误
func parseRuntimeVariables(raw json.RawMessage) ([]api.Variable, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, api.NewError("REST-0108", "变量格式非法", api.ErrBadRequest)
		}
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		sort.Strings(names)
		out := make([]api.Variable, 0, len(names))
		for _, name := range names {
			value, err := decodeAsyncVariableValue(values[name], "")
			if err != nil {
				return nil, err
			}
			out = append(out, api.Variable{Name: name, Value: value})
		}
		return out, nil
	}
	out := make([]api.Variable, 0, len(parts))
	for _, part := range parts {
		var variable struct {
			Name  string          `json:"name"`
			Type  string          `json:"type"`
			Scope string          `json:"scope"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(part, &variable); err != nil || variable.Name == "" {
			return nil, api.NewError("REST-0108", "变量名称或对象非法", api.ErrBadRequest)
		}
		// 所有变量先解码后再进入命令，后续非法值不能造成部分变量落库。
		value, err := decodeAsyncVariableValue(variable.Value, variable.Type)
		if err != nil {
			return nil, err
		}
		out = append(out, api.Variable{Name: variable.Name, Type: variable.Type, Scope: variable.Scope, Value: value})
	}
	return out, nil
}

// instanceUpdatePayload 保留可选属性与动作，空属性沿用原版忽略语义。
type instanceUpdatePayload struct {
	Action         string     `json:"action"`         // 实例动作
	Assignee       string     `json:"assignee"`       // 签收用户
	Name           *string    `json:"name"`           // 可选名称
	BusinessKey    *string    `json:"businessKey"`    // 可选业务键
	BusinessStatus *string    `json:"businessStatus"` // 可选业务状态
	DueDate        *time.Time `json:"dueDate"`        // 可选到期时间
}

// applyRuntimeInstanceAction 执行互斥的实例动作或属性更新，并返回动作后的完整快照。
// @param w 响应
// @param r 请求
// @param instance 当前实例
// @param payload 请求体
// @return 更新后的实例
// @return 是否成功
func (s *Server) applyRuntimeInstanceAction(w http.ResponseWriter, r *http.Request, instance api.ProcessInstance, payload instanceUpdatePayload) (api.ProcessInstance, bool) {
	var err error
	// 动作存在时忽略同体属性更新，避免一次请求混合两类原版操作。
	switch payload.Action {
	case "suspend", "activate":
		if payload.Action == "suspend" && instance.Suspended || payload.Action == "activate" && !instance.Suspended {
			state := "active"
			if instance.Suspended {
				state = "suspended"
			}
			writeRuntimeFailure(w, 409, fmt.Sprintf("Process instance with id '%s' is already %s.", instance.ID, state))
			return api.ProcessInstance{}, false
		}
		if payload.Action == "suspend" {
			err = s.engine.RuntimeService().SuspendProcessInstance(r.Context(), instance.ID)
		} else {
			err = s.engine.RuntimeService().ActivateProcessInstance(r.Context(), instance.ID)
		}
		if err == nil {
			instance, err = s.engine.RuntimeService().GetProcessInstance(r.Context(), instance.ID)
		}
	case "claim", "unclaim":
		user := payload.Assignee
		if payload.Action == "unclaim" {
			user = ""
		}
		instance, err = s.engine.RuntimeService().UpdateProcessInstance(r.Context(), instance.ID, api.ProcessInstanceUpdate{ClaimUserID: &user})
	case "":
		instance, err = s.engine.RuntimeService().UpdateProcessInstance(r.Context(), instance.ID, api.ProcessInstanceUpdate{
			Name: payload.Name, BusinessKey: payload.BusinessKey, BusinessStatus: payload.BusinessStatus, DueDate: payload.DueDate,
		})
	default:
		writeRuntimeFailure(w, 400, fmt.Sprintf("Invalid action: '%s'.", payload.Action))
		return api.ProcessInstance{}, false
	}
	if err != nil {
		writeRuntimeError(w, r, err, false)
		return api.ProcessInstance{}, false
	}
	return instance, true
}
