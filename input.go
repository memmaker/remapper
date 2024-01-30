package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func (e *Engine) handleInput() bool {

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
		return e.handleMouseClick()
	}

	_, dy := ebiten.Wheel()
	if dy != 0 {
		sensitity := 4.0
		e.scrollOffset += dy * sensitity
		e.updateElementBounds()
		return true
	}

	return false
}
