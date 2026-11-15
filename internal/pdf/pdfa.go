package pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// PDF/A-2b (ISO 19005-2, level B): the document embeds every font, carries
// XMP metadata that matches the document information, and declares its
// colours as sRGB through an output intent with an embedded ICC profile.

// UnembeddedFonts lists the built-in (base-14) fonts the document uses.
// PDF/A forbids them: every font must be embedded.
func (d *Document) UnembeddedFonts() []string {
	var out []string
	for _, f := range d.fonts {
		if f.refd && f.tt == nil {
			out = append(out, stdNames[f.std])
		}
	}
	return out
}

// srgbProfile builds an ICC v2 display profile for sRGB (IEC 61966-2-1):
// D50-adapted primaries, the D65 media white point and the sRGB tone curve.
func srgbProfile() []byte {
	s15 := func(v float64) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, uint32(int32(math.Round(v*65536))))
		return b
	}
	xyz := func(x, y, z float64) []byte {
		b := append([]byte("XYZ "), 0, 0, 0, 0)
		return append(append(append(b, s15(x)...), s15(y)...), s15(z)...)
	}
	// sRGB transfer function sampled at 1024 points
	curve := append([]byte("curv"), 0, 0, 0, 0)
	const n = 1024
	curve = binary.BigEndian.AppendUint32(curve, n)
	for i := 0; i < n; i++ {
		v := float64(i) / (n - 1)
		if v <= 0.04045 {
			v /= 12.92
		} else {
			v = math.Pow((v+0.055)/1.055, 2.4)
		}
		curve = binary.BigEndian.AppendUint16(curve, uint16(math.Round(v*65535)))
	}
	desc := func(s string) []byte {
		b := append([]byte("desc"), 0, 0, 0, 0)
		b = binary.BigEndian.AppendUint32(b, uint32(len(s)+1))
		b = append(append(b, s...), 0)
		b = append(b, make([]byte, 4+4+2+1+67)...) // no Unicode or ScriptCode descriptions
		return b
	}
	text := func(s string) []byte {
		return append(append([]byte("text"), 0, 0, 0, 0), append([]byte(s), 0)...)
	}
	type tag struct {
		sig  string
		data []byte
	}
	tags := []tag{
		{"desc", desc("sRGB IEC61966-2.1")},
		{"cprt", text("No copyright, use freely")},
		{"wtpt", xyz(0.95045, 1.0, 1.08905)},
		{"rXYZ", xyz(0.43607, 0.22249, 0.01392)},
		{"gXYZ", xyz(0.38515, 0.71687, 0.09708)},
		{"bXYZ", xyz(0.14307, 0.06061, 0.71410)},
		{"rTRC", curve}, {"gTRC", curve}, {"bTRC", curve},
	}
	headerLen := 128 + 4 + 12*len(tags)
	var body bytes.Buffer
	table := binary.BigEndian.AppendUint32(nil, uint32(len(tags)))
	offsets := map[string]int{}
	for _, t := range tags {
		key := string(t.data)
		off, shared := offsets[key]
		if !shared {
			for body.Len()%4 != 0 {
				body.WriteByte(0)
			}
			off = headerLen + body.Len()
			offsets[key] = off
			body.Write(t.data)
		}
		table = append(table, t.sig...)
		table = binary.BigEndian.AppendUint32(table, uint32(off))
		table = binary.BigEndian.AppendUint32(table, uint32(len(t.data)))
	}
	for body.Len()%4 != 0 {
		body.WriteByte(0)
	}
	size := headerLen + body.Len()
	h := make([]byte, 128)
	binary.BigEndian.PutUint32(h[0:], uint32(size))
	binary.BigEndian.PutUint32(h[8:], 0x02100000) // version 2.1
	copy(h[12:], "mntr")
	copy(h[16:], "RGB ")
	copy(h[20:], "XYZ ")
	for i, v := range []int{2026, 1, 1, 0, 0, 0} { // creation date
		binary.BigEndian.PutUint16(h[24+2*i:], uint16(v))
	}
	copy(h[36:], "acsp")
	copy(h[68:], s15(0.9642))
	copy(h[72:], s15(1.0))
	copy(h[76:], s15(0.8249))
	out := append(h, table...)
	return append(out, body.Bytes()...)
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// xmpMetadata is the XMP packet of a PDF/A-2b document; it mirrors the
// document information dictionary, as PDF/A requires.
func (d *Document) xmpMetadata(created time.Time) []byte {
	var b strings.Builder
	date := created.UTC().Format("2006-01-02T15:04:05Z")
	b.WriteString("<?xpacket begin=\"\xef\xbb\xbf\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")
	b.WriteString(`<rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/" xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:xmp="http://ns.adobe.com/xap/1.0/" xmlns:pdf="http://ns.adobe.com/pdf/1.3/">` + "\n")
	b.WriteString("<pdfaid:part>2</pdfaid:part><pdfaid:conformance>B</pdfaid:conformance>\n")
	b.WriteString("<dc:format>application/pdf</dc:format>\n")
	if d.Title != "" {
		fmt.Fprintf(&b, "<dc:title><rdf:Alt><rdf:li xml:lang=\"x-default\">%s</rdf:li></rdf:Alt></dc:title>\n", xmlEscape(d.Title))
	}
	if d.Author != "" {
		fmt.Fprintf(&b, "<dc:creator><rdf:Seq><rdf:li>%s</rdf:li></rdf:Seq></dc:creator>\n", xmlEscape(d.Author))
	}
	if d.Subject != "" {
		fmt.Fprintf(&b, "<dc:description><rdf:Alt><rdf:li xml:lang=\"x-default\">%s</rdf:li></rdf:Alt></dc:description>\n", xmlEscape(d.Subject))
	}
	if d.Keywords != "" {
		fmt.Fprintf(&b, "<pdf:Keywords>%s</pdf:Keywords>\n", xmlEscape(d.Keywords))
	}
	fmt.Fprintf(&b, "<pdf:Producer>%s</pdf:Producer>\n", xmlEscape(producer))
	if d.Creator != "" {
		fmt.Fprintf(&b, "<xmp:CreatorTool>%s</xmp:CreatorTool>\n", xmlEscape(d.Creator))
	}
	fmt.Fprintf(&b, "<xmp:CreateDate>%s</xmp:CreateDate><xmp:ModifyDate>%s</xmp:ModifyDate><xmp:MetadataDate>%s</xmp:MetadataDate>\n", date, date, date)
	b.WriteString("</rdf:Description></rdf:RDF></x:xmpmeta>\n")
	// padding lets tools update the packet in place
	b.WriteString(strings.Repeat(strings.Repeat(" ", 99)+"\n", 20))
	b.WriteString(`<?xpacket end="w"?>`)
	return []byte(b.String())
}

const producer = "html2pdf - Go standard library"
