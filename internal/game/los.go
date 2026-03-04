package game

import "math"

const losIndexCellSize = 128

type losItem struct {
	minX float64
	minY float64
	maxX float64
	maxY float64
}

// LOSIndex stores broad-phase bins for LOS-blocking geometry.
type LOSIndex struct {
	buildingItems []losItem
	coverItems    []losItem

	buildingBins map[[2]int][]int
	coverBins    map[[2]int][]int

	scratchSeenBuildings map[int]uint32
	scratchSeenCovers    map[int]uint32
	scratchBuildingIdx   []int
	scratchCoverIdx      []int

	cellSize        int
	buildingSeenGen uint32
	coverSeenGen    uint32
}

// NewLOSIndex builds a shared broad-phase index for building and cover LOS checks.
func NewLOSIndex(buildings []rect, covers []*CoverObject) *LOSIndex {
	idx := &LOSIndex{
		cellSize:             losIndexCellSize,
		buildingItems:        make([]losItem, len(buildings)),
		coverBins:            make(map[[2]int][]int),
		buildingBins:         make(map[[2]int][]int),
		scratchSeenBuildings: make(map[int]uint32),
		scratchSeenCovers:    make(map[int]uint32),
	}

	for i, b := range buildings {
		item := losItem{
			minX: float64(b.x),
			minY: float64(b.y),
			maxX: float64(b.x + b.w),
			maxY: float64(b.y + b.h),
		}
		idx.buildingItems[i] = item
		idx.insertToBins(idx.buildingBins, i, item)
	}

	for _, c := range covers {
		if c == nil || !c.BlocksLOS() {
			continue
		}
		item := losItem{
			minX: float64(c.x),
			minY: float64(c.y),
			maxX: float64(c.x + coverCellSize),
			maxY: float64(c.y + coverCellSize),
		}
		idx.coverItems = append(idx.coverItems, item)
		idx.insertToBins(idx.coverBins, len(idx.coverItems)-1, item)
	}

	return idx
}

func (idx *LOSIndex) insertToBins(bins map[[2]int][]int, itemIdx int, item losItem) {
	minCX, minCY, maxCX, maxCY := idx.cellsForAABB(item.minX, item.minY, item.maxX, item.maxY)
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			key := [2]int{cx, cy}
			bins[key] = append(bins[key], itemIdx)
		}
	}
}

func (idx *LOSIndex) queryBuildingCandidateIndices(ax, ay, bx, by float64) []int {
	idx.buildingSeenGen++
	if idx.buildingSeenGen == 0 {
		idx.buildingSeenGen = 1
		for k := range idx.scratchSeenBuildings {
			delete(idx.scratchSeenBuildings, k)
		}
	}
	return idx.queryCandidateIndices(idx.buildingBins, idx.scratchSeenBuildings, idx.buildingSeenGen, &idx.scratchBuildingIdx, ax, ay, bx, by)
}

func (idx *LOSIndex) queryCoverCandidateIndices(ax, ay, bx, by float64) []int {
	idx.coverSeenGen++
	if idx.coverSeenGen == 0 {
		idx.coverSeenGen = 1
		for k := range idx.scratchSeenCovers {
			delete(idx.scratchSeenCovers, k)
		}
	}
	return idx.queryCandidateIndices(idx.coverBins, idx.scratchSeenCovers, idx.coverSeenGen, &idx.scratchCoverIdx, ax, ay, bx, by)
}

func (idx *LOSIndex) queryCandidateIndices(
	bins map[[2]int][]int,
	seen map[int]uint32,
	gen uint32,
	out *[]int,
	ax, ay, bx, by float64,
) []int {
	candidates := (*out)[:0]
	minCX, minCY, maxCX, maxCY := idx.cellsForSegment(ax, ay, bx, by)
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			key := [2]int{cx, cy}
			for _, itemIdx := range bins[key] {
				if seen[itemIdx] == gen {
					continue
				}
				seen[itemIdx] = gen
				candidates = append(candidates, itemIdx)
			}
		}
	}
	*out = candidates
	return candidates
}

func (idx *LOSIndex) cellsForSegment(ax, ay, bx, by float64) (int, int, int, int) {
	minX := math.Min(ax, bx)
	minY := math.Min(ay, by)
	maxX := math.Max(ax, bx)
	maxY := math.Max(ay, by)
	return idx.cellsForAABB(minX, minY, maxX, maxY)
}

func (idx *LOSIndex) cellsForAABB(minX, minY, maxX, maxY float64) (int, int, int, int) {
	cs := float64(idx.cellSize)
	minCX := int(math.Floor(minX / cs))
	minCY := int(math.Floor(minY / cs))
	maxCX := int(math.Floor(maxX / cs))
	maxCY := int(math.Floor(maxY / cs))
	return minCX, minCY, maxCX, maxCY
}

// HasLineOfSight returns true if a straight line from (ax,ay) to (bx,by)
// does not intersect any building rectangle. Uses simple ray-vs-AABB tests.
func HasLineOfSight(ax, ay, bx, by float64, buildings []rect) bool {
	return HasLineOfSightIndexed(ax, ay, bx, by, buildings, nil)
}

// HasLineOfSightIndexed returns true if a straight line from (ax,ay) to (bx,by)
// does not intersect any building rectangle, using idx as a broad-phase if present.
func HasLineOfSightIndexed(ax, ay, bx, by float64, buildings []rect, idx *LOSIndex) bool {
	if idx == nil {
		for _, b := range buildings {
			if rayIntersectsAABB(ax, ay, bx, by,
				float64(b.x), float64(b.y),
				float64(b.x+b.w), float64(b.y+b.h)) {
				return false
			}
		}
		return true
	}

	for _, bi := range idx.queryBuildingCandidateIndices(ax, ay, bx, by) {
		b := idx.buildingItems[bi]
		if rayIntersectsAABB(ax, ay, bx, by, b.minX, b.minY, b.maxX, b.maxY) {
			return false
		}
	}
	return true
}

// HasLineOfSightWithCoverIndexed returns true if LOS is not blocked by buildings or LOS-blocking covers,
// using idx as a broad-phase when present.
func HasLineOfSightWithCoverIndexed(ax, ay, bx, by float64, buildings []rect, covers []*CoverObject, idx *LOSIndex) bool {
	if !HasLineOfSightIndexed(ax, ay, bx, by, buildings, idx) {
		return false
	}

	if idx == nil {
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

	for _, ci := range idx.queryCoverCandidateIndices(ax, ay, bx, by) {
		c := idx.coverItems[ci]
		if rayIntersectsAABB(ax, ay, bx, by, c.minX, c.minY, c.maxX, c.maxY) {
			return false
		}
	}
	return true
}

// HasLineOfSightWithCover returns true if a straight line from (ax,ay) to (bx,by)
// is not blocked by buildings or tall-wall cover objects.
// Chest walls and rubble do not block LOS.
func HasLineOfSightWithCover(ax, ay, bx, by float64, buildings []rect, covers []*CoverObject) bool {
	return HasLineOfSightWithCoverIndexed(ax, ay, bx, by, buildings, covers, nil)
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

// HasLineOfSightThroughWindow checks if LOS from (ax,ay) to (bx,by) is blocked by buildings,
// or if it passes through a window. Returns (hasLOS, throughWindow, windowPenalty).
// Phase 3: Window-based partial detection for building interiors.
func HasLineOfSightThroughWindow(ax, ay, bx, by float64, buildings, windows []rect) (bool, bool, float64) {
	// First check if any building blocks the ray
	var blockingBuilding *rect
	for i := range buildings {
		b := &buildings[i]
		if rayIntersectsAABB(ax, ay, bx, by,
			float64(b.x), float64(b.y),
			float64(b.x+b.w), float64(b.y+b.h)) {
			blockingBuilding = b
			break
		}
	}

	// If no building blocks, we have clear LOS
	if blockingBuilding == nil {
		return true, false, 0.0
	}

	// Building blocks - check if ray passes through a window
	for i := range windows {
		w := &windows[i]
		if rayIntersectsAABB(ax, ay, bx, by,
			float64(w.x), float64(w.y),
			float64(w.x+w.w), float64(w.y+w.h)) {
			// Ray passes through window - partial LOS with penalty
			// Interior environment multiplier: 2.8x concealment
			return true, true, 2.8
		}
	}

	// Building blocks and no window - no LOS
	return false, false, 0.0
}
