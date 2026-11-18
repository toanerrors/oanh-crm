package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"oanh.local/accounting/internal/platform/httpserver"
	"oanh.local/accounting/internal/platform/sqlite"
)

func main() {
	data := flag.String("data", "../data", "Thư mục dữ liệu trên ổ cục bộ")
	address := flag.String("addr", "127.0.0.1:8080", "Địa chỉ HTTP")
	assets := flag.String("web", "../web/dist", "Thư mục frontend đã build")
	secure := flag.Bool("secure-cookie", false, "Bật cookie Secure khi phục vụ qua HTTPS")
	resetKeepUsers := flag.String("reset-keep-users", "", "Tạo bản sao dự phòng rồi đặt lại dữ liệu ban đầu, chỉ giữ người dùng (nhập lý do)")
	resetKeepPolicies := flag.Bool("reset-keep-policies", true, "Giữ cấu hình chính sách nghiệp vụ khi chạy -reset-keep-users")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if strings.TrimSpace(*resetKeepUsers) != "" {
		db, err := sqlite.Open(ctx, *data)
		if err != nil {
			slog.Error("cannot open dataset", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		manifest, err := db.ResetKeepingUsers(ctx, sqlite.ResetOptions{
			Reason:       strings.TrimSpace(*resetKeepUsers),
			KeepPolicies: *resetKeepPolicies,
			KeepSessions: true,
		})
		if err != nil {
			slog.Error("cannot reset dataset", "error", err)
			os.Exit(1)
		}
		slog.Info("dataset reset completed", "company_id", manifest.CompanyID, "epoch", manifest.Epoch, "documents", manifest.Documents, "posted", manifest.Posted)
		return
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		slog.Error("cannot listen", "error", err)
		os.Exit(1)
	}
	db, err := sqlite.Open(ctx, *data)
	if err != nil {
		listener.Close()
		slog.Error("cannot open dataset", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	schedulerDone := make(chan struct{})
	go func() { defer close(schedulerDone); db.RunBackupScheduler(schedulerCtx) }()
	defer func() { cancelScheduler(); <-schedulerDone }()

	root, err := filepath.Abs(*assets)
	if err != nil {
		slog.Error("invalid assets directory")
		os.Exit(1)
	}
	server := &http.Server{Handler: httpserver.New(db, root, *secure), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			slog.Error("shutdown failed", "error", err)
		}
	}()
	slog.Info("server ready", "address", listener.Addr().String(), "sqlite", db.SQLiteVersion)
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
