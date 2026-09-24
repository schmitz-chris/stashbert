package api

import (
	"context"
	"database/sql"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/mqtt"
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
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns a Server using d.
func NewServer(d ServerDeps) *Server {
	return &Server{deps: d, queries: db.New(d.DB)}
}
