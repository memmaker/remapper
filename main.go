package main

import (
	"ReMapper/recfile"
	"ReMapper/renderer"
	"cmp"
	"errors"
	"github.com/hajimehoshi/ebiten/v2"
	"io"
	"log"
	"os"
	"strconv"
)
import "embed"

//go:embed FiraSans-Regular.ttf
var embedFS embed.FS

// buildCurrentMapping also returns each record's optional "name" field, shown in the list instead of
// internal_name, and its optional "reference_image" field: a path (relative to the working directory the
// remapper was launched from) to a picture shown beside the atlas while that entry is selected, to compare
// against while picking a tile.
func buildCurrentMapping(mappingRecFile string) ([]recfile.Record, map[string]int32, map[string]string, map[string]string) {
	mapping := make(map[string]int32)
	labels := make(map[string]string)
	referenceImages := make(map[string]string)
	file, _ := os.Open(mappingRecFile)
	records := recfile.Read(file)
	file.Close()

	for _, rec := range records {
		var icon int32
		var internalName, name, referenceImage string
		for _, field := range rec {
			if field.Name == "icon" {
				icon = field.AsInt32()
			} else if field.Name == "internal_name" {
				internalName = field.Value
			} else if field.Name == "name" {
				name = field.Value
			} else if field.Name == "reference_image" {
				referenceImage = field.Value
			}
		}
		mapping[internalName] = icon
		labels[internalName] = cmp.Or(name, internalName)
		if referenceImage != "" {
			referenceImages[internalName] = referenceImage
		}
	}
	return records, mapping, labels, referenceImages
}

func main() {

	if len(os.Args) == 2 { // a typed rec file: its Tileset record names the sheet and the cutting
		engine := NewEngine(1200, 800, "ReMapper")
		engine.SetTTFFont(mustOpenEmbedded("FiraSans-Regular.ttf"), 16)
		if err := engine.LoadTyped(os.Args[1]); err != nil {
			log.Fatal(err)
		}
		runAppWithEbiten(engine)
		return
	}
	if len(os.Args) < 5 {
		log.Fatal("Usage: remapper <cell width> <cell height> <atlas png file> <mapping rec file>\n       remapper <typed rec file>")
	}
	// read the first two command line arguments

	cellWidth, _ := strconv.Atoi(os.Args[1])
	cellHeight, _ := strconv.Atoi(os.Args[2])
	atlasName := os.Args[3]
	mappingFileName := os.Args[4]

	originalRecords, mapping, labels, referenceImages := buildCurrentMapping(mappingFileName)
	atlas := renderer.NewTextureAtlas(atlasName, cellWidth, cellHeight)

	engine := NewEngine(1200, 800, "ReMapper")
	engine.SetTTFFont(mustOpenEmbedded("FiraSans-Regular.ttf"), 16)
	engine.SetAtlas(atlas)
	engine.SetMapping(mappingFileName, mapping, labels, referenceImages, originalRecords)

	runAppWithEbiten(engine)
}
func runAppWithEbiten(engine *Engine) {
	screenSize := engine.GetDeviceIndependentScreenSize()

	ebiten.SetWindowTitle(engine.GetTitle())
	ebiten.SetWindowSize(screenSize.X, screenSize.Y)
	ebiten.SetScreenClearedEveryFrame(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(640, 480, -1, -1)

	if err := ebiten.RunGameWithOptions(engine, &ebiten.RunGameOptions{
		GraphicsLibrary: ebiten.GraphicsLibraryOpenGL,
	}); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}

func mustOpenEmbedded(filename string) io.ReaderAt {
	f, err := embedFS.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	return f.(io.ReaderAt)
}
