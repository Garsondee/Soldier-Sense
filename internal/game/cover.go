package game

import (
	"math"
	"math/rand"
)

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

	// CoverRubble is scattered debris. Passable. Only marginally effective.
	// Does not block LOS. Slightly reduces hit chance.
	CoverRubble
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
	case CoverRubble:
		return 0.65 // substantial debris cover
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
	case CoverRubble:
		return 0.40 // rubble is very hard to move through quickly
	default:
		return 1.0
	}
}

// BlocksLOS returns true if this cover object fully interrupts line of sight.
func (c *CoverObject) BlocksLOS() bool {
	return c.kind == CoverTallWall
}

// BlocksMovement returns true if soldiers cannot walk through this cover object.
func (c *CoverObject) BlocksMovement() bool {
	return c.kind == CoverTallWall
}

// SlowsMovement returns true if this cover object impedes but does not block movement.
func (c *CoverObject) SlowsMovement() bool {
	return c.kind == CoverChestWall || c.kind == CoverRubble
}

// --- Cover generation ---

const (
	// wallRunMinLen / wallRunMaxLen: length of a cover-wall run in cells.
	wallRunMinLen = 3
	wallRunMaxLen = 10
	// Number of cover wall runs and freestanding corridors to attempt.
	numWallRuns  = 20
	numCorridors = 6
	// Explosion radius for building-damage rubble (px).
	explosionRadius = 80
	numExplosions   = 30
)

// GenerateCover generates cover objects and returns two slices:
//
//	covers  – all non-rubble cover (tall walls + chest-high walls)
//	rubble  – rubble pieces to be placed by applyBuildingDamage
//
// Walls and chest-walls form long horizontal or vertical runs that extend
// from building edges or span open corridors between buildings.
func GenerateCover(mapW, mapH int, footprints []rect, walls []rect, rng *rand.Rand, roads []roadSegment) ([]*CoverObject, []*CoverObject) {
	cs := coverCellSize
	margin := cs * 3
	covers := make([]*CoverObject, 0, 64)

	// occupied tracks cells already used by cover so we don't double-place.
	type cellKey struct{ cx, cy int }
	occupied := map[cellKey]bool{}

	// roadSet: mark cells that sit on a road band so walls are never placed there.
	type roadBand struct{ x, y, w, h int }
	roadBands := make([]roadBand, 0, len(roads))
	for _, rd := range roads {
		roadBands = append(roadBands, roadBand{rd.x, rd.y, rd.w, rd.h})
	}
	onRoad := func(px, py int) bool {
		for _, rb := range roadBands {
			if px >= rb.x && px < rb.x+rb.w && py >= rb.y && py < rb.y+rb.h {
				return true
			}
		}
		return false
	}

	placeLine := func(startX, startY, length int, horizontal bool, kind CoverKind) {
		for i := 0; i < length; i++ {
			var x, y int
			if horizontal {
				x = startX + i*cs
				y = startY
			} else {
				x = startX
				y = startY + i*cs
			}
			if onRoad(x, y) {
				continue
			}
			k := cellKey{x / cs, y / cs}
			if occupied[k] {
				continue
			}
			c := &CoverObject{x: x, y: y, kind: kind}
			if coverOverlapsBuildings(c, walls) {
				continue
			}
			if coverOverlapsBuildings(c, footprintsToRects(footprints)) {
				continue
			}
			occupied[k] = true
			covers = append(covers, c)
		}
	}

	snap := func(v int) int { return (v / cs) * cs }

	// --- 1. Wall extensions: short runs adjacent to building edges ---
	for _, fp := range footprints {
		// Try to extend each face of the building outward.
		faceAttempts := []struct {
			x, y  int
			horiz bool
		}{
			// Extend leftward from west face.
			{fp.x - cs*(wallRunMinLen+rng.Intn(3)), fp.y + fp.h/2, true},
			// Extend rightward from east face.
			{fp.x + fp.w, fp.y + fp.h/2, true},
			// Extend upward from north face.
			{fp.x + fp.w/2, fp.y - cs*(wallRunMinLen+rng.Intn(3)), false},
			// Extend downward from south face.
			{fp.x + fp.w/2, fp.y + fp.h, false},
		}
		for _, fa := range faceAttempts {
			if rng.Float64() > 0.6 {
				continue
			}
			length := wallRunMinLen + rng.Intn(wallRunMaxLen-wallRunMinLen+1)
			kind := CoverChestWall
			if rng.Float64() < 0.3 {
				kind = CoverTallWall
			}
			sx := snap(fa.x)
			sy := snap(fa.y)
			if sx < margin || sy < margin || sx > mapW-margin || sy > mapH-margin {
				continue
			}
			placeLine(sx, sy, length, fa.horiz, kind)
		}
	}

	// --- 2. Freestanding runs (corridors / barriers in open ground) ---
	for attempt := 0; attempt < numWallRuns*6 && len(covers) < numWallRuns*wallRunMaxLen; attempt++ {
		horizontal := rng.Intn(2) == 0
		length := wallRunMinLen + rng.Intn(wallRunMaxLen-wallRunMinLen+1)
		var sx, sy int
		if horizontal {
			sx = margin + snap(rng.Intn((mapW-margin*2)/cs)*cs)
			sy = margin + snap(rng.Intn((mapH-margin*2)/cs)*cs)
		} else {
			sx = margin + snap(rng.Intn((mapW-margin*2)/cs)*cs)
			sy = margin + snap(rng.Intn((mapH-margin*2)/cs)*cs)
		}
		kind := CoverChestWall
		if rng.Float64() < 0.25 {
			kind = CoverTallWall
		}
		placeLine(sx, sy, length, horizontal, kind)
	}

	// --- 3. Corridors: pairs of parallel chest-wall runs forming a lane ---
	for i := 0; i < numCorridors; i++ {
		horizontal := rng.Intn(2) == 0
		length := wallRunMinLen + rng.Intn(wallRunMaxLen-wallRunMinLen+1)
		gap := (3 + rng.Intn(4)) * cs // gap between the two walls
		var sx, sy int
		if horizontal {
			sx = margin + snap(rng.Intn((mapW-margin*2)/cs)*cs)
			sy = margin + snap(rng.Intn(((mapH-margin*2)-gap)/cs)*cs)
			placeLine(sx, sy, length, true, CoverChestWall)
			placeLine(sx, sy+gap, length, true, CoverChestWall)
		} else {
			sx = margin + snap(rng.Intn(((mapW-margin*2)-gap)/cs)*cs)
			sy = margin + snap(rng.Intn((mapH-margin*2)/cs)*cs)
			placeLine(sx, sy, length, false, CoverChestWall)
			placeLine(sx+gap, sy, length, false, CoverChestWall)
		}
	}

	// --- 4. Generate rubble positions from building explosions (returned separately) ---
	rubble := generateExplosionRubble(mapW, mapH, footprints, walls, rng, roads)

	return covers, rubble
}

// footprintsToRects converts building footprints to rects for overlap testing.
func footprintsToRects(fps []rect) []rect { return fps }

// generateExplosionRubble fires numExplosions into the map, preferring to hit
// buildings. Each explosion that overlaps a building removes wall segments
// (handled by applyBuildingDamage) and drops rubble in a radius.
func generateExplosionRubble(mapW, mapH int, footprints []rect, _ []rect, rng *rand.Rand, roads []roadSegment) []*CoverObject {
	cs := coverCellSize
	rubble := make([]*CoverObject, 0, 32)
	type cellKey struct{ cx, cy int }
	placed := map[cellKey]bool{}

	for i := 0; i < numExplosions; i++ {
		// Bias explosions toward building footprints.
		var ex, ey int
		if len(footprints) > 0 && rng.Float64() < 0.75 {
			fp := footprints[rng.Intn(len(footprints))]
			// Hit a random point on the building perimeter or just outside.
			side := rng.Intn(4)
			switch side {
			case 0: // north edge
				ex = fp.x + rng.Intn(fp.w)
				ey = fp.y + rng.Intn(explosionRadius/2)
			case 1: // south edge
				ex = fp.x + rng.Intn(fp.w)
				ey = fp.y + fp.h - rng.Intn(explosionRadius/2)
			case 2: // west edge
				ex = fp.x + rng.Intn(explosionRadius/2)
				ey = fp.y + rng.Intn(fp.h)
			default: // east edge
				ex = fp.x + fp.w - rng.Intn(explosionRadius/2)
				ey = fp.y + rng.Intn(fp.h)
			}
		} else {
			ex = cs * (2 + rng.Intn((mapW-cs*4)/cs))
			ey = cs * (2 + rng.Intn((mapH-cs*4)/cs))
		}

		// Scatter rubble cells within the explosion radius.
		r := explosionRadius / cs
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx*dx+dy*dy > r*r {
					continue
				}
				rx := ((ex / cs) + dx) * cs
				ry := ((ey / cs) + dy) * cs
				if rx < cs || ry < cs || rx > mapW-cs*2 || ry > mapH-cs*2 {
					continue
				}
				k := cellKey{rx / cs, ry / cs}
				if placed[k] {
					continue
				}
				// Don't place rubble on roads — it would visually clash.
				onAnyRoad := false
				for _, rd := range roads {
					if rx >= rd.x && rx < rd.x+rd.w && ry >= rd.y && ry < rd.y+rd.h {
						onAnyRoad = true
						break
					}
				}
				if onAnyRoad {
					continue
				}
				placed[k] = true
				rubble = append(rubble, &CoverObject{x: rx, y: ry, kind: CoverRubble})
			}
		}
	}
	return rubble
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

// FindCoverForThreat searches the provided cover list for the best cover object
// that interposes between (sx,sy) and the threat direction (threatAngle, radians).
// It returns the nearest valid cover object, or nil if none found.
// maxSearchDist is the maximum distance (px) to search.
func FindCoverForThreat(sx, sy, threatAngle float64, covers []*CoverObject, buildings []rect, maxSearchDist float64) *CoverObject {
	var best *CoverObject
	bestScore := -math.MaxFloat64

	for _, c := range covers {
		// Centre of cover cell.
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

		// Prefer closer cover and better defence value.
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
