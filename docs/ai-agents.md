# AI agent setup

The server uses the standard Streamable HTTP MCP transport:

```text
http://127.0.0.1:18060/mcp
```

No client-specific skill or system prompt is required. During MCP
initialization, the server sends read-only, login, pacing, and token-handling
instructions to clients that support the MCP `instructions` field.

The agent must run on the same machine as the server. A cloud-hosted agent
cannot reach `127.0.0.1`, and exposing this server through a tunnel or reverse
proxy is outside the security model.

## One-command Linux setup

Choose the account site and client:

```bash
./scripts/setup-local --site rednote --agent copilot
./scripts/setup-local --site rednote --agent claude
./scripts/setup-local --site rednote --agent codex
```

Use `--site xiaohongshu` for a mainland Xiaohongshu account. Use
`--agent none` to install only the server.

The setup script builds the binary, installs and starts the systemd user
service, writes the selected site configuration, and registers the selected
client when applicable.

## Manual client registration

### GitHub Copilot CLI

```bash
copilot mcp add \
  --transport http \
  --timeout 900000 \
  --tools check_login_status,get_login_qrcode,list_feeds,search_feeds,get_feed_detail,user_profile \
  xiaohongshu-readonly \
  http://127.0.0.1:18060/mcp
```

### Claude Code

```bash
claude mcp add-json --scope user xiaohongshu-readonly \
  '{"type":"http","url":"http://127.0.0.1:18060/mcp","timeout":900000}'
```

### Codex CLI, desktop, and IDE extension

Add this to `~/.codex/config.toml`:

```toml
[mcp_servers.xiaohongshu-readonly]
url = "http://127.0.0.1:18060/mcp"
tool_timeout_sec = 900
default_tools_approval_mode = "auto"
```

### Cursor and other MCP clients

Add a Streamable HTTP server named `xiaohongshu-readonly` with the endpoint:

```text
http://127.0.0.1:18060/mcp
```

Set the tool timeout to at least 900 seconds when the client supports it.

## Fallback instruction

Clients that ignore MCP server instructions can use:

```text
Use xiaohongshu-readonly only for read operations. Check login status first.
If logged out, direct me to the local /login page. Call one tool at a time,
avoid repeated polling or bulk collection, and never show xsec tokens.
```
