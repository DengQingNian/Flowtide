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

	"github.com/flowable-go/flowable/api"
)

// Version 是数据库 Schema 版本，与 Flowable 8.1.0-SNAPSHOT 对齐。
const Version = "8.1.0.3"

//go:embed sql/*/*.sql
var assets embed.FS

var tablePattern = regexp.MustCompile(`(?i)create\s+table\s+([a-zA-Z0-9_]+)`)

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
