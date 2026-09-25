package recognize

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// openAI asks the Responses API of OpenAI with the image as a data URL and
// the answer as Structured Output (text.format with a strict json_schema)
// and returns the text of the answer. The response is not stored at OpenAI.
func (c *Client) openAI(ctx context.Context, model, key string, image []byte, mediaType string) (string, error) {
	properties := map[string]any{}
	for _, f := range fields {
		properties[f] = map[string]any{"type": []string{"string", "null"}, "description": fieldDescriptions[f]}
	}
	body := map[string]any{
		"model": model,
		"store": false,
		"input": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": prompt},
				map[string]any{"type": "input_image", "image_url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(image)},
			},
		}},
		"text": map[string]any{"format": map[string]any{
			"type":   "json_schema",
			"name":   "product",
			"strict": true,
			"schema": map[string]any{"type": "object", "properties": properties, "required": fields, "additionalProperties": false},
		}},
	}
	// The text is in the output_text parts of the message items; output_text
	// on the response itself exists only in the SDKs.
	var resp struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := c.do(ctx, OpenAI, http.MethodPost, c.endpoints.OpenAI+"/v1/responses", key, body, &resp); err != nil {
		return "", err
	}
	var text strings.Builder
	for _, item := range resp.Output {
		for _, part := range item.Content {
			if item.Type == "message" && part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	return answer(OpenAI, text.String())
}

// gemini asks generateContent of Google Gemini with the image as inline_data
// and the answer as JSON after responseSchema and returns the text of the
// answer.
func (c *Client) gemini(ctx context.Context, model, key string, image []byte, mediaType string) (string, error) {
	properties := map[string]any{}
	for _, f := range fields {
		properties[f] = map[string]any{"type": "STRING", "nullable": true, "description": fieldDescriptions[f]}
	}
	body := map[string]any{
		"contents": []any{map[string]any{"parts": []any{
			map[string]any{"inline_data": map[string]any{"mime_type": mediaType, "data": base64.StdEncoding.EncodeToString(image)}},
			map[string]any{"text": prompt},
		}}},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"responseSchema":   map[string]any{"type": "OBJECT", "properties": properties, "required": fields, "propertyOrdering": fields},
		},
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	u := c.endpoints.Gemini + "/v1beta/models/" + url.PathEscape(model) + ":generateContent"
	if err := c.do(ctx, Gemini, http.MethodPost, u, key, body, &resp); err != nil {
		return "", err
	}
	var text strings.Builder
	if len(resp.Candidates) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			if !part.Thought {
				text.WriteString(part.Text)
			}
		}
	}
	return answer(Gemini, text.String())
}

// geminiKeyInvalid reports whether the error body of a 400 answer from
// Gemini names the reason API_KEY_INVALID. Gemini answers an invalid key
// with 400 and this reason, not with 401.
func geminiKeyInvalid(body []byte) bool {
	var resp struct {
		Error struct {
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &resp) != nil {
		return false
	}
	for _, d := range resp.Error.Details {
		if d.Reason == "API_KEY_INVALID" {
			return true
		}
	}
	return false
}

// anthropic asks the Messages API of Anthropic with the image as a base64
// image block before the text and the answer as structured output
// (output_config.format with json_schema) and returns the text of the
// answer.
func (c *Client) anthropic(ctx context.Context, model, key string, image []byte, mediaType string) (string, error) {
	properties := map[string]any{}
	for _, f := range fields {
		properties[f] = map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}, "description": fieldDescriptions[f]}
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": mediaType, "data": base64.StdEncoding.EncodeToString(image)}},
				map[string]any{"type": "text", "text": prompt},
			},
		}},
		"output_config": map[string]any{"format": map[string]any{
			"type":   "json_schema",
			"schema": map[string]any{"type": "object", "properties": properties, "required": fields, "additionalProperties": false},
		}},
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := c.do(ctx, Anthropic, http.MethodPost, c.endpoints.Anthropic+"/v1/messages", key, body, &resp); err != nil {
		return "", err
	}
	if resp.StopReason == "refusal" {
		return "", fmt.Errorf("%s: %w: refusal", Anthropic, Failed())
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return answer(Anthropic, text.String())
}

// errNoAnswer is the cause of Failed when a response has no usable text,
// for example after a refusal.
var errNoAnswer = errors.New("no answer in the response")

// answer returns text, or an error wrapping Failed if it is empty.
func answer(provider, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s: %w: %w", provider, Failed(), errNoAnswer)
	}
	return text, nil
}
