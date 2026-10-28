package sqlstate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// fakeResult 是测试用执行结果。
type fakeResult struct{}

// RowsAffected 返回影响行数。
// @return 行数
// @return 错误
func (fakeResult) RowsAffected() (int64, error) { return 1, nil }

// fakeRows 是测试用结果集。
type fakeRows struct {
	data    []byte // 数据
	read    bool   // 是否已读
	empty   bool   // 是否为空
	err     error  // 遍历错误
	scanErr error  // 扫描错误
}

// Next 前进到下一行。
// @return 是否有下一行
func (r *fakeRows) Next() bool {
	if r.read {
		return false
	}
	r.read = true
	return !r.empty
}

// Scan 扫描数据。
// @param dest 目标
// @return 错误
func (r *fakeRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("目标数量错误")
	}
	if r.scanErr != nil {
		return r.scanErr
	}
	target, ok := dest[0].(*[]byte)
	if !ok {
		return errors.New("目标类型错误")
	}
	*target = append([]byte(nil), r.data...)
	return nil
}

// Close 关闭结果集。
// @return 错误
func (r *fakeRows) Close() error { return nil }

// Err 返回遍历错误。
// @return 错误
func (r *fakeRows) Err() error { return r.err }

// fakeDB 是测试用数据库。
type fakeDB struct {
	stored     []byte   // 当前状态
	has        bool     // 是否存在
	execs      []string // 执行语句
	execArgs   [][]any  // 执行参数
	queries    []string // 查询语句
	queryArgs  [][]any  // 查询参数
	execErr    error    // 执行错误
	queryErr   error    // 查询错误
	scanErr    error    // 扫描错误
	rowErr     error    // 遍历错误
	failInsert bool     // 仅插入失败
}

// ExecContext 执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (d *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (Result, error) {
	d.execs = append(d.execs, query)
	d.execArgs = append(d.execArgs, args)
	if d.execErr != nil {
		return nil, d.execErr
	}
	if d.failInsert && strings.HasPrefix(query, "INSERT") {
		return nil, errors.New("insert")
	}
	if strings.HasPrefix(query, "DELETE") {
		d.stored, d.has = nil, false
	}
	if strings.HasPrefix(query, "INSERT") {
		if len(args) >= 4 {
			if data, ok := args[3].([]byte); ok {
				d.stored, d.has = append([]byte(nil), data...), true
			}
		}
	}
	return fakeResult{}, nil
}

// QueryContext 执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 错误
func (d *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	d.queries = append(d.queries, query)
	d.queryArgs = append(d.queryArgs, args)
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	return &fakeRows{data: d.stored, empty: !d.has, err: d.rowErr, scanErr: d.scanErr}, nil
}

// TestNew 验证构造校验。
// @param t 测试上下文
func TestNew(t *testing.T) {
	if _, err := New(nil, "mysql"); err == nil {
		t.Fatalf("空数据库应失败")
	}
	if _, err := New(&fakeDB{}, "oracle"); err == nil {
		t.Fatalf("非法方言应失败")
	}
	store, err := New(&fakeDB{}, "postgresql")
	if err != nil || store.dialect != "postgres" {
		t.Fatalf("postgresql 归一化失败: %v %+v", err, store)
	}
}

// TestSaveLoadMySQL 验证 MySQL 保存读取。
// @param t 测试上下文
// @return 无
func TestSaveLoadMySQL(t *testing.T) {
	db := &fakeDB{}
	store, _ := New(db, "mysql")
	if err := store.Save(context.Background(), []byte("state")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("执行语句错误: %+v", db.execs)
	}
	if !strings.Contains(db.execs[0], "?, ?, ?, ?, ?") {
		t.Fatalf("MySQL 占位符错误: %s", db.execs[0])
	}
	data, err := store.Load(context.Background())
	if err != nil || string(data) != "state" {
		t.Fatalf("读取失败: %v %s", err, data)
	}
	if len(db.queryArgs) != 1 || db.queryArgs[0][0] != stateID {
		t.Fatalf("查询参数错误: %+v", db.queryArgs)
	}
}

// TestSaveLoadPostgres 验证 PostgreSQL 占位符。
// @param t 测试上下文
func TestSaveLoadPostgres(t *testing.T) {
	db := &fakeDB{}
	store, _ := New(db, "postgres")
	if err := store.Save(context.Background(), []byte("pg")); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if !strings.Contains(db.execs[1], "$1, $2, $3, $4, $5") {
		t.Fatalf("PostgreSQL 占位符错误: %s", db.execs[1])
	}
	data, err := store.Load(context.Background())
	if err != nil || string(data) != "pg" {
		t.Fatalf("读取失败: %v %s", err, data)
	}
}

// TestLoadEmpty 验证空状态。
// @param t 测试上下文
func TestLoadEmpty(t *testing.T) {
	store, _ := New(&fakeDB{}, "mysql")
	data, err := store.Load(context.Background())
	if err != nil || data != nil {
		t.Fatalf("空状态应返回 nil: %v %v", err, data)
	}
}

// TestErrors 验证 SQL 错误。
// @param t 测试上下文
func TestErrors(t *testing.T) {
	queryDB := &fakeDB{queryErr: errors.New("query")}
	store, _ := New(queryDB, "mysql")
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatalf("查询错误应返回")
	}
	execDB := &fakeDB{execErr: errors.New("exec")}
	store, _ = New(execDB, "mysql")
	if err := store.Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("删除错误应返回")
	}
	insertDB := &fakeDB{}
	store, _ = New(insertDB, "mysql")
	insertDB.execErr = errors.New("insert")
	if err := store.Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("插入错误应返回")
	}
}

// TestScanAndRowErrors 验证扫描与遍历错误。
// @param t 测试上下文
func TestScanAndRowErrors(t *testing.T) {
	db := &fakeDB{has: true, stored: []byte("x"), scanErr: errors.New("scan")}
	store, _ := New(db, "mysql")
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatalf("扫描错误应返回")
	}
	db = &fakeDB{rowErr: errors.New("rows")}
	store, _ = New(db, "mysql")
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatalf("遍历错误应返回")
	}
	db = &fakeDB{failInsert: true}
	store, _ = New(db, "mysql")
	if err := store.Save(context.Background(), []byte("x")); err == nil {
		t.Fatalf("插入失败应返回")
	}
}

// testDriver 是 database/sql 测试驱动。
type testDriver struct{}

// Open 打开连接。
// @param name 数据源
// @return 连接
// @return 错误
func (testDriver) Open(name string) (driver.Conn, error) { return testConn{}, nil }

// testConn 是测试连接。
type testConn struct{}

// Prepare 准备语句。
// @param query SQL
// @return 语句
// @return 错误
func (testConn) Prepare(query string) (driver.Stmt, error) { return nil, errors.New("unsupported") }

// Close 关闭连接。
// @return 错误
func (testConn) Close() error { return nil }

// Begin 开始事务。
// @return 事务
// @return 错误
func (testConn) Begin() (driver.Tx, error) { return nil, errors.New("unsupported") }

// ExecContext 执行语句。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果
// @return 错误
func (testConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

// QueryContext 执行查询。
// @param ctx 上下文
// @param query SQL
// @param args 参数
// @return 结果集
// @return 错误
func (testConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return &testSQLRows{}, nil
}

// testSQLRows 是空结果集。
type testSQLRows struct{}

// Columns 返回列名。
// @return 列名
func (testSQLRows) Columns() []string { return []string{"BYTES_"} }

// Close 关闭结果集。
// @return 错误
func (testSQLRows) Close() error { return nil }

// Next 前进到下一行。
// @param dest 目标
// @return 错误
func (testSQLRows) Next(dest []driver.Value) error { return io.EOF }

var registerOnce sync.Once

// TestSQLDBAdapter 验证 *sql.DB 适配器。
// @param t 测试上下文
func TestSQLDBAdapter(t *testing.T) {
	registerOnce.Do(func() { sql.Register("sqlstate-test", testDriver{}) })
	db, err := sql.Open("sqlstate-test", "")
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = db.Close() }()
	adapter := SQLDB{DB: db}
	if _, err := adapter.ExecContext(context.Background(), "DELETE FROM ACT_GE_BYTEARRAY"); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	rows, err := adapter.QueryContext(context.Background(), "SELECT BYTES_ FROM ACT_GE_BYTEARRAY")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	_ = rows.Close()
}
