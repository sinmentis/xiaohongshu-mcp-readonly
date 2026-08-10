# Maintenance and releases

## Upstream sync

Keep two remotes:

```bash
git remote add upstream https://github.com/xpzouying/xiaohongshu-mcp.git
git config remote.upstream.tagOpt --no-tags
git fetch --no-tags upstream
git rebase upstream/main
```

Resolve conflicts by preserving the fork invariants in `AGENTS.md`. Never merge
upstream mutation tools or routes into the public composition roots.

## Browser updates

Browser updates are manual and reviewed.

1. Update `browser/browser_version.txt`.
2. Download the three published archives.
3. Verify their provenance.
4. Update `browser/browser_sha256s.txt`.
5. Run browser download and startup tests on supported platforms.
6. Review the change through a pull request.

Do not fetch expected hashes dynamically from the same host that serves the
archives.

## Release process

1. Update `CHANGELOG.md`.
2. Confirm CI and CodeQL are green on `main`.
3. Confirm the fork remote has no inherited upstream tags.
4. Create a semantic version tag such as `v0.1.0` on `main`.
5. Push only that tag with `git push origin v0.1.0`.
6. Verify the GitHub release artifacts and `SHA256SUMS`.

Never use `git push --tags` or `git push --mirror` from an upstream-derived
clone. The release workflow also verifies that the tagged source contains this
fork's module path and `NOTICE` file before publishing.

Release workflows publish only GitHub Release artifacts. They do not push to
Docker Hub, third-party registries, or upstream infrastructure.

## Repository settings

Recommended `main` protection:

- require pull requests;
- require the CI and dependency-review checks;
- dismiss stale approvals;
- require linear history;
- block force pushes and branch deletion.

Set the default Actions token permission to read-only. Grant write permission
only inside the release workflow.
