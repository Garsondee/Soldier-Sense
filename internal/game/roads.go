package game

import "math"

// sqr returns the square of x.
func sqr(x float64) float64 {
	return x * x
}

// withinRadius returns true if the offset (dx, dy) from an origin falls
// within radius r. Uses squared distance to avoid sqrt in hot paths.
func withinRadius(dx, dy, r float64) bool {
	return dx*dx+dy*dy <= r*r
}

// withinRadius2 is a variant of withinRadius that takes r^2 directly, to avoid redundant squaring in hot paths.
func withinRadius2(dx, dy, r2 float64) bool {
	return dx*dx+dy*dy <= r2
}

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
