package api

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
	"github.com/schmitz-chris/stashbert/internal/recognize"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// ServerDeps holds the dependencies of the API handlers.
type ServerDeps struct {
	Version   string
	DB        *sql.DB
	Publisher events.Publisher
	// Lookuper looks up unknown barcodes booked with add or marked for shopping.
	Lookuper domain.Lookuper
	// ImageDir is the directory of the product image files.
	ImageDir string
	// Snapshots takes the requests for shopping.snapshot; nil without MQTT.
	Snapshots SnapshotRequester
	// MQTT reports the connection and chooses the target list; nil without
	// MQTT.
	MQTT MQTTIntegration
	// DBPath is the path of the database file, DATA_DIR/stashbert.db.
	DBPath string
	// BackupDir is the directory of the regular backups, DATA_DIR/backups.
	BackupDir string
	// BackupKeep is the number of kept daily backups (BACKUP_KEEP).
	BackupKeep int
	// OpenFoodFacts reports whether OFF_CONTACT is set.
	OpenFoodFacts bool
	// Logger logs what cannot be reported to the client, such as a failed
	// cleanup after a download.
	Logger *slog.Logger
	// DataDir is DATA_DIR, where POST /backup/restore lays an uploaded
	// backup ready in restore/ (ADR-0020).
	DataDir string
	// Restart asks for a restart in the same process after a backup was laid
	// ready, which applies it (ADR-0020). It must not block. nil means no
	// restart.
	Restart func()
	// Recognizer asks the provider of the product recognition (ADR-0021).
	Recognizer *recognize.Client
}

// SnapshotRequester takes requests for shopping.snapshot over MQTT
// (architecture.md, 11.3). *outbox.Snapshotter implements it.
type SnapshotRequester interface {
	// Request asks for a snapshot without blocking.
	Request()
}

// MQTTIntegration is the connection to the broker with the target lists
// that Home Assistant offers (architecture.md, 11.7). *outbox.Targets
// implements it.
type MQTTIntegration interface {
	// State reports the state of the connection.
	State() mqtt.State
	// Offer returns the offered target lists in their order.
	Offer() []domain.ShoppingTarget
	// SetTarget stores target as the chosen target list, or removes the
	// choice if target is nil, and writes the snapshots of the switch.
	SetTarget(ctx context.Context, target *domain.ShoppingTarget) error
}

// Server implements StrictServerInterface.
type Server struct {
	deps    ServerDeps
	queries *db.Queries
	// downloads holds a token while a backup download runs, so that
	// downloads run one after another.
	downloads chan struct{}
	// restores holds a token while an uploaded backup is checked, so that
	// only one runs at a time.
	restores chan struct{}
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns a Server using d.
func NewServer(d ServerDeps) *Server {
	return &Server{deps: d, queries: db.New(d.DB), downloads: make(chan struct{}, 1), restores: make(chan struct{}, 1)}
}
