package barcode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCode128Table(t *testing.T) {
	for i, p := range code128Patterns {
		sum := 0
		for _, c := range p {
			sum += int(c - '0')
		}
		want := 11
		if i == c128Stop {
			want = 13
		}
		if sum != want {
			t.Errorf("symbol %d (%s) is %d modules wide, want %d", i, p, sum, want)
		}
	}
}

func TestCode128SetChoice(t *testing.T) {
	for text, modules := range map[string]int{
		"123456":       11*(1+3+1) + 13,     // start C, three pairs, check
		"ABC":          11*(1+3+1) + 13,     // start B, three letters
		"ORDER-123456": 11*(1+6+1+3+1) + 13, // B for the text, C for the digits
		"12345":        11*(1+2+1+1+1) + 13, // C C, switch to B for the odd digit
	} {
		l, err := Code128(text)
		if err != nil {
			t.Fatal(err)
		}
		if len(l.Modules) != modules {
			t.Errorf("%q: %d modules, want %d", text, len(l.Modules), modules)
		}
	}
	if _, err := Code128("Ünï"); err == nil {
		t.Error("non-ASCII text must be rejected")
	}
}

func TestCode39Table(t *testing.T) {
	for r, p := range code39 {
		if strings.Count(p, "1") != 3 || len(p) != 9 {
			t.Errorf("%q: %s must have 9 elements, 3 of them wide", r, p)
		}
	}
}

func TestEAN(t *testing.T) {
	l, err := EAN13("400638133393") // check digit 1 is added
	if err != nil || l.Text != "4006381333931" || len(l.Modules) != 95 {
		t.Fatalf("EAN-13: %v %q %d", err, l.Text, len(l.Modules))
	}
	if _, err := EAN13("4006381333932"); err == nil || !strings.Contains(err.Error(), "check digit") {
		t.Errorf("a wrong check digit must be reported: %v", err)
	}
	if l, _ := EAN8("9638507"); l == nil || l.Text != "96385074" || len(l.Modules) != 67 {
		t.Error("EAN-8 with check digit 4, 67 modules")
	}
	if l, _ := UPCA("03600029145"); l == nil || l.Text != "036000291452" {
		t.Error("UPC-A check digit 2")
	}
	if l, _ := ISBN("0-306-40615-2"); l == nil || l.Text != "9780306406157" {
		t.Error("ISBN-10 to ISBN-13")
	}
	if l, _ := ISSN("0317-8471"); l == nil || !strings.HasPrefix(l.Text, "977031784700") {
		t.Errorf("ISSN as EAN-13: %v", l)
	}
}

func TestI25(t *testing.T) {
	l, err := I25("1234567", false) // odd: a leading 0 is added
	if err != nil || l.Text != "01234567" {
		t.Fatalf("%v %q", err, l.Text)
	}
	// start 4 + per pair (2 wide + 3 narrow) * 2 = 18 + stop 5
	if len(l.Modules) != 4+4*18+5 {
		t.Errorf("%d modules", len(l.Modules))
	}
}

func TestQRStructure(t *testing.T) {
	for _, c := range []struct {
		text string
		ecl  ECC
		ver  int
	}{
		{"HELLO WORLD", ECCQuartile, 1},
		{"01234567", ECCMedium, 1},
		{"https://example.com/invoice/2024-001?total=1290.00", ECCMedium, 4},
		{strings.Repeat("A", 1000), ECCLow, 0},
		{strings.Repeat("x", 2953), ECCLow, 40},
	} {
		m, err := QR(c.text, c.ecl)
		if err != nil {
			t.Fatalf("%q: %v", c.text[:min(10, len(c.text))], err)
		}
		ver := (m.Size - 17) / 4
		if c.ver > 0 && ver != c.ver {
			t.Errorf("%q: version %d, want %d", c.text[:min(10, len(c.text))], ver, c.ver)
		}
		// timing pattern and the dark module
		for i := 8; i < m.Size-8; i++ {
			if m.Dark[6][i] != (i%2 == 0) || m.Dark[i][6] != (i%2 == 0) {
				t.Fatalf("timing pattern broken at %d", i)
			}
		}
		if !m.Dark[m.Size-8][8] {
			t.Error("dark module missing")
		}
	}
	if _, err := QR(strings.Repeat("x", 2954), ECCLow); err != ErrTooLong {
		t.Error("too long text must fail")
	}
}

func TestSVGAndErrors(t *testing.T) {
	s, err := SVG("qr", "hello", Options{Size: 1})
	if err != nil || !strings.HasPrefix(s, "<svg") || !strings.Contains(s, `width="94.48`) {
		t.Errorf("QR SVG 25 mm wide: %v %.80s", err, s)
	}
	if _, err := SVG("pdf417", "x", Options{}); err == nil || !strings.Contains(err.Error(), "supported") {
		t.Errorf("unknown types are reported with the list: %v", err)
	}
	s, _ = SVG("EAN13", "4006381333931", Options{Text: -1})
	if strings.Count(s, "<text") != 3 {
		t.Error("EAN-13 shows its digits in three groups")
	}
}

// TestWriteSamples writes SVGs for decoding with a real scanner library
// when BARCODE_SAMPLES names a folder.
func TestWriteSamples(t *testing.T) {
	dir := os.Getenv("BARCODE_SAMPLES")
	if dir == "" {
		t.Skip("set BARCODE_SAMPLES to write sample SVGs")
	}
	samples := []struct{ typ, code, ecc string }{
		{"QR", "HELLO WORLD", "Q"}, {"QR", "01234567890123456789", "M"},
		{"QR", "https://example.com/invoice/2024-001?total=1290.00&cur=EUR", "M"},
		{"QR", "Grüße — Привет — 你好", "H"}, {"QR", strings.Repeat("Version test. ", 60), "L"},
		{"C128", "ORDER-123456", ""}, {"C128", "12345", ""}, {"C128", "Hello, World!", ""},
		{"EAN13", "4006381333931", ""}, {"EAN8", "96385074", ""}, {"UPCA", "036000291452", ""},
		{"ISBN", "978-0-306-40615-7", ""}, {"C39", "CODE 39-$/+%", ""}, {"I25", "12345670", ""},
	}
	os.MkdirAll(dir, 0o755)
	var list []string
	for i, s := range samples {
		svg, err := SVG(s.typ, s.code, Options{Size: 1, Text: 0, ECC: s.ecc, Color: "#000"})
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(dir, string(rune('a'+i))+".svg")
		os.WriteFile(name, []byte(svg), 0o644)
		list = append(list, `{"file":"`+string(rune('a'+i))+`.svg","type":"`+s.typ+`","code":`+jsonString(s.code)+`}`)
	}
	os.WriteFile(filepath.Join(dir, "samples.json"), []byte("["+strings.Join(list, ",")+"]"), 0o644)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
