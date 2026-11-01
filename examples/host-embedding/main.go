// Command host-embedding 演示业务宿主内嵌 Flowtide 引擎与 workflow 门面。
//
// 内存模式无需数据库，直接启动即可观察 Handler、门面调用与到期作业驱动：
//
//	go run ./examples/host-embedding
//
// 生产模式复用宿主连接池，连接已完成迁移的 MySQL 库：
//
//	go run ./examples/host-embedding \
//	  -dsn 'user:pass@tcp(127.0.0.1:3306)/flowtide?parseTime=true' \
//	  -database flowtide -tenant oa
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DengQingNian/Flowtide"
	_ "github.com/go-sql-driver/mysql"
)

// options 是示例进程的启动参数。
type options struct {
	addr       string        // 宿主监听地址
	dsn        string        // MySQL DSN，空表示内存模式
	database   string        // 库名，必须与连接实际库一致
	tenant     string        // 业务流程租户
	interval   time.Duration // 到期作业扫描间隔
	initSchema bool          // 是否允许首次初始化结构，生产应为 false
}

// main 解析参数、装配嵌入运行时并提供宿主自己的 HTTP 入口。
// @return 无；失败以非零状态退出
func main() {
	cfg := options{}
	flag.StringVar(&cfg.addr, "addr", ":8099", "宿主监听地址")
	flag.StringVar(&cfg.dsn, "dsn", "", "MySQL DSN，留空使用内存模式")
	flag.StringVar(&cfg.database, "database", "", "数据库名，必须与 SELECT DATABASE() 一致")
	flag.StringVar(&cfg.tenant, "tenant", "demo", "业务流程租户")
	flag.DurationVar(&cfg.interval, "jobs-interval", 5*time.Second, "到期作业扫描间隔")
	flag.BoolVar(&cfg.initSchema, "init-schema", false, "首次运行时初始化引擎与 workflow 结构")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app, closeDB, err := openRuntime(ctx, cfg)
	if err != nil {
		slog.Error("嵌入运行时装配失败", "error", err)
		os.Exit(1)
	}
	defer closeDB()

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           newHostHandler(app, cfg.tenant),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go driveJobs(ctx, app, cfg.interval)
	go func() {
		slog.Info("宿主入口已启动", "addr", cfg.addr, "prefix", "/flowtide/")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("宿主 HTTP 服务异常退出", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 先停止接入流量，再关闭引擎并释放数据库写者锁。
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("宿主 HTTP 停机超时", "error", err)
	}
	if err := app.Close(shutdownCtx); err != nil {
		slog.Error("嵌入运行时关闭失败", "error", err)
		os.Exit(1)
	}
	slog.Info("宿主已退出")
}

// openRuntime 按参数选择内存或 MySQL 行仓储模式并构造嵌入运行时。
// @param ctx 有界初始化上下文
// @param cfg 启动参数
// @return 嵌入运行时
// @return 连接池关闭函数，内存模式为空操作
// @return 装配错误
func openRuntime(ctx context.Context, cfg options) (*flowtide.Embedded, func(), error) {
	if cfg.dsn == "" {
		app, err := flowtide.NewEmbedded(ctx, flowtide.EmbeddedConfig{InMemory: true, TenantID: cfg.tenant})
		return app, func() {}, err
	}
	db, err := sql.Open("mysql", cfg.dsn)
	if err != nil {
		return nil, func() {}, err
	}
	// 持锁会话独占一个连接，连接池上限为一会立即耗尽。
	db.SetMaxOpenConns(10)
	// 结构初始化只在接入方显式允许时执行，之后仅做只读校验。
	if cfg.initSchema {
		if err := flowtide.EnsureMySQLSchema(ctx, db, true); err != nil {
			db.Close()
			return nil, func() {}, err
		}
	}
	app, err := flowtide.NewEmbedded(ctx, flowtide.EmbeddedConfig{
		DB:           db,
		Database:     cfg.database,
		TenantID:     cfg.tenant,
		HistoryLevel: "audit",
	})
	if err != nil {
		db.Close()
		return nil, func() {}, err
	}
	return app, func() { db.Close() }, nil
}

// newHostHandler 组装宿主路由：认证中间件 + 挂载到子路径的嵌入式处理器。
// @param app 嵌入运行时
// @param tenant 默认业务流程租户
// @return 宿主处理器
func newHostHandler(app *flowtide.Embedded, tenant string) http.Handler {
	mux := http.NewServeMux()
	// 引擎处理器的路径以 / 开头，挂在子路径时必须先剥离前缀。
	mux.Handle("/flowtide/", http.StripPrefix("/flowtide", hostAuth(tenant, app.Handler())))
	mux.HandleFunc("GET /host/deployed", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		result, err := app.Workflow().IsProcessDeployed(r.Context(), tenant, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// R.Success 只表示调用成功，业务结果在 Data。
		deployed, _ := result.Data.(bool)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deployed":` + boolText(deployed) + `}`))
	})
	return mux
}

// hostAuth 是宿主认证的最小示例：只信任已经过认证的主体，再注入租户与办理人。
// @param tenant 业务流程租户
// @param next 下游处理器
// @return 处理器
func hostAuth(tenant string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := r.Header.Get("X-Actor")
		if actor == "" {
			http.Error(w, "缺少宿主认证主体", http.StatusUnauthorized)
			return
		}
		// 引擎需要 X-Tenant-Id 参与租户解析，示例要求由宿主显式写入。
		r.Header.Set("X-Tenant-Id", tenant)
		next.ServeHTTP(w, r.WithContext(flowtide.WithExecutionContext(r.Context(), tenant, actor)))
	})
}

// driveJobs 由宿主驱动到期作业，调度频率与生命周期均由宿主掌握。
// @param ctx 宿主生命周期上下文
// @param app 嵌入运行时
// @param interval 扫描间隔
// @return 无
func driveJobs(ctx context.Context, app *flowtide.Embedded, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			executed, err := app.RunDueJobs(ctx)
			if err != nil {
				slog.Warn("到期作业执行失败", "error", err)
				continue
			}
			if executed > 0 {
				slog.Info("到期作业已执行", "count", executed)
			}
		}
	}
}

// boolText 把布尔值转为 JSON 字面量，避免示例引入额外依赖。
// @param value 布尔值
// @return JSON 字面量
func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
