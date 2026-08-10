# Xiaohongshu MCP Readonly

[![CI](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/ci.yml/badge.svg)](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/ci.yml)
[![CodeQL](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/codeql.yml/badge.svg)](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/codeql.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A conservative, localhost-only MCP server that lets GitHub Copilot CLI read
public Xiaohongshu and RedNote content.

> [!IMPORTANT]
> This fork exposes exactly six read-only MCP tools. It does not expose
> publishing, commenting, replying, liking, favoriting, notification, or cookie
> deletion tools. The websites can still record logins, searches, and page
> views performed by the automated browser.

This project is an unofficial modified fork of
[`xpzouying/xiaohongshu-mcp`](https://github.com/xpzouying/xiaohongshu-mcp).
See [NOTICE](NOTICE) and [docs/upstream.md](docs/upstream.md).

## Features

- Six positively registered read-only MCP tools.
- Separate Xiaohongshu and RedNote URLs, cookies, and browser identity seeds.
- QR login in Copilot CLI or at `http://127.0.0.1:18060/login`.
- Fresh-browser verification before login is reported as successful.
- One browser operation at a time, with cooldown and random jitter.
- Comment and reply limits with forced slow scrolling.
- Loopback-only listening, local Host/Origin checks, and JSON-only POSTs.
- Linux ARM64 fallback to an installed Chromium or Playwright Chromium.

## Requirements

- Go 1.25 or later.
- A supported bundled browser platform, or a local Chromium-compatible browser.
- GitHub Copilot CLI for the intended MCP integration.
- A dedicated Xiaohongshu or RedNote account is strongly recommended.

Bundled browser builds currently cover Linux AMD64, macOS ARM64, and Windows
AMD64. Other platforms must provide Chromium through `XHS_BROWSER_BIN`, the
system `PATH`, or an existing Playwright Chromium installation.

On supported platforms, startup first tries a bundled CloakBrowser archive from
`cdn.one-world.ai`, inherited from upstream and checked against repository-
pinned SHA256 hashes. If that download is unavailable, startup falls back to
system Chromium and then Playwright Chromium. Set `XHS_BROWSER_BIN` to always
use a browser you manage locally.

## Quick start

```bash
git clone https://github.com/sinmentis/xiaohongshu-mcp-readonly.git
cd xiaohongshu-mcp-readonly

go build -trimpath -o bin/xiaohongshu-mcp-readonly .

mkdir -p "$HOME/.local/share/xiaohongshu-mcp-readonly"

COOKIES_PATH="$HOME/.local/share/xiaohongshu-mcp-readonly/cookies.json" \
  ./bin/xiaohongshu-mcp-readonly \
  -site rednote \
  -port 127.0.0.1:18060
```

Use `-site rednote` for overseas RedNote accounts and `-site xiaohongshu` for
mainland Xiaohongshu accounts.

Open the local login page:

```text
http://127.0.0.1:18060/login
```

Then register the MCP server:

```bash
copilot mcp add \
  --transport http \
  --timeout 900000 \
  --tools check_login_status,get_login_qrcode,list_feeds,search_feeds,get_feed_detail,user_profile \
  xiaohongshu-readonly \
  http://127.0.0.1:18060/mcp
```

Inside Copilot CLI, run `/mcp`, then ask:

```text
Use xiaohongshu-readonly to check whether I am logged in.
```

## MCP tools

| Tool | Purpose |
| --- | --- |
| `check_login_status` | Check the current site session. |
| `get_login_qrcode` | Get the current login or device-verification QR code. |
| `list_feeds` | Read the home feed. |
| `search_feeds` | Search public notes. |
| `get_feed_detail` | Read a note, media metadata, and comments. |
| `user_profile` | Read a public user profile and visible notes. |

The MCP and HTTP surfaces are both tested against explicit allowlists.

## Access protection

Default browser access is deliberately slow:

- one operation at a time;
- at least 30 seconds between completed operations;
- up to 15 seconds of additional random delay;
- at most 50 top-level comments per request;
- reply expansion limited to threads with at most 10 replies;
- slow comment scrolling only.

These controls reduce accidental bursts. They cannot guarantee that an account
will never be challenged or restricted. Avoid unattended bulk collection.

## Documentation

- [Copilot CLI setup](docs/copilot-cli.md)
- [HTTP interface](docs/api.md)
- [Architecture](docs/architecture.md)
- [Security model](docs/security-model.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Maintenance and releases](docs/maintenance.md)
- [Upstream relationship](docs/upstream.md)

## Development

```bash
go fmt ./...
go vet ./...
go test ./...
go test ./... -race
```

Browser integration tests use the `integration` build tag and are compiled,
but not run, in CI.

Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

## Security and privacy

Cookie files contain full account sessions and are stored with mode `0600`.
Treat them like passwords. The server intentionally refuses non-loopback
listeners and cross-origin browser requests, but local processes running as the
same user remain inside the trust boundary.

Report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/sinmentis/xiaohongshu-mcp-readonly/security/advisories/new).
See [SECURITY.md](SECURITY.md).

## Disclaimer

This project is not affiliated with Xiaohongshu, RedNote, or their operators.
Those names are third-party trademarks used only to describe compatibility.
Automated access may be restricted by platform terms or local law. Users are
responsible for their accounts, data handling, access frequency, and compliance.

## License

Apache License 2.0. The original license and attribution are preserved in
[LICENSE](LICENSE) and [NOTICE](NOTICE).
