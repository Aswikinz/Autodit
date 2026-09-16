package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Aswikinz/Autodit/internal/analysis"
	"github.com/Aswikinz/Autodit/internal/api"
	"github.com/Aswikinz/Autodit/internal/auth"
	"github.com/Aswikinz/Autodit/internal/ingest"
	"github.com/Aswikinz/Autodit/internal/platform"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/Aswikinz/Autodit/internal/storage"
	"github.com/Aswikinz/Autodit/internal/storage/objectstore"
)

func main() {
	if analysis.WorkerMain() {
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if e := run(logger); e != nil {
		logger.Error("application stopped", "error_code", "startup_or_runtime_failure")
		os.Exit(1)
	}
}
func run(logger *slog.Logger) error {
	if len(os.Args) < 2 {
		return errors.New("subcommand required: api, worker, migrate, bootstrap, healthcheck")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 3 * time.Second}
		resp, e := client.Get("http://127.0.0.1:8080/healthz")
		if e != nil {
			return e
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New("not ready")
		}
		return nil
	}
	cfg, e := platform.Load()
	if e != nil {
		return e
	}
	if os.Args[1] == "migrate" {
		return storage.Migrate(ctx, cfg.DatabaseURL)
	}
	store, e := storage.Open(ctx, cfg.DatabaseURL)
	if e != nil {
		return e
	}
	defer store.Close()
	switch os.Args[1] {
	case "bootstrap":
		models, e := rules.LoadPack(cfg.Rulepack)
		if e != nil {
			return e
		}
		if cfg.AuthMode == "local" {
			e = store.BootstrapLocal(ctx, cfg.TenantID, cfg.TenantName, models)
		} else {
			e = store.Bootstrap(ctx, cfg.TenantID, cfg.TenantName, models)
		}
		if e != nil {
			return e
		}
		if cfg.AuthMode == "local" {
			tenant, e := store.ForTenant(cfg.TenantID)
			if e != nil {
				return e
			}
			return tenant.BootstrapAdmin(ctx, cfg.AdminPassword)
		}
		return nil
	case "worker":
		tenant, e := store.ForTenant(cfg.TenantID)
		if e != nil {
			return e
		}
		logger.Info("worker ready")
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if inbox := os.Getenv("AUTODIT_INBOX_DIR"); inbox != "" {
				landed, err := ingest.Inbox(ctx, inbox)
				if err != nil {
					logger.Warn("inbox inspection failed", "error_code", "invalid_extract")
				}
				for _, file := range landed {
					if _, err = tenant.SubmitInbox(ctx, file.Population, file.Receipt); err != nil {
						logger.Warn("inbox submission failed", "error_code", "source_or_contract_invalid")
					}
				}
			}
			worked, e := tenant.ProcessNext(ctx, objectstore.Files{Root: cfg.SnapshotDir}, time.Now)
			if ctx.Err() != nil {
				return nil
			}
			if e != nil {
				logger.Warn("run halted", "error_code", "pipeline_failure")
			}
			if worked {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	case "api":
		manager, e := auth.New(ctx, cfg)
		if e != nil {
			return e
		}
		webDir := os.Getenv("AUTODIT_WEB_DIR")
		if webDir == "" {
			webDir = "web/dist"
		}
		app := api.Server{Store: store, Auth: manager, Objects: objectstore.Files{Root: cfg.SnapshotDir}, WebDir: webDir, Now: time.Now}
		server := http.Server{Addr: cfg.Listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		done := make(chan error, 1)
		go func() { done <- server.ListenAndServe() }()
		logger.Info("API ready")
		select {
		case e := <-done:
			if errors.Is(e, http.ErrServerClosed) {
				return nil
			}
			return e
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		}
	default:
		return errors.New("unknown subcommand")
	}
}
