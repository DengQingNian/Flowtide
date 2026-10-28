// Package quality 提供可重复执行的覆盖率与文档质量门禁。
package quality

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Coverage 是语句覆盖统计。
type Coverage struct {
	Covered int // 已覆盖语句
	Total   int // 全部语句
}

// Ratio 返回覆盖率；没有可执行语句时视为完全覆盖。
// @return 覆盖率
func (c Coverage) Ratio() float64 {
	if c.Total == 0 {
		return 1
	}
	return float64(c.Covered) / float64(c.Total)
}

// CheckCoverage 读取 Go 覆盖率文件，按语句权重检查总量及每个包。
// @param reader 覆盖率文件
// @param excluded 明确排除的路径前缀
// @param minimum 最低比例
// @return 总覆盖统计
// @return 格式或阈值错误
func CheckCoverage(reader io.Reader, excluded []string, minimum float64) (Coverage, error) {
	if minimum < 0 || minimum > 1 {
		return Coverage{}, fmt.Errorf("覆盖率阈值必须介于零和一之间")
	}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "mode: ") {
		return Coverage{}, fmt.Errorf("覆盖率文件缺少模式头")
	}
	packages := make(map[string]Coverage)
	total := Coverage{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return total, fmt.Errorf("覆盖率记录格式错误")
		}
		ignored := false
		for _, prefix := range excluded {
			if prefix != "" && strings.HasPrefix(fields[0], prefix) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil || statements < 0 {
			return total, fmt.Errorf("覆盖率语句数量无效")
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return total, fmt.Errorf("覆盖率命中数量无效")
		}
		index := strings.LastIndex(fields[0], "/")
		if index < 0 {
			return total, fmt.Errorf("覆盖率包路径无效")
		}
		key := fields[0][:index]
		current := packages[key]
		current.Total += statements
		total.Total += statements
		if count > 0 {
			current.Covered += statements
			total.Covered += statements
		}
		packages[key] = current
	}
	if err := scanner.Err(); err != nil {
		return total, err
	}
	if total.Total == 0 {
		return total, fmt.Errorf("没有可检查的覆盖率数据")
	}
	// 总量和包阈值均检查，防止大包掩盖关键小包未覆盖的代码。
	if total.Ratio() < minimum {
		return total, fmt.Errorf("总语句覆盖率 %.4f%% 低于 %.2f%%", total.Ratio()*100, minimum*100)
	}
	for name, current := range packages {
		if current.Ratio() < minimum {
			return total, fmt.Errorf("包 %s 语句覆盖率 %.4f%% 低于 %.2f%%", name, current.Ratio()*100, minimum*100)
		}
	}
	return total, nil
}
