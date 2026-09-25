package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/oapi-codegen/nullable"

	"github.com/schmitz-chris/stashbert/internal/domain"
	"github.com/schmitz-chris/stashbert/internal/httpx"
	"github.com/schmitz-chris/stashbert/internal/lookup"
	"github.com/schmitz-chris/stashbert/internal/recognize"
)

// GetRecognitionSettings returns the settings of the product recognition
// without the API key, only whether one is set and its last four characters
// (ADR-0021).
func (s *Server) GetRecognitionSettings(ctx context.Context, request GetRecognitionSettingsRequestObject) (GetRecognitionSettingsResponseObject, error) {
	settings, err := recognize.LoadSettings(ctx, s.queries)
	if err != nil {
		return nil, err
	}
	return GetRecognitionSettings200JSONResponse(recognitionSettings(settings)), nil
}

// SetRecognitionSettings stores provider, model and API key with
// recognize.UpdateSettings, which checks a new key with the provider, and
// returns the settings afterwards (ADR-0021).
func (s *Server) SetRecognitionSettings(ctx context.Context, request SetRecognitionSettingsRequestObject) (SetRecognitionSettingsResponseObject, error) {
	var provider, model, key string
	if !request.Body.Provider.IsNull() {
		p, err := request.Body.Provider.Get()
		if err != nil {
			return nil, httpx.BadRequest("provider fehlt")
		}
		provider = string(p)
	}
	if request.Body.Model != nil {
		model = *request.Body.Model
	}
	if request.Body.ApiKey != nil {
		key = *request.Body.ApiKey
	}
	settings, err := recognize.UpdateSettings(ctx, s.deps.DB, s.deps.Recognizer, provider, model, key)
	if err != nil {
		s.logProviderError(ctx, err)
		return nil, err
	}
	return SetRecognitionSettings200JSONResponse(recognitionSettings(settings)), nil
}

// RecognizeProduct sends the stored photo of the product to the configured
// provider and returns what it reads on it; the product stays as it is
// (ADR-0021). An unknown id results in 404 not_found, a recognition without
// provider in 409 recognition_disabled and a product without photo (or
// without its image file) in 409 no_image.
func (s *Server) RecognizeProduct(ctx context.Context, request RecognizeProductRequestObject) (RecognizeProductResponseObject, error) {
	if _, err := s.queries.GetProduct(ctx, request.Id); errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("Produkt nicht gefunden")
	} else if err != nil {
		return nil, fmt.Errorf("recognize product %s: %w", request.Id, err)
	}
	settings, err := recognize.LoadSettings(ctx, s.queries)
	if err != nil {
		return nil, err
	}
	if settings.Provider == "" {
		return nil, recognize.Disabled()
	}
	img, err := domain.OpenProductImage(ctx, s.deps.DB, s.deps.ImageDir, request.Id)
	var problem *httpx.Error
	if errors.As(err, &problem) && problem.Code == "not_found" {
		return nil, recognize.NoImage()
	}
	if err != nil {
		return nil, err
	}
	defer img.File.Close()
	data, err := io.ReadAll(io.LimitReader(img.File, lookup.MaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("recognize product %s: read image: %w", request.Id, err)
	}
	if len(data) > lookup.MaxImageBytes {
		return nil, fmt.Errorf("recognize product %s: image larger than %d bytes", request.Id, lookup.MaxImageBytes)
	}
	result, err := s.deps.Recognizer.Recognize(ctx, settings.Provider, settings.EffectiveModel(), settings.APIKey, data, img.ContentType)
	if err != nil {
		s.logProviderError(ctx, err)
		return nil, err
	}
	return RecognizeProduct200JSONResponse{
		Name:        toNullable(result.Name),
		Brand:       toNullable(result.Brand),
		PackageSize: toNullable(result.PackageSize),
	}, nil
}

// recognitionSettings returns the RecognitionSettings of s without the key.
func recognitionSettings(s recognize.Settings) RecognitionSettings {
	r := RecognitionSettings{
		Provider: nonZero(RecognitionProvider(s.Provider)),
		Model:    nonZero(s.Model),
		KeySet:   s.APIKey != "",
		KeyHint:  nonZero(s.KeyHint()),
	}
	r.DefaultModels.Openai = recognize.DefaultOpenAIModel
	r.DefaultModels.Gemini = recognize.DefaultGeminiModel
	r.DefaultModels.Anthropic = recognize.DefaultAnthropicModel
	return r
}

// nonZero returns v, or null if v is the zero value.
func nonZero[T comparable](v T) nullable.Nullable[T] {
	var zero T
	if v == zero {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(v)
}

// logProviderError logs the cause of a failed request to the recognition
// provider, which the problem for the client leaves out. The cause names
// the provider and the status or the network error, never the key.
func (s *Server) logProviderError(ctx context.Context, err error) {
	var problem *httpx.Error
	if errors.As(err, &problem) && (problem.Code == "invalid_api_key" || problem.Code == "recognition_failed") {
		s.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "recognition provider request failed", slog.String("error", err.Error()))
	}
}
