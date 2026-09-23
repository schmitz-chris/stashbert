package api_test

import (
	"context"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/api"
)

// The generated strict server interface must offer GetHealth with this signature.
var _ func(api.StrictServerInterface, context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) = api.StrictServerInterface.GetHealth

func TestGetSwaggerLoadsEmbeddedSpec(t *testing.T) {
	spec, err := api.GetSwagger()
	if err != nil {
		t.Fatalf("GetSwagger: %v", err)
	}

	if want := "3.1.0"; spec.OpenAPI != want {
		t.Errorf("openapi = %q, want %q", spec.OpenAPI, want)
	}

	if item := spec.Paths.Value("/health"); item == nil || item.Get == nil {
		t.Error("GET /health missing in embedded spec")
	}
}
