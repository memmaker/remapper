package main

// Tile colours of typed rec files: the Tileset record's "palette" names a rec file with a "colors" record
// ("Name: r | g | b" per field, a game theme's colour list does), and an entry's "color" field is one of those
// names. The list, the atlas highlight and the preview draw the tile tinted with it; a strip of swatches at the
// bottom of the window sets it for the selected entry (X removes it). Saved as the color field, like icon.

import (
	"ReMapper/geometry"
	"ReMapper/recfile"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
)

type paletteEntry struct {
	name string
	c    color.RGBA
}

func loadPalette(path string) ([]paletteEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs := recfile.ReadMulti(f)["colors"]
	if len(recs) == 0 {
		return nil, fmt.Errorf("%s: no %%rec: colors record", path)
	}
	var out []paletteEntry
	for _, fl := range recs[0] {
		p := strings.Split(fl.Value, "|")
		if len(p) != 3 {
			continue
		}
		var v [3]uint8
		for i := range p {
			n, _ := strconv.Atoi(strings.TrimSpace(p[i]))
			v[i] = uint8(n)
		}
		out = append(out, paletteEntry{fl.Name, color.RGBA{v[0], v[1], v[2], 255}})
	}
	return out, nil
}

// tint is the colour an entry's tile is drawn in: its color field's palette entry, else white (names match
// case-insensitively, as the game resolves them).
func (e *Engine) tint(key string) color.Color {
	name := e.colors[key]
	for _, p := range e.palette {
		if strings.EqualFold(p.name, name) {
			return p.c
		}
	}
	return color.White
}

const swatchSize, swatchGap, swatchBottom = 18, 4, 8

// swatchRect is swatch i's place: a row along the bottom edge, wrapping into rows above when the window is narrow.
func (e *Engine) swatchRect(i int) geometry.Rect {
	perRow := max(1, (e.deviceIndependentScreenSize.X-2*int(e.padding))/(swatchSize+swatchGap))
	rows := (len(e.palette) + perRow - 1) / perRow
	x := int(e.padding) + (i%perRow)*(swatchSize+swatchGap)
	y := e.deviceIndependentScreenSize.Y - swatchBottom - (rows-i/perRow)*(swatchSize+swatchGap) + swatchGap
	return geometry.NewRect(x, y, x+swatchSize, y+swatchSize)
}

func (e *Engine) drawSwatches() {
	if !e.typed || len(e.palette) == 0 || e.selectedListIndex < 0 || e.selectedListIndex >= len(e.orderedKeys) {
		return
	}
	key := e.orderedKeys[e.selectedListIndex]
	for i, p := range e.palette {
		r := e.swatchRect(i)
		if strings.EqualFold(p.name, e.colors[key]) {
			e.renderer.DrawColoredRect(r.Min.Sub(geometry.Point{X: 2, Y: 2}), r.Size().Add(geometry.Point{X: 4, Y: 4}), color.White)
		}
		e.renderer.DrawColoredRect(r.Min, r.Size(), p.c)
	}
	label := "colour: " + e.colors[key]
	if e.colors[key] == "" {
		label = "colour: none (white)"
	}
	for i, p := range e.palette {
		if e.swatchRect(i).Contains(e.mousePosInPixels) {
			label = p.name
		}
	}
	y := e.swatchRect(0).Min.Y - 6
	e.renderer.DrawTTFOnScreen(e.padding, float64(y), label, color.White)
}

// swatchClick colours the selected entry when a swatch was clicked.
func (e *Engine) swatchClick() bool {
	if !e.typed || len(e.palette) == 0 || e.selectedListIndex < 0 || e.selectedListIndex >= len(e.orderedKeys) {
		return false
	}
	for i, p := range e.palette {
		if e.swatchRect(i).Contains(e.mousePosInPixels) {
			e.colors[e.orderedKeys[e.selectedListIndex]] = p.name
			return true
		}
	}
	return false
}
