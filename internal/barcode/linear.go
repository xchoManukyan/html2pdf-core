package barcode

import (
	"fmt"
	"strings"
)

// Linear is a one-dimensional barcode: Modules[i] is true for a bar.
// Text is the human-readable line; for EAN/UPC the guard bars are longer
// and the digits are placed between them (Groups).
type Linear struct {
	Modules []bool
	Text    string
	Guards  []bool  // per module: part of a guard pattern (drawn longer); nil for none
	Groups  []Group // EAN/UPC digit groups under the bars
}

// Group is a run of human-readable digits below modules [From, To).
type Group struct {
	From, To int
	Text     string
}

func widths(pattern string, out *[]bool, bar bool) bool {
	for _, c := range pattern {
		for i := 0; i < int(c-'0'); i++ {
			*out = append(*out, bar)
		}
		bar = !bar
	}
	return bar
}

// ---- Code 128 ----

var code128Patterns = [107]string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312", "132212", "221213",
	"221312", "231212", "112232", "122132", "122231", "113222", "123122", "123221", "223211", "221132",
	"221231", "213212", "223112", "312131", "311222", "321122", "321221", "312212", "322112", "322211",
	"212123", "212321", "232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
	"231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121", "313121", "211331",
	"231131", "213113", "213311", "213131", "311123", "311321", "331121", "312113", "312311", "332111",
	"314111", "221411", "431111", "111224", "111422", "121124", "121421", "141122", "141221", "112214",
	"112412", "122114", "122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
	"111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112", "421211", "212141",
	"214121", "412121", "111143", "111341", "131141", "114113", "114311", "411113", "411311", "113141",
	"114131", "311141", "411131", "211412", "211214", "211232", "2331112",
}

const (
	c128CodeC, c128CodeB, c128CodeA    = 99, 100, 101
	c128StartA, c128StartB, c128StartC = 103, 104, 105
	c128Stop                           = 106
)

// Code128 encodes text (ASCII 0–127), choosing code sets A, B and C as a
// reader would expect: C for runs of digits, B for text, A for control characters.
func Code128(text string) (*Linear, error) {
	for _, r := range text {
		if r > 127 {
			return nil, fmt.Errorf("Code 128 cannot encode %q; use a QR code for non-ASCII text", r)
		}
	}
	if text == "" {
		return nil, fmt.Errorf("Code 128 needs some text")
	}
	digitsAt := func(i int) int { // length of the digit run starting at i
		n := 0
		for i+n < len(text) && text[i+n] >= '0' && text[i+n] <= '9' {
			n++
		}
		return n
	}
	var codes []int
	set := 0 // 'A', 'B' or 'C'
	for i := 0; i < len(text); {
		d := digitsAt(i)
		var want int
		switch {
		case d >= 4, set == 'C' && d >= 2, set == 0 && d >= 2 && d == len(text) && d%2 == 0:
			want = 'C' // pairs of digits; an odd last digit falls back to A or B
		case text[i] < 32:
			want = 'A'
		case set == 'A' && text[i] < 96:
			want = 'A' // stay: A covers capitals, digits and punctuation
		default:
			want = 'B'
		}
		if want != set {
			switch {
			case set == 0 && want == 'A':
				codes = append(codes, c128StartA)
			case set == 0 && want == 'B':
				codes = append(codes, c128StartB)
			case set == 0 && want == 'C':
				codes = append(codes, c128StartC)
			case want == 'A':
				codes = append(codes, c128CodeA)
			case want == 'B':
				codes = append(codes, c128CodeB)
			default:
				codes = append(codes, c128CodeC)
			}
			set = want
		}
		switch set {
		case 'C':
			codes = append(codes, int(text[i]-'0')*10+int(text[i+1]-'0'))
			i += 2
		case 'A':
			c := int(text[i])
			if c < 32 {
				c += 64
			} else {
				c -= 32
			}
			codes = append(codes, c)
			i++
		default:
			codes = append(codes, int(text[i])-32)
			i++
		}
	}
	sum := codes[0]
	for i, c := range codes[1:] {
		sum += c * (i + 1)
	}
	codes = append(codes, sum%103, c128Stop)
	l := &Linear{Text: text}
	for _, c := range codes {
		widths(code128Patterns[c], &l.Modules, true)
	}
	return l, nil
}

// ---- EAN / UPC ----

var eanL = [10]string{"0001101", "0011001", "0010011", "0111101", "0100011", "0110001", "0101111", "0111011", "0110111", "0001011"}
var eanG = [10]string{"0100111", "0110011", "0011011", "0100001", "0011101", "0111001", "0000101", "0010001", "0001001", "0010111"}
var eanParity = [10]string{"LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG", "LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL"}

func eanR(d int) string {
	b := []byte(eanL[d])
	for i := range b {
		b[i] ^= 1 // R is the complement of L
	}
	return string(b)
}

// checkDigit is the GS1 mod-10 check digit of the digits (weights 3,1,3...
// from the right).
func checkDigit(digits string) int {
	sum := 0
	for i := len(digits) - 1; i >= 0; i-- {
		w := 1
		if (len(digits)-1-i)%2 == 0 {
			w = 3
		}
		sum += int(digits[i]-'0') * w
	}
	return (10 - sum%10) % 10
}

func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' {
			return -1
		}
		return 'x'
	}, s)
}

// gtin checks or completes a code of n digits (the last one the check digit).
func gtin(code string, n int) (string, error) {
	d := onlyDigits(code)
	if strings.Contains(d, "x") {
		return "", fmt.Errorf("%q must contain only digits", code)
	}
	switch len(d) {
	case n - 1:
		return d + string(rune('0'+checkDigit(d))), nil
	case n:
		if int(d[n-1]-'0') != checkDigit(d[:n-1]) {
			return "", fmt.Errorf("%q has a wrong check digit (should be %d)", code, checkDigit(d[:n-1]))
		}
		return d, nil
	}
	return "", fmt.Errorf("%q must have %d digits (or %d without the check digit)", code, n, n-1)
}

func (l *Linear) add(pattern string, guard bool) {
	for _, c := range pattern {
		l.Modules = append(l.Modules, c == '1')
		l.Guards = append(l.Guards, guard)
	}
}

// EAN13 encodes 12 or 13 digits (the check digit is added or verified).
func EAN13(code string) (*Linear, error) {
	d, err := gtin(code, 13)
	if err != nil {
		return nil, err
	}
	l := &Linear{Text: d}
	l.add("101", true)
	parity := eanParity[d[0]-'0']
	for i := 1; i <= 6; i++ {
		if parity[i-1] == 'L' {
			l.add(eanL[d[i]-'0'], false)
		} else {
			l.add(eanG[d[i]-'0'], false)
		}
	}
	l.add("01010", true)
	for i := 7; i <= 12; i++ {
		l.add(eanR(int(d[i]-'0')), false)
	}
	l.add("101", true)
	l.Groups = []Group{{-9, -2, d[:1]}, {3, 45, d[1:7]}, {50, 92, d[7:]}}
	return l, nil
}

// UPCA encodes 11 or 12 digits; it is an EAN-13 with a leading zero.
func UPCA(code string) (*Linear, error) {
	d, err := gtin(code, 12)
	if err != nil {
		return nil, err
	}
	l, err := EAN13("0" + d)
	if err != nil {
		return nil, err
	}
	l.Text = d
	l.Groups = []Group{{-9, -2, d[:1]}, {10, 45, d[1:6]}, {50, 85, d[6:11]}, {97, 104, d[11:]}}
	// UPC-A draws the first and last digit bars long as well
	for i := 3; i < 10; i++ {
		l.Guards[i] = true
	}
	for i := 85; i < 92; i++ {
		l.Guards[i] = true
	}
	return l, nil
}

// EAN8 encodes 7 or 8 digits.
func EAN8(code string) (*Linear, error) {
	d, err := gtin(code, 8)
	if err != nil {
		return nil, err
	}
	l := &Linear{Text: d}
	l.add("101", true)
	for i := 0; i < 4; i++ {
		l.add(eanL[d[i]-'0'], false)
	}
	l.add("01010", true)
	for i := 4; i < 8; i++ {
		l.add(eanR(int(d[i]-'0')), false)
	}
	l.add("101", true)
	l.Groups = []Group{{3, 31, d[:4]}, {36, 64, d[4:]}}
	return l, nil
}

// ISBN encodes an ISBN-13 (978/979...) or converts an ISBN-10 to it.
func ISBN(code string) (*Linear, error) {
	d := strings.ToUpper(strings.NewReplacer("-", "", " ", "", "ISBN", "", ":", "").Replace(code))
	if len(d) == 10 { // ISBN-10: drop its check character, prefix 978
		d = "978" + d[:9]
	}
	return EAN13(d)
}

// ISSN encodes an ISSN as EAN-13: 977 + the first 7 digits + 00.
func ISSN(code string) (*Linear, error) {
	d := strings.ToUpper(strings.NewReplacer("-", "", " ", "", "ISSN", "", ":", "").Replace(code))
	if len(d) == 8 {
		d = "977" + d[:7] + "00"
	}
	return EAN13(d)
}

// ---- Code 39 ----

var code39 = map[rune]string{
	'0': "000110100", '1': "100100001", '2': "001100001", '3': "101100000", '4': "000110001",
	'5': "100110000", '6': "001110000", '7': "000100101", '8': "100100100", '9': "001100100",
	'A': "100001001", 'B': "001001001", 'C': "101001000", 'D': "000011001", 'E': "100011000",
	'F': "001011000", 'G': "000001101", 'H': "100001100", 'I': "001001100", 'J': "000011100",
	'K': "100000011", 'L': "001000011", 'M': "101000010", 'N': "000010011", 'O': "100010010",
	'P': "001010010", 'Q': "000000111", 'R': "100000110", 'S': "001000110", 'T': "000010110",
	'U': "110000001", 'V': "011000001", 'W': "111000000", 'X': "010010001", 'Y': "110010000",
	'Z': "011010000", '-': "010000101", '.': "110000100", ' ': "011000100", '*': "010010100",
	'$': "010101000", '/': "010100010", '+': "010001010", '%': "000101010",
}

const code39Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-. $/+%"

// Code39 encodes digits, capital letters and - . space $ / + %; with
// check, a mod-43 check character is added.
func Code39(text string, check bool) (*Linear, error) {
	text = strings.ToUpper(text)
	sum := 0
	for _, r := range text {
		i := strings.IndexRune(code39Chars, r)
		if i < 0 {
			return nil, fmt.Errorf("Code 39 cannot encode %q", r)
		}
		sum += i
	}
	body := text
	if check {
		body += string(code39Chars[sum%43])
	}
	l := &Linear{Text: text}
	for i, r := range "*" + body + "*" {
		if i > 0 {
			l.Modules = append(l.Modules, false) // inter-character gap
		}
		bar := true
		for _, w := range code39[r] {
			n := 1
			if w == '1' {
				n = 3 // wide elements are three times the narrow ones
			}
			for k := 0; k < n; k++ {
				l.Modules = append(l.Modules, bar)
			}
			bar = !bar
		}
	}
	return l, nil
}

// ---- Interleaved 2 of 5 ----

var i25 = [10]string{"00110", "10001", "01001", "11000", "00101", "10100", "01100", "00011", "10010", "01010"}

// I25 encodes digits as Interleaved 2 of 5 (a leading zero is added to make
// the count even); with check, a mod-10 check digit is appended first.
func I25(code string, check bool) (*Linear, error) {
	d := onlyDigits(code)
	if d == "" || strings.Contains(d, "x") {
		return nil, fmt.Errorf("Interleaved 2 of 5 needs digits, got %q", code)
	}
	if check {
		d += string(rune('0' + checkDigit(d)))
	}
	if len(d)%2 == 1 {
		d = "0" + d
	}
	l := &Linear{Text: d}
	widths("1111", &l.Modules, true) // start: narrow bar, space, bar, space
	for i := 0; i < len(d); i += 2 {
		a, b := i25[d[i]-'0'], i25[d[i+1]-'0']
		for k := 0; k < 5; k++ {
			wa, wb := 1, 1
			if a[k] == '1' {
				wa = 3
			}
			if b[k] == '1' {
				wb = 3
			}
			for n := 0; n < wa; n++ {
				l.Modules = append(l.Modules, true)
			}
			for n := 0; n < wb; n++ {
				l.Modules = append(l.Modules, false)
			}
		}
	}
	widths("311", &l.Modules, true) // stop: wide bar, space, bar
	return l, nil
}
