// store_sql_cover_test.go 用 go-sqlmock 覆盖 SQLNextStore（store_sql.go 第 455-700 行）
// 与行转换函数的全部分支：双写插入、版本查询、未消耗查询、消耗回写、历史查询、
// 退回/撤回/委派清理、来源定位，以及裸 SQL 补齐通道在有/无连接池时的两条路径。
// 方言固定 MySQL（"?" 占位），时间源固定 ssnStamp，保证语句与参数断言确定。
package workflow

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// ssnStamp 是测试统一使用的固定时间（注入假时钟，避免真实时间）。
var ssnStamp = time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)

// ssnNextCols 是 workflow_next_activity / workflow_his_next_activity 的全列（与表定义顺序一致）。
var ssnNextCols = []string{
	"id", "next_activity_id", "group_id", "identity_id", "send_identity_id", "send_user_name",
	"process_instance_id", "agree_weight", "reject_weight", "used", "pre_task_id", "task_id", "version",
	"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id",
}

// ssnNewStores 创建 sqlmock 连接、固定假时钟的仓储入口与下一步适配器聚合（带连接池）。
// @param t 测试上下文
// @return 操作上下文
// @return sqlmock 控制器
// @return 适配器聚合
func ssnNewStores(t *testing.T) (context.Context, sqlmock.Sqlmock, *SQLStoreSet) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// 假时钟固定仓储自动填充的 create_time/update_time
	store, err := pwf.NewStore(db, pwf.DialectMySQL, pwf.WithClock(pwf.ClockFunc(func() time.Time { return ssnStamp })))
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	return context.Background(), mock, NewSQLStoresWithDB(store, db)
}

// ssnSelectSQL 拼装下一步表查询语句（列清单与仓储 list 方法一致）。
// @param table 表名
// @param where 条件片段（"?" 占位）
// @param order 排序列，空串表示不排序
// @return 查询语句
func ssnSelectSQL(table, where, order string) string {
	query := "SELECT " + strings.Join(ssnNextCols, ", ") + " FROM " + table + " WHERE " + where
	if order != "" {
		query += " ORDER BY " + order
	}
	return query
}

// ssnInsertSQL 拼装 INSERT 语句（列序与参数顺序一致）。
// @param table 表名
// @param columns 实际落库列
// @return 插入语句
func ssnInsertSQL(table string, columns []string) string {
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")
	return "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") VALUES (" + placeholders + ")"
}

// ssnUpdateUsedSQL 拼装消耗回写 UPDATE 语句（used/task_id/update_time 三列）。
// @param table 表名
// @return 更新语句
func ssnUpdateUsedSQL(table string) string {
	return "UPDATE " + table + " SET used = ?, task_id = ?, update_time = ? WHERE id = ? AND used = 0"
}

// ssnNextRow 构造一条下一步行值（仅填关键列，其余列由 mmRowsOf 补 NULL）。
// @param id 主键
// @param activity 目标节点 ID
// @param instance 流程实例 ID
// @param identity 处理人 ID
// @param used 消耗标记
// @param version 版本号
// @param created 创建时间
// @return 列名→值
func ssnNextRow(id, activity, instance, identity string, used, version int, created time.Time) map[string]any {
	return map[string]any{
		"id": id, "next_activity_id": activity, "process_instance_id": instance, "identity_id": identity,
		"used": used, "version": version, "create_time": created,
	}
}

// ssnExpectErrorCode 断言错误是携带指定稳定错误码的引擎错误。
// @param t 测试上下文
// @param err 实际错误
// @param code 期望错误码
func ssnExpectErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际无错误", code)
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("期望引擎错误 %s，实际 %T: %v", code, err, err)
	}
	if apiErr.Code != code {
		t.Fatalf("错误码应为 %s，实际 %s（%v）", code, apiErr.Code, err)
	}
}

// TestStoreSQLNextInsertDual 验证双写插入的列/参数、可空列省略与两表错误分支。
// @param t 测试上下文
func TestStoreSQLNextInsertDual(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	// 全量列：可空列与公共审计列均显式提供，两表同 ID 同内容
	act := &NextActivity{
		BaseColumns: BaseColumns{
			ID: "na-1", Creator: "op", CreatorName: "张三", CreateTime: ssnStamp,
			Updater: "editor", UpdateTime: ssnStamp, TenantID: "sys",
		},
		NextActivityID: "act-a", GroupID: "grp-1", IdentityID: "u1",
		SendIdentityID: "sender-1", SendUserName: "发起人", ProcessInstanceID: "pi-1",
		AgreeWeight: floatPtr(3.5), RejectWeight: floatPtr(1.5),
		Used: 1, PreTaskID: "task-0", TaskID: "task-1", Version: 2,
	}
	fullArgs := []driver.Value{
		"na-1", "act-a", "grp-1", "u1", "sender-1", "发起人", "pi-1", 3.5, 1.5, 1,
		"task-0", "task-1", 2, "op", "张三", ssnStamp, "editor", ssnStamp, "sys",
	}
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_next_activity", ssnNextCols))).
		WithArgs(fullArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_his_next_activity", ssnNextCols))).
		WithArgs(fullArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := stores.Next.InsertDual(ctx, act); err != nil {
		t.Fatalf("双写插入失败: %v", err)
	}
	// 可空列省略：send_identity_id/send_user_name/权重不落库，时间列由假时钟填充（15 列）
	minimalCols := []string{
		"id", "next_activity_id", "group_id", "identity_id", "process_instance_id", "used",
		"pre_task_id", "task_id", "version", "creator", "creator_name", "create_time",
		"updater", "update_time", "tenant_id",
	}
	minimalArgs := []driver.Value{
		"na-2", "act-b", "", "u2", "pi-1", 0, "task-0", "", 1, "op", "", ssnStamp, "", ssnStamp, "sys",
	}
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_next_activity", minimalCols))).
		WithArgs(minimalArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_his_next_activity", minimalCols))).
		WithArgs(minimalArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	minimal := &NextActivity{
		BaseColumns:       BaseColumns{ID: "na-2", Creator: "op", TenantID: "sys"},
		NextActivityID:    "act-b",
		IdentityID:        "u2",
		ProcessInstanceID: "pi-1",
		PreTaskID:         "task-0",
		Version:           1,
	}
	if err := stores.Next.InsertDual(ctx, minimal); err != nil {
		t.Fatalf("省略可空列的双写失败: %v", err)
	}
	// 运行态插入失败：直接上抛，不再写历史表
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_next_activity", ssnNextCols))).
		WithArgs(fullArgs...).WillReturnError(errors.New("运行态插入失败"))
	ssnExpectErrorCode(t, stores.Next.InsertDual(ctx, act), "WF-0009")
	// 历史表插入失败：运行态已写但整体返回错误（依赖外层事务回滚）
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_next_activity", ssnNextCols))).
		WithArgs(fullArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(ssnInsertSQL("workflow_his_next_activity", ssnNextCols))).
		WithArgs(fullArgs...).WillReturnError(errors.New("历史表插入失败"))
	ssnExpectErrorCode(t, stores.Next.InsertDual(ctx, act), "WF-0009")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextLatestVersion 验证版本降序查询中取首条已消耗记录的语义与错误分支。
// @param t 测试上下文
func TestStoreSQLNextLatestVersion(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	query := regexp.QuoteMeta(ssnSelectSQL("workflow_his_next_activity",
		"process_instance_id = ? AND next_activity_id = ?", "version DESC"))
	// 版本降序：v5 未消耗、v4 已消耗、v3 已消耗 → 取 v4 并停止
	rows := mmRowsOf(ssnNextCols,
		ssnNextRow("h5", "act-a", "pi-1", "u1", 0, 5, ssnStamp),
		ssnNextRow("h4", "act-a", "pi-1", "u1", 1, 4, ssnStamp),
		ssnNextRow("h3", "act-a", "pi-1", "u1", 1, 3, ssnStamp),
	)
	mock.ExpectQuery(query).WithArgs("pi-1", "act-a").WillReturnRows(rows)
	version, err := stores.Next.LatestVersion(ctx, "pi-1", "act-a")
	if err != nil || version != 4 {
		t.Fatalf("最大已消耗版本应为 4，实际 %d（%v）", version, err)
	}
	// 全部未消耗 → 0
	mock.ExpectQuery(query).WithArgs("pi-1", "act-a").
		WillReturnRows(mmRowsOf(ssnNextCols, ssnNextRow("h1", "act-a", "pi-1", "u1", 0, 1, ssnStamp)))
	if version, err := stores.Next.LatestVersion(ctx, "pi-1", "act-a"); err != nil || version != 0 {
		t.Fatalf("无已消耗记录版本应为 0，实际 %d（%v）", version, err)
	}
	// 空结果 → 0
	mock.ExpectQuery(query).WithArgs("pi-1", "act-a").WillReturnRows(sqlmock.NewRows(ssnNextCols))
	if version, err := stores.Next.LatestVersion(ctx, "pi-1", "act-a"); err != nil || version != 0 {
		t.Fatalf("空结果版本应为 0，实际 %d（%v）", version, err)
	}
	// 查询失败 → 错误上抛
	mock.ExpectQuery(query).WithArgs("pi-1", "act-a").WillReturnError(errors.New("db down"))
	_, err = stores.Next.LatestVersion(ctx, "pi-1", "act-a")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextListQueries 验证未消耗/按节点/按实例/历史列表查询与错误分支。
// @param t 测试上下文
func TestStoreSQLNextListQueries(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	// 实例+节点未消耗：版本升序，DECIMAL 以字节形式回读
	byActivity := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity",
		"process_instance_id = ? AND next_activity_id = ? AND used = 0", "version"))
	rowValues := ssnNextRow("na-1", "act-a", "pi-1", "u1", 0, 1, ssnStamp)
	rowValues["agree_weight"] = []byte("2.5")
	rowValues["send_identity_id"] = []byte("sender-1")
	mock.ExpectQuery(byActivity).WithArgs("pi-1", "act-a").
		WillReturnRows(mmRowsOf(ssnNextCols, rowValues))
	acts, err := stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-1", "act-a")
	if err != nil || len(acts) != 1 {
		t.Fatalf("按实例与节点查询失败: %d（%v）", len(acts), err)
	}
	if acts[0].ID != "na-1" || acts[0].AgreeWeight == nil || *acts[0].AgreeWeight != 2.5 || acts[0].SendIdentityID != "sender-1" {
		t.Fatalf("行映射不符: %+v", acts[0])
	}
	// 实例+节点查询失败
	mock.ExpectQuery(byActivity).WithArgs("pi-bad", "act-a").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListUnusedByInstanceAndActivity(ctx, "pi-bad", "act-a")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 实例全部未消耗
	byInstance := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity",
		"process_instance_id = ? AND used = 0", "version"))
	mock.ExpectQuery(byInstance).WithArgs("pi-1").
		WillReturnRows(mmRowsOf(ssnNextCols,
			ssnNextRow("na-1", "act-a", "pi-1", "u1", 0, 1, ssnStamp),
			ssnNextRow("na-2", "act-b", "pi-1", "u2", 0, 2, ssnStamp)))
	if acts, err := stores.Next.ListUnusedByInstance(ctx, "pi-1"); err != nil || len(acts) != 2 {
		t.Fatalf("按实例查询失败: %d（%v）", len(acts), err)
	}
	mock.ExpectQuery(byInstance).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListUnusedByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 实例全部历史（升序）
	his := regexp.QuoteMeta(ssnSelectSQL("workflow_his_next_activity", "process_instance_id = ?", "version"))
	mock.ExpectQuery(his).WithArgs("pi-1").
		WillReturnRows(mmRowsOf(ssnNextCols, ssnNextRow("h1", "act-a", "pi-1", "u1", 1, 1, ssnStamp)))
	if acts, err := stores.Next.ListHisByInstance(ctx, "pi-1"); err != nil || len(acts) != 1 || acts[0].ID != "h1" {
		t.Fatalf("历史列表查询失败: %d（%v）", len(acts), err)
	}
	mock.ExpectQuery(his).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListHisByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextGetFirstUnused 验证按创建时间取最早一条（覆盖插入排序交换分支）。
// @param t 测试上下文
func TestStoreSQLNextGetFirstUnused(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	query := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity",
		"process_instance_id = ? AND next_activity_id = ? AND used = 0", "version"))
	// 仓储按 version 返回，创建时间乱序：version 3 最早
	mock.ExpectQuery(query).WithArgs("pi-1", "act-a").WillReturnRows(mmRowsOf(ssnNextCols,
		ssnNextRow("na-v1", "act-a", "pi-1", "u1", 0, 1, ssnStamp.Add(2*time.Hour)),
		ssnNextRow("na-v2", "act-a", "pi-1", "u2", 0, 2, ssnStamp.Add(3*time.Hour)),
		ssnNextRow("na-v3", "act-a", "pi-1", "u3", 0, 3, ssnStamp),
	))
	act, found, err := stores.Next.GetFirstUnusedByActivity(ctx, "pi-1", "act-a")
	if err != nil || !found || act == nil {
		t.Fatalf("查询最早未消耗记录失败: %v（%v）", found, err)
	}
	if act.ID != "na-v3" || !act.CreateTime.Equal(ssnStamp) {
		t.Fatalf("应按创建时间取最早记录，实际 %+v", act)
	}
	// 无记录 → (nil,false,nil)
	mock.ExpectQuery(query).WithArgs("pi-1", "act-none").WillReturnRows(sqlmock.NewRows(ssnNextCols))
	if act, found, err := stores.Next.GetFirstUnusedByActivity(ctx, "pi-1", "act-none"); err != nil || found || act != nil {
		t.Fatalf("空结果应为未命中: %v %v %v", act, found, err)
	}
	// 查询失败 → 错误上抛
	mock.ExpectQuery(query).WithArgs("pi-bad", "act-a").WillReturnError(errors.New("db down"))
	_, _, err = stores.Next.GetFirstUnusedByActivity(ctx, "pi-bad", "act-a")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextListByPreTaskAndTask 验证按来源任务/任务 ID 查询与错误分支。
// @param t 测试上下文
func TestStoreSQLNextListByPreTaskAndTask(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	// 按来源任务：版本升序
	byPreTask := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity", "pre_task_id = ?", "version"))
	mock.ExpectQuery(byPreTask).WithArgs("task-0").
		WillReturnRows(mmRowsOf(ssnNextCols,
			ssnNextRow("na-1", "act-a", "pi-1", "u1", 0, 1, ssnStamp),
			ssnNextRow("na-2", "act-b", "pi-1", "u2", 0, 2, ssnStamp)))
	acts, err := stores.Next.ListByPreTask(ctx, "task-0")
	if err != nil || len(acts) != 2 || acts[1].NextActivityID != "act-b" {
		t.Fatalf("按来源任务查询失败: %d（%v）", len(acts), err)
	}
	mock.ExpectQuery(byPreTask).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListByPreTask(ctx, "task-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按任务 ID：不排序
	byTask := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity", "task_id = ?", ""))
	mock.ExpectQuery(byTask).WithArgs("task-1").
		WillReturnRows(mmRowsOf(ssnNextCols, ssnNextRow("na-3", "act-c", "pi-1", "u3", 1, 3, ssnStamp)))
	acts, err = stores.Next.ListByTaskID(ctx, "task-1")
	if err != nil || len(acts) != 1 || acts[0].Used != 1 || acts[0].Version != 3 {
		t.Fatalf("按任务查询失败: %+v（%v）", acts, err)
	}
	mock.ExpectQuery(byTask).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListByTaskID(ctx, "task-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextMarkUsedAndWriteBack 验证运行态与历史表同步消耗回写及错误分支。
// @param t 测试上下文
func TestStoreSQLNextMarkUsedAndWriteBack(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	nextUpdate := regexp.QuoteMeta(ssnUpdateUsedSQL("workflow_next_activity"))
	hisUpdate := regexp.QuoteMeta(ssnUpdateUsedSQL("workflow_his_next_activity"))
	// 两条记录：每条先运行态后历史表，update_time 取假时钟
	for _, id := range []string{"na-1", "na-2"} {
		mock.ExpectExec(nextUpdate).WithArgs(1, "task-9", ssnStamp, id).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(hisUpdate).WithArgs(1, "task-9", ssnStamp, id).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	if err := stores.Next.MarkUsedAndWriteBack(ctx, []string{"na-1", "na-2"}, "task-9"); err != nil {
		t.Fatalf("批量消耗回写失败: %v", err)
	}
	// 运行态更新失败 → 立即上抛，不写历史表
	mock.ExpectExec(nextUpdate).WithArgs(1, "task-9", ssnStamp, "na-3").WillReturnError(errors.New("next update failed"))
	ssnExpectErrorCode(t, stores.Next.MarkUsedAndWriteBack(ctx, []string{"na-3"}, "task-9"), "WF-0016")
	// 历史表更新失败 → 上抛
	mock.ExpectExec(nextUpdate).WithArgs(1, "task-9", ssnStamp, "na-4").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(hisUpdate).WithArgs(1, "task-9", ssnStamp, "na-4").WillReturnError(errors.New("his update failed"))
	ssnExpectErrorCode(t, stores.Next.MarkUsedAndWriteBack(ctx, []string{"na-4"}, "task-9"), "WF-0016")
	// 空 ID 集合：不产生任何语句，直接成功
	if err := stores.Next.MarkUsedAndWriteBack(ctx, nil, "task-9"); err != nil {
		t.Fatalf("空集合应直接成功: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextRawQueries 验证裸 SQL 补齐通道：注入连接池的成功/失败分支与缺连接池分支。
// @param t 测试上下文
func TestStoreSQLNextRawQueries(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	usedQuery := regexp.QuoteMeta("SELECT * FROM workflow_next_activity WHERE process_instance_id = ? AND task_id IS NOT NULL ORDER BY create_time")
	// 成功：驱动返回 []byte，queryRaw 归一化为字符串
	usedValues := ssnNextRow("na-raw", "act-raw", "pi-1", "u1", 1, 2, ssnStamp)
	usedValues["id"] = []byte("na-raw")
	usedValues["next_activity_id"] = []byte("act-raw")
	usedValues["process_instance_id"] = []byte("pi-1")
	usedValues["task_id"] = []byte("task-1")
	mock.ExpectQuery(usedQuery).WithArgs("pi-1").WillReturnRows(mmRowsOf(ssnNextCols, usedValues))
	acts, err := stores.Next.ListUsedByInstance(ctx, "pi-1")
	if err != nil || len(acts) != 1 {
		t.Fatalf("已回写记录查询失败: %d（%v）", len(acts), err)
	}
	if acts[0].ID != "na-raw" || acts[0].NextActivityID != "act-raw" || acts[0].TaskID != "task-1" || acts[0].Used != 1 {
		t.Fatalf("[]byte 归一化不符: %+v", acts[0])
	}
	// 查询失败 → SQL-ADAPTER-002
	mock.ExpectQuery(usedQuery).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListUsedByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "SQL-ADAPTER-002")
	// 遍历结果集失败 → 原样上抛 rows.Err()（先给一行再注入读取错误）
	errRows := sqlmock.NewRows(ssnNextCols)
	nilRow := make([]driver.Value, len(ssnNextCols))
	errRows.AddRow(nilRow...)
	errRows.RowError(0, errors.New("结果集读取失败"))
	mock.ExpectQuery(usedQuery).WithArgs("pi-err").WillReturnRows(errRows)
	if _, err := stores.Next.ListUsedByInstance(ctx, "pi-err"); err == nil {
		t.Fatal("结果集遍历失败应上抛错误")
	}
	// 历史表缺口查询（task_id 匹配且非自环）
	hisQuery := regexp.QuoteMeta("SELECT * FROM workflow_his_next_activity WHERE task_id = ? AND (pre_task_id IS NULL OR pre_task_id != ?) ORDER BY create_time")
	hisValues := ssnNextRow("h-raw", "act-a", "pi-1", "u1", 1, 3, ssnStamp)
	hisValues["task_id"] = "task-1"
	hisValues["pre_task_id"] = "task-0"
	mock.ExpectQuery(hisQuery).WithArgs("task-1", "task-1").WillReturnRows(mmRowsOf(ssnNextCols, hisValues))
	acts, err = stores.Next.ListHisByTaskID(ctx, "task-1")
	if err != nil || len(acts) != 1 || acts[0].ID != "h-raw" || acts[0].PreTaskID != "task-0" {
		t.Fatalf("历史缺口查询失败: %+v（%v）", acts, err)
	}
	mock.ExpectQuery(hisQuery).WithArgs("task-bad", "task-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Next.ListHisByTaskID(ctx, "task-bad")
	ssnExpectErrorCode(t, err, "SQL-ADAPTER-002")
	// 缺连接池分支：NewSQLStores 不注入 db，两个缺口方法均报 SQL-ADAPTER-001
	noDB := NewSQLStores(stores.Next.core.store)
	if _, err := noDB.Next.ListUsedByInstance(ctx, "pi-1"); err != nil {
		ssnExpectErrorCode(t, err, "SQL-ADAPTER-001")
	} else {
		t.Fatal("缺连接池的已回写查询应报错")
	}
	if _, err := noDB.Next.ListHisByTaskID(ctx, "task-1"); err != nil {
		ssnExpectErrorCode(t, err, "SQL-ADAPTER-001")
	} else {
		t.Fatal("缺连接池的历史缺口查询应报错")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextDelete 验证按实例删除、按节点集合删除与退回清理的成功/错误分支。
// @param t 测试上下文
func TestStoreSQLNextDelete(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	// 按实例删除运行态
	byInstance := regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE process_instance_id = ?")
	mock.ExpectExec(byInstance).WithArgs("pi-1").WillReturnResult(sqlmock.NewResult(0, 3))
	if err := stores.Next.DeleteByProcessInstance(ctx, "pi-1"); err != nil {
		t.Fatalf("按实例删除失败: %v", err)
	}
	mock.ExpectExec(byInstance).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Next.DeleteByProcessInstance(ctx, "pi-bad"), "WF-0018")
	// 按实例+节点集合删除：运行态与历史表各一条 DELETE
	byIDs := regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE process_instance_id = ? AND next_activity_id IN (?, ?)")
	hisByIDs := regexp.QuoteMeta("DELETE FROM workflow_his_next_activity WHERE process_instance_id = ? AND next_activity_id IN (?, ?)")
	mock.ExpectExec(byIDs).WithArgs("pi-1", "act-a", "act-b").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(hisByIDs).WithArgs("pi-1", "act-a", "act-b").WillReturnResult(sqlmock.NewResult(0, 2))
	if err := stores.Next.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"act-a", "act-b"}); err != nil {
		t.Fatalf("按节点集合删除失败: %v", err)
	}
	// 空集合：直接返回，无任何 SQL
	if err := stores.Next.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", nil); err != nil {
		t.Fatalf("空集合应直接成功: %v", err)
	}
	// 运行态删除失败 → 不删历史表
	mock.ExpectExec(byIDs).WithArgs("pi-bad", "act-a", "act-b").WillReturnError(errors.New("next delete failed"))
	ssnExpectErrorCode(t, stores.Next.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-bad", []string{"act-a", "act-b"}), "WF-0018")
	// 历史表删除失败 → 上抛
	oneNext := regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE process_instance_id = ? AND next_activity_id IN (?)")
	oneHis := regexp.QuoteMeta("DELETE FROM workflow_his_next_activity WHERE process_instance_id = ? AND next_activity_id IN (?)")
	mock.ExpectExec(oneNext).WithArgs("pi-1", "act-a").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(oneHis).WithArgs("pi-1", "act-a").WillReturnError(errors.New("his delete failed"))
	ssnExpectErrorCode(t, stores.Next.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"act-a"}), "WF-0018")
	// 退回清理：只删目标节点运行态（单元素 IN 子句）
	mock.ExpectExec(oneNext).WithArgs("pi-1", "act-a").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Next.DeleteForBackToStep(ctx, "pi-1", "act-a"); err != nil {
		t.Fatalf("退回清理失败: %v", err)
	}
	mock.ExpectExec(oneNext).WithArgs("pi-bad", "act-a").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Next.DeleteForBackToStep(ctx, "pi-bad", "act-a"), "WF-0018")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextDeleteForRollback 验证撤回清理的实例/保留节点过滤与错误分支。
// @param t 测试上下文
func TestStoreSQLNextDeleteForRollback(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	query := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity", "pre_task_id = ?", "version"))
	// 三条来源记录：仅第一条（同实例且非保留节点）被删除
	mock.ExpectQuery(query).WithArgs("task-0").WillReturnRows(mmRowsOf(ssnNextCols,
		ssnNextRow("rb-1", "act-drop", "pi-1", "u1", 0, 1, ssnStamp),
		ssnNextRow("rb-keep", "act-keep", "pi-1", "u2", 0, 2, ssnStamp),
		ssnNextRow("rb-other", "act-drop", "pi-2", "u3", 0, 3, ssnStamp),
	))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE id = ?")).
		WithArgs("rb-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Next.DeleteForRollback(ctx, "pi-1", "task-0", "act-keep"); err != nil {
		t.Fatalf("撤回清理失败: %v", err)
	}
	// 查询来源记录失败
	mock.ExpectQuery(query).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Next.DeleteForRollback(ctx, "pi-1", "task-bad", "act-keep"), "WF-0010")
	// 逐条删除失败
	mock.ExpectQuery(query).WithArgs("task-0").WillReturnRows(mmRowsOf(ssnNextCols,
		ssnNextRow("rb-1", "act-drop", "pi-1", "u1", 0, 1, ssnStamp)))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE id = ?")).
		WithArgs("rb-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Next.DeleteForRollback(ctx, "pi-1", "task-0", "act-keep"), "WF-0018")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextDeleteForResolve 验证处理委派清理的四条件过滤与错误分支。
// @param t 测试上下文
func TestStoreSQLNextDeleteForResolve(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	query := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity", "task_id = ?", ""))
	// 四条记录：仅实例/节点/任务/执行人全匹配的第一条被删除
	mock.ExpectQuery(query).WithArgs("task-1").WillReturnRows(mmRowsOf(ssnNextCols,
		ssnNextRow("rv-1", "act-a", "pi-1", "u1", 0, 1, ssnStamp),
		ssnNextRow("rv-2", "act-a", "pi-1", "u2", 0, 2, ssnStamp),
		ssnNextRow("rv-3", "act-b", "pi-1", "u1", 0, 3, ssnStamp),
		ssnNextRow("rv-4", "act-a", "pi-2", "u1", 0, 4, ssnStamp),
	))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE id = ?")).
		WithArgs("rv-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Next.DeleteForResolve(ctx, "pi-1", "act-a", "task-1", "u1"); err != nil {
		t.Fatalf("委派清理失败: %v", err)
	}
	// 查询任务记录失败
	mock.ExpectQuery(query).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Next.DeleteForResolve(ctx, "pi-1", "act-a", "task-bad", "u1"), "WF-0010")
	// 逐条删除失败
	mock.ExpectQuery(query).WithArgs("task-1").WillReturnRows(mmRowsOf(ssnNextCols,
		ssnNextRow("rv-1", "act-a", "pi-1", "u1", 0, 1, ssnStamp)))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow_next_activity WHERE id = ?")).
		WithArgs("rv-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Next.DeleteForResolve(ctx, "pi-1", "act-a", "task-1", "u1"), "WF-0018")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextFindAssignSource 验证按实例/节点/发送人/任务四条件定位与错误分支。
// @param t 测试上下文
func TestStoreSQLNextFindAssignSource(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	query := regexp.QuoteMeta(ssnSelectSQL("workflow_next_activity", "task_id = ?", ""))
	// 首条发送人不匹配，第二条命中并映射权重（DECIMAL 字节）
	hitValues := ssnNextRow("as-2", "act-a", "pi-1", "u2", 1, 4, ssnStamp)
	hitValues["task_id"] = "task-1"
	hitValues["send_identity_id"] = "sender-1"
	hitValues["agree_weight"] = []byte("4.5")
	missValues := ssnNextRow("as-1", "act-a", "pi-1", "u1", 0, 3, ssnStamp)
	missValues["task_id"] = "task-1"
	missValues["send_identity_id"] = "sender-2"
	mock.ExpectQuery(query).WithArgs("task-1").WillReturnRows(mmRowsOf(ssnNextCols, missValues, hitValues))
	act, found, err := stores.Next.FindAssignSource(ctx, "pi-1", "act-a", "sender-1", "task-1")
	if err != nil || !found || act == nil {
		t.Fatalf("来源定位失败: %v（%v）", found, err)
	}
	if act.ID != "as-2" || act.IdentityID != "u2" || act.AgreeWeight == nil || *act.AgreeWeight != 4.5 {
		t.Fatalf("来源记录映射不符: %+v", act)
	}
	// 无匹配 → (nil,false,nil)
	mock.ExpectQuery(query).WithArgs("task-1").WillReturnRows(mmRowsOf(ssnNextCols, missValues))
	if act, found, err := stores.Next.FindAssignSource(ctx, "pi-1", "act-a", "sender-1", "task-1"); err != nil || found || act != nil {
		t.Fatalf("无匹配应为未命中: %v %v %v", act, found, err)
	}
	// 查询失败 → 错误上抛
	mock.ExpectQuery(query).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	_, _, err = stores.Next.FindAssignSource(ctx, "pi-1", "act-a", "sender-1", "task-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLNextRowConverters 验证下一步行转换对可空数值/时间类型的兼容与省略语义。
// @param t 测试上下文
func TestStoreSQLNextRowConverters(t *testing.T) {
	// 可空浮点：float32/int64/int/非法串/非法字节/其他类型/缺失
	row := pwf.Row{
		"agree_weight":  float32(2.5),
		"reject_weight": int64(1),
		"float_int":     int(7),
		"float_bad":     "not-a-number",
		"float_bytes":   []byte("bad"),
		"float_other":   struct{}{},
	}
	if ptr := rowFloatPtr(row, "agree_weight"); ptr == nil || *ptr != 2.5 {
		t.Fatalf("float32 权重读取错误: %v", ptr)
	}
	if ptr := rowFloatPtr(row, "reject_weight"); ptr == nil || *ptr != 1 {
		t.Fatalf("int64 权重读取错误: %v", ptr)
	}
	if ptr := rowFloatPtr(row, "float_int"); ptr == nil || *ptr != 7 {
		t.Fatalf("int 权重读取错误: %v", ptr)
	}
	if rowFloatPtr(row, "float_bad") != nil || rowFloatPtr(row, "float_bytes") != nil ||
		rowFloatPtr(row, "float_other") != nil || rowFloatPtr(row, "float_missing") != nil {
		t.Fatal("非法/缺失浮点应为 nil")
	}
	// 时间：文本两种布局/非法文本/非时间类型/缺失
	timeRow := pwf.Row{
		"create_time": "2026-03-01 09:00:00",
		"update_time": "2026-03-01T09:00:00Z",
		"bad_time":    "2026/03/01",
		"int_time":    int64(1),
	}
	// 文本无时区信息，按 UTC 解析
	textStamp := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	if ptr := rowTimePtr(timeRow, "create_time"); ptr == nil || !ptr.Equal(textStamp) {
		t.Fatalf("日期时间文本解析错误: %v", ptr)
	}
	if ptr := rowTimePtr(timeRow, "update_time"); ptr == nil || !ptr.Equal(textStamp) {
		t.Fatalf("RFC3339 文本解析错误: %v", ptr)
	}
	if rowTimePtr(timeRow, "bad_time") != nil || rowTimePtr(timeRow, "int_time") != nil ||
		rowTimePtr(timeRow, "missing_time") != nil {
		t.Fatal("非法/缺失时间应为 nil")
	}
	// 实体映射：缺失列一律取零值（NULL 语义）
	empty := nextFromRow(pwf.Row{})
	if empty.ID != "" || empty.Used != 0 || empty.Version != 0 || empty.AgreeWeight != nil || !empty.CreateTime.IsZero() {
		t.Fatalf("缺失列应取零值: %+v", empty)
	}
	// 写入：可空列全空时对应键被省略，避免覆盖为 NULL 以外的值
	minimal := nextToRow(&NextActivity{BaseColumns: BaseColumns{ID: "na-min"}})
	for _, key := range []string{"send_identity_id", "send_user_name", "agree_weight", "reject_weight", "create_time", "update_time"} {
		if _, ok := minimal[key]; ok {
			t.Fatalf("空值列 %s 不应写入行", key)
		}
	}
}

// ─── 其余四域的全列表与通用期望构造 ─────────────────────────────

// sscMessageBaseCols 是待办/已办/委托镜像共用的业务扩展列（与仓储表定义一致，不含公共审计列）。
var sscMessageBaseCols = []string{
	"id", "task_id", "process_instance_id", "title", "message", "business_id", "target_url",
	"task_step_name", "task_step_code", "flow_category_code", "flow_category_name",
	"owner_id", "owner_name", "owner_group", "owner_group_id", "owner_group_paths", "owner_group_path_ids",
	"document_owner_id", "document_owner_name", "document_group_id", "document_group", "document_group_paths", "document_group_path_ids",
	"sender_id", "sender", "sender_step_name", "sender_step_code", "consigner_id", "consigner",
	"is_read", "read_time", "plan_finish_time", "task_receipt_time", "task_start_time", "is_delay", "delay_time", "message_group_id",
}

// sscTailCols 是公共审计列（所有 workflow 表共有的尾部列）。
var sscTailCols = []string{"creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// sscMessageCols 是 workflow_task_message_extend 全列。
var sscMessageCols = sscTableCols(sscMessageBaseCols, GetExtendColumns(), sscTailCols)

// sscHistoryCols 是 workflow_task_history_extend 全列（业务列后多 task_finish_time）。
var sscHistoryCols = sscTableCols(sscMessageBaseCols, []string{"task_finish_time"}, GetExtendColumns(), sscTailCols)

// sscOperationCols 是 workflow_operation_record 全列。
var sscOperationCols = sscTableCols([]string{
	"id", "business_id", "task_id", "process_instance_id", "message", "is_agree",
	"task_step_code", "task_step_name", "flow_category_code", "flow_category_name",
	"owner_id", "owner_name", "owner_group", "owner_group_id", "owner_group_paths", "owner_group_path_ids",
	"deal_time", "task_receipt_time", "task_start_time", "task_finish_time", "type",
}, sscTailCols)

// sscDocumentCols 是 workflow_document 全列。
var sscDocumentCols = sscTableCols([]string{
	"id", "business_id", "create_group_path_ids", "create_group_paths", "create_group_id", "create_group",
	"updater_name", "deleted", "document_url", "document_id", "document_table", "flow_title",
	"flow_task_id", "flow_instance_id", "flow_category_id", "flow_category", "flow_action_id", "flow_action",
	"flow_status", "flow_current_step_name", "flow_current_persons",
	"flow_start_time", "flow_finish_time", "flow_termination_time",
}, GetExtendColumns(), sscTailCols)

// sscTableCols 按组拼接列清单（复制切片，避免多个表定义共享底层数组）。
// @param groups 列组
// @return 拼接后的列清单
func sscTableCols(groups ...[]string) []string {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	cols := make([]string, 0, total)
	for _, group := range groups {
		cols = append(cols, group...)
	}
	return cols
}

// sscPresentCols 按表列序返回行中实际出现的列。
// @param tableCols 表全列
// @param row 行数据
// @return 实际出现的列
func sscPresentCols(tableCols []string, row pwf.Row) []string {
	cols := make([]string, 0, len(row))
	for _, column := range tableCols {
		if _, ok := row[column]; ok {
			cols = append(cols, column)
		}
	}
	return cols
}

// sscRowArgs 按列序从行中取值生成 SQL 参数。
// @param cols 列清单
// @param row 行数据
// @return 参数列表
func sscRowArgs(cols []string, row pwf.Row) []driver.Value {
	args := make([]driver.Value, 0, len(cols))
	for _, column := range cols {
		args = append(args, row[column])
	}
	return args
}

// sscSelectSQL 按列清单/表/条件/排序拼装查询语句。
// @param cols 列清单
// @param table 表名
// @param where 条件片段
// @param order 排序列，空串表示不排序
// @return 查询语句
func sscSelectSQL(cols []string, table, where, order string) string {
	query := "SELECT " + strings.Join(cols, ", ") + " FROM " + table + " WHERE " + where
	if order != "" {
		query += " ORDER BY " + order
	}
	return query
}

// sscInsertParts 生成插入语句与按表列序的参数。
// @param table 表名
// @param tableCols 表全列
// @param row 行数据
// @return 插入语句
// @return 参数列表
func sscInsertParts(table string, tableCols []string, row pwf.Row) (string, []driver.Value) {
	cols := sscPresentCols(tableCols, row)
	return ssnInsertSQL(table, cols), sscRowArgs(cols, row)
}

// sscUpdateParts 生成按主键更新的 SET 语句与参数（update_time 缺省时补假时钟，id 不参与更新）。
// @param table 表名
// @param tableCols 表全列
// @param changes 变更列
// @param id 主键
// @return 更新语句
// @return 参数列表
func sscUpdateParts(table string, tableCols []string, changes pwf.Row, id string) (string, []driver.Value) {
	data := make(pwf.Row, len(changes)+1)
	for key, value := range changes {
		data[key] = value
	}
	// 与仓储 updateWhere 一致：未显式提供 update_time 时由时间源填充
	if _, ok := data["update_time"]; !ok {
		data["update_time"] = ssnStamp
	}
	delete(data, "id")
	cols := sscPresentCols(tableCols, data)
	sets := make([]string, 0, len(cols))
	for _, column := range cols {
		sets = append(sets, column+" = ?")
	}
	args := sscRowArgs(cols, data)
	args = append(args, id)
	return "UPDATE " + table + " SET " + strings.Join(sets, ", ") + " WHERE id = ?", args
}

// sscFullMessage 构造字段齐全的待办扩展实体（用于落库列与行映射断言）。
// @param id 主键
// @param taskID 任务 Id
// @param instanceID 流程实例 Id
// @param ownerID 处理人 Id
// @param tenantID 租户
// @return 待办扩展实体
func sscFullMessage(id, taskID, instanceID, ownerID, tenantID string) *MessageExtend {
	return &MessageExtend{
		BaseColumns: BaseColumns{
			ID: id, Creator: "op", CreatorName: "张三", CreateTime: ssnStamp,
			Updater: "editor", UpdateTime: ssnStamp, TenantID: tenantID,
		},
		TaskID: taskID, ProcessInstanceID: instanceID, Title: "待办标题", Message: "待办说明",
		BusinessID: "biz-1", TargetURL: "/task", TaskStepName: "审批", TaskStepCode: "act-a",
		FlowCategoryCode: "leave", FlowCategoryName: "请假",
		OwnerID: ownerID, OwnerName: "处理人", OwnerGroup: "部门", OwnerGroupID: "g1",
		OwnerGroupPaths: "公司/部门", OwnerGroupPathIDs: "g0/g1",
		DocumentOwnerID: "u0", DocumentOwnerName: "发起人", DocumentGroupID: "dg1",
		DocumentGroup: "发起部门", DocumentGroupPaths: "公司/发起部门", DocumentGroupPathIDs: "g0/dg1",
		SenderID: "s1", Sender: "发送人", SenderStepName: "申请", SenderStepCode: "apply",
		ConsignerID: "c1", Consigner: "受托人",
		IsRead: 1, ReadTime: &ssnStamp, PlanFinishTime: &ssnStamp,
		TaskReceiptTime: &ssnStamp, TaskStartTime: &ssnStamp,
		IsDelay: 1, DelayTime: &ssnStamp, MessageGroupID: "mg-1",
		ExtendFields: ExtendFields{ExtendS1: "x1", ExtendI1: int64Ptr(1), ExtendF1: floatPtr(1.5), ExtendD1: &ssnStamp},
	}
}

// sscFullHistory 构造字段齐全的已办扩展实体。
// @param id 主键
// @param taskID 任务 Id
// @param instanceID 流程实例 Id
// @param ownerID 处理人 Id
// @param tenantID 租户
// @return 已办扩展实体
func sscFullHistory(id, taskID, instanceID, ownerID, tenantID string) *HistoryExtend {
	return &HistoryExtend{MessageExtend: *sscFullMessage(id, taskID, instanceID, ownerID, tenantID), TaskFinishTime: &ssnStamp}
}

// sscMessageQueryRow 构造待办查询结果集（最小必要列，覆盖行映射关键字段）。
// @param id 主键
// @param taskID 任务 Id
// @param instanceID 流程实例 Id
// @param ownerID 处理人 Id
// @param tenantID 租户
// @return 结果集
func sscMessageQueryRow(id, taskID, instanceID, ownerID, tenantID string) *sqlmock.Rows {
	cols := []string{"id", "task_id", "process_instance_id", "owner_id", "title", "is_read", "read_time", "extend_s1", "create_time", "tenant_id"}
	return sqlmock.NewRows(cols).AddRow(id, taskID, instanceID, ownerID, "待办标题", 1, ssnStamp, "x1", ssnStamp, tenantID)
}

// TestStoreSQLMessageSaveAndGet 验证待办写入的列/参数、执行错误与按任务查询的三种结果。
// @param t 测试上下文
func TestStoreSQLMessageSaveAndGet(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	msg := sscFullMessage("msg-1", "task-1", "pi-1", "u1", "sys")
	row := messageToRow(msg)
	// 落库行先做关键列断言，再以此生成期望 SQL 参数
	if row["task_id"] != "task-1" || row["process_instance_id"] != "pi-1" || row["owner_id"] != "u1" ||
		row["is_read"] != 1 || row["extend_s1"] != "x1" || row["tenant_id"] != "sys" {
		t.Fatalf("待办行转换不符: %+v", row)
	}
	insertSQL, insertArgs := sscInsertParts("workflow_task_message_extend", sscMessageCols, row)
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := stores.Message.Save(ctx, msg); err != nil {
		t.Fatalf("待办写入失败: %v", err)
	}
	// 执行失败 → WF-0009
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.Save(ctx, msg), "WF-0009")
	// 按任务命中：可空/扩展列映射
	getSQL := sscSelectSQL(sscMessageCols, "workflow_task_message_extend", "task_id = ?", "") + " LIMIT 1"
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(sscMessageQueryRow("msg-1", "task-1", "pi-1", "u1", "sys"))
	got, found, err := stores.Message.GetByTaskID(ctx, "task-1")
	if err != nil || !found {
		t.Fatalf("待办查询失败: %v（%v）", found, err)
	}
	if got.ID != "msg-1" || got.Title != "待办标题" || got.OwnerID != "u1" || got.IsRead != 1 ||
		got.ReadTime == nil || !got.ReadTime.Equal(ssnStamp) || got.ExtendS1 != "x1" {
		t.Fatalf("待办行映射不符: %+v", got)
	}
	// 查无
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-none").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if got, found, err := stores.Message.GetByTaskID(ctx, "task-none"); err != nil || found || got != nil {
		t.Fatalf("查无应返回未命中: %v %v %v", got, found, err)
	}
	// 查询失败 → WF-0010
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	_, _, err = stores.Message.GetByTaskID(ctx, "task-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLMessageLists 验证按实例/业务/处理人列表查询及租户内存过滤与分页。
// @param t 测试上下文
func TestStoreSQLMessageLists(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	cols := []string{"id", "task_id", "process_instance_id", "business_id", "owner_id", "tenant_id", "create_time"}
	// 按实例
	byInstance := sscSelectSQL(sscMessageCols, "workflow_task_message_extend", "process_instance_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "process_instance_id": "pi-1", "owner_id": "u1", "tenant_id": "sys"},
		map[string]any{"id": "msg-2", "task_id": "task-2", "process_instance_id": "pi-1", "owner_id": "u2", "tenant_id": "sys"},
	))
	if rows, err := stores.Message.ListByInstance(ctx, "pi-1"); err != nil || len(rows) != 2 || rows[1].TaskID != "task-2" {
		t.Fatalf("按实例查询失败: %d（%v）", len(rows), err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err := stores.Message.ListByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按业务单据与租户
	byBusiness := sscSelectSQL(sscMessageCols, "workflow_task_message_extend", "business_id = ? AND tenant_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "business_id": "biz-1", "tenant_id": "sys"}))
	if rows, err := stores.Message.ListByBusinessID(ctx, "biz-1", "sys"); err != nil || len(rows) != 1 {
		t.Fatalf("按业务查询失败: %d（%v）", len(rows), err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-bad", "sys").WillReturnError(errors.New("db down"))
	_, err = stores.Message.ListByBusinessID(ctx, "biz-bad", "sys")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按处理人：仓储全量取回后内存租户过滤 + 分页
	byOwner := sscSelectSQL(sscMessageCols, "workflow_task_message_extend", "owner_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byOwner)).WithArgs("u1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "owner_id": "u1", "tenant_id": "t1"},
		map[string]any{"id": "msg-2", "owner_id": "u1", "tenant_id": "t2"},
		map[string]any{"id": "msg-3", "owner_id": "u1", "tenant_id": "t1"},
	))
	page, total, err := stores.Message.ListByOwner(ctx, "u1", "t1", PageQuery{Start: 0, Size: 1})
	if err != nil || total != 2 || len(page) != 1 || page[0].ID != "msg-1" {
		t.Fatalf("分页查询不符: total=%d len=%d err=%v", total, len(page), err)
	}
	// 空租户：不过滤，分页起始越界返回空页但总数不变
	mock.ExpectQuery(regexp.QuoteMeta(byOwner)).WithArgs("u1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "owner_id": "u1", "tenant_id": "t1"},
		map[string]any{"id": "msg-2", "owner_id": "u1", "tenant_id": "t2"},
		map[string]any{"id": "msg-3", "owner_id": "u1", "tenant_id": "t1"},
	))
	page, total, err = stores.Message.ListByOwner(ctx, "u1", "", PageQuery{Start: 9, Size: 2})
	if err != nil || total != 3 || len(page) != 0 {
		t.Fatalf("越界分页应返回空页: total=%d len=%d err=%v", total, len(page), err)
	}
	// 查询失败
	mock.ExpectQuery(regexp.QuoteMeta(byOwner)).WithArgs("u-bad").WillReturnError(errors.New("db down"))
	_, _, err = stores.Message.ListByOwner(ctx, "u-bad", "sys", PageQuery{})
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLMessageDelete 验证待办各删除路径：定位后逐条删除、条件删除、空集合零 SQL 与错误分支。
// @param t 测试上下文
func TestStoreSQLMessageDelete(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_task_message_extend"
	getSQL := sscSelectSQL(sscMessageCols, table, "task_id = ?", "") + " LIMIT 1"
	delByPK := regexp.QuoteMeta("DELETE FROM " + table + " WHERE id = ?")
	listByInstance := sscSelectSQL(sscMessageCols, table, "process_instance_id = ?", "create_time")
	cols := []string{"id", "task_id", "process_instance_id", "business_id", "owner_id", "tenant_id"}
	// 按任务直接删除
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM " + table + " WHERE task_id = ?")).WithArgs("task-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.DeleteByTaskID(ctx, "task-1"); err != nil {
		t.Fatalf("按任务删除失败: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM " + table + " WHERE task_id = ?")).WithArgs("task-bad").
		WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByTaskID(ctx, "task-bad"), "WF-0018")
	// 按任务+处理人：处理人匹配才删
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "owner_id": "u1"}))
	mock.ExpectExec(delByPK).WithArgs("msg-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.DeleteByTaskIDAndOwner(ctx, "task-1", "u1"); err != nil {
		t.Fatalf("按任务与处理人删除失败: %v", err)
	}
	// 处理人不匹配：不删除
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "owner_id": "u2"}))
	if err := stores.Message.DeleteByTaskIDAndOwner(ctx, "task-1", "u1"); err != nil {
		t.Fatalf("处理人不匹配应直接成功: %v", err)
	}
	// 查无：直接成功
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-none").WillReturnRows(sqlmock.NewRows(cols))
	if err := stores.Message.DeleteByTaskIDAndOwner(ctx, "task-none", "u1"); err != nil {
		t.Fatalf("查无应直接成功: %v", err)
	}
	// 查询失败 / 删除失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByTaskIDAndOwner(ctx, "task-bad", "u1"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "owner_id": "u1"}))
	mock.ExpectExec(delByPK).WithArgs("msg-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByTaskIDAndOwner(ctx, "task-1", "u1"), "WF-0018")
	// 按实例：列表后逐条删除
	mock.ExpectQuery(regexp.QuoteMeta(listByInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1"}, map[string]any{"id": "msg-2"}))
	mock.ExpectExec(delByPK).WithArgs("msg-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(delByPK).WithArgs("msg-2").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.DeleteByInstance(ctx, "pi-1"); err != nil {
		t.Fatalf("按实例删除失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(listByInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByInstance(ctx, "pi-bad"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(listByInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1"}))
	mock.ExpectExec(delByPK).WithArgs("msg-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByInstance(ctx, "pi-1"), "WF-0018")
	// 按业务单据：列表后逐条删除
	listByBusiness := sscSelectSQL(sscMessageCols, table, "business_id = ? AND tenant_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(listByBusiness)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1"}))
	mock.ExpectExec(delByPK).WithArgs("msg-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.DeleteByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatalf("按业务删除失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(listByBusiness)).WithArgs("biz-bad", "sys").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByBusinessID(ctx, "biz-bad", "sys"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(listByBusiness)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1"}))
	mock.ExpectExec(delByPK).WithArgs("msg-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByBusinessID(ctx, "biz-1", "sys"), "WF-0018")
	// 按实例+步骤集合：多元素 IN；空集合直接成功不产生 SQL
	bySteps := regexp.QuoteMeta("DELETE FROM " + table + " WHERE process_instance_id = ? AND task_step_code IN (?, ?)")
	mock.ExpectExec(bySteps).WithArgs("pi-1", "act-a", "act-b").WillReturnResult(sqlmock.NewResult(0, 2))
	if err := stores.Message.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"act-a", "act-b"}); err != nil {
		t.Fatalf("按步骤集合删除失败: %v", err)
	}
	if err := stores.Message.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", nil); err != nil {
		t.Fatalf("空步骤集合应直接成功: %v", err)
	}
	mock.ExpectExec(bySteps).WithArgs("pi-bad", "act-a", "act-b").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-bad", []string{"act-a", "act-b"}), "WF-0018")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLMessageUpdate 验证已读/延时/计划办结时间回写及可空时间 nil 分支。
// @param t 测试上下文
func TestStoreSQLMessageUpdate(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_task_message_extend"
	getSQL := sscSelectSQL(sscMessageCols, table, "task_id = ?", "") + " LIMIT 1"
	cols := []string{"id", "task_id", "owner_id"}
	readSQL := regexp.QuoteMeta("UPDATE " + table + " SET is_read = ?, read_time = ?, update_time = ? WHERE task_id = ?")
	// 已读：处理人匹配才更新
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "owner_id": "u1"}))
	mock.ExpectExec(readSQL).WithArgs(1, ssnStamp, ssnStamp, "task-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.UpdateRead(ctx, "task-1", "u1", ssnStamp); err != nil {
		t.Fatalf("更新已读失败: %v", err)
	}
	// 处理人不匹配：不更新
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "owner_id": "u2"}))
	if err := stores.Message.UpdateRead(ctx, "task-1", "u1", ssnStamp); err != nil {
		t.Fatalf("处理人不匹配应直接成功: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.UpdateRead(ctx, "task-bad", "u1", ssnStamp), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1", "owner_id": "u1"}))
	mock.ExpectExec(readSQL).WithArgs(1, ssnStamp, ssnStamp, "task-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.UpdateRead(ctx, "task-1", "u1", ssnStamp), "WF-0016")
	// 延时：定位后按主键更新两列
	delaySQL := regexp.QuoteMeta("UPDATE " + table + " SET is_delay = ?, delay_time = ?, update_time = ? WHERE id = ?")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1"}))
	mock.ExpectExec(delaySQL).WithArgs(1, ssnStamp, ssnStamp, "msg-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.UpdateDelay(ctx, "task-1", ssnStamp); err != nil {
		t.Fatalf("更新延时失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-none").WillReturnRows(sqlmock.NewRows(cols))
	if err := stores.Message.UpdateDelay(ctx, "task-none", ssnStamp); err != nil {
		t.Fatalf("查无应直接成功: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.UpdateDelay(ctx, "task-bad", ssnStamp), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1"}))
	mock.ExpectExec(delaySQL).WithArgs(1, ssnStamp, ssnStamp, "msg-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.UpdateDelay(ctx, "task-1", ssnStamp), "WF-0016")
	// 计划办结时间：非 nil 与 nil 都写同一列（nil 表示清空）
	planSQL := regexp.QuoteMeta("UPDATE " + table + " SET plan_finish_time = ?, update_time = ? WHERE id = ?")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1"}))
	mock.ExpectExec(planSQL).WithArgs(ssnStamp, ssnStamp, "msg-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.SetPlanFinishTime(ctx, "task-1", &ssnStamp); err != nil {
		t.Fatalf("设置计划办结时间失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1"}))
	mock.ExpectExec(planSQL).WithArgs(nil, ssnStamp, "msg-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Message.SetPlanFinishTime(ctx, "task-1", nil); err != nil {
		t.Fatalf("清空计划办结时间失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.SetPlanFinishTime(ctx, "task-bad", &ssnStamp), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "msg-1", "task_id": "task-1"}))
	mock.ExpectExec(planSQL).WithArgs(nil, ssnStamp, "msg-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Message.SetPlanFinishTime(ctx, "task-1", nil), "WF-0016")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// sscHistoryQueryRow 构造已办查询结果集（最小必要列，覆盖行映射关键字段）。
// @param id 主键
// @param taskID 任务 Id
// @param instanceID 流程实例 Id
// @param ownerID 处理人 Id
// @param stepCode 步骤代号
// @param tenantID 租户
// @return 结果集
func sscHistoryQueryRow(id, taskID, instanceID, ownerID, stepCode, tenantID string) *sqlmock.Rows {
	cols := []string{"id", "task_id", "process_instance_id", "owner_id", "task_step_code", "task_finish_time", "create_time", "tenant_id"}
	return sqlmock.NewRows(cols).AddRow(id, taskID, instanceID, ownerID, stepCode, ssnStamp, ssnStamp, tenantID)
}

// TestStoreSQLHistorySaveAndLists 验证已办写入的列/参数、执行错误与三种列表查询。
// @param t 测试上下文
func TestStoreSQLHistorySaveAndLists(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_task_history_extend"
	history := sscFullHistory("his-1", "task-1", "pi-1", "u1", "sys")
	row := historyToRow(history)
	// 落库行先做关键列断言（含 task_finish_time 与扩展列）
	if row["task_id"] != "task-1" || row["task_finish_time"] != ssnStamp || row["extend_s1"] != "x1" {
		t.Fatalf("已办行转换不符: %+v", row)
	}
	insertSQL, insertArgs := sscInsertParts(table, sscHistoryCols, row)
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := stores.History.Save(ctx, history); err != nil {
		t.Fatalf("已办写入失败: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.Save(ctx, history), "WF-0009")
	// 按任务命中
	getSQL := sscSelectSQL(sscHistoryCols, table, "task_id = ?", "") + " LIMIT 1"
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(sscHistoryQueryRow("his-1", "task-1", "pi-1", "u1", "act-a", "sys"))
	got, found, err := stores.History.GetByTaskID(ctx, "task-1")
	if err != nil || !found {
		t.Fatalf("已办查询失败: %v（%v）", found, err)
	}
	if got.ID != "his-1" || got.TaskID != "task-1" || got.TaskStepCode != "act-a" ||
		got.TaskFinishTime == nil || !got.TaskFinishTime.Equal(ssnStamp) {
		t.Fatalf("已办行映射不符: %+v", got)
	}
	// 查无
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-none").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if got, found, err := stores.History.GetByTaskID(ctx, "task-none"); err != nil || found || got != nil {
		t.Fatalf("查无应返回未命中: %v %v %v", got, found, err)
	}
	// 查询失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	_, _, err = stores.History.GetByTaskID(ctx, "task-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按实例列表（按办结时间排序）
	cols := []string{"id", "task_id", "process_instance_id", "owner_id", "task_step_code", "task_finish_time"}
	byInstance := sscSelectSQL(sscHistoryCols, table, "process_instance_id = ?", "task_finish_time")
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1", "owner_id": "u1"},
		map[string]any{"id": "his-2", "task_id": "task-2", "process_instance_id": "pi-1", "owner_id": "u2"},
	))
	if rows, err := stores.History.ListByInstance(ctx, "pi-1"); err != nil || len(rows) != 2 || rows[1].ID != "his-2" {
		t.Fatalf("按实例历史查询失败: %d（%v）", len(rows), err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.History.ListByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按业务与租户列表
	byBusiness := sscSelectSQL(sscHistoryCols, table, "business_id = ? AND tenant_id = ?", "task_finish_time")
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "owner_id": "u1"}))
	if rows, err := stores.History.ListByBusinessID(ctx, "biz-1", "sys"); err != nil || len(rows) != 1 {
		t.Fatalf("按业务历史查询失败: %d（%v）", len(rows), err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-bad", "sys").WillReturnError(errors.New("db down"))
	_, err = stores.History.ListByBusinessID(ctx, "biz-bad", "sys")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按处理人裸 SQL：分页 + 总数
	raw := regexp.QuoteMeta("SELECT * FROM workflow_task_history_extend WHERE owner_id = ? AND tenant_id = ? ORDER BY task_finish_time")
	mock.ExpectQuery(raw).WithArgs("u1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "owner_id": "u1"},
		map[string]any{"id": "his-2", "owner_id": "u1"},
		map[string]any{"id": "his-3", "owner_id": "u1"},
	))
	page, total, err := stores.History.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 1, Size: 1})
	if err != nil || total != 3 || len(page) != 1 || page[0].ID != "his-2" {
		t.Fatalf("已办分页不符: total=%d len=%d err=%v", total, len(page), err)
	}
	// 不分页（Size<=0）返回全量
	mock.ExpectQuery(raw).WithArgs("u1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1"}, map[string]any{"id": "his-2"}))
	page, total, err = stores.History.ListByOwner(ctx, "u1", "sys", PageQuery{Start: 0, Size: 0})
	if err != nil || total != 2 || len(page) != 2 {
		t.Fatalf("不分页应返回全量: total=%d len=%d err=%v", total, len(page), err)
	}
	// 查询失败
	mock.ExpectQuery(raw).WithArgs("u-bad", "sys").WillReturnError(errors.New("db down"))
	_, _, err = stores.History.ListByOwner(ctx, "u-bad", "sys", PageQuery{})
	ssnExpectErrorCode(t, err, "SQL-ADAPTER-002")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLHistoryUpdateFinish 验证办结回写：命中处理人更新、其他处理人删除与错误分支。
// @param t 测试上下文
func TestStoreSQLHistoryUpdateFinish(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_task_history_extend"
	getSQL := sscSelectSQL(sscHistoryCols, table, "task_id = ?", "") + " LIMIT 1"
	listSQL := sscSelectSQL(sscHistoryCols, table, "process_instance_id = ?", "task_finish_time")
	cols := []string{"id", "task_id", "process_instance_id", "owner_id"}
	updateSQL := regexp.QuoteMeta("UPDATE " + table + " SET task_finish_time = ?, update_time = ? WHERE id = ?")
	deleteSQL := regexp.QuoteMeta("DELETE FROM " + table + " WHERE id = ?")
	// 锚点 + 同实例三条：其他任务跳过、命中处理人更新、其他处理人删除
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1", "owner_id": "u1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-other", "task_id": "task-2", "process_instance_id": "pi-1", "owner_id": "u1"},
		map[string]any{"id": "his-mine", "task_id": "task-1", "process_instance_id": "pi-1", "owner_id": "u1"},
		map[string]any{"id": "his-peer", "task_id": "task-1", "process_instance_id": "pi-1", "owner_id": "u2"},
	))
	mock.ExpectExec(updateSQL).WithArgs(ssnStamp, ssnStamp, "his-mine").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deleteSQL).WithArgs("his-peer").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.History.UpdateFinishTime(ctx, "task-1", "u1", ssnStamp); err != nil {
		t.Fatalf("办结回写失败: %v", err)
	}
	// 锚点查无：直接成功
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-none").WillReturnRows(sqlmock.NewRows(cols))
	if err := stores.History.UpdateFinishTime(ctx, "task-none", "u1", ssnStamp); err != nil {
		t.Fatalf("锚点查无应直接成功: %v", err)
	}
	// 锚点查询失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.UpdateFinishTime(ctx, "task-bad", "u1", ssnStamp), "WF-0010")
	// 实例列表查询失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.UpdateFinishTime(ctx, "task-1", "u1", ssnStamp), "WF-0010")
	// 更新写失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-mine", "task_id": "task-1", "owner_id": "u1"}))
	mock.ExpectExec(updateSQL).WithArgs(ssnStamp, ssnStamp, "his-mine").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.UpdateFinishTime(ctx, "task-1", "u1", ssnStamp), "WF-0016")
	// 删除其他处理人失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-peer", "task_id": "task-1", "owner_id": "u2"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-peer").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.UpdateFinishTime(ctx, "task-1", "u1", ssnStamp), "WF-0018")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLHistoryDelete 验证已办各删除路径：锚点过滤、按实例/业务逐条、步骤集合与空集合。
// @param t 测试上下文
func TestStoreSQLHistoryDelete(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_task_history_extend"
	getSQL := sscSelectSQL(sscHistoryCols, table, "task_id = ?", "") + " LIMIT 1"
	listSQL := sscSelectSQL(sscHistoryCols, table, "process_instance_id = ?", "task_finish_time")
	cols := []string{"id", "task_id", "process_instance_id", "owner_id", "task_step_code"}
	deleteSQL := regexp.QuoteMeta("DELETE FROM " + table + " WHERE id = ?")
	// 按任务：锚点定位实例后仅删同任务（deleteHistoryInInstance）
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1"},
		map[string]any{"id": "his-2", "task_id": "task-2"},
	))
	mock.ExpectExec(deleteSQL).WithArgs("his-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.History.DeleteByTaskID(ctx, "task-1"); err != nil {
		t.Fatalf("按任务删除已办失败: %v", err)
	}
	// 锚点查无 / 查询失败 / 列表失败 / 删除失败
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-none").WillReturnRows(sqlmock.NewRows(cols))
	if err := stores.History.DeleteByTaskID(ctx, "task-none"); err != nil {
		t.Fatalf("锚点查无应直接成功: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByTaskID(ctx, "task-bad"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByTaskID(ctx, "task-1"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("task-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1", "process_instance_id": "pi-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1", "task_id": "task-1"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByTaskID(ctx, "task-1"), "WF-0018")
	// 按实例：逐条删除
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1"}, map[string]any{"id": "his-2"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(deleteSQL).WithArgs("his-2").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.History.DeleteByInstance(ctx, "pi-1"); err != nil {
		t.Fatalf("按实例删除已办失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByInstance(ctx, "pi-bad"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByInstance(ctx, "pi-1"), "WF-0018")
	// 按业务：逐条删除
	businessSQL := sscSelectSQL(sscHistoryCols, table, "business_id = ? AND tenant_id = ?", "task_finish_time")
	mock.ExpectQuery(regexp.QuoteMeta(businessSQL)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-1").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.History.DeleteByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatalf("按业务删除已办失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(businessSQL)).WithArgs("biz-bad", "sys").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByBusinessID(ctx, "biz-bad", "sys"), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(businessSQL)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-1"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-1").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByBusinessID(ctx, "biz-1", "sys"), "WF-0018")
	// 按步骤集合：命中 task_step_code 才删；空集合仍查列表但不删除
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-a", "task_step_code": "act-a"},
		map[string]any{"id": "his-b", "task_step_code": "act-b"},
	))
	mock.ExpectExec(deleteSQL).WithArgs("his-a").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.History.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"act-a"}); err != nil {
		t.Fatalf("按步骤集合删除已办失败: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-a", "task_step_code": "act-a"}))
	if err := stores.History.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", nil); err != nil {
		t.Fatalf("空步骤集合应查列表但不删除: %v", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-bad", []string{"act-a"}), "WF-0010")
	mock.ExpectQuery(regexp.QuoteMeta(listSQL)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "his-a", "task_step_code": "act-a"}))
	mock.ExpectExec(deleteSQL).WithArgs("his-a").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.History.DeleteByProcessInstanceAndActivityIDs(ctx, "pi-1", []string{"act-a"}), "WF-0018")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// sscOperationEntity 构造字段齐全的操作记录实体（用于落库列与行映射断言）。
// @param id 主键
// @param businessID 业务单据 Id
// @param taskID 任务 Id
// @param instanceID 流程实例 Id
// @param ownerID 处理人 Id
// @param stepCode 步骤代号
// @param tenantID 租户
// @return 操作记录实体
func sscOperationEntity(id, businessID, taskID, instanceID, ownerID, stepCode, tenantID string) *OperationRecord {
	return &OperationRecord{
		BaseColumns: BaseColumns{
			ID: id, Creator: "op", CreatorName: "张三", CreateTime: ssnStamp,
			Updater: "editor", UpdateTime: ssnStamp, TenantID: tenantID,
		},
		BusinessID: businessID, TaskID: taskID, ProcessInstanceID: instanceID, Message: "同意本单",
		IsAgree: intPtr(1), TaskStepCode: stepCode, TaskStepName: "审批",
		FlowCategoryCode: "leave", FlowCategoryName: "请假",
		OwnerID: ownerID, OwnerName: "处理人", OwnerGroup: "部门", OwnerGroupID: "g1",
		OwnerGroupPaths: "公司/部门", OwnerGroupPathIDs: "g0/g1",
		DealTime: &ssnStamp, TaskReceiptTime: &ssnStamp, TaskStartTime: &ssnStamp, TaskFinishTime: &ssnStamp,
		Type: CommentTypeSP,
	}
}

// sscOperationQueryRow 构造操作记录查询结果集（最小必要列，覆盖行映射关键字段）。
// @param id 主键
// @param taskID 任务 Id
// @param stepCode 步骤代号
// @param opType 意见类型
// @return 结果集
func sscOperationQueryRow(id, taskID, stepCode, opType string) *sqlmock.Rows {
	cols := []string{"id", "task_id", "process_instance_id", "business_id", "owner_id", "task_step_code", "is_agree", "deal_time", "type", "create_time"}
	return sqlmock.NewRows(cols).AddRow(id, taskID, "pi-1", "biz-1", "u1", stepCode, 1, ssnStamp, opType, ssnStamp)
}

// sscFullDocument 构造字段齐全的单据状态实体。
// @param id 主键
// @param businessID 业务单据 Id
// @param tenantID 租户
// @return 单据状态实体
func sscFullDocument(id, businessID, tenantID string) *Document {
	return &Document{
		BaseColumns: BaseColumns{
			ID: id, Creator: "op", CreatorName: "张三", CreateTime: ssnStamp,
			Updater: "editor", UpdateTime: ssnStamp, TenantID: tenantID,
		},
		BusinessID: businessID, CreateGroupPathIDs: "g0/g1", CreateGroupPaths: "公司/部门",
		CreateGroupID: "g1", CreateGroup: "部门", UpdaterName: "更新人", Deleted: 0,
		DocumentURL: "/doc", DocumentID: businessID, DocumentTable: "t_leave",
		FlowTitle: "请假单", FlowTaskID: "task-1", FlowInstanceID: "pi-1",
		FlowCategoryID: "leave", FlowCategory: "请假", FlowActionID: "act-a", FlowAction: "审批",
		FlowStatus: FlowStatusRunning, FlowCurrentStepName: "审批", FlowCurrentPersons: "张三",
		FlowStartTime: &ssnStamp, FlowFinishTime: &ssnStamp, FlowTerminationTime: &ssnStamp,
		ExtendFields: ExtendFields{ExtendS1: "x", ExtendI1: int64Ptr(2), ExtendF1: floatPtr(2.5), ExtendD1: &ssnStamp},
	}
}

// TestStoreSQLOperationSaveAndLists 验证操作记录写入的列/参数、执行错误与三种单条件列表查询。
// @param t 测试上下文
func TestStoreSQLOperationSaveAndLists(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_operation_record"
	record := sscOperationEntity("op-1", "biz-1", "task-1", "pi-1", "u1", "act-a", "sys")
	row := operationToRow(record)
	// 落库行关键列断言（含可空 IsAgree/时间与意见类型）
	if row["task_id"] != "task-1" || row["is_agree"] != 1 || row["type"] != CommentTypeSP || row["deal_time"] != ssnStamp {
		t.Fatalf("操作记录行转换不符: %+v", row)
	}
	insertSQL, insertArgs := sscInsertParts(table, sscOperationCols, row)
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := stores.Operation.Save(ctx, record); err != nil {
		t.Fatalf("操作记录写入失败: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Operation.Save(ctx, record), "WF-0009")
	// 按任务
	byTask := sscSelectSQL(sscOperationCols, table, "task_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byTask)).WithArgs("task-1").WillReturnRows(mmRowsOf(
		[]string{"id", "task_id", "is_agree", "deal_time", "type"},
		map[string]any{"id": "op-1", "task_id": "task-1", "is_agree": 1, "deal_time": ssnStamp, "type": CommentTypeSP},
	))
	rows, err := stores.Operation.ListByTask(ctx, "task-1")
	if err != nil || len(rows) != 1 || rows[0].IsAgree == nil || *rows[0].IsAgree != 1 ||
		rows[0].DealTime == nil || rows[0].Type != CommentTypeSP {
		t.Fatalf("按任务查询操作记录失败: %+v（%v）", rows, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byTask)).WithArgs("task-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.ListByTask(ctx, "task-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按实例
	byInstance := sscSelectSQL(sscOperationCols, table, "process_instance_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(
		[]string{"id", "process_instance_id"},
		map[string]any{"id": "op-1", "process_instance_id": "pi-1"},
		map[string]any{"id": "op-2", "process_instance_id": "pi-1"},
	))
	if rows, err := stores.Operation.ListByInstance(ctx, "pi-1"); err != nil || len(rows) != 2 {
		t.Fatalf("按实例查询操作记录失败: %d（%v）", len(rows), err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.ListByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按业务单据
	byBusiness := sscSelectSQL(sscOperationCols, table, "business_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-1").WillReturnRows(mmRowsOf(
		[]string{"id", "business_id"},
		map[string]any{"id": "op-1", "business_id": "biz-1"},
	))
	if rows, err := stores.Operation.ListByBusinessID(ctx, "biz-1"); err != nil || len(rows) != 1 {
		t.Fatalf("按业务查询操作记录失败: %d（%v）", len(rows), err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.ListByBusinessID(ctx, "biz-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLOperationMerged 验证多单据归并排序、可签名过滤、计数与最后类型查询。
// @param t 测试上下文
func TestStoreSQLOperationMerged(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_operation_record"
	byBusiness := sscSelectSQL(sscOperationCols, table, "business_id = ?", "create_time")
	cols := []string{"id", "business_id", "task_step_code", "type", "deal_time", "create_time"}
	// 两个业务单据：第二个单据的处理时间更晚；op-b 无处理时间，退化用创建时间参与排序
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "op-a", "business_id": "biz-1", "deal_time": ssnStamp.Add(time.Hour), "create_time": ssnStamp},
		map[string]any{"id": "op-b", "business_id": "biz-1", "create_time": ssnStamp},
	))
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-2").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "op-c", "business_id": "biz-2", "deal_time": ssnStamp.Add(2 * time.Hour), "create_time": ssnStamp},
	))
	merged, err := stores.Operation.ListByBusinessIDs(ctx, []string{"biz-1", "biz-2"})
	if err != nil || len(merged) != 3 {
		t.Fatalf("多单据归并失败: %d（%v）", len(merged), err)
	}
	if merged[0].ID != "op-b" || merged[1].ID != "op-a" || merged[2].ID != "op-c" {
		t.Fatalf("归并排序不符: %s,%s,%s", merged[0].ID, merged[1].ID, merged[2].ID)
	}
	// 空集合：不产生 SQL，返回空列表
	if merged, err := stores.Operation.ListByBusinessIDs(ctx, nil); err != nil || len(merged) != 0 {
		t.Fatalf("空集合应返回空列表: %d（%v）", len(merged), err)
	}
	// 第二个单据查询失败 → 整体失败
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "op-a", "business_id": "biz-1"}))
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.ListByBusinessIDs(ctx, []string{"biz-1", "biz-bad"})
	ssnExpectErrorCode(t, err, "WF-0010")
	// 可签名过滤：仅 TJ/SP/TZ 且步骤代号非空
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "op-tj", "task_step_code": "act-a", "type": CommentTypeTJ},
		map[string]any{"id": "op-sp-empty", "task_step_code": "", "type": CommentTypeSP},
		map[string]any{"id": "op-th", "task_step_code": "act-b", "type": CommentTypeTH},
		map[string]any{"id": "op-tz", "task_step_code": "act-c", "type": CommentTypeTZ},
	))
	signable, err := stores.Operation.ListSignable(ctx, []string{"biz-1"})
	if err != nil || len(signable) != 2 || signable[0].ID != "op-tj" || signable[1].ID != "op-tz" {
		t.Fatalf("可签名过滤不符: %+v（%v）", signable, err)
	}
	// 可签名查询错误透传
	mock.ExpectQuery(regexp.QuoteMeta(byBusiness)).WithArgs("biz-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.ListSignable(ctx, []string{"biz-bad"})
	ssnExpectErrorCode(t, err, "WF-0010")
	// 计数
	byInstance := sscSelectSQL(sscOperationCols, table, "process_instance_id = ?", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "op-1"}, map[string]any{"id": "op-2"}))
	if count, err := stores.Operation.CountByInstance(ctx, "pi-1"); err != nil || count != 2 {
		t.Fatalf("实例计数不符: %d（%v）", count, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.CountByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 最后一条类型：取列表末行；空列表返回空串
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "op-1", "type": CommentTypeTJ},
		map[string]any{"id": "op-2", "type": CommentTypeSP},
	))
	if last, err := stores.Operation.LastTypeByInstance(ctx, "pi-1"); err != nil || last != CommentTypeSP {
		t.Fatalf("最后类型不符: %q（%v）", last, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-empty").WillReturnRows(sqlmock.NewRows(cols))
	if last, err := stores.Operation.LastTypeByInstance(ctx, "pi-empty"); err != nil || last != "" {
		t.Fatalf("空列表最后类型应为空串: %q（%v）", last, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, err = stores.Operation.LastTypeByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLDocumentSave 验证单据 upsert 的插入/更新分支与查询、写入错误。
// @param t 测试上下文
func TestStoreSQLDocumentSave(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_document"
	doc := sscFullDocument("doc-1", "biz-1", "sys")
	row := documentToRow(doc)
	// 落库行关键列断言
	if row["business_id"] != "biz-1" || row["flow_status"] != FlowStatusRunning || row["extend_s1"] != "x" {
		t.Fatalf("单据行转换不符: %+v", row)
	}
	getSQL := sscSelectSQL(sscDocumentCols, table, "business_id = ? AND tenant_id = ? AND deleted = 0", "") + " LIMIT 1"
	// 不存在 → 插入
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("biz-1", "sys").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	insertSQL, insertArgs := sscInsertParts(table, sscDocumentCols, row)
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := stores.Document.Save(ctx, doc); err != nil {
		t.Fatalf("单据插入失败: %v", err)
	}
	// 插入失败 → WF-0009
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("biz-1", "sys").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta(insertSQL)).WithArgs(insertArgs...).WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Document.Save(ctx, doc), "WF-0009")
	// 已存在 → 按既有主键更新（排除 id 与 create_time）
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("biz-1", "sys").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("doc-exist"))
	changes := make(pwf.Row, len(row))
	for key, value := range row {
		if key == "id" || key == "create_time" {
			continue
		}
		changes[key] = value
	}
	updateSQL, updateArgs := sscUpdateParts(table, sscDocumentCols, changes, "doc-exist")
	mock.ExpectExec(regexp.QuoteMeta(updateSQL)).WithArgs(updateArgs...).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Document.Save(ctx, doc); err != nil {
		t.Fatalf("单据更新失败: %v", err)
	}
	// 更新失败 → WF-0016
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("biz-1", "sys").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("doc-exist"))
	mock.ExpectExec(regexp.QuoteMeta(updateSQL)).WithArgs(updateArgs...).WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Document.Save(ctx, doc), "WF-0016")
	// 查询失败 → WF-0010
	mock.ExpectQuery(regexp.QuoteMeta(getSQL)).WithArgs("biz-bad", "sys").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Document.Save(ctx, sscFullDocument("doc-bad", "biz-bad", "sys")), "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLDocumentGet 验证按业务单据/流程实例读取单据及空结果/错误分支。
// @param t 测试上下文
func TestStoreSQLDocumentGet(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_document"
	getByBusiness := sscSelectSQL(sscDocumentCols, table, "business_id = ? AND tenant_id = ? AND deleted = 0", "") + " LIMIT 1"
	// 命中并校验行映射（含扩展列与可空时间）
	mock.ExpectQuery(regexp.QuoteMeta(getByBusiness)).WithArgs("biz-1", "sys").WillReturnRows(mmRowsOf(
		[]string{"id", "business_id", "flow_status", "flow_instance_id", "deleted", "flow_start_time", "extend_s1", "tenant_id"},
		map[string]any{"id": "doc-1", "business_id": "biz-1", "flow_status": FlowStatusRunning,
			"flow_instance_id": "pi-1", "deleted": 0, "flow_start_time": ssnStamp, "extend_s1": "x", "tenant_id": "sys"},
	))
	doc, found, err := stores.Document.GetByBusinessID(ctx, "biz-1", "sys")
	if err != nil || !found || doc.ID != "doc-1" || doc.FlowStatus != FlowStatusRunning ||
		doc.FlowInstanceID != "pi-1" || doc.FlowStartTime == nil || doc.ExtendS1 != "x" {
		t.Fatalf("按业务查询单据不符: %+v（%v）", doc, err)
	}
	// 查无
	mock.ExpectQuery(regexp.QuoteMeta(getByBusiness)).WithArgs("biz-none", "sys").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if doc, found, err := stores.Document.GetByBusinessID(ctx, "biz-none", "sys"); err != nil || found || doc != nil {
		t.Fatalf("查无应返回未命中: %v %v %v", doc, found, err)
	}
	// 查询失败
	mock.ExpectQuery(regexp.QuoteMeta(getByBusiness)).WithArgs("biz-bad", "sys").WillReturnError(errors.New("db down"))
	_, _, err = stores.Document.GetByBusinessID(ctx, "biz-bad", "sys")
	ssnExpectErrorCode(t, err, "WF-0010")
	// 按实例：多行取首行
	byInstance := sscSelectSQL(sscDocumentCols, table, "flow_instance_id = ? AND deleted = 0", "create_time")
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(
		[]string{"id", "flow_instance_id"},
		map[string]any{"id": "doc-1", "flow_instance_id": "pi-1"},
		map[string]any{"id": "doc-2", "flow_instance_id": "pi-1"},
	))
	if doc, found, err := stores.Document.GetByInstance(ctx, "pi-1"); err != nil || !found || doc.ID != "doc-1" {
		t.Fatalf("按实例查询单据不符: %+v（%v）", doc, err)
	}
	// 无行
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-empty").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if doc, found, err := stores.Document.GetByInstance(ctx, "pi-empty"); err != nil || found || doc != nil {
		t.Fatalf("无行应返回未命中: %v %v %v", doc, found, err)
	}
	// 查询失败
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	_, _, err = stores.Document.GetByInstance(ctx, "pi-bad")
	ssnExpectErrorCode(t, err, "WF-0010")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLDocumentUpdate 验证按实例逐条更新、空变更列错误与逻辑删除。
// @param t 测试上下文
func TestStoreSQLDocumentUpdate(t *testing.T) {
	ctx, mock, stores := ssnNewStores(t)
	table := "workflow_document"
	byInstance := sscSelectSQL(sscDocumentCols, table, "flow_instance_id = ? AND deleted = 0", "create_time")
	cols := []string{"id", "flow_instance_id"}
	changes := pwf.Row{"flow_status": FlowStatusFinished, "flow_current_step_name": "结束"}
	updateSQL, updateArgs := sscUpdateParts(table, sscDocumentCols, changes, "doc-1")
	// 两条同实例单据 → 各更新一次（UPDATE 语句相同，仅主键参数不同）
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "doc-1", "flow_instance_id": "pi-1"},
		map[string]any{"id": "doc-2", "flow_instance_id": "pi-1"},
	))
	mock.ExpectExec(regexp.QuoteMeta(updateSQL)).WithArgs(updateArgs...).WillReturnResult(sqlmock.NewResult(0, 1))
	secondArgs := append([]driver.Value{}, updateArgs[:len(updateArgs)-1]...)
	secondArgs = append(secondArgs, "doc-2")
	mock.ExpectExec(regexp.QuoteMeta(updateSQL)).WithArgs(secondArgs...).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Document.UpdateByInstance(ctx, "pi-1", map[string]any{"flow_status": FlowStatusFinished, "flow_current_step_name": "结束"}); err != nil {
		t.Fatalf("按实例更新单据失败: %v", err)
	}
	// 空变更列：仓储报 WF-0006，不产生 UPDATE
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "doc-1", "flow_instance_id": "pi-1"}))
	ssnExpectErrorCode(t, stores.Document.UpdateByInstance(ctx, "pi-1", nil), "WF-0006")
	// 列表查询失败
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-bad").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Document.UpdateByInstance(ctx, "pi-bad", map[string]any{"flow_status": FlowStatusFinished}), "WF-0010")
	// 更新失败
	mock.ExpectQuery(regexp.QuoteMeta(byInstance)).WithArgs("pi-1").WillReturnRows(mmRowsOf(cols,
		map[string]any{"id": "doc-1", "flow_instance_id": "pi-1"}))
	mock.ExpectExec(regexp.QuoteMeta(updateSQL)).WithArgs(updateArgs...).WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Document.UpdateByInstance(ctx, "pi-1", map[string]any{"flow_status": FlowStatusFinished, "flow_current_step_name": "结束"}), "WF-0016")
	// 逻辑删除：deleted=1
	markSQL := regexp.QuoteMeta("UPDATE " + table + " SET deleted = ?, update_time = ? WHERE business_id = ? AND tenant_id = ? AND deleted = 0")
	mock.ExpectExec(markSQL).WithArgs(1, ssnStamp, "biz-1", "sys").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := stores.Document.MarkDeletedByBusinessID(ctx, "biz-1", "sys"); err != nil {
		t.Fatalf("逻辑删除单据失败: %v", err)
	}
	mock.ExpectExec(markSQL).WithArgs(1, ssnStamp, "biz-bad", "sys").WillReturnError(errors.New("db down"))
	ssnExpectErrorCode(t, stores.Document.MarkDeletedByBusinessID(ctx, "biz-bad", "sys"), "WF-0016")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL 期望未全部满足: %v", err)
	}
}

// TestStoreSQLAdapterRowConverters 验证四域行转换的可空字段（nil 时间/nil 整型/[]byte）与非法值分支。
// @param t 测试上下文
func TestStoreSQLAdapterRowConverters(t *testing.T) {
	// 待办：可空时间缺失 → nil；[]byte 串列归一化；非法时间类型 → nil
	message := messageFromRow(pwf.Row{
		"id": []byte("msg-1"), "task_id": []byte("task-1"), "owner_id": []byte("u1"),
		"read_time": int64(1), "delay_time": []byte("2026-03-01 09:00:00"), "is_read": []byte("1"),
	})
	if message.ID != "msg-1" || message.TaskID != "task-1" || message.OwnerID != "u1" || message.IsRead != 1 {
		t.Fatalf("待办 []byte 映射不符: %+v", message)
	}
	if message.ReadTime != nil || message.DelayTime != nil {
		t.Fatalf("非法时间类型应映射为 nil: read=%v delay=%v", message.ReadTime, message.DelayTime)
	}
	// 已办：task_finish_time 缺失 → nil
	history := historyFromRow(pwf.Row{"id": "his-1", "task_id": "task-1"})
	if history.TaskFinishTime != nil || history.TaskID != "task-1" {
		t.Fatalf("已办可空办结时间不符: %+v", history)
	}
	// 操作记录：is_agree 缺失/[]byte、非法时间串 → nil
	record := operationFromRow(pwf.Row{
		"id": "op-1", "is_agree": []byte("0"), "deal_time": "not-a-time", "task_step_code": []byte("act-a"),
	})
	if record.IsAgree == nil || *record.IsAgree != 0 || record.TaskStepCode != "act-a" || record.DealTime != nil {
		t.Fatalf("操作记录可空字段映射不符: %+v", record)
	}
	if operationFromRow(pwf.Row{"id": "op-2"}).IsAgree != nil {
		t.Fatal("缺失 is_agree 应映射为 nil")
	}
	// 单据：deleted []byte、flow_status []byte、可空时间缺失 → nil
	document := documentFromRow(pwf.Row{
		"id": []byte("doc-1"), "deleted": []byte("0"), "flow_status": []byte(FlowStatusRunning),
		"flow_start_time": "2026-03-01 09:00:00",
	})
	if document.ID != "doc-1" || document.Deleted != 0 || document.FlowStatus != FlowStatusRunning ||
		document.FlowStartTime == nil || !document.FlowStartTime.Equal(time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("单据可空字段映射不符: %+v", document)
	}
	if document.FlowFinishTime != nil || document.FlowTerminationTime != nil {
		t.Fatal("缺失单据时间应为 nil")
	}
	// 写入方向：可空列全空时不落键（保持 NULL 语义）
	messageRow := messageToRow(&MessageExtend{BaseColumns: BaseColumns{ID: "msg-min"}})
	for _, key := range []string{"read_time", "plan_finish_time", "task_receipt_time", "task_start_time", "delay_time", "extend_s1", "extend_i1"} {
		if _, ok := messageRow[key]; ok {
			t.Fatalf("待办空值列 %s 不应写入行", key)
		}
	}
	historyRow := historyToRow(&HistoryExtend{MessageExtend: MessageExtend{BaseColumns: BaseColumns{ID: "his-min"}}})
	if _, ok := historyRow["task_finish_time"]; ok {
		t.Fatal("已办空办结时间不应写入行")
	}
	operationRow := operationToRow(&OperationRecord{BaseColumns: BaseColumns{ID: "op-min"}})
	for _, key := range []string{"is_agree", "deal_time", "task_receipt_time", "task_start_time", "task_finish_time"} {
		if _, ok := operationRow[key]; ok {
			t.Fatalf("操作记录空值列 %s 不应写入行", key)
		}
	}
	documentRow := documentToRow(&Document{BaseColumns: BaseColumns{ID: "doc-min"}})
	for _, key := range []string{"flow_start_time", "flow_finish_time", "flow_termination_time", "extend_s1"} {
		if _, ok := documentRow[key]; ok {
			t.Fatalf("单据空值列 %s 不应写入行", key)
		}
	}
}
