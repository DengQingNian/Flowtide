package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// mustQueryBody 解析查询体，失败时终止测试。
// @param t 测试上下文
// @param raw JSON 字符串
// @return 查询体
func mustQueryBody(t *testing.T, raw string) queryBody {
	t.Helper()
	var body queryBody
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("解析查询体失败: %v", err)
	}
	return body
}

// TestQueryBodyParsing 验证查询字段解析。
// @param t 测试上下文
func TestQueryBodyParsing(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	body, err := readQueryBody(request)
	if err != nil || len(body) != 0 {
		t.Fatalf("空请求体解析失败: %v", err)
	}
	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	if _, err := readQueryBody(request); err == nil {
		t.Fatalf("非法请求体应报错")
	}

	query := mustQueryBody(t, `{
		"text": "value",
		"flag": true,
		"count": 7,
		"array": ["a", "b"],
		"csv": "a, b , ",
		"empty": "",
		"null": null,
		"bad": 123,
		"badInt": "x"
	}`)
	if query.str("text") != "value" || query.str("missing") != "" || query.str("bad") != "" {
		t.Fatalf("字符串解析错误")
	}
	if flag, err := query.boolField("flag"); err != nil || flag == nil || !*flag {
		t.Fatalf("布尔解析错误: %v", err)
	}
	if flag, err := query.boolField("missing"); err != nil || flag != nil {
		t.Fatalf("缺失布尔应返回 nil: %v", err)
	}
	if flag, err := query.boolField("null"); err != nil || flag != nil {
		t.Fatalf("null 布尔应返回 nil: %v", err)
	}
	if _, err := query.boolField("bad"); err == nil {
		t.Fatalf("非法布尔应报错")
	}
	if count, err := query.intField("count"); err != nil || count == nil || *count != 7 {
		t.Fatalf("整数解析错误: %v", err)
	}
	if count, err := query.intField("missing"); err != nil || count != nil {
		t.Fatalf("缺失整数应返回 nil: %v", err)
	}
	if _, err := query.intField("badInt"); err == nil {
		t.Fatalf("非法整数应报错")
	}
	if values, err := query.stringsField("array"); err != nil || len(values) != 2 || values[1] != "b" {
		t.Fatalf("数组解析错误: %v %v", values, err)
	}
	if values, err := query.stringsField("csv"); err != nil || len(values) != 2 || values[1] != "b" {
		t.Fatalf("逗号数组解析错误: %v %v", values, err)
	}
	if values, err := query.stringsField("empty"); err != nil || values != nil {
		t.Fatalf("空数组应返回 nil: %v %v", values, err)
	}
	if values, err := query.stringsField("missing"); err != nil || values != nil {
		t.Fatalf("缺失数组应返回 nil: %v %v", values, err)
	}
	if _, err := query.stringsField("bad"); err == nil {
		t.Fatalf("非法数组应报错")
	}

	times := mustQueryBody(t, `{
		"rfc": "2026-10-28T01:02:03Z",
		"nano": "2026-10-28T01:02:03.123456789Z",
		"offset": "2026-10-28T01:02:03.000+0800",
		"local": "2026-10-28T01:02:03",
		"bad": "not-a-time"
	}`)
	for _, key := range []string{"rfc", "nano", "offset", "local"} {
		if parsed, err := times.timeField(key); err != nil || parsed == nil {
			t.Fatalf("时间解析失败 %s: %v", key, err)
		}
	}
	if parsed, err := times.timeField("missing"); err != nil || parsed != nil {
		t.Fatalf("缺失时间应返回 nil: %v", err)
	}
	if _, err := times.timeField("bad"); err == nil {
		t.Fatalf("非法时间应报错")
	}
}

// TestQueryPagination 验证分页参数解析。
// @param t 测试上下文
func TestQueryPagination(t *testing.T) {
	start, size, sortField, order, err := mustQueryBody(t, `{}`).pagination()
	if err != nil || start != 0 || size != 10 || sortField != "" || order != "asc" {
		t.Fatalf("默认分页错误: %d %d %s %s %v", start, size, sortField, order, err)
	}
	start, size, sortField, order, err = mustQueryBody(t, `{"start":2,"size":2000,"sort":"id","order":"DESC"}`).pagination()
	if err != nil || start != 2 || size != 1000 || sortField != "id" || order != "desc" {
		t.Fatalf("分页解析错误: %d %d %s %s %v", start, size, sortField, order, err)
	}
	start, size, _, _, err = mustQueryBody(t, `{"start":-1,"size":-1}`).pagination()
	if err != nil || start != 0 || size != 10 {
		t.Fatalf("负分页应回落默认: %d %d %v", start, size, err)
	}
	if _, _, _, _, err := mustQueryBody(t, `{"start":"x"}`).pagination(); err == nil {
		t.Fatalf("非法 start 应报错")
	}
	if _, _, _, _, err := mustQueryBody(t, `{"size":"x"}`).pagination(); err == nil {
		t.Fatalf("非法 size 应报错")
	}
	if _, _, _, _, err := mustQueryBody(t, `{"order":"up"}`).pagination(); err == nil {
		t.Fatalf("非法排序方向应报错")
	}
}

// TestQueryCompareValues 验证排序值比较。
// @param t 测试上下文
func TestQueryCompareValues(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	cases := []struct {
		left  any
		right any
		want  int
	}{
		{nil, nil, 0},
		{nil, 1, -1},
		{1, nil, 1},
		{"a", "b", -1},
		{"b", "a", 1},
		{"a", "a", 0},
		{1, 2, -1},
		{int64(4), int64(3), 1},
		{3.5, 3.5, 0},
		{4.5, 3.5, 1},
		{true, false, 1},
		{false, true, -1},
		{true, true, 0},
		{now, later, -1},
		{later, now, 1},
		{now, now, 0},
		{&now, now, 0},
		{&now, &later, -1},
		{&later, &now, 1},
		{&now, nil, 1},
		{(*time.Time)(nil), 1, -1},
		{[]byte("b"), []byte("a"), 1},
		{struct{}{}, struct{}{}, 0},
	}
	for _, tc := range cases {
		if got := compareQueryValues(tc.left, tc.right); signInt(got) != tc.want {
			t.Fatalf("比较 %v/%v 期望 %d 实际 %d", tc.left, tc.right, tc.want, got)
		}
	}
	if queryNumeric(1) != 1 || queryNumeric(int64(2)) != 2 || queryNumeric(3.5) != 3.5 || queryNumeric("x") != 0 {
		t.Fatalf("数值转换错误")
	}
	if queryStringValue("x") != "x" || queryStringValue([]byte("y")) != "y" ||
		queryStringValue(now) == "" || queryStringValue(1) != "1" {
		t.Fatalf("字符串转换错误")
	}
	if compareQueryFloat(1, 2) != -1 || compareQueryFloat(2, 1) != 1 || compareQueryFloat(1, 1) != 0 {
		t.Fatalf("浮点比较错误")
	}
}

// signInt 返回整数符号。
// @param value 整数
// @return -1/0/1
func signInt(value int) int {
	if value < 0 {
		return -1
	}
	if value > 0 {
		return 1
	}
	return 0
}

// TestQueryMatchHelpers 验证匹配辅助函数。
// @param t 测试上下文
func TestQueryMatchHelpers(t *testing.T) {
	if !matchQueryAny("a", nil) || !matchQueryAny("a", []string{"a"}) || matchQueryAny("a", []string{"b"}) {
		t.Fatalf("集合匹配错误")
	}
	if !matchQueryLike("Approve", "") || !matchQueryLike("Approve", "%pro%") || matchQueryLike("Approve", "zzz") {
		t.Fatalf("模糊匹配错误")
	}
	now := time.Now()
	after, before := now.Add(-time.Hour), now.Add(time.Hour)
	if !matchQueryTime(now, &after, &before) || matchQueryTime(now, &before, nil) || matchQueryTime(now, nil, &after) {
		t.Fatalf("时间范围匹配错误")
	}
	if !matchQueryTimePointer(nil, nil, nil) || matchQueryTimePointer(nil, &after, nil) ||
		!matchQueryTimePointer(&now, &after, &before) || matchQueryTimePointer(&now, &before, nil) {
		t.Fatalf("可空时间匹配错误")
	}
	if !containsString([]string{"a"}, "a") || containsString([]string{"a"}, "b") {
		t.Fatalf("字符串包含判断错误")
	}
	if !containsAnyString([]string{"a"}, []string{"b", "a"}) || containsAnyString([]string{"a"}, []string{"b"}) {
		t.Fatalf("集合包含判断错误")
	}
}

// TestQuerySortKeys 验证全部排序字段访问器。
// @param t 测试上下文
func TestQuerySortKeys(t *testing.T) {
	instance := api.ProcessInstance{ID: "i", ProcessDefinitionID: "d", ProcessDefinitionKey: "k", BusinessKey: "b", Name: "n", StartTime: time.Now(), EndTime: nil}
	instanceFields := []string{"id", "processDefinitionId", "processDefinitionKey", "businessKey", "name", "startTime", "endTime"}
	for _, field := range instanceFields {
		key, ok := processInstanceSortKey(field)
		if !ok {
			t.Fatalf("实例排序字段缺失: %s", field)
		}
		_ = key(instance)
	}
	if _, ok := processInstanceSortKey("missing"); ok {
		t.Fatalf("未知实例排序字段应不支持")
	}

	execution := api.ExecutionInfo{ID: "e", ProcessInstanceID: "i", ProcessDefinitionID: "d", ActivityID: "a"}
	for _, field := range []string{"id", "processInstanceId", "processDefinitionId", "activityId"} {
		key, ok := executionSortKey(field)
		if !ok {
			t.Fatalf("执行排序字段缺失: %s", field)
		}
		_ = key(execution)
	}
	if _, ok := executionSortKey("missing"); ok {
		t.Fatalf("未知执行排序字段应不支持")
	}

	task := api.Task{ID: "t", Name: "n", Description: "d", Assignee: "a", Owner: "o", Priority: 1, CreateTime: time.Now(), DueDate: nil}
	for _, field := range []string{"id", "name", "description", "assignee", "owner", "priority", "createTime", "dueDate"} {
		key, ok := taskSortKey(field)
		if !ok {
			t.Fatalf("任务排序字段缺失: %s", field)
		}
		_ = key(task)
	}
	if _, ok := taskSortKey("missing"); ok {
		t.Fatalf("未知任务排序字段应不支持")
	}

	historicProcess := api.HistoricProcessInstance{ID: "i", ProcessDefinitionKey: "k", BusinessKey: "b", Name: "n", StartTime: time.Now()}
	for _, field := range []string{"id", "processDefinitionKey", "businessKey", "name", "startTime", "endTime"} {
		key, ok := historicProcessSortKey(field)
		if !ok {
			t.Fatalf("历史实例排序字段缺失: %s", field)
		}
		_ = key(historicProcess)
	}
	if _, ok := historicProcessSortKey("missing"); ok {
		t.Fatalf("未知历史实例排序字段应不支持")
	}

	historicTask := api.HistoricTaskInstance{ID: "t", Name: "n", Assignee: "a", StartTime: time.Now()}
	for _, field := range []string{"id", "name", "assignee", "startTime", "endTime"} {
		key, ok := historicTaskSortKey(field)
		if !ok {
			t.Fatalf("历史任务排序字段缺失: %s", field)
		}
		_ = key(historicTask)
	}
	if _, ok := historicTaskSortKey("missing"); ok {
		t.Fatalf("未知历史任务排序字段应不支持")
	}

	historicActivity := api.HistoricActivityInstance{ID: "a", ActivityID: "a", ActivityType: "userTask", ActivityName: "n", StartTime: time.Now()}
	for _, field := range []string{"id", "activityId", "activityType", "activityName", "startTime", "endTime"} {
		key, ok := historicActivitySortKey(field)
		if !ok {
			t.Fatalf("历史活动排序字段缺失: %s", field)
		}
		_ = key(historicActivity)
	}
	if _, ok := historicActivitySortKey("missing"); ok {
		t.Fatalf("未知历史活动排序字段应不支持")
	}
}

// TestQuerySortAndWrite 验证排序与分页输出辅助。
// @param t 测试上下文
func TestQuerySortAndWrite(t *testing.T) {
	items := []api.ProcessInstance{{ID: "b"}, {ID: "a"}}
	if err := sortQueryItems(items, "", "asc", processInstanceSortKey); err != nil || items[0].ID != "b" {
		t.Fatalf("空排序不应改变顺序: %v", err)
	}
	if err := sortQueryItems(items, "id", "asc", processInstanceSortKey); err != nil || items[0].ID != "a" {
		t.Fatalf("升序排序失败: %v", err)
	}
	if err := sortQueryItems(items, "id", "desc", processInstanceSortKey); err != nil || items[0].ID != "b" {
		t.Fatalf("降序排序失败: %v", err)
	}
	if err := sortQueryItems(items, "missing", "asc", processInstanceSortKey); err == nil {
		t.Fatalf("未知排序字段应报错")
	}

	recorder := httptest.NewRecorder()
	writeQueryPage(recorder, []api.ProcessInstance{{ID: "a"}}, 5, 10, "", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"data":[]`) {
		t.Fatalf("越界分页输出错误: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	writeQueryPage(recorder, []api.ProcessInstance{{ID: "a"}, {ID: "b"}}, 0, 1, "id", "desc")
	body := recorder.Body.String()
	if !strings.Contains(body, `"total":2`) || !strings.Contains(body, `"sort":"id"`) || !strings.Contains(body, `"order":"desc"`) {
		t.Fatalf("分页输出错误: %s", body)
	}
}

// TestQueryProcessFilters 验证流程实例过滤条件。
// @param t 测试上下文
func TestQueryProcessFilters(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	items := []api.ProcessInstance{{
		ID: "i1", ProcessDefinitionID: "d1", ProcessDefinitionKey: "k1", BusinessKey: "bk1",
		Name: "Order", TenantID: "t1", StartTime: now, Ended: false, Suspended: true,
	}}
	cases := []struct {
		body string
		want int
	}{
		{`{"id":"i1"}`, 1}, {`{"id":"x"}`, 0},
		{`{"processDefinitionId":"d1"}`, 1}, {`{"processDefinitionId":"x"}`, 0},
		{`{"processDefinitionKey":"k1"}`, 1}, {`{"processDefinitionKey":"x"}`, 0},
		{`{"processDefinitionKeyIn":["k1"]}`, 1}, {`{"processDefinitionKeyIn":["k2"]}`, 0},
		{`{"businessKey":"bk1"}`, 1}, {`{"businessKey":"x"}`, 0},
		{`{"businessKeyLike":"%BK%"}`, 1}, {`{"businessKeyLike":"zzz"}`, 0},
		{`{"name":"Order"}`, 1}, {`{"name":"x"}`, 0},
		{`{"nameLike":"%ord%"}`, 1}, {`{"nameLike":"zzz"}`, 0},
		{`{"tenantId":"t1"}`, 1}, {`{"tenantId":"t2"}`, 0},
		{`{"startedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"startedAfter":"2026-11-01T00:00:00Z"}`, 0},
		{`{"startedBefore":"2026-11-01T00:00:00Z"}`, 1}, {`{"startedBefore":"2026-09-01T00:00:00Z"}`, 0},
		{`{"finished":false}`, 1}, {`{"finished":true}`, 0},
		{`{"suspended":true}`, 1}, {`{"suspended":false}`, 0},
	}
	for _, tc := range cases {
		got, err := filterQueryProcessInstances(items, mustQueryBody(t, tc.body), "")
		if err != nil || len(got) != tc.want {
			t.Fatalf("过滤 %s 期望 %d 实际 %d 错误 %v", tc.body, tc.want, len(got), err)
		}
	}
	for _, body := range []string{`{"startedAfter":"bad"}`, `{"startedBefore":"bad"}`, `{"finished":"x"}`, `{"suspended":"x"}`, `{"processDefinitionKeyIn":123}`} {
		if _, err := filterQueryProcessInstances(items, mustQueryBody(t, body), ""); err == nil {
			t.Fatalf("非法查询体应报错: %s", body)
		}
	}
}

// TestQueryExecutionFilters 验证执行过滤条件。
// @param t 测试上下文
func TestQueryExecutionFilters(t *testing.T) {
	items := []api.ExecutionInfo{{ID: "e1", ProcessInstanceID: "i1", ProcessDefinitionID: "d1", ActivityID: "a1", Active: true, Ended: false, Waiting: true}}
	cases := []struct {
		body string
		want int
	}{
		{`{"id":"e1"}`, 1}, {`{"id":"x"}`, 0},
		{`{"processInstanceId":"i1"}`, 1}, {`{"processInstanceId":"x"}`, 0},
		{`{"processDefinitionId":"d1"}`, 1}, {`{"processDefinitionId":"x"}`, 0},
		{`{"activityId":"a1"}`, 1}, {`{"activityId":"x"}`, 0},
		{`{"ended":false}`, 1}, {`{"ended":true}`, 0},
		{`{"active":true}`, 1}, {`{"active":false}`, 0},
		{`{"waiting":true}`, 1}, {`{"waiting":false}`, 0},
	}
	for _, tc := range cases {
		got, err := filterQueryExecutions(items, mustQueryBody(t, tc.body))
		if err != nil || len(got) != tc.want {
			t.Fatalf("执行过滤 %s 期望 %d 实际 %d 错误 %v", tc.body, tc.want, len(got), err)
		}
	}
	for _, body := range []string{`{"ended":"x"}`, `{"active":"x"}`, `{"waiting":"x"}`} {
		if _, err := filterQueryExecutions(items, mustQueryBody(t, body)); err == nil {
			t.Fatalf("非法执行查询体应报错: %s", body)
		}
	}
}

// TestQueryTaskFilters 验证任务过滤条件。
// @param t 测试上下文
func TestQueryTaskFilters(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	items := []api.Task{{
		ID: "t1", Name: "Approve", Description: "desc", Assignee: "alice", Owner: "bob",
		Priority: 50, CreateTime: now, DueDate: &now, CandidateUsers: []string{"carol"},
		CandidateGroups: []string{"g1"}, ProcessInstanceID: "i1", ProcessDefinitionID: "d1",
		TaskDefinitionKey: "task",
	}}
	lookup := &taskQueryLookup{
		tenants:        map[string]string{"i1": "t1"},
		businessKeys:   map[string]string{"i1": "bk1"},
		definitionKeys: map[string]string{"d1": "k1"},
	}
	cases := []struct {
		body string
		want int
	}{
		{`{"id":"t1"}`, 1}, {`{"taskId":"t1"}`, 1}, {`{"id":"x"}`, 0},
		{`{"name":"Approve"}`, 1}, {`{"name":"x"}`, 0},
		{`{"nameLike":"%prove%"}`, 1}, {`{"nameLike":"zzz"}`, 0},
		{`{"description":"desc"}`, 1}, {`{"description":"x"}`, 0},
		{`{"descriptionLike":"%esc%"}`, 1}, {`{"descriptionLike":"zzz"}`, 0},
		{`{"assignee":"alice"}`, 1}, {`{"assignee":"x"}`, 0},
		{`{"assigneeLike":"%lic%"}`, 1}, {`{"assigneeLike":"zzz"}`, 0},
		{`{"owner":"bob"}`, 1}, {`{"owner":"x"}`, 0},
		{`{"ownerLike":"%ob%"}`, 1}, {`{"ownerLike":"zzz"}`, 0},
		{`{"processDefinitionId":"d1"}`, 1}, {`{"processDefinitionId":"x"}`, 0},
		{`{"processInstanceId":"i1"}`, 1}, {`{"processInstanceId":"x"}`, 0},
		{`{"taskDefinitionKey":"task"}`, 1}, {`{"taskDefinitionKey":"x"}`, 0},
		{`{"processDefinitionKey":"k1"}`, 1}, {`{"processDefinitionKey":"x"}`, 0},
		{`{"processInstanceBusinessKey":"bk1"}`, 1}, {`{"processInstanceBusinessKey":"x"}`, 0},
		{`{"tenantId":"t1"}`, 1}, {`{"tenantId":"t2"}`, 0},
		{`{"unassigned":false}`, 1}, {`{"unassigned":true}`, 0},
		{`{"finished":false}`, 1}, {`{"finished":true}`, 0},
		{`{"priority":50}`, 1}, {`{"priority":51}`, 0},
		{`{"minimumPriority":51}`, 0}, {`{"minimumPriority":50}`, 1},
		{`{"maximumPriority":49}`, 0}, {`{"maximumPriority":50}`, 1},
		{`{"createdAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"createdAfter":"2026-11-01T00:00:00Z"}`, 0},
		{`{"createdBefore":"2026-11-01T00:00:00Z"}`, 1}, {`{"createdBefore":"2026-09-01T00:00:00Z"}`, 0},
		{`{"dueAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"dueAfter":"2026-11-01T00:00:00Z"}`, 0},
		{`{"dueBefore":"2026-11-01T00:00:00Z"}`, 1}, {`{"dueBefore":"2026-09-01T00:00:00Z"}`, 0},
		{`{"candidateUser":"carol"}`, 0}, {`{"candidateUser":"dave"}`, 0},
		{`{"candidateGroup":"g1"}`, 1}, {`{"candidateGroup":"g2"}`, 0},
		{`{"candidateGroupIn":["g1"]}`, 1}, {`{"candidateGroupIn":["g2"]}`, 0},
		{`{"involvedUser":"bob"}`, 1}, {`{"involvedUser":"alice"}`, 1}, {`{"involvedUser":"dave"}`, 0},
	}
	for _, tc := range cases {
		got, err := filterQueryTasks(items, mustQueryBody(t, tc.body), "", lookup)
		if err != nil || len(got) != tc.want {
			t.Fatalf("任务过滤 %s 期望 %d 实际 %d 错误 %v", tc.body, tc.want, len(got), err)
		}
	}
	unassigned := []api.Task{{ID: "t2", ProcessInstanceID: "i1", ProcessDefinitionID: "d1", CandidateUsers: []string{"carol"}}}
	if got, _ := filterQueryTasks(unassigned, mustQueryBody(t, `{"candidateUser":"carol"}`), "", lookup); len(got) != 1 {
		t.Fatalf("候选人查询应命中未签收任务")
	}
	for _, body := range []string{`{"unassigned":"x"}`, `{"finished":"x"}`, `{"priority":"x"}`, `{"minimumPriority":"x"}`, `{"maximumPriority":"x"}`, `{"createdAfter":"bad"}`, `{"dueBefore":"bad"}`} {
		if _, err := filterQueryTasks(items, mustQueryBody(t, body), "", lookup); err == nil {
			t.Fatalf("非法任务查询体应报错: %s", body)
		}
	}
}

// TestQueryFilterErrors 验证过滤条件的时间字段错误。
// @param t 测试上下文
func TestQueryFilterErrors(t *testing.T) {
	lookup := &taskQueryLookup{tenants: map[string]string{}, businessKeys: map[string]string{}, definitionKeys: map[string]string{}}
	taskBodies := []string{`{"createdBefore":"bad"}`, `{"dueAfter":"bad"}`, `{"dueBefore":"bad"}`, `{"candidateGroupIn":123}`}
	for _, body := range taskBodies {
		if _, err := filterQueryTasks(nil, mustQueryBody(t, body), "", lookup); err == nil {
			t.Fatalf("非法任务查询体应报错: %s", body)
		}
	}
	historicLookup := &historicProcessLookup{tenants: map[string]string{}, definitionKeys: map[string]string{}}
	historicBodies := []string{`{"endedAfter":"bad"}`, `{"endedBefore":"bad"}`}
	for _, body := range historicBodies {
		if _, err := filterQueryHistoricProcesses(nil, mustQueryBody(t, body), ""); err == nil {
			t.Fatalf("非法历史实例查询体应报错: %s", body)
		}
		if _, err := filterQueryHistoricTasks(nil, mustQueryBody(t, body), "", historicLookup); err == nil {
			t.Fatalf("非法历史任务查询体应报错: %s", body)
		}
		if _, err := filterQueryHistoricActivities(nil, mustQueryBody(t, body), "", historicLookup); err == nil {
			t.Fatalf("非法历史活动查询体应报错: %s", body)
		}
	}
}

// TestQueryHistoricFilters 验证历史实例、任务与活动过滤。
// @param t 测试上下文
func TestQueryHistoricFilters(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	instance := api.HistoricProcessInstance{
		ID: "i1", ProcessDefinitionID: "d1", ProcessDefinitionKey: "k1", BusinessKey: "bk1",
		Name: "Order", TenantID: "t1", StartTime: now, EndTime: &now, DeleteReason: "done", Ended: true,
	}
	instanceCases := []struct {
		body string
		want int
	}{
		{`{"id":"i1"}`, 1}, {`{"id":"x"}`, 0},
		{`{"processDefinitionId":"d1"}`, 1}, {`{"processDefinitionId":"x"}`, 0},
		{`{"processDefinitionKey":"k1"}`, 1}, {`{"processDefinitionKey":"x"}`, 0},
		{`{"processDefinitionKeyIn":["k1"]}`, 1}, {`{"processDefinitionKeyIn":["k2"]}`, 0},
		{`{"businessKey":"bk1"}`, 1}, {`{"businessKey":"x"}`, 0},
		{`{"businessKeyLike":"%BK%"}`, 1}, {`{"businessKeyLike":"zzz"}`, 0},
		{`{"name":"Order"}`, 1}, {`{"name":"x"}`, 0},
		{`{"nameLike":"%ord%"}`, 1}, {`{"nameLike":"zzz"}`, 0},
		{`{"deleteReason":"done"}`, 1}, {`{"deleteReason":"x"}`, 0},
		{`{"tenantId":"t1"}`, 1}, {`{"tenantId":"t2"}`, 0},
		{`{"finished":true}`, 1}, {`{"finished":false}`, 0},
		{`{"startedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"startedAfter":"2026-11-01T00:00:00Z"}`, 0},
		{`{"startedBefore":"2026-11-01T00:00:00Z"}`, 1}, {`{"startedBefore":"2026-09-01T00:00:00Z"}`, 0},
		{`{"endedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"endedAfter":"2026-11-01T00:00:00Z"}`, 0},
		{`{"endedBefore":"2026-11-01T00:00:00Z"}`, 1}, {`{"endedBefore":"2026-09-01T00:00:00Z"}`, 0},
	}
	for _, tc := range instanceCases {
		got, err := filterQueryHistoricProcesses([]api.HistoricProcessInstance{instance}, mustQueryBody(t, tc.body), "")
		if err != nil || len(got) != tc.want {
			t.Fatalf("历史实例过滤 %s 期望 %d 实际 %d 错误 %v", tc.body, tc.want, len(got), err)
		}
	}

	lookup := &historicProcessLookup{tenants: map[string]string{"i1": "t1"}, definitionKeys: map[string]string{"i1": "k1"}}
	historicTask := api.HistoricTaskInstance{ID: "t1", Name: "Approve", Assignee: "alice", TaskDefinitionKey: "task", ProcessInstanceID: "i1", StartTime: now, EndTime: &now}
	taskCases := []struct {
		body string
		want int
	}{
		{`{"id":"t1"}`, 1}, {`{"taskId":"t1"}`, 1}, {`{"id":"x"}`, 0},
		{`{"name":"Approve"}`, 1}, {`{"taskName":"Approve"}`, 1}, {`{"name":"x"}`, 0},
		{`{"nameLike":"%prove%"}`, 1}, {`{"taskNameLike":"%prove%"}`, 1}, {`{"nameLike":"zzz"}`, 0},
		{`{"assignee":"alice"}`, 1}, {`{"taskAssignee":"alice"}`, 1}, {`{"assignee":"x"}`, 0},
		{`{"assigneeLike":"%lic%"}`, 1}, {`{"taskAssigneeLike":"%lic%"}`, 1}, {`{"assigneeLike":"zzz"}`, 0},
		{`{"taskDefinitionKey":"task"}`, 1}, {`{"taskDefinitionKey":"x"}`, 0},
		{`{"taskDefinitionKeyLike":"%tas%"}`, 1}, {`{"taskDefinitionKeyLike":"zzz"}`, 0},
		{`{"processInstanceId":"i1"}`, 1}, {`{"processInstanceId":"x"}`, 0},
		{`{"tenantId":"t1"}`, 1}, {`{"tenantId":"t2"}`, 0},
		{`{"processDefinitionKey":"k1"}`, 1}, {`{"processDefinitionKey":"x"}`, 0},
		{`{"finished":true}`, 1}, {`{"finished":false}`, 0},
		{`{"startedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"startedBefore":"2026-09-01T00:00:00Z"}`, 0},
		{`{"endedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"endedBefore":"2026-09-01T00:00:00Z"}`, 0},
	}
	for _, tc := range taskCases {
		got, err := filterQueryHistoricTasks([]api.HistoricTaskInstance{historicTask}, mustQueryBody(t, tc.body), "", lookup)
		if err != nil || len(got) != tc.want {
			t.Fatalf("历史任务过滤 %s 期望 %d 实际 %d 错误 %v", tc.body, tc.want, len(got), err)
		}
	}

	historicActivity := api.HistoricActivityInstance{ID: "a1", ProcessInstanceID: "i1", ActivityID: "task", ActivityName: "Approve", ActivityType: "userTask", StartTime: now, EndTime: &now}
	activityCases := []struct {
		body string
		want int
	}{
		{`{"id":"a1"}`, 1}, {`{"id":"x"}`, 0},
		{`{"processInstanceId":"i1"}`, 1}, {`{"processInstanceId":"x"}`, 0},
		{`{"activityId":"task"}`, 1}, {`{"activityId":"x"}`, 0},
		{`{"activityType":"userTask"}`, 1}, {`{"activityType":"x"}`, 0},
		{`{"activityName":"Approve"}`, 1}, {`{"name":"Approve"}`, 1}, {`{"activityName":"x"}`, 0},
		{`{"activityNameLike":"%prove%"}`, 1}, {`{"activityNameLike":"zzz"}`, 0},
		{`{"tenantId":"t1"}`, 1}, {`{"tenantId":"t2"}`, 0},
		{`{"processDefinitionKey":"k1"}`, 1}, {`{"processDefinitionKey":"x"}`, 0},
		{`{"finished":true}`, 1}, {`{"finished":false}`, 0},
		{`{"startedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"startedBefore":"2026-09-01T00:00:00Z"}`, 0},
		{`{"endedAfter":"2026-10-01T00:00:00Z"}`, 1}, {`{"endedBefore":"2026-09-01T00:00:00Z"}`, 0},
	}
	for _, tc := range activityCases {
		got, err := filterQueryHistoricActivities([]api.HistoricActivityInstance{historicActivity}, mustQueryBody(t, tc.body), "", lookup)
		if err != nil || len(got) != tc.want {
			t.Fatalf("历史活动过滤 %s 期望 %d 实际 %d 错误 %v", tc.body, tc.want, len(got), err)
		}
	}

	for _, body := range []string{`{"finished":"x"}`, `{"startedAfter":"bad"}`, `{"processDefinitionKeyIn":123}`} {
		if _, err := filterQueryHistoricProcesses([]api.HistoricProcessInstance{instance}, mustQueryBody(t, body), ""); err == nil {
			t.Fatalf("非法历史实例查询体应报错: %s", body)
		}
	}
	for _, body := range []string{`{"finished":"x"}`, `{"startedBefore":"bad"}`} {
		if _, err := filterQueryHistoricTasks([]api.HistoricTaskInstance{historicTask}, mustQueryBody(t, body), "", lookup); err == nil {
			t.Fatalf("非法历史任务查询体应报错: %s", body)
		}
		if _, err := filterQueryHistoricActivities([]api.HistoricActivityInstance{historicActivity}, mustQueryBody(t, body), "", lookup); err == nil {
			t.Fatalf("非法历史活动查询体应报错: %s", body)
		}
	}
}
