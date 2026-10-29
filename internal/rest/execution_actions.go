package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// executionActionPayload 是执行动作请求。
type executionActionPayload struct {
	Action             string         `json:"action"`                       // 动作
	SignalName         string         `json:"signalName,omitempty"`         // 信号名
	MessageName        string         `json:"messageName,omitempty"`        // 消息名
	Variables          []api.Variable `json:"variables,omitempty"`          // 变量
	TransientVariables []api.Variable `json:"transientVariables,omitempty"` // 瞬态变量
}

// handleExecutionAction 对单个执行执行动作。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleExecutionAction(w http.ResponseWriter, r *http.Request) {
	var raw struct {
		Action             string          `json:"action"`
		SignalName         string          `json:"signalName"`
		MessageName        string          `json:"messageName"`
		Variables          json.RawMessage `json:"variables"`
		TransientVariables json.RawMessage `json:"transientVariables"`
	}
	if err := decodeTaskJSON(r, &raw); err != nil {
		writeError(w, err)
		return
	}
	variables, err := parseRuntimeVariables(raw.Variables)
	if err != nil {
		writeError(w, err)
		return
	}
	transient, err := parseRuntimeVariables(raw.TransientVariables)
	if err != nil {
		writeError(w, err)
		return
	}
	payload := executionActionPayload{Action: raw.Action, SignalName: raw.SignalName, MessageName: raw.MessageName, Variables: variables, TransientVariables: transient}
	action := strings.ToLower(payload.Action)
	switch action {
	case "trigger", "signal":
		if err := s.engine.RuntimeService().TriggerExecutionWithTransient(r.Context(), r.PathValue("id"), payload.Variables, payload.TransientVariables); err != nil {
			writeError(w, err)
			return
		}
	case "signal_event_received":
		if payload.SignalName == "" {
			writeError(w, api.NewError("REST-0138", "缺少 signalName", api.ErrBadRequest))
			return
		}
		if err := s.engine.RuntimeService().SignalEventReceivedForExecution(r.Context(), r.PathValue("id"), payload.SignalName, payload.Variables); err != nil {
			writeError(w, err)
			return
		}
	case "message_event_received", "message":
		if payload.MessageName == "" {
			writeError(w, api.NewError("REST-0139", "缺少 messageName", api.ErrBadRequest))
			return
		}
		if err := s.engine.RuntimeService().MessageEventReceivedForExecution(r.Context(), r.PathValue("id"), payload.MessageName, payload.Variables); err != nil {
			writeError(w, err)
			return
		}
	default:
		writeRuntimeFailure(w, 400, fmt.Sprintf("Invalid action: '%s'.", payload.Action))
		return
	}
	execution, err := s.engine.RuntimeService().GetExecution(r.Context(), r.PathValue("id"))
	if err != nil || execution.Ended {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, executionRESTResponse(r, execution))
}

// handleExecutionsAction 广播执行动作。
// @param w 响应
// @param r 请求
func (s *Server) handleExecutionsAction(w http.ResponseWriter, r *http.Request) {
	var payload executionActionPayload
	if err := decodeTaskJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if strings.ToLower(payload.Action) != "signal_event_received" {
		writeError(w, api.NewError("REST-0141", "只支持 signal_event_received", api.ErrBadRequest))
		return
	}
	if payload.SignalName == "" {
		writeError(w, api.NewError("REST-0138", "缺少 signalName", api.ErrBadRequest))
		return
	}
	if _, err := s.engine.RuntimeService().SignalEventReceived(r.Context(), payload.SignalName, payload.Variables); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
