package barcode

import (
	"bytes"
	"strings"
	"testing"
)

// Reference values from the widely used Thonky QR tutorial: "HELLO WORLD"
// as version 1 at levels M and Q.
func TestQRReferenceCodewords(t *testing.T) {
	ver, data, _ := encodeData("HELLO WORLD", ECCMedium)
	want := []byte{32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17}
	if ver != 1 || !bytes.Equal(data, want) {
		t.Fatalf("1-M data codewords %v, want %v", data, want)
	}
	ecc := addECC(data, 1, ECCMedium)[len(data):]
	wantECC := []byte{196, 35, 39, 119, 235, 215, 231, 226, 93, 23}
	if !bytes.Equal(ecc, wantECC) {
		t.Errorf("1-M error correction %v, want %v", ecc, wantECC)
	}
	_, data, _ = encodeData("HELLO WORLD", ECCQuartile)
	if !bytes.Equal(data, want[:13]) {
		t.Errorf("1-Q data codewords %v", data)
	}
}

// readQR decodes a symbol the way a scanner does, written separately from
// the encoder: format bits by BCH lookup, codewords by the zigzag walk,
// error correction checked by syndromes (not by the encoder's division).
func readQR(t *testing.T, m *Matrix) string {
	t.Helper()
	n := m.Size
	ver := (n - 17) / 4
	dark := func(x, y int) bool { return m.Dark[y][x] }

	// both copies of the format information must be the same valid code word
	var first, second int
	firstPos := [][2]int{{8, 0}, {8, 1}, {8, 2}, {8, 3}, {8, 4}, {8, 5}, {8, 7}, {8, 8}, {7, 8}, {5, 8}, {4, 8}, {3, 8}, {2, 8}, {1, 8}, {0, 8}}
	for i, p := range firstPos {
		if dark(p[0], p[1]) {
			first |= 1 << uint(i)
		}
	}
	for i := 0; i < 8; i++ {
		if dark(n-1-i, 8) {
			second |= 1 << uint(i)
		}
	}
	for i := 8; i < 15; i++ {
		if dark(8, n-15+i) {
			second |= 1 << uint(i)
		}
	}
	if first != second {
		t.Fatalf("format copies differ: %015b %015b", first, second)
	}
	level, mask := -1, -1
	for v := 0; v < 32; v++ {
		// BCH(15,5) by polynomial long division with x^10+x^8+x^5+x^4+x^2+x+1
		r := v << 10
		for bit := 14; bit >= 10; bit-- {
			if r>>uint(bit)&1 == 1 {
				r ^= 0x537 << uint(bit-10)
			}
		}
		if (v<<10|r)^0x5412 == first {
			level, mask = v>>3, v&7
		}
	}
	if level < 0 {
		t.Fatalf("format bits %015b are not a valid code word", first)
	}
	ecl := map[int]ECC{1: ECCLow, 0: ECCMedium, 3: ECCQuartile, 2: ECCHigh}[level]

	// unmask and read the codewords in the zigzag order
	fn := newQR(ver, ecl).function
	var bits []bool
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right--
		}
		upward := ((n-1-right)/2)%2 == 0
		if right < 6 {
			upward = ((n-1-(right+1))/2)%2 == 0
		}
		for k := 0; k < n; k++ {
			y := k
			if upward {
				y = n - 1 - k
			}
			for _, x := range []int{right, right - 1} {
				if fn[y][x] {
					continue
				}
				v := dark(x, y)
				var flip bool
				switch mask {
				case 0:
					flip = (y+x)%2 == 0
				case 1:
					flip = y%2 == 0
				case 2:
					flip = x%3 == 0
				case 3:
					flip = (y+x)%3 == 0
				case 4:
					flip = (y/2+x/3)%2 == 0
				case 5:
					flip = (y*x)%2+(y*x)%3 == 0
				case 6:
					flip = ((y*x)%2+(y*x)%3)%2 == 0
				case 7:
					flip = ((y+x)%2+(y*x)%3)%2 == 0
				}
				bits = append(bits, v != flip)
			}
		}
	}
	raw := make([]byte, rawDataModules(ver)/8)
	for i := range raw {
		for b := 0; b < 8; b++ {
			if bits[i*8+b] {
				raw[i] |= 0x80 >> uint(b)
			}
		}
	}

	// de-interleave into blocks and check each with syndromes
	numBlocks, eccLen := eccBlocks[ecl][ver], eccPerBlock[ecl][ver]
	numShort := numBlocks - len(raw)%numBlocks
	shortData := len(raw)/numBlocks - eccLen
	blocks := make([][]byte, numBlocks)
	k := 0
	for i := 0; i < shortData+1; i++ {
		for j := 0; j < numBlocks; j++ {
			if i == shortData && j < numShort {
				continue
			}
			blocks[j] = append(blocks[j], raw[k])
			k++
		}
	}
	for i := 0; i < eccLen; i++ {
		for j := 0; j < numBlocks; j++ {
			blocks[j] = append(blocks[j], raw[k])
			k++
		}
	}
	var data []byte
	for j, b := range blocks {
		alpha := byte(1)
		for s := 0; s < eccLen; s++ {
			// evaluate the block polynomial at alpha^s (Horner)
			var acc byte
			for _, c := range b {
				acc = gfMul(acc, alpha) ^ c
			}
			if acc != 0 {
				t.Fatalf("block %d: syndrome %d is %d, the error correction is wrong", j, s, acc)
			}
			alpha = gfMul(alpha, 2)
		}
		data = append(data, b[:len(b)-eccLen]...)
	}

	// parse the single segment
	pos := 0
	read := func(n int) int {
		v := 0
		for i := 0; i < n; i++ {
			v = v<<1 | int(data[pos>>3]>>uint(7-pos&7)&1)
			pos++
		}
		return v
	}
	mode := read(4)
	var out strings.Builder
	switch mode {
	case 1:
		count := read(modeNumeric.bits(ver))
		for count >= 3 {
			v := read(10)
			out.WriteString(string([]byte{byte('0' + v/100), byte('0' + v/10%10), byte('0' + v%10)}))
			count -= 3
		}
		if count == 2 {
			v := read(7)
			out.WriteString(string([]byte{byte('0' + v/10), byte('0' + v%10)}))
		} else if count == 1 {
			out.WriteByte(byte('0' + read(4)))
		}
	case 2:
		count := read(modeAlnum.bits(ver))
		for ; count >= 2; count -= 2 {
			v := read(11)
			out.WriteByte(alnumChars[v/45])
			out.WriteByte(alnumChars[v%45])
		}
		if count == 1 {
			out.WriteByte(alnumChars[read(6)])
		}
	case 4:
		count := read(modeByte.bits(ver))
		for i := 0; i < count; i++ {
			out.WriteByte(byte(read(8)))
		}
	default:
		t.Fatalf("unexpected mode %d", mode)
	}
	return out.String()
}

func TestQRRoundTrip(t *testing.T) {
	texts := []string{
		"HELLO WORLD", "0123456789012", "https://example.com/pay?invoice=2024-001&amount=12.90",
		"Grüße — Привет — 你好", strings.Repeat("Version 10+ needs more blocks. ", 12), strings.Repeat("7", 900),
		strings.Repeat("Long text for a large symbol with many interleaved blocks. ", 40),
	}
	for _, text := range texts {
		for _, ecl := range []ECC{ECCLow, ECCMedium, ECCQuartile, ECCHigh} {
			m, err := QR(text, ecl)
			if err == ErrTooLong {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := readQR(t, m); got != text {
				t.Errorf("version %d level %d: read %q, want %q", (m.Size-17)/4, ecl, got[:min(30, len(got))], text[:min(30, len(text))])
			}
		}
	}
}
