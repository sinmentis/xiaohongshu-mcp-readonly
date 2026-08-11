# GitHub Copilot CLI setup

## One-command setup

On Linux with a systemd user session:

```bash
git clone https://github.com/sinmentis/xiaohongshu-mcp-readonly.git
cd xiaohongshu-mcp-readonly
./scripts/setup-local --site rednote --agent copilot
```

Use `--site xiaohongshu` for a mainland Xiaohongshu account.

## Manual setup

Build the binary:

```bash
go build -trimpath -o "$HOME/.local/bin/xiaohongshu-mcp-readonly" .
mkdir -p "$HOME/.local/share/xiaohongshu-mcp-readonly"
```

## Run

For a RedNote account:

```bash
COOKIES_PATH="$HOME/.local/share/xiaohongshu-mcp-readonly/cookies.json" \
  "$HOME/.local/bin/xiaohongshu-mcp-readonly" \
  -site rednote \
  -port 127.0.0.1:18060 \
  -min-request-interval 30s \
  -request-jitter 15s \
  -max-comments 50 \
  -max-replies 10
```

Use `-site xiaohongshu` for a mainland Xiaohongshu account.

Each site gets a separate session file. With the base path above, RedNote uses
`cookies-rednote.json`.

## Login

Open:

```text
http://127.0.0.1:18060/login
```

The login attempt remains open for up to 10 minutes. That timer applies only to
the QR workflow, not to the authenticated account session.

The page reports success only after a fresh browser restores the exported
cookies and confirms that the site is still signed in.

For a terminal QR:

```bash
sudo apt-get install jq zbar-tools qrencode
install -m 0755 scripts/xhs-login-qr "$HOME/.local/bin/xhs-login-qr"
xhs-login-qr
```

## Register the MCP server

```bash
copilot mcp add \
  --transport http \
  --timeout 900000 \
  --tools check_login_status,get_login_qrcode,list_feeds,search_feeds,get_feed_detail,user_profile \
  xiaohongshu-readonly \
  http://127.0.0.1:18060/mcp
```

Run `/mcp` inside Copilot CLI to confirm the connection.

Example prompts:

```text
Use xiaohongshu-readonly to check whether I am logged in.
```

```text
Search RedNote for "Wellington coffee" and summarize the returned notes.
```

```text
Read this note and summarize no more than 20 top-level comments.
```

## systemd user service

Install the binary and unit:

```bash
install -m 0755 bin/xiaohongshu-mcp-readonly \
  "$HOME/.local/bin/xiaohongshu-mcp-readonly"
install -m 0644 deploy/systemd/xiaohongshu-mcp-readonly.service \
  "$HOME/.config/systemd/user/xiaohongshu-mcp-readonly.service"

systemctl --user daemon-reload
systemctl --user enable --now xiaohongshu-mcp-readonly.service
```

The provided unit defaults to RedNote. Set `XHS_SITE=xiaohongshu` in
`$HOME/.config/xiaohongshu-mcp-readonly/env` for a mainland account.

View logs:

```bash
journalctl --user -u xiaohongshu-mcp-readonly.service -f
```
