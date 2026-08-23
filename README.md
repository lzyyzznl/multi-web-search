# multi-web-search

多引擎搜索聚合插件。统一入口并行调用多个搜索引擎，自动去重合并排序。

## 支持的引擎

| 引擎 | 环境变量 | 说明 |
|------|---------|------|
| Serper (Google) | `SERPER_API_KEY` | Google 搜索结果 |
| 百度搜索 | `BAIDU_API_KEY` | 中文搜索 |
| Brave Search | `BRAVE_API_KEY` | 国际/英文内容 |
| Tavily | `TAVILY_API_KEY` | AI 优化搜索 |
| 阿里云 IQS | `ALIYUN_IQS_API_KEY` | 智能通用搜索 |
| Exa | `EXA_API_KEY` | 语义/AI 搜索 |

配置对应的 API Key 后即可使用，无需额外配置。至少需要配置一个引擎。

## 安装

### Claude Code

```bash
# 从 marketplace 安装
claude add plugin lzyyzznl/multi-web-search

# 或手动克隆
git clone git@github.com:lzyyzznl/multi-web-search.git ~/.claude/plugins/multi-web-search
```

### Codex

```bash
# 从 marketplace 安装
codex plugin marketplace add lzyyzznl/multi-web-search --ref main
codex plugin install multi-web-search

# 或手动克隆
git clone git@github.com:lzyyzznl/multi-web-search.git ~/.agents/plugins/multi-web-search
```

### 手动添加 marketplace（可选）

编辑 `~/.claude/settings.json`，在 `extraKnownMarketplaces` 中加入：

```json
{
  "extraKnownMarketplaces": {
    "multi-web-search-marketplace": {
      "source": {
        "source": "url",
        "url": "git@github.com:lzyyzznl/multi-web-search.git"
      }
    }
  }
}
```

## 使用

```bash
# 搜索（默认漂亮输出）
multi-web-search search "Rust 异步编程"

# JSON 格式
multi-web-search search "cloud computing" --json

# 管道友好
multi-web-search search "量子计算" --raw | jq '.results[] | {title, url, score}'

# 指定引擎 + 条数 + 跳过缓存
multi-web-search search "Rust" --engines serper,exa --num 5 --no-cache

# 自定义整体超时（秒）
multi-web-search search "大模型" --timeout 15

# 查看各引擎状态
multi-web-search status
```

### 参数说明

| 参数 | 说明 | 默认 |
|------|------|------|
| `--num N` | 每个引擎返回条数 | 10 |
| `--engines a,b` | 只用指定引擎（逗号分隔） | 全部已配置 |
| `--timeout N` | 整体超时（秒），超时引擎标记失败 | 8 |
| `--no-cache` | 跳过磁盘缓存，强制实时搜索 | 关 |
| `--json` / `--raw` | JSON 输出 | 漂亮输出 |

结果默认缓存 15 分钟（按 query 哈希存于 `~/.cache/multi-web-search/`），相同 query 重复搜索直接命中缓存，节省付费 API 调用。

### 输出示例

```
搜索 "Rust 异步编程" — 来自 5 个引擎

  ✅ serper      (10 条, 1680ms)
  ✅ baidu       (10 条, 933ms)
  ✅ brave       (10 条, 773ms)
  ✅ tavily      (10 条, 1174ms)
  ✅ aliyun-iqs  (10 条, 1192ms)

  1. 16 Rust学习笔记-异步编程(async/await/Future)
     https://zhuanlan.zhihu.com/p/611587154
     async/await 是 Rust 的异步编程模型...  [0.77 | tavily]

  2. GitHub - rustcn-org/async-book
     https://github.com/rustcn-org/async-book
     高质量手翻 Asynchronous Programming in Rust...  [0.71 | brave]
  ...

  📊 50 条原始结果，去重后 37 条唯一结果 (1.68s)
```

## 特性

- **全并行** — goroutine 同时调用所有可用引擎，互不影响
- **自动发现** — 自动检测环境变量中的 API Key
- **整体超时** — 单次搜索总超时（默认 8s），超时引擎标记失败不阻塞整体
- **熔断保护** — 配额耗尽(429/403)或连续失败后熔断 24h，到期自动恢复（进程级锁 + 原子写保护状态文件）
- **磁盘缓存** — 相同 query 15 分钟内命中缓存，节省付费 API 调用
- **智能去重** — URL 归一化后去重，保留最高评分
- **综合排序** — 按引擎权重 × 评分降序排列
- **跨会话持久化** — 熔断状态保存在 `~/.config/multi-web-search/circuit.json`
- **多平台构建** — build.sh 交叉编译 linux/darwin/windows (amd64/arm64)

## 熔断说明

每个引擎独立维护熔断器：

- 429/403 → 立即熔断 24 小时（配额已耗尽）
- 5xx/超时 → 连续失败 3 次后熔断 24 小时
- 到期自动恢复，无需人工干预

```bash
# 查看各引擎状态
multi-web-search status
```

## 开发

```bash
cd src
go mod tidy
bash build.sh   # 本地构建 + 多平台交叉编译到 dist/
```

构建产物自动部署到 `skills/multi-web-search/`、`bin/` 和 `dist/`，版本号由 git describe 注入。

## 插件结构

```
multi-web-search/
├── .claude-plugin/          # Claude Code 插件配置
├── .codex-plugin/           # Codex 插件配置
├── src/                     # Go 源码
│   ├── cmd/
│   │   ├── root.go          # CLI 入口（search + status）
│   │   ├── search.go        # 全并行编排器
│   │   ├── merge.go         # URL 去重 + 排序
│   │   ├── circuit.go       # 熔断器
│   │   ├── env.go           # API Key 自动检测
│   │   └── engines/         # 各引擎实现
│   ├── main.go
│   └── build.sh
├── skills/multi-web-search/ # Claude/Codex 技能文档 + 二进制
├── bin/                     # PATH 入口
├── hooks/                   # 生命周期钩子
└── AGENTS.md                # 项目配置
```

## License

MIT
