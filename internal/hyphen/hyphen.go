// Package hyphen finds hyphenation points with Liang's algorithm and the
// TeX hyphenation patterns of the hyph-utf8 project (the patterns browsers
// and LibreOffice use too), for about 75 languages. The patterns and their
// licences are in data/hyph-utf8.zip (see data/README.md).
package hyphen

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:embed data/hyph-utf8.zip
var archive []byte

// Dictionary holds the patterns of one language.
type Dictionary struct {
	patterns    map[string][]uint8 // letters -> values between them (len(letters)+1)
	maxLen      int
	exceptions  map[string][]int // word -> break positions (in runes)
	left, right int              // minimum letters before the first and after the last break
}

var (
	indexOnce sync.Once
	zr        *zip.Reader
	mins      map[string][2]int // pattern tag -> left, right minimums

	mu    sync.Mutex
	dicts = map[string]*Dictionary{}
)

// aliases maps language tags to the pattern set that covers them.
var aliases = map[string]string{
	"en": "en-us", "en-gb": "en-gb", "en-uk": "en-gb", "en-au": "en-gb", "en-nz": "en-gb", "en-ie": "en-gb",
	"en-za": "en-gb", "en-in": "en-gb",
	"de": "de-1996", "de-ch": "de-ch-1901", "de-li": "de-ch-1901", "gsw": "de-ch-1901",
	"el": "el-monoton", "mn": "mn-cyrl", "no": "nb", "nb": "nb", "nn": "nn",
	"sr": "sh-cyrl", "sr-cyrl": "sh-cyrl", "sr-latn": "sh-latn", "sh": "sh-latn", "bs": "sh-latn",
	"am": "mul-ethi", "ti": "mul-ethi", "gez": "mul-ethi", "ku": "kmr",
	"zh-latn": "zh-latn-pinyin", "la-classic": "la-x-classic", "la-liturgic": "la-x-liturgic",
}

func loadIndex() {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return
	}
	zr = r
	mins = map[string][2]int{}
	for _, line := range strings.Split(readFile("index.txt"), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 {
			l, _ := strconv.Atoi(f[1])
			r, _ := strconv.Atoi(f[2])
			mins[f[0]] = [2]int{max(l, 1), max(r, 1)}
		}
	}
}

func readFile(name string) string {
	if zr == nil {
		return ""
	}
	f, err := zr.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	return string(b)
}

// Languages returns the tags of all pattern sets.
func Languages() []string {
	indexOnce.Do(loadIndex)
	var out []string
	for tag := range mins {
		out = append(out, tag)
	}
	return out
}

// For returns the dictionary for a BCP 47 language tag such as "en",
// "en-GB", "de", "fr-CA" or "hy", or nil when the language is not
// supported. Dictionaries are loaded on first use.
func For(lang string) *Dictionary {
	tag := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(lang), "_", "-"))
	if tag == "" {
		return nil
	}
	indexOnce.Do(loadIndex)
	set := resolve(tag)
	if set == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	if d, ok := dicts[set]; ok {
		return d
	}
	m := mins[set]
	d := parse(readFile(set+".pat.txt"), readFile(set+".hyp.txt"), m[0], m[1])
	dicts[set] = d
	return d
}

// resolve finds the pattern set for a tag: the tag itself, an alias, or
// the tag with its last subtags removed ("fr-ca" -> "fr").
func resolve(tag string) string {
	for t := tag; t != ""; {
		if a, ok := aliases[t]; ok {
			return a
		}
		if _, ok := mins[t]; ok {
			return t
		}
		i := strings.LastIndexByte(t, '-')
		if i < 0 {
			break
		}
		t = t[:i]
	}
	return ""
}

func parse(patterns, exceptions string, left, right int) *Dictionary {
	d := &Dictionary{patterns: map[string][]uint8{}, exceptions: map[string][]int{}, left: left, right: right}
	for _, p := range strings.Fields(patterns) {
		var letters []rune
		values := []uint8{0}
		for _, r := range p {
			if r >= '0' && r <= '9' {
				values[len(values)-1] = uint8(r - '0')
				continue
			}
			letters = append(letters, r)
			values = append(values, 0)
		}
		d.patterns[string(letters)] = values
		d.maxLen = max(d.maxLen, len(letters))
	}
	for _, w := range strings.Fields(exceptions) {
		var pos []int
		n := 0
		for _, r := range w {
			if r == '-' {
				pos = append(pos, n)
				continue
			}
			n++
		}
		d.exceptions[strings.ReplaceAll(w, "-", "")] = pos
	}
	return d
}

// Points returns the hyphenation points of a word as rune offsets: a
// break is allowed before word[i] for each i returned. The word should
// consist of letters only; case does not matter.
func (d *Dictionary) Points(word string) []int {
	if d == nil {
		return nil
	}
	lower := []rune(strings.ToLower(word))
	n := len(lower)
	if n < d.left+d.right {
		return nil
	}
	if pos, ok := d.exceptions[string(lower)]; ok {
		return pos
	}
	padded := append(append([]rune{'.'}, lower...), '.')
	values := make([]uint8, len(padded)+1)
	for i := range padded {
		for l := 1; l <= d.maxLen && i+l <= len(padded); l++ {
			if v, ok := d.patterns[string(padded[i:i+l])]; ok {
				for k, x := range v {
					if x > values[i+k] {
						values[i+k] = x
					}
				}
			}
		}
	}
	var out []int
	// values[j] sits before padded[j]; padded[j] is lower[j-1]
	for i := d.left; i <= n-d.right; i++ {
		if values[i+1]%2 == 1 {
			out = append(out, i)
		}
	}
	return out
}

// Hyphenate returns the word with "-" at every hyphenation point (for tests
// and debugging).
func (d *Dictionary) Hyphenate(word string) string {
	r := []rune(word)
	var b strings.Builder
	pts := d.Points(word)
	k := 0
	for i, c := range r {
		if k < len(pts) && pts[k] == i {
			b.WriteByte('-')
			k++
		}
		b.WriteRune(c)
	}
	return b.String()
}

// IsWordRune reports whether r can be part of a hyphenatable word.
func IsWordRune(r rune) bool {
	// vowel signs and other marks are part of words in Indic scripts
	return unicode.IsLetter(r) || unicode.In(r, unicode.Mn, unicode.Mc) || r == 0x200C || r == 0x200D
}
