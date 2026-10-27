// Package expr 实现 UEL 兼容的表达式子集。
// 支持变量、属性、下标、算术、比较、逻辑、三元与内置函数。
package expr

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// Function 是表达式函数。
type Function func(args ...any) (any, error)

// Env 是表达式求值环境。
type Env struct {
	Vars      map[string]any      // 变量表
	Functions map[string]Function // 函数表
}

// Expression 是编译后的表达式。
type Expression struct {
	source string // 原始文本
	root   node   // AST 根节点
}

// Source 返回原始表达式文本。
// @return 表达式文本
func (e *Expression) Source() string { return e.source }

// Eval 在环境中求值。
// @param ctx 上下文，用于取消长表达式
// @param env 求值环境
// @return 结果
// @return 错误
func (e *Expression) Eval(ctx context.Context, env *Env) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.root == nil {
		return nil, nil
	}
	return e.root.eval(env)
}

// Compile 编译表达式。
// @param source 表达式文本，可带 ${} 或 #{} 定界符
// @return 编译结果
// @return 编译错误
func Compile(source string) (*Expression, error) {
	inner, isExpression := unwrap(source)
	if !isExpression {
		return &Expression{source: source, root: &literalNode{value: source}}, nil
	}
	if strings.TrimSpace(inner) == "" {
		return nil, fmt.Errorf("表达式为空")
	}
	tokens, err := lex(inner)
	if err != nil {
		return nil, err
	}
	parser := &parser{tokens: tokens}
	root, err := parser.parse()
	if err != nil {
		return nil, err
	}
	if parser.peek().kind != tokenEOF {
		return nil, fmt.Errorf("表达式存在多余内容: %s", parser.peek().text)
	}
	return &Expression{source: source, root: root}, nil
}

// CompileExpression 编译表达式，不带定界符时也按表达式解析。
// 用于 BPMN 条件表达式等允许裸表达式的场景。
// @param source 表达式文本
// @return 编译结果
// @return 编译错误
func CompileExpression(source string) (*Expression, error) {
	inner, _ := unwrap(source)
	if strings.TrimSpace(inner) == "" {
		return nil, fmt.Errorf("表达式为空")
	}
	tokens, err := lex(inner)
	if err != nil {
		return nil, err
	}
	parser := &parser{tokens: tokens}
	root, err := parser.parse()
	if err != nil {
		return nil, err
	}
	if parser.peek().kind != tokenEOF {
		return nil, fmt.Errorf("表达式存在多余内容: %s", parser.peek().text)
	}
	return &Expression{source: source, root: root}, nil
}

// Eval 编译并求值表达式。
// @param source 表达式文本
// @param vars 变量表
// @return 结果
// @return 错误
func Eval(source string, vars map[string]any) (any, error) {
	compiled, err := Compile(source)
	if err != nil {
		return nil, err
	}
	return compiled.Eval(context.Background(), &Env{Vars: vars})
}

// EvalString 编译求值并转换为字符串。
// @param source 表达式文本
// @param vars 变量表
// @return 字符串结果
// @return 错误
func EvalString(source string, vars map[string]any) (string, error) {
	value, err := Eval(source, vars)
	if err != nil {
		return "", err
	}
	return ToString(value), nil
}

// EvalBool 编译求值并转换为布尔值。
// @param source 表达式文本
// @param vars 变量表
// @return 布尔结果
// @return 错误
func EvalBool(source string, vars map[string]any) (bool, error) {
	compiled, err := CompileExpression(source)
	if err != nil {
		return false, err
	}
	value, err := compiled.Eval(context.Background(), &Env{Vars: vars})
	if err != nil {
		return false, err
	}
	return ToBool(value)
}

// ToString 将任意值转换为字符串。
// @param value 值
// @return 字符串
func ToString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

// ToBool 将任意值转换为布尔值。
// @param value 值
// @return 布尔值
// @return 错误
func ToBool(value any) (bool, error) {
	if value == nil {
		return false, nil
	}
	if b, ok := value.(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("值不是布尔类型: %T", value)
}

// node 是 AST 节点。
type node interface {
	// eval 在环境中求值。
	// @param env 求值环境
	// @return 结果
	// @return 错误
	eval(env *Env) (any, error)
}

// literalNode 表示字面量。
type literalNode struct {
	value any // 字面量值
}

// eval 返回字面量。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *literalNode) eval(*Env) (any, error) { return n.value, nil }

// identNode 表示变量引用。
type identNode struct {
	name string // 变量名
}

// eval 查找变量。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *identNode) eval(env *Env) (any, error) {
	if env == nil || env.Vars == nil {
		return nil, nil
	}
	return env.Vars[n.name], nil
}

// unaryNode 表示一元运算。
type unaryNode struct {
	op    string // 运算符
	child node   // 子节点
}

// eval 执行一元运算。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *unaryNode) eval(env *Env) (any, error) {
	value, err := n.child.eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "!", "not":
		return !truthy(value), nil
	case "-":
		number, convErr := toFloat(value)
		if convErr != nil {
			return nil, convErr
		}
		return -number, nil
	case "empty":
		return isEmpty(value), nil
	default:
		return nil, fmt.Errorf("未知一元运算符: %s", n.op)
	}
}

// binaryNode 表示二元运算。
type binaryNode struct {
	op          string // 运算符
	left, right node   // 操作数
}

// eval 执行二元运算。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *binaryNode) eval(env *Env) (any, error) {
	left, err := n.left.eval(env)
	if err != nil {
		return nil, err
	}
	// 逻辑运算短路，避免无意义求值
	switch n.op {
	case "&&", "and":
		if !truthy(left) {
			return false, nil
		}
		right, rightErr := n.right.eval(env)
		if rightErr != nil {
			return nil, rightErr
		}
		return truthy(right), nil
	case "||", "or":
		if truthy(left) {
			return true, nil
		}
		right, rightErr := n.right.eval(env)
		if rightErr != nil {
			return nil, rightErr
		}
		return truthy(right), nil
	}
	right, err := n.right.eval(env)
	if err != nil {
		return nil, err
	}
	return applyBinary(n.op, left, right)
}

// ternaryNode 表示三元表达式。
type ternaryNode struct {
	cond, then, otherwise node // 条件、真值、假值
}

// eval 执行三元表达式。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *ternaryNode) eval(env *Env) (any, error) {
	value, err := n.cond.eval(env)
	if err != nil {
		return nil, err
	}
	if truthy(value) {
		return n.then.eval(env)
	}
	return n.otherwise.eval(env)
}

// memberNode 表示属性访问。
type memberNode struct {
	target node   // 目标
	name   string // 属性名
}

// eval 读取属性。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *memberNode) eval(env *Env) (any, error) {
	target, err := n.target.eval(env)
	if err != nil {
		return nil, err
	}
	return getProperty(target, n.name)
}

// indexNode 表示下标访问。
type indexNode struct {
	target node // 目标
	index  node // 下标
}

// eval 读取下标。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *indexNode) eval(env *Env) (any, error) {
	target, err := n.target.eval(env)
	if err != nil {
		return nil, err
	}
	index, err := n.index.eval(env)
	if err != nil {
		return nil, err
	}
	return getIndex(target, index)
}

// callNode 表示函数调用。
type callNode struct {
	name string // 函数名
	args []node // 参数
}

// eval 调用函数。
// @param env 求值环境
// @return 结果
// @return 错误
func (n *callNode) eval(env *Env) (any, error) {
	if env == nil || env.Functions == nil {
		return nil, fmt.Errorf("未注册函数: %s", n.name)
	}
	fn, ok := env.Functions[n.name]
	if !ok {
		return nil, fmt.Errorf("未注册函数: %s", n.name)
	}
	args := make([]any, 0, len(n.args))
	for _, arg := range n.args {
		value, err := arg.eval(env)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	return fn(args...)
}

// applyBinary 执行二元运算。
// @param op 运算符
// @param left 左值
// @param right 右值
// @return 结果
// @return 错误
func applyBinary(op string, left, right any) (any, error) {
	switch op {
	case "==", "eq":
		return equal(left, right), nil
	case "!=", "ne":
		return !equal(left, right), nil
	case "<", "lt", "<=", "le", ">", "gt", ">=", "ge":
		return compare(op, left, right)
	case "+":
		if _, ok := left.(string); ok {
			return ToString(left) + ToString(right), nil
		}
		if _, ok := right.(string); ok {
			return ToString(left) + ToString(right), nil
		}
		return arithmetic(op, left, right)
	case "-", "*", "/", "%":
		return arithmetic(op, left, right)
	case "div":
		return arithmetic("/", left, right)
	case "mod":
		return arithmetic("%", left, right)
	default:
		return nil, fmt.Errorf("未知二元运算符: %s", op)
	}
}

// arithmetic 执行数值运算。
// @param op 运算符
// @param left 左值
// @param right 右值
// @return 结果
// @return 错误
func arithmetic(op string, left, right any) (any, error) {
	leftNumber, err := toFloat(left)
	if err != nil {
		return nil, err
	}
	rightNumber, err := toFloat(right)
	if err != nil {
		return nil, err
	}
	switch op {
	case "+":
		return leftNumber + rightNumber, nil
	case "-":
		return leftNumber - rightNumber, nil
	case "*":
		return leftNumber * rightNumber, nil
	case "/":
		if rightNumber == 0 {
			return nil, fmt.Errorf("除数不能为零")
		}
		return leftNumber / rightNumber, nil
	case "%":
		if rightNumber == 0 {
			return nil, fmt.Errorf("模数不能为零")
		}
		return math.Mod(leftNumber, rightNumber), nil
	default:
		return nil, fmt.Errorf("未知算术运算符: %s", op)
	}
}

// compare 比较两个值。
// @param op 运算符
// @param left 左值
// @param right 右值
// @return 比较结果
// @return 错误
func compare(op string, left, right any) (bool, error) {
	var cmp int
	leftString, leftIsString := left.(string)
	rightString, rightIsString := right.(string)
	if leftIsString && rightIsString {
		cmp = strings.Compare(leftString, rightString)
	} else {
		leftNumber, err := toFloat(left)
		if err != nil {
			return false, err
		}
		rightNumber, err := toFloat(right)
		if err != nil {
			return false, err
		}
		switch {
		case leftNumber < rightNumber:
			cmp = -1
		case leftNumber > rightNumber:
			cmp = 1
		default:
			cmp = 0
		}
	}
	switch op {
	case "<", "lt":
		return cmp < 0, nil
	case "<=", "le":
		return cmp <= 0, nil
	case ">", "gt":
		return cmp > 0, nil
	case ">=", "ge":
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("未知比较运算符: %s", op)
	}
}

// equal 判断两个值是否相等。
// @param left 左值
// @param right 右值
// @return 是否相等
func equal(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if isNumber(left) && isNumber(right) {
		leftNumber, leftErr := toFloat(left)
		rightNumber, rightErr := toFloat(right)
		if leftErr == nil && rightErr == nil {
			return leftNumber == rightNumber
		}
	}
	return reflect.DeepEqual(left, right)
}

// truthy 将值转换为布尔真值。
// @param value 值
// @return 是否为真
func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case nil:
		return false
	default:
		return true
	}
}

// isEmpty 判断值是否为空。
// @param value 值
// @return 是否为空
func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

// toFloat 将值转换为 float64。
// @param value 值
// @return 数值
// @return 错误
func toFloat(value any) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("无法转换为数字: %q", v)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("无法转换为数字: %T", value)
	}
}

// isNumber 判断是否为数值类型。
// @param value 值
// @return 是否为数值
func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

// getProperty 读取属性。
// @param target 目标
// @param name 属性名
// @return 属性值
// @return 错误
func getProperty(target any, name string) (any, error) {
	if target == nil {
		return nil, fmt.Errorf("无法读取 nil 的属性: %s", name)
	}
	switch v := target.(type) {
	case map[string]any:
		return v[name], nil
	case map[string]string:
		return v[name], nil
	}
	value := reflect.ValueOf(target)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("无法读取 nil 指针的属性: %s", name)
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Struct {
		field := value.FieldByNameFunc(func(candidate string) bool {
			return strings.EqualFold(candidate, name)
		})
		if field.IsValid() && field.CanInterface() {
			return field.Interface(), nil
		}
	}
	return nil, fmt.Errorf("属性不存在: %s", name)
}

// getIndex 读取下标。
// @param target 目标
// @param index 下标
// @return 元素值
// @return 错误
func getIndex(target, index any) (any, error) {
	switch v := target.(type) {
	case map[string]any:
		return v[ToString(index)], nil
	case []any:
		i, err := toInt(index)
		if err != nil {
			return nil, err
		}
		if i < 0 || i >= len(v) {
			return nil, fmt.Errorf("下标越界: %d", i)
		}
		return v[i], nil
	case []string:
		i, err := toInt(index)
		if err != nil {
			return nil, err
		}
		if i < 0 || i >= len(v) {
			return nil, fmt.Errorf("下标越界: %d", i)
		}
		return v[i], nil
	case string:
		i, err := toInt(index)
		if err != nil {
			return nil, err
		}
		runes := []rune(v)
		if i < 0 || i >= len(runes) {
			return nil, fmt.Errorf("下标越界: %d", i)
		}
		return string(runes[i]), nil
	default:
		return nil, fmt.Errorf("不支持下标访问: %T", target)
	}
}

// toInt 将值转换为整数。
// @param value 值
// @return 整数
// @return 错误
func toInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("无法转换为整数: %q", v)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("无法转换为整数: %T", value)
	}
}

// unwrap 去除表达式定界符。
// @param source 原始文本
// @return 内部表达式
// @return 是否为表达式
func unwrap(source string) (string, bool) {
	trimmed := strings.TrimSpace(source)
	if len(trimmed) >= 3 && (strings.HasPrefix(trimmed, "${") || strings.HasPrefix(trimmed, "#{")) && strings.HasSuffix(trimmed, "}") {
		return trimmed[2 : len(trimmed)-1], true
	}
	return source, false
}

// tokenKind 是词法单元类型。
type tokenKind int

// 词法单元类型常量。
const (
	tokenEOF tokenKind = iota
	tokenIdent
	tokenNumber
	tokenString
	tokenOperator
	tokenLParen
	tokenRParen
	tokenComma
	tokenDot
	tokenLBracket
	tokenRBracket
	tokenQuestion
	tokenColon
)

// token 是词法单元。
type token struct {
	kind tokenKind // 类型
	text string    // 文本
	pos  int       // 位置
}

// lex 将表达式拆分为词法单元。
// @param source 表达式文本
// @return 词法单元列表
// @return 词法错误
func lex(source string) ([]token, error) {
	tokens := make([]token, 0, len(source)/2)
	runes := []rune(source)
	for i := 0; i < len(runes); {
		ch := runes[i]
		if unicode.IsSpace(ch) {
			i++
			continue
		}
		if unicode.IsLetter(ch) || ch == '_' {
			start := i
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			text := string(runes[start:i])
			switch text {
			case "and", "or", "eq", "ne", "lt", "gt", "le", "ge", "div", "mod":
				tokens = append(tokens, token{kind: tokenOperator, text: text, pos: start})
			default:
				tokens = append(tokens, token{kind: tokenIdent, text: text, pos: start})
			}
			continue
		}
		if unicode.IsDigit(ch) || (ch == '.' && i+1 < len(runes) && unicode.IsDigit(runes[i+1])) {
			start := i
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			if i < len(runes) && (runes[i] == 'e' || runes[i] == 'E') {
				i++
				if i < len(runes) && (runes[i] == '+' || runes[i] == '-') {
					i++
				}
				for i < len(runes) && unicode.IsDigit(runes[i]) {
					i++
				}
			}
			tokens = append(tokens, token{kind: tokenNumber, text: string(runes[start:i]), pos: start})
			continue
		}
		if ch == '\'' || ch == '"' {
			value, next, err := readString(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenString, text: value, pos: i})
			i = next
			continue
		}
		switch ch {
		case '(':
			tokens = append(tokens, token{kind: tokenLParen, text: "(", pos: i})
			i++
		case ')':
			tokens = append(tokens, token{kind: tokenRParen, text: ")", pos: i})
			i++
		case ',':
			tokens = append(tokens, token{kind: tokenComma, text: ",", pos: i})
			i++
		case '.':
			tokens = append(tokens, token{kind: tokenDot, text: ".", pos: i})
			i++
		case '[':
			tokens = append(tokens, token{kind: tokenLBracket, text: "[", pos: i})
			i++
		case ']':
			tokens = append(tokens, token{kind: tokenRBracket, text: "]", pos: i})
			i++
		case '?':
			tokens = append(tokens, token{kind: tokenQuestion, text: "?", pos: i})
			i++
		case ':':
			tokens = append(tokens, token{kind: tokenColon, text: ":", pos: i})
			i++
		case '+', '-', '*', '/', '%', '!', '<', '>', '=':
			op, next, err := readOperator(runes, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token{kind: tokenOperator, text: op, pos: i})
			i = next
		case '&':
			if i+1 < len(runes) && runes[i+1] == '&' {
				tokens = append(tokens, token{kind: tokenOperator, text: "&&", pos: i})
				i += 2
				continue
			}
			return nil, fmt.Errorf("位置 %d: 非法字符 &", i)
		case '|':
			if i+1 < len(runes) && runes[i+1] == '|' {
				tokens = append(tokens, token{kind: tokenOperator, text: "||", pos: i})
				i += 2
				continue
			}
			return nil, fmt.Errorf("位置 %d: 非法字符 |", i)
		default:
			return nil, fmt.Errorf("位置 %d: 非法字符 %q", i, ch)
		}
	}
	tokens = append(tokens, token{kind: tokenEOF, pos: len(runes)})
	return tokens, nil
}

// readString 读取字符串字面量。
// @param runes 字符列表
// @param start 起始位置
// @return 字符串值
// @return 下一位置
// @return 错误
func readString(runes []rune, start int) (string, int, error) {
	quote := runes[start]
	var builder strings.Builder
	for i := start + 1; i < len(runes); i++ {
		ch := runes[i]
		if ch == quote {
			return builder.String(), i + 1, nil
		}
		if ch != '\\' {
			builder.WriteRune(ch)
			continue
		}
		i++
		if i >= len(runes) {
			return "", 0, fmt.Errorf("字符串转义未结束")
		}
		switch runes[i] {
		case 'n':
			builder.WriteRune('\n')
		case 't':
			builder.WriteRune('\t')
		case 'r':
			builder.WriteRune('\r')
		case '\\', '\'', '"':
			builder.WriteRune(runes[i])
		default:
			builder.WriteRune(runes[i])
		}
	}
	return "", 0, fmt.Errorf("字符串未闭合")
}

// readOperator 读取运算符。
// @param runes 字符列表
// @param start 起始位置
// @return 运算符
// @return 下一位置
// @return 错误
func readOperator(runes []rune, start int) (string, int, error) {
	ch := runes[start]
	if start+1 < len(runes) {
		two := string(runes[start : start+2])
		switch two {
		case "==", "!=", "<=", ">=", "&&", "||":
			return two, start + 2, nil
		}
	}
	switch ch {
	case '+', '-', '*', '/', '%', '!', '<', '>':
		return string(ch), start + 1, nil
	case '=':
		return "", 0, fmt.Errorf("位置 %d: 不支持单等号，请使用 ==", start)
	default:
		return "", 0, fmt.Errorf("位置 %d: 非法运算符 %q", start, ch)
	}
}

// parser 是递归下降解析器。
type parser struct {
	tokens []token // 词法单元
	pos    int     // 当前位置
}

// peek 返回当前词法单元。
// @return 词法单元
func (p *parser) peek() token { return p.tokens[p.pos] }

// next 前进并返回当前词法单元。
// @return 词法单元
func (p *parser) next() token {
	current := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return current
}

// expect 断言当前词法单元。
// @param kind 期望类型
// @param text 期望文本，空串表示不校验
// @return 词法单元
// @return 错误
func (p *parser) expect(kind tokenKind, text string) (token, error) {
	current := p.peek()
	if current.kind != kind || (text != "" && current.text != text) {
		return token{}, fmt.Errorf("位置 %d: 期望 %q，实际 %q", current.pos, text, current.text)
	}
	return p.next(), nil
}

// parse 解析表达式。
// @return AST 根节点
// @return 解析错误
func (p *parser) parse() (node, error) { return p.parseTernary() }

// parseTernary 解析三元表达式。
// @return AST 节点
// @return 解析错误
func (p *parser) parseTernary() (node, error) {
	condition, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokenQuestion {
		return condition, nil
	}
	p.next()
	thenNode, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if _, err = p.expect(tokenColon, ":"); err != nil {
		return nil, err
	}
	otherwiseNode, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	return &ternaryNode{cond: condition, then: thenNode, otherwise: otherwiseNode}, nil
}

// parseOr 解析逻辑或。
// @return AST 节点
// @return 解析错误
func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		current := p.peek()
		if current.kind != tokenOperator || (current.text != "||" && current.text != "or") {
			return left, nil
		}
		p.next()
		right, parseErr := p.parseAnd()
		if parseErr != nil {
			return nil, parseErr
		}
		left = &binaryNode{op: current.text, left: left, right: right}
	}
}

// parseAnd 解析逻辑与。
// @return AST 节点
// @return 解析错误
func (p *parser) parseAnd() (node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for {
		current := p.peek()
		if current.kind != tokenOperator || (current.text != "&&" && current.text != "and") {
			return left, nil
		}
		p.next()
		right, parseErr := p.parseEquality()
		if parseErr != nil {
			return nil, parseErr
		}
		left = &binaryNode{op: current.text, left: left, right: right}
	}
}

// parseEquality 解析相等比较。
// @return AST 节点
// @return 解析错误
func (p *parser) parseEquality() (node, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for {
		current := p.peek()
		if current.kind != tokenOperator || (current.text != "==" && current.text != "!=" && current.text != "eq" && current.text != "ne") {
			return left, nil
		}
		p.next()
		right, parseErr := p.parseRelational()
		if parseErr != nil {
			return nil, parseErr
		}
		left = &binaryNode{op: current.text, left: left, right: right}
	}
}

// parseRelational 解析大小比较。
// @return AST 节点
// @return 解析错误
func (p *parser) parseRelational() (node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		current := p.peek()
		if current.kind != tokenOperator {
			return left, nil
		}
		switch current.text {
		case "<", ">", "<=", ">=", "lt", "gt", "le", "ge":
		default:
			return left, nil
		}
		p.next()
		right, parseErr := p.parseAdditive()
		if parseErr != nil {
			return nil, parseErr
		}
		left = &binaryNode{op: current.text, left: left, right: right}
	}
}

// parseAdditive 解析加减。
// @return AST 节点
// @return 解析错误
func (p *parser) parseAdditive() (node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for {
		current := p.peek()
		if current.kind != tokenOperator || (current.text != "+" && current.text != "-") {
			return left, nil
		}
		p.next()
		right, parseErr := p.parseMultiplicative()
		if parseErr != nil {
			return nil, parseErr
		}
		left = &binaryNode{op: current.text, left: left, right: right}
	}
}

// parseMultiplicative 解析乘除模。
// @return AST 节点
// @return 解析错误
func (p *parser) parseMultiplicative() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		current := p.peek()
		if current.kind != tokenOperator {
			return left, nil
		}
		switch current.text {
		case "*", "/", "%", "div", "mod":
		default:
			return left, nil
		}
		p.next()
		right, parseErr := p.parseUnary()
		if parseErr != nil {
			return nil, parseErr
		}
		left = &binaryNode{op: current.text, left: left, right: right}
	}
}

// parseUnary 解析一元运算。
// @return AST 节点
// @return 解析错误
func (p *parser) parseUnary() (node, error) {
	current := p.peek()
	if current.kind == tokenOperator && (current.text == "!" || current.text == "-" || current.text == "not") {
		p.next()
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryNode{op: current.text, child: child}, nil
	}
	if current.kind == tokenIdent && (current.text == "not" || current.text == "empty") {
		p.next()
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryNode{op: current.text, child: child}, nil
	}
	return p.parsePostfix()
}

// parsePostfix 解析属性、下标与函数调用。
// @return AST 节点
// @return 解析错误
func (p *parser) parsePostfix() (node, error) {
	primary, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tokenDot:
			p.next()
			name, expectErr := p.expect(tokenIdent, "")
			if expectErr != nil {
				return nil, expectErr
			}
			primary = &memberNode{target: primary, name: name.text}
		case tokenLBracket:
			p.next()
			index, parseErr := p.parseTernary()
			if parseErr != nil {
				return nil, parseErr
			}
			if _, expectErr := p.expect(tokenRBracket, "]"); expectErr != nil {
				return nil, expectErr
			}
			primary = &indexNode{target: primary, index: index}
		default:
			return primary, nil
		}
	}
}

// parsePrimary 解析基础表达式。
// @return AST 节点
// @return 解析错误
func (p *parser) parsePrimary() (node, error) {
	current := p.peek()
	switch current.kind {
	case tokenNumber:
		p.next()
		value, err := strconv.ParseFloat(current.text, 64)
		if err != nil {
			return nil, fmt.Errorf("位置 %d: 非法数字 %q", current.pos, current.text)
		}
		return &literalNode{value: value}, nil
	case tokenString:
		p.next()
		return &literalNode{value: current.text}, nil
	case tokenIdent:
		p.next()
		switch current.text {
		case "true":
			return &literalNode{value: true}, nil
		case "false":
			return &literalNode{value: false}, nil
		case "null":
			return &literalNode{value: nil}, nil
		}
		if p.peek().kind == tokenLParen {
			args, err := p.parseArgs()
			if err != nil {
				return nil, err
			}
			return &callNode{name: current.text, args: args}, nil
		}
		return &identNode{name: current.text}, nil
	case tokenLParen:
		p.next()
		inner, err := p.parseTernary()
		if err != nil {
			return nil, err
		}
		if _, err = p.expect(tokenRParen, ")"); err != nil {
			return nil, err
		}
		return inner, nil
	default:
		return nil, fmt.Errorf("位置 %d: 意外符号 %q", current.pos, current.text)
	}
}

// parseArgs 解析函数参数。
// @return 参数列表
// @return 解析错误
func (p *parser) parseArgs() ([]node, error) {
	if _, err := p.expect(tokenLParen, "("); err != nil {
		return nil, err
	}
	args := make([]node, 0)
	if p.peek().kind == tokenRParen {
		p.next()
		return args, nil
	}
	for {
		arg, err := p.parseTernary()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.peek().kind == tokenRParen {
			p.next()
			return args, nil
		}
		if _, err = p.expect(tokenComma, ","); err != nil {
			return nil, err
		}
	}
}
