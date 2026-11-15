package render

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestWOFF2FontFace(t *testing.T) {
	font, err := os.ReadFile("../ttf/testdata/roboto-slab-bold-subset.woff2")
	if err != nil {
		t.Fatal(err)
	}
	css := `@font-face { font-family: Slab; font-weight: 700; src: url(data:font/woff2;base64,` +
		base64.StdEncoding.EncodeToString(font) + `) format("woff2"); }`
	out, c := convertWith(t, `<style>`+css+`</style><h1 style="font: 700 30px Slab">Hello World</h1>`, Options{NoSystemFonts: true})
	if !strings.Contains(out, "/FontFile2") || !strings.Contains(out, "RobotoSlab") {
		t.Error("the WOFF2 font should be embedded")
	}
	if strings.Contains(c, "/F1 22.5 Tf") && !strings.Contains(out, "FontFile2") {
		t.Error("the heading must not fall back to a built-in font")
	}
}
