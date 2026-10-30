package rest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// handleDeployModel 将流程模型的源码部署为流程定义。
// 资源名优先使用 {modelKey}.bpmn20.xml，模型 Key 为空时使用 model-{id}.bpmn20.xml。
// @param w 响应
// @param r 请求
func (s *Server) handleDeployModel(w http.ResponseWriter, r *http.Request) {
	repository := s.engine.RepositoryService()
	model, err := repository.GetModel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	source, err := repository.GetModelSource(r.Context(), model.ID)
	if err != nil {
		// 模型存在但没有源码属于请求内容缺失，转换为 400 而不是 404
		if errors.Is(err, api.ErrNotFound) {
			writeError(w, api.NewError("REST-0150", "模型没有可部署的源码", api.ErrBadRequest))
			return
		}
		writeError(w, err)
		return
	}
	// 空源码或纯空白源码无法生成流程定义，直接拒绝
	if len(bytes.TrimSpace(source)) == 0 {
		writeError(w, api.NewError("REST-0151", "模型源码为空，无法部署", api.ErrBadRequest))
		return
	}
	deployment, err := repository.Deploy(r.Context(), api.DeploymentRequest{
		Name:      model.Name,
		TenantID:  model.TenantID,
		Resources: map[string][]byte{modelDeploymentResourceName(model): source},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deployment)
}

// modelDeploymentResourceName 返回模型部署使用的资源名。
// @param model 流程模型
// @return 资源名
func modelDeploymentResourceName(model api.Model) string {
	key := strings.TrimSpace(model.Key)
	if key == "" {
		return fmt.Sprintf("model-%s.bpmn20.xml", model.ID)
	}
	return key + ".bpmn20.xml"
}

// handleCreateDeployment 创建部署，并按 enableDuplicateFiltering 决定是否复用已有部署。
// 该入口保留与现有 POST /service/repository/deployments 一致的行为，仅在显式开启时去重。
// @param w 响应
// @param r 请求
func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	s.createDeployment(w, r, duplicateFilteringEnabled(r))
}

// createDeployment 解析部署请求并创建部署。
// @param w 响应
// @param r 请求
// @param duplicateFiltering 是否启用重复过滤
func (s *Server) createDeployment(w http.ResponseWriter, r *http.Request, duplicateFiltering bool) {
	request, err := parseDeploymentRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if request.TenantID == "" {
		request.TenantID = requestTenant(r)
	}
	if duplicateFiltering {
		existing, found, findErr := s.findDuplicateDeployment(r.Context(), request)
		if findErr != nil {
			writeError(w, findErr)
			return
		}
		if found {
			writeJSON(w, http.StatusOK, existing)
			return
		}
	}
	deployment, err := s.engine.RepositoryService().Deploy(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, deployment)
}

// duplicateFilteringEnabled 判断是否启用部署重复过滤。
// @param r 请求
// @return 是否启用
func duplicateFilteringEnabled(r *http.Request) bool {
	value := strings.TrimSpace(r.URL.Query().Get("enableDuplicateFiltering"))
	return strings.EqualFold(value, "true")
}

// findDuplicateDeployment 查找同租户且资源名、资源字节完全一致的已有部署。
// 多个匹配时返回最后一个，即部署时间最新的一个。
// @param ctx 上下文
// @param request 部署请求
// @return 匹配的部署
// @return 是否存在匹配
// @return 查询错误
func (s *Server) findDuplicateDeployment(ctx context.Context, request api.DeploymentRequest) (api.Deployment, bool, error) {
	deployments, err := s.engine.RepositoryService().ListDeployments(ctx)
	if err != nil {
		return api.Deployment{}, false, err
	}
	var matched api.Deployment
	found := false
	for _, deployment := range deployments {
		// 只比较同租户部署，避免跨租户复用资源
		if deployment.TenantID != request.TenantID {
			continue
		}
		same, err := s.deploymentResourcesEqual(ctx, deployment.ID, request.Resources)
		if err != nil {
			return api.Deployment{}, false, err
		}
		if same {
			matched = deployment
			found = true
		}
	}
	return matched, found, nil
}

// deploymentResourcesEqual 比较部署资源与请求资源是否完全一致。
// 资源名集合和每个资源的内容都必须相同。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @param resources 请求资源
// @return 是否一致
// @return 查询错误
func (s *Server) deploymentResourcesEqual(ctx context.Context, deploymentID string, resources map[string][]byte) (bool, error) {
	repository := s.engine.RepositoryService()
	names, err := repository.ListDeploymentResources(ctx, deploymentID)
	if err != nil {
		return false, err
	}
	if len(names) != len(resources) {
		return false, nil
	}
	for _, name := range names {
		expected, ok := resources[name]
		if !ok {
			return false, nil
		}
		actual, err := repository.GetDeploymentResource(ctx, deploymentID, name)
		if err != nil {
			return false, err
		}
		if !bytes.Equal(actual, expected) {
			return false, nil
		}
	}
	return true, nil
}
