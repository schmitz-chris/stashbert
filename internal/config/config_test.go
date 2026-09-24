package config_test

import (
	"log/slog"
	"strings"
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
	want := config.Config{
		Port: 8080, DataDir: "/data", BackupKeep: 14, LogLevel: slog.LevelInfo,
		MQTT: config.MQTT{ClientID: "stashbert", TopicPrefix: "stashbert", HADiscovery: true, HAPrefix: "homeassistant"},
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"PORT":              "65535",
		"DATA_DIR":          "./.data",
		"OFF_CONTACT":       "stash@example.com",
		"BACKUP_KEEP":       "365",
		"LOG_LEVEL":         "debug",
		"MQTT_URL":          "mqtts://broker.example:8883",
		"MQTT_USERNAME":     "stashbert",
		"MQTT_PASSWORD":     "s3cret pass:@/",
		"MQTT_CLIENT_ID":    "stashbert-test",
		"MQTT_TOPIC_PREFIX": "vorrat_2",
		"MQTT_HA_DISCOVERY": "false",
		"MQTT_HA_PREFIX":    "Home-Assistant_2",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{
		Port:       65535,
		DataDir:    "./.data",
		OFFContact: "stash@example.com",
		BackupKeep: 365,
		LogLevel:   slog.LevelDebug,
		MQTT: config.MQTT{
			URL:         "mqtts://broker.example:8883",
			Username:    "stashbert",
			Password:    "s3cret pass:@/",
			ClientID:    "stashbert-test",
			TopicPrefix: "vorrat_2",
			HADiscovery: false,
			HAPrefix:    "Home-Assistant_2",
		},
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadValidMQTTValues(t *testing.T) {
	tests := []struct {
		key, value string
		get        func(config.MQTT) any
		want       any
	}{
		{"MQTT_URL", "mqtt://192.168.1.10:1883", func(m config.MQTT) any { return m.URL }, "mqtt://192.168.1.10:1883"},
		{"MQTT_URL", "mqtts://broker.example:8883", func(m config.MQTT) any { return m.URL }, "mqtts://broker.example:8883"},
		{"MQTT_URL", "mqtt://[::1]:1", func(m config.MQTT) any { return m.URL }, "mqtt://[::1]:1"},
		{"MQTT_URL", "mqtt://core-mosquitto:65535", func(m config.MQTT) any { return m.URL }, "mqtt://core-mosquitto:65535"},
		{"MQTT_USERNAME", "stashbert", func(m config.MQTT) any { return m.Username }, "stashbert"},
		{"MQTT_PASSWORD", "pw", func(m config.MQTT) any { return m.Password }, "pw"},
		{"MQTT_CLIENT_ID", "stashbert-2", func(m config.MQTT) any { return m.ClientID }, "stashbert-2"},
		{"MQTT_TOPIC_PREFIX", "a", func(m config.MQTT) any { return m.TopicPrefix }, "a"},
		{"MQTT_TOPIC_PREFIX", "stash_bert_09", func(m config.MQTT) any { return m.TopicPrefix }, "stash_bert_09"},
		{"MQTT_TOPIC_PREFIX", strings.Repeat("x", 64), func(m config.MQTT) any { return m.TopicPrefix }, strings.Repeat("x", 64)},
		{"MQTT_HA_DISCOVERY", "true", func(m config.MQTT) any { return m.HADiscovery }, true},
		{"MQTT_HA_DISCOVERY", "false", func(m config.MQTT) any { return m.HADiscovery }, false},
		{"MQTT_HA_PREFIX", "h", func(m config.MQTT) any { return m.HAPrefix }, "h"},
		{"MQTT_HA_PREFIX", "Home_Assistant-9", func(m config.MQTT) any { return m.HAPrefix }, "Home_Assistant-9"},
		{"MQTT_HA_PREFIX", strings.Repeat("H", 64), func(m config.MQTT) any { return m.HAPrefix }, strings.Repeat("H", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			cfg, err := config.Load(env(map[string]string{tt.key: tt.value}))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := tt.get(cfg.MQTT); got != tt.want {
				t.Errorf("%s=%q: got %v, want %v", tt.key, tt.value, got, tt.want)
			}
		})
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
		{"MQTT_URL", "192.168.1.10:1883"},
		{"MQTT_URL", "http://broker:1883"},
		{"MQTT_URL", "tcp://broker:1883"},
		{"MQTT_URL", "ws://broker:1883"},
		{"MQTT_URL", "mqtt://broker"},
		{"MQTT_URL", "mqtt://broker:"},
		{"MQTT_URL", "mqtt://broker:0"},
		{"MQTT_URL", "mqtt://broker:65536"},
		{"MQTT_URL", "mqtt://broker:port"},
		{"MQTT_URL", "mqtt://:1883"},
		{"MQTT_URL", "mqtt://user:pw@broker:1883"},
		{"MQTT_URL", "mqtt://broker:1883/"},
		{"MQTT_URL", "mqtt://broker:1883/stashbert"},
		{"MQTT_URL", "mqtt://broker:1883?x=1"},
		{"MQTT_URL", "mqtt://broker:1883#x"},
		{"MQTT_URL", "mqtt:broker:1883"},
		{"MQTT_TOPIC_PREFIX", strings.Repeat("x", 65)},
		{"MQTT_TOPIC_PREFIX", "Stashbert"},
		{"MQTT_TOPIC_PREFIX", "stash-bert"},
		{"MQTT_TOPIC_PREFIX", "stash/bert"},
		{"MQTT_TOPIC_PREFIX", "stash bert"},
		{"MQTT_TOPIC_PREFIX", "stash+"},
		{"MQTT_TOPIC_PREFIX", "vorräte"},
		{"MQTT_HA_DISCOVERY", "TRUE"},
		{"MQTT_HA_DISCOVERY", "1"},
		{"MQTT_HA_DISCOVERY", "yes"},
		{"MQTT_HA_PREFIX", strings.Repeat("h", 65)},
		{"MQTT_HA_PREFIX", "home/assistant"},
		{"MQTT_HA_PREFIX", "home assistant"},
		{"MQTT_HA_PREFIX", "home#"},
		{"MQTT_HA_PREFIX", "hä"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			if _, err := config.Load(env(map[string]string{tt.key: tt.value})); err == nil {
				t.Errorf("Load with %s=%q: got nil error, want error", tt.key, tt.value)
			}
		})
	}
}

// A URL with credentials is rejected without repeating the password, because
// the error ends up in the log.
func TestLoadMQTTURLErrorHidesPassword(t *testing.T) {
	_, err := config.Load(env(map[string]string{"MQTT_URL": "mqtt://stashbert:hunter2-secret@broker:1883"}))
	if err == nil {
		t.Fatal("Load: got nil error, want error")
	}
	if strings.Contains(err.Error(), "hunter2-secret") {
		t.Errorf("error %q contains the password", err)
	}
	_, err = config.Load(env(map[string]string{"MQTT_URL": "mqtt://stashbert:hunter2-secret@broker:port"}))
	if err == nil {
		t.Fatal("Load: got nil error, want error")
	}
	if strings.Contains(err.Error(), "hunter2-secret") {
		t.Errorf("error %q contains the password", err)
	}
}
