package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestShape(t *testing.T) {
	line := shape([2]int{0, 0}, [2]int{3, 1}, false)
	if want := [][2]int{{0, 0}, {1, 0}, {2, 1}, {3, 1}}; !slices.Equal(line, want) {
		t.Errorf("line = %v, want %v", line, want)
	}
	if back := shape([2]int{3, 1}, [2]int{0, 0}, false); len(back) != 4 || back[3] != [2]int{0, 0} {
		t.Errorf("reverse line = %v", back)
	}
	if rect := shape([2]int{2, 2}, [2]int{0, 1}, true); len(rect) != 6 {
		t.Errorf("rect = %v, want 6 cells", rect)
	}
}

func TestExtract(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 11, 5)) // offset 1,1 then 2 cols x 1 row of 4x4 tiles, gap 2
	src.Set(7, 1, color.White)
	a := &app{src: src, out: t.TempDir(), tw: 4, th: 4, gx: 2, ox: 1, oy: 1, sel: map[[2]int]bool{{1, 0}: true, {5, 5}: true}}
	if err := a.extract(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(a.out, "1.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(0, 0).RGBA(); img.Bounds().Dx() != 4 || r != 0xffff {
		t.Errorf("tile 1 = %v, top-left red %x, want 4px wide starting at the white pixel", img.Bounds(), r)
	}
}
