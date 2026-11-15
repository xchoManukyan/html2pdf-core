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

- **Local files:** a document can only read files inside its base folder (`-root`, or the document's own folder).
  The web server reads no local files at all; uploads are confined to their bundle.
- **Remote resources** are off by default. When enabled, requests to private, loopback and link-local addresses are
  refused (checked after DNS resolution), and size, time and redirects are limited.
- **Signing keys** given to the web server (`-sign-cert`, `-sign-key`) never leave it; requests only choose the
  reason, location and contact.
- **Passwords** can be passed in the environment (`HTML2PDF_USER_PASSWORD`, `HTML2PDF_OWNER_PASSWORD`) instead of the
  command line.
- **Release binaries** are built reproducibly by the release workflow; their checksums are published with them.

## Things to keep in mind

- The web server has no authentication. Run it on an internal network or behind a proxy that authenticates.
- Converting untrusted HTML costs CPU and memory. Limit the time and the size of what users can submit.
- PDF encryption is AES-256 by default; AES-128 and RC4 (`-encryption`) exist only for very old readers.
