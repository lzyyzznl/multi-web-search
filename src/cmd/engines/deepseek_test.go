package engines

import (
	"encoding/json"
	"strings"
	"testing"
)

// fixture 摘自真实 DeepSeek Search 响应（已裁剪 summary），覆盖：snippet 存在/缺失、date 为空非空、评分归一化.
const fixture = `{
  "results": [
    {
      "rerank_score": 2.0248324871063232,
      "title": "异步 Rust | 示例 A",
      "url": "https://a.example.com/async-rust",
      "snippet": "在 Rust 中，异步编程依赖于 future 的概念。",
      "summary": "# 异步 Rust\n\n长正文摘要……",
      "site_name": "Example 文档站",
      "authority": 4,
      "date": null
    },
    {
      "rerank_score": 1.5,
      "title": "无 snippet 的条目",
      "url": "https://b.example.com/async",
      "summary": "只有 summary，snippet 缺失时回退用它。",
      "authority": -1,
      "date": "2025-11-13T16:00:00+00:00"
    },
    {
      "rerank_score": 3.2,
      "title": "超上限评分",
      "url": "https://c.example.com/high",
      "snippet": "评分 3.2 应被截断为 1.0",
      "authority": 0,
      "date": "2026-01-01T00:00:00+00:00"
    }
  ]
}`

func TestParseDeepSeekResults(t *testing.T) {
	results, err := parseDeepSeekResults(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parseDeepSeekResults: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("期望 3 条，得到 %d", len(results))
	}

	// 首条：有 snippet 用它；date 为 null → 空串（omitempty 不输出）
	r0 := results[0]
	if r0.Snippet != "在 Rust 中，异步编程依赖于 future 的概念。" {
		t.Errorf("r0.Snippet = %q", r0.Snippet)
	}
	if r0.Source != "deepseek" {
		t.Errorf("r0.Source = %q", r0.Source)
	}
	if r0.SiteName != "Example 文档站" {
		t.Errorf("r0.SiteName = %q", r0.SiteName)
	}
	if r0.Authority == nil || *r0.Authority != 4 {
		t.Errorf("r0.Authority = %v, want 4", r0.Authority)
	}
	want0 := 2.0248324871063232 / 3.0
	if r0.Score != want0 {
		t.Errorf("r0.Score = %v, want %v", r0.Score, want0)
	}
	if r0.Date != "" {
		t.Errorf("r0.Date = %q, want 空（date:null）", r0.Date)
	}

	// 第二条：snippet 缺失回退 summary；date 透传
	r1 := results[1]
	if r1.Snippet != "只有 summary，snippet 缺失时回退用它。" {
		t.Errorf("r1.Snippet = %q", r1.Snippet)
	}
	if r1.Date != "2025-11-13T16:00:00+00:00" {
		t.Errorf("r1.Date = %q", r1.Date)
	}
	// authority=-1（未知）→ nil（不输出）
	if r1.Authority != nil {
		t.Errorf("r1.Authority = %v, want nil（-1 未知不输出）", r1.Authority)
	}

	// 第三条：评分归一化截断到 1.0；authority=0 应如实输出（指针非 nil）
	r2 := results[2]
	if r2.Score != 1.0 {
		t.Errorf("r2.Score = %v, want 1.0", r2.Score)
	}
	if r2.Authority == nil || *r2.Authority != 0 {
		t.Errorf("r2.Authority = %v, want 0", r2.Authority)
	}

	// 序列化时空 date 应被 omitempty 省略
	raw, _ := json.Marshal(r0)
	if strings.Contains(string(raw), `"date"`) {
		t.Errorf("空 date 不应序列化: %s", raw)
	}
}
