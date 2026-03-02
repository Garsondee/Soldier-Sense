package game

import (
	"container/heap"
	"math"
)

// Vec2 represents a 2D vector for flow field directions
type Vec2 struct {
	X, Y float64
}

func (v Vec2) Add(other Vec2) Vec2 {
	return Vec2{v.X + other.X, v.Y + other.Y}
}

func (v Vec2) Sub(other Vec2) Vec2 {
	return Vec2{v.X - other.X, v.Y - other.Y}
}

func (v Vec2) Scale(s float64) Vec2 {
	return Vec2{v.X * s, v.Y * s}
}

func (v Vec2) Length() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}

func (v Vec2) Normalize() Vec2 {
	l := v.Length()
	if l < 0.0001 {
		return Vec2{0, 0}
	}
	return Vec2{v.X / l, v.Y / l}
}

func (v Vec2) Distance(other Vec2) float64 {
	return v.Sub(other).Length()
}

// Vec2i represents integer grid coordinates
type Vec2i struct {
	X, Y int
}

// CostField represents the cost of traversing each cell in the grid
// This is the foundation for both strategic and tactical layers
type CostField struct {
	width  int
	height int

	// Base terrain cost (1.0 = normal, >1.0 = difficult, math.Inf = impassable)
	baseCost []float64

	// Cover bonus (negative cost, reduces total)
	coverBonus []float64

	// Threat cost from enemies (positive, increases total)
	threatCost []float64

	// Friendly occupancy (slight penalty to encourage spacing)
	occupancy []int

	// Dirty flags for incremental updates
	dirty []bool
}

func NewCostField(width, height int) *CostField {
	size := width * height
	return &CostField{
		width:      width,
		height:     height,
		baseCost:   make([]float64, size),
		coverBonus: make([]float64, size),
		threatCost: make([]float64, size),
		occupancy:  make([]int, size),
		dirty:      make([]bool, size),
	}
}

func (cf *CostField) InitializeFromNavGrid(navGrid *NavGrid) {
	for y := 0; y < cf.height; y++ {
		for x := 0; x < cf.width; x++ {
			idx := y*cf.width + x
			if navGrid.IsBlocked(x, y) {
				cf.baseCost[idx] = math.Inf(1)
			} else {
				cf.baseCost[idx] = 1.0
			}
		}
	}
}

func (cf *CostField) GetCost(x, y int) float64 {
	if x < 0 || x >= cf.width || y < 0 || y >= cf.height {
		return math.Inf(1)
	}
	idx := y*cf.width + x
	base := cf.baseCost[idx]
	if math.IsInf(base, 1) {
		return base
	}
	cover := cf.coverBonus[idx]
	threat := cf.threatCost[idx]
	occupancyCost := float64(cf.occupancy[idx]) * 0.2
	return base - cover + threat + occupancyCost
}

func (cf *CostField) SetOccupancy(x, y int, count int) {
	if x < 0 || x >= cf.width || y < 0 || y >= cf.height {
		return
	}
	idx := y*cf.width + x
	cf.occupancy[idx] = count
	cf.dirty[idx] = true
}

func (cf *CostField) UpdateThreats(enemies []*Soldier, threatRadius int) {
	// Clear previous threats
	for i := range cf.threatCost {
		cf.threatCost[i] = 0
	}

	// Add threat heat from each enemy
	for _, enemy := range enemies {
		if enemy.state == SoldierStateDead {
			continue
		}
		ex := int(enemy.x / cellSize)
		ey := int(enemy.y / cellSize)

		for dy := -threatRadius; dy <= threatRadius; dy++ {
			for dx := -threatRadius; dx <= threatRadius; dx++ {
				x, y := ex+dx, ey+dy
				if x < 0 || x >= cf.width || y < 0 || y >= cf.height {
					continue
				}

				dist := math.Sqrt(float64(dx*dx + dy*dy))
				if dist > float64(threatRadius) {
					continue
				}

				// Threat falls off with distance
				threat := 2.0 * (1.0 - dist/float64(threatRadius))
				idx := y*cf.width + x
				cf.threatCost[idx] += threat
				cf.dirty[idx] = true
			}
		}
	}
}

func (cf *CostField) UpdateCover(tacticalMap *TacticalMap) {
	if tacticalMap == nil {
		return
	}

	for y := 0; y < cf.height; y++ {
		for x := 0; x < cf.width; x++ {
			idx := y*cf.width + x
			wx, wy := CellToWorld(x, y)
			trait := tacticalMap.TraitAt(wx, wy)

			// Compute cover bonus based on tactical traits
			bonus := 0.0
			if trait&CellTraitCorner != 0 {
				bonus = 0.8 // Best cover
			} else if trait&CellTraitWallAdj != 0 {
				bonus = 0.5 // Good cover
			} else if trait&CellTraitWindowAdj != 0 {
				bonus = 0.6 // Good firing position
			}

			cf.coverBonus[idx] = bonus
		}
	}
}

// IntegrationField stores the cumulative cost to reach goals
// Used by both strategic and tactical layers
type IntegrationField struct {
	width  int
	height int
	cost   []float64
	goals  []Vec2i
}

func NewIntegrationField(width, height int, goals []Vec2i) *IntegrationField {
	size := width * height
	cost := make([]float64, size)
	for i := range cost {
		cost[i] = math.Inf(1)
	}
	return &IntegrationField{
		width:  width,
		height: height,
		cost:   cost,
		goals:  goals,
	}
}

// priorityQueueItem for Dijkstra's algorithm
type priorityQueueItem struct {
	x, y int
	cost float64
	idx  int
}

type priorityQueue []*priorityQueueItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].cost < pq[j].cost }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].idx = i
	pq[j].idx = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*priorityQueueItem)
	item.idx = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.idx = -1
	*pq = old[0 : n-1]
	return item
}

// Compute performs Dijkstra flood-fill from goals
func (ifield *IntegrationField) Compute(costField *CostField) {
	pq := make(priorityQueue, 0)
	heap.Init(&pq)

	// Initialize goals with cost 0
	for _, goal := range ifield.goals {
		if goal.X < 0 || goal.X >= ifield.width || goal.Y < 0 || goal.Y >= ifield.height {
			continue
		}
		idx := goal.Y*ifield.width + goal.X
		ifield.cost[idx] = 0
		heap.Push(&pq, &priorityQueueItem{x: goal.X, y: goal.Y, cost: 0})
	}

	// Dijkstra flood-fill
	directions := []Vec2i{
		{0, -1}, {1, -1}, {1, 0}, {1, 1},
		{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
	}

	for pq.Len() > 0 {
		current := heap.Pop(&pq).(*priorityQueueItem)
		currentIdx := current.y*ifield.width + current.x

		// Skip if we've found a better path already
		if current.cost > ifield.cost[currentIdx] {
			continue
		}

		// Check all neighbors
		for _, dir := range directions {
			nx, ny := current.x+dir.X, current.y+dir.Y
			if nx < 0 || nx >= ifield.width || ny < 0 || ny >= ifield.height {
				continue
			}

			neighborIdx := ny*ifield.width + nx
			traversalCost := costField.GetCost(nx, ny)
			if math.IsInf(traversalCost, 1) {
				continue
			}

			// Diagonal movement costs more
			if dir.X != 0 && dir.Y != 0 {
				traversalCost *= 1.414
			}

			newCost := current.cost + traversalCost
			if newCost < ifield.cost[neighborIdx] {
				ifield.cost[neighborIdx] = newCost
				heap.Push(&pq, &priorityQueueItem{x: nx, y: ny, cost: newCost})
			}
		}
	}
}

func (ifield *IntegrationField) GetCost(x, y int) float64 {
	if x < 0 || x >= ifield.width || y < 0 || y >= ifield.height {
		return math.Inf(1)
	}
	return ifield.cost[y*ifield.width+x]
}

// FlowField stores direction vectors for movement
type FlowField struct {
	width   int
	height  int
	vectors []Vec2
}

func NewFlowField(width, height int) *FlowField {
	size := width * height
	return &FlowField{
		width:   width,
		height:  height,
		vectors: make([]Vec2, size),
	}
}

// Generate computes flow vectors from integration field
func (ff *FlowField) Generate(integration *IntegrationField) {
	directions := []Vec2i{
		{0, -1}, {1, -1}, {1, 0}, {1, 1},
		{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
	}

	for y := 0; y < ff.height; y++ {
		for x := 0; x < ff.width; x++ {
			idx := y*ff.width + x
			currentCost := integration.GetCost(x, y)

			if math.IsInf(currentCost, 1) {
				ff.vectors[idx] = Vec2{0, 0}
				continue
			}

			// Find neighbor with lowest cost
			bestCost := currentCost
			bestDir := Vec2{0, 0}

			for _, dir := range directions {
				nx, ny := x+dir.X, y+dir.Y
				neighborCost := integration.GetCost(nx, ny)

				if neighborCost < bestCost {
					bestCost = neighborCost
					bestDir = Vec2{float64(dir.X), float64(dir.Y)}
				}
			}

			// Normalize direction
			if bestDir.Length() > 0 {
				ff.vectors[idx] = bestDir.Normalize()
			} else {
				ff.vectors[idx] = Vec2{0, 0}
			}
		}
	}
}

func (ff *FlowField) GetFlow(x, y int) Vec2 {
	if x < 0 || x >= ff.width || y < 0 || y >= ff.height {
		return Vec2{0, 0}
	}
	return ff.vectors[y*ff.width+x]
}

// Sample flow at world coordinates (interpolated)
func (ff *FlowField) SampleFlow(worldX, worldY float64) Vec2 {
	cellX := int(worldX / cellSize)
	cellY := int(worldY / cellSize)
	return ff.GetFlow(cellX, cellY)
}
