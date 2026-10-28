package quality

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDocumentationGate 验证中文说明、参数、返回值及忽略目录的文档门禁。
// @param t 测试上下文
// @return 无
func TestDocumentationGate(t *testing.T) {
	for _, test := range []struct {
		name, source string
		failed       bool
	}{
		{"完整", "package p\n// F 中文说明。\n// @param x 输入\n// @return 输出\nfunc F(x int) int{return x}\n", false},
		{"缺少说明", "package p\nfunc F() {}\n", true},
		{"缺少参数", "package p\n// F 中文说明。\nfunc F(x int) {}\n", true},
		{"缺少返回", "package p\n// F 中文说明。\nfunc F() int{return 1}\n", true},
		{"多返回", "package p\n// F 中文说明。\n// @return 数值\n// @return 错误\nfunc F()(int,error){return 1,nil}\n", false},
		{"具名返回", "package p\n// F 中文说明。\n// @return 数值\n// @return 状态\nfunc F()(a,b int){return}\n", false},
		{"方法", "package p\ntype T int\n// F 中文说明。\nfunc (t T)F(){}\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(test.source), 0600); err != nil {
				t.Fatal(err)
			}
			failures, err := CheckDocumentation(root)
			if err != nil || (len(failures) > 0) != test.failed {
				t.Fatalf("文档门禁结果错误: %v %v", failures, err)
			}
		})
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tmp", "bad.go"), []byte("无效 Go"), 0600); err != nil {
		t.Fatal(err)
	}
	if failures, err := CheckDocumentation(root); err != nil || len(failures) != 0 {
		t.Fatalf("忽略目录错误: %v %v", failures, err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.go"), []byte("无效 Go"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckDocumentation(root); err == nil {
		t.Fatal("解析失败应阻断")
	}
	if _, err := CheckDocumentation(filepath.Join(root, "missing")); err == nil {
		t.Fatal("缺少目录应阻断")
	}
}
