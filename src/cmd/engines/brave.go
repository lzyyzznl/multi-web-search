package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type braveEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewBrave(apiKey string) Engine {
	return &braveEngine{
		name:   "brave",
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *braveEngine) Name() string { return e.name }

func (e *braveEngine) Search(ctx context.Context, query string, num int) ([]SearchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.search.brave.com/res/v1/web/search", nil)
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprintf("%d", num))
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-Subscription-Token", e.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("brave: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Web struct {
			Results []struct {
				Title       string  `json:"title"`
				URL         string  `json:"url"`
				Description string  `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("brave: decode: %w", err)
	}

	results := make([]SearchResult, 0, len(data.Web.Results))
	for _, r := range data.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
			Source:  e.name,
		})
	}
	return results, nil
}
