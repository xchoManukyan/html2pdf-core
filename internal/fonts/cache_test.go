package fonts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFontIndexCache(t *testing.T) {
	src, err := os.ReadFile("../ttf/testdata/NotoSansTamil-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	fontDir, cacheDir := t.TempDir(), t.TempDir()
	t.Setenv("HTML2PDF_FONT_CACHE", cacheDir)
	os.WriteFile(filepath.Join(fontDir, "tamil.ttf"), src, 0o644)
	os.WriteFile(filepath.Join(fontDir, "broken.ttf"), []byte("not a font"), 0o644)

	scan := func() *DB {
		db, c := NewDB(), loadCache()
		scanDir(db, fontDir, c)
		c.save()
		return db
	}
	first := scan()
	if first.Match("Noto Sans Tamil", 400, false) == nil {
		t.Fatal("the font was not indexed")
	}
	var saved cacheFile
	data, err := os.ReadFile(filepath.Join(cacheDir, "fonts.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || len(saved.Files) != 2 {
		t.Fatalf("cache not written: %v %s", err, data)
	}

	// a forged entry: another name for the real file, and a file that is not in the font folder
	for p, e := range saved.Files {
		if filepath.Base(p) == "tamil.ttf" {
			e.Faces[0].Family = "Forged"
		}
	}
	saved.Files[filepath.Join(fontDir, "..", "secret.ttf")] = &cachedFile{Size: 1, Faces: []cachedFace{{Family: "Secret"}}}
	data, _ = json.Marshal(saved)
	os.WriteFile(filepath.Join(cacheDir, "fonts.json"), data, 0o600)
	second := scan()
	if second.Match("Secret", 400, false) != nil {
		t.Error("a cache entry for a file outside the font folders was used")
	}
	if second.Match("Forged", 400, false) == nil {
		t.Error("an unchanged file should come from the cache") // same size and time: the cache is trusted for names
	}

	// a changed file is read again
	os.WriteFile(filepath.Join(fontDir, "tamil.ttf"), append(src, 0), 0o644)
	if third := scan(); third.Match("Noto Sans Tamil", 400, false) == nil || third.Match("Forged", 400, false) != nil {
		t.Error("a changed file must be read again")
	}

	t.Setenv("HTML2PDF_FONT_CACHE", "off")
	if cachePath() != "" {
		t.Error("HTML2PDF_FONT_CACHE=off must turn the cache off")
	}
}
