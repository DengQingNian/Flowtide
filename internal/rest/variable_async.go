package rest

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/flowable-go/flowable/api"
)

// handleCreateExecutionVariable 创建执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateExecutionVariable(w http.ResponseWriter, r *http.Request) {
	variables, err := readVariableBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, variable := range variables {
		if err := s.engine.RuntimeService().SetExecutionVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusCreated, variables)
}

// handleAsyncInstanceVariables 持久排队实例变量集合，返回官方集合创建状态码。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleAsyncInstanceVariables(w http.ResponseWriter, r *http.Request) {
	s.handleAsyncVariables(w, r, true, false)
}

// handleAsyncInstanceVariable 持久排队单个既有实例变量更新。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleAsyncInstanceVariable(w http.ResponseWriter, r *http.Request) {
	s.handleAsyncVariables(w, r, true, true)
}

// handleAsyncExecutionVariables 持久排队执行作用域的变量集合。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleAsyncExecutionVariables(w http.ResponseWriter, r *http.Request) {
	s.handleAsyncVariables(w, r, false, false)
}

// handleAsyncExecutionVariable 持久排队单个既有执行变量更新。
// @param w 响应
// @param r 请求
// @return 无
func (s *Server) handleAsyncExecutionVariable(w http.ResponseWriter, r *http.Request) {
	s.handleAsyncVariables(w, r, false, true)
}

// handleAsyncVariables 统一执行异步请求校验及事务排队，不在 HTTP 请求内写入变量。
// @param w 响应
// @param r 请求
// @param instance 是否使用流程实例根作用域
// @param single 是否为单个既有变量更新
// @return 无
func (s *Server) handleAsyncVariables(w http.ResponseWriter, r *http.Request, instance, single bool) {
	variables, err := readAsyncVariables(r)
	if err != nil {
		writeAsyncVariableError(w, err)
		return
	}
	// 单变量端点必须同时校验路径和请求体名称，防止把错误变量写入作业。
	if single && (len(variables) != 1 || variables[0].Name != r.PathValue("name")) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Bad request", "exception": "Variable name in the body should be equal to the name used in the requested URL."})
		return
	}
	request := api.AsyncVariableRequest{Variables: variables, CreateOnly: r.Method == http.MethodPost, RequireExisting: single}
	if instance {
		request.ProcessInstanceID = r.PathValue("id")
	} else {
		request.ExecutionID = r.PathValue("id")
	}
	if err := s.engine.RuntimeService().SetVariablesAsync(r.Context(), request); err != nil {
		var conflict *api.VariableConflictError
		if errors.As(err, &conflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "Conflict", "exception": fmt.Sprintf("Variable '%s' is already present on execution '%s'.", conflict.Name, conflict.ExecutionID)})
			return
		}
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	// 固定 Java 执行集合控制器使用 ResponseStatus(204)，覆盖基类设置的 201；实例集合保持 201。
	if single || !instance {
		status = http.StatusNoContent
	}
	w.WriteHeader(status)
}

// writeAsyncVariableError 保持原版异步变量 JSON 格式错误的响应字段及文本。
// @param w 响应
// @param err 请求解析错误
// @return 无
func writeAsyncVariableError(w http.ResponseWriter, err error) {
	var apiError *api.Error
	if errors.As(err, &apiError) {
		message := ""
		// 固定控制器对集合反序列化、空集合和单对象反序列化使用不同异常文本。
		switch apiError.Code {
		case "REST-ASYNC-LIST":
			message = "Failed to serialize to a RestVariable instance"
		case "REST-ASYNC-EMPTY":
			message = "Request did not contain a list of variables to create."
		case "REST-ASYNC-SINGLE":
			message = "request body could not be transformed to a RestVariable instance."
		}
		if message != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Bad request", "exception": message})
			return
		}
	}
	writeError(w, err)
}
