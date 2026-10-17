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

// deepseekEngine 调用 DeepSeek Search 接口（文档：https://api-docs.deepseek.com/zh-cn/guides/search）.
type deepseekEngine struct {
	name   string
	apiKey string
	client *http.Client
}

func NewDeepSeek(apiKey string) Engine {
	return &deepseekEngine{
		name:   "deepseek",
		apiKey: apiKey,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (e *deepseekEngine) Name() string { return e.name }

func (e *deepseekEngine) Search(ctx context.Context, query string, num int) ([]SearchResult, error) {
	// need_content=false：正文内容很大且走独立 fetch 命令，避免污染搜索结果体积.
	reqBody := map[string]interface{}{
		"query":        query,
		"count":        num,
		"need_content": false,
	}
	payload, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.deepseek.com/v1/tools/search", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("deepseek: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("deepseek: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepseek: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return parseDeepSeekResults(resp.Body)
}

// parseDeepSeekResults 解析 Search 接口响应体为统一 SearchResult 列表.
func parseDeepSeekResults(body io.Reader) ([]SearchResult, error) {
	var data struct {
		Results []struct {
			RerankScore float64 `json:"rerank_score"`
			Title       string  `json:"title"`
			URL         string  `json:"url"`
			Snippet     string  `json:"snippet"`
			Summary     string  `json:"summary"`
			Date        string  `json:"date"`
		} `json:"results"`
	}
	if err := json.NewDecoder(body).Decode(&data); err != nil {
		return nil, fmt.Errorf("deepseek: decode: %w", err)
	}

	results := make([]SearchResult, 0, len(data.Results))
	for _, r := range data.Results {
		// snippet（100~200 字符短摘要）优先；实际响应中常缺失，缺失时回退到 summary（正文级内容，体积大）.
		snippet := r.Snippet
		if snippet == "" {
			snippet = r.Summary
		}
		// rerank_score 取值 0～3，归一化到 0～1 与其他引擎评分可比.
		score := r.RerankScore / 3.0
		if score > 1 {
			score = 1
		}
		if score < 0 {
			score = 0
		}
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
			Source:  "deepseek",
			Score:   score,
			Date:    r.Date,
		})
	}
	return results, nil
}
