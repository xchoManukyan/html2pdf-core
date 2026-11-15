package render

import (
	"unicode"

	"github.com/xchoManukyan/html2pdf-core/internal/bidi"
)

// visualLine puts the fragments of one line (in logical order) into display
// order with the Unicode Bidirectional Algorithm: right-to-left runs are
// reversed (keeping marks with their letters) and their brackets mirrored.
// Lines without right-to-left text in a left-to-right block are returned
// unchanged.
func (e *engine) visualLine(frags []frag, block *style) []frag {
	rtl := block.direction == "rtl"
	if !rtl {
		has := false
		for _, f := range frags {
			if bidi.HasRTL(f.text) {
				has = true
				break
			}
		}
		if !has {
			return frags
		}
	}
	// the line as text: owner[k] is the fragment of rune k, or -(i+1) for
	// the space before fragment i; boxes and gaps count as one neutral
	var runes []rune
	var owner []int
	for i, f := range frags {
		if i > 0 && f.space > 0 {
			runes = append(runes, ' ')
			owner = append(owner, -(i + 1))
		}
		if f.atomic != nil || f.text == "" {
			runes = append(runes, '￼')
			owner = append(owner, i)
			continue
		}
		for _, r := range f.text {
			runes = append(runes, r)
			owner = append(owner, i)
		}
	}
	if len(runes) == 0 {
		return frags
	}
	levels := bidi.Levels(runes, rtl)

	// pieces: runs of one fragment at one level
	type piece struct {
		owner int
		level uint8
		text  []rune
	}
	var pieces []piece
	for k := 0; k < len(runes); {
		j := k
		for j < len(runes) && owner[j] == owner[k] && levels[j] == levels[k] {
			j++
		}
		pieces = append(pieces, piece{owner[k], levels[k], runes[k:j]})
		k = j
	}
	plv := make([]uint8, len(pieces))
	for i, p := range pieces {
		plv[i] = p.level
	}

	var out []frag
	space := 0.0
	for _, idx := range bidi.Visual(plv) {
		p := pieces[idx]
		if p.owner < 0 {
			space += frags[-p.owner-1].space
			continue
		}
		f := frags[p.owner]
		nf := f
		if f.atomic == nil && f.text != "" {
			txt := p.text
			if p.level%2 == 1 {
				txt = reverseClusters(txt)
			}
			nf.text = string(txt)
			if nf.text != f.text {
				nf.w = e.textWidth(f.st, nf.text)
			}
		}
		nf.space = space
		space = 0
		out = append(out, nf)
	}
	return out
}

// reverseClusters reverses a right-to-left run for drawing, keeping each
// letter's combining marks after it, and mirrors brackets.
func reverseClusters(text []rune) []rune {
	var clusters [][]rune
	for _, r := range text {
		if len(clusters) > 0 && (unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r)) {
			clusters[len(clusters)-1] = append(clusters[len(clusters)-1], r)
			continue
		}
		clusters = append(clusters, []rune{bidi.Mirror(r)})
	}
	out := make([]rune, 0, len(text))
	for i := len(clusters) - 1; i >= 0; i-- {
		out = append(out, clusters[i]...)
	}
	return out
}
