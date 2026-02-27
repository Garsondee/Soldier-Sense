package game

import (
	"image/color"
	"math"
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
	squads    []*Squad
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
	g.initSquads()
	g.randomiseProfiles()
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

func (g *Game) initSquads() {
	// One squad per team for now.
	if len(g.soldiers) > 0 {
		g.squads = append(g.squads, NewSquad(0, TeamRed, g.soldiers))
	}
	if len(g.opfor) > 0 {
		g.squads = append(g.squads, NewSquad(1, TeamBlue, g.opfor))
	}
}

// randomiseProfiles gives each soldier slightly different stats so behaviour varies.
func (g *Game) randomiseProfiles() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 42))
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		p := &s.profile
		p.Physical.FitnessBase = 0.4 + rng.Float64()*0.5 // 0.4 - 0.9
		p.Skills.Marksmanship = 0.2 + rng.Float64()*0.6  // 0.2 - 0.8
		p.Skills.Fieldcraft = 0.2 + rng.Float64()*0.6
		p.Skills.Discipline = 0.3 + rng.Float64()*0.6 // 0.3 - 0.9
		p.Psych.Experience = rng.Float64() * 0.5      // 0.0 - 0.5
		p.Psych.Morale = 0.5 + rng.Float64()*0.4      // 0.5 - 0.9
		p.Psych.Composure = 0.3 + rng.Float64()*0.5   // 0.3 - 0.8
	}
}

func (g *Game) Update() error {
	// Vision pass: each soldier scans for enemies.
	for _, s := range g.soldiers {
		s.UpdateVision(g.opfor, g.buildings)
	}
	for _, s := range g.opfor {
		s.UpdateVision(g.soldiers, g.buildings)
	}

	// Formation pass: update slot targets before soldiers decide to move.
	for _, sq := range g.squads {
		sq.UpdateFormation()
	}

	// Decision + movement pass.
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

	// Debug: formation slot ghosts.
	g.drawFormationSlots(screen)

	// Debug: draw vision cone arc for each soldier.
	g.drawVisionCones(screen, g.soldiers, color.RGBA{R: 220, G: 30, B: 30, A: 30})
	g.drawVisionCones(screen, g.opfor, color.RGBA{R: 30, G: 80, B: 220, A: 30})

	// Debug: lines to spotted contacts (replaces old omniscient LOS).
	contactColor := color.RGBA{R: 255, G: 255, B: 0, A: 100}
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		for _, c := range s.vision.KnownContacts {
			ebitenutil.DrawLine(screen, s.x, s.y, c.x, c.y, contactColor)
		}
	}
}

// drawFormationSlots renders a small ghost circle at each member's current slot target.
func (g *Game) drawFormationSlots(screen *ebiten.Image) {
	for _, sq := range g.squads {
		if sq.Leader == nil || sq.Leader.state == SoldierStateDead {
			continue
		}
		offsets := formationOffsets(sq.Formation, len(sq.Members))
		for i, m := range sq.Members {
			if i == 0 || !m.formationMember {
				continue
			}
			off := offsets[i]
			wx, wy := SlotWorld(sq.Leader.x, sq.Leader.y, sq.smoothedHeading, off[0], off[1])
			// Faint diamond: four short lines.
			d := 4.0
			var c color.RGBA
			if m.team == TeamRed {
				c = color.RGBA{R: 220, G: 60, B: 60, A: 60}
			} else {
				c = color.RGBA{R: 60, G: 100, B: 220, A: 60}
			}
			ebitenutil.DrawLine(screen, wx-d, wy, wx, wy-d, c)
			ebitenutil.DrawLine(screen, wx, wy-d, wx+d, wy, c)
			ebitenutil.DrawLine(screen, wx+d, wy, wx, wy+d, c)
			ebitenutil.DrawLine(screen, wx, wy+d, wx-d, wy, c)
			// Line from member to their slot.
			ebitenutil.DrawLine(screen, m.x, m.y, wx, wy, color.RGBA{R: 255, G: 255, B: 255, A: 18})
		}
	}
}

// drawVisionCones renders a translucent arc showing each soldier's FOV.
func (g *Game) drawVisionCones(screen *ebiten.Image, soldiers []*Soldier, c color.Color) {
	for _, s := range soldiers {
		if s.state == SoldierStateDead {
			continue
		}
		v := &s.vision
		halfFOV := v.FOV / 2.0
		coneLen := 40.0 // short debug arc length
		steps := 8
		for i := 0; i <= steps; i++ {
			a := s.vision.Heading - halfFOV + (v.FOV/float64(steps))*float64(i)
			ex := s.x + math.Cos(a)*coneLen
			ey := s.y + math.Sin(a)*coneLen
			ebitenutil.DrawLine(screen, s.x, s.y, ex, ey, c)
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
