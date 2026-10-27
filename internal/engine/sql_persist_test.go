package engine

import (
	"context"
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
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (t *persistTx) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	t.executed++
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
	tx *persistTx // 事务
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d *persistDB) ExecContext(ctx context.Context, query string, args ...any) (flowablerow.Result, error) {
	return persistResult{}, nil
}

// BeginTx 开始事务。
// @param ctx 上下文
// @return 事务
// @return 错误
func (d *persistDB) BeginTx(ctx context.Context) (flowablerow.Transaction, error) {
	d.tx = &persistTx{}
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
