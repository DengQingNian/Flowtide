// business.go 提供业务系统注册管理（P1.1，对位 Java auth 模块 BusinessSystemService）：
// workflow_business_system 表的端口（Store）+ 内存实现 + SQL 适配器 + 注册服务。
// 系统代号 system_symbol 即流程租户 tenantId，是业务系统与流程绑定的查询键。
package workflow

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/idgen"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
)

// BusinessSystem 是业务系统注册信息（对位 Java auth BusinessSystemEntity，按 Flowtide DDL 裁剪）。
type BusinessSystem struct {
	ID                string    // 主键（internal/idgen 生成）
	SystemSymbol      string    // 系统代号（唯一，= 流程租户 tenantId）
	SystemName        string    // 系统名称
	TaskCallAddr      string    // 任务回调地址（BusinessCallTaskListener 目标，空则跳过回调）
	ExecutionCallAddr string    // 执行回调地址（BusinessCallExecutionListener 目标，空则跳过回调）
	Secret            string    // 回调签名密钥（注册时自动生成，可注入）
	TenantID          string    // 租户（惯例与系统代号一致，独立存储便于按租户检索）
	Creator           string    // 创建人
	CreatorName       string    // 创建人姓名
	CreateTime        time.Time // 创建时间
	Updater           string    // 更新人
	UpdateTime        time.Time // 更新时间
}

// businessSystemColumns 是 workflow_business_system 的列清单（与表定义一致，裸 SQL 使用）。
var businessSystemColumns = []string{"id", "system_symbol", "system_name", "task_call_addr", "execution_call_addr",
	"secret", "creator", "creator_name", "create_time", "updater", "update_time", "tenant_id"}

// businessSystem 错误码（对位既有 WORKFLOW- 前缀风格）。
const (
	errCodeBizSystemEmpty    = "WORKFLOW-1001" // 系统代号为空
	errCodeBizSystemConflict = "WORKFLOW-1002" // 系统代号已注册
	errCodeBizSystemNotFound = "WORKFLOW-1404" // 业务系统不存在
)

// BusinessSystemStore 是业务系统注册的存储端口（内存实现供单测/嵌入式，SQL 适配器走 workflow_business_system 表）。
type BusinessSystemStore interface {
	// Save 保存注册信息（按 SystemSymbol 幂等 upsert：不存在则新增，存在则覆盖业务列）。
	// @param ctx 操作上下文
	// @param system 注册信息（新增时 ID 必须非空，由调用方生成）
	// @return 保存错误
	Save(ctx context.Context, system *BusinessSystem) error
	// GetBySymbol 按系统代号查询。
	// @param ctx 操作上下文
	// @param symbol 系统代号
	// @return 注册信息
	// @return 是否存在
	// @return 查询错误
	GetBySymbol(ctx context.Context, symbol string) (*BusinessSystem, bool, error)
	// GetByTenant 按租户查询（回调时 tenantId → 注册信息的查询键）。
	// @param ctx 操作上下文
	// @param tenantID 租户
	// @return 注册信息
	// @return 是否存在
	// @return 查询错误
	GetByTenant(ctx context.Context, tenantID string) (*BusinessSystem, bool, error)
	// List 返回全部注册信息（按系统代号升序）。
	// @param ctx 操作上下文
	// @return 注册列表
	// @return 查询错误
	List(ctx context.Context) ([]BusinessSystem, error)
	// Delete 按系统代号删除注册。
	// @param ctx 操作上下文
	// @param symbol 系统代号
	// @return 删除错误（不存在时返回 not found 错误）
	Delete(ctx context.Context, symbol string) error
}

// MemoryBusinessSystemStore 是业务系统注册的内存实现（单测与嵌入式无 DB 模式），并发安全。
type MemoryBusinessSystemStore struct {
	mu      sync.RWMutex              // 保护 systems
	systems map[string]BusinessSystem // 键 = 系统代号
}

// NewMemoryBusinessSystemStore 创建内存业务系统存储。
// @return 内存存储
func NewMemoryBusinessSystemStore() *MemoryBusinessSystemStore {
	return &MemoryBusinessSystemStore{systems: make(map[string]BusinessSystem)}
}

// Save 保存注册信息（按系统代号 upsert）。
// @param ctx 操作上下文
// @param system 注册信息（新增时 ID 必须非空）
// @return 保存错误
func (m *MemoryBusinessSystemStore) Save(ctx context.Context, system *BusinessSystem) error {
	if system == nil {
		return api.NewError(errCodeBizSystemEmpty, "业务系统注册信息不能为空", api.ErrBadRequest)
	}
	if strings.TrimSpace(system.SystemSymbol) == "" {
		return api.NewError(errCodeBizSystemEmpty, "系统代号不能为空", api.ErrBadRequest)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// upsert：存在则覆盖（保留原 ID），不存在则要求调用方已生成 ID
	if existing, ok := m.systems[system.SystemSymbol]; ok {
		system.ID = existing.ID
	} else if strings.TrimSpace(system.ID) == "" {
		return api.NewError(errCodeBizSystemEmpty, "新增业务系统必须传入非空 ID", api.ErrBadRequest)
	}
	m.systems[system.SystemSymbol] = *system
	return nil
}

// GetBySymbol 按系统代号查询。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 注册信息
// @return 是否存在
// @return 查询错误
func (m *MemoryBusinessSystemStore) GetBySymbol(ctx context.Context, symbol string) (*BusinessSystem, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	system, ok := m.systems[symbol]
	if !ok {
		return nil, false, nil
	}
	return &system, true, nil
}

// GetByTenant 按租户查询（线性扫描，内存实现数据量小可接受）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 注册信息
// @return 是否存在
// @return 查询错误
func (m *MemoryBusinessSystemStore) GetByTenant(ctx context.Context, tenantID string) (*BusinessSystem, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, system := range m.systems {
		if system.TenantID == tenantID {
			copied := system
			return &copied, true, nil
		}
	}
	return nil, false, nil
}

// List 返回全部注册信息，按系统代号升序保证确定性。
// @param ctx 操作上下文
// @return 注册列表
// @return 查询错误
func (m *MemoryBusinessSystemStore) List(ctx context.Context) ([]BusinessSystem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]BusinessSystem, 0, len(m.systems))
	for _, system := range m.systems {
		out = append(out, system)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SystemSymbol < out[j].SystemSymbol })
	return out, nil
}

// Delete 按系统代号删除注册。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 删除错误（不存在时报 WORKFLOW-1404）
func (m *MemoryBusinessSystemStore) Delete(ctx context.Context, symbol string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.systems[symbol]; !ok {
		return api.NewError(errCodeBizSystemNotFound, fmt.Sprintf("业务系统不存在: %s", symbol), api.ErrNotFound)
	}
	delete(m.systems, symbol)
	return nil
}

// SQLBusinessSystemStore 是业务系统注册的 SQL 适配器：
// 组合 internal/persistence/workflow 已有 BusinessSystemRepository 完成常规 CRUD，
// 仓储缺失的方法（按租户查询）以裸 SQL 补齐（不修改 internal/persistence 包）。
type SQLBusinessSystemStore struct {
	repo    pwf.BusinessSystemRepository // 既有行仓储（Insert/UpdateByPK/DeleteByPK/GetBySymbol/ListAll）
	db      *sql.DB                      // 裸 SQL 补齐用连接池（与仓储同库）
	dialect string                       // 方言（mysql "?" / postgres "$n"）
}

// NewSQLBusinessSystemStore 创建 SQL 适配器。
// @param db 数据库连接池（与引擎共享同一连接池）
// @param dialect 数据库方言（mysql/postgres）
// @param store workflow 行仓储入口
// @return SQL 适配器
// @return 入参为空错误
func NewSQLBusinessSystemStore(db *sql.DB, dialect string, store *pwf.Store) (*SQLBusinessSystemStore, error) {
	if db == nil || store == nil {
		return nil, api.NewError(errCodeBizSystemEmpty, "创建业务系统 SQL 存储需要非空连接池与仓储入口", api.ErrBadRequest)
	}
	return &SQLBusinessSystemStore{
		repo:    pwf.NewBusinessSystemRepository(store),
		db:      db,
		dialect: dialect,
	}, nil
}

// Save 保存注册信息（按系统代号 upsert）。
// @param ctx 操作上下文
// @param system 注册信息（新增时 ID 必须非空）
// @return 保存错误
func (s *SQLBusinessSystemStore) Save(ctx context.Context, system *BusinessSystem) error {
	if system == nil {
		return api.NewError(errCodeBizSystemEmpty, "业务系统注册信息不能为空", api.ErrBadRequest)
	}
	if strings.TrimSpace(system.SystemSymbol) == "" {
		return api.NewError(errCodeBizSystemEmpty, "系统代号不能为空", api.ErrBadRequest)
	}
	existing, found, err := s.repo.GetBySymbol(ctx, system.SystemSymbol)
	if err != nil {
		return err
	}
	if found {
		// 已注册：保留原 ID，按主键覆盖业务列
		id, _ := existing["id"].(string)
		_, err = s.repo.UpdateByPK(ctx, id, s.toRow(system, false))
		return err
	}
	// 新注册：ID 必须由调用方生成传入（与仓储契约一致）
	if strings.TrimSpace(system.ID) == "" {
		return api.NewError(errCodeBizSystemEmpty, "新增业务系统必须传入非空 ID", api.ErrBadRequest)
	}
	return s.repo.Insert(ctx, s.toRow(system, true))
}

// GetBySymbol 按系统代号查询（委托既有仓储）。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 注册信息
// @return 是否存在
// @return 查询错误
func (s *SQLBusinessSystemStore) GetBySymbol(ctx context.Context, symbol string) (*BusinessSystem, bool, error) {
	row, found, err := s.repo.GetBySymbol(ctx, symbol)
	if err != nil || !found {
		return nil, false, err
	}
	return s.fromRow(row), true, nil
}

// GetByTenant 按租户查询（既有仓储无此方法，裸 SQL 补齐）。
// @param ctx 操作上下文
// @param tenantID 租户
// @return 注册信息
// @return 是否存在
// @return 查询错误
func (s *SQLBusinessSystemStore) GetByTenant(ctx context.Context, tenantID string) (*BusinessSystem, bool, error) {
	// 列名来自常量清单非用户输入，无注入风险；占位符按方言生成
	query := fmt.Sprintf("SELECT %s FROM workflow_business_system WHERE tenant_id = %s ORDER BY system_symbol LIMIT 1",
		strings.Join(businessSystemColumns, ", "), s.ph(1))
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, false, api.NewError(errCodeBizSystemNotFound, "按租户查询业务系统失败", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return nil, false, nil
	}
	row, err := scanBusinessSystemRow(rows, businessSystemColumns)
	if err != nil {
		return nil, false, err
	}
	return s.fromRow(row), true, nil
}

// List 返回全部注册信息（委托既有仓储 ListAll，按系统代号排序）。
// @param ctx 操作上下文
// @return 注册列表
// @return 查询错误
func (s *SQLBusinessSystemStore) List(ctx context.Context) ([]BusinessSystem, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BusinessSystem, 0, len(rows))
	for _, row := range rows {
		out = append(out, *s.fromRow(row))
	}
	return out, nil
}

// Delete 按系统代号删除（先查 ID 再按主键删除，复用既有仓储）。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 删除错误（不存在时报 WORKFLOW-1404）
func (s *SQLBusinessSystemStore) Delete(ctx context.Context, symbol string) error {
	row, found, err := s.repo.GetBySymbol(ctx, symbol)
	if err != nil {
		return err
	}
	if !found {
		return api.NewError(errCodeBizSystemNotFound, fmt.Sprintf("业务系统不存在: %s", symbol), api.ErrNotFound)
	}
	id, _ := row["id"].(string)
	_, err = s.repo.DeleteByPK(ctx, id)
	return err
}

// ph 返回第 n 个方言占位符。
// @param n 序号（从 1 开始）
// @return 占位符文本
func (s *SQLBusinessSystemStore) ph(n int) string {
	// MySQL 用 "?"，PostgreSQL 用 "$n"，与 workflow 行仓储方言约定一致
	if s.dialect == pwf.DialectMySQL {
		return "?"
	}
	return "$" + fmt.Sprintf("%d", n)
}

// toRow 将注册实体转为行数据（column 与 DDL 一致）。
// @param system 注册实体
// @param inserting 是否为插入（插入补 create_time，更新只补 update_time）
// @return 行数据
func (s *SQLBusinessSystemStore) toRow(system *BusinessSystem, inserting bool) pwf.Row {
	row := pwf.Row{
		"system_symbol":       system.SystemSymbol,
		"system_name":         system.SystemName,
		"task_call_addr":      system.TaskCallAddr,
		"execution_call_addr": system.ExecutionCallAddr,
		"secret":              system.Secret,
		"creator":             system.Creator,
		"creator_name":        system.CreatorName,
		"updater":             system.Updater,
		"tenant_id":           system.TenantID,
	}
	if inserting {
		row["id"] = system.ID
		row["create_time"] = system.CreateTime
	}
	row["update_time"] = system.UpdateTime
	return row
}

// fromRow 将行数据转为注册实体。
// @param row 行数据
// @return 注册实体
func (s *SQLBusinessSystemStore) fromRow(row pwf.Row) *BusinessSystem {
	return &BusinessSystem{
		ID:                bizString(row["id"]),
		SystemSymbol:      bizString(row["system_symbol"]),
		SystemName:        bizString(row["system_name"]),
		TaskCallAddr:      bizString(row["task_call_addr"]),
		ExecutionCallAddr: bizString(row["execution_call_addr"]),
		Secret:            bizString(row["secret"]),
		Creator:           bizString(row["creator"]),
		CreatorName:       bizString(row["creator_name"]),
		CreateTime:        bizTime(row["create_time"]),
		Updater:           bizString(row["updater"]),
		UpdateTime:        bizTime(row["update_time"]),
		TenantID:          bizString(row["tenant_id"]),
	}
}

// scanBusinessSystemRow 扫描一行结果为通用行（与行仓储 scanRow 同构，避免导出其内部方法）。
// @param rows 结果集
// @param columns 列清单
// @return 行数据
// @return 扫描错误
func scanBusinessSystemRow(rows *sql.Rows, columns []string) (pwf.Row, error) {
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, api.NewError(errCodeBizSystemNotFound, "扫描业务系统结果行失败", err)
	}
	row := make(pwf.Row, len(columns))
	for i, column := range columns {
		// 驱动可能返回 []byte，统一转为字符串
		if raw, ok := values[i].([]byte); ok {
			row[column] = string(raw)
		} else {
			row[column] = values[i]
		}
	}
	return row, nil
}

// bizString 读取行值为字符串（nil 容错，前缀 biz 避免与 memstore.go 同名工具冲突）。
// @param value 行值
// @return 字符串
func bizString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

// bizTime 读取行值为时间（nil 容错，前缀 biz 避免与既有工具冲突）。
// @param value 行值
// @return 时间
func bizTime(value any) time.Time {
	if t, ok := value.(time.Time); ok {
		return t
	}
	return time.Time{}
}

// SecretGenerator 是回调签名密钥生成器（可注入）。
// @return 密钥文本
// @return 生成错误
type SecretGenerator func() (string, error)

// BusinessSystemOptions 是业务系统注册服务的可注入依赖。
type BusinessSystemOptions struct {
	IDs     idgen.Generator  // ID 生成器（默认 UUIDv7）
	Secrets SecretGenerator  // 密钥生成器（默认 16 字节随机数 hex）
	Now     func() time.Time // 时间源（默认 time.Now，测试可注入固定时间）
}

// BusinessSystemOption 是服务选项函数。
type BusinessSystemOption func(*BusinessSystemOptions)

// WithBusinessSystemIDs 注入 ID 生成器。
// @param generator ID 生成器
// @return 选项
func WithBusinessSystemIDs(generator idgen.Generator) BusinessSystemOption {
	return func(o *BusinessSystemOptions) {
		if generator != nil {
			o.IDs = generator
		}
	}
}

// WithBusinessSystemSecrets 注入密钥生成器。
// @param generator 密钥生成器
// @return 选项
func WithBusinessSystemSecrets(generator SecretGenerator) BusinessSystemOption {
	return func(o *BusinessSystemOptions) {
		if generator != nil {
			o.Secrets = generator
		}
	}
}

// WithBusinessSystemNow 注入时间源。
// @param now 时间源
// @return 选项
func WithBusinessSystemNow(now func() time.Time) BusinessSystemOption {
	return func(o *BusinessSystemOptions) {
		if now != nil {
			o.Now = now
		}
	}
}

// BusinessSystemService 是业务系统注册门面服务（对位 Java BusinessSystemService 的注册/维护能力）。
type BusinessSystemService struct {
	store BusinessSystemStore   // 存储端口
	opts  BusinessSystemOptions // 可注入依赖
}

// NewBusinessSystemService 创建业务系统注册服务。
// @param store 存储端口
// @param options 可选依赖（ID/密钥/时间源注入）
// @return 服务实例
func NewBusinessSystemService(store BusinessSystemStore, options ...BusinessSystemOption) *BusinessSystemService {
	opts := BusinessSystemOptions{
		IDs: idgen.NewUUIDv7(),
		// 默认密钥：16 字节加密随机数 hex（32 字符），可按需替换为签名密钥分发方案
		Secrets: defaultSecretGenerator,
		Now:     time.Now,
	}
	for _, option := range options {
		option(&opts)
	}
	return &BusinessSystemService{store: store, opts: opts}
}

// defaultSecretGenerator 生成默认回调签名密钥（16 字节加密随机数 hex）。
// @return 密钥文本
// @return 生成错误
func defaultSecretGenerator() (string, error) {
	buf := make([]byte, 16)
	// 加密随机源失败即返回错误，绝不落盘可预测密钥
	if _, err := rand.Read(buf); err != nil {
		return "", api.NewError(errCodeBizSystemConflict, "生成业务系统密钥失败", err)
	}
	return hex.EncodeToString(buf), nil
}

// Register 注册新业务系统：system_symbol 唯一校验，自动生成 ID 与密钥。
// @param ctx 操作上下文
// @param input 注册入参（SystemSymbol/SystemName/回调地址/TenantID/Creator 必要，Secret 可预先指定否则自动生成）
// @return 注册成功后的完整信息
// @return 代号为空/已注册/密钥生成/存储错误
func (s *BusinessSystemService) Register(ctx context.Context, input *BusinessSystem) (*BusinessSystem, error) {
	if input == nil {
		return nil, api.NewError(errCodeBizSystemEmpty, "业务系统注册信息不能为空", api.ErrBadRequest)
	}
	symbol := strings.TrimSpace(input.SystemSymbol)
	if symbol == "" {
		return nil, api.NewError(errCodeBizSystemEmpty, "系统代号不能为空", api.ErrBadRequest)
	}
	if _, found, err := s.store.GetBySymbol(ctx, symbol); err != nil {
		return nil, err
	} else if found {
		return nil, api.NewError(errCodeBizSystemConflict, fmt.Sprintf("系统代号已注册: %s", symbol), api.ErrConflict)
	}
	secret := input.Secret
	if strings.TrimSpace(secret) == "" {
		generated, err := s.opts.Secrets()
		if err != nil {
			return nil, err
		}
		secret = generated
	}
	now := s.opts.Now()
	system := &BusinessSystem{
		ID:                s.opts.IDs.NewID(),
		SystemSymbol:      symbol,
		SystemName:        input.SystemName,
		TaskCallAddr:      input.TaskCallAddr,
		ExecutionCallAddr: input.ExecutionCallAddr,
		Secret:            secret,
		TenantID:          input.TenantID,
		Creator:           input.Creator,
		CreatorName:       input.CreatorName,
		CreateTime:        now,
		Updater:           input.Creator,
		UpdateTime:        now,
	}
	// 惯例：租户缺省取系统代号（Java 协议查询键 tenantId = systemSymbol）
	if strings.TrimSpace(system.TenantID) == "" {
		system.TenantID = symbol
	}
	if err := s.store.Save(ctx, system); err != nil {
		return nil, err
	}
	return system, nil
}

// Update 更新注册信息（按系统代号定位，仅覆盖传入的非空业务列；代号本身不可改）。
// @param ctx 操作上下文
// @param input 更新入参（SystemSymbol 定位必填；SystemName/回调地址/Secret 为空表示不修改）
// @return 更新后的完整信息
// @return 不存在/存储错误
func (s *BusinessSystemService) Update(ctx context.Context, input *BusinessSystem) (*BusinessSystem, error) {
	if input == nil {
		return nil, api.NewError(errCodeBizSystemEmpty, "业务系统更新信息不能为空", api.ErrBadRequest)
	}
	symbol := strings.TrimSpace(input.SystemSymbol)
	if symbol == "" {
		return nil, api.NewError(errCodeBizSystemEmpty, "系统代号不能为空", api.ErrBadRequest)
	}
	existing, found, err := s.store.GetBySymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError(errCodeBizSystemNotFound, fmt.Sprintf("业务系统不存在: %s", symbol), api.ErrNotFound)
	}
	// 仅覆盖非空字段：空值语义为"不修改"
	if strings.TrimSpace(input.SystemName) != "" {
		existing.SystemName = input.SystemName
	}
	if input.TaskCallAddr != "" {
		existing.TaskCallAddr = input.TaskCallAddr
	}
	if input.ExecutionCallAddr != "" {
		existing.ExecutionCallAddr = input.ExecutionCallAddr
	}
	if strings.TrimSpace(input.Secret) != "" {
		existing.Secret = input.Secret
	}
	existing.Updater = input.Updater
	existing.UpdateTime = s.opts.Now()
	if err := s.store.Save(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Get 按系统代号查询注册信息。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 注册信息
// @return 不存在/查询错误
func (s *BusinessSystemService) Get(ctx context.Context, symbol string) (*BusinessSystem, error) {
	system, found, err := s.store.GetBySymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, api.NewError(errCodeBizSystemNotFound, fmt.Sprintf("业务系统不存在: %s", symbol), api.ErrNotFound)
	}
	return system, nil
}

// List 返回全部注册信息（按系统代号升序）。
// @param ctx 操作上下文
// @return 注册列表
// @return 查询错误
func (s *BusinessSystemService) List(ctx context.Context) ([]BusinessSystem, error) {
	return s.store.List(ctx)
}

// Remove 注销业务系统。
// @param ctx 操作上下文
// @param symbol 系统代号
// @return 不存在/删除错误
func (s *BusinessSystemService) Remove(ctx context.Context, symbol string) error {
	return s.store.Delete(ctx, symbol)
}
