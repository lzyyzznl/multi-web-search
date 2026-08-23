package cmd

import (
	"testing"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"strip www", "http://www.example.com/path", "https://example.com/path"},
		{"strip trailing slash", "https://example.com/", "https://example.com"},
		{"strip fragment", "https://example.com/page#section", "https://example.com/page"},
		{"strip utm params", "https://example.com/p?utm_source=x&id=1", "https://example.com/p?id=1"},
		{"upgrade to https", "http://example.com/a", "https://example.com/a"},
		{"no change", "https://example.com/path?q=1", "https://example.com/path?q=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := engines.NormalizeURL(c.in)
			if got != c.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMergeResultsDedup(t *testing.T) {
	// 同一 URL 出现在两个引擎，应去重且保留最高分来源.
	input := [][]engines.SearchResult{
		{
			{Title: "A", URL: "https://example.com/p", Source: "serper"},
			{Title: "B", URL: "https://other.com", Source: "baidu"},
		},
		{
			{Title: "A dup", URL: "https://example.com/p", Source: "brave"},
		},
	}
	merged := mergeResults(input)
	if len(merged) != 2 {
		t.Fatalf("expected 2 unique results, got %d", len(merged))
	}
	// 应为两条：example.com/p 和 other.com.
}

func TestMergeResultsScoreOrdering(t *testing.T) {
	// serper 权重 1.0，这里无自带 score，fallback 0.5 → 0.5
	// baidu 权重 0.8，fallback 0.5 → 0.4
	// 所以 serper 结果应排在 baidu 前面.
	input := [][]engines.SearchResult{
		{
			{Title: "Baidu result", URL: "https://b.com", Source: "baidu"},
			{Title: "Serper result", URL: "https://s.com", Source: "serper"},
		},
	}
	merged := mergeResults(input)
	if len(merged) != 2 {
		t.Fatalf("expected 2, got %d", len(merged))
	}
	if merged[0].Source != "serper" {
		t.Errorf("expected serper first (higher weight), got %s", merged[0].Source)
	}
}

func TestMergeEngineScoresAccumulation(t *testing.T) {
	input := [][]engines.SearchResult{
		{
			{Title: "A", URL: "https://example.com/p", Source: "serper"},
		},
		{
			{Title: "A dup", URL: "https://example.com/p", Source: "brave"},
		},
	}
	merged := mergeResults(input)
	if len(merged) != 1 {
		t.Fatalf("expected 1, got %d", len(merged))
	}
	if len(merged[0].EngineScores) != 2 {
		t.Errorf("expected engine_scores of 2, got %d", len(merged[0].EngineScores))
	}
}
