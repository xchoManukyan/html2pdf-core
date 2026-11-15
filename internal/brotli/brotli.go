// Package brotli decompresses Brotli data (RFC 7932). It exists for WOFF2
// fonts and favours clarity over speed; the input is held in memory.
package brotli

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"io"
	"sync"
)

//go:embed dictionary.bin.gz
var dictionaryGz []byte

var (
	dictOnce sync.Once
	dict     []byte
	dictOff  [26]int // DOFFSET by word length
)

func dictionary() []byte {
	dictOnce.Do(func() {
		r, err := gzip.NewReader(bytes.NewReader(dictionaryGz))
		if err == nil {
			dict, _ = io.ReadAll(r)
		}
		for l := 0; l < 25; l++ {
			n := 0
			if ndBits[l] > 0 {
				n = 1 << ndBits[l]
			}
			dictOff[l+1] = dictOff[l] + l*n
		}
	})
	return dict
}

type transform struct {
	prefix string
	kind   int
	suffix string
}

// ErrCorrupt is returned for invalid or truncated data.
var ErrCorrupt = errors.New("brotli: corrupt or truncated data")

// MaxOutput limits the decompressed size (a guard against zip bombs).
const MaxOutput = 64 << 20

// Decode decompresses a complete Brotli stream.
func Decode(data []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok && e == ErrCorrupt {
				out, err = nil, ErrCorrupt
				return
			}
			panic(r)
		}
	}()
	d := &decoder{br: bitReader{data: data}}
	return d.run()
}

// ---- bits ----

type bitReader struct {
	data []byte
	pos  int // bit position
}

func (b *bitReader) bit() int {
	if b.pos>>3 >= len(b.data) {
		panic(ErrCorrupt)
	}
	v := int(b.data[b.pos>>3]>>uint(b.pos&7)) & 1
	b.pos++
	return v
}

func (b *bitReader) bits(n uint) int {
	v := 0
	for i := uint(0); i < n; i++ {
		v |= b.bit() << i
	}
	return v
}

// alignZero skips to the next byte boundary; the skipped bits must be 0.
func (b *bitReader) alignZero() {
	for b.pos&7 != 0 {
		if b.bit() != 0 {
			panic(ErrCorrupt)
		}
	}
}

// ---- prefix codes ----

type prefixCode struct {
	counts  [16]int // codes of each length
	symbols []int   // symbols in canonical order
	single  int     // the only symbol of a one-symbol code (no bits), or -1
}

func newCode(lengths []int) *prefixCode {
	c := &prefixCode{single: -1}
	used := 0
	for _, l := range lengths {
		if l > 0 {
			c.counts[l]++
			used++
		}
	}
	for l := 1; l < 16; l++ {
		for s, sl := range lengths {
			if sl == l {
				c.symbols = append(c.symbols, s)
			}
		}
	}
	if used == 1 {
		c.single = c.symbols[0]
	}
	return c
}

func (c *prefixCode) read(b *bitReader) int {
	if c.single >= 0 {
		return c.single
	}
	code, first, index := 0, 0, 0
	for l := 1; l < 16; l++ {
		code |= b.bit()
		count := c.counts[l]
		if code-first < count {
			return c.symbols[index+code-first]
		}
		index += count
		first = (first + count) << 1
		code <<= 1
	}
	panic(ErrCorrupt)
}

func bitsFor(alphabet int) uint {
	n := uint(0)
	for (1 << n) < alphabet {
		n++
	}
	return n
}

var codeLengthOrder = [18]int{1, 2, 3, 4, 0, 5, 17, 6, 16, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// static code of the code length code lengths, indexed by the next 4 bits
var clPrefixLength = [16]int{2, 2, 2, 3, 2, 2, 2, 4, 2, 2, 2, 3, 2, 2, 2, 4}
var clPrefixValue = [16]int{0, 4, 3, 2, 0, 4, 3, 1, 0, 4, 3, 2, 0, 4, 3, 5}

func readCode(b *bitReader, alphabet int) *prefixCode {
	lengths := make([]int, alphabet)
	hskip := b.bits(2)
	if hskip == 1 { // simple prefix code
		nsym := b.bits(2) + 1
		nb := bitsFor(alphabet)
		syms := make([]int, nsym)
		for i := range syms {
			syms[i] = b.bits(nb)
			if syms[i] >= alphabet {
				panic(ErrCorrupt)
			}
			for j := 0; j < i; j++ {
				if syms[j] == syms[i] {
					panic(ErrCorrupt)
				}
			}
		}
		switch nsym {
		case 1:
			return &prefixCode{single: syms[0]}
		case 2:
			lengths[syms[0]], lengths[syms[1]] = 1, 1
		case 3:
			lengths[syms[0]], lengths[syms[1]], lengths[syms[2]] = 1, 2, 2
		case 4:
			if b.bit() == 0 {
				for _, s := range syms {
					lengths[s] = 2
				}
			} else {
				lengths[syms[0]], lengths[syms[1]], lengths[syms[2]], lengths[syms[3]] = 1, 2, 3, 3
			}
		}
		return newCode(lengths)
	}

	// complex prefix code: first the code length code
	var clLengths [18]int
	space, num := 32, 0
	for i := hskip; i < 18; i++ {
		peek := 0
		for k := uint(0); k < 4; k++ { // peek 4 bits without consuming
			p := b.pos + int(k)
			if p>>3 < len(b.data) {
				peek |= int(b.data[p>>3]>>uint(p&7)) & 1 << k
			}
		}
		b.pos += clPrefixLength[peek]
		v := clPrefixValue[peek]
		clLengths[codeLengthOrder[i]] = v
		if v != 0 {
			space -= 32 >> uint(v)
			num++
			if space <= 0 {
				break
			}
		}
	}
	if num != 1 && space != 0 {
		panic(ErrCorrupt)
	}
	clCode := newCode(clLengths[:])

	// then the symbol code lengths
	symbol, prev, repeat, repeatLen := 0, 8, 0, 0
	space = 32768
	for symbol < alphabet && space > 0 {
		p := clCode.read(b)
		if p < 16 {
			repeat = 0
			lengths[symbol] = p
			symbol++
			if p != 0 {
				prev = p
				space -= 32768 >> uint(p)
			}
			continue
		}
		extra, newLen := uint(2), prev
		if p == 17 {
			extra, newLen = 3, 0
		}
		if repeatLen != newLen {
			repeat, repeatLen = 0, newLen
		}
		old := repeat
		if repeat > 0 {
			repeat = (repeat - 2) << extra
		}
		repeat += b.bits(extra) + 3
		delta := repeat - old
		if symbol+delta > alphabet {
			panic(ErrCorrupt)
		}
		for k := 0; k < delta; k++ {
			lengths[symbol] = repeatLen
			symbol++
		}
		if repeatLen != 0 {
			space -= delta * (32768 >> uint(repeatLen))
		}
	}
	if space != 0 {
		panic(ErrCorrupt)
	}
	return newCode(lengths)
}

// ---- tables of section 5 and 6 ----

var insBase = [24]int{0, 1, 2, 3, 4, 5, 6, 8, 10, 14, 18, 26, 34, 50, 66, 98, 130, 194, 322, 578, 1090, 2114, 6210, 22594}
var insExtra = [24]uint{0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 12, 14, 24}
var copyBase = [24]int{2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 14, 18, 22, 30, 38, 54, 70, 102, 134, 198, 326, 582, 1094, 2118}
var copyExtra = [24]uint{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 24}
var blockBase = [26]int{1, 5, 9, 13, 17, 25, 33, 41, 49, 65, 81, 97, 113, 145, 177, 209, 241, 305, 369, 497, 753, 1265, 2289, 4337, 8433, 16625}
var blockExtra = [26]uint{2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 6, 6, 7, 8, 9, 10, 11, 12, 13, 24}

// insert and copy length code ranges of each 64-symbol cell
var cellInsert = [11]int{0, 0, 0, 0, 8, 8, 0, 16, 8, 16, 16}
var cellCopy = [11]int{0, 8, 0, 8, 0, 8, 16, 0, 16, 8, 16}

// ---- decoder ----

type blockState struct {
	n         int // number of block types
	typeCode  *prefixCode
	countCode *prefixCode
	cur, last int // current and previous block type
	remaining int
}

func readVarLen8(b *bitReader) int {
	if b.bit() == 0 {
		return 0
	}
	n := uint(b.bits(3))
	if n == 0 {
		return 1
	}
	return 1<<n + b.bits(n)
}

func readBlockCount(b *bitReader, c *prefixCode) int {
	s := c.read(b)
	return blockBase[s] + b.bits(blockExtra[s])
}

func readBlockState(b *bitReader) *blockState {
	s := &blockState{n: readVarLen8(b) + 1, cur: 0, last: 1, remaining: 1 << 28}
	if s.n >= 2 {
		s.typeCode = readCode(b, s.n+2)
		s.countCode = readCode(b, 26)
		s.remaining = readBlockCount(b, s.countCode)
	}
	return s
}

// next switches to the next block when the current one is used up.
func (s *blockState) next(b *bitReader) {
	if s.remaining > 0 {
		s.remaining--
		return
	}
	t := s.typeCode.read(b)
	switch t {
	case 0:
		t = s.last
	case 1:
		t = s.cur + 1
	default:
		t -= 2
	}
	if t >= s.n {
		t -= s.n
	}
	s.last, s.cur = s.cur, t
	s.remaining = readBlockCount(b, s.countCode) - 1
}

func readContextMap(b *bitReader, size int) (int, []int) {
	trees := readVarLen8(b) + 1
	cmap := make([]int, size)
	if trees < 2 {
		return trees, cmap
	}
	rleMax := 0
	if b.bit() == 1 {
		rleMax = b.bits(4) + 1
	}
	code := readCode(b, trees+rleMax)
	for i := 0; i < size; {
		c := code.read(b)
		switch {
		case c == 0:
			cmap[i] = 0
			i++
		case c <= rleMax:
			reps := 1<<uint(c) + b.bits(uint(c))
			if i+reps > size {
				panic(ErrCorrupt)
			}
			i += reps // zeros
		default:
			cmap[i] = c - rleMax
			i++
		}
	}
	if b.bit() == 1 { // inverse move-to-front
		var mtf [256]int
		for i := range mtf {
			mtf[i] = i
		}
		for i, v := range cmap {
			val := mtf[v]
			cmap[i] = val
			copy(mtf[1:v+1], mtf[:v])
			mtf[0] = val
		}
	}
	for _, v := range cmap {
		if v >= trees {
			panic(ErrCorrupt)
		}
	}
	return trees, cmap
}

type decoder struct {
	br   bitReader
	out  []byte
	dist [4]int // last distances, dist[0] the most recent
}

func (d *decoder) run() ([]byte, error) {
	b := &d.br
	// window size: only bounds the distances
	wbits := 16
	if b.bit() == 1 {
		if n := b.bits(3); n != 0 {
			wbits = 17 + n
		} else if m := b.bits(3); m == 1 {
			return nil, ErrCorrupt // large-window Brotli is not part of RFC 7932
		} else if m != 0 {
			wbits = 8 + m
		} else {
			wbits = 17
		}
	}
	maxBackward := 1<<uint(wbits) - 16
	d.dist = [4]int{4, 11, 15, 16}

	for {
		last := b.bit() == 1
		if last && b.bit() == 1 { // ISLASTEMPTY
			break
		}
		nibbles := b.bits(2)
		if nibbles == 3 { // metadata block
			if b.bit() != 0 {
				return nil, ErrCorrupt
			}
			skipBytes := b.bits(2)
			skip := 0
			if skipBytes > 0 {
				skip = b.bits(uint(8*skipBytes)) + 1
			}
			b.alignZero()
			b.pos += skip * 8
			if last {
				break
			}
			continue
		}
		mlen := b.bits(uint(4*(nibbles+4))) + 1
		if len(d.out)+mlen > MaxOutput {
			return nil, errors.New("brotli: output too large")
		}
		if !last && b.bit() == 1 { // uncompressed
			b.alignZero()
			start := b.pos >> 3
			if start+mlen > len(b.data) {
				return nil, ErrCorrupt
			}
			d.out = append(d.out, b.data[start:start+mlen]...)
			b.pos += mlen * 8
			continue
		}
		d.metaBlock(mlen, maxBackward)
		if last {
			break
		}
	}
	return d.out, nil
}

func (d *decoder) metaBlock(mlen, maxBackward int) {
	b := &d.br
	lit := readBlockState(b)
	cmd := readBlockState(b)
	dst := readBlockState(b)
	npostfix := uint(b.bits(2))
	ndirect := b.bits(4) << npostfix
	modes := make([]int, lit.n)
	for i := range modes {
		modes[i] = b.bits(2)
	}
	litTrees, litMap := readContextMap(b, 64*lit.n)
	distTrees, distMap := readContextMap(b, 4*dst.n)
	litCodes := make([]*prefixCode, litTrees)
	for i := range litCodes {
		litCodes[i] = readCode(b, 256)
	}
	cmdCodes := make([]*prefixCode, cmd.n)
	for i := range cmdCodes {
		cmdCodes[i] = readCode(b, 704)
	}
	distAlphabet := 16 + ndirect + 48<<npostfix
	distCodes := make([]*prefixCode, distTrees)
	for i := range distCodes {
		distCodes[i] = readCode(b, distAlphabet)
	}
	postfixMask := 1<<npostfix - 1

	end := len(d.out) + mlen
	for len(d.out) < end {
		cmd.next(b)
		sym := cmdCodes[cmd.cur].read(b)
		cell := sym >> 6
		insCode := cellInsert[cell] + sym>>3&7
		copyCode := cellCopy[cell] + sym&7
		insLen := insBase[insCode] + b.bits(insExtra[insCode])
		copyLen := copyBase[copyCode] + b.bits(copyExtra[copyCode])

		for k := 0; k < insLen; k++ {
			lit.next(b)
			var p1, p2 byte
			if n := len(d.out); n > 0 {
				p1 = d.out[n-1]
				if n > 1 {
					p2 = d.out[n-2]
				}
			}
			var ctx int
			switch modes[lit.cur] {
			case 0:
				ctx = int(p1 & 0x3f)
			case 1:
				ctx = int(p1 >> 2)
			case 2:
				ctx = int(lut0[p1] | lut1[p2])
			default:
				ctx = int(lut2[p1])<<3 | int(lut2[p2])
			}
			d.out = append(d.out, byte(litCodes[litMap[lit.cur*64+ctx]].read(b)))
		}
		if len(d.out) >= end {
			if len(d.out) > end {
				panic(ErrCorrupt)
			}
			break
		}

		dcode := 0
		if cell >= 2 {
			dst.next(b)
			cdist := copyLen - 2
			if copyLen > 4 {
				cdist = 3
			}
			dcode = distCodes[distMap[dst.cur*4+cdist]].read(b)
		}
		var distance int
		switch {
		case dcode < 16:
			base := [16]int{0, 1, 2, 3, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1}[dcode]
			delta := [16]int{0, 0, 0, 0, -1, 1, -2, 2, -3, 3, -1, 1, -2, 2, -3, 3}[dcode]
			distance = d.dist[base] + delta
			if distance <= 0 {
				panic(ErrCorrupt)
			}
		case dcode < 16+ndirect:
			distance = dcode - 15
		default:
			x := dcode - ndirect - 16
			ndistbits := uint(1 + x>>(npostfix+1))
			hcode := x >> npostfix
			lcode := x & postfixMask
			offset := (2+hcode&1)<<ndistbits - 4
			distance = (offset+b.bits(ndistbits))<<npostfix + lcode + ndirect + 1
		}

		maxDist := min(maxBackward, len(d.out))
		if distance > maxDist { // a static dictionary word
			if copyLen < 4 || copyLen > 24 || ndBits[copyLen] == 0 {
				panic(ErrCorrupt)
			}
			words := dictionary()
			id := distance - maxDist - 1
			nb := ndBits[copyLen]
			t := id >> nb
			if t >= len(transforms) {
				panic(ErrCorrupt)
			}
			off := dictOff[copyLen] + (id&(1<<nb-1))*copyLen
			if off+copyLen > len(words) {
				panic(ErrCorrupt)
			}
			d.out = append(d.out, applyTransform(words[off:off+copyLen], transforms[t])...)
			if len(d.out) > end {
				panic(ErrCorrupt)
			}
			continue
		}
		if dcode != 0 { // code 0 reuses the last distance without pushing it again
			d.dist = [4]int{distance, d.dist[0], d.dist[1], d.dist[2]}
		}
		if len(d.out)+copyLen > end {
			panic(ErrCorrupt)
		}
		from := len(d.out) - distance
		for k := 0; k < copyLen; k++ { // may overlap
			d.out = append(d.out, d.out[from+k])
		}
	}
}

func applyTransform(word []byte, t transform) []byte {
	w := append([]byte(nil), word...)
	switch {
	case t.kind >= 3 && t.kind <= 11:
		n := t.kind - 2
		if n > len(w) {
			n = len(w)
		}
		w = w[n:]
	case t.kind >= 12:
		n := t.kind - 11
		if n > len(w) {
			n = len(w)
		}
		w = w[:len(w)-n]
	case t.kind == 1:
		ferment(w, 0)
	case t.kind == 2:
		for i := 0; i < len(w); {
			i += ferment(w, i)
		}
	}
	out := append([]byte(t.prefix), w...)
	return append(out, t.suffix...)
}

// ferment upper-cases one UTF-8 character the way the RFC defines it.
func ferment(w []byte, i int) int {
	if i >= len(w) {
		return 1
	}
	switch c := w[i]; {
	case c < 192:
		if c >= 'a' && c <= 'z' {
			w[i] ^= 32
		}
		return 1
	case c < 224:
		if i+1 < len(w) {
			w[i+1] ^= 32
		}
		return 2
	default:
		if i+2 < len(w) {
			w[i+2] ^= 5
		}
		return 3
	}
}
