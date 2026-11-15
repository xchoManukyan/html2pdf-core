package ttf

import (
	"encoding/binary"
	"errors"

	"github.com/xchoManukyan/html2pdf-core/internal/brotli"
)

// WOFF2 (W3C Recommendation): a font is a Brotli stream holding its tables,
// with glyf/loca (and optionally hmtx) stored in a smaller, transformed form
// that DecodeWOFF2 turns back into plain TrueType tables.

var errWOFF2 = errors.New("ttf: invalid WOFF2 font")

var woff2Tags = [63]string{
	"cmap", "head", "hhea", "hmtx", "maxp", "name", "OS/2", "post", "cvt ", "fpgm", "glyf", "loca", "prep",
	"CFF ", "VORG", "EBDT", "EBLC", "gasp", "hdmx", "kern", "LTSH", "PCLT", "VDMX", "vhea", "vmtx", "BASE",
	"GDEF", "GPOS", "GSUB", "EBSC", "JSTF", "MATH", "CBDT", "CBLC", "COLR", "CPAL", "SVG ", "sbix", "acnt",
	"avar", "bdat", "bloc", "bsln", "cvar", "fdsc", "feat", "fmtx", "fvar", "gvar", "hsty", "just", "lcar",
	"mort", "morx", "opbd", "prop", "trak", "Zapf", "Silf", "Glat", "Gloc", "Feat", "Sill",
}

type woffReader struct {
	b   []byte
	pos int
}

func (r *woffReader) need(n int) {
	if n < 0 || r.pos+n > len(r.b) {
		panic(errWOFF2)
	}
}

func (r *woffReader) u8() int {
	r.need(1)
	r.pos++
	return int(r.b[r.pos-1])
}

func (r *woffReader) u16() int {
	r.need(2)
	r.pos += 2
	return int(binary.BigEndian.Uint16(r.b[r.pos-2:]))
}

func (r *woffReader) s16() int { return int(int16(r.u16())) }

func (r *woffReader) u32() int {
	r.need(4)
	r.pos += 4
	return int(binary.BigEndian.Uint32(r.b[r.pos-4:]))
}

func (r *woffReader) bytes(n int) []byte {
	r.need(n)
	r.pos += n
	return r.b[r.pos-n : r.pos]
}

// base128 reads a UIntBase128 value.
func (r *woffReader) base128() int {
	v := 0
	for i := 0; i < 5; i++ {
		c := r.u8()
		if i == 0 && c == 0x80 {
			panic(errWOFF2) // no leading zeros
		}
		if v&0xFE000000 != 0 {
			panic(errWOFF2)
		}
		v = v<<7 | c&0x7F
		if c&0x80 == 0 {
			return v
		}
	}
	panic(errWOFF2)
}

// u255 reads a 255UInt16 value.
func (r *woffReader) u255() int {
	switch c := r.u8(); c {
	case 253:
		return r.u16()
	case 255:
		return r.u8() + 253
	case 254:
		return r.u8() + 506
	default:
		return c
	}
}

// DecodeWOFF2 converts a WOFF2 font to a plain sfnt font.
func DecodeWOFF2(data []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok && e == errWOFF2 {
				out, err = nil, errWOFF2
				return
			}
			panic(r)
		}
	}()
	r := &woffReader{b: data}
	if len(data) < 48 || string(data[:4]) != "wOF2" {
		return nil, errors.New("ttf: not a WOFF2 file")
	}
	r.pos = 4
	flavor := r.u32()
	if flavor == 0x74746366 { // 'ttcf'
		return nil, errors.New("ttf: WOFF2 font collections are not supported")
	}
	r.u32() // length
	numTables := r.u16()
	r.u16() // reserved
	r.u32() // totalSfntSize
	compressedSize := r.u32()
	r.pos = 48

	type entry struct {
		tag                string
		origLen, storedLen int
		transformed        bool
	}
	entries := make([]entry, numTables)
	for i := range entries {
		flags := r.u8()
		tag := ""
		if idx := flags & 0x3F; idx == 63 {
			tag = string(r.bytes(4))
		} else {
			tag = woff2Tags[idx]
		}
		version := flags >> 6
		e := entry{tag: tag, origLen: r.base128()}
		e.storedLen = e.origLen
		switch tag {
		case "glyf", "loca":
			e.transformed = version == 0
		default:
			e.transformed = version != 0
		}
		if e.transformed {
			e.storedLen = r.base128()
			if tag != "hmtx" && tag != "glyf" && tag != "loca" {
				return nil, errors.New("ttf: unknown WOFF2 table transform for " + tag)
			}
		}
		entries[i] = e
	}
	stream, err := brotli.Decode(r.bytes(compressedSize))
	if err != nil {
		return nil, errWOFF2
	}

	tables := map[string][]byte{}
	transformed := map[string][]byte{}
	pos := 0
	for _, e := range entries {
		if pos+e.storedLen > len(stream) {
			return nil, errWOFF2
		}
		blob := stream[pos : pos+e.storedLen]
		pos += e.storedLen
		if e.transformed {
			transformed[e.tag] = blob
		} else {
			tables[e.tag] = blob
		}
	}

	if g, ok := transformed["glyf"]; ok {
		glyf, loca, xMins, err := reconstructGlyf(g)
		if err != nil {
			return nil, err
		}
		tables["glyf"], tables["loca"] = glyf, loca
		if h, ok := transformed["hmtx"]; ok {
			hmtx, err := reconstructHmtx(h, tables, xMins)
			if err != nil {
				return nil, err
			}
			tables["hmtx"] = hmtx
		}
		if head := tables["head"]; len(head) >= 54 {
			head = append([]byte(nil), head...)
			binary.BigEndian.PutUint16(head[50:], uint16(locaFormat(g))) // indexToLocFormat
			tables["head"] = head
		}
	} else if _, ok := transformed["hmtx"]; ok {
		return nil, errors.New("ttf: WOFF2 hmtx transform without glyf")
	}
	return writeSfnt(tables), nil
}

func locaFormat(transformedGlyf []byte) int {
	if len(transformedGlyf) < 8 {
		return 1
	}
	return int(binary.BigEndian.Uint16(transformedGlyf[6:]))
}

// glyph flags of the glyf table
const (
	onCurve      = 0x01
	xShort       = 0x02
	yShort       = 0x04
	xSameOrPos   = 0x10
	ySameOrPos   = 0x20
	overlapSimpl = 0x40
)

func withSign(flag, v int) int {
	if flag&1 != 0 {
		return v
	}
	return -v
}

// reconstructGlyf rebuilds glyf and loca from the transformed glyf table,
// returning each glyph's xMin as well (for the hmtx transform).
func reconstructGlyf(t []byte) (glyf, loca []byte, xMins []int, err error) {
	h := &woffReader{b: t}
	h.u16() // version
	options := h.u16()
	numGlyphs := h.u16()
	indexFormat := h.u16()
	sizes := make([]int, 7)
	for i := range sizes {
		sizes[i] = h.u32()
	}
	streams := make([]*woffReader, 7)
	for i, n := range sizes {
		streams[i] = &woffReader{b: h.bytes(n)}
	}
	nContour, nPoints, flagS, glyphS, compS, bboxS, instrS := streams[0], streams[1], streams[2], streams[3], streams[4], streams[5], streams[6]
	bitmapLen := (numGlyphs + 31) / 32 * 4
	bboxBitmap := bboxS.bytes(bitmapLen)
	hasBBox := func(i int) bool { return bboxBitmap[i>>3]&(0x80>>uint(i&7)) != 0 }
	var overlap []byte
	if options&1 != 0 {
		overlap = h.bytes((numGlyphs + 7) / 8)
	}

	xMins = make([]int, numGlyphs)
	offsets := make([]int, numGlyphs+1)
	var out []byte
	put16 := func(v int) { out = append(out, byte(v>>8), byte(v)) }

	for g := 0; g < numGlyphs; g++ {
		offsets[g] = len(out)
		nc := nContour.s16()
		switch {
		case nc == 0:
			if hasBBox(g) {
				return nil, nil, nil, errWOFF2
			}
		case nc == -1: // composite
			if !hasBBox(g) {
				return nil, nil, nil, errWOFF2
			}
			put16(0xFFFF)
			box := bboxS.bytes(8)
			xMins[g] = int(int16(binary.BigEndian.Uint16(box)))
			out = append(out, box...)
			instructions := false
			for {
				flags := compS.u16()
				put16(flags)
				n := 2 // glyph index
				if flags&0x0001 != 0 {
					n += 4
				} else {
					n += 2
				}
				switch {
				case flags&0x0008 != 0:
					n += 2
				case flags&0x0040 != 0:
					n += 4
				case flags&0x0080 != 0:
					n += 8
				}
				out = append(out, compS.bytes(n)...)
				if flags&0x0100 != 0 {
					instructions = true
				}
				if flags&0x0020 == 0 {
					break
				}
			}
			if instructions {
				n := glyphS.u255()
				put16(n)
				out = append(out, instrS.bytes(n)...)
			}
		case nc > 0: // simple
			ends := make([]int, nc)
			total := 0
			for c := 0; c < nc; c++ {
				total += nPoints.u255()
				ends[c] = total - 1
			}
			xs, ys := make([]int, total), make([]int, total)
			on := make([]bool, total)
			x, y := 0, 0
			for p := 0; p < total; p++ {
				fb := flagS.u8()
				on[p] = fb&0x80 == 0
				f := fb & 0x7F
				var dx, dy int
				switch {
				case f < 10:
					dy = withSign(f, (f&14)<<7+glyphS.u8())
				case f < 20:
					dx = withSign(f, ((f-10)&14)<<7+glyphS.u8())
				case f < 84:
					b0, b1 := f-20, glyphS.u8()
					dx = withSign(f, 1+(b0&0x30)+b1>>4)
					dy = withSign(f>>1, 1+(b0&0x0C)<<2+b1&0x0F)
				case f < 120:
					b0 := f - 84
					b1, b2 := glyphS.u8(), glyphS.u8()
					dx = withSign(f, 1+(b0/12)<<8+b1)
					dy = withSign(f>>1, 1+((b0%12)>>2)<<8+b2)
				case f < 124:
					b1, b2, b3 := glyphS.u8(), glyphS.u8(), glyphS.u8()
					dx = withSign(f, b1<<4+b2>>4)
					dy = withSign(f>>1, (b2&0x0F)<<8+b3)
				default:
					b1, b2, b3, b4 := glyphS.u8(), glyphS.u8(), glyphS.u8(), glyphS.u8()
					dx = withSign(f, b1<<8+b2)
					dy = withSign(f>>1, b3<<8+b4)
				}
				x += dx
				y += dy
				xs[p], ys[p] = x, y
			}
			instrLen := glyphS.u255()
			instr := instrS.bytes(instrLen)

			var box [4]int
			if hasBBox(g) {
				box = [4]int{bboxS.s16(), bboxS.s16(), bboxS.s16(), bboxS.s16()}
			} else if total > 0 {
				box = [4]int{xs[0], ys[0], xs[0], ys[0]}
				for p := 1; p < total; p++ {
					box[0], box[1] = min(box[0], xs[p]), min(box[1], ys[p])
					box[2], box[3] = max(box[2], xs[p]), max(box[3], ys[p])
				}
			}
			xMins[g] = box[0]
			put16(nc)
			for _, v := range box {
				put16(v)
			}
			for _, e := range ends {
				put16(e)
			}
			put16(instrLen)
			out = append(out, instr...)
			// flags and coordinates, using the short forms where they fit
			var xb, yb []byte
			px, py := 0, 0
			for p := 0; p < total; p++ {
				fl := 0
				if on[p] {
					fl |= onCurve
				}
				if p == 0 && overlap != nil && overlap[g>>3]&(0x80>>uint(g&7)) != 0 {
					fl |= overlapSimpl
				}
				dx, dy := xs[p]-px, ys[p]-py
				px, py = xs[p], ys[p]
				switch {
				case dx == 0:
					fl |= xSameOrPos
				case dx > -256 && dx < 256:
					fl |= xShort
					if dx > 0 {
						fl |= xSameOrPos
					} else {
						dx = -dx
					}
					xb = append(xb, byte(dx))
				default:
					xb = append(xb, byte(dx>>8), byte(dx))
				}
				switch {
				case dy == 0:
					fl |= ySameOrPos
				case dy > -256 && dy < 256:
					fl |= yShort
					if dy > 0 {
						fl |= ySameOrPos
					} else {
						dy = -dy
					}
					yb = append(yb, byte(dy))
				default:
					yb = append(yb, byte(dy>>8), byte(dy))
				}
				out = append(out, byte(fl))
			}
			out = append(out, xb...)
			out = append(out, yb...)
		default:
			return nil, nil, nil, errWOFF2
		}
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	offsets[numGlyphs] = len(out)
	for _, o := range offsets {
		if indexFormat == 0 {
			loca = append(loca, byte(o>>9), byte(o>>1))
		} else {
			loca = append(loca, byte(o>>24), byte(o>>16), byte(o>>8), byte(o))
		}
	}
	return out, loca, xMins, nil
}

// reconstructHmtx rebuilds hmtx from its transformed form (left side
// bearings may be left out: they equal the glyphs' xMin).
func reconstructHmtx(t []byte, tables map[string][]byte, xMins []int) ([]byte, error) {
	hhea, maxp := tables["hhea"], tables["maxp"]
	if len(hhea) < 36 || len(maxp) < 6 {
		return nil, errWOFF2
	}
	numH := int(binary.BigEndian.Uint16(hhea[34:]))
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	if numH < 1 || numH > numGlyphs || numGlyphs > len(xMins) {
		return nil, errWOFF2
	}
	r := &woffReader{b: t}
	flags := r.u8()
	adv := make([]int, numH)
	for i := range adv {
		adv[i] = r.u16()
	}
	lsb := make([]int, numGlyphs)
	for i := 0; i < numH; i++ {
		if flags&1 == 0 {
			lsb[i] = r.s16()
		} else {
			lsb[i] = xMins[i]
		}
	}
	for i := numH; i < numGlyphs; i++ {
		if flags&2 == 0 {
			lsb[i] = r.s16()
		} else {
			lsb[i] = xMins[i]
		}
	}
	var out []byte
	for i := 0; i < numGlyphs; i++ {
		if i < numH {
			out = append(out, byte(adv[i]>>8), byte(adv[i]))
		}
		out = append(out, byte(lsb[i]>>8), byte(lsb[i]))
	}
	return out, nil
}
