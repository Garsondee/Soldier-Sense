package game

import "math"

// HasLineOfSight returns true if a straight line from (ax,ay) to (bx,by)
// does not intersect any building rectangle. Uses simple ray-vs-AABB tests.
func HasLineOfSight(ax, ay, bx, by float64, buildings []rect) bool {
	for _, b := range buildings {
		if rayIntersectsAABB(ax, ay, bx, by,
			float64(b.x), float64(b.y),
			float64(b.x+b.w), float64(b.y+b.h)) {
			return false
		}
	}
	return true
}

// HasLineOfSightWithCover returns true if a straight line from (ax,ay) to (bx,by)
// is not blocked by buildings or tall-wall cover objects.
// Chest walls and rubble do not block LOS.
func HasLineOfSightWithCover(ax, ay, bx, by float64, buildings []rect, covers []*CoverObject) bool {
	if !HasLineOfSight(ax, ay, bx, by, buildings) {
		return false
	}
	for _, c := range covers {
		if !c.BlocksLOS() {
			continue
		}
		if rayIntersectsAABB(ax, ay, bx, by,
			float64(c.x), float64(c.y),
			float64(c.x+coverCellSize), float64(c.y+coverCellSize)) {
			return false
		}
	}
	return true
}

// rayAABBHitT returns the first segment parameter t in [0,1] where the line
// from (ox,oy)->(ex,ey) enters the AABB. The bool is false when no hit exists.
func rayAABBHitT(ox, oy, ex, ey, minX, minY, maxX, maxY float64) (float64, bool) {
	dx := ex - ox
	dy := ey - oy

	tMin := 0.0
	tMax := 1.0

	// Check X slab
	if math.Abs(dx) < 1e-12 {
		if ox < minX || ox > maxX {
			return 0, false
		}
	} else {
		invD := 1.0 / dx
		t1 := (minX - ox) * invD
		t2 := (maxX - ox) * invD
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tMin = math.Max(tMin, t1)
		tMax = math.Min(tMax, t2)
		if tMin > tMax {
			return 0, false
		}
	}

	// Check Y slab
	if math.Abs(dy) < 1e-12 {
		if oy < minY || oy > maxY {
			return 0, false
		}
	} else {
		invD := 1.0 / dy
		t1 := (minY - oy) * invD
		t2 := (maxY - oy) * invD
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tMin = math.Max(tMin, t1)
		tMax = math.Min(tMax, t2)
		if tMin > tMax {
			return 0, false
		}
	}

	if tMax < 0 || tMin > 1 {
		return 0, false
	}

	if tMin < 0 {
		tMin = 0
	}
	if tMin > 1 {
		return 0, false
	}

	return tMin, true
}

// rayIntersectsAABB checks if the line segment from (ox,oy)->(ex,ey)
// intersects the axis-aligned bounding box defined by (minX,minY)-(maxX,maxY).
func rayIntersectsAABB(ox, oy, ex, ey, minX, minY, maxX, maxY float64) bool {
	_, hit := rayAABBHitT(ox, oy, ex, ey, minX, minY, maxX, maxY)
	return hit
}
