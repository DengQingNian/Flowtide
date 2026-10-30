package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
)

// batchFixture 是批处理端点测试环境。
type batchFixture struct {
	app        *engine.Engine   // 引擎
	server     *Server          // REST 服务，用于直接调用未注册路由的处理器
	httpServer *httptest.Server // HTTP 服务，用于已注册路由
	v1         string           // V1 流程定义 ID
	v2         string           // V2 流程定义 ID
}

// newBatchFixture 创建批处理测试环境并部署两个版本的迁移流程。
// @param t 测试上下文
// @return 测试环境
func newBatchFixture(t *testing.T) *batchFixture {
	t.Helper()
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := New(app)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		httpServer.Close()
		_ = app.Close(context.Background())
	})
	for _, xml := range []string{restMigrationV1, restMigrationV2} {
		response, postErr := http.Post(httpServer.URL+"/service/repository/deployments", "application/xml", bytes.NewBufferString(xml))
		if postErr != nil {
			t.Fatalf("部署失败: %v", postErr)
		}
		_ = response.Body.Close()
	}
	ids := definitionIDs(t, httpServer.URL)
	if ids[1] == "" || ids[2] == "" {
		t.Fatalf("流程定义缺失: %+v", ids)
	}
	return &batchFixture{app: app, server: server, httpServer: httpServer, v1: ids[1], v2: ids[2]}
}

// startInstance 启动一个 V1 流程实例。
// @param t 测试上下文
// @return 实例 ID
func (f *batchFixture) startInstance(t *testing.T) string {
	t.Helper()
	response := postJSON(t, f.httpServer.URL+"/service/runtime/process-instances", map[string]any{"processDefinitionId": f.v1})
	defer func() { _ = response.Body.Close() }()
	var instance struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("解析实例失败: %v", err)
	}
	if instance.ID == "" {
		t.Fatalf("启动实例失败")
	}
	return instance.ID
}

// batchMigrate 调用批处理迁移路由。
// @param t 测试上下文
// @param fixture 测试环境
// @param targetID 目标定义 ID
// @param instanceIDs 实例列表
// @param tenant 请求租户
// @return HTTP 响应
func batchMigrate(t *testing.T, fixture *batchFixture, targetID string, instanceIDs []string, tenant string) *http.Response {
	t.Helper()
	return postQuery(t, fixture.httpServer.URL, "/service/repository/process-definitions/"+targetID+"/batch-migrate", map[string]any{
		"processInstanceIds": instanceIDs,
		"activityMappings":   map[string]string{"oldTask": "newTask"},
	}, tenant)
}

// callBatchHandler 直接调用未注册路由的批处理处理器。
// @param t 测试上下文
// @param handler 处理器
// @param target 请求地址
// @param pathValues 路径参数
// @return 记录器
func callBatchHandler(t *testing.T, handler func(http.ResponseWriter, *http.Request), target string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range pathValues {
		request.SetPathValue(name, value)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

// decodeBatchBody 解析 JSON 响应体。
// @param t 测试上下文
// @param recorder 记录器
// @param target 目标对象
func decodeBatchBody(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
}

// TestREST_BatchLifecycle 验证批次成功迁移、查询、过滤、分页与统计。
// @param t 测试上下文
func TestREST_BatchLifecycle(t *testing.T) {
	fixture := newBatchFixture(t)
	first := fixture.startInstance(t)
	second := fixture.startInstance(t)

	response := batchMigrate(t, fixture, fixture.v2, []string{first, second}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("批量迁移应返回 201: %d", response.StatusCode)
	}
	var created api.Batch
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("解析批次失败: %v", err)
	}
	_ = response.Body.Close()
	if created.ID == "" || created.Type != engine.BatchTypeProcessInstanceMigration {
		t.Fatalf("批次字段错误: %+v", created)
	}
	if created.Total != 2 || created.Completed != 2 || created.Status != engine.BatchStatusCompleted {
		t.Fatalf("批次统计错误: %+v", created)
	}
	if created.StartTime.IsZero() || created.EndTime == nil {
		t.Fatalf("批次时间错误: %+v", created)
	}

	recorder := callBatchHandler(t, fixture.server.handleListBatches, "/service/management/batches", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("批次列表失败: %d", recorder.Code)
	}
	var page api.Page[api.Batch]
	decodeBatchBody(t, recorder, &page)
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0].ID != created.ID {
		t.Fatalf("批次列表错误: %+v", page)
	}

	filters := []struct {
		query string
		total int64
	}{
		{"?type=process-instance-migration", 1},
		{"?type=other", 0},
		{"?status=completed", 1},
		{"?status=failed", 0},
		{"?type=process-instance-migration&status=completed", 1},
		{"?tenantId=other", 0},
		{"?tenantId=", 1},
		{"?page=0&size=1", 1},
		{"?page=1&size=1", 1},
		{"?start=5&size=10", 1},
	}
	for _, filter := range filters {
		recorder = callBatchHandler(t, fixture.server.handleListBatches, "/service/management/batches"+filter.query, nil)
		var filtered api.Page[api.Batch]
		decodeBatchBody(t, recorder, &filtered)
		if filtered.Total != filter.total {
			t.Fatalf("过滤结果错误 %s: total=%d", filter.query, filtered.Total)
		}
		if filter.query == "?page=1&size=1" && (filtered.Start != 1 || len(filtered.Data) != 0) {
			t.Fatalf("分页参数错误: %+v", filtered)
		}
		if filter.query == "?start=5&size=10" && filtered.Start != 5 {
			t.Fatalf("显式 start 未生效: %+v", filtered)
		}
	}

	recorder = callBatchHandler(t, fixture.server.handleGetBatch, "/service/management/batches/"+created.ID, map[string]string{"id": created.ID})
	var got api.Batch
	decodeBatchBody(t, recorder, &got)
	if recorder.Code != http.StatusOK || got.ID != created.ID {
		t.Fatalf("查询批次失败: %d %+v", recorder.Code, got)
	}

	recorder = callBatchHandler(t, fixture.server.handleListBatchParts, "/service/management/batches/"+created.ID+"/batch-parts", map[string]string{"id": created.ID})
	var parts api.Page[api.BatchPart]
	decodeBatchBody(t, recorder, &parts)
	if recorder.Code != http.StatusOK || parts.Total != 2 {
		t.Fatalf("分片列表错误: %d %+v", recorder.Code, parts)
	}
	for _, part := range parts.Data {
		if part.Status != engine.BatchPartStatusCompleted || part.EndTime == nil || part.BatchID != created.ID {
			t.Fatalf("分片状态错误: %+v", part)
		}
	}

	recorder = callBatchHandler(t, fixture.server.handleGetBatchPart, "/service/management/batch-parts/"+parts.Data[0].ID, map[string]string{"id": parts.Data[0].ID})
	var part api.BatchPart
	decodeBatchBody(t, recorder, &part)
	if recorder.Code != http.StatusOK || part.ID != parts.Data[0].ID {
		t.Fatalf("查询分片失败: %d %+v", recorder.Code, part)
	}

	recorder = callBatchHandler(t, fixture.server.handleBatchSummary, "/service/management/batches/"+created.ID+"/summary", map[string]string{"id": created.ID})
	var summary struct {
		Total      int `json:"total"`
		Completed  int `json:"completed"`
		Failed     int `json:"failed"`
		InProgress int `json:"inProgress"`
	}
	decodeBatchBody(t, recorder, &summary)
	if recorder.Code != http.StatusOK || summary.Total != 2 || summary.Completed != 2 || summary.Failed != 0 || summary.InProgress != 0 {
		t.Fatalf("批次统计错误: %d %+v", recorder.Code, summary)
	}

	for _, call := range []struct {
		handler func(http.ResponseWriter, *http.Request)
		target  string
		values  map[string]string
	}{
		{fixture.server.handleGetBatch, "/service/management/batches/missing", map[string]string{"id": "missing"}},
		{fixture.server.handleListBatchParts, "/service/management/batches/missing/batch-parts", map[string]string{"id": "missing"}},
		{fixture.server.handleGetBatchPart, "/service/management/batch-parts/missing", map[string]string{"id": "missing"}},
		{fixture.server.handleBatchSummary, "/service/management/batches/missing/summary", map[string]string{"id": "missing"}},
	} {
		recorder = callBatchHandler(t, call.handler, call.target, call.values)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("缺失资源应返回 404 %s: %d", call.target, recorder.Code)
		}
	}
}

// TestREST_BatchPartialFailure 验证分片失败不影响其余实例，并统计失败分片。
// @param t 测试上下文
func TestREST_BatchPartialFailure(t *testing.T) {
	fixture := newBatchFixture(t)
	valid := fixture.startInstance(t)

	response := batchMigrate(t, fixture, fixture.v2, []string{valid, "missing"}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("批量迁移应返回 201: %d", response.StatusCode)
	}
	var batch api.Batch
	if err := json.NewDecoder(response.Body).Decode(&batch); err != nil {
		t.Fatalf("解析批次失败: %v", err)
	}
	_ = response.Body.Close()
	if batch.Total != 2 || batch.Completed != 1 || batch.Status != engine.BatchStatusFailed {
		t.Fatalf("部分失败批次错误: %+v", batch)
	}

	recorder := callBatchHandler(t, fixture.server.handleListBatchParts, "/service/management/batches/"+batch.ID+"/batch-parts", map[string]string{"id": batch.ID})
	var parts api.Page[api.BatchPart]
	decodeBatchBody(t, recorder, &parts)
	if parts.Total != 2 {
		t.Fatalf("分片数量错误: %+v", parts)
	}
	for _, part := range parts.Data {
		switch part.ProcessInstanceID {
		case valid:
			if part.Status != engine.BatchPartStatusCompleted || part.ErrorMessage != "" {
				t.Fatalf("成功分片状态错误: %+v", part)
			}
		case "missing":
			if part.Status != engine.BatchPartStatusFailed || part.ErrorMessage == "" || part.EndTime == nil {
				t.Fatalf("失败分片状态错误: %+v", part)
			}
		default:
			t.Fatalf("未知分片: %+v", part)
		}
	}

	recorder = callBatchHandler(t, fixture.server.handleBatchSummary, "/service/management/batches/"+batch.ID+"/summary", map[string]string{"id": batch.ID})
	var summary struct {
		Total      int `json:"total"`
		Completed  int `json:"completed"`
		Failed     int `json:"failed"`
		InProgress int `json:"inProgress"`
	}
	decodeBatchBody(t, recorder, &summary)
	if summary.Total != 2 || summary.Completed != 1 || summary.Failed != 1 || summary.InProgress != 0 {
		t.Fatalf("部分失败统计错误: %+v", summary)
	}

	recorder = callBatchHandler(t, fixture.server.handleListBatches, "/service/management/batches?status=failed", nil)
	var failedPage api.Page[api.Batch]
	decodeBatchBody(t, recorder, &failedPage)
	if failedPage.Total != 1 || failedPage.Data[0].ID != batch.ID {
		t.Fatalf("失败过滤错误: %+v", failedPage)
	}

	// 再次创建批次，覆盖多批次排序与查询
	second := fixture.startInstance(t)
	response = batchMigrate(t, fixture, fixture.v2, []string{second}, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("第二次批量迁移失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	recorder = callBatchHandler(t, fixture.server.handleListBatches, "/service/management/batches", nil)
	var all api.Page[api.Batch]
	decodeBatchBody(t, recorder, &all)
	if all.Total != 2 {
		t.Fatalf("批次数量错误: %+v", all)
	}

	response = batchMigrate(t, fixture, fixture.v2, []string{}, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("空实例列表应 400: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = batchMigrate(t, fixture, "missing", []string{second}, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("缺失目标定义应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = batchMigrate(t, fixture, fixture.v2, []string{second}, "other-tenant")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("跨租户批次迁移应 404: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	request := httptest.NewRequest(http.MethodPost, "/service/repository/process-definitions/"+fixture.v2+"/batch-migrate", bytes.NewBufferString("{"))
	request.Header.Set("Content-Type", "application/json")
	request.SetPathValue("id", fixture.v2)
	malformed := httptest.NewRecorder()
	fixture.server.handleDefinitionBatchMigrate(malformed, request)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("非法请求体应 400: %d", malformed.Code)
	}
}

// TestBatchSnapshotRestore 验证批次与分片随状态快照持久化和恢复。
// @param t 测试上下文
func TestBatchSnapshotRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "engine.json")
	first, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	v1 := deployBatchDefinition(t, first, restMigrationV1)
	v2 := deployBatchDefinition(t, first, restMigrationV2)
	instance, err := first.RuntimeService().StartProcessInstance(context.Background(), api.StartRequest{ProcessDefinitionID: v1})
	if err != nil {
		t.Fatalf("启动实例失败: %v", err)
	}
	batch, err := first.ManagementService().BatchMigrateProcessInstances(context.Background(), v2,
		[]string{instance.ID, "missing"}, map[string]string{"oldTask": "newTask"})
	if err != nil {
		t.Fatalf("批量迁移失败: %v", err)
	}
	if batch.Total != 2 || batch.Completed != 1 || batch.Status != engine.BatchStatusFailed {
		t.Fatalf("批次结果错误: %+v", batch)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("关闭引擎失败: %v", err)
	}

	second, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit, StatePath: path})
	if err != nil {
		t.Fatalf("恢复引擎失败: %v", err)
	}
	defer func() { _ = second.Close(context.Background()) }()
	batches, err := second.ManagementService().ListBatches(context.Background())
	if err != nil || len(batches) != 1 {
		t.Fatalf("恢复批次失败: %v %+v", err, batches)
	}
	restored := batches[0]
	if restored.ID != batch.ID || restored.Total != 2 || restored.Completed != 1 || restored.EndTime == nil {
		t.Fatalf("恢复批次字段错误: %+v", restored)
	}
	parts, err := second.ManagementService().ListBatchParts(context.Background(), batch.ID)
	if err != nil || len(parts) != 2 {
		t.Fatalf("恢复分片失败: %v %+v", err, parts)
	}
	completed, failed := 0, 0
	for _, part := range parts {
		if part.Status == engine.BatchPartStatusCompleted {
			completed++
		}
		if part.Status == engine.BatchPartStatusFailed {
			failed++
			if part.ErrorMessage == "" {
				t.Fatalf("失败分片缺少原因: %+v", part)
			}
		}
	}
	if completed != 1 || failed != 1 {
		t.Fatalf("恢复分片状态错误: %+v", parts)
	}
	gotPart, err := second.ManagementService().GetBatchPart(context.Background(), parts[0].ID)
	if err != nil || gotPart.ID != parts[0].ID || gotPart.BatchID != batch.ID {
		t.Fatalf("恢复分片查询失败: %v %+v", err, gotPart)
	}
	gotBatch, err := second.ManagementService().GetBatch(context.Background(), batch.ID)
	if err != nil || gotBatch.ID != batch.ID || gotBatch.Status != engine.BatchStatusFailed {
		t.Fatalf("恢复批次查询失败: %v %+v", err, gotBatch)
	}
}

// deployBatchDefinition 部署单个流程定义并返回定义 ID。
// @param t 测试上下文
// @param app 引擎
// @param xml BPMN XML
// @return 流程定义 ID
func deployBatchDefinition(t *testing.T, app *engine.Engine, xml string) string {
	t.Helper()
	deployment, err := app.RepositoryService().Deploy(context.Background(), api.DeploymentRequest{
		Name:      "batch-test",
		Resources: map[string][]byte{"process.bpmn20.xml": []byte(xml)},
	})
	if err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	definitions, err := app.RepositoryService().ListProcessDefinitions(context.Background())
	if err != nil {
		t.Fatalf("查询定义失败: %v", err)
	}
	for _, definition := range definitions {
		if definition.DeploymentID == deployment.ID {
			return definition.ID
		}
	}
	t.Fatalf("未找到部署对应的流程定义")
	return ""
}
