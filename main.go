package main

import (
	"ReMapper/recfile"
	"ReMapper/renderer"
	"errors"
	"github.com/hajimehoshi/ebiten/v2"
	"io"
	"log"
	"os"
	"strconv"
)

func buildCurrentMapping(mappingRecFile string) ([]recfile.Record, map[string]int32) {
	mapping := make(map[string]int32)
	file, _ := os.Open(mappingRecFile)
	records := recfile.Read(file)
	file.Close()

	for _, rec := range records {
		var icon int32
		var internalName string
		for _, field := range rec {
			if field.Name == "icon" {
				icon = field.AsInt32()
			} else if field.Name == "internal_name" {
				internalName = field.Value
			}
		}
		mapping[internalName] = icon
	}
	return records, mapping
}

func main() {

	if len(os.Args) < 5 {
		log.Fatal("Usage: remapper <cell width> <cell height> <atlas png file> <mapping rec file>")
	}
	// read the first two command line arguments

	cellWidth, _ := strconv.Atoi(os.Args[1])
	cellHeight, _ := strconv.Atoi(os.Args[2])
	atlasName := os.Args[3]
	mappingFileName := os.Args[4]

	originalRecords, mapping := buildCurrentMapping(mappingFileName)
	atlas := renderer.NewTextureAtlas(atlasName, cellWidth, cellHeight)

	engine := NewEngine(1200, 800, "ReMapper")
	engine.SetTTFFont(mustOpen("FiraSans-Regular.ttf"), 16)
	engine.SetAtlas(atlas)
	engine.SetMapping(mappingFileName, mapping, originalRecords)

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

func mustOpen(filename string) io.ReaderAt {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	return f
}
