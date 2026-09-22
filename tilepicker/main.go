// tilepicker cherry-picks tiles from a sprite sheet. Click toggles a tile, drag toggles every tile it passes,
// shift+drag a line, ctrl+drag a rect; the tile the drag starts on decides select or deselect. e writes each selected tile to
// <sheet>_tiles/<index>.png, index counted row by row.
//
// Keys: arrows tile width/height, shift+arrows gap x/y, alt+arrows offset x/y, +/- or cmd/ctrl+wheel zoom, wheel scroll, c clear, e extract,
// F1 help, F10/Esc quit. Selected tiles are listed on the right; clicking one there deselects it.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"image/color"
)

var (
	dark     = color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	gridLine = color.NRGBA{R: 255, G: 255, B: 255, A: 40}
	yellow   = color.NRGBA{R: 255, G: 220, B: 0, A: 90}
	preview  = color.NRGBA{R: 255, G: 220, B: 0, A: 160}
	unselect = color.NRGBA{R: 255, G: 60, B: 60, A: 110}
	panel    = color.NRGBA{R: 45, G: 45, B: 45, A: 255}
)

// UI sizes in device pixels, set in main from the display scale.
var (
	ui     float64
	panelW int // selected tiles list on the right
	rowH   int
	thumb  int
	barH   int // status bar at the bottom
)

type app struct {
	img            *ebiten.Image
	src            image.Image
	out            string
	status         string
	tw, th, gx, gy int
	ox, oy         int // where the first tile starts
	zoom           float64
	sx, sy         float64
	sel            map[[2]int]bool
	dragging       bool
	from, hover    [2]int
	prev           [2]int          // hover of the previous frame
	painted        map[[2]int]bool // tiles a plain drag passed over
	showHelp       bool
	w, h           int // screen size in device pixels
	listScroll     float64
	textBuf        *ebiten.Image // scratch image for scaled debug text
}

const help = `TilePicker hotkeys

F1           toggle this help
arrows       tile width / height
shift+arrows gap x / y
alt+arrows   offset x / y
+ / -        zoom
cmd/ctrl+wheel zoom at the cursor (touchpad: two-finger scroll)
mouse wheel  scroll the sheet, or the list when over it
click list   remove that tile from the selection
click        toggle a tile
drag         toggle every tile the mouse passes
shift+drag   toggle a line of tiles
ctrl+drag    toggle a rect of tiles
c            clear selection
e            extract selected tiles to <sheet>_tiles/
F10 / Esc    quit`

func main() {
	if len(os.Args) < 4 {
		log.Fatal("Usage: tilepicker <sheet png> <tile width> <tile height> [gap x] [gap y] [offset x] [offset y]")
	}
	arg := func(i int) int {
		if i >= len(os.Args) {
			return 0
		}
		v, err := strconv.Atoi(os.Args[i])
		if err != nil {
			log.Fatalf("argument %d: %v", i, err)
		}
		return v
	}
	ui = ebiten.DeviceScaleFactor()
	panelW, rowH, thumb, barH = int(150*ui), int(24*ui), int(20*ui), int(20*ui)

	img, src, err := ebitenutil.NewImageFromFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	a := &app{img: img, src: src, out: strings.TrimSuffix(os.Args[1], filepath.Ext(os.Args[1])) + "_tiles", tw: max(1, arg(2)), th: max(1, arg(3)), gx: arg(4), gy: arg(5), ox: arg(6), oy: arg(7),
		zoom: math.Round(2 * ebiten.DeviceScaleFactor()), sel: map[[2]int]bool{}}

	ebiten.SetWindowTitle("TilePicker")
	ebiten.SetWindowSize(1200, 800)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	// OpenGL like remapper: Metal flashed at random intervals (on macOS 26, ebiten 2.6.4) while the drawn frames were identical.
	if err := ebiten.RunGameWithOptions(a, &ebiten.RunGameOptions{GraphicsLibrary: ebiten.GraphicsLibraryOpenGL}); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}

func (a *app) grid() (cols, rows int) {
	b := a.src.Bounds()
	return max(0, b.Dx()-a.ox+a.gx) / (a.tw + a.gx), max(0, b.Dy()-a.oy+a.gy) / (a.th + a.gy)
}

// tilePos returns the top-left sheet pixel of a tile.
func (a *app) tilePos(c [2]int) (x, y int) {
	return a.ox + c[0]*(a.tw+a.gx), a.oy + c[1]*(a.th+a.gy)
}

// selected returns the selected tiles inside the current grid, ordered by index.
func (a *app) selected() [][2]int {
	cols, rows := a.grid()
	var cells [][2]int
	for c := range a.sel {
		if c[0] < cols && c[1] < rows { // tiles selected before the grid shrank are skipped
			cells = append(cells, c)
		}
	}
	slices.SortFunc(cells, func(p, q [2]int) int { return cmp.Compare(p[1]*cols+p[0], q[1]*cols+q[0]) })
	return cells
}

func (a *app) extract() error {
	if err := os.MkdirAll(a.out, 0o755); err != nil {
		return err
	}
	cols, _ := a.grid()
	sub := a.src.(interface {
		SubImage(image.Rectangle) image.Image
	})
	for _, c := range a.selected() {
		x, y := a.tilePos(c)
		f, err := os.Create(filepath.Join(a.out, fmt.Sprintf("%d.png", c[1]*cols+c[0])))
		if err != nil {
			return err
		}
		err = png.Encode(f, sub.SubImage(image.Rect(x, y, x+a.tw, y+a.th)))
		if err := errors.Join(err, f.Close()); err != nil {
			return err
		}
	}
	return nil
}

// shape returns the cells of a line (Bresenham) or rect from p to q.
func shape(p, q [2]int, rect bool) [][2]int {
	var out [][2]int
	if rect {
		for y := min(p[1], q[1]); y <= max(p[1], q[1]); y++ {
			for x := min(p[0], q[0]); x <= max(p[0], q[0]); x++ {
				out = append(out, [2]int{x, y})
			}
		}
		return out
	}
	dx, dy := abs(q[0]-p[0]), -abs(q[1]-p[1])
	stepX, stepY := sign(q[0]-p[0]), sign(q[1]-p[1])
	for err := dx + dy; ; {
		out = append(out, p)
		if p == q {
			return out
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			p[0] += stepX
		}
		if e2 <= dx {
			err += dx
			p[1] += stepY
		}
	}
}

func abs(v int) int { return max(v, -v) }

func sign(v int) int {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

func (a *app) dragCells() [][2]int {
	cols, rows := a.grid()
	var cells [][2]int
	switch {
	case ebiten.IsKeyPressed(ebiten.KeyShift):
		cells = shape(a.from, a.hover, false)
	case ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta):
		cells = shape(a.from, a.hover, true)
	default:
		for c := range a.painted {
			cells = append(cells, c)
		}
	}
	return slices.DeleteFunc(cells, func(c [2]int) bool {
		return c[0] < 0 || c[1] < 0 || c[0] >= cols || c[1] >= rows
	})
}

func pressed(k ebiten.Key) bool {
	d := inpututil.KeyPressDuration(k)
	return d == 1 || d > 20 && d%3 == 0
}

func (a *app) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		a.showHelp = !a.showHelp
	}
	if a.showHelp {
		return nil
	}
	w, h, minSize := &a.tw, &a.th, 1
	switch {
	case ebiten.IsKeyPressed(ebiten.KeyAlt):
		w, h, minSize = &a.ox, &a.oy, 0
	case ebiten.IsKeyPressed(ebiten.KeyShift):
		w, h, minSize = &a.gx, &a.gy, 0
	}
	for k, d := range map[ebiten.Key]struct {
		v     *int
		delta int
	}{ebiten.KeyArrowLeft: {w, -1}, ebiten.KeyArrowRight: {w, 1}, ebiten.KeyArrowUp: {h, -1}, ebiten.KeyArrowDown: {h, 1}} {
		if pressed(k) {
			*d.v = max(minSize, *d.v+d.delta)
		}
	}
	mx, my := ebiten.CursorPosition()
	for _, r := range ebiten.AppendInputChars(nil) { // typed chars, so + and - work on any keyboard layout
		switch r {
		case '+', '=':
			a.zoomAt(1.25, mx, my)
		case '-':
			a.zoomAt(1/1.25, mx, my)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		clear(a.sel)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyE) {
		a.status = fmt.Sprintf("extracted %d tiles to %s", len(a.selected()), a.out)
		if err := a.extract(); err != nil {
			a.status = err.Error()
		}
	}
	dx, dy := ebiten.Wheel()
	switch {
	case ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta):
		a.zoomAt(math.Pow(1.1, dy), mx, my)
	case mx >= a.w-panelW:
		a.listScroll = max(0, a.listScroll-dy*20)
	default:
		a.sx, a.sy = max(0, a.sx-dx*20), max(0, a.sy-dy*20)
	}

	a.hover = [2]int{
		int(math.Floor(((float64(mx)+a.sx)/a.zoom - float64(a.ox)) / float64(a.tw+a.gx))),
		int(math.Floor(((float64(my)+a.sy)/a.zoom - float64(a.oy)) / float64(a.th+a.gy))),
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && mx >= a.w-panelW && my < a.h-barH {
		if cells, i := a.selected(), int((float64(my)+a.listScroll)/float64(rowH)); i < len(cells) {
			delete(a.sel, cells[i])
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && mx < a.w-panelW && my < a.h-barH {
		a.dragging, a.from, a.prev, a.painted = true, a.hover, a.hover, map[[2]int]bool{}
	}
	if a.dragging {
		for _, c := range shape(a.prev, a.hover, false) { // line fills cells skipped by fast mouse moves
			a.painted[c] = true
		}
		a.prev = a.hover
	}
	if a.dragging && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		a.dragging = false
		on := !a.sel[a.from]
		for _, c := range a.dragCells() {
			if on {
				a.sel[c] = true
			} else {
				delete(a.sel, c)
			}
		}
	}
	return nil
}

// zoomAt zooms by factor, keeping the sheet point under the cursor in place.
func (a *app) zoomAt(factor float64, mx, my int) {
	z := min(64, max(0.25, a.zoom*factor))
	a.sx = max(0, (float64(mx)+a.sx)/a.zoom*z-float64(mx))
	a.sy = max(0, (float64(my)+a.sy)/a.zoom*z-float64(my))
	a.zoom = z
}

// print draws debug text scaled up, since the debug font is tiny in device pixels.
func (a *app) print(screen *ebiten.Image, text string, x, y int, scale float64) {
	lines := strings.Split(text, "\n")
	w := len(slices.MaxFunc(lines, func(p, q string) int { return cmp.Compare(len(p), len(q)) }))*6 + 8 // glyphs are 6x16
	h := len(lines)*16 + 8
	if b := a.textBuf; b == nil || b.Bounds().Dx() < w || b.Bounds().Dy() < h {
		a.textBuf = ebiten.NewImage(max(w, 1024), max(h, 512))
	}
	a.textBuf.Clear()
	ebitenutil.DebugPrint(a.textBuf, text)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(a.textBuf.SubImage(image.Rect(0, 0, w, h)).(*ebiten.Image), op)
}

func (a *app) fill(screen *ebiten.Image, c [2]int, clr color.Color) {
	x, y, w, h := a.cellRect(c)
	vector.FillRect(screen, x, y, w, h, clr, false)
}

func (a *app) cellRect(c [2]int) (x, y, w, h float32) {
	z := a.zoom
	x0, y0 := a.tilePos(c)
	return float32(float64(x0)*z - a.sx), float32(float64(y0)*z - a.sy),
		float32(float64(a.tw) * z), float32(float64(a.th) * z)
}

func (a *app) Draw(screen *ebiten.Image) {
	screen.Fill(dark)
	if a.showHelp {
		a.print(screen, help, 16, 16, 2*ui) // 4x on Retina
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(a.zoom, a.zoom)
	op.GeoM.Translate(-a.sx, -a.sy)
	screen.DrawImage(a.img, op)

	cols, rows := a.grid()
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			x, y, w, h := a.cellRect([2]int{col, row})
			vector.StrokeRect(screen, x, y, w, h, 1, gridLine, false)
		}
	}
	for c := range a.sel {
		a.fill(screen, c, yellow)
	}
	if a.dragging {
		clr := color.Color(preview)
		if a.sel[a.from] {
			clr = unselect
		}
		for _, c := range a.dragCells() {
			a.fill(screen, c, clr)
		}
	}
	x, y, w, h := a.cellRect(a.hover)
	vector.StrokeRect(screen, x, y, w, h, 2, color.White, false)

	cells := a.selected()
	px := a.w - panelW
	vector.FillRect(screen, float32(px), 0, float32(panelW), float32(a.h), panel, false)
	for i, c := range cells {
		y := float64(i*rowH+(rowH-thumb)/2) - a.listScroll
		if y < -float64(rowH) || y > float64(a.h) {
			continue
		}
		x0, y0 := a.tilePos(c)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(float64(thumb)/float64(max(a.tw, a.th)), float64(thumb)/float64(max(a.tw, a.th)))
		op.GeoM.Translate(float64(px)+4*ui, y)
		screen.DrawImage(a.img.SubImage(image.Rect(x0, y0, x0+a.tw, y0+a.th)).(*ebiten.Image), op)
		a.print(screen, fmt.Sprintf("#%d (%d,%d)", c[1]*cols+c[0], c[0], c[1]), px+thumb+int(8*ui), int(y), ui) // 2x on Retina
	}

	vector.FillRect(screen, 0, float32(a.h-barH), float32(a.w), float32(barH), panel, false)
	a.print(screen, fmt.Sprintf(
		"tile %dx%d  gap %d,%d  offset %d,%d  zoom %.2f  selected %d  cell %d,%d  F1: help  %s",
		a.tw, a.th, a.gx, a.gy, a.ox, a.oy, a.zoom, len(cells), a.hover[0], a.hover[1], a.status), int(4*ui), a.h-barH, ui) // 2x on Retina
}

func (a *app) Layout(int, int) (int, int) { panic("LayoutF is used") }

func (a *app) LayoutF(w, h float64) (float64, float64) {
	s := ebiten.DeviceScaleFactor()
	a.w, a.h = int(w*s), int(h*s)
	return w * s, h * s
}
