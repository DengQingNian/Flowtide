package migration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	"github.com/DengQingNian/Flowtide/internal/migration"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	_ "github.com/lib/pq"
)

// TestIntegrationPreflightMySQL 验证真实 MySQL 中只读扫描、实体统计与拒绝报告。
// @param t 测试上下文
// @return 无
func TestIntegrationPreflightMySQL(t *testing.T) {
	ctx, db := preflightDatabase(t, "mysql")
	verifyPreflightDatabase(t, ctx, db, "mysql")
}

// TestIntegrationPreflightPostgres 验证真实 PostgreSQL 中小写表发现与只读报告。
// @param t 测试上下文
// @return 无
func TestIntegrationPreflightPostgres(t *testing.T) {
	ctx, db := preflightDatabase(t, "postgres")
	verifyPreflightDatabase(t, ctx, db, "postgres")
}

// preflightDatabase 创建并在测试结束清理专用临时数据库。
// @param t 测试上下文
// @param dialect 数据库方言
// @return 有限时长的上下文
// @return 临时库连接池
func preflightDatabase(t *testing.T, dialect string) (context.Context, *sql.DB) {
	t.Helper()
	flag := "FLOWABLE_MYSQL_INTEGRATION"
	if dialect == "postgres" {
		flag = "FLOWABLE_POSTGRES_INTEGRATION"
	}
	if os.Getenv(flag) != "1" {
		t.Skip("设置 " + flag + "=1 启用真实数据库预检验收")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	name := fmt.Sprintf("flowable_go_preflight_%d", time.Now().UnixNano())
	var admin, db *sql.DB
	var err error
	// 两种方言均只在测试生成的数据库写入夹具，绝不初始化开发业务库。
	if dialect == "mysql" {
		path := os.Getenv("FLOWABLE_MYSQL_ENV")
		if path == "" {
			path = "../../.env"
		}
		cfg, configErr := mysqlstore.LoadConfig(path)
		if configErr != nil {
			t.Fatal(configErr)
		}
		admin, err = mysqlstore.Open(ctx, cfg)
		if err != nil {
			t.Fatal("连接 MySQL 验收环境失败")
		}
		t.Cleanup(func() { _ = admin.Close() })
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4"); err != nil {
			t.Fatal("创建 MySQL 临时库失败")
		}
		cfg.Database = name
		db, err = mysqlstore.Open(ctx, cfg)
	} else {
		endpoint, parseErr := url.Parse(os.Getenv("FLOWABLE_POSTGRES_DSN"))
		if parseErr != nil || endpoint.Host == "" {
			t.Fatal("需要有效的 FLOWABLE_POSTGRES_DSN 连接地址")
		}
		admin, err = sql.Open("postgres", endpoint.String())
		if err != nil {
			t.Fatal("打开 PostgreSQL 验收环境失败")
		}
		t.Cleanup(func() { _ = admin.Close() })
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
			t.Fatal("创建 PostgreSQL 临时库失败")
		}
		endpoint.Path = "/" + name
		db, err = sql.Open("postgres", endpoint.String())
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE "+name); err != nil {
			t.Error("清理预检临时库失败")
		}
	})
	if err != nil {
		t.Fatal("打开预检临时库失败")
	}
	t.Cleanup(func() { _ = db.Close() })
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatal("读取真实数据库版本失败")
	}
	t.Logf("数据库验收版本：%s", version)
	ddl, err := schema.Load(dialect)
	if err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 0)
	for _, line := range strings.Split(ddl, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("初始化测试专用 Schema 失败：%v", err)
		}
	}
	return ctx, db
}

// verifyPreflightDatabase 验证真实行扫描、重复读取不变及未知实体阻断。
// @param t 测试上下文
// @param ctx 数据库上下文
// @param db 临时库连接池
// @param dialect 数据库方言
// @return 无
func verifyPreflightDatabase(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	readonly, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal("数据库不支持预检要求的只读事务")
	}
	_, writeErr := readonly.ExecContext(ctx, "INSERT INTO ACT_GE_PROPERTY (NAME_, VALUE_, REV_) VALUES ('preflight-readonly-probe', 'rejected', 1)")
	if err := readonly.Rollback(); err != nil {
		t.Fatal("只读验证事务回滚失败")
	}
	if writeErr == nil {
		t.Fatal("数据库没有拒绝只读事务中的写操作")
	}
	repository, err := flowablerow.NewRepository(flowablerow.SQLDB{DB: db}, dialect)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="preflight" isExecutable="true"><startEvent id="start"/><sequenceFlow id="a" sourceRef="start" targetRef="task"/><userTask id="task"/><sequenceFlow id="b" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	if _, err := workflow.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"preflight.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	instance, err := workflow.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "preflight", Variables: []api.Variable{{Name: "备注", Value: "敏感变量内容不可出现在报告"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := workflow.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 直接写入 Java 形态的根执行、子执行和用户任务，不添加 Go 专用控制快照。
	preflightExec(t, ctx, db, dialect, "INSERT INTO ACT_RU_EXECUTION (ID_, REV_, PROC_INST_ID_, PROC_DEF_ID_, IS_ACTIVE_, IS_SCOPE_, TENANT_ID_) VALUES (?, 1, ?, ?, true, true, '')", "java-root", "java-root", instance.ProcessDefinitionID)
	preflightExec(t, ctx, db, dialect, "INSERT INTO ACT_RU_EXECUTION (ID_, REV_, PROC_INST_ID_, PROC_DEF_ID_, PARENT_ID_, ACT_ID_, IS_ACTIVE_, IS_SCOPE_, TENANT_ID_) VALUES (?, 1, ?, ?, ?, 'task', true, false, '')", "java-child", "java-root", instance.ProcessDefinitionID, "java-root")
	preflightExec(t, ctx, db, dialect, "INSERT INTO ACT_RU_TASK (ID_, REV_, EXECUTION_ID_, PROC_INST_ID_, PROC_DEF_ID_, TASK_DEF_KEY_, TENANT_ID_) VALUES (?, 1, ?, ?, ?, 'task', '')", "java-task", "java-child", "java-root", instance.ProcessDefinitionID)
	before, err := migration.Scan(ctx, db, dialect)
	if err != nil {
		t.Fatal(err)
	}
	if before.Dialect != dialect || before.SchemaVersion != schema.Version {
		t.Fatalf("数据库方言或 Schema 版本识别错误：%s %s", before.Dialect, before.SchemaVersion)
	}
	if len(before.Tables) == 0 || len(before.Rows) == 0 {
		t.Fatal("真实数据库没有扫描出表与实体")
	}
	report := migration.Analyze(before, migration.Options{})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "敏感变量内容不可出现在报告") || strings.Contains(string(encoded), xml) {
		t.Fatal("报告泄露变量值或部署源码")
	}
	found := false
	for _, tenant := range report.Tenants {
		if tenant.Instances == 2 && tenant.Tasks == 2 && tenant.Variables >= 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("报告缺少实例 %s 对应的实体统计", instance.ID)
	}
	after, err := migration.Scan(ctx, db, dialect)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotFingerprint(t, before) != snapshotFingerprint(t, after) {
		t.Fatal("只读预检改变了源库行数据")
	}
	preflightExec(t, ctx, db, dialect, "INSERT INTO ACT_RU_VARIABLE (ID_, REV_, TYPE_, NAME_, EXECUTION_ID_, PROC_INST_ID_) VALUES (?, 1, 'serializable', 'java-object', ?, ?)", "java-serializable", "java-root", "java-root")
	serialized, err := migration.Scan(ctx, db, dialect)
	if err != nil {
		t.Fatal(err)
	}
	serializedReport := migration.Analyze(serialized, migration.Options{})
	serializedFound := false
	for _, issue := range serializedReport.Issues {
		if issue.Code == "VARIABLE_TYPE" && issue.EntityID == "java-serializable" && issue.Blocking {
			serializedFound = true
		}
	}
	if !serializedFound || !serializedReport.Blocked {
		t.Fatal("Java 序列化变量没有进入逐实体阻断报告")
	}
	// 非空未知 ACT 表必须阻断，不能因为核心流程正常就忽略其存量数据。
	if _, err := db.ExecContext(ctx, "CREATE TABLE ACT_RU_PREFLIGHT_UNKNOWN (ID_ VARCHAR(64) PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ACT_RU_PREFLIGHT_UNKNOWN (ID_) VALUES ('unknown')"); err != nil {
		t.Fatal(err)
	}
	unknown, err := migration.Scan(ctx, db, dialect)
	if err != nil {
		t.Fatal(err)
	}
	unknownReport := migration.Analyze(unknown, migration.Options{})
	if !unknownReport.Blocked {
		t.Fatal("存在未知实体时预检必须阻断")
	}
	issueFound := false
	for _, issue := range unknownReport.Issues {
		if issue.Table == "ACT_RU_PREFLIGHT_UNKNOWN" && issue.Blocking {
			issueFound = true
		}
	}
	if !issueFound {
		t.Fatal("未知实体没有进入逐表阻断问题报告")
	}
	t.Log("真实行扫描、Java 形态执行树、序列化变量拒绝、租户统计、敏感值隔离、读取不变及未知表阻断通过")
}

// preflightExec 按目标方言执行测试夹具写入。
// @param t 测试上下文
// @param ctx 数据库上下文
// @param db 临时库连接池
// @param dialect 数据库方言
// @param query 使用问号占位符的测试语句
// @param args 测试参数
// @return 无
func preflightExec(t *testing.T, ctx context.Context, db *sql.DB, dialect, query string, args ...any) {
	t.Helper()
	if dialect == "postgres" {
		for i := range args {
			query = strings.Replace(query, "?", fmt.Sprintf("$%d", i+1), 1)
		}
	}
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("写入临时库 Java 形态夹具失败：%v", err)
	}
}

// snapshotFingerprint 将无序数据库结果规范化为可比较的实体快照。
// @param t 测试上下文
// @param snapshot 扫描结果
// @return 稳定的实体快照摘要文本
func snapshotFingerprint(t *testing.T, snapshot migration.SourceSnapshot) string {
	t.Helper()
	rows := make([]string, 0, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, string(data))
	}
	sort.Strings(rows)
	data, err := json.Marshal(struct {
		Tables []migration.TableSummary
		Rows   []string
	}{Tables: snapshot.Tables, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
