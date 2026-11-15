# Changelog

All notable changes to the converter are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).
Changes to the command line, the `-json` report or the HTTP API (which the PHP and Node packages use) are marked
**Interface**.

## [Unreleased]

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

[Unreleased]: https://github.com/xchoManukyan/html2pdf-core/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/xchoManukyan/html2pdf-core/releases/tag/v0.1.0
