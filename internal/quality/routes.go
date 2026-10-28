package quality

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// RouteReport 对照官方规范和实现生成以方法及规范化路径为主键的清单。
// @param specification 已固定的上游规范
// @param implementation 路由源代码
// @param differences 已声明的部分支持差异
// @return 可重复生成的中文端点表
func RouteReport(specification, implementation string, differences map[string]string) string {
	official := make(map[string]string)
	implemented := make(map[string]string)
	pathPattern := regexp.MustCompile(`^  (/[^:]+):\s*$`)
	methodPattern := regexp.MustCompile(`^    (get|post|put|delete|patch|head|options):`)
	routePattern := regexp.MustCompile(`"(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS) /service([^"\s]+)"`)
	variable := regexp.MustCompile(`\{[^}]+\}`)
	// 方法和路径参数统一后逐项比较，参数名差异不计作缺失端点。
	current := ""
	for _, line := range strings.Split(specification, "\n") {
		if match := pathPattern.FindStringSubmatch(line); match != nil {
			current = match[1]
		}
		if match := methodPattern.FindStringSubmatch(line); match != nil && current != "" {
			method := strings.ToUpper(match[1])
			official[method+" "+variable.ReplaceAllString(current, "{}")] = current
		}
	}
	for _, match := range routePattern.FindAllStringSubmatch(implementation, -1) {
		implemented[match[1]+" "+variable.ReplaceAllString(match[2], "{}")] = match[2]
	}
	keys := make([]string, 0)
	for key := range official {
		keys = append(keys, key)
	}
	for key := range implemented {
		if _, exists := official[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var output strings.Builder
	fmt.Fprintf(&output, "# 004 REST 端点对照\n\n> 自动生成：`make contract-update`；校验：`make contract`。以方法及规范化路径为主键。\n> 官方方法端点 %d；实现方法端点 %d。已编码表示路由存在，尚未证明响应模式和 Java 行为兼容。\n\n| 方法与规范化路径 | 官方路径 | 实现路径 | 状态与限制 |\n| --- | --- | --- | --- |\n", len(official), len(implemented))
	for _, key := range keys {
		source, present := official[key]
		target, exists := implemented[key]
		status := "已编码；待契约与 Java 行为验收"
		if !present {
			status = "实现扩展；不计入官方覆盖"
		} else if !exists {
			status = "缺失；迁移验收阻断"
		}
		if partial, ok := differences[key]; ok && exists {
			status = partial
		}
		fmt.Fprintf(&output, "| `%s` | `%s` | `%s` | %s |\n", key, source, target, status)
	}
	return output.String()
}
