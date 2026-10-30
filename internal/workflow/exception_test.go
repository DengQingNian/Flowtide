// exception_test.go 验证异常记录域服务：四类判定、消息截断、处理流转、计数原子性与 SQL 适配。
package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DengQingNian/Flowtide/api"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// TestClassifyException 验证四类异常判定（对位 Java instanceof 链）。
// @param t 测试上下文
func TestClassifyException(t *testing.T) {
	cases := []struct {
		name string // 用例名
		err  error  // 原始错误
		want string // 期望类型
	}{
		{"空错误兜底运行时", nil, ExceptionTypeRuntime},
		{"业务参数错误", api.NewError("WORKFLOW-0400", "参数为空", api.ErrBadRequest), ExceptionTypeBusiness},
		{"业务错误含参数原因", &api.Error{Code: "X-1", Message: "非法", Cause: api.ErrBadRequest}, ExceptionTypeBusiness},
		{"引擎错误码", api.NewError("ENGINE-0500", "引擎内部错误", nil), ExceptionTypeFlowable},
		{"错误链含 flowable", errors.New("flowable exception happened"), ExceptionTypeFlowable},
		{"普通错误兜底运行时", errors.New("null pointer"), ExceptionTypeRuntime},
		{"非引擎错误码按业务", api.NewError("WORKFLOW-2004", "任务不存在", nil), ExceptionTypeBusiness},
	}
	for _, tc := range cases {
		if got := ClassifyException(tc.err); got != tc.want {
			t.Errorf("%s: 期望 %s 实得 %s", tc.name, tc.want, got)
		}
	}
}

// TestTruncateExceptionMessage 验证异常消息截断（Java :131 语义）。
// @param t 测试上下文
func TestTruncateExceptionMessage(t *testing.T) {
	// 短消息原样保留
	if got := truncateExceptionMessage("短消息", nil); got != "短消息" {
		t.Errorf("短消息应原样: %q", got)
	}
	// 超长截断 997 + "..."
	long := make([]byte, 1200)
	for i := range long {
		long[i] = 'a'
	}
	got := truncateExceptionMessage(string(long), nil)
	if len(got) != exceptionMessageLimit || got[997:] != "..." {
		t.Errorf("超长应截断为 %d 字符且以省略号结尾: %d", exceptionMessageLimit, len(got))
	}
	// 空消息回退错误文本
	if got := truncateExceptionMessage("", errors.New("boom")); got != "boom" {
		t.Errorf("空消息应回退错误文本: %q", got)
	}
	// 全空回退运行时常量
	if got := truncateExceptionMessage("", nil); got != ExceptionTypeRuntime {
		t.Errorf("全空应回退占位: %q", got)
	}
}

// newExceptionFixture 构造异常服务测试环境。
// @param t 测试上下文
// @return 域服务
// @return 内存存储
func newExceptionFixture(t *testing.T) (*ExceptionService, *MemTaskExceptionStore) {
	t.Helper()
	store := NewMemTaskExceptionStore(nil)
	return NewExceptionService(store, nil), store
}

// TestExceptionServiceRecordAndHandle 验证记录 → 未处理列表 → 处理/忽略流转。
// @param t 测试上下文
func TestExceptionServiceRecordAndHandle(t *testing.T) {
	svc, store := newExceptionFixture(t)
	ctx := context.Background()
	// 记录两条
	first, err := svc.Record(ctx, &ExceptionRequest{
		TaskID: "task-1", ProcessInstanceID: "pi-1", ClassName: "ProcessesServiceImpl", MethodName: "complete",
		Message: "办结失败", ExceptionClass: "NPE", TenantID: "dev", Err: errors.New("boom"),
	})
	if err != nil {
		t.Fatalf("记录失败: %v", err)
	}
	// 推断类型按运行时
	if first.ExceptionType != ExceptionTypeRuntime {
		t.Errorf("推断类型错误: %s", first.ExceptionType)
	}
	if first.HandleStatus != ExceptionUnhandled {
		t.Error("新记录应为未处理")
	}
	// 显式类型优先于推断
	second, err := svc.Record(ctx, &ExceptionRequest{Message: "业务校验失败", ExceptionType: ExceptionTypeBusiness})
	if err != nil {
		t.Fatalf("记录失败: %v", err)
	}
	if second.ExceptionType != ExceptionTypeBusiness {
		t.Errorf("显式类型应优先: %s", second.ExceptionType)
	}
	// 计数与未处理列表
	if svc.Count() != 2 {
		t.Errorf("计数应为 2: %d", svc.Count())
	}
	unhandled, err := svc.ListUnhandled(ctx)
	if err != nil || len(unhandled) != 2 {
		t.Fatalf("未处理应为 2 条: %d, %v", len(unhandled), err)
	}
	// 按实例/任务查询
	if list, err := svc.ListByInstance(ctx, "pi-1"); err != nil || len(list) != 1 {
		t.Fatalf("按实例查询错误: %d, %v", len(list), err)
	}
	if list, err := svc.ListByTask(ctx, "task-1"); err != nil || len(list) != 1 {
		t.Fatalf("按任务查询错误: %d, %v", len(list), err)
	}
	// 处理第一条
	if err := svc.Handle(ctx, []string{first.ID}, true); err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	got, found, err := store.Get(ctx, first.ID)
	if err != nil || !found || got.HandleStatus != ExceptionHandled {
		t.Fatalf("处理后应为已处理: %v %v %v", found, got, err)
	}
	// 未处理列表只剩一条
	if list, _ := svc.ListUnhandled(ctx); len(list) != 1 {
		t.Errorf("处理后未处理应为 1 条: %d", len(list))
	}
	// 忽略第二条（同为已处理终态）
	if err := svc.Handle(ctx, []string{second.ID}, false); err != nil {
		t.Fatalf("忽略失败: %v", err)
	}
	if list, _ := svc.ListUnhandled(ctx); len(list) != 0 {
		t.Errorf("忽略后未处理应为 0 条: %d", len(list))
	}
	// 空列表处理直接返回
	if err := svc.Handle(ctx, nil, true); err != nil {
		t.Errorf("空列表处理应直接返回: %v", err)
	}
}

// TestExceptionServiceRecordValidation 验证空请求与存储失败路径。
// @param t 测试上下文
func TestExceptionServiceRecordValidation(t *testing.T) {
	svc, _ := newExceptionFixture(t)
	ctx := context.Background()
	// 空请求
	if _, err := svc.Record(ctx, nil); err == nil {
		t.Error("空请求应报错")
	}
	// 存储失败（消息缺失拒绝落库）
	failing := &failingExceptionStore{err: errors.New("db down")}
	broken := NewExceptionService(failing, nil)
	if _, err := broken.Record(ctx, &ExceptionRequest{Message: "x"}); err == nil {
		t.Error("存储失败应上抛")
	}
	// 失败不计数
	if broken.Count() != 0 {
		t.Errorf("失败不应计数: %d", broken.Count())
	}
}

// failingExceptionStore 是恒失败的异常存储桩（覆盖存储错误路径）。
type failingExceptionStore struct {
	err error // 注入的失败错误
}

// Save 恒返回注入错误。
// @param ctx 操作上下文
// @param e 异常记录
// @return 注入错误
func (f *failingExceptionStore) Save(ctx context.Context, e *TaskException) error { return f.err }

// Get 恒返回注入错误。
// @param ctx 操作上下文
// @param id 记录 Id
// @return 空记录
// @return false
// @return 注入错误
func (f *failingExceptionStore) Get(ctx context.Context, id string) (*TaskException, bool, error) {
	return nil, false, f.err
}

// ListUnhandled 恒返回注入错误。
// @param ctx 操作上下文
// @return nil
// @return 注入错误
func (f *failingExceptionStore) ListUnhandled(ctx context.Context) ([]*TaskException, error) {
	return nil, f.err
}

// ListByInstance 恒返回注入错误。
// @param ctx 操作上下文
// @param processInstanceID 实例 Id
// @return nil
// @return 注入错误
func (f *failingExceptionStore) ListByInstance(ctx context.Context, processInstanceID string) ([]*TaskException, error) {
	return nil, f.err
}

// ListByTask 恒返回注入错误。
// @param ctx 操作上下文
// @param taskID 任务 Id
// @return nil
// @return 注入错误
func (f *failingExceptionStore) ListByTask(ctx context.Context, taskID string) ([]*TaskException, error) {
	return nil, f.err
}

// ListByTypeAndInstances 恒返回注入错误。
// @param ctx 操作上下文
// @param exceptionType 异常类型
// @param instanceIDs 实例集合
// @return nil
// @return 注入错误
func (f *failingExceptionStore) ListByTypeAndInstances(ctx context.Context, exceptionType string, instanceIDs []string) ([]*TaskException, error) {
	return nil, f.err
}

// MarkHandled 恒返回注入错误。
// @param ctx 操作上下文
// @param ids 记录 Id 列表
// @param status 处理状态
// @return 注入错误
func (f *failingExceptionStore) MarkHandled(ctx context.Context, ids []string, status int) error {
	return f.err
}

// TestExceptionServiceCountConcurrent 验证计数原子性（并发记录后计数等于记录数）。
// @param t 测试上下文
func TestExceptionServiceCountConcurrent(t *testing.T) {
	svc, _ := newExceptionFixture(t)
	ctx := context.Background()
	const workers = 32
	const perWorker = 10
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				// 并发记录（计数为 atomic 语义）
				_, _ = svc.Record(ctx, &ExceptionRequest{Message: fmt.Sprintf("w%d-%d", worker, j)})
			}
		}(i)
	}
	wg.Wait()
	if got := svc.Count(); got != workers*perWorker {
		t.Errorf("并发计数应等于 %d: %d", workers*perWorker, got)
	}
}

// TestExceptionServiceRecordZombieInstance 验证僵尸流程异常记录与去重。
// @param t 测试上下文
func TestExceptionServiceRecordZombieInstance(t *testing.T) {
	svc, store := newExceptionFixture(t)
	ctx := context.Background()
	instance := api.ProcessInstance{
		ID:                   "pi-9",
		Name:                 "僵尸实例",
		ProcessDefinitionID:  "def-1",
		ProcessDefinitionKey: "flowComplete",
		StartUserID:          "alice",
		TenantID:             "dev",
	}
	// 首次记录
	record, err := svc.RecordZombieInstance(ctx, instance)
	if err != nil || record == nil {
		t.Fatalf("首次记录失败: %v, %v", record, err)
	}
	if record.ExceptionType != ExceptionTypeNotTaskMessage || record.Message != "流程未生成待办" {
		t.Errorf("记录内容错误: %s / %s", record.ExceptionType, record.Message)
	}
	// 重复记录跳过
	dup, err := svc.RecordZombieInstance(ctx, instance)
	if err != nil || dup != nil {
		t.Errorf("重复记录应跳过: %v, %v", dup, err)
	}
	// 仅一条落库
	if list, _ := store.ListByInstance(ctx, "pi-9"); len(list) != 1 {
		t.Errorf("应仅一条记录: %d", len(list))
	}
	// 计数只加一
	if svc.Count() != 1 {
		t.Errorf("计数应为 1: %d", svc.Count())
	}
}

// TestExceptionServiceRecordCallbackFailure 验证回调失败记录适配。
// @param t 测试上下文
func TestExceptionServiceRecordCallbackFailure(t *testing.T) {
	svc, store := newExceptionFixture(t)
	ctx := context.Background()
	// 经适配器记录（CallbackClient 的 ExceptionRecorder 端口）
	recorder := NewCallbackExceptionRecorder(svc)
	err := recorder.RecordCallbackException(ctx, &CallbackException{
		Kind:      CallbackKindTask,
		TenantID:  "dev",
		Addr:      "http://biz/callback",
		Payload:   `{"taskId":"t1"}`,
		Attempts:  4,
		Cause:     errors.New("connection refused"),
		OccurTime: time.Now(),
	})
	if err != nil {
		t.Fatalf("回调异常记录失败: %v", err)
	}
	// 落库内容
	list, _ := store.ListUnhandled(ctx)
	if len(list) != 1 {
		t.Fatalf("应落一条记录: %d", len(list))
	}
	if list[0].TenantID != "dev" || list[0].Params != `{"taskId":"t1"}` || list[0].ExceptionClass != "CallbackException" {
		t.Errorf("记录字段错误: %+v", list[0])
	}
	// 直接经服务记录：空异常跳过
	if err := svc.RecordCallbackFailure(ctx, nil); err != nil {
		t.Errorf("空异常应跳过: %v", err)
	}
	// 服务缺失适配器静默跳过
	empty := NewCallbackExceptionRecorder(nil)
	if err := empty.RecordCallbackException(ctx, &CallbackException{}); err != nil {
		t.Errorf("服务缺失应静默: %v", err)
	}
}

// ─── SQL 适配（sqlmock） ─────────────────────────────────────────────

// taskExceptionColumns 是查询列清单文本（与表定义顺序一致）。
const taskExceptionColumns = "SELECT id, task_id, process_instance_id, process_instance_name, class_name, method_name, message, params, exception_type, exception_class, process_definition_id, process_definition_key, process_definition_name, start_user_id, handle_status, creator, creator_name, create_time, updater, update_time, tenant_id FROM workflow_task_exception"

// newExceptionSQLStore 创建任务异常 SQL 适配器与其 sqlmock。
// @param t 测试上下文
// @return 适配器
// @return 数据库模拟
func newExceptionSQLStore(t *testing.T) (*SQLTaskExceptionStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("创建 sqlmock 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := pwf.NewStore(db, pwf.DialectMySQL)
	if err != nil {
		t.Fatalf("创建 workflow 仓储入口失败: %v", err)
	}
	return NewSQLTaskExceptionStore(store), mock
}

// TestSQLTaskExceptionStoreSaveAndGet 验证插入行映射与按主键回读。
// @param t 测试上下文
func TestSQLTaskExceptionStoreSaveAndGet(t *testing.T) {
	adapter, mock := newExceptionSQLStore(t)
	ctx := context.Background()
	entity := &TaskException{
		BaseColumns:         BaseColumns{ID: "ex-1", TenantID: "dev", CreateTime: time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC), UpdateTime: time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC)},
		TaskID:              "task-1",
		ProcessInstanceID:   "pi-1",
		ProcessInstanceName: "实例一",
		ClassName:           "Svc",
		MethodName:          "complete",
		Message:             "失败",
		Params:              "{}",
		ExceptionType:       ExceptionTypeFlowable,
		ExceptionClass:      "FlowableException",
		HandleStatus:        ExceptionUnhandled,
	}
	mock.ExpectExec("INSERT INTO workflow_task_exception").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := adapter.Save(ctx, entity); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	// 回读
	mock.ExpectQuery(taskExceptionColumns).WithArgs("ex-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "task_id", "process_instance_id", "process_instance_name", "class_name", "method_name", "message", "params", "exception_type", "exception_class", "process_definition_id", "process_definition_key", "process_definition_name", "start_user_id", "handle_status", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}).
			AddRow("ex-1", "task-1", "pi-1", "实例一", "Svc", "complete", "失败", "{}", ExceptionTypeFlowable, "FlowableException", "", "", "", "", 0, "", "", time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC), "", time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC), "dev"))
	got, found, err := adapter.Get(ctx, "ex-1")
	if err != nil || !found {
		t.Fatalf("回读失败: %v %v", found, err)
	}
	if got.ExceptionType != ExceptionTypeFlowable || got.TaskID != "task-1" {
		t.Errorf("回读字段错误: %+v", got)
	}
	// 查无
	mock.ExpectQuery(taskExceptionColumns).WithArgs("ex-x").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, found, err := adapter.Get(ctx, "ex-x"); err != nil || found {
		t.Errorf("查无应返回不存在: %v %v", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未满足的期望: %v", err)
	}
}

// TestSQLTaskExceptionStoreLists 验证未处理/实例/任务/类型组合查询。
// @param t 测试上下文
func TestSQLTaskExceptionStoreLists(t *testing.T) {
	adapter, mock := newExceptionSQLStore(t)
	ctx := context.Background()
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "task_id", "process_instance_id", "exception_type", "handle_status"}).
			AddRow("ex-1", "task-1", "pi-1", ExceptionTypeNotTaskMessage, 0)
	}
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_exception WHERE handle_status").WillReturnRows(rows())
	if list, err := adapter.ListUnhandled(ctx); err != nil || len(list) != 1 {
		t.Fatalf("未处理查询错误: %d, %v", len(list), err)
	}
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_exception WHERE process_instance_id").WithArgs("pi-1").WillReturnRows(rows())
	if list, err := adapter.ListByInstance(ctx, "pi-1"); err != nil || len(list) != 1 {
		t.Fatalf("按实例查询错误: %d, %v", len(list), err)
	}
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_exception WHERE task_id").WithArgs("task-1").WillReturnRows(rows())
	if list, err := adapter.ListByTask(ctx, "task-1"); err != nil || len(list) != 1 {
		t.Fatalf("按任务查询错误: %d, %v", len(list), err)
	}
	// 类型组合查询（逐实例）
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_exception WHERE process_instance_id").WithArgs("pi-1").WillReturnRows(rows())
	mock.ExpectQuery("SELECT (.+) FROM workflow_task_exception WHERE process_instance_id").WithArgs("pi-2").WillReturnRows(
		sqlmock.NewRows([]string{"id", "task_id", "process_instance_id", "exception_type", "handle_status"}).
			AddRow("ex-2", "task-2", "pi-2", ExceptionTypeRuntime, 0))
	list, err := adapter.ListByTypeAndInstances(ctx, ExceptionTypeNotTaskMessage, []string{"pi-1", "pi-2"})
	if err != nil || len(list) != 1 {
		t.Fatalf("类型组合查询错误: %d, %v", len(list), err)
	}
	// 标记处理：已处理走专用方法
	mock.ExpectExec("UPDATE workflow_task_exception SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := adapter.MarkHandled(ctx, []string{"ex-1"}, ExceptionHandled); err != nil {
		t.Fatalf("标记处理失败: %v", err)
	}
	// 复位为未处理走通用更新
	mock.ExpectExec("UPDATE workflow_task_exception SET").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := adapter.MarkHandled(ctx, []string{"ex-1"}, ExceptionUnhandled); err != nil {
		t.Fatalf("复位失败: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未满足的期望: %v", err)
	}
}
