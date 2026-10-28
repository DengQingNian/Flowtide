package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
	statestore "github.com/flowable-go/flowable/internal/persistence/state"
	"github.com/flowable-go/flowable/internal/tenant"
)

// persistResult 是快照测试结果。
type persistResult struct{}

// RowsAffected 返回影响行数。
// @return 行数
// @return 错误
func (persistResult) RowsAffected() (int64, error) { return 1, nil }

// persistTx 是快照测试事务。
type persistTx struct {
	executed   int  // 执行次数
	committed  bool // 是否提交
	rolledBack bool // 是否回滚
	fail       bool // 是否失败
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (t *persistTx) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	t.executed++
	if t.fail {
		return nil, errors.New("tx")
	}
	return persistResult{}, nil
}

// Commit 提交事务。
// @return 错误
func (t *persistTx) Commit() error {
	t.committed = true
	return nil
}

// Rollback 回滚事务。
// @return 错误
func (t *persistTx) Rollback() error {
	t.rolledBack = true
	return nil
}

// persistDB 是快照测试数据库。
type persistDB struct {
	tx   *persistTx // 事务
	fail bool       // 是否失败
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d *persistDB) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	if d.fail {
		return nil, errors.New("exec")
	}
	return persistResult{}, nil
}

// BeginTx 开始事务。
// @param ctx 上下文
// @return 事务
// @return 错误
func (d *persistDB) BeginTx(ctx context.Context) (flowablerow.Transaction, error) {
	d.tx = &persistTx{fail: d.fail}
	return d.tx, nil
}

// TestPersistSnapshot 验证整份快照写入。
// @param t 测试上下文
func TestPersistSnapshot(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	db := &persistDB{}
	repository, err := flowablerow.NewRepository(db, "postgres")
	if err != nil {
		t.Fatalf("创建仓储失败: %v", err)
	}
	if err := engine.PersistSnapshot(context.Background(), repository); err != nil {
		t.Fatalf("写入快照失败: %v", err)
	}
	if db.tx == nil || !db.tx.committed || db.tx.executed == 0 {
		t.Fatalf("事务状态错误: %+v", db.tx)
	}
}

// TestPersistSnapshot_Errors 验证快照写入错误。
// @param t 测试上下文
func TestPersistSnapshot_Errors(t *testing.T) {
	engine := newTestEngine(t)
	if err := engine.PersistSnapshot(context.Background(), nil); err == nil {
		t.Fatalf("空仓储应失败")
	}
	_ = engine.Close(context.Background())
	repository, _ := flowablerow.NewRepository(&persistDB{}, "mysql")
	if err := engine.PersistSnapshot(context.Background(), repository); err == nil {
		t.Fatalf("关闭后写入应失败")
	}
}

// TestPersistDiff 验证增量快照写入。
// @param t 测试上下文
func TestPersistDiff(t *testing.T) {
	engine := newTestEngine(t)
	deployXML(t, engine, simpleProcess)
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	previous, err := engine.SnapshotRows(context.Background())
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	db := &persistDB{}
	repository, _ := flowablerow.NewRepository(db, "mysql")
	if err := engine.PersistDiff(context.Background(), repository, nil); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	current, _ := engine.SnapshotRows(context.Background())
	if db.tx == nil || !db.tx.committed || db.tx.executed != len(current) {
		t.Fatalf("首次写入次数错误: %+v current=%d", db.tx, len(current))
	}
	task := listTasks(t, engine)[0]
	if err := engine.Complete(context.Background(), task.ID, api.CompleteTaskRequest{}); err != nil {
		t.Fatalf("完成任务失败: %v", err)
	}
	db = &persistDB{}
	repository, _ = flowablerow.NewRepository(db, "mysql")
	if err := engine.PersistDiff(context.Background(), repository, previous); err != nil {
		t.Fatalf("增量写入失败: %v", err)
	}
	if db.tx == nil || !db.tx.committed || db.tx.executed == 0 {
		t.Fatalf("增量写入无操作: %+v", db.tx)
	}
	if err := engine.PersistDiff(context.Background(), nil, nil); err == nil {
		t.Fatalf("空仓储应失败")
	}
}

// TestEngine_AutoPersistRows 验证命令后自动持久化。
// @param t 测试上下文
func TestEngine_AutoPersistRows(t *testing.T) {
	db := &persistDB{}
	repository, _ := flowablerow.NewRepository(db, "mysql")
	engine, err := New(Config{RowRepository: repository})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, engine, simpleProcess)
	if db.tx == nil || !db.tx.committed || db.tx.executed == 0 {
		t.Fatalf("部署后未持久化: %+v", db.tx)
	}
	first := db.tx.executed
	db.tx = nil
	if _, err := engine.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	}); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if db.tx == nil || db.tx.executed == 0 {
		t.Fatalf("启动后未增量持久化")
	}
	if len(engine.lastRows) == 0 || len(engine.lastRows) < first {
		t.Fatalf("lastRows 未更新")
	}
}

// TestEngine_AutoPersistError 验证持久化失败回滚。
// @param t 测试上下文
func TestEngine_AutoPersistError(t *testing.T) {
	db := &persistDB{fail: true}
	repository, _ := flowablerow.NewRepository(db, "mysql")
	engine, err := New(Config{RowRepository: repository})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	if _, err := engine.Deploy(context.Background(), api.DeploymentRequest{
		Resources: map[string][]byte{"test.bpmn20.xml": []byte(simpleProcess)},
	}); err == nil {
		t.Fatalf("行持久化失败应返回错误")
	}
	if len(engine.state.Deployments) != 0 {
		t.Fatalf("失败后部署应回滚")
	}
}

// rowSet 是快照查询结果集。
type rowSet struct {
	values [][]any // 行
	index  int     // 当前行
}

// Next 前进到下一行。
// @return 是否有下一行
func (r *rowSet) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}

// Scan 扫描当前行。
// @param dest 目标
// @return 错误
func (r *rowSet) Scan(dest ...any) error {
	for i, value := range r.values[r.index-1] {
		target, ok := dest[i].(*any)
		if !ok {
			return errors.New("目标类型错误")
		}
		*target = value
	}
	return nil
}

// Close 关闭结果集。
// @return 错误
func (r *rowSet) Close() error { return nil }

// Err 返回错误。
// @return 错误
func (r *rowSet) Err() error { return nil }

// snapshotQueryDB 是按表提供快照行的数据库。
type snapshotQueryDB struct {
	rows map[string][]flowablerow.Row // 表到行
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d *snapshotQueryDB) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	return persistResult{}, nil
}

// QueryContext 执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 错误
func (d *snapshotQueryDB) QueryContext(ctx context.Context, query string, args ...any) (flowablerow.Rows, error) {
	index := strings.LastIndex(query, " FROM ")
	if index < 0 {
		return &rowSet{}, nil
	}
	table := strings.TrimSpace(query[index+6:])
	rows := d.rows[table]
	if strings.Contains(table, " WHERE ") {
		table = strings.TrimSpace(strings.SplitN(table, " WHERE ", 2)[0])
		rows = d.rows[table]
		if len(args) > 0 {
			filtered := make([]flowablerow.Row, 0)
			for _, row := range rows {
				if row.Values["TENANT_ID_"] == args[0] {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}
	}
	values := make([][]any, 0, len(rows))
	for _, row := range rows {
		columns := row.Columns()
		line := make([]any, len(columns))
		for i, column := range columns {
			line[i] = row.Values[column]
		}
		values = append(values, line)
	}
	return &rowSet{values: values}, nil
}

// TestEngine_LoadRows 验证启动时从行恢复。
// @param t 测试上下文
func TestEngine_LoadRows(t *testing.T) {
	source := newTestEngine(t)
	deployXML(t, source, simpleProcess)
	instance, _ := source.StartProcessInstance(context.Background(), api.StartRequest{
		ProcessDefinitionKey: "simple",
		Variables:            []api.Variable{{Name: "initiator", Value: "alice"}},
	})
	rows, _ := source.SnapshotRows(context.Background())
	grouped := make(map[string][]flowablerow.Row)
	for _, row := range rows {
		grouped[row.Table] = append(grouped[row.Table], row)
	}
	repository, _ := flowablerow.NewRepository(&snapshotQueryDB{rows: grouped}, "mysql")
	target, err := New(Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatalf("从行恢复失败: %v", err)
	}
	definition, err := target.GetProcessDefinitionByKey(context.Background(), "simple")
	if err != nil || definition.Key != "simple" {
		t.Fatalf("恢复定义错误: %v", err)
	}
	tasks, _ := target.ListTasks(context.Background(), api.TaskFilter{})
	if tasks.Total != 1 {
		t.Fatalf("恢复任务错误: %+v", tasks)
	}
	values, _ := target.GetVariables(context.Background(), instance.ID)
	if values["initiator"] != "alice" {
		t.Fatalf("恢复变量错误: %+v", values)
	}
}

// TestEngine_RowSourceOnly 验证行仓储为唯一状态源。
// @param t 测试上下文
func TestEngine_RowSourceOnly(t *testing.T) {
	db := &snapshotQueryDB{rows: make(map[string][]flowablerow.Row)}
	repository, _ := flowablerow.NewRepository(db, "mysql")
	store := statestore.NewMemory()
	engine, err := New(Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatalf("创建引擎失败: %v", err)
	}
	deployXML(t, engine, simpleProcess)
	if data, err := store.Load(context.Background()); err != nil || data != nil {
		t.Fatalf("行源模式不应写状态存储: %v %v", data, err)
	}
	rows, _ := engine.SnapshotRows(context.Background())
	grouped := make(map[string][]flowablerow.Row)
	for _, row := range rows {
		grouped[row.Table] = append(grouped[row.Table], row)
	}
	db.rows = grouped
	restored, err := New(Config{RowRepository: repository, LoadRows: true})
	if err != nil {
		t.Fatalf("重新创建失败: %v", err)
	}
	if _, err := restored.GetProcessDefinitionByKey(context.Background(), "simple"); err != nil {
		t.Fatalf("从行源恢复定义失败: %v", err)
	}
}

// TestEngine_LoadRowsTenant 验证启动按租户恢复。
// @param t 测试上下文
func TestEngine_LoadRowsTenant(t *testing.T) {
	source := newTestEngine(t)
	if _, err := source.Deploy(tenant.With(context.Background(), "t2"), api.DeploymentRequest{
		TenantID:  "t2",
		Resources: map[string][]byte{"test.bpmn20.xml": []byte(simpleProcess)},
	}); err != nil {
		t.Fatalf("部署失败: %v", err)
	}
	rows, _ := source.SnapshotRows(context.Background())
	grouped := make(map[string][]flowablerow.Row)
	for _, row := range rows {
		grouped[row.Table] = append(grouped[row.Table], row)
	}
	db := &snapshotQueryDB{rows: grouped}
	repository, _ := flowablerow.NewRepository(db, "mysql")
	empty, err := New(Config{RowRepository: repository, LoadRows: true, LoadTenant: "t1"})
	if err != nil {
		t.Fatalf("t1 恢复失败: %v", err)
	}
	if definitions, _ := empty.ListProcessDefinitions(context.Background()); len(definitions) != 0 {
		t.Fatalf("t1 不应恢复定义: %+v", definitions)
	}
	tenant, err := New(Config{RowRepository: repository, LoadRows: true, LoadTenant: "t2", TenantID: "t2"})
	if err != nil {
		t.Fatalf("t2 恢复失败: %v", err)
	}
	if _, err := tenant.GetProcessDefinitionByKey(context.Background(), "simple"); err != nil {
		t.Fatalf("t2 应恢复定义: %v", err)
	}
}

// BeginTx 为恢复测试替身提供命令级事务能力。
// @param ctx 上下文
// @return 事务
// @return 创建错误
func (d *snapshotQueryDB) BeginTx(ctx context.Context) (flowablerow.Transaction, error) {
	return &persistTx{}, nil
}
