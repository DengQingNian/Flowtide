package rest

import (
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// deploymentResourceResponse 是官方 DeploymentResourceResponse 的响应结构。
type deploymentResourceResponse struct {
	ID         string `json:"id"`         // 资源名
	URL        string `json:"url"`        // 资源元数据地址
	ContentURL string `json:"contentUrl"` // 资源原始内容地址
	MediaType  string `json:"mediaType"`  // 资源媒体类型
	Type       string `json:"type"`       // 资源类型
}

// restIdentityLink 是官方 RestIdentityLink 的响应结构。
type restIdentityLink struct {
	URL   string `json:"url"`             // 链接地址
	User  string `json:"user,omitempty"`  // 用户
	Group string `json:"group,omitempty"` // 组
	Type  string `json:"type"`            // 链接类型
}

// handleDeleteBatch 删除批次及其分片。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.ManagementService().DeleteBatch(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetBatchDocument 返回批次文档。
// @param w 响应
// @param r 请求
func (s *Server) handleGetBatchDocument(w http.ResponseWriter, r *http.Request) {
	document, err := s.engine.ManagementService().GetBatchDocument(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeRawJSON(w, document)
}

// handleGetBatchPartDocument 返回批次分片文档。
// @param w 响应
// @param r 请求
func (s *Server) handleGetBatchPartDocument(w http.ResponseWriter, r *http.Request) {
	document, err := s.engine.ManagementService().GetBatchPartDocument(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeRawJSON(w, document)
}

// writeRawJSON 输出已经序列化的 JSON 文档。
// @param w 响应
// @param document 文档字节
func writeRawJSON(w http.ResponseWriter, document []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

// handleDeleteDeadLetterJob 删除死信作业。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteDeadLetterJob(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.ManagementService().DeleteDeadLetterJob(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteSuspendedJob 删除挂起作业。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteSuspendedJob(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.ManagementService().DeleteSuspendedJob(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteTimerJob 删除定时器作业。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteTimerJob(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.ManagementService().DeleteTimerJob(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// timerJobActionPayload 是定时器作业动作请求体。
type timerJobActionPayload struct {
	Action  string `json:"action"`  // 动作
	DueDate string `json:"dueDate"` // 新的到期时间，reschedule 使用
	Date    string `json:"date"`    // 到期时间兼容别名
}

// handleTimerJobAction 执行定时器作业动作。
// 支持 move 与 reschedule，其他动作返回 400。
// @param w 响应
// @param r 请求
func (s *Server) handleTimerJobAction(w http.ResponseWriter, r *http.Request) {
	var payload timerJobActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	var err error
	switch payload.Action {
	case "move":
		err = s.engine.ManagementService().MoveTimerJobToExecutable(r.Context(), r.PathValue("id"))
	case "reschedule":
		dueDate := firstNonEmpty(payload.DueDate, payload.Date)
		// Flowable 要求 reschedule 必须提供有效到期时间
		if dueDate == "" {
			writeError(w, api.NewError("REST-0113", "reschedule 动作必须提供 dueDate", api.ErrBadRequest))
			return
		}
		err = s.engine.ManagementService().RescheduleTimerJob(r.Context(), r.PathValue("id"), dueDate)
	default:
		writeError(w, api.NewError("REST-0112", "不支持的定时器作业动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeadLetterJobAction 执行死信作业动作。
// 支持 move；moveToHistoryJob 因缺少历史作业模型返回 400。
// @param w 响应
// @param r 请求
func (s *Server) handleDeadLetterJobAction(w http.ResponseWriter, r *http.Request) {
	var payload jobActionPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, err)
		return
	}
	switch payload.Action {
	case "move":
		if err := s.engine.ManagementService().MoveDeadLetterJobToExecutable(r.Context(), r.PathValue("id"), payload.Retries); err != nil {
			writeError(w, err)
			return
		}
	case "moveToHistoryJob":
		writeError(w, api.NewError("REST-0114", "moveToHistoryJob 暂不支持：引擎没有历史作业模型", api.ErrBadRequest))
		return
	default:
		writeError(w, api.NewError("REST-0112", "不支持的死信作业动作: "+payload.Action, api.ErrBadRequest))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetDeploymentResourceData 返回部署资源的原始字节。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDeploymentResourceData(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("resourceName")
	data, err := s.engine.RepositoryService().GetDeploymentResource(r.Context(), r.PathValue("id"), name)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", deploymentResourceMediaType(name))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+strings.ReplaceAll(path.Base(name), "\"", "")+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// deploymentResourceMediaType 按 Flowable DefaultContentTypeResolver 的扩展名映射返回媒体类型。
// @param name 资源名
// @return 媒体类型
func deploymentResourceMediaType(name string) string {
	lower := strings.ToLower(name)
	index := strings.LastIndexByte(lower, '.')
	if index < 0 || index == len(lower)-1 {
		return "application/octet-stream"
	}
	switch lower[index+1:] {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "svg":
		return "image/svg+xml"
	case "txt":
		return "text/plain"
	case "xml", "bpmn", "cmmn", "dmn":
		return "text/xml"
	case "app", "event", "form", "channel", "json":
		return "application/json"
	case "zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

// deploymentResourceType 返回资源类型。
// @param name 资源名
// @return processDefinition、processImage 或 resource
func deploymentResourceType(name string) string {
	lower := strings.ToLower(name)
	// 与 Flowable ResourceNameUtil 一致，BPMN 资源标记为流程定义
	if strings.HasSuffix(lower, ".bpmn20.xml") || strings.HasSuffix(lower, ".bpmn") {
		return "processDefinition"
	}
	// 图形资源对应官方枚举 processImage
	for _, suffix := range []string{".png", ".jpg", ".jpeg", ".gif", ".svg"} {
		if strings.HasSuffix(lower, suffix) {
			return "processImage"
		}
	}
	return "resource"
}

// requestBaseURL 返回请求的协议与主机前缀。
// @param r 请求
// @return 基础地址
func requestBaseURL(r *http.Request) string {
	scheme := "http"
	// 反向代理场景优先使用转发协议
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// deploymentResourceResponseOf 构造单个资源元数据。
// @param r 请求
// @param deploymentID 部署 ID
// @param name 资源名
// @return 资源元数据
func deploymentResourceResponseOf(r *http.Request, deploymentID, name string) deploymentResourceResponse {
	base := requestBaseURL(r) + "/service/repository/deployments/" + url.PathEscape(deploymentID)
	encoded := url.PathEscape(name)
	return deploymentResourceResponse{
		ID:         name,
		URL:        base + "/resources/" + encoded,
		ContentURL: base + "/resourcedata/" + encoded,
		MediaType:  deploymentResourceMediaType(name),
		Type:       deploymentResourceType(name),
	}
}

// deploymentResources 返回部署的资源元数据列表。
// @param r 请求
// @param deploymentID 部署 ID
// @return 资源元数据
// @return 查询错误
func (s *Server) deploymentResources(r *http.Request, deploymentID string) ([]deploymentResourceResponse, error) {
	names, err := s.engine.RepositoryService().ListDeploymentResources(r.Context(), deploymentID)
	if err != nil {
		return nil, err
	}
	out := make([]deploymentResourceResponse, 0, len(names))
	for _, name := range names {
		out = append(out, deploymentResourceResponseOf(r, deploymentID, name))
	}
	return out, nil
}

// handleGetDeploymentResourceMetadata 返回单个部署资源的元数据。
// @param w 响应
// @param r 请求
func (s *Server) handleGetDeploymentResourceMetadata(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// 先确认部署存在，再确认资源名精确匹配
	resources, err := s.deploymentResources(r, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	for _, resource := range resources {
		if resource.ID == name {
			writeJSON(w, http.StatusOK, resource)
			return
		}
	}
	writeError(w, api.NewError("ENGINE-0404", "部署资源不存在: "+name, api.ErrNotFound))
}

// handleGetInstanceUserIdentityLink 返回流程实例的 RestIdentityLink。
// @param w 响应
// @param r 请求
func (s *Server) handleGetInstanceUserIdentityLink(w http.ResponseWriter, r *http.Request) {
	links, err := s.engine.RuntimeService().ListInstanceIdentityLinks(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	link, ok := findIdentityLink(links, "users", r.PathValue("identityId"), r.PathValue("type"))
	if !ok {
		writeError(w, api.NewError("ENGINE-0404", "身份链接不存在", api.ErrNotFound))
		return
	}
	writeJSON(w, http.StatusOK, restIdentityLink{
		URL:  instanceIdentityLinkURL(r, r.PathValue("id"), "users", link.UserID, link.Type),
		User: link.UserID,
		Type: link.Type,
	})
}

// handleDeleteInstanceUserIdentityLink 删除流程实例的用户身份链接。
// @param w 响应
// @param r 请求
func (s *Server) handleDeleteInstanceUserIdentityLink(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.RuntimeService().DeleteInstanceIdentityLink(
		r.Context(), r.PathValue("id"), "users", r.PathValue("identityId"), r.PathValue("type"),
	); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// instanceIdentityLinkURL 构造身份链接的绝对地址。
// @param r 请求
// @param instanceID 实例 ID
// @param family 族
// @param identityID 身份 ID
// @param linkType 链接类型
// @return 绝对地址
func instanceIdentityLinkURL(r *http.Request, instanceID, family, identityID, linkType string) string {
	return requestBaseURL(r) + "/service/runtime/process-instances/" + url.PathEscape(instanceID) +
		"/identitylinks/" + family + "/" + url.PathEscape(identityID) + "/" + url.PathEscape(linkType)
}
