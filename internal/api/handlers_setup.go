package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/schmitz-chris/stashbert/internal/auth"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/store"
	"github.com/schmitz-chris/stashbert/internal/store/db"
)

// passwordHashKey is the settings key of the household password hash.
const passwordHashKey = "password_hash"

// Length of a member name after trimming (architecture.md, 5).
const (
	minMemberName = 1
	maxMemberName = 40
)

// GetSetup reports whether the household password is set.
func (s *Server) GetSetup(ctx context.Context, _ GetSetupRequestObject) (GetSetupResponseObject, error) {
	configured, err := isConfigured(ctx, s.queries)
	if err != nil {
		return nil, err
	}
	return GetSetup200JSONResponse{Configured: configured}, nil
}

// CreateSetup stores the household password and the members in one
// transaction. It is only possible while no password is set.
func (s *Server) CreateSetup(ctx context.Context, request CreateSetupRequestObject) (CreateSetupResponseObject, error) {
	names := make([]string, len(request.Body.Members))
	for i, name := range request.Body.Members {
		name = strings.TrimSpace(name)
		if n := utf8.RuneCountInString(name); n < minMemberName || n > maxMemberName {
			return nil, httpx.BadRequest(fmt.Sprintf("Namen müssen nach dem Trimmen %d bis %d Zeichen lang sein", minMemberName, maxMemberName))
		}
		names[i] = name
	}

	// The transaction starts with BEGIN IMMEDIATE, so concurrent setups are serialized.
	tx, err := s.deps.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin setup: %w", err)
	}
	defer tx.Rollback() // no effect after Commit
	q := s.queries.WithTx(tx)

	configured, err := isConfigured(ctx, q)
	if err != nil {
		return nil, err
	}
	if configured {
		return nil, httpx.NewError(http.StatusConflict, "already_configured", "Die Instanz ist bereits eingerichtet")
	}

	now := store.FormatTime(time.Now())
	for _, name := range names {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("generate member id: %w", err)
		}
		err = q.CreateMember(ctx, db.CreateMemberParams{ID: id.String(), Name: name, CreatedAt: now})
		if store.IsUniqueViolation(err) {
			return nil, httpx.BadRequest("Namen der Mitglieder müssen verschieden sein")
		}
		if err != nil {
			return nil, fmt.Errorf("create member: %w", err)
		}
	}

	hash, err := auth.HashPassword(request.Body.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: passwordHashKey, Value: hash}); err != nil {
		return nil, fmt.Errorf("store password hash: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit setup: %w", err)
	}
	return CreateSetup204Response{}, nil
}

// isConfigured reports whether a password hash is stored.
func isConfigured(ctx context.Context, q *db.Queries) (bool, error) {
	_, err := q.GetSetting(ctx, passwordHashKey)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read password hash: %w", err)
	}
	return true, nil
}
