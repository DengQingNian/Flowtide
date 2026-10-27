package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/flowable-go/flowable/api"
	flowablerow "github.com/flowable-go/flowable/internal/persistence/flowable"
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
	if db.tx == nil || !db.tx.committed || db.tx.executed != len(previous) {
		t.Fatalf("首次写入次数错误: %+v", db.tx)
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
