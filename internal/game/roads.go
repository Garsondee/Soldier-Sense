package game

import (
	"math/rand"
)

// roadWidth is the pixel width of a road corridor.
const roadWidth = 64

// roadSegment is an axis-aligned road band across the battlefield.
type roadSegment struct {
	x, y  int  // top-left of the road band
	w, h  int  // width and height
	horiz bool // true = horizontal (runs left-right), false = vertical (runs top-bottom)
}

// Rect returns the AABB of this road segment.
func (r *roadSegment) Rect() rect {
	return rect{x: r.x, y: r.y, w: r.w, h: r.h}
}

// initRoads generates the road network.
// Strategy: 1-2 horizontal roads + 1-2 vertical roads, meeting near the map centre.
// Roads bisect or trisect the map with slight random offsets from the exact centre.
func (g *Game) initRoads() {
	rng := rand.New(rand.NewSource(0xB0AD)) // deterministic road layout
	g.roads = g.roads[:0]

	W := g.gameWidth
	H := g.gameHeight
	half := roadWidth / 2

	// Horizontal roads: 1 or 2 bands running full width.
	// Primary: near vertical centre, jittered ±15% of height.
	jitterH := int(float64(H) * 0.15)
	hRoad1Y := H/2 - half + rng.Intn(jitterH*2+1) - jitterH
	g.roads = append(g.roads, roadSegment{
		x: 0, y: hRoad1Y, w: W, h: roadWidth, horiz: true,
	})

	// Secondary horizontal road: upper or lower third.
	if rng.Intn(2) == 0 {
		hRoad2Y := H/4 - half + rng.Intn(jitterH) - jitterH/2
		g.roads = append(g.roads, roadSegment{
			x: 0, y: hRoad2Y, w: W, h: roadWidth, horiz: true,
		})
	} else {
		hRoad2Y := 3*H/4 - half + rng.Intn(jitterH) - jitterH/2
		g.roads = append(g.roads, roadSegment{
			x: 0, y: hRoad2Y, w: W, h: roadWidth, horiz: true,
		})
	}

	// Vertical roads: 1 or 2 bands running full height.
	jitterW := int(float64(W) * 0.15)
	vRoad1X := W/2 - half + rng.Intn(jitterW*2+1) - jitterW
	g.roads = append(g.roads, roadSegment{
		x: vRoad1X, y: 0, w: roadWidth, h: H, horiz: false,
	})

	// Secondary vertical road.
	if rng.Intn(2) == 0 {
		vRoad2X := W/4 - half + rng.Intn(jitterW) - jitterW/2
		g.roads = append(g.roads, roadSegment{
			x: vRoad2X, y: 0, w: roadWidth, h: H, horiz: false,
		})
	} else {
		vRoad2X := 3*W/4 - half + rng.Intn(jitterW) - jitterW/2
		g.roads = append(g.roads, roadSegment{
			x: vRoad2X, y: 0, w: roadWidth, h: H, horiz: false,
		})
	}
}

// pointOnRoad returns true if world point (x,y) is inside any road band.
func (g *Game) pointOnRoad(x, y int) bool {
	for i := range g.roads {
		r := &g.roads[i]
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return true
		}
	}
	return false
}

// rectOverlapsRoad returns true if the given rect overlaps any road.
func (g *Game) rectOverlapsRoad(r rect) bool {
	for i := range g.roads {
		rd := &g.roads[i]
		if r.x < rd.x+rd.w && r.x+r.w > rd.x &&
			r.y < rd.y+rd.h && r.y+r.h > rd.y {
			return true
		}
	}
	return false
}

// buildingCandidatesAlongRoads returns a shuffled list of candidate building
// positions that sit adjacent to a road edge (with a small gap).
// Buildings are snapped to the road edge and spaced along the road.
func (g *Game) buildingCandidatesAlongRoads(
	rng *rand.Rand,
	unitW, unitH int,
	minGap, maxGap int,
) []rect {
	snap := 64 // align to 64px grid
	results := make([]rect, 0, 64)

	for i := range g.roads {
		rd := &g.roads[i]
		gap := minGap + rng.Intn(maxGap-minGap+1)

		if rd.horiz {
			// Buildings above and below this horizontal road.
			sides := []int{
				rd.y - unitH - gap, // above
				rd.y + rd.h + gap,  // below
			}
			for _, buildY := range sides {
				if buildY < snap || buildY+unitH > g.gameHeight-snap {
					continue
				}
				// Walk along the road placing candidates at intervals.
				x := snap
				for x+unitW < g.gameWidth-snap {
					spacing := snap + rng.Intn(3)*snap
					results = append(results, rect{x: x, y: buildY, w: unitW, h: unitH})
					x += unitW + spacing
				}
			}
		} else {
			// Buildings left and right of this vertical road.
			sides := []int{
				rd.x - unitW - gap, // left
				rd.x + rd.w + gap,  // right
			}
			for _, buildX := range sides {
				if buildX < snap || buildX+unitW > g.gameWidth-snap {
					continue
				}
				y := snap
				for y+unitH < g.gameHeight-snap {
					spacing := snap + rng.Intn(3)*snap
					results = append(results, rect{x: buildX, y: y, w: unitW, h: unitH})
					y += unitH + spacing
				}
			}
		}
	}

	// Shuffle so we don't always place in the same order.
	rng.Shuffle(len(results), func(i, j int) {
		results[i], results[j] = results[j], results[i]
	})
	return results
}
