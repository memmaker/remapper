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
        X: a.imageData.Bounds().Dx() / a.tileSizeX,
        Y: a.imageData.Bounds().Dy() / a.tileSizeY,
    }
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
