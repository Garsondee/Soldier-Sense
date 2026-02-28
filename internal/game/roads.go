package game

import (
	"math"
	"math/rand"
)

// roadWidth is the pixel width of a road corridor.
const roadWidth = 64

// roadSegment is an axis-aligned road tile used for collision / overlap tests.
// Curved roads are decomposed into many small segments.
type roadSegment struct {
	x, y  int  // top-left of the road band
	w, h  int  // width and height
	horiz bool // true = horizontal (runs left-right), false = vertical (runs top-bottom)
}

// Rect returns the AABB of this road segment.
func (r *roadSegment) Rect() rect {
	return rect{x: r.x, y: r.y, w: r.w, h: r.h}
}

// roadPolyline stores the visual centreline of a road for smooth rendering.
type roadPolyline struct {
	points [][2]float64 // ordered waypoints (centreline)
	width  float64      // half-width of the road band
}

// --- Cubic Bézier helpers ---

func bezierPoint(p0, p1, p2, p3 [2]float64, t float64) [2]float64 {
	u := 1 - t
	return [2]float64{
		u*u*u*p0[0] + 3*u*u*t*p1[0] + 3*u*t*t*p2[0] + t*t*t*p3[0],
		u*u*u*p0[1] + 3*u*u*t*p1[1] + 3*u*t*t*p2[1] + t*t*t*p3[1],
	}
}

// curvedPolyline generates a smooth polyline from a list of waypoints using
// Catmull-Rom-style tangents converted to cubic Bézier segments.
func curvedPolyline(waypoints [][2]float64, segmentsPerSpan int) [][2]float64 {
	if len(waypoints) < 2 {
		return waypoints
	}
	if segmentsPerSpan < 2 {
		segmentsPerSpan = 2
	}
	out := make([][2]float64, 0, len(waypoints)*segmentsPerSpan)
	for i := 0; i < len(waypoints)-1; i++ {
		p0 := waypoints[i]
		p3 := waypoints[i+1]
		// Tangent estimation (Catmull-Rom style).
		var t0, t1 [2]float64
		if i > 0 {
			t0 = [2]float64{(p3[0] - waypoints[i-1][0]) * 0.25, (p3[1] - waypoints[i-1][1]) * 0.25}
		} else {
			t0 = [2]float64{(p3[0] - p0[0]) * 0.25, (p3[1] - p0[1]) * 0.25}
		}
		if i+2 < len(waypoints) {
			t1 = [2]float64{(waypoints[i+2][0] - p0[0]) * 0.25, (waypoints[i+2][1] - p0[1]) * 0.25}
		} else {
			t1 = [2]float64{(p3[0] - p0[0]) * 0.25, (p3[1] - p0[1]) * 0.25}
		}
		cp1 := [2]float64{p0[0] + t0[0], p0[1] + t0[1]}
		cp2 := [2]float64{p3[0] - t1[0], p3[1] - t1[1]}
		for s := 0; s < segmentsPerSpan; s++ {
			t := float64(s) / float64(segmentsPerSpan)
			out = append(out, bezierPoint(p0, cp1, cp2, p3, t))
		}
	}
	out = append(out, waypoints[len(waypoints)-1])
	return out
}

// --- Road generation ---

// initRoads generates the road network.
// Strategy: 2-3 main roads (mix of horizontal-ish and vertical-ish) that
// curve organically across the battlefield. Each road is defined by waypoints
// that are smoothed into a polyline, then decomposed into small axis-aligned
// segments for collision testing.
func (g *Game) initRoads(rng *rand.Rand) {
	g.roads = g.roads[:0]
	g.roadPolylines = g.roadPolylines[:0]

	W := float64(g.gameWidth)
	H := float64(g.gameHeight)
	hw := float64(roadWidth) / 2

	// Number of roads: 3-4 total (2 predominantly-horizontal + 1-2 predominantly-vertical).
	numH := 2
	numV := 1 + rng.Intn(2) // 1 or 2 vertical roads

	// --- Horizontal-ish roads ---
	hYSlots := []float64{H * 0.35, H * 0.65}
	rng.Shuffle(len(hYSlots), func(i, j int) { hYSlots[i], hYSlots[j] = hYSlots[j], hYSlots[i] })
	for ri := 0; ri < numH; ri++ {
		baseY := hYSlots[ri] + (rng.Float64()-0.5)*H*0.12
		numWaypoints := 5 + rng.Intn(4) // 5-8 waypoints
		waypoints := make([][2]float64, numWaypoints)
		for j := 0; j < numWaypoints; j++ {
			t := float64(j) / float64(numWaypoints-1)
			x := t * W
			// Gentle vertical wander: ±8% of map height
			yOff := (rng.Float64() - 0.5) * H * 0.16
			waypoints[j] = [2]float64{x, baseY + yOff}
		}
		// Clamp ends to map edges.
		waypoints[0][0] = 0
		waypoints[numWaypoints-1][0] = W
		poly := curvedPolyline(waypoints, 12)
		g.addRoadPolyline(poly, hw)
	}

	// --- Vertical-ish roads ---
	vXSlots := []float64{W * 0.35, W * 0.65}
	rng.Shuffle(len(vXSlots), func(i, j int) { vXSlots[i], vXSlots[j] = vXSlots[j], vXSlots[i] })
	for ri := 0; ri < numV; ri++ {
		baseX := vXSlots[ri] + (rng.Float64()-0.5)*W*0.12
		numWaypoints := 5 + rng.Intn(4)
		waypoints := make([][2]float64, numWaypoints)
		for j := 0; j < numWaypoints; j++ {
			t := float64(j) / float64(numWaypoints-1)
			y := t * H
			xOff := (rng.Float64() - 0.5) * W * 0.16
			waypoints[j] = [2]float64{baseX + xOff, y}
		}
		waypoints[0][1] = 0
		waypoints[numWaypoints-1][1] = H
		poly := curvedPolyline(waypoints, 12)
		g.addRoadPolyline(poly, hw)
	}
}

// addRoadPolyline stores a polyline for rendering and decomposes it into
// axis-aligned roadSegments for collision testing.
func (g *Game) addRoadPolyline(poly [][2]float64, halfWidth float64) {
	g.roadPolylines = append(g.roadPolylines, roadPolyline{
		points: poly,
		width:  halfWidth,
	})

	// Rasterise: for each sub-segment, generate an AABB tile.
	step := float64(roadWidth) / 2 // tile every half-width for smooth coverage
	for i := 0; i < len(poly)-1; i++ {
		ax, ay := poly[i][0], poly[i][1]
		bx, by := poly[i+1][0], poly[i+1][1]
		dx := bx - ax
		dy := by - ay
		segLen := math.Sqrt(dx*dx + dy*dy)
		if segLen < 1 {
			continue
		}
		steps := int(math.Ceil(segLen / step))
		if steps < 1 {
			steps = 1
		}
		for s := 0; s <= steps; s++ {
			t := float64(s) / float64(steps)
			cx := ax + dx*t
			cy := ay + dy*t
			// Axis-aligned tile centred on (cx,cy).
			tileW := int(halfWidth*2) + 4 // slight padding for coverage
			tileH := tileW
			tx := int(cx) - tileW/2
			ty := int(cy) - tileH/2
			horiz := math.Abs(dx) > math.Abs(dy)
			g.roads = append(g.roads, roadSegment{
				x: tx, y: ty, w: tileW, h: tileH, horiz: horiz,
			})
		}
	}
}

// pointOnRoad returns true if world point (x,y) is inside any road band.
func (g *Game) pointOnRoad(x, y int) bool {
	// Fast check: use polyline distance (more accurate for curves).
	px, py := float64(x), float64(y)
	for _, rp := range g.roadPolylines {
		for i := 0; i < len(rp.points)-1; i++ {
			ax, ay := rp.points[i][0], rp.points[i][1]
			bx, by := rp.points[i+1][0], rp.points[i+1][1]
			d := pointToSegmentDist(px, py, ax, ay, bx, by)
			if d <= rp.width {
				return true
			}
		}
	}
	return false
}

// rectOverlapsRoad returns true if the given rect overlaps any road.
func (g *Game) rectOverlapsRoad(r rect) bool {
	// Check rect centre + corners against polyline distance for accuracy.
	cx := float64(r.x) + float64(r.w)/2
	cy := float64(r.y) + float64(r.h)/2
	halfDiag := math.Sqrt(float64(r.w*r.w+r.h*r.h)) / 2
	if false {
		_ = g.pointOnRoad(r.x, r.y)
	}

	for _, rp := range g.roadPolylines {
		for i := 0; i < len(rp.points)-1; i++ {
			ax, ay := rp.points[i][0], rp.points[i][1]
			bx, by := rp.points[i+1][0], rp.points[i+1][1]
			d := pointToSegmentDist(cx, cy, ax, ay, bx, by)
			if d <= rp.width+halfDiag {
				return true
			}
		}
	}
	return false
}

// pointToSegmentDist returns the minimum distance from point (px,py) to the
// line segment (ax,ay)-(bx,by).
func pointToSegmentDist(px, py, ax, ay, bx, by float64) float64 {
	dx := bx - ax
	dy := by - ay
	lenSq := dx*dx + dy*dy
	if lenSq < 1e-9 {
		return math.Sqrt((px-ax)*(px-ax) + (py-ay)*(py-ay))
	}
	t := ((px-ax)*dx + (py-ay)*dy) / lenSq
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	cx := ax + t*dx
	cy := ay + t*dy
	return math.Sqrt((px-cx)*(px-cx) + (py-cy)*(py-cy))
}

// buildingCandidatesAlongRoads returns a shuffled list of candidate building
// positions that sit adjacent to the road polylines (with a small gap).
// Buildings are spaced along the road at varying intervals.
func (g *Game) buildingCandidatesAlongRoads(
	rng *rand.Rand,
	unitW, unitH int,
	minGap, maxGap int,
) []rect {
	results := make([]rect, 0, 128)

	for _, rp := range g.roadPolylines {
		gap := float64(minGap + rng.Intn(maxGap-minGap+1))
		totalLen := polylineLength(rp.points)
		if totalLen < 1 {
			continue
		}
		// Walk along the polyline.
		along := 0.0
		for along < totalLen {
			px, py, nx, ny := polylinePointAndNormal(rp.points, along)
			// Place candidates on both sides of the road.
			for _, side := range []float64{-1, 1} {
				offset := rp.width + gap + float64(unitW)/2
				cx := px + nx*side*offset
				cy := py + ny*side*offset
				bx := int(cx) - unitW/2
				by := int(cy) - unitH/2
				// Snap to 16px grid.
				bx = (bx / 16) * 16
				by = (by / 16) * 16
				if bx < 64 || by < 64 || bx+unitW > g.gameWidth-64 || by+unitH > g.gameHeight-64 {
					continue
				}
				results = append(results, rect{x: bx, y: by, w: unitW, h: unitH})
			}
			// Variable spacing.
			along += float64(unitW) + float64(32+rng.Intn(5)*32)
		}
	}

	// Also add some off-road candidates in the spaces between roads.
	offRoadCount := len(results) / 3
	for i := 0; i < offRoadCount; i++ {
		bx := 96 + rng.Intn(g.gameWidth-192-unitW)
		by := 96 + rng.Intn(g.gameHeight-192-unitH)
		bx = (bx / 16) * 16
		by = (by / 16) * 16
		results = append(results, rect{x: bx, y: by, w: unitW, h: unitH})
	}

	// Shuffle so we don't always place in the same order.
	rng.Shuffle(len(results), func(i, j int) {
		results[i], results[j] = results[j], results[i]
	})
	return results
}

// --- Polyline geometry helpers ---

func polylineLength(pts [][2]float64) float64 {
	total := 0.0
	for i := 1; i < len(pts); i++ {
		dx := pts[i][0] - pts[i-1][0]
		dy := pts[i][1] - pts[i-1][1]
		total += math.Sqrt(dx*dx + dy*dy)
	}
	return total
}

// polylinePointAndNormal returns the position and perpendicular normal at
// a given distance along the polyline.
func polylinePointAndNormal(pts [][2]float64, dist float64) (px, py, nx, ny float64) {
	accum := 0.0
	for i := 1; i < len(pts); i++ {
		dx := pts[i][0] - pts[i-1][0]
		dy := pts[i][1] - pts[i-1][1]
		segLen := math.Sqrt(dx*dx + dy*dy)
		if segLen < 1e-9 {
			continue
		}
		if accum+segLen >= dist || i == len(pts)-1 {
			t := (dist - accum) / segLen
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			px = pts[i-1][0] + dx*t
			py = pts[i-1][1] + dy*t
			// Normal is perpendicular to the tangent.
			invLen := 1.0 / segLen
			nx = -dy * invLen
			ny = dx * invLen
			return
		}
		accum += segLen
	}
	// Fallback: end of polyline.
	last := pts[len(pts)-1]
	px, py = last[0], last[1]
	if len(pts) >= 2 {
		prev := pts[len(pts)-2]
		dx := last[0] - prev[0]
		dy := last[1] - prev[1]
		l := math.Sqrt(dx*dx + dy*dy)
		if l > 0 {
			nx = -dy / l
			ny = dx / l
		}
	}
	return
}
