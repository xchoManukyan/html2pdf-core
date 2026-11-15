package ttf

// Arabic shaping with the font's own rules, for fonts that have no
// presentation-form characters: the joining form of every letter is
// chosen by the caller (forms), and the font's init/medi/fina/isol, rlig
// (lam-alef), calt and liga features and mark positioning are applied.

// Joining forms passed to ShapeArabic.
const (
	FormNone uint8 = iota // not a joining letter
	FormIsol
	FormFina
	FormInit
	FormMedi
)

var arabicTags = []string{"arab"}

// HasArabic reports whether the font has Arabic joining rules.
func (f *Font) HasArabic() bool {
	l := f.layout()
	ls := l.gsub.langSys(arabicTags)
	return ls != nil && len(l.gsub.featureLookups(ls, "fina")) > 0
}

// ShapeArabic shapes Arabic text in logical order; forms gives the joining
// form of each rune. The glyphs are returned in visual (left-to-right) order;
// Cluster still indexes text.
func (f *Font) ShapeArabic(text []rune, forms []uint8) []Glyph {
	if !f.HasArabic() {
		return nil
	}
	s := f.newShaper(text, arabicTags)
	const (
		mIsol = 1 << iota
		mFina
		mInit
		mMedi
		mGlobal
	)
	for i := range s.buf {
		m := uint32(mGlobal)
		if i < len(forms) {
			switch forms[i] {
			case FormIsol:
				m |= mIsol
			case FormFina:
				m |= mFina
			case FormInit:
				m |= mInit
			case FormMedi:
				m |= mMedi
			}
		}
		s.buf[i].mask = m
	}
	all := uint32(^uint32(0))
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"ccmp": all, "locl": all})
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"isol": mIsol})
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"fina": mFina})
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"medi": mMedi})
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"init": mInit})
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"rlig": all})
	s.applyFeatures(s.l.gsub, false, map[string]uint32{"calt": all, "liga": all, "mset": all})
	s.position(arabicTags, "mark", "mkmk")
	for i := range s.buf {
		if s.buf[i].class == 3 {
			s.buf[i].adv = 0 // marks take no space
		}
	}
	s.reverseRTL()
	return s.glyphs()
}
