package xml

import (
	"strings"
	"testing"
)

// TestDecode_Helpers 验证节点辅助方法。
// @param t 测试上下文
func TestDecode_Helpers(t *testing.T) {
	data := `<a xmlns:x="urn:test" x:y="1"><b>text<c>inner</c></b></a>`
	root, err := Decode(strings.NewReader(data), 1024)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if root.Name.Local != "a" {
		t.Fatalf("根元素错误")
	}
	if root.Attr("y") != "1" || root.AttrNS("urn:test", "y") != "1" {
		t.Fatalf("属性读取错误")
	}
	if root.Attr("missing") != "" || root.AttrNS("other", "y") != "" {
		t.Fatalf("缺失属性应返回空串")
	}
	child, ok := root.Child("b")
	if !ok {
		t.Fatalf("子节点不存在")
	}
	if child.TextContent() != "textinner" {
		t.Fatalf("文本内容错误: %q", child.TextContent())
	}
	if _, ok := root.Child("missing"); ok {
		t.Fatalf("不存在子节点应返回 false")
	}
	if len(root.ChildrenNamed("b")) != 1 {
		t.Fatalf("子节点列表错误")
	}
}

// TestDecode_Errors 验证解码错误。
// @param t 测试上下文
func TestDecode_Errors(t *testing.T) {
	cases := []struct {
		name  string
		input string
		limit int64
	}{
		{"空输入", "", 1024},
		{"多根", "<a/><b/>", 1024},
		{"未闭合", "<a>", 1024},
		{"深嵌套", strings.Repeat("<a>", 205) + strings.Repeat("</a>", 205), 1 << 20},
		{"DTD", "<!DOCTYPE a><a/>", 1024},
		{"超限", "<a>1234567890</a>", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(tc.input), tc.limit); err == nil {
				t.Fatalf("期望解码失败")
			}
		})
	}
}

// TestDecode_Comments 验证注释被忽略。
// @param t 测试上下文
func TestDecode_Comments(t *testing.T) {
	root, err := Decode(strings.NewReader(`<a><!-- comment -->x</a>`), 1024)
	if err != nil || root.TextContent() != "x" {
		t.Fatalf("注释处理错误: %v %q", err, root.TextContent())
	}
}
