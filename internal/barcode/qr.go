package barcode

import (
	"errors"
	"strings"
)

// QR code encoder (ISO/IEC 18004): numeric, alphanumeric or byte mode,
// versions 1–40, error correction levels L, M, Q and H, automatic mask
// choice by the standard's penalty rules.

// ECC is a QR error correction level.
type ECC int

const (
	ECCLow      ECC = iota // L, about 7% of the code can be damaged
	ECCMedium              // M, about 15%
	ECCQuartile            // Q, about 25%
	ECCHigh                // H, about 30%
)

var eccFormatBits = [4]int{1, 0, 3, 2}

// error correction codewords per block, by level and version (index 0 unused)
var eccPerBlock = [4][41]int{
	{-1, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	{-1, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{-1, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

// number of error correction blocks, by level and version
var eccBlocks = [4][41]int{
	{-1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	{-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	{-1, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	{-1, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}

const alnumChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

// ErrTooLong is returned when the text does not fit in a version 40 code.
var ErrTooLong = errors.New("the text is too long for a QR code")

type qrMode struct {
	indicator int
	countBits [3]int // versions 1-9, 10-26, 27-40
}

var (
	modeNumeric = qrMode{1, [3]int{10, 12, 14}}
	modeAlnum   = qrMode{2, [3]int{9, 11, 13}}
	modeByte    = qrMode{4, [3]int{8, 16, 16}}
)

func (m qrMode) bits(ver int) int {
	switch {
	case ver <= 9:
		return m.countBits[0]
	case ver <= 26:
		return m.countBits[1]
	}
	return m.countBits[2]
}

type bitBuffer []bool

func (b *bitBuffer) put(val, n int) {
	for i := n - 1; i >= 0; i-- {
		*b = append(*b, val>>uint(i)&1 == 1)
	}
}

// QR encodes text as a QR code. The result has no quiet zone.
func QR(text string, ecl ECC) (*Matrix, error) {
	ver, codewords, err := encodeData(text, ecl)
	if err != nil {
		return nil, err
	}
	q := newQR(ver, ecl)
	q.drawCodewords(addECC(codewords, ver, ecl))
	best, bestPenalty := 0, -1
	for mask := 0; mask < 8; mask++ {
		q.applyMask(mask)
		q.drawFormat(mask)
		if p := q.penalty(); bestPenalty < 0 || p < bestPenalty {
			best, bestPenalty = mask, p
		}
		q.applyMask(mask) // XOR again: undo
	}
	q.applyMask(best)
	q.drawFormat(best)
	return &Matrix{Size: q.size, Dark: q.modules}, nil
}

// encodeData picks the smallest version and returns the data codewords
// (mode, count, payload, terminator and padding), before error correction.
func encodeData(text string, ecl ECC) (int, []byte, error) {
	mode := modeByte
	switch {
	case text != "" && strings.Trim(text, "0123456789") == "":
		mode = modeNumeric
	case text != "" && strings.IndexFunc(text, func(r rune) bool { return !strings.ContainsRune(alnumChars, r) }) < 0:
		mode = modeAlnum
	}
	data := []byte(text) // UTF-8; readers decode byte mode as UTF-8 in practice

	var payload bitBuffer
	switch mode {
	case modeNumeric:
		for i := 0; i < len(data); i += 3 {
			end := min(i+3, len(data))
			v := 0
			for _, c := range data[i:end] {
				v = v*10 + int(c-'0')
			}
			payload.put(v, (end-i)*3+1)
		}
	case modeAlnum:
		for i := 0; i < len(data); i += 2 {
			a := strings.IndexByte(alnumChars, data[i])
			if i+1 < len(data) {
				payload.put(a*45+strings.IndexByte(alnumChars, data[i+1]), 11)
			} else {
				payload.put(a, 6)
			}
		}
	default:
		for _, c := range data {
			payload.put(int(c), 8)
		}
	}

	ver := 1
	for ; ver <= 40; ver++ {
		need := 4 + mode.bits(ver) + len(payload)
		if len(data) < 1<<uint(mode.bits(ver)) && need <= dataCodewords(ver, ecl)*8 {
			break
		}
	}
	if ver > 40 {
		return 0, nil, ErrTooLong
	}

	var bits bitBuffer
	bits.put(mode.indicator, 4)
	bits.put(len(data), mode.bits(ver))
	bits = append(bits, payload...)
	capacity := dataCodewords(ver, ecl) * 8
	bits.put(0, min(4, capacity-len(bits)))
	bits.put(0, (8-len(bits)%8)%8)
	for pad := 0xEC; len(bits) < capacity; pad ^= 0xEC ^ 0x11 {
		bits.put(pad, 8)
	}
	codewords := make([]byte, len(bits)/8)
	for i, b := range bits {
		if b {
			codewords[i>>3] |= 1 << uint(7-i&7)
		}
	}
	return ver, codewords, nil
}

// Matrix is a square QR symbol; Dark[y][x] is a dark module.
type Matrix struct {
	Size int
	Dark [][]bool
}

func rawDataModules(ver int) int {
	n := (16*ver+128)*ver + 64
	if ver >= 2 {
		align := ver/7 + 2
		n -= (25*align-10)*align - 55
		if ver >= 7 {
			n -= 36
		}
	}
	return n
}

func dataCodewords(ver int, ecl ECC) int {
	return rawDataModules(ver)/8 - eccPerBlock[ecl][ver]*eccBlocks[ecl][ver]
}

// addECC splits the data into blocks, appends Reed-Solomon codewords to each
// and interleaves them.
func addECC(data []byte, ver int, ecl ECC) []byte {
	numBlocks := eccBlocks[ecl][ver]
	eccLen := eccPerBlock[ecl][ver]
	raw := rawDataModules(ver) / 8
	numShort := numBlocks - raw%numBlocks
	shortLen := raw / numBlocks
	divisor := rsDivisor(eccLen)
	var blocks [][]byte
	k := 0
	for i := 0; i < numBlocks; i++ {
		n := shortLen - eccLen
		if i >= numShort {
			n++
		}
		dat := append([]byte(nil), data[k:k+n]...)
		k += n
		ecc := rsRemainder(dat, divisor)
		if i < numShort {
			dat = append(dat, 0) // placeholder, skipped when interleaving
		}
		blocks = append(blocks, append(dat, ecc...))
	}
	out := make([]byte, 0, raw)
	for i := range blocks[0] {
		for j, b := range blocks {
			if i != shortLen-eccLen || j >= numShort {
				out = append(out, b[i])
			}
		}
	}
	return out
}

func gfMul(x, y byte) byte {
	var z byte
	for i := 7; i >= 0; i-- {
		hi := z >> 7
		z = z<<1 ^ hi*0x1D
		z ^= (y >> uint(i) & 1) * x
	}
	return z
}

func rsDivisor(degree int) []byte {
	r := make([]byte, degree)
	r[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range r {
			r[j] = gfMul(r[j], root)
			if j+1 < len(r) {
				r[j] ^= r[j+1]
			}
		}
		root = gfMul(root, 0x02)
	}
	return r
}

func rsRemainder(data, divisor []byte) []byte {
	r := make([]byte, len(divisor))
	for _, b := range data {
		factor := b ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i, c := range divisor {
			r[i] ^= gfMul(c, factor)
		}
	}
	return r
}

type qrBuilder struct {
	ver, size int
	ecl       ECC
	modules   [][]bool
	function  [][]bool
}

func newQR(ver int, ecl ECC) *qrBuilder {
	size := ver*4 + 17
	q := &qrBuilder{ver: ver, size: size, ecl: ecl}
	q.modules = make([][]bool, size)
	q.function = make([][]bool, size)
	for i := range q.modules {
		q.modules[i] = make([]bool, size)
		q.function[i] = make([]bool, size)
	}
	// timing patterns
	for i := 0; i < size; i++ {
		q.set(6, i, i%2 == 0)
		q.set(i, 6, i%2 == 0)
	}
	// finder patterns with their separators
	for _, c := range [][2]int{{3, 3}, {size - 4, 3}, {3, size - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if x < 0 || y < 0 || x >= size || y >= size {
					continue
				}
				d := max(abs(dx), abs(dy))
				q.set(x, y, d != 2 && d != 4)
			}
		}
	}
	// alignment patterns
	pos := alignmentPositions(ver)
	n := len(pos)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == 0 && j == 0 || i == 0 && j == n-1 || i == n-1 && j == 0 {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					q.set(pos[i]+dx, pos[j]+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	q.drawFormat(0) // reserve the format areas
	if ver >= 7 {
		rem := ver
		for i := 0; i < 12; i++ {
			rem = rem<<1 ^ (rem>>11)*0x1F25
		}
		bits := ver<<12 | rem
		for i := 0; i < 18; i++ {
			bit := bits>>uint(i)&1 == 1
			a, b := size-11+i%3, i/3
			q.set(a, b, bit)
			q.set(b, a, bit)
		}
	}
	return q
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func alignmentPositions(ver int) []int {
	if ver == 1 {
		return nil
	}
	n := ver/7 + 2
	step := (ver*8 + n*3 + 5) / (n*4 - 4) * 2
	out := make([]int, n)
	out[0] = 6
	for i, p := n-1, ver*4+17-7; i >= 1; i, p = i-1, p-step {
		out[i] = p
	}
	return out
}

func (q *qrBuilder) set(x, y int, dark bool) {
	q.modules[y][x] = dark
	q.function[y][x] = true
}

func (q *qrBuilder) drawFormat(mask int) {
	data := eccFormatBits[q.ecl]<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = rem<<1 ^ (rem>>9)*0x537
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return bits>>uint(i)&1 == 1 }
	for i := 0; i <= 5; i++ {
		q.set(8, i, bit(i))
	}
	q.set(8, 7, bit(6))
	q.set(8, 8, bit(7))
	q.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		q.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		q.set(q.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		q.set(8, q.size-15+i, bit(i))
	}
	q.set(8, q.size-8, true) // the dark module
}

func (q *qrBuilder) drawCodewords(data []byte) {
	i := 0
	for right := q.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < q.size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = q.size - 1 - vert
				}
				if !q.function[y][x] && i < len(data)*8 {
					q.modules[y][x] = data[i>>3]>>uint(7-i&7)&1 == 1
					i++
				}
			}
		}
	}
}

func (q *qrBuilder) applyMask(mask int) {
	for y := 0; y < q.size; y++ {
		for x := 0; x < q.size; x++ {
			if q.function[y][x] {
				continue
			}
			var flip bool
			switch mask {
			case 0:
				flip = (x+y)%2 == 0
			case 1:
				flip = y%2 == 0
			case 2:
				flip = x%3 == 0
			case 3:
				flip = (x+y)%3 == 0
			case 4:
				flip = (x/3+y/2)%2 == 0
			case 5:
				flip = x*y%2+x*y%3 == 0
			case 6:
				flip = (x*y%2+x*y%3)%2 == 0
			default:
				flip = ((x+y)%2+x*y%3)%2 == 0
			}
			if flip {
				q.modules[y][x] = !q.modules[y][x]
			}
		}
	}
}

// penalty scores a masked symbol by the four rules of the standard.
func (q *qrBuilder) penalty() int {
	n, score, dark := q.size, 0, 0
	at := func(x, y int, vertical bool) bool {
		if vertical {
			return q.modules[x][y]
		}
		return q.modules[y][x]
	}
	finder := []bool{true, false, true, true, true, false, true}
	for _, vertical := range []bool{false, true} {
		for y := 0; y < n; y++ {
			run := 1
			for x := 1; x <= n; x++ {
				if x < n && at(x, y, vertical) == at(x-1, y, vertical) {
					run++
					continue
				}
				if run >= 5 { // rule 1: runs of five or more
					score += 3 + run - 5
				}
				run = 1
			}
			// rule 3: a finder-like 1:1:3:1:1 pattern with four light modules on one side
			for x := 0; x+7 <= n; x++ {
				match := true
				for k, v := range finder {
					if at(x+k, y, vertical) != v {
						match = false
						break
					}
				}
				if !match {
					continue
				}
				lightBefore, lightAfter := x >= 4, x+11 <= n
				for k := 1; k <= 4 && lightBefore; k++ {
					lightBefore = !at(x-k, y, vertical)
				}
				for k := 7; k < 11 && lightAfter; k++ {
					lightAfter = !at(x+k, y, vertical)
				}
				if lightBefore || lightAfter {
					score += 40
				}
			}
		}
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if q.modules[y][x] {
				dark++
			}
			// rule 2: 2x2 blocks of one colour
			if x+1 < n && y+1 < n {
				c := q.modules[y][x]
				if q.modules[y][x+1] == c && q.modules[y+1][x] == c && q.modules[y+1][x+1] == c {
					score += 3
				}
			}
		}
	}
	// rule 4: balance of dark and light
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return score + max(0, k)*10
}
