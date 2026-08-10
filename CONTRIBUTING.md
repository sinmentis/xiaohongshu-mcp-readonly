# Contributing

Thank you for helping improve Xiaohongshu MCP Readonly.

## Before opening a change

- Search existing issues and pull requests.
- Keep each pull request focused on one feature or fix.
- Discuss changes that alter the six-tool interface, access pacing, login flow,
  browser identity, cookie format, or supported sites before implementation.
- Do not add account mutation tools or routes. Publishing, commenting, liking,
  favoriting, notifications, and session deletion are out of scope.

## Development workflow

1. Fork the repository.
2. Create a feature branch from `main`.
3. Make the smallest complete change.
4. Run the required checks.
5. Open a pull request using the template.

Use Conventional Commits for commit messages:

```text
feat: add a read-only profile filter
fix: reject cross-origin login requests
docs: clarify RedNote login setup
```

Keep history linear. Rebase feature branches instead of merging `main` into
them.

## Required checks

```bash
go fmt ./...
go vet ./...
go test ./...
go test ./... -race
```

When changing browser integration code, also compile the integration tests:

```bash
go test -tags integration -run '^$' ./...
```

## Browser automation

Use go-rod operations for page interaction. Avoid large JavaScript injections;
they are harder to audit and more likely to diverge from browser behavior.
Small scripts that only read structured page state are acceptable when go-rod
does not provide an equivalent interface.

## Documentation

Public documentation, identifiers, commit messages, and new comments should be
English. Keep terminology consistent with [docs/architecture.md](docs/architecture.md).

Document security assumptions and failure modes. Do not claim that rate limits
prevent account restrictions.

## Privacy

Never commit cookies, QR codes, account identifiers, private notes, browser
profiles, proxy credentials, or screenshots containing personal data.

## Pull requests

Pull requests should explain:

- what changed;
- why it is needed;
- which user-visible behavior changed;
- how it was verified;
- any impact on account safety, cookies, browser identity, or upstream rebases.

Security vulnerabilities must be reported privately as described in
[SECURITY.md](SECURITY.md), not through public issues.
