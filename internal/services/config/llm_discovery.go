package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type DiscoveredModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContextSize int    `json:"contextSize,omitempty"`
}

// DiscoverModels probes the saved endpoint only when the user explicitly requests it.
func (s *LLMConfigService) DiscoverModels(ctx context.Context, providerID string) ([]DiscoveredModel, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	provider, err := s.providers.GetLLMProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:       12 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	path := strings.TrimRight(provider.BaseURL, "/")
	if provider.Protocol == "anthropic" && !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	path += "/models"
	models := make(map[string]DiscoveredModel)
	nextCursor := ""
	for page := 0; page < 10 && len(models) < 2000; page++ {
		requestURL, err := url.Parse(path)
		if err != nil {
			return nil, err
		}
		if provider.Protocol == "anthropic" {
			query := requestURL.Query()
			query.Set("limit", "200")
			if page > 0 {
				// Cursor is set at the end of the previous page.
				query.Set("after_id", nextCursor)
			}
			requestURL.RawQuery = query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, err
		}
		if provider.Protocol == "anthropic" {
			req.Header.Set("x-api-key", provider.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else if provider.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+provider.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("model discovery request failed: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("model discovery failed: HTTP %d", resp.StatusCode)
		}
		var result struct {
			Data []struct {
				ID             string `json:"id"`
				DisplayName    string `json:"display_name"`
				MaxInputTokens int    `json:"max_input_tokens"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("invalid model list response: %w", err)
		}
		for _, model := range result.Data {
			id := strings.TrimSpace(model.ID)
			if id == "" {
				continue
			}
			name := strings.TrimSpace(model.DisplayName)
			if name == "" {
				name = id
			}
			models[id] = DiscoveredModel{ID: id, Name: name, ContextSize: model.MaxInputTokens}
		}
		if provider.Protocol != "anthropic" || !result.HasMore || result.LastID == "" {
			break
		}
		nextCursor = result.LastID
	}
	items := make([]DiscoveredModel, 0, len(models))
	for _, model := range models {
		items = append(items, model)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}
