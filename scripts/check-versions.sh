#!/usr/bin/env sh
# Checks that the version in cli.go has a CHANGELOG.md section and that the
# workflows use the Go toolchain pinned in go.mod (binaries are only
# reproducible with the same toolchain).
#
#   scripts/check-versions.sh [expected-version]   e.g. the release tag without "v"
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
version=$(sed -n 's/^const version = "\(.*\)"$/\1/p' "$here/cli.go")
toolchain=$(sed -n 's/^toolchain go\(.*\)$/\1/p' "$here/go.mod")
expected=${1:-$version}
status=0
[ "$version" = "$expected" ] || { echo "cli.go has version $version, expected $expected" >&2; status=1; }
grep -q "^## \[$expected\]" "$here/CHANGELOG.md" || { echo "CHANGELOG.md has no section for $expected" >&2; status=1; }
for wf in "$here"/.github/workflows/*.yml; do
  pinned=$(sed -n "s/^ *GO_VERSION: *'\{0,1\}\([0-9.]*\)'\{0,1\}$/\1/p" "$wf")
  [ -z "$pinned" ] || [ "$pinned" = "$toolchain" ] || { echo "$(basename "$wf") pins Go $pinned, go.mod has toolchain $toolchain" >&2; status=1; }
done
[ $status = 0 ] && echo "version $expected; Go toolchain $toolchain"
exit $status
