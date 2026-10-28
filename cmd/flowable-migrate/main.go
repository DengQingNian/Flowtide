// Command flowable-migrate 输出 Schema、初始化开发库、只读预检或离线准备 Go 状态文件。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flowable-go/flowable/internal/migration"
	mysqlstore "github.com/flowable-go/flowable/internal/persistence/mysql"
	"github.com/flowable-go/flowable/internal/persistence/schema"
	_ "github.com/lib/pq"
)

// commandDependencies 提供命令使用的数据库操作与报告分析入口。
type commandDependencies struct {
	scan    func(context.Context, string, string) (migration.SourceSnapshot, error)                              // 只读扫描
	analyze func(migration.SourceSnapshot, migration.Options) migration.Report                                   // 风险分析
	convert func(context.Context, migration.SourceSnapshot, migration.Options) ([]byte, migration.Report, error) // 离线状态转换
	apply   func(string) error                                                                                   // 开发库初始化
}

// main 运行迁移工具并将结果映射为进程退出码。
// @return 无
func main() {
	os.Exit(runWithDependencies(os.Args[1:], os.Stdout, os.Stderr, commandDependencies{scan: scanSource, analyze: migration.Analyze, convert: migration.Convert, apply: applyMySQL}))
}

// runWithDependencies 校验参数并执行所选模式，预检阻断返回二。
// @param args 命令参数
// @param stdout 标准输出
// @param stderr 错误输出
// @param dependencies 数据库与分析依赖
// @return 退出码，零成功、一操作失败、二预检阻断
func runWithDependencies(args []string, stdout, stderr io.Writer, dependencies commandDependencies) int {
	flags := flag.NewFlagSet("flowable-migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dialect := flags.String("dialect", "mysql", "数据库方言：mysql 或 postgres")
	output := flags.String("out", "", "输出文件路径，为空时输出到标准输出")
	listTables := flags.Bool("tables", false, "仅列出表名")
	apply := flags.Bool("apply", false, "连接 MySQL 并初始化或校验 Schema")
	preflight := flags.Bool("preflight", false, "只读扫描既有 Java 数据库并输出迁移预检 JSON")
	convertMode := flags.Bool("convert", false, "只读源库并离线准备 Go 状态文件，必须指定不存在的 -out；不协调停写或执行者切换")
	envPath := flags.String("env", ".env", "MySQL 开发配置文件路径")
	tenant := flags.String("tenant", "", "预检或转换租户，不提供时检查全部，显式空值检查默认租户")
	timeout := flags.Duration("timeout", 5*time.Minute, "只读预检或离线转换超时时间")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	tenantSet := false
	timeoutSet := false
	flags.Visit(func(value *flag.Flag) {
		if value.Name == "tenant" {
			tenantSet = true
		}
		if value.Name == "timeout" {
			timeoutSet = true
		}
	})
	// 在任何连接和写文件之前检查模式互斥、租户范围与输出位置。
	if flags.NArg() != 0 || (*dialect != "mysql" && *dialect != "postgres") || (*preflight && (*apply || *listTables || *convertMode)) || (*convertMode && (*apply || *listTables || *output == "")) || (*apply && (*dialect != "mysql" || *listTables || *output != "")) || ((tenantSet || timeoutSet) && !*preflight && !*convertMode) || ((*preflight || *convertMode) && *timeout <= 0) {
		fmt.Fprintln(stderr, "参数无效：请检查方言、互斥模式、租户范围和超时时间")
		return 1
	}
	if (*preflight || *convertMode) && *output != "" {
		if err := validateReportPath(*output, *envPath); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if *convertMode {
			if _, err := os.Lstat(*output); err == nil || !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintln(stderr, "离线状态输出目标已存在或无法访问，必须选择新的文件路径")
				return 1
			}
		}
	}
	if *preflight || *convertMode {
		options := migration.Options{}
		if tenantSet {
			options.TenantID = tenant
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		snapshot, err := dependencies.scan(ctx, *envPath, *dialect)
		if err != nil {
			fmt.Fprintln(stderr, "只读预检扫描失败，请检查数据库配置、只读权限、连接与源表结构")
			return 1
		}
		if *convertMode {
			return runConversion(ctx, snapshot, options, *output, stdout, stderr, dependencies)
		}
		report := dependencies.analyze(snapshot, options)
		content, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, "生成预检报告失败")
			return 1
		}
		content = append(content, '\n')
		if *output != "" {
			err = writeReport(*output, content)
		} else {
			_, err = stdout.Write(content)
		}
		if err != nil {
			fmt.Fprintln(stderr, "写入预检报告失败")
			return 1
		}
		if report.Blocked {
			return 2
		}
		return 0
	}
	if *apply {
		if err := dependencies.apply(*envPath); err != nil {
			fmt.Fprintln(stderr, "初始化 MySQL 失败，请检查配置、连接与 Schema")
			return 1
		}
		fmt.Fprintln(stdout, "MySQL Schema 初始化及校验完成，版本", schema.Version)
		return 0
	}
	if *listTables {
		tables, err := schema.Tables(*dialect)
		if err != nil {
			fmt.Fprintln(stderr, "读取表名失败:", err)
			return 1
		}
		fmt.Fprintln(stdout, strings.Join(tables, "\n"))
		return 0
	}
	content, err := schema.Load(*dialect)
	if err != nil {
		fmt.Fprintln(stderr, "读取 Schema 失败:", err)
		return 1
	}
	if *output == "" {
		fmt.Fprint(stdout, content)
		return 0
	}
	if err := os.WriteFile(*output, []byte(content), 0644); err != nil {
		fmt.Fprintln(stderr, "写入 Schema 失败:", err)
		return 1
	}
	fmt.Fprintln(stdout, "已写入", *output)
	return 0
}

// runConversion 分析并转换源快照，仅在未阻断时发布新的私有状态文件。
// @param ctx 转换上下文
// @param snapshot 源库只读快照
// @param options 租户范围
// @param output 新状态文件路径
// @param stdout 报告输出
// @param stderr 错误输出
// @param dependencies 转换依赖
// @return 退出码，零成功、一失败、二预检阻断
func runConversion(ctx context.Context, snapshot migration.SourceSnapshot, options migration.Options, output string, stdout, stderr io.Writer, dependencies commandDependencies) int {
	state, report, err := dependencies.convert(ctx, snapshot, options)
	if err != nil {
		fmt.Fprintln(stderr, "离线状态转换失败；未执行源库写入，请检查来源结构与转换支持范围")
		return 1
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "生成转换报告失败")
		return 1
	}
	content = append(content, '\n')
	// 阻断报告不产生状态；通过分析也不能替代停写协调与 Java 续跑验收。
	if !report.Blocked {
		if len(state) == 0 || ctx.Err() != nil {
			fmt.Fprintln(stderr, "离线状态为空或转换已超时，未发布状态文件")
			return 1
		}
		if err := writeNewState(output, state); err != nil {
			fmt.Fprintln(stderr, "创建离线状态文件失败，目标可能已存在；已有文件未被覆盖")
			return 1
		}
	}
	if _, err := stdout.Write(content); err != nil {
		fmt.Fprintln(stderr, "输出转换报告失败；如状态已创建，请检查指定输出文件")
		return 1
	}
	if report.Blocked {
		return 2
	}
	return 0
}

// validateReportPath 拒绝配置文件及其链接作为报告输出，并检查父目录。
// @param output 报告路径
// @param envPath 配置文件路径
// @return 路径错误
func validateReportPath(output, envPath string) error {
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return errors.New("预检报告路径无效")
	}
	envAbsolute, err := filepath.Abs(envPath)
	if err != nil {
		return errors.New("配置文件路径无效")
	}
	// 显式配置、默认配置及硬链接均不能被报告替换。
	if outputPath == envAbsolute || filepath.Base(outputPath) == ".env" {
		return errors.New("预检报告不能覆盖数据库配置文件")
	}
	outputInfo, outputErr := os.Stat(outputPath)
	envInfo, envErr := os.Stat(envAbsolute)
	if outputErr == nil && envErr == nil && os.SameFile(outputInfo, envInfo) {
		return errors.New("预检报告不能覆盖数据库配置文件")
	}
	if outputErr != nil && !errors.Is(outputErr, os.ErrNotExist) {
		return errors.New("无法访问预检报告路径")
	}
	if outputErr == nil && !outputInfo.Mode().IsRegular() {
		return errors.New("预检报告路径必须是普通文件")
	}
	directory, err := os.Stat(filepath.Dir(outputPath))
	if err != nil || !directory.IsDir() {
		return errors.New("预检报告父目录不存在")
	}
	return nil
}

// writeReport 以私有权限将完整报告原子替换至目标路径。
// @param path 报告目标路径
// @param content 完整报告内容
// @return 创建、写入、同步或替换错误
func writeReport(path string, content []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".flowable-preflight-*")
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	// 临时文件与目标同目录，确保成功写入后才能替换旧报告。
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// writeNewState 完整写入私有临时文件后原子创建目标，任何已有目标均不覆盖。
// @param path 新状态文件路径
// @param content 完整状态内容
// @return 创建、写入、同步或发布错误
func writeNewState(path string, content []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".flowable-convert-*")
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	// 同目录硬链接仅在目标不存在时成功，消除检查和发布之间的覆盖竞争。
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Link(file.Name(), path)
}

// scanSource 只连接源数据库，扫描器负责建立一致性只读事务。
// @param ctx 扫描截止上下文
// @param envPath MySQL 配置路径
// @param dialect 数据库方言
// @return 源数据库快照
// @return 不含连接凭据的扫描错误
func scanSource(ctx context.Context, envPath, dialect string) (migration.SourceSnapshot, error) {
	var db *sql.DB
	var err error
	// 各方言只建立连接；预检路径不能调用 Schema 初始化。
	if dialect == "mysql" {
		cfg, configErr := mysqlstore.LoadConfig(envPath)
		if configErr != nil {
			return migration.SourceSnapshot{}, errors.New("读取 MySQL 配置失败")
		}
		db, err = mysqlstore.Open(ctx, cfg)
	} else {
		dsn := os.Getenv("FLOWABLE_POSTGRES_DSN")
		if dsn == "" {
			return migration.SourceSnapshot{}, errors.New("缺少 PostgreSQL 连接配置")
		}
		db, err = sql.Open("postgres", dsn)
	}
	if err != nil {
		return migration.SourceSnapshot{}, errors.New("连接源数据库失败")
	}
	defer func() { _ = db.Close() }()
	snapshot, err := migration.Scan(ctx, db, dialect)
	if err != nil {
		return migration.SourceSnapshot{}, errors.New("读取源数据库失败")
	}
	return snapshot, nil
}

// applyMySQL 读取配置并在有限时间内完成开发库初始化。
// @param envPath 开发配置文件路径
// @return 配置、连接或迁移错误
func applyMySQL(envPath string) error {
	// 开发库初始化保留独立入口，避免与只读扫描路径交叉。
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
