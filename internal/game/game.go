package game

import (
	"image/color"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

type Game struct {
	width     int
	height    int
	buildings []rect
	navGrid   *NavGrid
	soldiers  []*Soldier // red friendlies
	opfor     []*Soldier // blue OpFor
}

type rect struct {
	x int
	y int
	w int
	h int
}

func New() *Game {
	g := &Game{width: 1280, height: 720}
	g.initBuildings()
	g.navGrid = NewNavGrid(g.width, g.height, g.buildings, soldierRadius)
	g.initSoldiers()
	g.initOpFor()
	return g
}

func (g *Game) initBuildings() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	unit := 64
	count := 5
	maxAttempts := 200

	margin := unit
	minWUnits, maxWUnits := 2, 6
	minHUnits, maxHUnits := 2, 6

	g.buildings = g.buildings[:0]
	for attempts := 0; len(g.buildings) < count && attempts < maxAttempts; attempts++ {
		wUnits := rng.Intn(maxWUnits-minWUnits+1) + minWUnits
		hUnits := rng.Intn(maxHUnits-minHUnits+1) + minHUnits
		w := wUnits * unit
		h := hUnits * unit

		maxX := g.width - margin - w
		maxY := g.height - margin - h
		if maxX <= margin || maxY <= margin {
			break
		}

		xCells := (maxX - margin) / unit
		yCells := (maxY - margin) / unit
		x := margin + rng.Intn(xCells+1)*unit
		y := margin + rng.Intn(yCells+1)*unit

		candidate := rect{x: x, y: y, w: w, h: h}
		if g.overlapsAny(candidate) {
			continue
		}
		g.buildings = append(g.buildings, candidate)
	}
}

func (g *Game) overlapsAny(r rect) bool {
	pad := 16
	rx0 := r.x - pad
	ry0 := r.y - pad
	rx1 := r.x + r.w + pad
	ry1 := r.y + r.h + pad

	for _, b := range g.buildings {
		bx0 := b.x
		by0 := b.y
		bx1 := b.x + b.w
		by1 := b.y + b.h
		if rx0 < bx1 && rx1 > bx0 && ry0 < by1 && ry1 > by0 {
			return true
		}
	}
	return false
}

func (g *Game) initSoldiers() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	count := 6
	margin := 32.0

	for i := 0; i < count; i++ {
		y := margin + rng.Float64()*float64(g.height-int(margin)*2)

		startX := margin
		endX := float64(g.width) - margin

		start := [2]float64{startX, y}
		end := [2]float64{endX, y}

		s := NewSoldier(startX, y, TeamRed, start, end, g.navGrid)
		if s.path != nil {
			g.soldiers = append(g.soldiers, s)
		}
	}
}

func (g *Game) initOpFor() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 999))
	count := 6
	margin := 32.0

	for i := 0; i < count; i++ {
		y := margin + rng.Float64()*float64(g.height-int(margin)*2)

		startX := float64(g.width) - margin
		endX := margin

		start := [2]float64{startX, y}
		end := [2]float64{endX, y}

		s := NewSoldier(startX, y, TeamBlue, start, end, g.navGrid)
		if s.path != nil {
			g.opfor = append(g.opfor, s)
		}
	}
}

func (g *Game) Update() error {
	for _, s := range g.soldiers {
		s.Update()
	}
	for _, s := range g.opfor {
		s.Update()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 0, G: 0, B: 0, A: 255})

	w, h := screen.Size()

	gridFine := 16
	gridMid := gridFine * 4
	gridCoarse := gridMid * 4

	drawGrid(screen, w, h, gridFine, color.RGBA{R: 22, G: 22, B: 22, A: 255})
	drawGrid(screen, w, h, gridMid, color.RGBA{R: 40, G: 40, B: 40, A: 255})
	drawGrid(screen, w, h, gridCoarse, color.RGBA{R: 70, G: 70, B: 70, A: 255})

	fill := color.RGBA{R: 120, G: 120, B: 120, A: 255}
	stroke := color.RGBA{R: 160, G: 160, B: 160, A: 255}
	for _, b := range g.buildings {
		ebitenutil.DrawRect(screen, float64(b.x), float64(b.y), float64(b.w), float64(b.h), fill)
		ebitenutil.DrawLine(screen, float64(b.x), float64(b.y), float64(b.x+b.w), float64(b.y), stroke)
		ebitenutil.DrawLine(screen, float64(b.x), float64(b.y), float64(b.x), float64(b.y+b.h), stroke)
		ebitenutil.DrawLine(screen, float64(b.x+b.w), float64(b.y), float64(b.x+b.w), float64(b.y+b.h), stroke)
		ebitenutil.DrawLine(screen, float64(b.x), float64(b.y+b.h), float64(b.x+b.w), float64(b.y+b.h), stroke)
	}

	for _, s := range g.soldiers {
		s.Draw(screen)
	}
	for _, s := range g.opfor {
		s.Draw(screen)
	}

	// Debug LOS lines: yellow line between each red-blue pair with clear sight.
	losColor := color.RGBA{R: 255, G: 255, B: 0, A: 100}
	for _, r := range g.soldiers {
		for _, b := range g.opfor {
			if HasLineOfSight(r.x, r.y, b.x, b.y, g.buildings) {
				ebitenutil.DrawLine(screen, r.x, r.y, b.x, b.y, losColor)
			}
		}
	}
}

func drawGrid(screen *ebiten.Image, w, h, spacing int, c color.Color) {
	if spacing <= 0 {
		return
	}

	for x := 0; x <= w; x += spacing {
		xf := float64(x)
		ebitenutil.DrawLine(screen, xf, 0, xf, float64(h), c)
	}

	for y := 0; y <= h; y += spacing {
		yf := float64(y)
		ebitenutil.DrawLine(screen, 0, yf, float64(w), yf, c)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.width, g.height
}
