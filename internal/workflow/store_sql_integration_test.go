package workflow

import (
	"context"
	"database/sql"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/clock"
	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
	"github.com/DengQingNian/Flowtide/internal/persistence/schema"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// integrationEnvPath 返回仓库根目录的 .env 路径。
// @return 配置文件路径
func integrationEnvPath() string {
	// 支持环境变量覆盖配置位置
	if custom := os.Getenv("FLOWABLE_ENV"); custom != "" {
		return custom
	}
	return filepath.Join("..", "..", ".env")
}

// runIntegrationScript 执行内置 DDL 脚本（去掉整行注释后按分号拆分）。
// @param t 测试上下文
// @param ctx 操作上下文
// @param db 数据库连接池
// @param script 脚本内容
func runIntegrationScript(t *testing.T, ctx context.Context, db *sql.DB, script string) {
	t.Helper()
	lines := make([]string, 0)
	// 去掉整行注释，避免驱动不支持注释前导
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

// newIntegrationMySQL 创建 MySQL 隔离测试库并执行 workflow DDL。
// @param t 测试上下文
// @return 操作上下文
// @return SQL 适配器聚合
// @return 连接池
func newIntegrationMySQL(t *testing.T) (context.Context, *SQLStoreSet, *sql.DB) {
	t.Helper()
	// 未开启集成开关时跳过（与 P0 集成测试同款环境变量）
	if os.Getenv("FLOWABLE_MYSQL_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_MYSQL_INTEGRATION=1 启用真实 MySQL 测试")
	}
	cfg, err := mysqlstore.LoadConfig(integrationEnvPath())
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
	runIntegrationScript(t, ctx, db, ddl)
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, NewSQLStoresWithDB(store, db), db
}

// newIntegrationPostgres 创建 PostgreSQL 隔离测试库并执行 workflow DDL。
// @param t 测试上下文
// @return 操作上下文
// @return SQL 适配器聚合
// @return 连接池
func newIntegrationPostgres(t *testing.T) (context.Context, *SQLStoreSet, *sql.DB) {
	t.Helper()
	// 未开启集成开关时跳过
	if os.Getenv("FLOWABLE_POSTGRES_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_POSTGRES_INTEGRATION=1 启用真实 PostgreSQL 测试")
	}
	dsn := os.Getenv("FLOWABLE_POSTGRES_DSN")
	if dsn == "" {
		var err error
		if dsn, err = pgstore.LoadDSN(integrationEnvPath()); err != nil {
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
	endpoint, err := neturl.Parse(dsn)
	if err != nil || endpoint.Host == "" {
		t.Fatalf("连接地址非法: %v", err)
	}
	endpoint.Path = "/" + name
	db, err := pgstore.Open(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// 隔离库需要重建 search_path 指向的 Schema，保证 DDL 落在预期位置
	if target := endpoint.Query().Get("search_path"); target != "" && target != "public" {
		if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+strconv.Quote(target)); err != nil {
			t.Fatalf("创建测试 Schema 失败: %v", err)
		}
	}
	ddl, err := schema.LoadFile("postgres", schema.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	runIntegrationScript(t, ctx, db, ddl)
	store, err := pwf.NewStore(db, pwf.DialectPostgres)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, NewSQLStoresWithDB(store, db), db
}

// TestIntegrationWorkflowStoresMySQL 在 MySQL 上验证五大域端口的预生成-消耗闭环、
// 待办/已办镜像、操作记录补全与表单状态机。
// @param t 测试上下文
func TestIntegrationWorkflowStoresMySQL(t *testing.T) {
	ctx, stores, _ := newIntegrationMySQL(t)
	testSQLDomainLifecycle(ctx, t, stores)
}

// TestIntegrationWorkflowStoresPostgres 在 PostgreSQL 上验证五大域端口的同套语义。
// @param t 测试上下文
func TestIntegrationWorkflowStoresPostgres(t *testing.T) {
	ctx, stores, _ := newIntegrationPostgres(t)
	testSQLDomainLifecycle(ctx, t, stores)
}

// testSQLDomainLifecycle 在真实数据库上跑通域闭环：
// 下一步双写→消耗→退回清理；待办→已办镜像→办结回写；操作记录→状态机→单据刷新。
// @param ctx 操作上下文
// @param t 测试上下文
// @param stores SQL 适配器聚合
func testSQLDomainLifecycle(ctx context.Context, t *testing.T, stores *SQLStoreSet) {
	t.Helper()
	fake := clock.NewFake(time.Date(2026, 1, 1, 8, 0, 0, 0, time.Local))
	next := NewNextActivityService(stores.Next, fake)
	message := NewMessageExtendService(stores.Message, fake)
	history := NewHistoryExtendService(stores.History, fake)
	document := NewDocumentService(stores.Document, stores.Message, stores.Operation, &fakeInstances{
		running: map[string]api.ProcessInstance{"pi-it": {ID: "pi-it", BusinessKey: "biz-it", TenantID: "sys"}},
	}, &fakeTaskHistory{}, fake)

	// 1、双写与消耗（事务内）
	err := stores.Next.core.store.WithTx(ctx, func(tx context.Context) error {
		if err := next.SaveAgreeNext(tx, NextActivitySaveRequest{
			ProcessInstanceID: "pi-it", ActivityID: "act-b", IdentityIDs: []string{"u1", "u2"},
			PreTaskID: "task-0", SendIdentityID: "sender", SendUserName: "发送人", TenantID: "sys", Creator: "op",
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("同意预存失败: %v", err)
	}
	rows, err := next.ListUnusedByInstanceAndActivity(ctx, "pi-it", "act-b")
	if err != nil || len(rows) != 2 {
		t.Fatalf("未消耗查询失败: %d %v", len(rows), err)
	}
	if rows[0].Version != 1 {
		t.Fatalf("首版应为 1: %d", rows[0].Version)
	}
	ids := []string{rows[0].ID, rows[1].ID}
	if err := stores.Next.core.store.WithTx(ctx, func(tx context.Context) error {
		return next.MarkUsedAndWriteBack(tx, ids, "task-1")
	}); err != nil {
		t.Fatalf("消耗回写失败: %v", err)
	}
	// 两套表同步：历史表 used=1 + taskId；版本推进
	his, err := next.ListHisByInstance(ctx, "pi-it")
	if err != nil || len(his) != 2 {
		t.Fatalf("历史双写查询失败: %d %v", len(his), err)
	}
	for _, row := range his {
		if row.Used != 1 || row.TaskID != "task-1" {
			t.Fatalf("历史表未同步消耗: %+v", row)
		}
	}
	if version, err := next.LatestVersion(ctx, "pi-it", "act-b"); err != nil || version != 1 {
		t.Fatalf("最新版本失败: %d %v", version, err)
	}
	if err := next.SaveAgreeNext(ctx, NextActivitySaveRequest{ProcessInstanceID: "pi-it", ActivityID: "act-b", IdentityIDs: []string{"u1"}, TenantID: "sys"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = next.ListUnusedByInstanceAndActivity(ctx, "pi-it", "act-b")
	if len(rows) != 1 || rows[0].Version != 2 {
		t.Fatalf("消耗后版本应推进为 2: %+v", rows)
	}
	// 首步预生成（used=1 + taskId）
	if err := next.SaveFirstTaskUser(ctx, FirstStepRequest{
		ProcessInstanceID: "pi-it2", ActivityID: "first", IdentityIDs: []string{"u1"},
		SendIdentityID: "starter", SendUserName: "发起人", TaskID: "task-first", TenantID: "sys", Creator: "starter",
	}); err != nil {
		t.Fatalf("首步预生成失败: %v", err)
	}
	firstRows, err := stores.Next.ListUsedByInstance(ctx, "pi-it2")
	if err != nil || len(firstRows) != 1 || firstRows[0].Used != 1 || firstRows[0].TaskID != "task-first" {
		t.Fatalf("首步语义失败: %d %v %+v", len(firstRows), err, firstRows)
	}
	// 清理规则
	if err := next.DeleteForBackToStep(ctx, "pi-it", "act-b"); err != nil {
		t.Fatalf("退回清理失败: %v", err)
	}
	if rows, _ := next.ListUnusedByInstanceAndActivity(ctx, "pi-it", "act-b"); len(rows) != 0 {
		t.Fatalf("退回清理后仍有记录: %d", len(rows))
	}

	// 2、待办 → 已办镜像 → 办结回写
	msg := &MessageCreateRequest{
		BaseColumns: BaseColumns{TenantID: "sys"}, TaskID: "task-m1", ProcessInstanceID: "pi-it",
		BusinessID: "biz-it", Title: "标题", TaskStepCode: "step-a", TaskStepName: "步骤A",
		FlowCategoryCode: "leave", FlowCategoryName: "请假", OwnerID: "u1", OwnerName: "张三",
		ExtendFields: ExtendFields{ExtendS1: "1001"},
	}
	created, err := message.CreateForTask(ctx, msg)
	if err != nil {
		t.Fatalf("待办创建失败: %v", err)
	}
	// 幂等：同任务再次创建返回既有记录
	if again, err := message.CreateForTask(ctx, msg); err != nil || again.ID != created.ID {
		t.Fatalf("待办幂等失败: %v", err)
	}
	histEntity := &HistoryExtend{MessageExtend: *created}
	if _, err := history.CreateForTask(ctx, histEntity); err != nil {
		t.Fatalf("已办创建失败: %v", err)
	}
	if err := history.UpdateFinishTime(ctx, "task-m1", "u1", fake.Now()); err != nil {
		t.Fatalf("办结回写失败: %v", err)
	}
	gotHist, found, err := history.GetByTaskID(ctx, "task-m1")
	if err != nil || !found || gotHist.TaskFinishTime == nil {
		t.Fatalf("办结时间回写校验失败: %v %v", found, err)
	}
	// 待办已读
	if err := message.UpdateRead(ctx, "task-m1", "u1", fake.Now()); err != nil {
		t.Fatalf("已读回写失败: %v", err)
	}
	gotMsg, _, _ := message.GetByTaskID(ctx, "task-m1")
	if gotMsg.IsRead != 1 || gotMsg.ExtendS1 != "1001" {
		t.Fatalf("已读/扩展列校验失败: %+v", gotMsg)
	}

	// 3、操作记录（引擎桩补全）+ 表单状态机
	operations := NewOperationRecordService(stores.Operation,
		&fakeTaskHistory{tasks: map[string]api.HistoricTaskInstance{
			"task-m1": {ID: "task-m1", Name: "步骤A", ProcessInstanceID: "pi-it", TaskDefinitionKey: "step-a", StartTime: fake.Now()},
		}},
		&fakeInstances{
			running: map[string]api.ProcessInstance{"pi-it": {ID: "pi-it", BusinessKey: "biz-it", TenantID: "sys", ProcessDefinitionID: "def-it"}},
		},
		&fakeDefinitions{definitions: map[string]api.ProcessDefinition{"def-it": {ID: "def-it", Key: "leave", Name: "请假流程"}}},
		&fakeUsers{users: map[string]api.User{"u1": {ID: "u1", DisplayName: "张三"}}},
		fake)
	// 模拟发起记录（TJ）+ 办结记录（SP）→ 操作记录数 = 2 → 流转中（Java 状态机判据）
	if _, err := operations.Save(ctx, OperationRequest{ProcessInstanceID: "pi-it", OwnerID: "u1", Type: CommentTypeTJ, Message: "提交"}); err != nil {
		t.Fatalf("发起记录落库失败: %v", err)
	}
	record, err := operations.Save(ctx, OperationRequest{TaskID: "task-m1", ProcessInstanceID: "pi-it", OwnerID: "u1", Type: CommentTypeSP, Message: "同意"})
	if err != nil {
		t.Fatalf("操作记录落库失败: %v", err)
	}
	if record.TaskStepCode != "step-a" || record.BusinessID != "biz-it" || record.FlowCategoryCode != "leave" || record.OwnerName != "张三" {
		t.Fatalf("操作记录补全失败: %+v", record)
	}
	// 可签名与最后类型
	signable, err := operations.ListSignable(ctx, []string{"biz-it"})
	if err != nil || len(signable) != 1 {
		t.Fatalf("可签名查询失败: %d %v", len(signable), err)
	}
	if lastType, err := stores.Operation.LastTypeByInstance(ctx, "pi-it"); err != nil || lastType != CommentTypeSP {
		t.Fatalf("最后类型失败: %s %v", lastType, err)
	}
	// 表单状态机：流转中聚合
	doc, err := document.Save(ctx, DocumentSaveRequest{Document: &Document{
		BaseColumns: BaseColumns{TenantID: "sys"}, BusinessID: "biz-it", FlowInstanceID: "pi-it",
		FlowTitle: "标题", DocumentTable: "t_leave",
	}})
	if err != nil {
		t.Fatalf("单据保存失败: %v", err)
	}
	if doc.FlowStatus != FlowStatusNotSubmitted {
		t.Fatalf("默认状态应为未提交: %s", doc.FlowStatus)
	}
	if err := document.UpdateByProcessInstanceOrTask(ctx, "pi-it", ""); err != nil {
		t.Fatalf("状态机刷新失败: %v", err)
	}
	gotDoc, found, err := document.GetByBusinessID(ctx, "biz-it", "sys")
	if err != nil || !found {
		t.Fatalf("单据查询失败: %v %v", found, err)
	}
	if gotDoc.FlowStatus != FlowStatusRunning {
		t.Fatalf("状态应为流转中: %s", gotDoc.FlowStatus)
	}
	if gotDoc.FlowCurrentPersons != "张三" || gotDoc.FlowActionID != "step-a" || gotDoc.DocumentTable != "t_leave" {
		t.Fatalf("待办聚合失败: %+v", gotDoc)
	}
	// 签收步骤 → 已完成
	signMsg := &MessageCreateRequest{
		BaseColumns: BaseColumns{TenantID: "sys"}, TaskID: "task-sign", ProcessInstanceID: "pi-it",
		BusinessID: "biz-it", TaskStepCode: "Sign", TaskStepName: "签收", OwnerID: "u2", OwnerName: "李四",
	}
	if _, err := message.CreateForTask(ctx, signMsg); err != nil {
		t.Fatal(err)
	}
	if err := document.UpdateByProcessInstanceOrTask(ctx, "pi-it", ""); err != nil {
		t.Fatal(err)
	}
	gotDoc, _, _ = document.GetByBusinessID(ctx, "biz-it", "sys")
	if gotDoc.FlowStatus != FlowStatusFinished {
		t.Fatalf("签收后状态应为已完成: %s", gotDoc.FlowStatus)
	}
	// 终止（LCZZ 记录）
	if _, err := operations.Save(ctx, OperationRequest{ProcessInstanceID: "pi-it", OwnerID: "u1", Type: CommentTypeLCZZ}); err != nil {
		t.Fatal(err)
	}
	if err := document.Terminate(ctx, "pi-it"); err != nil {
		t.Fatal(err)
	}
	gotDoc, _, _ = document.GetByBusinessID(ctx, "biz-it", "sys")
	if gotDoc.FlowStatus != FlowStatusTerminated || gotDoc.FlowTerminationTime == nil || gotDoc.FlowCurrentPersons != "" {
		t.Fatalf("终止语义失败: %+v", gotDoc)
	}
	// 裸 SQL 补齐查询（ListUsedByInstance / ListHisByTaskID / 已办分页）
	if rows, err := stores.Next.ListUsedByInstance(ctx, "pi-it2"); err != nil || len(rows) != 1 {
		t.Fatalf("裸 SQL 已回写查询失败: %d %v", len(rows), err)
	}
	if rows, err := stores.Next.ListHisByTaskID(ctx, "task-first"); err != nil || len(rows) != 1 {
		t.Fatalf("裸 SQL 历史查询失败: %d %v", len(rows), err)
	}
	if page, err := history.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 10}); err != nil || page.Total != 1 {
		t.Fatalf("已办分页查询失败: %d %v", page.Total, err)
	}
}
