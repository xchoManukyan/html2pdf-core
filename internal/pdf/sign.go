package pdf

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Digital signatures: the document is signed with a PAdES baseline (B-B)
// signature, a detached CMS SignedData over the whole file except the
// signature value itself.

// Signature signs the document. Certs[0] is the signing certificate; the
// rest of the chain (intermediates) is embedded so readers can build it.
type Signature struct {
	Certs    []*x509.Certificate
	Key      crypto.Signer // *rsa.PrivateKey or *ecdsa.PrivateKey
	Name     string        // signer name shown by readers (default: the certificate's common name)
	Reason   string
	Location string
	Contact  string
	Time     time.Time // signing time (default: now)
}

// ParseSigner reads a PEM certificate chain and a PEM private key (PKCS#8,
// PKCS#1 or SEC 1, unencrypted).
func ParseSigner(certPEM, keyPEM []byte) (*Signature, error) {
	s := &Signature{}
	for rest := certPEM; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("signing certificate: %w", err)
		}
		s.Certs = append(s.Certs, c)
	}
	if len(s.Certs) == 0 {
		return nil, errors.New("signing certificate: no PEM CERTIFICATE block found")
	}
	var key any
	for rest := keyPEM; key == nil; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			return nil, errors.New("signing key: no PEM private key found")
		}
		if b.Headers["Proc-Type"] != "" || b.Type == "ENCRYPTED PRIVATE KEY" {
			return nil, errors.New("signing key: the key is encrypted; export it without a password (openssl pkey -in key.pem -out plain.pem)")
		}
		var err error
		switch b.Type {
		case "PRIVATE KEY":
			key, err = x509.ParsePKCS8PrivateKey(b.Bytes)
		case "RSA PRIVATE KEY":
			key, err = x509.ParsePKCS1PrivateKey(b.Bytes)
		case "EC PRIVATE KEY":
			key, err = x509.ParseECPrivateKey(b.Bytes)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("signing key: %w", err)
		}
	}
	switch k := key.(type) {
	case *rsa.PrivateKey:
		s.Key = k
	case *ecdsa.PrivateKey:
		s.Key = k
	default:
		return nil, fmt.Errorf("signing key: %T keys are not supported in PDF signatures; use RSA or ECDSA", key)
	}
	if !publicKeyMatches(s.Certs[0].PublicKey, s.Key.Public()) {
		// the first certificate must be the signer's: reorder a chain given leaf-last
		found := false
		for i, c := range s.Certs {
			if publicKeyMatches(c.PublicKey, s.Key.Public()) {
				s.Certs[0], s.Certs[i] = s.Certs[i], s.Certs[0]
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("signing key does not belong to any of the certificates")
		}
	}
	return s, nil
}

func publicKeyMatches(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	e, ok := a.(equaler)
	return ok && e.Equal(b)
}

func (s *Signature) signerName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Certs[0].Subject.CommonName
}

// contentsSize is the space reserved for the CMS signature, in bytes.
func (s *Signature) contentsSize() int {
	n := 2048 // structure, attributes and the signature value
	for _, c := range s.Certs {
		n += len(c.Raw) + 16
	}
	return n
}

const byteRangeHolder = "/ByteRange [0 0000000000 0000000000 0000000000]"

// sigValue is the signature dictionary (the field's /V) with placeholders.
func (s *Signature) sigValue(w *writer, id int, when time.Time) string {
	var b strings.Builder
	b.WriteString("<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /ETSI.CAdES.detached ")
	b.WriteString(byteRangeHolder)
	b.WriteString(" /Contents <")
	b.WriteString(strings.Repeat("0", 2*s.contentsSize()))
	b.WriteString(">")
	fmt.Fprintf(&b, " /M %s", w.str(id, when.Format("D:20060102150405Z")))
	for _, kv := range [][2]string{{"Name", s.signerName()}, {"Reason", s.Reason}, {"Location", s.Location}, {"ContactInfo", s.Contact}} {
		if kv[1] != "" {
			fmt.Fprintf(&b, " /%s %s", kv[0], w.str(id, kv[1]))
		}
	}
	b.WriteString(" >>")
	return b.String()
}

// applySignature fills in the byte range and the signature of a finished
// file that contains exactly one placeholder.
func (s *Signature) applySignature(file []byte) error {
	br := bytes.Index(file, []byte(byteRangeHolder))
	if br < 0 {
		return errors.New("signature placeholder not found")
	}
	open := bytes.Index(file[br:], []byte("/Contents <"))
	if open < 0 {
		return errors.New("signature contents not found")
	}
	start := br + open + len("/Contents ") // at '<'
	end := start + 2 + 2*s.contentsSize()  // after '>'
	ranges := fmt.Sprintf("/ByteRange [0 %d %d %d]", start, end, len(file)-end)
	if len(ranges) > len(byteRangeHolder) {
		return errors.New("file too large to sign")
	}
	copy(file[br:], ranges+strings.Repeat(" ", len(byteRangeHolder)-len(ranges)))

	h := sha256.New()
	h.Write(file[:start])
	h.Write(file[end:])
	cms, err := s.cms(h.Sum(nil))
	if err != nil {
		return err
	}
	if len(cms) > s.contentsSize() {
		return errors.New("signature larger than the space reserved for it")
	}
	hex.Encode(file[start+1:], cms)
	return nil
}

// --- CMS (RFC 5652) SignedData with the PAdES/CAdES attributes ---

var (
	oidData              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningCertV2     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	oidSHA256            = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAEncryption     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidECDSAWithSHA256   = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	sha256AlgID          = pkix.AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.NullRawValue}
	errUnsupportedSigner = errors.New("unsupported signing key")
)

type issuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type attribute struct {
	Type   asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

type signerInfo struct {
	Version            int
	SID                issuerAndSerial
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignedAttrs        asn1.RawValue
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
}

type encapContent struct {
	Type asn1.ObjectIdentifier
}

type signedData struct {
	Version          int
	DigestAlgorithms []pkix.AlgorithmIdentifier `asn1:"set"`
	Content          encapContent
	Certificates     asn1.RawValue
	SignerInfos      []signerInfo `asn1:"set"`
}

type contentInfo struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue // [0] EXPLICIT, built by hand
}

// essCertIDv2 / signingCertificateV2 (RFC 5035); SHA-256 is the default
// hash, so the algorithm is omitted.
type essCertIDv2 struct {
	Hash []byte
}
type signingCertificateV2 struct {
	Certs []essCertIDv2
}

func mustMarshal(v any) []byte {
	b, err := asn1.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func attr(oid asn1.ObjectIdentifier, value []byte) attribute {
	return attribute{Type: oid, Values: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: value}}
}

func (s *Signature) cms(digest []byte) ([]byte, error) {
	leaf := s.Certs[0]
	certHash := sha256.Sum256(leaf.Raw)
	attrs := []attribute{
		attr(oidContentType, mustMarshal(oidData)),
		attr(oidMessageDigest, mustMarshal(digest)),
		attr(oidSigningCertV2, mustMarshal(signingCertificateV2{Certs: []essCertIDv2{{Hash: certHash[:]}}})),
	}
	// The signature covers the attributes encoded as a SET OF (sorted DER);
	// in SignerInfo they are stored with the [0] IMPLICIT tag instead.
	var encoded [][]byte
	for _, a := range attrs {
		encoded = append(encoded, mustMarshal(a))
	}
	sortDER(encoded)
	body := bytes.Join(encoded, nil)
	setOf := mustMarshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: body})
	attrHash := sha256.Sum256(setOf)

	var sigAlg pkix.AlgorithmIdentifier
	var sig []byte
	var err error
	switch k := s.Key.(type) {
	case *rsa.PrivateKey:
		sigAlg = pkix.AlgorithmIdentifier{Algorithm: oidRSAEncryption, Parameters: asn1.NullRawValue}
		sig, err = rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, attrHash[:])
	case *ecdsa.PrivateKey:
		sigAlg = pkix.AlgorithmIdentifier{Algorithm: oidECDSAWithSHA256}
		sig, err = ecdsa.SignASN1(rand.Reader, k, attrHash[:])
	default:
		return nil, errUnsupportedSigner
	}
	if err != nil {
		return nil, fmt.Errorf("signing: %w", err)
	}
	var certs []byte
	for _, c := range s.Certs {
		certs = append(certs, c.Raw...)
	}
	sd := signedData{
		Version:          1,
		DigestAlgorithms: []pkix.AlgorithmIdentifier{sha256AlgID},
		Content:          encapContent{Type: oidData},
		Certificates:     asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: certs},
		SignerInfos: []signerInfo{{
			Version:            1,
			SID:                issuerAndSerial{Issuer: asn1.RawValue{FullBytes: leaf.RawIssuer}, Serial: leaf.SerialNumber},
			DigestAlgorithm:    sha256AlgID,
			SignedAttrs:        asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: body},
			SignatureAlgorithm: sigAlg,
			Signature:          sig,
		}},
	}
	inner := mustMarshal(sd)
	return asn1.Marshal(contentInfo{Type: oidSignedData, Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: inner}})
}

// sortDER orders the encodings of a DER SET OF.
func sortDER(items [][]byte) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && bytes.Compare(items[j-1], items[j]) > 0; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
}

func (d *Document) signTime() time.Time {
	if d.Sign.Time.IsZero() {
		d.Sign.Time = time.Now().UTC()
	}
	return d.Sign.Time.UTC()
}
