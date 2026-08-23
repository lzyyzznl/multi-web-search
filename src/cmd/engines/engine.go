package engines

import (
	"context"
	"net/url"
	"strings"
)

// SearchResult 单条搜索结果.
type SearchResult struct {
	Title        string             `json:"title"`
	URL          string             `json:"url"`
	Snippet      string             `json:"snippet"`
	Source       string             `json:"source"`
	Score        float64            `json:"score"`
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
