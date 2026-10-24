package cmd

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

// EngineStatus 各引擎执行状态.
type EngineStatus struct {
	Status    string `json:"status"`
	Results   int    `json:"results"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// SearchResult 聚合搜索结果.
type SearchResult = engines.SearchResult

// SearchResponse 统一输出结构.
type SearchResponse struct {
	Query string `json:"query"`
	// Primary 本次生效的主引擎；未配置或未参与检索时省略.
	Primary      string                  `json:"primary,omitempty"`
	Meta         SearchMeta              `json:"meta"`
	EngineStatus map[string]EngineStatus `json:"engine_status"`
	Results      []SearchResult          `json:"results"`
}

// SearchMeta 元信息.
type SearchMeta struct {
	TotalRaw    int   `json:"total_raw"`
	TotalUnique int   `json:"total_unique"`
	DurationMs  int64 `json:"duration_ms"`
}

// searchTimeout 单次搜索的总超时，超时的引擎将被标记为错误而非阻塞整体.
const searchTimeout = 8 * time.Second

// SearchOptions 搜索可配置参数.
type SearchOptions struct {
	// Num 每个引擎请求的条数（默认 10）.
	Num int
	// Engines 限定使用的引擎，空则用所有已配置引擎.
	Engines []string
	// Timeout 整体超时秒数（默认 8）.
	Timeout time.Duration
	// NoCache 跳过缓存.
	NoCache bool
	// Primary 主引擎名（默认 deepseek）；为空表示不设主引擎优先级.
	Primary string
}

// defaultPrimary 默认主引擎：DeepSeek 搜索 + Fetch 能力均由同一个 Key 提供.
const defaultPrimary = "deepseek"

// defaultNum 每个引擎默认请求条数.
const defaultNum = 10

// doSearch 是核心编排函数：并行调用所有可用引擎，收集结果后去重排序.
func doSearch(query string, opts SearchOptions) (*SearchResponse, error) {
	enabled := resolveEngines(DetectEnabledEngines(), opts.Engines, opts.Primary)
	if len(enabled) == 0 {
		return nil, fmt.Errorf("未检测到任何搜索引擎的 API Key，请至少配置一个环境变量或指定引擎")
	}

	// 主引擎只有在本次真正参与检索时才生效；未配置则退化为普通多引擎聚合.
	primary := opts.Primary
	if primary == "" || !containsEngine(enabled, primary) {
		primary = ""
	}

	// 缓存命中直接返回，节省付费 API 调用。签名覆盖引擎集合 / 主引擎 / 条数，
	// 避免不同检索参数复用同一份结果（尤其是主引擎优先级不同的响应）.
	sig := cacheSignature(enabled, primary, opts.Num)
	if !opts.NoCache {
		if cached, ok := loadCache(query, sig); ok {
			cached.Meta.DurationMs = 0
			return cached, nil
		}
	}

	num := opts.Num
	if num <= 0 {
		num = defaultNum
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = searchTimeout
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var mu sync.Mutex
	var allResults [][]SearchResult
	engineStatus := make(map[string]EngineStatus)

	var wg sync.WaitGroup
	for _, name := range enabled {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			cb := NewCircuitBreaker(name)
			if cb.IsOpen() {
				mu.Lock()
				engineStatus[name] = EngineStatus{Status: "circuit_open", Error: cb.Status()}
				mu.Unlock()
				return
			}

			engineStart := time.Now()
			results, err := callEngine(ctx, name, query, num)
			latency := time.Since(engineStart).Milliseconds()

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				engineStatus[name] = EngineStatus{
					Status:    "error",
					LatencyMs: latency,
					Error:     err.Error(),
				}
				return
			}
			cb.RecordSuccess()
			allResults = append(allResults, results)
			engineStatus[name] = EngineStatus{
				Status:    "ok",
				Results:   len(results),
				LatencyMs: latency,
			}
		}(name)
	}
	wg.Wait()

	totalRaw := 0
	for _, s := range engineStatus {
		totalRaw += s.Results
	}

	merged := mergeResults(allResults, primary)
	duration := time.Since(start).Milliseconds()

	resp := &SearchResponse{
		Query:   query,
		Primary: primary,
		Meta: SearchMeta{
			TotalRaw:    totalRaw,
			TotalUnique: len(merged),
			DurationMs:  duration,
		},
		EngineStatus: engineStatus,
		Results:      merged,
	}
	if !opts.NoCache {
		saveCache(query, sig, resp)
	}
	return resp, nil
}

// resolveEngines 把用户指定的引擎集合解析为实际参与检索的已配置引擎.
//
// 特殊 token：all（全部已配置引擎）、aux（除主引擎外的辅助引擎）；
// 其余按引擎名精确匹配，未配置的名字忽略.
func resolveEngines(enabled, requested []string, primary string) []string {
	if len(requested) == 0 {
		sorted := append([]string(nil), enabled...)
		sort.Strings(sorted)
		return sorted
	}
	wantAll, wantAux := false, false
	wanted := make(map[string]bool, len(requested))
	for _, name := range requested {
		switch name {
		case "all":
			wantAll = true
		case "aux":
			wantAux = true
		default:
			wanted[name] = true
		}
	}
	if wantAll {
		sorted := append([]string(nil), enabled...)
		sort.Strings(sorted)
		return sorted
	}

	chosen := make([]string, 0, len(enabled))
	for _, name := range enabled {
		if wanted[name] || (wantAux && name != primary) {
			chosen = append(chosen, name)
		}
	}
	sort.Strings(chosen)
	return chosen
}

// containsEngine 判断引擎是否在列表中.
func containsEngine(engines []string, name string) bool {
	for _, engine := range engines {
		if engine == name {
			return true
		}
	}
	return false
}
