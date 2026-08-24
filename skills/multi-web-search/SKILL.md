---
name: multi-web-search
description: "聚合搜索插件，并行调用 Serper(Google)/Baidu/Brave/Tavily/阿里云IQS/Exa 六个搜索引擎，自动去重合并排序。触发关键词：搜索、查资料、找文档、web search、research。"
---

# Multi Web Search

多引擎搜索聚合，统一入口，自动发现 API Key。

## 用法

```
同级 `scripts/multi-web-search` 是平台感知启动器：首次运行自动下载当前平台对应 架构二进制并缓存到 `~/.cache/multi-web-search/`；可用 `MULTI_WEB_SEARCH_VERSION` 锁版本、`MULTI_WEB_SEARCH_BIN` 指向本地二进制。
```

```bash
scripts/multi-web-search search <query>          # 默认漂亮输出
scripts/multi-web-search search <query> --json   # JSON 格式
scripts/multi-web-search search <query> --raw    # 管道友好
scripts/multi-web-search search <query> --engines serper,exa --num 5 --no-cache
scripts/multi-web-search status                  # 查看引擎状态
```

## 参数

`--num N` 每引擎条数（默认10）；`--engines a,b` 只用指定引擎；`--timeout N` 超时秒数（默认8）；`--no-cache` 跳过缓存。结果默认缓存15分钟。

## 支持的引擎

| 引擎 | 环境变量 |
|------|---------|
| Serper (Google) | `SERPER_API_KEY` |
| 百度搜索 | `BAIDU_API_KEY` |
| Brave Search | `BRAVE_API_KEY` |
| Tavily | `TAVILY_API_KEY` |
| 阿里云 IQS | `ALIYUN_IQS_API_KEY` |
| Exa | `EXA_API_KEY` |

配置对应 API Key 后即可使用，无需额外配置。至少需要配置一个引擎。

## jq 管道

```bash
scripts/multi-web-search search "query" --raw | jq '.results[] | {title, url, score}'
scripts/multi-web-search search "query" --raw | jq '.engine_status'
```

## 熔断说明

每个引擎独立熔断。429/403（配额耗尽）或连续失败超过阈值后自动熔断 24 小时，到期自动恢复。
运行 `scripts/multi-web-search status` 查看各引擎状态。
