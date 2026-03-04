package game

import (
	"container/heap"
	"math"
	"sort"
	"time"
)

const cellSize = 16

// NavGrid is a 2D walkability grid where true = blocked.
type NavGrid struct { //nolint:govet
	cols    int
	rows    int
	blocked []bool

	pfCalls          int64
	pfFails          int64
	pfExpandedNodes  int64
	pfRuntimeNS      int64
	pfRuntimeSamples []int64
}

// PathfindingStatsSnapshot captures aggregate A* runtime counters.
type PathfindingStatsSnapshot struct {
	CallCount          int64
	FailCount          int64
	ExpandedNodes      int64
	AverageRuntimeMS   float64
	P95RuntimeMS       float64
	FailureRatePercent float64
}

// NewNavGrid builds a walkability grid from the map dimensions and buildings.
// Each cell that overlaps a building (with padding for soldier radius) is blocked.
// Windows block movement (no padding). Tall wall cover objects are also blocked.
func NewNavGrid(mapW, mapH int, buildings []rect, soldierRadius int, covers []*CoverObject, windows []rect) *NavGrid {
	cols := mapW / cellSize
	rows := mapH / cellSize
	ng := &NavGrid{
		cols:    cols,
		rows:    rows,
		blocked: make([]bool, cols*rows),
	}

	pad := soldierRadius
	for _, b := range buildings {
		// Expand building bounds by soldier radius so paths keep clearance.
		bx0 := b.x - pad
		by0 := b.y - pad
		bx1 := b.x + b.w + pad
		by1 := b.y + b.h + pad

		cMinX := max(0, bx0/cellSize)
		cMinY := max(0, by0/cellSize)
		cMaxX := min(cols-1, (bx1-1)/cellSize)
		cMaxY := min(rows-1, (by1-1)/cellSize)

		for cy := cMinY; cy <= cMaxY; cy++ {
			for cx := cMinX; cx <= cMaxX; cx++ {
				ng.blocked[cy*cols+cx] = true
			}
		}
	}

	// Windows block movement but not LOS — no padding, just the cell itself.
	for _, w := range windows {
		cx := w.x / cellSize
		cy := w.y / cellSize
		if cx >= 0 && cx < cols && cy >= 0 && cy < rows {
			ng.blocked[cy*cols+cx] = true
		}
	}

	// Tall walls block movement like buildings (no padding — they are thin).
	for _, c := range covers {
		if !c.BlocksMovement() {
			continue
		}
		cx := c.x / cellSize
		cy := c.y / cellSize
		if cx >= 0 && cx < cols && cy >= 0 && cy < rows {
			ng.blocked[cy*cols+cx] = true
		}
	}

	return ng
}

// IsBlocked returns true if the cell at (cx, cy) is not walkable.
func (ng *NavGrid) IsBlocked(cx, cy int) bool {
	if cx < 0 || cy < 0 || cx >= ng.cols || cy >= ng.rows {
		return true
	}
	return ng.blocked[cy*ng.cols+cx]
}

// WorldToCell converts world pixel coordinates to grid cell coordinates.
func WorldToCell(wx, wy float64) (int, int) {
	return int(wx) / cellSize, int(wy) / cellSize
}

// CellToWorld converts grid cell coordinates to world pixel center.
func CellToWorld(cx, cy int) (float64, float64) {
	return float64(cx*cellSize) + float64(cellSize)/2, float64(cy*cellSize) + float64(cellSize)/2
}

// --- A* pathfinding ---

type pathNode struct { //nolint:govet
	cx, cy int
	g, h   float64
	parent *pathNode
	index  int // heap index
}

type openList []*pathNode

func (ol openList) Len() int           { return len(ol) }
func (ol openList) Less(i, j int) bool { return (ol[i].g + ol[i].h) < (ol[j].g + ol[j].h) }
func (ol openList) Swap(i, j int)      { ol[i], ol[j] = ol[j], ol[i]; ol[i].index = i; ol[j].index = j }
func (ol *openList) Push(x interface{}) {
	n, ok := x.(*pathNode)
	if !ok {
		return
	}
	n.index = len(*ol)
	*ol = append(*ol, n)
}
func (ol *openList) Pop() interface{} {
	old := *ol
	n := old[len(old)-1]
	old[len(old)-1] = nil
	*ol = old[:len(old)-1]
	return n
}

var dirs = [8][2]int{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
}

// FindPath returns a slice of world-coordinate waypoints from (sx,sy) to (gx,gy).
// Returns nil if no path exists.
func (ng *NavGrid) FindPath(sx, sy, gx, gy float64) [][2]float64 { //nolint:gocognit,gocyclo
	pfStart := time.Now()
	expanded := int64(0)
	fail := true
	defer func() {
		ng.recordPathfindingSample(time.Since(pfStart), expanded, fail)
	}()

	scx, scy := WorldToCell(sx, sy)
	gcx, gcy := WorldToCell(gx, gy)

	if ng.IsBlocked(scx, scy) || ng.IsBlocked(gcx, gcy) {
		return nil
	}

	key := func(cx, cy int) int { return cy*ng.cols + cx }
	heuristic := func(ax, ay, bx, by int) float64 {
		dx := math.Abs(float64(ax - bx))
		dy := math.Abs(float64(ay - by))
		return dx + dy + (math.Sqrt2-2)*math.Min(dx, dy)
	}

	start := &pathNode{cx: scx, cy: scy, g: 0, h: heuristic(scx, scy, gcx, gcy)}
	ol := &openList{start}
	heap.Init(ol)

	closed := make(map[int]bool)
	best := make(map[int]*pathNode)
	best[key(scx, scy)] = start

	for ol.Len() > 0 {
		curNode, ok := heap.Pop(ol).(*pathNode)
		if !ok {
			continue
		}
		cur := curNode
		if cur.cx == gcx && cur.cy == gcy {
			fail = false
			return buildPath(cur)
		}
		k := key(cur.cx, cur.cy)
		if closed[k] {
			continue
		}
		closed[k] = true
		expanded++

		for _, d := range dirs {
			nx, ny := cur.cx+d[0], cur.cy+d[1]
			if ng.IsBlocked(nx, ny) {
				continue
			}
			// Prevent diagonal corner-cutting through blocked cells.
			if d[0] != 0 && d[1] != 0 {
				if ng.IsBlocked(cur.cx+d[0], cur.cy) || ng.IsBlocked(cur.cx, cur.cy+d[1]) {
					continue
				}
			}
			nk := key(nx, ny)
			if closed[nk] {
				continue
			}
			cost := 1.0
			if d[0] != 0 && d[1] != 0 {
				cost = math.Sqrt2
			}
			ng := cur.g + cost
			if prev, ok := best[nk]; ok && ng >= prev.g {
				continue
			}
			node := &pathNode{cx: nx, cy: ny, g: ng, h: heuristic(nx, ny, gcx, gcy), parent: cur}
			best[nk] = node
			heap.Push(ol, node)
		}
	}
	return nil
}

func (ng *NavGrid) recordPathfindingSample(runtime time.Duration, expanded int64, fail bool) {
	ng.pfCalls++
	ng.pfExpandedNodes += expanded
	ng.pfRuntimeNS += runtime.Nanoseconds()
	ng.pfRuntimeSamples = append(ng.pfRuntimeSamples, runtime.Nanoseconds())
	if fail {
		ng.pfFails++
	}
}

// ResetPathfindingStats clears accumulated pathfinding counters.
func (ng *NavGrid) ResetPathfindingStats() {
	ng.pfCalls = 0
	ng.pfFails = 0
	ng.pfExpandedNodes = 0
	ng.pfRuntimeNS = 0
	ng.pfRuntimeSamples = ng.pfRuntimeSamples[:0]
}

// PathfindingStats returns a snapshot of current pathfinding counters.
func (ng *NavGrid) PathfindingStats() PathfindingStatsSnapshot {
	stats := PathfindingStatsSnapshot{
		CallCount:     ng.pfCalls,
		FailCount:     ng.pfFails,
		ExpandedNodes: ng.pfExpandedNodes,
	}
	if ng.pfCalls <= 0 {
		return stats
	}

	stats.AverageRuntimeMS = float64(ng.pfRuntimeNS) / float64(ng.pfCalls) / 1_000_000.0
	stats.FailureRatePercent = float64(ng.pfFails) / float64(ng.pfCalls) * 100

	samples := make([]int64, len(ng.pfRuntimeSamples))
	copy(samples, ng.pfRuntimeSamples)
	sort.Slice(samples, func(i, j int) bool {
		return samples[i] < samples[j]
	})
	p95Idx := int(math.Ceil(float64(len(samples))*0.95)) - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	if p95Idx >= len(samples) {
		p95Idx = len(samples) - 1
	}
	stats.P95RuntimeMS = float64(samples[p95Idx]) / 1_000_000.0

	return stats
}

func buildPath(end *pathNode) [][2]float64 {
	var cells [][2]int
	for n := end; n != nil; n = n.parent {
		cells = append(cells, [2]int{n.cx, n.cy})
	}
	// Reverse
	for i, j := 0, len(cells)-1; i < j; i, j = i+1, j-1 {
		cells[i], cells[j] = cells[j], cells[i]
	}
	path := make([][2]float64, len(cells))
	for i, c := range cells {
		wx, wy := CellToWorld(c[0], c[1])
		path[i] = [2]float64{wx, wy}
	}
	return path
}
