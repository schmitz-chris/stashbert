// Command stashbert is the StashBert server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/schmitz-chris/stashbert/internal/app"
	"github.com/schmitz-chris/stashbert/internal/backup"
	"github.com/schmitz-chris/stashbert/internal/config"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("stashbert stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	// Before pending migrations of an existing database, a copy is written
	// to DATA_DIR/backups (architecture.md, 9.3).
	backupDir := filepath.Join(cfg.DataDir, "backups")
	db, err := app.OpenAndMigrate(ctx, filepath.Join(cfg.DataDir, "stashbert.db"), backupDir, store.Migrations, time.Now())
	if err != nil {
		return err
	}
	defer db.Close()

	// All requests to Open Food Facts share one limiter: one token every 6 s,
	// burst 1 (architecture.md, 7.3).
	offLimiter := rate.NewLimiter(rate.Every(6*time.Second), 1)
	off := lookup.NewDisabledClient()
	if cfg.OFFContact != "" {
		off = lookup.NewClient("https://world.openfoodfacts.org",
			"StashBert/"+version+" ("+cfg.OFFContact+")", &http.Client{}, offLimiter)
	}
	// The background jobs end with ctx. Every return waits for them before
	// the database is closed.
	var jobs sync.WaitGroup
	defer func() {
		stop()
		jobs.Wait()
	}()
	// Looks up pending products every 60 s with the same client and limiter
	// (architecture.md, 7.3).
	jobs.Go(func() { lookup.NewEnricher(db, off, logger).Start(ctx, 60*time.Second) })
	// Loads product images every 60 s into DATA_DIR/images
	// (architecture.md, 7.3).
	imageDir := filepath.Join(cfg.DataDir, "images")
	images := lookup.NewImageFetcher(db, &http.Client{}, imageDir, lookup.DefaultImageHosts, logger)
	jobs.Go(func() { images.Start(ctx, 60*time.Second) })
	// Writes a backup at the start and then every 24 h into DATA_DIR/backups
	// and keeps the newest BACKUP_KEEP (architecture.md, 9.3).
	jobs.Go(func() { backup.Start(ctx, db, backupDir, cfg.BackupKeep, 24*time.Hour, logger) })

	handler, err := app.NewHandler(cfg, app.Deps{
		Logger: logger, Version: version, DB: db, Publisher: events.Nop{}, Lookuper: off, ImageDir: imageDir,
	})
	if err != nil {
		return fmt.Errorf("build handler: %w", err)
	}

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	logger.Info("server started", slog.String("addr", srv.Addr), slog.String("version", version))

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}
	stop()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
