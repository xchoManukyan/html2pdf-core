package bidi

import "testing"

func visual(s string, rtl bool) string {
	r := []rune(s)
	order := Visual(Levels(r, rtl))
	out := make([]rune, len(r))
	lv := Levels(r, rtl)
	for i, k := range order {
		c := r[k]
		if lv[k]%2 == 1 {
			c = Mirror(c)
		}
		out[i] = c
	}
	return string(out)
}

func TestReorder(t *testing.T) {
	for _, c := range []struct {
		in   string
		rtl  bool
		want string
	}{
		{"Hello שלום world", false, "Hello םולש world"},
		{"שלום world", true, "world םולש"},
		{"מחיר 120 ש\"ח", true, "ח\"ש 120 ריחמ"},
		{"(שלום)", true, "(םולש)"}, // brackets mirror: the visual result still opens on the left
		{"abc", true, "abc"},
		{"נקודה.", true, ".הדוקנ"},
		{"version 1.2.3 של", false, "version 1.2.3 לש"},
		{"العدد ١٢٣ هنا", true, "انه ١٢٣ ددعلا"},
	} {
		if got := visual(c.in, c.rtl); got != c.want {
			t.Errorf("%q (rtl %v): got %q, want %q", c.in, c.rtl, got, c.want)
		}
	}
}

func TestFirstStrong(t *testing.T) {
	if rtl, ok := FirstStrong("123 שלום abc"); !rtl || !ok {
		t.Error("first strong letter is Hebrew")
	}
	if _, ok := FirstStrong("123 ..."); ok {
		t.Error("no strong letter")
	}
}

func TestShape(t *testing.T) {
	for in, want := range map[string]string{
		"سلام": "ﺳﻼﻡ", // seen initial, lam-alef final (the ligature is obligatory), meem isolated
		"لا":   "ﻻ",   // lam-alef ligature
		"سلا":  "ﺳﻼ",  // final lam-alef after a joining letter
		"كتاب": "ﻛﺘﺎﺏ",
		"ب":    "ﺏ",
		"abc":  "abc",
		"پدر":  "ﭘﺪﺭ",  // Persian peh initial, dal final, reh isolated
		"بَاب": "ﺑَﺎﺏ", // a fatha between beh and alef does not break the join
	} {
		if got := Shape(in); got != want {
			t.Errorf("Shape(%q) = %U, want %U", in, []rune(got), []rune(want))
		}
	}
}
