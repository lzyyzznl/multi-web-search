---
name: multi-web-search
description: 多引擎搜索聚合，六个引擎并行搜索、自动去重合并排序。适用于需要多源聚合、国内可访问搜索、指定引擎搜索的场景。
---

# Multi Web Search

## 何时触发

用户请求「搜索」「查找资料」「web search」「research」等搜索意图时触发。适用于：
- 需要多源聚合结果（单一引擎可能遗漏）
- 需要中文搜索（百度、阿里云 IQS）
- 需要绕过地域限制的搜索引擎（Brave 在国内直连被墙）
- 用户指定使用搜索引擎

## 执行

统一入口（跨平台同名，按当前系统自动选择就地二进制）：

```bash
scripts/multi-web-search search "query"          # 搜索
scripts/multi-web-search status                   # 检查引擎状态
scripts/multi-web-search --version                # 版本
```

`scripts/` 下携带各平台就地二进制（`multi-web-search.exe` Windows / `multi-web-search-linux` Linux），由 `scripts/multi-web-search` 启动器按平台选择执行。darwin 暂无分发二进制。

## 输出格式

```bash
scripts/multi-web-search search "query" --json   # JSON（推荐，解析用）
scripts/multi-web-search search "query" --raw    # 纯 JSON，管道友好
```

## 参数

| 参数 | 说明 | 默认 |
|------|------|------|
| `--num N` | 每个引擎返回条数 | 10 |
| `--engines a,b` | 只用指定引擎，逗号分隔 | 全部已配置 |
| `--timeout N` | 整体超时（秒） | 8 |
| `--no-cache` | 跳过缓存，强制实时搜索 | 关 |

## 引擎与 API Key

支持的引擎需要对应环境变量：

| 引擎 | 环境变量 |
|------|---------|
| Serper (Google) | `SERPER_API_KEY` |
| 百度搜索 | `BAIDU_API_KEY` |
| Brave Search | `BRAVE_API_KEY` |
| Tavily | `TAVILY_API_KEY` |
| 阿里云 IQS | `ALIYUN_IQS_API_KEY` |
| Exa | `EXA_API_KEY` |

至少需要配置一个引擎。代理自动检测：已配置本地代理（Windows 注册表 / Linux 常见端口）时自动走代理，否则直连。

## 输出解析

建议使用 `--raw` 获取纯 JSON 解析：

```bash
scripts/multi-web-search search "query" --raw | jq '.results[] | {title, url, score}'
scripts/multi-web-search search "query" --raw | jq '.engine_status'
```

JSON 结构：
```json
{
  "query": "...",
  "meta": { "total_raw": N, "total_unique": N, "duration_ms": N },
  "engine_status": { "engine_name": { "status": "ok|error", "results": N, "latency_ms": N } },
  "results": [{ "title": "...", "url": "...", "snippet": "...", "source": "engine", "score": N }]
}
```

## 熔断

每个引擎独立熔断。429/403 或连续失败超过阈值后自动熔断 24 小时，到期自动恢复。用 `status` 命令查看状态。
