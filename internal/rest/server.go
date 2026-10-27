// Package rest 提供 Flowable 兼容的 HTTP API。
package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// MaxUploadSize 是部署上传的最大字节数。
const MaxUploadSize = 64 << 20

// Server 是 HTTP API 服务。
type Server struct {
	engine api.Engine     // 引擎
	mux    *http.ServeMux // 路由
}

// New 创建 REST 服务。
// @param engine 引擎
// @return 服务
func New(engine api.Engine) *Server {
	server := &Server{engine: engine, mux: http.NewServeMux()}
	server.routes()
	return server
}

// Handler 返回 HTTP 处理器。
// @return 处理器
func (s *Server) Handler() http.Handler { return s.mux }

// routes 注册全部路由。
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /service/management/engine", s.handleEngineInfo)
	s.mux.HandleFunc("POST /service/management/jobs/run", s.handleRunJobs)
	s.mux.HandleFunc("GET /service/management/jobs", s.handleListJobs)
	s.mux.HandleFunc("GET /service/management/deadletter-jobs", s.handleListDeadLetterJobs)
	s.mux.HandleFunc("POST /service/management/jobs/{id}", s.handleJobAction)
	s.mux.HandleFunc("GET /service/repository/deployments", s.handleListDeployments)
	s.mux.HandleFunc("POST /service/repository/deployments", s.handleDeploy)
	s.mux.HandleFunc("GET /service/repository/deployments/{id}", s.handleGetDeployment)
	s.mux.HandleFunc("DELETE /service/repository/deployments/{id}", s.handleDeleteDeployment)
	s.mux.HandleFunc("GET /service/repository/deployments/{id}/resources", s.handleListDeploymentResources)
	s.mux.HandleFunc("GET /service/repository/deployments/{id}/resources/{name}", s.handleGetDeploymentResource)
	s.mux.HandleFunc("GET /service/repository/process-definitions", s.handleListDefinitions)
	s.mux.HandleFunc("GET /service/repository/process-definitions/{id}", s.handleGetDefinition)
	s.mux.HandleFunc("GET /service/repository/process-definitions/{id}/xml", s.handleGetDefinitionXML)
	s.mux.HandleFunc("PUT /service/repository/process-definitions/{id}", s.handleDefinitionAction)
	s.mux.HandleFunc("GET /service/runtime/executions", s.handleListExecutions)
	s.mux.HandleFunc("GET /service/runtime/executions/{id}", s.handleGetExecution)
	s.mux.HandleFunc("POST /service/runtime/process-instances", s.handleStartInstance)
	s.mux.HandleFunc("GET /service/runtime/process-instances", s.handleListInstances)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}", s.handleGetInstance)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}", s.handleDeleteInstance)
	s.mux.HandleFunc("PUT /service/runtime/process-instances/{id}", s.handleInstanceAction)
	s.mux.HandleFunc("POST /service/runtime/process-instances/{id}/migrate", s.handleMigrate)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/variables", s.handleGetVariables)
	s.mux.HandleFunc("GET /service/form/form-data", s.handleGetFormData)
	s.mux.HandleFunc("POST /service/form/form-data", s.handleSubmitFormData)
	s.mux.HandleFunc("GET /service/identity/users", s.handleListUsers)
	s.mux.HandleFunc("POST /service/identity/users", s.handleCreateUser)
	s.mux.HandleFunc("GET /service/identity/users/{id}", s.handleGetUser)
	s.mux.HandleFunc("DELETE /service/identity/users/{id}", s.handleDeleteUser)
	s.mux.HandleFunc("GET /service/identity/groups", s.handleListGroups)
	s.mux.HandleFunc("POST /service/identity/groups", s.handleCreateGroup)
	s.mux.HandleFunc("GET /service/identity/groups/{id}", s.handleGetGroup)
	s.mux.HandleFunc("DELETE /service/identity/groups/{id}", s.handleDeleteGroup)
	s.mux.HandleFunc("GET /service/identity/groups/{id}/members", s.handleListMembers)
	s.mux.HandleFunc("POST /service/identity/groups/{id}/members", s.handleAddMember)
	s.mux.HandleFunc("DELETE /service/identity/groups/{id}/members/{userId}", s.handleRemoveMember)
	s.mux.HandleFunc("POST /service/runtime/messages", s.handleMessage)
	s.mux.HandleFunc("POST /service/runtime/signals", s.handleSignal)
	s.mux.HandleFunc("GET /service/runtime/tasks", s.handleListTasks)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/comments", s.handleListComments)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}/comments", s.handleAddComment)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}/comments/{commentId}", s.handleDeleteComment)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/attachments", s.handleListAttachments)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}/attachments", s.handleAddAttachment)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}", s.handleTaskAction)
	s.mux.HandleFunc("GET /service/history/historic-process-instances", s.handleHistoricProcesses)
	s.mux.HandleFunc("GET /service/history/historic-process-instances/{id}", s.handleHistoricProcess)
	s.mux.HandleFunc("GET /service/history/historic-task-instances/{id}", s.handleHistoricTask)
	s.mux.HandleFunc("GET /service/history/historic-activity-instances", s.handleHistoricActivities)
	s.mux.HandleFunc("GET /service/history/historic-task-instances", s.handleHistoricTasks)
	s.mux.HandleFunc("GET /service/history/historic-variable-instances", s.handleHistoricVariables)
}

// handleHealth 返回存活状态。
// @param w 响应
// @param r 请求
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
}

// handleReady 返回就绪状态。
// @param w 响应
// @param r 请求
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	info, err := s.engine.ManagementService().EngineInfo(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	// 引擎关闭后不应再接收流量
	if closed, ok := info["closed"].(bool); ok && closed {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "DOWN", "engine": info})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "UP", "engine": info})
}

// handleEngineInfo 返回引擎信息。
// @param w 响应
// @param r 请求
func (s *Server) handleEngineInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.engine.ManagementService().EngineInfo(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleRunJobs 执行到期定时器作业。
// @param w 响应
// @param r 请求
func (s *Server) handleRunJobs(w http.ResponseWriter, r *http.Request) {
	executed, err := s.engine.ManagementService().RunDueJobs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executed": executed})
}

// handleListJobs 返回可执行作业。
// @param w 响应
// @param r 请求
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.engine.ManagementService().ListJobs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, jobs, start, size)
}

// handleListDeadLetterJobs 返回死信作业。
// @param w 响应
// @param r 请求
func (s *Server) handleListDeadLetterJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.engine.ManagementService().ListDeadLetterJobs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, jobs, start, size)
}

// jobActionPayload 是作业动作请求体。
type jobActionPayload struct {
	Action  string `json:"action"`  // 动作
	Retries int    `json:"retries"` // 重试次数
}

// handleJobAction 处理作业动作。
// @param w 响应
// @param r 请求
func (s *Server) handleJobAction(w http.ResponseWriter, r *http.Request) {
	var payload jobActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if payload.Action != "moveToExecutable" {
		writeError(w, api.NewError("REST-0112", "不支持的作业动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	if err := s.engine.ManagementService().MoveDeadLetterJobToExecutable(r.Context(), r.PathValue("id"), payload.Retries); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeploy 处理部署请求。
// @param w 响应
// @param r 请求
func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	request, err := parseDeploymentRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	deployment, err := s.engine.RepositoryService().Deploy(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deployment)
}

// handleListDeployments 返回部署列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	deployments, err := s.engine.RepositoryService().ListDeployments(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, deployments, start, size)
}

// handleGetDeployment 返回部署详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	deployment, err := s.engine.RepositoryService().GetDeployment(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deployment)
}

// handleDeleteDeployment 删除部署。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteDeployment(w http.ResponseWriter, r *http.Request) {
	cascade := r.URL.Query().Get("cascade") == "true"
	if err := s.engine.RepositoryService().DeleteDeployment(r.Context(), r.PathValue("id"), cascade); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListDeploymentResources 返回部署资源列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListDeploymentResources(w http.ResponseWriter, r *http.Request) {
	names, err := s.engine.RepositoryService().ListDeploymentResources(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, names)
}

// handleGetDeploymentResource 返回部署资源内容。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDeploymentResource(w http.ResponseWriter, r *http.Request) {
	data, err := s.engine.RepositoryService().GetDeploymentResource(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// parseDeploymentRequest 解析部署请求。
// @param r 请求
// @return 部署请求
// @return 解析错误
func parseDeploymentRequest(r *http.Request) (api.DeploymentRequest, error) {
	request := api.DeploymentRequest{
		Name:      r.URL.Query().Get("deploymentName"),
		TenantID:  r.URL.Query().Get("tenantId"),
		Resources: make(map[string][]byte),
	}
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
			return api.DeploymentRequest{}, api.NewError("REST-0101", "解析 multipart 失败", err)
		}
		for field, headers := range r.MultipartForm.File {
			for _, header := range headers {
				file, err := header.Open()
				if err != nil {
					return api.DeploymentRequest{}, api.NewError("REST-0102", "打开上传文件失败", err)
				}
				data, readErr := io.ReadAll(io.LimitReader(file, MaxUploadSize))
				_ = file.Close()
				if readErr != nil {
					return api.DeploymentRequest{}, api.NewError("REST-0103", "读取上传文件失败", readErr)
				}
				name := header.Filename
				if name == "" {
					name = field
				}
				request.Resources[name] = data
			}
		}
		if request.Name == "" {
			request.Name = r.FormValue("deploymentName")
		}
		if request.TenantID == "" {
			request.TenantID = r.FormValue("tenantId")
		}
	} else {
		data, err := io.ReadAll(io.LimitReader(r.Body, MaxUploadSize+1))
		if err != nil {
			return api.DeploymentRequest{}, api.NewError("REST-0104", "读取请求体失败", err)
		}
		if len(data) > MaxUploadSize {
			return api.DeploymentRequest{}, api.NewError("REST-0105", "部署内容超过大小限制", api.ErrBadRequest)
		}
		name := r.URL.Query().Get("resourceName")
		if name == "" {
			name = "deployment.bpmn20.xml"
		}
		request.Resources[name] = data
	}
	if request.Name == "" {
		request.Name = "deployment"
	}
	if len(request.Resources) == 0 {
		return api.DeploymentRequest{}, api.NewError("REST-0106", "未提供部署文件", api.ErrBadRequest)
	}
	return request, nil
}

// handleListDefinitions 返回流程定义列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListDefinitions(w http.ResponseWriter, r *http.Request) {
	definitions, err := s.engine.RepositoryService().ListProcessDefinitions(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, definitions, start, size)
}

// handleGetDefinition 返回流程定义。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDefinition(w http.ResponseWriter, r *http.Request) {
	definition, err := s.engine.RepositoryService().GetProcessDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, definition)
}

// definitionActionPayload 是流程定义动作请求体。
type definitionActionPayload struct {
	Action                  string `json:"action"`                  // suspend/activate
	IncludeProcessInstances bool   `json:"includeProcessInstances"` // 是否包含实例
}

// handleDefinitionAction 处理定义挂起/激活。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionAction(w http.ResponseWriter, r *http.Request) {
	var payload definitionActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	var err error
	switch payload.Action {
	case "suspend":
		err = s.engine.RepositoryService().SuspendProcessDefinition(r.Context(), r.PathValue("id"), payload.IncludeProcessInstances)
	case "activate":
		err = s.engine.RepositoryService().ActivateProcessDefinition(r.Context(), r.PathValue("id"), payload.IncludeProcessInstances)
	default:
		writeError(w, api.NewError("REST-0110", "不支持的定义动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetDefinitionXML 返回流程定义 XML。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDefinitionXML(w http.ResponseWriter, r *http.Request) {
	xml, err := s.engine.RepositoryService().GetProcessDefinitionXML(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xml)
}

// startPayload 是启动实例请求体。
type startPayload struct {
	ProcessDefinitionKey string          `json:"processDefinitionKey"` // 流程定义 Key
	ProcessDefinitionID  string          `json:"processDefinitionId"`  // 流程定义 ID
	BusinessKey          string          `json:"businessKey"`          // 业务键
	Name                 string          `json:"name"`                 // 名称
	TenantID             string          `json:"tenantId"`             // 租户
	Variables            json.RawMessage `json:"variables"`            // 变量
}

// handleListExecutions 返回执行列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.RuntimeService().ListExecutions(r.Context(), r.URL.Query().Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, items, start, size)
}

// handleGetExecution 返回执行详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	execution, err := s.engine.RuntimeService().GetExecution(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, execution)
}

// handleStartInstance 启动流程实例。
// @param w 响应
// @param r 请求
func (s *Server) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	var payload startPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	variables, err := parseVariables(payload.Variables)
	if err != nil {
		writeError(w, err)
		return
	}
	instance, err := s.engine.RuntimeService().StartProcessInstance(r.Context(), api.StartRequest{
		ProcessDefinitionKey: payload.ProcessDefinitionKey,
		ProcessDefinitionID:  payload.ProcessDefinitionID,
		BusinessKey:          payload.BusinessKey,
		Name:                 payload.Name,
		TenantID:             payload.TenantID,
		Variables:            variables,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, instance)
}

// eventPayload 是消息/信号请求体。
type eventPayload struct {
	MessageName string          `json:"messageName"` // 消息名
	SignalName  string          `json:"signalName"`  // 信号名
	Variables   json.RawMessage `json:"variables"`   // 变量
}

// handleGetFormData 返回表单数据。
// @param w 响应
// @param r 请求
func (s *Server) handleGetFormData(w http.ResponseWriter, r *http.Request) {
	if taskID := r.URL.Query().Get("taskId"); taskID != "" {
		data, err := s.engine.FormService().GetTaskFormData(r.Context(), taskID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, data)
		return
	}
	definitionID := r.URL.Query().Get("processDefinitionId")
	if definitionID == "" {
		writeError(w, api.NewError("REST-0113", "缺少 taskId 或 processDefinitionId", api.ErrBadRequest))
		return
	}
	data, err := s.engine.FormService().GetStartFormData(r.Context(), definitionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// formSubmitPayload 是表单提交请求体。
type formSubmitPayload struct {
	TaskID string         `json:"taskId"` // 任务 ID
	Values map[string]any `json:"values"` // 字段值
}

// handleSubmitFormData 提交任务表单。
// @param w 响应
// @param r 请求
func (s *Server) handleSubmitFormData(w http.ResponseWriter, r *http.Request) {
	var payload formSubmitPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if payload.TaskID == "" {
		writeError(w, api.NewError("REST-0114", "缺少 taskId", api.ErrBadRequest))
		return
	}
	if err := s.engine.FormService().SubmitTaskFormData(r.Context(), payload.TaskID, payload.Values); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListUsers 返回用户列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.engine.IdentityService().ListUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, users, start, size)
}

// handleCreateUser 创建用户。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var user api.User
	if err := decodeJSON(r, &user); err != nil {
		writeError(w, err)
		return
	}
	created, err := s.engine.IdentityService().CreateUser(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleGetUser 查询用户。
// @param w 响应
// @param r 请求
func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	user, err := s.engine.IdentityService().GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// handleDeleteUser 删除用户。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().DeleteUser(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListGroups 返回组列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.engine.IdentityService().ListGroups(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, groups, start, size)
}

// handleCreateGroup 创建组。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var group api.Group
	if err := decodeJSON(r, &group); err != nil {
		writeError(w, err)
		return
	}
	created, err := s.engine.IdentityService().CreateGroup(r.Context(), group)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleGetGroup 查询组。
// @param w 响应
// @param r 请求
func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	group, err := s.engine.IdentityService().GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, group)
}

// handleDeleteGroup 删除组。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().DeleteGroup(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// membershipPayload 是成员关系请求体。
type membershipPayload struct {
	UserID string `json:"userId"` // 用户 ID
}

// handleListMembers 返回组成员。
// @param w 响应
// @param r 请求
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	users, err := s.engine.IdentityService().ListMembers(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// handleAddMember 添加成员。
// @param w 响应
// @param r 请求
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var payload membershipPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.IdentityService().AddMembership(r.Context(), payload.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveMember 移除成员。
// @param w 响应
// @param r 请求
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().RemoveMembership(r.Context(), r.PathValue("userId"), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMessage 投递消息。
// @param w 响应
// @param r 请求
func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	var payload eventPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	variables, err := parseVariables(payload.Variables)
	if err != nil {
		writeError(w, err)
		return
	}
	triggered, err := s.engine.RuntimeService().MessageEventReceived(r.Context(), payload.MessageName, variables)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"triggered": triggered})
}

// handleSignal 广播信号。
// @param w 响应
// @param r 请求
func (s *Server) handleSignal(w http.ResponseWriter, r *http.Request) {
	var payload eventPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	variables, err := parseVariables(payload.Variables)
	if err != nil {
		writeError(w, err)
		return
	}
	triggered, err := s.engine.RuntimeService().SignalEventReceived(r.Context(), payload.SignalName, variables)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"triggered": triggered})
}

// handleListInstances 返回运行中实例。
// @param w 响应
// @param r 请求
func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := s.engine.RuntimeService().ListProcessInstances(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, instances, start, size)
}

// handleGetInstance 返回实例详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	instance, err := s.engine.RuntimeService().GetProcessInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, instance)
}

// instanceActionPayload 是实例动作请求体。
type instanceActionPayload struct {
	Action string `json:"action"` // suspend/activate
}

// handleInstanceAction 处理实例挂起/激活。
// @param w 响应
// @param r 请求
func (s *Server) handleInstanceAction(w http.ResponseWriter, r *http.Request) {
	var payload instanceActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	var err error
	switch payload.Action {
	case "suspend":
		err = s.engine.RuntimeService().SuspendProcessInstance(r.Context(), r.PathValue("id"))
	case "activate":
		err = s.engine.RuntimeService().ActivateProcessInstance(r.Context(), r.PathValue("id"))
	default:
		writeError(w, api.NewError("REST-0111", "不支持的实例动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// migratePayload 是迁移请求体。
type migratePayload struct {
	TargetProcessDefinitionID string            `json:"targetProcessDefinitionId"` // 目标定义 ID
	ActivityMappings          map[string]string `json:"activityMappings"`          // 活动映射
}

// handleMigrate 迁移流程实例。
// @param w 响应
// @param r 请求
func (s *Server) handleMigrate(w http.ResponseWriter, r *http.Request) {
	var payload migratePayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.RuntimeService().MigrateProcessInstance(r.Context(), api.MigrationRequest{
		ProcessInstanceID:         r.PathValue("id"),
		TargetProcessDefinitionID: payload.TargetProcessDefinitionID,
		ActivityMappings:          payload.ActivityMappings,
	}); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteInstance 删除实例。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	reason := r.URL.Query().Get("deleteReason")
	if err := s.engine.RuntimeService().DeleteProcessInstance(r.Context(), r.PathValue("id"), reason); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetVariables 返回实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleGetVariables(w http.ResponseWriter, r *http.Request) {
	variables, err := s.engine.RuntimeService().GetVariables(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, variableResponse(variables))
}

// handleListTasks 返回任务列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	start, size := pageParams(r)
	page, err := s.engine.TaskService().ListTasks(r.Context(), api.TaskFilter{
		Assignee:             r.URL.Query().Get("assignee"),
		Owner:                r.URL.Query().Get("owner"),
		ProcessInstanceID:    r.URL.Query().Get("processInstanceId"),
		ProcessDefinitionKey: r.URL.Query().Get("processDefinitionKey"),
		CandidateUser:        r.URL.Query().Get("candidateUser"),
		Start:                start,
		Size:                 size,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetTask 返回任务详情。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.engine.TaskService().GetTask(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// taskActionPayload 是任务动作请求体。
type taskActionPayload struct {
	Action         string          `json:"action"`         // 动作
	UserID         string          `json:"userId"`         // 操作人
	Variables      json.RawMessage `json:"variables"`      // 流程变量
	LocalVariables json.RawMessage `json:"localVariables"` // 任务变量
}

// commentPayload 是评论请求体。
type commentPayload struct {
	Message string `json:"message"` // 内容
	UserID  string `json:"userId"`  // 作者
}

// handleListComments 返回任务评论。
// @param w 响应
// @param r 请求
func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	comments, err := s.engine.TaskService().ListComments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, comments, start, size)
}

// handleAddComment 添加任务评论。
// @param w 响应
// @param r 请求
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	var payload commentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	comment, err := s.engine.TaskService().AddComment(r.Context(), r.PathValue("id"), payload.UserID, payload.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}

// handleDeleteComment 删除评论。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.TaskService().DeleteComment(r.Context(), r.PathValue("commentId")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListAttachments 返回任务附件。
// @param w 响应
// @param r 请求
func (s *Server) handleListAttachments(w http.ResponseWriter, r *http.Request) {
	attachments, err := s.engine.TaskService().ListAttachments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, attachments, start, size)
}

// handleAddAttachment 添加任务附件。
// @param w 响应
// @param r 请求
func (s *Server) handleAddAttachment(w http.ResponseWriter, r *http.Request) {
	var attachment api.Attachment
	if err := decodeJSON(r, &attachment); err != nil {
		writeError(w, err)
		return
	}
	created, err := s.engine.TaskService().AddAttachment(r.Context(), r.PathValue("id"), attachment)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleTaskAction 处理任务动作。
// @param w 响应
// @param r 请求
func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	var payload taskActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	userID := payload.UserID
	if userID == "" {
		userID = requestUserID(r)
	}
	taskID := r.PathValue("id")
	switch payload.Action {
	case "claim":
		if err := s.engine.TaskService().Claim(r.Context(), taskID, userID); err != nil {
			writeError(w, err)
			return
		}
	case "delegate":
		if err := s.engine.TaskService().Delegate(r.Context(), taskID, userID); err != nil {
			writeError(w, err)
			return
		}
	case "complete":
		variables, err := parseVariables(payload.Variables)
		if err != nil {
			writeError(w, err)
			return
		}
		localVariables, err := parseVariables(payload.LocalVariables)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := s.engine.TaskService().Complete(r.Context(), taskID, api.CompleteTaskRequest{
			UserID:         userID,
			Variables:      variables,
			LocalVariables: localVariables,
		}); err != nil {
			writeError(w, err)
			return
		}
	default:
		writeError(w, api.NewError("REST-0107", "不支持的任务动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleHistoricProcesses 返回历史流程实例。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricProcesses(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.HistoryService().ListHistoricProcessInstances(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, items, start, size)
}

// handleHistoricProcess 返回历史流程实例详情。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricProcess(w http.ResponseWriter, r *http.Request) {
	item, err := s.engine.HistoryService().GetHistoricProcessInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleHistoricTask 返回历史任务详情。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricTask(w http.ResponseWriter, r *http.Request) {
	item, err := s.engine.HistoryService().GetHistoricTaskInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleHistoricActivities 返回历史活动。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricActivities(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.HistoryService().ListHistoricActivityInstances(r.Context(), r.URL.Query().Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, items, start, size)
}

// handleHistoricTasks 返回历史任务。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricTasks(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.HistoryService().ListHistoricTaskInstances(r.Context(), r.URL.Query().Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, items, start, size)
}

// handleHistoricVariables 返回历史变量。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricVariables(w http.ResponseWriter, r *http.Request) {
	items, err := s.engine.HistoryService().ListHistoricVariableInstances(r.Context(), r.URL.Query().Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, items, start, size)
}

// parseVariables 解析变量数组或对象。
// @param raw 原始 JSON
// @return 变量列表
// @return 解析错误
func parseVariables(raw json.RawMessage) ([]api.Variable, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var variables []api.Variable
	if err := json.Unmarshal(raw, &variables); err == nil {
		return variables, nil
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, api.NewError("REST-0108", "变量格式非法", api.ErrBadRequest)
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]api.Variable, 0, len(values))
	for _, name := range names {
		out = append(out, api.Variable{Name: name, Value: values[name]})
	}
	return out, nil
}

// variableResponse 将变量映射转换为 Flowable 风格数组。
// @param variables 变量映射
// @return 变量数组
func variableResponse(variables map[string]any) []api.Variable {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]api.Variable, 0, len(names))
	for _, name := range names {
		out = append(out, api.Variable{Name: name, Value: variables[name]})
	}
	return out
}

// decodeJSON 解码 JSON 请求体。
// @param r 请求
// @param target 目标对象
// @return 解析错误
func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, MaxUploadSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return api.NewError("REST-0109", "请求体 JSON 非法", err)
	}
	return nil
}

// requestUserID 从请求中解析操作人。
// @param r 请求
// @return 用户 ID
func requestUserID(r *http.Request) string {
	if userID := r.Header.Get("X-User-Id"); userID != "" {
		return userID
	}
	if user, _, ok := r.BasicAuth(); ok {
		return user
	}
	return ""
}

// pageParams 解析分页参数。
// @param r 请求
// @return 偏移
// @return 每页数量
func pageParams(r *http.Request) (int, int) {
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if start < 0 {
		start = 0
	}
	if size <= 0 {
		size = 10
	}
	if size > 1000 {
		size = 1000
	}
	return start, size
}

// writePage 输出分页结果。
// @param w 响应
// @param items 全部数据
// @param start 偏移
// @param size 每页数量
func writePage(w http.ResponseWriter, items any, start, size int) {
	data := reflectSlice(items)
	total := len(data)
	if start >= total {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": total, "start": start, "size": size})
		return
	}
	end := start + size
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data[start:end], "total": total, "start": start, "size": size})
}

// reflectSlice 将切片接口化。
// @param items 切片
// @return 元素列表
func reflectSlice(items any) []any {
	switch typed := items.(type) {
	case []api.Deployment:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.User:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.Group:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.Comment:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.Attachment:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.ExecutionInfo:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.JobInfo:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.ProcessDefinition:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.ProcessInstance:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.HistoricProcessInstance:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.HistoricActivityInstance:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.HistoricVariableInstance:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.HistoricTaskInstance:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	default:
		return []any{}
	}
}

// writeJSON 输出 JSON。
// @param w 响应
// @param status HTTP 状态码
// @param payload 响应体
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError 输出错误响应。
// @param w 响应
// @param err 错误
func writeError(w http.ResponseWriter, err error) {
	status := statusFor(err)
	body := map[string]any{"message": http.StatusText(status)}
	var engineErr *api.Error
	if errors.As(err, &engineErr) {
		body["message"] = engineErr.Message
		body["code"] = engineErr.Code
		if engineErr.Cause != nil {
			body["exception"] = engineErr.Cause.Error()
		}
	} else {
		body["exception"] = err.Error()
	}
	writeJSON(w, status, body)
}

// statusFor 将错误映射为 HTTP 状态码。
// @param err 错误
// @return HTTP 状态码
func statusFor(err error) int {
	var engineErr *api.Error
	if errors.As(err, &engineErr) {
		switch {
		case strings.HasPrefix(engineErr.Code, "BPMN-"):
			return http.StatusBadRequest
		case strings.HasPrefix(engineErr.Code, "REST-"):
			return http.StatusBadRequest
		}
	}
	switch {
	case errors.Is(err, api.ErrBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, api.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, api.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, api.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, api.ErrEngineClosed):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// String 返回服务描述。
// @return 描述
func (s *Server) String() string {
	return fmt.Sprintf("flowable rest server (%T)", s.engine)
}
