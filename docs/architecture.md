# Architecture

## Public interface

The product interface is deliberately smaller than the upstream implementation:
six read-only MCP tools, matching local HTTP routes, and a localhost login page.

The composition roots are:

- `mcp_server.go` for MCP tool registration;
- `routes.go` for HTTP route registration.

Both use positive allowlists. A new upstream action is not exposed unless it is
explicitly added and reviewed.

## Request path

```text
Copilot CLI or local page
        |
        v
MCP / HTTP composition root
        |
        v
XiaohongshuService
        |
        +--> accessGate
        |      serializes requests
        |      applies cooldown and jitter
        |
        +--> browser factory
               resolves Chromium
               injects site cookies
               pins browser identity inputs
        |
        v
site action in xiaohongshu/
        |
        v
Xiaohongshu or RedNote
```

## Deep modules

### Access gate

`access_gate.go` exposes one operation interface and hides serialization,
cooldown, jitter, cancellation, and completion timing. Callers receive safety
behavior without coordinating it themselves.

### Site configuration

`xiaohongshu/site.go` owns the site name, base URLs, locale behavior, and URL
matching. Cookie storage and browser creation receive the selected site so
RedNote and Xiaohongshu sessions never share state.

### Login session

`login_session.go` and the login portion of `service.go` keep one live browser
session, expose progress stages, persist cookies after a stable login signal,
then verify those cookies in a fresh browser before reporting success.

### Browser resolution

`browser/resolve.go` tries:

1. `XHS_BROWSER_BIN`;
2. the pinned bundled browser on supported platforms;
3. system Chromium;
4. Playwright Chromium.

Downloaded archives are checked against repository-pinned SHA256 values and
extracted without links or path traversal.

## Upstream-derived mutation code

Some account-mutation implementation remains in the source tree to keep
upstream rebases reviewable. It is outside the public composition roots and is
not reachable through MCP or HTTP. CI tests assert the exact registered tool
and route sets.

Removing that implementation is a possible later refactor once upstream churn
stabilizes. The public interface should not change when that implementation is
deleted.

## Current design debt

Site and browser configuration are process-global values. That is acceptable
for the current single-account process model, but it prevents one process from
serving multiple sites or accounts safely.

A future deepening should pass an immutable runtime configuration into a
browser-session module rather than adding more package globals.
