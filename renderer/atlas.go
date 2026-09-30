package renderer

import (
    "ReMapper/geometry"
    "github.com/hajimehoshi/ebiten/v2"
    "image"
    "image/color"
    _ "image/png"
    "io"
    "log"
    "os"
)

type CellDrawInfo struct {
    Icon  int32
    Color color.Color
    Atlas TextureAtlas
}
type TextureAtlas struct {
    imageData *ebiten.Image
    tileSizeX int
    tileSizeY int
    // cutting: the first cell's top left corner and the empty pixels between cells
    Offset geometry.Point
    Gap    geometry.Point
}

func (a TextureAtlas) GetTileSize() geometry.Point {
    return geometry.Point{X: a.tileSizeX, Y: a.tileSizeY}
}

func (a TextureAtlas) GetAtlasSize() geometry.Point {
    sizeX := a.imageData.Bounds().Dx()
    sizeY := a.imageData.Bounds().Dy()
    return geometry.Point{X: sizeX, Y: sizeY}
}

func (a TextureAtlas) GetImage() *ebiten.Image {
    return a.imageData
}

func (a TextureAtlas) GetCellCount() geometry.Point {
    return geometry.Point{
        X: max(1, (a.imageData.Bounds().Dx()-a.Offset.X+a.Gap.X)/(a.tileSizeX+a.Gap.X)),
        Y: max(1, (a.imageData.Bounds().Dy()-a.Offset.Y+a.Gap.Y)/(a.tileSizeY+a.Gap.Y)),
    }
}

// SetTileSize changes the cutting; sizes below one pixel are clamped.
func (a *TextureAtlas) SetTileSize(w, h int) {
    a.tileSizeX, a.tileSizeY = max(1, w), max(1, h)
}

// CellOrigin is the pixel position of grid cell (x, y) in the image.
func (a TextureAtlas) CellOrigin(cell geometry.Point) geometry.Point {
    return geometry.Point{X: a.Offset.X + cell.X*(a.tileSizeX+a.Gap.X), Y: a.Offset.Y + cell.Y*(a.tileSizeY+a.Gap.Y)}
}

// CellAt is the grid cell under an image pixel (a gap pixel counts to the cell before it).
func (a TextureAtlas) CellAt(pixel geometry.Point) geometry.Point {
    return geometry.Point{X: (pixel.X - a.Offset.X) / (a.tileSizeX + a.Gap.X), Y: (pixel.Y - a.Offset.Y) / (a.tileSizeY + a.Gap.Y)}
}

func NewTextureAtlasFromImage(img image.Image, tileSizeX, tileSizeY int) TextureAtlas {
    return TextureAtlas{imageData: ebiten.NewImageFromImage(img), tileSizeX: max(1, tileSizeX), tileSizeY: max(1, tileSizeY)}
}

func NewTextureAtlas(imageFilename string, tileSizeX, tileSizeY int) TextureAtlas {
    return TextureAtlas{
        imageData: ebiten.NewImageFromImage(mustLoadImage(imageFilename)),
        tileSizeX: tileSizeX,
        tileSizeY: tileSizeY,
    }
}
func mustOpen(filename string) io.ReadCloser {
    f, err := os.Open(filename)
    if err != nil {
        log.Fatal(err)
    }
    return f
}
func mustLoadImage(filename string) image.Image {
    img, _, err := image.Decode(mustOpen(filename))
    if err != nil {
        log.Fatal(err)
    }
    return img
}
