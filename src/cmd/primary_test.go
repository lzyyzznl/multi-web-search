package cmd

import (
	"reflect"
	"testing"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

func TestMergePrimaryRecordWins(t *testing.T) {
	// 辅助引擎先写入同一 URL，主引擎记录必须覆盖元数据并保留双方评分.
	input := [][]engines.SearchResult{
		{{Title: "aux title", URL: "https://example.com/p", Source: "serper"}},
		{{Title: "primary title", URL: "https://example.com/p", Source: "deepseek", SiteName: "Example", Score: 2.0}},
	}
	merged := mergeResults(input, "deepseek")
	if len(merged) != 1 {
		t.Fatalf("expected 1 unique result, got %d", len(merged))
	}
	got := merged[0]
	if !got.Primary || got.Source != "deepseek" {
		t.Errorf("expected primary record from deepseek, got primary=%v source=%s", got.Primary, got.Source)
	}
	if got.Title != "primary title" || got.SiteName != "Example" {
		t.Errorf("primary metadata not preserved: %+v", got)
	}
	if got.Agreement != 2 {
		t.Errorf("expected agreement 2, got %d", got.Agreement)
	}
	if _, ok := got.EngineScores["serper"]; !ok {
		t.Errorf("aux score missing from engine_scores: %v", got.EngineScores)
	}
}

func TestMergePrimaryGroupOrdering(t *testing.T) {
	// 主引擎结果即便分数更低，也排在辅助引擎独有结果之前.
	input := [][]engines.SearchResult{
		{{Title: "primary", URL: "https://a.example", Source: "deepseek", Score: 0.1}},
		{{Title: "aux", URL: "https://b.example", Source: "serper", Score: 1.0}},
	}
	merged := mergeResults(input, "deepseek")
	if merged[0].Title != "primary" || merged[1].Title != "aux" {
		t.Fatalf("expected primary first, got %s then %s", merged[0].Title, merged[1].Title)
	}
}

func TestMergeWithoutPrimaryKeepsScoreOrder(t *testing.T) {
	input := [][]engines.SearchResult{
		{{Title: "low", URL: "https://a.example", Source: "baidu", Score: 0.5}},
		{{Title: "high", URL: "https://b.example", Source: "serper", Score: 1.0}},
	}
	merged := mergeResults(input, "")
	if merged[0].Title != "high" {
		t.Fatalf("expected score ordering without primary, got %s first", merged[0].Title)
	}
	if merged[0].Primary || merged[1].Primary {
		t.Errorf("primary flag must stay false when no primary engine is active")
	}
}

func TestResolveEngines(t *testing.T) {
	enabled := []string{"aliyun-iqs", "baidu", "brave", "deepseek", "exa", "serper", "tavily"}
	cases := []struct {
		name      string
		requested []string
		primary   string
		want      []string
	}{
		{"empty requests all", nil, "deepseek", enabled},
		{"aux excludes primary", []string{"aux"}, "deepseek",
			[]string{"aliyun-iqs", "baidu", "brave", "exa", "serper", "tavily"}},
		{"all token", []string{"all"}, "deepseek", enabled},
		{"explicit names", []string{"deepseek", "brave"}, "deepseek", []string{"brave", "deepseek"}},
		{"unknown ignored", []string{"nope"}, "deepseek", []string{}},
		{"aux with explicit", []string{"aux", "exa"}, "deepseek",
			[]string{"aliyun-iqs", "baidu", "brave", "exa", "serper", "tavily"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveEngines(enabled, c.requested, c.primary)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("resolveEngines(%v, %v, %s) = %v, want %v", enabled, c.requested, c.primary, got, c.want)
			}
		})
	}
}
