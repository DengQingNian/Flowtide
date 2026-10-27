package expr

import (
	"context"
	"strings"
	"testing"
)

// testUser 是属性访问测试结构体。
type testUser struct {
	Name string
	Age  int
}

// TestEval_WordOperators 验证单词运算符与更多运算。
// @param t 测试上下文
func TestEval_WordOperators(t *testing.T) {
	vars := map[string]any{"a": 10, "b": 3, "flag": true}
	cases := []struct {
		source string
		want   any
	}{
		{"${a and flag}", true},
		{"${false or flag}", true},
		{"${a eq 10}", true},
		{"${a ne 3}", true},
		{"${b lt a}", true},
		{"${b le 3}", true},
		{"${a gt b}", true},
		{"${a ge 10}", true},
		{"${a div b}", 3.3333333333333335},
		{"${a mod b}", 1.0},
		{"${not false}", true},
		{"${!false}", true},
		{"${-a}", -10.0},
		{"${empty('')}", true},
		{"${a != b}", true},
		{"${b <= a}", true},
		{"${a >= b}", true},
	}
	for _, tc := range cases {
		got, err := Eval(tc.source, vars)
		if err != nil {
			t.Fatalf("%s 求值失败: %v", tc.source, err)
		}
		if got != tc.want {
			t.Fatalf("%s 期望 %#v，实际 %#v", tc.source, tc.want, got)
		}
	}
}

// TestEval_ContextCanceled 验证上下文取消。
// @param t 测试上下文
func TestEval_ContextCanceled(t *testing.T) {
	compiled, err := Compile("${1 + 1}")
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := compiled.Eval(ctx, &Env{}); err == nil {
		t.Fatalf("期望上下文取消错误")
	}
}

// TestSource 验证原始表达式文本。
// @param t 测试上下文
func TestSource(t *testing.T) {
	compiled, _ := Compile("${1 + 1}")
	if compiled.Source() != "${1 + 1}" {
		t.Fatalf("Source 返回错误")
	}
}

// TestCompileExpression 验证裸表达式与 # 定界符。
// @param t 测试上下文
func TestCompileExpression(t *testing.T) {
	compiled, err := CompileExpression("#{1 + 1}")
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	got, _ := compiled.Eval(context.Background(), &Env{})
	if got != 2.0 {
		t.Fatalf("期望 2，实际 %#v", got)
	}
	if _, err := CompileExpression(" "); err == nil {
		t.Fatalf("期望空表达式错误")
	}
	if _, err := CompileExpression("1 +"); err == nil {
		t.Fatalf("期望语法错误")
	}
}

// TestConversionHelpers 验证类型转换。
// @param t 测试上下文
func TestConversionHelpers(t *testing.T) {
	floats := []any{int(1), int8(1), int16(1), int32(1), int64(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1), float32(1), 1.0, "1"}
	for _, value := range floats {
		if got, err := toFloat(value); err != nil || got != 1 {
			t.Fatalf("toFloat(%T) 失败: %v %v", value, got, err)
		}
	}
	if _, err := toFloat("x"); err == nil {
		t.Fatalf("期望 toFloat 字符串错误")
	}
	if _, err := toFloat(struct{}{}); err == nil {
		t.Fatalf("期望 toFloat 类型错误")
	}
	ints := []any{int(1), int64(1), 1.0, "1"}
	for _, value := range ints {
		if got, err := toInt(value); err != nil || got != 1 {
			t.Fatalf("toInt(%T) 失败: %v %v", value, got, err)
		}
	}
	if _, err := toInt("x"); err == nil {
		t.Fatalf("期望 toInt 字符串错误")
	}
	if _, err := toInt(struct{}{}); err == nil {
		t.Fatalf("期望 toInt 类型错误")
	}
}

// TestEqualAndTruthy 验证相等与真值判断。
// @param t 测试上下文
func TestEqualAndTruthy(t *testing.T) {
	if !equal(nil, nil) || equal(nil, 1) {
		t.Fatalf("nil 相等判断错误")
	}
	if !equal(1, 1.0) || equal("1", 1) {
		t.Fatalf("跨类型相等判断错误")
	}
	if !equal(map[string]any{"a": 1}, map[string]any{"a": 1}) {
		t.Fatalf("map 深比较错误")
	}
	if !truthy(true) || truthy(false) || truthy(nil) || !truthy("x") {
		t.Fatalf("真值判断错误")
	}
	if !isEmpty(nil) || !isEmpty("") || !isEmpty([]any{}) || !isEmpty(map[string]any{}) {
		t.Fatalf("空判断错误")
	}
	if isEmpty([]any{1}) || isEmpty(1) {
		t.Fatalf("非空判断错误")
	}
	if value, err := ToBool(nil); err != nil || value {
		t.Fatalf("nil 转布尔错误")
	}
	if value, err := ToBool(true); err != nil || !value {
		t.Fatalf("true 转布尔错误")
	}
	if _, err := ToBool("true"); err == nil {
		t.Fatalf("字符串转布尔应报错")
	}
}

// TestPropertyAccess 验证属性访问。
// @param t 测试上下文
func TestPropertyAccess(t *testing.T) {
	if got, err := getProperty(map[string]string{"name": "alice"}, "name"); err != nil || got != "alice" {
		t.Fatalf("map[string]string 属性错误: %v %v", got, err)
	}
	if got, err := getProperty(&testUser{Name: "bob"}, "name"); err != nil || got != "bob" {
		t.Fatalf("结构体属性错误: %v %v", got, err)
	}
	if got, err := getProperty(testUser{Age: 3}, "Age"); err != nil || got != 3 {
		t.Fatalf("结构体字段错误: %v %v", got, err)
	}
	if _, err := getProperty(nil, "x"); err == nil {
		t.Fatalf("nil 属性应报错")
	}
	if _, err := getProperty((*testUser)(nil), "x"); err == nil {
		t.Fatalf("nil 指针属性应报错")
	}
	if _, err := getProperty(testUser{}, "missing"); err == nil {
		t.Fatalf("不存在属性应报错")
	}
}

// TestIndexAccess 验证下标访问。
// @param t 测试上下文
func TestIndexAccess(t *testing.T) {
	if got, err := getIndex(map[string]any{"a": 1}, "a"); err != nil || got != 1 {
		t.Fatalf("map 下标错误: %v %v", got, err)
	}
	if got, err := getIndex([]string{"x", "y"}, 1); err != nil || got != "y" {
		t.Fatalf("[]string 下标错误: %v %v", got, err)
	}
	if got, err := getIndex("abc", 1); err != nil || got != "b" {
		t.Fatalf("字符串下标错误: %v %v", got, err)
	}
	if _, err := getIndex([]any{1}, 5); err == nil {
		t.Fatalf("越界应报错")
	}
	if _, err := getIndex([]string{"x"}, "bad"); err == nil {
		t.Fatalf("非法下标应报错")
	}
	if _, err := getIndex("abc", -1); err == nil {
		t.Fatalf("负下标应报错")
	}
	if _, err := getIndex(1, 0); err == nil {
		t.Fatalf("不支持类型应报错")
	}
}

// TestLexerEdges 验证词法边界。
// @param t 测试上下文
func TestLexerEdges(t *testing.T) {
	tokens, err := lex("'a\\nb' 1e3")
	if err != nil || len(tokens) < 3 {
		t.Fatalf("词法失败: %v %v", err, tokens)
	}
	if !strings.Contains(tokens[0].text, "\n") {
		t.Fatalf("转义解析错误: %q", tokens[0].text)
	}
	for _, source := range []string{"&", "|", "=", "@", "'abc"} {
		if _, err := lex(source); err == nil {
			t.Fatalf("%q 应词法失败", source)
		}
	}
}

// TestBinaryErrors 验证二元运算错误。
// @param t 测试上下文
func TestBinaryErrors(t *testing.T) {
	if _, err := arithmetic("/", 1, 0); err == nil {
		t.Fatalf("除零应报错")
	}
	if _, err := arithmetic("%", 1, 0); err == nil {
		t.Fatalf("模零应报错")
	}
	if _, err := arithmetic("+", "a", 1); err == nil {
		t.Fatalf("非数字加法应报错")
	}
	if _, err := compare("<", "a", 1); err == nil {
		t.Fatalf("字符串与数字比较应报错")
	}
	if _, err := compare("??", 1, 2); err == nil {
		t.Fatalf("未知比较应报错")
	}
	if _, err := applyBinary("??", 1, 2); err == nil {
		t.Fatalf("未知二元运算应报错")
	}
	if !isNumber(1) || isNumber("1") {
		t.Fatalf("数值判断错误")
	}
}
