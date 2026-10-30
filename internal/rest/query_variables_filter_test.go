package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
)

// variableFilter 构造单个变量过滤条件。
// @param name 变量名
// @param value 期望值
// @param operation 操作符，省略或为空时使用 equals
// @return 变量条件
func variableFilter(name string, value any, operation ...string) map[string]any {
	condition := map[string]any{"name": name, "value": value}
	if len(operation) > 0 && operation[0] != "" {
		condition["operation"] = operation[0]
	}
	return condition
}

// withVariables 构造携带变量过滤条件的查询体。
// @param conditions 变量条件
// @return 查询体
func withVariables(conditions ...map[string]any) map[string]any {
	return map[string]any{"variables": conditions}
}

// startVariableQueryInstance 启动带变量的测试流程实例。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param businessKey 业务键
// @param values 变量集合
// @return 流程实例 ID
func startVariableQueryInstance(t *testing.T, serverURL, businessKey string, values map[string]any) string {
	t.Helper()
	return startKeyedVariableQueryInstance(t, serverURL, "rest", businessKey, values)
}

// startKeyedVariableQueryInstance 启动指定流程定义的带变量测试实例。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param key 流程定义 Key
// @param businessKey 业务键
// @param values 变量集合
// @return 流程实例 ID
func startKeyedVariableQueryInstance(t *testing.T, serverURL, key, businessKey string, values map[string]any) string {
	t.Helper()
	variables := make([]map[string]any, 0, len(values)+1)
	variables = append(variables, map[string]any{"name": "initiator", "value": "alice"})
	for name, value := range values {
		variables = append(variables, map[string]any{"name": name, "value": value})
	}
	response := postJSON(t, serverURL+"/service/runtime/process-instances", map[string]any{
		"processDefinitionKey": key,
		"businessKey":          businessKey,
		"variables":            variables,
	})
	defer response.Body.Close()
	var instance map[string]any
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("解析启动响应失败: %v", err)
	}
	id, _ := instance["id"].(string)
	if response.StatusCode != http.StatusCreated || id == "" {
		t.Fatalf("启动带变量实例失败: %d", response.StatusCode)
	}
	return id
}

// variableQueryTaskID 查询实例的首个任务 ID。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param instanceID 流程实例 ID
// @return 任务 ID
func variableQueryTaskID(t *testing.T, serverURL, instanceID string) string {
	t.Helper()
	response, err := http.Get(serverURL + "/service/runtime/tasks?processInstanceId=" + instanceID)
	if err != nil {
		t.Fatalf("查询任务失败: %v", err)
	}
	page := decodeQueryPage[map[string]any](t, response)
	if len(page.Data) == 0 {
		t.Fatalf("实例没有待办任务")
	}
	id, _ := page.Data[0]["id"].(string)
	if id == "" {
		t.Fatalf("任务 ID 为空")
	}
	return id
}

// assertVariableQueryTotal 断言变量查询命中数量。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param path 查询路径
// @param body 查询体
// @param want 期望命中数量
// @return 查询分页结果
func assertVariableQueryTotal(t *testing.T, serverURL, path string, body map[string]any, want int) queryPageResult[map[string]any] {
	t.Helper()
	response := postQuery(t, serverURL, path, body, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("变量查询应成功: %d %+v", response.StatusCode, body)
	}
	page := decodeQueryPage[map[string]any](t, response)
	if page.Total != want {
		t.Fatalf("变量查询 %+v 期望 %d 实际 %d", body, want, page.Total)
	}
	return page
}

// assertVariableQueryBadRequest 断言变量查询返回 400。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param path 查询路径
// @param body 查询体
func assertVariableQueryBadRequest(t *testing.T, serverURL, path string, body any) {
	t.Helper()
	response := postQuery(t, serverURL, path, body, "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("非法变量条件应 400: %d %+v", response.StatusCode, body)
	}
	_ = response.Body.Close()
}

// TestParseQueryVariableConditions 验证变量过滤条件解析。
// @param t 测试上下文
func TestParseQueryVariableConditions(t *testing.T) {
	emptyBodies := []string{`{}`, `{"variables":null}`, `{"variables":[]}`}
	for _, raw := range emptyBodies {
		conditions, err := parseQueryVariableConditions(mustQueryBody(t, raw))
		if err != nil || len(conditions) != 0 {
			t.Fatalf("空变量条件解析失败 %s: %v", raw, err)
		}
	}

	conditions, err := parseQueryVariableConditions(mustQueryBody(t, `{"variables":[{"name":" amount ","value":100},{"name":"status","value":"open","operation":"NotEquals"},{"name":"title","operation":"LIKE"},{"name":"score","value":10,"operation":"GREATERTHAN"},{"name":"flag","value":true}]}`))
	if err != nil || len(conditions) != 5 {
		t.Fatalf("合法变量条件解析失败: %v %d", err, len(conditions))
	}
	if conditions[0].Name != "amount" || conditions[0].Value != float64(100) ||
		conditions[0].Operation != variableOperationEquals {
		t.Fatalf("默认操作符解析错误: %+v", conditions[0])
	}
	if conditions[1].Operation != variableOperationNotEquals || conditions[2].Operation != variableOperationLike ||
		conditions[2].Value != nil || conditions[3].Operation != variableOperationGreaterThan {
		t.Fatalf("操作符规范化错误: %+v", conditions)
	}
	if conditions[4].Value != true {
		t.Fatalf("布尔变量值解析错误: %+v", conditions[4])
	}

	badBodies := []string{
		`{"variables":123}`,
		`{"variables":["x"]}`,
		`{"variables":[{"value":1}]}`,
		`{"variables":[{"name":"a","operation":"between"}]}`,
	}
	for _, raw := range badBodies {
		if _, err := parseQueryVariableConditions(mustQueryBody(t, raw)); err == nil {
			t.Fatalf("非法变量条件应报错: %s", raw)
		}
	}
}

// TestParseVariableOperation 验证操作符解析与大小写兼容。
// @param t 测试上下文
func TestParseVariableOperation(t *testing.T) {
	cases := map[string]variableOperation{
		"":                   variableOperationEquals,
		"equals":             variableOperationEquals,
		" EQUALS ":           variableOperationEquals,
		"notEquals":          variableOperationNotEquals,
		"NOTEQUALS":          variableOperationNotEquals,
		"Like":               variableOperationLike,
		"greaterThan":        variableOperationGreaterThan,
		"GreaterThanOrEqual": variableOperationGreaterThanOrEqual,
		"lessThan":           variableOperationLessThan,
		"LESSTHANOREQUAL":    variableOperationLessThanOrEqual,
	}
	for raw, want := range cases {
		got, err := parseVariableOperation(raw)
		if err != nil || got != want {
			t.Fatalf("操作符解析错误 %q: %v %q", raw, err, got)
		}
	}
	if operation, err := parseVariableOperation("between"); err == nil || operation != "" {
		t.Fatalf("非法操作符应报错")
	}
}

// TestVariableValueHelpers 验证变量值辅助转换。
// @param t 测试上下文
func TestVariableValueHelpers(t *testing.T) {
	now := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	if variableStringValue(nil) != "" || variableStringValue("x") != "x" ||
		variableStringValue([]byte("y")) != "y" || variableStringValue(now) == "" ||
		variableStringValue(&now) == "" || variableStringValue((*time.Time)(nil)) != "" ||
		variableStringValue(json.Number("12")) != "12" || variableStringValue(true) != "true" ||
		variableStringValue(3) != "3" {
		t.Fatalf("变量字符串转换错误")
	}

	numbers := []any{int(1), int8(1), int16(1), int32(1), int64(1), uint(1), uint8(1),
		uint16(1), uint32(1), uint64(1), float32(1), float64(1), json.Number("1")}
	for _, value := range numbers {
		if converted, ok := variableNumberValue(value); !ok || converted != 1 {
			t.Fatalf("数值转换错误: %T %v", value, value)
		}
	}
	if _, ok := variableNumberValue(json.Number("bad")); ok {
		t.Fatalf("非法 JSON 数值应返回 false")
	}
	if _, ok := variableNumberValue("1"); ok {
		t.Fatalf("字符串不应视为数值")
	}

	timeCases := []struct {
		value any
		ok    bool
	}{
		{now, true},
		{&now, true},
		{(*time.Time)(nil), false},
		{"2026-10-28T01:02:03Z", true},
		{"2026-10-28T01:02:03.123+0800", true},
		{"2026-10-28T01:02:03", true},
		{"2026-10-28", true},
		{[]byte("2026-10-28T01:02:03Z"), true},
		{"", false},
		{"not-a-time", false},
		{1, false},
	}
	for _, tc := range timeCases {
		if _, ok := variableTimeValue(tc.value); ok != tc.ok {
			t.Fatalf("时间转换错误 %v: 期望 %v", tc.value, tc.ok)
		}
	}
	if _, ok := parseVariableTime(" "); ok {
		t.Fatalf("空白时间应解析失败")
	}
}

// TestCompareVariableValues 验证变量值比较。
// @param t 测试上下文
func TestCompareVariableValues(t *testing.T) {
	now := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	later := now.Add(time.Hour)
	cases := []struct {
		left  any
		right any
		want  int
	}{
		{nil, nil, 0},
		{nil, 1, -1},
		{1, nil, 1},
		{int8(1), float64(2), -1},
		{int16(2), float64(2), 0},
		{int32(3), float64(2), 1},
		{uint(2), float64(2), 0},
		{uint8(2), float64(3), -1},
		{uint16(2), float64(1), 1},
		{uint32(2), float64(2), 0},
		{uint64(2), float64(3), -1},
		{float32(2), float64(2), 0},
		{json.Number("3"), float64(3), 0},
		{now, later, -1},
		{later, now, 1},
		{now, "2026-10-28T01:02:03Z", 0},
		{[]byte("2026-10-28T01:02:03Z"), "2026-10-28T01:02:03Z", 0},
		{"a", "b", -1},
		{"b", "a", 1},
		{"a", "a", 0},
		{true, false, 1},
		{false, true, -1},
	}
	for _, tc := range cases {
		if got := compareVariableValues(tc.left, tc.right); signInt(got) != tc.want {
			t.Fatalf("比较 %v/%v 期望 %d 实际 %d", tc.left, tc.right, tc.want, got)
		}
	}
}

// TestMatchVariableConditions 验证变量条件匹配语义。
// @param t 测试上下文
func TestMatchVariableConditions(t *testing.T) {
	now := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	variables := map[string]any{
		"amount":        float64(100),
		"count":         int(10),
		"status":        "open",
		"title":         "Urgent request",
		"enabled":       true,
		"when":          now,
		"pointerWhen":   &now,
		"empty":         nil,
		"numericString": "100",
		"number":        json.Number("42"),
		"bytes":         []byte("urgent"),
	}
	cases := []struct {
		name      string
		condition variableCondition
		want      bool
	}{
		{"数值等于", variableCondition{Name: "amount", Value: float64(100), Operation: variableOperationEquals}, true},
		{"整数等于", variableCondition{Name: "count", Value: float64(10), Operation: variableOperationEquals}, true},
		{"字符串等于", variableCondition{Name: "status", Value: "open", Operation: variableOperationEquals}, true},
		{"布尔等于", variableCondition{Name: "enabled", Value: true, Operation: variableOperationEquals}, true},
		{"布尔字符串等于", variableCondition{Name: "enabled", Value: "true", Operation: variableOperationEquals}, true},
		{"时间等于", variableCondition{Name: "when", Value: "2026-10-28T01:02:03Z", Operation: variableOperationEquals}, true},
		{"时间指针等于", variableCondition{Name: "pointerWhen", Value: now, Operation: variableOperationEquals}, true},
		{"空值等于", variableCondition{Name: "empty", Value: nil, Operation: variableOperationEquals}, true},
		{"字节等于", variableCondition{Name: "bytes", Value: "urgent", Operation: variableOperationEquals}, true},
		{"JSON 数值等于", variableCondition{Name: "number", Value: json.Number("42"), Operation: variableOperationEquals}, true},
		{"不等于", variableCondition{Name: "status", Value: "closed", Operation: variableOperationNotEquals}, true},
		{"不等于相同值", variableCondition{Name: "status", Value: "open", Operation: variableOperationNotEquals}, false},
		{"模糊匹配", variableCondition{Name: "title", Value: "%URGENT%", Operation: variableOperationLike}, true},
		{"字节模糊匹配", variableCondition{Name: "bytes", Value: "%urge%", Operation: variableOperationLike}, true},
		{"模糊匹配未命中", variableCondition{Name: "title", Value: "zzz", Operation: variableOperationLike}, false},
		{"模糊匹配空值", variableCondition{Name: "title", Value: nil, Operation: variableOperationLike}, false},
		{"大于", variableCondition{Name: "amount", Value: float64(50), Operation: variableOperationGreaterThan}, true},
		{"不大于", variableCondition{Name: "amount", Value: float64(100), Operation: variableOperationGreaterThan}, false},
		{"大于等于", variableCondition{Name: "amount", Value: float64(100), Operation: variableOperationGreaterThanOrEqual}, true},
		{"小于", variableCondition{Name: "count", Value: float64(20), Operation: variableOperationLessThan}, true},
		{"小于等于", variableCondition{Name: "count", Value: float64(10), Operation: variableOperationLessThanOrEqual}, true},
		{"时间大于", variableCondition{Name: "when", Value: "2026-10-01T00:00:00Z", Operation: variableOperationGreaterThan}, true},
		{"时间小于", variableCondition{Name: "when", Value: "2026-11-01T00:00:00Z", Operation: variableOperationLessThan}, true},
		{"未知操作符", variableCondition{Name: "amount", Value: float64(100), Operation: variableOperation("between")}, false},
	}
	for _, tc := range cases {
		if got := matchVariableCondition(variables[tc.condition.Name], tc.condition); got != tc.want {
			t.Fatalf("%s 期望 %v 实际 %v", tc.name, tc.want, got)
		}
	}

	all := []variableCondition{
		{Name: "status", Value: "open", Operation: variableOperationEquals},
		{Name: "amount", Value: float64(50), Operation: variableOperationGreaterThan},
	}
	if !matchVariableConditions(all, variables) {
		t.Fatalf("多条件 AND 应命中")
	}
	if matchVariableConditions(append(all, variableCondition{Name: "missing", Value: 1}), variables) {
		t.Fatalf("缺失变量应不满足 AND")
	}
	if !matchVariableConditions(nil, variables) {
		t.Fatalf("空条件应视为满足")
	}
}

// TestHistoricVariableSnapshot 验证历史变量快照合并。
// @param t 测试上下文
func TestHistoricVariableSnapshot(t *testing.T) {
	base := time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)
	entries := []api.HistoricVariableInstance{
		{ID: "1", Name: "amount", Value: 1, CreateTime: base, LastUpdatedTime: base},
		{ID: "2", Name: "amount", Value: 2, CreateTime: base, LastUpdatedTime: base.Add(time.Hour)},
		{ID: "3", Name: "local", Value: "global", CreateTime: base, LastUpdatedTime: base},
		{ID: "4", Name: "local", Value: "task", TaskID: "t1", CreateTime: base, LastUpdatedTime: base},
		{ID: "5", Name: "other", Value: "otherTask", TaskID: "t2", CreateTime: base, LastUpdatedTime: base},
	}
	values := historicVariableSnapshot(entries, "")
	if values["amount"] != 2 || values["local"] != "global" || values["other"] != "otherTask" {
		t.Fatalf("实例历史变量快照错误: %+v", values)
	}
	values = historicVariableSnapshot(entries, "t1")
	if values["local"] != "task" || values["amount"] != 2 {
		t.Fatalf("任务局部历史变量快照错误: %+v", values)
	}
	values = historicVariableSnapshot(entries, "t3")
	if values["local"] != "global" {
		t.Fatalf("其它任务应回退实例变量: %+v", values)
	}
	if len(historicVariableSnapshot(nil, "")) != 0 {
		t.Fatalf("空历史变量应返回空快照")
	}

	createEarlier := api.HistoricVariableInstance{ID: "a", Name: "x", Value: 1, CreateTime: base, LastUpdatedTime: base}
	createLater := api.HistoricVariableInstance{ID: "b", Name: "x", Value: 2, CreateTime: base.Add(time.Minute), LastUpdatedTime: base}
	if !preferHistoricVariable(createLater, createEarlier, "") {
		t.Fatalf("创建时间更晚应优先")
	}
	if preferHistoricVariable(createEarlier, createLater, "") {
		t.Fatalf("创建时间更早不应覆盖")
	}
	sameTimeLower := api.HistoricVariableInstance{ID: "a", Name: "x", Value: 1, CreateTime: base, LastUpdatedTime: base}
	sameTimeHigher := api.HistoricVariableInstance{ID: "b", Name: "x", Value: 2, CreateTime: base, LastUpdatedTime: base}
	if !preferHistoricVariable(sameTimeHigher, sameTimeLower, "") || preferHistoricVariable(sameTimeLower, sameTimeHigher, "") {
		t.Fatalf("相同时间应按 ID 稳定选择")
	}
}

// TestFilterItemsByVariables 验证变量过滤与错误传播。
// @param t 测试上下文
func TestFilterItemsByVariables(t *testing.T) {
	conditions, err := parseQueryVariableConditions(mustQueryBody(t, `{"variables":[{"name":"status","value":"open"}]}`))
	if err != nil {
		t.Fatalf("解析条件失败: %v", err)
	}
	values := map[string]map[string]any{
		"a": {"status": "open"},
		"b": {"status": "closed"},
	}
	lookup := func(_ context.Context, item string) (map[string]any, error) {
		return values[item], nil
	}
	got, err := filterItemsByVariables(context.Background(), []string{"a", "b"}, conditions, lookup)
	if err != nil || len(got) != 1 || got[0] != "a" {
		t.Fatalf("变量过滤结果错误: %v %v", got, err)
	}

	calls := 0
	got, err = filterItemsByVariables(context.Background(), []string{"a", "b"}, nil, func(context.Context, string) (map[string]any, error) {
		calls++
		return nil, nil
	})
	if err != nil || calls != 0 || len(got) != 2 {
		t.Fatalf("空条件不应查询变量: %v %d %v", got, calls, err)
	}

	sentinel := errors.New("变量查询失败")
	if _, err := filterItemsByVariables(context.Background(), []string{"a"}, conditions, func(context.Context, string) (map[string]any, error) {
		return nil, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("变量查询错误应透传: %v", err)
	}

	if _, err := applyQueryVariableFilter(context.Background(), []string{"a"}, mustQueryBody(t, `{"variables":123}`), lookup); err == nil {
		t.Fatalf("非法 variables 应报错")
	}
	got, err = applyQueryVariableFilter(context.Background(), []string{"a", "b"}, mustQueryBody(t, "{}"), lookup)
	if err != nil || len(got) != 2 {
		t.Fatalf("无变量条件应返回原列表: %v %v", got, err)
	}
	got, err = applyQueryVariableFilter(context.Background(), []string{"a", "b"}, mustQueryBody(t, `{"variables":[{"name":"status","value":"closed"}]}`), lookup)
	if err != nil || len(got) != 1 || got[0] != "b" {
		t.Fatalf("apply 变量过滤错误: %v %v", got, err)
	}
}

// TestHistoricVariablesEmptyInstance 验证空实例的历史变量查询。
// @param t 测试上下文
func TestHistoricVariablesEmptyInstance(t *testing.T) {
	server := &Server{}
	values, err := server.historicVariables(context.Background(), "", "")
	if err != nil || len(values) != 0 {
		t.Fatalf("空实例历史变量应为空: %v %v", values, err)
	}
	if values, err := server.historicProcessVariables(context.Background(), api.HistoricProcessInstance{}); err != nil || len(values) != 0 {
		t.Fatalf("空历史实例变量应为空: %v %v", values, err)
	}
	if values, err := server.historicTaskVariables(context.Background(), api.HistoricTaskInstance{}); err != nil || len(values) != 0 {
		t.Fatalf("空历史任务变量应为空: %v %v", values, err)
	}
}

// TestREST_QueryProcessInstancesVariables 验证运行流程实例变量过滤。
// @param t 测试上下文
func TestREST_QueryProcessInstancesVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	startVariableQueryInstance(t, server.URL, "var-a", map[string]any{
		"amount": 100, "status": "open", "title": "Urgent order", "score": 10,
		"when": "2026-10-28T01:02:03Z",
	})
	startVariableQueryInstance(t, server.URL, "var-b", map[string]any{
		"amount": 5, "status": "closed", "title": "normal order", "score": 2,
		"when": "2026-09-28T01:02:03Z",
	})
	const path = "/service/query/process-instances"
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"数值等于", withVariables(variableFilter("amount", 100)), 1},
		{"默认等于", withVariables(variableFilter("status", "open")), 1},
		{"不等于", withVariables(variableFilter("status", "open", "notEquals")), 1},
		{"模糊匹配", withVariables(variableFilter("title", "%URGENT%", "like")), 1},
		{"大于", withVariables(variableFilter("score", 5, "greaterThan")), 1},
		{"大于等于", withVariables(variableFilter("amount", 100, "greaterThanOrEqual")), 1},
		{"小于", withVariables(variableFilter("amount", 100, "lessThan")), 1},
		{"小于等于", withVariables(variableFilter("score", 2, "lessThanOrEqual")), 1},
		{"多条件 AND", map[string]any{"variables": []map[string]any{
			variableFilter("amount", 0, "greaterThan"),
			variableFilter("status", "open"),
		}}, 1},
		{"缺失变量", withVariables(variableFilter("missing", "x")), 0},
		{"缺失变量不等于", withVariables(variableFilter("missing", "x", "notEquals")), 0},
		{"空数组", map[string]any{"variables": []any{}}, 2},
		{"操作符大小写", withVariables(variableFilter("score", 5, "GREATERTHAN")), 1},
		{"时间等于", withVariables(variableFilter("when", "2026-10-28T01:02:03Z")), 1},
		{"时间大于", withVariables(variableFilter("when", "2026-10-01T00:00:00Z", "greaterThan")), 1},
		{"时间小于", withVariables(variableFilter("when", "2026-10-01T00:00:00Z", "lessThan")), 1},
		{"模糊未命中", withVariables(variableFilter("title", "%missing%", "like")), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVariableQueryTotal(t, server.URL, path, tc.body, tc.want)
		})
	}

	page := assertVariableQueryTotal(t, server.URL, path, map[string]any{
		"variables": []map[string]any{variableFilter("amount", 0, "greaterThan")},
		"sort":      "businessKey",
		"order":     "desc",
		"size":      1,
	}, 2)
	if len(page.Data) != 1 || page.Data[0]["businessKey"] != "var-b" {
		t.Fatalf("变量过滤应先于分页排序: %+v", page)
	}

	combined := withVariables(variableFilter("status", "open"))
	combined["processBusinessKey"] = "var-a"
	assertVariableQueryTotal(t, server.URL, path, combined, 1)
	combined = withVariables(variableFilter("status", "closed"))
	combined["processBusinessKey"] = "var-a"
	assertVariableQueryTotal(t, server.URL, path, combined, 0)

	invalid := []any{
		withVariables(variableFilter("amount", 100, "between")),
		map[string]any{"variables": "bad"},
		map[string]any{"variables": []map[string]any{{"value": 1}}},
	}
	for _, body := range invalid {
		assertVariableQueryBadRequest(t, server.URL, path, body)
	}
}

// TestREST_QueryTasksVariables 验证运行任务变量过滤。
// @param t 测试上下文
func TestREST_QueryTasksVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceA := startVariableQueryInstance(t, server.URL, "var-a", map[string]any{
		"amount": 100, "status": "open", "title": "Urgent order",
	})
	startVariableQueryInstance(t, server.URL, "var-b", map[string]any{
		"amount": 5, "status": "closed", "title": "normal order",
	})
	taskA := variableQueryTaskID(t, server.URL, instanceA)
	response := postJSON(t, server.URL+"/service/runtime/tasks/"+taskA+"/variables", map[string]any{"name": "localTag", "value": "vip"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("设置任务局部变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	const path = "/service/query/tasks"
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"执行变量等于", withVariables(variableFilter("status", "open")), 1},
		{"不等于", withVariables(variableFilter("status", "open", "notEquals")), 1},
		{"模糊匹配", withVariables(variableFilter("title", "%urgent%", "like")), 1},
		{"大于", withVariables(variableFilter("amount", 50, "greaterThan")), 1},
		{"任务局部变量", withVariables(variableFilter("localTag", "vip")), 1},
		{"缺失变量", withVariables(variableFilter("missing", "x")), 0},
		{"缺失变量不等于", withVariables(variableFilter("missing", "x", "notEquals")), 0},
		{"空数组", map[string]any{"variables": []any{}}, 2},
		{"操作符大小写", withVariables(variableFilter("amount", 10, "LESSTHAN")), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVariableQueryTotal(t, server.URL, path, tc.body, tc.want)
		})
	}

	combined := withVariables(variableFilter("status", "open"))
	combined["processInstanceBusinessKey"] = "var-a"
	assertVariableQueryTotal(t, server.URL, path, combined, 1)

	page := assertVariableQueryTotal(t, server.URL, path, map[string]any{
		"variables": []map[string]any{variableFilter("amount", 0, "greaterThan")},
		"sort":      "id",
		"size":      1,
	}, 2)
	if len(page.Data) != 1 {
		t.Fatalf("变量过滤应先于任务分页: %+v", page)
	}

	assertVariableQueryBadRequest(t, server.URL, path, withVariables(variableFilter("amount", 1, "between")))
	assertVariableQueryBadRequest(t, server.URL, path, map[string]any{"variables": 1})
}

// TestREST_QueryHistoricProcessesVariables 验证历史流程实例变量过滤。
// @param t 测试上下文
func TestREST_QueryHistoricProcessesVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceA := startVariableQueryInstance(t, server.URL, "var-a", map[string]any{
		"amount": 100, "status": "open", "title": "Urgent order",
	})
	startVariableQueryInstance(t, server.URL, "var-b", map[string]any{
		"amount": 5, "status": "closed", "title": "normal order",
	})
	completeQueryTask(t, server.URL, instanceA)

	const path = "/service/query/historic-process-instances"
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"历史变量等于", withVariables(variableFilter("status", "open")), 1},
		{"历史变量不等于", withVariables(variableFilter("status", "open", "notEquals")), 1},
		{"历史变量模糊", withVariables(variableFilter("title", "%URGENT%", "like")), 1},
		{"历史变量大于", withVariables(variableFilter("amount", 50, "greaterThan")), 1},
		{"缺失变量", withVariables(variableFilter("missing", "x")), 0},
		{"缺失变量不等于", withVariables(variableFilter("missing", "x", "notEquals")), 0},
		{"空数组", map[string]any{"variables": []any{}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVariableQueryTotal(t, server.URL, path, tc.body, tc.want)
		})
	}

	finished := withVariables(variableFilter("status", "open"))
	finished["finished"] = true
	assertVariableQueryTotal(t, server.URL, path, finished, 1)
	running := withVariables(variableFilter("status", "open"))
	running["finished"] = false
	assertVariableQueryTotal(t, server.URL, path, running, 0)

	page := assertVariableQueryTotal(t, server.URL, path, map[string]any{
		"variables": []map[string]any{variableFilter("amount", 0, "greaterThan")},
		"sort":      "businessKey",
		"order":     "desc",
		"size":      1,
	}, 2)
	if len(page.Data) != 1 || page.Data[0]["businessKey"] != "var-b" {
		t.Fatalf("历史实例变量过滤应先于分页排序: %+v", page)
	}

	assertVariableQueryBadRequest(t, server.URL, path, withVariables(variableFilter("amount", 1, "between")))
}

// TestREST_QueryHistoricTasksVariables 验证历史任务变量过滤。
// @param t 测试上下文
func TestREST_QueryHistoricTasksVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceA := startVariableQueryInstance(t, server.URL, "var-a", map[string]any{
		"amount": 100, "status": "open", "title": "Urgent order",
	})
	instanceB := startVariableQueryInstance(t, server.URL, "var-b", map[string]any{
		"amount": 5, "status": "closed", "title": "normal order",
	})
	completeQueryTask(t, server.URL, instanceA)
	completeQueryTask(t, server.URL, instanceB)

	const path = "/service/query/historic-task-instances"
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"历史任务变量等于", withVariables(variableFilter("status", "open")), 1},
		{"历史任务变量不等于", withVariables(variableFilter("status", "open", "notEquals")), 1},
		{"历史任务变量模糊", withVariables(variableFilter("title", "%NORMAL%", "like")), 1},
		{"历史任务变量大于", withVariables(variableFilter("amount", 50, "greaterThan")), 1},
		{"缺失变量", withVariables(variableFilter("missing", "x")), 0},
		{"缺失变量不等于", withVariables(variableFilter("missing", "x", "notEquals")), 0},
		{"空数组", map[string]any{"variables": []any{}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVariableQueryTotal(t, server.URL, path, tc.body, tc.want)
		})
	}

	finished := withVariables(variableFilter("status", "open"))
	finished["finished"] = true
	assertVariableQueryTotal(t, server.URL, path, finished, 1)

	combined := withVariables(variableFilter("status", "open"))
	combined["taskNameLike"] = "%approve%"
	assertVariableQueryTotal(t, server.URL, path, combined, 1)

	page := assertVariableQueryTotal(t, server.URL, path, map[string]any{
		"variables": []map[string]any{variableFilter("amount", 0, "greaterThan")},
		"sort":      "id",
		"size":      1,
	}, 2)
	if len(page.Data) != 1 {
		t.Fatalf("历史任务变量过滤应先于分页: %+v", page)
	}

	assertVariableQueryBadRequest(t, server.URL, path, withVariables(variableFilter("amount", 1, "between")))
}

// variablesParallelProcess 是执行局部变量过滤测试使用的并行流程。
const variablesParallelProcess = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"
             xmlns:flowable="http://flowable.org/bpmn">
  <process id="parallelVars" name="ParallelVars" isExecutable="true">
    <startEvent id="start"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="fork"/>
    <parallelGateway id="fork"/>
    <sequenceFlow id="f2" sourceRef="fork" targetRef="taskA"/>
    <sequenceFlow id="f3" sourceRef="fork" targetRef="taskB"/>
    <userTask id="taskA" name="Branch A"/>
    <userTask id="taskB" name="Branch B"/>
    <sequenceFlow id="f4" sourceRef="taskA" targetRef="join"/>
    <sequenceFlow id="f5" sourceRef="taskB" targetRef="join"/>
    <parallelGateway id="join"/>
    <sequenceFlow id="f6" sourceRef="join" targetRef="end"/>
    <endEvent id="end"/>
  </process>
</definitions>`

// deployVariableQueryProcess 部署变量查询测试流程。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param xml BPMN 内容
func deployVariableQueryProcess(t *testing.T, serverURL, xml string) {
	t.Helper()
	response, err := http.Post(serverURL+"/service/repository/deployments?deploymentName=variables", "application/xml", bytes.NewBufferString(xml))
	if err != nil {
		t.Fatalf("部署变量查询流程失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("部署变量查询流程状态错误: %d", response.StatusCode)
	}
}

// queryVariableTotal 返回查询命中总数。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param path 查询路径
// @param body 查询体
// @return 命中总数
func queryVariableTotal(t *testing.T, serverURL, path string, body map[string]any) int {
	t.Helper()
	response := postQuery(t, serverURL, path, body, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("查询应成功: %d %+v", response.StatusCode, body)
	}
	return decodeQueryPage[map[string]any](t, response).Total
}

// executionIDByActivity 返回实例中指定活动对应的执行 ID。
// @param t 测试上下文
// @param serverURL 测试服务地址
// @param instanceID 流程实例 ID
// @param activityID 活动 ID
// @return 执行 ID
func executionIDByActivity(t *testing.T, serverURL, instanceID, activityID string) string {
	t.Helper()
	response := postQuery(t, serverURL, "/service/query/executions", map[string]any{"processInstanceId": instanceID}, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("查询实例执行失败: %d", response.StatusCode)
	}
	page := decodeQueryPage[map[string]any](t, response)
	for _, item := range page.Data {
		if item["activityId"] != activityID {
			continue
		}
		if id, _ := item["id"].(string); id != "" {
			return id
		}
	}
	t.Fatalf("实例 %s 不存在活动 %s 的执行", instanceID, activityID)
	return ""
}

// TestCachedQueryVariableLookup 验证查询内缓存复用、错误透传与错误不缓存。
// @param t 测试上下文
func TestCachedQueryVariableLookup(t *testing.T) {
	loads := make(map[string]int)
	lookup := cachedQueryVariableLookup(
		func(item string) string { return item },
		func(_ context.Context, item string) (map[string]any, error) {
			loads[item]++
			return map[string]any{"key": item}, nil
		},
	)
	for _, item := range []string{"p1", "p1", "p2", "p1"} {
		values, err := lookup(context.Background(), item)
		if err != nil || values["key"] != item {
			t.Fatalf("缓存变量查询错误 %s: %v %+v", item, err, values)
		}
	}
	if loads["p1"] != 1 || loads["p2"] != 1 {
		t.Fatalf("同一缓存键应只加载一次: %+v", loads)
	}

	sentinel := errors.New("变量加载失败")
	failures := 0
	retry := cachedQueryVariableLookup(
		func(item string) string { return item },
		func(context.Context, string) (map[string]any, error) {
			failures++
			if failures == 1 {
				return nil, sentinel
			}
			return map[string]any{"ok": true}, nil
		},
	)
	if _, err := retry(context.Background(), "p1"); !errors.Is(err, sentinel) {
		t.Fatalf("变量加载错误应透传: %v", err)
	}
	// 首次加载失败不写缓存，重试应重新加载
	if values, err := retry(context.Background(), "p1"); err != nil || values["ok"] != true || failures != 2 {
		t.Fatalf("失败结果不应缓存: %v %+v %d", err, values, failures)
	}
}

// TestHistoricActivityVariablesLookup 验证历史活动变量访问器按实例返回快照。
// @param t 测试上下文
func TestHistoricActivityVariablesLookup(t *testing.T) {
	server := &Server{}
	lookup := server.historicActivityVariablesLookup()
	values, err := lookup(context.Background(), api.HistoricActivityInstance{ActivityID: "task"})
	if err != nil || len(values) != 0 {
		t.Fatalf("空实例历史活动变量应为空: %v %+v", err, values)
	}
}

// TestREST_QueryExecutionsVariables 验证执行查询按执行可见变量过滤。
// @param t 测试上下文
func TestREST_QueryExecutionsVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployVariableQueryProcess(t, server.URL, variablesParallelProcess)
	instanceA := startKeyedVariableQueryInstance(t, server.URL, "parallelVars", "var-a", map[string]any{
		"amount": 100, "status": "open", "title": "Urgent order", "score": 10,
	})
	instanceB := startKeyedVariableQueryInstance(t, server.URL, "parallelVars", "var-b", map[string]any{
		"amount": 5, "status": "closed", "title": "normal order", "score": 2,
	})
	taskA := executionIDByActivity(t, server.URL, instanceA, "taskA")
	taskB := executionIDByActivity(t, server.URL, instanceA, "taskB")
	if taskA == taskB {
		t.Fatalf("并行分支执行应相互独立: %s", taskA)
	}
	// 在 taskA 分支执行上写入局部变量，仅该执行可见
	response := postJSONMethod(t, http.MethodPut, server.URL+"/service/runtime/executions/"+taskA+"/variables/localTag", map[string]any{"value": "vip"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("设置执行局部变量失败: %d", response.StatusCode)
	}
	_ = response.Body.Close()

	const path = "/service/query/executions"
	totalA := queryVariableTotal(t, server.URL, path, map[string]any{"processInstanceId": instanceA})
	totalB := queryVariableTotal(t, server.URL, path, map[string]any{"processInstanceId": instanceB})
	if totalA == 0 || totalB == 0 {
		t.Fatalf("执行基线查询为空: %d %d", totalA, totalB)
	}

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"执行变量等于", withVariables(variableFilter("status", "open")), totalA},
		{"不等于", withVariables(variableFilter("status", "open", "notEquals")), totalB},
		{"模糊匹配", withVariables(variableFilter("title", "%URGENT%", "like")), totalA},
		{"大于", withVariables(variableFilter("amount", 50, "greaterThan")), totalA},
		{"小于", withVariables(variableFilter("score", 5, "lessThan")), totalA},
		{"缺失变量", withVariables(variableFilter("missing", "x")), 0},
		{"缺失变量不等于", withVariables(variableFilter("missing", "x", "notEquals")), 0},
		{"空数组", map[string]any{"variables": []any{}}, totalA + totalB},
		{"操作符大小写", withVariables(variableFilter("amount", 50, "GREATERTHAN")), totalA},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVariableQueryTotal(t, server.URL, path, tc.body, tc.want)
		})
	}

	// 执行局部变量只命中所写入的分支执行
	local := withVariables(variableFilter("localTag", "vip"))
	assertVariableQueryTotal(t, server.URL, path, local, 1)
	scopedLocal := withVariables(variableFilter("localTag", "vip"))
	scopedLocal["processInstanceId"] = instanceA
	assertVariableQueryTotal(t, server.URL, path, scopedLocal, 1)
	otherInstanceLocal := withVariables(variableFilter("localTag", "vip"))
	otherInstanceLocal["processInstanceId"] = instanceB
	assertVariableQueryTotal(t, server.URL, path, otherInstanceLocal, 0)
	// 局部变量不会提升为实例变量
	assertVariableQueryTotal(t, server.URL, "/service/query/process-instances", withVariables(variableFilter("localTag", "vip")), 0)

	page := assertVariableQueryTotal(t, server.URL, path, map[string]any{
		"variables": []map[string]any{variableFilter("amount", 0, "greaterThan")},
		"sort":      "processInstanceId",
		"size":      1,
	}, totalA+totalB)
	if len(page.Data) != 1 {
		t.Fatalf("执行变量过滤应先于分页排序: %+v", page)
	}

	invalid := []any{
		map[string]any{"processInstanceId": "missing", "variables": []map[string]any{variableFilter("amount", 1, "between")}},
		withVariables(variableFilter("amount", 100, "between")),
		map[string]any{"variables": "bad"},
		map[string]any{"variables": []map[string]any{{"value": 1}}},
	}
	for _, body := range invalid {
		assertVariableQueryBadRequest(t, server.URL, path, body)
	}
}

// TestREST_QueryHistoricActivitiesVariables 验证历史活动查询按历史变量过滤。
// @param t 测试上下文
func TestREST_QueryHistoricActivitiesVariables(t *testing.T) {
	server, _ := newTestServer(t)
	deployREST(t, server)
	instanceA := startVariableQueryInstance(t, server.URL, "var-a", map[string]any{
		"amount": 100, "status": "open", "title": "Urgent order",
	})
	instanceB := startVariableQueryInstance(t, server.URL, "var-b", map[string]any{
		"amount": 5, "status": "closed", "title": "normal order",
	})

	const path = "/service/query/historic-activity-instances"
	totalA := queryVariableTotal(t, server.URL, path, map[string]any{"processInstanceId": instanceA})
	totalB := queryVariableTotal(t, server.URL, path, map[string]any{"processInstanceId": instanceB})
	if totalA == 0 || totalB == 0 {
		t.Fatalf("历史活动基线查询为空: %d %d", totalA, totalB)
	}
	if totalA < 2 {
		t.Fatalf("同一实例应包含多个历史活动以验证缓存: %d", totalA)
	}

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"历史变量等于", withVariables(variableFilter("status", "open")), totalA},
		{"历史变量不等于", withVariables(variableFilter("status", "open", "notEquals")), totalB},
		{"历史变量模糊", withVariables(variableFilter("title", "%URGENT%", "like")), totalA},
		{"历史变量大于", withVariables(variableFilter("amount", 50, "greaterThan")), totalA},
		{"历史变量小于", withVariables(variableFilter("amount", 50, "lessThan")), totalB},
		{"缺失变量", withVariables(variableFilter("missing", "x")), 0},
		{"缺失变量不等于", withVariables(variableFilter("missing", "x", "notEquals")), 0},
		{"空数组", map[string]any{"variables": []any{}}, totalA + totalB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertVariableQueryTotal(t, server.URL, path, tc.body, tc.want)
		})
	}

	// 同一实例的多个活动共享变量快照，缓存前后结果一致
	scoped := withVariables(variableFilter("status", "open"))
	scoped["processInstanceId"] = instanceA
	assertVariableQueryTotal(t, server.URL, path, scoped, totalA)
	if second := queryVariableTotal(t, server.URL, path, scoped); second != totalA {
		t.Fatalf("缓存复用不应改变结果: %d != %d", second, totalA)
	}
	repeated := withVariables(variableFilter("status", "open"))
	if first, second := queryVariableTotal(t, server.URL, path, repeated), queryVariableTotal(t, server.URL, path, repeated); first != second || first != totalA {
		t.Fatalf("重复查询结果应稳定: %d %d %d", first, second, totalA)
	}

	page := assertVariableQueryTotal(t, server.URL, path, map[string]any{
		"variables": []map[string]any{variableFilter("amount", 0, "greaterThan")},
		"sort":      "startTime",
		"order":     "desc",
		"size":      1,
	}, totalA+totalB)
	if len(page.Data) != 1 {
		t.Fatalf("历史活动变量过滤应先于分页排序: %+v", page)
	}

	invalid := []any{
		withVariables(variableFilter("amount", 1, "between")),
		map[string]any{"variables": "bad"},
		map[string]any{"variables": []map[string]any{{"value": 1}}},
	}
	for _, body := range invalid {
		assertVariableQueryBadRequest(t, server.URL, path, body)
	}
}
