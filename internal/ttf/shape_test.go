package ttf

import (
	"os"
	"testing"
)

func loadTestFont(t *testing.T, name string) *Font {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// ids lists the glyph IDs.
func ids(gs []Glyph) []uint16 {
	out := make([]uint16, len(gs))
	for i, g := range gs {
		out[i] = g.ID
	}
	return out
}

func TestDevanagari(t *testing.T) {
	f := loadTestFont(t, "NotoSansDevanagari-Regular.ttf")
	nominal := func(r rune) uint16 { return f.GlyphIndex(r) }

	// हिन्दी: the i-matra is drawn first, before the consonant
	gs := f.ShapeIndic([]rune("हिन्दी"))
	if len(gs) == 0 || gs[0].Cluster != 1 {
		t.Fatalf("हिन्दी: the i-matra should come first: %+v", gs)
	}
	// कर्म: reph (Ra + virama) becomes one glyph after Ma
	gs = f.ShapeIndic([]rune("कर्म"))
	if len(gs) != 3 || gs[0].ID != nominal('क') || gs[1].ID != nominal('म') || gs[2].Cluster != 1 || gs[2].ID == nominal('र') {
		t.Errorf("कर्म: want ka, ma, reph; got %+v", gs)
	}
	// शर्मा: reph after the aa-matra, as Chrome (HarfBuzz) draws it
	gs = f.ShapeIndic([]rune("शर्मा"))
	if len(gs) != 4 || gs[3].Cluster != 1 || gs[2].ID != nominal('ा') {
		t.Errorf("शर्मा: want sha, ma, aa, reph; got %+v", gs)
	}
	// प्र: the conjunct pa + rakaar is one glyph
	gs = f.ShapeIndic([]rune("प्र"))
	if len(gs) != 1 {
		t.Errorf("प्र: want one conjunct glyph, got %v", ids(gs))
	}
	// क्षत्रिय: kssa ligature, and the i-matra goes before the tra conjunct
	gs = f.ShapeIndic([]rune("क्षत्रिय"))
	if len(gs) != 4 || gs[1].Cluster != 6 || gs[2].Cluster != 3 {
		t.Errorf("क्षत्रिय: got %+v", gs)
	}
	// स्थिति: half sa, the i-matra before the whole cluster
	gs = f.ShapeIndic([]rune("स्थिति"))
	if len(gs) != 5 || gs[0].Cluster != 3 || gs[1].Cluster != 0 {
		t.Errorf("स्थिति: got %+v", gs)
	}
}

func TestTamilSplitVowel(t *testing.T) {
	f := loadTestFont(t, "NotoSansTamil-Regular.ttf")
	// மொ = ம + ொ, drawn as ெ ம ா
	gs := f.ShapeIndic([]rune("மொ"))
	if len(gs) != 3 || gs[0].ID != f.GlyphIndex(0x0BC6) || gs[1].ID != f.GlyphIndex('ம') || gs[2].ID != f.GlyphIndex(0x0BBE) {
		t.Errorf("மொ: got %v", ids(gs))
	}
	// கை: the ai sign goes before ka
	gs = f.ShapeIndic([]rune("கை"))
	if len(gs) != 2 || gs[0].ID != f.GlyphIndex(0x0BC8) {
		t.Errorf("கை: got %v", ids(gs))
	}
}

func TestArabicGSUB(t *testing.T) {
	f := loadTestFont(t, "NotoSansArabic-Regular.ttf")
	if !f.HasArabic() {
		t.Fatal("Noto Sans Arabic has Arabic joining rules")
	}
	// بب: initial and final forms are other glyphs than the isolated one
	// (Noto draws them as a dotless letter plus a separate dot)
	gs := f.ShapeArabic([]rune{0x0628, 0x0628}, []uint8{FormInit, FormFina})
	iso := f.GlyphIndex(0x0628)
	var letters []Glyph
	for _, g := range gs {
		if g.Advance > 0 {
			letters = append(letters, g)
		}
	}
	if len(letters) != 2 || letters[0].ID == iso || letters[1].ID == iso || letters[0].ID == letters[1].ID {
		t.Errorf("beh beh: got %v", ids(gs))
	}
	// visual order: the final form (logically second) is on the left
	if letters[0].Cluster != 1 {
		t.Errorf("glyphs should be in visual order, got clusters %d %d", letters[0].Cluster, letters[1].Cluster)
	}
	// لا: lam + alef take their joined (ligature) shapes
	gs = f.ShapeArabic([]rune{0x0644, 0x0627}, []uint8{FormInit, FormFina})
	for _, g := range gs {
		if g.ID == f.GlyphIndex(0x0644) || g.ID == f.GlyphIndex(0x0627) {
			t.Errorf("lam-alef: the isolated letters remain: %v", ids(gs))
		}
	}
	// a damma is placed on its letter by GPOS mark attachment
	gs = f.ShapeArabic([]rune{0x0645, 0x064F}, []uint8{FormIsol, FormNone})
	if len(gs) != 2 || gs[1].XOff == 0 || gs[1].Advance != 0 {
		t.Errorf("meem + damma: the mark should be moved onto the letter with no advance, got %+v", gs)
	}
}
