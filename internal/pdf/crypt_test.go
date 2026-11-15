package pdf

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"io"
	"regexp"
	"strconv"
	"testing"
)

// These tests run the reader's side of the Standard Security Handler
// (password check, key recovery, decryption) on documents we wrote.

func encryptedDoc(t *testing.T, enc *Encryption) (pdf []byte, entries map[string][]byte, id []byte) {
	t.Helper()
	d := New(595, 842)
	d.Encrypt = enc
	p := d.AddPage()
	p.Text(50, 100, TextStyle{Font: d.Std(Helvetica), Size: 12}, "Secret text 42")
	pdf = d.Bytes()
	dict := regexp.MustCompile(`/Filter /Standard[^\n]*`).Find(pdf)
	if dict == nil {
		t.Fatal("no encryption dictionary")
	}
	entries = map[string][]byte{}
	for _, m := range regexp.MustCompile(`/(O|U|OE|UE|Perms) <([0-9a-f]+)>`).FindAllSubmatch(dict, -1) {
		entries[string(m[1])], _ = hex.DecodeString(string(m[2]))
	}
	m := regexp.MustCompile(`/P (-?\d+)`).FindSubmatch(dict)
	entries["P"] = m[1]
	idm := regexp.MustCompile(`/ID \[<([0-9a-f]+)>`).FindSubmatch(pdf)
	id, _ = hex.DecodeString(string(idm[1]))
	return pdf, entries, id
}

// lastStream returns the raw bytes of the page content stream (the last stream).
func lastStream(t *testing.T, pdf []byte) (objID int, data []byte) {
	t.Helper()
	all := regexp.MustCompile(`(?s)(\d+) 0 obj\n<<[^\n]*/Length (\d+) >>\nstream\n`).FindAllSubmatchIndex(pdf, -1)
	m := all[len(all)-1]
	n := 0
	for _, c := range pdf[m[4]:m[5]] {
		n = n*10 + int(c-'0')
	}
	id := 0
	for _, c := range pdf[m[2]:m[3]] {
		id = id*10 + int(c-'0')
	}
	return id, pdf[m[1] : m[1]+n]
}

func aesCBCDecrypt(t *testing.T, key, data []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil || len(data) < 32 || len(data)%16 != 0 {
		t.Fatalf("bad AES data (%d bytes): %v", len(data), err)
	}
	out := make([]byte, len(data)-16)
	cipher.NewCBCDecrypter(block, data[:16]).CryptBlocks(out, data[16:])
	pad := int(out[len(out)-1])
	if pad < 1 || pad > 16 {
		t.Fatalf("bad padding %d", pad)
	}
	return out[:len(out)-pad]
}

func contentHasText(t *testing.T, deflated []byte) {
	t.Helper()
	plain := inflate(t, deflated)
	if !bytes.Contains(plain, []byte("Secret text 42")) && !bytes.Contains(plain, []byte("Secret")) {
		t.Fatalf("decrypted content does not contain the text: %q", plain)
	}
}

func TestAES256(t *testing.T) {
	pdf, e, _ := encryptedDoc(t, &Encryption{UserPassword: "open sesame", OwnerPassword: "boss", AllowPrint: true})
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.7")) || !bytes.Contains(pdf, []byte("/ExtensionLevel 8")) {
		t.Error("AES-256 needs PDF 1.7 with Adobe extension level 8")
	}
	if bytes.Contains(pdf, []byte("Secret")) {
		t.Fatal("text readable without decryption")
	}
	if len(e["U"]) != 48 || len(e["O"]) != 48 || len(e["UE"]) != 32 || len(e["OE"]) != 32 || len(e["Perms"]) != 16 {
		t.Fatalf("entry sizes: U %d O %d UE %d OE %d Perms %d", len(e["U"]), len(e["O"]), len(e["UE"]), len(e["OE"]), len(e["Perms"]))
	}
	// algorithm 11: the user password matches U
	pw := []byte("open sesame")
	if !bytes.Equal(hash2B(pw, e["U"][32:40], nil), e["U"][:32]) {
		t.Fatal("user password does not validate")
	}
	if bytes.Equal(hash2B([]byte("wrong"), e["U"][32:40], nil), e["U"][:32]) {
		t.Fatal("a wrong password validates")
	}
	// algorithm 12: the owner password matches O
	if !bytes.Equal(hash2B([]byte("boss"), e["O"][32:40], e["U"]), e["O"][:32]) {
		t.Fatal("owner password does not validate")
	}
	// the file key, unwrapped through the user and through the owner password
	unwrap := func(k, wrapped []byte) []byte {
		block, _ := aes.NewCipher(k)
		out := make([]byte, 32)
		cipher.NewCBCDecrypter(block, make([]byte, 16)).CryptBlocks(out, wrapped)
		return out
	}
	key := unwrap(hash2B(pw, e["U"][40:48], nil), e["UE"])
	if !bytes.Equal(key, unwrap(hash2B([]byte("boss"), e["O"][40:48], e["U"]), e["OE"])) {
		t.Fatal("user and owner passwords give different keys")
	}
	// algorithm 13: Perms decrypts to the permissions
	block, _ := aes.NewCipher(key)
	perms := make([]byte, 16)
	block.Decrypt(perms, e["Perms"])
	if string(perms[9:12]) != "adb" || perms[8] != 'T' {
		t.Fatalf("Perms does not decrypt correctly: %x", perms)
	}
	if p := int32(binary.LittleEndian.Uint32(perms[:4])); string(e["P"]) != itoa(int(p)) {
		t.Errorf("Perms holds P=%d, the dictionary says %s", p, e["P"])
	}
	_, data := lastStream(t, pdf)
	contentHasText(t, aesCBCDecrypt(t, key, data))
}

func TestAES128(t *testing.T) {
	pdf, e, id := encryptedDoc(t, &Encryption{OwnerPassword: "boss", AllowPrint: true, Method: AES128})
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.6")) || !bytes.Contains(pdf, []byte("/V 4 /R 4")) || !bytes.Contains(pdf, []byte("/CFM /AESV2")) {
		t.Fatal("AES-128 dictionary or version wrong")
	}
	// algorithm 2 with the empty user password, then the per-object key with "sAlT"
	h := md5.New()
	h.Write(padPassword(""))
	h.Write(e["O"])
	var pb [4]byte
	p := 0
	for i, c := range e["P"] {
		if i == 0 && c == '-' {
			continue
		}
		p = p*10 + int(c-'0')
	}
	if e["P"][0] == '-' {
		p = -p
	}
	binary.LittleEndian.PutUint32(pb[:], uint32(int32(p)))
	h.Write(pb[:])
	h.Write(id)
	key := h.Sum(nil)
	for i := 0; i < 50; i++ {
		s := md5.Sum(key[:16])
		key = s[:]
	}
	objID, data := lastStream(t, pdf)
	ok := md5.Sum(append(append(append([]byte(nil), key[:16]...), byte(objID), byte(objID>>8), byte(objID>>16), 0, 0), "sAlT"...))
	contentHasText(t, aesCBCDecrypt(t, ok[:], data))
}

func TestRC4StillAvailable(t *testing.T) {
	pdf, _, _ := encryptedDoc(t, &Encryption{UserPassword: "x", Method: RC4128})
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.Contains(pdf, []byte("/V 2 /R 3 /Length 128")) {
		t.Error("RC4 dictionary or version wrong")
	}
}

func TestMethodNames(t *testing.T) {
	for in, want := range map[string]string{"": AES256, "AES-256": AES256, "aes128": AES128, "rc4": RC4128} {
		if got, ok := MethodOf(in); !ok || got != want {
			t.Errorf("MethodOf(%q) = %q %v", in, got, ok)
		}
	}
	if _, ok := MethodOf("des"); ok {
		t.Error("unknown methods must be rejected")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func inflate(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not deflated data: %v", err)
	}
	out, _ := io.ReadAll(r)
	return out
}

// TestRC4RoundTrip re-derives the key from the written /O /P /ID
// entries (spec algorithms 1 and 2) and decrypts a content stream.
func TestRC4RoundTrip(t *testing.T) {
	d := New(595, 842)
	d.Encrypt = &Encryption{UserPassword: "pw", AllowPrint: true, Method: RC4128}
	p := d.AddPage()
	p.Text(50, 100, TextStyle{Font: d.Std(Helvetica), Size: 12}, "Hello secret")
	out := d.Bytes()

	o := regexp.MustCompile(`/O <([0-9a-f]+)>`).FindSubmatch(out)
	pm := regexp.MustCompile(`/P (-?\d+)`).FindSubmatch(out)
	id := regexp.MustCompile(`/ID \[<([0-9a-f]+)>`).FindSubmatch(out)
	if o == nil || pm == nil || id == nil {
		t.Fatal("missing /O, /P or /ID")
	}
	ob, _ := hex.DecodeString(string(o[1]))
	idb, _ := hex.DecodeString(string(id[1]))
	pv, _ := strconv.ParseInt(string(pm[1]), 10, 64)

	h := md5.New()
	h.Write(padPassword("pw"))
	h.Write(ob)
	var pb [4]byte
	binary.LittleEndian.PutUint32(pb[:], uint32(int32(pv)))
	h.Write(pb[:])
	h.Write(idb)
	key := h.Sum(nil)
	for i := 0; i < 50; i++ {
		s := md5.Sum(key[:16])
		key = s[:]
	}
	key = key[:16]

	// find "<n> 0 obj\n<< /Filter /FlateDecode /Length L >>\nstream\n"
	re := regexp.MustCompile(`(?s)(\d+) 0 obj\n<< /Filter /FlateDecode /Length (\d+) >>\nstream\n`)
	found := false
	for _, m := range re.FindAllSubmatchIndex(out, -1) {
		num, _ := strconv.Atoi(string(out[m[2]:m[3]]))
		n, _ := strconv.Atoi(string(out[m[4]:m[5]]))
		k := append(append([]byte(nil), key...), byte(num), byte(num>>8), byte(num>>16), 0, 0)
		ok := md5.Sum(k)
		plain := rc4Crypt(ok[:16], out[m[1]:m[1]+n])
		zr, err := zlib.NewReader(bytes.NewReader(plain))
		if err != nil {
			t.Fatalf("object %d does not decrypt to a zlib stream: %v", num, err)
		}
		data, _ := io.ReadAll(zr)
		if bytes.Contains(data, []byte("Hello secret")) {
			found = true
		}
	}
	if !found {
		t.Error("decrypted content stream with the text not found")
	}
}
