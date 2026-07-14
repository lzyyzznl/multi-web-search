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

// doSearch 是核心编排函数：并行调用所有可用引擎，收集结果后去重排序.
func doSearch(query string) (*SearchResponse, error) {
	enabled := DetectEnabledEngines()
	if len(enabled) == 0 {
		return nil, fmt.Errorf("未检测到任何搜索引擎的 API Key，请至少配置一个环境变量")
	}

	start := time.Now()
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
			results, err := callEngine(context.Background(), name, query)
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

	return &SearchResponse{
		Query: query,
		Meta: SearchMeta{
			TotalRaw:    totalRaw,
			TotalUnique: len(merged),
			DurationMs:  duration,
		},
		EngineStatus: engineStatus,
		Results:      merged,
	}, nil
}
