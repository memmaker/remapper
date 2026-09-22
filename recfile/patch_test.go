package recfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A save must keep the file as the author wrote it: comments, blank lines and untouched fields stay put.
func TestPatchKeepsTheRestOfTheFile(t *testing.T) {
	src := "%rec: default\n# docs: every field\n#   icon  the tile\n\ninternal_name: a\n# a note on a\nicon: 1\nname: A\n\ninternal_name: b\nname: B\nsound: x.ogg\nsound: y.ogg\n# trailing\n"
	path := filepath.Join(t.TempDir(), "t.rec")
	os.WriteFile(path, []byte(src), 0o644)
	same := func(id string) ([]string, bool) { return map[string][]string{"a": {"1"}}[id], id == "a" }
	if err := PatchFieldInFile(path, "internal_name", "icon", same); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != src {
		t.Fatalf("an unchanged save rewrote the file:\n%s", got)
	}
	PatchFieldInFile(path, "internal_name", "icon", func(id string) ([]string, bool) { return []string{"7"}, id == "a" })
	PatchFieldInFile(path, "internal_name", "sound", func(id string) ([]string, bool) { return []string{"z.ogg"}, id == "b" })
	want := strings.Replace(strings.Replace(src, "icon: 1", "icon: 7", 1), "sound: x.ogg\nsound: y.ogg", "sound: z.ogg", 1)
	if got, _ := os.ReadFile(path); string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// The real files: re-saving them unchanged must not change a byte.
func TestPatchRealModFilesUnchanged(t *testing.T) {
	root := os.ExpandEnv("$HOME/Projects/heavenAndHell/files/mods")
	files, _ := filepath.Glob(root + "/*/data/*.rec")
	icons, _ := filepath.Glob(root + "/*/data/icons/*.rec")
	for _, f := range append(files, icons...) {
		orig, _ := os.ReadFile(f)
		cp := filepath.Join(t.TempDir(), filepath.Base(f))
		os.WriteFile(cp, orig, 0o644)
		byID := map[string][]string{}
		for _, r := range Read(strings.NewReader(string(orig))) {
			id := r.FindFirstFieldValue("internal_name")
			if _, seen := byID[id+".seen"]; seen { // a repeated id gets the first record's values, as the tools give it
				continue
			}
			byID[id+".seen"] = nil
			for _, fl := range r {
				if fl.Name == "sound" || fl.Name == "icon" {
					byID[id+"."+fl.Name] = append(byID[id+"."+fl.Name], fl.Value)
				}
			}
		}
		for _, field := range []string{"icon", "sound"} {
			PatchFieldInFile(cp, "internal_name", field, func(id string) ([]string, bool) {
				v, ok := byID[id+"."+field]
				return v, ok
			})
		}
		if got, _ := os.ReadFile(cp); string(got) != string(orig) {
			t.Errorf("%s changed on an unchanged save", f)
		}
	}
}
