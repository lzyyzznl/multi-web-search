package engines

import (
	"context"
	"net/url"
	"strings"
)

// SearchResult 单条搜索结果.
type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Source  string  `json:"source"`
	Score   float64 `json:"score"`
	Date    string  `json:"date,omitempty"`
	// 以下两字段仅 DeepSeek 引擎填充（其他引擎省略，保持输出兼容）。
	// authority：0～4 权威性（≥3 极权威），未知(-1)或缺失时不输出。
	SiteName  string `json:"site_name,omitempty"`
	Authority *int   `json:"authority,omitempty"`

	// Primary 标记该结果由主引擎返回：排序时恒在辅助引擎独有结果之前。
	Primary bool `json:"primary,omitempty"`
	// Agreement 返回该 URL 的引擎去重数量，>1 才输出（多引擎互证信号）。
	Agreement int `json:"agreement,omitempty"`

	EngineScores map[string]float64 `json:"engine_scores,omitempty"`
}

// Engine 搜索引擎接口.
type Engine interface {
	Name() string
	Search(ctx context.Context, query string, num int) ([]SearchResult, error)
}

// NormalizeURL 对 URL 做归一化用于去重.
//
// ponytail: O(n²) naive dedup, switch to bloom filter if 1000+ results become common.
func NormalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// scheme 统一为 https
	u.Scheme = "https"
	// 去掉 www. 前缀
	u.Host = strings.TrimPrefix(u.Host, "www.")
	// 去掉末尾斜杠
	u.Path = strings.TrimSuffix(u.Path, "/")
	// 去掉 fragment
	u.Fragment = ""
	// 去掉追踪参数
	q := u.Query()
	for key := range q {
		if strings.HasPrefix(strings.ToLower(key), "utm_") {
			delete(q, key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
