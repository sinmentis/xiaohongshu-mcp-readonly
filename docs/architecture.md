# Architecture

## Public interface

The product interface is six read-only MCP tools, matching local HTTP routes,
and a localhost login page. Account-mutation implementations from upstream are
not carried in this fork. MCP initialization supplies server-wide read-only and
pacing instructions to compatible clients.

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
        |      enforces queue and operation deadlines
        |      reports progress and stuck work
        |
        +--> reusable browser runtime
               launches Chromium once
               opens a fresh page per operation
               resets after transport failures or cancellation
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
cooldown, jitter, queue limits, server deadlines, cancellation, progress
heartbeats, and completion timing. A running operation that ignores
cancellation no longer holds the MCP response open indefinitely. The gate
returns a timeout, marks itself degraded until cleanup finishes, and releases
already queued callers with a clear error.

### Browser runtime

`browser_runtime.go` owns the reusable read browser. The process is started
lazily, while each operation still receives a fresh page. Browser startup,
page creation, page work, and cleanup are bounded by context. Transport
failures reset the process before the next read.

Login QR sessions and fresh-cookie verification remain separate browser
lifecycles because they have different durability and freshness requirements.

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

## Current design debt

Site and browser configuration are process-global values. That is acceptable
for the current single-account process model, but it prevents one process from
serving multiple sites or accounts safely.

A future deepening should pass an immutable runtime configuration into a
browser-session module rather than adding more package globals.
