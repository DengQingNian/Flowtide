package rest

import (
	"net/http/httptest"
	"testing"

	"github.com/flowable-go/flowable/api"
)

// TestTenantHelpers 验证租户辅助函数。
// @param t 测试上下文
func TestTenantHelpers(t *testing.T) {
	if firstNonEmpty("", "b", "c") != "b" || firstNonEmpty("", "") != "" {
		t.Fatalf("firstNonEmpty 错误")
	}
	request := httptest.NewRequest("GET", "/service/runtime/tasks?tenantId=q1", nil)
	if requestTenant(request) != "q1" {
		t.Fatalf("查询租户解析错误")
	}
	request.Header.Set("X-Tenant-Id", "h1")
	if requestTenant(request) != "h1" {
		t.Fatalf("请求头租户优先错误")
	}
	deployments := []api.Deployment{{ID: "d1", TenantID: "t1"}, {ID: "d2", TenantID: "t2"}}
	if len(filterDeployments(deployments, "")) != 2 || len(filterDeployments(deployments, "t1")) != 1 {
		t.Fatalf("部署过滤错误")
	}
	definitions := []api.ProcessDefinition{{ID: "p1", TenantID: "t1"}}
	if len(filterDefinitions(definitions, "")) != 1 || len(filterDefinitions(definitions, "t1")) != 1 || len(filterDefinitions(definitions, "x")) != 0 {
		t.Fatalf("定义过滤错误")
	}
	instances := []api.ProcessInstance{{ID: "i1", TenantID: "t1"}}
	if len(filterInstances(instances, "")) != 1 || len(filterInstances(instances, "t1")) != 1 || len(filterInstances(instances, "x")) != 0 {
		t.Fatalf("实例过滤错误")
	}
}
