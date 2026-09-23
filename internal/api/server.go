package api

// ServerDeps holds the dependencies of the API handlers.
type ServerDeps struct {
	Version string
}

// Server implements StrictServerInterface.
type Server struct {
	deps ServerDeps
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns a Server using d.
func NewServer(d ServerDeps) *Server {
	return &Server{deps: d}
}
