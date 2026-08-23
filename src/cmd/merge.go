package cmd

import (
	"sort"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

// engineWeight 各引擎排序权重.
var engineWeight = map[string]float64{
	"serper":     1.0,
	"brave":      0.9,
	"tavily":     0.85,
	"baidu":      0.8,
	"aliyun-iqs": 0.75,
	"exa":        0.9,
}

// mergeResults 按 URL 去重 + 综合评分排序.
//
// ponytail: linear scan dedup on sorted keys, fine for <1000 results.
func mergeResults(all [][]engines.SearchResult) []engines.SearchResult {
	seen := make(map[string]engines.SearchResult)
	for _, results := range all {
		for _, r := range results {
			key := engines.NormalizeURL(r.URL)
			existing, ok := seen[key]
			if !ok {
				// 第一次出现，计算初始评分
				r.Score = engineWeight[r.Source] * fallbackScore(r)
				r.EngineScores = map[string]float64{r.Source: r.Score}
				seen[key] = r
				continue
			}
			// 已存在：补充 engineScores，更新最高评分
			weight := engineWeight[r.Source]
			score := weight * fallbackScore(r)
			if score > existing.Score {
				existing.Score = score
			}
			if existing.EngineScores == nil {
				existing.EngineScores = make(map[string]float64)
			}
			existing.EngineScores[r.Source] = score
			seen[key] = existing
		}
	}

	result := make([]engines.SearchResult, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})
	return result
}

// fallbackScore 引擎无评分时按位置折算.
func fallbackScore(r engines.SearchResult) float64 {
	if r.Score > 0 {
		return r.Score
	}
	return 0.5 // 无评分时的默认值
}
