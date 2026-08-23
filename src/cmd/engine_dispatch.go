package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

// callEngine 根据引擎名称路由到对应实现.
func callEngine(ctx context.Context, name, query string, num int) ([]engines.SearchResult, error) {
	apiKey := os.Getenv(envVarMap[name])
	if apiKey == "" {
		return nil, fmt.Errorf("%s: API Key 未配置", name)
	}

	var engine engines.Engine
	switch name {
	case "serper":
		engine = engines.NewSerper(apiKey)
	case "baidu":
		engine = engines.NewBaidu(apiKey)
	case "brave":
		engine = engines.NewBrave(apiKey)
	case "tavily":
		engine = engines.NewTavily(apiKey)
	case "aliyun-iqs":
		engine = engines.NewAliyunIQS(apiKey)
	case "exa":
		engine = engines.NewExa(apiKey)
	default:
		return nil, fmt.Errorf("未知引擎: %s", name)
	}

	results, err := engine.Search(ctx, query, num)
	if err != nil {
		// 检查是否需要熔断
		errStr := err.Error()
		cb := NewCircuitBreaker(name)

		// 429/403 = 配额耗尽, 立即熔断 24h.
		if isQuotaError(errStr) {
			cb.Open("quota_exhausted", errStr)
		} else if isTransientError(errStr) {
			// 5xx / 网络 / 超时：递增失败计数，连续超阈值后熔断.
			cb.RecordFailure(errStr)
		}
		return nil, err
	}
	return results, nil
}

// isQuotaError 判断错误是否由配额耗尽引起.
func isQuotaError(err string) bool {
	return containsAny(err, "429", "403", "quota", "rate limit", "insufficient_quota")
}

// isTransientError 判断是否为可重试的瞬时错误（5xx/网络/超时）.
func isTransientError(err string) bool {
	// 匹配 "HTTP 5xx" 或 "HTTP 500"-"HTTP 599"
	for _, code := range []string{"500", "501", "502", "503", "504", "505", "508", "521", "522", "523", "524", "529", "530"} {
		if strings.Contains(err, "HTTP "+code) {
			return true
		}
	}
	return containsAny(err, "timeout", "deadline exceeded", "connection", "refused", "unreachable", "temporary", "EOF")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
