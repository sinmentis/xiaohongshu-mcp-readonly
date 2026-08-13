# 小红书 MCP 只读版

**简体中文** | [English](README.en.md)

[![CI](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/ci.yml/badge.svg)](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/ci.yml)
[![CodeQL](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/codeql.yml/badge.svg)](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/codeql.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

这是一个只跑在本机、只提供读取功能的 MCP 服务。它可以让 GitHub
Copilot CLI 等 AI Agent 读取小红书或 RedNote 上的公开内容。

> [!IMPORTANT]
> 这个版本只开放 6 个只读 MCP 工具。不能发笔记、评论、回复、点赞、收藏，
> 也不能读取通知或删除 Cookie。自动化浏览器进行登录、搜索和浏览时，平台仍然
> 可能记录这些操作。

本项目基于
[`xpzouying/xiaohongshu-mcp`](https://github.com/xpzouying/xiaohongshu-mcp)
修改，并非官方项目。上游来源和授权信息见 [NOTICE](NOTICE) 与
[docs/upstream.md](docs/upstream.md)。

## 能做什么

- 提供 6 个明确注册的只读 MCP 工具。
- AI Agent 连接后会自动收到使用说明，不用额外写一大段提示词。
- 小红书和 RedNote 使用各自的网址、Cookie 和浏览器身份。
- 可以在 Copilot CLI 中扫码登录，也可以打开
  `http://127.0.0.1:18060/login` 登录。
- 只有用全新浏览器确认登录有效后，才会告诉 Agent “已经登录”。
- 同一时间只让一个浏览器操作运行，并在操作之间留出冷却时间。
- Chromium 启动一次后会继续复用，每次读取只新开一个页面。
- 每个操作都有超时。卡住时会明确报错，不会让 Agent 一直傻等。
- `/health` 可以看到当前任务、排队数量、超时和浏览器状态。
- 评论和回复有数量限制，而且强制慢速滚动。
- 只监听本机地址，并检查 Host、Origin 和 JSON POST 请求。
- Linux ARM64 可以使用系统里已有的 Chromium 或 Playwright Chromium。

## 运行要求

- Go 1.25.12 或更高版本。
- 支持项目自带浏览器的平台，或者本机已经安装兼容 Chromium 的浏览器。
- 一个支持 Streamable HTTP MCP 的本地客户端，例如 GitHub Copilot CLI、
  Claude Code、Codex CLI 或 Cursor。
- 强烈建议使用专门的小红书或 RedNote 账号，不要拿常用主账号做自动化测试。

项目自带的浏览器目前支持 Linux AMD64、macOS ARM64 和 Windows AMD64。
其他平台需要通过 `XHS_BROWSER_BIN`、系统 `PATH`，或者已有的 Playwright
Chromium 提供浏览器。

在支持的平台上，服务会先尝试下载上游使用的 CloakBrowser 压缩包。下载地址是
`cdn.one-world.ai`，文件会用仓库里固定的 SHA256 哈希校验。下载失败时，会依次
尝试系统 Chromium 和 Playwright Chromium。如果你只想使用自己管理的浏览器，
直接设置 `XHS_BROWSER_BIN`。

## 快速开始

Linux 上有 systemd 用户会话的话，直接运行：

```bash
git clone https://github.com/sinmentis/xiaohongshu-mcp-readonly.git
cd xiaohongshu-mcp-readonly

./scripts/setup-local --site rednote --agent copilot
```

海外 RedNote 账号用 `--site rednote`，中国大陆小红书账号用
`--site xiaohongshu`。

如果你不用 Copilot CLI，可以把 `copilot` 换成 `claude`、`codex` 或
`none`。这个脚本会完成编译、安装、启动，并按需把 MCP 服务注册到 Agent。
兼容的 MCP 客户端会在初始化时自动拿到服务说明，不需要再手动复制提示词。

然后打开本地登录页：

```text
http://127.0.0.1:18060/login
```

在 Copilot CLI 里运行 `/mcp`，然后直接说：

```text
用 xiaohongshu-readonly 检查一下我有没有登录。
```

## MCP 工具

| 工具 | 用途 |
| --- | --- |
| `check_login_status` | 检查当前账号是否已经登录。 |
| `get_login_qrcode` | 获取登录或设备验证二维码。 |
| `list_feeds` | 读取首页推荐。 |
| `search_feeds` | 搜索公开笔记。 |
| `get_feed_detail` | 读取笔记、媒体信息和评论。 |
| `user_profile` | 读取公开用户资料和可见笔记。 |

MCP 和 HTTP 接口都使用明确的白名单测试，避免不小心开放写操作。

## 为什么有时看起来比较慢

这是故意的。默认规则是：

- 同一时间只运行一个浏览器操作；
- 如果前面有任务，最多排队 1 分钟；
- 两次操作之间至少等 30 秒；
- 再随机多等最多 15 秒；
- 每次最多读取 50 条顶层评论；
- 只展开回复数不超过 10 条的评论线程；
- 评论只能慢速滚动读取。

这些限制可以减少短时间内连续访问，但不能保证账号永远不会遇到验证或限制。
不建议把它当成无人值守的批量采集工具。

第一次调用浏览器相关工具时需要启动 Chromium，所以会慢一些。后面的读取会复用
同一个 Chromium，只打开新页面，不会每次都重新启动浏览器。

每个工具还有服务端超时。如果操作超时，调用方会马上收到明确错误，`/health`
会在后台清理完成前显示 `degraded`。排队中的请求也不会一直堵在已经卡死的任务
后面。

## 出问题时先看这里

先检查服务状态：

```bash
curl http://127.0.0.1:18060/health
```

它会告诉你服务是空闲、忙碌还是异常，也能看到当前操作、排队数量、剩余时间和
浏览器状态。更详细的处理方法见
[故障排查文档](docs/troubleshooting.md)。

## 更多文档

- [Copilot CLI 配置](docs/copilot-cli.md)
- [其他 AI Agent](docs/ai-agents.md)
- [HTTP 接口](docs/api.md)
- [架构说明](docs/architecture.md)
- [安全模型](docs/security-model.md)
- [故障排查](docs/troubleshooting.md)
- [维护和发布](docs/maintenance.md)
- [与上游项目的关系](docs/upstream.md)

这些详细文档目前以英文为主。

## 开发

```bash
go fmt ./...
go vet ./...
go test ./...
go test ./... -race
```

浏览器集成测试使用 `integration` build tag。CI 会确认它们能编译，但不会真的
连接网站运行。

准备提交代码前，请先看 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 安全和隐私

Cookie 文件包含完整的账号登录状态，权限设置为 `0600`。请把它当成密码，不要
上传、分享或写进日志。

服务只允许本机访问，也会拒绝跨域浏览器请求。不过，同一台机器上以同一用户运行
的其他进程仍然处在信任范围内。

如果发现安全问题，请通过
[GitHub Security Advisories](https://github.com/sinmentis/xiaohongshu-mcp-readonly/security/advisories/new)
私下报告，不要直接发公开 Issue。更多信息见 [SECURITY.md](SECURITY.md)。

## 免责声明

本项目与小红书、RedNote 及其运营方没有关联。这些名称只用于说明兼容性，相关
商标归原权利人所有。

自动化访问可能受到平台条款或当地法律限制。账号安全、数据处理、访问频率和合规
责任都由使用者自行承担。

## 开源协议

项目使用 Apache License 2.0。原项目的协议和署名保留在
[LICENSE](LICENSE) 与 [NOTICE](NOTICE) 中。
