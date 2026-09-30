package main

import (
    "github.com/hajimehoshi/ebiten/v2"
    "github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (e *Engine) handleInput() bool {

    if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
        e.shouldQuit = true
    }

    if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
        e.showHelp = !e.showHelp
    }
    if e.showHelp {
        return false
    }
    e.checkDrop()
    if e.pendingDrop != nil {
        if inpututil.IsKeyJustPressed(ebiten.KeyK) {
            e.resolveDrop(false)
        } else if inpututil.IsKeyJustPressed(ebiten.KeyU) {
            e.resolveDrop(true)
        } else if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
            e.pendingDrop = nil
        }
        return true
    }
    if inpututil.IsKeyJustPressed(ebiten.KeyC) {
        e.cutMode = !e.cutMode
        e.updateTitle()
    }
    if e.cutMode {
        if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
            e.cutMode = false
            e.updateTitle()
        }
        e.handleCutKeys()
    }
    if e.typed && inpututil.IsKeyJustPressed(ebiten.KeyTab) {
        e.cycleCategory()
    }
    if e.typed && inpututil.IsKeyJustPressed(ebiten.KeyF2) {
        e.showPreview = !e.showPreview
    }
    if e.showPreview && inpututil.IsKeyJustPressed(ebiten.KeySpace) {
        e.sceneIndex++
    }
    if e.typed && inpututil.IsKeyJustPressed(ebiten.KeyDelete) && e.selectedListIndex >= 0 && e.selectedListIndex < len(e.orderedKeys) {
        e.iconMapping[e.orderedKeys[e.selectedListIndex]] = -1
        e.selectedAtlasIndex = -1
    }

    if inpututil.IsKeyJustPressed(ebiten.KeyS) {
        e.saveChanges(e.mappingFileName)
        e.saveTicks = 30
        return true
    }

    mousePosInPixelsX, mousePosInPixelsY := ebiten.CursorPosition()
    mousePosInPixelsX = int(float64(mousePosInPixelsX) / e.deviceDPIScale)
    mousePosInPixelsY = int(float64(mousePosInPixelsY) / e.deviceDPIScale)

    if e.mousePosInPixels.X != mousePosInPixelsX || e.mousePosInPixels.Y != mousePosInPixelsY {
        e.mousePosInPixels.X = mousePosInPixelsX
        e.mousePosInPixels.Y = mousePosInPixelsY
        e.OnMouseMoved(e.mousePosInPixels)
    }

    if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
        if e.showPreview {
            e.previewClick()
            return true
        }
        return e.handleMouseClick()
    }

    if keyRepeat(ebiten.KeyEqual) || keyRepeat(ebiten.KeyKPAdd) {
        e.atlasScale = min(8, e.atlasScale+0.5)
        e.updateElementBounds()
    } else if keyRepeat(ebiten.KeyMinus) || keyRepeat(ebiten.KeyKPSubtract) {
        e.atlasScale = max(0.5, e.atlasScale-0.5)
        e.updateElementBounds()
    }
    _, dy := ebiten.Wheel()
    if dy != 0 && !e.showPreview && e.mousePosInPixels.X > int(e.listWidth) {
        e.atlasScroll = max(0, e.atlasScroll-dy*16*e.atlasScale)
        e.updateElementBounds()
        e.OnMouseMoved(e.mousePosInPixels)
        return true
    }
    if dy != 0 {
        sensitity := 4.0
        e.scrollOffset += dy * sensitity
        e.updateElementBounds()
        return true
    }

    return false
}
