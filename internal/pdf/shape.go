package pdf

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/xchoManukyan/html2pdf-core/internal/bidi"
	"github.com/xchoManukyan/html2pdf-core/internal/ttf"
)

// Complex scripts: Indic text and Arabic text are shaped with the font's
// OpenType rules (GSUB/GPOS) instead of being drawn character by
// character. The run then carries ActualText, so copying the text gives
// the original characters in logical order.

type shapedRun struct {
	glyphs []ttf.Glyph
	text   []rune // the logical text the clusters index
	width  int    // total advance, font units
}

func isArabicRune(r rune) bool {
	return r >= 0x0600 && r <= 0x06FF || r >= 0x0750 && r <= 0x077F || bidi.IsPresentationForm(r)
}

// shaped returns the shaped form of s, or nil when s is drawn glyph by
// glyph. Arabic text arrives in visual order (as laid out).
func (f *Font) shaped(s string) *shapedRun {
	if f.tt == nil {
		return nil
	}
	if run, ok := f.runs[s]; ok {
		return run
	}
	var run *shapedRun
	indic, arabic := false, false
	for _, r := range s {
		switch {
		case ttf.IsIndic(r):
			indic = true
		case isArabicRune(r):
			arabic = true
		}
	}
	switch {
	case indic:
		text := []rune(s)
		if gs := f.tt.ShapeIndic(text); gs != nil {
			run = &shapedRun{glyphs: gs, text: text}
		}
	case arabic && f.tt.HasArabic():
		logical := reverseClusters([]rune(s))
		base, forms := bidi.Decompose(logical)
		tf := make([]uint8, len(forms))
		for i, fm := range forms {
			if fm >= 0 {
				tf[i] = uint8(fm) + ttf.FormIsol
			}
		}
		if gs := f.tt.ShapeArabic(base, tf); gs != nil {
			run = &shapedRun{glyphs: gs, text: base}
		}
	}
	if run != nil {
		for _, g := range run.glyphs {
			run.width += g.Advance
		}
	}
	if f.runs == nil {
		f.runs = map[string]*shapedRun{}
	}
	f.runs[s] = run
	return run
}

// reverseClusters reverses text, keeping combining marks after their base.
func reverseClusters(rs []rune) []rune {
	out := make([]rune, 0, len(rs))
	end := len(rs)
	for end > 0 {
		start := end - 1
		for start > 0 && unicode.Is(unicode.Mn, rs[start]) {
			start--
		}
		out = append(out, rs[start:end]...)
		end = start
	}
	return out
}

// canShape reports whether the font draws r through shaping although it
// has no glyph for it (an Arabic presentation form whose base letters the
// font has, with Arabic joining rules).
func (f *Font) canShape(r rune) bool {
	if f.tt == nil || !bidi.IsPresentationForm(r) || !f.tt.HasArabic() {
		return false
	}
	base, _ := bidi.Decompose([]rune{r})
	for _, b := range base {
		if f.tt.GlyphIndex(b) == 0 {
			return false
		}
	}
	return true
}

// recordShaped notes which characters each glyph shows (for ToUnicode).
func (f *Font) recordShaped(run *shapedRun) {
	starts := map[int]bool{}
	for _, g := range run.glyphs {
		starts[g.Cluster] = true
	}
	for _, g := range run.glyphs {
		if _, ok := f.used[g.ID]; ok || g.ID == 0 {
			continue
		}
		end := g.Cluster + 1
		for end < len(run.text) && !starts[end] {
			end++
		}
		if g.Cluster < len(run.text) {
			f.used[g.ID] = string(run.text[g.Cluster:min(end, len(run.text))])
		}
	}
}

// showShaped returns the operators that draw a shaped run at size.
func (f *Font) showShaped(run *shapedRun, size float64, flipY bool) string {
	f.refd = true
	f.recordShaped(run)
	unit := size / float64(f.tt.UnitsPerEm)
	yUnit := unit
	if flipY { // the text matrix points y down
		yUnit = -unit
	}
	var sb strings.Builder
	sb.WriteString("/Span << /ActualText <FEFF")
	for _, u := range utf16.Encode(run.text) {
		fmt.Fprintf(&sb, "%04X", u)
	}
	sb.WriteString("> >> BDC ")
	offsets := false
	for _, g := range run.glyphs {
		if g.XOff != 0 || g.YOff != 0 {
			offsets = true
			break
		}
	}
	if !offsets {
		sb.WriteString("[<")
		for _, g := range run.glyphs {
			fmt.Fprintf(&sb, "%04X", g.ID)
			if d := g.Advance - f.tt.Advance(g.ID); d != 0 {
				fmt.Fprintf(&sb, "> %s <", num(-float64(d)*1000/float64(f.tt.UnitsPerEm)))
			}
		}
		sb.WriteString(">] TJ EMC")
		return sb.String()
	}
	// glyph by glyph: each Td moves from the previous glyph's position
	pen, lastX, lastY := 0, 0, 0
	for _, g := range run.glyphs {
		x, y := pen+g.XOff, g.YOff
		if x != lastX || y != lastY {
			fmt.Fprintf(&sb, "%s %s Td ", num(float64(x-lastX)*unit), num(float64(y-lastY)*yUnit))
			lastX, lastY = x, y
		}
		fmt.Fprintf(&sb, "<%04X> Tj ", g.ID)
		pen += g.Advance
	}
	if pen != lastX || lastY != 0 { // leave the pen at the end of the run
		fmt.Fprintf(&sb, "%s %s Td ", num(float64(pen-lastX)*unit), num(float64(-lastY)*yUnit))
	}
	sb.WriteString("EMC")
	return sb.String()
}
