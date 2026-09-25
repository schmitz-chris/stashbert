package app

// NewHandlerWithRestoreLimit is NewHandler with limit as the largest body of
// POST /api/v1/backup/restore instead of 1 GB.
var NewHandlerWithRestoreLimit = newHandler
