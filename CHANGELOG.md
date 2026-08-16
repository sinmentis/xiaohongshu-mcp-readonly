# Changelog

All notable changes are documented here.

## Unreleased

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
- Require Go 1.25.13 to include the latest standard-library security fixes.
- Remove unused upstream mutation implementations and dead handlers.
