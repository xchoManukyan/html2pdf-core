# Hyphenation patterns

`hyph-utf8.zip` holds the hyphenation patterns of the [hyph-utf8](https://www.hyphenation.org/tex) project, as
distributed on CTAN (`install/language/hyph-utf8.tds.zip`, folder `tex/generic/hyph-utf8/patterns`). For every
language tag it contains:

- `<tag>.pat.txt` and `<tag>.hyp.txt`: the patterns and exceptions, unchanged;
- `<tag>.LICENSE.txt`: the original header of `hyph-<tag>.tex`, with the copyright, authors and licence;
- `index.txt`: each tag with the minimum letters before and after a break.

Each language keeps its own licence (MIT, LPPL, BSD, MPL/LGPL options, public domain and similar permissive terms);
read `<tag>.LICENSE.txt` in the zip. Patterns licensed only under the GPL (Czech, Indonesian, Macedonian, Serbian
Cyrillic) and Romanian, which states no licence, are left out. Serbian uses the Serbo-Croatian patterns instead.

Rebuild with `go run gen_patterns.go <patterns folder> data/hyph-utf8.zip` from `internal/hyphen`.
