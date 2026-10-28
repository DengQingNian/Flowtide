package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

// newRepositoryExtraServer 创建仓储扩展测试服务。
// @param t 测试上下文
// @return REST 服务
// @return 引擎
func newRepositoryExtraServer(t *testing.T) (*Server, *engine.Engine) {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })
	return New(app), app
}

// createRepositoryExtraModel 创建测试模型。
// @param t 测试上下文
// @param app 引擎
// @param name 模型名称
// @param key 模型 Key
// @param tenant 租户
// @return 模型
func createRepositoryExtraModel(t *testing.T, app *engine.Engine, name, key, tenant string) api.Model {
	t.Helper()
	model, err := app.RepositoryService().CreateModel(context.Background(), api.CreateModelRequest{
		Name: name, Key: key, TenantID: tenant,
	})
	if err != nil {
		t.Fatalf("创建模型失败: %v", err)
	}
	return model
}

// repositoryExtraRequest 构造直接调用 handler 的请求。
// @param method HTTP 方法
// @param target 请求地址
// @param body 请求体
// @param tenant 租户
// @param id 路径参数
// @return 请求
func repositoryExtraRequest(method, target string, body []byte, tenant, id string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	if tenant != "" {
		request.Header.Set("X-Tenant-Id", tenant)
	}
	if id != "" {
		request.SetPathValue("id", id)
	}
	return request
}

// callRepositoryExtraHandler 直接调用 handler 并返回响应记录器。
// @param handler 处理器
// @param request 请求
// @return 响应记录器
func callRepositoryExtraHandler(handler http.HandlerFunc, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

// decodeRepositoryExtraDeployment 解析部署响应。
// @param t 测试上下文
// @param recorder 响应记录器
// @return 部署
func decodeRepositoryExtraDeployment(t *testing.T, recorder *httptest.ResponseRecorder) api.Deployment {
	t.Helper()
	var deployment api.Deployment
	if err := json.Unmarshal(recorder.Body.Bytes(), &deployment); err != nil {
		t.Fatalf("解析部署响应失败: %v，响应=%s", err, recorder.Body.String())
	}
	return deployment
}

// decodeRepositoryExtraError 解析错误响应。
// @param t 测试上下文
// @param recorder 响应记录器
// @return 响应体
func decodeRepositoryExtraError(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析错误响应失败: %v，响应=%s", err, recorder.Body.String())
	}
	return body
}

// repositoryExtraCountDefinitions 统计指定 Key 的流程定义数量。
// @param t 测试上下文
// @param app 引擎
// @param key 流程定义 Key
// @return 数量
func repositoryExtraCountDefinitions(t *testing.T, app *engine.Engine, key string) int {
	t.Helper()
	definitions, err := app.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil {
		t.Fatalf("查询流程定义失败: %v", err)
	}
	count := 0
	for _, definition := range definitions {
		if definition.Key == key {
			count++
		}
	}
	return count
}

// TestRepositoryExtra_DeployModel 验证模型部署成功、资源名与回退资源名。
// @param t 测试上下文
func TestRepositoryExtra_DeployModel(t *testing.T) {
	server, app := newRepositoryExtraServer(t)
	model := createRepositoryExtraModel(t, app, "Order Model", "orderKey", "t1")
	if err := app.RepositoryService().SetModelSource(context.Background(), model.ID, []byte(restProcess)); err != nil {
		t.Fatalf("设置模型源码失败: %v", err)
	}

	request := repositoryExtraRequest(http.MethodPost, "/service/repository/models/"+model.ID+"/deployment", nil, "", model.ID)
	recorder := callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("模型部署应返回 201: %d %s", recorder.Code, recorder.Body.String())
	}
	deployment := decodeRepositoryExtraDeployment(t, recorder)
	if deployment.Name != "Order Model" || deployment.TenantID != "t1" || deployment.ID == "" {
		t.Fatalf("部署字段错误: %+v", deployment)
	}
	resources, err := app.RepositoryService().ListDeploymentResources(context.Background(), deployment.ID)
	if err != nil || len(resources) != 1 || resources[0] != "orderKey.bpmn20.xml" {
		t.Fatalf("部署资源名错误: %v %v", resources, err)
	}
	data, err := app.RepositoryService().GetDeploymentResource(context.Background(), deployment.ID, resources[0])
	if err != nil || !bytes.Equal(data, []byte(restProcess)) {
		t.Fatalf("部署资源内容错误: %v", err)
	}

	fallback := createRepositoryExtraModel(t, app, "Fallback Model", "fallbackKey", "")
	if err := app.RepositoryService().SetModelSource(context.Background(), fallback.ID, []byte(restProcess)); err != nil {
		t.Fatalf("设置回退模型源码失败: %v", err)
	}
	emptyKey := ""
	if _, err := app.RepositoryService().UpdateModel(context.Background(), fallback.ID, api.ModelUpdate{Key: &emptyKey}); err != nil {
		t.Fatalf("清空模型 Key 失败: %v", err)
	}
	request = repositoryExtraRequest(http.MethodPost, "/service/repository/models/"+fallback.ID+"/deployment", nil, "", fallback.ID)
	recorder = callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("回退资源名部署应返回 201: %d %s", recorder.Code, recorder.Body.String())
	}
	deployment = decodeRepositoryExtraDeployment(t, recorder)
	resources, err = app.RepositoryService().ListDeploymentResources(context.Background(), deployment.ID)
	if err != nil || len(resources) != 1 || resources[0] != "model-"+fallback.ID+".bpmn20.xml" {
		t.Fatalf("回退资源名错误: %v %v", resources, err)
	}
}

// TestRepositoryExtra_DeployModelErrors 验证模型部署的缺失、空源码与校验失败。
// @param t 测试上下文
func TestRepositoryExtra_DeployModelErrors(t *testing.T) {
	server, app := newRepositoryExtraServer(t)

	request := repositoryExtraRequest(http.MethodPost, "/service/repository/models/missing/deployment", nil, "", "missing")
	recorder := callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("缺失模型应返回 404: %d %s", recorder.Code, recorder.Body.String())
	}

	model := createRepositoryExtraModel(t, app, "Empty Model", "emptyKey", "")
	request = repositoryExtraRequest(http.MethodPost, "/service/repository/models/"+model.ID+"/deployment", nil, "", model.ID)
	recorder = callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusBadRequest || decodeRepositoryExtraError(t, recorder)["code"] != "REST-0150" {
		t.Fatalf("无源码模型应返回 400: %d %s", recorder.Code, recorder.Body.String())
	}

	if err := app.RepositoryService().SetModelSource(context.Background(), model.ID, []byte{}); err != nil {
		t.Fatalf("设置空源码失败: %v", err)
	}
	request = repositoryExtraRequest(http.MethodPost, "/service/repository/models/"+model.ID+"/deployment", nil, "", model.ID)
	recorder = callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("空源码模型应返回 400: %d %s", recorder.Code, recorder.Body.String())
	}

	if err := app.RepositoryService().SetModelSource(context.Background(), model.ID, []byte{32, 10, 9}); err != nil {
		t.Fatalf("设置空白源码失败: %v", err)
	}
	request = repositoryExtraRequest(http.MethodPost, "/service/repository/models/"+model.ID+"/deployment", nil, "", model.ID)
	recorder = callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("空白源码模型应返回 400: %d %s", recorder.Code, recorder.Body.String())
	}

	if err := app.RepositoryService().SetModelSource(context.Background(), model.ID, []byte("<bad>")); err != nil {
		t.Fatalf("设置非法源码失败: %v", err)
	}
	request = repositoryExtraRequest(http.MethodPost, "/service/repository/models/"+model.ID+"/deployment", nil, "", model.ID)
	recorder = callRepositoryExtraHandler(server.handleDeployModel, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法源码应透传部署错误: %d %s", recorder.Code, recorder.Body.String())
	}
}

// TestRepositoryExtra_DuplicateFiltering 验证部署去重命中、租户隔离与新内容创建新部署。
// @param t 测试上下文
func TestRepositoryExtra_DuplicateFiltering(t *testing.T) {
	server, app := newRepositoryExtraServer(t)
	target := "/service/repository/deployments"

	first := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target, []byte(restProcess), "", ""))
	if first.Code != http.StatusCreated {
		t.Fatalf("首次部署应返回 201: %d %s", first.Code, first.Body.String())
	}
	firstDeployment := decodeRepositoryExtraDeployment(t, first)
	if count := repositoryExtraCountDefinitions(t, app, "rest"); count != 1 {
		t.Fatalf("首次部署版本数错误: %d", count)
	}

	duplicate := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=true", []byte(restProcess), "", ""))
	if duplicate.Code != http.StatusOK {
		t.Fatalf("重复部署应返回 200: %d %s", duplicate.Code, duplicate.Body.String())
	}
	if existing := decodeRepositoryExtraDeployment(t, duplicate); existing.ID != firstDeployment.ID {
		t.Fatalf("重复部署应返回现有部署: %+v", existing)
	}
	deployments, err := app.RepositoryService().ListDeployments(context.Background())
	if err != nil || len(deployments) != 1 {
		t.Fatalf("重复部署不应创建新部署: %v %+v", err, deployments)
	}
	if count := repositoryExtraCountDefinitions(t, app, "rest"); count != 1 {
		t.Fatalf("重复部署不应创建新版本: %d", count)
	}

	otherTenant := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=true", []byte(restProcess), "t2", ""))
	if otherTenant.Code != http.StatusCreated {
		t.Fatalf("不同租户应创建新部署: %d %s", otherTenant.Code, otherTenant.Body.String())
	}

	otherResource := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=true&resourceName=other.bpmn20.xml", []byte(restProcess), "", ""))
	if otherResource.Code != http.StatusCreated {
		t.Fatalf("不同资源名应创建新部署: %d %s", otherResource.Code, otherResource.Body.String())
	}

	differentContent := strings.Replace(restProcess, `id="rest"`, `id="rest2"`, 1)
	changed := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=true", []byte(differentContent), "", ""))
	if changed.Code != http.StatusCreated {
		t.Fatalf("不同内容应创建新部署: %d %s", changed.Code, changed.Body.String())
	}

	disabled := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=false", []byte(restProcess), "", ""))
	if disabled.Code != http.StatusCreated {
		t.Fatalf("关闭去重应创建新部署: %d %s", disabled.Code, disabled.Body.String())
	}
	absent := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target, []byte(restProcess), "", ""))
	if absent.Code != http.StatusCreated {
		t.Fatalf("缺省参数应保持原行为: %d %s", absent.Code, absent.Body.String())
	}
	if count := repositoryExtraCountDefinitions(t, app, "rest"); count != 5 {
		t.Fatalf("去重后的版本数错误: %d", count)
	}
}

// TestRepositoryExtra_CreateDeploymentErrors 验证部署解析、部署校验与查询失败。
// @param t 测试上下文
func TestRepositoryExtra_CreateDeploymentErrors(t *testing.T) {
	server, _ := newRepositoryExtraServer(t)
	target := "/service/repository/deployments"

	malformed := httptest.NewRequest(http.MethodPost, target+"?enableDuplicateFiltering=true", strings.NewReader("x"))
	malformed.Header.Set("Content-Type", "multipart/form-data")
	recorder := callRepositoryExtraHandler(server.handleCreateDeployment, malformed)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法 multipart 应返回 400: %d %s", recorder.Code, recorder.Body.String())
	}

	empty := callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=true", nil, "", ""))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("空部署内容应返回 400: %d %s", empty.Code, empty.Body.String())
	}

	closedApp, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建关闭引擎失败: %v", err)
	}
	closedServer := New(closedApp)
	if err := closedApp.Close(context.Background()); err != nil {
		t.Fatalf("关闭引擎失败: %v", err)
	}
	closed := callRepositoryExtraHandler(closedServer.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target+"?enableDuplicateFiltering=true", []byte(restProcess), "", ""))
	if closed.Code != http.StatusServiceUnavailable {
		t.Fatalf("关闭引擎应返回 503: %d %s", closed.Code, closed.Body.String())
	}
}

// TestRepositoryExtra_DeploymentResourcesEqual 验证资源集合与字节比较。
// @param t 测试上下文
func TestRepositoryExtra_DeploymentResourcesEqual(t *testing.T) {
	server, app := newRepositoryExtraServer(t)
	deployment, err := app.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name: "multi",
		Resources: map[string][]byte{
			"process.bpmn20.xml": []byte(restProcess),
			"notes.txt":          []byte("notes"),
		},
	})
	if err != nil {
		t.Fatalf("创建多资源部署失败: %v", err)
	}
	resources := map[string][]byte{
		"process.bpmn20.xml": []byte(restProcess),
		"notes.txt":          []byte("notes"),
	}
	equal, err := server.deploymentResourcesEqual(context.Background(), deployment.ID, resources)
	if err != nil || !equal {
		t.Fatalf("相同资源应相等: %v %v", equal, err)
	}

	extra := map[string][]byte{
		"process.bpmn20.xml": []byte(restProcess),
		"notes.txt":          []byte("notes"),
		"extra.txt":          []byte("extra"),
	}
	if equal, err = server.deploymentResourcesEqual(context.Background(), deployment.ID, extra); err != nil || equal {
		t.Fatalf("资源数量不同应不相等: %v %v", equal, err)
	}

	differentName := map[string][]byte{
		"process.bpmn20.xml": []byte(restProcess),
		"other.txt":          []byte("notes"),
	}
	if equal, err = server.deploymentResourcesEqual(context.Background(), deployment.ID, differentName); err != nil || equal {
		t.Fatalf("资源名不同应不相等: %v %v", equal, err)
	}

	differentBytes := map[string][]byte{
		"process.bpmn20.xml": []byte(restProcess),
		"notes.txt":          []byte("changed"),
	}
	if equal, err = server.deploymentResourcesEqual(context.Background(), deployment.ID, differentBytes); err != nil || equal {
		t.Fatalf("资源内容不同应不相等: %v %v", equal, err)
	}

	if equal, err = server.deploymentResourcesEqual(context.Background(), "missing", resources); err == nil || equal {
		t.Fatalf("缺失部署应返回错误: %v %v", equal, err)
	}
}

// repositoryExtraFakeRepository 是用于覆盖错误分支的仓库服务桩。
type repositoryExtraFakeRepository struct {
	api.RepositoryService                  // 未实现的方法保持空实现
	model                 api.Model        // 模型
	source                []byte           // 模型源码
	sourceErr             error            // 源码读取错误
	deployments           []api.Deployment // 部署列表
	names                 []string         // 资源名列表
	listNamesErr          error            // 资源名查询错误
	resourceErr           error            // 资源内容读取错误
}

// GetModel 返回桩模型。
// @param ctx 上下文
// @param modelID 模型 ID
// @return 模型
// @return 错误
func (f *repositoryExtraFakeRepository) GetModel(ctx context.Context, modelID string) (api.Model, error) {
	return f.model, nil
}

// GetModelSource 返回桩源码或错误。
// @param ctx 上下文
// @param modelID 模型 ID
// @return 源码
// @return 错误
func (f *repositoryExtraFakeRepository) GetModelSource(ctx context.Context, modelID string) ([]byte, error) {
	return f.source, f.sourceErr
}

// ListDeployments 返回桩部署列表。
// @param ctx 上下文
// @return 部署列表
// @return 错误
func (f *repositoryExtraFakeRepository) ListDeployments(ctx context.Context) ([]api.Deployment, error) {
	return f.deployments, nil
}

// ListDeploymentResources 返回桩资源名列表。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @return 资源名列表
// @return 错误
func (f *repositoryExtraFakeRepository) ListDeploymentResources(ctx context.Context, deploymentID string) ([]string, error) {
	return f.names, f.listNamesErr
}

// GetDeploymentResource 返回桩资源内容或错误。
// @param ctx 上下文
// @param deploymentID 部署 ID
// @param resourceName 资源名
// @return 资源内容
// @return 错误
func (f *repositoryExtraFakeRepository) GetDeploymentResource(ctx context.Context, deploymentID, resourceName string) ([]byte, error) {
	return nil, f.resourceErr
}

// repositoryExtraFakeEngine 是仅提供仓库服务的引擎桩。
type repositoryExtraFakeEngine struct {
	api.Engine                       // 未实现的服务保持空实现
	repository api.RepositoryService // 仓库服务
}

// RepositoryService 返回桩仓库服务。
// @return 仓库服务
func (f repositoryExtraFakeEngine) RepositoryService() api.RepositoryService {
	return f.repository
}

// TestRepositoryExtra_ErrorPropagation 验证源码读取、部署列表与资源读取错误的分支。
// @param t 测试上下文
func TestRepositoryExtra_ErrorPropagation(t *testing.T) {
	sourceServer := New(repositoryExtraFakeEngine{repository: &repositoryExtraFakeRepository{
		model:     api.Model{ID: "m", Name: "M", Key: "k"},
		sourceErr: errors.New("源码读取失败"),
	}})
	request := repositoryExtraRequest(http.MethodPost, "/service/repository/models/m/deployment", nil, "", "m")
	recorder := callRepositoryExtraHandler(sourceServer.handleDeployModel, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("源码读取错误应返回 500: %d %s", recorder.Code, recorder.Body.String())
	}

	repo := &repositoryExtraFakeRepository{
		deployments:  []api.Deployment{{ID: "d1"}},
		listNamesErr: errors.New("资源列表读取失败"),
	}
	server := New(repositoryExtraFakeEngine{repository: repo})
	target := "/service/repository/deployments?enableDuplicateFiltering=true"
	recorder = callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target, []byte(restProcess), "", ""))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("资源列表错误应返回 500: %d %s", recorder.Code, recorder.Body.String())
	}

	repo.listNamesErr = nil
	repo.names = []string{"deployment.bpmn20.xml"}
	repo.resourceErr = errors.New("资源读取失败")
	recorder = callRepositoryExtraHandler(server.handleCreateDeployment,
		repositoryExtraRequest(http.MethodPost, target, []byte(restProcess), "", ""))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("资源读取错误应返回 500: %d %s", recorder.Code, recorder.Body.String())
	}
}
