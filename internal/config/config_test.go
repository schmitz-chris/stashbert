package config_test

import (
	"log/slog"
	"testing"

	"github.com/schmitz-chris/stashbert/internal/config"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{Port: 8080, DataDir: "/data", CookieSecure: true, BackupKeep: 14, LogLevel: slog.LevelInfo}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"PORT":          "65535",
		"DATA_DIR":      "./.data",
		"PUBLIC_URL":    "https://stash.example.com/",
		"COOKIE_SECURE": "false",
		"OFF_CONTACT":   "stash@example.com",
		"BACKUP_KEEP":   "365",
		"LOG_LEVEL":     "debug",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{
		Port:         65535,
		DataDir:      "./.data",
		PublicURL:    "https://stash.example.com/",
		CookieSecure: false,
		OFFContact:   "stash@example.com",
		BackupKeep:   365,
		LogLevel:     slog.LevelDebug,
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadInvalidValues(t *testing.T) {
	tests := []struct{ key, value string }{
		{"PORT", "0"},
		{"PORT", "65536"},
		{"PORT", "http"},
		{"BACKUP_KEEP", "0"},
		{"BACKUP_KEEP", "366"},
		{"BACKUP_KEEP", "many"},
		{"LOG_LEVEL", "trace"},
		{"LOG_LEVEL", "INFO"},
		{"COOKIE_SECURE", "yes"},
		{"PUBLIC_URL", "stash.example.com"},
		{"PUBLIC_URL", "ftp://stash.example.com"},
		{"PUBLIC_URL", "https://"},
		{"PUBLIC_URL", "https://stash.example.com/app"},
		{"PUBLIC_URL", "https://stash example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			if _, err := config.Load(env(map[string]string{tt.key: tt.value})); err == nil {
				t.Errorf("Load with %s=%q: got nil error, want error", tt.key, tt.value)
			}
		})
	}
}
