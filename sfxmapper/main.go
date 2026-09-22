// sfxmapper assigns sound files to sound events in a rec file: pick an event on the left, its assigned sounds show
// in the middle (play, Del), browse folders on the right and add sounds with "Add" or by dragging them onto the
// middle pane. s saves, F10 quits, F1 lists the hotkeys.
//
// Rec format, one record per event: internal_name, optional name (list label), optional tile (an internal_name
// looked up in the icon rec files), and one "sound:" field per file, relative to the sound root.
package main

import (
	"ReMapper/geometry"
	"ReMapper/recfile"
	"ReMapper/renderer"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"image/color"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

const (
	rowH    = 28
	leftW   = 420 // events pane, then the assigned pane
	midW    = 420
	rightX  = leftW + midW
	playW   = 44
	btnW    = 44
	padding = 8
)

var (
	white = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	red   = color.RGBA{R: 255, G: 76, B: 67, A: 255}
	gray  = color.RGBA{R: 140, G: 140, B: 140, A: 255}
	frame = color.RGBA{R: 90, G: 90, B: 90, A: 255}
	dark  = color.RGBA{R: 20, G: 20, B: 20, A: 255}
	green = color.RGBA{R: 30, G: 110, B: 30, A: 255}
)

type event struct {
	rec    recfile.Record
	label  string
	icon   int32
	tiled  bool
	sounds []string
}

type app struct {
	r                       *renderer.TileRenderer
	atlas                   renderer.TextureAtlas
	dpi                     float64
	w, h                    int
	events                  []*event
	recFile, root           string
	sel                     int
	dir                     string   // relative to root, "" is the root
	entries                 []string // current folder: ".." and "name/" folders first, then sound files
	leftScroll, rightScroll float64
	midScroll               float64
	drag                    string // browser sound under a held mouse button
	dragFrom                [2]float64
	dragging                bool // drag moved far enough to count as drag and drop
	audioCtx                *audio.Context
	player                  *audio.Player
	saveTicks               int
	showHelp                bool
}

var helpLines = []string{
	"SfxMapper hotkeys",
	"",
	"F1                toggle this help",
	"s                 save",
	"F10               quit",
	"mouse wheel       scroll the pane under the cursor",
	"click event       select it (left pane)",
	"click play        play the sound",
	"click Del         remove the sound from the event",
	"click Add         add the sound to the event",
	"click folder      open it",
	"drag sound        drop it on the middle pane to add it",
}

func main() {
	if len(os.Args) < 7 {
		log.Fatal("Usage: sfxmapper <tile width> <tile height> <atlas png> <sounds rec file> <sound root dir> <icon rec file>...")
	}
	tileW, _ := strconv.Atoi(os.Args[1])
	tileH, _ := strconv.Atoi(os.Args[2])
	a := &app{
		atlas:    renderer.NewTextureAtlas(os.Args[3], tileW, tileH),
		recFile:  os.Args[4],
		root:     os.Args[5],
		dpi:      ebiten.DeviceScaleFactor(),
		w:        1400,
		h:        900,
		sel:      -1,
		audioCtx: audio.NewContext(44100),
	}
	a.r = renderer.NewTileRenderer(func() float64 { return a.dpi }, func() float64 { return 3 })
	a.r.SetDefaultAtlas(a.atlas)
	a.r.SetWhiteTile(18)
	tt, _ := opentype.Parse(goregular.TTF)
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{Size: 16, DPI: 72 * a.dpi, Hinting: font.HintingVertical})
	if err != nil {
		log.Fatal(err)
	}
	a.r.SetTTF(face)

	icons := map[string]int32{}
	for _, name := range os.Args[6:] {
		for _, rec := range readRec(name) {
			icons[rec.FindFirstFieldValue("internal_name")] = int32(recfile.StrInt(rec.FindFirstFieldValue("icon")))
		}
	}
	for _, rec := range readRec(a.recFile) {
		e := &event{rec: rec, label: cmp.Or(rec.FindFirstFieldValue("name"), rec.FindFirstFieldValue("internal_name"))}
		e.icon, e.tiled = icons[rec.FindFirstFieldValue("tile")]
		for _, f := range rec {
			if f.Name == "sound" {
				e.sounds = append(e.sounds, f.Value)
			}
		}
		a.events = append(a.events, e)
	}
	a.openDir("")

	ebiten.SetWindowTitle("SfxMapper")
	ebiten.SetWindowSize(a.w, a.h)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(a); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}

func readRec(name string) []recfile.Record {
	f, err := os.Open(name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	return recfile.Read(f)
}

func (a *app) openDir(dir string) {
	a.dir, a.rightScroll, a.entries = dir, 0, nil
	if dir != "" {
		a.entries = append(a.entries, "../")
	}
	list, _ := os.ReadDir(filepath.Join(a.root, dir))
	var sounds []string
	for _, e := range list {
		if e.IsDir() {
			a.entries = append(a.entries, e.Name()+"/")
		} else if strings.HasSuffix(e.Name(), ".ogg") {
			sounds = append(sounds, path.Join(dir, e.Name()))
		}
	}
	a.entries = append(a.entries, sounds...)
}

func (a *app) play(sound string) {
	if a.player != nil {
		a.player.Close()
	}
	data, err := os.ReadFile(filepath.Join(a.root, sound))
	if err != nil {
		println(err.Error())
		return
	}
	stream, err := vorbis.DecodeWithSampleRate(a.audioCtx.SampleRate(), bytes.NewReader(data))
	if err != nil {
		println(err.Error())
		return
	}
	a.player, _ = a.audioCtx.NewPlayer(stream)
	a.player.Play()
}

func (a *app) save() {
	// patched in place, so the file's comments and every other field survive the save
	sounds := map[string][]string{}
	for _, e := range a.events {
		sounds[e.rec.FindFirstFieldValue("internal_name")] = e.sounds
	}
	err := recfile.PatchFieldInFile(a.recFile, "internal_name", "sound", func(id string) ([]string, bool) {
		s, ok := sounds[id]
		return s, ok
	})
	if err != nil {
		log.Fatal(err)
	}
	a.saveTicks = 30
}

func (a *app) assigned() []string {
	if a.sel < 0 {
		return nil
	}
	return a.events[a.sel].sounds
}

func (a *app) add(sound string) {
	if a.sel >= 0 && !slices.Contains(a.events[a.sel].sounds, sound) {
		a.events[a.sel].sounds = append(a.events[a.sel].sounds, sound)
	}
}

// row is the index of the list row under y in a pane with a header row, or -1.
func row(y, scroll float64, count int) int {
	i := int((y - rowH - scroll) / rowH)
	if y < rowH || i < 0 || i >= count {
		return -1
	}
	return i
}

func (a *app) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		a.showHelp = !a.showHelp
	}
	if a.showHelp {
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyS) {
		a.save()
	}
	if a.saveTicks > 0 {
		a.saveTicks--
	}
	cx, cy := ebiten.CursorPosition()
	mx, my := float64(cx)/a.dpi, float64(cy)/a.dpi
	if _, dy := ebiten.Wheel(); dy != 0 {
		scroll, count := &a.rightScroll, len(a.entries)+1
		if mx < leftW {
			scroll, count = &a.leftScroll, len(a.events)
		} else if mx < rightX {
			scroll, count = &a.midScroll, len(a.assigned())+1
		}
		*scroll = min(0, max(*scroll+dy*20, float64(rowH*(1-count))))
	}

	if a.drag != "" {
		if max(abs(mx-a.dragFrom[0]), abs(my-a.dragFrom[1])) > 4 {
			a.dragging = true
		}
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			if a.dragging && mx >= leftW && mx < rightX {
				a.add(a.drag)
			}
			a.drag, a.dragging = "", false
		}
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	switch {
	case mx < leftW:
		if i := int((my - a.leftScroll) / rowH); i >= 0 && i < len(a.events) {
			a.sel, a.midScroll = i, 0
		}
	case mx < rightX:
		sounds := a.assigned()
		if i := row(my, a.midScroll, len(sounds)); i >= 0 {
			if mx < leftW+padding+playW {
				a.play(sounds[i])
			} else if mx >= rightX-padding-btnW {
				a.events[a.sel].sounds = slices.Delete(sounds, i, i+1)
			}
		}
	default:
		i := row(my, a.rightScroll, len(a.entries))
		if i < 0 {
			return nil
		}
		switch entry := a.entries[i]; {
		case entry == "../":
			a.openDir(strings.TrimPrefix(path.Dir(a.dir), "."))
		case strings.HasSuffix(entry, "/"):
			a.openDir(path.Join(a.dir, entry))
		case mx < rightX+padding+playW:
			a.play(entry)
		case mx >= float64(a.w)-padding-btnW:
			a.add(entry)
		default: // the frame itself does nothing on click, only starts a drag
			a.drag, a.dragFrom = entry, [2]float64{mx, my}
		}
	}
	return nil
}

func abs(f float64) float64 { return max(f, -f) }

// drawSound draws a framed row: play button, name, and an action button on the right.
func (a *app) drawSound(x, y, w int, name, button string, fill color.Color) {
	a.r.DrawColoredBorder(geometry.Point{X: x, Y: y + 2}, geometry.Point{X: w, Y: rowH - 4}, fill, frame)
	a.r.DrawColoredRect(geometry.Point{X: x + 2, Y: y + 4}, geometry.Point{X: playW - 4, Y: rowH - 8}, frame)
	a.r.DrawTTFOnScreen(float64(x+8), float64(y+20), "play", white)
	a.r.DrawTTFOnScreen(float64(x+playW+padding), float64(y+20), name, white)
	if button != "" {
		a.r.DrawColoredRect(geometry.Point{X: x + w - btnW + 2, Y: y + 4}, geometry.Point{X: btnW - 4, Y: rowH - 8}, frame)
		a.r.DrawTTFOnScreen(float64(x+w-btnW+10), float64(y+20), button, white)
	}
}

func (a *app) Draw(screen *ebiten.Image) {
	a.r.SetRenderTarget(screen)
	if a.saveTicks > 0 {
		a.r.DrawTTFOnScreen(float64(a.w)/2-60, float64(a.h)/2, "Saved Changes!", white)
		return
	}
	if a.showHelp {
		for i, line := range helpLines {
			a.r.DrawTTFOnScreen(padding, float64(30+i*rowH), line, white)
		}
		return
	}
	for i, e := range a.events {
		y := a.leftScroll + float64(i*rowH)
		if y < -rowH || y > float64(a.h) {
			continue
		}
		if e.tiled {
			a.r.DrawScaledTile(padding, y+2, a.atlas, e.icon, geometry.PointF{X: 1, Y: 1}, color.White)
		}
		textColor := white
		if i == a.sel {
			textColor = red
		}
		a.r.DrawTTFOnScreen(padding+32, y+20, fmt.Sprintf("%s  (%d)", e.label, len(e.sounds)), textColor)
	}

	selected := a.assigned()
	x := leftW + padding
	if a.sel < 0 {
		a.r.DrawTTFOnScreen(float64(x), 20, "select an event", gray)
	} else {
		a.r.DrawTTFOnScreen(float64(x), 20, a.events[a.sel].label, gray)
	}
	if a.dragging && a.sel >= 0 {
		a.r.DrawColoredRect(geometry.Point{X: leftW, Y: rowH}, geometry.Point{X: midW, Y: a.h - rowH}, color.RGBA{R: 30, G: 60, B: 30, A: 255})
	}
	for i, sound := range selected {
		if y := rowH + int(a.midScroll) + i*rowH; y >= rowH && y <= a.h {
			a.drawSound(x, y, midW-2*padding, strings.TrimSuffix(sound, ".ogg"), "Del", dark)
		}
	}

	x = rightX + padding
	a.r.DrawTTFOnScreen(float64(x), 20, "/"+a.dir, gray)
	for i, entry := range a.entries {
		y := rowH + int(a.rightScroll) + i*rowH
		if y < rowH || y > a.h {
			continue
		}
		if strings.HasSuffix(entry, "/") {
			a.r.DrawTTFOnScreen(float64(x), float64(y+20), "[ "+entry+" ]", white)
			continue
		}
		fill := dark
		if slices.Contains(selected, entry) {
			fill = green
		}
		a.drawSound(x, y, a.w-x-padding, strings.TrimSuffix(path.Base(entry), ".ogg"), "Add", fill)
	}

	if a.dragging {
		cx, cy := ebiten.CursorPosition()
		a.drawSound(int(float64(cx)/a.dpi)+8, int(float64(cy)/a.dpi)-rowH/2, midW-2*padding, strings.TrimSuffix(path.Base(a.drag), ".ogg"), "", green)
	}
}

func (a *app) Layout(int, int) (int, int) { panic("LayoutF is used") }

func (a *app) LayoutF(w, h float64) (float64, float64) {
	a.w, a.h = int(w), int(h)
	return w * a.dpi, h * a.dpi
}
