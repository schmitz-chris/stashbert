package api_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oapi-codegen/nullable"

	"github.com/schmitz-chris/stashbert/internal/api"
)

// The generated strict server interface must offer GetHealth with this signature.
var _ func(api.StrictServerInterface, context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) = api.StrictServerInterface.GetHealth

func TestGetSpecLoadsEmbeddedSpec(t *testing.T) {
	spec, err := api.GetSpec()
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}

	if want := "3.1.0"; spec.OpenAPI != want {
		t.Errorf("openapi = %q, want %q", spec.OpenAPI, want)
	}

	if item := spec.Paths.Value("/health"); item == nil || item.Get == nil {
		t.Error("GET /health missing in embedded spec")
	}
}

func TestProductPatchUsesNullable(t *testing.T) {
	// The struct literal only compiles if the nullable fields are nullable.Nullable.
	patch := api.ProductPatch{
		Brand:       nullable.Nullable[string]{},
		PackageSize: nullable.Nullable[string]{},
		MinStock:    nullable.Nullable[int]{},
		Note:        nullable.Nullable[string]{},
	}

	if err := json.Unmarshal([]byte(`{"brand": null, "min_stock": 2}`), &patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	if !patch.Brand.IsNull() {
		t.Errorf("brand: IsNull = false, want true")
	}
	if got, err := patch.MinStock.Get(); err != nil || got != 2 {
		t.Errorf("min_stock = %d, %v, want 2", got, err)
	}
	if patch.PackageSize.IsSpecified() || patch.Note.IsSpecified() {
		t.Errorf("package_size and note: IsSpecified = true, want false")
	}
}
