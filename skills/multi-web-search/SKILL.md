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

统一入口（跨平台同名，按当前系统自动选择就地二进制，缺失时回落 Python）：

```bash
scripts/multi-web-search search "query"          # 搜索
scripts/multi-web-search status                   # 检查引擎状态
scripts/multi-web-search --version                # 版本
```

实现有两种，启动器自动选择：

| 实现 | 文件 | 说明 |
|------|------|------|
| 就地二进制（首选） | `multi-web-search.exe`（Windows）/ `multi-web-search-linux`（Linux） | 随仓库携带；也认 release 资产名（`multi-web-search-windows-amd64.exe` 等） |
| Python 回落 | `multi-web-search.py` | 二进制缺失时启用，纯标准库、无需安装依赖 |

两种实现**共享同一套磁盘状态**（缓存 / 熔断文件）与输出格式，可互换使用。唯一差异：`key add` 需要平台级环境变量持久化，只有二进制实现；回落实现会提示改用二进制或手动 `export`。

## 输出格式

```bash
scripts/multi-web-search search "query" --json   # JSON（推荐，解析用）
scripts/multi-web-search search "query" --raw    # 纯 JSON，管道友好
```

## 参数

| 参数 | 说明 | 默认 |
|------|------|------|
| `--num N` | 每个引擎返回条数 | 10 |
| `--engines a,b` | 只用指定引擎，逗号分隔（引擎名见下表） | 全部已配置 |
| `--timeout N` | 整体超时（秒） | 8 |
| `--no-cache` | 跳过缓存，强制实时搜索 | 关 |

## 引擎与 API Key

| 引擎名（`--engines` 用） | 环境变量 | 说明 |
|------|---------|------|
| `serper` | `SERPER_API_KEY` | Serper (Google) |
| `baidu` | `BAIDU_API_KEY` | 百度千帆 AI 搜索 |
| `brave` | `BRAVE_API_KEY` | Brave Search |
| `tavily` | `TAVILY_API_KEY` | Tavily |
| `aliyun-iqs` | `ALIYUN_IQS_API_KEY` | 阿里云信息查询服务 IQS |
| `exa` | `EXA_API_KEY` | Exa |

至少需要配置一个引擎。代理自动检测：已配置本地代理（Windows 注册表 / Linux 常见端口）时自动走代理，否则直连；`MULTI_WEB_SEARCH_NO_PROXY=1` 强制直连。

## 输出解析

建议使用 `--raw` 获取 JSON 解析：

```bash
scripts/multi-web-search search "query" --raw | jq '.results[] | {title, url, score}'
scripts/multi-web-search search "query" --raw | jq '.engine_status'
```

JSON 结构：
```json
{
  "query": "...",
  "meta": { "total_raw": N, "total_unique": N, "duration_ms": N },
  "engine_status": { "engine_name": { "status": "ok|error|circuit_open", "results": N, "latency_ms": N, "error": "..." } },
  "results": [{ "title": "...", "url": "...", "snippet": "...", "source": "engine", "score": N,
                "engine_scores": { "engine": N } }]
}
```

缓存命中时 `engine_status` 为 `null`、`duration_ms` 为 0。

## 熔断与缓存

- 每引擎独立熔断：429/403 立即熔断 24 小时；5xx/网络/超时连续 3 次熔断 24 小时，到期自动恢复。用 `status` 查看。
- 结果缓存 15 分钟于 `~/.cache/multi-web-search/`（按 query 哈希），熔断状态在 `~/.config/multi-web-search/circuit.json`。
