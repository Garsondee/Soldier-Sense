package game

import (
	"errors"
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

// hudScale is the integer upscale factor applied to all HUD text (3 = 3× larger).
const hudScale = 3

const (
	menuOptionQuit = iota
	menuOptionRestart
	menuOptionCount
)

var (
	// ErrQuit cleanly exits the whole program when returned from Game.Update.
	ErrQuit = errors.New("quit game")
	// ErrRestart requests a fresh simulation instance with a new seed.
	ErrRestart = errors.New("restart game")
)

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
	width              int
	height             int
	gameWidth          int            // playfield width (log panel takes the rest)
	gameHeight         int            // playfield height (inside border)
	offX               int            // pixel offset from window left to battlefield left
	offY               int            // pixel offset from window top to battlefield top
	buildings          []rect         // individual wall segments (1-cell wide), used for LOS/nav
	windows            []rect         // window segments: block movement, transparent to LOS
	buildingFootprints []rect         // overall floor area of each structure, used for rendering
	roads              []roadSegment  // road layout (decomposed segments for collision)
	roadPolylines      []roadPolyline // smooth centrelines for rendering
	covers             []*CoverObject
	navGrid            *NavGrid
	soldiers           []*Soldier // red friendlies
	opfor              []*Soldier // blue OpFor
	squads             []*Squad
	thoughtLog         *ThoughtLog
	combat             *CombatManager
	intel              *IntelStore
	tacticalMap        *TacticalMap
	tick               int
	nextID             int

	// Overlay toggle state.
	// showOverlay[team][layer] = visible?
	showOverlay [2][intelMapCount]bool
	overlayTeam int  // 0 = red, 1 = blue (which team's maps are shown)
	showHUD     bool // toggle HUD key labels
	prevKeys    map[ebiten.Key]bool

	// Offscreen buffer for vision cone rendering (avoids additive blowout).
	visionBuf *ebiten.Image
	// Offscreen buffer for the full battlefield — camera transform applied on blit.
	worldBuf *ebiten.Image
	// Offscreen buffer for HUD text — rendered at 1x then blitted at hudScale.
	hudBuf *ebiten.Image
	// Offscreen buffer for the thought log panel — rendered at 1x then blitted at logScale.
	logBuf *ebiten.Image
	// Offscreen buffer for the inspector panel — rendered at 1x then blitted at inspScale.
	inspBuf *ebiten.Image

	// Deterministic terrain noise patches, generated once.
	terrainPatches []terrainPatch

	// Camera pan + zoom.
	camX    float64 // world-space X of the camera centre
	camY    float64 // world-space Y of the camera centre
	camZoom float64 // zoom factor (1.0 = native, >1 = zoomed in)

	// Soldier speech bubbles.
	speechBubbles []*SpeechBubble
	speechRng     *rand.Rand

	// Soldier inspector (click-to-select panel).
	inspector     Inspector
	prevMouseLeft bool // for edge-triggered click detection

	// Simulation speed control.
	simSpeed  float64 // multiplier: 0=paused, 0.5, 1, 2, 4
	tickAccum float64 // fractional tick accumulator for sub-1x speeds

	// ESC menu state.
	menuOpen        bool
	menuSelection   int
	menuResumeSpeed float64
	pendingExit     error

	// Analytics reporter — collects behaviour stats periodically.
	reporter *SimReporter

	// Master map seed — printed at startup so layouts can be reproduced.
	mapSeed int64
}

type rect struct {
	x int
	y int
	w int
	h int
}

// terrainPatch is a subtle ground colour variation tile.
type terrainPatch struct {
	x, y  float32
	w, h  float32
	shade uint8 // offset from base green
}

func clampToByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// terrainHash returns a deterministic pseudo-random value for a cell pair.
func terrainHash(x, y int) uint32 {
	if x < 0 {
		x = -x
	}
	if y < 0 {
		y = -y
	}
	ux := uint64(x) // #nosec G115 -- x is non-negative and bounded (cell coords)
	uy := uint64(y) // #nosec G115 -- y is non-negative and bounded (cell coords)
	v64 := (ux * 73856093) ^ (uy * 19349663)
	v := uint32(v64 & 0xffffffff) // #nosec G115 -- masked to 32 bits before cast; no overflow possible
	v ^= v >> 13
	v *= 1274126177
	v ^= v >> 16
	return v
}

func New() *Game {
	// Battlefield is 3072x1728 — double the original size.
	battleW := 3072
	battleH := 1728

	// Master map seed — random each game, printed to console for reproducibility.
	mapSeed := time.Now().UnixNano()
	fmt.Printf("MAP SEED: %d\n", mapSeed)

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
		mapSeed:    mapSeed,
	}
	mapRng := rand.New(rand.NewSource(mapSeed)) // #nosec G404 -- game only
	g.initRoads(mapRng)
	g.initBuildings(mapRng)
	g.initCover()
	g.navGrid = NewNavGrid(g.gameWidth, g.gameHeight, g.buildings, soldierRadius, g.covers, g.windows)
	g.tacticalMap = NewTacticalMap(g.gameWidth, g.gameHeight, g.buildings, g.windows, g.buildingFootprints)
	g.initSoldiers()
	g.initOpFor()
	g.initSquads()
	g.randomiseProfiles()
	g.combat = NewCombatManager(time.Now().UnixNano() + 7777)
	g.intel = NewIntelStore(g.gameWidth, g.gameHeight)
	g.visionBuf = ebiten.NewImage(battleW, battleH)
	g.worldBuf = ebiten.NewImage(battleW, battleH)
	// HUD buffer: 1/hudScale of screen so it renders crisply when scaled up.
	g.hudBuf = ebiten.NewImage(g.width/hudScale, g.height/hudScale)
	// Log buffer: 1/logScale of the log panel area.
	g.logBuf = ebiten.NewImage(logPanelWidth/logScale, g.height/logScale)
	// Inspector buffer: 1/inspScale of the inspector panel area.
	g.inspBuf = ebiten.NewImage(inspBufW, inspBufH)
	g.initTerrainPatches()
	// Default camera: centred on battlefield, zoom 0.5 so the full map is visible.
	g.camX = float64(battleW) / 2
	g.camY = float64(battleH) / 2
	g.camZoom = 0.5
	g.simSpeed = 1.0
	g.speechRng = rand.New(rand.NewSource(time.Now().UnixNano() + 9999)) // #nosec G404 -- non-crypto RNG for local flavor text
	g.reporter = NewSimReporter(reportWindowTicks, false)
	return g
}

// initTerrainPatches generates deterministic subtle ground colour patches.
func (g *Game) initTerrainPatches() {
	rng := rand.New(rand.NewSource(54321)) // #nosec G404 -- cosmetic only
	count := 600                           // more patches for the larger map
	g.terrainPatches = make([]terrainPatch, 0, count)
	for i := 0; i < count; i++ {
		w := float32(24 + rng.Intn(80))
		h := float32(24 + rng.Intn(80))
		x := float32(rng.Intn(g.gameWidth))
		y := float32(rng.Intn(g.gameHeight))
		// shade offset: -6 to +6 from base green
		shade := clampToByte(rng.Intn(13))
		g.terrainPatches = append(g.terrainPatches, terrainPatch{x: x, y: y, w: w, h: h, shade: shade})
	}
}

func (g *Game) initCover() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 12345)) // #nosec G404 -- game only
	var rubble []*CoverObject
	g.covers, rubble = GenerateCover(g.gameWidth, g.gameHeight, g.buildingFootprints, g.buildings, rng, g.roads)
	// Rubble replaces wall segments where explosions hit — remove those walls and add rubble.
	g.applyBuildingDamage(rubble)
}

// applyBuildingDamage removes wall segments that overlap rubble zones and appends
// the rubble pieces to g.covers. This simulates pre-battle artillery damage:
// holes are blown in building walls and rubble scatters across the interior.
func (g *Game) applyBuildingDamage(rubble []*CoverObject) {
	cs := coverCellSize
	// Build a set of rubble cells for fast lookup.
	type cellKey struct{ cx, cy int }
	rubbleSet := make(map[cellKey]bool, len(rubble))
	for _, r := range rubble {
		rubbleSet[cellKey{r.x / cs, r.y / cs}] = true
	}
	// Remove wall segments that are covered by rubble.
	kept := g.buildings[:0]
	for _, w := range g.buildings {
		k := cellKey{w.x / cs, w.y / cs}
		if !rubbleSet[k] {
			kept = append(kept, w)
		}
	}
	g.buildings = kept
	// Add rubble to the cover list.
	g.covers = append(g.covers, rubble...)
}

func (g *Game) initBuildings(rng *rand.Rand) {
	wall := cellSize // 16px
	unit := 64
	targetCount := 32 // more buildings

	g.buildings = g.buildings[:0]
	g.buildingFootprints = g.buildingFootprints[:0]

	// Weighted size pool — larger buildings are more common.
	// Each entry: {wUnits, hUnits, weight} where weight controls how many
	// candidates are generated (higher = more likely to appear).
	type sizeEntry struct {
		w, h   int
		weight int
	}
	sizes := []sizeEntry{
		// Small buildings (uncommon).
		{3, 3, 1}, {3, 4, 1}, {4, 3, 1},
		// Medium buildings.
		{4, 4, 2}, {4, 5, 2}, {5, 4, 2}, {5, 5, 3},
		// Large buildings (common).
		{5, 6, 3}, {6, 5, 3}, {6, 6, 4}, {6, 7, 3}, {7, 6, 3},
		// Very large buildings.
		{7, 7, 3}, {7, 8, 2}, {8, 7, 2}, {8, 8, 2},
	}

	var candidates []rect
	for _, sz := range sizes {
		for rep := 0; rep < sz.weight; rep++ {
			c := g.buildingCandidatesAlongRoads(rng, sz.w*unit, sz.h*unit, unit/2, unit*3)
			candidates = append(candidates, c...)
		}
	}
	// Shuffle the combined pool.
	rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	// Variable separation between buildings.
	for _, candidate := range candidates {
		if len(g.buildingFootprints) >= targetCount {
			break
		}
		if g.overlapsAnyBuilding(candidate, rng) {
			continue
		}
		if g.rectOverlapsRoad(candidate) {
			continue
		}
		g.buildingFootprints = append(g.buildingFootprints, candidate)
		g.addBuildingWalls(rng, candidate, wall, unit)
	}
}

// addBuildingWalls generates wall segments for a building footprint.
// It places perimeter walls with doorways and windows, plus recursive
// internal room subdivision. Windows are evenly spaced along each face,
// proportional to the face length. Only 1-2 faces get exterior doorways.
func (g *Game) addBuildingWalls(rng *rand.Rand, fp rect, wall, unit int) {
	x, y, w, h := fp.x, fp.y, fp.w, fp.h
	wUnits := w / unit
	hUnits := h / unit

	if wUnits < 3 {
		wUnits = 3
	}
	if hUnits < 3 {
		hUnits = 3
	}

	// --- Choose which faces get exterior doorways (1-2 doors, not all 4). ---
	type face int
	const (
		faceN face = iota
		faceS
		faceE
		faceW
	)
	faces := []face{faceN, faceS, faceE, faceW}
	rng.Shuffle(len(faces), func(i, j int) { faces[i], faces[j] = faces[j], faces[i] })
	numDoors := 1
	if wUnits >= 5 || hUnits >= 5 {
		numDoors = 2
	}
	if rng.Float64() < 0.3 {
		numDoors++ // occasional 3-door building
	}
	if numDoors > 4 {
		numDoors = 4
	}
	doorFaces := make(map[face]bool)
	for i := 0; i < numDoors && i < len(faces); i++ {
		doorFaces[faces[i]] = true
	}

	// --- Compute door positions for selected faces. ---
	doorPositions := make(map[face]int) // unit-aligned position of doorway
	if doorFaces[faceN] || doorFaces[faceS] {
		for _, f := range []face{faceN, faceS} {
			if doorFaces[f] {
				doorPositions[f] = x + (rng.Intn(wUnits-2)+1)*unit
			}
		}
	}
	if doorFaces[faceE] || doorFaces[faceW] {
		for _, f := range []face{faceE, faceW} {
			if doorFaces[f] {
				doorPositions[f] = y + (rng.Intn(hUnits-2)+1)*unit
			}
		}
	}

	// --- Window placement: evenly spaced, ~1 window per 2 units of wall. ---
	// Returns the set of unit-positions that are windows on a given face.
	windowPositions := func(faceStart, faceLen int, doorPos int, hasDoor bool) map[int]bool {
		faceUnits := faceLen / unit
		if faceUnits < 3 {
			return nil
		}
		// Place windows every 2 units, starting from unit 1, skipping corners.
		wins := make(map[int]bool)
		for u := 1; u < faceUnits-1; u++ {
			pos := faceStart + u*unit
			if hasDoor && pos == doorPos {
				continue
			}
			// Place a window roughly every 2 units (skip some for realism).
			if u%2 == 1 {
				wins[pos] = true
			}
		}
		return wins
	}

	winN := windowPositions(x, w, doorPositions[faceN], doorFaces[faceN])
	winS := windowPositions(x, w, doorPositions[faceS], doorFaces[faceS])
	winW := windowPositions(y, h, doorPositions[faceW], doorFaces[faceW])
	winE := windowPositions(y, h, doorPositions[faceE], doorFaces[faceE])

	isCornerH := func(wx, faceX, faceW int) bool {
		return wx < faceX+unit || wx >= faceX+faceW-unit
	}
	isCornerV := func(wy, faceY, faceH int) bool {
		return wy < faceY+unit || wy >= faceY+faceH-unit
	}

	// --- Place perimeter walls. ---
	// North wall (top).
	for wx := x; wx < x+w; wx += wall {
		if doorFaces[faceN] && wx >= doorPositions[faceN] && wx < doorPositions[faceN]+unit {
			continue
		}
		r := rect{x: wx, y: y, w: wall, h: wall}
		isWin := false
		for wpos := range winN {
			if wx >= wpos && wx < wpos+unit && !isCornerH(wx, x, w) {
				isWin = true
				break
			}
		}
		if isWin {
			g.windows = append(g.windows, r)
		} else {
			g.buildings = append(g.buildings, r)
		}
	}
	// South wall (bottom).
	for wx := x; wx < x+w; wx += wall {
		if doorFaces[faceS] && wx >= doorPositions[faceS] && wx < doorPositions[faceS]+unit {
			continue
		}
		r := rect{x: wx, y: y + h - wall, w: wall, h: wall}
		isWin := false
		for wpos := range winS {
			if wx >= wpos && wx < wpos+unit && !isCornerH(wx, x, w) {
				isWin = true
				break
			}
		}
		if isWin {
			g.windows = append(g.windows, r)
		} else {
			g.buildings = append(g.buildings, r)
		}
	}
	// West wall (left), skip corners.
	for wy := y + wall; wy < y+h-wall; wy += wall {
		if doorFaces[faceW] && wy >= doorPositions[faceW] && wy < doorPositions[faceW]+unit {
			continue
		}
		r := rect{x: x, y: wy, w: wall, h: wall}
		isWin := false
		for wpos := range winW {
			if wy >= wpos && wy < wpos+unit && !isCornerV(wy, y, h) {
				isWin = true
				break
			}
		}
		if isWin {
			g.windows = append(g.windows, r)
		} else {
			g.buildings = append(g.buildings, r)
		}
	}
	// East wall (right), skip corners.
	for wy := y + wall; wy < y+h-wall; wy += wall {
		if doorFaces[faceE] && wy >= doorPositions[faceE] && wy < doorPositions[faceE]+unit {
			continue
		}
		r := rect{x: x + w - wall, y: wy, w: wall, h: wall}
		isWin := false
		for wpos := range winE {
			if wy >= wpos && wy < wpos+unit && !isCornerV(wy, y, h) {
				isWin = true
				break
			}
		}
		if isWin {
			g.windows = append(g.windows, r)
		} else {
			g.buildings = append(g.buildings, r)
		}
	}

	// --- Recursive internal room subdivision (BSP-style). ---
	// Split the interior into rooms. Each room is at least 2×2 units.
	// Partition walls have doorways.
	type room struct{ rx, ry, rw, rh int }
	interior := room{x + unit, y + unit, w - 2*unit, h - 2*unit}
	var subdivide func(rm room, depth int)
	subdivide = func(rm room, depth int) {
		rmWU := rm.rw / unit
		rmHU := rm.rh / unit
		// Stop if room is too small to split (min 3 units on the split axis).
		if rmWU < 4 && rmHU < 4 {
			return
		}
		// Stop probabilistically at deeper levels.
		if depth > 0 && rng.Float64() < 0.15 {
			return
		}
		// Choose split axis: prefer splitting the longer dimension.
		splitH := rmWU > rmHU // true = vertical partition (split horizontally)
		if rmWU == rmHU {
			splitH = rng.Intn(2) == 0
		}
		// If one axis is too short, force the other.
		if rmWU < 4 {
			splitH = false
		}
		if rmHU < 4 {
			splitH = true
		}

		if splitH && rmWU >= 4 {
			// Vertical partition at a random x within the room.
			minU := 2
			maxU := rmWU - 2
			if maxU <= minU {
				return
			}
			splitU := minU + rng.Intn(maxU-minU)
			px := rm.rx + splitU*unit
			// Doorway in this partition.
			doorU := rng.Intn(rmHU)
			doorY := rm.ry + doorU*unit
			for wy := rm.ry; wy < rm.ry+rm.rh; wy += wall {
				if wy >= doorY && wy < doorY+unit {
					continue
				}
				g.buildings = append(g.buildings, rect{x: px, y: wy, w: wall, h: wall})
			}
			// Recurse into the two sub-rooms.
			leftW := splitU * unit
			rightW := rm.rw - splitU*unit - wall
			if leftW >= 2*unit {
				subdivide(room{rm.rx, rm.ry, leftW, rm.rh}, depth+1)
			}
			if rightW >= 2*unit {
				subdivide(room{px + wall, rm.ry, rightW, rm.rh}, depth+1)
			}
		} else if rmHU >= 4 {
			// Horizontal partition at a random y within the room.
			minU := 2
			maxU := rmHU - 2
			if maxU <= minU {
				return
			}
			splitU := minU + rng.Intn(maxU-minU)
			py := rm.ry + splitU*unit
			// Doorway in this partition.
			doorU := rng.Intn(rmWU)
			doorX := rm.rx + doorU*unit
			for wx := rm.rx; wx < rm.rx+rm.rw; wx += wall {
				if wx >= doorX && wx < doorX+unit {
					continue
				}
				g.buildings = append(g.buildings, rect{x: wx, y: py, w: wall, h: wall})
			}
			// Recurse into the two sub-rooms.
			topH := splitU * unit
			bottomH := rm.rh - splitU*unit - wall
			if topH >= 2*unit {
				subdivide(room{rm.rx, rm.ry, rm.rw, topH}, depth+1)
			}
			if bottomH >= 2*unit {
				subdivide(room{rm.rx, py + wall, rm.rw, bottomH}, depth+1)
			}
		}
	}

	// Only subdivide buildings that are big enough (>=4 units on both axes).
	if wUnits >= 4 && hUnits >= 4 {
		subdivide(interior, 0)
	} else if wUnits >= 4 || hUnits >= 4 {
		// Simple single partition for medium buildings.
		if rng.Intn(3) > 0 { // 66% chance
			if wUnits >= 4 && rng.Intn(2) == 0 {
				px := x + (rng.Intn(wUnits-2)+1)*unit
				doorY := y + (rng.Intn(hUnits-2)+1)*unit
				for wy := y + wall; wy < y+h-wall; wy += wall {
					if wy >= doorY && wy < doorY+unit {
						continue
					}
					g.buildings = append(g.buildings, rect{x: px, y: wy, w: wall, h: wall})
				}
			} else if hUnits >= 4 {
				py := y + (rng.Intn(hUnits-2)+1)*unit
				doorX := x + (rng.Intn(wUnits-2)+1)*unit
				for wx := x + wall; wx < x+w-wall; wx += wall {
					if wx >= doorX && wx < doorX+unit {
						continue
					}
					g.buildings = append(g.buildings, rect{x: wx, y: py, w: wall, h: wall})
				}
			}
		}
	}
}

// overlapsAnyBuilding checks if the candidate rect overlaps any existing
// building footprint, using a variable padding for organic spacing.
// Larger gaps appear randomly so that open plazas form naturally.
func (g *Game) overlapsAnyBuilding(r rect, rng *rand.Rand) bool {
	// Variable gap: most buildings get a small gap, some get a large one (plaza).
	pad := 24 + rng.Intn(64) // 24-88px gap
	rx0 := r.x - pad
	ry0 := r.y - pad
	rx1 := r.x + r.w + pad
	ry1 := r.y + r.h + pad

	for _, b := range g.buildingFootprints {
		if rx0 < b.x+b.w && rx1 > b.x && ry0 < b.y+b.h && ry1 > b.y {
			return true
		}
	}
	return false
}

// spawnCluster creates squadSize soldiers of the given team at startX,
// spread vertically around clusterCenterY. Returns the created soldiers.
func (g *Game) spawnCluster(rng *rand.Rand, team Team, squadSize int, clusterCenterY, startX, endX float64) []*Soldier {
	margin := 64.0
	spacing := 18.0 // tighter — squad spawns as a compact cluster, not a long line
	out := make([]*Soldier, 0, squadSize)
	sy := clusterCenterY - spacing*float64(squadSize-1)/2
	for i := 0; i < squadSize; i++ {
		jitter := (rng.Float64() - 0.5) * 3.0 // reduced jitter
		y := sy + float64(i)*spacing + jitter
		if y < margin {
			y = margin
		}
		if y > float64(g.gameHeight)-margin {
			y = float64(g.gameHeight) - margin
		}
		id := g.nextID
		g.nextID++
		s := NewSoldier(id, startX, y, team,
			[2]float64{startX, y}, [2]float64{endX, y},
			g.navGrid, g.covers, g.buildings, g.thoughtLog, &g.tick, g.tacticalMap)
		s.buildingFootprints = g.buildingFootprints
		s.blackboard.ClaimedBuildingIdx = -1
		if s.path != nil {
			out = append(out, s)
		}
	}
	return out
}

func (g *Game) initSoldiers() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404
	sqSz := 8
	margin := 64.0
	startX := margin
	endX := float64(g.gameWidth) - margin
	g.soldiers = append(g.soldiers, g.spawnCluster(rng, TeamRed, sqSz, float64(g.gameHeight)*0.28, startX, endX)...)
	g.soldiers = append(g.soldiers, g.spawnCluster(rng, TeamRed, sqSz, float64(g.gameHeight)*0.72, startX, endX)...)
}

func (g *Game) initOpFor() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 999)) // #nosec G404
	sqSz := 8
	margin := 64.0
	startX := float64(g.gameWidth) - margin
	endX := margin
	g.opfor = append(g.opfor, g.spawnCluster(rng, TeamBlue, sqSz, float64(g.gameHeight)*0.28, startX, endX)...)
	g.opfor = append(g.opfor, g.spawnCluster(rng, TeamBlue, sqSz, float64(g.gameHeight)*0.72, startX, endX)...)
}

func (g *Game) initSquads() {
	sqSz := 8
	for i := 0; i < len(g.soldiers); i += sqSz {
		end := i + sqSz
		if end > len(g.soldiers) {
			end = len(g.soldiers)
		}
		sq := NewSquad(len(g.squads), TeamRed, g.soldiers[i:end])
		sq.buildingFootprints = g.buildingFootprints
		g.squads = append(g.squads, sq)
	}
	for i := 0; i < len(g.opfor); i += sqSz {
		end := i + sqSz
		if end > len(g.opfor) {
			end = len(g.opfor)
		}
		sq := NewSquad(len(g.squads), TeamBlue, g.opfor[i:end])
		sq.buildingFootprints = g.buildingFootprints
		g.squads = append(g.squads, sq)
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

		// Initialise commitment-based decision thresholds from discipline.
		s.blackboard.InitCommitment(p.Skills.Discipline)
	}
}

func (g *Game) Update() error {
	// Handle input every frame regardless of sim speed.
	g.handleInput()
	if g.pendingExit != nil {
		return g.pendingExit
	}

	if g.simSpeed <= 0 {
		// Paused: still update tracers so muzzle flashes fade.
		g.combat.UpdateTracers()
		return nil
	}

	// For speeds > 1 run multiple sim ticks per frame.
	// For speeds < 1 accumulate fractions.
	g.tickAccum += g.simSpeed
	for g.tickAccum >= 1.0 {
		g.tickAccum -= 1.0
		g.simTick()
	}
	return nil
}

// simTick runs one simulation tick.
func (g *Game) simTick() {
	g.tick++

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
	g.combat.ResolveCombat(g.soldiers, g.opfor, g.soldiers, g.buildings, all)
	g.combat.ResolveCombat(g.opfor, g.soldiers, g.opfor, g.buildings, all)
	g.combat.UpdateTracers()

	// 2.1. SOUND: broadcast gunfire events to enemy soldiers (infinite range).
	g.combat.BroadcastGunfire(g.soldiers, g.opfor, g.tick)

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

	// 6. SQUAD POLL: periodic summary to thought log (~every 5s).
	if g.tick%squadPollInterval == 0 {
		for _, sq := range g.squads {
			g.thoughtLog.AddSquadPoll(g.tick, sq)
		}
	}

	// 7. SPEECH: update soldier speech bubbles.
	g.UpdateSpeech(g.speechRng)

	// 8. ANALYTICS: collect behaviour report every ~1s.
	if g.tick%60 == 0 && g.reporter != nil {
		g.reporter.Collect(g.tick, g.soldiers, g.opfor, g.squads)
	}
}

// handleInput processes overlay toggle keypresses (edge-triggered).
func (g *Game) handleInput() {
	currentKeys := map[ebiten.Key]bool{}
	currentKeys[ebiten.KeyEscape] = ebiten.IsKeyPressed(ebiten.KeyEscape)
	if currentKeys[ebiten.KeyEscape] && !g.prevKeys[ebiten.KeyEscape] {
		if g.menuOpen {
			g.menuOpen = false
			g.simSpeed = g.menuResumeSpeed
		} else {
			g.menuOpen = true
			g.menuSelection = menuOptionQuit
			g.menuResumeSpeed = g.simSpeed
			g.simSpeed = 0
		}
	}

	if g.menuOpen {
		currentKeys[ebiten.KeyArrowUp] = ebiten.IsKeyPressed(ebiten.KeyArrowUp)
		currentKeys[ebiten.KeyArrowDown] = ebiten.IsKeyPressed(ebiten.KeyArrowDown)
		currentKeys[ebiten.KeyW] = ebiten.IsKeyPressed(ebiten.KeyW)
		currentKeys[ebiten.KeyS] = ebiten.IsKeyPressed(ebiten.KeyS)
		currentKeys[ebiten.KeyEnter] = ebiten.IsKeyPressed(ebiten.KeyEnter)

		moveUp := (currentKeys[ebiten.KeyArrowUp] && !g.prevKeys[ebiten.KeyArrowUp]) ||
			(currentKeys[ebiten.KeyW] && !g.prevKeys[ebiten.KeyW])
		moveDown := (currentKeys[ebiten.KeyArrowDown] && !g.prevKeys[ebiten.KeyArrowDown]) ||
			(currentKeys[ebiten.KeyS] && !g.prevKeys[ebiten.KeyS])
		if moveUp {
			g.menuSelection = (g.menuSelection + menuOptionCount - 1) % menuOptionCount
		}
		if moveDown {
			g.menuSelection = (g.menuSelection + 1) % menuOptionCount
		}

		if currentKeys[ebiten.KeyEnter] && !g.prevKeys[ebiten.KeyEnter] {
			switch g.menuSelection {
			case menuOptionQuit:
				g.pendingExit = ErrQuit
			case menuOptionRestart:
				g.pendingExit = ErrRestart
			}
		}

		g.prevKeys = currentKeys
		g.prevMouseLeft = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		return
	}

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

	// Camera pan: WASD or arrow keys.
	panSpeed := 6.0 / g.camZoom // pan slower when zoomed in
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		g.camY -= panSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.camY += panSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.camX -= panSpeed
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.camX += panSpeed
	}

	// Camera zoom: mouse wheel or =/- keys.
	const zoomMin, zoomMax = 0.5, 4.0
	_, wy := ebiten.Wheel()
	if wy != 0 {
		g.camZoom *= math.Pow(1.12, wy)
	}
	currentKeys[ebiten.KeyEqual] = ebiten.IsKeyPressed(ebiten.KeyEqual)
	if currentKeys[ebiten.KeyEqual] && !g.prevKeys[ebiten.KeyEqual] {
		g.camZoom *= 1.25
	}
	currentKeys[ebiten.KeyMinus] = ebiten.IsKeyPressed(ebiten.KeyMinus)
	if currentKeys[ebiten.KeyMinus] && !g.prevKeys[ebiten.KeyMinus] {
		g.camZoom /= 1.25
	}
	if g.camZoom < zoomMin {
		g.camZoom = zoomMin
	}
	if g.camZoom > zoomMax {
		g.camZoom = zoomMax
	}

	// Clamp camera centre to battlefield bounds (accounting for zoom).
	halfVW := float64(g.gameWidth) / 2 / g.camZoom
	halfVH := float64(g.gameHeight) / 2 / g.camZoom
	if g.camX < halfVW {
		g.camX = halfVW
	}
	if g.camX > float64(g.gameWidth)-halfVW {
		g.camX = float64(g.gameWidth) - halfVW
	}
	if g.camY < halfVH {
		g.camY = halfVH
	}
	if g.camY > float64(g.gameHeight)-halfVH {
		g.camY = float64(g.gameHeight) - halfVH
	}

	// Sim speed controls: P=pause/resume, ,=slower, .=faster.
	speeds := []float64{0, 0.5, 1, 2, 4}
	currentKeys[ebiten.KeyP] = ebiten.IsKeyPressed(ebiten.KeyP)
	if currentKeys[ebiten.KeyP] && !g.prevKeys[ebiten.KeyP] {
		if g.simSpeed > 0 {
			g.simSpeed = 0
		} else {
			g.simSpeed = 1
		}
	}
	currentKeys[ebiten.KeyComma] = ebiten.IsKeyPressed(ebiten.KeyComma)
	if currentKeys[ebiten.KeyComma] && !g.prevKeys[ebiten.KeyComma] {
		idx := 0
		for i := 0; i < len(speeds); i++ {
			if speeds[i] == g.simSpeed {
				idx = i
				break
			}
		}
		if idx > 0 {
			g.simSpeed = speeds[idx-1]
		}
	}
	currentKeys[ebiten.KeyPeriod] = ebiten.IsKeyPressed(ebiten.KeyPeriod)
	if currentKeys[ebiten.KeyPeriod] && !g.prevKeys[ebiten.KeyPeriod] {
		for i, s := range speeds {
			if s <= g.simSpeed && i < len(speeds)-1 {
				if speeds[i+1] > g.simSpeed {
					g.simSpeed = speeds[i+1]
					break
				}
			}
		}
	}

	// Left mouse click: try to select a soldier.
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !g.prevMouseLeft {
			mx, my := ebiten.CursorPosition()
			g.handleInspectorClick(mx, my)
		}
	}
	g.prevMouseLeft = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

	// I: toggle inspector raw/curated view.
	currentKeys[ebiten.KeyI] = ebiten.IsKeyPressed(ebiten.KeyI)
	if currentKeys[ebiten.KeyI] && !g.prevKeys[ebiten.KeyI] {
		g.inspector.rawView = !g.inspector.rawView
	}

	g.prevKeys = currentKeys
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Window background: very dark, outside battlefield.
	screen.Fill(color.RGBA{R: 12, G: 14, B: 12, A: 255})
	if false {
		g.drawFormationSlots(screen)
	}

	// Render world content to worldBuf at (0,0) origin, then blit with camera transform.
	g.worldBuf.Clear()
	g.drawWorld(g.worldBuf)

	// Camera transform: translate so camX/camY is at viewport centre, then scale.
	vpW := float64(g.gameWidth)
	vpH := float64(g.gameHeight)
	var cam ebiten.GeoM
	cam.Translate(-g.camX, -g.camY)
	cam.Scale(g.camZoom, g.camZoom)
	cam.Translate(vpW/2, vpH/2)
	// Draw worldBuf onto screen at the battlefield offset.
	var blit ebiten.DrawImageOptions
	blit.GeoM = cam
	blit.GeoM.Translate(float64(g.offX), float64(g.offY))
	screen.DrawImage(g.worldBuf, &blit)

	// Battlefield border frame (drawn at screen coords, not transformed).
	ox := float32(g.offX)
	oy := float32(g.offY)
	gw := float32(g.gameWidth)
	gh := float32(g.gameHeight)
	// Outer glow.
	vector.StrokeRect(screen, ox-4, oy-4, gw+8, gh+8, 1.0, color.RGBA{R: 30, G: 50, B: 30, A: 60}, false)
	// Mid border.
	vector.StrokeRect(screen, ox-2, oy-2, gw+4, gh+4, 1.5, color.RGBA{R: 50, G: 80, B: 50, A: 140}, false)
	// Main bright border.
	vector.StrokeRect(screen, ox-1, oy-1, gw+2, gh+2, 2.0, color.RGBA{R: 75, G: 110, B: 75, A: 255}, false)
	// Inner highlight.
	vector.StrokeRect(screen, ox, oy, gw, gh, 1.0, color.RGBA{R: 90, G: 140, B: 90, A: 60}, false)

	// Thought log panel — rendered to scaled buffer, then blitted.
	logBufW := logPanelWidth / logScale
	logBufH := g.height / logScale
	g.thoughtLog.Draw(g.logBuf, logBufW, logBufH)
	logX := g.offX + g.gameWidth + g.offX
	logOpts := &ebiten.DrawImageOptions{}
	logOpts.GeoM.Scale(float64(logScale), float64(logScale))
	logOpts.GeoM.Translate(float64(logX), 0)
	screen.DrawImage(g.logBuf, logOpts)

	// HUD key legend.
	if g.showHUD {
		g.drawHUD(screen)
	}

	// Soldier inspector panel (screen-space, drawn over everything).
	g.drawInspector(screen)

	if g.menuOpen {
		g.drawPauseMenu(screen)
	}
}

func (g *Game) drawPauseMenu(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, float32(g.width), float32(g.height), color.RGBA{R: 0, G: 0, B: 0, A: 175}, false)

	const menuW = 360
	const menuH = 170
	mx := (g.width - menuW) / 2
	my := (g.height - menuH) / 2

	vector.FillRect(screen, float32(mx), float32(my), menuW, menuH, color.RGBA{R: 12, G: 18, B: 12, A: 245}, false)
	vector.StrokeRect(screen, float32(mx), float32(my), menuW, menuH, 2, color.RGBA{R: 92, G: 140, B: 92, A: 255}, false)

	ebitenutil.DebugPrintAt(screen, "PAUSED", mx+140, my+18)
	ebitenutil.DebugPrintAt(screen, "ESC: resume", mx+18, my+44)
	ebitenutil.DebugPrintAt(screen, "W/S or Up/Down: select", mx+18, my+58)
	ebitenutil.DebugPrintAt(screen, "Enter: confirm", mx+18, my+72)

	options := []string{"Quit Program", "Restart (New Seed)"}
	for i, label := range options {
		prefix := "  "
		if i == g.menuSelection {
			prefix = "> "
		}
		ebitenutil.DebugPrintAt(screen, prefix+label, mx+90, my+102+i*20)
	}
}

func (g *Game) drawWorld(screen *ebiten.Image) {
	// For drawWorld, all coordinates are in world-space (no offX/offY needed).
	ox, oy := float32(0), float32(0)
	gw, gh := float32(g.gameWidth), float32(g.gameHeight)

	// Ground fill inside battlefield.
	vector.FillRect(screen, ox, oy, gw, gh, color.RGBA{R: 30, G: 45, B: 30, A: 255}, false)
	// Broad grass bands to keep the field from looking uniformly flat.
	for y := 0; y < g.gameHeight; y += 48 {
		band := (y / 48) % 5
		bandCol := color.RGBA{R: 0, G: clampToByte(36 + band*3), B: 0, A: 18}
		vector.FillRect(screen, ox, oy+float32(y), gw, 24, bandCol, false)
	}

	// Terrain patches — richer grassy variation and subtle texture.
	for _, tp := range g.terrainPatches {
		delta := int(tp.shade) - 6
		baseG := 42 + delta
		baseR := 28 + delta/2
		baseB := 26 + delta/4
		vector.FillRect(screen, ox+tp.x, oy+tp.y, tp.w, tp.h,
			color.RGBA{R: clampToByte(baseR), G: clampToByte(baseG), B: clampToByte(baseB), A: 38}, false)
		// Secondary tint for less boxy, more natural variation.
		vector.FillRect(screen,
			ox+tp.x+tp.w*0.15, oy+tp.y+tp.h*0.15,
			tp.w*0.65, tp.h*0.65,
			color.RGBA{R: clampToByte(baseR - 3), G: clampToByte(baseG + 4), B: clampToByte(baseB - 2), A: 24}, false)
	}

	// Sparse grass tufts for visual detail at medium zoom.
	for gy := 8; gy < g.gameHeight; gy += 12 {
		for gx := 8; gx < g.gameWidth; gx += 12 {
			h := terrainHash(gx/12, gy/12)
			if h%11 != 0 {
				continue
			}
			height := float32(3 + int((h>>4)%5))
			tilt := float32(int(h>>8)%3 - 1)
			c := color.RGBA{R: clampToByte(46 + int((h>>12)%14)), G: clampToByte(70 + int((h>>16)%28)), B: clampToByte(44 + int((h>>20)%12)), A: 85}
			x0 := ox + float32(gx)
			y0 := oy + float32(gy)
			vector.StrokeLine(screen, x0, y0, x0+tilt, y0-height, 0.5, c, false)
		}
	}

	gridFine := 16
	gridMid := gridFine * 4
	gridCoarse := gridMid * 4

	drawGridOffset(screen, 0, 0, g.gameWidth, g.gameHeight, gridFine, color.RGBA{R: 32, G: 47, B: 32, A: 255})
	drawGridOffset(screen, 0, 0, g.gameWidth, g.gameHeight, gridMid, color.RGBA{R: 38, G: 55, B: 38, A: 255})
	drawGridOffset(screen, 0, 0, g.gameWidth, g.gameHeight, gridCoarse, color.RGBA{R: 48, G: 68, B: 48, A: 255})

	// Roads — drawn as smooth curved polylines before buildings.
	roadFill := color.RGBA{R: 48, G: 46, B: 42, A: 255}     // dark asphalt
	roadEdge := color.RGBA{R: 62, G: 60, B: 54, A: 255}     // slightly lighter kerb
	roadMark := color.RGBA{R: 70, G: 68, B: 58, A: 120}     // faint centre-line
	roadShoulder := color.RGBA{R: 40, G: 38, B: 34, A: 255} // darker shoulder
	for _, rp := range g.roadPolylines {
		pts := rp.points
		hw := float32(rp.width)
		if len(pts) < 2 {
			continue
		}
		// Draw road as thick line segments following the polyline.
		// Shoulder (slightly wider, darker).
		for i := 0; i < len(pts)-1; i++ {
			x0 := ox + float32(pts[i][0])
			y0 := oy + float32(pts[i][1])
			x1 := ox + float32(pts[i+1][0])
			y1 := oy + float32(pts[i+1][1])
			vector.StrokeLine(screen, x0, y0, x1, y1, hw*2+6, roadShoulder, false)
		}
		// Main asphalt body.
		for i := 0; i < len(pts)-1; i++ {
			x0 := ox + float32(pts[i][0])
			y0 := oy + float32(pts[i][1])
			x1 := ox + float32(pts[i+1][0])
			y1 := oy + float32(pts[i+1][1])
			vector.StrokeLine(screen, x0, y0, x1, y1, hw*2, roadFill, false)
		}
		// Edge kerb lines.
		for i := 0; i < len(pts)-1; i++ {
			ax, ay := pts[i][0], pts[i][1]
			bx, by := pts[i+1][0], pts[i+1][1]
			dx := bx - ax
			dy := by - ay
			l := math.Sqrt(dx*dx + dy*dy)
			if l < 1 {
				continue
			}
			nx := float32(-dy / l * float64(hw))
			ny := float32(dx / l * float64(hw))
			// Left edge.
			vector.StrokeLine(screen,
				ox+float32(ax)+nx, oy+float32(ay)+ny,
				ox+float32(bx)+nx, oy+float32(by)+ny,
				1.0, roadEdge, false)
			// Right edge.
			vector.StrokeLine(screen,
				ox+float32(ax)-nx, oy+float32(ay)-ny,
				ox+float32(bx)-nx, oy+float32(by)-ny,
				1.0, roadEdge, false)
		}
		// Dashed centre-line.
		dashLen := float32(24)
		gapLen := float32(16)
		accum := float32(0)
		drawing := true
		for i := 0; i < len(pts)-1; i++ {
			x0 := ox + float32(pts[i][0])
			y0 := oy + float32(pts[i][1])
			x1 := ox + float32(pts[i+1][0])
			y1 := oy + float32(pts[i+1][1])
			dx := x1 - x0
			dy := y1 - y0
			segLen := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			if segLen < 1 {
				continue
			}
			pos := float32(0)
			for pos < segLen {
				threshold := dashLen
				if !drawing {
					threshold = gapLen
				}
				remain := threshold - accum
				advance := remain
				if pos+advance > segLen {
					advance = segLen - pos
				}
				if drawing {
					t0 := pos / segLen
					t1 := (pos + advance) / segLen
					vector.StrokeLine(screen,
						x0+dx*t0, y0+dy*t0,
						x0+dx*t1, y0+dy*t1,
						1.0, roadMark, false)
				}
				accum += advance
				pos += advance
				if accum >= threshold {
					accum = 0
					drawing = !drawing
				}
			}
		}
	}

	// Intel heatmap overlays (drawn under buildings and soldiers).
	team := Team(g.overlayTeam)
	if im := g.intel.For(team); im != nil {
		for k := IntelMapKind(0); k < intelMapCount; k++ {
			if g.showOverlay[g.overlayTeam][k] {
				g.drawHeatLayer(screen, im.Layer(k), overlayColors[k])
			}
		}
	}

	// Build a set of claimed building indices → team for tinting.
	claimedTeam := make(map[int]Team)
	for _, sq := range g.squads {
		if sq.ClaimedBuildingIdx >= 0 {
			claimedTeam[sq.ClaimedBuildingIdx] = sq.Team
		}
	}

	// Building interiors: floor first, then walls on top.
	// Floor areas (from footprints) — dark interior.
	for i, b := range g.buildingFootprints {
		x0 := ox + float32(b.x)
		y0 := oy + float32(b.y)
		bw := float32(b.w)
		bh := float32(b.h)

		// Soft drop shadow.
		vector.FillRect(screen, x0+4, y0+4, bw, bh, color.RGBA{R: 10, G: 8, B: 6, A: 80}, false)
		// Floor fill — dark concrete.
		vector.FillRect(screen, x0, y0, bw, bh, color.RGBA{R: 38, G: 36, B: 32, A: 255}, false)

		// Team tint overlay for claimed buildings.
		if t, ok := claimedTeam[i]; ok {
			var tint color.RGBA
			if t == TeamRed {
				tint = color.RGBA{R: 140, G: 30, B: 20, A: 35}
			} else {
				tint = color.RGBA{R: 20, G: 40, B: 140, A: 35}
			}
			vector.FillRect(screen, x0, y0, bw, bh, tint, false)
			// Thin team-colour border.
			var border color.RGBA
			if t == TeamRed {
				border = color.RGBA{R: 200, G: 60, B: 40, A: 80}
			} else {
				border = color.RGBA{R: 40, G: 80, B: 200, A: 80}
			}
			vector.StrokeRect(screen, x0, y0, bw, bh, 1.0, border, false)
		}

		// Subtle tile grid on the floor.
		tileCol := color.RGBA{R: 46, G: 44, B: 39, A: 180}
		tileStep := float32(cellSize)
		for tx := x0 + tileStep; tx < x0+bw; tx += tileStep {
			vector.StrokeLine(screen, tx, y0, tx, y0+bh, 0.5, tileCol, false)
		}
		for ty := y0 + tileStep; ty < y0+bh; ty += tileStep {
			vector.StrokeLine(screen, x0, ty, x0+bw, ty, 0.5, tileCol, false)
		}
	}
	// Build occupancy sets for orientation-aware wall/window rendering.
	solidSet := make(map[[2]int]bool, len(g.buildings)+len(g.windows))
	for _, b := range g.buildings {
		solidSet[[2]int{b.x / cellSize, b.y / cellSize}] = true
	}
	for _, w := range g.windows {
		solidSet[[2]int{w.x / cellSize, w.y / cellSize}] = true
	}

	// Wall segments — neighbour-aware lighting so edges/corners read correctly.
	wallFill := color.RGBA{R: 86, G: 80, B: 66, A: 255}
	wallLight := color.RGBA{R: 124, G: 115, B: 96, A: 210}
	wallDark := color.RGBA{R: 44, G: 40, B: 33, A: 220}
	for _, b := range g.buildings {
		x0 := ox + float32(b.x)
		y0 := oy + float32(b.y)
		bw := float32(b.w)
		bh := float32(b.h)
		cx := b.x / cellSize
		cy := b.y / cellSize
		hasN := solidSet[[2]int{cx, cy - 1}]
		hasS := solidSet[[2]int{cx, cy + 1}]
		hasW := solidSet[[2]int{cx - 1, cy}]
		hasE := solidSet[[2]int{cx + 1, cy}]

		vector.FillRect(screen, x0, y0, bw, bh, wallFill, false)
		// Exposed edges only (more accurate wall runs).
		if !hasN {
			vector.StrokeLine(screen, x0, y0, x0+bw, y0, 0.8, wallLight, false)
		}
		if !hasW {
			vector.StrokeLine(screen, x0, y0, x0, y0+bh, 0.8, wallLight, false)
		}
		if !hasS {
			vector.StrokeLine(screen, x0, y0+bh, x0+bw, y0+bh, 0.8, wallDark, false)
		}
		if !hasE {
			vector.StrokeLine(screen, x0+bw, y0, x0+bw, y0+bh, 0.8, wallDark, false)
		}
		// Mortar-like midline for larger contiguous pieces.
		if (hasW || hasE) && !(hasN && hasS) {
			vector.StrokeLine(screen, x0, y0+bh*0.5, x0+bw, y0+bh*0.5, 0.5, color.RGBA{R: 72, G: 66, B: 55, A: 110}, false)
		} else if hasN || hasS {
			vector.StrokeLine(screen, x0+bw*0.5, y0, x0+bw*0.5, y0+bh, 0.5, color.RGBA{R: 72, G: 66, B: 55, A: 110}, false)
		}
	}

	// Window segments — framed and orientation-aware glass panes.
	for _, w := range g.windows {
		x0 := ox + float32(w.x)
		y0 := oy + float32(w.y)
		bw := float32(w.w)
		bh := float32(w.h)
		cx := w.x / cellSize
		cy := w.y / cellSize
		hasN := solidSet[[2]int{cx, cy - 1}]
		hasS := solidSet[[2]int{cx, cy + 1}]
		hasW := solidSet[[2]int{cx - 1, cy}]
		hasE := solidSet[[2]int{cx + 1, cy}]

		// Frame.
		vector.FillRect(screen, x0, y0, bw, bh, color.RGBA{R: 60, G: 65, B: 72, A: 220}, false)
		inset := float32(2)
		paneX := x0 + inset
		paneY := y0 + inset
		paneW := bw - inset*2
		paneH := bh - inset*2
		if hasW && hasE && !(hasN && hasS) {
			paneY = y0 + bh*0.25
			paneH = bh * 0.5
		}
		if hasN && hasS && !(hasW && hasE) {
			paneX = x0 + bw*0.25
			paneW = bw * 0.5
		}
		if paneW < 2 {
			paneW = 2
		}
		if paneH < 2 {
			paneH = 2
		}
		// Glass pane.
		vector.FillRect(screen, paneX, paneY, paneW, paneH, color.RGBA{R: 68, G: 92, B: 126, A: 190}, false)
		// Reflective highlight + lower shadow for depth.
		vector.StrokeLine(screen, paneX, paneY, paneX+paneW, paneY, 0.8, color.RGBA{R: 145, G: 188, B: 232, A: 220}, false)
		vector.StrokeLine(screen, paneX, paneY+paneH, paneX+paneW, paneY+paneH, 0.8, color.RGBA{R: 30, G: 44, B: 70, A: 190}, false)
		vector.StrokeLine(screen, paneX+paneW*0.2, paneY+paneH*0.15, paneX+paneW*0.8, paneY+paneH*0.85, 0.5, color.RGBA{R: 175, G: 215, B: 245, A: 110}, false)
	}

	// Cover objects.
	g.drawCoverObjects(screen, 0, 0)

	// Vision cones (rendered to offscreen buffer then composited; world-space).
	g.drawVisionConesBuffered(screen, g.soldiers, color.RGBA{R: 180, G: 40, B: 30, A: 255}, 0.06)
	g.drawVisionConesBuffered(screen, g.opfor, color.RGBA{R: 30, G: 60, B: 180, A: 255}, 0.06)

	for _, s := range g.soldiers {
		s.Draw(screen, 0, 0)
	}
	for _, s := range g.opfor {
		s.Draw(screen, 0, 0)
	}

	// Movement intent lines: faint dashed line from soldier to path endpoint.
	g.drawMovementIntentLines(screen)

	// Active officer orders (leader-issued command markers).
	g.drawOfficerOrders(screen)

	// Squad intent labels near leaders.
	g.drawSquadIntentLabels(screen)

	// Selection ring for inspector target (world-space).
	if g.inspector.selected != nil {
		sel := g.inspector.selected
		if sel.state != SoldierStateDead {
			sr := float32(soldierRadius + 5)
			sx := float32(sel.x)
			sy := float32(sel.y)
			ringCol := color.RGBA{R: 255, G: 240, B: 60, A: 220}
			for a := 0; a < 16; a++ {
				ang0 := float64(a) / 16.0 * 2 * math.Pi
				ang1 := float64(a+1) / 16.0 * 2 * math.Pi
				vector.StrokeLine(screen,
					sx+sr*float32(math.Cos(ang0)),
					sy+sr*float32(math.Sin(ang0)),
					sx+sr*float32(math.Cos(ang1)),
					sy+sr*float32(math.Sin(ang1)),
					1.5, ringCol, false)
			}
		}
	}

	// Dynamic gunfire light blooms — drawn before muzzle flashes so they sit underneath.
	g.drawGunfireLighting(screen, 0, 0)

	// Muzzle flashes and tracers.
	g.combat.DrawMuzzleFlashes(screen, 0, 0)
	g.combat.DrawTracers(screen, 0, 0)

	// Speech bubbles above soldiers.
	g.drawSpeechBubbles(screen, 0, 0)

	// Detailed info panel for selected soldier (world-space).
	g.drawSelectedSoldierInfo(screen)

	// Spotted indicator.
	g.drawSpottedIndicators(screen, 0, 0)

	// Edge vignette — deeper for more atmosphere.
	g.drawVignette(screen, 0, 0)
}

// drawCoverObjects renders cover objects with orientation-aware visuals.
// Chest-walls detect their H/V neighbours to draw correct cross-sections and corners.
func (g *Game) drawCoverObjects(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	cs := float32(coverCellSize) // 16px
	iCS := coverCellSize

	// Build a fast set of chest-wall cell positions for neighbour lookup.
	type cellKey struct{ cx, cy int }
	chestSet := map[cellKey]bool{}
	for _, c := range g.covers {
		if c.kind == CoverChestWall {
			chestSet[cellKey{c.x / iCS, c.y / iCS}] = true
		}
	}

	// Palette.
	slabBody := color.RGBA{R: 100, G: 94, B: 80, A: 255}
	slabTop := color.RGBA{R: 138, G: 130, B: 112, A: 255} // bright top-face highlight
	slabSide := color.RGBA{R: 72, G: 68, B: 56, A: 255}   // darker side face
	slabShadow := color.RGBA{R: 8, G: 6, B: 4, A: 90}
	slabW := float32(5) // thickness of the wall strip in pixels

	for _, c := range g.covers {
		x0 := ox + float32(c.x)
		y0 := oy + float32(c.y)

		switch c.kind {
		case CoverTallWall:
			vector.FillRect(screen, x0+2, y0+2, cs, cs, color.RGBA{R: 8, G: 6, B: 4, A: 120}, false)
			vector.FillRect(screen, x0, y0, cs, cs, color.RGBA{R: 88, G: 82, B: 70, A: 255}, false)
			vector.StrokeLine(screen, x0, y0, x0+cs, y0, 1.0, color.RGBA{R: 118, G: 112, B: 96, A: 220}, false)
			vector.StrokeLine(screen, x0, y0, x0, y0+cs, 1.0, color.RGBA{R: 108, G: 102, B: 88, A: 180}, false)
			vector.StrokeLine(screen, x0, y0+cs, x0+cs, y0+cs, 1.0, color.RGBA{R: 45, G: 42, B: 34, A: 200}, false)

		case CoverChestWall:
			// Determine which neighbours exist.
			cx := c.x / iCS
			cy := c.y / iCS
			hasLeft := chestSet[cellKey{cx - 1, cy}]
			hasRight := chestSet[cellKey{cx + 1, cy}]
			hasUp := chestSet[cellKey{cx, cy - 1}]
			hasDown := chestSet[cellKey{cx, cy + 1}]
			hasH := hasLeft || hasRight
			hasV := hasUp || hasDown

			// Drop shadow first.
			vector.FillRect(screen, x0+2, y0+2, cs, cs, slabShadow, false)

			// Horizontal run: slab centred vertically across the cell (3D top-down wall).
			// The "top" of the wall is a bright horizontal stripe across the centre.
			if hasH && !hasV {
				// Pure horizontal — draw a horizontal slab.
				mid := y0 + cs/2 - slabW/2
				vector.FillRect(screen, x0, mid, cs, slabW, slabBody, false)
				vector.StrokeLine(screen, x0, mid, x0+cs, mid, 1.5, slabTop, false)
				vector.StrokeLine(screen, x0, mid+slabW, x0+cs, mid+slabW, 0.8, slabSide, false)
				// End caps only where wall ends.
				if !hasLeft {
					vector.StrokeLine(screen, x0, mid, x0, mid+slabW, 1.0, slabTop, false)
				}
				if !hasRight {
					vector.StrokeLine(screen, x0+cs, mid, x0+cs, mid+slabW, 1.0, slabSide, false)
				}
			} else if hasV && !hasH {
				// Pure vertical — draw a vertical slab.
				mid := x0 + cs/2 - slabW/2
				vector.FillRect(screen, mid, y0, slabW, cs, slabBody, false)
				vector.StrokeLine(screen, mid, y0, mid, y0+cs, 1.5, slabTop, false)
				vector.StrokeLine(screen, mid+slabW, y0, mid+slabW, y0+cs, 0.8, slabSide, false)
				if !hasUp {
					vector.StrokeLine(screen, mid, y0, mid+slabW, y0, 1.0, slabTop, false)
				}
				if !hasDown {
					vector.StrokeLine(screen, mid, y0+cs, mid+slabW, y0+cs, 1.0, slabSide, false)
				}
			} else {
				// Corner / T-junction / cross — draw both H and V slabs overlapping.
				// Horizontal band.
				midY := y0 + cs/2 - slabW/2
				vector.FillRect(screen, x0, midY, cs, slabW, slabBody, false)
				// Vertical band.
				midX := x0 + cs/2 - slabW/2
				vector.FillRect(screen, midX, y0, slabW, cs, slabBody, false)
				// Top-face highlights along both axes.
				vector.StrokeLine(screen, x0, midY, x0+cs, midY, 1.5, slabTop, false)
				vector.StrokeLine(screen, midX, y0, midX, y0+cs, 1.5, slabTop, false)
				vector.StrokeLine(screen, x0, midY+slabW, x0+cs, midY+slabW, 0.8, slabSide, false)
				vector.StrokeLine(screen, midX+slabW, y0, midX+slabW, y0+cs, 0.8, slabSide, false)
			}

		case CoverRubble:
			vector.FillRect(screen, x0, y0, cs, cs, color.RGBA{R: 42, G: 38, B: 30, A: 160}, false)
			vector.FillRect(screen, x0+1, y0+cs-7, 7, 5, color.RGBA{R: 92, G: 84, B: 68, A: 240}, false)
			vector.StrokeLine(screen, x0+1, y0+cs-7, x0+8, y0+cs-7, 0.5, color.RGBA{R: 118, G: 110, B: 90, A: 200}, false)
			vector.FillRect(screen, x0+cs-7, y0+2, 5, 4, color.RGBA{R: 82, G: 76, B: 62, A: 230}, false)
			vector.FillRect(screen, x0+5, y0+5, 3, 3, color.RGBA{R: 100, G: 92, B: 76, A: 210}, false)
			vector.FillRect(screen, x0+3, y0+cs-4, 2, 2, color.RGBA{R: 60, G: 55, B: 44, A: 180}, false)
			vector.FillRect(screen, x0+cs-5, y0+cs-5, 2, 2, color.RGBA{R: 60, G: 55, B: 44, A: 160}, false)
			vector.FillRect(screen, x0+8, y0+10, 2, 2, color.RGBA{R: 72, G: 66, B: 54, A: 150}, false)
		}
	}
}

// drawHeatLayer renders one HeatLayer as an alpha-blended colour wash.
func (g *Game) drawHeatLayer(screen *ebiten.Image, layer *HeatLayer, baseCol color.RGBA) {
	ox, oy := float32(0), float32(0)
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
// Text is drawn into hudBuf at 1x then composited onto the screen at hudScale (3×).
func (g *Game) drawHUD(screen *ebiten.Image) {
	teamLabel := "RED"
	if g.overlayTeam == 1 {
		teamLabel = "BLUE"
	}

	speedStr := "1x"
	if g.simSpeed == 0 {
		speedStr = "PAUSED"
	} else if g.simSpeed == 2 {
		speedStr = "2x"
	} else if g.simSpeed == 4 {
		speedStr = "4x"
	} else if g.simSpeed != 1 {
		speedStr = fmt.Sprintf("%.1fx", g.simSpeed)
	}

	lines := []string{
		fmt.Sprintf("SIM: %s  P=pause  ,/. speed", speedStr),
		fmt.Sprintf("Intel: [%s]  Tab=switch", teamLabel),
	}
	for k := IntelMapKind(0); k < intelMapCount; k++ {
		on := " "
		if g.showOverlay[g.overlayTeam][k] {
			on = "*"
		}
		lines = append(lines, fmt.Sprintf("  [%d]%s %s", k+1, on, IntelMapKindName(k)))
	}
	lines = append(lines, "[H] toggle HUD")
	lines = append(lines, "WASD/arrows=pan  scroll=zoom")
	lines = append(lines, fmt.Sprintf("zoom: %.1fx  click=inspect", g.camZoom))

	// Render into hudBuf at 1x, then scale up.
	const lineH = 12 // debug font line height at 1x
	const charW = 6  // debug font char width at 1x
	const padX = 5
	const padY = 4

	maxLen := 0
	for _, l := range lines {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	boxW := float32(maxLen*charW + padX*2)
	boxH := float32(len(lines)*lineH + padY*2)

	// Position in unscaled coordinates (hudBuf is screen/hudScale).
	bufW := float32(g.width / hudScale)
	bufH := float32(g.height / hudScale)
	bx := float32(4)
	by := bufH - boxH - 4

	g.hudBuf.Clear()
	// Panel background.
	vector.FillRect(g.hudBuf, bx, by, boxW, boxH,
		color.RGBA{R: 6, G: 10, B: 6, A: 210}, false)
	vector.StrokeRect(g.hudBuf, bx, by, boxW, boxH,
		1.0, color.RGBA{R: 60, G: 100, B: 60, A: 180}, false)
	// Inner highlight line along top edge.
	vector.StrokeLine(g.hudBuf, bx+1, by+1, bx+boxW-1, by+1,
		1.0, color.RGBA{R: 80, G: 140, B: 80, A: 80}, false)
	_ = bufW

	for i, line := range lines {
		tx := int(bx) + padX
		ty := int(by) + padY + i*lineH
		ebitenutil.DebugPrintAt(g.hudBuf, line, tx, ty)
	}

	// Blit hudBuf onto screen at hudScale.
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(float64(hudScale), float64(hudScale))
	screen.DrawImage(g.hudBuf, opts)
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

// drawSpottedIndicators renders a subtle "!" above soldiers who currently see enemies.
func (g *Game) drawSpottedIndicators(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	all := append(g.soldiers[:len(g.soldiers):len(g.soldiers)], g.opfor...)
	for _, s := range all {
		if s.state == SoldierStateDead || len(s.vision.KnownContacts) == 0 {
			continue
		}
		sx, sy := ox+float32(s.x), oy+float32(s.y)
		// "!" drawn as a short vertical stroke + dot, offset above the soldier.
		var c color.RGBA
		if s.team == TeamRed {
			c = color.RGBA{R: 255, G: 200, B: 100, A: 160}
		} else {
			c = color.RGBA{R: 100, G: 200, B: 255, A: 160}
		}
		topY := sy - float32(soldierRadius) - 10
		// Stroke of the "!"
		vector.StrokeLine(screen, sx, topY, sx, topY+5, 1.5, c, false)
		// Dot of the "!"
		vector.FillCircle(screen, sx, topY+7, 1.0, c, false)
	}
}

// drawVisionConesBuffered renders all FOV fans for a team into an offscreen buffer,
// then composites that buffer onto the main screen with a single controlled opacity.
// This eliminates additive blowout from overlapping cones.
func (g *Game) drawVisionConesBuffered(screen *ebiten.Image, soldiers []*Soldier, tint color.RGBA, opacity float64) {
	buf := g.visionBuf
	buf.Clear()

	// Draw solid white fans into the buffer; we'll tint + fade on composite.
	for _, s := range soldiers {
		if s.state == SoldierStateDead {
			continue
		}
		v := &s.vision
		halfFOV := v.FOV / 2.0
		coneLen := v.MaxRange * 0.45
		if coneLen < 60 {
			coneLen = 60
		}
		const steps = 36
		sx, sy := float32(s.x), float32(s.y)

		var path vector.Path
		path.MoveTo(sx, sy)
		for i := 0; i <= steps; i++ {
			a := s.vision.Heading - halfFOV + (v.FOV/float64(steps))*float64(i)
			pxW, pyW := g.clipVisionRayToBuildings(s.x, s.y, a, coneLen)
			path.LineTo(float32(pxW), float32(pyW))
		}
		path.Close()

		vector.FillPath(buf, &path, &vector.FillOptions{}, &vector.DrawPathOptions{AntiAlias: true})

		// Faint outline on the buffer too.
		edgeCol := color.RGBA{R: 255, G: 255, B: 255, A: 80}
		// Bounding rays.
		a0 := s.vision.Heading - halfFOV
		a1 := s.vision.Heading + halfFOV
		p0x, p0y := g.clipVisionRayToBuildings(s.x, s.y, a0, coneLen)
		p1x, p1y := g.clipVisionRayToBuildings(s.x, s.y, a1, coneLen)
		vector.StrokeLine(buf, sx, sy, float32(p0x), float32(p0y), 1.0, edgeCol, false)
		vector.StrokeLine(buf, sx, sy, float32(p1x), float32(p1y), 1.0, edgeCol, false)
		// Arc edge.
		var prevPx, prevPy float32
		for i := 0; i <= steps; i++ {
			a := s.vision.Heading - halfFOV + (v.FOV/float64(steps))*float64(i)
			axW, ayW := g.clipVisionRayToBuildings(s.x, s.y, a, coneLen)
			px, py := float32(axW), float32(ayW)
			if i > 0 {
				vector.StrokeLine(buf, prevPx, prevPy, px, py, 1.0, edgeCol, false)
			}
			prevPx, prevPy = px, py
		}
	}

	// Composite buffer onto screen with team tint and low opacity.
	opts := &ebiten.DrawImageOptions{}
	opts.ColorScale.ScaleWithColor(tint)
	opts.ColorScale.ScaleAlpha(float32(opacity))
	screen.DrawImage(buf, opts)
}

// clipVisionRayToBuildings returns the endpoint of a vision ray after clipping
// against the closest building intersection.
func (g *Game) clipVisionRayToBuildings(ox, oy, angle, maxLen float64) (float64, float64) {
	ex := ox + math.Cos(angle)*maxLen
	ey := oy + math.Sin(angle)*maxLen

	bestT := 1.0
	hitAny := false
	for _, b := range g.buildings {
		t, hit := rayAABBHitT(
			ox, oy, ex, ey,
			float64(b.x), float64(b.y),
			float64(b.x+b.w), float64(b.y+b.h),
		)
		if hit && t < bestT {
			bestT = t
			hitAny = true
		}
	}

	if hitAny {
		clipT := math.Max(0, bestT-0.01)
		ex = ox + (ex-ox)*clipT
		ey = oy + (ey-oy)*clipT
	}

	return ex, ey
}

// drawGunfireLighting draws radial light blooms at active muzzle flash positions.
// Each bloom is a series of concentric translucent circles, fading with distance.
// Team-tinted: red/orange for friendlies, blue/cyan for OpFor.
func (g *Game) drawGunfireLighting(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	flashes := g.combat.ActiveFlashes()
	for _, f := range flashes {
		fx := ox + float32(f.x)
		fy := oy + float32(f.y)
		progress := float32(f.age) / float32(flashLifetime)
		fade := 1.0 - progress

		var lR, lG, lB uint8
		if f.team == TeamRed {
			lR, lG, lB = 255, 160, 60 // warm orange bloom
		} else {
			lR, lG, lB = 80, 180, 255 // cool blue bloom
		}

		// Three concentric rings of decreasing opacity and increasing radius.
		vector.FillCircle(screen, fx, fy, 40*fade,
			color.RGBA{R: lR, G: lG, B: lB, A: uint8(30 * fade)}, false)
		vector.FillCircle(screen, fx, fy, 22*fade,
			color.RGBA{R: lR, G: lG, B: lB, A: uint8(50 * fade)}, false)
		vector.FillCircle(screen, fx, fy, 10*fade,
			color.RGBA{R: lR, G: lG, B: lB, A: uint8(80 * fade)}, false)
		// Hot white core.
		vector.FillCircle(screen, fx, fy, 4*fade,
			color.RGBA{R: 255, G: 255, B: 230, A: uint8(120 * fade)}, false)
	}
}

// drawVignette darkens the edges of the battlefield for atmosphere.
// Three-layer vignette: outer hard + mid + inner soft for cinematic depth.
func (g *Game) drawVignette(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	gw, gh := float32(g.gameWidth), float32(g.gameHeight)

	// Outer hard strip — strong darkening at the absolute edge.
	outer := float32(60)
	outerDark := color.RGBA{R: 0, G: 0, B: 0, A: 100}
	vector.FillRect(screen, ox, oy, gw, outer, outerDark, false)
	vector.FillRect(screen, ox, oy+gh-outer, gw, outer, outerDark, false)
	vector.FillRect(screen, ox, oy, outer, gh, outerDark, false)
	vector.FillRect(screen, ox+gw-outer, oy, outer, gh, outerDark, false)

	// Mid band — moderate darkening.
	mid := float32(160)
	midDark := color.RGBA{R: 0, G: 0, B: 0, A: 45}
	vector.FillRect(screen, ox, oy, gw, mid, midDark, false)
	vector.FillRect(screen, ox, oy+gh-mid, gw, mid, midDark, false)
	vector.FillRect(screen, ox, oy, mid, gh, midDark, false)
	vector.FillRect(screen, ox+gw-mid, oy, mid, gh, midDark, false)

	// Inner soft band — subtle atmosphere gradient.
	inner := float32(280)
	innerDark := color.RGBA{R: 0, G: 0, B: 0, A: 18}
	vector.FillRect(screen, ox, oy, gw, inner, innerDark, false)
	vector.FillRect(screen, ox, oy+gh-inner, gw, inner, innerDark, false)
	vector.FillRect(screen, ox, oy, inner, gh, innerDark, false)
	vector.FillRect(screen, ox+gw-inner, oy, inner, gh, innerDark, false)
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
