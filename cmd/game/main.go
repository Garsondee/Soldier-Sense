package main

import (
	"log"

	"github.com/Garsondee/Soldier-Sense/internal/game"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ebiten.SetWindowTitle("Soldier Sense")
	ebiten.SetFullscreen(true)
	if err := ebiten.RunGame(game.New()); err != nil {
		log.Fatal(err)
	}
}
