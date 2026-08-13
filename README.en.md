# Xiaohongshu / RedNote MCP Server (Read-only)

[简体中文](README.md) | **English**

[![CI](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/ci.yml/badge.svg)](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/ci.yml)
[![CodeQL](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/codeql.yml/badge.svg)](https://github.com/sinmentis/xiaohongshu-mcp-readonly/actions/workflows/codeql.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A localhost-only, read-only Model Context Protocol (MCP) server for Xiaohongshu
and RedNote. It works with GitHub Copilot CLI, Claude Code, Codex CLI, and
Cursor to search public notes, read comments, and view user profiles.

> [!IMPORTANT]
> This server exposes exactly six read-only tools. It cannot publish, comment,
> reply, like, favorite, read notifications, or delete cookies. The platform
> can still record browser logins, searches, and page views.

![Local Xiaohongshu and RedNote MCP login page with a nonfunctional mock QR code](docs/images/login-page.webp)

> The QR code is only a preview. It cannot log in and contains no real account
> data. The actual page is available only at
> `http://127.0.0.1:18060/login`.

## Quick start

Requires Go 1.25.12 or later. A dedicated test account is recommended.

On Linux with a systemd user session:

```bash
git clone https://github.com/sinmentis/xiaohongshu-mcp-readonly.git
cd xiaohongshu-mcp-readonly
./scripts/setup-local --site rednote --agent copilot
```

Use `--site rednote` for overseas RedNote accounts and `--site xiaohongshu` for
mainland Xiaohongshu accounts. `--agent` also accepts `claude`, `codex`, or
`none`. The script builds, installs, starts, and registers the server when
requested.

Then open:

```text
http://127.0.0.1:18060/login
```

In Copilot CLI, run `/mcp`, then ask:

```text
Use xiaohongshu-readonly to check whether I am logged in.
```

The server selects a bundled browser, system Chromium, or Playwright Chromium.
Set `XHS_BROWSER_BIN` to use only a browser you manage. Bundled browser
downloads are checked against repository-pinned SHA256 hashes.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `check_login_status` | Check the current account session. |
| `get_login_qrcode` | Get the login or device-verification QR code. |
| `list_feeds` | Read the home feed. |
| `search_feeds` | Search public notes. |
| `get_feed_detail` | Read a note, media metadata, and comments. |
| `user_profile` | Read a public profile and visible notes. |

## Runtime behavior

- Browser operations run through a queue and wait for at most one minute.
- Completed operations are separated by at least 30 seconds plus brief jitter.
- Every operation has a server deadline and returns an error instead of waiting
  forever.
- Comment reads are capped and deliberately slow, so this is not a bulk
  collection tool.
- For current status, run `curl http://127.0.0.1:18060/health`.

More documentation:
[Copilot CLI](docs/copilot-cli.md) ·
[other AI agents](docs/ai-agents.md) ·
[troubleshooting](docs/troubleshooting.md) ·
[HTTP interface](docs/api.md) ·
[architecture](docs/architecture.md) ·
[security model](docs/security-model.md) ·
[contributing](CONTRIBUTING.md)

## Security and attribution

Cookie files contain full account sessions. Treat them like passwords and do
not upload, share, or log them. The server listens only on loopback and checks
Host, Origin, and JSON POST requests, but local processes running as the same
user remain inside the trust boundary. Report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/sinmentis/xiaohongshu-mcp-readonly/security/advisories/new).

This is an unofficial modified fork of
[`xpzouying/xiaohongshu-mcp`](https://github.com/xpzouying/xiaohongshu-mcp).
It is not affiliated with Xiaohongshu, RedNote, or their operators. Users are
responsible for account safety, data handling, access frequency, and compliance.

Licensed under Apache License 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[docs/upstream.md](docs/upstream.md) for licensing and attribution.
