// Command flowable-migrate 输出 Schema SQL 或初始化 MySQL 开发库。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
	"github.com/flowable-go/flowable/internal/persistence/schema"
)

// main 解析参数并输出 Schema 或执行 MySQL 初始化。
// @return 无
func main() {
	dialect := flag.String("dialect", "mysql", "数据库方言：mysql 或 postgres")
	output := flag.String("out", "", "输出文件路径，为空时输出到标准输出")
	listTables := flag.Bool("tables", false, "仅列出表名")
	apply := flag.Bool("apply", false, "连接 MySQL 并初始化或校验 Schema")
	envPath := flag.String("env", ".env", "MySQL 开发配置文件路径")
	flag.Parse()

	// 执行模式与 SQL 导出互斥，防止命令含义不明确。
	if *apply {
		if *dialect != "mysql" || *listTables || *output != "" {
			fmt.Fprintln(os.Stderr, "-apply 仅支持 mysql，且不能与 -tables、-out 同时使用")
			os.Exit(1)
		}
		if err := applyMySQL(*envPath); err != nil {
			fmt.Fprintln(os.Stderr, "初始化 MySQL 失败:", err)
			os.Exit(1)
		}
		fmt.Println("MySQL Schema 初始化及校验完成，版本", schema.Version)
		return
	}
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

// applyMySQL 读取配置并在有限时间内完成开发库初始化。
// @param envPath 开发配置文件路径
// @return 配置、连接或迁移错误
func applyMySQL(envPath string) error {
	cfg, err := mysqlstore.LoadConfig(envPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return mysqlstore.EnsureSchema(ctx, db, true)
}
