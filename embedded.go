package flowtide

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/DengQingNian/Flowtide/api"
	"github.com/DengQingNian/Flowtide/internal/engine"
	flowablerow "github.com/DengQingNian/Flowtide/internal/persistence/flowable"
	mysqlstore "github.com/DengQingNian/Flowtide/internal/persistence/mysql"
	pwf "github.com/DengQingNian/Flowtide/internal/persistence/workflow"
	"github.com/DengQingNian/Flowtide/internal/rest"
	"github.com/DengQingNian/Flowtide/internal/tenant"
	"github.com/DengQingNian/Flowtide/internal/workflow"
)

// EmbeddedConfig 是业务宿主嵌入完整 workflow 的配置；生产仅接受已完成迁移的 MySQL 行仓储。
type EmbeddedConfig struct {
	DB               *sql.DB                                       // 宿主拥有的连接池，SDK 关闭时不关闭连接池，最大连接数至少为二
	Database         string                                        // 数据库名，必须与 SELECT DATABASE() 相同，作为全库写者锁命名空间
	TenantID         string                                        // 业务流程租户，宿主负责将其与数据库租户显式绑定
	HistoryLevel     string                                        // 历史级别，默认 audit
	InMemory         bool                                          // 仅供显式原型和测试，禁止与 DB、Database 同时配置
	DecorateIdentity func(api.IdentityService) api.IdentityService // 可选宿主身份读取适配，返回值不能为空
}

// Embedded 持有引擎、workflow 门面、兼容 HTTP 入口和数据库写者锁；不自行监听端口或启动调度。
type Embedded struct {
	engine   *Engine           // 唯一执行内核
	services api.Engine        // 可选身份装饰后的公共服务集合
	workflow *workflow.Service // 与内核配套的 workflow 门面
	handler  http.Handler      // 未附加宿主认证的处理器，必须由宿主保护
	release  func() error      // 只释放持锁会话，连接池所有权属于宿主
	once     sync.Once         // 关闭只执行一次
	closeErr error             // 首次关闭结果
}

// NewEmbedded 构造完整嵌入运行时，只读校验结构，失败时释放已获得的资源。
// @param ctx 有界初始化上下文
// @param cfg 嵌入配置，生产必须提供 DB 和 Database
// @return 运行时
// @return 配置、结构、锁、恢复或 workflow 装配错误
func NewEmbedded(ctx context.Context, cfg EmbeddedConfig) (*Embedded, error) {
	if cfg.InMemory && (cfg.DB != nil || cfg.Database != "") || !cfg.InMemory && (cfg.DB == nil || cfg.Database == "") {
		return nil, errors.New("嵌入模式必须显式选择内存或完整 MySQL 连接配置")
	}
	app := &Embedded{}
	engineConfig := Config{TenantID: cfg.TenantID, HistoryLevel: cfg.HistoryLevel, StartupContext: ctx}
	var opts []workflow.Option
	// 生产装配只验证已完成迁移的结构，不自动初始化或接管 Java 原库。
	if !cfg.InMemory {
		var database string
		if err := cfg.DB.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
			return nil, err
		}
		if database != cfg.Database {
			return nil, errors.New("嵌入数据库名与连接实际数据库不一致")
		}
		if cfg.DB.Stats().MaxOpenConnections == 1 {
			return nil, errors.New("嵌入连接池至少需要两个连接")
		}
		if err := EnsureMySQLSchema(ctx, cfg.DB, false); err != nil {
			return nil, err
		}
		locked, err := mysqlstore.NewLockedRows(ctx, cfg.DB, database)
		if err != nil {
			return nil, err
		}
		app.release = locked.Release
		if err := mysqlstore.CheckNoSnapshot(ctx, cfg.DB); err != nil {
			return nil, errors.Join(err, app.release())
		}
		repo, err := flowablerow.NewRepository(locked, "mysql")
		if err != nil {
			return nil, errors.Join(err, app.release())
		}
		store, err := pwf.NewStore(cfg.DB, "mysql")
		if err != nil {
			return nil, errors.Join(err, app.release())
		}
		engineConfig.RowRepository, engineConfig.LoadRows = repo, true
		opts = append(opts, workflow.WithSQLStores(store, cfg.DB))
	}
	var err error
	app.engine, err = New(engineConfig)
	if err != nil {
		return nil, errors.Join(err, app.Close(ctx))
	}
	app.services = app.engine
	if cfg.DecorateIdentity != nil {
		identity := cfg.DecorateIdentity(app.engine.IdentityService())
		if identity == nil {
			return nil, errors.Join(errors.New("宿主身份装饰器返回空服务"), app.Close(ctx))
		}
		app.services = &embeddedServices{Engine: app.engine, identity: identity}
	}
	app.workflow, err = workflow.NewWorkflowServiceChecked(app.services, opts...)
	if err != nil {
		return nil, errors.Join(err, app.Close(ctx))
	}
	// 三组生产钩子与独立服务一致；预热失败必须显式终止，不能带缺失审批语义启动。
	for _, register := range []func(context.Context) error{app.workflow.RegisterExecutorRewriteHook, app.workflow.RegisterCountersignRuntime, app.workflow.RegisterBusinessCallbackHook} {
		if err := register(ctx); err != nil {
			return nil, errors.Join(fmt.Errorf("嵌入 workflow 钩子装配失败: %w", err), app.Close(ctx))
		}
	}
	app.handler = rest.New(app.services, rest.WithWorkflowService(app.workflow)).Handler()
	return app, nil
}

// Engine 返回用于定义部署、任务查询等原生操作的公共接口。
// @return 引擎服务
func (e *Embedded) Engine() api.Engine { return e.services }

// Workflow 返回兼容 Java 业务语义的公共门面。
// @return workflow 门面
func (e *Embedded) Workflow() api.WorkflowFacade { return e.workflow }

// Handler 返回 /workflow/** 与 /service/** 处理器；认证、授权和入口范围由宿主控制。
// @return HTTP 处理器
func (e *Embedded) Handler() http.Handler { return e.handler }

// RunDueJobs 执行到期定时任务，调度频率和上下文生命周期由宿主管理。
// @param ctx 调度上下文
// @return 执行数量
// @return 执行错误
func (e *Embedded) RunDueJobs(ctx context.Context) (int, error) { return e.engine.RunDueJobs(ctx) }

// Close 先保存并关闭引擎，再释放写者锁，不关闭宿主连接池；可重复调用。
// @param ctx 有界关闭上下文
// @return 首次关闭错误
func (e *Embedded) Close(ctx context.Context) error {
	e.once.Do(func() {
		if e.engine != nil {
			e.closeErr = e.engine.Close(ctx)
		}
		if e.release != nil {
			e.closeErr = errors.Join(e.closeErr, e.release())
		}
	})
	return e.closeErr
}

// WithExecutionContext 将宿主校验后的业务租户与办理人写入引擎上下文，不执行认证。
// @param ctx 宿主上下文
// @param tenantID 可信业务租户
// @param actorID 可信办理身份主键
// @return 引擎执行上下文
func WithExecutionContext(ctx context.Context, tenantID, actorID string) context.Context {
	return engine.WithActor(tenant.With(ctx, tenantID), actorID)
}

// embeddedServices 保留具体引擎的扩展能力，仅将身份读取交给宿主适配器。
type embeddedServices struct {
	*Engine                      // 全部原生服务、事件钩子和会签监听器注册
	identity api.IdentityService // 宿主身份服务
}

// IdentityService 返回宿主适配后的身份服务，不修改引擎状态中的身份快照。
// @return 身份服务
func (e *embeddedServices) IdentityService() api.IdentityService { return e.identity }
