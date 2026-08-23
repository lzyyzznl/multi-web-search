# multi-web-search

多引擎搜索聚合 Claude Code 插件。自动检测可用搜索引擎的 API Key，全并行搜索后去重合并、综合评分排序。

## 支持的引擎

| 引擎 | 环境变量 |
|------|---------|
| Serper (Google) | `SERPER_API_KEY` |
| 百度搜索 | `BAIDU_API_KEY` |
| Brave Search | `BRAVE_API_KEY` |
| Tavily | `TAVILY_API_KEY` |
| 阿里云 IQS | `ALIYUN_IQS_API_KEY` |
| Exa | `EXA_API_KEY` |

## 使用

```bash
multi-web-search search <query>          # 搜索
multi-web-search status                   # 查看引擎状态
multi-web-search --version                # 版本
```

## 开发

```bash
cd src && bash build.sh
```
