package game

import (
	"math"
	"math/rand"
)

// cellKey represents a grid cell coordinate for placement tracking.
type cellKey struct{ cx, cy int }

// CoverKind identifies the type of cover object.
type CoverKind int

const (
	// CoverTallWall is an impassable wall taller than a person.
	// Fully blocks LOS. No one can see or shoot over it.
	CoverTallWall CoverKind = iota

	// CoverChestWall is a chest-high wall. Passable but slows movement.
	// Blocks LOS and bullets for crouching/prone soldiers behind it.
	// Excellent cover — heavily reduces hit chance.
	CoverChestWall

	// CoverRubbleLight is scattered small debris and chunks.
	// Passable with minor movement penalty. Provides minimal cover.
	CoverRubbleLight

	// CoverRubbleMedium is moderate debris piles from collapsed structures.
	// Passable but slows movement significantly. Decent cover value.
	CoverRubbleMedium

	// CoverRubbleHeavy is large concrete/masonry chunks and twisted metal.
	// Very difficult to traverse. Excellent cover, can partially block LOS.
	CoverRubbleHeavy

	// CoverRubbleMetal is twisted steel beams, vehicle parts, and machinery debris.
	// Moderate movement penalty but excellent ballistic protection.
	CoverRubbleMetal

	// CoverRubbleWood is splintered timber, furniture debris, and organic matter.
	// Easy to move through but minimal ballistic protection.
	CoverRubbleWood
)

// coverCellSize is the size of a cover object in pixels.
// Matches one grid cell (16px).
const coverCellSize = cellSize // 16px

// CoverObject is a single cover element on the battlefield.
type CoverObject struct {
	x, y int // world pixel coords of top-left corner
	kind CoverKind
}

// Rect returns the AABB of this cover object (pixel coords).
func (c *CoverObject) Rect() (x0, y0, x1, y1 int) {
	return c.x, c.y, c.x + coverCellSize, c.y + coverCellSize
}

// CoverDefence returns the fraction of incoming fire negated by this cover
// when a soldier is positioned correctly behind it.
// 0 = no protection, 1 = immune.
func (c *CoverObject) CoverDefence() float64 {
	switch c.kind {
	case CoverTallWall:
		return 0.85
	case CoverChestWall:
		return 0.70
	case CoverRubbleLight:
		return 0.25 // minimal debris cover
	case CoverRubbleMedium:
		return 0.50 // moderate debris cover
	case CoverRubbleHeavy:
		return 0.75 // excellent debris cover
	case CoverRubbleMetal:
		return 0.80 // superior ballistic protection
	case CoverRubbleWood:
		return 0.20 // poor ballistic protection
	default:
		return 0.0
	}
}

// MovementMul returns a speed multiplier for soldiers moving through this cover.
// 1.0 = no effect, <1.0 = slowed.
func (c *CoverObject) MovementMul() float64 {
	switch c.kind {
	case CoverTallWall:
		return 1.0 // impassable, not reached
	case CoverChestWall:
		return 0.65 // climb-over penalty
	case CoverRubbleLight:
		return 0.85 // minor debris, easy to navigate
	case CoverRubbleMedium:
		return 0.60 // moderate debris piles
	case CoverRubbleHeavy:
		return 0.35 // large chunks, very difficult
	case CoverRubbleMetal:
		return 0.45 // twisted metal, sharp edges
	case CoverRubbleWood:
		return 0.75 // splintered wood, easier than concrete
	default:
		return 1.0
	}
}

// BlocksLOS returns true if this cover object fully interrupts line of sight.
func (c *CoverObject) BlocksLOS() bool {
	switch c.kind {
	case CoverTallWall:
		return true
	case CoverRubbleHeavy:
		return true // large debris piles can block LOS
	default:
		return false
	}
}

// BlocksMovement returns true if soldiers cannot walk through this cover object.
func (c *CoverObject) BlocksMovement() bool {
	return c.kind == CoverTallWall
}

// SlowsMovement returns true if this cover object impedes but does not block movement.
func (c *CoverObject) SlowsMovement() bool {
	switch c.kind {
	case CoverChestWall, CoverRubbleLight, CoverRubbleMedium, CoverRubbleHeavy, CoverRubbleMetal, CoverRubbleWood:
		return true
	default:
		return false
	}
}

// --- Cover generation ---

const (
	// WallRunMinLen / wallRunMaxLen are the length bounds of a cover-wall run in cells.
	wallRunMinLen = 3
	wallRunMaxLen = 10
	// Number of cover wall runs and freestanding corridors to attempt.
	numWallRuns  = 20
	numCorridors = 6
)

type wallFaceAttempt struct {
	x, y  int
	horiz bool
}

// GenerateCover generates cover objects and returns two slices:
//
//	covers  – all non-rubble cover (tall walls + chest-high walls)
//	rubble  – rubble pieces to be placed by applyBuildingDamage
//
// Walls and chest-walls form long horizontal or vertical runs that extend
// from building edges or span open corridors between buildings.
func GenerateCover(mapW, mapH int, footprints, walls []rect, rng *rand.Rand, tm *TileMap) ([]*CoverObject, []*CoverObject) {
	covers := make([]*CoverObject, 0, 64)
	occupied := map[cellKey]bool{}
	cellSize := coverCellSize
	margin := cellSize * 3
	footprintRects := footprintsToRects(footprints)

	generateWallExtensions(footprints, walls, footprintRects, occupied, &covers, rng, tm, cellSize, margin, mapW, mapH)
	generateFreestandingRuns(walls, footprintRects, occupied, &covers, rng, tm, cellSize, margin, mapW, mapH)
	generateCoverCorridors(walls, footprintRects, occupied, &covers, rng, tm, cellSize, margin, mapW, mapH)

	// --- 4. Generate rubble positions from building explosions (returned separately) ---
	rubble := generateExplosionRubble(mapW, mapH, footprints, walls, rng, tm)

	return covers, rubble
}

func generateWallExtensions(
	footprints, walls, footprintRects []rect,
	occupied map[cellKey]bool,
	covers *[]*CoverObject,
	rng *rand.Rand,
	tm *TileMap,
	cellSize, margin, mapW, mapH int,
) {
	for _, fp := range footprints {
		faceAttempts := buildWallFaceAttempts(fp, cellSize, rng)
		for _, fa := range faceAttempts {
			if rng.Float64() > 0.6 {
				continue
			}
			length := wallRunMinLen + rng.Intn(wallRunMaxLen-wallRunMinLen+1)
			kind := CoverChestWall
			if rng.Float64() < 0.3 {
				kind = CoverTallWall
			}
			sx := snapToCoverCell(fa.x, cellSize)
			sy := snapToCoverCell(fa.y, cellSize)
			if !isWithinCoverMargins(sx, sy, margin, mapW, mapH) {
				continue
			}
			placeCoverLine(walls, footprintRects, occupied, covers, tm, cellSize, sx, sy, length, fa.horiz, kind)
		}
	}
}

func buildWallFaceAttempts(fp rect, cs int, rng *rand.Rand) []wallFaceAttempt {
	return []wallFaceAttempt{
		{x: fp.x - cs*(wallRunMinLen+rng.Intn(3)), y: fp.y + fp.h/2, horiz: true},
		{x: fp.x + fp.w, y: fp.y + fp.h/2, horiz: true},
		{x: fp.x + fp.w/2, y: fp.y - cs*(wallRunMinLen+rng.Intn(3)), horiz: false},
		{x: fp.x + fp.w/2, y: fp.y + fp.h, horiz: false},
	}
}

func generateFreestandingRuns(
	walls, footprintRects []rect,
	occupied map[cellKey]bool,
	covers *[]*CoverObject,
	rng *rand.Rand,
	tm *TileMap,
	cellSize, margin, mapW, mapH int,
) {
	for attempt := 0; attempt < numWallRuns*6 && len(*covers) < numWallRuns*wallRunMaxLen; attempt++ {
		horizontal := rng.Intn(2) == 0
		length := wallRunMinLen + rng.Intn(wallRunMaxLen-wallRunMinLen+1)
		sx := margin + snapToCoverCell(rng.Intn((mapW-margin*2)/cellSize)*cellSize, cellSize)
		sy := margin + snapToCoverCell(rng.Intn((mapH-margin*2)/cellSize)*cellSize, cellSize)
		kind := CoverChestWall
		if rng.Float64() < 0.25 {
			kind = CoverTallWall
		}
		placeCoverLine(walls, footprintRects, occupied, covers, tm, cellSize, sx, sy, length, horizontal, kind)
	}
}

func generateCoverCorridors(
	walls, footprintRects []rect,
	occupied map[cellKey]bool,
	covers *[]*CoverObject,
	rng *rand.Rand,
	tm *TileMap,
	cellSize, margin, mapW, mapH int,
) {
	for i := 0; i < numCorridors; i++ {
		horizontal := rng.Intn(2) == 0
		length := wallRunMinLen + rng.Intn(wallRunMaxLen-wallRunMinLen+1)
		gap := (3 + rng.Intn(4)) * cellSize
		if horizontal {
			sx := margin + snapToCoverCell(rng.Intn((mapW-margin*2)/cellSize)*cellSize, cellSize)
			sy := margin + snapToCoverCell(rng.Intn(((mapH-margin*2)-gap)/cellSize)*cellSize, cellSize)
			placeCoverLine(walls, footprintRects, occupied, covers, tm, cellSize, sx, sy, length, true, CoverChestWall)
			placeCoverLine(walls, footprintRects, occupied, covers, tm, cellSize, sx, sy+gap, length, true, CoverChestWall)
			continue
		}
		sx := margin + snapToCoverCell(rng.Intn(((mapW-margin*2)-gap)/cellSize)*cellSize, cellSize)
		sy := margin + snapToCoverCell(rng.Intn((mapH-margin*2)/cellSize)*cellSize, cellSize)
		placeCoverLine(walls, footprintRects, occupied, covers, tm, cellSize, sx, sy, length, false, CoverChestWall)
		placeCoverLine(walls, footprintRects, occupied, covers, tm, cellSize, sx+gap, sy, length, false, CoverChestWall)
	}
}

func placeCoverLine(
	walls, footprintRects []rect,
	occupied map[cellKey]bool,
	covers *[]*CoverObject,
	tm *TileMap,
	cellSize int,
	startX, startY, length int,
	horizontal bool,
	kind CoverKind,
) {
	for i := 0; i < length; i++ {
		x, y := coverLineCell(startX, startY, i, cellSize, horizontal)
		if coverCellBlocked(walls, footprintRects, occupied, tm, cellSize, x, y, kind) {
			continue
		}
		k := cellKey{x / cellSize, y / cellSize}
		occupied[k] = true
		*covers = append(*covers, &CoverObject{x: x, y: y, kind: kind})
	}
}

func coverLineCell(startX, startY, step, cellSize int, horizontal bool) (int, int) {
	if horizontal {
		return startX + step*cellSize, startY
	}
	return startX, startY + step*cellSize
}

func coverCellBlocked(walls, footprintRects []rect, occupied map[cellKey]bool, tm *TileMap, cellSize, x, y int, kind CoverKind) bool {
	if tileOnRoadForCover(tm, x, y, cellSize) {
		return true
	}
	k := cellKey{x / cellSize, y / cellSize}
	if occupied[k] {
		return true
	}
	c := &CoverObject{x: x, y: y, kind: kind}
	if coverOverlapsBuildings(c, walls) {
		return true
	}
	return coverOverlapsBuildings(c, footprintRects)
}

func tileOnRoadForCover(tm *TileMap, px, py, cellSize int) bool {
	if tm == nil {
		return false
	}
	return tileOnRoad(tm, px/cellSize, py/cellSize)
}

func snapToCoverCell(v, cellSize int) int {
	return (v / cellSize) * cellSize
}

func isWithinCoverMargins(x, y, margin, mapW, mapH int) bool {
	return !(x < margin || y < margin || x > mapW-margin || y > mapH-margin)
}

// footprintsToRects converts building footprints to rects for overlap testing.
func footprintsToRects(fps []rect) []rect { return fps }

// generateExplosionRubble creates sophisticated rubble formations using realistic
// blast patterns, debris fields, and structural collapse simulations.
func generateExplosionRubble(mapW, mapH int, footprints, _ []rect, rng *rand.Rand, tm *TileMap) []*CoverObject {
	cs := coverCellSize
	rubble := make([]*CoverObject, 0, 128)
	placed := map[cellKey]bool{}

	// Generate different types of debris formations
	formations := []func(){
		// Building collapse formations
		func() { generateBuildingCollapseRubble(mapW, mapH, footprints, rng, tm, &rubble, placed, cs) },
		// Artillery/explosion debris fields
		func() { generateExplosionDebrisFields(mapW, mapH, footprints, rng, tm, &rubble, placed, cs) },
		// Vehicle wreck scatter patterns
		func() { generateVehicleWreckDebris(mapW, mapH, rng, tm, &rubble, placed, cs) },
		// Linear demolition paths
		func() { generateDemolitionPaths(mapW, mapH, footprints, rng, tm, &rubble, placed, cs) },
	}

	// Execute multiple formation types
	for _, formation := range formations {
		if rng.Float64() < 0.7 { // 70% chance for each type
			formation()
		}
	}

	return rubble
}

type rubbleChoice struct {
	threshold float64
	kind      CoverKind
}

type rubbleContext int

const (
	rubbleOpenGround rubbleContext = iota
	rubbleNearBuilding
	rubbleInsideResidential
	rubbleInsideCommercial
	rubbleInsideIndustrial
)

var rubbleChoicesByContext = map[rubbleContext][]rubbleChoice{
	rubbleInsideIndustrial: {
		{threshold: 0.45, kind: CoverRubbleHeavy},
		{threshold: 0.75, kind: CoverRubbleMetal},
		{threshold: 0.85, kind: CoverRubbleMedium},
		{threshold: 0.95, kind: CoverRubbleWood},
		{threshold: 1.00, kind: CoverRubbleLight},
	},
	rubbleInsideCommercial: {
		{threshold: 0.30, kind: CoverRubbleHeavy},
		{threshold: 0.50, kind: CoverRubbleMedium},
		{threshold: 0.65, kind: CoverRubbleMetal},
		{threshold: 0.80, kind: CoverRubbleWood},
		{threshold: 1.00, kind: CoverRubbleLight},
	},
	rubbleInsideResidential: {
		{threshold: 0.20, kind: CoverRubbleHeavy},
		{threshold: 0.35, kind: CoverRubbleMedium},
		{threshold: 0.45, kind: CoverRubbleMetal},
		{threshold: 0.75, kind: CoverRubbleWood},
		{threshold: 1.00, kind: CoverRubbleLight},
	},
	rubbleNearBuilding: {
		{threshold: 0.25, kind: CoverRubbleHeavy},
		{threshold: 0.45, kind: CoverRubbleMedium},
		{threshold: 0.60, kind: CoverRubbleMetal},
		{threshold: 0.75, kind: CoverRubbleWood},
		{threshold: 1.00, kind: CoverRubbleLight},
	},
	rubbleOpenGround: {
		{threshold: 0.10, kind: CoverRubbleHeavy},
		{threshold: 0.25, kind: CoverRubbleMedium},
		{threshold: 0.40, kind: CoverRubbleMetal},
		{threshold: 0.55, kind: CoverRubbleWood},
		{threshold: 1.00, kind: CoverRubbleLight},
	},
}

func rubbleContextAt(footprints []rect, rx, ry int) rubbleContext {
	nearBuilding := false
	for _, fp := range footprints {
		if rx >= fp.x && rx < fp.x+fp.w && ry >= fp.y && ry < fp.y+fp.h {
			area := fp.w * fp.h
			switch {
			case area > 4000:
				return rubbleInsideIndustrial
			case area > 1500:
				return rubbleInsideCommercial
			default:
				return rubbleInsideResidential
			}
		}
		if rx >= fp.x-48 && rx < fp.x+fp.w+48 && ry >= fp.y-48 && ry < fp.y+fp.h+48 {
			nearBuilding = true
		}
	}
	if nearBuilding {
		return rubbleNearBuilding
	}
	return rubbleOpenGround
}

func chooseRubbleFromContext(ctx rubbleContext, rng *rand.Rand) CoverKind {
	choices, ok := rubbleChoicesByContext[ctx]
	if !ok {
		return CoverRubbleLight
	}
	roll := rng.Float64()
	for i := range choices {
		if roll < choices[i].threshold {
			return choices[i].kind
		}
	}
	return choices[len(choices)-1].kind
}

// chooseRubbleType selects rubble type based on context and building materials.
func chooseRubbleType(rng *rand.Rand, footprints []rect, rx, ry int) CoverKind {
	ctx := rubbleContextAt(footprints, rx, ry)
	return chooseRubbleFromContext(ctx, rng)
}

// generateBuildingCollapseRubble simulates realistic building collapse patterns.
func generateBuildingCollapseRubble(mapW, mapH int, footprints []rect, rng *rand.Rand, tm *TileMap, rubble *[]*CoverObject, placed map[cellKey]bool, cs int) {
	// Basic implementation for now - creates debris piles around buildings
	for _, fp := range footprints {
		if rng.Float64() > 0.4 { // 40% chance per building
			continue
		}

		// Create debris field around building
		for i := 0; i < 8+rng.Intn(12); i++ {
			rx := fp.x + rng.Intn(fp.w) + (rng.Intn(64) - 32)
			ry := fp.y + rng.Intn(fp.h) + (rng.Intn(64) - 32)
			rx = (rx / cs) * cs
			ry = (ry / cs) * cs

			if !isValidRubblePosition(rx, ry, mapW, mapH, cs, tm, placed) {
				continue
			}

			k := cellKey{rx / cs, ry / cs}
			placed[k] = true
			rubbleType := chooseRubbleType(rng, footprints, rx, ry)
			*rubble = append(*rubble, &CoverObject{x: rx, y: ry, kind: rubbleType})
		}
	}
}

// generateExplosionDebrisFields creates realistic blast patterns.
func generateExplosionDebrisFields(mapW, mapH int, footprints []rect, rng *rand.Rand, tm *TileMap, rubble *[]*CoverObject, placed map[cellKey]bool, cs int) {
	numBlasts := 2 + rng.Intn(4)

	for i := 0; i < numBlasts; i++ {
		// Choose blast center
		var blastX, blastY int
		if len(footprints) > 0 && rng.Float64() < 0.6 {
			fp := footprints[rng.Intn(len(footprints))]
			blastX = fp.x + rng.Intn(fp.w)
			blastY = fp.y + rng.Intn(fp.h)
		} else {
			blastX = cs * (3 + rng.Intn((mapW-cs*6)/cs))
			blastY = cs * (3 + rng.Intn((mapH-cs*6)/cs))
		}

		// Create blast radius debris
		radius := 32 + rng.Intn(48)
		for j := 0; j < 15+rng.Intn(20); j++ {
			angle := rng.Float64() * 6.28 // 2*Pi
			dist := float64(rng.Intn(radius))
			rx := blastX + int(math.Cos(angle)*dist)
			ry := blastY + int(math.Sin(angle)*dist)
			rx = (rx / cs) * cs
			ry = (ry / cs) * cs

			if !isValidRubblePosition(rx, ry, mapW, mapH, cs, tm, placed) {
				continue
			}

			k := cellKey{rx / cs, ry / cs}
			placed[k] = true
			rubbleType := chooseRubbleType(rng, footprints, rx, ry)
			*rubble = append(*rubble, &CoverObject{x: rx, y: ry, kind: rubbleType})
		}
	}
}

// generateVehicleWreckDebris creates vehicle-related debris patterns.
func generateVehicleWreckDebris(mapW, mapH int, rng *rand.Rand, tm *TileMap, rubble *[]*CoverObject, placed map[cellKey]bool, cs int) {
	numWrecks := rng.Intn(3)

	for i := 0; i < numWrecks; i++ {
		crashX := cs * (4 + rng.Intn((mapW-cs*8)/cs))
		crashY := cs * (4 + rng.Intn((mapH-cs*8)/cs))

		// Create metal debris cluster
		for j := 0; j < 5+rng.Intn(10); j++ {
			rx := crashX + (rng.Intn(96) - 48)
			ry := crashY + (rng.Intn(96) - 48)
			rx = (rx / cs) * cs
			ry = (ry / cs) * cs

			if !isValidRubblePosition(rx, ry, mapW, mapH, cs, tm, placed) {
				continue
			}

			k := cellKey{rx / cs, ry / cs}
			placed[k] = true
			*rubble = append(*rubble, &CoverObject{x: rx, y: ry, kind: CoverRubbleMetal})
		}
	}
}

// generateDemolitionPaths creates linear destruction patterns.
func generateDemolitionPaths(mapW, mapH int, footprints []rect, rng *rand.Rand, tm *TileMap, rubble *[]*CoverObject, placed map[cellKey]bool, cs int) {
	if rng.Float64() > 0.3 { // 30% chance
		return
	}

	// Simple linear debris path
	startX := cs * (2 + rng.Intn((mapW-cs*4)/cs))
	startY := cs * 2
	endX := cs * (2 + rng.Intn((mapW-cs*4)/cs))
	endY := mapH - cs*2

	steps := 20 + rng.Intn(30)
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps)
		rx := int(float64(startX)*(1-t) + float64(endX)*t)
		ry := int(float64(startY)*(1-t) + float64(endY)*t)
		rx = (rx / cs) * cs
		ry = (ry / cs) * cs

		if rng.Float64() > 0.4 { // sparse placement
			continue
		}

		if !isValidRubblePosition(rx, ry, mapW, mapH, cs, tm, placed) {
			continue
		}

		k := cellKey{rx / cs, ry / cs}
		placed[k] = true
		rubbleType := chooseRubbleType(rng, footprints, rx, ry)
		*rubble = append(*rubble, &CoverObject{x: rx, y: ry, kind: rubbleType})
	}
}

// isValidRubblePosition checks if a rubble position is valid.
func isValidRubblePosition(rx, ry, mapW, mapH, cs int, tm *TileMap, placed map[cellKey]bool) bool {
	if rx < cs || ry < cs || rx > mapW-cs*2 || ry > mapH-cs*2 {
		return false
	}

	k := cellKey{rx / cs, ry / cs}
	if placed[k] {
		return false
	}

	if tm != nil && tileOnRoad(tm, rx/cs, ry/cs) {
		return false
	}

	return true
}

// coverOverlapsBuildings returns true if the cover object overlaps a building (with padding).
func coverOverlapsBuildings(c *CoverObject, buildings []rect) bool {
	pad := coverCellSize
	cx0 := c.x - pad
	cy0 := c.y - pad
	cx1 := c.x + coverCellSize + pad
	cy1 := c.y + coverCellSize + pad
	for _, b := range buildings {
		if cx0 < b.x+b.w && cx1 > b.x && cy0 < b.y+b.h && cy1 > b.y {
			return true
		}
	}
	return false
}

// --- Cover query helpers ---

// FindCoverForThreat searches the provided cover list for the best cover object.
// That object should interpose between (sx,sy) and the threat direction (threatAngle, radians).
// It returns the nearest valid cover object, or nil if none found.
// MaxSearchDist is the maximum distance (px) to search.
func FindCoverForThreat(sx, sy, threatAngle float64, covers []*CoverObject, _ []rect, maxSearchDist float64) *CoverObject {
	var best *CoverObject
	bestScore := -math.MaxFloat64

	for _, c := range covers {
		// Center of cover cell.
		cx := float64(c.x) + coverCellSize/2.0
		cy := float64(c.y) + coverCellSize/2.0

		// Distance from soldier to cover.
		dx := cx - sx
		dy := cy - sy
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist > maxSearchDist || dist < 1.0 {
			continue
		}

		// The cover is useful if the threat direction passes through or near it
		// when the soldier stands on the far side (soldier uses cover as shield).
		// We check: is the cover roughly between the soldier and the threat?
		// Angle from soldier toward cover.
		angleToCover := math.Atan2(dy, dx)
		// Threat comes from threatAngle, so the soldier wants cover in that direction.
		angDiff := math.Abs(normalizeAngle(angleToCover - threatAngle))
		if angDiff > math.Pi/2.5 {
			// Cover is not in the threat direction — not useful.
			continue
		}

		// Prefer closer cover and better defense value.
		coverVal := c.CoverDefence()
		score := coverVal*2.0 - dist/maxSearchDist - angDiff/(math.Pi/2.5)*0.5

		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

// CoverPositionBehind returns the world position where a soldier should stand
// to use the cover object against a threat coming from threatAngle.
// The soldier stands on the opposite side of the cover from the threat.
func CoverPositionBehind(c *CoverObject, threatAngle float64) (float64, float64) {
	cx := float64(c.x) + coverCellSize/2.0
	cy := float64(c.y) + coverCellSize/2.0

	// Stand one cell-width behind the cover (away from threat).
	offsetDist := float64(coverCellSize) * 1.2
	// threatAngle points from soldier toward enemy.
	// We want to be behind the cover = opposite side from threat.
	px := cx - math.Cos(threatAngle)*offsetDist
	py := cy - math.Sin(threatAngle)*offsetDist
	return px, py
}

// IsBehindCover returns true if the soldier at (sx,sy) has a cover object
// interposing between them and the threat at (tx,ty).
// The cover reduces the threat's ability to hit the soldier.
func IsBehindCover(sx, sy, tx, ty float64, covers []*CoverObject) (bool, float64) {
	bestDefence := 0.0
	found := false
	for _, c := range covers {
		cx0 := float64(c.x)
		cy0 := float64(c.y)
		cx1 := float64(c.x + coverCellSize)
		cy1 := float64(c.y + coverCellSize)
		if rayIntersectsAABB(tx, ty, sx, sy, cx0, cy0, cx1, cy1) {
			d := c.CoverDefence()
			if d > bestDefence {
				bestDefence = d
				found = true
			}
		}
	}
	return found, bestDefence
}
