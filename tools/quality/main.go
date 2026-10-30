// Command quality 执行仓库质量门禁。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DengQingNian/Flowtide/internal/quality"
)

// main 检查覆盖率，失败时以非零状态退出。
// @return 无
func main() {
	documentation := flag.Bool("doccheck", false, "检查中文函数注释")
	profile := flag.String("coverage", "coverage.out", "Go 覆盖率文件")
	minimum := flag.Float64("min", .9, "最低语句覆盖率")
	flag.Parse()
	if *documentation {
		failures, err := quality.CheckDocumentation(".")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, failure)
		}
		if len(failures) > 0 {
			os.Exit(1)
		}
		fmt.Println("中文函数注释门禁通过")
		return
	}
	file, err := os.Open(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	ignored, err := os.ReadFile(".coverignore")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	prefixes := make([]string, 0)
	for _, line := range strings.Split(string(ignored), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			prefixes = append(prefixes, line)
		}
	}
	report, err := quality.CheckCoverage(file, prefixes, *minimum)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("语句覆盖率门禁通过：%d/%d = %.4f%%，总量及每个包阈值 %.2f%%\n", report.Covered, report.Total, report.Ratio()*100, *minimum*100)
}
