# Xiaohongshu QR login never becomes durable

Research date: 2026-08-11

## Conclusion

The highest-confidence root cause is a site mismatch:

- The account is authenticated on `rednote.com`.
- The MCP hardcodes `https://www.xiaohongshu.com/explore`.
- RedNote and Xiaohongshu use separate Web account and session systems.

The observed cookies support this conclusion: the login produces
`web_session` and `id_token` cookies for `.rednote.com`. Reusing those cookies
or the browser profile on `xiaohongshu.com` cannot create a valid Xiaohongshu
session.

This explains why the live login page can briefly look authenticated while
every cold check against `xiaohongshu.com` returns guest state.

## Primary evidence

### Maintainer confirmation

In [issue #609](https://github.com/xpzouying/xiaohongshu-mcp/issues/609#issuecomment-5159293159),
the repository owner confirms:

> Accounts registered with overseas phone numbers belong to the RedNote
> platform. Their login cookies are under `.rednote.com`, while this project
> accesses `xiaohongshu.com`. The sessions are not shared, so login always
> appears false.

The maintainer also notes that replacing the hostname alone is insufficient
because the RedNote page structure and selectors differ.

### Exact reproduced symptom

[PR #798](https://github.com/xpzouying/xiaohongshu-mcp/pull/798) states that
`rednote.com` and `xiaohongshu.com` have separate Web login systems. It reports
that scanning a Xiaohongshu-domain QR code with a RedNote account does not
return the phone confirmation to the Xiaohongshu session.

The PR implements:

- `-site xiaohongshu|rednote`
- Per-site URLs and login selectors
- Per-site cookies and fingerprint seeds
- RedNote locale handling through Chrome DevTools Protocol
- A 10-minute polling login window

### Other matching reports

- [Issue #579](https://github.com/xpzouying/xiaohongshu-mcp/issues/579):
  RedNote users are redirected to `rednote.com`; `.rednote.com` cookies cannot
  authenticate `xiaohongshu.com`.
- [Issue #699](https://github.com/xpzouying/xiaohongshu-mcp/issues/699):
  Overseas account cannot log into the Xiaohongshu-domain MCP.
- [PR #704](https://github.com/xpzouying/xiaohongshu-mcp/pull/704):
  Adds domain configuration because hardcoded Xiaohongshu URLs reject RedNote
  sessions.
- [PR #679](https://github.com/xpzouying/xiaohongshu-mcp/pull/679):
  Proposes cross-domain login detection based on the sidebar login button.

## Related but secondary issues

- [Issue #799](https://github.com/xpzouying/xiaohongshu-mcp/issues/799):
  Optional device-security QR after the first scan. The local fork already
  handles this flow.
- [Issue #613](https://github.com/xpzouying/xiaohongshu-mcp/issues/613#issuecomment-5094503189):
  The maintainer intentionally uses exported cookies plus a persisted
  fingerprint seed instead of relying on Chromium's user-data directory.
- [PR #755](https://github.com/xpzouying/xiaohongshu-mcp/pull/755):
  Persists the fingerprint seed to avoid presenting a new browser identity on
  every launch.

These issues can cause similar symptoms, but they do not explain the consistent
RedNote cookie domain combined with hardcoded Xiaohongshu navigation as directly
as the site mismatch does.

## Recommended fix

Adopt the site abstraction from
[PR #798](https://github.com/xpzouying/xiaohongshu-mcp/pull/798), then run this
installation with `-site rednote`.

The change must cover every browser URL, login selector, locale rule, cookie
file, and fingerprint seed. Replacing only the login URL or rewriting cookie
domains will not work because the session itself belongs to the RedNote
platform.
