// Package rest 提供 Flowable 兼容的 HTTP API。
package rest

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/tenant"
)

// MaxUploadSize 是部署上传的最大字节数。
const MaxUploadSize = 64 << 20

// Server 是 HTTP API 服务。
type Server struct {
	engine          api.Engine          // 引擎
	mux             *http.ServeMux      // 路由
	basicUsers      map[string]string   // Basic Auth 用户
	bearerToken     string              // Bearer Token
	userRoles       map[string][]string // 用户到角色
	rolePermissions map[string][]string // 角色到权限
	identityAuth    bool                // 使用身份库校验密码
	limiter         *rateLimiter        // 限流器
	auditLog        *auditLog           // 审计日志
	httpMetrics     *httpMetrics        // HTTP 指标
}

// Option 是 REST 服务选项。
type Option func(*Server)

// WithBasicAuth 启用 Basic Auth。
// @param users 用户名到密码
// @return 选项
func WithBasicAuth(users map[string]string) Option {
	return func(server *Server) {
		server.basicUsers = make(map[string]string, len(users))
		for user, password := range users {
			server.basicUsers[user] = password
		}
	}
}

// WithBearerToken 启用 Bearer Token。
// @param token 令牌
// @return 选项
func WithBearerToken(token string) Option {
	return func(server *Server) {
		server.bearerToken = token
	}
}

// WithRBAC 启用基于角色的端点授权。
// @param userRoles 用户到角色
// @param rolePermissions 角色到权限（read/write/admin）
// @return 选项
func WithRBAC(userRoles, rolePermissions map[string][]string) Option {
	return func(server *Server) {
		server.userRoles = userRoles
		server.rolePermissions = rolePermissions
	}
}

// WithIdentityAuth 使用身份库校验 Basic Auth 密码。
// @return 选项
func WithIdentityAuth() Option {
	return func(server *Server) {
		server.identityAuth = true
	}
}

// WithRateLimit 启用令牌桶限流。
// @param rate 每秒令牌数
// @param burst 桶容量
// @return 选项
func WithRateLimit(rate float64, burst int) Option {
	return func(server *Server) {
		if rate > 0 && burst > 0 {
			server.limiter = newRateLimiter(rate, burst)
		}
	}
}

// WithAuditLog 启用内存审计日志。
// @param size 保留事件数
// @return 选项
func WithAuditLog(size int) Option {
	return func(server *Server) {
		if size > 0 {
			server.auditLog = newAuditLog(size)
		}
	}
}

// WithMetrics 启用 Prometheus 文本指标。
// @return 选项
func WithMetrics() Option {
	return func(server *Server) {
		server.httpMetrics = newHTTPMetrics()
	}
}

// New 创建 REST 服务。
// @param engine 引擎
// @param options 选项
// @return 服务
func New(engine api.Engine, options ...Option) *Server {
	server := &Server{engine: engine, mux: http.NewServeMux()}
	for _, option := range options {
		option(server)
	}
	server.routes()
	return server
}

// Handler 返回 HTTP 处理器。
// @return 处理器
func (s *Server) Handler() http.Handler {
	return s.metrics(s.rateLimit(s.audit(s.withTenant(s.authenticate(s.mux)))))
}

// withTenant 将请求租户写入上下文。
// @param next 下游处理器
// @return 处理器
func (s *Server) withTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(tenant.With(r.Context(), requestTenant(r))))
	})
}

// authenticate 对 /service 路径执行认证。
// @param next 下游处理器
// @return 处理器
func (s *Server) authenticate(next http.Handler) http.Handler {
	if !s.identityAuth && len(s.basicUsers) == 0 && s.bearerToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/service/") {
			next.ServeHTTP(w, r)
			return
		}
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="flowable"`)
			writeError(w, api.NewError("REST-0401", "未认证", api.ErrUnauthorized))
			return
		}
		if !s.permitted(r) {
			writeError(w, api.NewError("REST-0403", "无权限", api.ErrForbidden))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// permitted 判断请求是否有端点权限。
// @param r 请求
// @return 是否允许
func (s *Server) permitted(r *http.Request) bool {
	if len(s.rolePermissions) == 0 && !s.identityAuth {
		return true
	}
	// Bearer Token 视为服务账号，拥有全部权限
	if s.bearerToken != "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return true
	}
	user, _, ok := r.BasicAuth()
	if !ok {
		return false
	}
	required := requiredPermission(r.Method, r.URL.Path)
	roles := append([]string(nil), s.userRoles[user]...)
	if s.identityAuth {
		if identityUser, err := s.engine.IdentityService().GetUser(r.Context(), user); err == nil {
			roles = append(roles, identityUser.Roles...)
		}
	}
	for _, role := range roles {
		permissions := append([]string(nil), s.rolePermissions[role]...)
		if s.identityAuth {
			if roleEntity, err := s.engine.IdentityService().GetRole(r.Context(), role); err == nil {
				permissions = append(permissions, roleEntity.Permissions...)
			}
		}
		for _, permission := range permissions {
			if permission == "admin" || permission == required {
				return true
			}
		}
	}
	return false
}

// requiredPermission 返回端点所需权限。
// @param method HTTP 方法
// @param path 路径
// @return 权限
func requiredPermission(method, path string) string {
	if strings.HasPrefix(path, "/service/management/") || strings.HasPrefix(path, "/service/identity/") {
		return "admin"
	}
	if method == http.MethodGet || method == http.MethodHead {
		return "read"
	}
	return "write"
}

// authorized 判断请求是否通过认证。
// @param r 请求
// @return 是否通过
func (s *Server) authorized(r *http.Request) bool {
	if s.bearerToken != "" {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.bearerToken)) == 1 {
			return true
		}
	}
	if user, password, ok := r.BasicAuth(); ok {
		if expected, exists := s.basicUsers[user]; exists && subtle.ConstantTimeCompare([]byte(password), []byte(expected)) == 1 {
			return true
		}
		if s.identityAuth && s.engine.IdentityService().CheckPassword(r.Context(), user, password) == nil {
			return true
		}
	}
	return false
}

// routes 注册全部路由。
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("GET /service/management/engine", s.handleEngineInfo)
	s.mux.HandleFunc("GET /service/management/properties", s.handleEngineProperties)
	s.mux.HandleFunc("GET /service/management/engine-properties", s.handleEngineProperties)
	s.mux.HandleFunc("GET /service/management/tables", s.handleListTables)
	s.mux.HandleFunc("GET /service/management/tables/{name}", s.handleGetTable)
	s.mux.HandleFunc("GET /service/management/tables/{name}/columns", s.handleTableColumns)
	s.mux.HandleFunc("GET /service/management/tables/{name}/data", s.handleTableData)
	s.mux.HandleFunc("GET /service/management/audit-logs", s.handleAuditLogs)
	s.mux.HandleFunc("POST /service/management/jobs/run", s.handleRunJobs)
	s.mux.HandleFunc("GET /service/management/jobs", s.handleListJobs)
	s.mux.HandleFunc("GET /service/management/deadletter-jobs", s.handleListDeadLetterJobs)
	s.mux.HandleFunc("GET /service/management/jobs/{id}", s.handleGetJob)
	s.mux.HandleFunc("GET /service/management/deadletter-jobs/{id}", s.handleGetDeadLetterJob)
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
	s.mux.HandleFunc("GET /service/repository/process-definitions/{id}/resourcedata", s.handleDefinitionResourceData)
	s.mux.HandleFunc("GET /service/repository/process-definitions/{id}/identitylinks", s.handleListDefinitionIdentityLinks)
	s.mux.HandleFunc("POST /service/repository/process-definitions/{id}/identitylinks", s.handleAddDefinitionIdentityLink)
	s.mux.HandleFunc("GET /service/repository/process-definitions/{id}/identitylinks/{family}/{identityId}", s.handleGetDefinitionIdentityLink)
	s.mux.HandleFunc("DELETE /service/repository/process-definitions/{id}/identitylinks/{family}/{identityId}", s.handleDeleteDefinitionIdentityLink)
	s.mux.HandleFunc("GET /service/repository/models", s.handleListModels)
	s.mux.HandleFunc("POST /service/repository/models", s.handleCreateModel)
	s.mux.HandleFunc("GET /service/repository/models/{id}", s.handleGetModel)
	s.mux.HandleFunc("PUT /service/repository/models/{id}", s.handleUpdateModel)
	s.mux.HandleFunc("DELETE /service/repository/models/{id}", s.handleDeleteModel)
	s.mux.HandleFunc("GET /service/repository/models/{id}/source", s.handleGetModelSource)
	s.mux.HandleFunc("PUT /service/repository/models/{id}/source", s.handleSetModelSource)
	s.mux.HandleFunc("GET /service/repository/models/{id}/source-extra", s.handleGetModelSourceExtra)
	s.mux.HandleFunc("PUT /service/repository/models/{id}/source-extra", s.handleSetModelSourceExtra)
	s.mux.HandleFunc("GET /service/repository/process-definitions/{id}/start-form", s.handleDefinitionStartForm)
	s.mux.HandleFunc("PUT /service/repository/process-definitions/{id}", s.handleDefinitionAction)
	s.mux.HandleFunc("GET /service/runtime/executions", s.handleListExecutions)
	s.mux.HandleFunc("GET /service/runtime/executions/{id}", s.handleGetExecution)
	s.mux.HandleFunc("GET /service/runtime/executions/{id}/variables", s.handleGetExecutionVariables)
	s.mux.HandleFunc("DELETE /service/runtime/executions/{id}/variables", s.handleDeleteAllExecutionVariables)
	s.mux.HandleFunc("GET /service/runtime/executions/{id}/variables/{name}", s.handleGetExecutionVariable)
	s.mux.HandleFunc("PUT /service/runtime/executions/{id}/variables/{name}", s.handleSetExecutionVariable)
	s.mux.HandleFunc("DELETE /service/runtime/executions/{id}/variables/{name}", s.handleDeleteExecutionVariable)
	s.mux.HandleFunc("GET /service/runtime/executions/{id}/variables/{name}/data", s.handleGetExecutionVariableData)
	s.mux.HandleFunc("POST /service/runtime/process-instances", s.handleStartInstance)
	s.mux.HandleFunc("GET /service/runtime/process-instances", s.handleListInstances)
	s.mux.HandleFunc("POST /service/runtime/process-instances/delete", s.handleBulkDeleteInstances)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}", s.handleGetInstance)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}", s.handleDeleteInstance)
	s.mux.HandleFunc("PUT /service/runtime/process-instances/{id}", s.handleInstanceAction)
	s.mux.HandleFunc("POST /service/runtime/process-instances/{id}/migrate", s.handleMigrate)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/attachments", s.handleListProcessAttachments)
	s.mux.HandleFunc("POST /service/runtime/process-instances/{id}/attachments", s.handleAddProcessAttachment)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}/attachments/{attachmentId}", s.handleDeleteAttachment)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}/comments/{commentId}", s.handleDeleteProcessComment)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/comments", s.handleListProcessComments)
	s.mux.HandleFunc("POST /service/runtime/process-instances/{id}/comments", s.handleAddProcessComment)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/identitylinks", s.handleListInstanceIdentityLinks)
	s.mux.HandleFunc("POST /service/runtime/process-instances/{id}/identitylinks", s.handleAddInstanceIdentityLink)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}/identitylinks/{family}/{identityId}/{type}", s.handleDeleteInstanceIdentityLink)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/tasks", s.handleGetInstanceTasks)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/variables", s.handleGetVariables)
	s.mux.HandleFunc("PUT /service/runtime/process-instances/{id}/variables", s.handleUpdateInstanceVariables)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}/variables", s.handleDeleteAllInstanceVariables)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/variables/{name}/data", s.handleGetInstanceVariableData)
	s.mux.HandleFunc("POST /service/runtime/process-instances/{id}/variables", s.handleCreateInstanceVariable)
	s.mux.HandleFunc("GET /service/runtime/process-instances/{id}/variables/{name}", s.handleGetInstanceVariable)
	s.mux.HandleFunc("PUT /service/runtime/process-instances/{id}/variables/{name}", s.handleUpdateInstanceVariable)
	s.mux.HandleFunc("DELETE /service/runtime/process-instances/{id}/variables/{name}", s.handleDeleteInstanceVariable)
	s.mux.HandleFunc("GET /service/form/form-data", s.handleGetFormData)
	s.mux.HandleFunc("POST /service/form/form-data", s.handleSubmitFormData)
	s.mux.HandleFunc("GET /service/identity/roles", s.handleListRoles)
	s.mux.HandleFunc("POST /service/identity/roles", s.handleCreateRole)
	s.mux.HandleFunc("GET /service/identity/roles/{id}", s.handleGetRole)
	s.mux.HandleFunc("PUT /service/identity/roles/{id}", s.handleUpdateRole)
	s.mux.HandleFunc("DELETE /service/identity/roles/{id}", s.handleDeleteRole)
	s.mux.HandleFunc("GET /service/identity/users", s.handleListUsers)
	s.mux.HandleFunc("POST /service/identity/users", s.handleCreateUser)
	s.mux.HandleFunc("GET /service/identity/users/{id}", s.handleGetUser)
	s.mux.HandleFunc("PUT /service/identity/users/{id}", s.handleUpdateUser)
	s.mux.HandleFunc("PUT /service/identity/users/{id}/roles", s.handleSetUserRoles)
	s.mux.HandleFunc("GET /service/identity/users/{id}/groups", s.handleListUserGroups)
	s.mux.HandleFunc("POST /service/identity/users/{id}/groups", s.handleAddUserGroup)
	s.mux.HandleFunc("DELETE /service/identity/users/{id}/groups/{groupId}", s.handleRemoveUserGroup)
	s.mux.HandleFunc("DELETE /service/identity/users/{id}", s.handleDeleteUser)
	s.mux.HandleFunc("PUT /service/identity/users/{id}/password", s.handleChangePassword)
	s.mux.HandleFunc("GET /service/identity/groups", s.handleListGroups)
	s.mux.HandleFunc("POST /service/identity/groups", s.handleCreateGroup)
	s.mux.HandleFunc("GET /service/identity/groups/{id}", s.handleGetGroup)
	s.mux.HandleFunc("DELETE /service/identity/groups/{id}", s.handleDeleteGroup)
	s.mux.HandleFunc("GET /service/identity/groups/{id}/members", s.handleListMembers)
	s.mux.HandleFunc("POST /service/identity/groups/{id}/members", s.handleAddMember)
	s.mux.HandleFunc("DELETE /service/identity/groups/{id}/members/{userId}", s.handleRemoveMember)
	s.mux.HandleFunc("POST /service/runtime/messages", s.handleMessage)
	s.mux.HandleFunc("POST /service/runtime/signals", s.handleSignal)
	s.mux.HandleFunc("GET /service/runtime/event-subscriptions", s.handleListEventSubscriptions)
	s.mux.HandleFunc("GET /service/runtime/event-subscriptions/{id}", s.handleGetEventSubscription)
	s.mux.HandleFunc("POST /service/query/process-instances", s.handleQueryProcessInstances)
	s.mux.HandleFunc("POST /service/query/executions", s.handleQueryExecutions)
	s.mux.HandleFunc("POST /service/query/tasks", s.handleQueryTasks)
	s.mux.HandleFunc("POST /service/query/historic-process-instances", s.handleQueryHistoricProcesses)
	s.mux.HandleFunc("POST /service/query/historic-task-instances", s.handleQueryHistoricTasks)
	s.mux.HandleFunc("POST /service/query/historic-activity-instances", s.handleQueryHistoricActivities)
	s.mux.HandleFunc("GET /service/runtime/tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /service/runtime/tasks", s.handleCreateTask)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("PUT /service/runtime/tasks/{id}", s.handleUpdateTask)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}", s.handleDeleteTask)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/subtasks", s.handleListSubTasks)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/form", s.handleGetTaskForm)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/identitylinks", s.handleListIdentityLinks)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}/identitylinks", s.handleAddIdentityLink)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}/identitylinks/{family}/{identityId}", s.handleDeleteIdentityLink)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/variables", s.handleGetTaskVariables)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}/variables", s.handleCreateTaskVariable)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/variables/{name}", s.handleGetTaskVariable)
	s.mux.HandleFunc("PUT /service/runtime/tasks/{id}/variables/{name}", s.handleUpdateTaskVariable)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}/variables/{name}", s.handleDeleteTaskVariable)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}/variables", s.handleDeleteAllTaskVariables)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/variables/{name}/data", s.handleGetTaskVariableData)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/comments", s.handleListComments)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}/comments", s.handleAddComment)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/comments/{commentId}", s.handleGetComment)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}/comments/{commentId}", s.handleDeleteComment)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/attachments", s.handleListAttachments)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}/attachments", s.handleAddAttachment)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/attachments/{attachmentId}", s.handleGetAttachment)
	s.mux.HandleFunc("DELETE /service/runtime/tasks/{id}/attachments/{attachmentId}", s.handleDeleteAttachment)
	s.mux.HandleFunc("GET /service/runtime/tasks/{id}/identitylinks/{family}", s.handleListIdentityLinksFamily)
	s.mux.HandleFunc("POST /service/runtime/tasks/{id}", s.handleTaskAction)
	s.mux.HandleFunc("GET /service/history/historic-process-instances", s.handleHistoricProcesses)
	s.mux.HandleFunc("POST /service/history/historic-process-instances/delete", s.handleBulkDeleteHistoricInstances)
	s.mux.HandleFunc("GET /service/history/historic-process-instances/{id}", s.handleHistoricProcess)
	s.mux.HandleFunc("GET /service/history/historic-process-instances/{id}/variables", s.handleHistoricProcessVariables)
	s.mux.HandleFunc("GET /service/history/historic-process-instances/{id}/variables/{name}/data", s.handleGetHistoricProcessVariableData)
	s.mux.HandleFunc("GET /service/history/historic-process-instances/{id}/identitylinks", s.handleHistoricProcessLinks)
	s.mux.HandleFunc("GET /service/history/historic-process-instances/{id}/comments", s.handleHistoricProcessComments)
	s.mux.HandleFunc("POST /service/history/historic-process-instances/{id}/comments", s.handleCreateHistoricProcessComment)
	s.mux.HandleFunc("DELETE /service/history/historic-process-instances/{id}/comments/{commentId}", s.handleDeleteHistoricProcessComment)
	s.mux.HandleFunc("DELETE /service/history/historic-process-instances/{id}", s.handleDeleteHistoricProcess)
	s.mux.HandleFunc("GET /service/history/historic-task-instances/{id}", s.handleHistoricTask)
	s.mux.HandleFunc("GET /service/history/historic-task-instances/{id}/identitylinks", s.handleHistoricTaskLinks)
	s.mux.HandleFunc("GET /service/history/historic-task-instances/{id}/variables/{name}/data", s.handleGetHistoricTaskVariableData)
	s.mux.HandleFunc("DELETE /service/history/historic-task-instances/{id}", s.handleDeleteHistoricTask)
	s.mux.HandleFunc("GET /service/history/historic-activity-instances", s.handleHistoricActivities)
	s.mux.HandleFunc("GET /service/history/historic-activity-instances/{id}", s.handleHistoricActivity)
	s.mux.HandleFunc("GET /service/history/historic-task-instances", s.handleHistoricTasks)
	s.mux.HandleFunc("GET /service/history/historic-task-log-entries", s.handleHistoricTaskLogs)
	s.mux.HandleFunc("GET /service/history/historic-detail", s.handleHistoricDetails)
	s.mux.HandleFunc("GET /service/history/historic-detail/{id}/data", s.handleGetHistoricDetailData)
	s.mux.HandleFunc("DELETE /service/history/historic-detail/{id}", s.handleDeleteHistoricDetail)
	s.mux.HandleFunc("DELETE /service/history/historic-task-log-entries/{id}", s.handleDeleteHistoricTaskLog)
	s.mux.HandleFunc("GET /service/history/historic-identitylinks", s.handleHistoricIdentityLinks)
	s.mux.HandleFunc("GET /service/history/historic-variable-instances", s.handleHistoricVariables)
	s.mux.HandleFunc("GET /service/history/historic-variable-instances/{id}", s.handleHistoricVariable)
	s.mux.HandleFunc("GET /service/history/historic-variable-instances/{id}/data", s.handleGetHistoricVariableData)
	s.mux.HandleFunc("DELETE /service/history/historic-variable-instances/{id}", s.handleDeleteHistoricVariable)
}

// handleEngineProperties 返回引擎属性。
// @param w 响应
// @param r 请求
func (s *Server) handleEngineProperties(w http.ResponseWriter, r *http.Request) {
	properties, err := s.engine.ManagementService().Properties(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, properties)
}

// handleListTables 返回 ACT_* 表及行数。
// @param w 响应
// @param r 请求
func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	counts, err := s.engine.ManagementService().TableCount(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	tables := make([]api.TableInfo, 0, len(names))
	for _, name := range names {
		tables = append(tables, api.TableInfo{
			Name:  name,
			Count: counts[name],
			URL:   "/service/management/tables/" + name,
		})
	}
	writeJSON(w, http.StatusOK, tables)
}

// handleGetTable 返回单张表的行数。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTable(w http.ResponseWriter, r *http.Request) {
	name := strings.ToUpper(r.PathValue("name"))
	counts, err := s.engine.ManagementService().TableCount(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	count, ok := counts[name]
	if !ok {
		writeError(w, api.NewError("ENGINE-0918", "表不存在: "+name, api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, api.TableInfo{Name: name, Count: count, URL: "/service/management/tables/" + name})
}

// handleTableColumns 返回表列元数据。
// @param w 响应
// @param r 请求
func (s *Server) handleTableColumns(w http.ResponseWriter, r *http.Request) {
	meta, err := s.engine.ManagementService().GetTableMetaData(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

// handleTableData 分页返回表数据。
// @param w 响应
// @param r 请求
func (s *Server) handleTableData(w http.ResponseWriter, r *http.Request) {
	ascending := r.URL.Query().Get("orderAscendingColumn")
	descending := r.URL.Query().Get("orderDescendingColumn")
	if ascending != "" && descending != "" {
		writeError(w, api.NewError("REST-0405", "只能指定一个排序方向", api.ErrBadRequest))
		return
	}
	sortColumn, order := ascending, "asc"
	if descending != "" {
		sortColumn, order = descending, "desc"
	}
	start, size := pageParams(r)
	page, err := s.engine.ManagementService().QueryTableData(r.Context(), r.PathValue("name"), start, size, sortColumn, order)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleGetTaskForm 返回任务表单。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTaskForm(w http.ResponseWriter, r *http.Request) {
	data, err := s.engine.FormService().GetTaskFormData(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if data.FormKey == "" {
		writeError(w, api.NewError("REST-0406", "任务未定义表单", api.ErrBadRequest))
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// historicProcessForRequest 查询历史流程实例并校验租户。
// @param r 请求
// @return 历史实例
// @return 查询错误
func (s *Server) historicProcessForRequest(r *http.Request) (api.HistoricProcessInstance, error) {
	instance, err := s.engine.HistoryService().GetHistoricProcessInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		return api.HistoricProcessInstance{}, err
	}
	if tenant := requestTenant(r); tenant != "" && instance.TenantID != tenant {
		return api.HistoricProcessInstance{}, api.NewError("ENGINE-0919", "历史流程实例不存在", api.ErrNotFound)
	}
	return instance, nil
}

// handleHistoricProcessVariables 返回历史流程实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricProcessVariables(w http.ResponseWriter, r *http.Request) {
	if _, err := s.historicProcessForRequest(r); err != nil {
		writeError(w, err)
		return
	}
	variables, err := s.engine.HistoryService().ListHistoricVariableInstances(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, variables, start, size)
}

// handleHistoricProcessLinks 返回历史流程实例身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricProcessLinks(w http.ResponseWriter, r *http.Request) {
	if _, err := s.historicProcessForRequest(r); err != nil {
		writeError(w, err)
		return
	}
	links, err := s.engine.HistoryService().ListHistoricProcessIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, links, start, size)
}

// handleHistoricProcessComments 返回历史流程实例评论。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricProcessComments(w http.ResponseWriter, r *http.Request) {
	if _, err := s.historicProcessForRequest(r); err != nil {
		writeError(w, err)
		return
	}
	comments, err := s.engine.TaskService().ListInstanceComments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, comments, start, size)
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

// handleGetJob 返回可执行作业。
// @param w 响应
// @param r 请求
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// handleGetDeadLetterJob 返回死信作业。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDeadLetterJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.engine.ManagementService().GetDeadLetterJob(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
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
	switch payload.Action {
	case "moveToExecutable":
		if err := s.engine.ManagementService().MoveDeadLetterJobToExecutable(r.Context(), r.PathValue("id"), payload.Retries); err != nil {
			writeError(w, err)
			return
		}
	case "setRetries":
		if err := s.engine.ManagementService().SetJobRetries(r.Context(), r.PathValue("id"), payload.Retries); err != nil {
			writeError(w, err)
			return
		}
	default:
		writeError(w, api.NewError("REST-0112", "不支持的作业动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAuditLogs 返回审计日志。
// @param w 响应
// @param r 请求
func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	start, size := pageParams(r)
	if s.auditLog == nil {
		writePage(w, []api.AuditEvent{}, start, size)
		return
	}
	writePage(w, s.auditLog.list(), start, size)
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
	if request.TenantID == "" {
		request.TenantID = requestTenant(r)
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
	deployments = filterDeployments(deployments, requestTenant(r))
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
	definitions = filterDefinitions(definitions, requestTenant(r))
	if key := r.URL.Query().Get("key"); key != "" {
		filtered := make([]api.ProcessDefinition, 0, len(definitions))
		for _, definition := range definitions {
			if definition.Key == key {
				filtered = append(filtered, definition)
			}
		}
		definitions = filtered
	}
	if version := r.URL.Query().Get("version"); version != "" {
		if parsed, parseErr := strconv.Atoi(version); parseErr == nil {
			filtered := make([]api.ProcessDefinition, 0, len(definitions))
			for _, definition := range definitions {
				if definition.Version == parsed {
					filtered = append(filtered, definition)
				}
			}
			definitions = filtered
		}
	}
	start, size := pageParams(r)
	writePage(w, definitions, start, size)
}

// handleDefinitionStartForm 返回流程定义启动表单。
// @param w 响应
// @param r 请求
func (s *Server) handleDefinitionStartForm(w http.ResponseWriter, r *http.Request) {
	data, err := s.engine.FormService().GetStartFormData(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
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

// handleGetExecutionVariables 返回执行变量。
// @param w 响应
// @param r 请求
func (s *Server) handleGetExecutionVariables(w http.ResponseWriter, r *http.Request) {
	values, err := s.engine.RuntimeService().GetExecutionVariables(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, variableResponse(values))
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
		TenantID:             firstNonEmpty(payload.TenantID, requestTenant(r)),
		UserID:               requestUserID(r),
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

// handleListRoles 返回角色列表。
// @param w 响应
// @param r 请求
func (s *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.engine.IdentityService().ListRoles(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, roles, start, size)
}

// handleCreateRole 创建角色。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var role api.Role
	if err := decodeJSON(r, &role); err != nil {
		writeError(w, err)
		return
	}
	created, err := s.engine.IdentityService().CreateRole(r.Context(), role)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleGetRole 查询角色。
// @param w 响应
// @param r 请求
func (s *Server) handleGetRole(w http.ResponseWriter, r *http.Request) {
	role, err := s.engine.IdentityService().GetRole(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// handleUpdateRole 更新角色。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	var role api.Role
	if err := decodeJSON(r, &role); err != nil {
		writeError(w, err)
		return
	}
	role.ID = r.PathValue("id")
	updated, err := s.engine.IdentityService().UpdateRole(r.Context(), role)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteRole 删除角色。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().DeleteRole(r.Context(), r.PathValue("id")); err != nil {
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

// handleUpdateUser 更新用户。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var user api.User
	if err := decodeJSON(r, &user); err != nil {
		writeError(w, err)
		return
	}
	user.ID = r.PathValue("id")
	updated, err := s.engine.IdentityService().UpdateUser(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// rolesPayload 是角色设置请求体。
type rolesPayload struct {
	Roles []string `json:"roles"` // 角色
}

// handleListUserGroups 返回用户所属组。
// @param w 响应
// @param r 请求
func (s *Server) handleListUserGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.engine.IdentityService().ListUserGroups(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

// userGroupPayload 是用户组成员请求体。
type userGroupPayload struct {
	GroupID string `json:"groupId"` // 组 ID
}

// handleAddUserGroup 添加用户到组。
// @param w 响应
// @param r 请求
func (s *Server) handleAddUserGroup(w http.ResponseWriter, r *http.Request) {
	var payload userGroupPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.IdentityService().AddMembership(r.Context(), r.PathValue("id"), payload.GroupID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveUserGroup 从组移除用户。
// @param w 响应
// @param r 请求
func (s *Server) handleRemoveUserGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.IdentityService().RemoveMembership(r.Context(), r.PathValue("id"), r.PathValue("groupId")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetUserRoles 设置用户角色。
// @param w 响应
// @param r 请求
func (s *Server) handleSetUserRoles(w http.ResponseWriter, r *http.Request) {
	var payload rolesPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.IdentityService().SetUserRoles(r.Context(), r.PathValue("id"), payload.Roles); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// passwordPayload 是密码修改请求体。
type passwordPayload struct {
	OldPassword string `json:"oldPassword"` // 旧密码
	NewPassword string `json:"newPassword"` // 新密码
}

// handleChangePassword 修改或重置密码。
// @param w 响应
// @param r 请求
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var payload passwordPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	userID := r.PathValue("id")
	if payload.OldPassword != "" {
		if err := s.engine.IdentityService().ChangePassword(r.Context(), userID, payload.OldPassword, payload.NewPassword); err != nil {
			writeError(w, err)
			return
		}
	} else if err := s.engine.IdentityService().UpdatePassword(r.Context(), userID, payload.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	instances = filterInstances(instances, requestTenant(r))
	if key := r.URL.Query().Get("processDefinitionKey"); key != "" {
		filtered := make([]api.ProcessInstance, 0, len(instances))
		for _, instance := range instances {
			if instance.ProcessDefinitionKey == key {
				filtered = append(filtered, instance)
			}
		}
		instances = filtered
	}
	if businessKey := r.URL.Query().Get("businessKey"); businessKey != "" {
		filtered := make([]api.ProcessInstance, 0, len(instances))
		for _, instance := range instances {
			if instance.BusinessKey == businessKey {
				filtered = append(filtered, instance)
			}
		}
		instances = filtered
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

// handleCreateInstanceVariable 创建实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateInstanceVariable(w http.ResponseWriter, r *http.Request) {
	var variable api.Variable
	if err := decodeJSON(r, &variable); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.RuntimeService().SetVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, variable)
}

// handleGetInstanceVariable 返回单个实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleGetInstanceVariable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	value, err := s.engine.RuntimeService().GetVariable(r.Context(), r.PathValue("id"), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Variable{Name: name, Value: value})
}

// handleUpdateInstanceVariable 更新实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateInstanceVariable(w http.ResponseWriter, r *http.Request) {
	var variable api.Variable
	if err := decodeJSON(r, &variable); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.RuntimeService().SetVariable(r.Context(), r.PathValue("id"), r.PathValue("name"), variable.Value); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteInstanceVariable 删除实例变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteInstanceVariable(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().DeleteVariable(r.Context(), r.PathValue("id"), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListProcessAttachments 返回流程实例附件。
// @param w 响应
// @param r 请求
func (s *Server) handleListProcessAttachments(w http.ResponseWriter, r *http.Request) {
	attachments, err := s.engine.TaskService().ListProcessAttachments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, attachments, start, size)
}

// handleAddProcessAttachment 添加流程实例附件。
// @param w 响应
// @param r 请求
func (s *Server) handleAddProcessAttachment(w http.ResponseWriter, r *http.Request) {
	var attachment api.Attachment
	if err := decodeJSON(r, &attachment); err != nil {
		writeError(w, err)
		return
	}
	created, err := s.engine.TaskService().AddProcessAttachment(r.Context(), r.PathValue("id"), attachment)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleListProcessComments 返回流程实例评论。
// @param w 响应
// @param r 请求
func (s *Server) handleListProcessComments(w http.ResponseWriter, r *http.Request) {
	comments, err := s.engine.TaskService().ListProcessComments(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, comments, start, size)
}

// handleAddProcessComment 添加流程实例评论。
// @param w 响应
// @param r 请求
func (s *Server) handleAddProcessComment(w http.ResponseWriter, r *http.Request) {
	var payload commentPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	comment, err := s.engine.TaskService().AddProcessComment(r.Context(), r.PathValue("id"), payload.UserID, payload.Message)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, comment)
}

// handleGetInstanceTasks 返回实例任务。
// @param w 响应
// @param r 请求
func (s *Server) handleGetInstanceTasks(w http.ResponseWriter, r *http.Request) {
	start, size := pageParams(r)
	page, err := s.engine.TaskService().ListTasks(r.Context(), api.TaskFilter{
		ProcessInstanceID: r.PathValue("id"),
		TenantID:          requestTenant(r),
		Start:             start,
		Size:              size,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
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
		TenantID:             requestTenant(r),
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

// handleListIdentityLinks 返回任务参与者。
// @param w 响应
// @param r 请求
func (s *Server) handleListIdentityLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.TaskService().ListIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// identityLinkPayload 是身份链接请求体。
type identityLinkPayload struct {
	Type    string `json:"type"`    // 类型
	UserID  string `json:"userId"`  // 用户 ID
	GroupID string `json:"groupId"` // 组 ID
}

// handleAddIdentityLink 添加候选用户/组。
// @param w 响应
// @param r 请求
func (s *Server) handleAddIdentityLink(w http.ResponseWriter, r *http.Request) {
	var payload identityLinkPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	if payload.Type != "candidate" {
		writeError(w, api.NewError("REST-0115", "当前仅支持 candidate 类型", api.ErrBadRequest))
		return
	}
	taskID := r.PathValue("id")
	var err error
	if payload.UserID != "" {
		err = s.engine.TaskService().AddCandidateUser(r.Context(), taskID, payload.UserID)
	} else if payload.GroupID != "" {
		err = s.engine.TaskService().AddCandidateGroup(r.Context(), taskID, payload.GroupID)
	} else {
		writeError(w, api.NewError("REST-0116", "缺少 userId 或 groupId", api.ErrBadRequest))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleDeleteIdentityLink 移除候选用户/组。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteIdentityLink(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	family := r.PathValue("family")
	identityID := r.PathValue("identityId")
	var err error
	switch family {
	case "users":
		err = s.engine.TaskService().DeleteCandidateUser(r.Context(), taskID, identityID)
	case "groups":
		err = s.engine.TaskService().DeleteCandidateGroup(r.Context(), taskID, identityID)
	default:
		writeError(w, api.NewError("REST-0117", "不支持的身份链接族: "+family, api.ErrBadRequest))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetTaskVariables 返回任务变量。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTaskVariables(w http.ResponseWriter, r *http.Request) {
	values, err := s.engine.TaskService().GetTaskVariables(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, variableResponse(values))
}

// handleCreateTaskVariable 创建任务变量。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateTaskVariable(w http.ResponseWriter, r *http.Request) {
	var variable api.Variable
	if err := decodeJSON(r, &variable); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.TaskService().SetTaskVariable(r.Context(), r.PathValue("id"), variable.Name, variable.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, variable)
}

// handleGetTaskVariable 返回单个任务变量。
// @param w 响应
// @param r 请求
func (s *Server) handleGetTaskVariable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	value, err := s.engine.TaskService().GetTaskVariable(r.Context(), r.PathValue("id"), name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Variable{Name: name, Value: value})
}

// handleUpdateTaskVariable 更新任务变量。
// @param w 响应
// @param r 请求
func (s *Server) handleUpdateTaskVariable(w http.ResponseWriter, r *http.Request) {
	var variable api.Variable
	if err := decodeJSON(r, &variable); err != nil {
		writeError(w, err)
		return
	}
	if err := s.engine.TaskService().SetTaskVariable(r.Context(), r.PathValue("id"), r.PathValue("name"), variable.Value); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteTaskVariable 删除任务变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteTaskVariable(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.TaskService().DeleteTaskVariable(r.Context(), r.PathValue("id"), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// handleDeleteAttachment 删除附件。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.TaskService().DeleteAttachment(r.Context(), r.PathValue("attachmentId")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteProcessComment 删除流程评论。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteProcessComment(w http.ResponseWriter, r *http.Request) {
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
	if key := r.URL.Query().Get("processDefinitionKey"); key != "" {
		filtered := make([]api.HistoricProcessInstance, 0, len(items))
		for _, item := range items {
			if item.ProcessDefinitionKey == key {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if businessKey := r.URL.Query().Get("businessKey"); businessKey != "" {
		filtered := make([]api.HistoricProcessInstance, 0, len(items))
		for _, item := range items {
			if item.BusinessKey == businessKey {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if tenant := requestTenant(r); tenant != "" {
		filtered := make([]api.HistoricProcessInstance, 0, len(items))
		for _, item := range items {
			if item.TenantID == tenant {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if ended := r.URL.Query().Get("ended"); ended != "" {
		want := ended == "true"
		filtered := make([]api.HistoricProcessInstance, 0, len(items))
		for _, item := range items {
			if item.Ended == want {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	start, size := pageParams(r)
	writePage(w, items, start, size)
}

// handleHistoricActivity 返回历史活动详情。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricActivity(w http.ResponseWriter, r *http.Request) {
	item, err := s.engine.HistoryService().GetHistoricActivityInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleHistoricVariable 返回历史变量详情。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricVariable(w http.ResponseWriter, r *http.Request) {
	item, err := s.engine.HistoryService().GetHistoricVariableInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// handleDeleteHistoricVariable 删除历史变量。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteHistoricVariable(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.HistoryService().DeleteHistoricVariableInstance(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// handleHistoricTaskLinks 返回历史任务身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricTaskLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.HistoryService().ListHistoricIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, links, start, size)
}

// handleDeleteHistoricProcess 删除历史流程实例。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteHistoricProcess(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.HistoryService().DeleteHistoricProcessInstance(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteHistoricTask 删除历史任务。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteHistoricTask(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.HistoryService().DeleteHistoricTaskInstance(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteHistoricDetail 删除历史明细。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteHistoricDetail(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.HistoryService().DeleteHistoricDetail(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteHistoricTaskLog 删除任务日志。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteHistoricTaskLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, api.NewError("REST-0118", "非法日志 ID", api.ErrBadRequest))
		return
	}
	if err := s.engine.HistoryService().DeleteHistoricTaskLog(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// handleHistoricTaskLogs 返回任务生命周期日志。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricTaskLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.engine.HistoryService().ListHistoricTaskLogs(r.Context(), r.URL.Query().Get("taskId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, logs, start, size)
}

// handleHistoricDetails 返回历史变量明细。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricDetails(w http.ResponseWriter, r *http.Request) {
	details, err := s.engine.HistoryService().ListHistoricDetails(r.Context(), r.URL.Query().Get("processInstanceId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, details, start, size)
}

// handleHistoricIdentityLinks 返回历史身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleHistoricIdentityLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.HistoryService().ListHistoricIdentityLinks(r.Context(), r.URL.Query().Get("taskId"))
	if err != nil {
		writeError(w, err)
		return
	}
	start, size := pageParams(r)
	writePage(w, links, start, size)
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
	case []api.AuditEvent:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.Role:
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
	case []api.IdentityLink:
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
	case []api.HistoricTaskLog:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = item
		}
		return out
	case []api.HistoricDetail:
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

// requestTenant 返回请求租户。
// @param r 请求
// @return 租户
func requestTenant(r *http.Request) string {
	if tenant := r.Header.Get("X-Tenant-Id"); tenant != "" {
		return tenant
	}
	return r.URL.Query().Get("tenantId")
}

// firstNonEmpty 返回第一个非空字符串。
// @param values 候选值
// @return 非空值
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// filterDeployments 按租户过滤部署。
// @param items 部署
// @param tenant 租户
// @return 过滤结果
func filterDeployments(items []api.Deployment, tenant string) []api.Deployment {
	if tenant == "" {
		return items
	}
	out := make([]api.Deployment, 0, len(items))
	for _, item := range items {
		if item.TenantID == tenant {
			out = append(out, item)
		}
	}
	return out
}

// filterDefinitions 按租户过滤流程定义。
// @param items 定义
// @param tenant 租户
// @return 过滤结果
func filterDefinitions(items []api.ProcessDefinition, tenant string) []api.ProcessDefinition {
	if tenant == "" {
		return items
	}
	out := make([]api.ProcessDefinition, 0, len(items))
	for _, item := range items {
		if item.TenantID == tenant {
			out = append(out, item)
		}
	}
	return out
}

// filterInstances 按租户过滤流程实例。
// @param items 实例
// @param tenant 租户
// @return 过滤结果
func filterInstances(items []api.ProcessInstance, tenant string) []api.ProcessInstance {
	if tenant == "" {
		return items
	}
	out := make([]api.ProcessInstance, 0, len(items))
	for _, item := range items {
		if item.TenantID == tenant {
			out = append(out, item)
		}
	}
	return out
}

// statusFor 将错误映射为 HTTP 状态码。
// @param err 错误
// @return HTTP 状态码
func statusFor(err error) int {
	if errors.Is(err, api.ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, api.ErrForbidden) {
		return http.StatusForbidden
	}
	if errors.Is(err, api.ErrTooManyRequests) {
		return http.StatusTooManyRequests
	}
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
	case errors.Is(err, api.ErrUnauthorized):
		return http.StatusUnauthorized
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
