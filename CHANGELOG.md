# Changelog

All notable changes are documented here.

## Unreleased

- Report login and security-verification redirects separately from empty search
  results. Search stops without automatically retrying or attempting verification.
  Adapted from [xpzouying/xiaohongshu-mcp#872](https://github.com/xpzouying/xiaohongshu-mcp/pull/872).

- Establish the public read-only fork.
- Add positive MCP and HTTP allowlists.
- Add RedNote and Xiaohongshu site separation.
- Add conservative access pacing and comment limits.
- Add durable QR login verification and a localhost login page.
- Add Linux ARM64 browser fallback.
- Add loopback request validation and pinned browser archive hashes.
- Reuse one browser process across read operations.
- Add server-side deadlines, MCP progress, stuck-operation fail-fast behavior,
  and detailed health reporting.
- Add typed MCP outputs, stable recovery errors, token-free source URLs, and
  normalized search filters.
- Require Go 1.26.11 and golang.org/x/net v0.60.0 to include the latest standard-library security fixes.
- Make browser archive containment explicit to CodeQL and add traversal and
  symlink regression coverage.
- Remove unused upstream mutation implementations and dead handlers.
