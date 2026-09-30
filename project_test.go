package main

import (
	"image/color"
	"ReMapper/geometry"
	"ReMapper/renderer"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A typed rec round trip: load, filter, edit, save, reload; then a dropped sheet makes a new rec.
func TestTypedRec(t *testing.T) {
	dir := t.TempDir()
	writeBlankPNG(t, filepath.Join(dir, "sheet.png"), 64, 32)
	os.WriteFile(filepath.Join(dir, "game-sheet.rec"), []byte(`# kept
%rec: Tileset
id: sheet
name: Sheet
file: sheet.png
anim_file: sheet.png
tile_w: 16
tile_h: 16
scenes: scenes.rec

%rec: world
%doc: World

id: floor
name: Floor
icon: 1

%rec: monster
%doc: Monsters

id: floor
name: A monster called floor
icon: 2

id: orc
icon: -1
`), 0o644)
	os.WriteFile(filepath.Join(dir, "scenes.rec"), []byte("%rec: Scene\n\nid: s\ncategory: monster\nunder: world/floor\nlegend: . world/floor\nlegend: o monster/orc\nmap:\n+ ...\n+ .o.\n"), 0o644)

	// NewEngine asks ebiten for the display scale, which needs the main thread
	e := &Engine{deviceDPIScale: 1, tileScale: 4, padding: 10, atlasScale: 3, selectedListIndex: -1, selectedAtlasIndex: -1,
		deviceIndependentScreenSize: geometry.Point{X: 800, Y: 600}}
	e.renderer = renderer.NewTileRenderer(e.GetDeviceDPIScale, e.GetTileScale)
	e.SetTTFFont(mustOpenEmbedded("FiraSans-Regular.ttf"), 16)
	if err := e.LoadTyped(filepath.Join(dir, "game-sheet.rec")); err != nil {
		t.Fatal(err)
	}
	if len(e.allKeys) != 3 || e.iconMapping["world/floor"] != 1 || e.iconMapping["monster/floor"] != 2 || e.iconMapping["monster/orc"] != -1 {
		t.Fatalf("keys %v mapping %v", e.allKeys, e.iconMapping)
	}
	if e.labels["monster/orc"] != "orc" || e.typeDocs["monster"] != "Monsters" {
		t.Fatal("labels", e.labels, e.typeDocs)
	}
	e.cycleCategory() // world
	e.cycleCategory() // monster
	if len(e.orderedKeys) != 2 || e.categoryName() != "Monsters" || len(e.visibleScenes()) != 1 {
		t.Fatal("filter", e.orderedKeys, e.categoryName())
	}
	if s := e.currentScene(); string(s.rows[1]) != ".o." || s.legend['o'] != "monster/orc" {
		t.Fatalf("scene %+v", s)
	}

	e.iconMapping["monster/orc"] = 5
	e.tileAtlas.Offset.X, e.tileAtlas.Gap.Y = 1, 2
	e.saveTyped()
	saved, _ := os.ReadFile(filepath.Join(dir, "game-sheet.rec"))
	for _, want := range []string{"# kept", "id: orc\nicon: 5", "name: Floor\nicon: 1", "off_x: 1", "gap_y: 2"} {
		if !strings.Contains(string(saved), want) {
			t.Fatalf("saved file lacks %q:\n%s", want, saved)
		}
	}

	// shift+click: tile 1 (red left half) over tile 0 (blue) -> the first free cell, 2; saved with the rec
	e.tileAtlas.Offset, e.tileAtlas.Gap = geometry.Point{}, geometry.Point{}
	e.pixels = image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			e.pixels.Set(x, y, color.RGBA{B: 255, A: 255})
			if x < 8 {
				e.pixels.Set(16+x, y, color.RGBA{R: 255, A: 255})
			}
		}
	}
	e.animPath = ""
	if at, err := e.composeTile(0, 1); err != nil || at != 2 {
		t.Fatal("compose", at, err)
	}
	if e.pixels.RGBAAt(32+2, 5).R != 255 || e.pixels.RGBAAt(32+12, 5).B != 255 {
		t.Fatal("composed pixels", e.pixels.RGBAAt(34, 5), e.pixels.RGBAAt(44, 5))
	}
	for i := 3; i < 8; i++ { // fill the rest: the next one needs a new row
		e.composeTile(0, 1)
	}
	if at, _ := e.composeTile(0, 1); at != 8 || e.pixels.Bounds().Dy() != 48 {
		t.Fatal("grow", at, e.pixels.Bounds())
	}
	e.saveTyped()
	e.saveSheets()
	if img, err := decodeImageFile(filepath.Join(dir, "sheet.png")); err != nil || img.Bounds().Dy() != 48 {
		t.Fatal("sheet not saved", err)
	}
	saved, _ = os.ReadFile(filepath.Join(dir, "game-sheet.rec"))

	// drop: new rec next to the old one, the old one untouched
	f, _ := os.ReadFile(filepath.Join(dir, "sheet.png"))
	img, _, _ := image.Decode(strings.NewReader(string(f)))
	e.pendingDrop = &droppedSheet{name: "other.png", data: append([]byte(nil), f...), img: img}
	e.pendingDrop.data = append(e.pendingDrop.data, 0) // different bytes than sheet.png
	e.resolveDrop(true)
	if filepath.Base(e.mappingFileName) != "game-other.rec" {
		t.Fatal("new rec", e.mappingFileName)
	}
	fresh, _ := os.ReadFile(e.mappingFileName)
	for _, want := range []string{"file: other.png", "id: other", "icon: -1", "off_x: 0"} {
		if !strings.Contains(string(fresh), want) {
			t.Fatalf("new rec lacks %q:\n%s", want, fresh)
		}
	}
	if strings.Contains(string(fresh), "anim_file") || strings.Contains(string(fresh), "icon: 5") {
		t.Fatalf("new rec kept the old sheet's anim_file or icons:\n%s", fresh)
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "game-sheet.rec")); string(again) != string(saved) {
		t.Fatal("old rec changed")
	}
}

func writeBlankPNG(t *testing.T, path string, w, h int) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h)))
}
