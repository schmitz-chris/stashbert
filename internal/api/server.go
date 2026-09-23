package api

import (
	"database/sql"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/events"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// ServerDeps holds the dependencies of the API handlers.
type ServerDeps struct {
	Version   string
	DB        *sql.DB
	Publisher events.Publisher
	// Lookuper looks up unknown barcodes booked with add.
	Lookuper domain.Lookuper
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
