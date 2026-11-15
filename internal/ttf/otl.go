package ttf

import (
	"sync"
	"unicode"
)

// OpenType layout: the GSUB (glyph substitution) and GPOS (glyph
// positioning) tables that complex scripts need. This is a compact engine:
// it applies the lookups of selected features to a glyph buffer, with
// per-glyph feature masks (as Indic shaping requires), lookup flags and
// GDEF glyph classes. Supported: GSUB types 1-7, GPOS types 1, 2, 4, 5, 6,
// 7, 8 and 9 (cursive attachment and reverse chaining are not applied).

// Glyph is one shaped glyph. Advance and offsets are in font units.
type Glyph struct {
	ID      uint16
	Cluster int // index of the first input rune this glyph comes from
	Advance int
	XOff    int
	YOff    int
}

type otTable struct {
	data                                []byte
	scriptList, featureList, lookupList []byte
}

type gdefTable struct {
	classDef, markAttach []byte
	markSets             []byte // MarkGlyphSetsDef
}

type layout struct {
	gsub, gpos *otTable
	gdef       gdefTable
}

var layoutCache sync.Map // *Font -> *layout

func (f *Font) layout() *layout {
	if v, ok := layoutCache.Load(f); ok {
		return v.(*layout)
	}
	l := &layout{gsub: parseOT(f.tables["GSUB"]), gpos: parseOT(f.tables["GPOS"])}
	if g := f.tables["GDEF"]; len(g) >= 12 {
		l.gdef.classDef = sub(g, int(u16(g, 4)))
		l.gdef.markAttach = sub(g, int(u16(g, 10)))
		if u32(g, 0) >= 0x00010002 && len(g) >= 14 {
			l.gdef.markSets = sub(g, int(u16(g, 12)))
		}
	}
	v, _ := layoutCache.LoadOrStore(f, l)
	return v.(*layout)
}

func parseOT(b []byte) *otTable {
	if len(b) < 10 {
		return nil
	}
	t := &otTable{data: b, scriptList: sub(b, int(u16(b, 4))), featureList: sub(b, int(u16(b, 6))), lookupList: sub(b, int(u16(b, 8)))}
	if t.scriptList == nil || t.featureList == nil || t.lookupList == nil {
		return nil
	}
	return t
}

// langSys returns the default language system of the first script found.
func (t *otTable) langSys(scripts []string) []byte {
	if t == nil {
		return nil
	}
	sl := t.scriptList
	n := int(u16(sl, 0))
	for _, want := range scripts {
		for i := 0; i < n; i++ {
			rec := 2 + 6*i
			if string(sl[rec:rec+4]) == want {
				st := sub(sl, int(u16(sl, rec+4)))
				if ls := sub(st, int(u16(st, 0))); ls != nil {
					return ls
				}
				if u16(st, 2) > 0 { // no default: take the first language
					return sub(st, int(u16(st, 6)))
				}
			}
		}
	}
	return nil
}

// HasScript reports whether the font's GSUB has rules for one of the
// script tags (such as "dev2", "deva" or "arab").
func (f *Font) HasScript(tags ...string) bool {
	return f.layout().gsub.langSys(tags) != nil
}

// featureLookups returns the lookup indices of a feature, for the given
// language system.
func (t *otTable) featureLookups(ls []byte, tag string) []int {
	if t == nil || ls == nil {
		return nil
	}
	fl := t.featureList
	var out []int
	add := func(fi int) {
		if fi >= int(u16(fl, 0)) {
			return
		}
		rec := 2 + 6*fi
		if string(fl[rec:rec+4]) != tag {
			return
		}
		ft := sub(fl, int(u16(fl, rec+4)))
		for k := 0; k < int(u16(ft, 2)); k++ {
			out = append(out, int(u16(ft, 4+2*k)))
		}
	}
	if req := u16(ls, 2); req != 0xFFFF {
		add(int(req))
	}
	for i := 0; i < int(u16(ls, 4)); i++ {
		add(int(u16(ls, 6+2*i)))
	}
	return out
}

type lookup struct {
	typ, flag uint16
	subs      [][]byte
	markSet   int
}

func (t *otTable) lookup(i int, gpos bool) *lookup {
	ll := t.lookupList
	if i >= int(u16(ll, 0)) {
		return nil
	}
	lt := sub(ll, int(u16(ll, 2+2*i)))
	if lt == nil {
		return nil
	}
	l := &lookup{typ: u16(lt, 0), flag: u16(lt, 2), markSet: -1}
	n := int(u16(lt, 4))
	ext := uint16(7)
	if gpos {
		ext = 9
	}
	for k := 0; k < n; k++ {
		st := sub(lt, int(u16(lt, 6+2*k)))
		if st == nil {
			continue
		}
		if l.typ == ext { // extension: the real type and subtable
			l.subs = append(l.subs, sub(st, int(u32(st, 4))))
			if k == 0 {
				l.typ = u16(st, 2)
			}
			continue
		}
		l.subs = append(l.subs, st)
	}
	if l.flag&0x10 != 0 {
		l.markSet = int(u16(lt, 6+2*n))
	}
	return l
}

// --- the glyph buffer ---

type bufGlyph struct {
	id       uint16
	cluster  int
	mask     uint32
	class    uint8 // GDEF class: 1 base, 2 ligature, 3 mark, 4 component
	adv      int
	dx, dy   int
	attach   int  // index of the glyph a mark is attached to (-1: none)
	ch       rune // the character the glyph started as
	syl      int  // syllable number (Indic)
	cat, pos uint8
	subst    bool // changed by a substitution
	lig      bool // result of a ligature substitution
}

type shaper struct {
	f    *Font
	l    *layout
	buf  []bufGlyph
	ls   []byte // language system in use
	gpos bool
	look *lookup
}

func (s *shaper) glyphClass(g uint16) uint8 {
	if s.l.gdef.classDef != nil {
		return uint8(classOf(s.l.gdef.classDef, g))
	}
	return 0
}

func (s *shaper) setClass(i int) {
	if c := s.glyphClass(s.buf[i].id); c != 0 || s.l.gdef.classDef != nil {
		s.buf[i].class = c
	}
}

// skip reports whether the current lookup ignores the glyph.
func (s *shaper) skip(g *bufGlyph) bool {
	flag := s.look.flag
	switch g.class {
	case 1:
		return flag&2 != 0
	case 2:
		return flag&4 != 0
	case 3:
		if flag&8 != 0 {
			return true
		}
		if flag&0x10 != 0 {
			return !s.inMarkSet(s.look.markSet, g.id)
		}
		if mat := flag >> 8; mat != 0 {
			return uint16(classOf(s.l.gdef.markAttach, g.id)) != mat
		}
	}
	return false
}

func (s *shaper) inMarkSet(set int, g uint16) bool {
	ms := s.l.gdef.markSets
	if ms == nil || set < 0 || set >= int(u16(ms, 2)) {
		return false
	}
	cov := sub(ms, int(u32(ms, 4+4*set)))
	return coverageIndex(cov, g) >= 0
}

func (s *shaper) next(i int) int {
	for j := i + 1; j < len(s.buf); j++ {
		if !s.skip(&s.buf[j]) {
			return j
		}
	}
	return -1
}

func (s *shaper) prev(i int) int {
	for j := i - 1; j >= 0; j-- {
		if !s.skip(&s.buf[j]) {
			return j
		}
	}
	return -1
}

// applyFeatures applies the lookups of the features (with the masks that
// select the glyphs each one applies to), in lookup-list order.
func (s *shaper) applyFeatures(t *otTable, gpos bool, feats map[string]uint32) {
	if t == nil || s.ls == nil {
		return
	}
	masks := map[int]uint32{}
	for tag, m := range feats {
		for _, li := range t.featureLookups(s.ls, tag) {
			masks[li] |= m
		}
	}
	idx := make([]int, 0, len(masks))
	for li := range masks {
		idx = append(idx, li)
	}
	sortInts(idx)
	s.gpos = gpos
	for _, li := range idx {
		l := t.lookup(li, gpos)
		if l == nil {
			continue
		}
		s.look = l
		mask := masks[li]
		for i := 0; i < len(s.buf); {
			g := &s.buf[i]
			if g.mask&mask == 0 || s.skip(g) {
				i++
				continue
			}
			if next, ok := s.applyAt(t, l, i); ok {
				i = max(next, i+1)
			} else {
				i++
			}
		}
	}
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

// applyAt tries the lookup's subtables at position i. It returns the
// position after the glyphs it consumed.
func (s *shaper) applyAt(t *otTable, l *lookup, i int) (int, bool) {
	saved := s.look
	s.look = l
	defer func() { s.look = saved }()
	for _, st := range l.subs {
		var next int
		var ok bool
		if s.gpos {
			next, ok = s.gposSub(t, l.typ, st, i)
		} else {
			next, ok = s.gsubSub(t, l.typ, st, i)
		}
		if ok {
			return next, true
		}
	}
	return i, false
}

// --- GSUB ---

func (s *shaper) replace(i int, g uint16) {
	s.buf[i].id = g
	s.buf[i].subst = true
	s.setClass(i)
}

func (s *shaper) gsubSub(t *otTable, typ uint16, st []byte, i int) (int, bool) {
	g := s.buf[i].id
	switch typ {
	case 1: // single
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 {
			return i, false
		}
		if u16(st, 0) == 1 {
			s.replace(i, g+u16(st, 4))
		} else {
			if ci >= int(u16(st, 4)) {
				return i, false
			}
			s.replace(i, u16(st, 6+2*ci))
		}
		return i + 1, true
	case 2, 3: // multiple, alternate (the first alternate)
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 || ci >= int(u16(st, 4)) {
			return i, false
		}
		seq := sub(st, int(u16(st, 6+2*ci)))
		n := int(u16(seq, 0))
		if typ == 3 {
			if n == 0 {
				return i, false
			}
			s.replace(i, u16(seq, 2))
			return i + 1, true
		}
		if n == 0 {
			s.buf = append(s.buf[:i], s.buf[i+1:]...) // deletion
			return i, true
		}
		base := s.buf[i]
		repl := make([]bufGlyph, n)
		for k := range repl {
			repl[k] = base
			repl[k].id = u16(seq, 2+2*k)
			repl[k].subst = true
		}
		s.buf = append(s.buf[:i], append(repl, s.buf[i+1:]...)...)
		for k := 0; k < n; k++ {
			s.setClass(i + k)
		}
		return i + n, true
	case 4: // ligature
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 || ci >= int(u16(st, 4)) {
			return i, false
		}
		set := sub(st, int(u16(st, 6+2*ci)))
		for k := 0; k < int(u16(set, 0)); k++ {
			lig := sub(set, int(u16(set, 2+2*k)))
			comps := int(u16(lig, 2))
			pos := []int{i}
			j := i
			ok := true
			for c := 1; c < comps; c++ {
				j = s.next(j)
				if j < 0 || s.buf[j].id != u16(lig, 4+2*(c-1)) {
					ok = false
					break
				}
				pos = append(pos, j)
			}
			if !ok {
				continue
			}
			first := &s.buf[i]
			first.id = u16(lig, 0)
			first.subst, first.lig = true, true
			for _, p := range pos[1:] {
				first.cluster = min(first.cluster, s.buf[p].cluster)
			}
			for c := len(pos) - 1; c >= 1; c-- {
				p := pos[c]
				s.buf = append(s.buf[:p], s.buf[p+1:]...)
			}
			s.setClass(i)
			if s.buf[i].class == 0 {
				s.buf[i].class = 2
			}
			return i + 1, true
		}
		return i, false
	case 5, 6:
		return s.context(t, typ == 6, st, i)
	}
	return i, false
}

// --- contextual lookups (GSUB 5/6, GPOS 7/8) ---

type matcher func(k int, g uint16) bool

func (s *shaper) matchForward(i, count int, m matcher) []int {
	pos := []int{i}
	j := i
	for k := 1; k < count; k++ {
		j = s.next(j)
		if j < 0 || !m(k, s.buf[j].id) {
			return nil
		}
		pos = append(pos, j)
	}
	return pos
}

func (s *shaper) matchBack(i, count int, m matcher) bool {
	j := i
	for k := 0; k < count; k++ {
		j = s.prev(j)
		if j < 0 || !m(k, s.buf[j].id) {
			return false
		}
	}
	return true
}

func (s *shaper) matchAhead(last, count int, m matcher) bool {
	j := last
	for k := 0; k < count; k++ {
		j = s.next(j)
		if j < 0 || !m(k, s.buf[j].id) {
			return false
		}
	}
	return true
}

// applyRecords runs the nested lookups of a matched context.
func (s *shaper) applyRecords(t *otTable, pos []int, recs []byte, n int) int {
	end := pos[len(pos)-1] + 1
	for r := 0; r < n; r++ {
		seqIdx := int(u16(recs, 4*r))
		li := int(u16(recs, 4*r+2))
		if seqIdx >= len(pos) {
			continue
		}
		l := t.lookup(li, s.gpos)
		if l == nil {
			continue
		}
		before := len(s.buf)
		at := pos[seqIdx]
		if s.skipWith(l, at) {
			continue
		}
		s.applyAt(t, l, at)
		if d := len(s.buf) - before; d != 0 {
			for k := range pos {
				if pos[k] > at {
					pos[k] += d
				}
			}
			end += d
		}
	}
	return end
}

func (s *shaper) skipWith(l *lookup, i int) bool {
	saved := s.look
	s.look = l
	defer func() { s.look = saved }()
	return i >= len(s.buf) || s.skip(&s.buf[i])
}

func (s *shaper) context(t *otTable, chained bool, st []byte, i int) (int, bool) {
	g := s.buf[i].id
	format := u16(st, 0)
	switch {
	case !chained && format == 1, chained && format == 1:
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 || ci >= int(u16(st, 4)) {
			return i, false
		}
		set := sub(st, int(u16(st, 6+2*ci)))
		for k := 0; k < int(u16(set, 0)); k++ {
			rule := sub(set, int(u16(set, 2+2*k)))
			if end, ok := s.rule(t, chained, rule, i, func(_ int, want, got uint16) bool { return want == got }, nil, nil, nil); ok {
				return end, true
			}
		}
	case format == 2:
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 {
			return i, false
		}
		var back, input, ahead []byte
		setsAt := 6
		if chained {
			back, input, ahead = sub(st, int(u16(st, 4))), sub(st, int(u16(st, 6))), sub(st, int(u16(st, 8)))
			setsAt = 10
		} else {
			input = sub(st, int(u16(st, 4)))
		}
		c := classOf(input, g)
		if c >= int(u16(st, setsAt)) {
			return i, false
		}
		set := sub(st, int(u16(st, setsAt+2+2*c)))
		if set == nil {
			return i, false
		}
		for k := 0; k < int(u16(set, 0)); k++ {
			rule := sub(set, int(u16(set, 2+2*k)))
			if end, ok := s.rule(t, chained, rule, i, nil, back, input, ahead); ok {
				return end, true
			}
		}
	case format == 3:
		return s.coverageRule(t, chained, st, i)
	}
	return i, false
}

// rule matches a format 1 (glyphs) or format 2 (classes) rule.
func (s *shaper) rule(t *otTable, chained bool, rule []byte, i int, eq func(int, uint16, uint16) bool, backCD, inCD, aheadCD []byte) (int, bool) {
	val := func(cd []byte, want, got uint16) bool {
		if eq != nil {
			return want == got
		}
		return int(want) == classOf(cd, got)
	}
	off := 0
	var backCount int
	if chained {
		backCount = int(u16(rule, 0))
		off = 2 + 2*backCount
	}
	inCount := int(u16(rule, off))
	inOff := off + 2
	if inCount == 0 {
		return i, false
	}
	var aheadCount, aheadOff, recCount, recOff int
	if chained {
		aheadCount = int(u16(rule, inOff+2*(inCount-1)))
		aheadOff = inOff + 2*(inCount-1) + 2
		recCount = int(u16(rule, aheadOff+2*aheadCount))
		recOff = aheadOff + 2*aheadCount + 2
	} else {
		recCount = int(u16(rule, off+2))
		inOff = off + 4
		recOff = inOff + 2*(inCount-1)
	}
	pos := s.matchForward(i, inCount, func(k int, g uint16) bool { return val(inCD, u16(rule, inOff+2*(k-1)), g) })
	if pos == nil {
		return i, false
	}
	if chained {
		if !s.matchBack(i, backCount, func(k int, g uint16) bool { return val(backCD, u16(rule, 2+2*k), g) }) {
			return i, false
		}
		if !s.matchAhead(pos[len(pos)-1], aheadCount, func(k int, g uint16) bool { return val(aheadCD, u16(rule, aheadOff+2*k), g) }) {
			return i, false
		}
	}
	return s.applyRecords(t, pos, rule[recOff:], recCount), true
}

func (s *shaper) coverageRule(t *otTable, chained bool, st []byte, i int) (int, bool) {
	cov := func(base []byte, off int) []byte { return sub(st, int(u16(base, off))) }
	if !chained {
		n := int(u16(st, 2))
		recCount := int(u16(st, 4))
		pos := s.matchForward(i, n, func(k int, g uint16) bool { return coverageIndex(cov(st, 6+2*k), g) >= 0 })
		if pos == nil || coverageIndex(cov(st, 6), s.buf[i].id) < 0 {
			return i, false
		}
		return s.applyRecords(t, pos, st[6+2*n:], recCount), true
	}
	backN := int(u16(st, 2))
	inAt := 4 + 2*backN
	inN := int(u16(st, inAt))
	aheadAt := inAt + 2 + 2*inN
	aheadN := int(u16(st, aheadAt))
	recAt := aheadAt + 2 + 2*aheadN
	if inN == 0 || coverageIndex(cov(st, inAt+2), s.buf[i].id) < 0 {
		return i, false
	}
	pos := s.matchForward(i, inN, func(k int, g uint16) bool { return coverageIndex(cov(st, inAt+2+2*k), g) >= 0 })
	if pos == nil {
		return i, false
	}
	if !s.matchBack(i, backN, func(k int, g uint16) bool { return coverageIndex(cov(st, 4+2*k), g) >= 0 }) {
		return i, false
	}
	if !s.matchAhead(pos[len(pos)-1], aheadN, func(k int, g uint16) bool { return coverageIndex(cov(st, aheadAt+2+2*k), g) >= 0 }) {
		return i, false
	}
	return s.applyRecords(t, pos, st[recAt+2:], int(u16(st, recAt))), true
}

// --- GPOS ---

type valueRec struct{ dx, dy, adv int }

func readValue(b []byte, off int, vf uint16) (valueRec, int) {
	var v valueRec
	for bit := uint16(1); bit <= 0x80; bit <<= 1 {
		if vf&bit == 0 {
			continue
		}
		val := int(int16(u16(b, off)))
		switch bit {
		case 1:
			v.dx = val
		case 2:
			v.dy = val
		case 4:
			v.adv = val
		}
		off += 2
	}
	return v, off
}

func (s *shaper) addValue(i int, v valueRec) {
	s.buf[i].dx += v.dx
	s.buf[i].dy += v.dy
	s.buf[i].adv += v.adv
}

func anchor(b []byte) (int, int, bool) {
	if b == nil {
		return 0, 0, false
	}
	return int(int16(u16(b, 2))), int(int16(u16(b, 4))), true
}

func (s *shaper) gposSub(t *otTable, typ uint16, st []byte, i int) (int, bool) {
	g := s.buf[i].id
	switch typ {
	case 1:
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 {
			return i, false
		}
		vf := u16(st, 4)
		if u16(st, 0) == 1 {
			v, _ := readValue(st, 6, vf)
			s.addValue(i, v)
		} else {
			v, _ := readValue(st, 8+ci*valueSize(vf), vf)
			s.addValue(i, v)
		}
		return i + 1, true
	case 2:
		j := s.next(i)
		if j < 0 {
			return i, false
		}
		ci := coverageIndex(sub(st, int(u16(st, 2))), g)
		if ci < 0 {
			return i, false
		}
		vf1, vf2 := u16(st, 4), u16(st, 6)
		recSize := valueSize(vf1) + valueSize(vf2)
		var rec int = -1
		if u16(st, 0) == 1 {
			if ci >= int(u16(st, 8)) {
				return i, false
			}
			set := sub(st, int(u16(st, 10+2*ci)))
			n := int(u16(set, 0))
			lo, hi := 0, n-1
			for lo <= hi {
				mid := (lo + hi) / 2
				off := 2 + mid*(2+recSize)
				second := u16(set, off)
				switch {
				case second == s.buf[j].id:
					v1, next := readValue(set, off+2, vf1)
					v2, _ := readValue(set, next, vf2)
					s.addValue(i, v1)
					s.addValue(j, v2)
					rec = off
					lo = hi + 1
				case second < s.buf[j].id:
					lo = mid + 1
				default:
					hi = mid - 1
				}
			}
		} else {
			c1 := classOf(sub(st, int(u16(st, 8))), g)
			c2 := classOf(sub(st, int(u16(st, 10))), s.buf[j].id)
			n1, n2 := int(u16(st, 12)), int(u16(st, 14))
			if c1 < n1 && c2 < n2 {
				off := 16 + (c1*n2+c2)*recSize
				v1, next := readValue(st, off, vf1)
				v2, _ := readValue(st, next, vf2)
				s.addValue(i, v1)
				s.addValue(j, v2)
				rec = off
			}
		}
		if rec < 0 {
			return i, false
		}
		if vf2 != 0 {
			return j + 1, true
		}
		return j, true
	case 4, 5, 6: // mark to base, ligature, mark
		if s.buf[i].class != 3 && s.glyphClass(g) != 3 {
			return i, false
		}
		mi := coverageIndex(sub(st, int(u16(st, 2))), g)
		if mi < 0 {
			return i, false
		}
		// the glyph the mark attaches to
		j := -1
		for k := i - 1; k >= 0; k-- {
			c := s.buf[k].class
			if typ == 6 {
				if s.skip(&s.buf[k]) {
					continue
				}
				if c == 3 {
					j = k
				}
				break
			}
			if c != 3 {
				j = k
				break
			}
		}
		if j < 0 {
			return i, false
		}
		bi := coverageIndex(sub(st, int(u16(st, 4))), s.buf[j].id)
		if bi < 0 {
			return i, false
		}
		classCount := int(u16(st, 6))
		markArray := sub(st, int(u16(st, 8)))
		baseArray := sub(st, int(u16(st, 10)))
		markClass := int(u16(markArray, 2+4*mi))
		mx, my, ok := anchor(sub(markArray, int(u16(markArray, 4+4*mi))))
		if !ok || markClass >= classCount {
			return i, false
		}
		var bAnchor []byte
		if typ == 5 {
			attach := sub(baseArray, int(u16(baseArray, 2+2*bi)))
			comps := int(u16(attach, 0))
			if comps == 0 {
				return i, false
			}
			comp := comps - 1 // the last component
			bAnchor = sub(attach, int(u16(attach, 2+2*(comp*classCount+markClass))))
		} else {
			if bi >= int(u16(baseArray, 0)) {
				return i, false
			}
			bAnchor = sub(baseArray, int(u16(baseArray, 2+2*(bi*classCount+markClass))))
		}
		bx, by, ok := anchor(bAnchor)
		if !ok {
			return i, false
		}
		s.buf[i].attach = j
		s.buf[i].dx = bx - mx
		s.buf[i].dy = by - my
		return i + 1, true
	case 7, 8:
		return s.context(t, typ == 8, st, i)
	}
	return i, false
}

// positions resolves mark attachments into offsets relative to each
// glyph's pen position and returns the glyphs.
func (s *shaper) glyphs() []Glyph {
	n := len(s.buf)
	pen := make([]int, n+1)
	for i := range s.buf {
		pen[i+1] = pen[i] + s.buf[i].adv
	}
	out := make([]Glyph, n)
	for i, g := range s.buf {
		x, y := g.dx, g.dy
		if g.attach >= 0 && g.attach < i {
			b := out[g.attach]
			x = pen[g.attach] + b.XOff + g.dx - pen[i]
			y = b.YOff + g.dy
		}
		out[i] = Glyph{ID: g.id, Cluster: g.cluster, Advance: g.adv, XOff: x, YOff: y}
	}
	return out
}

// newShaper fills the buffer with the nominal glyphs of runes.
func (f *Font) newShaper(runes []rune, scripts []string) *shaper {
	l := f.layout()
	s := &shaper{f: f, l: l}
	s.ls = l.gsub.langSys(scripts)
	for i, r := range runes {
		g := bufGlyph{id: f.GlyphIndex(r), cluster: i, mask: ^uint32(0), attach: -1, ch: r}
		s.buf = append(s.buf, g)
		s.setClass(len(s.buf) - 1)
		if l.gdef.classDef == nil && unicode.In(r, unicode.Mn, unicode.Me) {
			s.buf[len(s.buf)-1].class = 3
		}
	}
	return s
}

// position runs GPOS features (with the language system of GPOS).
func (s *shaper) position(scripts []string, feats ...string) {
	for i := range s.buf {
		s.buf[i].adv = s.f.Advance(s.buf[i].id)
	}
	gpos := s.l.gpos
	if gpos == nil {
		return
	}
	saved := s.ls
	s.ls = gpos.langSys(scripts)
	m := map[string]uint32{}
	for _, f := range feats {
		m[f] = ^uint32(0)
	}
	s.applyFeatures(gpos, true, m)
	s.ls = saved
}

// reverseRTL turns a logical right-to-left buffer into visual order,
// keeping each base glyph followed by its marks.
func (s *shaper) reverseRTL() {
	n := len(s.buf)
	var groups [][2]int
	for i := 0; i < n; {
		j := i + 1
		for j < n && s.buf[j].class == 3 {
			j++
		}
		groups = append(groups, [2]int{i, j})
		i = j
	}
	out := make([]bufGlyph, 0, n)
	newIdx := make([]int, n)
	for g := len(groups) - 1; g >= 0; g-- {
		for k := groups[g][0]; k < groups[g][1]; k++ {
			newIdx[k] = len(out)
			out = append(out, s.buf[k])
		}
	}
	for i := range out {
		if a := out[i].attach; a >= 0 {
			out[i].attach = newIdx[a]
		}
	}
	s.buf = out
}
