#!/usr/bin/env bash
# Downloads a repo's Go module dependencies into GOMODCACHE at one or more
# refs, so eval runs work with GOPROXY=off.
# Usage: warm-modules.sh <repo-url> <ref>...
set -euo pipefail

url=$1
shift

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git clone --quiet "$url" "$tmp"
# --force: `go mod download all` may add entries to go.sum, which would
# otherwise block switching to the next ref.
for ref in "$@"; do
  git -C "$tmp" checkout --quiet --force --detach "$ref"
  (cd "$tmp" && go mod download all)
done
