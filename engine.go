package main

import (
	"ReMapper/geometry"
	"ReMapper/recfile"
	"ReMapper/renderer"
	"cmp"
	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"image/color"
	"io"
	"os"
	"slices"
	"strconv"
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
	iconMapping        map[string]int32
	orderedKeys        []string
	tileAtlas          renderer.TextureAtlas
	scrollOffset       float64
	listWidth          float64
	padding            float64
	drawInfos          []ElementInfo
	bounds             [][2]int
	selectedListIndex  int
	atlasScale         float64
	atlasBounds        geometry.Rect
	atlasSelectorPos   geometry.Point
	drawAtlasCursor    bool
	selectedAtlasIndex int32
	originalRecords    []recfile.Record
	mappingFileName    string
	saveTicks          int
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
	records := e.originalRecords
	for recIndex, rec := range records {
		internalName := rec.FindFirstFieldValue("internal_name")
		for fieldIndex, field := range rec {
			if field.Name == "icon" {
				changedIcon := e.iconMapping[internalName]
				field.Value = strconv.Itoa(int(changedIcon))
				records[recIndex][fieldIndex] = field
			}
		}
	}
	file, _ := os.Create(fileName)
	recfile.Write(file, records)
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
	iconScale := geometry.PointF{X: 1, Y: 1}

	if e.saveTicks > 0 {
		saveText := "Saved Changes!"
		saveTextWidth, saveTextHeight := e.renderer.MeasureString(saveText)
		// center on screen
		saveTextX := (float64(e.deviceIndependentScreenSize.X) - float64(saveTextWidth)) / 2
		saveTextY := (float64(e.deviceIndependentScreenSize.Y) - float64(saveTextHeight)) / 2
		e.renderer.DrawTTFOnScreen(saveTextX, saveTextY, saveText, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		return
	}
	// list
	for index, drawInfo := range e.drawInfos {
		key := e.orderedKeys[index]
		currentIcon := e.iconMapping[key]
		e.renderer.DrawScaledTile(drawInfo.IconPosition.X, drawInfo.IconPosition.Y, e.tileAtlas, currentIcon, iconScale, color.White)
		drawColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		if index == e.selectedListIndex {
			drawColor = color.RGBA{R: 255, G: 76, B: 67, A: 255}
		}
		e.renderer.DrawTTFOnScreen(drawInfo.TextPosition.X, drawInfo.TextPosition.Y, key, drawColor)
	}

	// atlas
	e.renderer.DrawImageOnScreen(e.atlasBounds.Min.X, e.atlasBounds.Min.Y, e.atlasBounds.Size(), e.tileAtlas.GetImage())

	atlasTileSize := e.tileAtlas.GetTileSize().MulF(e.atlasScale)

	if e.drawAtlasCursor { // selection cursor
		e.renderer.DrawColoredRect(e.atlasSelectorPos, atlasTileSize, color.RGBA{R: 30, G: 200, B: 30, A: 75})
	}

	if e.selectedAtlasIndex > 0 {
		cellCountX := e.tileAtlas.GetCellCount().X
		gridPosX, gridPosY := IndexToXY(int(e.selectedAtlasIndex), cellCountX)
		drawPos := e.gridToScreen(geometry.Point{X: gridPosX, Y: gridPosY})
		e.renderer.DrawColoredRect(drawPos, atlasTileSize, color.RGBA{R: 30, G: 25, B: 200, A: 75})
	}
}

type ElementInfo struct {
	IconPosition geometry.PointF
	TextPosition geometry.PointF
}

func (e *Engine) updateElementBounds() {
	iconScale := geometry.PointF{X: 1, Y: 1}
	maxWidth := 0.0
	maxHeight := 0.0
	tileSize := e.tileAtlas.GetTileSize()
	scaledIconSize := geometry.PointF{X: float64(tileSize.X) * e.tileScale * iconScale.X, Y: float64(tileSize.Y) * e.tileScale * iconScale.Y}

	lineDistance := 20.0

	drawX := e.padding
	drawY := e.scrollOffset
	var drawInfo []ElementInfo
	var boundsInfo [][2]int
	for _, key := range e.orderedKeys {
		//e.renderer.DrawScaledTile(drawX, drawY, e.tileAtlas, currentIcon, iconScale, color.White)
		iconPosition := geometry.PointF{X: drawX, Y: drawY}
		tW, tH := e.renderer.MeasureString(key)
		if tW > maxWidth {
			maxWidth = tW
		}
		if tH > maxHeight {
			maxHeight = tH
		}
		textPosition := geometry.PointF{X: drawX + scaledIconSize.X + e.padding, Y: drawY + tH}
		//e.renderer.DrawTTFOnScreen(drawX+scaledIconSize.X+e.padding, drawY+tH, key, color.White)

		drawInfo = append(drawInfo, ElementInfo{
			IconPosition: iconPosition,
			TextPosition: textPosition,
		})

		boundsInfo = append(boundsInfo, [2]int{int(drawY), int(drawY + scaledIconSize.Y)})

		drawY += tH + lineDistance

	}
	e.listWidth = maxWidth + scaledIconSize.X + e.padding*3
	e.bounds = boundsInfo
	e.drawInfos = drawInfo

	atlasX := int(e.listWidth + e.padding)
	atlasSize := e.tileAtlas.GetAtlasSize().MulF(e.atlasScale)
	e.atlasBounds = geometry.NewRect(atlasX, 0, atlasX+atlasSize.X, atlasSize.Y)
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

func (e *Engine) SetMapping(mappingFileName string, mapping map[string]int32, records []recfile.Record) {
	e.iconMapping = mapping
	e.mappingFileName = mappingFileName
	var orderedKeys []string

	for k := range mapping {
		orderedKeys = append(orderedKeys, k)
	}

	slices.SortStableFunc(orderedKeys, func(i, j string) int {
		return cmp.Compare(i, j)
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
	tileSize := e.tileAtlas.GetTileSize()
	gridPos := geometry.Point{X: relativeToAtlas.X / tileSize.X, Y: relativeToAtlas.Y / tileSize.Y}
	return gridPos
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
	tileSize := e.tileAtlas.GetTileSize()
	drawPosForSelector := geometry.Point{
		X: int(float64(gridPos.X)*float64(tileSize.X)*e.atlasScale) + e.atlasBounds.Min.X,
		Y: int(float64(gridPos.Y)*float64(tileSize.Y)*e.atlasScale) + e.atlasBounds.Min.Y,
	}
	return drawPosForSelector
}

func IndexToXY(index int, width int) (int, int) {
	return index % width, index / width
}

func XYToIndex(x int, y int, width int) int {
	return y*width + x
}
