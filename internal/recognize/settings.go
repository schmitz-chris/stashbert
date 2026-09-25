package recognize

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// The settings keys of the recognition (architecture.md, 5).
const (
	settingProvider = "recognition_provider"
	settingModel    = "recognition_model"
	// SettingAPIKey is the settings key of the API key, which GET /backup
	// removes from its copy of the database (ADR-0021).
	SettingAPIKey = "recognition_api_key"
)

// Settings are the stored settings of the recognition.
type Settings struct {
	// Provider is "" while the recognition is off.
	Provider string
	// Model is "" for the default model of Provider.
	Model string
	// APIKey is never logged or sent to a client.
	APIKey string
}

// EffectiveModel returns Model, or the default model of Provider if Model
// is empty.
func (s Settings) EffectiveModel() string {
	if s.Model != "" {
		return s.Model
	}
	return DefaultModel(s.Provider)
}

// KeyHint returns the last four characters of APIKey, or "" without key.
func (s Settings) KeyHint() string {
	r := []rune(s.APIKey)
	return string(r[max(0, len(r)-4):])
}

// LoadSettings reads the settings through q. Missing settings are empty.
func LoadSettings(ctx context.Context, q *db.Queries) (Settings, error) {
	var s Settings
	for key, dst := range map[string]*string{settingProvider: &s.Provider, settingModel: &s.Model, SettingAPIKey: &s.APIKey} {
		v, err := q.GetSetting(ctx, key)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Settings{}, fmt.Errorf("read setting %s: %w", key, err)
		}
		*dst = v
	}
	return s, nil
}

// UpdateSettings stores the settings of PUT /integrations/recognition and
// returns them (ADR-0021). model and key are trimmed first.
//   - provider "" switches the recognition off and removes all three
//     settings.
//   - A key is checked with c.CheckKey before anything is stored.
//   - Without key, the stored key stays if provider is the stored provider;
//     otherwise the result is 400 invalid_request.
//   - An empty model removes the stored model, so the default applies.
func UpdateSettings(ctx context.Context, sqlDB *sql.DB, c *Client, provider, model, key string) (Settings, error) {
	next := Settings{Provider: provider, Model: strings.TrimSpace(model), APIKey: strings.TrimSpace(key)}
	switch {
	case provider == "":
		next = Settings{}
	case DefaultModel(provider) == "":
		return Settings{}, httpx.BadRequest("Unbekannter Anbieter")
	case next.APIKey != "":
		// No transaction is open while the provider is asked.
		if err := c.CheckKey(ctx, provider, next.APIKey); err != nil {
			return Settings{}, err
		}
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Settings{}, fmt.Errorf("update recognition settings: begin transaction: %w", err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	if next.Provider != "" && next.APIKey == "" {
		cur, err := LoadSettings(ctx, q)
		if err != nil {
			return Settings{}, err
		}
		if cur.Provider != next.Provider || cur.APIKey == "" {
			return Settings{}, httpx.BadRequest("Für einen neuen Anbieter fehlt der API-Schlüssel")
		}
		next.APIKey = cur.APIKey
	}
	for _, s := range []db.UpsertSettingParams{
		{Key: settingProvider, Value: next.Provider},
		{Key: settingModel, Value: next.Model},
		{Key: SettingAPIKey, Value: next.APIKey},
	} {
		if s.Value == "" {
			err = q.DeleteSetting(ctx, s.Key)
		} else {
			err = q.UpsertSetting(ctx, s)
		}
		if err != nil {
			return Settings{}, fmt.Errorf("update recognition setting %s: %w", s.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Settings{}, fmt.Errorf("update recognition settings: commit: %w", err)
	}
	return next, nil
}
