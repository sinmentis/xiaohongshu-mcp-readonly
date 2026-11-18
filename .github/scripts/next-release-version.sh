#!/usr/bin/env bash
set -euo pipefail

target=$(git rev-parse --verify "${1:?Usage: next-release-version.sh COMMIT}^{commit}")
stable_tags=$(git tag --merged "$target" --sort=-version:refname)
previous=$(awk '/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ {print; exit}' <<< "$stable_tags")

range=$target
if [[ -n "$previous" ]]; then
  if [[ "$(git rev-parse "$previous^{commit}")" == "$target" ]]; then
    exit 0
  fi
  range="$previous..$target"
fi

subjects=$(git log --format=%s "$range")
bodies=$(git log --format=%b "$range")
breaking_header='^[[:alnum:]_-]+(\([^()]+\))?!:'
breaking_footer='^BREAKING[ -]CHANGE:'
feature_header='^feat(\([^()]+\))?:'
IFS=. read -r major minor patch <<< "${previous:-v0.0.0}"
major=${major#v}

if grep -Eq "$breaking_header" <<< "$subjects" || grep -Eq "$breaking_footer" <<< "$bodies"; then
  printf 'v%d.0.0\n' "$((major + 1))"
elif grep -Eq "$feature_header" <<< "$subjects"; then
  printf 'v%d.%d.0\n' "$major" "$((minor + 1))"
else
  printf 'v%d.%d.%d\n' "$major" "$minor" "$((patch + 1))"
fi
