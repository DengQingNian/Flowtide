// Package schema 提供 Flowable 8.1 ACT_* Schema 的嵌入式 SQL 资产。
package schema

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/DengQingNian/Flowtide/api"
)

// Version 是数据库 Schema 版本，与 Flowable 8.1.0-SNAPSHOT 对齐。
const Version = "8.1.0.3"

//go:embed sql/*/*.sql
var assets embed.FS

var tablePattern = regexp.MustCompile(`(?i)create\s+table\s+([a-zA-Z0-9_]+)`)

// 约束声明前缀，解析列时跳过
var constraintPrefixes = []string{"primary", "constraint", "unique", "foreign", "check", "key", "index"}

// Dialects 返回支持的数据库方言。
// @return 方言列表
func Dialects() []string { return []string{"mysql", "postgres"} }

// Load 返回指定方言的完整 Schema SQL。
// @param dialect 数据库方言
// @return SQL 内容
// @return 加载错误
func Load(dialect string) (string, error) {
	path, err := schemaPath(dialect)
	if err != nil {
		return "", err
	}
	data, readErr := assets.ReadFile(path)
	if readErr != nil {
		return "", api.NewError("SCHEMA-0001", "读取 Schema 失败: "+dialect, readErr)
	}
	return string(data), nil
}

// Tables 返回指定方言包含的表名。
// @param dialect 数据库方言
// @return 表名列表
// @return 解析错误
func Tables(dialect string) ([]string, error) {
	content, err := Load(dialect)
	if err != nil {
		return nil, err
	}
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
	return tables, nil
}

// TableColumns 返回指定表的列名与列类型。
// 列名与类型保持 DDL 中的顺序，便于管理端点展示。
// @param dialect 数据库方言
// @param table 表名
// @return 列名列表
// @return 列类型列表
// @return 查询错误
func TableColumns(dialect, table string) ([]string, []string, error) {
	content, err := Load(dialect)
	if err != nil {
		return nil, nil, err
	}
	name := strings.ToUpper(strings.TrimSpace(table))
	pattern := regexp.MustCompile(`(?is)create\s+table\s+` + regexp.QuoteMeta(name) + `\s*\((.*?)\)\s*;`)
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		return nil, nil, api.NewError("SCHEMA-0003", "表不存在: "+name, api.ErrNotFound)
	}
	definitions := splitColumnDefinitions(match[1])
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
		columns = append(columns, strings.ToUpper(fields[0]))
		types = append(types, strings.ToLower(fields[1]))
	}
	return columns, types, nil
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

// Checksum 返回 Schema SQL 的 SHA-256。
// @param dialect 数据库方言
// @return 十六进制校验和
// @return 计算错误
func Checksum(dialect string) (string, error) {
	content, err := Load(dialect)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:]), nil
}

// schemaPath 返回方言对应的 SQL 路径。
// @param dialect 数据库方言
// @return 路径
// @return 错误
func schemaPath(dialect string) (string, error) {
	switch strings.ToLower(dialect) {
	case "mysql":
		return "sql/mysql/001-flowable-8.1.sql", nil
	case "postgres", "postgresql":
		return "sql/postgres/001-flowable-8.1.sql", nil
	default:
		return "", api.NewError("SCHEMA-0002", fmt.Sprintf("不支持的数据库方言: %s", dialect), api.ErrBadRequest)
	}
}
