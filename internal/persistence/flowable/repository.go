package flowable

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/flowable-go/flowable/api"
)

// Result 是 SQL 执行结果。
type Result interface {
	// RowsAffected 返回影响行数。
	// @return 影响行数
	// @return 错误
	RowsAffected() (int64, error)
}

// DB 是行写入接口。
type DB interface {
	// ExecContext 执行 SQL。
	// @param ctx 上下文
	// @param query SQL
	// @param args 参数
	// @return 结果
	// @return 错误
	ExecContext(ctx context.Context, query string, args ...any) (Result, error)
}

// Transaction 是数据库事务。
type Transaction interface {
	DB
	// Commit 提交事务。
	// @return 提交错误
	Commit() error
	// Rollback 回滚事务。
	// @return 回滚错误
	Rollback() error
}

// TransactionalDB 是支持事务的数据库。
type TransactionalDB interface {
	DB
	// BeginTx 开始事务。
	// @param ctx 上下文
	// @return 事务
	// @return 错误
	BeginTx(ctx context.Context) (Transaction, error)
}

// 操作类型。
const (
	OperationInsert = "insert" // 插入
	OperationUpdate = "update" // 更新
	OperationDelete = "delete" // 删除
)

// Operation 是行操作。
type Operation struct {
	Kind        string // 类型
	Row         Row    // 行
	OldRevision int64  // 更新时旧版本
}

// Rows 是查询结果集。
type Rows interface {
	// Next 前进到下一行。
	// @return 是否有下一行
	Next() bool
	// Scan 扫描当前行。
	// @param dest 目标
	// @return 扫描错误
	Scan(dest ...any) error
	// Close 关闭结果集。
	// @return 关闭错误
	Close() error
	// Err 返回遍历错误。
	// @return 遍历错误
	Err() error
}

// QueryDB 是支持查询的数据库。
type QueryDB interface {
	DB
	// QueryContext 执行查询。
	// @param ctx 上下文
	// @param query SQL
	// @param args 参数
	// @return 结果集
	// @return 错误
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
}

// Repository 提供 ACT 行的插入、更新与删除。
type Repository struct {
	db      DB     // 数据库
	dialect string // 方言
}

// NewRepository 创建行仓储。
// @param db 数据库
// @param dialect 方言
// @return 仓储
// @return 创建错误
func NewRepository(db DB, dialect string) (*Repository, error) {
	if db == nil {
		return nil, api.NewError("ROW-0008", "数据库不能为空", api.ErrBadRequest)
	}
	if err := validateDialect(dialect); err != nil {
		return nil, err
	}
	return &Repository{db: db, dialect: dialect}, nil
}

// Insert 插入一行。
// @param ctx 上下文
// @param row 行
// @return 影响行数
// @return 插入错误
func (r *Repository) Insert(ctx context.Context, row Row) (int64, error) {
	query, args, err := row.InsertSQL(r.dialect)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0009", "插入行失败", err)
	}
	return result.RowsAffected()
}

// Update 更新一行并校验 REV_。
// @param ctx 上下文
// @param row 行
// @param oldRevision 旧版本
// @return 影响行数
// @return 更新错误
func (r *Repository) Update(ctx context.Context, row Row, oldRevision int64) (int64, error) {
	query, args, err := row.UpdateSQL(r.dialect, oldRevision)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0010", "更新行失败", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, api.NewError("ROW-0011", "读取更新行数失败", err)
	}
	if affected == 0 && tableHasColumn(row.Table, "REV_") {
		return 0, api.NewError("ROW-0012", "乐观锁冲突: "+row.Table, api.ErrConflict)
	}
	return affected, nil
}

// Delete 删除一行。
// @param ctx 上下文
// @param row 行
// @return 影响行数
// @return 删除错误
func (r *Repository) Delete(ctx context.Context, row Row) (int64, error) {
	query, args, err := row.DeleteSQL(r.dialect)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, api.NewError("ROW-0013", "删除行失败", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, versioned := revisionValue(row); versioned && affected == 0 {
		return 0, api.NewError("ROW-0012", "删除时发生乐观锁冲突", api.ErrConflict)
	}
	return affected, nil
}

// Apply 在事务中执行一组行操作。
// @param ctx 上下文
// @param operations 操作列表
// @return 执行错误
func (r *Repository) Apply(ctx context.Context, operations []Operation) error {
	run := func(repository *Repository) error {
		for _, operation := range operations {
			var err error
			switch operation.Kind {
			case OperationInsert:
				_, err = repository.Insert(ctx, operation.Row)
			case OperationUpdate:
				_, err = repository.Update(ctx, operation.Row, operation.OldRevision)
			case OperationDelete:
				_, err = repository.Delete(ctx, operation.Row)
			default:
				return api.NewError("ROW-0014", "不支持的操作: "+operation.Kind, api.ErrBadRequest)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
	transactional, ok := r.db.(TransactionalDB)
	if !ok {
		return run(r)
	}
	transaction, err := transactional.BeginTx(ctx)
	if err != nil {
		return api.NewError("ROW-0015", "开始事务失败", err)
	}
	repository := &Repository{db: transaction, dialect: r.dialect}
	if err := run(repository); err != nil {
		_ = transaction.Rollback()
		return err
	}
	if err := transaction.Commit(); err != nil {
		_ = transaction.Rollback()
		return api.NewError("ROW-0016", "提交事务失败", err)
	}
	return nil
}

// SnapshotOperations 为整份快照生成"先删后插"操作。
// @param rows 行列表
// @return 操作列表
func SnapshotOperations(rows []Row) []Operation {
	operations := make([]Operation, 0, len(rows)*2)
	for _, row := range rows {
		deletion := withRevision(row, Row{})
		delete(deletion.Values, "REV_")
		operations = append(operations, Operation{Kind: OperationDelete, Row: deletion})
	}
	operations = orderOperations(operations)
	for _, row := range orderedRows(rows) {
		operations = append(operations, Operation{Kind: OperationInsert, Row: row})
	}
	return operations
}

// ReplaceAll 在事务中替换整份快照。
// @param ctx 上下文
// @param rows 行列表
// @return 替换错误
func (r *Repository) ReplaceAll(ctx context.Context, rows []Row) error {
	return r.Apply(ctx, SnapshotOperations(rows))
}

// DiffOperations 计算新旧行快照的增量操作。
// @param oldRows 旧行
// @param newRows 新行
// @return 操作列表
func DiffOperations(oldRows, newRows []Row) []Operation {
	oldIndex := make(map[string]Row, len(oldRows))
	for _, row := range oldRows {
		if id, ok := rowIDValue(row); ok {
			oldIndex[row.Table+"\x00"+id] = row
		}
	}
	operations := make([]Operation, 0)
	seen := make(map[string]bool, len(newRows))
	for _, row := range newRows {
		id, ok := rowIDValue(row)
		if !ok {
			continue
		}
		key := row.Table + "\x00" + id
		seen[key] = true
		old, exists := oldIndex[key]
		if !exists {
			operations = append(operations, Operation{Kind: OperationInsert, Row: row})
			continue
		}
		row = withRevision(row, old)
		if reflect.DeepEqual(old.Values, row.Values) {
			continue
		}
		revision, hasRevision := revisionValue(old)
		if !hasRevision {
			operations = append(operations, Operation{Kind: OperationUpdate, Row: row})
			continue
		}
		row.Values["REV_"] = revision + 1
		operations = append(operations, Operation{Kind: OperationUpdate, Row: row, OldRevision: revision})
	}
	for _, row := range oldRows {
		id, ok := rowIDValue(row)
		if !ok {
			continue
		}
		if !seen[row.Table+"\x00"+id] {
			operations = append(operations, Operation{Kind: OperationDelete, Row: row})
		}
	}
	return orderOperations(operations)
}

// rowIDValue 读取完整行标识，复合键按 JSON 数组编码以避免分隔符冲突。
// @param row 行
// @return ID
// @return 是否存在
func rowIDValue(row Row) (string, bool) {
	parts := make([]string, 0)
	for _, key := range primaryKeys(row.Table) {
		value, ok := rowKeyString(row.Values[key])
		if !ok {
			return "", false
		}
		parts = append(parts, value)
	}
	// 单列主键沿用旧标识，复合键的编码不受标识中竖线或零字节影响。
	if len(parts) == 1 {
		return parts[0], true
	}
	data, _ := json.Marshal(parts)
	return string(data), true
}

// rowKeyString 将支持的 SQL 主键标量转成规范字符串，拒绝空值和结构体。
// @param value 主键分量
// @return 标识文本
// @return 是否支持
func rowKeyString(value any) (string, bool) {
	// 保持数值主键与既有任务日志行的标识语义一致。
	switch typed := value.(type) {
	case string:
		return typed, true
	case int:
		return fmt.Sprint(typed), true
	case int64:
		return fmt.Sprint(typed), true
	case float64:
		return fmt.Sprint(int64(typed)), true
	default:
		return "", false
	}
}

// revisionValue 读取行的 REV_。
// @param row 行
// @return 版本
// @return 是否存在
func revisionValue(row Row) (int64, bool) {
	value, ok := row.Values["REV_"]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}

// LoadAll 读取全部支持表的行。
// @param ctx 上下文
// @return 行列表
// @return 读取错误
func (r *Repository) LoadAll(ctx context.Context) ([]Row, error) {
	return r.loadRows(ctx, "")
}

// LoadAllByTenant 读取指定租户的行。
// @param ctx 上下文
// @param tenantID 租户
// @return 行列表
// @return 读取错误
func (r *Repository) LoadAllByTenant(ctx context.Context, tenantID string) ([]Row, error) {
	if tenantID == "" {
		return r.LoadAll(ctx)
	}
	return r.loadRows(ctx, tenantID)
}

// loadRows 读取行，tenantID 非空时按租户过滤。
// @param ctx 上下文
// @param tenantID 租户
// @return 行列表
// @return 读取错误
func (r *Repository) loadRows(ctx context.Context, tenantID string) ([]Row, error) {
	queryDB, ok := r.db.(QueryDB)
	if !ok {
		return nil, api.NewError("ROW-0017", "数据库不支持查询", api.ErrBadRequest)
	}
	rows := make([]Row, 0)
	for _, tableName := range Tables() {
		table := tables[tableName]
		query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(table.Columns, ", "), table.Name)
		args := make([]any, 0, 1)
		if tenantID != "" && hasColumn(table, "TENANT_ID_") {
			query += " WHERE TENANT_ID_ = " + placeholder(r.dialect, 1)
			args = append(args, tenantID)
		}
		result, err := queryDB.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, api.NewError("ROW-0018", "查询表失败: "+table.Name, err)
		}
		for result.Next() {
			values := make([]any, len(table.Columns))
			targets := make([]any, len(table.Columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err := result.Scan(targets...); err != nil {
				_ = result.Close()
				return nil, api.NewError("ROW-0019", "扫描表失败: "+table.Name, err)
			}
			row, err := NewRow(table.Name)
			if err != nil {
				_ = result.Close()
				return nil, err
			}
			for i, column := range table.Columns {
				row.Values[column] = normalizeValue(column, values[i])
			}
			rows = append(rows, *row)
		}
		if err := result.Err(); err != nil {
			_ = result.Close()
			return nil, api.NewError("ROW-0020", "遍历表失败: "+table.Name, err)
		}
		_ = result.Close()
	}
	if tenantID != "" {
		rows = FilterRowsByTenant(rows, tenantID)
	}
	return rows, nil
}

// hasColumn 判断表是否包含列。
// @param table 表
// @param column 列名
// @return 是否包含
func hasColumn(table Table, column string) bool {
	for _, name := range table.Columns {
		if name == column {
			return true
		}
	}
	return false
}

// normalizeValue 将驱动返回值统一为通用类型。
// @param column 列名
// @param value 驱动值
// @return 通用值
func normalizeValue(column string, value any) any {
	if column == "BYTES_" || column == "FULL_MSG_" || column == "PASSWORD_" {
		return value
	}
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return value
	}
}

// SupportsTransactions 判断仓储是否能原子提交一条引擎命令。
// @return 是否支持事务
func (r *Repository) SupportsTransactions() bool {
	_, ok := r.db.(TransactionalDB)
	return ok
}
