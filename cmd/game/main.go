package main

import (
	"log"

	"github.com/Garsondee/Soldier-Sense/internal/game"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	ebiten.SetWindowTitle("Soldier Sense")
	ebiten.SetWindowSize(1280, 720)
	if err := ebiten.RunGame(game.New()); err != nil {
		log.Fatal(err)
	}
}
