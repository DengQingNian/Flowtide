package schema

import (
	"strings"
	"testing"
)

// 引擎（001）脚本的校验和基准：一旦变化说明 001 被改动，既有库的升级校验会被破坏。
// 基准值 = 001 脚本原始内容的 SHA-256（自 0644acf 引入后 001 文件从未改动）。
// 57973e4 / 1206d0a 两次"重算基准"填入的值与任一 001 文件均不匹配，属误改；016 号
// 审计记录已把基准恢复为经实测确认的真实值。
const (
	engineChecksumMySQL    = "b684b4038eab32a2c08983fb54216072f954c424117f5c6b57563a9ddb941ae4"
	engineChecksumPostgres = "6a0feefe5383945366893e0c5395785eb2abd1f04225c99f5f0584b394ed34ea"
)

// TestLoad 验证两种方言的引擎 Schema 加载。
// @param t 测试上下文
func TestLoad(t *testing.T) {
	for _, dialect := range Dialects() {
		content, err := Load(dialect)
		if err != nil || len(content) == 0 {
			t.Fatalf("加载 %s 失败: %v", dialect, err)
		}
		if !strings.Contains(content, "ACT_RE_PROCDEF") {
			t.Fatalf("%s 缺少 ACT_RE_PROCDEF", dialect)
		}
		// Load 只返回引擎脚本，不含 workflow 表，保证既有初始化路径不变
		if strings.Contains(content, "workflow_model") {
			t.Fatalf("%s 引擎脚本不应包含 workflow 表", dialect)
		}
	}
}

// TestTables 验证引擎表名解析。
// @param t 测试上下文
func TestTables(t *testing.T) {
	for _, dialect := range Dialects() {
		tables, err := Tables(dialect)
		if err != nil {
			t.Fatalf("解析 %s 表名失败: %v", dialect, err)
		}
		if len(tables) != 41 {
			t.Fatalf("%s 表数量错误: %d", dialect, len(tables))
		}
		if tables[0] != "ACT_EVT_LOG" {
			t.Fatalf("%s 表名未排序: %s", dialect, tables[0])
		}
	}
}

// TestChecksum 验证校验和。
// @param t 测试上下文
func TestChecksum(t *testing.T) {
	first, err := Checksum("mysql")
	if err != nil || len(first) != 64 {
		t.Fatalf("校验和错误: %v %s", err, first)
	}
	second, _ := Checksum("mysql")
	if first != second {
		t.Fatalf("校验和应稳定")
	}
}

// TestChecksumStable 验证引擎（001）校验和保持基准值，保障既有库升级路径。
// @param t 测试上下文
func TestChecksumStable(t *testing.T) {
	if got, _ := Checksum("mysql"); got != engineChecksumMySQL {
		t.Fatalf("MySQL 引擎校验和发生变化，禁止修改 001 脚本: %s", got)
	}
	if got, _ := Checksum("postgres"); got != engineChecksumPostgres {
		t.Fatalf("PostgreSQL 引擎校验和发生变化，禁止修改 001 脚本: %s", got)
	}
}

// TestFiles 验证方言脚本目录枚举按文件名升序。
// @param t 测试上下文
func TestFiles(t *testing.T) {
	for _, dialect := range Dialects() {
		files, err := Files(dialect)
		if err != nil {
			t.Fatalf("枚举 %s 脚本失败: %v", dialect, err)
		}
		if len(files) != 2 || files[0] != EngineFile || files[1] != WorkflowFile {
			t.Fatalf("%s 脚本列表错误: %v", dialect, files)
		}
	}
	if _, err := Files("oracle"); err == nil {
		t.Fatalf("不支持方言应报错")
	}
}

// TestLoadFile 验证单脚本加载与文件名校验。
// @param t 测试上下文
func TestLoadFile(t *testing.T) {
	content, err := LoadFile("mysql", WorkflowFile)
	if err != nil {
		t.Fatalf("加载 workflow 脚本失败: %v", err)
	}
	if !strings.Contains(content, "workflow_business_system") {
		t.Fatalf("workflow 脚本缺少 workflow_business_system")
	}
	// 非法文件名与未知文件必须报错，不能越权读取其他资源
	if _, err := LoadFile("mysql", "../schema.go"); err == nil {
		t.Fatalf("路径穿越应报错")
	}
	if _, err := LoadFile("mysql", "003-unknown.sql"); err == nil {
		t.Fatalf("未知脚本应报错")
	}
	if _, err := LoadFile("oracle", WorkflowFile); err == nil {
		t.Fatalf("不支持方言应报错")
	}
}

// TestFileTables 验证 workflow 脚本表名解析与双库表集合一致。
// @param t 测试上下文
func TestFileTables(t *testing.T) {
	mysqlTables, err := FileTables("mysql", WorkflowFile)
	if err != nil {
		t.Fatalf("解析 MySQL workflow 表失败: %v", err)
	}
	pgTables, err := FileTables("postgres", WorkflowFile)
	if err != nil {
		t.Fatalf("解析 PostgreSQL workflow 表失败: %v", err)
	}
	// 003 文档规定 21 张保留表 + workflow_business_system，双库表集合必须完全一致
	if len(mysqlTables) != 22 {
		t.Fatalf("workflow 表数量错误: %d", len(mysqlTables))
	}
	if strings.Join(mysqlTables, ",") != strings.Join(pgTables, ",") {
		t.Fatalf("双库 workflow 表集合不一致:\n%s\n%s", strings.Join(mysqlTables, ","), strings.Join(pgTables, ","))
	}
	if !strings.Contains(strings.Join(mysqlTables, ","), "WORKFLOW_BUSINESS_SYSTEM") {
		t.Fatalf("缺少 workflow_business_system")
	}
}

// TestChecksumFile 验证 workflow 脚本独立校验和。
// @param t 测试上下文
func TestChecksumFile(t *testing.T) {
	workflow, err := ChecksumFile("mysql", WorkflowFile)
	if err != nil || len(workflow) != 64 {
		t.Fatalf("workflow 校验和错误: %v %s", err, workflow)
	}
	engine, _ := Checksum("mysql")
	// workflow 与引擎脚本校验和相互独立
	if workflow == engine {
		t.Fatalf("workflow 校验和不应等于引擎校验和")
	}
	again, _ := ChecksumFile("mysql", WorkflowFile)
	if workflow != again {
		t.Fatalf("workflow 校验和应稳定")
	}
	if _, err := ChecksumFile("mysql", "003-unknown.sql"); err == nil {
		t.Fatalf("未知脚本应报错")
	}
}

// TestTableColumns 验证引擎表列解析。
// @param t 测试上下文
func TestTableColumns(t *testing.T) {
	for _, dialect := range Dialects() {
		columns, types, err := TableColumns(dialect, "act_ru_task")
		if err != nil {
			t.Fatalf("解析 %s 列失败: %v", dialect, err)
		}
		if len(columns) != len(types) || len(columns) == 0 {
			t.Fatalf("%s 列数量错误: %d/%d", dialect, len(columns), len(types))
		}
		if columns[0] != "ID_" || types[0] == "" {
			t.Fatalf("%s 首列解析错误: %s %s", dialect, columns[0], types[0])
		}
		if columns[len(columns)-1] == "PRIMARY" {
			t.Fatalf("%s 不应包含约束声明", dialect)
		}
	}
}

// TestTableColumnsWorkflow 验证 workflow 表列解析（含保留字与转义列）。
// @param t 测试上下文
func TestTableColumnsWorkflow(t *testing.T) {
	for _, dialect := range Dialects() {
		columns, types, err := TableColumns(dialect, "workflow_model")
		if err != nil {
			t.Fatalf("解析 %s workflow_model 失败: %v", dialect, err)
		}
		// 改进后的表体解析不应吞并表外的索引或插入语句
		if columns[0] != "ID" || types[0] != "varchar(64)" {
			t.Fatalf("%s workflow_model 首列解析错误: %s %s", dialect, columns[0], types[0])
		}
		joined := strings.Join(columns, ",")
		if strings.Contains(strings.ToLower(joined), "index") || strings.Contains(strings.ToLower(joined), "unique") {
			t.Fatalf("%s workflow_model 列不应包含表外语句: %s", dialect, joined)
		}
		if !strings.Contains(joined, "MODEL_CODE") || !strings.Contains(joined, "LAST_DEPLOY_TIME") {
			t.Fatalf("%s workflow_model 缺少关键列", dialect)
		}
	}
	// workflow_task_delegate 的保留字列 condition 应能解析
	columns, _, err := TableColumns("mysql", "workflow_task_delegate")
	if err != nil {
		t.Fatalf("解析 workflow_task_delegate 失败: %v", err)
	}
	found := false
	for _, column := range columns {
		if column == "CONDITION" {
			found = true
		}
	}
	if !found {
		t.Fatalf("workflow_task_delegate 缺少 condition 列: %v", columns)
	}
}

// TestTableColumnsMissing 验证未知表返回未找到。
// @param t 测试上下文
func TestTableColumnsMissing(t *testing.T) {
	if _, _, err := TableColumns("postgres", "ACT_UNKNOWN"); err == nil {
		t.Fatalf("未知表应返回错误")
	}
	if _, _, err := TableColumns("oracle", "ACT_RU_TASK"); err == nil {
		t.Fatalf("不支持方言应返回错误")
	}
}

// TestSplitColumnDefinitions 验证列定义拆分。
// @param t 测试上下文
func TestSplitColumnDefinitions(t *testing.T) {
	parts := splitColumnDefinitions("A numeric(10,2), B varchar(32), primary key (A)")
	if len(parts) != 3 {
		t.Fatalf("拆分数量错误: %d", len(parts))
	}
	if !isConstraint("Primary") || isConstraint("A") {
		t.Fatalf("约束判断错误")
	}
}

// TestErrors 验证方言错误。
// @param t 测试上下文
func TestErrors(t *testing.T) {
	if _, err := Load("oracle"); err == nil {
		t.Fatalf("不支持方言应报错")
	}
	if _, err := Tables("oracle"); err == nil {
		t.Fatalf("不支持方言表名应报错")
	}
	if _, err := Checksum("oracle"); err == nil {
		t.Fatalf("不支持方言校验和应报错")
	}
	if Version != "8.1.0.3" {
		t.Fatalf("Schema 版本错误")
	}
}
