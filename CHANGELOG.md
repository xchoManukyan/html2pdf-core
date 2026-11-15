# Changelog

All notable changes to the converter are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).
Changes to the command line, the `-json` report or the HTTP API (which the PHP and Node packages use) are marked
**Interface**.

## [Unreleased]

## [0.1.2]

Faster start and much less memory per conversion.

- Fallback fonts (for characters the requested fonts lack) are loaded only when a character needs them, instead of
  every installed fallback font at the start. A one-page invoice as its own process: 44 ms instead of 145 ms, and a
  peak of about 10–30 MB instead of 166 MB (Windows, many installed fonts).
- The index of installed fonts is cached in the user's cache folder (`fonts.json`: names, styles, file sizes and
  times; no font data), so a new process does not read every font. An entry is used only for a file found in the font
  folders with the same size and time. `HTML2PDF_FONT_CACHE=off` turns it off, `HTML2PDF_FONT_CACHE=DIR` moves it.
- **Interface:** the server keeps a finished PDF for the web page's download link only when the request sends
  `keep=true` (the web page does). API requests get the PDF in the response and no copy stays on the server; this
  also cut the server's peak memory in a load test from 595 MB to 162 MB.

## [0.1.1]

Security fixes from a review and fuzzing of the converter.

- **Interface:** a server started with `-sign-cert` now needs a secret, `-sign-token` (or `HTML2PDF_SIGN_TOKEN`), and
  signs only requests that send it as `Authorization: Bearer SECRET`. Before, anyone who could reach the server could
  have documents signed with its certificate.
- **Interface:** `-max-jobs` limits the conversions a server runs at once (default: the number of CPUs); further
  requests wait, so a burst of requests cannot exhaust memory.
- Remote resources: requests to more internal addresses are refused: 0.0.0.0/8 (reaches the local machine on Linux),
  100.64.0.0/10 (carrier-grade NAT, some cloud metadata services), other reserved ranges, and private IPv4 addresses
  inside NAT64 and 6to4 IPv6 addresses.
- Local files are opened through Go's `os.Root`: symbolic links cannot leave the base folder even when swapped during
  the conversion, and Windows device names (CON, NUL) are refused.
- WOFF 1 fonts are limited to 64 MB decompressed, like WOFF2 (a forged size could make the decoder allocate gigabytes).
- An unclosed nested CSS block (`a{b{`) no longer makes the conversion fail (found by fuzzing).
- Built with Go 1.27.4.

## [0.1.0]

First release, as a repository of its own (it started inside the PHP package).

- Layout: flexbox, grid with named lines and subgrid, floats, positioning, tables across pages, `@page` rules and
  margin boxes, page numbers, transforms, SVG, WebP, BMP, `srcset` and `<picture>`.
- Tailwind CSS 3 and Bootstrap 5/4 applied automatically when a page uses them, or chosen with `-css`.
- Layout width (`-viewport`) to print a site's desktop or mobile layout.
- Web fonts: TrueType, WOFF and WOFF2, with kerning.
- Right-to-left text with the Unicode Bidirectional Algorithm and Arabic shaping; OpenType shaping (GSUB/GPOS) for
  the Indic scripts and for Arabic fonts without presentation forms.
- Hyphenation in 76 language variants; `<barcode>` (QR, Code 128, EAN/UPC, ISBN, ISSN, Code 39, I2of5).
- Fillable forms (`-forms`), PAdES digital signatures (`-sign-cert`, `-sign-key`), PDF/A-2b (`-pdfa`), AES-256
  encryption with permissions.
- Local files confined to a base folder; remote resources opt-in with SSRF protection.
- **Interface:** command line with `-json` report, HTTP API `POST /api/convert`, web upload page.

[Unreleased]: https://github.com/xchoManukyan/html2pdf-core/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/xchoManukyan/html2pdf-core/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/xchoManukyan/html2pdf-core/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/xchoManukyan/html2pdf-core/releases/tag/v0.1.0
