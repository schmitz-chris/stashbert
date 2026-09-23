// Package config reads and validates the StashBert configuration from
// environment variables (architecture.md, 9.2).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
)

// Config holds the validated configuration. The comments name the variables.
type Config struct {
	Port       int        // PORT
	DataDir    string     // DATA_DIR
	OFFContact string     // OFF_CONTACT
	BackupKeep int        // BACKUP_KEEP
	LogLevel   slog.Level // LOG_LEVEL
}

// Load reads the configuration through getenv, usually os.Getenv.
// An empty value means the variable is unset and its default applies.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:       8080,
		DataDir:    "/data",
		BackupKeep: 14,
		LogLevel:   slog.LevelInfo,
	}

	var errs []error
	set := func(key string, parse func(v string) error) {
		if v := getenv(key); v != "" {
			if err := parse(v); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", key, err))
			}
		}
	}
	set("PORT", func(v string) (err error) { cfg.Port, err = intInRange(v, 1, 65535); return err })
	set("DATA_DIR", func(v string) error { cfg.DataDir = v; return nil })
	set("OFF_CONTACT", func(v string) error { cfg.OFFContact = v; return nil })
	set("BACKUP_KEEP", func(v string) (err error) { cfg.BackupKeep, err = intInRange(v, 1, 365); return err })
	set("LOG_LEVEL", func(v string) (err error) { cfg.LogLevel, err = parseLogLevel(v); return err })

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func intInRange(v string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%q is not an integer between %d and %d", v, lo, hi)
	}
	return n, nil
}

func parseLogLevel(v string) (slog.Level, error) {
	switch v {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("%q is not one of debug, info, warn, error", v)
}
