package rest

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/clock"
	"github.com/flowable-go/flowable/internal/engine"
)

// TestSortHistoricMaps 验证响应对象按原版排序字段与方向排列。
// @param t 测试上下文
// @return 无
func TestSortHistoricMaps(t *testing.T) {
	items := []any{
		map[string]any{"processInstanceId": "b", "startTime": "2026-01-02T00:00:00Z"},
		map[string]any{"processInstanceId": "a", "startTime": "2026-01-01T00:00:00Z"},
	}
	sortHistoricMaps(items, "startTime", "desc", "processInstanceId", "asc")
	if items[0].(map[string]any)["startTime"] != "2026-01-02T00:00:00Z" {
		t.Fatalf("降序排序错误: %+v", items)
	}
	sortHistoricMaps(items, "startTime", "", "processInstanceId", "asc")
	if items[0].(map[string]any)["startTime"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("升序排序错误: %+v", items)
	}
	variables := []any{
		map[string]any{"variable": map[string]any{"name": "b"}},
		map[string]any{"variable": map[string]any{"name": "a"}},
	}
	sortHistoricMaps(variables, "variableName", "", "processInstanceId", "asc")
	if variables[0].(map[string]any)["variable"].(map[string]any)["name"] != "a" {
		t.Fatalf("变量名排序错误: %+v", variables)
	}
	logs := []any{
		map[string]any{"logNumber": int64(2)},
		map[string]any{"logNumber": int64(1)},
	}
	sortHistoricMaps(logs, "logNumber", "", "logNumber", "asc")
	if logs[0].(map[string]any)["logNumber"] != int64(1) {
		t.Fatalf("日志序号排序错误: %+v", logs)
	}
	missing := []any{map[string]any{"other": "x"}}
	sortHistoricMaps(missing, "", "", "unknownField", "asc")
}

// TestHistoricVariableValue 验证变量值对象的二进制数据端点地址。
// @param t 测试上下文
// @return 无
func TestHistoricVariableValue(t *testing.T) {
	request := httptest.NewRequest("GET", "http://localhost/service/history/historic-variable-instances/v1", nil)
	binary := historicVariableValue(request, "payload", "global", "binary", []byte{1, 2}, "history/historic-variable-instances", "v1")
	if binary["value"] != nil || binary["valueUrl"] != "http://localhost/service/history/historic-variable-instances/v1/data" {
		t.Fatalf("二进制变量值对象错误: %+v", binary)
	}
	plain := historicVariableValue(request, "flag", "global", "string", "after", "history/historic-variable-instances", "v2")
	if plain["value"] != "after" || plain["valueUrl"] != nil {
		t.Fatalf("普通变量值对象错误: %+v", plain)
	}
}

// TestHistoricRESTValue 验证日期以 UTC 毫秒输出。
// @param t 测试上下文
// @return 无
func TestHistoricRESTValue(t *testing.T) {
	stamp := time.Date(2026, 2, 3, 4, 5, 6, 789000000, time.UTC)
	if historicRESTValue(nil) != nil {
		t.Fatal("空值应输出空")
	}
	if historicRESTValue([]byte{1}) != nil {
		t.Fatal("字节值应输出空")
	}
	out, err := json.Marshal(historicRESTValue(stamp))
	if err != nil || string(out) != "\"2026-02-03T04:05:06.789Z\"" {
		t.Fatalf("日期值输出错误: %s %v", out, err)
	}
}

// TestWriteHistoricPageClamp 验证分页外壳的越界截断与实际条数。
// @param t 测试上下文
// @return 无
func TestWriteHistoricPageClamp(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost/service/history/historic-process-instances", nil)
	writeHistoricPage(recorder, request, []map[string]any{}, 5, 10, "processInstanceId")
	var page struct {
		Total int              `json:"total"`
		Size  int              `json:"size"`
		Data  []map[string]any `json:"data"`
		Sort  string           `json:"sort"`
		Order string           `json:"order"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.Size != 0 || page.Sort != "processInstanceId" || page.Order != "asc" || len(page.Data) != 0 {
		t.Fatalf("越界分页外壳错误: %+v", page)
	}
	if reflectValueSlice("不是切片") != nil {
		t.Fatal("非切片输入应返回空")
	}
}

// TestSortHistoricMapsFallback 验证缺失字段与非整数序号的排序回退。
// @param t 测试上下文
// @return 无
func TestSortHistoricMapsFallback(t *testing.T) {
	items := []any{
		map[string]any{"logNumber": "非整数"},
		map[string]any{"logNumber": int64(1)},
		map[string]any{},
	}
	sortHistoricMaps(items, "logNumber", "", "logNumber", "asc")
	sortHistoricMaps(items, "未知字段", "", "未知字段", "asc")
}

// TestWriteHistoricQueryPageClamp 验证查询分页的越界截断。
// @param t 测试上下文
// @return 无
func TestWriteHistoricQueryPageClamp(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost/service/query/historic-process-instances", nil)
	writeHistoricQueryPage(recorder, request, []map[string]any{}, 9, 10, "", "", "processInstanceId")
	var page struct {
		Total int              `json:"total"`
		Size  int              `json:"size"`
		Start int              `json:"start"`
		Data  []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.Size != 0 || page.Start != 0 || len(page.Data) != 0 {
		t.Fatalf("越界查询分页错误: %+v", page)
	}
}

// TestHistoricProcessMapsDefinitionMissing 验证定义缺失时以空定义兜底。
// @param t 测试上下文
// @return 无
func TestHistoricProcessMapsDefinitionMissing(t *testing.T) {
	fake := clock.NewFake(time.Now())
	app, err := engine.New(engine.Config{Clock: fake, IDs: &historyJavaIDs{}, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close(context.Background()) })
	wrapped := &Server{engine: app}
	request := httptest.NewRequest("GET", "http://localhost/service/history/historic-process-instances", nil)
	maps, err := historicProcessMaps(wrapped, request, []api.HistoricProcessInstance{{ID: "x", ProcessDefinitionID: "missing"}}, false)
	if err != nil || len(maps) != 1 || maps[0]["processDefinitionName"] != nil {
		t.Fatalf("缺失定义兜底错误: %+v %v", maps, err)
	}
}
