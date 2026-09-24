package domain_test

import (
	"fmt"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/domain"
)

func TestMissing(t *testing.T) {
	tests := []struct {
		stock, target int64
		minStock      *int64
		want          int64
	}{
		{2, 5, nil, 3},
		{6, 5, nil, 0},
		{0, 4, nil, 4},
		{2, 4, new(int64(1)), 0},
		{0, 4, new(int64(1)), 4},
		{1, 4, new(int64(1)), 0},
		{0, 0, nil, 0},
		{5, 5, nil, 0},
	}
	for _, tt := range tests {
		minStock := "nil"
		if tt.minStock != nil {
			minStock = fmt.Sprint(*tt.minStock)
		}
		if got := domain.Missing(tt.stock, tt.target, tt.minStock); got != tt.want {
			t.Errorf("Missing(%d, %d, %s) = %d, want %d", tt.stock, tt.target, minStock, got, tt.want)
		}
	}
}

func TestShoppingQuantity(t *testing.T) {
	tests := []struct {
		name      string
		missing   int64
		crateSize *int64
		quantity  int64
		unit      string
	}{
		{"without crate size", 3, nil, 3, "piece"},
		{"without crate size, nothing missing", 0, nil, 0, "piece"},
		{"crate size, rounded up", 17, new(int64(20)), 1, "crate"},
		{"crate size, one bottle over a crate", 21, new(int64(20)), 2, "crate"},
		{"crate size, exactly two crates", 40, new(int64(20)), 2, "crate"},
		{"crate size, one bottle", 1, new(int64(24)), 1, "crate"},
		{"crate size, nothing missing", 0, new(int64(20)), 0, "crate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quantity, unit := domain.ShoppingQuantity(tt.missing, tt.crateSize)
			if quantity != tt.quantity || unit != tt.unit {
				t.Errorf("ShoppingQuantity(%d, %v) = %d, %q, want %d, %q",
					tt.missing, tt.crateSize, quantity, unit, tt.quantity, tt.unit)
			}
		})
	}
	if domain.UnitPiece != "piece" || domain.UnitCrate != "crate" {
		t.Errorf("units = %q, %q, want piece, crate", domain.UnitPiece, domain.UnitCrate)
	}
}
