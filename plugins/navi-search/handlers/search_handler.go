package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	coreskill "github.com/open-navi/navi/internal/navi/skill"
)

type SkillEntry = coreskill.SkillEntry
type Interface = coreskill.Interface
type Hub = coreskill.Hub

var RegisterInternalHandler = coreskill.RegisterInternalHandler

type braveSearchResult struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// RegisterSearchHandler adds the internal handler for web searching.
// It is called during agent initialization with the configured API key.
func RegisterSearchHandler(id string, key string) {
	RegisterInternalHandler(id, "search", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		if key == "" {
			return nil, fmt.Errorf("brave_search_key not configured; search is disabled")
		}

		query, _ := args["query"].(string)
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}

		count := 5
		if c, ok := args["count"].(float64); ok {
			count = int(c)
		} else if c, ok := args["count"].(int); ok {
			count = c
		}

		u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
		q := u.Query()
		q.Set("q", query)
		q.Set("count", fmt.Sprint(count))
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Subscription-Token", key)
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("brave search api returned status %d", resp.StatusCode)
		}

		var result braveSearchResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		type outputResult struct {
			Title       string `json:"title" json:"title"`
			URL         string `json:"url" json:"url"`
			Description string `json:"description" json:"description"`
		}
		results := make([]outputResult, 0, len(result.Web.Results))
		for _, r := range result.Web.Results {
			results = append(results, outputResult{
				Title:       r.Title,
				URL:         r.URL,
				Description: r.Description,
			})
		}

		payload := map[string]any{
			"results": results,
		}
		return payload, nil
	})
}

// RegisterHubSearchHandler adds the internal handler for searching the NAVI skill hub.
func RegisterHubSearchHandler(id string, hub Hub) {
	RegisterInternalHandler(id, "search_hub", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		if hub == nil {
			return nil, fmt.Errorf("hub not configured")
		}

		query, _ := args["query"].(string)
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}

		results, err := hub.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("hub search failed: %w", err)
		}

		payload := map[string]any{
			"results": results,
		}
		return payload, nil
	})
}
