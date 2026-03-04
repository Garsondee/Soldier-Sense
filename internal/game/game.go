package game

import (
	"errors"
	"fmt"
	"image"
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
	// Squad status panels — rendered into a buffer at 1x then blitted at logScale.
	squadBufW     = 240 // same width as log buffer (logPanelWidth / logScale)
	squadBufH     = 88  // buffer height per panel; on screen = 88 * logScale = 264px
	squadPanelGap = 4   // screen-space gap between panels (pixels, at 1x before scale)
)

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

// overlayColors maps each IntelMapKind to its debug render color.
var overlayColors = [intelMapCount]color.RGBA{
	IntelContact:          {R: 255, G: 50, B: 50, A: 180},  // bright red
	IntelRecentContact:    {R: 255, G: 140, B: 0, A: 140},  // orange
	IntelThreatDensity:    {R: 200, G: 0, B: 200, A: 120},  // purple
	IntelFriendlyPresence: {R: 30, G: 160, B: 255, A: 130}, // sky blue
	IntelDangerZone:       {R: 255, G: 220, B: 0, A: 140},  // yellow
	IntelCasualtyDanger:   {R: 255, G: 110, B: 0, A: 140},  // amber
	IntelOpenGround:       {R: 255, G: 255, B: 255, A: 90}, // white
	IntelSafeTerritory:    {R: 0, G: 220, B: 90, A: 120},   // green
	IntelUnexplored:       {R: 20, G: 20, B: 20, A: 160},   // dark grey
}

func (g *Game) renderTerrainLayer(screen *ebiten.Image) { //nolint:gocognit,gocyclo
	ox, oy := float32(0), float32(0)
	gw, gh := float32(g.gameWidth), float32(g.gameHeight)

	// Per-tile ground rendering from TileMap.
	if g.tileMap != nil {
		cs := float32(cellSize)
		for row := 0; row < g.tileMap.Rows; row++ {
			for col := 0; col < g.tileMap.Cols; col++ {
				gt := g.tileMap.Ground(col, row)
				r, gr, b := groundBaseColor(gt)
				h := terrainHash(col, row)
				jitter := int(h%13) - 6
				r = clampToByte(int(r) + jitter/2)
				gr = clampToByte(int(gr) + jitter)
				b = clampToByte(int(b) + jitter/3)
				if gt == GroundTile && (col+row)%2 == 0 {
					gr = clampToByte(int(gr) + 4)
				}
				if gt == GroundWood && col%3 == 0 {
					gr = clampToByte(int(gr) - 3)
					b = clampToByte(int(b) - 2)
				}
				px := ox + float32(col)*cs
				py := oy + float32(row)*cs
				vector.FillRect(screen, px, py, cs, cs, color.RGBA{R: r, G: gr, B: b, A: 255}, false)
			}
		}
	} else {
		vector.FillRect(screen, ox, oy, gw, gh, color.RGBA{R: 30, G: 45, B: 30, A: 255}, false)
	}

	gridFine := 16
	gridMid := gridFine * 4
	gridCoarse := gridMid * 4
	drawGridOffset(screen, 0, 0, g.gameWidth, g.gameHeight, gridFine, color.RGBA{R: 32, G: 47, B: 32, A: 255})
	drawGridOffset(screen, 0, 0, g.gameWidth, g.gameHeight, gridMid, color.RGBA{R: 38, G: 55, B: 38, A: 255})
	drawGridOffset(screen, 0, 0, g.gameWidth, g.gameHeight, gridCoarse, color.RGBA{R: 48, G: 68, B: 48, A: 255})

	if g.tileMap == nil {
		return
	}
	cs := float32(cellSize)
	markCol := color.RGBA{R: 70, G: 68, B: 58, A: 100}
	for row := 0; row < g.tileMap.Rows; row++ {
		for col := 0; col < g.tileMap.Cols; col++ {
			if g.tileMap.Ground(col, row) != GroundTarmac {
				continue
			}
			hasL := col > 0 && g.tileMap.Ground(col-1, row) == GroundTarmac
			hasR := col < g.tileMap.Cols-1 && g.tileMap.Ground(col+1, row) == GroundTarmac
			hasU := row > 0 && g.tileMap.Ground(col, row-1) == GroundTarmac
			hasD := row < g.tileMap.Rows-1 && g.tileMap.Ground(col, row+1) == GroundTarmac
			px := ox + float32(col)*cs
			py := oy + float32(row)*cs
			if hasL && hasR && (col/2)%2 == 0 {
				mid := py + cs/2
				vector.StrokeLine(screen, px, mid, px+cs, mid, 0.5, markCol, false)
			}
			if hasU && hasD && (row/2)%2 == 0 {
				mid := px + cs/2
				vector.StrokeLine(screen, mid, py, mid, py+cs, 0.5, markCol, false)
			}
		}
	}
}

func (g *Game) drawTerrainLayerVisible(screen *ebiten.Image, minX, minY, maxX, maxY float64) {
	if g.terrainBuf == nil {
		g.renderTerrainLayer(screen)
		return
	}

	x0 := int(math.Floor(minX))
	y0 := int(math.Floor(minY))
	x1 := int(math.Ceil(maxX))
	y1 := int(math.Ceil(maxY))
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > g.gameWidth {
		x1 = g.gameWidth
	}
	if y1 > g.gameHeight {
		y1 = g.gameHeight
	}
	if x1 <= x0 || y1 <= y0 {
		return
	}

	sub, ok := g.terrainBuf.SubImage(image.Rect(x0, y0, x1, y1)).(*ebiten.Image)
	if !ok {
		screen.DrawImage(g.terrainBuf, nil)
		return
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x0), float64(y0))
	screen.DrawImage(sub, opts)
}

// drawSquadStatusPanels renders all squad status panels into the top of the
// right-side column using the same buffer→3x-blit technique as the thought log.
// Returns the total screen-space height consumed so the log can start below.
func drawSquadStatusPanels(g *Game, screen *ebiten.Image, panelX int) int {
	if len(g.squads) == 0 || g.squadBuf == nil {
		return 0
	}
	screenY := 0
	for _, sq := range g.squads {
		g.squadBuf.Clear()
		renderSquadPanel(g.squadBuf, sq)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(float64(logScale), float64(logScale))
		opts.GeoM.Translate(float64(panelX), float64(screenY))
		screen.DrawImage(g.squadBuf, opts)
		screenY += squadBufH*logScale + squadPanelGap*logScale
	}
	return screenY
}

// renderSquadPanel draws a single squad's status into buf at 1× scale.
func renderSquadPanel(buf *ebiten.Image, sq *Squad) {
	bw := float32(squadBufW)
	bh := float32(squadBufH)

	// Panel background.
	vector.FillRect(buf, 0, 0, bw, bh, color.RGBA{R: 10, G: 14, B: 10, A: 248}, false)
	vector.StrokeRect(buf, 0, 0, bw, bh, 1.0, color.RGBA{R: 55, G: 85, B: 60, A: 255}, false)

	stats := gatherSquadPanelStats(sq)

	// ── Row 0: Title bar (y 0..13) ──
	titleBg := color.RGBA{R: 28, G: 14, B: 14, A: 255}
	if sq.Team == TeamBlue {
		titleBg = color.RGBA{R: 14, G: 18, B: 32, A: 255}
	}
	vector.FillRect(buf, 0, 0, bw, 14, titleBg, false)
	vector.StrokeLine(buf, 0, 14, bw, 14, 1.0, color.RGBA{R: 60, G: 90, B: 60, A: 200}, false)

	teamStr := "RED"
	if sq.Team == TeamBlue {
		teamStr = "BLU"
	}
	statusLabel := squadStatusLabel(sq, stats.avgFear)
	effectiveness := clamp01(
		(1.0-float64(stats.casualties)/float64(max(1, len(sq.Members))))*0.35 +
			stats.avgHP*0.30 + stats.avgMorale*0.20 + (1.0-sq.Stress)*0.15)

	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("%s SQ-%d  %s  eff:%2.0f%%", teamStr, sq.ID, statusLabel, effectiveness*100), 4, 2)

	// ── Row 1: Health squares + strength summary (y 16..27) ──
	const sqSize = 6
	const sqGap = 2
	sqX0 := 4
	sqY0 := 17
	for i, m := range sq.Members {
		cx := sqX0 + (i%8)*(sqSize+sqGap)
		cy := sqY0 + (i/8)*(sqSize+sqGap)
		fill := color.RGBA{R: 30, G: 10, B: 10, A: 230}
		if m.state != SoldierStateDead {
			hp := clamp01(m.health() / soldierMaxHP)
			if sq.Team == TeamRed {
				fill = color.RGBA{R: uint8(50 + 190*hp), G: uint8(20 + 100*hp), B: uint8(20 + 60*hp), A: 240}
			} else {
				fill = color.RGBA{R: uint8(25 + 80*hp), G: uint8(45 + 130*hp), B: uint8(55 + 180*hp), A: 240}
			}
		}
		vector.FillRect(buf, float32(cx), float32(cy), sqSize, sqSize, fill, false)
		vector.StrokeRect(buf, float32(cx), float32(cy), sqSize, sqSize, 0.5, color.RGBA{R: 20, G: 30, B: 20, A: 200}, false)
	}

	lead := "-"
	if sq.Leader != nil && sq.Leader.state != SoldierStateDead {
		lead = sq.Leader.label
	}
	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("%d/%d up  lead:%s", stats.alive, len(sq.Members), lead), 72, 17)

	// ── Row 2: Objective + casualties (y 29..40) ──
	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("obj:%s  cas:%d", stats.mainGoal, stats.casualties), 4, 30)

	// ── Row 3: Phase / intent / formation (y 42..53) ──
	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("ph:%s int:%s", sq.Phase, sq.Intent), 4, 42)
	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("form:%d coh:%.0f%% dC:%+3.1f", sq.Formation, sq.Cohesion*100, sq.CohesionDelta*100), 4, 54)
	ebitenutil.DebugPrintAt(buf, fmt.Sprintf("dS:%+3.1f dM:%+3.1f", sq.StressDelta*100, sq.MoraleDelta*100), 134, 42)

	// ── Metric bars (y 68..84) — 4 bars, each 3px tall with 1px gap ──
	barX := 4
	barW := int(bw) - 8
	barH := 3
	barY0 := 68
	drawSquadMetricBars(buf, sq, stats, barX, barW, barH, barY0)
	// Bar legend — single line below bars.
	ebitenutil.DebugPrintAt(buf, "STR  FER  MOR  COH", 4+barW/2-54, barY0+4*4)
}

type squadPanelStats struct {
	alive      int
	casualties int
	avgFear    float64
	avgMorale  float64
	avgHP      float64
	mainGoal   GoalKind
}

func gatherSquadPanelStats(sq *Squad) squadPanelStats {
	stats := squadPanelStats{mainGoal: GoalAdvance}
	objectiveCounts := map[GoalKind]int{}
	for _, m := range sq.Members {
		if m.state == SoldierStateDead {
			stats.casualties++
			continue
		}
		stats.alive++
		stats.avgFear += m.profile.Psych.EffectiveFear()
		stats.avgMorale += m.profile.Psych.Morale
		stats.avgHP += clamp01(m.health() / soldierMaxHP)
		objectiveCounts[m.blackboard.CurrentGoal]++
	}
	if stats.alive > 0 {
		inv := 1.0 / float64(stats.alive)
		stats.avgFear *= inv
		stats.avgMorale *= inv
		stats.avgHP *= inv
	}
	stats.mainGoal = dominantGoal(objectiveCounts)
	return stats
}

func dominantGoal(objectiveCounts map[GoalKind]int) GoalKind {
	mainGoal := GoalAdvance
	bestCount := -1
	for goal, cnt := range objectiveCounts {
		if cnt > bestCount {
			bestCount = cnt
			mainGoal = goal
		}
	}
	return mainGoal
}

func squadStatusLabel(sq *Squad, avgFear float64) string {
	switch {
	case sq.Broken:
		return "SHATTERED"
	case sq.Stress > 0.62 || avgFear > 0.58:
		return "SHAKEN"
	case sq.Intent == IntentEngage:
		return "CONTACT"
	default:
		return "STEADY"
	}
}

func (g *Game) recordPerfFrame() {
	g.perfFrameCount++
	const sampleFrames = 120
	if g.perfFrameCount < sampleFrames {
		return
	}

	frameN := float64(g.perfFrameCount)
	simN := float64(g.perfSimTickRuns)
	avgWorldMS := (float64(g.perfWorldDrawNS) / frameN) / 1e6
	avgUIMS := (float64(g.perfUIDrawNS) / frameN) / 1e6
	avgSimMS := 0.0
	if simN > 0 {
		avgSimMS = (float64(g.perfSimTickNS) / simN) / 1e6
	}
	g.perfHUDLine = fmt.Sprintf("perf avg: sim %.2fms/tick world %.2fms ui %.2fms", avgSimMS, avgWorldMS, avgUIMS)

	g.perfFrameCount = 0
	g.perfSimTickRuns = 0
	g.perfSimTickNS = 0
	g.perfWorldDrawNS = 0
	g.perfUIDrawNS = 0
}

func (g *Game) updateAutoPerfCapture(now time.Time) {
	if !g.autoPerfCaptureEnabled {
		return
	}
	if g.autoPerfCaptureStart.IsZero() {
		g.autoPerfCaptureStart = now
	}
	if now.Sub(g.autoPerfCaptureStart) < g.autoPerfCaptureDur {
		return
	}

	duration := now.Sub(g.autoPerfCaptureStart).Seconds()
	frames := float64(g.perfTotalFrameCount)
	ticks := float64(g.perfTotalSimTickRuns)
	avgWorld := 0.0
	avgUI := 0.0
	avgSim := 0.0
	fps := 0.0
	if frames > 0 {
		avgWorld = float64(g.perfTotalWorldDrawNS) / frames / 1e6
		avgUI = float64(g.perfTotalUIDrawNS) / frames / 1e6
		if duration > 0 {
			fps = frames / duration
		}
	}
	if ticks > 0 {
		avgSim = float64(g.perfTotalSimTickNS) / ticks / 1e6
	}

	g.autoPerfCaptureResult = PerfCaptureStats{
		DurationSeconds:      duration,
		FrameCount:           g.perfTotalFrameCount,
		FPS:                  fps,
		SimTickCount:         g.perfTotalSimTickRuns,
		AvgSimMSPerTick:      avgSim,
		AvgWorldMSPerFrame:   avgWorld,
		AvgUIMSPerFrame:      avgUI,
		AvgFrameCPUmsBuckets: avgWorld + avgUI,
	}
	g.autoPerfCaptureEnabled = false
	g.pendingExit = ErrQuit
}

func drawSquadMetricBars(buf *ebiten.Image, sq *Squad, stats squadPanelStats, barX, barW, barH, barY0 int) {
	type metricBar struct {
		value float64
		col   color.RGBA
	}
	bars := []metricBar{
		{value: sq.Stress, col: color.RGBA{R: 210, G: 80, B: 60, A: 220}},
		{value: stats.avgFear, col: color.RGBA{R: 220, G: 150, B: 55, A: 220}},
		{value: stats.avgMorale, col: color.RGBA{R: 70, G: 180, B: 110, A: 220}},
		{value: sq.Cohesion, col: color.RGBA{R: 80, G: 140, B: 220, A: 220}},
	}
	for i, b := range bars {
		by := barY0 + i*4
		vector.FillRect(buf, float32(barX), float32(by), float32(barW), float32(barH), color.RGBA{R: 20, G: 28, B: 22, A: 220}, false)
		filled := int(clamp01(b.value) * float64(barW))
		if filled > 0 {
			vector.FillRect(buf, float32(barX), float32(by), float32(filled), float32(barH), b.col, false)
		}
	}
}

// WallInfo stores additional information about wall segments.
type WallInfo struct {
	Rect       rect     // Wall rectangle
	WallType   WallType // Type of wall construction
	IsExterior bool     // True if exterior wall, false if interior
}

// Game is the main simulation and rendering state container.
type Game struct { //nolint:govet,gocritic
	width              int
	height             int
	gameWidth          int               // playfield width (log panel takes the rest)
	gameHeight         int               // playfield height (inside border)
	offX               int               // pixel offset from window left to battlefield left
	offY               int               // pixel offset from window top to battlefield top
	buildings          []rect            // individual wall segments (1-cell wide), used for LOS/nav
	windows            []rect            // window segments: block movement, transparent to LOS
	buildingFootprints []rect            // overall floor area of each structure, used for rendering
	wallInfo           []WallInfo        // enhanced wall information with types and properties
	organicRoads       []OrganicRoad     // curved road network replacing grid roads
	lots               []Lot             // land subdivision for building placement
	compounds          []Compound        // multi-building complexes with perimeters
	exteriorFeatures   []ExteriorFeature // porches, loading docks, etc.
	buildingQualities  []BuildingQuality // pre-computed tactical metrics per footprint
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
	currentKeys map[ebiten.Key]bool

	// Offscreen buffer for vision cone rendering (avoids additive blowout).
	visionBuf *ebiten.Image
	// Cached static terrain layer (ground, grids, road markings) drawn once.
	terrainBuf *ebiten.Image
	// Cached static vignette layer drawn once and composited per frame.
	vignetteBuf *ebiten.Image
	// Offscreen buffer for the full battlefield — camera transform applied on blit.
	worldBuf *ebiten.Image
	// Offscreen buffer for HUD text — rendered at 1x then blitted at hudScale.
	hudBuf *ebiten.Image
	// Offscreen buffer for the thought log panel — rendered at 1x then blitted at logScale.
	logBuf *ebiten.Image
	// Offscreen buffer for the inspector panel — rendered at 1x then blitted at inspScale.
	inspBuf *ebiten.Image
	// Offscreen buffer for squad status panels — reused per panel, blitted at logScale.
	squadBuf *ebiten.Image

	// Deterministic terrain noise patches, generated once.
	terrainPatches []terrainPatch

	// Per-tile terrain map — authoritative ground/object data for every cell.
	tileMap *TileMap

	// Camera pan + zoom.
	camX    float64 // world-space X of the camera center
	camY    float64 // world-space Y of the camera center
	camZoom float64 // zoom factor (1.0 = native, >1 = zoomed in)

	autoFrameCamera  bool
	autoFramePadding float64

	// Soldier speech bubbles.
	speechBubbles []*SpeechBubble
	speechRng     *rand.Rand
	allSoldiers   []*Soldier

	// Soldier inspector (click-to-select panel).
	inspector Inspector

	// Cached maps for rendering (avoid per-frame allocations).
	cachedClaimedTeam map[int]Team
	cachedSolidSet    map[[2]int]bool
	cachedChestSet    map[[2]int]bool
	chestSetReady     bool
	hudLinesScratch   []string
	hudIntelLines     []string
	hudFilterLine     string
	hudIntelDirty     bool
	hudFilterDirty    bool
	hudOverlayTeam    int
	hudOverlayState   [2][intelMapCount]bool
	hudFilterState    [logCatCount]bool
	prevMouseLeft     bool // for edge-triggered click detection

	// Simulation speed control.
	simSpeed  float64 // multiplier: 0=paused, 0.5, 1, 2, 4
	tickAccum float64 // fractional tick accumulator for sub-1x speeds

	// Frame-rate independent interpolation for smooth visuals.
	lastUpdateTime time.Time // timestamp of last Update call
	interpolation  float64   // sub-tick interpolation [0, 1) for smooth rendering

	// ESC menu state.
	menuOpen        bool
	menuSelection   int
	menuResumeSpeed float64
	pendingExit     error

	// AAR overlay state.
	aarOpen      bool
	aarSelection int
	aarReason    BattleOutcomeReason

	// Analytics reporter — collects behavior stats periodically.
	reporter *SimReporter

	// Master map seed — printed at startup so layouts can be reproduced.
	mapSeed int64

	// Spatial partitioning for performance optimization.
	spatialHashRed  *SpatialHash
	spatialHashBlue *SpatialHash
	losIndex        *LOSIndex

	// Lightweight performance counters (rolling window).
	perfFrameCount  int
	perfSimTickRuns int
	perfSimTickNS   int64
	perfWorldDrawNS int64
	perfUIDrawNS    int64
	perfHUDLine     string

	perfTotalFrameCount  int
	perfTotalSimTickRuns int
	perfTotalSimTickNS   int64
	perfTotalWorldDrawNS int64
	perfTotalUIDrawNS    int64

	autoPerfCaptureEnabled bool
	autoPerfCaptureStart   time.Time
	autoPerfCaptureDur     time.Duration
	autoPerfCaptureResult  PerfCaptureStats
}

// PerfCaptureStats summarizes rendered runtime performance over a capture window.
type PerfCaptureStats struct {
	DurationSeconds      float64
	FrameCount           int
	FPS                  float64
	SimTickCount         int
	AvgSimMSPerTick      float64
	AvgWorldMSPerFrame   float64
	AvgUIMSPerFrame      float64
	AvgFrameCPUmsBuckets float64
}

// EnableAutoPerfCapture enables timed runtime capture and auto-exit.
func (g *Game) EnableAutoPerfCapture(d time.Duration) {
	if d <= 0 {
		return
	}
	g.autoPerfCaptureEnabled = true
	g.autoPerfCaptureDur = d
	g.autoPerfCaptureStart = time.Time{}
	g.autoPerfCaptureResult = PerfCaptureStats{}

	g.perfTotalFrameCount = 0
	g.perfTotalSimTickRuns = 0
	g.perfTotalSimTickNS = 0
	g.perfTotalWorldDrawNS = 0
	g.perfTotalUIDrawNS = 0
}

// AutoPerfCaptureResult returns the last automatic performance capture snapshot.
func (g *Game) AutoPerfCaptureResult() PerfCaptureStats {
	return g.autoPerfCaptureResult
}

type rect struct {
	x int
	y int
	w int
	h int
}

// terrainPatch is a subtle ground color variation tile.
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

// New creates a new game instance with a time-based map seed.
func New() *Game {
	return NewWithMapSeed(time.Now().UnixNano())
}

// NewWithMapSeed creates a new game instance using an explicit map seed.
func NewWithMapSeed(mapSeed int64) *Game {
	// Battlefield is 3072x1728 — double the original size.
	battleW := 3072
	battleH := 1728

	// Master map seed — printed to console for reproducibility.
	fmt.Printf("MAP SEED: %d\n", mapSeed)

	g := &Game{
		width:       borderWidth + battleW + borderWidth + logPanelWidth,
		height:      borderWidth + battleH + borderWidth,
		gameWidth:   battleW,
		gameHeight:  battleH,
		offX:        borderWidth,
		offY:        borderWidth,
		thoughtLog:  NewThoughtLog(),
		showHUD:     true,
		prevKeys:    make(map[ebiten.Key]bool),
		currentKeys: make(map[ebiten.Key]bool),
		mapSeed:     mapSeed,
	}
	mapRng := rand.New(rand.NewSource(mapSeed)) // #nosec G404 -- game only
	// Create the TileMap first — organic roads and lot-based buildings write directly into it.
	g.tileMap = NewTileMap(battleW/cellSize, battleH/cellSize)

	// Generate organic road network first
	fmt.Printf("DEBUG: Generating organic road network...\n")
	organicRoads := generateOrganicRoadNetwork(g.tileMap, mapRng)
	fmt.Printf("DEBUG: Generated %d organic roads\n", len(organicRoads))

	// Generate lot subdivision based on roads
	fmt.Printf("DEBUG: Generating lot subdivision...\n")
	lots := generateLotSubdivision(g.tileMap, organicRoads, mapRng)
	fmt.Printf("DEBUG: Generated %d lots\n", len(lots))

	// Generate buildings within lots
	fmt.Printf("DEBUG: Placing buildings from lots...\n")
	g.initBuildingsFromLots(lots, mapRng)
	fmt.Printf("DEBUG: Generated %d buildings\n", len(g.buildingFootprints))

	// Generate compounds (multi-building complexes)
	compounds := generateCompounds(g.tileMap, g.buildingFootprints, mapRng)

	// Generate exterior features for buildings (porches, loading docks, sheds, etc.)
	fmt.Printf("DEBUG: Generating exterior features...\n")
	buildingTypes := g.getBuildingTypes() // Get building types from the generation process
	exteriorFeatures := generateExteriorFeatures(g.buildingFootprints, buildingTypes, mapRng)
	fmt.Printf("DEBUG: Generated %d exterior features\n", len(exteriorFeatures))

	// Generate building names
	fmt.Printf("DEBUG: Generating building names...\n")
	buildingShapes := g.getBuildingShapes()
	buildingNames := generateBuildingNames(g.buildingFootprints, buildingTypes, buildingShapes, mapRng)
	fmt.Printf("DEBUG: Generated %d building names\n", len(buildingNames))

	// Store road and lot data for future use
	g.organicRoads = organicRoads
	g.lots = lots
	g.compounds = compounds
	g.exteriorFeatures = exteriorFeatures
	g.initCover()
	g.losIndex = NewLOSIndex(g.buildings, g.covers)
	g.initTileMap() // stamp buildings/cover into tileMap after generation
	fmt.Printf("DEBUG: Running biome generation...\n")
	generateBiome(g.tileMap, mapRng, &defaultBiomeConfig)
	fmt.Printf("DEBUG: Running fortification generation...\n")
	generateFortifications(g.tileMap, mapRng, defaultFortConfig)
	fmt.Printf("DEBUG: Map generation complete\n")
	g.navGrid = NewNavGrid(g.gameWidth, g.gameHeight, g.buildings, soldierRadius, g.covers, g.windows)
	g.tacticalMap = NewTacticalMap(g.gameWidth, g.gameHeight, g.buildings, g.windows, g.buildingFootprints)
	g.buildingQualities = ComputeBuildingQualities(g.buildingFootprints, g.buildings, g.windows, g.gameWidth, g.gameHeight, g.navGrid)
	g.initSoldiers()
	g.initOpFor()
	g.initSquads()
	g.randomiseProfiles()
	g.combat = NewCombatManager(time.Now().UnixNano() + 7777)
	g.intel = NewIntelStore(g.gameWidth, g.gameHeight)
	g.intel.SetTileMap(g.tileMap)
	for _, s := range g.soldiers {
		s.setIntel(g.intel)
	}
	for _, s := range g.opfor {
		s.setIntel(g.intel)
	}
	g.visionBuf = ebiten.NewImage(battleW, battleH)
	g.terrainBuf = ebiten.NewImage(battleW, battleH)
	g.vignetteBuf = ebiten.NewImage(battleW, battleH)
	g.worldBuf = ebiten.NewImage(battleW, battleH)
	// HUD buffer: 1/hudScale of screen so it renders crisply when scaled up.
	g.hudBuf = ebiten.NewImage(g.width/hudScale, g.height/hudScale)
	// Log buffer: 1/logScale of the log panel area.
	g.logBuf = ebiten.NewImage(logPanelWidth/logScale, g.height/logScale)
	// Inspector buffer: 1/inspScale of the inspector panel area.
	g.inspBuf = ebiten.NewImage(inspBufW, inspBufH)
	// Squad status panel buffer: reused for each panel, blitted at logScale.
	g.squadBuf = ebiten.NewImage(squadBufW, squadBufH)
	g.initTerrainPatches()
	// Default camera: centered on battlefield, zoom 0.5 so the full map is visible.
	g.camX = float64(battleW) / 2
	g.camY = float64(battleH) / 2
	g.camZoom = 0.5
	g.autoFrameCamera = true
	g.autoFramePadding = 96
	g.applyAutoFrameCamera(false)
	g.simSpeed = 1.0
	// Initialize cached maps for rendering.
	g.cachedClaimedTeam = make(map[int]Team)
	g.cachedSolidSet = make(map[[2]int]bool, len(g.buildings)+len(g.windows))
	g.cachedChestSet = make(map[[2]int]bool)
	g.rebuildChestSet()
	g.renderTerrainLayer(g.terrainBuf)
	g.renderVignetteLayer(g.vignetteBuf)
	g.hudIntelDirty = true
	g.hudFilterDirty = true
	g.speechRng = rand.New(rand.NewSource(time.Now().UnixNano() + 9999)) // #nosec G404 -- non-crypto RNG for local flavor text
	g.reporter = NewSimReporter(reportWindowTicks, false)
	// Initialize spatial hashes with cell size = max vision range for optimal performance.
	g.spatialHashRed = NewSpatialHash(defaultViewDist)
	g.spatialHashBlue = NewSpatialHash(defaultViewDist)
	return g
}

// getBuildingTypes returns the building types for all generated buildings.
func (g *Game) getBuildingTypes() []BuildingType {
	// For now, classify each building based on its footprint
	// In a full implementation, this would be stored during generation
	buildingTypes := make([]BuildingType, len(g.buildingFootprints))
	unit := 64
	rng := rand.New(rand.NewSource(g.mapSeed + 1)) // #nosec G404 -- deterministic map classification

	for i, fp := range g.buildingFootprints {
		buildingTypes[i] = classifyBuildingType(fp, unit, rng)
	}

	return buildingTypes
}

// getBuildingShapes returns the building shapes for all generated buildings.
func (g *Game) getBuildingShapes() []BuildingShape {
	// For now, all buildings are rectangular unless we track complex shapes during generation
	// In a full implementation, this would be stored during the complex footprint generation
	buildingShapes := make([]BuildingShape, len(g.buildingFootprints))

	for i := range g.buildingFootprints {
		buildingShapes[i] = ShapeRectangular // Default for now
		// TODO: Track actual complex shapes (L/T/U) during generation
	}

	return buildingShapes
}

// initBuildingsFromLots generates buildings within lot boundaries using the new lot-based system.
func (g *Game) initBuildingsFromLots(lots []Lot, rng *rand.Rand) {
	wall := cellSize // 16px
	unit := 64

	g.buildings = g.buildings[:0]
	g.buildingFootprints = g.buildingFootprints[:0]

	// Generate building candidates from lots
	candidates := buildingCandidatesInLots(lots, rng)

	// Place buildings from lot candidates
	for _, candidate := range candidates {
		// Check for overlaps with existing buildings
		if g.overlapsAnyBuilding(candidate, rng) {
			continue
		}

		// Check for overlaps with roads with buffer zone
		buffer := 32 // 32 pixel buffer around roads
		expandedCandidate := rect{
			x: candidate.x - buffer,
			y: candidate.y - buffer,
			w: candidate.w + 2*buffer,
			h: candidate.h + 2*buffer,
		}
		if rectOverlapsRoadTiles(g.tileMap, expandedCandidate) {
			continue
		}

		// Classify building type first
		buildingType := classifyBuildingType(candidate, unit, rng)

		// Complex building shapes for larger buildings
		if candidate.w/unit >= 8 && candidate.h/unit >= 8 {
			shapeRoll := rng.Float64()
			var complexFootprint ComplexFootprint

			switch {
			case shapeRoll < 0.15: // 15% L-shaped
				complexFootprint = generateLShapedFootprint(candidate, rng)
			case shapeRoll < 0.25: // 10% T-shaped
				complexFootprint = generateTShapedFootprint(candidate, rng)
			case shapeRoll < 0.32: // 7% U-shaped (rarest, most complex)
				complexFootprint = generateUShapedFootprint(candidate, rng)
			default: // 68% rectangular (still majority)
				complexFootprint = ComplexFootprint{
					Sections: []rect{candidate},
				}
			}

			// Add each section as a separate footprint and generate walls with type context
			for _, section := range complexFootprint.Sections {
				g.buildingFootprints = append(g.buildingFootprints, section)
				g.addBuildingWallsWithType(rng, section, wall, unit, buildingType)
			}
		} else {
			// Standard rectangular building for smaller sizes
			g.buildingFootprints = append(g.buildingFootprints, candidate)
			g.addBuildingWallsWithType(rng, candidate, wall, unit, buildingType)
		}
	}
}

// initTerrainPatches generates deterministic subtle ground color patches.
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

// initTileMap stamps buildings and cover into the existing TileMap.
// The TileMap is created in New() and grid roads are already stamped before this runs.
func (g *Game) initTileMap() {
	// Mark building footprints as indoor with concrete floor.
	for _, fp := range g.buildingFootprints {
		cMin := fp.x / cellSize
		rMin := fp.y / cellSize
		cMax := (fp.x + fp.w - 1) / cellSize
		rMax := (fp.y + fp.h - 1) / cellSize
		for r := rMin; r <= rMax; r++ {
			for c := cMin; c <= cMax; c++ {
				g.tileMap.SetGround(c, r, GroundConcrete)
				g.tileMap.AddFlag(c, r, TileFlagIndoor)
			}
		}
	}

	// Stamp wall segments.
	for _, b := range g.buildings {
		c := b.x / cellSize
		r := b.y / cellSize
		g.tileMap.SetObject(c, r, ObjectWall)
	}

	// Stamp window segments.
	for _, w := range g.windows {
		c := w.x / cellSize
		r := w.y / cellSize
		g.tileMap.SetObject(c, r, ObjectWindow)
	}

	// Stamp cover objects.
	for _, co := range g.covers {
		c := co.x / cellSize
		r := co.y / cellSize
		switch co.kind {
		case CoverTallWall:
			g.tileMap.SetObject(c, r, ObjectTallWall)
		case CoverChestWall:
			g.tileMap.SetObject(c, r, ObjectChestWall)
		case CoverRubbleLight:
			g.tileMap.SetObject(c, r, ObjectRubblePile)
			g.tileMap.SetGround(c, r, GroundRubbleLight)
		case CoverRubbleMedium:
			g.tileMap.SetObject(c, r, ObjectRubblePile)
			g.tileMap.SetGround(c, r, GroundRubbleLight)
		case CoverRubbleHeavy:
			g.tileMap.SetObject(c, r, ObjectRubblePile)
			g.tileMap.SetGround(c, r, GroundRubbleHeavy)
		case CoverRubbleMetal:
			g.tileMap.SetObject(c, r, ObjectRubblePile)
			g.tileMap.SetGround(c, r, GroundRubbleLight)
		case CoverRubbleWood:
			g.tileMap.SetObject(c, r, ObjectRubblePile)
			g.tileMap.SetGround(c, r, GroundRubbleLight)
		}
	}
}

func (g *Game) initCover() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 12345)) // #nosec G404 -- game only
	var rubble []*CoverObject
	g.covers, rubble = GenerateCover(g.gameWidth, g.gameHeight, g.buildingFootprints, g.buildings, rng, g.tileMap)
	// Rubble replaces wall segments where explosions hit — remove those walls and add rubble.
	g.applyBuildingDamage(rubble)
	g.chestSetReady = false
}

func (g *Game) rebuildChestSet() {
	if g.cachedChestSet == nil {
		return
	}
	for k := range g.cachedChestSet {
		delete(g.cachedChestSet, k)
	}
	for _, c := range g.covers {
		if c.kind != CoverChestWall {
			continue
		}
		g.cachedChestSet[[2]int{c.x / coverCellSize, c.y / coverCellSize}] = true
	}
	g.chestSetReady = true
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

// NOTE: Old initBuildings function removed - replaced by initBuildingsFromLots
// This function was using buildingCandidatesAlongGridRoads which conflicts with organic system

// generateHallwayLayout creates a central hallway with rooms branching off it.
func (g *Game) generateHallwayLayout(rng *rand.Rand, fp rect, wall, unit int, leafRooms *[]interiorRoom) { //nolint:gocognit,gocyclo
	// Determine hallway orientation based on building dimensions
	horizontal := fp.w > fp.h

	if horizontal {
		// Horizontal hallway running east-west
		hallwayWidth := unit
		hallwayY := fp.y + fp.h/2 - hallwayWidth/2

		// Create hallway walls (north and south of corridor)
		for wx := fp.x; wx < fp.x+fp.w; wx += wall {
			// North hallway wall
			if hallwayY > fp.y {
				g.buildings = append(g.buildings, rect{x: wx, y: hallwayY - wall, w: wall, h: wall})
			}
			// South hallway wall
			if hallwayY+hallwayWidth+wall < fp.y+fp.h {
				g.buildings = append(g.buildings, rect{x: wx, y: hallwayY + hallwayWidth, w: wall, h: wall})
			}
		}

		// Create rooms north and south of hallway
		northHeight := hallwayY - wall - fp.y
		southHeight := fp.y + fp.h - (hallwayY + hallwayWidth + wall)

		// Divide each side into 2-4 rooms
		if northHeight >= 2*unit {
			roomCount := 2 + rng.Intn(3) // 2-4 rooms
			roomWidth := fp.w / roomCount
			for i := 0; i < roomCount; i++ {
				roomX := fp.x + i*roomWidth
				actualWidth := roomWidth
				if i == roomCount-1 { // Last room gets any remainder
					actualWidth = fp.x + fp.w - roomX
				}
				if actualWidth >= 2*unit {
					*leafRooms = append(*leafRooms, interiorRoom{
						rx: roomX, ry: fp.y, rw: actualWidth, rh: northHeight,
					})

					// Add door to hallway
					doorX := roomX + actualWidth/2
					placeDoorInDoorway(g.tileMap, rng, doorX, hallwayY-wall, unit, false)
				}
			}
		}

		if southHeight >= 2*unit {
			roomCount := 2 + rng.Intn(3) // 2-4 rooms
			roomWidth := fp.w / roomCount
			for i := 0; i < roomCount; i++ {
				roomX := fp.x + i*roomWidth
				actualWidth := roomWidth
				if i == roomCount-1 { // Last room gets any remainder
					actualWidth = fp.x + fp.w - roomX
				}
				if actualWidth >= 2*unit {
					*leafRooms = append(*leafRooms, interiorRoom{
						rx: roomX, ry: hallwayY + hallwayWidth + wall, rw: actualWidth, rh: southHeight,
					})

					// Add door to hallway
					doorX := roomX + actualWidth/2
					placeDoorInDoorway(g.tileMap, rng, doorX, hallwayY+hallwayWidth, unit, false)
				}
			}
		}
	} else {
		// Vertical hallway running north-south
		hallwayWidth := unit
		hallwayX := fp.x + fp.w/2 - hallwayWidth/2

		// Create hallway walls (west and east of corridor)
		for wy := fp.y; wy < fp.y+fp.h; wy += wall {
			// West hallway wall
			if hallwayX > fp.x {
				g.buildings = append(g.buildings, rect{x: hallwayX - wall, y: wy, w: wall, h: wall})
			}
			// East hallway wall
			if hallwayX+hallwayWidth+wall < fp.x+fp.w {
				g.buildings = append(g.buildings, rect{x: hallwayX + hallwayWidth, y: wy, w: wall, h: wall})
			}
		}

		// Create rooms west and east of hallway
		westWidth := hallwayX - wall - fp.x
		eastWidth := fp.x + fp.w - (hallwayX + hallwayWidth + wall)

		// Divide each side into 2-4 rooms
		if westWidth >= 2*unit {
			roomCount := 2 + rng.Intn(3) // 2-4 rooms
			roomHeight := fp.h / roomCount
			for i := 0; i < roomCount; i++ {
				roomY := fp.y + i*roomHeight
				actualHeight := roomHeight
				if i == roomCount-1 { // Last room gets any remainder
					actualHeight = fp.y + fp.h - roomY
				}
				if actualHeight >= 2*unit {
					*leafRooms = append(*leafRooms, interiorRoom{
						rx: fp.x, ry: roomY, rw: westWidth, rh: actualHeight,
					})

					// Add door to hallway
					doorY := roomY + actualHeight/2
					placeDoorInDoorway(g.tileMap, rng, hallwayX-wall, doorY, unit, false)
				}
			}
		}

		if eastWidth >= 2*unit {
			roomCount := 2 + rng.Intn(3) // 2-4 rooms
			roomHeight := fp.h / roomCount
			for i := 0; i < roomCount; i++ {
				roomY := fp.y + i*roomHeight
				actualHeight := roomHeight
				if i == roomCount-1 { // Last room gets any remainder
					actualHeight = fp.y + fp.h - roomY
				}
				if actualHeight >= 2*unit {
					*leafRooms = append(*leafRooms, interiorRoom{
						rx: hallwayX + hallwayWidth + wall, ry: roomY, rw: eastWidth, rh: actualHeight,
					})

					// Add door to hallway
					doorY := roomY + actualHeight/2
					placeDoorInDoorway(g.tileMap, rng, hallwayX+hallwayWidth, doorY, unit, false)
				}
			}
		}
	}
}

// BuildingType represents the function/purpose of a building.
type BuildingType uint8

const (
	// BuildingTypeResidential indicates houses, apartments, and residential structures.
	BuildingTypeResidential BuildingType = iota // Houses, apartments, residential
	// BuildingTypeCommercial indicates shops, offices, and services.
	BuildingTypeCommercial // Shops, offices, services
	// BuildingTypeIndustrial indicates warehouses and factories.
	BuildingTypeIndustrial // Warehouses, factories, manufacturing
	// BuildingTypeMilitary indicates bunkers and command posts.
	BuildingTypeMilitary // Bunkers, barracks, command posts
	// BuildingTypeAgricultural indicates barns and farm buildings.
	BuildingTypeAgricultural // Barns, silos, farm buildings
	// BuildingTypeGeneric indicates a fallback unknown type.
	BuildingTypeGeneric // Fallback/unknown
)

// BuildingShape represents different building footprint shapes.
type BuildingShape uint8

const (
	// ShapeRectangular indicates a standard rectangular building.
	ShapeRectangular BuildingShape = iota // Standard rectangular building
	// ShapeLShaped indicates an L-shaped building with two wings.
	ShapeLShaped // L-shaped building with two wings
	// ShapeTShaped indicates a T-shaped building footprint.
	ShapeTShaped // T-shaped building with main body and perpendicular wing
	// ShapeUShaped indicates a U-shaped building with a courtyard.
	ShapeUShaped // U-shaped building with courtyard in center
)

// ComplexFootprint represents a building with multiple rectangular sections.
type ComplexFootprint struct {
	Sections []rect        // List of rectangular sections that make up the building
	Shape    BuildingShape // Type of complex shape
	Type     BuildingType  // Function/purpose of the building
}

// determineWallType returns the appropriate wall type based on building type and whether it's exterior/interior.
func determineWallType(buildingType BuildingType, isExterior bool) WallType {
	if isExterior {
		switch buildingType {
		case BuildingTypeResidential:
			return WallTypeResidentialExterior
		case BuildingTypeCommercial:
			return WallTypeCommercialExterior
		case BuildingTypeIndustrial:
			return WallTypeIndustrialExterior
		case BuildingTypeMilitary:
			return WallTypeMilitaryExterior
		case BuildingTypeAgricultural:
			return WallTypeAgriculturalExterior
		default:
			return WallTypeResidentialExterior // Default fallback
		}
	} else {
		switch buildingType {
		case BuildingTypeResidential:
			return WallTypeResidentialInterior
		case BuildingTypeCommercial:
			return WallTypeCommercialInterior
		case BuildingTypeIndustrial:
			return WallTypeIndustrialInterior
		case BuildingTypeMilitary:
			return WallTypeMilitaryInterior
		case BuildingTypeAgricultural:
			return WallTypeAgriculturalInterior
		default:
			return WallTypeResidentialInterior // Default fallback
		}
	}
}

// addBuildingWallsWithType generates building walls with type-specific customizations.
func (g *Game) addBuildingWallsWithType(rng *rand.Rand, fp rect, wall, unit int, buildingType BuildingType) { //nolint:gocognit,gocyclo
	// Apply type-specific modifications to building generation
	wUnits := fp.w / unit
	hUnits := fp.h / unit

	if wUnits < 3 {
		wUnits = 3
	}
	if hUnits < 3 {
		hUnits = 3
	}

	// Type-specific door count and placement preferences
	var numDoors int
	var preferFrontBack bool // vs all sides equally

	switch buildingType {
	case BuildingTypeResidential:
		// Residential: usually front and back doors
		numDoors = 1
		if wUnits >= 5 || hUnits >= 5 {
			numDoors = 2
		}
		preferFrontBack = true

	case BuildingTypeCommercial:
		// Commercial: front entrance emphasized, maybe service entrance
		numDoors = 1
		if wUnits >= 6 && hUnits >= 6 {
			numDoors = 2 // Main + service entrance
		}
		preferFrontBack = true

	case BuildingTypeIndustrial:
		// Industrial: multiple loading/access doors
		numDoors = 2
		if wUnits >= 8 || hUnits >= 8 {
			numDoors = 3 // Loading dock, office entrance, service door
		}
		preferFrontBack = false

	case BuildingTypeMilitary:
		// Military: minimal entrances (security)
		numDoors = 1
		if wUnits >= 7 && hUnits >= 7 {
			numDoors = 2 // Main + emergency exit
		}
		preferFrontBack = false

	case BuildingTypeAgricultural:
		// Agricultural: large doors for equipment
		numDoors = 1
		if wUnits >= 6 {
			numDoors = 2 // Large barn doors
		}
		preferFrontBack = false

	default:
		// Generic: use original logic
		numDoors = 1
		if wUnits >= 5 || hUnits >= 5 {
			numDoors = 2
		}
		preferFrontBack = false
	}

	// Occasional extra door
	if rng.Float64() < 0.3 {
		numDoors++
	}
	if numDoors > 4 {
		numDoors = 4
	}

	// Choose door faces with type-specific preferences
	type face int
	const (
		faceN face = iota
		faceS
		faceE
		faceW
	)

	var faces []face
	if preferFrontBack {
		// Residential/Commercial prefer N/S doors (front/back)
		faces = []face{faceN, faceS, faceE, faceW}
		// Weight toward N/S by putting them first
		if rng.Intn(2) == 0 {
			faces = []face{faceS, faceN, faceE, faceW} // Sometimes prefer south first
		}
	} else {
		// Industrial/Military/Agricultural use all sides equally
		faces = []face{faceN, faceS, faceE, faceW}
		rng.Shuffle(len(faces), func(i, j int) { faces[i], faces[j] = faces[j], faces[i] })
	}

	doorFaces := make(map[face]bool)
	for i := 0; i < numDoors && i < len(faces); i++ {
		doorFaces[faces[i]] = true
	}

	// Use enhanced building walls with wall type tracking
	g.addBuildingWallsEnhanced(rng, fp, wall, unit, buildingType)

	// Type-specific post-processing could be added here in the future:
	// - Different window patterns per building type
	// - Military firing slits vs residential large windows
	// - Industrial loading dock doors
}

// addBuildingWallsEnhanced generates building walls with proper wall type tracking.
func (g *Game) addBuildingWallsEnhanced(rng *rand.Rand, fp rect, wall, unit int, buildingType BuildingType) {
	// Determine wall types for this building
	exteriorWallType := determineWallType(buildingType, true)
	interiorWallType := determineWallType(buildingType, false)

	// For now, use the existing addBuildingWalls but track the walls we create
	beforeWallCount := len(g.buildings)

	// Call existing building wall generation
	g.addBuildingWalls(rng, fp, wall, unit)

	// Post-process: assign wall types to newly created walls
	// This is a simplified approach - future enhancement would integrate directly into wall placement
	newWalls := g.buildings[beforeWallCount:]

	// Clear the wall info for the new walls and rebuild with proper types
	if len(g.wallInfo) < len(g.buildings) {
		// Extend wallInfo to match buildings length
		for len(g.wallInfo) < beforeWallCount {
			g.wallInfo = append(g.wallInfo, WallInfo{}) // Fill gaps with empty info
		}
	}

	// Analyze each new wall to determine if it's exterior or interior
	for i, wallRect := range newWalls {
		isExterior := isWallExterior(wallRect, fp, wall)
		wallType := exteriorWallType
		if !isExterior {
			wallType = interiorWallType
		}

		g.wallInfo = append(g.wallInfo, WallInfo{
			Rect:       wallRect,
			WallType:   wallType,
			IsExterior: isExterior,
		})

		// Update the wall index to track which building it belongs to
		_ = i // Wall index within this building (could be used for future enhancements)
	}
}

// isWallExterior determines if a wall segment is part of the building's exterior perimeter.
func isWallExterior(wallRect, buildingFootprint rect, wallThickness int) bool {
	// Check if the wall is at the building perimeter
	// Exterior walls are those touching the building footprint edges

	// Check if wall touches any edge of the building footprint
	touchesNorth := wallRect.y <= buildingFootprint.y+wallThickness
	touchesSouth := wallRect.y+wallRect.h >= buildingFootprint.y+buildingFootprint.h-wallThickness
	touchesWest := wallRect.x <= buildingFootprint.x+wallThickness
	touchesEast := wallRect.x+wallRect.w >= buildingFootprint.x+buildingFootprint.w-wallThickness

	return touchesNorth || touchesSouth || touchesWest || touchesEast
}

// classifyBuildingType determines building type based on size and location context.
func classifyBuildingType(fp rect, unit int, rng *rand.Rand) BuildingType {
	wUnits := fp.w / unit
	hUnits := fp.h / unit
	area := wUnits * hUnits

	// Very large buildings (100+ units) are usually industrial
	if area >= 100 {
		if rng.Float64() < 0.7 {
			return BuildingTypeIndustrial // Warehouse/factory
		}
		return BuildingTypeMilitary // Large military facility
	}

	// Large buildings (60-99 units)
	if area >= 60 {
		typeRoll := rng.Float64()
		switch {
		case typeRoll < 0.4:
			return BuildingTypeIndustrial // Warehouse
		case typeRoll < 0.6:
			return BuildingTypeCommercial // Large office/store
		case typeRoll < 0.8:
			return BuildingTypeResidential // Large house/apartment
		default:
			return BuildingTypeMilitary // Barracks/bunker
		}
	}

	// Medium buildings (25-59 units)
	if area >= 25 {
		typeRoll := rng.Float64()
		switch {
		case typeRoll < 0.5:
			return BuildingTypeResidential // House
		case typeRoll < 0.7:
			return BuildingTypeCommercial // Office/shop
		case typeRoll < 0.85:
			return BuildingTypeIndustrial // Small warehouse
		default:
			return BuildingTypeAgricultural // Barn
		}
	}

	// Small buildings (< 25 units)
	typeRoll := rng.Float64()
	switch {
	case typeRoll < 0.6:
		return BuildingTypeResidential // Small house
	case typeRoll < 0.8:
		return BuildingTypeCommercial // Small shop/office
	case typeRoll < 0.9:
		return BuildingTypeAgricultural // Shed/outbuilding
	default:
		return BuildingTypeMilitary // Guard post/bunker
	}
}

// generateLShapedFootprint creates an L-shaped building from a base rectangle.
func generateLShapedFootprint(base rect, rng *rand.Rand) ComplexFootprint {
	// L-shape consists of a primary wing and a secondary wing that meet at a corner

	// Primary wing is the larger section (60-80% of base area)
	primaryRatio := 0.6 + rng.Float64()*0.2 // 0.6 to 0.8

	// Choose L orientation (4 possibilities)
	orientation := rng.Intn(4)

	var primary, secondary rect

	switch orientation {
	case 0: // Primary horizontal on bottom, secondary vertical on right
		primaryH := int(float64(base.h) * primaryRatio)
		primary = rect{x: base.x, y: base.y + base.h - primaryH, w: base.w, h: primaryH}

		secondaryW := int(float64(base.w) * (0.4 + rng.Float64()*0.4)) // 0.4 to 0.8 of width
		secondary = rect{x: base.x + base.w - secondaryW, y: base.y, w: secondaryW, h: base.h - primaryH}

	case 1: // Primary horizontal on top, secondary vertical on right
		primaryH := int(float64(base.h) * primaryRatio)
		primary = rect{x: base.x, y: base.y, w: base.w, h: primaryH}

		secondaryW := int(float64(base.w) * (0.4 + rng.Float64()*0.4))
		secondary = rect{x: base.x + base.w - secondaryW, y: base.y + primaryH, w: secondaryW, h: base.h - primaryH}

	case 2: // Primary horizontal on bottom, secondary vertical on left
		primaryH := int(float64(base.h) * primaryRatio)
		primary = rect{x: base.x, y: base.y + base.h - primaryH, w: base.w, h: primaryH}

		secondaryW := int(float64(base.w) * (0.4 + rng.Float64()*0.4))
		secondary = rect{x: base.x, y: base.y, w: secondaryW, h: base.h - primaryH}

	default: // Primary horizontal on top, secondary vertical on left
		primaryH := int(float64(base.h) * primaryRatio)
		primary = rect{x: base.x, y: base.y, w: base.w, h: primaryH}

		secondaryW := int(float64(base.w) * (0.4 + rng.Float64()*0.4))
		secondary = rect{x: base.x, y: base.y + primaryH, w: secondaryW, h: base.h - primaryH}
	}

	// Ensure minimum sizes for both wings
	if primary.w < 3*16 || primary.h < 3*16 || secondary.w < 3*16 || secondary.h < 3*16 {
		// Fall back to rectangular if L-shape would be too small
		return ComplexFootprint{
			Sections: []rect{base},
			Shape:    ShapeRectangular,
		}
	}

	return ComplexFootprint{
		Sections: []rect{primary, secondary},
		Shape:    ShapeLShaped,
	}
}

// generateTShapedFootprint creates a T-shaped building with main body and perpendicular wing.
func generateTShapedFootprint(base rect, rng *rand.Rand) ComplexFootprint {
	// T-shape consists of a main body (stem) and a perpendicular wing (crossbar)

	// Choose orientation: horizontal T or vertical T
	horizontal := rng.Intn(2) == 0

	var mainBody, crossWing rect

	if horizontal {
		// Horizontal T: main body runs east-west, wing extends north or south
		mainBodyH := int(float64(base.h) * (0.4 + rng.Float64()*0.3)) // 40-70% of height
		mainBody = rect{x: base.x, y: base.y + (base.h-mainBodyH)/2, w: base.w, h: mainBodyH}

		// Wing extends from center of main body
		wingW := int(float64(base.w) * (0.4 + rng.Float64()*0.4)) // 40-80% of width
		wingStart := base.x + (base.w-wingW)/2

		if rng.Intn(2) == 0 {
			// Wing extends north
			crossWing = rect{x: wingStart, y: base.y, w: wingW, h: mainBody.y - base.y}
		} else {
			// Wing extends south
			wingY := mainBody.y + mainBody.h
			crossWing = rect{x: wingStart, y: wingY, w: wingW, h: base.y + base.h - wingY}
		}
	} else {
		// Vertical T: main body runs north-south, wing extends east or west
		mainBodyW := int(float64(base.w) * (0.4 + rng.Float64()*0.3)) // 40-70% of width
		mainBody = rect{x: base.x + (base.w-mainBodyW)/2, y: base.y, w: mainBodyW, h: base.h}

		// Wing extends from center of main body
		wingH := int(float64(base.h) * (0.4 + rng.Float64()*0.4)) // 40-80% of height
		wingStart := base.y + (base.h-wingH)/2

		if rng.Intn(2) == 0 {
			// Wing extends west
			crossWing = rect{x: base.x, y: wingStart, w: mainBody.x - base.x, h: wingH}
		} else {
			// Wing extends east
			wingX := mainBody.x + mainBody.w
			crossWing = rect{x: wingX, y: wingStart, w: base.x + base.w - wingX, h: wingH}
		}
	}

	// Ensure minimum sizes
	if mainBody.w < 3*16 || mainBody.h < 3*16 || crossWing.w < 2*16 || crossWing.h < 2*16 {
		// Fall back to rectangular if T-shape would be too small
		return ComplexFootprint{
			Sections: []rect{base},
			Shape:    ShapeRectangular,
		}
	}

	return ComplexFootprint{
		Sections: []rect{mainBody, crossWing},
		Shape:    ShapeTShaped,
	}
}

// generateUShapedFootprint creates a U-shaped building with central courtyard.
func generateUShapedFootprint(base rect, rng *rand.Rand) ComplexFootprint {
	// U-shape consists of three sections: two side wings and a connecting section
	// Creates a courtyard in the center

	// Choose orientation: U opening north/south or east/west
	horizontal := rng.Intn(2) == 0

	var leftWing, rightWing, connecting rect
	courtyardRatio := 0.3 + rng.Float64()*0.3 // Courtyard takes 30-60% of space

	if horizontal {
		// Horizontal U: opening to north or south
		courtyardW := int(float64(base.w) * courtyardRatio)
		wingW := (base.w - courtyardW) / 2

		// Connecting section (back of U)
		connectingH := int(float64(base.h) * (0.2 + rng.Float64()*0.2)) // 20-40% of height

		if rng.Intn(2) == 0 {
			// U opens to north
			connecting = rect{x: base.x, y: base.y + base.h - connectingH, w: base.w, h: connectingH}

			// Side wings
			wingH := base.h - connectingH
			leftWing = rect{x: base.x, y: base.y, w: wingW, h: wingH}
			rightWing = rect{x: base.x + base.w - wingW, y: base.y, w: wingW, h: wingH}
		} else {
			// U opens to south
			connecting = rect{x: base.x, y: base.y, w: base.w, h: connectingH}

			// Side wings
			wingH := base.h - connectingH
			leftWing = rect{x: base.x, y: base.y + connectingH, w: wingW, h: wingH}
			rightWing = rect{x: base.x + base.w - wingW, y: base.y + connectingH, w: wingW, h: wingH}
		}
	} else {
		// Vertical U: opening to east or west
		courtyardH := int(float64(base.h) * courtyardRatio)
		wingH := (base.h - courtyardH) / 2

		// Connecting section (back of U)
		connectingW := int(float64(base.w) * (0.2 + rng.Float64()*0.2)) // 20-40% of width

		if rng.Intn(2) == 0 {
			// U opens to west
			connecting = rect{x: base.x + base.w - connectingW, y: base.y, w: connectingW, h: base.h}

			// Side wings
			wingW := base.w - connectingW
			leftWing = rect{x: base.x, y: base.y, w: wingW, h: wingH}
			rightWing = rect{x: base.x, y: base.y + base.h - wingH, w: wingW, h: wingH}
		} else {
			// U opens to east
			connecting = rect{x: base.x, y: base.y, w: connectingW, h: base.h}

			// Side wings
			wingW := base.w - connectingW
			leftWing = rect{x: base.x + connectingW, y: base.y, w: wingW, h: wingH}
			rightWing = rect{x: base.x + connectingW, y: base.y + base.h - wingH, w: wingW, h: wingH}
		}
	}

	// Ensure minimum sizes for all sections
	if leftWing.w < 2*16 || leftWing.h < 2*16 ||
		rightWing.w < 2*16 || rightWing.h < 2*16 ||
		connecting.w < 2*16 || connecting.h < 2*16 {
		// Fall back to rectangular if U-shape would be too small
		return ComplexFootprint{
			Sections: []rect{base},
			Shape:    ShapeRectangular,
		}
	}

	return ComplexFootprint{
		Sections: []rect{connecting, leftWing, rightWing},
		Shape:    ShapeUShaped,
	}
}

// GenerateBuildings creates building footprints and populates them with walls, doorways, and windows plus recursive
// internal room subdivision. Windows are evenly spaced along each face,
// proportional to the face length. Only 1-2 faces get exterior doorways.
func (g *Game) addBuildingWalls(rng *rand.Rand, fp rect, wall, unit int) { //nolint:gocognit,gocyclo
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

	// --- Place doors in exterior doorways into TileMap. ---
	for _, f := range []face{faceN, faceS, faceE, faceW} {
		if !doorFaces[f] {
			continue
		}
		var dx, dy int
		switch f {
		case faceN:
			dx, dy = doorPositions[f], y
		case faceS:
			dx, dy = doorPositions[f], y+h-wall
		case faceW:
			dx, dy = x, doorPositions[f]
		case faceE:
			dx, dy = x+w-wall, doorPositions[f]
		}
		placeDoorInDoorway(g.tileMap, rng, dx, dy, unit, true)
	}

	// --- Recursive internal room subdivision (BSP-style). ---
	// Split the interior into rooms. Each room is at least 2×2 units.
	// Partition walls have doorways. Leaf rooms are collected for furnishing.
	type room struct{ rx, ry, rw, rh int }
	interior := room{x + unit, y + unit, w - 2*unit, h - 2*unit}
	var leafRooms []interiorRoom
	var subdivide func(rm room, depth int)
	subdivide = func(rm room, depth int) {
		rmWU := rm.rw / unit
		rmHU := rm.rh / unit
		// Stop if room is too small to split (min 3 units on the split axis).
		if rmWU < 4 && rmHU < 4 {
			leafRooms = append(leafRooms, interiorRoom{rx: rm.rx, ry: rm.ry, rw: rm.rw, rh: rm.rh})
			return
		}
		// Stop probabilistically at deeper levels.
		if depth > 0 && rng.Float64() < 0.05 {
			leafRooms = append(leafRooms, interiorRoom{rx: rm.rx, ry: rm.ry, rw: rm.rw, rh: rm.rh})
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
				leafRooms = append(leafRooms, interiorRoom{rx: rm.rx, ry: rm.ry, rw: rm.rw, rh: rm.rh})
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
			// Place interior door in the doorway gap.
			placeDoorInDoorway(g.tileMap, rng, px, doorY, unit, false)
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
				leafRooms = append(leafRooms, interiorRoom{rx: rm.rx, ry: rm.ry, rw: rm.rw, rh: rm.rh})
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
			// Place interior door in the doorway gap.
			placeDoorInDoorway(g.tileMap, rng, doorX, py, unit, false)
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
	switch {
	case wUnits >= 4 && hUnits >= 4:
		// For larger buildings (6+ units on both axes), 30% chance to use hallway layout.
		if wUnits >= 6 && hUnits >= 6 && rng.Float64() < 0.30 {
			g.generateHallwayLayout(rng, rect{x: interior.rx, y: interior.ry, w: interior.rw, h: interior.rh}, wall, unit, &leafRooms)
		} else {
			subdivide(interior, 0)
		}
	case wUnits >= 4 || hUnits >= 4:
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
				placeDoorInDoorway(g.tileMap, rng, px, doorY, unit, false)
				// Collect the two resulting rooms.
				leftW := px - (x + unit)
				rightW := (x + w - unit) - (px + wall)
				if leftW > 0 {
					leafRooms = append(leafRooms, interiorRoom{rx: x + unit, ry: y + unit, rw: leftW, rh: h - 2*unit})
				}
				if rightW > 0 {
					leafRooms = append(leafRooms, interiorRoom{rx: px + wall, ry: y + unit, rw: rightW, rh: h - 2*unit})
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
				placeDoorInDoorway(g.tileMap, rng, doorX, py, unit, false)
				// Collect the two resulting rooms.
				topH := py - (y + unit)
				bottomH := (y + h - unit) - (py + wall)
				if topH > 0 {
					leafRooms = append(leafRooms, interiorRoom{rx: x + unit, ry: y + unit, rw: w - 2*unit, rh: topH})
				}
				if bottomH > 0 {
					leafRooms = append(leafRooms, interiorRoom{rx: x + unit, ry: py + wall, rw: w - 2*unit, rh: bottomH})
				}
			}
		} else {
			// No partition — whole interior is one room.
			leafRooms = append(leafRooms, interiorRoom{rx: x + unit, ry: y + unit, rw: w - 2*unit, rh: h - 2*unit})
		}
	default:
		// Small building — whole interior is one room.
		leafRooms = append(leafRooms, interiorRoom{rx: x + unit, ry: y + unit, rw: w - 2*unit, rh: h - 2*unit})
	}

	// --- Furnish interior rooms ---
	furnishBuilding(g.tileMap, rng, fp, leafRooms)
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

// findValidSpawnLocation searches for a walkable spawn position near the desired location.
// Returns the validated position or the original if no valid position found within search radius.
func (g *Game) findValidSpawnLocation(x, y, searchRadius float64) (float64, float64) {
	cx, cy := WorldToCell(x, y)
	if !g.navGrid.IsBlocked(cx, cy) {
		return x, y
	}

	// Spiral search outward for valid cell
	maxOffset := int(searchRadius / cellSize)
	for radius := 1; radius <= maxOffset; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				if dx*dx+dy*dy > radius*radius {
					continue
				}
				testCX := cx + dx
				testCY := cy + dy
				if !g.navGrid.IsBlocked(testCX, testCY) {
					return CellToWorld(testCX, testCY)
				}
			}
		}
	}

	// Fallback to original position
	return x, y
}

// spawnCluster creates squadSize soldiers of the given team at startX,
// spread vertically around clusterCenterY. Returns the created soldiers.
func (g *Game) spawnCluster(rng *rand.Rand, team Team, clusterCenterY, startX, endX float64) []*Soldier {
	squadSize := 8
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

		// Validate spawn position is walkable
		validX, validY := g.findValidSpawnLocation(startX, y, 48.0)

		id := g.nextID
		g.nextID++
		s := NewSoldier(id, validX, validY, team,
			[2]float64{validX, validY}, [2]float64{endX, validY},
			g.navGrid, g.covers, g.buildings, g.thoughtLog, &g.tick, g.tacticalMap)
		s.losIndex = g.losIndex
		s.buildingFootprints = g.buildingFootprints
		s.tileMap = g.tileMap
		s.blackboard.ClaimedBuildingIdx = -1
		if s.path != nil {
			out = append(out, s)
		}
	}
	return out
}

func (g *Game) initSoldiers() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404
	margin := 64.0
	startX := margin
	endX := float64(g.gameWidth) - margin
	// Spawn 3 squads with better vertical distribution
	g.soldiers = append(g.soldiers, g.spawnCluster(rng, TeamRed, float64(g.gameHeight)*0.20, startX, endX)...)
	g.soldiers = append(g.soldiers, g.spawnCluster(rng, TeamRed, float64(g.gameHeight)*0.50, startX, endX)...)
	g.soldiers = append(g.soldiers, g.spawnCluster(rng, TeamRed, float64(g.gameHeight)*0.80, startX, endX)...)
}

func (g *Game) initOpFor() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 999)) // #nosec G404
	margin := 64.0
	startX := float64(g.gameWidth) - margin
	endX := margin
	// Spawn 3 squads with better vertical distribution
	g.opfor = append(g.opfor, g.spawnCluster(rng, TeamBlue, float64(g.gameHeight)*0.20, startX, endX)...)
	g.opfor = append(g.opfor, g.spawnCluster(rng, TeamBlue, float64(g.gameHeight)*0.50, startX, endX)...)
	g.opfor = append(g.opfor, g.spawnCluster(rng, TeamBlue, float64(g.gameHeight)*0.80, startX, endX)...)
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
		sq.buildingQualities = g.buildingQualities
		sq.InitializeFlowField(g.navGrid, g.tacticalMap)
		g.squads = append(g.squads, sq)
	}
	for i := 0; i < len(g.opfor); i += sqSz {
		end := i + sqSz
		if end > len(g.opfor) {
			end = len(g.opfor)
		}
		sq := NewSquad(len(g.squads), TeamBlue, g.opfor[i:end])
		sq.buildingFootprints = g.buildingFootprints
		sq.buildingQualities = g.buildingQualities
		sq.InitializeFlowField(g.navGrid, g.tacticalMap)
		g.squads = append(g.squads, sq)
	}

	// Initialize steering behaviors for all soldiers
	for _, s := range g.soldiers {
		s.steeringBehavior = NewSteeringBehavior(s)
	}
	for _, s := range g.opfor {
		s.steeringBehavior = NewSteeringBehavior(s)
	}
}

func (g *Game) allUnits() []*Soldier {
	g.allSoldiers = g.allSoldiers[:0]
	g.allSoldiers = append(g.allSoldiers, g.soldiers...)
	g.allSoldiers = append(g.allSoldiers, g.opfor...)
	return g.allSoldiers
}

func (g *Game) frameKeys() map[ebiten.Key]bool {
	clear(g.currentKeys)
	return g.currentKeys
}

func (g *Game) rememberKeys(currentKeys map[ebiten.Key]bool) {
	clear(g.prevKeys)
	for key, pressed := range currentKeys {
		if pressed {
			g.prevKeys[key] = true
		}
	}
}

// randomiseProfiles gives each soldier slightly different stats so behavior varies.
func (g *Game) randomiseProfiles() {
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + 42)) // #nosec G404 -- game only, crypto/rand not needed
	all := make([]*Soldier, 0, len(g.soldiers)+len(g.opfor))
	all = append(all, g.soldiers...)
	all = append(all, g.opfor...)
	for _, s := range all {
		p := &s.profile
		p.Physical.FitnessBase = 0.4 + rng.Float64()*0.5 // 0.4 - 0.9
		p.Skills.Marksmanship = 0.2 + rng.Float64()*0.6  // 0.2 - 0.8
		p.Skills.Fieldcraft = 0.2 + rng.Float64()*0.6
		p.Skills.Discipline = 0.3 + rng.Float64()*0.6 // 0.3 - 0.9
		p.Psych.Experience = rng.Float64() * 0.5      // 0.0 - 0.5
		p.Psych.Morale = 0.5 + rng.Float64()*0.4      // 0.5 - 0.9
		p.Psych.Composure = 0.3 + rng.Float64()*0.5   // 0.3 - 0.8

		// Initialize commitment-based decision thresholds from full profile.
		s.blackboard.InitCommitmentWithProfile(p)
	}
}

// Update advances simulation and handles per-frame logic.
func (g *Game) Update() error {
	// Handle input every frame regardless of sim speed.
	g.handleInput()
	if g.pendingExit != nil {
		return g.pendingExit
	}

	now := time.Now()
	if g.lastUpdateTime.IsZero() {
		g.lastUpdateTime = now
	}
	_ = now.Sub(g.lastUpdateTime)
	g.lastUpdateTime = now

	if g.simSpeed <= 0 {
		// Paused: still update tracers so muzzle flashes fade.
		g.combat.UpdateTracers()
		if g.autoFrameCamera {
			g.applyAutoFrameCamera(true)
		}
		return nil
	}

	// For speeds > 1 run multiple sim ticks per frame.
	// For speeds < 1 accumulate fractions.
	g.tickAccum += g.simSpeed
	for g.tickAccum >= 1.0 {
		g.tickAccum -= 1.0
		simStart := time.Now()
		g.simTick()
		simNS := time.Since(simStart).Nanoseconds()
		g.perfSimTickRuns++
		g.perfSimTickNS += simNS
		g.perfTotalSimTickRuns++
		g.perfTotalSimTickNS += simNS
	}

	// Update interpolation for smooth sub-tick rendering.
	// tickAccum represents progress toward the next tick [0, 1).
	g.interpolation = g.tickAccum

	// Update tracer fractional ages for smooth rendering every frame.
	g.combat.UpdateTracerInterpolation(g.interpolation)

	if g.autoFrameCamera {
		g.applyAutoFrameCamera(true)
	}

	g.updateAutoPerfCapture(now)
	if g.pendingExit != nil {
		return g.pendingExit
	}

	return nil
}

// simTick runs one simulation tick.
func (g *Game) simTick() { //nolint:gocyclo
	g.tick++

	// 0. SPATIAL HASH: populate spatial partitioning structures for efficient queries.
	g.spatialHashRed.Clear()
	g.spatialHashBlue.Clear()
	for _, s := range g.soldiers {
		if s.state != SoldierStateDead {
			g.spatialHashRed.Insert(s)
		}
	}
	for _, s := range g.opfor {
		if s.state != SoldierStateDead {
			g.spatialHashBlue.Insert(s)
		}
	}

	// 1. SENSE: each soldier scans for enemies using spatial hash.
	for _, s := range g.soldiers {
		s.UpdateVisionSpatial(g.spatialHashBlue, g.buildings)
	}
	for _, s := range g.opfor {
		s.UpdateVisionSpatial(g.spatialHashRed, g.buildings)
	}

	// 2. COMBAT: fire decisions and resolution.
	all := g.allUnits()
	g.combat.ResetFireCounts(all)
	g.combat.tick = g.tick
	g.combat.ResolveCombat(g.soldiers, g.opfor, g.soldiers, g.buildings, all)
	g.combat.ResolveCombat(g.opfor, g.soldiers, g.opfor, g.buildings, all)
	g.combat.UpdateTracers()

	// 2.1. SOUND: broadcast gunfire events using spatial hash for performance.
	g.combat.BroadcastGunfireSpatial(g.spatialHashRed, g.spatialHashBlue, g.soldiers, g.opfor, g.tick)

	// 2.5. INTEL: update all heatmap layers from current soldier state.
	g.intel.Update(g.soldiers, g.opfor, g.buildings)

	// 3. SQUAD THINK: leaders evaluate and set intent/orders.
	for _, sq := range g.squads {
		sq.SquadThink(g.intel)
	}

	// 3.5 + 3.6 COMMS PLAN/RESOLVE: phase-A squad radio messaging.
	for _, sq := range g.squads {
		sq.PlanComms(g.tick)
		sq.ResolveComms(g.tick, g.thoughtLog)
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

	// 5.5. MEDICAL AID: advance treatment for wounded soldiers.
	integrateBuddyAidTick(g.soldiers, g.tick)
	integrateBuddyAidTick(g.opfor, g.tick)

	// 6. SQUAD POLL: periodic summary to thought log (~every 5s).
	if g.tick%squadPollInterval == 0 {
		for _, sq := range g.squads {
			g.thoughtLog.AddSquadPoll(g.tick, sq)
		}
	}

	// 7. SPEECH: update soldier speech bubbles.
	g.UpdateSpeech(g.speechRng)

	// 8. ANALYTICS: collect behavior report every ~1s.
	if g.tick%60 == 0 && g.reporter != nil {
		g.reporter.Collect(g.tick, g.soldiers, g.opfor, g.squads)
	}

	if !g.aarOpen {
		g.checkCombatEnd()
	}
}

func (g *Game) checkCombatEnd() {
	redSquads := make([]*Squad, 0, len(g.squads))
	blueSquads := make([]*Squad, 0, len(g.squads))
	for _, sq := range g.squads {
		if sq == nil {
			continue
		}
		switch sq.Team {
		case TeamRed:
			redSquads = append(redSquads, sq)
		case TeamBlue:
			blueSquads = append(blueSquads, sq)
		}
	}

	reason := DetermineBattleOutcome(g.soldiers, g.opfor, redSquads, blueSquads)
	if reason.Outcome == OutcomeInconclusive {
		return
	}

	g.aarOpen = true
	g.aarSelection = 0
	g.aarReason = reason
	g.menuOpen = false
	g.simSpeed = 0
}

// handleInput processes overlay toggle keypresses (edge-triggered).
func (g *Game) handleInput() { //nolint:gocognit,gocyclo
	currentKeys := g.frameKeys()
	if g.aarOpen {
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
			g.aarSelection = (g.aarSelection + 2) % 3
		}
		if moveDown {
			g.aarSelection = (g.aarSelection + 1) % 3
		}

		if currentKeys[ebiten.KeyEnter] && !g.prevKeys[ebiten.KeyEnter] {
			switch g.aarSelection {
			case 0:
				g.pendingExit = ErrRestart
			case 1:
				g.pendingExit = ErrQuit
			case 2:
				g.aarOpen = false
			}
		}

		g.rememberKeys(currentKeys)
		g.prevMouseLeft = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		return
	}

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

		g.rememberKeys(currentKeys)
		g.prevMouseLeft = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
		return
	}

	// Layer toggles for active team: 1-9.
	layerKeys := [intelMapCount]ebiten.Key{
		ebiten.Key1, ebiten.Key2, ebiten.Key3,
		ebiten.Key4, ebiten.Key5, ebiten.Key6,
		ebiten.Key7, ebiten.Key8, ebiten.Key9,
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

	// F5-F8: toggle log category filters.
	filterKeys := [logCatCount]ebiten.Key{ebiten.KeyF5, ebiten.KeyF6, ebiten.KeyF7, ebiten.KeyF8}
	filterCats := [logCatCount]LogCategory{LogCatRadio, LogCatSquad, LogCatSpeech, LogCatThought}
	for i, fk := range filterKeys {
		currentKeys[fk] = ebiten.IsKeyPressed(fk)
		if currentKeys[fk] && !g.prevKeys[fk] {
			g.thoughtLog.ToggleFilter(filterCats[i])
		}
	}

	if !g.autoFrameCamera {
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
		g.clampCameraToBattlefield()
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

	g.rememberKeys(currentKeys)
}

func (g *Game) clampCameraToBattlefield() {
	if g.camZoom <= 0 {
		g.camZoom = 0.01
	}
	mapW := float64(g.gameWidth)
	mapH := float64(g.gameHeight)
	halfVW := mapW / 2 / g.camZoom
	halfVH := mapH / 2 / g.camZoom

	if halfVW >= mapW/2 {
		g.camX = mapW / 2
	} else {
		if g.camX < halfVW {
			g.camX = halfVW
		}
		if g.camX > mapW-halfVW {
			g.camX = mapW - halfVW
		}
	}
	if halfVH >= mapH/2 {
		g.camY = mapH / 2
	} else {
		if g.camY < halfVH {
			g.camY = halfVH
		}
		if g.camY > mapH-halfVH {
			g.camY = mapH - halfVH
		}
	}
}

func (g *Game) computeBattleframeCamera() (camX, camY, camZoom float64, ok bool) {
	all := g.allUnits()
	if len(all) == 0 {
		return 0, 0, 0, false
	}

	minX := math.MaxFloat64
	minY := math.MaxFloat64
	maxX := -math.MaxFloat64
	maxY := -math.MaxFloat64
	haveAlive := false
	pad := g.autoFramePadding + float64(soldierRadius)
	for _, s := range all {
		if s == nil || s.state == SoldierStateDead {
			continue
		}
		haveAlive = true
		if x0 := s.x - pad; x0 < minX {
			minX = x0
		}
		if y0 := s.y - pad; y0 < minY {
			minY = y0
		}
		if x1 := s.x + pad; x1 > maxX {
			maxX = x1
		}
		if y1 := s.y + pad; y1 > maxY {
			maxY = y1
		}
	}
	if !haveAlive {
		return 0, 0, 0, false
	}

	const minSpan = 220.0
	spanX := math.Max(minSpan, maxX-minX)
	spanY := math.Max(minSpan, maxY-minY)

	zoomX := float64(g.gameWidth) / spanX
	zoomY := float64(g.gameHeight) / spanY
	targetZoom := math.Min(zoomX, zoomY)
	const minAutoZoom, maxAutoZoom = 0.18, 4.0
	if targetZoom < minAutoZoom {
		targetZoom = minAutoZoom
	}
	if targetZoom > maxAutoZoom {
		targetZoom = maxAutoZoom
	}

	return (minX + maxX) * 0.5, (minY + maxY) * 0.5, targetZoom, true
}

func (g *Game) applyAutoFrameCamera(smooth bool) {
	targetX, targetY, targetZoom, ok := g.computeBattleframeCamera()
	if !ok {
		return
	}
	if !smooth {
		g.camX = targetX
		g.camY = targetY
		g.camZoom = targetZoom
		g.clampCameraToBattlefield()
		return
	}

	const posLerp = 0.18
	const zoomLerp = 0.14
	g.camX += (targetX - g.camX) * posLerp
	g.camY += (targetY - g.camY) * posLerp
	g.camZoom += (targetZoom - g.camZoom) * zoomLerp
	g.clampCameraToBattlefield()
}

// Draw renders one frame of world and UI.
func (g *Game) Draw(screen *ebiten.Image) {
	// Window background: very dark, outside battlefield.
	screen.Fill(color.RGBA{R: 12, G: 14, B: 12, A: 255})
	if false {
		g.drawFormationSlots(screen)
	}

	// Render world content to worldBuf at (0,0) origin, then blit with camera transform.
	worldStart := time.Now()
	g.worldBuf.Clear()
	g.drawWorld(g.worldBuf)
	worldNS := time.Since(worldStart).Nanoseconds()
	g.perfWorldDrawNS += worldNS
	g.perfTotalWorldDrawNS += worldNS

	// Camera transform: translate so camX/camY is at viewport center, then scale.
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

	// Right-side column: squad status panels on top, thought log below.
	uiStart := time.Now()
	logX := g.offX + g.gameWidth + g.offX
	squadAreaH := drawSquadStatusPanels(g, screen, logX)

	// Thought log panel — rendered to scaled buffer, then blitted below squad panels.
	logBufW := logPanelWidth / logScale
	logAvailH := g.height - squadAreaH
	if logAvailH < logScale*20 {
		logAvailH = logScale * 20
	}
	logBufH := logAvailH / logScale
	g.thoughtLog.Draw(g.logBuf, logBufW, logBufH)
	logOpts := &ebiten.DrawImageOptions{}
	logOpts.GeoM.Scale(float64(logScale), float64(logScale))
	logOpts.GeoM.Translate(float64(logX), float64(squadAreaH))
	screen.DrawImage(g.logBuf, logOpts)

	// HUD key legend.
	if g.showHUD {
		g.drawHUD(screen)
	}
	uiNS := time.Since(uiStart).Nanoseconds()
	g.perfUIDrawNS += uiNS
	g.perfTotalUIDrawNS += uiNS
	g.recordPerfFrame()
	g.perfTotalFrameCount++

	// Soldier inspector panel (screen-space, drawn over everything).
	g.drawInspector(screen)

	if g.menuOpen {
		g.drawPauseMenu(screen)
	}
	if g.aarOpen {
		g.drawAAR(screen)
	}
}

func (g *Game) aarTitle() string {
	switch g.aarReason.Outcome {
	case OutcomeRedVictory:
		return "RED VICTORY"
	case OutcomeBlueVictory:
		return "BLUE VICTORY"
	case OutcomeDraw:
		return "DRAW"
	default:
		return "INCONCLUSIVE"
	}
}

func (g *Game) aarQuality() string { //nolint:gocyclo
	redLoss := 0
	blueLoss := 0
	if g.aarReason.RedTotal > 0 {
		redLoss = g.aarReason.RedTotal - g.aarReason.RedSurvivors
	}
	if g.aarReason.BlueTotal > 0 {
		blueLoss = g.aarReason.BlueTotal - g.aarReason.BlueSurvivors
	}
	redCas := 0.0
	blueCas := 0.0
	if g.aarReason.RedTotal > 0 {
		redCas = float64(redLoss) / float64(g.aarReason.RedTotal)
	}
	if g.aarReason.BlueTotal > 0 {
		blueCas = float64(blueLoss) / float64(g.aarReason.BlueTotal)
	}

	switch g.aarReason.Outcome {
	case OutcomeRedVictory:
		switch {
		case redCas < 0.15 && blueCas > 0.75:
			return "DECISIVE"
		case redCas < 0.35:
			return "CLEAR"
		case redCas < 0.55:
			return "COSTLY"
		default:
			return "PYRRHIC"
		}
	case OutcomeBlueVictory:
		switch {
		case blueCas < 0.15 && redCas > 0.75:
			return "DECISIVE"
		case blueCas < 0.35:
			return "CLEAR"
		case blueCas < 0.55:
			return "COSTLY"
		default:
			return "PYRRHIC"
		}
	case OutcomeDraw:
		switch {
		case redCas > 0.55 && blueCas > 0.55:
			return "BLOODY"
		case redCas > 0.25 || blueCas > 0.25:
			return "CONTESTED"
		default:
			return "STALEMATE"
		}
	default:
		return ""
	}
}

func (g *Game) drawAAR(screen *ebiten.Image) {
	vector.FillRect(screen, 0, 0, float32(g.width), float32(g.height), color.RGBA{R: 0, G: 0, B: 0, A: 200}, false)

	const panelW = 520
	const panelH = 280
	px := (g.width - panelW) / 2
	py := (g.height - panelH) / 2

	vector.FillRect(screen, float32(px), float32(py), panelW, panelH, color.RGBA{R: 12, G: 18, B: 12, A: 245}, false)
	vector.StrokeRect(screen, float32(px), float32(py), panelW, panelH, 2, color.RGBA{R: 92, G: 140, B: 92, A: 255}, false)

	title := g.aarTitle()
	quality := g.aarQuality()
	if quality != "" {
		title = title + " — " + quality
	}

	ebitenutil.DebugPrintAt(screen, "AFTER ACTION REPORT", px+170, py+18)
	ebitenutil.DebugPrintAt(screen, title, px+170, py+40)

	redLoss := g.aarReason.RedTotal - g.aarReason.RedSurvivors
	blueLoss := g.aarReason.BlueTotal - g.aarReason.BlueSurvivors
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("RED:  %d/%d losses  squads_broken=%d/%d", redLoss, g.aarReason.RedTotal, g.aarReason.RedSquadsBroken, g.aarReason.RedSquadsTotal), px+30, py+82)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("BLUE: %d/%d losses  squads_broken=%d/%d", blueLoss, g.aarReason.BlueTotal, g.aarReason.BlueSquadsBroken, g.aarReason.BlueSquadsTotal), px+30, py+98)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("reason: %s", g.aarReason.Description), px+30, py+126)

	ebitenutil.DebugPrintAt(screen, "W/S or Up/Down: select", px+30, py+156)
	ebitenutil.DebugPrintAt(screen, "Enter: confirm", px+30, py+170)

	options := []string{"Restart (New Seed)", "Quit Program", "Close AAR"}
	for i, label := range options {
		prefix := "  "
		if i == g.aarSelection {
			prefix = "> "
		}
		ebitenutil.DebugPrintAt(screen, prefix+label, px+150, py+206+i*20)
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

func (g *Game) drawWorld(screen *ebiten.Image) { //nolint:gocognit,gocyclo
	// For drawWorld, all coordinates are in world-space (no offX/offY needed).
	ox, oy := float32(0), float32(0)

	// Camera-visible world bounds for draw culling.
	halfVW := float64(g.gameWidth) / 2 / g.camZoom
	halfVH := float64(g.gameHeight) / 2 / g.camZoom
	minX := g.camX - halfVW
	maxX := g.camX + halfVW
	minY := g.camY - halfVH
	maxY := g.camY + halfVH

	// Small pad to avoid edge pop-in for thick strokes/glows.
	const cullPad = 64.0
	minX -= cullPad
	minY -= cullPad
	maxX += cullPad
	maxY += cullPad

	// Vision cones: drawn early so buildings and units sit on top.
	// Rendered into an offscreen buffer to avoid additive blowout.
	g.drawVisionConesBuffered(screen, g.soldiers, color.RGBA{R: 200, G: 60, B: 40, A: 35}, 0.12)
	g.drawVisionConesBuffered(screen, g.opfor, color.RGBA{R: 40, G: 80, B: 200, A: 35}, 0.12)

	g.drawTerrainLayerVisible(screen, minX, minY, maxX, maxY)

	// Intel heatmap overlays (drawn under buildings and soldiers).
	team := Team(g.overlayTeam)
	const minHeatOverlayZoom = 0.35
	if g.camZoom >= minHeatOverlayZoom {
		if im := g.intel.For(team); im != nil {
			for k := IntelMapKind(0); k < intelMapCount; k++ {
				if g.showOverlay[g.overlayTeam][k] {
					g.drawHeatLayer(screen, im.Layer(k), overlayColors[k], minX, minY, maxX, maxY)
				}
			}
		}
	}

	// Build a set of claimed building indices → team for tinting.
	// Clear and reuse cached map to avoid per-frame allocation.
	for k := range g.cachedClaimedTeam {
		delete(g.cachedClaimedTeam, k)
	}
	for _, sq := range g.squads {
		if sq.ClaimedBuildingIdx >= 0 {
			g.cachedClaimedTeam[sq.ClaimedBuildingIdx] = sq.Team
		}
	}
	claimedTeam := g.cachedClaimedTeam

	// Building interiors: floor first, then walls on top.
	// Floor areas (from footprints) — dark interior.
	for i, b := range g.buildingFootprints {
		if float64(b.x+b.w) < minX || float64(b.x) > maxX || float64(b.y+b.h) < minY || float64(b.y) > maxY {
			continue
		}
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
			// Thin team-color border.
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
	// Clear and reuse cached map to avoid per-frame allocation.
	for k := range g.cachedSolidSet {
		delete(g.cachedSolidSet, k)
	}
	for _, b := range g.buildings {
		g.cachedSolidSet[[2]int{b.x / cellSize, b.y / cellSize}] = true
	}
	for _, w := range g.windows {
		g.cachedSolidSet[[2]int{w.x / cellSize, w.y / cellSize}] = true
	}
	solidSet := g.cachedSolidSet

	// Wall segments — neighbor-aware lighting so edges/corners read correctly.
	wallFill := color.RGBA{R: 86, G: 80, B: 66, A: 255}
	wallLight := color.RGBA{R: 124, G: 115, B: 96, A: 210}
	wallDark := color.RGBA{R: 44, G: 40, B: 33, A: 220}
	for _, b := range g.buildings {
		if float64(b.x+b.w) < minX || float64(b.x) > maxX || float64(b.y+b.h) < minY || float64(b.y) > maxY {
			continue
		}
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
		if float64(w.x+w.w) < minX || float64(w.x) > maxX || float64(w.y+w.h) < minY || float64(w.y) > maxY {
			continue
		}
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

	// TileMap interior objects (doors, furniture, pillars, crates).
	g.drawTileMapObjects(screen, ox, oy, minX, minY, maxX, maxY)

	// Cover objects.
	g.drawCoverObjects(screen, 0, 0, minX, minY, maxX, maxY)

	for _, s := range g.soldiers {
		if s.state == SoldierStateDead || s.x < minX || s.x > maxX || s.y < minY || s.y > maxY {
			continue
		}
		s.Draw(screen, 0, 0)
	}
	for _, s := range g.opfor {
		if s.state == SoldierStateDead || s.x < minX || s.x > maxX || s.y < minY || s.y > maxY {
			continue
		}
		s.Draw(screen, 0, 0)
	}

	// Radio transmission arcs (transient comms visual effects).
	g.drawRadioVisualEffects(screen)

	if g.camZoom > 0.75 {
		// Movement intent lines: faint dashed line from soldier to path endpoint.
		g.drawMovementIntentLines(screen)

		// Active officer orders (leader-issued command markers).
		g.drawOfficerOrders(screen)
	}

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

	if g.camZoom > 0.75 {
		// Squad intent labels near squad leaders (world-space).
		g.drawSquadIntentLabels(screen)

		// Spotted indicator.
		g.drawSpottedIndicators(screen, 0, 0)
	}

	// Edge vignette — deeper for more atmosphere.
	if g.vignetteBuf != nil {
		screen.DrawImage(g.vignetteBuf, nil)
	} else {
		g.drawVignette(screen, 0, 0)
	}
}

func (g *Game) renderVignetteLayer(screen *ebiten.Image) {
	screen.Clear()
	g.drawVignette(screen, 0, 0)
}

// drawTileMapObjects renders interior objects from the TileMap (doors, furniture,
// pillars, crates) that aren't already drawn by the wall/window/cover renderers.
func (g *Game) drawTileMapObjects(screen *ebiten.Image, ox, oy float32, minX, minY, maxX, maxY float64) { //nolint:gocyclo
	if g.tileMap == nil {
		return
	}
	cs := float32(cellSize)
	minCol, minRow := WorldToCell(minX, minY)
	maxCol, maxRow := WorldToCell(maxX, maxY)
	if minCol < 0 {
		minCol = 0
	}
	if minRow < 0 {
		minRow = 0
	}
	if maxCol >= g.tileMap.Cols {
		maxCol = g.tileMap.Cols - 1
	}
	if maxRow >= g.tileMap.Rows {
		maxRow = g.tileMap.Rows - 1
	}
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			obj := g.tileMap.ObjectAt(col, row)
			px := ox + float32(col)*cs
			py := oy + float32(row)*cs

			switch obj {
			case ObjectDoor:
				// Closed door — brown rect with darker frame.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 72, G: 52, B: 32, A: 255}, false)
				vector.StrokeRect(screen, px+1, py+1, cs-2, cs-2, 0.8, color.RGBA{R: 50, G: 36, B: 22, A: 220}, false)
				// Door handle hint.
				vector.FillRect(screen, px+cs*0.7, py+cs*0.45, 2, 2, color.RGBA{R: 140, G: 130, B: 100, A: 200}, false)

			case ObjectDoorOpen:
				// Open door — thin line along one wall edge.
				vector.StrokeLine(screen, px, py, px, py+cs, 1.5, color.RGBA{R: 72, G: 52, B: 32, A: 180}, false)

			case ObjectDoorBroken:
				// Broken door — just scattered debris marks.
				vector.FillRect(screen, px+2, py+cs*0.3, 3, 2, color.RGBA{R: 60, G: 44, B: 28, A: 120}, false)
				vector.FillRect(screen, px+cs*0.5, py+cs*0.6, 4, 2, color.RGBA{R: 55, G: 40, B: 25, A: 100}, false)

			case ObjectPillar:
				// Structural column — dark filled square, slightly inset.
				inset := float32(2)
				vector.FillRect(screen, px+inset, py+inset, cs-inset*2, cs-inset*2, color.RGBA{R: 70, G: 66, B: 58, A: 255}, false)
				// Light edge top-left, dark edge bottom-right.
				vector.StrokeLine(screen, px+inset, py+inset, px+cs-inset, py+inset, 0.8, color.RGBA{R: 100, G: 94, B: 82, A: 200}, false)
				vector.StrokeLine(screen, px+inset, py+inset, px+inset, py+cs-inset, 0.8, color.RGBA{R: 100, G: 94, B: 82, A: 200}, false)
				vector.StrokeLine(screen, px+cs-inset, py+inset, px+cs-inset, py+cs-inset, 0.8, color.RGBA{R: 44, G: 40, B: 34, A: 200}, false)
				vector.StrokeLine(screen, px+inset, py+cs-inset, px+cs-inset, py+cs-inset, 0.8, color.RGBA{R: 44, G: 40, B: 34, A: 200}, false)

			case ObjectTable:
				// Table — brown filled rect, edge highlights.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 82, G: 60, B: 36, A: 240}, false)
				vector.StrokeLine(screen, px+1, py+1, px+cs-1, py+1, 0.5, color.RGBA{R: 105, G: 80, B: 50, A: 180}, false)
				vector.StrokeLine(screen, px+1, py+cs-1, px+cs-1, py+cs-1, 0.5, color.RGBA{R: 55, G: 40, B: 24, A: 180}, false)

			case ObjectChair:
				// Chair — smaller lighter rect.
				inset := float32(3)
				vector.FillRect(screen, px+inset, py+inset, cs-inset*2, cs-inset*2, color.RGBA{R: 90, G: 70, B: 46, A: 220}, false)

			case ObjectCrate:
				// Wooden crate — tan/brown with cross-bracing.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 76, G: 62, B: 42, A: 255}, false)
				vector.StrokeRect(screen, px+1, py+1, cs-2, cs-2, 0.8, color.RGBA{R: 56, G: 44, B: 28, A: 220}, false)
				// Cross bracing.
				vector.StrokeLine(screen, px+2, py+2, px+cs-2, py+cs-2, 0.5, color.RGBA{R: 56, G: 44, B: 28, A: 150}, false)
				vector.StrokeLine(screen, px+cs-2, py+2, px+2, py+cs-2, 0.5, color.RGBA{R: 56, G: 44, B: 28, A: 150}, false)

			case ObjectWindowBroken:
				// Broken window — empty frame with glass shards hint.
				vector.FillRect(screen, px, py, cs, cs, color.RGBA{R: 50, G: 55, B: 60, A: 160}, false)
				vector.StrokeLine(screen, px+2, py+cs*0.3, px+cs*0.4, py+cs*0.7, 0.5, color.RGBA{R: 100, G: 130, B: 160, A: 100}, false)
				vector.StrokeLine(screen, px+cs*0.6, py+2, px+cs*0.8, py+cs*0.5, 0.5, color.RGBA{R: 100, G: 130, B: 160, A: 80}, false)

			case ObjectTreeTrunk:
				// Tree trunk — dark brown circle-ish square.
				inset := float32(4)
				vector.FillRect(screen, px+inset, py+inset, cs-inset*2, cs-inset*2, color.RGBA{R: 50, G: 36, B: 22, A: 255}, false)
				// Bark texture lines.
				vector.StrokeLine(screen, px+cs*0.4, py+inset, px+cs*0.4, py+cs-inset, 0.5, color.RGBA{R: 40, G: 28, B: 16, A: 150}, false)
				vector.StrokeLine(screen, px+cs*0.6, py+inset, px+cs*0.6, py+cs-inset, 0.5, color.RGBA{R: 60, G: 44, B: 28, A: 120}, false)

			case ObjectTreeCanopy:
				// Canopy — translucent green with variation.
				h := terrainHash(col, row)
				gVar := int(h%12) - 6
				vector.FillRect(screen, px, py, cs, cs, color.RGBA{R: 20, G: clampToByte(55 + gVar), B: 18, A: 140}, false)
				// Leaf detail.
				if h%3 == 0 {
					vector.FillRect(screen, px+2, py+2, 3, 3, color.RGBA{R: 25, G: clampToByte(65 + gVar), B: 20, A: 100}, false)
				}

			case ObjectBush:
				// Bush — small green blob.
				inset := float32(2)
				h := terrainHash(col, row)
				gVar := int(h%10) - 5
				vector.FillRect(screen, px+inset, py+inset, cs-inset*2, cs-inset*2, color.RGBA{R: 28, G: clampToByte(52 + gVar), B: 24, A: 210}, false)
				// Highlight.
				vector.FillRect(screen, px+inset+1, py+inset+1, cs*0.4, cs*0.3, color.RGBA{R: 35, G: clampToByte(68 + gVar), B: 30, A: 120}, false)

			case ObjectHedgerow:
				// Hedgerow — dense green fill, darker than bushes.
				h := terrainHash(col, row)
				gVar := int(h%8) - 4
				vector.FillRect(screen, px, py, cs, cs, color.RGBA{R: 22, G: clampToByte(44 + gVar), B: 20, A: 240}, false)
				// Top highlight stripe.
				vector.StrokeLine(screen, px, py+1, px+cs, py+1, 0.8, color.RGBA{R: 30, G: clampToByte(58 + gVar), B: 26, A: 160}, false)

			case ObjectRubblePile:
				// Rubble pile — already drawn by cover renderer, but add detail if from TileMap directly.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 55, G: 50, B: 42, A: 220}, false)
				vector.StrokeLine(screen, px+2, py+cs*0.4, px+cs*0.6, py+2, 0.5, color.RGBA{R: 65, G: 58, B: 48, A: 150}, false)

			case ObjectSandbag:
				// Sandbag wall — tan filled rect with horizontal lines.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 85, G: 78, B: 55, A: 255}, false)
				vector.StrokeLine(screen, px+1, py+cs*0.33, px+cs-1, py+cs*0.33, 0.5, color.RGBA{R: 70, G: 64, B: 44, A: 200}, false)
				vector.StrokeLine(screen, px+1, py+cs*0.66, px+cs-1, py+cs*0.66, 0.5, color.RGBA{R: 70, G: 64, B: 44, A: 200}, false)

			case ObjectSlitTrench:
				// Slit trench — dark recessed rectangle with shadow.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 28, G: 24, B: 18, A: 255}, false)
				// Inner shadow for depth.
				vector.StrokeRect(screen, px+2, py+2, cs-4, cs-4, 0.8, color.RGBA{R: 18, G: 14, B: 10, A: 200}, false)
				// Dirt lip on edges.
				vector.StrokeLine(screen, px, py, px+cs, py, 0.8, color.RGBA{R: 52, G: 44, B: 32, A: 180}, false)
				vector.StrokeLine(screen, px, py+cs, px+cs, py+cs, 0.8, color.RGBA{R: 52, G: 44, B: 32, A: 180}, false)

			case ObjectWire:
				// Barbed wire — crisscross lines.
				vector.StrokeLine(screen, px+1, py+1, px+cs-1, py+cs-1, 0.5, color.RGBA{R: 80, G: 78, B: 74, A: 180}, false)
				vector.StrokeLine(screen, px+cs-1, py+1, px+1, py+cs-1, 0.5, color.RGBA{R: 80, G: 78, B: 74, A: 180}, false)
				// Barb dots.
				vector.FillRect(screen, px+cs*0.25, py+cs*0.25, 1, 1, color.RGBA{R: 100, G: 96, B: 90, A: 200}, false)
				vector.FillRect(screen, px+cs*0.75, py+cs*0.75, 1, 1, color.RGBA{R: 100, G: 96, B: 90, A: 200}, false)

			case ObjectATBarrier:
				// Anti-tank barrier — concrete block with X marks.
				vector.FillRect(screen, px+1, py+1, cs-2, cs-2, color.RGBA{R: 72, G: 70, B: 66, A: 255}, false)
				vector.StrokeLine(screen, px+2, py+2, px+cs-2, py+cs-2, 1.0, color.RGBA{R: 58, G: 56, B: 52, A: 200}, false)
				vector.StrokeLine(screen, px+cs-2, py+2, px+2, py+cs-2, 1.0, color.RGBA{R: 58, G: 56, B: 52, A: 200}, false)

			case ObjectFence:
				// Fence — thin vertical lines with horizontal rail.
				vector.StrokeLine(screen, px+cs*0.25, py+1, px+cs*0.25, py+cs-1, 0.5, color.RGBA{R: 68, G: 60, B: 44, A: 200}, false)
				vector.StrokeLine(screen, px+cs*0.75, py+1, px+cs*0.75, py+cs-1, 0.5, color.RGBA{R: 68, G: 60, B: 44, A: 200}, false)
				vector.StrokeLine(screen, px, py+cs*0.35, px+cs, py+cs*0.35, 0.5, color.RGBA{R: 74, G: 66, B: 48, A: 180}, false)
				vector.StrokeLine(screen, px, py+cs*0.65, px+cs, py+cs*0.65, 0.5, color.RGBA{R: 74, G: 66, B: 48, A: 180}, false)

			case ObjectVehicleWreck:
				// Vehicle wreck — dark charred rectangle.
				vector.FillRect(screen, px, py, cs, cs, color.RGBA{R: 36, G: 32, B: 28, A: 255}, false)
				vector.StrokeLine(screen, px+2, py+cs*0.5, px+cs-2, py+cs*0.5, 1.0, color.RGBA{R: 50, G: 44, B: 36, A: 200}, false)
				// Flame hint.
				vector.FillRect(screen, px+cs*0.4, py+1, 3, 3, color.RGBA{R: 120, G: 60, B: 20, A: 100}, false)
			}
		}
	}
}

// drawCoverObjects renders cover objects with orientation-aware visuals.
// Chest-walls detect their H/V neighbors to draw correct cross-sections and corners.
func (g *Game) drawCoverObjects(screen *ebiten.Image, offX, offY int, minX, minY, maxX, maxY float64) { //nolint:gocognit,gocyclo
	ox, oy := float32(offX), float32(offY)
	cs := float32(coverCellSize) // 16px
	iCS := coverCellSize

	// Build a fast set of chest-wall cell positions for neighbor lookup once.
	if !g.chestSetReady {
		g.rebuildChestSet()
	}
	chestSet := g.cachedChestSet

	// Palette.
	slabBody := color.RGBA{R: 100, G: 94, B: 80, A: 255}
	slabTop := color.RGBA{R: 138, G: 130, B: 112, A: 255} // bright top-face highlight
	slabSide := color.RGBA{R: 72, G: 68, B: 56, A: 255}   // darker side face
	slabShadow := color.RGBA{R: 8, G: 6, B: 4, A: 90}
	slabW := float32(5) // thickness of the wall strip in pixels

	for _, c := range g.covers {
		if float64(c.x+iCS) < minX || float64(c.x) > maxX || float64(c.y+iCS) < minY || float64(c.y) > maxY {
			continue
		}
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
			// Determine which neighbors exist.
			cx := c.x / iCS
			cy := c.y / iCS
			hasLeft := chestSet[[2]int{cx - 1, cy}]
			hasRight := chestSet[[2]int{cx + 1, cy}]
			hasUp := chestSet[[2]int{cx, cy - 1}]
			hasDown := chestSet[[2]int{cx, cy + 1}]
			hasH := hasLeft || hasRight
			hasV := hasUp || hasDown

			// Drop shadow first.
			vector.FillRect(screen, x0+2, y0+2, cs, cs, slabShadow, false)

			// Horizontal run: slab centered vertically across the cell (3D top-down wall).
			// The "top" of the wall is a bright horizontal stripe across the center.
			switch {
			case hasH && !hasV:
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
			case hasV && !hasH:
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
			default:
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

		case CoverRubbleLight:
			g.drawRubbleLight(screen, x0, y0, cs, c.x, c.y)
		case CoverRubbleMedium:
			g.drawRubbleMedium(screen, x0, y0, cs, c.x, c.y)
		case CoverRubbleHeavy:
			g.drawRubbleHeavy(screen, x0, y0, cs, c.x, c.y)
		case CoverRubbleMetal:
			g.drawRubbleMetal(screen, x0, y0, cs, c.x, c.y)
		case CoverRubbleWood:
			g.drawRubbleWood(screen, x0, y0, cs, c.x, c.y)
		}
	}
}

// drawHeatLayer renders one HeatLayer as an alpha-blended color wash.
func (g *Game) drawHeatLayer(screen *ebiten.Image, layer *HeatLayer, baseCol color.RGBA, minX, minY, maxX, maxY float64) {
	ox, oy := float32(0), float32(0)
	cs := float32(cellSize)
	minCol, minRow := WorldToCell(minX, minY)
	maxCol, maxRow := WorldToCell(maxX, maxY)
	if minCol < 0 {
		minCol = 0
	}
	if minRow < 0 {
		minRow = 0
	}
	if maxCol >= layer.cols {
		maxCol = layer.cols - 1
	}
	if maxRow >= layer.rows {
		maxRow = layer.rows - 1
	}
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			v := layer.At(row, col)
			if v <= 0 {
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
	switch g.simSpeed {
	case 0:
		speedStr = "PAUSED"
	case 2:
		speedStr = "2x"
	case 4:
		speedStr = "4x"
	default:
		if g.simSpeed != 1 {
			speedStr = fmt.Sprintf("%.1fx", g.simSpeed)
		}
	}

	g.refreshHUDCaches()

	lines := g.hudLinesScratch[:0]
	lines = append(lines,
		fmt.Sprintf("SIM: %s  tick:%d  P=pause  ,/. speed", speedStr, g.tick),
		fmt.Sprintf("Intel: [%s]  Tab=switch", teamLabel),
	)
	lines = append(lines, g.hudIntelLines...)
	lines = append(
		lines,
		"[H] toggle HUD",
		"WASD/arrows=pan  scroll=zoom",
		fmt.Sprintf("zoom: %.1fx  click=inspect", g.camZoom),
		g.hudFilterLine,
		g.perfHUDLine,
	)
	g.hudLinesScratch = lines

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

func (g *Game) refreshHUDCaches() {
	if g.hudOverlayTeam != g.overlayTeam {
		g.hudOverlayTeam = g.overlayTeam
		g.hudIntelDirty = true
	}
	for k := IntelMapKind(0); k < intelMapCount; k++ {
		state := g.showOverlay[g.overlayTeam][k]
		if g.hudOverlayState[g.overlayTeam][k] != state {
			g.hudOverlayState[g.overlayTeam][k] = state
			g.hudIntelDirty = true
		}
	}
	for i := LogCategory(0); i < logCatCount; i++ {
		state := g.thoughtLog.FilterEnabled(i)
		if g.hudFilterState[i] != state {
			g.hudFilterState[i] = state
			g.hudFilterDirty = true
		}
	}

	if g.hudIntelDirty {
		g.hudIntelLines = g.hudIntelLines[:0]
		for k := IntelMapKind(0); k < intelMapCount; k++ {
			on := " "
			if g.showOverlay[g.overlayTeam][k] {
				on = "*"
			}
			g.hudIntelLines = append(g.hudIntelLines, fmt.Sprintf("  [%d]%s %s", k+1, on, IntelMapKindName(k)))
		}
		g.hudIntelDirty = false
	}

	if g.hudFilterDirty {
		line := "Log:"
		filterFKeys := [logCatCount]string{"F5", "F6", "F7", "F8"}
		for i := LogCategory(0); i < logCatCount; i++ {
			on := "-"
			if g.thoughtLog.FilterEnabled(i) {
				on = "*"
			}
			line += fmt.Sprintf(" [%s]%s%s", filterFKeys[i], on, i.ShortName())
		}
		g.hudFilterLine = line
		g.hudFilterDirty = false
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

// drawSpottedIndicators renders a subtle "!" above soldiers who currently see enemies.
func (g *Game) drawSpottedIndicators(screen *ebiten.Image, offX, offY int) {
	ox, oy := float32(offX), float32(offY)
	minX, minY, maxX, maxY := g.cameraCullBounds(48)
	all := g.allUnits()
	for _, s := range all {
		if s.state == SoldierStateDead || len(s.vision.KnownContacts) == 0 {
			continue
		}
		if !pointInBounds(s.x, s.y, minX, minY, maxX, maxY) {
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

func (g *Game) drawRadioVisualEffects(screen *ebiten.Image) { //nolint:gocognit,gocyclo
	const baseOpacity = 0.08 // very faint/transparent
	minX, minY, maxX, maxY := g.cameraCullBounds(128)
	segments := 22
	switch {
	case g.camZoom <= 0.7:
		segments = 10
	case g.camZoom <= 1.0:
		segments = 14
	case g.camZoom <= 1.5:
		segments = 18
	}
	for _, sq := range g.squads {
		sq.pruneRadioVisualEvents(g.tick)
		for _, ev := range sq.radioVisualEvents {
			age := g.tick - ev.StartTick
			if age < 0 || age >= ev.Duration {
				continue
			}
			life := 1.0 - float64(age)/float64(ev.Duration)
			if life <= 0 {
				continue
			}

			// Console-green baseline, capped at ~25% alpha.
			base := color.RGBA{R: 76, G: 255, B: 136, A: uint8(255 * baseOpacity * life)}
			coreWidth := float32(1.5)
			grain := 3.0
			switch ev.Delivery {
			case radioDeliveryGarbled:
				base = color.RGBA{R: 90, G: 250, B: 145, A: uint8(255 * 0.22 * life)}
				coreWidth = 1.35
				grain = 5.0
			case radioDeliveryDrop:
				base = color.RGBA{R: 110, G: 225, B: 140, A: uint8(255 * 0.18 * life)}
				coreWidth = 1.2
				grain = 6.5
			}

			sx := float32(ev.SenderX)
			sy := float32(ev.SenderY)
			rx := float32(ev.ReceiverX)
			ry := float32(ev.ReceiverY)
			if !pointInBounds(ev.SenderX, ev.SenderY, minX, minY, maxX, maxY) &&
				!pointInBounds(ev.ReceiverX, ev.ReceiverY, minX, minY, maxX, maxY) {
				continue
			}
			dx := rx - sx
			dy := ry - sy
			dist := math.Hypot(float64(dx), float64(dy))
			if dist < 1 {
				continue
			}
			nx := -dy / float32(dist)
			ny := dx / float32(dist)
			arch := float32(math.Min(30.0, dist*0.06))
			mx := (sx + rx) * 0.5
			my := (sy + ry) * 0.5
			cx := mx + nx*arch
			cy := my + ny*arch

			prevX := sx
			prevY := sy
			for i := 1; i <= segments; i++ {
				t := float32(i) / float32(segments)
				omt := 1.0 - t
				arcX := omt*omt*sx + 2*omt*t*cx + t*t*rx
				arcY := omt*omt*sy + 2*omt*t*cy + t*t*ry
				phase := float64(ev.MessageID*31+uint64(i*17)+uint64(g.tick*7)) * 0.11 // #nosec G115 -- intentional bit-mixing for deterministic noise
				jitter := float32(math.Sin(phase)) * float32(grain*(1.0-life)*0.9)
				jx := arcX + nx*jitter
				jy := arcY + ny*jitter

				glowCol := color.RGBA{R: 76, G: 255, B: 136, A: uint8(float64(base.A) * 0.35)}
				vector.StrokeLine(screen, prevX, prevY, jx, jy, coreWidth+1.3, glowCol, false)
				vector.StrokeLine(screen, prevX, prevY, jx, jy, coreWidth, base, false)

				// Grain speckle along the arc for analog/static feel.
				if i%2 == 0 {
					sparkA := uint8(float64(base.A) * 0.45)
					vector.FillCircle(screen, jx+nx*0.6, jy+ny*0.6, 0.9, color.RGBA{R: 110, G: 255, B: 165, A: sparkA}, false)
				}

				prevX, prevY = jx, jy
			}

			// Endpoint pings for readability.
			pingAlpha := uint8(255 * baseOpacity * (0.7 + 0.3*life))
			vector.FillCircle(screen, sx, sy, 2.0, color.RGBA{R: 120, G: 255, B: 170, A: pingAlpha}, false)
			vector.FillCircle(screen, rx, ry, 1.8, color.RGBA{R: 120, G: 255, B: 170, A: pingAlpha}, false)
		}
	}
}

// drawVisionConesBuffered renders all FOV fans for a team into an offscreen buffer,
// then composites that buffer onto the main screen with a single controlled opacity.
// This eliminates additive blowout from overlapping cones.
func (g *Game) drawVisionConesBuffered(screen *ebiten.Image, soldiers []*Soldier, tint color.RGBA, opacity float64) {
	// Quality gate: at wide zoom-out, cone detail is low value but high cost.
	if g.camZoom <= 0.65 {
		return
	}

	buf := g.visionBuf
	buf.Clear()

	// Culling bounds in world-space for the current camera viewport.
	halfVW := float64(g.gameWidth) / 2 / g.camZoom
	halfVH := float64(g.gameHeight) / 2 / g.camZoom
	minX := g.camX - halfVW
	maxX := g.camX + halfVW
	minY := g.camY - halfVH
	maxY := g.camY + halfVH

	steps := 36
	switch {
	case g.camZoom <= 0.6:
		steps = 14
	case g.camZoom <= 0.8:
		steps = 20
	case g.camZoom <= 1.2:
		steps = 28
	}

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
		if s.x+coneLen < minX || s.x-coneLen > maxX || s.y+coneLen < minY || s.y-coneLen > maxY {
			continue
		}
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
	if g.losIndex == nil {
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
	} else {
		for _, bi := range g.losIndex.queryBuildingCandidateIndices(ox, oy, ex, ey) {
			b := g.losIndex.buildingItems[bi]
			t, hit := rayAABBHitT(ox, oy, ex, ey, b.minX, b.minY, b.maxX, b.maxY)
			if hit && t < bestT {
				bestT = t
				hitAny = true
			}
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
	minX, minY, maxX, maxY := g.cameraCullBounds(72)
	flashes := g.combat.ActiveFlashes()
	for _, f := range flashes {
		if !pointInBounds(f.x, f.y, minX, minY, maxX, maxY) {
			continue
		}
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

// Layout reports the fixed logical screen size for Ebiten.
func (g *Game) Layout(_, _ int) (int, int) {
	return g.width, g.height
}

// GameWidth returns the playfield width (excluding log panel).
func (g *Game) GameWidth() int {
	return g.gameWidth
}
