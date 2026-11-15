package render

import (
	"strings"
	"testing"
)

func TestSoftHyphen(t *testing.T) {
	// fits: the soft hyphen is invisible
	_, c := convertWith(t, `<p>super&shy;cali&shy;fragilistic</p>`, Options{})
	if strings.Contains(c, "-") && strings.Contains(c, "(super-") || !strings.Contains(c, "supercalifragilistic") {
		t.Errorf("an unused soft hyphen must not show:\n%s", c)
	}
	// narrow box: breaks at a soft hyphen and shows "-"
	_, c = convertWith(t, `<p style="width:90px;font:16px Helvetica">super&shy;cali&shy;fragilistic</p>`, Options{})
	if !strings.Contains(c, "(super-)") && !strings.Contains(c, "(supercali-)") {
		t.Errorf("the word should break at a soft hyphen with a visible hyphen:\n%s", c)
	}
	// hyphens: none ignores soft hyphens (the word overflows or splits by characters)
	_, c = convertWith(t, `<p style="width:90px;font:16px Helvetica;hyphens:none">super&shy;cali&shy;fragilistic</p>`, Options{})
	if strings.Contains(c, "-)") {
		t.Error("hyphens: none must not break at soft hyphens")
	}
}

func TestHyphensAuto(t *testing.T) {
	text := `Hyphenation makes justified typesetting considerably more even.`
	_, c := convertWith(t, `<p lang="en" style="width:120px;font:16px Helvetica;hyphens:auto">`+text+`</p>`, Options{})
	if !strings.Contains(c, "-)") {
		t.Errorf("hyphens: auto should hyphenate in a narrow column:\n%s", c)
	}
	for _, bad := range []string{"(Hyp-)", "(Hyphenatio-)", "(eve-)"} {
		if strings.Contains(c, bad) {
			t.Errorf("break %s is not an English hyphenation point", bad)
		}
	}
	// no lang: browsers don't hyphenate automatically
	_, c = convertWith(t, `<p style="width:120px;font:16px Helvetica;hyphens:auto">`+text+`</p>`, Options{})
	if strings.Contains(c, "-)") {
		t.Error("without a lang attribute there is no automatic hyphenation")
	}
	// inherited lang and -webkit-hyphens
	_, c = convertWith(t, `<html lang="en-GB"><body><div style="-webkit-hyphens:auto"><p style="width:120px;font:16px Helvetica">`+text+`</p></div>`, Options{})
	if !strings.Contains(c, "-)") {
		t.Error("lang and hyphens are inherited")
	}
}

func TestBreakAfterHyphen(t *testing.T) {
	_, c := convertWith(t, `<p style="width:110px;font:16px Helvetica">a state-of-the-art design</p>`, Options{})
	if !strings.Contains(c, "-)") || strings.Contains(c, "--") {
		t.Errorf("a compound may break after one of its hyphens, without adding another:\n%s", c)
	}
}
