package api

import "context"

// GetHealth reports that the server is running and its version.
func (s *Server) GetHealth(ctx context.Context, request GetHealthRequestObject) (GetHealthResponseObject, error) {
	return GetHealth200JSONResponse{Status: Ok, Version: s.deps.Version}, nil
}
