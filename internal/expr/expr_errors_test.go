package expr

import (
	"context"
	"errors"
	"testing"
)

// TestNodeEvalErrors 验证 AST 节点错误分支。
// @param t 测试上下文
func TestNodeEvalErrors(t *testing.T) {
	if value, err := Eval("${missing}", nil); err != nil || value != nil {
		t.Fatalf("缺失变量应返回 nil: %v %v", value, err)
	}
	if _, err := Eval("${missing.name}", nil); err == nil {
		t.Fatalf("nil 属性访问应报错")
	}
	if _, err := (&unaryNode{op: "?", child: &literalNode{value: 1}}).eval(nil); err == nil {
		t.Fatalf("未知一元运算应报错")
	}
	if _, err := (&binaryNode{op: "?", left: &literalNode{value: 1}, right: &literalNode{value: 2}}).eval(nil); err == nil {
		t.Fatalf("未知二元运算应报错")
	}
	if _, err := (&callNode{name: "missing"}).eval(nil); err == nil {
		t.Fatalf("无环境函数调用应报错")
	}
	if _, err := (&callNode{name: "fail"}).eval(&Env{Functions: map[string]Function{
		"fail": func(args ...any) (any, error) { return nil, errors.New("失败") },
	}}); err == nil {
		t.Fatalf("函数错误应传播")
	}
	if _, err := (&callNode{name: "miss"}).eval(&Env{Functions: map[string]Function{}}); err == nil {
		t.Fatalf("未注册函数应报错")
	}
}

// TestEvalBoolAndStringErrors 验证转换错误。
// @param t 测试上下文
func TestEvalBoolAndStringErrors(t *testing.T) {
	if _, err := EvalBool("${1}", nil); err == nil {
		t.Fatalf("非布尔应报错")
	}
	if _, err := EvalString("${1 / 0}", nil); err == nil {
		t.Fatalf("求值错误应传播")
	}
	if _, err := EvalBool("1 +", nil); err == nil {
		t.Fatalf("编译错误应传播")
	}
}

// TestParserEdgeErrors 验证解析边界错误。
// @param t 测试上下文
func TestParserEdgeErrors(t *testing.T) {
	cases := []string{
		"${true ? 1}",
		"${f(1,)}",
		"${f(}",
		"${a.}",
		"${a[1}",
		"${(1 + 2}",
	}
	for _, source := range cases {
		if _, err := Compile(source); err == nil {
			t.Fatalf("%q 应解析失败", source)
		}
	}
}

// TestEmptyExpressionEval 验证空表达式求值。
// @param t 测试上下文
func TestEmptyExpressionEval(t *testing.T) {
	expression := &Expression{}
	value, err := expression.Eval(context.Background(), nil)
	if err != nil || value != nil {
		t.Fatalf("空表达式应返回 nil")
	}
}
