// Command contract 校验固定兼容资产与方法级端点清单。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DengQingNian/Flowtide/internal/quality"
)

// main 校验基线并检查或更新端点表，错误时返回非零退出码。
// @return 无
func main() {
	write := flag.Bool("write", false, "更新端点对照表")
	flag.Parse()
	err := run(*write)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("兼容基线与端点清单检查通过；行为契约仍须独立验收")
}

// run 执行确定性制品与路由检查。
// @param write 是否写入生成表
// @return 校验或读写错误
func run(write bool) error {
	if err := quality.CheckBaseline("."); err != nil {
		return err
	}
	spec, err := os.ReadFile("testdata/compatibility/flowable-swagger-process.yaml")
	if err != nil {
		return err
	}
	source, err := os.ReadFile("internal/rest/server.go")
	if err != nil {
		return err
	}
	data, err := os.ReadFile("testdata/compatibility/differences.json")
	if err != nil {
		return err
	}
	differences := map[string]string{}
	if err := json.Unmarshal(data, &differences); err != nil {
		return err
	}
	report := quality.RouteReport(string(spec), string(source), differences)
	path := "docs/003-迁移审计/004-REST端点对照.md"
	if write {
		return os.WriteFile(path, []byte(report), 0644)
	}
	previous, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(previous) != report {
		return fmt.Errorf("端点对照表已过期，请执行 make contract-update 并审核差异")
	}
	return nil
}
