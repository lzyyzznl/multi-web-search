# 2026-09-02 multi-web-search key 子命令设计

## 背景

multi-web-search 的六个引擎 API Key 均从环境变量读取（见 `cmd/env.go` 的 `envVarMap`）。当前添加 Key 需用户手动写入系统环境变量（Windows 注册表 / Linux shell 配置文件），跨平台姿势不同、易错。本设计新增一条 Go CLI 能力，用统一命令把 Key 写入系统环境变量，区分平台持久化机制。

## 需求

- 新增子命令：`multi-web-search key add <engine> <api_key>`
- `<engine>` 必须是已支持引擎名（serper/baidu/brave/tavily/aliyun-iqs/exa），否则报错并列出可用项
- `<api_key>` 非空，否则报错
- 写入系统环境变量（用户级，无需管理员），并让当前进程立即生效
- 区分 Windows / Linux / macOS 的持久化机制
- 重复 add = 覆盖（幂等）
- 不新增 Go 依赖（保持 go.mod 仅 cobra）

## 平台持久化机制

| 平台 | 落点 | 说明 |
|------|------|------|
| Windows | `HKCU\Environment` 注册表值 | `reg add "HKCU\Environment" /v <VAR> /t REG_SZ /d <key> /f`，用户级无需管理员；随后广播 `WM_SETTINGCHANGE` 让 explorer/新进程立即感知 |
| Linux/macOS | `~/.profile` | 追加/替换 `export <VAR>='<key>'` 行，单引号转义（`'` → `'\''`）防注入，原子写（tmp + rename）；新登录 shell 读取 |
| 其他 | 报错 unsupported | repo 仅分发 linux/windows amd64 |

当前进程 `os.Setenv` 让同一进程内后续 `search`/`status` 立即读到，不依赖重开终端。

## 命令

```
multi-web-search key add <engine> <api_key>
```

- `add` 挂在 rootCmd，参数 2 个（cobra `ExactArgs(2)`）
- 引擎名校验用 `envVarMap`（与 status/search 共用单一事实源）
- 成功输出：`✅ 已写入系统环境变量 SERPER_API_KEY（当前会话立即生效）`

## 文件结构

- `src/cmd/key.go` — cobra 命令定义 + 引擎校验 + os.Setenv + 调平台函数
- `src/cmd/key_windows.go` — `//go:build windows`：reg add + WM_SETTINGCHANGE 广播
- `src/cmd/key_unix.go` — `//go:build !windows`：~/.profile 行级更新
- `src/cmd/key_test.go` — 引擎校验 + profile 写入逻辑（t.TempDir，不碰真实 HOME）

## 测试

- 引擎白名单：合法名通过、非法名报错
- key 非空校验
- profile 写入：不存在的文件创建、已存在追加/替换、特殊字符（含单引号）转义正确、原子写不残留 tmp
- Windows 注册表逻辑不单元测试（依赖 reg，本机实测覆盖）

## 文档

- `skills/multi-web-search/SKILL.md` 加「配置 API Key」段：命令 + 引擎表 + 平台说明 + `status` 验证
- `README.md` 使用段加 `key add` 示例

## 分发与验证

1. `go test ./...` 全绿
2. 交叉编译 windows/amd64 + linux/amd64，替换 `skills/.../scripts/` 就地二进制
3. 本机实测：用某引擎**现值重写**（不改变值）验证 `key add` → `status` 生效链路
4. commit + push，同步本地技能目录
