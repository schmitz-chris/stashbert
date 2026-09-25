// Command stashbert is the StashBert server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/outbox"
	"github.com/schmitz-chris/stashbert/internal/store"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	done, err := parseFlags(context.Background(), os.Args[1:], version, os.Getenv, os.Stdout, os.Stderr)
	if err != nil {
		os.Exit(exitCode(err))
	}
	if done {
		return
	}
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).Error("stashbert stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// errUnhealthy marks a failed -healthcheck.
var errUnhealthy = errors.New("unhealthy")

// parseFlags parses the command line arguments without the program name.
// It reports done when the program should end without starting the server:
// after -version has written the version to stdout, or after -healthcheck
// has asked the running server (healthcheck). A failed check returns an
// error that wraps errUnhealthy. Parse errors, the usage and the reason of a
// failed check go to stderr.
func parseFlags(ctx context.Context, args []string, version string, getenv func(string) string, stdout, stderr io.Writer) (done bool, err error) {
	fs := flag.NewFlagSet("stashbert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	checkHealth := fs.Bool("healthcheck", false, "check GET /api/v1/health on 127.0.0.1:$PORT and exit with 0 on 200, otherwise with 1")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	if *showVersion {
		if _, err := fmt.Fprintln(stdout, version); err != nil {
			return false, fmt.Errorf("print version: %w", err)
		}
		return true, nil
	}
	if *checkHealth {
		if err := healthcheck(ctx, getenv); err != nil {
			fmt.Fprintf(stderr, "healthcheck: %v\n", err)
			return true, fmt.Errorf("%w: %w", errUnhealthy, err)
		}
		return true, nil
	}
	return false, nil
}

// exitCode returns the exit code for an error of parseFlags. It matches
// flag.ExitOnError (0 for -h, otherwise 2), except for a failed
// -healthcheck, which ends with 1.
func exitCode(err error) int {
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUnhealthy):
		return 1
	}
	return 2
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
	dbPath := filepath.Join(cfg.DataDir, "stashbert.db")
	db, err := app.OpenAndMigrate(ctx, dbPath, backupDir, store.Migrations, time.Now())
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
	// Loads product images every 60 s into DATA_DIR/images
	// (architecture.md, 7.3).
	imageDir := filepath.Join(cfg.DataDir, "images")
	// The image servers of Open Food Facts sometimes need more than the
	// default 10 s for the TLS handshake. The fetcher still limits each image
	// to 30 s in total.
	imageTransport := http.DefaultTransport.(*http.Transport).Clone()
	imageTransport.TLSHandshakeTimeout = 30 * time.Second
	images := lookup.NewImageFetcher(db, &http.Client{Transport: imageTransport}, imageDir, lookup.DefaultImageHosts, logger)

	// Without MQTT_URL, StashBert runs without MQTT and without outbox, and
	// the events go to events.Nop (architecture.md, 11). With it, the events
	// go into the outbox, and the deliverer publishes them after every
	// connection and every new event (architecture.md, 11.4). The
	// snapshotter writes shopping.snapshot into the outbox on requests from
	// <p>/in/snapshot and POST /shopping-list/snapshot (architecture.md, 11.3).
	// The summary goes out retained after every connection, 1 s after the
	// last of several events or successful changing API requests in quick
	// succession, which the writer and the handler report, and every 5 min
	// (architecture.md, 11.5). The discovery for Home Assistant goes out after
	// every connection, before the summary, and answers the birth message of
	// Home Assistant (architecture.md, 11.6). The target lists offered on
	// <p>/in/targets stay in memory for GET /integrations/mqtt and
	// PUT /integrations/mqtt/target (architecture.md, 11.7).
	var (
		mqttClient  *mqtt.Client
		deliverer   *outbox.Deliverer
		snapshotter *outbox.Snapshotter
		summary     *outbox.SummaryPublisher
		discovery   *outbox.Discovery
		targets     *outbox.Targets
		publisher   events.Publisher = events.Nop{}
	)
	if cfg.MQTT.URL != "" {
		if mqttClient, err = mqtt.New(cfg.MQTT, logger); err != nil {
			return fmt.Errorf("create mqtt client: %w", err)
		}
		deliverer = outbox.NewDeliverer(db, mqttClient, cfg.MQTT.TopicPrefix, logger)
		summary = outbox.NewSummaryPublisher(db, mqttClient, cfg.MQTT.TopicPrefix, logger)
		if discovery, err = outbox.NewDiscovery(mqttClient, cfg.MQTT, version, summary.Publish, logger); err != nil {
			return fmt.Errorf("create discovery: %w", err)
		}
		mqttClient.OnConnect(func(context.Context) { deliverer.Wake() })
		mqttClient.OnConnect(discovery.Publish)
		mqttClient.OnConnect(summary.Publish)
		mqttClient.OnMessage(discovery.Receive)
		writer := outbox.NewWriter(db, deliverer.Wake, summary.Request, logger)
		publisher = writer
		snapshotter = outbox.NewSnapshotter(writer, cfg.MQTT.TopicPrefix, logger)
		mqttClient.OnMessage(snapshotter.Receive)
		targets = outbox.NewTargets(writer, mqttClient, cfg.MQTT.TopicPrefix, logger)
		mqttClient.OnMessage(targets.Receive)
	}

	deps := app.Deps{
		Logger: logger, Version: version, DB: db, Publisher: publisher, Lookuper: off, ImageDir: imageDir,
		DBPath: dbPath, BackupDir: backupDir,
	}
	// Without MQTT, Snapshots and MQTT stay nil interfaces, not ones holding
	// a nil pointer, and OnChange stays nil.
	if snapshotter != nil {
		deps.Snapshots = snapshotter
		deps.MQTT = targets
		deps.OnChange = summary.Request
	}
	handler, err := app.NewHandler(cfg, deps)
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
	// The background jobs start only after the port is open. A service
	// that fails to listen (port in use) and is restarted by systemd would
	// otherwise write a new backup on every attempt and push the older
	// daily backups out (BACKUP_KEEP).
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
	jobs.Go(func() { images.Start(ctx, 60*time.Second) })
	// Writes a backup at the start and then every 24 h into DATA_DIR/backups
	// and keeps the newest BACKUP_KEEP (architecture.md, 9.3).
	jobs.Go(func() { backup.Start(ctx, db, backupDir, cfg.BackupKeep, 24*time.Hour, logger) })

	// With MQTT_URL, the client keeps the connection to the broker
	// (architecture.md, 11.1). It has its own context, which ends only on
	// return, after the HTTP server has shut down; the deferred call waits
	// until the client has published offline and disconnected.
	if mqttClient != nil {
		mqttCtx, stopMQTT := context.WithCancel(context.Background())
		mqttDone := make(chan struct{})
		go func() {
			defer close(mqttDone)
			if err := mqttClient.Run(mqttCtx); err != nil {
				logger.LogAttrs(mqttCtx, slog.LevelError, "mqtt stopped", slog.String("error", err.Error()))
			}
		}()
		defer func() {
			stopMQTT()
			<-mqttDone
		}()

		// The deliverer, the snapshotter, the summary publisher and the
		// discovery also have their own context. Their deferred call runs
		// before the one of the client, so they stop after the HTTP server
		// and before the client publishes offline; undelivered events stay in
		// the outbox for the next start.
		outboxCtx, stopOutbox := context.WithCancel(context.Background())
		var outboxJobs sync.WaitGroup
		outboxJobs.Go(func() { deliverer.Run(outboxCtx, outbox.MinBackoff, outbox.MaxBackoff) })
		outboxJobs.Go(func() { snapshotter.Run(outboxCtx, outbox.SnapshotWindow) })
		outboxJobs.Go(func() { summary.Run(outboxCtx, outbox.SummaryDelay, outbox.SummaryInterval) })
		outboxJobs.Go(func() { discovery.Run(outboxCtx, outbox.MinBirthDelay, outbox.MaxBirthDelay) })
		defer func() {
			stopOutbox()
			outboxJobs.Wait()
		}()
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
