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

type baiduEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewBaidu(apiKey string) Engine {
	return &baiduEngine{
		name:   "baidu",
		apiKey: apiKey,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (e *baiduEngine) Name() string { return e.name }

func (e *baiduEngine) Search(ctx context.Context, query string) ([]SearchResult, error) {
	reqBody := map[string]interface{}{
		"messages": []map[string]string{
			{"content": query, "role": "user"},
		},
		"search_source":       "baidu_search_v2",
		"resource_type_filter": []map[string]interface{}{{"type": "web", "top_k": 10}},
	}
	payload, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://qianfan.baidubce.com/v2/ai_search/web_search",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("baidu: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("X-Appbuilder-From", "openclaw")
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("baidu: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("baidu: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		References []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
		} `json:"references"`
		Code string `json:"code,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("baidu: decode: %w", err)
	}
	if data.Code != "" && data.Code != "200" {
		return nil, fmt.Errorf("baidu: API error code=%s", data.Code)
	}

	results := make([]SearchResult, 0, len(data.References))
	for _, r := range data.References {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Source:  e.name,
		})
	}
	return results, nil
}
