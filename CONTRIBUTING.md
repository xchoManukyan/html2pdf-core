# Contributing

Thanks for helping! Bug reports with a small HTML document that shows the problem are the most useful thing; see the
issue templates. For security issues, follow [SECURITY.md](SECURITY.md).

## Development

You need the Go toolchain pinned in `go.mod` (`go` downloads it automatically). No third-party modules are used, and
new code should keep it that way.

```bash
go run .                          # web UI on http://localhost:8080
go run . -in samples/invoice.html # one document
go test ./...                     # tests
gofmt -l . && go vet ./...        # formatting and checks (CI runs them)
sh scripts/build.sh --host        # a binary for this machine in dist/
```

Layout changes should be compared with a browser rendering of the same HTML. `HTML2PDF_SITE=path go test
./internal/converter -run TestSite` converts every page of a folder and fails on blank pages or errors.

## The packages built on the converter

The [PHP](https://github.com/xchoManukyan/html2pdf-php) and [Node.js](https://github.com/xchoManukyan/html2pdf-node)
packages run the binary (command line with `-json`) or call the HTTP API. These interfaces are described in the README
("Interface for the packages"). Change them only compatibly, and mark changes **Interface** in `CHANGELOG.md`.

## Releasing

1. Set the version in `cli.go` and move the "Unreleased" notes in `CHANGELOG.md` under the new version.
2. `sh scripts/check-versions.sh` must pass.
3. Commit, tag `vX.Y.Z` and push the tag. The release workflow tests, builds every platform and publishes the
   GitHub release with the binaries, `SHA256SUMS` and `checksums.json`.
4. Update the packages: in the PHP package `sh scripts/update-core.sh X.Y.Z`, in the Node package
   `npm run update-core -- X.Y.Z`. Each pins the new version and its checksums; release them as usual.
