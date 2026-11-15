package render

import (
	"strings"
	"testing"

	"github.com/xchoManukyan/html2pdf-core/internal/css"
	"github.com/xchoManukyan/html2pdf-core/internal/fonts"
	"github.com/xchoManukyan/html2pdf-core/internal/htmlparse"
	"github.com/xchoManukyan/html2pdf-core/internal/pdf"
)

func lineText(frags []frag) string {
	var b strings.Builder
	for i, f := range frags {
		if i > 0 && f.space > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(f.text)
	}
	return b.String()
}

func words(st *style, ws ...string) []frag {
	var out []frag
	for i, w := range ws {
		f := frag{text: w, st: st, w: 10}
		if i > 0 {
			f.space = 3
		}
		out = append(out, f)
	}
	return out
}

func TestVisualLine(t *testing.T) {
	e := newTestEngine(t)
	ltr := &style{size: 12, family: "serif", weight: 400}
	rtl := &style{size: 12, family: "serif", weight: 400, direction: "rtl"}
	for _, c := range []struct {
		st   *style
		in   []string
		want string
	}{
		{ltr, []string{"Hello", "שלום", "עולם", "world"}, "Hello םלוע םולש world"},
		{rtl, []string{"שלום", "עולם"}, "םלוע םולש"},
		{rtl, []string{"מחיר:", "120", "ש\"ח"}, "ח\"ש 120 :ריחמ"},
		{rtl, []string{"(הערה)"}, "(הרעה)"},
		{rtl, []string{"שלום", "world", "123"}, "world 123 םולש"},
		{ltr, []string{"only", "latin"}, "only latin"},
	} {
		if got := lineText(e.visualLine(words(c.st, c.in...), c.st)); got != c.want {
			t.Errorf("%v: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDirectionAndAlignment(t *testing.T) {
	e := newTestEngine(t)
	root := htmlparse.Parse(`<html dir="rtl"><body><p id="a">x</p><p id="b" dir="ltr">y</p><p id="c" dir="auto">שלום</p><p id="d" style="text-align:end">z</p></body></html>`)
	get := func(id string) *style {
		var find func(n *htmlparse.Node, parent *style) *style
		find = func(n *htmlparse.Node, parent *style) *style {
			st := parent
			if n.Type == htmlparse.ElementNode {
				st = e.computeStyle(n, parent)
				if n.Attr("id") == id {
					return st
				}
			}
			for _, c := range n.Children {
				if s := find(c, st); s != nil {
					return s
				}
			}
			return nil
		}
		return find(root, rootStyle())
	}
	for id, want := range map[string]string{"a": "right", "b": "left", "c": "right", "d": "left"} {
		if got := get(id).physAlign(); got != want {
			t.Errorf("#%s: aligned %s, want %s", id, got, want)
		}
	}
}

// newTestEngine is an engine with the user-agent styles and only the
// built-in fonts, for unit tests of layout helpers.
func newTestEngine(t *testing.T) *engine {
	t.Helper()
	env := css.Env{Width: 595, Height: 842}
	return &engine{
		doc: pdf.New(595, 842), rootSize: 12, ua: css.ParseStylesheet(userAgentCSS, env), author: css.NewStylesheet(),
		styles: map[*htmlparse.Node]*style{}, pseudoStyles: map[pseudoKey]*style{}, hasPseudo: map[string]bool{},
		counters: map[string]int{}, pictures: map[string]*picture{}, imgPictures: map[*htmlparse.Node]*picture{}, env: env,
		inlineSVG: map[*htmlparse.Node]*picture{}, formNodes: map[*htmlparse.Node]*htmlparse.Node{},
		docFonts: fonts.NewDB(), sysFonts: fonts.NewDB(), chains: map[string]*fontChain{},
	}
}
