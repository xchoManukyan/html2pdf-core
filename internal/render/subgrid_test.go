package render

import (
	"os"
	"strings"
	"testing"
)

func TestNamedGridLines(t *testing.T) {
	_, c := convertWith(t, `<style>body{margin:0}.g{display:grid;grid-template-columns:[full-start] 50px [main-start] 100px [main-end] 50px [full-end]}
		.r{display:grid;grid-template-columns:repeat(4,[col] 60px) [last]}</style>
		<div class="g"><div style="grid-column:main-start / main-end">MMM</div><div style="grid-column:full-start / full-end">NNN</div><div style="grid-column:main-end">HHH</div></div>
		<div class="r"><div style="grid-column:col 2">III</div><div style="grid-column:col -1">EEE</div></div>`, Options{MarginMM: 10})
	left := textX(t, c, "NNN")
	for word, px := range map[string]float64{"MMM": 50, "HHH": 150, "III": 60, "EEE": 180} {
		if got := (textX(t, c, word) - left) / 0.75; !near(got, px, 1) {
			t.Errorf("%s at %.1fpx, want %vpx", word, got, px)
		}
	}
}

func TestSubgridRowsAlignAcrossCards(t *testing.T) {
	_, c := convertWith(t, `<style>body{margin:0;font:16px Helvetica;line-height:20px}
		.cards{display:grid;grid-template-columns:repeat(3,150px);column-gap:20px}
		.card{display:grid;grid-row:span 3;grid-template-rows:subgrid;padding:4px}
		h2,p{margin:0;font-size:16px}</style>
		<div class="cards">
		<div class="card"><h2>One</h2><p>short</p><div>FOOTA</div></div>
		<div class="card"><h2>Two with a much longer title that wraps</h2><p>text</p><div>FOOTB</div></div>
		<div class="card"><h2>Three</h2><p>long body text that wraps over several lines</p><div>FOOTC</div></div>
		</div>`, Options{})
	if y1, y2 := textY(t, c, "short"), textY(t, c, "text"); y1 != y2 {
		t.Errorf("second rows must line up across cards: %.1f vs %.1f", y1, y2)
	}
	fa, fb, fc := textY(t, c, "FOOTA"), textY(t, c, "FOOTB"), textY(t, c, "FOOTC")
	if fa != fb || fb != fc {
		t.Errorf("footers must line up: %.1f %.1f %.1f", fa, fb, fc)
	}
}

func TestSubgridColumns(t *testing.T) {
	_, c := convertWith(t, `<style>body{margin:0}.cols{display:grid;grid-template-columns:120px 60px 120px;column-gap:10px}
		.inner{grid-column:1 / 4;display:grid;grid-template-columns:subgrid}</style>
		<div class="cols"><div class="inner"><div>SONE</div><div>STWO</div><div>STHREE</div></div></div>`, Options{})
	x1 := textX(t, c, "SONE")
	for word, px := range map[string]float64{"STWO": 130, "STHREE": 200} {
		if got := (textX(t, c, word) - x1) / 0.75; !near(got, px, 1) {
			t.Errorf("%s at %.1fpx, want %vpx (the parent's columns)", word, got, px)
		}
	}
}

// TestGridCompareSample keeps the sample used for the Chromium comparison parseable.
func TestGridCompareSample(t *testing.T) {
	if p := os.Getenv("GRID_SAMPLE"); p != "" {
		b, _ := os.ReadFile(p)
		out, c := convertWith(t, string(b), Options{MarginMM: 5})
		_ = out
		for _, w := range []string{"MMM", "NNN", "HHH", "III", "EEE", "UUU", "777", "1111", "2222", "3333", "4444", "5555", "6666"} {
			t.Logf("%s x=%.1fpx y=%.1fpx", w, (textX(t, c, w)-14.17)/0.75, (841.89-14.17-textY(t, c, w))/0.75)
		}
		if !strings.Contains(c, "MMM") {
			t.Fatal("sample not rendered")
		}
	}
}
