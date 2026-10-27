// Command flowable-migrate 输出 Flowable ACT_* Schema SQL。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/flowable-go/flowable/internal/persistence/schema"
)

// main 解析参数并输出 Schema。
// @return 无
func main() {
	dialect := flag.String("dialect", "mysql", "数据库方言：mysql 或 postgres")
	output := flag.String("out", "", "输出文件路径，为空时输出到标准输出")
	listTables := flag.Bool("tables", false, "仅列出表名")
	flag.Parse()

	if *listTables {
		tables, err := schema.Tables(*dialect)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取表名失败:", err)
			os.Exit(1)
		}
		fmt.Println(strings.Join(tables, "\n"))
		return
	}
	content, err := schema.Load(*dialect)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 Schema 失败:", err)
		os.Exit(1)
	}
	if *output == "" {
		fmt.Print(content)
		return
	}
	if err := os.WriteFile(*output, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "写入 Schema 失败:", err)
		os.Exit(1)
	}
	fmt.Println("已写入", *output)
}
