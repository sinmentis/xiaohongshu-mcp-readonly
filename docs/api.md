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
  "error": "...",
  "code": "ERROR_CODE"
}
```

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Process health and version. |
| `POST` | `/api/v1/login/status` | Check the restored site session. |
| `GET` | `/api/v1/login/session` | Inspect the active login session. |
| `POST` | `/api/v1/login/session` | Start or reuse a login session. |
| `POST` | `/api/v1/feeds/list` | Read the home feed. |
| `POST` | `/api/v1/feeds/search` | Search notes. |
| `POST` | `/api/v1/feeds/detail` | Read a note and comments. |
| `POST` | `/api/v1/user/profile` | Read a public user profile. |
| `POST` | `/mcp` | Stateless MCP endpoint. |

No mutation endpoint is registered.

## Search

```http
POST /api/v1/feeds/search
Content-Type: application/json
```

```json
{
  "keyword": "Wellington coffee",
  "filters": {
    "sort_by": "最新",
    "note_type": "不限",
    "publish_time": "一周内",
    "search_scope": "不限",
    "location": "不限"
  }
}
```

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
