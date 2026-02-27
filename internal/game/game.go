package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// borderWidth is the pixel gap between the window edge and the battlefield.
const borderWidth = 24

// overlayColors maps each IntelMapKind to its debug render colour.
var overlayColors = [intelMapCount]color.RGBA{
	IntelContact:          {R: 255, G: 50, B: 50, A: 180},  // bright red
	IntelRecentContact:    {R: 255, G: 140, B: 0, A: 140},  // orange
	IntelThreatDensity:    {R: 200, G: 0, B: 200, A: 120},  // purple
	IntelFriendlyPresence: {R: 30, G: 160, B: 255, A: 130}, // sky blue
	IntelDangerZone:       {R: 255, G: 220, B: 0, A: 140},  // yellow
	IntelUnexplored:       {R: 20, G: 20, B: 20, A: 160},   // dark grey
}

type Game struct {
	width      int
	height     int
	gameWidth  int // playfield width (log panel takes the rest)
	gameHeight int // playfield height (inside border)
	offX       int // pixel offset from window left to battlefield left
	offY       int // pixel offset from window top to battlefield top
	buildings  []rect
	navGrid    *NavGrid
	soldiers   []*Soldier // red friendlies
	opfor      []*Soldier // blue OpFor
	squads     []*Squad
	thoughtLog *ThoughtLog
	combat     *CombatManager
	intel      *IntelStore
	tick       int
	nextID     int

	// Overlay toggle state.
	// showOverlay[team][layer] = visible?
	showOverlay [2][intelMapCount]bool
	overlayTeam int  // 0 = red, 1 = blue (which team's maps are shown)
	showHUD     bool // toggle HUD key labels
	prevKeys    map[ebiten.Key]bool
}

type rect struct {
	x int
	y int
	w int
	h int
}

func New() *Game {
	// Battlefield is 1536x864 inside a border, giving a 1080p window with log panel.
	battleW := 1536
	battleH := 864
	g := &Game{
		width:      borderWidth + battleW + borderWidth + logPanelWidth,
		height:     borderWidth + battleH + borderWidth,
		gameWidth:  battleW,
		gameHeight: battleH,
		offX:       borderWidth,
		offY:       borderWidth,
		thoughtLog: NewThoughtLog(),
		showHUD:    true,
		prevKeys:   make(map[ebiten.Key]bool),
	}
	g.initBuildings()
	g.navGrid = NewNavGrid(g.gameWidth, g.gameHeight, g.buildings, soldierRadius)
	g.initSoldiers()
	g.initOpFor()
	g.initSquads()
	g.randomiseProfiles()
	g.combat = NewCombatManager(time.Now().UnixNano() + 7777)
	g.intel = NewIntelStore(g.gameWidth, g.gameHeight)
	return g
}

func (g *Game) initBuildings() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404 -- game only, crypto/rand not needed

	unit := 64
	count := 8
	maxAttempts := 400

	margin := unit
	minWUnits, maxWUnits := 2, 6
	minHUnits, maxHUnits := 2, 6

	g.buildings = g.buildings[:0]
	for attempts := 0; len(g.buildings) < count && attempts < maxAttempts; attempts++ {
		wUnits := rng.Intn(maxWUnits-minWUnits+1) + minWUnits
		hUnits := rng.Intn(maxHUnits-minHUnits+1) + minHUnits
		w := wUnits * unit
		h := hUnits * unit

		maxX := g.gameWidth - margin - w
		maxY := g.gameHeight - margin - h
		if maxX <= margin || maxY <= margin {
			break
		}

		xCells := (maxX - margin) / unit
		yCells := (maxY - margin) / unit
		if xCells < 0 || yCells < 0 {
			continue
		}
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
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404 -- game only, crypto/rand not needed
	count := 6
	margin := 32.0
	// Spawn as a tight cluster to avoid immediate regroup intent from excessive spread.
	centerY := float64(g.height) * 0.5
	clusterSpacing := 28.0
	startY := centerY - clusterSpacing*float64(count-1)/2

	for i := 0; i < count; i++ {
		// Add a tiny random jitter so formations aren't perfectly rigid.
		jitter := (rng.Float64() - 0.5) * 4.0
		y := startY + float64(i)*clusterSpacing + jitter
		if y < margin {
			y = margin
		}
		if y > float64(g.gameHeight)-margin {
			y = float64(g.gameHeight) - margin
		}

		startX := margin
		endX := float64(g.gameWidth) - margin

		start := [2]float64{startX, y}
		end := [2]float64{endX, y}

		id := g.nextID
		g.nextID++
		s := NewSoldier(id, startX, y, TeamRed, start, end, g.navGrid, g.thoughtLog, &g.tick)
		if s.path != nil {
			g.soldiers = append(g.soldiers, s)
		}
	}
}

func (g *Game) initOpFor() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 999)) // #nosec G404 -- game only, crypto/rand not needed
	count := 6
	margin := 32.0
	// Spawn as a tight cluster to avoid immediate regroup intent from excessive spread.
	centerY := float64(g.height) * 0.5
	clusterSpacing := 28.0
	startY := centerY - clusterSpacing*float64(count-1)/2

	for i := 0; i < count; i++ {
		jitter := (rng.Float64() - 0.5) * 4.0
		y := startY + float64(i)*clusterSpacing + jitter
		if y < margin {
			y = margin
		}
		if y > float64(g.gameHeight)-margin {
			y = float64(g.gameHeight) - margin
		}

		startX := float64(g.gameWidth) - margin
		endX := margin

		start := [2]float64{startX, y}
		end := [2]float64{endX, y}

		id := g.nextID
		g.nextID++
		s := NewSoldier(id, startX, y, TeamBlue, start, end, g.navGrid, g.thoughtLog, &g.tick)
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
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 42)) // #nosec G404 -- game only, crypto/rand not needed
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
	g.tick++

	// Handle overlay toggle input.
	g.handleInput()

	// 1. SENSE: each soldier scans for enemies.
	for _, s := range g.soldiers {
		s.UpdateVision(g.opfor, g.buildings)
	}
	for _, s := range g.opfor {
		s.UpdateVision(g.soldiers, g.buildings)
	}

	// 2. COMBAT: fire decisions and resolution.
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	g.combat.ResetFireCounts(all)
	g.combat.ResolveCombat(g.soldiers, g.opfor, g.soldiers, g.buildings)
	g.combat.ResolveCombat(g.opfor, g.soldiers, g.opfor, g.buildings)
	g.combat.UpdateTracers()

	// 2.5. INTEL: update all heatmap layers from current soldier state.
	g.intel.Update(g.soldiers, g.opfor, g.buildings)

	// 3. SQUAD THINK: leaders evaluate and set intent/orders.
	for _, sq := range g.squads {
		sq.SquadThink(g.intel)
	}

	// Formation pass: update slot targets before soldiers decide to move.
	for _, sq := range g.squads {
		sq.UpdateFormation()
	}

	// 4+5. INDIVIDUAL THINK + ACT.
	for _, s := range g.soldiers {
		s.Update()
	}
	for _, s := range g.opfor {
		s.Update()
	}
	return nil
}

// handleInput processes overlay toggle keypresses (edge-triggered).
func (g *Game) handleInput() {
	currentKeys := map[ebiten.Key]bool{}

	// Layer toggles for active team: 1-6.
	layerKeys := [intelMapCount]ebiten.Key{
		ebiten.Key1, ebiten.Key2, ebiten.Key3,
		ebiten.Key4, ebiten.Key5, ebiten.Key6,
	}
	for i, k := range layerKeys {
		currentKeys[k] = ebiten.IsKeyPressed(k)
		if currentKeys[k] && !g.prevKeys[k] {
			g.showOverlay[g.overlayTeam][i] = !g.showOverlay[g.overlayTeam][i]
		}
	}

	// Tab: switch which team's maps are displayed.
	currentKeys[ebiten.KeyTab] = ebiten.IsKeyPressed(ebiten.KeyTab)
	if currentKeys[ebiten.KeyTab] && !g.prevKeys[ebiten.KeyTab] {
		g.overlayTeam = 1 - g.overlayTeam
	}

	// H: toggle HUD key legend.
	currentKeys[ebiten.KeyH] = ebiten.IsKeyPressed(ebiten.KeyH)
	if currentKeys[ebiten.KeyH] && !g.prevKeys[ebiten.KeyH] {
		g.showHUD = !g.showHUD
	}

	g.prevKeys = currentKeys
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Window background: very dark, outside battlefield.
	screen.Fill(color.RGBA{R: 12, G: 14, B: 12, A: 255})

	// Battlefield sub-image, offset by border.
	ox, oy := float32(g.offX), float32(g.offY)
	gw, gh := float32(g.gameWidth), float32(g.gameHeight)

	// Ground fill inside battlefield.
	vector.FillRect(screen, ox, oy, gw, gh, color.RGBA{R: 28, G: 42, B: 28, A: 255}, false)

	gridFine := 16
	gridMid := gridFine * 4
	gridCoarse := gridMid * 4

	drawGridOffset(screen, g.offX, g.offY, g.gameWidth, g.gameHeight, gridFine, color.RGBA{R: 34, G: 50, B: 34, A: 255})
	drawGridOffset(screen, g.offX, g.offY, g.gameWidth, g.gameHeight, gridMid, color.RGBA{R: 42, G: 60, B: 42, A: 255})
	drawGridOffset(screen, g.offX, g.offY, g.gameWidth, g.gameHeight, gridCoarse, color.RGBA{R: 55, G: 78, B: 55, A: 255})

	// Intel heatmap overlays (drawn under buildings and soldiers).
	team := Team(g.overlayTeam)
	if im := g.intel.For(team); im != nil {
		for k := IntelMapKind(0); k < intelMapCount; k++ {
			if g.showOverlay[g.overlayTeam][k] {
				g.drawHeatLayer(screen, im.Layer(k), overlayColors[k])
			}
		}
	}

	// Buildings.
	buildFill := color.RGBA{R: 75, G: 72, B: 60, A: 255}   // warm grey-tan
	buildEdge := color.RGBA{R: 110, G: 105, B: 88, A: 255} // lighter edge
	buildShadow := color.RGBA{R: 18, G: 16, B: 12, A: 180} // drop shadow
	hatchCol := color.RGBA{R: 60, G: 57, B: 46, A: 120}    // subtle interior hatch
	for _, b := range g.buildings {
		x0 := ox + float32(b.x)
		y0 := oy + float32(b.y)
		x1 := ox + float32(b.x+b.w)
		y1 := oy + float32(b.y+b.h)
		// Drop shadow (offset 3px).
		vector.FillRect(screen, x0+3, y0+3, float32(b.w), float32(b.h), buildShadow, false)
		// Main fill.
		vector.FillRect(screen, x0, y0, float32(b.w), float32(b.h), buildFill, false)
		// Cross-hatch diagonal lines for roof texture.
		hatchStep := float32(16)
		for d := -float32(b.h); d < float32(b.w); d += hatchStep {
			lx0 := x0 + d
			ly0 := y0
			lx1 := x0 + d + float32(b.h)
			ly1 := y1
			if lx0 < x0 {
				ly0 = y0 + (x0 - lx0)
				lx0 = x0
			}
			if lx1 > x1 {
				ly1 = y1 - (lx1 - x1)
				lx1 = x1
			}
			if lx0 <= lx1 && ly0 <= ly1 {
				vector.StrokeLine(screen, lx0, ly0, lx1, ly1, 1.0, hatchCol, false)
			}
		}
		// Border.
		vector.StrokeLine(screen, x0, y0, x1, y0, 1.5, buildEdge, false)
		vector.StrokeLine(screen, x0, y0, x0, y1, 1.5, buildEdge, false)
		vector.StrokeLine(screen, x1, y0, x1, y1, 1.5, buildEdge, false)
		vector.StrokeLine(screen, x0, y1, x1, y1, 1.5, buildEdge, false)
	}

	// Vision cones (under soldiers).
	g.drawVisionCones(screen, g.soldiers, color.RGBA{R: 220, G: 30, B: 30, A: 22})
	g.drawVisionCones(screen, g.opfor, color.RGBA{R: 30, G: 80, B: 220, A: 22})

	for _, s := range g.soldiers {
		s.Draw(screen, g.offX, g.offY)
	}
	for _, s := range g.opfor {
		s.Draw(screen, g.offX, g.offY)
	}

	// Tracers (drawn above soldiers).
	g.combat.DrawTracers(screen, g.offX, g.offY)

	// Debug: formation slot ghosts.
	g.drawFormationSlots(screen)

	// Debug: lines to spotted contacts.
	contactColor := color.RGBA{R: 255, G: 255, B: 0, A: 100}
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		for _, c := range s.vision.KnownContacts {
			vector.StrokeLine(screen,
				ox+float32(s.x), oy+float32(s.y),
				ox+float32(c.x), oy+float32(c.y),
				1.0, contactColor, false)
		}
	}

	// Battlefield border frame.
	borderCol := color.RGBA{R: 80, G: 110, B: 80, A: 255}
	vector.StrokeRect(screen, ox-1, oy-1, gw+2, gh+2, 2.0, borderCol, false)
	// Outer glow line.
	vector.StrokeRect(screen, ox-3, oy-3, gw+6, gh+6, 1.0, color.RGBA{R: 50, G: 80, B: 50, A: 120}, false)

	// Thought log panel (drawn at absolute screen coords, to the right of border+battlefield).
	logX := g.offX + g.gameWidth + g.offX
	g.thoughtLog.Draw(screen, logX, g.height)

	// HUD key legend.
	if g.showHUD {
		g.drawHUD(screen)
	}
}

// drawHeatLayer renders one HeatLayer as an alpha-blended colour wash.
func (g *Game) drawHeatLayer(screen *ebiten.Image, layer *HeatLayer, baseCol color.RGBA) {
	ox, oy := float32(g.offX), float32(g.offY)
	cs := float32(cellSize)
	for row := 0; row < layer.rows; row++ {
		for col := 0; col < layer.cols; col++ {
			v := layer.At(row, col)
			if v < 0.01 {
				continue
			}
			alpha := uint8(float32(baseCol.A) * v)
			if alpha < 2 {
				continue
			}
			c := color.RGBA{
				R: baseCol.R,
				G: baseCol.G,
				B: baseCol.B,
				A: alpha,
			}
			vector.FillRect(screen,
				ox+float32(col)*cs, oy+float32(row)*cs,
				cs, cs, c, false)
		}
	}
}

// drawHUD renders keyboard shortcut hints in the bottom-left corner.
func (g *Game) drawHUD(screen *ebiten.Image) {
	teamLabel := "RED"
	if g.overlayTeam == 1 {
		teamLabel = "BLUE"
	}
	lines := []string{
		fmt.Sprintf("Intel maps: [%s team]  Tab=switch", teamLabel),
	}
	for k := IntelMapKind(0); k < intelMapCount; k++ {
		on := " "
		if g.showOverlay[g.overlayTeam][k] {
			on = "*"
		}
		lines = append(lines, fmt.Sprintf("  [%d] %s %s", k+1, on, IntelMapKindName(k)))
	}
	lines = append(lines, "  [H] toggle this HUD")

	baseX := g.offX + 6
	baseY := g.offY + g.gameHeight - (len(lines)+1)*12
	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, baseX, baseY+i*12)
	}
}

// drawFormationSlots renders a small ghost circle at each member's current slot target.
func (g *Game) drawFormationSlots(screen *ebiten.Image) {
	ox, oy := float32(g.offX), float32(g.offY)
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
			d := float32(4.0)
			var c color.RGBA
			if m.team == TeamRed {
				c = color.RGBA{R: 220, G: 60, B: 60, A: 60}
			} else {
				c = color.RGBA{R: 60, G: 100, B: 220, A: 60}
			}
			swx, swy := ox+float32(wx), oy+float32(wy)
			vector.StrokeLine(screen, swx-d, swy, swx, swy-d, 1.0, c, false)
			vector.StrokeLine(screen, swx, swy-d, swx+d, swy, 1.0, c, false)
			vector.StrokeLine(screen, swx+d, swy, swx, swy+d, 1.0, c, false)
			vector.StrokeLine(screen, swx, swy+d, swx-d, swy, 1.0, c, false)
			// Line from member to their slot.
			vector.StrokeLine(screen, ox+float32(m.x), oy+float32(m.y), swx, swy, 1.0, color.RGBA{R: 255, G: 255, B: 255, A: 18}, false)
		}
	}
}

// drawVisionCones renders a subtle gradient-filled fan showing each soldier's FOV.
// The fan is clipped by building occlusion via ray-vs-AABB intersection.
func (g *Game) drawVisionCones(screen *ebiten.Image, soldiers []*Soldier, c color.Color) {
	rc, _ := c.(color.RGBA)
	ox, oy := float32(g.offX), float32(g.offY)
	for _, s := range soldiers {
		if s.state == SoldierStateDead {
			continue
		}
		v := &s.vision
		halfFOV := v.FOV / 2.0
		coneLen := v.MaxRange * 0.6 // 60% of vision range
		if coneLen < 60 {
			coneLen = 60
		}
		const steps = 16
		sx, sy := ox+float32(s.x), oy+float32(s.y)
		// Fill the fan with small triangles from origin to arc.
		for i := 0; i < steps; i++ {
			a0 := s.vision.Heading - halfFOV + (v.FOV/float64(steps))*float64(i)
			a1 := s.vision.Heading - halfFOV + (v.FOV/float64(steps))*float64(i+1)
			p0x := ox + float32(s.x+math.Cos(a0)*coneLen)
			p0y := oy + float32(s.y+math.Sin(a0)*coneLen)
			p1x := ox + float32(s.x+math.Cos(a1)*coneLen)
			p1y := oy + float32(s.y+math.Sin(a1)*coneLen)
			vector.StrokeLine(screen, sx, sy, p0x, p0y, 1.0, rc, false)
			vector.StrokeLine(screen, p0x, p0y, p1x, p1y, 1.0, rc, false)
		}
		// Final closing ray.
		a := s.vision.Heading + halfFOV
		ex := ox + float32(s.x+math.Cos(a)*coneLen)
		ey := oy + float32(s.y+math.Sin(a)*coneLen)
		vector.StrokeLine(screen, sx, sy, ex, ey, 1.0, rc, false)
	}
}

func drawGridOffset(screen *ebiten.Image, offX, offY, w, h, spacing int, c color.Color) {
	if spacing <= 0 {
		return
	}
	ox, oy := float32(offX), float32(offY)
	for x := 0; x <= w; x += spacing {
		xf := ox + float32(x)
		vector.StrokeLine(screen, xf, oy, xf, oy+float32(h), 1.0, c, false)
	}
	for y := 0; y <= h; y += spacing {
		yf := oy + float32(y)
		vector.StrokeLine(screen, ox, yf, ox+float32(w), yf, 1.0, c, false)
	}
}

func (g *Game) Layout(_, _ int) (int, int) {
	return g.width, g.height
}

// GameWidth returns the playfield width (excluding log panel).
func (g *Game) GameWidth() int {
	return g.gameWidth
}
