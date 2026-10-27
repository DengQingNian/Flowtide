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

	server := &http.Server{
		Addr:              *addr,
		Handler:           rest.New(app).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("Flowable Go 服务启动", "addr", *addr)
		if listenErr := server.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常退出", "error", listenErr)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("收到退出信号，开始优雅停机")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("优雅停机失败", "error", err)
	}
}
