package domain_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/httpx"
)

// targetsJSON returns a JSON array of n target lists with ids todo.l<i>.
func targetsJSON(n int) string {
	entries := make([]string, n)
	for i := range entries {
		entries[i] = fmt.Sprintf(`{"id": "todo.l%d", "name": "Liste %d"}`, i, i)
	}
	return "[" + strings.Join(entries, ",") + "]"
}

func TestParseShoppingTargets(t *testing.T) {
	// ä has two bytes but is one character.
	id255, name100 := "todo."+strings.Repeat("ä", 250), strings.Repeat("ä", 100)
	fifty := make([]domain.ShoppingTarget, 50)
	for i := range fifty {
		fifty[i] = domain.ShoppingTarget{ID: fmt.Sprintf("todo.l%d", i), Name: fmt.Sprintf("Liste %d", i)}
	}
	tests := []struct {
		name    string
		payload string
		want    []domain.ShoppingTarget
	}{
		{
			name:    "offer of Home Assistant",
			payload: `[{"id":"todo.stashbert_test","name":"StashBert Test"}]`,
			want:    []domain.ShoppingTarget{{ID: "todo.stashbert_test", Name: "StashBert Test"}},
		},
		{
			name:    "several in their order, trimmed, other fields ignored",
			payload: `[{"id": " todo.zuhause\t", "name": " Zuhause ", "icon": "mdi:cart"}, {"id": "todo.arbeit", "name": "Arbeit"}]`,
			want:    []domain.ShoppingTarget{{ID: "todo.zuhause", Name: "Zuhause"}, {ID: "todo.arbeit", Name: "Arbeit"}},
		},
		{name: "no lists", payload: ` [] `, want: []domain.ShoppingTarget{}},
		{name: "50 lists", payload: targetsJSON(50), want: fifty},
		{
			name:    "longest id and name in characters",
			payload: `[{"id": "` + id255 + `", "name": "` + name100 + `"}]`,
			want:    []domain.ShoppingTarget{{ID: id255, Name: name100}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseShoppingTargets([]byte(tt.payload))
			if err != nil {
				t.Fatalf("ParseShoppingTargets: %v", err)
			}
			if got == nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("targets = %#v\n      want %#v", got, tt.want)
			}
		})
	}
}

func TestParseShoppingTargetsInvalid(t *testing.T) {
	tests := []struct{ name, payload string }{
		{"empty payload", ""},
		{"no JSON", "todo.zuhause"},
		{"null", "null"},
		{"object instead of array", `{"id": "todo.zuhause", "name": "Zuhause"}`},
		{"array of strings", `["todo.zuhause"]`},
		{"null entry", `[null]`},
		{"id is a number", `[{"id": 1, "name": "Zuhause"}]`},
		{"trailing data", `[{"id": "todo.zuhause", "name": "Zuhause"}] []`},
		{"51 lists", targetsJSON(51)},
		{"id missing", `[{"name": "Zuhause"}]`},
		{"id empty after trimming", `[{"id": " ", "name": "Zuhause"}]`},
		{"id too long", `[{"id": "todo.` + strings.Repeat("ä", 251) + `", "name": "Zuhause"}]`},
		{"name missing", `[{"id": "todo.zuhause"}]`},
		{"name empty after trimming", `[{"id": "todo.zuhause", "name": "\t"}]`},
		{"name too long", `[{"id": "todo.zuhause", "name": "` + strings.Repeat("ä", 101) + `"}]`},
		{"one invalid of several", `[{"id": "todo.zuhause", "name": "Zuhause"}, {"id": "", "name": "Arbeit"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseShoppingTargets([]byte(tt.payload))
			if err == nil {
				t.Errorf("ParseShoppingTargets = %#v, want an error", got)
			}
		})
	}
}

func TestFindShoppingTarget(t *testing.T) {
	offer := []domain.ShoppingTarget{{ID: "todo.zuhause", Name: "Zuhause"}, {ID: "todo.arbeit", Name: "Arbeit"}}

	got, err := domain.FindShoppingTarget(offer, "todo.arbeit")
	if err != nil || got != offer[1] {
		t.Errorf("FindShoppingTarget = %#v, %v, want %#v", got, err, offer[1])
	}

	for _, id := range []string{"todo.einkauf", "", "todo.zuhause "} {
		_, err := domain.FindShoppingTarget(offer, id)
		var e *httpx.Error
		if !errors.As(err, &e) || e.Status != http.StatusUnprocessableEntity || e.Code != "unknown_target" {
			t.Errorf("FindShoppingTarget(%q) error = %v, want 422 unknown_target", id, err)
		}
	}
	if _, err := domain.FindShoppingTarget(nil, "todo.zuhause"); err == nil {
		t.Error("FindShoppingTarget without offer = nil error, want unknown_target")
	}
}
