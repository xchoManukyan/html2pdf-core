package hyphen

import "testing"

func TestEnglish(t *testing.T) {
	us := For("en-US")
	for word, want := range map[string]string{
		"hyphenation":   "hy-phen-ation",
		"computer":      "com-puter",
		"algorithm":     "al-go-rithm",
		"typesetting":   "type-set-ting",
		"international": "in-ter-na-tional",
		"Extraordinary": "Ex-tra-or-di-nary",
		"table":         "ta-ble",  // exception list
		"project":       "project", // exception list: no break
		"cat":           "cat",     // too short
	} {
		if got := us.Hyphenate(word); got != want {
			t.Errorf("%s: got %s, want %s", word, got, want)
		}
	}
	if For("en-GB") == us || For("en-GB") == nil {
		t.Error("en-GB has its own patterns")
	}
	if For("en") != us || For("") != nil || For("xx") != nil {
		t.Error("en means US English; no tag or an unknown language: none")
	}
}

func TestOtherLanguages(t *testing.T) {
	for _, c := range [][3]string{
		{"de", "Silbentrennung", "Sil-ben-tren-nung"},
		{"de-AT", "Donaudampfschifffahrt", "Do-nau-dampf-schiff-fahrt"},
		{"fr-CA", "hyphénation", "hy-phé-na-tion"},
		{"it", "sillabazione", "sil-la-ba-zio-ne"},
		{"ru", "переносить", "пе-ре-но-сить"},
		{"pl", "dzielenie", "dzie-le-nie"},
		{"nl", "lettergrepen", "let-ter-gre-pen"},
		{"sr", "Србија", "Ср-би-ја"},            // Serbian Cyrillic via the Serbo-Croatian patterns
		{"no", "orddeling", "ord-de-ling"},      // Norwegian means Bokmål
		{"hi", "हिन्दीभाषा", "हि-न्दी-भा-षा"},   // vowel signs stay with their letters
		{"el", "συλλαβισμός", "συλ-λα-βι-σμός"}, // modern Greek
		{"hy", "Հայաստանի", "Հա-յաստա-նի"},
	} {
		if got := For(c[0]).Hyphenate(c[1]); got != c[2] {
			t.Errorf("%s %s: got %s, want %s", c[0], c[1], got, c[2])
		}
	}
	if n := len(Languages()); n < 70 {
		t.Errorf("%d pattern sets", n)
	}
	for _, gpl := range []string{"cs", "id", "mk", "ro"} {
		if For(gpl) != nil {
			t.Errorf("%s patterns are not included (licence)", gpl)
		}
	}
}
