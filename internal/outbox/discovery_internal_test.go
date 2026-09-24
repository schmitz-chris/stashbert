package outbox

import (
	"testing"
	"time"
)

// The delay after the birth message lies between 1 and 5 s and varies.
func TestRandomDelay(t *testing.T) {
	lowest, highest := MaxBirthDelay, MinBirthDelay
	for range 1000 {
		d := randomDelay(MinBirthDelay, MaxBirthDelay)
		if d < time.Second || d > 5*time.Second {
			t.Fatalf("randomDelay = %v, want between 1 s and 5 s", d)
		}
		lowest, highest = min(lowest, d), max(highest, d)
	}
	if lowest > 2*time.Second || highest < 4*time.Second {
		t.Errorf("randomDelay between %v and %v in 1000 draws, want a spread over 1 s to 5 s", lowest, highest)
	}
	if d := randomDelay(time.Second, time.Second); d != time.Second {
		t.Errorf("randomDelay(1s, 1s) = %v, want 1s", d)
	}
	if d := randomDelay(0, 0); d != 0 {
		t.Errorf("randomDelay(0, 0) = %v, want 0", d)
	}
}
