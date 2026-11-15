package pdf

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
	"unicode/utf16"
)

// Encryption methods of the PDF Standard Security Handler.
const (
	AES256 = "aes-256" // revision 6 (PDF 2.0, readable by Acrobat X+ and all current viewers); the default
	AES128 = "aes-128" // revision 4 (PDF 1.6)
	RC4128 = "rc4-128" // revision 3 (PDF 1.4); weak, only for very old readers
)

// Encryption protects the document with the PDF Standard Security Handler.
type Encryption struct {
	UserPassword  string // needed to open the document ("" = opens without a password)
	OwnerPassword string // full access; defaults to a random value
	AllowPrint    bool
	AllowCopy     bool
	AllowModify   bool
	AllowAnnotate bool
	Method        string // AES256 (default), AES128 or RC4128
}

var padding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

func padPassword(p string) []byte {
	b := []byte(p)
	if len(b) > 32 {
		b = b[:32]
	}
	return append(b, padding[:32-len(b)]...)
}

func rc4Crypt(key, data []byte) []byte {
	c, _ := rc4.NewCipher(key)
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

func random(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

type cryptor struct {
	method string
	key    []byte // file encryption key
	o, u   []byte
	oe, ue []byte // revision 6
	perms  []byte // revision 6
	p      int32
	id     []byte
}

// MethodOf returns the normalised encryption method ("" means AES256).
func MethodOf(m string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "", "aes-256", "aes256", "aesv3":
		return AES256, true
	case "aes-128", "aes128", "aesv2":
		return AES128, true
	case "rc4-128", "rc4", "rc4128":
		return RC4128, true
	}
	return "", false
}

func newCryptor(enc *Encryption) *cryptor {
	method, ok := MethodOf(enc.Method)
	if !ok {
		method = AES256
	}
	owner := enc.OwnerPassword
	if owner == "" {
		owner = hex.EncodeToString(random(16))
	}
	// permissions: bits 7-8 and 13-32 must be 1, bits 1-2 must be 0
	p := uint32(0xFFFFF0C0)
	if enc.AllowPrint {
		p |= 4 | 2048
	}
	if enc.AllowModify {
		p |= 8 | 1024
	}
	if enc.AllowCopy {
		p |= 16 | 512
	}
	if enc.AllowAnnotate {
		p |= 32 | 256
	}
	c := &cryptor{method: method, p: int32(p), id: random(16)}
	if method == AES256 {
		c.initR6(enc.UserPassword, owner, p)
	} else {
		c.initR34(enc.UserPassword, owner, p)
	}
	return c
}

// initR34 computes the keys of revisions 3 (RC4) and 4 (AES-128), which
// share the MD5-based algorithms 2, 3 and 5 of ISO 32000-1.
func (c *cryptor) initR34(user, owner string, p uint32) {
	// Algorithm 3: the O entry
	h := md5.Sum(padPassword(owner))
	for i := 0; i < 50; i++ {
		h = md5.Sum(h[:])
	}
	okey := h[:16]
	o := rc4Crypt(okey, padPassword(user))
	for i := 1; i <= 19; i++ {
		k := make([]byte, len(okey))
		for j := range okey {
			k[j] = okey[j] ^ byte(i)
		}
		o = rc4Crypt(k, o)
	}
	c.o = o

	// Algorithm 2: the file encryption key (metadata is encrypted, so no 0xFFFFFFFF suffix)
	hh := md5.New()
	hh.Write(padPassword(user))
	hh.Write(c.o)
	var pb [4]byte
	binary.LittleEndian.PutUint32(pb[:], p)
	hh.Write(pb[:])
	hh.Write(c.id)
	key := hh.Sum(nil)
	for i := 0; i < 50; i++ {
		s := md5.Sum(key[:16])
		key = s[:]
	}
	c.key = key[:16]

	// Algorithm 5: the U entry
	uh := md5.New()
	uh.Write(padding)
	uh.Write(c.id)
	u := rc4Crypt(c.key, uh.Sum(nil))
	for i := 1; i <= 19; i++ {
		k := make([]byte, len(c.key))
		for j := range c.key {
			k[j] = c.key[j] ^ byte(i)
		}
		u = rc4Crypt(k, u)
	}
	c.u = append(u, random(16)...) // the last 16 bytes are arbitrary
}

// initR6 computes the entries of revision 6 (ISO 32000-2, algorithms 8, 9
// and 10): a random 256-bit file key, wrapped once for each password.
func (c *cryptor) initR6(user, owner string, p uint32) {
	c.key = random(32)
	up, op := saslPassword(user), saslPassword(owner)

	// Algorithm 8: U and UE
	uValSalt, uKeySalt := random(8), random(8)
	c.u = append(append(hash2B(up, uValSalt, nil), uValSalt...), uKeySalt...)
	c.ue = aesNoPad(hash2B(up, uKeySalt, nil), c.key)

	// Algorithm 9: O and OE (they depend on the 48-byte U)
	oValSalt, oKeySalt := random(8), random(8)
	c.o = append(append(hash2B(op, oValSalt, c.u), oValSalt...), oKeySalt...)
	c.oe = aesNoPad(hash2B(op, oKeySalt, c.u), c.key)

	// Algorithm 10: Perms, the permissions encrypted with the file key
	perms := make([]byte, 16)
	binary.LittleEndian.PutUint32(perms[0:4], p)
	copy(perms[4:8], []byte{0xFF, 0xFF, 0xFF, 0xFF})
	perms[8] = 'T' // metadata is encrypted
	copy(perms[9:12], "adb")
	copy(perms[12:16], random(4))
	block, _ := aes.NewCipher(c.key)
	c.perms = make([]byte, 16)
	block.Encrypt(c.perms, perms) // ECB of a single block
}

// saslPassword prepares a revision 6 password: UTF-8, at most 127 bytes.
// (The full SASLprep profile only changes unusual Unicode input.)
func saslPassword(s string) []byte {
	b := []byte(s)
	if len(b) > 127 {
		b = b[:127]
	}
	return b
}

// hash2B is algorithm 2.B of ISO 32000-2: the hardened password hash of
// revision 6. udata is the 48-byte U entry when hashing the owner password.
func hash2B(password, salt, udata []byte) []byte {
	h := sha256.New()
	h.Write(password)
	h.Write(salt)
	h.Write(udata)
	k := h.Sum(nil)
	for round := 0; ; round++ {
		seq := make([]byte, 0, len(password)+len(k)+len(udata))
		seq = append(append(append(seq, password...), k...), udata...)
		k1 := make([]byte, 0, 64*len(seq))
		for i := 0; i < 64; i++ {
			k1 = append(k1, seq...)
		}
		block, _ := aes.NewCipher(k[:16])
		e := make([]byte, len(k1))
		cipher.NewCBCEncrypter(block, k[16:32]).CryptBlocks(e, k1)
		sum := 0
		for _, b := range e[:16] {
			sum += int(b)
		}
		var next hash.Hash
		switch sum % 3 {
		case 0:
			next = sha256.New()
		case 1:
			next = sha512.New384()
		default:
			next = sha512.New()
		}
		next.Write(e)
		k = next.Sum(nil)
		if round >= 63 && int(e[len(e)-1]) <= round+1-32 {
			break
		}
	}
	return k[:32]
}

// aesNoPad encrypts data (a multiple of 16 bytes) with AES-256-CBC and a
// zero IV, as UE and OE require.
func aesNoPad(key, data []byte) []byte {
	block, _ := aes.NewCipher(key)
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, make([]byte, 16)).CryptBlocks(out, data)
	return out
}

// objectKey derives the key of one object (algorithm 1); revision 6 uses
// the file key itself.
func (c *cryptor) objectKey(id int) []byte {
	if c.method == AES256 {
		return c.key
	}
	b := append([]byte(nil), c.key...)
	b = append(b, byte(id), byte(id>>8), byte(id>>16), 0, 0)
	if c.method == AES128 {
		b = append(b, 's', 'A', 'l', 'T')
	}
	s := md5.Sum(b)
	return s[:16]
}

func (c *cryptor) encrypt(id int, data []byte) []byte {
	key := c.objectKey(id)
	if c.method == RC4128 {
		return rc4Crypt(key, data)
	}
	// AES-CBC with a random IV in front and PKCS#7 padding
	pad := 16 - len(data)%16
	plain := make([]byte, len(data)+pad)
	copy(plain, data)
	for i := len(data); i < len(plain); i++ {
		plain[i] = byte(pad)
	}
	iv := random(16)
	block, _ := aes.NewCipher(key)
	out := make([]byte, 16+len(plain))
	copy(out, iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out[16:], plain)
	return out
}

func (c *cryptor) dict() string {
	switch c.method {
	case AES256:
		return fmt.Sprintf("<< /Filter /Standard /V 5 /R 6 /Length 256 /CF << /StdCF << /AuthEvent /DocOpen /CFM /AESV3 /Length 32 >> >>"+
			" /StmF /StdCF /StrF /StdCF /P %d /O <%s> /U <%s> /OE <%s> /UE <%s> /Perms <%s> >>",
			c.p, hex.EncodeToString(c.o), hex.EncodeToString(c.u), hex.EncodeToString(c.oe), hex.EncodeToString(c.ue), hex.EncodeToString(c.perms))
	case AES128:
		return fmt.Sprintf("<< /Filter /Standard /V 4 /R 4 /Length 128 /CF << /StdCF << /AuthEvent /DocOpen /CFM /AESV2 /Length 16 >> >>"+
			" /StmF /StdCF /StrF /StdCF /P %d /O <%s> /U <%s> >>",
			c.p, hex.EncodeToString(c.o), hex.EncodeToString(c.u))
	}
	return fmt.Sprintf("<< /Filter /Standard /V 2 /R 3 /Length 128 /P %d /O <%s> /U <%s> >>",
		c.p, hex.EncodeToString(c.o), hex.EncodeToString(c.u))
}

// version is the PDF version the encryption needs.
func (c *cryptor) version() string {
	switch c.method {
	case AES256:
		return "1.7" // with Adobe extension level 8, see catalogExtensions
	case AES128:
		return "1.6"
	}
	return "1.4"
}

// catalogExtensions declares AES-256 (revision 6) to PDF 1.7 readers.
func (c *cryptor) catalogExtensions() string {
	if c.method == AES256 {
		return " /Extensions << /ADBE << /BaseVersion /1.7 /ExtensionLevel 8 >> >>"
	}
	return ""
}

// textBytes encodes a text string as PDFDocEncoding-compatible ASCII or
// UTF-16BE with a byte-order mark.
func textBytes(s string) []byte {
	ascii := true
	for _, r := range s {
		if r > 126 || r < 32 {
			ascii = false
			break
		}
	}
	if ascii {
		return []byte(s)
	}
	b := []byte{0xFE, 0xFF}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u>>8), byte(u))
	}
	return b
}

// str writes a string object belonging to object id, encrypted when the
// document is protected.
func (w *writer) str(id int, s string) string {
	if w.crypt == nil {
		if strings.IndexFunc(s, func(r rune) bool { return r > 126 || r < 32 }) >= 0 {
			return pdfText(s)
		}
		return escape([]byte(s))
	}
	return "<" + hex.EncodeToString(w.crypt.encrypt(id, textBytes(s))) + ">"
}
