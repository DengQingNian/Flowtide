// Package schema 提供 Flowable 8.1 ACT_* Schema 与 workflow 二次封装 Schema 的嵌入式 SQL 资产。
package schema

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// Version 是数据库 Schema 版本，与 Flowable 8.1.0-SNAPSHOT 对齐。
const Version = "8.1.0.3"

// 内置脚本文件名，EnsureSchema 按文件名序执行（001 引擎表 → 002 workflow 表）。
const (
	// EngineFile 是 Flowable 8.1 引擎表脚本（001）。
	EngineFile = "001-flowable-8.1.sql"
	// WorkflowFile 是 workflow 二次封装表脚本（002）。
	WorkflowFile = "002-workflow-1.0.sql"
)

//go:embed sql/*/*.sql
var assets embed.FS

var tablePattern = regexp.MustCompile(`(?i)create\s+table\s+([a-zA-Z0-9_]+)`)

// 约束声明前缀，解析列时跳过
var constraintPrefixes = []string{"primary", "constraint", "unique", "foreign", "check", "key", "index"}

// Dialects 返回支持的数据库方言。
// @return 方言列表
func Dialects() []string { return []string{"mysql", "postgres"} }

// Files 返回方言包含的全部脚本文件名，按文件名升序排列。
// @param dialect 数据库方言
// @return 文件名列表
// @return 读取错误
func Files(dialect string) ([]string, error) {
	dir, err := dialectDir(dialect)
	if err != nil {
		return nil, err
	}
	entries, readErr := fs.ReadDir(assets, dir)
	if readErr != nil {
		return nil, api.NewError("SCHEMA-0004", "读取 Schema 目录失败: "+dialect, readErr)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	// 按文件名升序保证 001 引擎表先于 002 workflow 表执行
	sort.Strings(names)
	return names, nil
}

// Load 返回指定方言的引擎（001）Schema SQL，保持既有校验和语义不变。
// @param dialect 数据库方言
// @return SQL 内容
// @return 加载错误
func Load(dialect string) (string, error) {
	return LoadFile(dialect, EngineFile)
}

// LoadFile 返回指定方言单个脚本文件的内容。
// @param dialect 数据库方言
// @param file 脚本文件名（如 002-workflow-1.0.sql）
// @return SQL 内容
// @return 加载错误
func LoadFile(dialect, file string) (string, error) {
	dir, err := dialectDir(dialect)
	if err != nil {
		return "", err
	}
	if !validFileName(file) {
		return "", api.NewError("SCHEMA-0002", fmt.Sprintf("非法脚本文件名: %s", file), api.ErrBadRequest)
	}
	data, readErr := assets.ReadFile(dir + "/" + file)
	if readErr != nil {
		return "", api.NewError("SCHEMA-0001", "读取 Schema 失败: "+dialect+"/"+file, readErr)
	}
	return string(data), nil
}

// Tables 返回指定方言引擎（001）脚本包含的表名。
// @param dialect 数据库方言
// @return 表名列表
// @return 解析错误
func Tables(dialect string) ([]string, error) {
	return FileTables(dialect, EngineFile)
}

// FileTables 解析指定脚本包含的表名，统一返回大写并排序。
// @param dialect 数据库方言
// @param file 脚本文件名
// @return 表名列表
// @return 解析错误
func FileTables(dialect, file string) ([]string, error) {
	content, err := LoadFile(dialect, file)
	if err != nil {
		return nil, err
	}
	return parseTables(content), nil
}

// TableColumns 返回指定表的列名与列类型（在全部脚本中检索）。
// 列名与类型保持 DDL 中的顺序，便于管理端点展示。
// @param dialect 数据库方言
// @param table 表名
// @return 列名列表
// @return 列类型列表
// @return 查询错误
func TableColumns(dialect, table string) ([]string, []string, error) {
	name := strings.ToUpper(strings.TrimSpace(table))
	files, err := Files(dialect)
	if err != nil {
		return nil, nil, err
	}
	for _, file := range files {
		content, loadErr := LoadFile(dialect, file)
		if loadErr != nil {
			return nil, nil, loadErr
		}
		// 表体必须以 ")" 结尾（可带 ENGINE 等表选项）后接分号，避免吞并后续语句
		pattern := regexp.MustCompile(`(?is)create\s+table\s+` + regexp.QuoteMeta(name) + `\s*\((.*?)\)\s*(?:engine[^;]*)?;`)
		match := pattern.FindStringSubmatch(content)
		if match == nil {
			continue
		}
		return parseColumns(match[1])
	}
	return nil, nil, api.NewError("SCHEMA-0003", "表不存在: "+name, api.ErrNotFound)
}

// parseColumns 解析表体为列名与类型列表。
// @param body CREATE TABLE 括号内容
// @return 列名列表
// @return 列类型列表
// @return 解析错误
func parseColumns(body string) ([]string, []string, error) {
	definitions := splitColumnDefinitions(body)
	columns := make([]string, 0, len(definitions))
	types := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		fields := strings.Fields(strings.TrimSpace(definition))
		if len(fields) < 2 {
			continue
		}
		// 跳过主键、唯一约束、外键等声明
		if isConstraint(fields[0]) {
			continue
		}
		columns = append(columns, strings.ToUpper(strings.Trim(fields[0], "`\"")))
		types = append(types, strings.ToLower(fields[1]))
	}
	return columns, types, nil
}

// Checksum 返回引擎（001）Schema SQL 的 SHA-256，既有库的升级校验依赖该值稳定。
// @param dialect 数据库方言
// @return 十六进制校验和
// @return 计算错误
func Checksum(dialect string) (string, error) {
	return ChecksumFile(dialect, EngineFile)
}

// ChecksumFile 返回指定脚本文件的 SHA-256，workflow 脚本与引擎脚本的校验和相互独立。
// @param dialect 数据库方言
// @param file 脚本文件名
// @return 十六进制校验和
// @return 计算错误
func ChecksumFile(dialect, file string) (string, error) {
	content, err := LoadFile(dialect, file)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:]), nil
}

// parseTables 从 SQL 内容解析全部表名，统一大写并排序。
// @param content SQL 内容
// @return 表名列表
func parseTables(content string) []string {
	matches := tablePattern.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	tables := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.ToUpper(match[1])
		if seen[name] {
			continue
		}
		seen[name] = true
		tables = append(tables, name)
	}
	sort.Strings(tables)
	return tables
}

// dialectDir 返回方言对应的嵌入式 SQL 目录。
// @param dialect 数据库方言
// @return 目录路径
// @return 方言错误
func dialectDir(dialect string) (string, error) {
	switch strings.ToLower(dialect) {
	case "mysql":
		return "sql/mysql", nil
	case "postgres", "postgresql":
		return "sql/postgres", nil
	default:
		return "", api.NewError("SCHEMA-0002", fmt.Sprintf("不支持的数据库方言: %s", dialect), api.ErrBadRequest)
	}
}

// validFileName 校验脚本文件名，防止路径穿越与非法字符。
// @param file 文件名
// @return 是否合法
func validFileName(file string) bool {
	if file == "" || strings.ContainsAny(file, "/\\") || strings.Contains(file, "..") {
		return false
	}
	return strings.HasSuffix(file, ".sql") && file == strings.TrimSpace(file)
}

// splitColumnDefinitions 按顶层逗号拆分列定义。
// 括号内逗号（如 numeric(10,2)）不拆分。
// @param body CREATE TABLE 括号内容
// @return 列定义列表
func splitColumnDefinitions(body string) []string {
	definitions := make([]string, 0)
	depth := 0
	start := 0
	for index, char := range body {
		switch char {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			// 仅在顶层逗号处切分列定义
			if depth == 0 {
				definitions = append(definitions, body[start:index])
				start = index + 1
			}
		}
	}
	if strings.TrimSpace(body[start:]) != "" {
		definitions = append(definitions, body[start:])
	}
	return definitions
}

// isConstraint 判断列定义是否为约束声明。
// @param token 首字段
// @return 是否为约束
func isConstraint(token string) bool {
	lower := strings.ToLower(token)
	for _, prefix := range constraintPrefixes {
		if lower == prefix {
			return true
		}
	}
	return false
}
