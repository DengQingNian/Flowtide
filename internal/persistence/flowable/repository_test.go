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
