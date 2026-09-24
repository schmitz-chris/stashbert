// Package config reads and validates the StashBert configuration from
// environment variables (architecture.md, 9.2).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
)

// Config holds the validated configuration. The comments name the variables.
type Config struct {
	Port       int        // PORT
	DataDir    string     // DATA_DIR
	OFFContact string     // OFF_CONTACT
	BackupKeep int        // BACKUP_KEEP
	LogLevel   slog.Level // LOG_LEVEL
	MQTT       MQTT       // MQTT_*
}

// MQTT holds the connection to the MQTT broker (architecture.md, 9.2 and 11).
// An empty URL means StashBert runs without MQTT.
type MQTT struct {
	URL         string // MQTT_URL
	Username    string // MQTT_USERNAME
	Password    string // MQTT_PASSWORD, never logged
	ClientID    string // MQTT_CLIENT_ID
	TopicPrefix string // MQTT_TOPIC_PREFIX
	HADiscovery bool   // MQTT_HA_DISCOVERY
	HAPrefix    string // MQTT_HA_PREFIX
}

var (
	// topicPrefix is also used for entity IDs in Home Assistant.
	topicPrefix = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	haPrefix    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// Load reads the configuration through getenv, usually os.Getenv.
// An empty value means the variable is unset and its default applies.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:       8080,
		DataDir:    "/data",
		BackupKeep: 14,
		LogLevel:   slog.LevelInfo,
		MQTT: MQTT{
			ClientID:    "stashbert",
			TopicPrefix: "stashbert",
			HADiscovery: true,
			HAPrefix:    "homeassistant",
		},
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
	set("MQTT_URL", func(v string) error { cfg.MQTT.URL = v; return checkMQTTURL(v) })
	set("MQTT_USERNAME", func(v string) error { cfg.MQTT.Username = v; return nil })
	set("MQTT_PASSWORD", func(v string) error { cfg.MQTT.Password = v; return nil })
	set("MQTT_CLIENT_ID", func(v string) error { cfg.MQTT.ClientID = v; return nil })
	set("MQTT_TOPIC_PREFIX", func(v string) error {
		cfg.MQTT.TopicPrefix = v
		return matches(v, topicPrefix, "1 to 64 characters from a to z, 0 to 9 and _")
	})
	set("MQTT_HA_DISCOVERY", func(v string) (err error) { cfg.MQTT.HADiscovery, err = parseBool(v); return err })
	set("MQTT_HA_PREFIX", func(v string) error {
		cfg.MQTT.HAPrefix = v
		return matches(v, haPrefix, "1 to 64 characters from letters, digits, _ and -")
	})

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

func parseBool(v string) (bool, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%q is not one of true, false", v)
}

func matches(v string, re *regexp.Regexp, rule string) error {
	if !re.MatchString(v) {
		return fmt.Errorf("%q does not have %s", v, rule)
	}
	return nil
}

// checkMQTTURL accepts only mqtt://host:port and mqtts://host:port. The
// errors never contain the value: a URL with credentials would otherwise put
// the password into the log (architecture.md, 11.1).
func checkMQTTURL(v string) error {
	u, err := url.Parse(v)
	if err != nil {
		return errors.New("is not a URL of the form mqtt://host:port or mqtts://host:port")
	}
	switch {
	case u.Scheme != "mqtt" && u.Scheme != "mqtts":
		return errors.New("the scheme is not mqtt or mqtts")
	case u.User != nil:
		return errors.New("contains credentials, use MQTT_USERNAME and MQTT_PASSWORD")
	case u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "":
		return errors.New("contains more than scheme, host and port")
	case u.Hostname() == "":
		return errors.New("the host is missing")
	}
	if n, err := strconv.Atoi(u.Port()); err != nil || n < 1 || n > 65535 {
		return errors.New("the port is missing or not between 1 and 65535")
	}
	return nil
}
