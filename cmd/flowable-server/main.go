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
	jobsInterval := flag.Duration("jobs-interval", time.Second, "定时器扫描间隔，0 表示关闭后台扫描")
	basicAuth := flag.String("basic-auth", "", "Basic Auth 用户，格式 user:pass")
	tlsCert := flag.String("tls-cert", "", "TLS 证书路径")
	tlsKey := flag.String("tls-key", "", "TLS 私钥路径")
	rateLimit := flag.Float64("rate-limit", 0, "每秒请求数，0 表示关闭限流")
	rateBurst := flag.Int("rate-burst", 0, "限流桶容量")
	bearerToken := flag.String("bearer-token", "", "Bearer Token")
	flag.Parse()

	app, err := engine.New(engine.Config{HistoryLevel: *historyLevel, TenantID: *tenantID, StatePath: *statePath})
	if err != nil {
		slog.Error("创建引擎失败", "error", err)
		os.Exit(1)
	}
	defer func() {
		// 关闭引擎，释放内部状态
		_ = app.Close(context.Background())
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
	if (*tlsCert == "") != (*tlsKey == "") {
		slog.Error("TLS 证书与私钥必须同时配置")
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              *addr,
		Handler:           rest.New(app, options...).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
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
