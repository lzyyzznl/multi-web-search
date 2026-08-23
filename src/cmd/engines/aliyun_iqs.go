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

type aliyunIqsEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewAliyunIQS(apiKey string) Engine {
	return &aliyunIqsEngine{
		name:   "aliyun-iqs",
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *aliyunIqsEngine) Name() string { return e.name }

func (e *aliyunIqsEngine) Search(ctx context.Context, query string, num int) ([]SearchResult, error) {
	reqBody := map[string]interface{}{
		"query":      query,
		"engineType": "Generic",
		"timeRange":  "NoLimit",
		"pageSize":   num,
		"contents": map[string]interface{}{
			"mainText":     false,
			"markdownText": false,
			"summary":      true,
			"rerankScore":  true,
		},
	}
	payload, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://cloud-iqs.aliyuncs.com/search/unified",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("aliyun-iqs: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aliyun-iqs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("aliyun-iqs: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aliyun-iqs: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// 实际的 API 返回结构：pageItems 在顶层
	var data struct {
		RequestID string `json:"requestId"`
		PageItems []struct {
			Title       string  `json:"title"`
			Link        string  `json:"link"`
			Snippet     string  `json:"snippet"`
			Summary     string  `json:"summary"`
			RerankScore float64 `json:"rerankScore"`
		} `json:"pageItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("aliyun-iqs: decode: %w", err)
	}

	results := make([]SearchResult, 0, len(data.PageItems))
	for i, r := range data.PageItems {
		if i >= num {
			break
		}
		snippet := r.Summary
		if snippet == "" {
			snippet = r.Snippet
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.Link,
			Snippet: snippet,
			Source:  e.name,
			Score:   r.RerankScore,
		})
	}
	return results, nil
}
