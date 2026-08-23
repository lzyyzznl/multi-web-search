package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

// EngineStatus 各引擎执行状态.
type EngineStatus struct {
	Status      string  `json:"status"`
	Results     int     `json:"results"`
	LatencyMs   int64   `json:"latency_ms"`
	Error       string  `json:"error,omitempty"`
}

// SearchResult 聚合搜索结果.
type SearchResult = engines.SearchResult

// SearchResponse 统一输出结构.
type SearchResponse struct {
	Query        string                  `json:"query"`
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
}

// defaultNum 每个引擎默认请求条数.
const defaultNum = 10

// doSearch 是核心编排函数：并行调用所有可用引擎，收集结果后去重排序.
func doSearch(query string, opts SearchOptions) (*SearchResponse, error) {
	// 缓存命中直接返回，节省付费 API 调用.
	if !opts.NoCache {
		if cached, ok := loadCache(query); ok {
			cached.Meta.DurationMs = 0
			return cached, nil
		}
	}

	enabled := DetectEnabledEngines()
	if len(opts.Engines) > 0 {
		// 只跑用户指定的引擎（需已配置）.
		var chosen []string
		wanted := make(map[string]bool)
		for _, name := range opts.Engines {
			wanted[name] = true
		}
		for _, name := range enabled {
			if wanted[name] {
				chosen = append(chosen, name)
			}
		}
		enabled = chosen
	}
	if len(enabled) == 0 {
		return nil, fmt.Errorf("未检测到任何搜索引擎的 API Key，请至少配置一个环境变量或指定引擎")
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

	merged := mergeResults(allResults)
	duration := time.Since(start).Milliseconds()

	resp := &SearchResponse{
		Query: query,
		Meta: SearchMeta{
			TotalRaw:    totalRaw,
			TotalUnique: len(merged),
			DurationMs:  duration,
		},
		EngineStatus: engineStatus,
		Results:      merged,
	}
	if !opts.NoCache {
		saveCache(query, resp)
	}
	return resp, nil
}
