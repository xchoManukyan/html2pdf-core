package ttf

import "unicode"

// Indic shaping (Devanagari, Bengali, Gurmukhi, Gujarati, Oriya, Tamil,
// Telugu, Kannada, Malayalam), following the OpenType "Indic2" model:
// split the text into syllables, find each syllable's base consonant,
// move pre-base vowel signs and reph, then apply the GSUB features with
// per-glyph masks and position marks with GPOS.

type indicScript struct {
	base        rune
	tags        []string
	reph        uint8 // where reph goes: 0 none, 1 after the base (and below forms), 2 at the end before syllable modifiers
	preMatras   []rune
	splitMatras map[rune][2]rune // two-part vowel signs: pre-base part, post-base part
}

var indicScripts = []indicScript{
	{0x0900, []string{"dev2", "deva"}, 2, []rune{0x093F, 0x094E}, nil},
	{0x0980, []string{"bng2", "beng"}, 1, []rune{0x09BF, 0x09C7, 0x09C8}, map[rune][2]rune{0x09CB: {0x09C7, 0x09BE}, 0x09CC: {0x09C7, 0x09D7}}},
	{0x0A00, []string{"gur2", "guru"}, 0, []rune{0x0A3F}, nil},
	{0x0A80, []string{"gjr2", "gujr"}, 2, []rune{0x0ABF}, nil},
	{0x0B00, []string{"ory2", "orya"}, 1, []rune{0x0B47}, map[rune][2]rune{0x0B48: {0x0B47, 0x0B56}, 0x0B4B: {0x0B47, 0x0B3E}, 0x0B4C: {0x0B47, 0x0B57}}},
	{0x0B80, []string{"tml2", "taml"}, 0, []rune{0x0BC6, 0x0BC7, 0x0BC8}, map[rune][2]rune{0x0BCA: {0x0BC6, 0x0BBE}, 0x0BCB: {0x0BC7, 0x0BBE}, 0x0BCC: {0x0BC6, 0x0BD7}}},
	{0x0C00, []string{"tel2", "telu"}, 0, nil, nil},
	{0x0C80, []string{"knd2", "knda"}, 2, nil, nil},
	{0x0D00, []string{"mlm2", "mlym"}, 0, []rune{0x0D46, 0x0D47, 0x0D48}, map[rune][2]rune{0x0D4A: {0x0D46, 0x0D3E}, 0x0D4B: {0x0D47, 0x0D3E}, 0x0D4C: {0x0D46, 0x0D57}}},
}

// IndicScript returns the script block a rune belongs to (nil: not Indic).
func indicScriptOf(r rune) *indicScript {
	if r < 0x0900 || r > 0x0D7F {
		return nil
	}
	return &indicScripts[(r-0x0900)/0x80]
}

// IsIndic reports whether r belongs to one of the Indic scripts handled here.
func IsIndic(r rune) bool { return indicScriptOf(r) != nil }

// character categories
const (
	icOther uint8 = iota
	icC           // consonant
	icV           // independent vowel
	icN           // nukta
	icH           // virama (halant)
	icM           // vowel sign (matra)
	icSM          // syllable modifier: candrabindu, anusvara, visarga
	icA           // vedic accent
	icZWJ
	icZWNJ
)

// glyph positions in a syllable (sorted for reordering)
const (
	posRaToBecomeReph uint8 = iota
	posPreM
	posPreC
	posBase
	posAfterMain
	posBelowC
	posPostC
	posMatra
	posSMVD
	posEnd
)

func indicCategory(r rune) uint8 {
	switch r {
	case 0x200D:
		return icZWJ
	case 0x200C:
		return icZWNJ
	}
	sc := indicScriptOf(r)
	if sc == nil {
		return icOther
	}
	o := r - sc.base
	switch {
	case o >= 0x15 && o <= 0x39, o >= 0x58 && o <= 0x5F, sc.base == 0x0900 && o >= 0x78 && o <= 0x7F,
		sc.base == 0x0980 && (o == 0x70 || o == 0x71), sc.base == 0x0D00 && o == 0x3A:
		return icC
	case o >= 0x04 && o <= 0x14, o == 0x60 || o == 0x61, sc.base == 0x0900 && o >= 0x72 && o <= 0x77:
		return icV
	case o == 0x3C:
		return icN
	case o == 0x4D:
		return icH
	case o >= 0x01 && o <= 0x03, o == 0x00 && sc.base == 0x0900:
		return icSM
	case o >= 0x51 && o <= 0x54:
		return icA
	case o == 0x3A || o == 0x3B || o >= 0x3E && o <= 0x4C || o == 0x4E || o == 0x4F || o >= 0x55 && o <= 0x57 || o == 0x62 || o == 0x63:
		if unicode.In(r, unicode.Mn, unicode.Mc) {
			return icM
		}
	}
	return icOther
}

// ShapeIndic shapes text of one Indic script (logical order). It returns
// nil when the font has no layout rules for the script.
func (f *Font) ShapeIndic(text []rune) []Glyph {
	var sc *indicScript
	for _, r := range text {
		if sc = indicScriptOf(r); sc != nil {
			break
		}
	}
	if sc == nil || !f.HasScript(sc.tags...) {
		return nil
	}
	// split two-part vowel signs; remember the input index of every rune
	runes := make([]rune, 0, len(text)+4)
	src := make([]int, 0, len(text)+4)
	for i, r := range text {
		if p, ok := sc.splitMatras[r]; ok {
			runes = append(runes, p[0], p[1])
			src = append(src, i, i)
			continue
		}
		runes = append(runes, r)
		src = append(src, i)
	}
	s := f.newShaper(runes, sc.tags)
	for i := range s.buf {
		s.buf[i].cluster = src[i]
		s.buf[i].cat = indicCategory(runes[i])
		s.buf[i].pos = posEnd
	}
	// locl and ccmp first, on everything
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"locl": ^uint32(0), "ccmp": ^uint32(0)})

	const (
		mRphf = 1 << iota
		mHalf
		mPref
		mBlwf
		mAbvf
		mPstf
		mGlobal = 1 << 20
	)
	// syllables
	var out []bufGlyph
	n := 0
	for i := 0; i < len(s.buf); {
		end := s.syllableEnd(i)
		syl := append([]bufGlyph(nil), s.buf[i:end]...)
		for k := range syl {
			syl[k].syl = n
		}
		n++
		out = append(out, s.reorderSyllable(sc, syl, mRphf, mHalf, mBlwf|mAbvf|mPstf|mPref, mGlobal)...)
		i = end
	}
	s.buf = out
	// basic shaping features, one stage each, in order
	for _, st := range []struct {
		tag  string
		mask uint32
	}{{"nukt", mGlobal}, {"akhn", mGlobal}, {"rphf", mRphf}, {"rkrf", mGlobal}, {"pref", mPref}, {"blwf", mBlwf},
		{"abvf", mAbvf}, {"half", mHalf}, {"pstf", mPstf}, {"vatu", mGlobal}, {"cjct", mGlobal}} {
		s.applyFeatures(s.l.gsub, false, map[string]uint32{st.tag: st.mask})
	}
	s.finalReorder(sc)
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"pres": mGlobal, "abvs": mGlobal, "blws": mGlobal,
		"psts": mGlobal, "haln": mGlobal, "calt": mGlobal, "clig": mGlobal})
	// zero-width joiners and non-joiners are not drawn
	kept := s.buf[:0]
	for _, g := range s.buf {
		if g.cat == icZWJ || g.cat == icZWNJ {
			if g.id == 0 || !g.subst {
				continue
			}
		}
		kept = append(kept, g)
	}
	s.buf = kept
	s.position(sc.tags, "kern", "dist", "abvm", "blwm", "mark", "mkmk")
	return s.glyphs()
}

// syllableEnd returns the end of the syllable starting at i.
func (s *shaper) syllableEnd(i int) int {
	n := len(s.buf)
	cat := func(k int) uint8 {
		if k < n {
			return s.buf[k].cat
		}
		return 255
	}
	j := i
	switch cat(j) {
	case icC:
		for {
			j++ // the consonant
			if cat(j) == icN {
				j++
			}
			if cat(j) == icH {
				k := j + 1
				if cat(k) == icZWJ || cat(k) == icZWNJ {
					k++
				}
				if cat(k) == icC {
					j = k
					continue
				}
				j = k // final halant (with joiner)
				return j
			}
			break
		}
	case icV:
		j++
		if cat(j) == icN {
			j++
		}
	default:
		if cat(j) != icM && cat(j) != icSM && cat(j) != icN && cat(j) != icH {
			return j + 1
		}
	}
	for j < n {
		switch cat(j) {
		case icM, icN, icSM, icA:
			j++
			continue
		case icH:
			j++
			continue
		case icZWJ, icZWNJ:
			if cat(j+1) == icM {
				j++
				continue
			}
		}
		break
	}
	if j == i {
		j++
	}
	return j
}

// wouldSubstitute reports whether a feature turns the glyph sequence into
// something else (used to find below-base and post-base forms).
func (s *shaper) wouldSubstitute(tag string, glyphs ...uint16) bool {
	t := s.l.gsub
	for _, li := range t.featureLookups(s.ls, tag) {
		l := t.lookup(li, false)
		if l == nil {
			continue
		}
		probe := &shaper{f: s.f, l: s.l, ls: s.ls}
		for _, g := range glyphs {
			probe.buf = append(probe.buf, bufGlyph{id: g, mask: 1, attach: -1})
		}
		for i := range probe.buf {
			probe.setClass(i)
		}
		probe.look = l
		if _, ok := probe.applyAt(t, l, 0); ok && (len(probe.buf) != len(glyphs) || probe.buf[0].id != glyphs[0]) {
			return true
		}
	}
	return false
}

func (s *shaper) reorderSyllable(sc *indicScript, syl []bufGlyph, mRphf, mHalf, mPost, mGlobal uint32) []bufGlyph {
	for k := range syl {
		syl[k].mask = mGlobal
	}
	if len(syl) == 0 || syl[0].cat != icC {
		for k := range syl {
			syl[k].pos = posBase
			if syl[k].cat == icSM || syl[k].cat == icA {
				syl[k].pos = posSMVD
			}
		}
		return syl
	}
	// reph: Ra + halant at the start of a syllable with more consonants
	start := 0
	hasReph := false
	if sc.reph != 0 && len(syl) >= 3 && syl[0].ch == sc.base+0x30 && syl[1].cat == icH && syl[2].cat == icC &&
		s.wouldSubstitute("rphf", syl[0].id, syl[1].id) {
		hasReph, start = true, 2
	}
	// base: the last consonant that has no below-base or post-base form
	base := -1
	for k := len(syl) - 1; k >= start; k-- {
		if syl[k].cat != icC {
			continue
		}
		if k-1 > start && syl[k-1].cat == icH {
			h := syl[k-1].id
			if s.wouldSubstitute("blwf", h, syl[k].id) || s.wouldSubstitute("blwf", syl[k].id, h) ||
				s.wouldSubstitute("pstf", h, syl[k].id) || s.wouldSubstitute("pstf", syl[k].id, h) {
				continue
			}
		}
		base = k
		break
	}
	if base < 0 {
		for k := start; k < len(syl); k++ {
			if syl[k].cat == icC {
				base = k
				break
			}
		}
	}
	for k := range syl {
		g := &syl[k]
		switch {
		case hasReph && k < 2:
			g.pos = posRaToBecomeReph
			g.mask |= mRphf
		case k < base:
			g.pos = posPreC
			g.mask |= mHalf
		case k == base:
			g.pos = posBase
		default:
			g.mask |= mPost
			switch {
			case g.cat == icM && isPreMatra(sc, g.ch):
				g.pos = posPreM
			case g.cat == icM:
				g.pos = posMatra
			case g.cat == icSM || g.cat == icA:
				g.pos = posSMVD
			case g.cat == icC:
				g.pos = posBelowC
			case g.cat == icN || g.cat == icH:
				g.pos = syl[k-1].pos // stays with the consonant before it
				if g.pos == posPreM || g.pos == posMatra {
					g.pos = posMatra
				}
			default:
				g.pos = posAfterMain
			}
		}
	}
	// stable sort by position: pre-base matras move before the consonants
	order := make([]bufGlyph, 0, len(syl))
	for p := posRaToBecomeReph; p <= posEnd; p++ {
		for _, g := range syl {
			if g.pos == p {
				order = append(order, g)
			}
		}
	}
	return order
}

func isPreMatra(sc *indicScript, r rune) bool {
	for _, m := range sc.preMatras {
		if m == r {
			return true
		}
	}
	return false
}

// finalReorder moves reph to its place and a pre-base matra past any
// halant that did not form a ligature, syllable by syllable.
func (s *shaper) finalReorder(sc *indicScript) {
	for i := 0; i < len(s.buf); {
		end := i + 1
		for end < len(s.buf) && s.buf[end].syl == s.buf[i].syl {
			end++
		}
		s.finalSyllable(sc, s.buf[i:end])
		i = end
	}
}

func (s *shaper) finalSyllable(sc *indicScript, buf []bufGlyph) {
	// reph: Ra + halant that rphf turned into one glyph
	if len(buf) > 1 && buf[0].pos == posRaToBecomeReph && buf[0].subst && buf[1].pos != posRaToBecomeReph {
		reph := buf[0]
		target := 0
		for k := 1; k < len(buf); k++ {
			if sc.reph == 1 && buf[k].pos <= posBelowC || sc.reph != 1 && buf[k].pos < posSMVD {
				target = k
			}
		}
		copy(buf[0:target], buf[1:target+1])
		buf[target] = reph
	}
	// pre-base matra: after the last standalone halant before the base
	for k := 0; k < len(buf); k++ {
		if buf[k].pos != posPreM {
			continue
		}
		last := -1
		for j := k + 1; j < len(buf) && buf[j].pos < posBase; j++ {
			if buf[j].cat == icH && !buf[j].subst {
				last = j
			}
		}
		if last > k {
			m := buf[k]
			copy(buf[k:last], buf[k+1:last+1])
			buf[last] = m
		}
		break
	}
}
