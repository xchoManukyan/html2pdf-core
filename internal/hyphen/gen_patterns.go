//go:build ignore

// gen_patterns packs hyph-utf8 patterns (CTAN install/language/hyph-utf8.tds.zip,
// tex/generic/hyph-utf8/patterns) into data/hyph-utf8.zip:
//
//	go run gen_patterns.go <patterns dir> data/hyph-utf8.zip
//
// Packs hyph-utf8 patterns into one zip: <tag>.pat.txt, <tag>.hyp.txt,
// <tag>.LICENSE.txt (the original header) and index.txt (tag left right).
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var excluded = map[string]string{"cs": "GPL", "id": "GPL", "mk": "GPL", "sr-cyrl": "GPL", "ro": "no licence"}

func main() {
	src, out := os.Args[1], os.Args[2] // patterns dir (with tex/ and txt/), output zip
	pats, _ := filepath.Glob(filepath.Join(src, "txt", "hyph-*.pat.txt"))
	sort.Strings(pats)
	f, _ := os.Create(out)
	zw := zip.NewWriter(f)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) { return flate.NewWriter(w, flate.BestCompression) })
	var index bytes.Buffer
	add := func(name string, data []byte) {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		w.Write(data)
	}
	for _, p := range pats {
		tag := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "hyph-"), ".pat.txt")
		if why, ok := excluded[tag]; ok {
			fmt.Fprintf(os.Stderr, "skip %s (%s)\n", tag, why)
			continue
		}
		pat, _ := os.ReadFile(p)
		add(tag+".pat.txt", pat)
		if hyp, err := os.ReadFile(filepath.Join(src, "txt", "hyph-"+tag+".hyp.txt")); err == nil {
			add(tag+".hyp.txt", hyp)
		}
		tex, err := os.ReadFile(filepath.Join(src, "tex", "hyph-"+tag+".tex"))
		if err != nil {
			panic(err)
		}
		var header bytes.Buffer
		sc := bufio.NewScanner(bytes.NewReader(tex))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		left, right, section := "", "", ""
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "%") {
				break
			}
			header.WriteString(line + "\n")
			t := strings.TrimSpace(strings.TrimPrefix(line, "%"))
			if i := strings.Index(t, "#"); i >= 0 {
				t = strings.TrimSpace(t[:i])
			}
			switch {
			case t == "generation:" || t == "typesetting:":
				section = t
			case strings.HasPrefix(t, "left:") && (section == "typesetting:" || left == ""):
				left = strings.TrimSpace(strings.TrimPrefix(t, "left:"))
			case strings.HasPrefix(t, "right:") && (section == "typesetting:" || right == ""):
				right = strings.TrimSpace(strings.TrimPrefix(t, "right:"))
			}
		}
		if left == "" {
			left, right = "2", "2"
		}
		add(tag+".LICENSE.txt", header.Bytes())
		fmt.Fprintf(&index, "%s %s %s\n", tag, left, right)
	}
	add("index.txt", index.Bytes())
	zw.Close()
	f.Close()
	fmt.Print(index.String())
}
