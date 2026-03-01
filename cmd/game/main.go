package main

import (
	"errors"
	"log"

	"github.com/Garsondee/Soldier-Sense/internal/game"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ebiten.SetWindowTitle("Soldier Sense")
	ebiten.SetFullscreen(true)
	for {
		err := ebiten.RunGame(game.New())
		switch {
		case err == nil:
			return
		case errors.Is(err, game.ErrQuit):
			return
		case errors.Is(err, game.ErrRestart):
			continue
		default:
			log.Fatal(err)
		}
	}
}
