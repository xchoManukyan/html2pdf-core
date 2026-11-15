package resource

import (
	"archive/zip"
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestResolve(t *testing.T) {
	cases := map[[2]string]string{
		{"local:/site/index.html", "css/a.css"}:             "local:/site/css/a.css",
		{"local:/site/css/a.css", "../img/b.png"}:           "local:/site/img/b.png",
		{"local:/index.html", "../../etc/passwd"}:           "local:/etc/passwd", // clamped at the root
		{"local:/index.html", "/abs.png"}:                   "local:/abs.png",
		{"https://example.com/a/b.html", "c.png"}:           "https://example.com/a/c.png",
		{"https://example.com/a/b.html", "//cdn.example/x"}: "https://cdn.example/x",
		{"local:/index.html", "data:image/png;base64,AA"}:   "data:image/png;base64,AA",
	}
	for in, want := range cases {
		if got := Resolve(in[0], in[1]); got != want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestLocalChroot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0o644)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644)
	l := &Loader{BaseDir: root}
	if b, _, err := l.Fetch("local:/ok.txt"); err != nil || string(b) != "ok" {
		t.Fatalf("ok.txt: %q %v", b, err)
	}
	if _, _, err := l.Fetch(Resolve("local:/index.html", "../secret.txt")); err == nil {
		t.Error("files outside the base directory must not be readable")
	}
}

func TestZipBundle(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"site/index.html": "<p>x</p>", "site/css/a.css": "p{}", "other.html": "<p>y</p>"} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	files, main, err := ReadZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if main != "site/index.html" {
		t.Errorf("main document = %q", main)
	}
	l := &Loader{Files: files}
	if b, _, err := l.Fetch(Resolve("local:/"+main, "css/a.css")); err != nil || string(b) != "p{}" {
		t.Errorf("bundled css: %q %v", b, err)
	}
}

func TestRemoteGuards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte("body{}"))
	}))
	defer srv.Close()
	off := &Loader{}
	if _, _, err := off.Fetch(srv.URL + "/a.css"); !errors.Is(err, ErrRemoteDisabled) {
		t.Errorf("remote must be off by default, got %v", err)
	}
	guarded := &Loader{AllowRemote: true}
	if _, _, err := guarded.Fetch(srv.URL + "/a.css"); err == nil {
		t.Error("loopback addresses must be refused (SSRF protection)")
	}
	open := &Loader{AllowRemote: true, AllowPrivate: true}
	b, typ, err := open.Fetch(srv.URL + "/a.css")
	if err != nil || string(b) != "body{}" || typ != "text/css" {
		t.Errorf("remote fetch: %q %q %v", b, typ, err)
	}
}

func TestLocalSymlinkCannotEscape(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o644)
	if err := os.Symlink(filepath.Join(dir, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Skip("symbolic links not available:", err)
	}
	l := &Loader{BaseDir: root}
	if b, _, err := l.Fetch("local:/link.txt"); err == nil {
		t.Errorf("a link out of the base directory was followed: %q", b)
	}
	for _, p := range []string{"local:/../secret.txt", "local:/" + filepath.ToSlash(filepath.Join(dir, "secret.txt"))} {
		if _, _, err := l.Fetch(p); err == nil {
			t.Errorf("%s was readable", p)
		}
	}
}

func TestPrivateAddresses(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "0.1.2.3",
		"100.100.100.200", "198.18.0.1", "255.255.255.255", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1",
		"64:ff9b::a00:1", "2002:7f00:1::", "fec0::1",
	} {
		if !privateIP(net.ParseIP(s)) {
			t.Errorf("%s should be refused", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "140.82.112.3", "2606:4700::1111", "64:ff9b::808:808"} {
		if privateIP(net.ParseIP(s)) {
			t.Errorf("%s is public", s)
		}
	}
}
