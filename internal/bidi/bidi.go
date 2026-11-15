// Package bidi implements the parts of the Unicode Bidirectional Algorithm
// (UAX #9) needed to print right-to-left text: character classes, the
// resolution of weak and neutral types, embedding levels, and reordering a
// line for display. Explicit embedding controls (LRE, RLO, ...) are treated
// as neutrals; directional isolation comes from CSS instead (direction on
// block elements).
package bidi

import "unicode"

// Class is a bidirectional character type.
type Class uint8

const (
	L   Class = iota // left-to-right letter
	R                // right-to-left letter (Hebrew ...)
	AL               // Arabic letter
	EN               // European number
	ES               // European separator (+ -)
	ET               // European terminator (# $ % ...)
	AN               // Arabic number
	CS               // common separator (, . / : no-break space)
	NSM              // non-spacing mark
	BN               // boundary neutral (zero-width characters)
	B                // paragraph separator
	S                // segment separator (tab)
	WS               // white space
	ON               // other neutral
)

// ClassOf returns the bidirectional type of r (a compact approximation of
// the Unicode data that is exact for the scripts and symbols in common use).
func ClassOf(r rune) Class {
	switch {
	case r >= '0' && r <= '9':
		return EN
	case r == '+' || r == '-' || r == 0x207A || r == 0x207B || r == 0x208A || r == 0x208B || r == 0x2212:
		return ES
	case r == ',' || r == '.' || r == '/' || r == ':' || r == 0x00A0 || r == 0x060C || r == 0x202F || r == 0x2044:
		return CS
	case r == '#' || r == '$' || r == '%' || r == 0x00B0 || r == 0x00B1 || r == 0x2030 || r == 0x2031 || r == 0x066A ||
		unicode.Is(unicode.Sc, r):
		return ET
	case r == '\n' || r == '\r' || r == 0x2029 || r == 0x1C || r == 0x1D || r == 0x1E || r == 0x85:
		return B
	case r == '\t' || r == 0x0B || r == 0x1F:
		return S
	case r == ' ' || r == '\f' || r >= 0x2000 && r <= 0x200A || r == 0x2028 || r == 0x205F || r == 0x3000:
		return WS
	case r >= 0x200B && r <= 0x200D || r == 0x2060 || r == 0xFEFF || r == 0x00AD:
		return BN
	case r == 0x200F: // right-to-left mark
		return R
	case r == 0x200E: // left-to-right mark
		return L
	case r >= 0x00B2 && r <= 0x00B3 || r == 0x00B9 || r >= 0x2070 && r <= 0x2079 || r >= 0x2080 && r <= 0x2089 || r >= 0x06F0 && r <= 0x06F9:
		return EN
	case r >= 0x0660 && r <= 0x0669 || r == 0x066B || r == 0x066C || r >= 0x0600 && r <= 0x0605 || r == 0x06DD || r == 0x08E2:
		return AN
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return NSM
	case r >= 0x0590 && r <= 0x05FF || r >= 0x07C0 && r <= 0x085F || r >= 0xFB1D && r <= 0xFB4F ||
		r >= 0x10800 && r <= 0x10FFF || r >= 0x1E800 && r <= 0x1EFFF:
		return R
	case r >= 0x0600 && r <= 0x07BF || r >= 0x0860 && r <= 0x08FF || r >= 0xFB50 && r <= 0xFDFF || r >= 0xFE70 && r <= 0xFEFF:
		return AL
	case unicode.IsLetter(r) || unicode.Is(unicode.Mc, r):
		return L
	}
	return ON
}

// HasRTL reports whether s contains right-to-left letters.
func HasRTL(s string) bool {
	for _, r := range s {
		if r >= 0x0590 {
			if c := ClassOf(r); c == R || c == AL {
				return true
			}
		}
	}
	return false
}

// FirstStrong returns the direction of the first strong letter of s
// (dir="auto"): rtl true for R/AL, ok false when s has no strong letter.
func FirstStrong(s string) (rtl, ok bool) {
	for _, r := range s {
		switch ClassOf(r) {
		case L:
			return false, true
		case R, AL:
			return true, true
		}
	}
	return false, false
}

// Levels resolves the embedding level of each character of a line
// (rules W1–W7, N1–N2, I1–I2 and L1) for a paragraph of the given direction.
func Levels(text []rune, rtl bool) []uint8 {
	n := len(text)
	base := uint8(0)
	if rtl {
		base = 1
	}
	sos := L
	if rtl {
		sos = R
	}
	cls := make([]Class, n)
	for i, r := range text {
		cls[i] = ClassOf(r)
	}
	orig := append([]Class(nil), cls...)

	// W1: a non-spacing mark takes the type of the character before it
	prev := sos
	for i, c := range cls {
		switch c {
		case NSM:
			cls[i] = prev
		case BN:
			// ignored: keeps prev
		default:
			prev = c
		}
	}
	// W2: European numbers after an Arabic letter are Arabic numbers; W3: AL -> R
	last := sos
	for i, c := range cls {
		switch c {
		case L, R:
			last = c
		case AL:
			last = AL
			cls[i] = R
		case EN:
			if last == AL {
				cls[i] = AN
			}
		}
	}
	// W4: a single separator between two numbers of the same kind
	for i := 1; i+1 < n; i++ {
		switch {
		case cls[i] == ES && cls[i-1] == EN && cls[i+1] == EN:
			cls[i] = EN
		case cls[i] == CS && cls[i-1] == EN && cls[i+1] == EN:
			cls[i] = EN
		case cls[i] == CS && cls[i-1] == AN && cls[i+1] == AN:
			cls[i] = AN
		}
	}
	// W5: terminators next to European numbers become European numbers
	for i := 0; i < n; i++ {
		if cls[i] != ET {
			continue
		}
		j := i
		for j < n && cls[j] == ET {
			j++
		}
		if i > 0 && cls[i-1] == EN || j < n && cls[j] == EN {
			for k := i; k < j; k++ {
				cls[k] = EN
			}
		}
		i = j
	}
	// W6: remaining separators and terminators are neutral
	for i, c := range cls {
		if c == ES || c == ET || c == CS || c == BN {
			cls[i] = ON
		}
	}
	// W7: European numbers after a left-to-right letter are left-to-right
	last = sos
	for i, c := range cls {
		switch c {
		case L, R:
			last = c
		case EN:
			if last == L {
				cls[i] = L
			}
		}
	}
	// N1/N2: neutrals take the direction around them, else the paragraph's
	strong := func(c Class) (Class, bool) {
		switch c {
		case L:
			return L, true
		case R, AN, EN:
			return R, true
		}
		return 0, false
	}
	for i := 0; i < n; i++ {
		if _, ok := strong(cls[i]); ok {
			continue
		}
		j := i
		for j < n {
			if _, ok := strong(cls[j]); ok {
				break
			}
			j++
		}
		before, after := sos, sos
		if i > 0 {
			before, _ = strong(cls[i-1])
		}
		if j < n {
			after, _ = strong(cls[j])
		}
		dir := sos
		if before == after {
			dir = before
		}
		for k := i; k < j; k++ {
			cls[k] = dir
		}
		i = j
	}
	// I1/I2: levels
	levels := make([]uint8, n)
	for i, c := range cls {
		lv := base
		switch {
		case base == 0 && c == R:
			lv = 1
		case base == 0 && (c == AN || c == EN):
			lv = 2
		case base == 1 && (c == L || c == EN || c == AN):
			lv = 2
		}
		levels[i] = lv
	}
	// L1: trailing white space goes back to the paragraph level
	for i := n - 1; i >= 0 && (orig[i] == WS || orig[i] == BN || orig[i] == S); i-- {
		levels[i] = base
	}
	return levels
}

// Visual returns the display order of the characters (rule L2): visual[i]
// is the logical index shown at position i.
func Visual(levels []uint8) []int {
	order := make([]int, len(levels))
	for i := range order {
		order[i] = i
	}
	hi, lowOdd := uint8(0), uint8(255)
	for _, l := range levels {
		hi = max(hi, l)
		if l%2 == 1 {
			lowOdd = min(lowOdd, l)
		}
	}
	for lv := hi; lv >= lowOdd && lv > 0; lv-- {
		for i := 0; i < len(order); {
			if levels[order[i]] < lv {
				i++
				continue
			}
			j := i
			for j < len(order) && levels[order[j]] >= lv {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}
	return order
}

var mirrors = map[rune]rune{
	'(': ')', ')': '(', '[': ']', ']': '[', '{': '}', '}': '{', '<': '>', '>': '<',
	'«': '»', '»': '«', '‹': '›', '›': '‹', '⁅': '⁆', '⁆': '⁅', '≤': '≥', '≥': '≤',
	'⟨': '⟩', '⟩': '⟨', '〈': '〉', '〉': '〈',
}

// Mirror returns the mirrored glyph of r for right-to-left runs (rule L4).
func Mirror(r rune) rune {
	if m, ok := mirrors[r]; ok {
		return m
	}
	return r
}
