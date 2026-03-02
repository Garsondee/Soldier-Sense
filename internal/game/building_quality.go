package game

import "math"

// BuildingQuality stores pre-computed tactical metrics for a building footprint.
type BuildingQuality struct {
	TacticalValue      float64 // 0-1: overall tactical worth
	CoverQuality       float64 // 0-1: protection level (size, solidity)
	SightlineScore     float64 // 0-1: observation value
	AccessibilityScore float64 // 0-1: entry/exit options (doors, windows)
	InteriorComplexity float64 // 0-1: room count, layout richness
	DominanceScore     float64 // 0-1: height/position advantage
}

// ComputeBuildingQualities analyzes all building footprints and returns quality metrics.
// This should be called once at map initialization.
func ComputeBuildingQualities(
	footprints []rect,
	buildings []rect,
	windows []rect,
	mapW, mapH int,
	navGrid *NavGrid,
) []BuildingQuality {
	qualities := make([]BuildingQuality, len(footprints))

	for i, fp := range footprints {
		qualities[i] = computeSingleBuildingQuality(fp, buildings, windows, mapW, mapH, navGrid)
	}

	return qualities
}

func computeSingleBuildingQuality(
	fp rect,
	buildings []rect,
	windows []rect,
	mapW, mapH int,
	navGrid *NavGrid,
) BuildingQuality {
	q := BuildingQuality{}

	// 1. Cover Quality: based on size and perimeter-to-area ratio
	area := float64(fp.w * fp.h)
	perimeter := float64(2 * (fp.w + fp.h))

	// Larger buildings provide more cover options
	normalizedArea := math.Min(1.0, area/50000.0) // 50k px² = large building

	// More compact buildings (lower perimeter/area) are better cover
	compactness := 1.0 - math.Min(1.0, perimeter/math.Sqrt(area)/8.0)

	q.CoverQuality = 0.6*normalizedArea + 0.4*compactness

	// 2. Accessibility Score: count doors and windows
	doorCount := 0
	windowCount := 0

	// Count windows that border this footprint
	for _, w := range windows {
		if rectsAdjacent(fp, w) {
			windowCount++
		}
	}

	// Estimate doors: gaps in building walls along footprint perimeter
	doorCount = estimateDoorCount(fp, buildings)

	// More access points = better (up to a point)
	accessPoints := float64(doorCount + windowCount)
	q.AccessibilityScore = math.Min(1.0, accessPoints/8.0) // 8+ access points = max score

	// 3. Sightline Score: how much of the map is visible from building center
	cx := float64(fp.x + fp.w/2)
	cy := float64(fp.y + fp.h/2)

	if navGrid != nil {
		q.SightlineScore = ScoreSightline(cx, cy, navGrid, buildings)
	} else {
		// Fallback: buildings near map edges have worse sightlines
		edgeDist := math.Min(
			math.Min(cx, float64(mapW)-cx),
			math.Min(cy, float64(mapH)-cy),
		)
		q.SightlineScore = math.Min(1.0, edgeDist/300.0)
	}

	// 4. Interior Complexity: estimate based on size and shape
	// Larger buildings likely have more rooms
	roomEstimate := math.Sqrt(area) / 48.0                 // ~1 room per 48px of building dimension
	q.InteriorComplexity = math.Min(1.0, roomEstimate/4.0) // 4+ rooms = max complexity

	// 5. Dominance Score: position on map and size
	// Buildings in the center of the map have better dominance
	mapCenterX := float64(mapW) / 2.0
	mapCenterY := float64(mapH) / 2.0
	distToCenter := math.Hypot(cx-mapCenterX, cy-mapCenterY)
	maxDistToCenter := math.Hypot(mapCenterX, mapCenterY)
	centralityScore := 1.0 - math.Min(1.0, distToCenter/maxDistToCenter)

	// Larger buildings dominate more
	sizeScore := normalizedArea

	q.DominanceScore = 0.5*centralityScore + 0.5*sizeScore

	// 6. Overall Tactical Value: weighted combination
	q.TacticalValue =
		q.CoverQuality*0.25 +
			q.SightlineScore*0.30 +
			q.AccessibilityScore*0.20 +
			q.InteriorComplexity*0.10 +
			q.DominanceScore*0.15

	return q
}

// rectsAdjacent returns true if two rects share an edge or corner.
func rectsAdjacent(a, b rect) bool {
	// Check if rectangles are within 1 cell of each other
	return !(a.x+a.w < b.x-cellSize || b.x+b.w < a.x-cellSize ||
		a.y+a.h < b.y-cellSize || b.y+b.h < a.y-cellSize)
}

// estimateDoorCount counts gaps in building walls along the footprint perimeter.
// A door is a gap where the footprint edge is not covered by a building wall.
func estimateDoorCount(fp rect, buildings []rect) int {
	doors := 0

	// Check each edge of the footprint for gaps
	// North edge
	doors += countGapsAlongEdge(fp.x, fp.y, fp.x+fp.w, fp.y, buildings)
	// South edge
	doors += countGapsAlongEdge(fp.x, fp.y+fp.h, fp.x+fp.w, fp.y+fp.h, buildings)
	// West edge
	doors += countGapsAlongEdge(fp.x, fp.y, fp.x, fp.y+fp.h, buildings)
	// East edge
	doors += countGapsAlongEdge(fp.x+fp.w, fp.y, fp.x+fp.w, fp.y+fp.h, buildings)

	return doors
}

// countGapsAlongEdge counts the number of gaps (potential doors) along a line segment.
func countGapsAlongEdge(x1, y1, x2, y2 int, buildings []rect) int {
	// Simple heuristic: check every cellSize pixels along the edge
	gaps := 0
	inGap := false

	dx := x2 - x1
	dy := y2 - y1
	length := int(math.Sqrt(float64(dx*dx + dy*dy)))

	if length == 0 {
		return 0
	}

	steps := length / cellSize
	if steps == 0 {
		steps = 1
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		px := x1 + int(float64(dx)*t)
		py := y1 + int(float64(dy)*t)

		// Check if this point is covered by a building wall
		covered := false
		for _, b := range buildings {
			if px >= b.x && px < b.x+b.w && py >= b.y && py < b.y+b.h {
				covered = true
				break
			}
		}

		if !covered && !inGap {
			gaps++
			inGap = true
		} else if covered {
			inGap = false
		}
	}

	return gaps
}
