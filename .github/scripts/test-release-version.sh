#!/usr/bin/env bash
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/next-release-version.sh"
repo=$(mktemp -d)
trap 'rm -rf -- "$repo"' EXIT
cd "$repo"
git init --quiet
git config user.name "Release Test"
git config user.email "release-test@example.invalid"
git config commit.gpgsign false
git config tag.gpgsign false

commit() {
  git commit --quiet --allow-empty "$@"
}

expect() {
  local actual
  actual=$(bash "$script" HEAD)
  if [[ "$actual" != "$1" ]]; then
    printf 'Expected "%s", got "%s"\n' "$1" "$actual" >&2
    exit 1
  fi
}

commit -m 'chore: base for automatic releases'
git tag v0.2.9
git tag -a v0.2.10 -m 'Stable release'
expect ''

commit -m 'fix: search' -m 'Mention feat: and BREAKING CHANGE: inline without changing the bump.'
git tag v1.0.0-rc.1
expect v0.2.11

commit -m 'feat(search): add filtering'
commit -m 'docs: document filtering'
expect v0.3.0

commit -m 'fix: remove the legacy field' -m 'BREAKING CHANGE: use the new field instead.'
expect v1.0.0

git tag v1.0.0
commit -m 'refactor(api)!: remove an old field'
expect v2.0.0
