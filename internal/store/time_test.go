package store_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/schmitz-chris/stashbert/internal/store"
)

var storedTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

func TestFormatTime(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60)
	tests := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC), "2026-09-23T12:00:00.000Z"},
		{time.Date(2026, 9, 23, 14, 5, 6, 7_000_000, cest), "2026-09-23T12:05:06.007Z"},
		{time.Date(2026, 1, 2, 3, 4, 5, 123_999_999, time.UTC), "2026-01-02T03:04:05.123Z"},
		{time.Date(2026, 12, 31, 23, 59, 59, 990_000_000, time.UTC), "2026-12-31T23:59:59.990Z"},
	}
	for _, tt := range tests {
		got := store.FormatTime(tt.in)
		if got != tt.want {
			t.Errorf("FormatTime(%v) = %q, want %q", tt.in, got, tt.want)
		}
		if !storedTime.MatchString(got) {
			t.Errorf("FormatTime(%v) = %q, want 3 fractional digits and Z", tt.in, got)
		}

		parsed, err := store.ParseTime(got)
		if err != nil {
			t.Errorf("ParseTime(%q): %v", got, err)
			continue
		}
		if want := tt.in.Truncate(time.Millisecond); !parsed.Equal(want) {
			t.Errorf("ParseTime(FormatTime(%v)) = %v, want %v", tt.in, parsed, want)
		}
		if parsed.Location() != time.UTC {
			t.Errorf("ParseTime(%q) location = %v, want UTC", got, parsed.Location())
		}
		if again := store.FormatTime(parsed); again != got {
			t.Errorf("FormatTime(ParseTime(%q)) = %q", got, again)
		}
	}
}

func TestParseTimeInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"2026-09-23T12:00:00Z",
		"2026-09-23T12:00:00.1Z",
		"2026-09-23T12:00:00.0000Z",
		"2026-09-23T12:00:00.000",
		"2026-09-23T12:00:00.000+02:00",
		"2026-09-23 12:00:00.000Z",
	} {
		if got, err := store.ParseTime(s); err == nil {
			t.Errorf("ParseTime(%q) = %v, want error", s, got)
		}
	}
}
