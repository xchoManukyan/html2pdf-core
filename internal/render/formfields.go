package render

import (
	"strconv"
	"strings"

	"github.com/xchoManukyan/html2pdf-core/internal/htmlparse"
	"github.com/xchoManukyan/html2pdf-core/internal/pdf"
)

// With Options.Forms, inputs, selects and textareas become fillable PDF
// fields placed over the boxes the page draws for them. The printed value
// is hidden so that it does not show through the field.

// fillable reports whether n (an original element or a form stand-in) is
// turned into a PDF field.
func (e *engine) fillable(n *htmlparse.Node) bool {
	if !e.forms {
		return false
	}
	switch n.Tag {
	case "textarea", "#select":
		return true
	case "#input":
		switch strings.ToLower(strings.TrimSpace(n.Attrs["type"])) {
		case "submit", "reset", "button", "image", "color", "range", "file", "hidden":
			return false
		}
		return true
	}
	return false
}

// hiddenText is st with its text made invisible (the box still paints).
func hiddenText(st *style) *style {
	cp := *st
	cp.visible = false
	return &cp
}

// addFormField registers the field for a control whose border box is
// (x, y, w, h) on the current page.
func (e *engine) addFormField(n *htmlparse.Node, st *style, x, y, w, h float64) {
	if e.dry || e.fieldDone[n] {
		return
	}
	e.fieldDone[n] = true
	orig := n
	if o, ok := e.formOrig[n]; ok {
		orig = o
	}
	f := pdf.Field{
		ReadOnly: orig.HasAttr("readonly") || orig.HasAttr("disabled"),
		Required: orig.HasAttr("required"),
		Tooltip:  strings.TrimSpace(orDefault(orig.Attr("title"), orig.Attr("placeholder"))),
		Size:     st.size,
		Color:    pdf.Color{R: st.color.R, G: st.color.G, B: st.color.B},
	}
	switch st.physAlign() {
	case "center":
		f.Align = 1
	case "right":
		f.Align = 2
	}
	name := strings.TrimSpace(orig.Attr("name"))
	if name == "" {
		name = strings.TrimSpace(orig.Attr("id"))
	}
	switch orig.Tag {
	case "textarea":
		f.Kind = pdf.FieldMultiline
		text := orig.TextContent()
		f.Value = strings.TrimPrefix(strings.TrimPrefix(text, "\r"), "\n")
	case "select":
		f.Kind = pdf.FieldCombo
		size, _ := strconv.Atoi(orig.Attr("size"))
		if orig.HasAttr("multiple") || size > 1 {
			f.Kind = pdf.FieldList
			f.Multi = orig.HasAttr("multiple")
		}
		for _, o := range orig.FindAll("option") {
			label := strings.Join(strings.Fields(orDefault(o.Attr("label"), o.TextContent())), " ")
			f.Options = append(f.Options, label)
			if o.HasAttr("selected") {
				f.Selected = append(f.Selected, label)
			}
		}
		if len(f.Selected) == 0 && len(f.Options) > 0 && f.Kind == pdf.FieldCombo {
			f.Selected = f.Options[:1]
		}
		if !f.Multi && len(f.Selected) > 1 {
			f.Selected = f.Selected[:1]
		}
	default:
		switch typ := strings.ToLower(strings.TrimSpace(orig.Attr("type"))); typ {
		case "checkbox", "radio":
			f.Kind = pdf.FieldCheckbox
			if typ == "radio" {
				f.Kind = pdf.FieldRadio
			}
			f.Checked = orig.HasAttr("checked")
			f.Value = orDefault(orig.Attr("value"), "Yes")
			f.Tooltip = strings.TrimSpace(orig.Attr("title"))
		case "password":
			f.Kind = pdf.FieldPassword
			f.Value = orig.Attr("value")
		default:
			f.Kind = pdf.FieldText
			f.Value = orig.Attr("value")
		}
		if ml, err := strconv.Atoi(orig.Attr("maxlength")); err == nil && ml > 0 {
			f.MaxLen = ml
		}
	}
	f.Name = e.fieldName(name, f.Kind == pdf.FieldRadio)
	// the field covers the padding box, inside the border the page draws
	x, y = x+st.bw(3), y+st.bw(0)
	w, h = w-st.bw(1)-st.bw(3), h-st.bw(0)-st.bw(2)
	if w <= 0 || h <= 0 {
		return
	}
	e.page().AddField(f, x, y, w, h)
}

// fieldName makes field names unique (in a PDF, fields with the same name
// share one value) and free of the "." that separates name parts. Radio
// buttons keep their shared name: it makes them one group.
func (e *engine) fieldName(name string, radio bool) string {
	name = strings.ReplaceAll(name, ".", "_")
	if name == "" {
		name = "field"
		radio = false
	}
	if radio {
		if _, clash := e.fieldNames[name]; clash && !e.radioNames[name] {
			name += "_radio"
		}
		e.radioNames[name] = true
		e.fieldNames[name]++
		return name
	}
	base := name
	for i := 2; ; i++ {
		if _, used := e.fieldNames[name]; !used {
			break
		}
		name = base + "_" + strconv.Itoa(i)
	}
	e.fieldNames[name]++
	return name
}
