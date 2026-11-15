package ttf

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// sfntTables splits a plain sfnt font into its tables.
func sfntTables(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	n := int(binary.BigEndian.Uint16(b[4:]))
	out := map[string][]byte{}
	for i := 0; i < n; i++ {
		e := b[12+16*i:]
		off, l := binary.BigEndian.Uint32(e[8:]), binary.BigEndian.Uint32(e[12:])
		out[string(e[:4])] = b[off : off+l]
	}
	return out
}

type glyph struct {
	contours        int
	box             [4]int16
	ends            []int
	on              []bool
	xs, ys          []int
	instr, compound []byte
}

// glyphs parses every glyph outline (written independently of the decoder).
func glyphs(t *testing.T, tb map[string][]byte) []glyph {
	t.Helper()
	head, loca, glyf := tb["head"], tb["loca"], tb["glyf"]
	long := binary.BigEndian.Uint16(head[50:]) == 1
	numGlyphs := int(binary.BigEndian.Uint16(tb["maxp"][4:]))
	at := func(i int) int {
		if long {
			return int(binary.BigEndian.Uint32(loca[4*i:]))
		}
		return int(binary.BigEndian.Uint16(loca[2*i:])) * 2
	}
	var out []glyph
	for i := 0; i < numGlyphs; i++ {
		s, e := at(i), at(i+1)
		if s == e {
			out = append(out, glyph{})
			continue
		}
		g := glyf[s:e]
		var gl glyph
		gl.contours = int(int16(binary.BigEndian.Uint16(g)))
		for k := 0; k < 4; k++ {
			gl.box[k] = int16(binary.BigEndian.Uint16(g[2+2*k:]))
		}
		if gl.contours < 0 {
			gl.compound = bytes.TrimRight(g[10:], "\x00") // components and instructions; padding differs
			out = append(out, gl)
			continue
		}
		p := 10
		for c := 0; c < gl.contours; c++ {
			gl.ends = append(gl.ends, int(binary.BigEndian.Uint16(g[p:])))
			p += 2
		}
		il := int(binary.BigEndian.Uint16(g[p:]))
		gl.instr = g[p+2 : p+2+il]
		p += 2 + il
		total := gl.ends[len(gl.ends)-1] + 1
		flags := make([]byte, 0, total)
		for len(flags) < total {
			f := g[p]
			p++
			flags = append(flags, f)
			if f&8 != 0 {
				for r := int(g[p]); r > 0; r-- {
					flags = append(flags, f)
				}
				p++
			}
		}
		read := func(short, same byte) []int {
			v, cur := make([]int, total), 0
			for k, f := range flags {
				switch {
				case f&short != 0:
					d := int(g[p])
					p++
					if f&same == 0 {
						d = -d
					}
					cur += d
				case f&same == 0:
					cur += int(int16(binary.BigEndian.Uint16(g[p:])))
					p += 2
				}
				v[k] = cur
			}
			return v
		}
		gl.xs = read(0x02, 0x10)
		gl.ys = read(0x04, 0x20)
		for _, f := range flags {
			gl.on = append(gl.on, f&1 != 0)
		}
		out = append(out, gl)
	}
	return out
}

func TestWOFF2MatchesWOFF1(t *testing.T) {
	for _, name := range []string{"noto-sans-subset", "noto-sans-italic-subset", "roboto-slab-bold-subset"} {
		w2, _ := os.ReadFile("testdata/" + name + ".woff2")
		w1, _ := os.ReadFile("testdata/" + name + ".woff")
		got, err := DecodeWOFF2(w2)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, err := DecodeWOFF(w1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(got, 0); err != nil {
			t.Fatalf("%s: decoded font does not parse: %v", name, err)
		}
		a, b := sfntTables(t, got), sfntTables(t, want)
		for tag, data := range b {
			switch tag {
			case "glyf", "loca", "DSIG":
				continue // compared as outlines below; signatures are dropped by WOFF2
			case "head":
				x, y := append([]byte(nil), a[tag]...), append([]byte(nil), data...)
				copy(x[8:12], "\x00\x00\x00\x00") // checkSumAdjustment
				copy(y[8:12], "\x00\x00\x00\x00")
				x[16] &^= 0x08 // flags bit 11: set by WOFF2 encoders after transforming glyf (spec 5.1)
				y[16] &^= 0x08
				if !bytes.Equal(x, y) {
					t.Errorf("%s: head differs", name)
				}
				continue
			}
			if !bytes.Equal(a[tag], data) {
				t.Errorf("%s: table %q differs (%d vs %d bytes)", name, tag, len(a[tag]), len(data))
			}
		}
		ga, gb := glyphs(t, a), glyphs(t, b)
		if len(ga) != len(gb) {
			t.Fatalf("%s: %d glyphs, want %d", name, len(ga), len(gb))
		}
		for i := range gb {
			x, y := ga[i], gb[i]
			if x.contours != y.contours || x.box != y.box || !equalInts(x.ends, y.ends) || !equalInts(x.xs, y.xs) ||
				!equalInts(x.ys, y.ys) || !bytes.Equal(x.instr, y.instr) || !bytes.Equal(x.compound, y.compound) || !equalBools(x.on, y.on) {
				t.Errorf("%s: glyph %d differs:\n got %+v\nwant %+v", name, i, x, y)
			}
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestWOFF2Rejects(t *testing.T) {
	w2, _ := os.ReadFile("testdata/noto-sans-subset.woff2")
	for i := 60; i < len(w2); i += 97 {
		bad := append([]byte(nil), w2...)
		bad[i] ^= 0x5A
		DecodeWOFF2(bad) // must not panic or hang on damaged input
	}
	if _, err := DecodeWOFF2(w2[:200]); err == nil {
		t.Error("a truncated font must fail")
	}
}
