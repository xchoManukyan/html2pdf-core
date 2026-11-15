package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/xchoManukyan/html2pdf-core/internal/pdf"
)

func TestPDFA(t *testing.T) {
	html := `<head><title>Annual report</title><meta name="author" content="Ann Writer"></head>
		<h1 style="font-family: Arial, sans-serif">Report</h1><p style="font-family: Arial, sans-serif"><a href="https://example.com">link</a> € ü</p>`
	out, err := Convert(html, Options{PDFA: true, PageNumbers: true})
	if err != nil {
		if errors.Is(err, ErrPDFAFonts) {
			t.Skip("no system fonts on this machine: ", err)
		}
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"%PDF-1.7", "/Type /Metadata /Subtype /XML", "<pdfaid:part>2</pdfaid:part>", "<pdfaid:conformance>B</pdfaid:conformance>",
		"/OutputIntents [<< /Type /OutputIntent /S /GTS_PDFA1", "/DestOutputProfile", "/ID [<", "/Link /F 4",
		"<dc:title><rdf:Alt><rdf:li xml:lang=\"x-default\">Annual report</rdf:li>", "<rdf:li>Ann Writer</rdf:li>", "/ModDate"} {
		if !strings.Contains(s, want) {
			t.Errorf("PDF/A output lacks %q", want)
		}
	}
	if strings.Contains(s, "/Subtype /Type1") {
		t.Error("PDF/A must not use the built-in (unembedded) fonts, page numbers included")
	}
}

func TestPDFARequiresEmbeddableFonts(t *testing.T) {
	_, err := Convert(`<p>Hello</p>`, Options{PDFA: true, NoSystemFonts: true})
	if !errors.Is(err, ErrPDFAFonts) || !strings.Contains(err.Error(), "Times") && !strings.Contains(err.Error(), "Helvetica") {
		t.Errorf("text without an embeddable font must fail with a clear message, got %v", err)
	}
}

func TestSRGBProfileIsAvailable(t *testing.T) {
	d := pdf.New(100, 100)
	d.PDFA = true
	d.AddPage()
	if !strings.Contains(string(d.Bytes()), "/N 3 /Filter /FlateDecode") {
		t.Error("the sRGB ICC profile must be embedded")
	}
}
