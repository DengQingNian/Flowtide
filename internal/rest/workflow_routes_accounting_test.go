package rest

import (
	"testing"
)

// workflowRetainedStubPaths 是保留 501（服务已注入也维持桩）的端点集合。
// 该清单与 workflowHandlerFor 中"保留 501 的端点"分支严格对应，口径由
// 016 号审计记录固定：路由表 95 条 = 已接线 84 条 + 保留 501 11 条。
var workflowRetainedStubPaths = map[string]bool{
	"/workflow/process/import/xml":          true,
	"/workflow/process/importProcess":       true,
	"/workflow/process/exportProcess":       true,
	"/workflow/process/inject/node":         true,
	"/workflow/process/exmple/node/list":    true,
	"/workflow/process/wait/node/list":      true,
	"/workflow/process/public/deploymentId": true,
	"/workflow/process/public/processes/id": true,
	"/workflow/hisstep/add":                 true,
	"/workflow/hisstep/edit":                true,
	"/workflow/hisstep/delete/{ids}":        true,
}

// TestWorkflowRouteTableAccounting 锁定 workflow 路由表口径，防止文档与实现的数字漂移。
// 口径：路由表条目 = 已接线 + 保留 501；任一变化都会使本用例失败并要求同步修订文档 015。
// @param t 测试上下文
func TestWorkflowRouteTableAccounting(t *testing.T) {
	const wantTotal = 95
	const wantRetained = 11
	const wantWired = 84

	if len(workflowRouteTable) != wantTotal {
		t.Fatalf("路由表条目数变化：期望 %d，实际 %d（请同步修订文档 015 并更新本基准）", wantTotal, len(workflowRouteTable))
	}
	if len(workflowRetainedStubPaths) != wantRetained {
		t.Fatalf("保留 501 路径数变化：期望 %d，实际 %d", wantRetained, len(workflowRetainedStubPaths))
	}
	if got := wantTotal - wantRetained; got != wantWired {
		t.Fatalf("已接线数不自洽：%d - %d = %d，期望 %d", wantTotal, wantRetained, got, wantWired)
	}
}

// TestWorkflowRetainedStubPathsInRouteTable 验证保留 501 清单全部登记在路由表中。
// @param t 测试上下文
func TestWorkflowRetainedStubPathsInRouteTable(t *testing.T) {
	registered := make(map[string]bool, len(workflowRouteTable))
	for _, route := range workflowRouteTable {
		registered[route.path] = true
	}
	for path := range workflowRetainedStubPaths {
		if !registered[path] {
			t.Errorf("保留 501 路径未登记在路由表：%s", path)
		}
	}
}
