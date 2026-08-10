# Troubleshooting

## Login stays false after scanning

Confirm the selected site matches the account:

- overseas account: `-site rednote`;
- mainland account: `-site xiaohongshu`.

The two sites use separate sessions. Rewriting cookie domains does not convert
one session into the other.

## The page shows a second QR code

The platform requested device verification. Scan the new code and keep the
local page open until fresh-browser verification finishes.

## No QR code appears

Check the service log:

```bash
journalctl --user -u xiaohongshu-mcp-readonly.service -n 100 --no-pager
```

Confirm the browser can start and the selected site is reachable.

## Browser unavailable

Set an explicit Chromium-compatible executable:

```bash
export XHS_BROWSER_BIN=/path/to/chromium
```

Linux ARM64 does not have a bundled browser archive. Install Chromium or an
existing Playwright Chromium build.

## Requests appear slow

The delay is intentional. By default, every completed operation is followed by
at least 30 seconds plus up to 15 seconds of jitter.

## `403 LOCAL_ONLY`

Use `127.0.0.1`, `localhost`, or `::1`. The server intentionally rejects LAN
addresses, reverse-proxy Host values, and cross-origin browser requests.

## `415 JSON_REQUIRED`

POST requests must include:

```http
Content-Type: application/json
```

## Login worked but later expired

The 10-minute value shown during login is only the QR-attempt window. Account
session lifetime is controlled by the platform. Start a new login session if
the restored cookies are no longer accepted.

## Reporting a bug

Include the operating system, architecture, selected site, browser source, and
redacted logs. Never include cookie files, QR payloads, tokens, account
identifiers, or proxy credentials.
