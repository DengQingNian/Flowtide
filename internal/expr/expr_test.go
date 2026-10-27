package expr

import (
	"context"
	"testing"
)

// TestEval_Cases 验证表达式求值。
// @param t 测试上下文
func TestEval_Cases(t *testing.T) {
	vars := map[string]any{
		"amount":   150,
		"approved": true,
		"status":   "open",
		"user":     map[string]any{"name": "alice"},
		"items":    []any{"a", "b"},
	}
	cases := []struct {
		name   string
		source string
		want   any
	}{
		{"数字加法", "${1 + 2}", 3.0},
		{"字符串拼接", "${'a' + 'b'}", "ab"},
		{"比较", "${amount > 100}", true},
		{"相等", "${status == 'open'}", true},
		{"逻辑与", "${approved && amount >= 100}", true},
		{"逻辑或", "${approved || false}", true},
		{"取反", "${!approved}", false},
		{"三元", "${approved ? 'yes' : 'no'}", "yes"},
		{"属性访问", "${user.name}", "alice"},
		{"下标访问", "${items[1]}", "b"},
		{"空判断", "${empty(items)}", false},
		{"空值判断", "${empty(null)}", true},
		{"括号", "${(1 + 2) * 3}", 9.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Eval(tc.source, vars)
			if err != nil {
				t.Fatalf("求值失败: %v", err)
			}
			if got != tc.want {
				t.Fatalf("期望 %#v，实际 %#v", tc.want, got)
			}
		})
	}
}

// TestEvalBool_BareExpression 验证裸条件表达式。
// @param t 测试上下文
func TestEvalBool_BareExpression(t *testing.T) {
	got, err := EvalBool("amount > 100", map[string]any{"amount": 150})
	if err != nil || !got {
		t.Fatalf("期望 true，实际 %v %v", got, err)
	}
	got, err = EvalBool("amount > 100", map[string]any{"amount": 50})
	if err != nil || got {
		t.Fatalf("期望 false，实际 %v %v", got, err)
	}
}

// TestEvalString_Literal 验证无定界符按字面量处理。
// @param t 测试上下文
func TestEvalString_Literal(t *testing.T) {
	got, err := EvalString("alice", nil)
	if err != nil || got != "alice" {
		t.Fatalf("期望 alice，实际 %q %v", got, err)
	}
}

// TestEval_Function 验证函数调用。
// @param t 测试上下文
func TestEval_Function(t *testing.T) {
	compiled, err := Compile("${add(1, 2)}")
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	got, err := compiled.Eval(context.Background(), &Env{Functions: map[string]Function{
		"add": func(args ...any) (any, error) {
			left, _ := args[0].(float64)
			right, _ := args[1].(float64)
			return left + right, nil
		},
	}})
	if err != nil || got != 3.0 {
		t.Fatalf("期望 3，实际 %#v %v", got, err)
	}
}

// TestEval_Errors 验证错误场景。
// @param t 测试上下文
func TestEval_Errors(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{"语法错误", "${1 +}"},
		{"除零", "${1 / 0}"},
		{"未知函数", "${missing()}"},
		{"单等号", "${a = 1}"},
		{"未闭合字符串", "${'abc}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Eval(tc.source, nil); err == nil {
				t.Fatalf("期望错误")
			}
		})
	}
}

// TestCompile_Empty 验证空表达式。
// @param t 测试上下文
func TestCompile_Empty(t *testing.T) {
	if _, err := Compile("${}"); err == nil {
		t.Fatalf("期望空表达式错误")
	}
}

// TestToString 验证字符串转换。
// @param t 测试上下文
func TestToString(t *testing.T) {
	if ToString(nil) != "" || ToString(12) != "12" || ToString([]byte("x")) != "x" {
		t.Fatalf("字符串转换错误")
	}
}
