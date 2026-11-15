# Security policy

## Supported versions

Security fixes go into the latest release. Please upgrade before reporting an issue.

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately through GitHub:
[Report a vulnerability](https://github.com/xchoManukyan/html2pdf-core/security/advisories/new).

Include what an attacker can do, the affected version, and an HTML document or steps that show it. You will get an
answer within a week; fixes are released as soon as they are ready and credited to you unless you prefer otherwise.

## What the converter protects against

These are deliberate guarantees, so a way around any of them is a vulnerability:

- **Local files:** a document can only read files inside its base folder (`-root`, or the document's own folder). The
  folder is opened with Go's `os.Root`, so `..`, absolute paths and symbolic links cannot leave it.
  The web server reads no local files at all; uploads are confined to their bundle.
- **Remote resources** are off by default. When enabled, requests to private, loopback and link-local addresses are
  refused (checked after DNS resolution, including carrier-grade NAT, "this network" 0.0.0.0/8 and IPv4 addresses
  inside NAT64 and 6to4 IPv6 addresses), and size, time and redirects are limited.
- **Signing:** a server with a signing certificate needs a secret (`-sign-token`) and signs only requests that send it
  (`Authorization: Bearer ...`). The key never leaves the server; requests only choose the reason, location and contact.
- **Passwords** can be passed in the environment (`HTML2PDF_USER_PASSWORD`, `HTML2PDF_OWNER_PASSWORD`) instead of the
  command line.
- **Release binaries** are built reproducibly by the release workflow; their checksums are published with them.

## Things to keep in mind

- The web server has no authentication. Run it on an internal network or behind a proxy that authenticates.
- Converting untrusted HTML costs CPU and memory. The server runs at most `-max-jobs` conversions at once (default:
  the number of CPUs); also limit the time and the size of what users can submit, e.g. in a proxy in front of it.
- PDF encryption is AES-256 by default; AES-128 and RC4 (`-encryption`) exist only for very old readers.
