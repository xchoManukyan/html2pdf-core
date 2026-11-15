package render

import (
	"bytes"
	"testing"
)

func TestFillableForms(t *testing.T) {
	html := `<html><body><form>
<input type="text" name="name" value="Ada">
<input type="text" name="name" value="Second">
<input type="password" name="pin" value="1234">
<textarea name="notes">Line one</textarea>
<select name="plan"><option>Basic</option><option selected>Standard</option></select>
<input type="radio" name="contact" value="email" checked><input type="radio" name="contact" value="phone">
<input type="checkbox" name="terms" checked>
<input type="submit" value="Send">
</form></body></html>`
	out, err := Convert(html, Options{Forms: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/FT /Tx /T (name)", "/FT /Tx /T (name_2)", "/FT /Tx /T (pin) /Ff 8192", "/T (notes) /Ff 4096",
		"/V (Line one)", "/FT /Ch /T (plan)", "/V (Standard)", "/FT /Btn /T (contact)", "/V /email", "/FT /Btn /T (terms) /Ff 0 /V /Yes"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %q", want)
		}
	}
	if n := bytes.Count(out, []byte("/Subtype /Widget")); n != 8 {
		t.Errorf("%d widgets, want 8 (the submit button stays printed)", n)
	}
	static, err := Convert(html, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(static, []byte("/AcroForm")) {
		t.Error("fields were made without Options.Forms")
	}
}
