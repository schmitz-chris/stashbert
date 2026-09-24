package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// healthcheckTimeout limits the whole request of -healthcheck.
const healthcheckTimeout = 3 * time.Second

// healthURL returns the URL of GET /api/v1/health on 127.0.0.1:$PORT. PORT
// has the same default and range as in internal/config; -healthcheck does
// not load the whole configuration.
func healthURL(getenv func(string) string) (string, error) {
	port := 8080
	if v := getenv("PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("PORT: %q is not an integer between 1 and 65535", v)
		}
		port = n
	}
	return "http://127.0.0.1:" + strconv.Itoa(port) + "/api/v1/health", nil
}

// healthcheck asks the server on 127.0.0.1:$PORT for its health and
// succeeds only if it answers 200 within healthcheckTimeout.
func healthcheck(ctx context.Context, getenv func(string) string) error {
	url, err := healthURL(getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d, want 200", url, resp.StatusCode)
	}
	return nil
}
