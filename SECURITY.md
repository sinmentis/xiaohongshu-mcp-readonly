# Security policy

## Supported versions

Security fixes are provided for the latest release and the current `main`
branch.

## Reporting a vulnerability

Use
[GitHub Security Advisories](https://github.com/sinmentis/xiaohongshu-mcp-readonly/security/advisories/new)
for private reports. Do not open a public issue for an undisclosed
vulnerability.

Include reproduction steps, affected versions, impact, and any suggested fix.
Expect an initial response within seven days.

## Threat model

Trusted:

- the local user running the server;
- local processes running with the same operating-system account.

Untrusted:

- Xiaohongshu and RedNote page content;
- browser responses and QR image sources;
- remote websites opened in the user's normal browser;
- proxy configuration;
- downloaded browser archives and their distribution host.

The server binds only to loopback, validates local Host and Origin values, and
requires JSON POST requests. Reverse proxies, tunnels, port-forwarding, or local
malware can move the server outside this threat model.

## Read-only scope

The server positively registers six read-only MCP tools and a matching HTTP
route set. It does not expose account mutation operations.

Read-only does not mean invisible. The platform can record logins, searches,
page views, and automated browsing behavior.

The fork omits upstream account-mutation implementations. Tests assert the
exact MCP tool and HTTP route sets.

## Session data

Cookie files contain a full authenticated session. They are written with mode
`0600`, but they are not encrypted. Store them on a trusted local filesystem,
never commit them, and never share them.

Deleting the local cookie file does not necessarily revoke the remote session.
Use the platform's account controls to revoke sessions.

## Browser supply chain

Supported platforms can download a pinned browser archive. Expected SHA256
hashes are stored in `browser/browser_sha256s.txt`, and extraction rejects
links and paths outside the cache directory.

The archives are served by `cdn.one-world.ai`, inherited from upstream. Hash
pinning protects integrity against unexpected archive changes but does not
provide independently reproducible build provenance.

Users who do not trust the bundled browser distribution can set
`XHS_BROWSER_BIN` to a locally managed Chromium-compatible executable.

## Account safety

The default access gate serializes requests, waits at least 30 seconds between
operations, adds up to 15 seconds of jitter, limits comment loading, and forces
slow scrolling.

These controls reduce burst risk but do not prevent account challenges or
restrictions. Use a dedicated account and avoid unattended bulk collection.
