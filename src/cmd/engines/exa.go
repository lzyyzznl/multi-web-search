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

type exaEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewExa(apiKey string) Engine {
	return &exaEngine{
		name:   "exa",
		apiKey: apiKey,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (e *exaEngine) Name() string { return e.name }

func (e *exaEngine) Search(ctx context.Context, query string, num int) ([]SearchResult, error) {
	reqBody := map[string]interface{}{
		"query":      query,
		"numResults": num,
		"type":       "auto",
		"contents": map[string]interface{}{
			"text": map[string]interface{}{
				"maxCharacters": 300,
			},
		},
	}
	payload, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.exa.ai/search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("exa: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("exa: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("exa: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Results []struct {
			Title string  `json:"title"`
			URL   string  `json:"url"`
			Text  string  `json:"text"`
			Score float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("exa: decode: %w", err)
	}

	results := make([]SearchResult, 0, len(data.Results))
	for i, r := range data.Results {
		// Exa 不带 score 时按位置折减，避免同权重.
		score := r.Score
		if score <= 0 {
			score = 0.8 - 0.05*float64(i) // 位置衰减，首条 0.8
			if score < 0.1 {
				score = 0.1
			}
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Text,
			Source:  e.name,
			Score:   score,
		})
	}
	return results, nil
}
