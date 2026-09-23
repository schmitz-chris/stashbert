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
