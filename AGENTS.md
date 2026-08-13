# Repository guidance

## Product invariant

This repository ships a localhost-only, read-only MCP server. The public
interface is exactly:

- `check_login_status`
- `get_login_qrcode`
- `list_feeds`
- `search_feeds`
- `get_feed_detail`
- `user_profile`

Never expose publishing, commenting, replying, liking, favoriting,
notifications, or cookie deletion through MCP or HTTP.

The MCP initialization response must keep server-wide read-only, pacing, login,
and token-handling instructions for clients that consume MCP instructions.

## Architecture

- `mcp_server.go` is the MCP registration seam. Register tools positively.
- `routes.go` is the HTTP registration seam. Keep an explicit read-only route
  set.
- `access_gate.go` is the deep module that serializes browser access and applies
  cooldown and jitter.
- `xiaohongshu/site.go` owns site-specific URLs and locale behavior.
- `browser/` owns browser resolution, identity, download verification, and
  cookie injection.
- `login_session.go` and `service.go` own durable login verification.

Keep upstream-derived implementation changes surgical so rebases remain
reviewable. Do not add abstractions without a real second adapter or test seam.

## Security rules

- Keep the listener loopback-only.
- Preserve Host, Origin, and JSON POST validation.
- Treat cookie files and proxy configuration as secrets.
- Keep browser archive hashes pinned in `browser/browser_sha256s.txt`.
- Reject archive links and paths outside the extraction directory.
- Never log cookies, QR payloads, tokens, proxy credentials, or full query
  strings.

## Code style

- - Keep `README.md` in plain Simplified Chinese as the default repository
 introduction, with the English version in `README.en.md`. Use English for
 other public documentation, identifiers, commits, and new comments.
- Existing concise Chinese comments in upstream-derived code may remain unless
  a whole section is being rewritten.
- Prefer go-rod operations over large JavaScript injections.
- Keep interfaces small and implementations deep.
- Run `go fmt ./...` after Go changes.

## Verification

```bash
go vet ./...
go test ./...
go test ./... -race
go test -tags integration -run '^$' ./...
```

Do not add new build or lint tools unless the repository already uses them or
the change genuinely requires them.

## Git

- Develop features on branches.
- Prefer rebase over merge.
- Do not rewrite shared history without approval.
- Preserve upstream history and attribution.
- Do not push to the upstream repository.
