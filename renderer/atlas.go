package renderer

import (
    "ReMapper/geometry"
    "github.com/hajimehoshi/ebiten/v2"
    "image"
    "image/color"
    "image/draw"
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

// SetImage swaps the picture and keeps the cutting.
func (a *TextureAtlas) SetImage(img image.Image) {
    a.imageData = ebiten.NewImageFromImage(img)
}

// cellRect is cell i's pixels in an image cols cells wide.
func (a TextureAtlas) cellRect(i int32, cols int) image.Rectangle {
    o := a.CellOrigin(geometry.Point{X: int(i) % cols, Y: int(i) / cols})
    return image.Rect(o.X, o.Y, o.X+a.tileSizeX, o.Y+a.tileSizeY)
}

// FreeCell is the cell after the last one with a visible pixel in img (an image cut like this atlas).
func (a TextureAtlas) FreeCell(img *image.RGBA) int32 {
    n := a.GetCellCount()
    for i := int32(n.X*n.Y) - 1; i >= 0; i-- {
        r := a.cellRect(i, n.X).Intersect(img.Bounds())
        for y := r.Min.Y; y < r.Max.Y; y++ {
            for x := r.Min.X; x < r.Max.X; x++ {
                if img.RGBAAt(x, y).A > 0 {
                    return i + 1
                }
            }
        }
    }
    return 0
}

// Compose draws cell over on top of cell base into cell at of img (an image cut like this atlas), growing img
// downwards when at lies below it. Returns the (possibly new) image.
func (a TextureAtlas) Compose(img *image.RGBA, base, over, at int32) *image.RGBA {
    cols := a.GetCellCount().X
    dst := a.cellRect(at, cols)
    if dst.Max.Y > img.Bounds().Dy() {
        grown := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), dst.Max.Y))
        draw.Draw(grown, img.Bounds(), img, image.Point{}, draw.Src)
        img = grown
    }
    draw.Draw(img, dst, img, a.cellRect(base, cols).Min, draw.Src)
    draw.Draw(img, dst, img, a.cellRect(over, cols).Min, draw.Over)
    return img
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
