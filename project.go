package main

// Typed rec files (optional; the classic "<w> <h> <atlas> <rec>" mode works as before):
//
//	%rec: Tileset            the sheet and how it is cut; the remapper saves the cutting here
//	id / name / file / tile_w / tile_h / off_x / off_y / gap_x / gap_y / scenes (design time only)
//
//	%rec: monster            one section per mapping category (Tab cycles them)
//	%doc: Monsters           the category's display name
//	id / name / icon         icon -1: unassigned
//
// scenes: a rec file of "%rec: Scene" records (id, name, category, under, legend..., map) drawn by the preview (F2).

import (
	"ReMapper/geometry"
	"ReMapper/recfile"
	"ReMapper/renderer"
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type scene struct {
	id, name, under string
	categories      []string
	legend          map[rune]string // map character -> "<category>/<id>"
	rows            [][]rune
}

type droppedSheet struct {
	name string
	data []byte
	img  image.Image
}

var cutFields = []string{"tile_w", "tile_h", "off_x", "off_y", "gap_x", "gap_y"}

func (e *Engine) cutValues() []int {
	a := e.tileAtlas
	s := a.GetTileSize()
	return []int{s.X, s.Y, a.Offset.X, a.Offset.Y, a.Gap.X, a.Gap.Y}
}

// LoadTyped reads a rec file with a Tileset record and one section per category.
func (e *Engine) LoadTyped(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sections := recfile.ReadMulti(bytes.NewReader(data))
	if len(sections["Tileset"]) == 0 {
		return fmt.Errorf("%s: no %%rec: Tileset record", path)
	}
	ts := sections["Tileset"][0].ToMap("|")
	dir := filepath.Dir(path)
	img, err := decodeImageFile(filepath.Join(dir, ts["file"]))
	if err != nil {
		return err
	}
	atlas := renderer.NewTextureAtlasFromImage(img, ts.GetIntOrDefault("tile_w", 16), ts.GetIntOrDefault("tile_h", 16))
	atlas.Offset = geometry.Point{X: ts.GetIntOrDefault("off_x", 0), Y: ts.GetIntOrDefault("off_y", 0)}
	atlas.Gap = geometry.Point{X: ts.GetIntOrDefault("gap_x", 0), Y: ts.GetIntOrDefault("gap_y", 0)}

	// section order and %doc names; the reader keeps neither
	var types []string
	docs := map[string]string{}
	cur := ""
	for sc := bufio.NewScanner(bytes.NewReader(data)); sc.Scan(); {
		if v, ok := strings.CutPrefix(sc.Text(), "%rec:"); ok {
			cur = strings.TrimSpace(v)
			if cur != "Tileset" && !slices.Contains(types, cur) {
				types = append(types, cur)
			}
		} else if v, ok := strings.CutPrefix(sc.Text(), "%doc:"); ok {
			docs[cur] = strings.TrimSpace(v)
		}
	}

	mapping, labels, refs := map[string]int32{}, map[string]string{}, map[string]string{}
	for _, t := range types {
		for _, rec := range sections[t] {
			m := rec.ToMap("|")
			if m["id"] == "" {
				continue
			}
			key := t + "/" + m["id"]
			mapping[key] = int32(m.GetIntOrDefault("icon", -1))
			labels[key] = cmp.Or(m["name"], m["id"])
			if m["reference_image"] != "" {
				refs[key] = m["reference_image"]
			}
		}
	}
	e.typed, e.types, e.typeDocs, e.category = true, types, docs, -1
	e.scenes, e.sceneIndex = nil, 0
	if ts["scenes"] != "" {
		if e.scenes, err = loadScenes(filepath.Join(dir, ts["scenes"])); err != nil {
			log.Println(err)
		}
	}
	e.selectedListIndex, e.selectedAtlasIndex, e.scrollOffset = -1, -1, 0
	e.SetAtlas(atlas)
	e.SetMapping(path, mapping, labels, refs, nil)
	e.allKeys = e.orderedKeys
	e.updateTitle()
	return nil
}

func loadScenes(path string) ([]scene, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []scene
	for _, rec := range recfile.ReadMulti(f)["Scene"] {
		s := scene{legend: map[rune]string{}}
		for _, fl := range rec {
			switch fl.Name {
			case "id":
				s.id = fl.Value
			case "name":
				s.name = fl.Value
			case "under":
				s.under = fl.Value
			case "category":
				s.categories = strings.Fields(fl.Value)
			case "legend": // "<char> <category>/<id>"
				if ch, key, ok := strings.Cut(fl.Value, " "); ok && ch != "" {
					s.legend[[]rune(ch)[0]] = strings.TrimSpace(key)
				}
			case "map":
				for _, row := range strings.Split(strings.TrimPrefix(fl.Value, "\n"), "\n") {
					s.rows = append(s.rows, []rune(row))
				}
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func (e *Engine) saveTyped() {
	path := e.mappingFileName
	err := recfile.PatchTypedFieldInFile(path, "id", "icon", func(t, id string) ([]string, bool) {
		icon, ok := e.iconMapping[t+"/"+id]
		return []string{strconv.Itoa(int(icon))}, ok
	})
	for i, v := range e.cutValues() {
		err = cmp.Or(err, e.patchTileset(path, cutFields[i], strconv.Itoa(v)))
	}
	if err != nil {
		log.Println(err)
	}
}

func (e *Engine) patchTileset(path, field string, values ...string) error {
	return recfile.PatchTypedFieldInFile(path, "id", field, func(t, _ string) ([]string, bool) { return values, t == "Tileset" })
}

// categoryName is the Tab filter's label.
func (e *Engine) categoryName() string {
	if e.category < 0 {
		return "all"
	}
	t := e.types[e.category]
	return cmp.Or(e.typeDocs[t], t)
}

func (e *Engine) cycleCategory() {
	e.category++
	if e.category >= len(e.types) {
		e.category = -1
	}
	e.orderedKeys = nil
	for _, k := range e.allKeys {
		if e.category < 0 || strings.HasPrefix(k, e.types[e.category]+"/") {
			e.orderedKeys = append(e.orderedKeys, k)
		}
	}
	e.selectedListIndex, e.scrollOffset, e.sceneIndex = -1, 0, 0
	e.updateElementBounds()
	e.updateTitle()
}

func (e *Engine) updateTitle() {
	a := e.tileAtlas
	s := a.GetTileSize()
	title := fmt.Sprintf("ReMapper - %s - %dx%d off %d,%d gap %d,%d", filepath.Base(e.mappingFileName), s.X, s.Y, a.Offset.X, a.Offset.Y, a.Gap.X, a.Gap.Y)
	if e.typed {
		title += " - " + e.categoryName()
	}
	if e.cutMode {
		title += " - CUT MODE (arrows offset, shift size, alt gap, C done)"
	}
	e.title = title
	ebiten.SetWindowTitle(title)
}

// ---- cutting ----------------------------------------------------------------

func keyRepeat(k ebiten.Key) bool {
	d := inpututil.KeyPressDuration(k)
	return d == 1 || (d > 20 && d%3 == 0)
}

func (e *Engine) handleCutKeys() {
	dx, dy := 0, 0
	if keyRepeat(ebiten.KeyArrowLeft) {
		dx = -1
	} else if keyRepeat(ebiten.KeyArrowRight) {
		dx = 1
	} else if keyRepeat(ebiten.KeyArrowUp) {
		dy = -1
	} else if keyRepeat(ebiten.KeyArrowDown) {
		dy = 1
	}
	if dx == 0 && dy == 0 {
		return
	}
	a := &e.tileAtlas
	switch {
	case ebiten.IsKeyPressed(ebiten.KeyShift):
		s := a.GetTileSize()
		a.SetTileSize(s.X+dx, s.Y+dy)
	case ebiten.IsKeyPressed(ebiten.KeyAlt):
		a.Gap = geometry.Point{X: max(0, a.Gap.X+dx), Y: max(0, a.Gap.Y+dy)}
	default:
		a.Offset = geometry.Point{X: max(0, a.Offset.X+dx), Y: max(0, a.Offset.Y+dy)}
	}
	e.SetAtlas(*a)
	e.updateTitle()
}

// ---- drag and drop ------------------------------------------------------------

func (e *Engine) checkDrop() {
	files := ebiten.DroppedFiles()
	if files == nil {
		return
	}
	entries, _ := fs.ReadDir(files, ".")
	for _, en := range entries {
		if en.IsDir() {
			continue
		}
		data, err := fs.ReadFile(files, en.Name())
		if err != nil {
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			log.Println(en.Name(), err)
			continue
		}
		e.pendingDrop = &droppedSheet{name: en.Name(), data: data, img: img}
		return
	}
}

// resolveDrop answers the "keep old indices?" question for the dropped sheet.
func (e *Engine) resolveDrop(clearIcons bool) {
	d := e.pendingDrop
	e.pendingDrop = nil
	if clearIcons {
		for k := range e.iconMapping {
			e.iconMapping[k] = -1
		}
	}
	if !e.typed { // classic mode: no Tileset record to keep it in, the sheet is swapped for this session
		atlas := renderer.NewTextureAtlasFromImage(d.img, e.tileAtlas.GetTileSize().X, e.tileAtlas.GetTileSize().Y)
		e.SetAtlas(atlas)
		e.updateTitle()
		return
	}
	if err := e.newRecForSheet(d); err != nil {
		log.Println(err)
	}
}

// newRecForSheet writes <prefix>-<sheet>.rec (and the sheet) next to the current rec, with the current,
// unsaved mapping and cutting, pointing at the dropped sheet, and switches to it. The old file stays as it is.
func (e *Engine) newRecForSheet(d *droppedSheet) error {
	dir := filepath.Dir(e.mappingFileName)
	ext := filepath.Ext(d.name)
	base := strings.TrimSuffix(d.name, ext)
	sheet, err := uniquePath(dir, base, ext, d.data)
	if err != nil {
		return err
	}
	prefix, _, _ := strings.Cut(strings.TrimSuffix(filepath.Base(e.mappingFileName), ".rec"), "-")
	old, err := os.ReadFile(e.mappingFileName)
	if err != nil {
		return err
	}
	rec, err := uniquePath(dir, prefix+"-"+base, ".rec", nil)
	if err != nil {
		return err
	}
	if err := os.WriteFile(rec, old, 0o644); err != nil {
		return err
	}
	cur := e.mappingFileName
	e.mappingFileName = rec
	e.saveTyped()
	e.mappingFileName = cur
	err = cmp.Or(
		e.patchTileset(rec, "file", filepath.Base(sheet)),
		e.patchTileset(rec, "name", base),
		e.patchTileset(rec, "anim_file"), // the old sheet's second frame doesn't fit the new one
		recfile.PatchTypedFieldInFile(rec, "id", "id", func(t, _ string) ([]string, bool) { return []string{base}, t == "Tileset" }),
	)
	if err != nil {
		return err
	}
	return e.LoadTyped(rec)
}

// uniquePath is dir/stem+ext, or stem-2, -3, ... when that exists; with data, an existing file with the same
// bytes is reused and a new one is written.
func uniquePath(dir, stem, ext string, data []byte) (string, error) {
	for i := 1; ; i++ {
		p := filepath.Join(dir, stem+ext)
		if i > 1 {
			p = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
		}
		old, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			if data != nil {
				return p, os.WriteFile(p, data, 0o644)
			}
			return p, nil
		}
		if data != nil && bytes.Equal(old, data) {
			return p, nil
		}
	}
}

// ---- scene preview ------------------------------------------------------------------

// visibleScenes are the scenes of the current category (all of them with the "all" filter).
func (e *Engine) visibleScenes() []scene {
	if e.category < 0 {
		return e.scenes
	}
	var out []scene
	for _, s := range e.scenes {
		if slices.Contains(s.categories, e.types[e.category]) {
			out = append(out, s)
		}
	}
	return out
}

func (e *Engine) currentScene() *scene {
	sc := e.visibleScenes()
	if len(sc) == 0 {
		return nil
	}
	s := sc[e.sceneIndex%len(sc)]
	return &s
}

// sceneLayout: cell size on screen and the map's top left corner.
func (e *Engine) sceneLayout(s *scene) (float64, geometry.PointF) {
	w := 1
	for _, r := range s.rows {
		w = max(w, len(r))
	}
	h := max(1, len(s.rows))
	ts := e.tileAtlas.GetTileSize()
	scr := e.deviceIndependentScreenSize
	scale := min(float64(scr.X)/float64(w*ts.X), float64(scr.Y-60)/float64(h*ts.Y))
	return scale, geometry.PointF{X: (float64(scr.X) - float64(w*ts.X)*scale) / 2, Y: 30}
}

func (e *Engine) sceneKeyAt(s *scene, mouse geometry.Point) string {
	scale, origin := e.sceneLayout(s)
	ts := e.tileAtlas.GetTileSize()
	x := int((float64(mouse.X) - origin.X) / (float64(ts.X) * scale))
	y := int((float64(mouse.Y) - origin.Y) / (float64(ts.Y) * scale))
	if float64(mouse.X) < origin.X || float64(mouse.Y) < origin.Y || y >= len(s.rows) || x >= len(s.rows[y]) {
		return ""
	}
	return s.legend[s.rows[y][x]]
}

var (
	unassignedColor = color.RGBA{R: 200, G: 30, B: 30, A: 200}
	highlightColor  = color.RGBA{R: 255, G: 230, B: 0, A: 110}
)

// drawIcon draws a mapped tile, or a red box for an unassigned or missing one.
func (e *Engine) drawIcon(x, y float64, key string, scale float64) {
	icon, ok := e.iconMapping[key]
	cells := e.tileAtlas.GetCellCount()
	if !ok || icon < 0 || int(icon) >= cells.X*cells.Y {
		ts := e.tileAtlas.GetTileSize().MulF(scale)
		e.renderer.DrawColoredRect(geometry.Point{X: int(x), Y: int(y)}, ts, unassignedColor)
		return
	}
	// DrawScaledTile would multiply by the renderer's tileScale; scale here is screen points per tile pixel
	e.renderer.DrawTileWithDefaultOrientation(x, y, e.tileAtlas, icon, geometry.PointF{X: scale, Y: scale}, color.White)
}

func (e *Engine) drawPreview() {
	s := e.currentScene()
	if s == nil {
		e.renderer.DrawTTFOnScreen(e.padding, 30, "no scenes for "+e.categoryName()+" (Tileset field scenes:)", color.White)
		return
	}
	scale, origin := e.sceneLayout(s)
	ts := e.tileAtlas.GetTileSize()
	cw, ch := float64(ts.X)*scale, float64(ts.Y)*scale
	selected := ""
	if e.selectedListIndex >= 0 && e.selectedListIndex < len(e.orderedKeys) {
		selected = e.orderedKeys[e.selectedListIndex]
	}
	for y, row := range s.rows {
		for x, r := range row {
			key, ok := s.legend[r]
			if !ok {
				continue
			}
			px, py := origin.X+float64(x)*cw, origin.Y+float64(y)*ch
			if !strings.HasPrefix(key, "world/") && s.under != "" {
				e.drawIcon(px, py, s.under, scale)
			}
			e.drawIcon(px, py, key, scale)
			if key == selected {
				e.renderer.DrawColoredRect(geometry.Point{X: int(px), Y: int(py)}, geometry.Point{X: int(cw), Y: int(ch)}, highlightColor)
			}
		}
	}
	n := len(e.visibleScenes())
	e.renderer.DrawTTFOnScreen(e.padding, 20, fmt.Sprintf("%s (%d/%d, Space: next, click: select, F2: close)", cmp.Or(s.name, s.id), e.sceneIndex%n+1, n), color.White)
	if key := e.sceneKeyAt(s, e.mousePosInPixels); key != "" {
		e.renderer.DrawTTFOnScreen(e.padding, float64(e.deviceIndependentScreenSize.Y)-10, e.labels[key]+"  ["+key+"]", color.White)
	}
}

// previewClick selects the clicked cell's entry in the list and closes the preview.
func (e *Engine) previewClick() {
	s := e.currentScene()
	if s == nil {
		return
	}
	key := e.sceneKeyAt(s, e.mousePosInPixels)
	if key == "" {
		return
	}
	if !slices.Contains(e.orderedKeys, key) {
		e.category = len(e.types) - 1 // cycles to "all"
		e.cycleCategory()
	}
	i := slices.Index(e.orderedKeys, key)
	if i < 0 {
		return // in the scene's legend but not in the rec file
	}
	e.selectedListIndex = i
	e.selectedAtlasIndex = e.iconMapping[key]
	e.scrollOffset = 0
	e.updateElementBounds()
	e.scrollOffset = -float64(e.bounds[i][0]) + 100
	e.updateElementBounds()
	e.showPreview = false
	e.revealAtlasIndex()
}

func (e *Engine) drawDropQuestion() {
	lines := []string{
		"New tileset: " + e.pendingDrop.name,
		"",
		"K   keep the current icon indexes (they may point at the wrong tiles)",
		"U   mark every entry unassigned",
		"Esc cancel",
	}
	if e.typed {
		lines = append(lines, "", "A new rec file is written next to "+filepath.Base(e.mappingFileName)+"; that one stays as it is.")
	}
	for i, l := range lines {
		e.renderer.DrawTTFOnScreen(e.padding, 40+float64(i)*24, l, color.White)
	}
}
