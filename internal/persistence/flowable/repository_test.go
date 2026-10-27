package flowable

import (
	"context"
	"errors"
	"testing"
)

// fakeResult 是测试结果。
type fakeResult struct {
	affected int64 // 影响行数
	err      error // 错误
}

// RowsAffected 返回影响行数。
// @return 行数
// @return 错误
func (r fakeResult) RowsAffected() (int64, error) { return r.affected, r.err }

// fakeDB 是测试数据库。
type fakeDB struct {
	queries []string // SQL
	args    [][]any  // 参数
	result  Result   // 结果
	err     error    // 错误
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	d.queries = append(d.queries, query)
	d.args = append(d.args, args)
	if d.err != nil {
		return nil, d.err
	}
	return d.result, nil
}

// TestRepository 验证行仓储插入/更新/删除。
// @param t 测试上下文
func TestRepository(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	repository, err := NewRepository(db, "mysql")
	if err != nil {
		t.Fatalf("创建仓储失败: %v", err)
	}
	row, _ := NewRow("ACT_GE_BYTEARRAY")
	_ = row.Set("ID_", "b1")
	_ = row.Set("REV_", 1)
	_ = row.Set("NAME_", "n")
	if affected, err := repository.Insert(context.Background(), *row); err != nil || affected != 1 {
		t.Fatalf("插入失败: %v %d", err, affected)
	}
	if affected, err := repository.Update(context.Background(), *row, 1); err != nil || affected != 1 {
		t.Fatalf("更新失败: %v %d", err, affected)
	}
	if affected, err := repository.Delete(context.Background(), *row); err != nil || affected != 1 {
		t.Fatalf("删除失败: %v %d", err, affected)
	}
	if len(db.queries) != 3 {
		t.Fatalf("SQL 数量错误: %+v", db.queries)
	}
}

// TestRepository_Errors 验证仓储错误。
// @param t 测试上下文
func TestRepository_Errors(t *testing.T) {
	if _, err := NewRepository(nil, "mysql"); err == nil {
		t.Fatalf("空数据库应失败")
	}
	if _, err := NewRepository(&fakeDB{}, "oracle"); err == nil {
		t.Fatalf("非法方言应失败")
	}
	db := &fakeDB{result: fakeResult{affected: 0}}
	repository, _ := NewRepository(db, "mysql")
	row, _ := NewRow("ACT_GE_BYTEARRAY")
	_ = row.Set("ID_", "b1")
	_ = row.Set("REV_", 1)
	if _, err := repository.Update(context.Background(), *row, 1); err == nil {
		t.Fatalf("零行更新应冲突")
	}
	db.result = fakeResult{err: errors.New("rows")}
	if _, err := repository.Update(context.Background(), *row, 1); err == nil {
		t.Fatalf("读取行数错误应失败")
	}
	db.err = errors.New("exec")
	if _, err := repository.Insert(context.Background(), *row); err == nil {
		t.Fatalf("插入错误应失败")
	}
	if _, err := repository.Update(context.Background(), *row, 1); err == nil {
		t.Fatalf("更新错误应失败")
	}
	if _, err := repository.Delete(context.Background(), *row); err == nil {
		t.Fatalf("删除错误应失败")
	}
}

// fakeTransaction 是测试事务。
type fakeTransaction struct {
	fakeDB
	committed  bool  // 是否提交
	rolledBack bool  // 是否回滚
	commitErr  error // 提交错误
}

// Commit 提交事务。
// @return 错误
func (t *fakeTransaction) Commit() error {
	t.committed = true
	return t.commitErr
}

// Rollback 回滚事务。
// @return 错误
func (t *fakeTransaction) Rollback() error {
	t.rolledBack = true
	return nil
}

// fakeTransactionalDB 是测试事务数据库。
type fakeTransactionalDB struct {
	fakeDB
	tx       *fakeTransaction // 事务
	beginErr error            // 开始错误
}

// BeginTx 开始事务。
// @param ctx 上下文
// @return 事务
// @return 错误
func (d *fakeTransactionalDB) BeginTx(ctx context.Context) (Transaction, error) {
	if d.beginErr != nil {
		return nil, d.beginErr
	}
	d.tx = &fakeTransaction{fakeDB: fakeDB{result: fakeResult{affected: 1}}}
	return d.tx, nil
}

// TestRepositoryApply 验证事务批量操作。
// @param t 测试上下文
func TestRepositoryApply(t *testing.T) {
	db := &fakeTransactionalDB{}
	repository, _ := NewRepository(db, "postgres")
	row, _ := NewRow("ACT_GE_BYTEARRAY")
	_ = row.Set("ID_", "b1")
	_ = row.Set("REV_", 1)
	operations := []Operation{
		{Kind: OperationInsert, Row: *row},
		{Kind: OperationUpdate, Row: *row, OldRevision: 1},
		{Kind: OperationDelete, Row: *row},
	}
	if err := repository.Apply(context.Background(), operations); err != nil {
		t.Fatalf("事务执行失败: %v", err)
	}
	if db.tx == nil || !db.tx.committed || db.tx.rolledBack {
		t.Fatalf("事务状态错误: %+v", db.tx)
	}
	if len(db.tx.queries) != 3 {
		t.Fatalf("事务 SQL 数量错误: %+v", db.tx.queries)
	}
}

// TestRepositoryApply_Errors 验证事务错误回滚。
// @param t 测试上下文
func TestRepositoryApply_Errors(t *testing.T) {
	db := &fakeTransactionalDB{}
	repository, _ := NewRepository(db, "mysql")
	if err := repository.Apply(context.Background(), []Operation{{Kind: "bad"}}); err == nil {
		t.Fatalf("非法操作应失败")
	}
	if db.tx == nil || !db.tx.rolledBack {
		t.Fatalf("非法操作应回滚")
	}
	row, _ := NewRow("ACT_GE_BYTEARRAY")
	_ = row.Set("ID_", "b1")
	_ = row.Set("REV_", 1)
	db = &fakeTransactionalDB{}
	repository, _ = NewRepository(db, "mysql")
	if err := repository.Apply(context.Background(), []Operation{{Kind: OperationUpdate, Row: *row, OldRevision: 1}}); err != nil {
		t.Fatalf("更新事务失败: %v", err)
	}
	db = &fakeTransactionalDB{beginErr: errors.New("begin")}
	repository, _ = NewRepository(db, "mysql")
	if err := repository.Apply(context.Background(), nil); err == nil {
		t.Fatalf("开始事务错误应失败")
	}
}

// TestRepositoryApply_CommitError 验证提交错误。
// @param t 测试上下文
func TestRepositoryApply_CommitError(t *testing.T) {
	db := &fakeTransactionalDB{}
	repository, _ := NewRepository(db, "mysql")
	// 直接构造带提交错误的事务
	tx := &fakeTransaction{fakeDB: fakeDB{result: fakeResult{affected: 1}}, commitErr: errors.New("commit")}
	db.tx = tx
	transactional := &commitErrorDB{tx: tx}
	repository, _ = NewRepository(transactional, "mysql")
	if err := repository.Apply(context.Background(), nil); err == nil {
		t.Fatalf("提交错误应失败")
	}
	if !tx.rolledBack {
		t.Fatalf("提交失败应回滚")
	}
}

// commitErrorDB 返回固定事务。
type commitErrorDB struct {
	tx *fakeTransaction // 事务
}

// ExecContext 执行 SQL。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d *commitErrorDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	return fakeResult{affected: 1}, nil
}

// BeginTx 开始事务。
// @param ctx 上下文
// @return 事务
// @return 错误
func (d *commitErrorDB) BeginTx(ctx context.Context) (Transaction, error) { return d.tx, nil }
