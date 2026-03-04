package game

import (
	"container/heap"
	"math"
)

// Vec2 represents a 2D vector for flow field directions.
type Vec2 struct {
	X, Y float64
}

// Add returns the component-wise sum of two vectors.
func (v Vec2) Add(other Vec2) Vec2 {
	return Vec2{v.X + other.X, v.Y + other.Y}
}

// Sub returns the component-wise difference of two vectors.
func (v Vec2) Sub(other Vec2) Vec2 {
	return Vec2{v.X - other.X, v.Y - other.Y}
}

// Scale returns the vector multiplied by a scalar.
func (v Vec2) Scale(s float64) Vec2 {
	return Vec2{v.X * s, v.Y * s}
}

// Length returns the Euclidean length of the vector.
func (v Vec2) Length() float64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}

// Normalize returns a unit-length vector, or zero when near zero length.
func (v Vec2) Normalize() Vec2 {
	l := v.Length()
	if l < 0.0001 {
		return Vec2{0, 0}
	}
	return Vec2{v.X / l, v.Y / l}
}

// Distance returns the Euclidean distance between two vectors.
func (v Vec2) Distance(other Vec2) float64 {
	return v.Sub(other).Length()
}

// Vec2i represents integer grid coordinates.
type Vec2i struct {
	X, Y int
}

// CostField represents the cost of traversing each cell in the grid.
// This is the foundation for both strategic and tactical layers.
type CostField struct {
	baseCost   []float64
	coverBonus []float64
	threatCost []float64
	occupancy  []int
	dirty      []bool

	width  int
	height int
}

// NewCostField creates a cost field with initialized backing slices.
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

// InitializeFromNavGrid seeds base traversal cost from the nav grid.
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

// GetCost returns the total traversal cost at grid cell x,y.
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

// SetOccupancy updates dynamic occupancy pressure at grid cell x,y.
func (cf *CostField) SetOccupancy(x, y, count int) {
	if x < 0 || x >= cf.width || y < 0 || y >= cf.height {
		return
	}
	idx := y*cf.width + x
	cf.occupancy[idx] = count
	cf.dirty[idx] = true
}

// UpdateThreats rebuilds threat heat from visible enemies.
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

// UpdateCover updates cover bonuses from tactical map traits.
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
			switch {
			case trait&CellTraitCorner != 0:
				bonus = 0.8 // Best cover.
			case trait&CellTraitWallAdj != 0:
				bonus = 0.5 // Good cover.
			case trait&CellTraitWindowAdj != 0:
				bonus = 0.6 // Good firing position.
			}

			cf.coverBonus[idx] = bonus
		}
	}
}

// IntegrationField stores the cumulative cost to reach goals.
// Used by both strategic and tactical layers.
type IntegrationField struct {
	cost  []float64
	goals []Vec2i

	width  int
	height int
}

// NewIntegrationField creates an integration field for the supplied goals.
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

// priorityQueueItem is a queue entry for Dijkstra's algorithm.
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
	item, ok := x.(*priorityQueueItem)
	if !ok {
		return
	}
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

// Compute performs Dijkstra flood-fill from goals.
func (ifield *IntegrationField) Compute(costField *CostField) {
	pq := make(priorityQueue, 0)
	heap.Init(&pq)
	directions := integrationDirections()

	seedIntegrationGoals(ifield, &pq)

	for pq.Len() > 0 {
		currentAny := heap.Pop(&pq)
		current, ok := currentAny.(*priorityQueueItem)
		if !ok {
			continue
		}
		if skipIntegrationQueueEntry(ifield, current) {
			continue
		}
		relaxIntegrationNeighbors(ifield, costField, &pq, current, directions)
	}
}

func seedIntegrationGoals(ifield *IntegrationField, pq *priorityQueue) {
	for _, goal := range ifield.goals {
		if goal.X < 0 || goal.X >= ifield.width || goal.Y < 0 || goal.Y >= ifield.height {
			continue
		}
		idx := goal.Y*ifield.width + goal.X
		ifield.cost[idx] = 0
		heap.Push(pq, &priorityQueueItem{x: goal.X, y: goal.Y, cost: 0})
	}
}

func integrationDirections() []Vec2i {
	return []Vec2i{
		{0, -1}, {1, -1}, {1, 0}, {1, 1},
		{0, 1}, {-1, 1}, {-1, 0}, {-1, -1},
	}
}

func skipIntegrationQueueEntry(ifield *IntegrationField, current *priorityQueueItem) bool {
	currentIdx := current.y*ifield.width + current.x
	return current.cost > ifield.cost[currentIdx]
}

func relaxIntegrationNeighbors(
	ifield *IntegrationField,
	costField *CostField,
	pq *priorityQueue,
	current *priorityQueueItem,
	directions []Vec2i,
) {
	for _, dir := range directions {
		nx, ny := current.x+dir.X, current.y+dir.Y
		if nx < 0 || nx >= ifield.width || ny < 0 || ny >= ifield.height {
			continue
		}

		newCost, ok := integrationNeighborCost(current.cost, costField, nx, ny, dir)
		if !ok {
			continue
		}

		neighborIdx := ny*ifield.width + nx
		if newCost >= ifield.cost[neighborIdx] {
			continue
		}
		ifield.cost[neighborIdx] = newCost
		heap.Push(pq, &priorityQueueItem{x: nx, y: ny, cost: newCost})
	}
}

func integrationNeighborCost(baseCost float64, costField *CostField, nx, ny int, dir Vec2i) (float64, bool) {
	traversalCost := costField.GetCost(nx, ny)
	if math.IsInf(traversalCost, 1) {
		return 0, false
	}
	if dir.X != 0 && dir.Y != 0 {
		traversalCost *= 1.414
	}
	return baseCost + traversalCost, true
}

// GetCost returns integration cost at x,y.
func (ifield *IntegrationField) GetCost(x, y int) float64 {
	if x < 0 || x >= ifield.width || y < 0 || y >= ifield.height {
		return math.Inf(1)
	}
	return ifield.cost[y*ifield.width+x]
}

// FlowField stores direction vectors for movement.
type FlowField struct {
	vectors []Vec2

	width  int
	height int
}

// NewFlowField creates an empty flow vector field.
func NewFlowField(width, height int) *FlowField {
	size := width * height
	return &FlowField{
		width:   width,
		height:  height,
		vectors: make([]Vec2, size),
	}
}

// Generate computes flow vectors from integration field.
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

// GetFlow returns the direction vector at x,y.
func (ff *FlowField) GetFlow(x, y int) Vec2 {
	if x < 0 || x >= ff.width || y < 0 || y >= ff.height {
		return Vec2{0, 0}
	}
	return ff.vectors[y*ff.width+x]
}

// SampleFlow samples flow at world coordinates.
func (ff *FlowField) SampleFlow(worldX, worldY float64) Vec2 {
	cellX := int(worldX / cellSize)
	cellY := int(worldY / cellSize)
	return ff.GetFlow(cellX, cellY)
}
