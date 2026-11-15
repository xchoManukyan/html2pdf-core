package bidi

import "unicode"

// Arabic shaping: letters change shape with their neighbours. Shape picks
// each letter's isolated, final, initial or medial form from the Unicode
// presentation forms (which common fonts such as Arial, Times New Roman,
// Tahoma, Segoe UI and DejaVu contain) and joins lam + alef into the
// obligatory ligature. Text is in logical order before and after.

type joining uint8

const (
	jNone        joining = iota // U: does not join
	jRight                      // R: joins to the preceding letter only
	jDual                       // D: joins on both sides
	jCausing                    // C: tatweel, makes neighbours join
	jTransparent                // T: marks, skipped when finding neighbours
)

// forms[base] = presentation forms: isolated, final, initial, medial (0 = none)
var forms = map[rune][4]rune{
	0x0621: {0xFE80, 0, 0, 0},
	0x0622: {0xFE81, 0xFE82, 0, 0}, 0x0623: {0xFE83, 0xFE84, 0, 0}, 0x0624: {0xFE85, 0xFE86, 0, 0},
	0x0625: {0xFE87, 0xFE88, 0, 0}, 0x0626: {0xFE89, 0xFE8A, 0xFE8B, 0xFE8C}, 0x0627: {0xFE8D, 0xFE8E, 0, 0},
	0x0628: {0xFE8F, 0xFE90, 0xFE91, 0xFE92}, 0x0629: {0xFE93, 0xFE94, 0, 0}, 0x062A: {0xFE95, 0xFE96, 0xFE97, 0xFE98},
	0x062B: {0xFE99, 0xFE9A, 0xFE9B, 0xFE9C}, 0x062C: {0xFE9D, 0xFE9E, 0xFE9F, 0xFEA0}, 0x062D: {0xFEA1, 0xFEA2, 0xFEA3, 0xFEA4},
	0x062E: {0xFEA5, 0xFEA6, 0xFEA7, 0xFEA8}, 0x062F: {0xFEA9, 0xFEAA, 0, 0}, 0x0630: {0xFEAB, 0xFEAC, 0, 0},
	0x0631: {0xFEAD, 0xFEAE, 0, 0}, 0x0632: {0xFEAF, 0xFEB0, 0, 0}, 0x0633: {0xFEB1, 0xFEB2, 0xFEB3, 0xFEB4},
	0x0634: {0xFEB5, 0xFEB6, 0xFEB7, 0xFEB8}, 0x0635: {0xFEB9, 0xFEBA, 0xFEBB, 0xFEBC}, 0x0636: {0xFEBD, 0xFEBE, 0xFEBF, 0xFEC0},
	0x0637: {0xFEC1, 0xFEC2, 0xFEC3, 0xFEC4}, 0x0638: {0xFEC5, 0xFEC6, 0xFEC7, 0xFEC8}, 0x0639: {0xFEC9, 0xFECA, 0xFECB, 0xFECC},
	0x063A: {0xFECD, 0xFECE, 0xFECF, 0xFED0}, 0x0641: {0xFED1, 0xFED2, 0xFED3, 0xFED4}, 0x0642: {0xFED5, 0xFED6, 0xFED7, 0xFED8},
	0x0643: {0xFED9, 0xFEDA, 0xFEDB, 0xFEDC}, 0x0644: {0xFEDD, 0xFEDE, 0xFEDF, 0xFEE0}, 0x0645: {0xFEE1, 0xFEE2, 0xFEE3, 0xFEE4},
	0x0646: {0xFEE5, 0xFEE6, 0xFEE7, 0xFEE8}, 0x0647: {0xFEE9, 0xFEEA, 0xFEEB, 0xFEEC}, 0x0648: {0xFEED, 0xFEEE, 0, 0},
	0x0649: {0xFEEF, 0xFEF0, 0xFBE8, 0xFBE9}, 0x064A: {0xFEF1, 0xFEF2, 0xFEF3, 0xFEF4},
	// Persian and Urdu letters (Presentation Forms-A)
	0x067E: {0xFB56, 0xFB57, 0xFB58, 0xFB59}, 0x0686: {0xFB7A, 0xFB7B, 0xFB7C, 0xFB7D}, 0x0698: {0xFB8A, 0xFB8B, 0, 0},
	0x06A9: {0xFB8E, 0xFB8F, 0xFB90, 0xFB91}, 0x06AF: {0xFB92, 0xFB93, 0xFB94, 0xFB95}, 0x06CC: {0xFBFC, 0xFBFD, 0xFBFE, 0xFBFF},
	0x0679: {0xFB66, 0xFB67, 0xFB68, 0xFB69}, 0x0688: {0xFB88, 0xFB89, 0, 0}, 0x0691: {0xFB8C, 0xFB8D, 0, 0},
	0x06BA: {0xFB9E, 0xFB9F, 0, 0}, 0x06BE: {0xFBAA, 0xFBAB, 0xFBAC, 0xFBAD}, 0x06C1: {0xFBA6, 0xFBA7, 0xFBA8, 0xFBA9},
	0x06D2: {0xFBAE, 0xFBAF, 0, 0}, 0x0671: {0xFB50, 0xFB51, 0, 0},
}

// lam-alef ligatures: alef variant -> isolated, final
var lamAlef = map[rune][2]rune{0x0622: {0xFEF5, 0xFEF6}, 0x0623: {0xFEF7, 0xFEF8}, 0x0625: {0xFEF9, 0xFEFA}, 0x0627: {0xFEFB, 0xFEFC}}

func joiningOf(r rune) joining {
	switch {
	case r == 0x0640 || r == 0x200D: // tatweel, zero-width joiner
		return jCausing
	case unicode.Is(unicode.Mn, r) || r == 0x0670 || r == 0x200B:
		return jTransparent
	}
	if f, ok := forms[r]; ok {
		switch {
		case f[2] != 0:
			return jDual
		case f[1] != 0:
			return jRight
		}
		return jNone
	}
	if r >= 0x0620 && r <= 0x06FF && unicode.IsLetter(r) {
		return jDual // letters without presentation forms in the table: keep as they are
	}
	return jNone
}

// NeedsShaping reports whether s contains Arabic letters.
func NeedsShaping(s string) bool {
	for _, r := range s {
		if r >= 0x0620 && r <= 0x06FF {
			return true
		}
	}
	return false
}

// Shape returns s with Arabic letters replaced by their contextual forms.
func Shape(s string) string {
	if !NeedsShaping(s) {
		return s
	}
	in := []rune(s)
	n := len(in)
	types := make([]joining, n)
	for i, r := range in {
		types[i] = joiningOf(r)
	}
	// neighbour that is not a mark
	prevJoins := func(i int) bool {
		for k := i - 1; k >= 0; k-- {
			if types[k] != jTransparent {
				return types[k] == jDual || types[k] == jCausing
			}
		}
		return false
	}
	nextIdx := func(i int) int {
		for k := i + 1; k < n; k++ {
			if types[k] != jTransparent {
				return k
			}
		}
		return -1
	}
	nextJoins := func(i int) bool {
		k := nextIdx(i)
		return k >= 0 && (types[k] == jDual || types[k] == jRight || types[k] == jCausing)
	}
	out := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		r := in[i]
		f, ok := forms[r]
		if !ok {
			out = append(out, r)
			continue
		}
		p := prevJoins(i)
		if r == 0x0644 { // lam + alef
			if k := nextIdx(i); k >= 0 {
				if lig, ok := lamAlef[in[k]]; ok {
					if p {
						out = append(out, lig[1])
					} else {
						out = append(out, lig[0])
					}
					out = append(out, in[i+1:k]...) // marks on the lam stay
					i = k
					continue
				}
			}
		}
		nx := nextJoins(i) && types[i] == jDual
		var c rune
		switch {
		case p && nx && f[3] != 0:
			c = f[3]
		case p && f[1] != 0:
			c = f[1]
		case nx && f[2] != 0:
			c = f[2]
		default:
			c = f[0]
		}
		out = append(out, c)
	}
	return string(out)
}

// Unshape maps presentation forms back to base letters (for fonts that do
// not contain the forms).
func Unshape(r rune) []rune {
	for base, f := range forms {
		for _, c := range f {
			if c == r && c != 0 {
				return []rune{base}
			}
		}
	}
	for alef, lig := range lamAlef {
		if lig[0] == r || lig[1] == r {
			return []rune{0x0644, alef}
		}
	}
	return []rune{r}
}

// IsPresentationForm reports whether r is an Arabic presentation form.
func IsPresentationForm(r rune) bool {
	return r >= 0xFB50 && r <= 0xFDFF || r >= 0xFE70 && r <= 0xFEFF
}

// Decompose maps text in logical order to base letters and the joining
// form of each (0 isolated, 1 final, 2 initial, 3 medial; -1 not a joining
// letter). Presentation forms keep the form they encode; base letters get
// the form their neighbours in text give them.
func Decompose(text []rune) (base []rune, forms []int) {
	for _, r := range text {
		if IsPresentationForm(r) {
			if alef, isFinal, ok := lamAlefOf(r); ok {
				lam := 2 // initial lam
				if isFinal {
					lam = 3
				}
				base = append(base, 0x0644, alef)
				forms = append(forms, lam, 1)
				continue
			}
			if b, form, ok := formOf(r); ok {
				base = append(base, b)
				forms = append(forms, form)
				continue
			}
		}
		base = append(base, r)
		forms = append(forms, -2) // decide from context
	}
	for i := range forms {
		if forms[i] != -2 {
			continue
		}
		forms[i] = -1
		if t := joiningOf(base[i]); t == jDual || t == jRight {
			forms[i] = contextForm(base, i)
		}
	}
	return base, forms
}

func formOf(r rune) (rune, int, bool) {
	for b, f := range forms {
		for k, c := range f {
			if c == r && c != 0 {
				return b, k, true
			}
		}
	}
	return 0, 0, false
}

func lamAlefOf(r rune) (alef rune, final, ok bool) {
	for a, lig := range lamAlef {
		if lig[0] == r {
			return a, false, true
		}
		if lig[1] == r {
			return a, true, true
		}
	}
	return 0, false, false
}

// contextForm decides the form of a joining letter without presentation
// forms from its neighbours.
func contextForm(text []rune, i int) int {
	joins := func(k int, step int) bool {
		for k += step; k >= 0 && k < len(text); k += step {
			t := joiningOf(text[k])
			if t == jTransparent {
				continue
			}
			if step < 0 {
				return t == jDual || t == jCausing
			}
			return t == jDual || t == jRight || t == jCausing
		}
		return false
	}
	p := joins(i, -1)
	n := joins(i, 1) && joiningOf(text[i]) == jDual
	switch {
	case p && n:
		return 3
	case p:
		return 1
	case n:
		return 2
	}
	return 0
}
