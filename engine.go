package main

import (
	"ReMapper/geometry"
	"ReMapper/recfile"
	"ReMapper/renderer"
	"cmp"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"slices"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

type Engine struct {
	shouldQuit                  bool
	deviceIndependentScreenSize geometry.Point
	deviceDPIScale              float64
	title                       string
	renderer                    *renderer.TileRenderer
	tileScale                   float64
	mousePosInPixels            geometry.Point

	// use-case specific
	iconMapping         map[string]int32
	labels              map[string]string        // internal_name -> text shown in the list
	referenceImages     map[string]string        // internal_name -> reference_image path, from the rec file
	referenceImageCache map[string]*ebiten.Image // path -> loaded image, or nil for one that failed to load
	orderedKeys         []string
	tileAtlas           renderer.TextureAtlas
	scrollOffset        float64
	listWidth           float64
	padding             float64
	drawInfos           []ElementInfo
	bounds              [][2]int
	selectedListIndex   int
	atlasScale          float64
	atlasBounds         geometry.Rect
	atlasSelectorPos    geometry.Point
	drawAtlasCursor     bool
	selectedAtlasIndex  int32
	originalRecords     []recfile.Record
	mappingFileName     string
	saveTicks           int
	showHelp            bool

	// typed rec files (project.go)
	typed       bool
	types       []string          // category sections in file order
	typeDocs    map[string]string // category -> %doc display name
	category    int               // index into types, -1: all
	allKeys     []string
	scenes      []scene
	sceneIndex  int
	showPreview bool
	cutMode     bool
	atlasScroll float64 // pixels the atlas is scrolled up (mouse wheel over it)
	pendingDrop *droppedSheet
}

var helpLines = []string{
	"ReMapper hotkeys",
	"",
	"F1          toggle this help",
	"s           save changes",
	"F10         quit",
	"mouse wheel scroll the list (or the atlas, over it)",
	"+ / -       zoom the atlas",
	"click list  select an entry",
	"click atlas assign that icon to the selected entry",
	"drop a png  use it as the tileset (asks about the old indexes)",
	"c           cut mode: arrows offset, shift+arrows tile size, alt+arrows gap",
	"",
	"typed rec files (remapper <file.rec>):",
	"Tab         next category (list filter)",
	"F2          scene preview on/off",
	"Space       next scene (preview)",
	"click       (preview) select that entry",
	"Del         mark the selected entry unassigned",
}

func NewEngine(width, height int, title string) *Engine {
	engine := &Engine{
		deviceDPIScale:              ebiten.DeviceScaleFactor(),
		deviceIndependentScreenSize: geometry.Point{X: width, Y: height},
		title:                       title,
		tileScale:                   4,
		padding:                     10.0,
		selectedListIndex:           -1,
		selectedAtlasIndex:          -1,
		atlasScale:                  3,
	}
	engine.renderer = renderer.NewTileRenderer(engine.GetDeviceDPIScale, engine.GetTileScale)
	engine.renderer.SetWhiteTile(18)
	return engine
}

func (e *Engine) saveChanges(fileName string) {
	if e.typed {
		e.saveTyped()
		return
	}
	// patched in place, so the file's comments and other fields survive the save
	err := recfile.PatchFieldInFile(fileName, "internal_name", "icon", func(id string) ([]string, bool) {
		icon, ok := e.iconMapping[id]
		return []string{strconv.Itoa(int(icon))}, ok
	})
	if err != nil {
		log.Println(err)
	}
}
func (e *Engine) GetDeviceDPIScale() float64 {
	return e.deviceDPIScale
}

func (e *Engine) GetTileScale() float64 {
	return e.tileScale
}
func (e *Engine) SetTTFFont(file io.ReaderAt, size float64) {
	tt, err := opentype.ParseReaderAt(file)
	if err != nil {
		println(err.Error())
		return
	}
	dpi := 72 * e.deviceDPIScale
	fontFace, faceErr := opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingVertical,
	})
	if faceErr != nil {
		println(faceErr.Error())
	}
	//mplusBigFont = text.FaceWithLineHeight(mplusBigFont, 54) // adjust line height
	e.renderer.SetTTF(fontFace)
}

func (e *Engine) Update() error {
	if e.shouldQuit {
		return ebiten.Termination
	}
	e.handleInput()
	if e.saveTicks > 0 {
		e.saveTicks--
	}
	return nil
}

func (e *Engine) Draw(screen *ebiten.Image) {
	e.renderer.SetRenderTarget(screen)

	if e.saveTicks > 0 {
		saveText := "Saved Changes!"
		saveTextWidth, saveTextHeight := e.renderer.MeasureString(saveText)
		// center on screen
		saveTextX := (float64(e.deviceIndependentScreenSize.X) - float64(saveTextWidth)) / 2
		saveTextY := (float64(e.deviceIndependentScreenSize.Y) - float64(saveTextHeight)) / 2
		e.renderer.DrawTTFOnScreen(saveTextX, saveTextY, saveText, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		return
	}
	if e.showHelp {
		for i, line := range helpLines {
			e.renderer.DrawTTFOnScreen(e.padding, 30+float64(i)*24, line, color.White)
		}
		return
	}
	if e.pendingDrop != nil {
		e.drawDropQuestion()
		return
	}
	if e.showPreview {
		e.drawPreview()
		return
	}
	// list
	for index, drawInfo := range e.drawInfos {
		key := e.orderedKeys[index]
		e.drawIcon(drawInfo.IconPosition.X, drawInfo.IconPosition.Y, key, e.listIconScale())
		drawColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		if index == e.selectedListIndex {
			drawColor = color.RGBA{R: 255, G: 76, B: 67, A: 255}
		}
		e.renderer.DrawTTFOnScreen(drawInfo.TextPosition.X, drawInfo.TextPosition.Y, e.labels[key], drawColor)
	}

	// atlas
	e.renderer.DrawImageOnScreen(e.atlasBounds.Min.X, e.atlasBounds.Min.Y, e.atlasBounds.Size(), e.tileAtlas.GetImage())

	atlasTileSize := e.tileAtlas.GetTileSize().MulF(e.atlasScale)

	if e.drawAtlasCursor { // selection cursor
		e.renderer.DrawColoredRect(e.atlasSelectorPos, atlasTileSize, color.RGBA{R: 30, G: 200, B: 30, A: 75})
	}

	if e.selectedAtlasIndex >= 0 {
		cellCountX := e.tileAtlas.GetCellCount().X
		gridPosX, gridPosY := IndexToXY(int(e.selectedAtlasIndex), cellCountX)
		drawPos := e.gridToScreen(geometry.Point{X: gridPosX, Y: gridPosY})
		e.renderer.DrawColoredRect(drawPos, atlasTileSize, color.RGBA{R: 30, G: 25, B: 200, A: 75})
	}

	e.drawReferenceImage()
}

// referenceImageBoxSize is how much screen space the reference picture gets, right of the atlas.
var referenceImageBoxSize = geometry.Point{X: 320, Y: 320}

// drawReferenceImage shows the selected entry's reference_image (if it has one) beside the atlas, scaled down
// to fit referenceImageBoxSize without distorting it, so the user can compare it against candidate tiles.
func (e *Engine) drawReferenceImage() {
	if e.selectedListIndex < 0 || e.selectedListIndex >= len(e.orderedKeys) {
		return
	}
	path, ok := e.referenceImages[e.orderedKeys[e.selectedListIndex]]
	if !ok {
		return
	}
	img := e.loadReferenceImage(path)
	if img == nil {
		return
	}
	boxX := e.atlasBounds.Max.X + int(e.padding)
	bounds := img.Bounds()
	scale := min(float64(referenceImageBoxSize.X)/float64(bounds.Dx()), float64(referenceImageBoxSize.Y)/float64(bounds.Dy()))
	size := geometry.Point{X: int(float64(bounds.Dx()) * scale), Y: int(float64(bounds.Dy()) * scale)}
	e.renderer.DrawImageOnScreen(boxX, 0, size, img)
}

// loadReferenceImage lazily decodes and caches a reference_image path; a load failure is cached as nil so a
// missing/bad file is only ever attempted once.
func (e *Engine) loadReferenceImage(path string) *ebiten.Image {
	if img, cached := e.referenceImageCache[path]; cached {
		return img
	}
	f, err := os.Open(path)
	if err != nil {
		e.referenceImageCache[path] = nil
		return nil
	}
	defer f.Close()
	decoded, _, err := image.Decode(f)
	if err != nil {
		e.referenceImageCache[path] = nil
		return nil
	}
	img := ebiten.NewImageFromImage(decoded)
	e.referenceImageCache[path] = img
	return img
}

type ElementInfo struct {
	IconPosition geometry.PointF
	TextPosition geometry.PointF
}

// listIconSize is the height of a list icon in screen points; rows are that tall plus listRowGap.
const listIconSize, listRowGap = 32.0, 6.0

// listIconScale scales an atlas tile to listIconSize, whatever the tile size.
func (e *Engine) listIconScale() float64 {
	return listIconSize / float64(e.tileAtlas.GetTileSize().Y)
}

func (e *Engine) updateElementBounds() {
	maxWidth := 0.0
	maxHeight := 0.0
	tileSize := e.tileAtlas.GetTileSize()
	scaledIconSize := geometry.PointF{X: float64(tileSize.X) * e.listIconScale(), Y: listIconSize}

	drawX := e.padding
	drawY := e.scrollOffset
	var drawInfo []ElementInfo
	var boundsInfo [][2]int
	for _, key := range e.orderedKeys {
		//e.renderer.DrawScaledTile(drawX, drawY, e.tileAtlas, currentIcon, iconScale, color.White)
		iconPosition := geometry.PointF{X: drawX, Y: drawY}
		tW, tH := e.renderer.MeasureString(e.labels[key])
		if tW > maxWidth {
			maxWidth = tW
		}
		if tH > maxHeight {
			maxHeight = tH
		}
		// the text's baseline, so that the text sits in the middle of the icon's height
		textPosition := geometry.PointF{X: drawX + scaledIconSize.X + e.padding, Y: drawY + (listIconSize+tH)/2 - tH/5}
		//e.renderer.DrawTTFOnScreen(drawX+scaledIconSize.X+e.padding, drawY+tH, key, color.White)

		drawInfo = append(drawInfo, ElementInfo{
			IconPosition: iconPosition,
			TextPosition: textPosition,
		})

		boundsInfo = append(boundsInfo, [2]int{int(drawY), int(drawY + listIconSize + listRowGap)})

		drawY += listIconSize + listRowGap

	}
	e.listWidth = maxWidth + scaledIconSize.X + e.padding*3
	e.bounds = boundsInfo
	e.drawInfos = drawInfo

	atlasX := int(e.listWidth + e.padding)
	atlasSize := e.tileAtlas.GetAtlasSize().MulF(e.atlasScale)
	top := -int(e.atlasScroll)
	e.atlasBounds = geometry.NewRect(atlasX, top, atlasX+atlasSize.X, top+atlasSize.Y)
}
func (e *Engine) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	panic("implement me")
}

func (e *Engine) LayoutF(outsideWidth, outsideHeight float64) (screenWidth, screenHeight float64) {
	//e.deviceDPIScale = ebiten.DeviceScaleFactor()
	intWidth := int(outsideWidth)
	intHeight := int(outsideHeight)
	if e.deviceIndependentScreenSize.X != intWidth || e.deviceIndependentScreenSize.Y != intHeight {
		e.deviceIndependentScreenSize = geometry.Point{X: intWidth, Y: intHeight}
		e.OnScreenSizeChanged()
	}
	return outsideWidth * e.deviceDPIScale, outsideHeight * e.deviceDPIScale
}

func (e *Engine) OnScreenSizeChanged() {
	newW, newH := ebiten.WindowSize()
	//e.deviceDPIScale = ebiten.DeviceScaleFactor()
	e.deviceIndependentScreenSize = geometry.Point{X: newW, Y: newH}
}

func (e *Engine) GetDeviceIndependentScreenSize() geometry.Point {
	return e.deviceIndependentScreenSize
}

func (e *Engine) GetTitle() string {
	return e.title
}

func (e *Engine) SetAtlas(atlas renderer.TextureAtlas) {
	e.tileAtlas = atlas
	e.renderer.SetDefaultAtlas(atlas)
	e.updateElementBounds()
}

func (e *Engine) SetMapping(mappingFileName string, mapping map[string]int32, labels map[string]string, referenceImages map[string]string, records []recfile.Record) {
	e.iconMapping = mapping
	e.labels = labels
	e.referenceImages = referenceImages
	e.referenceImageCache = make(map[string]*ebiten.Image)
	e.mappingFileName = mappingFileName
	var orderedKeys []string

	for k := range mapping {
		orderedKeys = append(orderedKeys, k)
	}

	slices.SortStableFunc(orderedKeys, func(i, j string) int {
		return cmp.Or(cmp.Compare(labels[i], labels[j]), cmp.Compare(i, j))
	})

	e.orderedKeys = orderedKeys
	e.updateElementBounds()

	e.originalRecords = records
}

func (e *Engine) handleMouseClick() bool {
	// find the selected icon
	for index, bound := range e.bounds {
		if e.mousePosInPixels.X <= int(e.listWidth) {
			if e.mousePosInPixels.Y >= bound[0] && e.mousePosInPixels.Y <= bound[1] {
				key := e.orderedKeys[index]
				e.selectedListIndex = index
				e.selectedAtlasIndex = e.iconMapping[key]
				e.revealAtlasIndex()
				return true
			}
		} else if e.selectedListIndex >= 0 && e.selectedListIndex < len(e.orderedKeys) {
			// atlas clicked..
			atlasPos := e.atlasGridFromScreenPos(e.mousePosInPixels)
			atlasIndex := XYToIndex(atlasPos.X, atlasPos.Y, e.tileAtlas.GetCellCount().X)
			//println(fmt.Sprintf("atlas %s", atlasPos.String()))
			selectedKey := e.orderedKeys[e.selectedListIndex]
			e.iconMapping[selectedKey] = int32(atlasIndex)
			e.selectedAtlasIndex = int32(atlasIndex)
		}
	}
	return false
}

func (e *Engine) atlasGridFromScreenPos(screenPos geometry.Point) geometry.Point {
	relativeToAtlas := screenPos.Sub(e.atlasBounds.Min)
	relativeToAtlas = relativeToAtlas.DivF(e.atlasScale)
	return e.tileAtlas.CellAt(relativeToAtlas)
}

func (e *Engine) OnMouseMoved(mousePos geometry.Point) {
	e.drawAtlasCursor = false
	if !e.atlasBounds.Contains(mousePos) {
		return
	}

	gridPos := e.atlasGridFromScreenPos(e.mousePosInPixels)
	drawPosForSelector := e.gridToScreen(gridPos)
	e.atlasSelectorPos = drawPosForSelector
	e.drawAtlasCursor = true
}

func (e *Engine) gridToScreen(gridPos geometry.Point) geometry.Point {
	origin := e.tileAtlas.CellOrigin(gridPos)
	drawPosForSelector := geometry.Point{
		X: int(float64(origin.X)*e.atlasScale) + e.atlasBounds.Min.X,
		Y: int(float64(origin.Y)*e.atlasScale) + e.atlasBounds.Min.Y,
	}
	return drawPosForSelector
}

func IndexToXY(index int, width int) (int, int) {
	return index % width, index / width
}

func XYToIndex(x int, y int, width int) int {
	return y*width + x
}

// revealAtlasIndex scrolls the atlas so the selected icon is on screen.
func (e *Engine) revealAtlasIndex() {
	if e.selectedAtlasIndex < 0 {
		return
	}
	_, y := IndexToXY(int(e.selectedAtlasIndex), e.tileAtlas.GetCellCount().X)
	top := e.gridToScreen(geometry.Point{Y: y}).Y + int(e.atlasScroll)
	if top < int(e.atlasScroll) || top > int(e.atlasScroll)+e.deviceIndependentScreenSize.Y-100 {
		e.atlasScroll = max(0, float64(top-e.deviceIndependentScreenSize.Y/3))
		e.updateElementBounds()
	}
}
