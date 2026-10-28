package quality

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBaselineGate 验证基线摘要变化、非法版本及路径会阻断验收。
// @param t 测试上下文
// @return 无
func TestBaselineGate(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "testdata/compatibility")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := CheckBaseline(root); err == nil {
		t.Fatal("缺少基线应失败")
	}
	if err := os.WriteFile(filepath.Join(root, "asset"), []byte("基线"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("基线"))
	for _, test := range []struct {
		name, commit, path, digest string
		version                    int
		failed                     bool
	}{
		{"完整", "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e", "asset", hex.EncodeToString(sum[:]), 1, false},
		{"摘要变化", "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e", "asset", "错误", 1, true},
		{"非法提交", "bad", "asset", hex.EncodeToString(sum[:]), 1, true},
		{"未知版本", "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e", "asset", hex.EncodeToString(sum[:]), 2, true},
		{"越界路径", "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e", "../outside", hex.EncodeToString(sum[:]), 1, true},
		{"缺失制品", "f6017e32a4bdd3e59a8881f58b0ee181652d1d9e", "missing", hex.EncodeToString(sum[:]), 1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"version": test.version, "java_commit": test.commit, "schema_version": "8.1.0.3", "artifacts": []map[string]string{{"path": test.path, "sha256": test.digest}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "baseline.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := CheckBaseline(root); (err != nil) != test.failed {
				t.Fatalf("基线检查错误: %v", err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(directory, "baseline.json"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CheckBaseline(root); err == nil {
		t.Fatal("损坏基线应失败")
	}
}

// TestRouteReport 验证方法级匹配、参数规范化、部分支持、缺失与扩展分类。
// @param t 测试上下文
// @return 无
func TestRouteReport(t *testing.T) {
	spec := "paths:\n  /runtime/items/{itemId}:\n    get:\n    delete:\n  /runtime/variables-async:\n    post:\n"
	implementation := `s.mux.HandleFunc("GET /service/runtime/items/{id}", handler)
s.mux.HandleFunc("POST /service/runtime/variables-async", handler)
s.mux.HandleFunc("GET /service/extension", handler)`
	report := RouteReport(spec, implementation, map[string]string{"POST /runtime/variables-async": "部分支持：同步执行"})
	for _, expected := range []string{"官方方法端点 3", "实现方法端点 3", "GET /runtime/items/{}", "缺失；迁移验收阻断", "实现扩展", "部分支持：同步执行"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("端点表缺少 %s: %s", expected, report)
		}
	}
	if report != RouteReport(spec, implementation, map[string]string{"POST /runtime/variables-async": "部分支持：同步执行"}) {
		t.Fatal("端点表必须可重复生成")
	}
}

// TestNormalizeRoutePathPrecision 验证单段变量与跨段通配不会互相误匹配。
// @param t 测试上下文
func TestNormalizeRoutePathPrecision(t *testing.T) {
	cases := map[string]string{
		"/repository/deployments/{id}/resources/**":        "/repository/deployments/{}/resources/{**}",
		"/repository/deployments/{id}/resources/{name...}": "/repository/deployments/{}/resources/{**}",
		"/runtime/items/{itemId}":                          "/runtime/items/{}",
	}
	for input, expected := range cases {
		if actual := normalizeRoutePath(input); actual != expected {
			t.Fatalf("规范化错误 %s: %s", input, actual)
		}
	}

	spec := "paths:\n  /repository/deployments/{deploymentId}/resources/**:\n    get:\n"
	single := `s.mux.HandleFunc("GET /service/repository/deployments/{id}/resources/{name}", handler)`
	report := RouteReport(spec, single, nil)
	if !strings.Contains(report, "缺失；迁移验收阻断") {
		t.Fatalf("单段通配不应匹配官方跨段通配: %s", report)
	}
	multi := `s.mux.HandleFunc("GET /service/repository/deployments/{id}/resources/{name...}", handler)`
	report = RouteReport(spec, multi, nil)
	if strings.Contains(report, "缺失；迁移验收阻断") {
		t.Fatalf("跨段通配应匹配官方路径: %s", report)
	}
}
