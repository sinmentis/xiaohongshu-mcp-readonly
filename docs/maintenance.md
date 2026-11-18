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

The Release workflow publishes automatically when both CI and CodeQL succeed
for the same push to the current `main`. It chooses one version from all commits
since the highest stable tag reachable from that commit:

- **Major:** a Conventional Commit subject with `!` (for example `feat(api)!:`),
  or a `BREAKING CHANGE:` / `BREAKING-CHANGE:` line in a commit body.
- **Minor:** a `feat:` or `feat(scope):` subject, with no breaking change.
- **Patch:** any other change, including dependency, documentation and CI updates.

The strongest bump wins. Prerelease tags are ignored for version selection.
If several changes land quickly, the next successful `main` release includes
changes from any superseded commits. A commit already tagged with a stable
version is not automatically tagged again. Keep conventional commit subjects
when merging and document meaningful changes in `CHANGELOG.md`.

The workflow builds the archives, creates `SHA256SUMS`, attests the artifacts,
and creates the tag and GitHub Release without a personal token. For a failed
release run, rerun the Release workflow from its Actions run page. For an
intentional manual release, push only the selected tag; e.g.
`git push origin v1.0.0-rc.1`. The tagged commit must be in `main` history.

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
