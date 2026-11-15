package pdf

import (
	"fmt"
	"strings"
)

// Fillable form fields (AcroForm). Each field is a widget annotation on a
// page with its own appearance, so it shows in every viewer; viewers also
// regenerate the appearance once the user edits the value.

// FieldKind is the type of a form field.
type FieldKind int

const (
	FieldText FieldKind = iota
	FieldMultiline
	FieldPassword
	FieldCheckbox
	FieldRadio
	FieldCombo // drop-down list (<select>)
	FieldList  // list box (<select multiple> or size > 1)
)

// Field describes one form control.
type Field struct {
	Kind     FieldKind
	Name     string   // field name; radio buttons with the same name form one group
	Value    string   // text value, selected option, or a checkbox/radio export value
	Checked  bool     // checkbox and radio state
	Options  []string // choices of a combo or list box
	Selected []string // selected choices (several for a multi-select list)
	Multi    bool     // list box allows several choices
	ReadOnly bool
	Required bool
	MaxLen   int
	Tooltip  string // shown on hover (the HTML title or placeholder)
	Size     float64
	Color    Color
	Align    int // 0 left, 1 centre, 2 right
}

type widget struct {
	Field
	page           int
	x1, y1, x2, y2 float64 // PDF coordinates, unscaled
}

// AddField places a form field over the rectangle (top-left coordinates).
func (p *Page) AddField(f Field, x, y, w, h float64) {
	d := p.doc
	idx := 0
	for i, pg := range d.pages {
		if pg == p {
			idx = i
		}
	}
	if f.Size <= 0 {
		f.Size = 10
	}
	ph := d.Height
	d.widgets = append(d.widgets, &widget{Field: f, page: idx, x1: x, y1: ph - y - h, x2: x + w, y2: ph - y})
}

// HasFields reports whether the document has fillable fields.
func (d *Document) HasFields() bool { return len(d.widgets) > 0 }

const (
	ffReadOnly    = 1 << 0
	ffRequired    = 1 << 1
	ffMultiline   = 1 << 12
	ffPassword    = 1 << 13
	ffNoToggleOff = 1 << 14
	ffRadio       = 1 << 15
	ffCombo       = 1 << 17
	ffMultiSelect = 1 << 21
)

// formLayout assigns object numbers: the two fonts, one parent object per
// radio group, and per widget the field/widget object plus its appearance
// streams.
type formLayout struct {
	helv, zadb int
	groups     map[string]int // radio group name -> parent field object
	groupOrder []string
	ids        []int   // widget object per widget
	aps        [][]int // appearance stream objects per widget
	fields     []int   // top-level fields for /AcroForm /Fields
	pageAnnots map[int][]int
	sigField   int // signature field + widget (0: not signed)
	sigValue   int // signature dictionary
}

func (d *Document) layoutForm(next int) (*formLayout, int) {
	fl := &formLayout{groups: map[string]int{}, pageAnnots: map[int][]int{}}
	if len(d.widgets) > 0 {
		fl.helv, fl.zadb = next, next+1
		next += 2
	}
	if d.Sign != nil {
		fl.sigField, fl.sigValue = next, next+1
		next += 2
		fl.fields = append(fl.fields, fl.sigField)
		fl.pageAnnots[0] = append(fl.pageAnnots[0], fl.sigField)
	}
	for _, wd := range d.widgets {
		if wd.Kind == FieldRadio {
			if _, ok := fl.groups[wd.Name]; !ok {
				fl.groups[wd.Name] = next
				fl.groupOrder = append(fl.groupOrder, wd.Name)
				fl.fields = append(fl.fields, next)
				next++
			}
		}
	}
	for _, wd := range d.widgets {
		fl.ids = append(fl.ids, next)
		if wd.Kind != FieldRadio {
			fl.fields = append(fl.fields, next)
		}
		fl.pageAnnots[wd.page] = append(fl.pageAnnots[wd.page], next)
		next++
		n := 1
		if wd.Kind == FieldCheckbox || wd.Kind == FieldRadio {
			n = 2 // on and off
		}
		var ap []int
		for i := 0; i < n; i++ {
			ap = append(ap, next)
			next++
		}
		fl.aps = append(fl.aps, ap)
	}
	return fl, next
}

// acroForm returns the catalog entry.
func (fl *formLayout) acroForm() string {
	var b strings.Builder
	b.WriteString(" /AcroForm << /Fields [")
	for _, id := range fl.fields {
		fmt.Fprintf(&b, " %d 0 R", id)
	}
	b.WriteString(" ]")
	if fl.sigField != 0 {
		b.WriteString(" /SigFlags 3") // signed, and only incremental updates keep the signature
	} else {
		b.WriteString(" /NeedAppearances true")
	}
	if fl.helv != 0 {
		fmt.Fprintf(&b, " /DR << /Font << /Helv %d 0 R /ZaDb %d 0 R >> >> /DA (/Helv 0 Tf 0 g) >>", fl.helv, fl.zadb)
		return b.String()
	}
	b.WriteString(" >>")
	return b.String()
}

// exportName turns a value into a PDF name for checkbox/radio states.
func exportName(v string) string {
	var sb strings.Builder
	for _, b := range []byte(v) {
		if b > 32 && b < 127 && !strings.ContainsRune("()<>[]{}/%#", rune(b)) {
			sb.WriteByte(b)
		} else {
			fmt.Fprintf(&sb, "#%02X", b)
		}
	}
	if sb.Len() == 0 || v == "Off" {
		return "/Yes" // HTML's default value for a checkbox is "on"
	}
	return "/" + sb.String()
}

func (d *Document) writeForm(w *writer, fl *formLayout, pageIDs []int) {
	s := d.scale()
	if fl.helv != 0 {
		w.object(fl.helv, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
		w.object(fl.zadb, "<< /Type /Font /Subtype /Type1 /BaseFont /ZapfDingbats >>")
	}
	res := fmt.Sprintf("/Resources << /Font << /Helv %d 0 R /ZaDb %d 0 R >> >>", fl.helv, fl.zadb)
	if fl.sigField != 0 {
		// an invisible signature field on the first page
		w.object(fl.sigField, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /T %s /Rect [0 0 0 0] /F 132 /P %d 0 R /V %d 0 R >>",
			w.str(fl.sigField, "Signature1"), pageIDs[0], fl.sigValue))
		w.object(fl.sigValue, d.Sign.sigValue(w, fl.sigValue, d.signTime()))
	}

	for _, name := range fl.groupOrder {
		id := fl.groups[name]
		var kids strings.Builder
		value := "/Off"
		flags := ffRadio | ffNoToggleOff
		for i, wd := range d.widgets {
			if wd.Kind == FieldRadio && wd.Name == name {
				fmt.Fprintf(&kids, " %d 0 R", fl.ids[i])
				if wd.Checked {
					value = exportName(wd.Value)
				}
				if wd.ReadOnly {
					flags |= ffReadOnly
				}
				if wd.Required {
					flags |= ffRequired
				}
			}
		}
		w.object(id, fmt.Sprintf("<< /FT /Btn /T %s /Ff %d /V %s /Kids [%s ] >>", w.str(id, name), flags, value, kids.String()))
	}

	for i, wd := range d.widgets {
		id := fl.ids[i]
		width, height := (wd.x2-wd.x1)*s, (wd.y2-wd.y1)*s
		size := wd.Size * s
		rect := fmt.Sprintf("/Rect [%s %s %s %s]", num(wd.x1*s), num(wd.y1*s), num(wd.x2*s), num(wd.y2*s))
		bbox := fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 %s %s] %s", num(width), num(height), res)
		da := fmt.Sprintf("/Helv %s Tf %s", num(size), colorOp(wd.Color, "rg"))
		flags := 0
		if wd.ReadOnly {
			flags |= ffReadOnly
		}
		if wd.Required {
			flags |= ffRequired
		}
		var dict strings.Builder
		fmt.Fprintf(&dict, "<< /Type /Annot /Subtype /Widget %s /F 4 /P %d 0 R", rect, pageIDs[wd.page])
		if wd.Tooltip != "" && wd.Kind != FieldRadio {
			fmt.Fprintf(&dict, " /TU %s", w.str(id, wd.Tooltip))
		}
		switch wd.Kind {
		case FieldCheckbox, FieldRadio:
			on := exportName(wd.Value)
			state := "/Off"
			if wd.Checked {
				state = on
			}
			if wd.Kind == FieldRadio {
				fmt.Fprintf(&dict, " /Parent %d 0 R", fl.groups[wd.Name])
			} else {
				fmt.Fprintf(&dict, " /FT /Btn /T %s /Ff %d /V %s", w.str(id, wd.Name), flags, state)
			}
			char := "4" // check mark
			if wd.Kind == FieldRadio {
				char = "l" // filled circle
			}
			glyph := min(width, height) * 0.8
			fmt.Fprintf(&dict, " /AS %s /DA (/ZaDb 0 Tf %s) /MK << /CA (%s) >> /AP << /N << %s %d 0 R /Off %d 0 R >> >> >>",
				state, colorOp(wd.Color, "rg"), char, on, fl.aps[i][0], fl.aps[i][1])
			w.object(id, dict.String())
			gw := glyph * 0.846 // ZapfDingbats '4' is 846 units wide, 'l' 791
			if char == "l" {
				gw = glyph * 0.791
			}
			onOps := fmt.Sprintf("q BT /ZaDb %s Tf %s %s %s Td (%s) Tj ET Q", num(glyph), colorOp(wd.Color, "rg"),
				num((width-gw)/2), num((height-glyph*0.7)/2), char)
			w.stream(fl.aps[i][0], bbox, []byte(onOps))
			w.stream(fl.aps[i][1], bbox, []byte(""))
			continue
		case FieldCombo, FieldList:
			flags |= map[FieldKind]int{FieldCombo: ffCombo, FieldList: 0}[wd.Kind]
			if wd.Multi {
				flags |= ffMultiSelect
			}
			var opts strings.Builder
			for _, o := range wd.Options {
				opts.WriteString(" " + w.str(id, o))
			}
			fmt.Fprintf(&dict, " /FT /Ch /T %s /Ff %d /Opt [%s ]", w.str(id, wd.Name), flags, opts.String())
			switch {
			case len(wd.Selected) > 1:
				var v strings.Builder
				for _, sel := range wd.Selected {
					v.WriteString(" " + w.str(id, sel))
				}
				fmt.Fprintf(&dict, " /V [%s ]", v.String())
			case len(wd.Selected) == 1:
				fmt.Fprintf(&dict, " /V %s", w.str(id, wd.Selected[0]))
			}
		default:
			switch wd.Kind {
			case FieldMultiline:
				flags |= ffMultiline
			case FieldPassword:
				flags |= ffPassword
			}
			fmt.Fprintf(&dict, " /FT /Tx /T %s /Ff %d", w.str(id, wd.Name), flags)
			if wd.Value != "" {
				fmt.Fprintf(&dict, " /V %s", w.str(id, wd.Value))
			}
			if wd.MaxLen > 0 {
				fmt.Fprintf(&dict, " /MaxLen %d", wd.MaxLen)
			}
		}
		fmt.Fprintf(&dict, " /DA %s /Q %d /AP << /N %d 0 R >> >>", w.str(id, da), wd.Align, fl.aps[i][0])
		w.object(id, dict.String())
		w.stream(fl.aps[i][0], bbox, []byte(textAppearance(wd, width, height, size)))
	}
}

// textAppearance draws the value of a text or choice field.
func textAppearance(wd *widget, width, height, size float64) string {
	var lines []string
	switch wd.Kind {
	case FieldPassword:
		lines = []string{strings.Repeat("*", len([]rune(wd.Value)))}
	case FieldMultiline:
		lines = strings.Split(strings.ReplaceAll(wd.Value, "\r\n", "\n"), "\n")
	case FieldCombo:
		if len(wd.Selected) > 0 {
			lines = []string{wd.Selected[0]}
		}
	case FieldList:
		lines = wd.Options
	default:
		lines = []string{wd.Value}
	}
	var b strings.Builder
	b.WriteString("/Tx BMC q ")
	fmt.Fprintf(&b, "1 1 %s %s re W n ", num(max(width-2, 0)), num(max(height-2, 0)))
	lead := size * 1.15
	y := (height-size*0.72)/2 + size*0.08 // one line: vertically centred
	if wd.Kind == FieldMultiline || wd.Kind == FieldList {
		y = height - 2 - size*0.9
	}
	selected := map[string]bool{}
	for _, s := range wd.Selected {
		selected[s] = true
	}
	for i, line := range lines {
		ly := y - float64(i)*lead
		if ly < -size {
			break
		}
		if wd.Kind == FieldList && selected[line] {
			fmt.Fprintf(&b, "0.6 0.75 0.95 rg 1 %s %s %s re f ", num(ly-size*0.25), num(max(width-2, 0)), num(lead))
		}
		tw := TextWidth(Helvetica, size, line)
		x := 2.0
		switch wd.Align {
		case 1:
			x = (width - tw) / 2
		case 2:
			x = width - 2 - tw
		}
		fmt.Fprintf(&b, "BT /Helv %s Tf %s %s %s Td %s Tj ET ", num(size), colorOp(wd.Color, "rg"), num(x), num(ly), escape(Encode(line)))
	}
	b.WriteString("Q EMC")
	return b.String()
}
