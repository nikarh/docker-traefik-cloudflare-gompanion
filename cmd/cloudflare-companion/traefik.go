package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type TraefikRouter struct {
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Status string `json:"status"`
}

func FetchTraefikRouters(ctx context.Context, baseURL string) ([]TraefikRouter, int, string, error) {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	url := strings.TrimRight(baseURL, "/") + "/api/http/routers"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, "", err
	}
	body := string(bodyBytes)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, body, nil
	}

	var routers []TraefikRouter
	if err := json.Unmarshal(bodyBytes, &routers); err != nil {
		return nil, resp.StatusCode, body, fmt.Errorf("failed to decode JSON from Traefik: %w", err)
	}
	return routers, resp.StatusCode, body, nil
}
