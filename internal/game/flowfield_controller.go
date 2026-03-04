package game

import "math"

// FlowFieldLayer represents the hierarchical level of a flow field.
type FlowFieldLayer int

const (
	// FlowFieldLayerStrategic is the squad-level objective layer.
	FlowFieldLayerStrategic FlowFieldLayer = iota // Squad-level objectives
	// FlowFieldLayerTactical is the individual positioning layer.
	FlowFieldLayerTactical // Individual positioning
)

// SquadFlowController manages hierarchical flow fields for a squad.
// Strategic layer: squad-level movement toward objectives.
// Tactical layer: individual soldier positioning and spacing.
type SquadFlowController struct {
	squad                *Squad
	costField            *CostField
	strategicIntegration *IntegrationField
	strategicFlow        *FlowField
	tacticalIntegration  *IntegrationField
	tacticalFlow         *FlowField
	navGrid              *NavGrid
	tacticalMap          *TacticalMap
	strategicGoals       []Vec2i
	tacticalGoals        []Vec2i

	width             int
	height            int
	updateTicks       int
	recomputeInterval int

	strategicDirty bool
	tacticalDirty  bool
}

// NewSquadFlowController creates a squad flow controller.
func NewSquadFlowController(squad *Squad, navGrid *NavGrid, tacticalMap *TacticalMap) *SquadFlowController {
	width := navGrid.cols
	height := navGrid.rows

	sfc := &SquadFlowController{
		squad:             squad,
		width:             width,
		height:            height,
		navGrid:           navGrid,
		tacticalMap:       tacticalMap,
		recomputeInterval: 60, // Recompute every 60 ticks (1 second)
	}

	// Initialize cost field
	sfc.costField = NewCostField(width, height)
	sfc.costField.InitializeFromNavGrid(navGrid)
	sfc.costField.UpdateCover(tacticalMap)

	// Initialize flow fields
	sfc.strategicFlow = NewFlowField(width, height)
	sfc.tacticalFlow = NewFlowField(width, height)

	return sfc
}

// Update is called every tick to maintain flow fields.
func (sfc *SquadFlowController) Update(enemies []*Soldier) {
	sfc.updateTicks++

	// Update cost field with dynamic elements
	if sfc.updateTicks%10 == 0 { // Update threats every 10 ticks
		sfc.costField.UpdateThreats(enemies, 15)
		sfc.strategicDirty = true
		sfc.tacticalDirty = true
	}

	// Update occupancy
	sfc.updateOccupancy()

	// Recompute fields if dirty or interval elapsed
	if sfc.strategicDirty || sfc.updateTicks >= sfc.recomputeInterval {
		sfc.RecomputeStrategic()
		sfc.updateTicks = 0
		sfc.strategicDirty = false
	}

	if sfc.tacticalDirty {
		sfc.RecomputeTactical()
		sfc.tacticalDirty = false
	}
}

// SetStrategicGoal sets the squad-level objective.
func (sfc *SquadFlowController) SetStrategicGoal(worldX, worldY float64) {
	cellX := int(worldX / cellSize)
	cellY := int(worldY / cellSize)

	// Check if goal changed significantly
	if len(sfc.strategicGoals) == 0 ||
		abs(sfc.strategicGoals[0].X-cellX) > 2 ||
		abs(sfc.strategicGoals[0].Y-cellY) > 2 {
		sfc.strategicGoals = []Vec2i{{cellX, cellY}}
		sfc.strategicDirty = true
	}
}

// SetStrategicGoals sets multiple strategic goals (for complex objectives).
func (sfc *SquadFlowController) SetStrategicGoals(goals []Vec2i) {
	sfc.strategicGoals = goals
	sfc.strategicDirty = true
}

// SetFormationGoals sets tactical goals for formation positioning.
func (sfc *SquadFlowController) SetFormationGoals(positions []Vec2) {
	goals := make([]Vec2i, 0, len(positions))

	for _, pos := range positions {
		cellX := int(pos.X / cellSize)
		cellY := int(pos.Y / cellSize)
		goals = append(goals, Vec2i{cellX, cellY})
	}

	sfc.tacticalGoals = goals
	sfc.tacticalDirty = true
}

// SetTacticalGoal sets a single tactical goal (for individual positioning).
func (sfc *SquadFlowController) SetTacticalGoal(worldX, worldY float64) {
	cellX := int(worldX / cellSize)
	cellY := int(worldY / cellSize)
	sfc.tacticalGoals = []Vec2i{{cellX, cellY}}
	sfc.tacticalDirty = true
}

// RecomputeStrategic regenerates the strategic layer flow field.
func (sfc *SquadFlowController) RecomputeStrategic() {
	if len(sfc.strategicGoals) == 0 {
		return
	}

	sfc.strategicIntegration = NewIntegrationField(sfc.width, sfc.height, sfc.strategicGoals)
	sfc.strategicIntegration.Compute(sfc.costField)
	sfc.strategicFlow.Generate(sfc.strategicIntegration)
}

// RecomputeTactical regenerates the tactical layer flow field.
func (sfc *SquadFlowController) RecomputeTactical() {
	if len(sfc.tacticalGoals) == 0 {
		return
	}

	sfc.tacticalIntegration = NewIntegrationField(sfc.width, sfc.height, sfc.tacticalGoals)
	sfc.tacticalIntegration.Compute(sfc.costField)
	sfc.tacticalFlow.Generate(sfc.tacticalIntegration)
}

// GetStrategicFlow returns the strategic layer flow vector at world position.
func (sfc *SquadFlowController) GetStrategicFlow(worldX, worldY float64) Vec2 {
	if sfc.strategicFlow == nil {
		return Vec2{0, 0}
	}
	return sfc.strategicFlow.SampleFlow(worldX, worldY)
}

// GetTacticalFlow returns the tactical layer flow vector at world position.
func (sfc *SquadFlowController) GetTacticalFlow(worldX, worldY float64) Vec2 {
	if sfc.tacticalFlow == nil {
		return Vec2{0, 0}
	}
	return sfc.tacticalFlow.SampleFlow(worldX, worldY)
}

// GetBlendedFlow returns a weighted blend of strategic and tactical flows.
func (sfc *SquadFlowController) GetBlendedFlow(worldX, worldY, tacticalWeight float64) Vec2 {
	strategic := sfc.GetStrategicFlow(worldX, worldY)
	tactical := sfc.GetTacticalFlow(worldX, worldY)

	// Clamp tactical weight
	if tacticalWeight < 0 {
		tacticalWeight = 0
	}
	if tacticalWeight > 1 {
		tacticalWeight = 1
	}

	strategicWeight := 1.0 - tacticalWeight

	blended := Vec2{
		X: strategic.X*strategicWeight + tactical.X*tacticalWeight,
		Y: strategic.Y*strategicWeight + tactical.Y*tacticalWeight,
	}

	return blended.Normalize()
}

// GetDistanceToStrategicGoal returns the integration field cost to strategic goal.
func (sfc *SquadFlowController) GetDistanceToStrategicGoal(worldX, worldY float64) float64 {
	if sfc.strategicIntegration == nil {
		return math.Inf(1)
	}
	cellX := int(worldX / cellSize)
	cellY := int(worldY / cellSize)
	return sfc.strategicIntegration.GetCost(cellX, cellY)
}

// GetDistanceToTacticalGoal returns the integration field cost to tactical goal.
func (sfc *SquadFlowController) GetDistanceToTacticalGoal(worldX, worldY float64) float64 {
	if sfc.tacticalIntegration == nil {
		return math.Inf(1)
	}
	cellX := int(worldX / cellSize)
	cellY := int(worldY / cellSize)
	return sfc.tacticalIntegration.GetCost(cellX, cellY)
}

// updateOccupancy updates the cost field with current soldier positions.
func (sfc *SquadFlowController) updateOccupancy() {
	// Clear previous occupancy
	for i := range sfc.costField.occupancy {
		sfc.costField.occupancy[i] = 0
	}

	// Mark cells occupied by squad members
	for _, soldier := range sfc.squad.Members {
		if soldier.state == SoldierStateDead {
			continue
		}
		cellX := int(soldier.x / cellSize)
		cellY := int(soldier.y / cellSize)
		if cellX >= 0 && cellX < sfc.width && cellY >= 0 && cellY < sfc.height {
			idx := cellY*sfc.width + cellX
			sfc.costField.occupancy[idx]++
		}
	}
}
