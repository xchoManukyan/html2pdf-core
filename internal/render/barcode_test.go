package render

import (
	"strings"
	"testing"
)

func TestBarcodeElement(t *testing.T) {
	_, c := convertWith(t, `<p>Ticket <barcode code="HELLO WORLD" type="QR" size="0.8" error="Q"> after</p>`+
		`<div><barcode code="4006381333931" type="EAN13" /></div><p>tail</p>`, Options{})
	if strings.Count(c, " h f") < 50 {
		t.Errorf("the QR code and the EAN-13 should be drawn as rectangles, found %d", strings.Count(c, " h f"))
	}
	for _, s := range []string{"(Ticket", "after)", "(tail)", "(4)", "(006381)", "(333931)"} {
		if !strings.Contains(c, s) {
			t.Errorf("missing %s: the text around and under the barcodes must stay", s)
		}
	}
	if textY(t, c, "after") != textY(t, c, "Ticket") {
		t.Error("an inline barcode keeps the line together")
	}
}

func TestBarcodeErrorsAreVisible(t *testing.T) {
	_, c := convertWith(t, `<barcode code="12" type="EAN13">`, Options{})
	if !strings.Contains(c, "barcode:") {
		t.Error("an invalid barcode should show its error instead of disappearing")
	}
}

func TestBarcodeCSS(t *testing.T) {
	_, c := convertWith(t, `<barcode code="ABC-123" type="C128" style="width: 300px; height: 60px; color: #00f">`, Options{})
	if !strings.Contains(c, "0 0 1 rg") {
		t.Error("color applies to the bars (currentColor)")
	}
}
