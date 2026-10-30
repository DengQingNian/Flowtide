package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	pgstore "github.com/DengQingNian/Flowtide/internal/persistence/postgres"
)

// postgresServiceDatabase 创建服务模式验收专用库，测试结束只删除该临时库。
// @param t 测试上下文
// @return 操作上下文
// @return 已初始化的临时连接池
func postgresServiceDatabase(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	if os.Getenv("FLOWABLE_POSTGRES_INTEGRATION") != "1" {
		t.Skip("设置 FLOWABLE_POSTGRES_INTEGRATION=1 启用 PostgreSQL 服务验收")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	endpoint, err := url.Parse(os.Getenv("FLOWABLE_POSTGRES_DSN"))
	if err != nil || endpoint.Host == "" {
		t.Fatal("需要 PostgreSQL 验收 URI")
	}
	admin, err := pgstore.Open(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("flowable_go_service_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	endpoint.Path = "/" + name
	db, err := pgstore.Open(ctx, endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := pgstore.EnsureSchema(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	if err := pgstore.EnsureSchema(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	return ctx, db
}

// TestIntegrationPostgresRowsServiceMode 验证会话独占、重启续跑、历史明细和真实断线回滚。
// @param t 测试上下文
// @return 无
func TestIntegrationPostgresRowsServiceMode(t *testing.T) {
	ctx, db := postgresServiceDatabase(t)
	lock, err := pgstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := pgstore.NewLockedRows(ctx, db); err == nil {
		_ = duplicate.Release()
		t.Fatal("第二个服务获得了独占权")
	}
	repository, err := flowablerow.NewRepository(lock, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.New(engine.Config{RowRepository: repository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}
	xml := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="pg_service" isExecutable="true"><startEvent id="s"/><sequenceFlow id="a" sourceRef="s" targetRef="task"/><userTask id="task"/><sequenceFlow id="b" sourceRef="task" targetRef="end"/><endEvent id="end"/></process></definitions>`
	if _, err := first.Deploy(ctx, api.DeploymentRequest{Resources: map[string][]byte{"service.bpmn20.xml": []byte(xml)}}); err != nil {
		t.Fatal(err)
	}
	instance, err := first.StartProcessInstance(ctx, api.StartRequest{ProcessDefinitionKey: "pg_service", UserID: "starter", Variables: []api.Variable{{Name: "金额", Value: int64(9007199254740993)}, {Name: "二进制", Value: []byte{1, 2, 3}}}})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := first.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(initial.Data) != 1 {
		t.Fatalf("PG 任务创建: %v", err)
	}
	due := time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("验收时区", 8*3600))
	instanceName, businessStatus, instanceUser := "实例元数据验收", "review", "bob"
	updatedInstance, err := first.UpdateProcessInstance(ctx, instance.ID, api.ProcessInstanceUpdate{Name: &instanceName, BusinessStatus: &businessStatus, DueDate: &due, ClaimUserID: &instanceUser})
	if err != nil || updatedInstance.ClaimTime == nil {
		t.Fatalf("实例签收元数据更新失败: %+v %v", updatedInstance, err)
	}

	if _, err := first.UpdateTask(ctx, initial.Data[0].ID, api.TaskUpdate{DueDate: &due}); err != nil {
		t.Fatal(err)
	}
	if err := first.Claim(ctx, initial.Data[0].ID, "alice"); err != nil {
		t.Fatal(err)
	}
	claimed, err := first.GetTask(ctx, initial.Data[0].ID)
	if err != nil || claimed.ClaimTime == nil {
		t.Fatalf("PG 签收时间缺失: %v", err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	// 新执行者重新取锁并从真实行表恢复，不能复用首个引擎的内存。
	restartedLock, err := pgstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	restartedRepository, _ := flowablerow.NewRepository(restartedLock, "postgres")
	second, err := engine.New(engine.Config{RowRepository: restartedRepository, LoadRows: true, HistoryLevel: engine.HistoryFull})
	if err != nil {
		t.Fatal(err)
	}

	restoredInstance, err := second.GetProcessInstance(ctx, instance.ID)
	if err != nil || restoredInstance.Name != instanceName || restoredInstance.StartUserID != "starter" || restoredInstance.BusinessStatus != businessStatus || restoredInstance.ClaimedBy != instanceUser || restoredInstance.DueDate == nil || !restoredInstance.DueDate.Equal(due) || restoredInstance.ClaimTime == nil || !restoredInstance.ClaimTime.Equal(*updatedInstance.ClaimTime) {
		t.Fatalf("标准实例元数据重启后丢失: %+v %v", restoredInstance, err)
	}
	var storedName, storedUser, storedStatus, storedClaimedBy string
	var storedDue, storedClaim time.Time
	if err := db.QueryRowContext(ctx, "SELECT NAME_, START_USER_ID_, BUSINESS_STATUS_, CLAIMED_BY_, DUE_DATE_, CLAIM_TIME_ FROM ACT_RU_EXECUTION WHERE ID_=$1", instance.ID).Scan(&storedName, &storedUser, &storedStatus, &storedClaimedBy, &storedDue, &storedClaim); err != nil || storedName != instanceName || storedUser != "starter" || storedStatus != businessStatus || storedClaimedBy != instanceUser || !storedDue.Equal(due) || !storedClaim.Equal(*updatedInstance.ClaimTime) {
		t.Fatalf("标准实例行不是元数据真实来源: 名称=%s 用户=%s 状态=%s 签收人=%s 到期=%s 签收=%s 错误=%v", storedName, storedUser, storedStatus, storedClaimedBy, storedDue, storedClaim, err)
	}
	tasks, err := second.ListTasks(ctx, api.TaskFilter{ProcessInstanceID: instance.ID})
	if err != nil || len(tasks.Data) != 1 {
		t.Fatalf("PG 服务任务恢复: %+v %v", tasks, err)
	}
	if tasks.Data[0].ExecutionID == "" || tasks.Data[0].ExecutionID == instance.ID || tasks.Data[0].ClaimTime == nil || !tasks.Data[0].ClaimTime.Equal(claimed.ClaimTime.Truncate(time.Millisecond)) {
		t.Fatalf("PG 标准任务行恢复丢失执行或签收元数据: %+v", tasks.Data[0])
	}
	var rootActivity, rootParent, childBusiness sql.NullString
	var rootScope bool
	if err := db.QueryRowContext(ctx, "SELECT ACT_ID_, PARENT_ID_, IS_SCOPE_ FROM ACT_RU_EXECUTION WHERE ID_=$1", instance.ID).Scan(&rootActivity, &rootParent, &rootScope); err != nil || rootActivity.Valid || rootParent.Valid || !rootScope {
		t.Fatalf("PG 实例根标准行不符合空活动作用域: 活动=%+v 父级=%+v 作用域=%v 错误=%v", rootActivity, rootParent, rootScope, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT BUSINESS_KEY_ FROM ACT_RU_EXECUTION WHERE ID_=$1", tasks.Data[0].ExecutionID).Scan(&childBusiness); err != nil || childBusiness.Valid {
		t.Fatalf("PG 子执行重复存储根业务键: %+v %v", childBusiness, err)
	}
	if tasks.Data[0].CreateTime.Sub(claimed.CreateTime).Abs() > time.Millisecond {
		t.Fatalf("PG 任务创建时间重启后漂移: 原值=%s 恢复=%s", claimed.CreateTime, tasks.Data[0].CreateTime)
	}
	if tasks.Data[0].DueDate == nil || !tasks.Data[0].DueDate.Equal(due) {
		t.Fatalf("PG 到期时间重启后漂移: %+v", tasks.Data[0].DueDate)
	}
	value, err := second.GetVariable(ctx, instance.ID, "金额")
	if err != nil || value != int64(9007199254740993) {
		t.Fatalf("PG 大整数恢复: %v %v", value, err)
	}
	if err := second.Complete(ctx, tasks.Data[0].ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatal(err)
	}
	if variable, err := second.GetHistoricVariableByName(ctx, instance.ID, "金额"); err != nil || variable.Value != int64(9007199254740993) {
		t.Fatalf("PG 完成后历史查询: %+v %v", variable, err)
	}
	if details, err := second.ListHistoricDetails(ctx, instance.ID); err != nil || len(details) != 2 {
		t.Fatalf("PG full 明细恢复: %+v %v", details, err)
	}
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
	// 在持锁物理连接上开始事务，外部终止会话后不得透明重连并提交。
	rows, err := restartedLock.QueryContext(ctx, "SELECT pg_backend_pid()")
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	if !rows.Next() {
		t.Fatal("缺少会话标识")
	}
	if err := rows.Scan(&pid); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	tx, err := restartedLock.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO ACT_GE_PROPERTY (NAME_,VALUE_,REV_) VALUES ('disconnect-probe','未提交',1)"); err != nil {
		t.Fatal(err)
	}
	var killed bool
	if err := db.QueryRowContext(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&killed); err != nil || !killed {
		t.Fatalf("终止测试会话失败: %v", err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("断线事务错误提交")
	}
	if _, err := restartedLock.ExecContext(ctx, "DELETE FROM ACT_GE_PROPERTY"); err == nil {
		t.Fatal("失效连接恢复了写入")
	}
	_ = restartedLock.Release()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ACT_GE_PROPERTY WHERE NAME_='disconnect-probe'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("断线写入未回滚: %d %v", count, err)
	}
	finalLock, err := pgstore.NewLockedRows(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := finalLock.Release(); err != nil {
		t.Fatal(err)
	}
	t.Log("PostgreSQL Schema 原子初始化、独占竞争、重启续跑、full 历史及断线回滚通过")
}
