// Package xml 提供 BPMN XML 的安全解码与模型转换。
package xml

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Node 是保留命名空间与顺序的通用 XML 节点。
type Node struct {
	Name     xml.Name   // 元素名
	Attrs    []xml.Attr // 属性
	Children []*Node    // 子节点
	Text     string     // 文本内容
}

// Decode 将 XML 解码为通用节点树。
// @param r 输入流
// @param limit 最大字节数
// @return 根节点
// @return 解码错误
func Decode(r io.Reader, limit int64) (*Node, error) {
	decoder := xml.NewDecoder(io.LimitReader(r, limit))
	var root *Node
	stack := make([]*Node, 0, 16)
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析 XML 失败: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			// 防止恶意深嵌套导致栈溢出
			if depth > 200 {
				return nil, fmt.Errorf("XML 嵌套深度超过限制")
			}
			node := &Node{Name: t.Name, Attrs: append([]xml.Attr(nil), t.Attr...)}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("XML 存在多个根元素")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("XML 结束标签不匹配")
			}
			stack = stack[:len(stack)-1]
			depth--
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		case xml.Directive:
			// 拒绝 DTD，避免实体膨胀与外部实体风险
			if strings.Contains(strings.ToUpper(string(t)), "DOCTYPE") {
				return nil, fmt.Errorf("不允许 DTD 声明")
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("XML 为空")
	}
	return root, nil
}

// Child 返回第一个指定本地名的子节点。
// @param local 本地名
// @return 子节点
// @return 是否存在
func (n *Node) Child(local string) (*Node, bool) {
	for _, child := range n.Children {
		if child.Name.Local == local {
			return child, true
		}
	}
	return nil, false
}

// ChildrenNamed 返回全部指定本地名的子节点。
// @param local 本地名
// @return 子节点列表
func (n *Node) ChildrenNamed(local string) []*Node {
	out := make([]*Node, 0)
	for _, child := range n.Children {
		if child.Name.Local == local {
			out = append(out, child)
		}
	}
	return out
}

// Attr 返回第一个指定本地名的属性值。
// @param local 本地名
// @return 属性值，不存在时为空串
func (n *Node) Attr(local string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

// AttrNS 返回指定命名空间与本地名的属性值。
// @param space 命名空间 URI
// @param local 本地名
// @return 属性值，不存在时为空串
func (n *Node) AttrNS(space, local string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Space == space && attr.Name.Local == local {
			return attr.Value
		}
	}
	return ""
}

// TextContent 返回节点及其子节点的文本内容并去除首尾空白。
// @return 文本内容
func (n *Node) TextContent() string {
	var builder strings.Builder
	builder.WriteString(n.Text)
	for _, child := range n.Children {
		builder.WriteString(child.TextContent())
	}
	return strings.TrimSpace(builder.String())
}
