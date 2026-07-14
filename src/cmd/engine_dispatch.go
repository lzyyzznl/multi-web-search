package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/lzyyzznl/multi-web-search/cmd/engines"
)

// callEngine 根据引擎名称路由到对应实现.
func callEngine(ctx context.Context, name, query string) ([]engines.SearchResult, error) {
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
	default:
		return nil, fmt.Errorf("未知引擎: %s", name)
	}

	results, err := engine.Search(ctx, query)
	if err != nil {
		// 检查是否需要熔断
		errStr := err.Error()
		cb := NewCircuitBreaker(name)

		// 429/403 = 配额耗尽, 直接熔断
		if isQuotaError(errStr) {
			cb.Open("quota_exhausted", errStr)
		}
		return nil, err
	}
	return results, nil
}

// isQuotaError 判断错误是否由配额耗尽引起.
func isQuotaError(err string) bool {
	return containsAny(err, "429", "403", "quota", "rate limit", "insufficient_quota")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) <= len(s) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
