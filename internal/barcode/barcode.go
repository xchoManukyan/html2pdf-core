// Package barcode draws QR codes and the common one-dimensional barcodes
// (Code 128, EAN-13/8, UPC-A, ISBN, ISSN, Code 39, Interleaved 2 of 5) as
// SVG, for the <barcode> element (the same attributes as mPDF's).
package barcode

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// Options are the <barcode> attributes besides code and type.
type Options struct {
	Size    float64 // scale: 1 = standard size (0.33 mm modules, 25 mm QR codes)
	Height  float64 // extra scale of the bar height of 1D codes
	Text    int     // human-readable text: 1 show, 0 hide, -1 the type's default
	ECC     string  // QR error correction: L, M (default), Q or H
	NoQuiet bool    // leave out the quiet zone around the code (mPDF: disableborder)
	Color   string  // bar colour (default currentColor)
}

// Types lists the supported type names.
const Types = "QR, C128 (also C128A, C128B, C128C), EAN13, EAN8, UPCA, ISBN, ISSN, C39, C39+, I25, I25+"

const mmToPx = 96 / 25.4

// SVG draws the barcode as an SVG document sized in CSS pixels.
func SVG(typ, code string, o Options) (string, error) {
	if o.Size <= 0 {
		o.Size = 1
	}
	if o.Height <= 0 {
		o.Height = 1
	}
	color := o.Color
	if color == "" {
		color = "currentColor"
	}
	color = html.EscapeString(color)
	t := strings.ToUpper(strings.TrimSpace(typ))
	if t == "QR" || t == "QRCODE" {
		return qrSVG(code, o, color)
	}

	var l *Linear
	var err error
	ean := false
	quietL, quietR := 10, 10
	switch t {
	case "C128", "C128A", "C128B", "C128C", "CODE128", "":
		l, err = Code128(code)
	case "EAN13", "EAN", "EAN-13":
		l, err = EAN13(code)
		ean, quietL, quietR = true, 11, 7
	case "ISBN":
		l, err = ISBN(code)
		ean, quietL, quietR = true, 11, 7
	case "ISSN":
		l, err = ISSN(code)
		ean, quietL, quietR = true, 11, 7
	case "UPCA", "UPC-A", "UPC":
		l, err = UPCA(code)
		ean, quietL, quietR = true, 9, 9
	case "EAN8", "EAN-8":
		l, err = EAN8(code)
		ean, quietL, quietR = true, 7, 7
	case "C39", "CODE39":
		l, err = Code39(code, false)
	case "C39+", "CODE39+":
		l, err = Code39(code, true)
	case "I25", "ITF", "I2OF5":
		l, err = I25(code, false)
	case "I25+", "ITF+":
		l, err = I25(code, true)
	default:
		return "", fmt.Errorf("unknown barcode type %q (supported: %s)", typ, Types)
	}
	if err != nil {
		return "", err
	}
	if o.NoQuiet && !ean {
		quietL, quietR = 0, 0
	}

	showText := o.Text == 1 || o.Text == -1 && ean
	n := len(l.Modules)
	moduleMM := 0.33 * o.Size
	barH := 15 / 0.33 * o.Height // in modules (15 mm at size 1)
	if ean {
		barH = 22.85 / 0.33 * o.Height
	}
	fontSize := 9.0 // modules (about 3 mm)
	textH := 0.0
	if showText {
		textH = fontSize * 1.1
		if ean {
			textH = fontSize * 0.6 // EAN digits sit beside the longer guard bars
		}
	}
	totalW := float64(quietL + n + quietR)
	totalH := barH + textH
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" fill="%s">`,
		f(totalW*moduleMM*mmToPx), f(totalH*moduleMM*mmToPx), f(totalW), f(totalH), color)
	guardExtra := 0.0
	if ean && showText {
		guardExtra = fontSize * 0.55
	}
	for i := 0; i < n; {
		if !l.Modules[i] {
			i++
			continue
		}
		j := i
		guard := l.Guards != nil && l.Guards[i]
		for j < n && l.Modules[j] && (l.Guards == nil || l.Guards[j] == guard) {
			j++
		}
		h := barH
		if ean && showText && !guard {
			h = barH - guardExtra // regular bars stop above the digits
		}
		fmt.Fprintf(&b, `<rect x="%d" y="0" width="%d" height="%s"/>`, quietL+i, j-i, f(h))
		i = j
	}
	if showText {
		style := fmt.Sprintf(`font-family="Helvetica, Arial, sans-serif" font-size="%s" text-anchor="middle"`, f(fontSize))
		if ean {
			y := barH - guardExtra + fontSize*0.8
			for _, g := range l.Groups {
				cx := float64(quietL) + float64(g.From+g.To)/2
				fmt.Fprintf(&b, `<text x="%s" y="%s" %s>%s</text>`, f(cx), f(y), style, html.EscapeString(g.Text))
			}
		} else {
			fmt.Fprintf(&b, `<text x="%s" y="%s" %s>%s</text>`, f(totalW/2), f(barH+fontSize), style, html.EscapeString(l.Text))
		}
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

func qrSVG(code string, o Options, color string) (string, error) {
	ecl := ECCMedium
	switch strings.ToUpper(strings.TrimSpace(o.ECC)) {
	case "L":
		ecl = ECCLow
	case "Q":
		ecl = ECCQuartile
	case "H":
		ecl = ECCHigh
	}
	m, err := QR(code, ecl)
	if err != nil {
		return "", err
	}
	quiet := 4
	if o.NoQuiet {
		quiet = 0
	}
	total := m.Size + 2*quiet
	px := 25 * o.Size * mmToPx // 25 mm at size 1
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %d %d" fill="%s" shape-rendering="crispEdges">`,
		f(px), f(px), total, total, color)
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; {
			if !m.Dark[y][x] {
				x++
				continue
			}
			end := x
			for end < m.Size && m.Dark[y][end] {
				end++
			}
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="1"/>`, quiet+x, quiet+y, end-x)
			x = end
		}
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

// ErrorSVG is shown in place of a barcode that cannot be drawn.
func ErrorSVG(msg string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="240" height="40" viewBox="0 0 240 40">` +
		`<rect x="0.5" y="0.5" width="239" height="39" fill="#fff0f0" stroke="#c00"/>` +
		`<text x="6" y="24" font-family="Helvetica, Arial, sans-serif" font-size="10" fill="#c00">` +
		html.EscapeString(truncate("barcode: "+msg, 60)) + `</text></svg>`
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func f(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ParseOptions reads mPDF's <barcode> attributes.
func ParseOptions(attr func(string) string) Options {
	o := Options{Text: -1}
	o.Size, _ = strconv.ParseFloat(strings.TrimSpace(attr("size")), 64)
	o.Height, _ = strconv.ParseFloat(strings.TrimSpace(attr("height")), 64)
	switch strings.TrimSpace(attr("text")) {
	case "1", "true", "yes":
		o.Text = 1
	case "0", "false", "no":
		o.Text = 0
	}
	o.ECC = attr("error")
	switch strings.TrimSpace(attr("disableborder")) {
	case "1", "true", "yes":
		o.NoQuiet = true
	}
	o.Color = strings.TrimSpace(attr("color"))
	return o
}
