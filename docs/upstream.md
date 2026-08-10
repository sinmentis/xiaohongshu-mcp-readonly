# Upstream relationship

This repository is a modified fork of
[`xpzouying/xiaohongshu-mcp`](https://github.com/xpzouying/xiaohongshu-mcp).

The fork preserves upstream Git history and the Apache License 2.0 notice.
Upstream contributors retain credit through that history and the original
license.

## Why this fork exists

The fork has a narrower product interface:

- GitHub Copilot CLI is the primary client;
- only six read-only MCP tools are exposed;
- HTTP routes match the same read-only scope;
- all traffic remains local;
- browser access is serialized and deliberately slow;
- RedNote and Xiaohongshu sessions are separated;
- QR login can be completed from a localhost page.

## Divergence policy

Security and read-only invariants take precedence over a conflict-free rebase.
Implementation changes should still remain surgical where possible so useful
upstream fixes can be adopted.

Upstream publishing workflows, donation material, examples, registry settings,
and maintainer-specific automation are intentionally not included.
