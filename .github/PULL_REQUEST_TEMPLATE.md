# Pull request

## What changed

Describe the change and why it is needed.

## User-visible behavior

Describe any change to tools, routes, login, site behavior, browser identity,
cookies, pacing, or output.

## Verification

- [ ] `go fmt ./...`
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `go test ./... -race`
- [ ] Integration tests compile when browser code changes

## Safety

- [ ] The six-tool read-only interface is unchanged or intentionally reviewed
- [ ] No mutation route or tool was added
- [ ] No cookies, QR payloads, tokens, account data, or proxy credentials are included
- [ ] Account-safety and upstream-rebase impact are documented
