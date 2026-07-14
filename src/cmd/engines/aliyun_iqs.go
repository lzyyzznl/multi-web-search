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

func (e *aliyunIqsEngine) Search(ctx context.Context, query string) ([]SearchResult, error) {
	reqBody := map[string]interface{}{
		"query":     query,
		"engineType": "Generic",
		"timeRange": "NoLimit",
		"contents": map[string]interface{}{
			"mainText":     false,
			"markdownText": true,
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

	// 读取完整响应用于调试
	rawBody, _ := io.ReadAll(resp.Body)

	// 尝试解析 data 字段，可能是对象或数组
	var wrapper struct {
		Success bool            `json:"success"`
		Code    int             `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &wrapper); err != nil {
		return nil, fmt.Errorf("aliyun-iqs: decode wrapper: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("aliyun-iqs: API returned success=false code=%d", wrapper.Code)
	}

	// 尝试将 data 解析为对象数组（字段名 url）
	var items []struct {
		Title       string  `json:"title"`
		URL         string  `json:"url"`
		Content     string  `json:"content"`
		Summary     string  `json:"summary"`
		RerankScore float64 `json:"rerankScore"`
	}
	if err := json.Unmarshal(wrapper.Data, &items); err != nil || len(items) == 0 {
		// 回退：解析 pageItems 结构（字段名 link）
		var pageResp struct {
			PageItems []struct {
				Title       string  `json:"title"`
				Link        string  `json:"link"`
				Snippet     string  `json:"snippet"`
				Summary     string  `json:"summary"`
				RerankScore float64 `json:"rerankScore"`
			} `json:"pageItems"`
		}
		if err2 := json.Unmarshal(wrapper.Data, &pageResp); err2 != nil || len(pageResp.PageItems) == 0 {
			// 最终回退：从根对象解析 pageItems
			var direct struct {
				PageItems []struct {
					Title       string  `json:"title"`
					Link        string  `json:"link"`
					Snippet     string  `json:"snippet"`
					Summary     string  `json:"summary"`
					RerankScore float64 `json:"rerankScore"`
				} `json:"pageItems"`
			}
			if err3 := json.Unmarshal(rawBody, &direct); err3 != nil {
				return nil, fmt.Errorf("aliyun-iqs: decode: %w (raw: %s)", err, string(rawBody[:200]))
			}
			for _, r := range direct.PageItems {
				items = append(items, struct {
					Title       string  `json:"title"`
					URL         string  `json:"url"`
					Content     string  `json:"content"`
					Summary     string  `json:"summary"`
					RerankScore float64 `json:"rerankScore"`
				}{
					Title:       r.Title,
					URL:         r.Link,
					Content:     r.Snippet,
					Summary:     r.Summary,
					RerankScore: r.RerankScore,
				})
			}
		} else {
			for _, r := range pageResp.PageItems {
				items = append(items, struct {
					Title       string  `json:"title"`
					URL         string  `json:"url"`
					Content     string  `json:"content"`
					Summary     string  `json:"summary"`
					RerankScore float64 `json:"rerankScore"`
				}{
					Title:       r.Title,
					URL:         r.Link,
					Content:     r.Snippet,
					Summary:     r.Summary,
					RerankScore: r.RerankScore,
				})
			}
		}
	}

	results := make([]SearchResult, 0, len(items))
	for _, r := range items {
		snippet := r.Summary
		if snippet == "" {
			snippet = r.Content
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
			Source:  e.name,
			Score:   r.RerankScore,
		})
	}
	return results, nil
}
