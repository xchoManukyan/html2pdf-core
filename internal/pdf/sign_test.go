package pdf

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func testSigner(t *testing.T, key crypto.Signer) *Signature {
	t.Helper()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "Test Signer"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalPKCS8PrivateKey(key)
	s, err := ParseSigner(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// verifySigned checks a signed file the way a reader does: the byte range
// covers everything except the contents, the message digest matches, and
// the signed attributes verify with the certificate's key.
func verifySigned(t *testing.T, file []byte) {
	t.Helper()
	m := regexp.MustCompile(`/ByteRange \[0 (\d+) (\d+) (\d+)\s*\]`).FindSubmatch(file)
	if m == nil {
		t.Fatal("no byte range")
	}
	a, _ := strconv.Atoi(string(m[1]))
	b, _ := strconv.Atoi(string(m[2]))
	c, _ := strconv.Atoi(string(m[3]))
	if b+c != len(file) || file[a] != '<' || file[b-1] != '>' {
		t.Fatalf("byte range %d %d %d does not frame the contents of a %d-byte file", a, b, c, len(file))
	}
	raw, err := hex.DecodeString(string(file[a+1 : b-1]))
	if err != nil {
		t.Fatal(err)
	}
	var ci struct {
		Type    asn1.ObjectIdentifier
		Content asn1.RawValue
	}
	if _, err := asn1.Unmarshal(raw, &ci); err != nil || !ci.Type.Equal(oidSignedData) {
		t.Fatalf("content info: %v %v", err, ci.Type)
	}
	var sd struct {
		Version          int
		DigestAlgorithms asn1.RawValue
		Content          asn1.RawValue
		Certificates     asn1.RawValue `asn1:"tag:0"`
		SignerInfos      []struct {
			Version            int
			SID                asn1.RawValue
			DigestAlgorithm    pkix.AlgorithmIdentifier
			SignedAttrs        asn1.RawValue `asn1:"tag:0"`
			SignatureAlgorithm pkix.AlgorithmIdentifier
			Signature          []byte
		} `asn1:"set"`
	}
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(sd.Certificates.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	si := sd.SignerInfos[0]
	h := sha256.New()
	h.Write(file[:a])
	h.Write(file[b:])
	if !bytes.Contains(si.SignedAttrs.Bytes, h.Sum(nil)) {
		t.Fatal("message digest does not match the signed byte range")
	}
	set := append([]byte{0x31}, si.SignedAttrs.FullBytes[1:]...)
	if err := cert.CheckSignature(map[bool]x509.SignatureAlgorithm{true: x509.ECDSAWithSHA256, false: x509.SHA256WithRSA}[si.SignatureAlgorithm.Algorithm.Equal(oidECDSAWithSHA256)], set, si.Signature); err != nil {
		t.Fatalf("signature: %v", err)
	}
}

func TestSignRSAAndECDSA(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for name, key := range map[string]crypto.Signer{"rsa": rsaKey, "ecdsa": ecKey} {
		for _, enc := range []bool{false, true} {
			d := New(595, 842)
			d.AddPage().Text(50, 50, TextStyle{Font: d.Std(Helvetica), Size: 12}, "signed")
			d.Sign = testSigner(t, key)
			d.Sign.Reason = "Approved"
			if enc {
				d.Encrypt = &Encryption{OwnerPassword: "owner", Method: AES256}
			}
			var out bytes.Buffer
			if _, err := d.WriteTo(&out); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, want := range []string{"/SigFlags 3", "/FT /Sig", "/SubFilter /ETSI.CAdES.detached"} {
				if !bytes.Contains(out.Bytes(), []byte(want)) {
					t.Errorf("%s: missing %s", name, want)
				}
			}
			verifySigned(t, out.Bytes())
		}
	}
}

func TestParseSignerRejectsWrongKey(t *testing.T) {
	k1, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	k2, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	s := testSigner(t, k1)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certs[0].Raw})
	keyDER, _ := x509.MarshalECPrivateKey(k2)
	if _, err := ParseSigner(certPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})); err == nil {
		t.Fatal("a key that does not match the certificate was accepted")
	}
}

func TestFormFields(t *testing.T) {
	d := New(595, 842)
	p := d.AddPage()
	p.AddField(Field{Kind: FieldText, Name: "name", Value: "Ada"}, 50, 50, 200, 20)
	p.AddField(Field{Kind: FieldRadio, Name: "c", Value: "email", Checked: true}, 50, 80, 12, 12)
	p.AddField(Field{Kind: FieldRadio, Name: "c", Value: "phone"}, 80, 80, 12, 12)
	p.AddField(Field{Kind: FieldCheckbox, Name: "ok"}, 50, 100, 12, 12)
	p.AddField(Field{Kind: FieldCombo, Name: "plan", Options: []string{"A", "B"}, Selected: []string{"B"}}, 50, 120, 100, 20)
	out := d.Bytes()
	for _, want := range []string{"/AcroForm", "/NeedAppearances true", "/FT /Tx /T (name)", "/V (Ada)",
		"/FT /Btn /T (c) /Ff 49152 /V /email", "/AS /email", "/AS /Off", "/FT /Ch /T (plan) /Ff 131072 /Opt [ (A) (B) ] /V (B)",
		"/FT /Btn /T (ok) /Ff 0 /V /Off"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("missing %q", want)
		}
	}
	if n := bytes.Count(out, []byte("/Subtype /Widget")); n != 5 {
		t.Errorf("%d widgets, want 5", n)
	}
}
