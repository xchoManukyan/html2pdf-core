package fonts

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSystemScan(t *testing.T) {
	start := time.Now()
	db := System()
	t.Logf("scanned %d faces in %s", len(db.all), time.Since(start))
	if db.Empty() {
		t.Skip("no system fonts")
	}
	// Every installed family can be found by its own name, whatever the
	// operating system has installed.
	seen := map[string]bool{}
	for _, f := range db.all {
		if f.Family == "" || seen[strings.ToLower(f.Family)] {
			continue
		}
		seen[strings.ToLower(f.Family)] = true
		m := db.Match(f.Family, f.Weight, f.Italic)
		if m == nil || !strings.EqualFold(m.Family, f.Family) {
			t.Errorf("%q (w%d i%v) is installed but not matched", f.Family, f.Weight, f.Italic)
		}
	}
	// On Windows the core fonts are always there.
	if runtime.GOOS == "windows" {
		for _, q := range []struct {
			fam    string
			w      int
			italic bool
		}{{"Arial", 400, false}, {"Arial", 700, false}, {"Times New Roman", 700, true}, {"Segoe UI", 600, false}, {"Courier New", 400, false}} {
			f := db.Match(q.fam, q.w, q.italic)
			if f == nil {
				t.Errorf("%v: no match", q)
				continue
			}
			t.Logf("%v -> %s #%d (w%d i%v)", q, f.Path, f.Index, f.Weight, f.Italic)
		}
	}
	for _, r := range "ԱЖ漢ა" {
		if f := db.Fallback(r); f != nil {
			t.Logf("fallback %c -> %s", r, f.Path)
		} else {
			t.Logf("fallback %c -> none", r)
		}
	}
}
