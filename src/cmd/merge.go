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
	"deepseek":   0.9,
}

// mergeResults 按 URL 去重 + 综合评分排序.
//
// primary 非空时，来自该引擎的结果优先：同一 URL 保留主引擎的标题/摘要/站点等元数据，
// 且主引擎结果整体排在辅助引擎独有结果之前；辅助引擎只贡献 engine_scores / agreement，
// 用于交叉验证与发散补全。primary 为空时退化为纯分数排序。
//
// ponytail: O(n) hash dedup; 输出排序 O(n log n)，千级结果足够.
func mergeResults(all [][]engines.SearchResult, primary string) []engines.SearchResult {
	type accumulated struct {
		rec    engines.SearchResult
		scores map[string]float64
	}

	seen := make(map[string]*accumulated)
	for _, results := range all {
		for _, r := range results {
			key := engines.NormalizeURL(r.URL)
			score := engineWeight[r.Source] * fallbackScore(r)

			acc, ok := seen[key]
			if !ok {
				r.Score = score
				r.Primary = primary != "" && r.Source == primary
				acc = &accumulated{rec: r, scores: map[string]float64{}}
				seen[key] = acc
			} else if primary != "" && r.Source == primary && !acc.rec.Primary {
				// 主引擎记录优先：用主引擎元数据覆盖辅助引擎首次写入的记录.
				priorScore := acc.rec.Score
				acc.rec = r
				acc.rec.Primary = true
				acc.rec.Score = priorScore
			}
			if score > acc.rec.Score {
				acc.rec.Score = score
			}
			acc.scores[r.Source] = score
		}
	}

	result := make([]engines.SearchResult, 0, len(seen))
	for _, acc := range seen {
		acc.rec.EngineScores = acc.scores
		if len(acc.scores) > 1 {
			acc.rec.Agreement = len(acc.scores)
		}
		result = append(result, acc.rec)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Primary != result[j].Primary {
			return result[i].Primary
		}
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		if result[i].Agreement != result[j].Agreement {
			return result[i].Agreement > result[j].Agreement
		}
		return result[i].URL < result[j].URL
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
