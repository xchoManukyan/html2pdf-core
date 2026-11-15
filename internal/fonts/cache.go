package fonts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/xchoManukyan/html2pdf-core/internal/ttf"
)

// The font index cache: reading the name tables of every installed font
// takes tens of milliseconds, which every converter process would pay. The
// cache keeps, per font file, its size, modification time and the faces it
// holds. It stores no font data, and an entry is only used for a file
// found in the font folders with the same size and time, so a changed or
// forged cache can never make the converter read another file.
// HTML2PDF_FONT_CACHE=off turns it off; HTML2PDF_FONT_CACHE=DIR moves it.

const cacheVersion = 1

type cachedFile struct {
	Size  int64        `json:"size"`
	MTime int64        `json:"mtime"`
	Faces []cachedFace `json:"faces"` // empty: not a usable font
}

type cachedFace struct {
	Index    int    `json:"i"`
	Family   string `json:"f"`
	Legacy   string `json:"l,omitempty"`
	FullName string `json:"n,omitempty"`
	Weight   int    `json:"w"`
	Stretch  int    `json:"s"`
	Italic   bool   `json:"it,omitempty"`
}

type cacheFile struct {
	Version int                    `json:"version"`
	Files   map[string]*cachedFile `json:"files"`
}

// fontCache is the index loaded from disk and updated while scanning.
type fontCache struct {
	mu      sync.Mutex
	path    string
	files   map[string]*cachedFile
	used    map[string]bool
	changed bool
}

func cachePath() string {
	v := os.Getenv("HTML2PDF_FONT_CACHE")
	switch strings.ToLower(v) {
	case "off", "0", "false", "no":
		return ""
	case "":
		dir, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		v = filepath.Join(dir, "html2pdf")
	}
	return filepath.Join(v, "fonts.json")
}

func loadCache() *fontCache {
	c := &fontCache{path: cachePath(), files: map[string]*cachedFile{}, used: map[string]bool{}}
	if c.path == "" {
		return c
	}
	data, err := os.ReadFile(c.path)
	if err != nil || len(data) > 32<<20 {
		return c
	}
	var f cacheFile
	if json.Unmarshal(data, &f) == nil && f.Version == cacheVersion && f.Files != nil {
		c.files = f.Files
	}
	return c
}

// faces returns the faces of a font file, from the cache when the file is
// unchanged, otherwise by reading it.
func (c *fontCache) faces(path string, info os.FileInfo) []cachedFace {
	c.mu.Lock()
	c.used[path] = true
	if e, ok := c.files[path]; ok && e.Size == info.Size() && e.MTime == info.ModTime().UnixNano() {
		c.mu.Unlock()
		return e.Faces
	}
	c.mu.Unlock()

	faces := readFaces(path)
	c.mu.Lock()
	c.files[path] = &cachedFile{Size: info.Size(), MTime: info.ModTime().UnixNano(), Faces: faces}
	c.changed = true
	c.mu.Unlock()
	return faces
}

func readFaces(path string) []cachedFace {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var out []cachedFace
	n := ttf.NumFaces(file)
	for i := 0; i < n && i < 64; i++ {
		info, err := ttf.ReadInfo(file, i)
		if err != nil || !info.TrueType || !info.Embeddable {
			continue
		}
		out = append(out, cachedFace{Index: i, Family: info.Family, Legacy: info.LegacyFamily, FullName: info.FullName,
			Weight: info.Weight, Stretch: info.Stretch, Italic: info.Italic})
	}
	if out == nil {
		out = []cachedFace{} // remembered as "not usable"
	}
	return out
}

// save writes the cache when something changed, dropping files that were
// not found any more. Written to a temporary file and renamed, so a
// parallel process never reads half a file.
func (c *fontCache) save() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for p := range c.files {
		if !c.used[p] {
			delete(c.files, p)
			c.changed = true
		}
	}
	if c.path == "" || !c.changed {
		return
	}
	data, err := json.Marshal(cacheFile{Version: cacheVersion, Files: c.files})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), "fonts-*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), c.path) != nil {
		os.Remove(tmp.Name())
	}
}
