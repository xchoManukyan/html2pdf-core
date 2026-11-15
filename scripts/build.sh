#!/usr/bin/env sh
# Builds the converter for every platform into dist/, with their SHA-256
# checksums in dist/SHA256SUMS and dist/checksums.json. The PHP and Node
# packages pin a converter version and copy checksums.json from its release
# to verify the binaries they install.
#
#   scripts/build.sh           all platforms
#   scripts/build.sh --host    only this machine's platform (development, wrapper tests)
#
# Builds are reproducible: the Go toolchain pinned in go.mod produces
# byte-identical binaries on any machine, so anyone can check a release.
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
out="$here/dist"
mkdir -p "$out"

version=$(sed -n 's/^const version = "\(.*\)"$/\1/p' "$here/cli.go")
[ -n "$version" ] || { echo "could not read the version from cli.go" >&2; exit 1; }

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

if [ "${1:-}" = --host ]; then
  targets="$(cd "$here" && go env GOOS)/$(cd "$here" && go env GOARCH)"
else
  targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
fi

entries=""
sums=""
for target in $targets; do
  os=${target%/*}
  arch=${target#*/}
  name="html2pdf-$os-$arch"
  [ "$os" = windows ] && name="$name.exe"
  (cd "$here" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -buildvcs=false -ldflags "-s -w" -o "$out/$name" .)
  chmod +x "$out/$name"
  sum=$(sha256 "$out/$name")
  size=$(wc -c < "$out/$name" | tr -d ' ')
  echo "$name  $sum  $size bytes"
  entries="$entries${entries:+,
}        \"$name\": { \"sha256\": \"$sum\", \"size\": $size }"
  sums="$sums$sum  $name
"
done

if [ "${1:-}" != --host ]; then
  printf '{\n    "version": "%s",\n    "files": {\n%s\n    }\n}\n' "$version" "$entries" > "$out/checksums.json"
  printf '%s' "$sums" > "$out/SHA256SUMS"
  echo "wrote dist/checksums.json and dist/SHA256SUMS for version $version"
fi
