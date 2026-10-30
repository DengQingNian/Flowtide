package rest

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
)

// historicActivityIDs 提取历史活动查询结果中的活动主键。
// @param page 历史活动查询分页
// @return 活动主键列表
func historicActivityIDs(page queryPageResult[map[string]any]) []string {
	ids := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		id, _ := item["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// findHistoricActivityID 按流程实例与活动定义查找历史活动主键。
// @param page 历史活动查询分页
// @param instanceID 流程实例 ID
// @param activityID 活动定义 ID
// @return 历史活动主键，未找到时为空
func findHistoricActivityID(page queryPageResult[map[string]any], instanceID, activityID string) string {
	for _, item := range page.Data {
		// 同时匹配实例与活动定义，避免跨实例误取
		if item["processInstanceId"] == instanceID && item["activityId"] == activityID {
			id, _ := item["id"].(string)
			return id
		}
	}
	return ""
}

// historicVariableNames 提取历史变量查询结果中的变量名。
// @param page 历史变量查询分页
// @return 变量名列表
func historicVariableNames(page queryPageResult[map[string]any]) []string {
	names := make([]string, 0, len(page.Data))
	for _, item := range page.Data {
		variable, _ := item["variable"].(map[string]any)
		name, _ := variable["name"].(string)
		names = append(names, name)
	}
	return names
}

// queryInstanceTask 返回流程实例当前任务的任务 ID 与执行 ID。
// @param t 测试上下文
// @param serverURL 服务地址
// @param instanceID 流程实例 ID
// @return 任务 ID
// @return 执行 ID
func queryInstanceTask(t *testing.T, serverURL, instanceID string) (string, string) {
	t.Helper()
	response, err := http.Get(serverURL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("查询实例任务失败: %v", err)
	}
	page := decodeQueryPage[map[string]any](t, response)
	if len(page.Data) == 0 {
		t.Fatalf("实例没有待办任务: %s", instanceID)
	}
	taskID, _ := page.Data[0]["id"].(string)
	executionID, _ := page.Data[0]["executionId"].(string)
	if taskID == "" || executionID == "" {
		t.Fatalf("任务字段缺失: %+v", page.Data[0])
	}
	return taskID, executionID
}

// TestREST_HistoricActivityQueryPushdown 验证历史活动查询接入引擎下推后过滤、分页与总量语义不变。
// @param t 测试上下文
func TestREST_HistoricActivityQueryPushdown(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	completedID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	runningID := startQueryInstance(t, server.URL, "rest", "bk2", "Order B")
	completeQueryTask(t, server.URL, completedID)

	// 已结束实例记录开始、两条顺序流、用户任务与结束共五条活动，进行中实例记录三条
	response := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 8 || len(page.Data) != 8 {
		t.Fatalf("全量历史活动查询失败: %d %+v", response.StatusCode, page)
	}

	// 等值字段与时间区间经下推后与内存复核一致
	cases := []struct {
		name string         // 用例说明
		body map[string]any // 查询体
		want int            // 期望总数
	}{
		{"活动定义 ID", map[string]any{"activityId": "task"}, 2},
		{"陌生活动定义 ID", map[string]any{"activityId": "missing"}, 0},
		{"活动名称", map[string]any{"activityName": "Approve"}, 2},
		{"活动名称别名", map[string]any{"name": "Approve"}, 2},
		{"陌生活动名称", map[string]any{"activityName": "missing"}, 0},
		{"用户任务类型", map[string]any{"activityType": "userTask"}, 2},
		{"开始事件类型", map[string]any{"activityType": "startEvent"}, 2},
		{"顺序流类型", map[string]any{"activityType": "sequenceFlow"}, 3},
		{"陌生活动类型", map[string]any{"activityType": "missing"}, 0},
		{"进行中实例", map[string]any{"processInstanceId": runningID}, 3},
		{"定义 Key", map[string]any{"processDefinitionKey": "rest"}, 8},
		{"陌生定义 Key", map[string]any{"processDefinitionKey": "missing"}, 0},
		{"已结束活动", map[string]any{"finished": true}, 7},
		{"未结束活动", map[string]any{"finished": false}, 1},
		{"开始时间下界", map[string]any{"startedAfter": "2020-01-01T00:00:00Z"}, 8},
		{"开始时间上界", map[string]any{"startedBefore": "2000-01-01T00:00:00Z"}, 0},
		{"结束时间下界", map[string]any{"endedAfter": "2020-01-01T00:00:00Z"}, 7},
		{"结束时间上界", map[string]any{"endedBefore": "2000-01-01T00:00:00Z"}, 0},
		{"结束时间区间", map[string]any{"endedAfter": "2020-01-01T00:00:00Z", "endedBefore": "2040-01-01T00:00:00Z"}, 7},
	}
	for _, tc := range cases {
		response = postQuery(t, server.URL, "/service/query/historic-activity-instances", tc.body, "")
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK || page.Total != tc.want {
			t.Fatalf("%s 查询失败: %d %+v", tc.name, response.StatusCode, page)
		}
	}

	// 活动主键等值过滤命中唯一记录
	taskActivityID := findHistoricActivityID(page, completedID, "task")
	if taskActivityID == "" {
		t.Fatalf("未找到已完成实例的用户任务活动")
	}
	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"id": taskActivityID}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || page.Data[0]["id"] != taskActivityID {
		t.Fatalf("按活动主键查询失败: %+v", page)
	}

	// 非法布尔与时间参数必须在进入引擎前被拒绝
	for _, body := range []map[string]any{
		{"finished": "x"},
		{"startedAfter": "bad"},
		{"startedBefore": "bad"},
		{"endedAfter": "bad"},
		{"endedBefore": "bad"},
		{"start": "x"},
		{"sort": "missing"},
		{"order": "sideways"},
	} {
		response = postQuery(t, server.URL, "/service/query/historic-activity-instances", body, "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法参数应 400 %+v: %d", body, response.StatusCode)
		}
		_ = response.Body.Close()
	}

	// 显式 size=0 按请求生效，分页偏移与截断均不影响总数
	sizeZero := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"size": 0}, "")
	zeroPage := decodeQueryPage[map[string]any](t, sizeZero)
	if zeroPage.Total != 8 || len(zeroPage.Data) != 0 || zeroPage.Size != 0 {
		t.Fatalf("size=0 历史活动查询失败: %+v", zeroPage)
	}
	paged := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"start": 2, "size": 3}, "")
	pagedPage := decodeQueryPage[map[string]any](t, paged)
	if pagedPage.Total != 8 || len(pagedPage.Data) != 3 || pagedPage.Start != 2 || pagedPage.Size != 3 {
		t.Fatalf("历史活动分页失败: %+v", pagedPage)
	}
	beyond := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"start": 8, "size": 10}, "")
	beyondPage := decodeQueryPage[map[string]any](t, beyond)
	if beyondPage.Total != 8 || len(beyondPage.Data) != 0 || beyondPage.Start != 8 {
		t.Fatalf("越界历史活动分页失败: %+v", beyondPage)
	}
}

// TestREST_HistoricVariableQueryPushdown 验证历史变量查询下推后过滤与分页语义不变，模糊匹配仍走内存复核。
// @param t 测试上下文
func TestREST_HistoricVariableQueryPushdown(t *testing.T) {
	server, _ := newHistoryTestServer(t, engine.HistoryFull)
	deployREST(t, server)
	startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	secondID := startQueryInstance(t, server.URL, "rest", "bk2", "Order B")
	taskID, executionID := queryInstanceTask(t, server.URL, secondID)

	// 任务局部变量写入后历史变量包含流程变量与任务变量两类
	response := postJSON(t, server.URL+"/service/runtime/tasks/"+taskID+"/variables", map[string]any{"name": "amount", "value": 12})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建任务变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 3 {
		t.Fatalf("全量历史变量查询失败: %d %+v", response.StatusCode, page)
	}
	if names := historicVariableNames(page); strings.Join(names, ",") != "amount,initiator,initiator" {
		t.Fatalf("历史变量默认排序错误: %v", names)
	}

	// 名称、类型别名与实例、执行、任务等值字段经下推后结果一致
	cases := []struct {
		name string         // 用例说明
		body map[string]any // 查询体
		want int            // 期望总数
	}{
		{"变量名称", map[string]any{"variableName": "initiator"}, 2},
		{"变量名称别名", map[string]any{"name": "initiator"}, 2},
		{"任务变量名称", map[string]any{"variableName": "amount"}, 1},
		{"陌生变量名称", map[string]any{"variableName": "missing"}, 0},
		{"变量类型", map[string]any{"variableType": "string"}, 2},
		{"变量类型别名", map[string]any{"type": "string"}, 2},
		{"任务变量类型", map[string]any{"variableType": "double"}, 1},
		{"陌生变量类型", map[string]any{"variableType": "missing"}, 0},
		{"流程实例", map[string]any{"processInstanceId": secondID}, 2},
		{"执行", map[string]any{"executionId": executionID}, 1},
		{"任务", map[string]any{"taskId": taskID}, 1},
		{"实例与名称组合", map[string]any{"processInstanceId": secondID, "variableName": "amount"}, 1},
	}
	for _, tc := range cases {
		response = postQuery(t, server.URL, "/service/query/historic-variable-instances", tc.body, "")
		page = decodeQueryPage[map[string]any](t, response)
		if response.StatusCode != http.StatusOK || page.Total != tc.want {
			t.Fatalf("%s 查询失败: %d %+v", tc.name, response.StatusCode, page)
		}
	}

	// 模糊匹配尚未下推，仍由内存复核并保持忽略大小写语义
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"variableNameLike": "%mou%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || historicVariableNames(page)[0] != "amount" {
		t.Fatalf("变量模糊匹配失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"nameLike": "%MO%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 {
		t.Fatalf("变量模糊匹配别名失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"variableNameLike": "%absent%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 {
		t.Fatalf("陌生模糊匹配应无结果: %+v", page)
	}

	// 分页语义与既有历史查询一致，总数不受 size 影响
	sizeZero := postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"size": 0}, "")
	zeroPage := decodeQueryPage[map[string]any](t, sizeZero)
	if zeroPage.Total != 3 || len(zeroPage.Data) != 0 || zeroPage.Size != 0 {
		t.Fatalf("size=0 历史变量查询失败: %+v", zeroPage)
	}
	paged := postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"start": 1, "size": 1}, "")
	pagedPage := decodeQueryPage[map[string]any](t, paged)
	if pagedPage.Total != 3 || len(pagedPage.Data) != 1 || pagedPage.Start != 1 || pagedPage.Size != 1 {
		t.Fatalf("历史变量分页失败: %+v", pagedPage)
	}
	beyond := postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"start": 3, "size": 10}, "")
	beyondPage := decodeQueryPage[map[string]any](t, beyond)
	if beyondPage.Total != 3 || len(beyondPage.Data) != 0 {
		t.Fatalf("越界历史变量分页失败: %+v", beyondPage)
	}
}

// TestREST_HistoricQueryGetEndpoints 验证历史活动与历史变量的 GET 列表按实例过滤并返回完整分页外壳。
// @param t 测试上下文
func TestREST_HistoricQueryGetEndpoints(t *testing.T) {
	server, _ := newHistoryTestServer(t, engine.HistoryFull)
	deployREST(t, server)
	completedID := startQueryInstance(t, server.URL, "rest", "bk1", "Order A")
	runningID := startQueryInstance(t, server.URL, "rest", "bk2", "Order B")
	completeQueryTask(t, server.URL, completedID)

	// GET 历史活动按实例过滤，start/size 只影响当页数据而不影响总数
	response, err := http.Get(server.URL + "/service/history/historic-activity-instances?processInstanceId=" + completedID + "&start=0&size=100")
	if err != nil {
		t.Fatalf("GET 历史活动失败: %v", err)
	}
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 5 || len(page.Data) != 5 {
		t.Fatalf("GET 历史活动查询失败: %d %+v", response.StatusCode, page)
	}
	for _, item := range page.Data {
		if item["processInstanceId"] != completedID {
			t.Fatalf("GET 历史活动实例过滤失败: %+v", item)
		}
	}
	response, err = http.Get(server.URL + "/service/history/historic-activity-instances?processInstanceId=" + completedID + "&start=1&size=2&sort=activityId&order=desc")
	if err != nil {
		t.Fatalf("GET 历史活动失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 5 || len(page.Data) != 2 || page.Start != 1 || page.Size != 2 || page.Sort != "activityId" || page.Order != "desc" {
		t.Fatalf("GET 历史活动分页失败: %+v", page)
	}
	response, err = http.Get(server.URL + "/service/history/historic-activity-instances?processInstanceId=missing")
	if err != nil {
		t.Fatalf("GET 历史活动失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("GET 陌生实例历史活动应无结果: %+v", page)
	}

	// GET 历史变量按实例过滤并支持偏移分页
	response, err = http.Get(server.URL + "/service/history/historic-variable-instances?processInstanceId=" + runningID + "&start=0&size=10")
	if err != nil {
		t.Fatalf("GET 历史变量失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || len(page.Data) != 1 {
		t.Fatalf("GET 历史变量查询失败: %d %+v", response.StatusCode, page)
	}
	response, err = http.Get(server.URL + "/service/history/historic-variable-instances?processInstanceId=" + runningID + "&start=0&size=1")
	if err != nil {
		t.Fatalf("GET 历史变量失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || len(page.Data) != 1 || page.Size != 1 {
		t.Fatalf("GET 历史变量分页失败: %+v", page)
	}
	response, err = http.Get(server.URL + "/service/history/historic-variable-instances?processInstanceId=" + runningID + "&start=1&size=10")
	if err != nil {
		t.Fatalf("GET 历史变量失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1 || len(page.Data) != 0 || page.Start != 1 {
		t.Fatalf("越界 GET 历史变量分页失败: %+v", page)
	}
}

// TestREST_HistoricActivityPageLoop 验证历史活动超过引擎单页一千条时仍读完全部匹配项再分页。
// 用例先经引擎接口批量构造跨页历史数据，再通过 REST 查询断言总量与分页一致。
// @param t 测试上下文
func TestREST_HistoricActivityPageLoop(t *testing.T) {
	ctx := context.Background()
	source, sourceEngine := newTestServer(t)
	deployREST(t, source)
	// 每个已完成实例记录五条历史活动，两百零一个实例共一千零五条，跨越单页一千条边界
	for i := 0; i < 201; i++ {
		instance, err := sourceEngine.StartProcessInstance(ctx, api.StartRequest{
			ProcessDefinitionKey: "rest",
			BusinessKey:          fmt.Sprintf("loop-%d", i),
			Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
		})
		if err != nil {
			t.Fatalf("启动批量实例失败: %v", err)
		}
		tasks, err := sourceEngine.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
		if err != nil || len(tasks.Data) == 0 {
			t.Fatalf("批量实例任务缺失: %v %+v", err, tasks)
		}
		if err := sourceEngine.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
			t.Fatalf("完成批量任务失败: %v", err)
		}
	}
	rows, err := sourceEngine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("读取批量快照失败: %v", err)
	}
	server, serverEngine := newTestServer(t)
	if err := serverEngine.RestoreRows(ctx, rows); err != nil {
		t.Fatalf("恢复批量历史失败: %v", err)
	}

	// 单页一千条时总数必须覆盖第二页剩余五条
	response := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"size": 1000}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1005 || len(page.Data) != 1000 || page.Size != 1000 {
		t.Fatalf("跨页历史活动查询失败: %d total=%d len=%d", response.StatusCode, page.Total, len(page.Data))
	}
	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"start": 1000, "size": 1000}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 1005 || len(page.Data) != 5 || page.Start != 1000 {
		t.Fatalf("跨页历史活动第二页失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"activityType": "endEvent", "size": 1000}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 201 {
		t.Fatalf("跨页历史活动类型过滤失败: %+v", page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"size": 1000}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 201 || len(page.Data) != 201 {
		t.Fatalf("跨页历史变量查询失败: %+v", page)
	}
}

// expectHistoricRowLoadAll 按表登记行仓储启动恢复所需的读表期望，行内容与快照逐列一致。
// @param t 测试上下文
// @param mock SQL 断言器
// @param rows 行快照
func expectHistoricRowLoadAll(t *testing.T, mock sqlmock.Sqlmock, rows []flowablerow.Row) {
	t.Helper()
	grouped := make(map[string][]flowablerow.Row)
	for _, row := range rows {
		grouped[row.Table] = append(grouped[row.Table], row)
	}
	for _, table := range flowablerow.Tables() {
		sample, err := flowablerow.NewRow(table)
		if err != nil {
			t.Fatalf("构造行模板失败 %s: %v", table, err)
		}
		columns := sample.Columns()
		set := sqlmock.NewRows(columns)
		for _, row := range grouped[table] {
			line := make([]driver.Value, len(columns))
			for i, column := range columns {
				line[i] = row.Values[column]
			}
			set.AddRow(line...)
		}
		// 读表 SQL 与行仓储 loadRows 的拼接结果逐字一致
		mock.ExpectQuery("SELECT " + strings.Join(columns, ", ") + " FROM " + table).WillReturnRows(set)
	}
}

// newHistoricRowServer 使用 sqlmock 行仓储从行快照恢复状态并创建 REST 测试服务。
// @param t 测试上下文
// @param rows 行快照
// @param defaultTenant 引擎默认租户
// @return 测试服务器
// @return SQL 断言器
func newHistoricRowServer(t *testing.T, rows []flowablerow.Row, defaultTenant string) (*httptest.Server, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	expectHistoricRowLoadAll(t, mock, rows)
	repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, "mysql")
	if err != nil {
		t.Fatalf("创建行仓储失败: %v", err)
	}
	app, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryAudit, TenantID: defaultTenant})
	if err != nil {
		t.Fatalf("创建行仓储引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app).Handler())
	t.Cleanup(func() {
		server.Close()
		_ = app.Close(context.Background())
	})
	// 恢复阶段只读表不写表，期望应全部满足
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("行恢复读表期望未满足: %v", err)
	}
	return server, mock
}

// userTaskActivityRowIDs 提取行快照中用户任务历史活动的主键并按字典序返回。
// @param rows 行快照
// @return 活动主键列表
func userTaskActivityRowIDs(rows []flowablerow.Row) []string {
	ids := make([]string, 0)
	for _, row := range rows {
		if row.Table != "ACT_HI_ACTINST" || row.Values["ACT_TYPE_"] != "userTask" {
			continue
		}
		id, _ := row.Values["ID_"].(string)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// instanceActivityRowIDs 提取行快照中指定实例的历史活动主键。
// @param rows 行快照
// @param instanceID 流程实例 ID
// @return 活动主键列表
func instanceActivityRowIDs(rows []flowablerow.Row, instanceID string) []string {
	ids := make([]string, 0)
	for _, row := range rows {
		if row.Table != "ACT_HI_ACTINST" || row.Values["PROC_INST_ID_"] != instanceID {
			continue
		}
		id, _ := row.Values["ID_"].(string)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// TestREST_HistoricQueryRowPushdown 验证行仓储引擎下历史活动查询按 TENANT_ID_ 下推 SQL，
// 历史变量在租户作用域下回退内存过滤仍保持隔离，HTTP 层过滤、分页与总数与下推结果一致。
// @param t 测试上下文
func TestREST_HistoricQueryRowPushdown(t *testing.T) {
	ctx := context.Background()
	source, sourceEngine := newTestServer(t, "t1")
	deployREST(t, source)
	firstID := startQueryInstance(t, source.URL, "rest", "bk1", "Order A")
	secondID := startQueryInstance(t, source.URL, "rest", "bk2", "Order B")
	completeQueryTask(t, source.URL, secondID)
	rows, err := sourceEngine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("读取行快照失败: %v", err)
	}
	server, mock := newHistoricRowServer(t, rows, "t1")

	// 显式 tenantId 的历史活动查询以 ACT_TYPE_ 与 TENANT_ID_ 等值条件下推
	userTaskIDs := userTaskActivityRowIDs(rows)
	if len(userTaskIDs) != 2 {
		t.Fatalf("用户任务活动行数量错误: %v", userTaskIDs)
	}
	argIDs := make([]driver.Value, 0, len(userTaskIDs))
	for _, id := range userTaskIDs {
		argIDs = append(argIDs, id)
	}
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_ACTINST A WHERE A.ACT_TYPE_ = ? AND A.TENANT_ID_ = ?").
		WithArgs("userTask", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery("SELECT A.ID_ FROM ACT_HI_ACTINST A WHERE A.ACT_TYPE_ = ? AND A.TENANT_ID_ = ? ORDER BY A.START_TIME_ ASC, A.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs("userTask", "t1", int64(1000), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(argIDs[0]).AddRow(argIDs[1]))
	response := postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"tenantId": "t1", "activityType": "userTask"}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 2 || len(page.Data) != 2 {
		t.Fatalf("行仓储历史活动查询失败: %d %+v", response.StatusCode, page)
	}
	gotIDs := historicActivityIDs(page)
	sort.Strings(gotIDs)
	if strings.Join(gotIDs, ",") != strings.Join(userTaskIDs, ",") {
		t.Fatalf("行仓储历史活动结果错误: %v", gotIDs)
	}

	// 陌生租户条件同样下推且无结果
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_ACTINST A WHERE A.TENANT_ID_ = ?").
		WithArgs("t2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT A.ID_ FROM ACT_HI_ACTINST A WHERE A.TENANT_ID_ = ? ORDER BY A.START_TIME_ ASC, A.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs("t2", int64(1000), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}))
	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"tenantId": "t2"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("陌生租户历史活动应无结果: %+v", page)
	}

	// GET 历史活动按实例与默认租户下推并保留 start/size 分页外壳
	instanceIDs := instanceActivityRowIDs(rows, secondID)
	if len(instanceIDs) != 5 {
		t.Fatalf("已完成实例活动行数量错误: %v", instanceIDs)
	}
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_ACTINST A WHERE A.PROC_INST_ID_ = ? AND A.TENANT_ID_ = ?").
		WithArgs(secondID, "t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
	instanceRows := sqlmock.NewRows([]string{"ID_"})
	for _, id := range instanceIDs {
		instanceRows.AddRow(id)
	}
	mock.ExpectQuery("SELECT A.ID_ FROM ACT_HI_ACTINST A WHERE A.PROC_INST_ID_ = ? AND A.TENANT_ID_ = ? ORDER BY A.START_TIME_ ASC, A.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs(secondID, "t1", int64(1000), int64(0)).
		WillReturnRows(instanceRows)
	getResponse, err := http.Get(server.URL + "/service/history/historic-activity-instances?processInstanceId=" + secondID + "&start=1&size=2")
	if err != nil {
		t.Fatalf("GET 行仓储历史活动失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, getResponse)
	if page.Total != 5 || len(page.Data) != 2 || page.Start != 1 || page.Size != 2 {
		t.Fatalf("GET 行仓储历史活动分页失败: %+v", page)
	}

	// 历史变量表无租户列，租户作用域下不得访问 SQL，仍按内存过滤返回隔离结果
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"processInstanceId": firstID}, "t1")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || page.Data[0]["processInstanceId"] != firstID {
		t.Fatalf("租户作用域历史变量查询失败: %d %+v", response.StatusCode, page)
	}
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{}, "t2")
	page = decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 0 {
		t.Fatalf("跨租户历史变量查询应无结果: %d %+v", response.StatusCode, page)
	}
	variableResponse, err := http.Get(server.URL + "/service/history/historic-variable-instances?processInstanceId=" + firstID)
	if err != nil {
		t.Fatalf("GET 行仓储历史变量失败: %v", err)
	}
	page = decodeQueryPage[map[string]any](t, variableResponse)
	if page.Total != 1 || len(page.Data) != 1 || page.Data[0]["processInstanceId"] != firstID {
		t.Fatalf("GET 行仓储历史变量失败: %+v", page)
	}
	// 历史活动 SQL 失败必须返回 500，不得静默回退内存掩盖数据库问题
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_ACTINST A WHERE A.TENANT_ID_ = ?").
		WithArgs("t1").
		WillReturnError(errors.New("数据库不可用"))
	response = postQuery(t, server.URL, "/service/query/historic-activity-instances", map[string]any{"tenantId": "t1"}, "")
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("历史活动 SQL 失败应 500: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// 全部 SQL 期望必须恰好消费完毕，未消费即说明下推或恢复路径偏离
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 期望未满足: %v", err)
	}
}

// TestREST_HistoricVariableRowPushdown 验证无租户作用域时历史变量查询下推到 ACT_HI_VARINST，
// 模糊匹配仍由内存复核收敛，SQL 失败直接返回 500 而不回退内存。
// @param t 测试上下文
func TestREST_HistoricVariableRowPushdown(t *testing.T) {
	ctx := context.Background()
	source, sourceEngine := newTestServer(t)
	deployREST(t, source)
	firstID := startQueryInstance(t, source.URL, "rest", "bk1", "Order A")
	secondID := startQueryInstance(t, source.URL, "rest", "bk2", "Order B")
	rows, err := sourceEngine.SnapshotRows(ctx)
	if err != nil {
		t.Fatalf("读取行快照失败: %v", err)
	}
	server, mock := newHistoricRowServer(t, rows, "")

	// 变量名与实例等值条件下推，SQL 返回 ID 与内存状态一致
	firstVariableID := variableRowID(rows, firstID, "initiator")
	secondVariableID := variableRowID(rows, secondID, "initiator")
	if firstVariableID == "" || secondVariableID == "" {
		t.Fatalf("历史变量行缺失: %s %s", firstVariableID, secondVariableID)
	}
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_VARINST V WHERE V.NAME_ = ? AND V.PROC_INST_ID_ = ?").
		WithArgs("initiator", firstID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT V.ID_ FROM ACT_HI_VARINST V WHERE V.NAME_ = ? AND V.PROC_INST_ID_ = ? ORDER BY V.NAME_ ASC, V.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs("initiator", firstID, int64(1000), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(firstVariableID))
	response := postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"variableName": "initiator", "processInstanceId": firstID}, "")
	page := decodeQueryPage[map[string]any](t, response)
	if response.StatusCode != http.StatusOK || page.Total != 1 || page.Data[0]["processInstanceId"] != firstID {
		t.Fatalf("行仓储历史变量查询失败: %d %+v", response.StatusCode, page)
	}
	if names := historicVariableNames(page); len(names) != 1 || names[0] != "initiator" {
		t.Fatalf("行仓储历史变量结果错误: %v", names)
	}

	// 模糊匹配条件不参与下推，SQL 返回全部变量后由内存复核收敛为零条
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_VARINST V").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery("SELECT V.ID_ FROM ACT_HI_VARINST V ORDER BY V.NAME_ ASC, V.ID_ ASC LIMIT ? OFFSET ?").
		WithArgs(int64(1000), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"ID_"}).AddRow(firstVariableID).AddRow(secondVariableID))
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{"variableNameLike": "%absent%"}, "")
	page = decodeQueryPage[map[string]any](t, response)
	if page.Total != 0 || len(page.Data) != 0 {
		t.Fatalf("行仓储变量模糊匹配应被内存过滤: %+v", page)
	}

	// SQL 失败必须返回 500，不能静默回退内存
	mock.ExpectQuery("SELECT COUNT(*) FROM ACT_HI_VARINST V").WillReturnError(errors.New("数据库不可用"))
	response = postQuery(t, server.URL, "/service/query/historic-variable-instances", map[string]any{}, "")
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("历史变量 SQL 失败应 500: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	// 全部 SQL 期望必须恰好消费完毕
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL 期望未满足: %v", err)
	}
}

// variableRowID 返回行快照中指定实例与变量名的历史变量主键。
// @param rows 行快照
// @param instanceID 流程实例 ID
// @param name 变量名
// @return 历史变量主键，未找到时为空
func variableRowID(rows []flowablerow.Row, instanceID, name string) string {
	for _, row := range rows {
		// 只匹配历史变量表内的同实例同名字段
		if row.Table != "ACT_HI_VARINST" || row.Values["PROC_INST_ID_"] != instanceID || row.Values["NAME_"] != name {
			continue
		}
		id, _ := row.Values["ID_"].(string)
		return id
	}
	return ""
}
