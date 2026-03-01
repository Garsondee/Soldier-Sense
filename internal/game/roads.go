package game

import "math"

// pointToSegmentDist returns the minimum distance from point (px,py) to the
// line segment (ax,ay)-(bx,by). Used by combat ricochet calculations.
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
