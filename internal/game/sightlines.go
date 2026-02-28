package game

import "math"

// sightlineRayCount is how many evenly-spaced rays we cast from a position.
const sightlineRayCount = 24

// sightlineMaxCells is the max ray length in grid cells.
const sightlineMaxCells = 20

// ScoreSightline returns a 0-1 score for how many grid cells are visible from
// world position (wx, wy). Higher = more open sightlines. Buildings block rays.
// This is moderately expensive so should be called sparingly (not every tick).
func ScoreSightline(wx, wy float64, ng *NavGrid, buildings []rect) float64 {
	if ng == nil {
		return 0.5
	}
	totalCells := 0
	visibleCells := 0
	step := 2 * math.Pi / float64(sightlineRayCount)

	for i := 0; i < sightlineRayCount; i++ {
		angle := float64(i) * step
		dx := math.Cos(angle)
		dy := math.Sin(angle)
		for d := 1; d <= sightlineMaxCells; d++ {
			totalCells++
			px := wx + dx*float64(d*cellSize)
			py := wy + dy*float64(d*cellSize)
			cx, cy := WorldToCell(px, py)
			if ng.IsBlocked(cx, cy) {
				break // this ray is done
			}
			// Also check LOS against building AABBs for accuracy.
			if rayHitsAnyBuilding(wx, wy, px, py, buildings) {
				break
			}
			visibleCells++
		}
	}

	if totalCells == 0 {
		return 0.5
	}
	return float64(visibleCells) / float64(totalCells)
}

// rayHitsAnyBuilding returns true if the ray from (ox,oy) to (ex,ey) intersects
// any building AABB before reaching the endpoint.
func rayHitsAnyBuilding(ox, oy, ex, ey float64, buildings []rect) bool {
	for _, b := range buildings {
		t, hit := rayAABBHitT(
			ox, oy, ex, ey,
			float64(b.x), float64(b.y),
			float64(b.x+b.w), float64(b.y+b.h),
		)
		if hit && t < 1.0 {
			return true
		}
	}
	return false
}

// FindBestSightlinePosition searches nearby walkable cells for the one with the
// highest sightline score. Returns the world position and the score.
// searchRadius is in grid cells.
func FindBestSightlinePosition(wx, wy float64, searchRadius int, ng *NavGrid, buildings []rect) (float64, float64, float64) {
	cx, cy := WorldToCell(wx, wy)
	bestX, bestY := wx, wy
	bestScore := 0.0

	for dy := -searchRadius; dy <= searchRadius; dy++ {
		for dx := -searchRadius; dx <= searchRadius; dx++ {
			nx, ny := cx+dx, cy+dy
			if ng.IsBlocked(nx, ny) {
				continue
			}
			twx, twy := CellToWorld(nx, ny)
			score := ScoreSightline(twx, twy, ng, buildings)
			if score > bestScore {
				bestScore = score
				bestX, bestY = twx, twy
			}
		}
	}
	return bestX, bestY, bestScore
}
