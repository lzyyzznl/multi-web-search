package engines

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type serperEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewSerper(apiKey string) Engine {
	return &serperEngine{
		name:   "serper",
		apiKey: apiKey,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *serperEngine) Name() string { return e.name }

func (e *serperEngine) Search(ctx context.Context, query string) ([]SearchResult, error) {
	body := map[string]interface{}{
		"q":   query,
		"num": 10,
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://google.serper.dev/search",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("serper: %w", err)
	}
	req.Header.Set("X-API-KEY", e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serper: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("serper: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("serper: HTTP %d", resp.StatusCode)
	}

	var data struct {
		Organic []struct {
			Title   string  `json:"title"`
			Link    string  `json:"link"`
			Snippet string  `json:"snippet"`
		} `json:"organic"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("serper: decode: %w", err)
	}

	results := make([]SearchResult, 0, len(data.Organic))
	for _, r := range data.Organic {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.Link,
			Snippet: r.Snippet,
			Source:  e.name,
		})
	}
	return results, nil
}
