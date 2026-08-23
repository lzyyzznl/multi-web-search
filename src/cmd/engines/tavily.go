package engines

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type tavilyEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewTavily(apiKey string) Engine {
	return &tavilyEngine{
		name:   "tavily",
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *tavilyEngine) Name() string { return e.name }

func (e *tavilyEngine) Search(ctx context.Context, query string, num int) ([]SearchResult, error) {
	reqBody := map[string]interface{}{
		"api_key":     e.apiKey,
		"query":       query,
		"max_results": num,
	}
	payload, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.tavily.com/search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("tavily: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("tavily: decode: %w", err)
	}

	results := make([]SearchResult, 0, len(data.Results))
	for _, r := range data.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Source:  e.name,
			Score:   r.Score,
		})
	}
	return results, nil
}
