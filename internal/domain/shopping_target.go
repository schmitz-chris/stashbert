package domain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// ShoppingTarget is a list in Home Assistant that the shopping list can go
// to (architecture.md, 11.7). StashBert knows only its opaque id and its
// name.
type ShoppingTarget struct {
	ID   string
	Name string
}

// Limits of the offer on <p>/in/targets (architecture.md, 11.7); lengths
// in characters after trimming.
const (
	maxShoppingTargets          = 50
	maxShoppingTargetIDLength   = 255
	maxShoppingTargetNameLength = 100
)

// The settings keys of the chosen target list (architecture.md, 5 and 11.7).
const (
	settingShoppingTargetID   = "shopping_target_id"
	settingShoppingTargetName = "shopping_target_name"
)

// ParseShoppingTargets returns the target lists of payload, the offer of
// Home Assistant on <p>/in/targets (architecture.md, 11.7): a JSON array of
// at most 50 objects with id from 1 to 255 and name from 1 to 100
// characters after trimming. It returns them trimmed and in their order,
// never nil, or an error that says why payload is invalid.
func ParseShoppingTargets(payload []byte) ([]ShoppingTarget, error) {
	var entries []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &entries); err != nil {
		return nil, fmt.Errorf("decode target lists: %w", err)
	}
	// null decodes without error but is no array.
	if entries == nil {
		return nil, errors.New("target lists are null, want a JSON array")
	}
	if len(entries) > maxShoppingTargets {
		return nil, fmt.Errorf("%d target lists, want at most %d", len(entries), maxShoppingTargets)
	}
	targets := make([]ShoppingTarget, len(entries))
	for i, e := range entries {
		t := ShoppingTarget{ID: strings.TrimSpace(e.ID), Name: strings.TrimSpace(e.Name)}
		if n := utf8.RuneCountInString(t.ID); n < 1 || n > maxShoppingTargetIDLength {
			return nil, fmt.Errorf("target list %d: id has %d characters, want 1 to %d", i, n, maxShoppingTargetIDLength)
		}
		if n := utf8.RuneCountInString(t.Name); n < 1 || n > maxShoppingTargetNameLength {
			return nil, fmt.Errorf("target list %d: name has %d characters, want 1 to %d", i, n, maxShoppingTargetNameLength)
		}
		targets[i] = t
	}
	return targets, nil
}

// FindShoppingTarget returns the target list with id from offer. An id that
// is not offered results in 422 unknown_target.
func FindShoppingTarget(offer []ShoppingTarget, id string) (ShoppingTarget, error) {
	for _, t := range offer {
		if t.ID == id {
			return t, nil
		}
	}
	return ShoppingTarget{}, httpx.NewError(http.StatusUnprocessableEntity, "unknown_target",
		"Die Liste wird von Home Assistant gerade nicht angeboten")
}

// StoredShoppingTarget returns the chosen target list stored in settings,
// or nil if none is chosen.
func StoredShoppingTarget(ctx context.Context, q *db.Queries) (*ShoppingTarget, error) {
	id, err := q.GetSetting(ctx, settingShoppingTargetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read setting %s: %w", settingShoppingTargetID, err)
	}
	name, err := q.GetSetting(ctx, settingShoppingTargetName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read setting %s: %w", settingShoppingTargetName, err)
	}
	return &ShoppingTarget{ID: id, Name: name}, nil
}

// StoreShoppingTarget stores t as the chosen target list in settings, or
// removes the choice if t is nil. Call it with the queries of a
// transaction, because it writes two settings.
func StoreShoppingTarget(ctx context.Context, q *db.Queries, t *ShoppingTarget) error {
	if t == nil {
		for _, key := range []string{settingShoppingTargetID, settingShoppingTargetName} {
			if err := q.DeleteSetting(ctx, key); err != nil {
				return fmt.Errorf("delete setting %s: %w", key, err)
			}
		}
		return nil
	}
	for _, s := range []db.UpsertSettingParams{
		{Key: settingShoppingTargetID, Value: t.ID},
		{Key: settingShoppingTargetName, Value: t.Name},
	} {
		if err := q.UpsertSetting(ctx, s); err != nil {
			return fmt.Errorf("store setting %s: %w", s.Key, err)
		}
	}
	return nil
}
