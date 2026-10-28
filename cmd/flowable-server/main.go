// Command flowable-server 启动 Flowable Go REST 服务。
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flowable-go/flowable/internal/engine"
	"github.com/flowable-go/flowable/internal/rest"
)

// main 解析参数并启动服务。
// @return 无
func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	historyLevel := flag.String("history", engine.HistoryAudit, "历史级别")
	tenantID := flag.String("tenant", "", "默认租户")
	statePath := flag.String("state", "", "状态文件路径，为空时仅使用内存")
	storage := flag.String("storage", "", "状态存储：mysql 快照或 mysql-rows 行仓储；为空时使用原有内存或文件模式")
	envPath := flag.String("env", ".env", "MySQL 开发配置文件路径")
	jobsInterval := flag.Duration("jobs-interval", time.Second, "定时器扫描间隔，0 表示关闭后台扫描")
	basicAuth := flag.String("basic-auth", "", "Basic Auth 用户，格式 user:pass")
	tlsCert := flag.String("tls-cert", "", "TLS 证书路径")
	tlsKey := flag.String("tls-key", "", "TLS 私钥路径")
	tlsClientCA := flag.String("tls-client-ca", "", "客户端 CA 证书路径（PEM，可含多个证书），设置后要求并校验客户端证书")
	tlsClientAuth := flag.String("tls-client-auth", "none", "客户端证书认证模式：none/request/require-any/require-verify")
	tlsMinVersion := flag.String("tls-min-version", "1.2", "TLS 最低版本：1.2 或 1.3")
	tlsClientIdentity := flag.Bool("tls-client-identity", false, "使用已校验的客户端证书建立 REST 身份，要求 -tls-client-auth=require-verify")
	rateLimit := flag.Float64("rate-limit", 0, "每秒请求数，0 表示关闭限流")
	rateBurst := flag.Int("rate-burst", 0, "限流桶容量")
	bearerToken := flag.String("bearer-token", "", "Bearer Token")
	flag.Parse()

	initCtx, cancelInit := context.WithTimeout(context.Background(), 30*time.Second)
	setup, err := newStorage(initCtx, *storage, *statePath, *envPath)
	cancelInit()
	if err != nil {
		slog.Error("初始化状态存储失败", "error", err)
		os.Exit(1)
	}
	// defer 的入栈顺序保证先关闭引擎并持久化行差异，再释放独占锁和连接池。
	defer func() {
		if err := setup.Close(); err != nil {
			slog.Error("释放存储资源失败", "error", err)
		}
	}()
	app, err := engine.New(engine.Config{
		HistoryLevel:  *historyLevel,
		TenantID:      *tenantID,
		StatePath:     *statePath,
		StateStore:    setup.Store,
		RowRepository: setup.Repository,
		LoadRows:      setup.LoadRows,
	})
	if err != nil {
		slog.Error("创建引擎失败", "error", err)
		if closeErr := setup.Close(); closeErr != nil {
			slog.Error("释放存储资源失败", "error", closeErr)
		}
		os.Exit(1)
	}
	defer func() {
		// 先保存引擎状态，再由外层清理函数释放数据库连接池。
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelClose()
		if err := app.Close(closeCtx); err != nil {
			slog.Error("关闭引擎时保存状态失败", "error", err)
		}
	}()

	jobCtx, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	if *jobsInterval > 0 {
		go func() {
			ticker := time.NewTicker(*jobsInterval)
			defer ticker.Stop()
			for {
				select {
				case <-jobCtx.Done():
					return
				case <-ticker.C:
					if executed, err := app.RunDueJobs(jobCtx); err != nil {
						slog.Error("执行定时器作业失败", "error", err)
					} else if executed > 0 {
						slog.Info("执行定时器作业", "count", executed)
					}
				}
			}
		}()
	}

	options := make([]rest.Option, 0)
	if *basicAuth != "" {
		parts := strings.SplitN(*basicAuth, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			slog.Error("非法 Basic Auth 配置，应为 user:pass")
			os.Exit(1)
		}
		options = append(options, rest.WithBasicAuth(map[string]string{parts[0]: parts[1]}))
	}
	if *bearerToken != "" {
		options = append(options, rest.WithBearerToken(*bearerToken))
	}
	if *rateLimit > 0 && *rateBurst > 0 {
		options = append(options, rest.WithRateLimit(*rateLimit, *rateBurst))
	}
	tlsConfig, err := buildTLSConfig(*tlsCert, *tlsKey, *tlsClientCA, *tlsClientAuth, *tlsMinVersion)
	if err != nil {
		slog.Error("TLS 配置非法", "error", err)
		os.Exit(1)
	}
	// 证书身份映射只接受 TLS 层已验证的证书链，避免误配后所有请求 401 或使用未验证身份
	if *tlsClientIdentity {
		if err := validateClientCertIdentity(*tlsCert, *tlsClientCA, *tlsClientAuth); err != nil {
			slog.Error("TLS 客户端身份配置非法", "error", err)
			os.Exit(1)
		}
		options = append(options, rest.WithClientCertificateIdentity())
	}
	server := &http.Server{
		Addr:              *addr,
		Handler:           rest.New(app, options...).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// 仅在启用 TLS 时注入配置，未启用时保持原有明文监听行为。
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig
	}

	go func() {
		slog.Info("Flowable Go 服务启动", "addr", *addr)
		var listenErr error
		if *tlsCert != "" {
			listenErr = server.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			listenErr = server.ListenAndServe()
		}
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常退出", "error", listenErr)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("收到退出信号，开始优雅停机")
	stopJobs()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("优雅停机失败", "error", err)
	}
}
