package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// crudRepo 是行往返测试所需的通用 CRUD 接口（各表仓储均满足）。
type crudRepo interface {
	Insert(ctx context.Context, row Row) error
	UpdateByPK(ctx context.Context, id string, changes Row) (int64, error)
	DeleteByPK(ctx context.Context, id string) (int64, error)
	GetByPK(ctx context.Context, id string) (Row, bool, error)
}

// roundtripCase 是一张表的往返测试用例。
type roundtripCase struct {
	table string   // 表名
	repo  crudRepo // 表仓储
}

// roundtrips 构造全部 22 张表的往返用例（顺序与 DDL 一致）。
// @param store 仓储入口
// @return 往返用例列表
func roundtrips(store *Store) []roundtripCase {
	repos := NewRepositories(store)
	return []roundtripCase{
		{"workflow_business_system", repos.BusinessSystems},
		{"workflow_model", repos.Models},
		{"workflow_step", repos.Steps},
		{"workflow_his_step", repos.HisSteps},
		{"workflow_template", repos.Templates},
		{"workflow_act_extend", repos.ActExtends},
		{"workflow_countersign_weight", repos.CountersignWeights},
		{"workflow_next_activity", repos.NextActivities},
		{"workflow_his_next_activity", repos.HisNextActivities},
		{"workflow_task_message_extend", repos.MessageExtends},
		{"workflow_task_history_extend", repos.HistoryExtends},
		{"workflow_delegate_task", repos.DelegateTasks},
		{"workflow_document", repos.Documents},
		{"workflow_operation_record", repos.OperationRecords},
		{"workflow_review_task", repos.ReviewTasks},
		{"workflow_task_exception", repos.TaskExceptions},
		{"workflow_complete_to_activity", repos.CompleteToActivities},
		{"workflow_task_delegate", repos.TaskDelegates},
		{"workflow_task_consigners", repos.TaskConsigners},
		{"workflow_task_consign_obj", repos.TaskConsignObjs},
		{"workflow_remind", repos.Reminds},
		{"workflow_remind_target", repos.RemindTargets},
	}
}

// envPath 返回仓库根目录的 .env 路径。
// @return 配置文件路径
func envPath() string {
	if custom := os.Getenv("FLOWABLE_ENV"); custom != "" {
		return custom
	}
	return filepath.Join("..", "..", "..", ".env")
}

// runScript 执行内置 DDL 脚本（去掉整行注释后按分号拆分）。
// @param t 测试上下文
// @param ctx 操作上下文
// @param db 数据库连接池
// @param script 脚本内容
func runScript(t *testing.T, ctx context.Context, db *sql.DB, script string) {
	t.Helper()
	lines := make([]string, 0)
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for i, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("执行 workflow DDL 第 %d 条语句失败: %v", i+1, err)
		}
	}
}

// newMySQLDB 创建 MySQL 隔离测试库并执行 workflow DDL。
// @param t 测试上下文
// @return 操作上下文
// @return 仓储入口
// @return 连接池
func newMySQLDB(t *testing.T) (context.Context, *Store, *sql.DB) {
	t.Helper()
	if os.Getenv("FLOWABLE_MYSQL_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_MYSQL_INTEGRATION=1 启用真实 MySQL 测试")
	}
	cfg, err := mysqlstore.LoadConfig(envPath())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	admin, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	// 仅创建与清理本次测试的临时库，不触碰开发库现有数据
	name := "flowable_wf_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Logf("MySQL 隔离测试库：%s", name)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4"); err != nil {
		t.Fatalf("创建隔离测试库失败，需要 CREATE/DROP DATABASE 权限: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+name); err != nil {
			t.Errorf("清理隔离测试库失败: %v", err)
		}
	})
	cfg.Database = name
	db, err := mysqlstore.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ddl, err := schema.LoadFile("mysql", schema.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	runScript(t, ctx, db, ddl)
	store, err := NewStore(db, DialectMySQL)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, db
}

// newPostgresDB 创建 PostgreSQL 隔离测试库并执行 workflow DDL。
// @param t 测试上下文
// @return 操作上下文
// @return 仓储入口
// @return 连接池
func newPostgresDB(t *testing.T) (context.Context, *Store, *sql.DB) {
	t.Helper()
	if os.Getenv("FLOWABLE_POSTGRES_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_POSTGRES_INTEGRATION=1 启用真实 PostgreSQL 测试")
	}
	dsn := os.Getenv("FLOWABLE_POSTGRES_DSN")
	if dsn == "" {
		var err error
		if dsn, err = pgstore.LoadDSN(envPath()); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	admin, err := pgstore.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := "flowable_wf_it_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Logf("PostgreSQL 隔离测试库：%s", name)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("创建隔离测试库失败，需要 CREATEDB 权限: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		// 先关闭业务连接再强制删库，避免残留连接阻塞清理
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("清理隔离测试库失败: %v", err)
		}
	})
	endpoint, err := url.Parse(dsn)
	if err != nil || endpoint.Host == "" {
		t.Fatalf("连接地址非法: %v", err)
	}
	endpoint.Path = "/" + name
	// 先建 search_path 指向的 Schema：连接 DSN 携带 search_path，Schema 不存在时
	// 连接本身即失败（libpq 在建连阶段应用 search_path）。
	if target := endpoint.Query().Get("search_path"); target != "" && target != "public" {
		bootstrapEndpoint := *endpoint
		bootstrapQuery := bootstrapEndpoint.Query()
		bootstrapQuery.Del("search_path")
		bootstrapEndpoint.RawQuery = bootstrapQuery.Encode()
		bootstrap, err := pgstore.Open(ctx, bootstrapEndpoint.String())
		if err != nil {
			t.Fatalf("连接隔离测试库失败: %v", err)
		}
		// target 可能已带双引号（含连字符的 Schema 名由 LoadDSN 加引号），避免二次加引号
		schemaIdent := target
		if !strings.HasPrefix(schemaIdent, "\"") {
			schemaIdent = strconv.Quote(schemaIdent)
		}
		if _, err := bootstrap.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+schemaIdent); err != nil {
			_ = bootstrap.Close()
			t.Fatalf("创建测试 Schema 失败: %v", err)
		}
		_ = bootstrap.Close()
	}
	db, err := pgstore.Open(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ddl, err := schema.LoadFile("postgres", schema.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	runScript(t, ctx, db, ddl)
	store, err := NewStore(db, DialectPostgres)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, db
}

// TestIntegrationWorkflowMySQL 验证 MySQL 上 22 张表 CRUD 往返、预生成-消耗语义与工作单元。
// @param t 测试上下文
func TestIntegrationWorkflowMySQL(t *testing.T) {
	ctx, store, _ := newMySQLDB(t)
	testRoundtrip(ctx, t, store)
	testNextActivityLifecycle(ctx, t, store)
	testDocumentUpsert(ctx, t, store)
	testExceptionHandling(ctx, t, store)
	testWorkUnit(ctx, t, store)
}

// TestIntegrationWorkflowPostgres 验证 PostgreSQL 上 22 张表 CRUD 往返、预生成-消耗语义与工作单元。
// @param t 测试上下文
func TestIntegrationWorkflowPostgres(t *testing.T) {
	ctx, store, _ := newPostgresDB(t)
	testRoundtrip(ctx, t, store)
	testNextActivityLifecycle(ctx, t, store)
	testDocumentUpsert(ctx, t, store)
	testExceptionHandling(ctx, t, store)
	testWorkUnit(ctx, t, store)
}

// testRoundtrip 对全部 22 张表执行插入、按主键查询、更新、删除往返。
// @param ctx 操作上下文
// @param t 测试上下文
// @param store 仓储入口
func testRoundtrip(ctx context.Context, t *testing.T, store *Store) {
	t.Helper()
	for i, item := range roundtrips(store) {
		id := fmt.Sprintf("it-%02d", i)
		if err := item.repo.Insert(ctx, Row{"id": id, "creator": "tester", "tenant_id": "it-tenant"}); err != nil {
			t.Fatalf("%s 插入失败: %v", item.table, err)
		}
		row, found, err := item.repo.GetByPK(ctx, id)
		if err != nil || !found {
			t.Fatalf("%s 查询失败: %v %v", item.table, err, found)
		}
		// create_time 由仓储自动填充；varchar 经 MySQL 驱动返回为字符串
		if row["tenant_id"] != "it-tenant" {
			t.Fatalf("%s 租户列错误: %v", item.table, row["tenant_id"])
		}
		if _, ok := row["create_time"].(time.Time); !ok {
			t.Fatalf("%s create_time 应为时间值: %T", item.table, row["create_time"])
		}
		if _, err := item.repo.UpdateByPK(ctx, id, Row{"creator_name": "集成测试"}); err != nil {
			t.Fatalf("%s 更新失败: %v", item.table, err)
		}
		row, found, err = item.repo.GetByPK(ctx, id)
		if err != nil || !found || row["creator_name"] != "集成测试" {
			t.Fatalf("%s 更新后校验失败: %v %v %v", item.table, err, found, row["creator_name"])
		}
		// update_time 应随更新刷新为测试时钟之外的当前时间
		if _, ok := row["update_time"].(time.Time); !ok {
			t.Fatalf("%s update_time 应为时间值: %T", item.table, row["update_time"])
		}
		if _, err := item.repo.DeleteByPK(ctx, id); err != nil {
			t.Fatalf("%s 删除失败: %v", item.table, err)
		}
		if _, found, err := item.repo.GetByPK(ctx, id); err != nil || found {
			t.Fatalf("%s 删除后仍可查询: %v %v", item.table, err, found)
		}
	}
	t.Log("22 张表插入/查询/更新/删除往返通过")
}

// testNextActivityLifecycle 验证下一步记录的预生成-双写-消耗闭环。
// @param ctx 操作上下文
// @param t 测试上下文
// @param store 仓储入口
func testNextActivityLifecycle(ctx context.Context, t *testing.T, store *Store) {
	t.Helper()
	repos := NewRepositories(store)
	record := Row{
		"id":                  "na-1",
		"next_activity_id":    "act-approve",
		"identity_id":         "user-1",
		"process_instance_id": "pi-1",
		"used":                0,
		"version":             1,
		"tenant_id":           "it-tenant",
	}
	if err := repos.NextActivities.InsertDual(ctx, record); err != nil {
		t.Fatalf("双写失败: %v", err)
	}
	// 未消耗记录可查
	unused, err := repos.NextActivities.ListUnusedByActivity(ctx, "pi-1", "act-approve")
	if err != nil || len(unused) != 1 {
		t.Fatalf("未消耗查询失败: %v %d", err, len(unused))
	}
	// 标记消耗并回写任务 ID
	if _, err := repos.NextActivities.MarkUsed(ctx, "na-1", "task-1"); err != nil {
		t.Fatal(err)
	}
	if unused, err = repos.NextActivities.ListUnusedByActivity(ctx, "pi-1", "act-approve"); err != nil || len(unused) != 0 {
		t.Fatalf("消耗后仍有未消耗记录: %v %d", err, len(unused))
	}
	byTask, err := repos.NextActivities.ListByTask(ctx, "task-1")
	if err != nil || len(byTask) != 1 || byTask[0]["used"] != int64(1) {
		t.Fatalf("任务回写校验失败: %v %v", err, byTask)
	}
	// 历史表同步可查
	history, err := repos.HisNextActivities.ListByProcessInstance(ctx, "pi-1")
	if err != nil || len(history) != 1 {
		t.Fatalf("历史双写校验失败: %v %d", err, len(history))
	}
	// 清理
	if _, err := repos.NextActivities.DeleteByProcessInstance(ctx, "pi-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.HisNextActivities.DeleteByProcessInstanceAndActivities(ctx, "pi-1", []string{"act-approve"}); err != nil {
		t.Fatal(err)
	}
}

// testDocumentUpsert 验证单据状态的按业务单据保存与逻辑删除。
// @param ctx 操作上下文
// @param t 测试上下文
// @param store 仓储入口
func testDocumentUpsert(ctx context.Context, t *testing.T, store *Store) {
	t.Helper()
	repos := NewRepositories(store)
	first := Row{"id": "doc-1", "business_id": "biz-1", "tenant_id": "it-tenant", "flow_status": "流转中", "flow_instance_id": "pi-9"}
	if err := repos.Documents.UpsertByBusinessID(ctx, first); err != nil {
		t.Fatalf("单据插入失败: %v", err)
	}
	// 重复保存更新同一行而非新增
	second := Row{"id": "doc-2", "business_id": "biz-1", "tenant_id": "it-tenant", "flow_status": "已办结"}
	if err := repos.Documents.UpsertByBusinessID(ctx, second); err != nil {
		t.Fatalf("单据更新失败: %v", err)
	}
	row, found, err := repos.Documents.GetByBusinessID(ctx, "biz-1", "it-tenant")
	if err != nil || !found || row["flow_status"] != "已办结" {
		t.Fatalf("单据状态校验失败: %v %v %v", err, found, row["flow_status"])
	}
	if row["id"] != "doc-1" {
		t.Fatalf("upsert 不应更换主键: %v", row["id"])
	}
	// 逻辑删除后查询过滤
	if _, err := repos.Documents.MarkDeletedByBusinessID(ctx, "biz-1", "it-tenant"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := repos.Documents.GetByBusinessID(ctx, "biz-1", "it-tenant"); err != nil || found {
		t.Fatalf("逻辑删除后仍可查询: %v %v", err, found)
	}
	if _, err := repos.Documents.DeleteByPK(ctx, "doc-1"); err != nil {
		t.Fatal(err)
	}
}

// testExceptionHandling 验证异常落库与处理状态流转。
// @param ctx 操作上下文
// @param t 测试上下文
// @param store 仓储入口
func testExceptionHandling(ctx context.Context, t *testing.T, store *Store) {
	t.Helper()
	repos := NewRepositories(store)
	// handle_status 不传时使用数据库默认值 0（未处理）
	if err := repos.TaskExceptions.Insert(ctx, Row{"id": "exc-1", "task_id": "task-9", "process_instance_id": "pi-9", "message": "回调失败", "tenant_id": "it-tenant"}); err != nil {
		t.Fatalf("异常插入失败: %v", err)
	}
	unhandled, err := repos.TaskExceptions.ListUnhandled(ctx)
	if err != nil || len(unhandled) != 1 {
		t.Fatalf("未处理查询失败: %v %d", err, len(unhandled))
	}
	if _, err := repos.TaskExceptions.MarkHandled(ctx, "exc-1"); err != nil {
		t.Fatal(err)
	}
	if unhandled, err = repos.TaskExceptions.ListUnhandled(ctx); err != nil || len(unhandled) != 0 {
		t.Fatalf("处理后仍有未处理记录: %v %d", err, len(unhandled))
	}
	if _, err := repos.TaskExceptions.DeleteByPK(ctx, "exc-1"); err != nil {
		t.Fatal(err)
	}
}

// testWorkUnit 验证 WithTx 跨表原子提交与回滚。
// @param ctx 操作上下文
// @param t 测试上下文
// @param store 仓储入口
func testWorkUnit(ctx context.Context, t *testing.T, store *Store) {
	t.Helper()
	repos := NewRepositories(store)
	// 回滚路径：多表写入随错误一起回滚
	txErr := store.WithTx(ctx, func(tx context.Context) error {
		if err := repos.Models.Insert(tx, Row{"id": "wu-model", "tenant_id": "it-tenant"}); err != nil {
			return err
		}
		if err := repos.Documents.Insert(tx, Row{"id": "wu-doc", "business_id": "wu-biz", "tenant_id": "it-tenant"}); err != nil {
			return err
		}
		return errors.New("触发回滚")
	})
	if txErr == nil {
		t.Fatal("应返回回滚错误")
	}
	for _, item := range []struct {
		table string
		repo  crudRepo
		id    string
	}{
		{"workflow_model", repos.Models, "wu-model"},
		{"workflow_document", repos.Documents, "wu-doc"},
	} {
		if _, found, err := item.repo.GetByPK(ctx, item.id); err != nil || found {
			t.Fatalf("%s 回滚失败: %v %v", item.table, err, found)
		}
	}
	// 提交路径：多表写入原子生效后清理
	if err := store.WithTx(ctx, func(tx context.Context) error {
		if err := repos.Models.Insert(tx, Row{"id": "wu-model", "tenant_id": "it-tenant"}); err != nil {
			return err
		}
		return repos.Documents.Insert(tx, Row{"id": "wu-doc", "business_id": "wu-biz", "tenant_id": "it-tenant"})
	}); err != nil {
		t.Fatalf("事务提交失败: %v", err)
	}
	if _, found, err := repos.Models.GetByPK(ctx, "wu-model"); err != nil || !found {
		t.Fatalf("模型事务写入丢失: %v %v", err, found)
	}
	if _, found, err := repos.Documents.GetByPK(ctx, "wu-doc"); err != nil || !found {
		t.Fatalf("单据事务写入丢失: %v %v", err, found)
	}
	if _, err := repos.Models.DeleteByPK(ctx, "wu-model"); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Documents.DeleteByPK(ctx, "wu-doc"); err != nil {
		t.Fatal(err)
	}
}
