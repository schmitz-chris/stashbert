package store

import (
	"fmt"
	"time"
)

// TimeLayout is the storage format of all timestamps: RFC 3339 in UTC with
// exactly three fractional digits (architecture.md, 5).
const TimeLayout = "2006-01-02T15:04:05.000Z"

// FormatTime formats t in UTC using TimeLayout. Digits below milliseconds are truncated.
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeLayout)
}

// ParseTime parses a timestamp in TimeLayout. The result is in UTC.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(TimeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", s, err)
	}
	return t, nil
}
