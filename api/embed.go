// Package apispec embeds the OpenAPI specification of the StashBert API.
package apispec

import _ "embed"

// OpenAPI is the content of api/openapi.yaml. internal/app serves it under
// GET /api/v1/openapi.yaml (architecture.md, 6.2). Callers must not modify it.
//
//go:embed openapi.yaml
var OpenAPI []byte
