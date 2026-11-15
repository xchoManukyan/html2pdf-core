package render

import (
	"strings"

	"github.com/xchoManukyan/html2pdf-core/internal/barcode"
	"github.com/xchoManukyan/html2pdf-core/internal/htmlparse"
)

// expandBarcodes replaces every <barcode> element (mPDF's syntax:
// <barcode code="..." type="QR|C128|EAN13|..." size="1" height="1"
// text="1" error="M" disableborder="1">) with the inline SVG that draws it,
// so it is laid out like an image: CSS width, margins and alignment apply.
// A barcode that cannot be drawn shows its error instead.
func expandBarcodes(root *htmlparse.Node) {
	for _, n := range root.FindAll("barcode") {
		code := n.Attr("code")
		if code == "" {
			code = n.Attr("value")
		}
		svg, err := barcode.SVG(n.Attr("type"), code, barcode.ParseOptions(n.Attr))
		if err != nil {
			svg = barcode.ErrorSVG(err.Error())
		}
		el := htmlparse.Parse(svg).Find("svg")
		if el == nil || n.Parent == nil {
			continue
		}
		for _, name := range []string{"id", "class", "style", "title"} {
			if v := n.Attr(name); v != "" {
				el.Attrs[name] = v
			}
		}
		if !strings.Contains(el.Attrs["style"], "vertical-align") {
			el.Attrs["style"] = strings.TrimSpace("vertical-align: middle; " + el.Attrs["style"])
		}
		el.Parent = n.Parent
		for i, c := range n.Parent.Children {
			if c == n {
				n.Parent.Children[i] = el
				break
			}
		}
	}
}
