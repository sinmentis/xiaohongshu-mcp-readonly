# Local HTTP interface

The HTTP interface is intended for the bundled login page and local automation.
It is not a network service.

Base URL:

```text
http://127.0.0.1:18060
```

The server rejects non-loopback Host values, cross-site or cross-origin browser
requests, and POST requests without `Content-Type: application/json`.

## Response envelope

Success:

```json
{
  "success": true,
  "data": {},
  "message": "..."
}
```

Error:

```json
{
  "error": "Browser operation timed out",
  "code": "OPERATION_TIMEOUT",
  "details": "search_feeds timed out after 2m0s...",
  "source": "server",
  "retryable": true,
  "next_action": "inspect_health",
  "action_path": "/health",
  "request_id": "req-42"
}
```

Every response includes an `X-Request-ID` header that also appears in server
logs. Error responses repeat it in `request_id`.

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Process, access-gate, and browser-runtime health. |
| `POST` | `/api/v1/login/status` | Check the restored site session. |
| `GET` | `/api/v1/login/session` | Inspect the active login session. |
| `POST` | `/api/v1/login/session` | Start or reuse a login session. |
| `POST` | `/api/v1/feeds/list` | Read the home feed. |
| `POST` | `/api/v1/feeds/search` | Search notes. |
| `POST` | `/api/v1/feeds/detail` | Read a note and comments. |
| `POST` | `/api/v1/user/profile` | Read a public user profile. |
| `POST` | `/mcp` | Stateless MCP endpoint. |

No mutation endpoint is registered.

MCP responses use Server-Sent Events (SSE) so progress notifications can share
the request stream. MCP clients should send:

```http
Accept: application/json, text/event-stream
```

Every MCP tool declares an output schema and returns `structuredContent`.
Text and image content remain available for clients that do not yet consume
structured results. Tool errors use the same stable codes and recovery actions
as the HTTP interface.

## Health

`GET /health` returns `200` for `healthy` and normally busy states. It returns
`503` while a canceled operation or browser runtime is still stuck:

```json
{
  "success": true,
  "data": {
    "status": "busy",
    "service": "xiaohongshu-mcp-readonly",
    "version": "<build-version>",
    "site": "rednote",
    "access": {
      "state": "busy",
      "operation_id": 12,
      "operation": "get_feed_detail",
      "phase": "running",
      "elapsed": "42s",
      "remaining": "9m18s",
      "queued": 0
    },
    "policy": {
      "min_interval": "30s",
      "max_jitter": "15s",
      "max_queue_wait": "1m0s",
      "max_comments": 50,
      "max_replies": 10
    },
    "browser": {
      "state": "ready",
      "launches": 1
    }
  },
  "message": "Service health"
}
```

Browser-backed HTTP errors use:

- `503 SERVICE_BUSY` when an operation cannot start within the queue limit;
- `503 SERVICE_DEGRADED` while timed-out work is still stopping;
- `503 BROWSER_UNAVAILABLE` while the browser runtime is unavailable;
- `504 OPERATION_TIMEOUT` when a tool reaches its server deadline.

## Search

```http
POST /api/v1/feeds/search
Content-Type: application/json
```

```json
{
  "keyword": "Wellington coffee",
  "filters": {
    "sort_by": "latest",
    "note_type": "all",
    "publish_time": "week",
    "search_scope": "all",
    "location": "all"
  }
}
```

Stable filter values are:

- `sort_by`: `relevance`, `latest`, `most_liked`, `most_commented`,
  `most_collected`;
- `note_type`: `all`, `video`, `image`;
- `publish_time`: `all`, `day`, `week`, `half_year`;
- `search_scope`: `all`, `viewed`, `unviewed`, `following`;
- `location`: `all`, `same_city`, `nearby`.

Legacy Chinese values remain accepted for compatibility. MCP callers may also
set `limit` from 1 to 20 on `list_feeds` and `search_feeds`; omitting it
preserves the full current-page result.

Feed summaries and details include a token-free `sourceUrl`. Use that URL for
citations. Keep `xsecToken` only for subsequent tool inputs.

## Feed detail

```http
POST /api/v1/feeds/detail
Content-Type: application/json
```

```json
{
  "feed_id": "note-id",
  "xsec_token": "token-from-feed-result",
  "load_all_comments": true,
  "comment_config": {
    "max_comment_items": 20,
    "click_more_replies": false,
    "max_replies_threshold": 10,
    "scroll_speed": "slow"
  }
}
```

Server policy can reduce requested comment and reply limits.

## User profile

```http
POST /api/v1/user/profile
Content-Type: application/json
```

```json
{
  "user_id": "user-id",
  "xsec_token": "token-from-feed-result",
  "tab": "note"
}
```

Valid tabs are `note`, `fav`, and `liked`. The latter two may be private.
The response includes a token-free `sourceUrl` for the public profile.
