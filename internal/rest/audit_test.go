package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/api"
	"github.com/flowable-go/flowable/internal/engine"
)

// TestAuditLogRing 验证审计日志容量。
// @param t 测试上下文
func TestAuditLogRing(t *testing.T) {
	log := newAuditLog(2)
	for i := 0; i < 3; i++ {
		log.record(api.AuditEvent{Path: "p"})
	}
	events := log.list()
	if len(events) != 2 || events[0].ID != 2 || events[1].ID != 3 {
		t.Fatalf("审计环错误: %+v", events)
	}
}

// TestAuditMiddleware 验证审计记录。
// @param t 测试上下文
func TestAuditMiddleware(t *testing.T) {
	app, err := engine.New(engine.Config{HistoryLevel: engine.HistoryAudit})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	server := httptest.NewServer(New(app, WithAuditLog(10)).Handler())
	defer func() {
		server.Close()
		_ = app.Close(context.Background())
	}()
	response, _ := http.Get(server.URL + "/service/repository/deployments")
	_ = response.Body.Close()
	response, _ = http.Get(server.URL + "/service/management/audit-logs")
	var page struct {
		Total int              `json:"total"`
		Data  []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(response.Body).Decode(&page)
	_ = response.Body.Close()
	if page.Total == 0 {
		t.Fatalf("审计日志为空")
	}
	if page.Data[0]["path"] != "/service/management/audit-logs" && page.Data[0]["path"] != "/service/repository/deployments" {
		t.Fatalf("审计路径错误: %+v", page.Data[0])
	}
}
