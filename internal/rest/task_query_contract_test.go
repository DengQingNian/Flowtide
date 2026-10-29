package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/tenant"
)

// TestTaskQueryTenantAndLargePage 验证独立任务租户过滤和千条之后的匹配项不丢失。
// @param t 测试上下文
// @return 无
func TestTaskQueryTenantAndLargePage(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryNone})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tenant-a", "tenant-b"} {
		ctx := tenant.With(context.Background(), id)
		if _, err := app.CreateTask(ctx, api.CreateTaskRequest{Name: "独立任务", TenantID: id}); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(app).Handler()
	r := httptest.NewRequest("GET", "/service/runtime/tasks?tenantId=tenant-a", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatalf("独立任务租户过滤丢失: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < 1001; i++ {
		if _, err := app.CreateTask(context.Background(), api.CreateTaskRequest{Name: fmt.Sprintf("task-%04d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/service/runtime/tasks?name=task-1000", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) || !strings.Contains(w.Body.String(), `"name":"task-1000"`) {
		t.Fatalf("千条之后的查询结果丢失: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/service/runtime/tasks?size=2000", nil))
	var page api.Page[map[string]any]
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Data) != 1001 || page.Total != 1001 || page.Size != 1001 {
		t.Fatalf("大页结果被引擎单页上限截断: 数据=%d 总量=%d 页大小=%d 错误=%v", len(page.Data), page.Total, page.Size, err)
	}
}
