# Security model

## What read-only means

The server exposes no account mutation tool or HTTP route. It can read feeds,
search results, notes, comments, and visible profiles.

The automated browser still creates observable activity:

- login attempts;
- page views;
- searches;
- comment scrolling;
- normal website requests.

Read-only is an interface guarantee, not anonymity.

## Local transport

The listener accepts only loopback addresses. Request middleware also validates:

- a loopback Host;
- an absent Origin or an exact same-origin loopback Origin;
- Fetch Metadata that does not identify a cross-site browser request;
- `application/json` for POST requests.

These checks reduce DNS rebinding, cross-origin data access, and simple form
CSRF. They do not protect against local malware, another process running as the
same user, or a user who deliberately exposes the port through a tunnel or
reverse proxy.

## Cookies

Cookies are stored in a site-specific JSON file with mode `0600`. The file is
not encrypted and grants the same access as the browser session.

Use a private local directory, never add cookie files to version control, and
revoke sessions through the platform if a file may have been copied.

## Browser identity

Supported bundled-browser platforms use a persisted fingerprint seed. Fallback
Chromium uses go-rod stealth plus a stable seed-derived viewport. The fallback
does not provide the same source-level fingerprint controls as the bundled
browser.

## Browser downloads

Browser versions and expected hashes are committed in:

- `browser/browser_version.txt`
- `browser/browser_sha256s.txt`

Archive extraction rejects absolute paths, parent traversal, symlinks, and hard
links. `XHS_BROWSER_BIN` bypasses downloading when users prefer a locally
managed browser.

The bundled archives are distributed by `cdn.one-world.ai`, as inherited from
the upstream project, and are preferred on supported platforms. This fork pins
and reviews their hashes but does not independently reproduce their builds. If
the download is unavailable, startup falls back to system Chromium and then an
existing Playwright Chromium installation.

## Rate limiting

The access gate serializes all exposed browser operations and applies cooldown
and jitter. This is best-effort risk reduction, not a promise that the platform
will accept automated access.

## Logs

Request logs contain a process-local request ID, method, path, status, and
latency. They intentionally omit query strings and request bodies. Browser
operation logs contain only operation names, IDs, phases, timing, and error
types. Application logs must not include cookies, QR payloads, tokens, search
keywords, or proxy credentials.
