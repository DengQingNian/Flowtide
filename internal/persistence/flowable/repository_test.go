package flowable

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

// TestDiffOperations 验证增量操作。
// @param t 测试上下文
func TestDiffOperations(t *testing.T) {
	row := func(id string, revision int, name string) Row {
		row, _ := NewRow("ACT_RU_JOB")
		_ = row.Set("ID_", id)
		_ = row.Set("REV_", revision)
		_ = row.Set("TYPE_", name)
		return *row
	}
	first := row("j1", 1, "timer")
	changed := row("j1", 1, "async")
	added := row("j2", 1, "timer")
	operations := DiffOperations(nil, []Row{first})
	if len(operations) != 1 || operations[0].Kind != OperationInsert {
		t.Fatalf("新增操作错误: %+v", operations)
	}
	operations = DiffOperations([]Row{first}, []Row{first})
	if len(operations) != 0 {
		t.Fatalf("无变化应无操作: %+v", operations)
	}
	operations = DiffOperations([]Row{first}, []Row{changed})
	if len(operations) != 1 || operations[0].Kind != OperationUpdate || operations[0].OldRevision != 1 {
		t.Fatalf("更新操作错误: %+v", operations)
	}
	operations = DiffOperations([]Row{first}, []Row{changed, added})
	if len(operations) != 2 {
		t.Fatalf("更新+新增操作错误: %+v", operations)
	}
	operations = DiffOperations([]Row{first}, nil)
	if len(operations) != 1 || operations[0].Kind != OperationDelete {
		t.Fatalf("删除操作错误: %+v", operations)
	}
	noRevision, _ := NewRow("ACT_RE_DEPLOYMENT")
	_ = noRevision.Set("ID_", "d1")
	_ = noRevision.Set("NAME_", "a")
	changedNoRevision, _ := NewRow("ACT_RE_DEPLOYMENT")
	_ = changedNoRevision.Set("ID_", "d1")
	_ = changedNoRevision.Set("NAME_", "b")
	operations = DiffOperations([]Row{*noRevision}, []Row{*changedNoRevision})
	if len(operations) != 1 || operations[0].Kind != OperationUpdate {
		t.Fatalf("无 REV 行应直接更新: %+v", operations)
	}
	if operations := DiffOperations([]Row{{Table: "ACT_RU_JOB"}}, nil); len(operations) != 0 {
		t.Fatalf("无 ID 行应忽略")
	}
}

// queryRows 是测试查询结果集。
type queryRows struct {
	values [][]any // 行值
	index  int     // 当前行
	err    error   // 错误
	closed bool    // 是否关闭
}

// Next 前进到下一行。
// @return 是否有下一行
func (r *queryRows) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}

// Scan 扫描当前行。
// @param dest 目标
// @return 错误
func (r *queryRows) Scan(dest ...any) error {
	if len(dest) != len(r.values[r.index-1]) {
		return errors.New("列数量不匹配")
	}
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
func (r *queryRows) Close() error {
	r.closed = true
	return nil
}

// Err 返回错误。
// @return 错误
func (r *queryRows) Err() error { return r.err }

// queryDB 是测试查询数据库。
type queryDB struct {
	fakeDB
	deployment [][]any // 部署行
	queryErr   error   // 查询错误
}

// QueryContext 执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 错误
func (d *queryDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	d.queries = append(d.queries, query)
	d.args = append(d.args, args)
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	if strings.Contains(query, "ACT_RE_DEPLOYMENT") {
		return &queryRows{values: d.deployment}, nil
	}
	return &queryRows{}, nil
}

// TestRepositoryLoadAll 验证行加载。
// @param t 测试上下文
func TestRepositoryLoadAll(t *testing.T) {
	db := &queryDB{deployment: [][]any{{"d1", "demo", "", "", "t1", time.Now(), "", "", "", "8.1.0.3"}}}
	repository, _ := NewRepository(db, "mysql")
	rows, err := repository.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(rows) != 1 || rows[0].Table != "ACT_RE_DEPLOYMENT" || rows[0].Values["ID_"] != "d1" {
		t.Fatalf("加载行错误: %+v", rows)
	}
	if _, err := (&Repository{db: &fakeDB{}, dialect: "mysql"}).LoadAll(context.Background()); err == nil {
		t.Fatalf("不支持查询应失败")
	}
	if _, err := (&Repository{db: &queryDB{queryErr: errors.New("q")}, dialect: "mysql"}).LoadAll(context.Background()); err == nil {
		t.Fatalf("查询错误应失败")
	}
}

// TestRowIDValue 验证数值 ID 处理。
// @param t 测试上下文
func TestRowIDValue(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{"s", "s"}, {1, "1"}, {int64(2), "2"}, {3.0, "3"},
	}
	for _, tc := range cases {
		row := Row{Table: "ACT_HI_TSK_LOG", Values: map[string]any{"ID_": tc.value}}
		if id, ok := rowIDValue(row); !ok || id != tc.want {
			t.Fatalf("ID 转换错误: %v %s", ok, id)
		}
	}
	if _, ok := rowIDValue(Row{Table: "ACT_HI_TSK_LOG", Values: map[string]any{"ID_": struct{}{}}}); ok {
		t.Fatalf("非法 ID 应失败")
	}
}

// TestLoadAllByTenant 验证租户查询与后置过滤。
// @param t 测试上下文
func TestLoadAllByTenant(t *testing.T) {
	db := &queryDB{deployment: [][]any{{"d1", "demo", "", "", "t1", time.Now(), "", "", "", "8.1.0.3"}}}
	repository, _ := NewRepository(db, "mysql")
	rows, err := repository.LoadAllByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("租户加载失败: %v", err)
	}
	tenantQueries := 0
	for i, query := range db.queries {
		if strings.Contains(query, "WHERE TENANT_ID_ = ?") {
			tenantQueries++
			if len(db.args[i]) != 1 || db.args[i][0] != "t1" {
				t.Fatalf("租户参数错误: %+v", db.args[i])
			}
		}
	}
	if tenantQueries == 0 {
		t.Fatalf("未生成租户查询")
	}
	if len(rows) != 1 || rows[0].Values["ID_"] != "d1" {
		t.Fatalf("租户行错误: %+v", rows)
	}
}

// TestFilterTenantRows 验证无租户列的行过滤。
// @param t 测试上下文
func TestFilterTenantRows(t *testing.T) {
	row := func(table string, values map[string]any) Row {
		return Row{Table: table, Values: values}
	}
	rows := []Row{
		row("ACT_RE_DEPLOYMENT", map[string]any{"ID_": "d1"}),
		row("ACT_RU_EXECUTION", map[string]any{"PROC_INST_ID_": "i1"}),
		row("ACT_GE_BYTEARRAY", map[string]any{"DEPLOYMENT_ID_": "d1"}),
		row("ACT_GE_BYTEARRAY", map[string]any{"DEPLOYMENT_ID_": "d2"}),
		row("ACT_HI_DETAIL", map[string]any{"PROC_INST_ID_": "i1"}),
		row("ACT_HI_DETAIL", map[string]any{"PROC_INST_ID_": "i2"}),
	}
	filtered := FilterRowsByTenant(rows, "")
	if len(filtered) != 4 {
		t.Fatalf("过滤结果错误: %+v", filtered)
	}
}
