package render

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xchoManukyan/html2pdf-core/internal/hyphen"
)

// wordBreak is a place inside a word where a line may end: at is a byte
// offset into the word as displayed; hyphen means a "-" is shown when the
// line breaks there (soft hyphens and dictionary points, not after a hyphen
// that is already in the text).
type wordBreak struct {
	at     int
	hyphen bool
}

// breakPoints removes soft hyphens (U+00AD) from a word and lists where it
// may be broken across lines, following CSS hyphens:
//   - none: nowhere (soft hyphens are still invisible)
//   - manual (the default): at soft hyphens
//   - auto: at soft hyphens and the hyphenation points of the element's
//     language (lang attribute), when patterns for it are available
//
// In every mode a word may also break after a hyphen between two letters
// or digits ("well-known"), as browsers do.
func breakPoints(text string, st *style) (string, []wordBreak) {
	mode := st.hyphens
	hasShy := strings.ContainsRune(text, '­')
	if !hasShy && mode != "auto" && !strings.ContainsAny(text, "-‐") {
		return text, nil
	}
	var b strings.Builder
	var brks []wordBreak
	prev := rune(0)
	for i, r := range text {
		if r == '­' {
			if mode != "none" && b.Len() > 0 && i+2 < len(text) {
				brks = append(brks, wordBreak{b.Len(), true})
			}
			continue
		}
		b.WriteRune(r)
		if (r == '-' || r == '‐') && isAlnum(prev) {
			if next, _ := utf8.DecodeRuneInString(text[i+utf8.RuneLen(r):]); isAlnum(next) {
				brks = append(brks, wordBreak{b.Len(), false})
			}
		}
		prev = r
	}
	disp := b.String()

	if mode == "auto" {
		if dict := hyphen.For(st.lang); dict != nil {
			// hyphenate each run of letters, leaving out all-capital words (acronyms)
			start := -1
			flush := func(end int) {
				if start < 0 {
					return
				}
				run := disp[start:end]
				if strings.ToUpper(run) != run {
					offsets := runeOffsets(run)
					for _, p := range dict.Points(run) {
						brks = append(brks, wordBreak{start + offsets[p], true})
					}
				}
				start = -1
			}
			for i, r := range disp {
				if hyphen.IsWordRune(r) {
					if start < 0 {
						start = i
					}
				} else {
					flush(i)
				}
			}
			flush(len(disp))
		}
	}
	if len(brks) == 0 {
		return disp, nil
	}
	sort.SliceStable(brks, func(i, j int) bool { return brks[i].at < brks[j].at })
	out := brks[:0]
	for _, br := range brks {
		if br.at <= 0 || br.at >= len(disp) || len(out) > 0 && out[len(out)-1].at == br.at {
			continue
		}
		out = append(out, br)
	}
	return disp, out
}

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// runeOffsets maps rune indexes of s to byte offsets.
func runeOffsets(s string) []int {
	out := make([]int, 0, len(s)+1)
	for i := range s {
		out = append(out, i)
	}
	return append(out, len(s))
}

// hyphenPiece is the start of a word up to a break, with its hyphen.
func hyphenPiece(text string, b wordBreak) string {
	if b.hyphen {
		return text[:b.at] + "-"
	}
	return text[:b.at]
}
