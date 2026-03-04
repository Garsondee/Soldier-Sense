package game

import "math"

// BuildingEntryState tracks coordinated building entry progress.
type BuildingEntryState int

const (
	// EntryStateNone indicates no active entry process.
	EntryStateNone BuildingEntryState = iota
	// EntryStateApproaching indicates the squad is moving toward the building.
	EntryStateApproaching // squad moving toward building
	// EntryStateStacking indicates the entry team is staged at the door.
	EntryStateStacking // entry team at door, overwatch covering
	// EntryStateBreaching indicates the entry team is crossing the threshold.
	EntryStateBreaching // entry team entering
	// EntryStateClearing indicates the interior is being cleared.
	EntryStateClearing // inside, clearing rooms
	// EntryStateSecured indicates the building is cleared and occupied.
	EntryStateSecured // building cleared and occupied
)

// BuildingEntryPlan coordinates squad entry into a building.
type BuildingEntryPlan struct {
	EntryTeam         []*Soldier // soldiers designated to enter
	OverwatchTeam     []*Soldier // soldiers providing cover
	EntryPointX       float64    // door/breach point
	EntryPointY       float64
	TargetBuildingIdx int
	InitiatedTick     int
	StateChangeTick   int
	State             BuildingEntryState
}

// CreateEntryPlan designates entry and overwatch teams for building assault.
// Entry team: 2-3 soldiers with highest discipline.
// Overwatch team: remainder, provide suppressive fire.
func CreateEntryPlan(
	buildingIdx int,
	footprints []rect,
	members []*Soldier,
	enemyBearing float64,
	tick int,
) *BuildingEntryPlan {
	if buildingIdx < 0 || buildingIdx >= len(footprints) {
		return nil
	}

	// Filter alive members
	alive := []*Soldier{}
	for _, m := range members {
		if m.state != SoldierStateDead {
			alive = append(alive, m)
		}
	}

	if len(alive) < 2 {
		return nil // need at least 2 soldiers
	}

	// Sort by discipline (best soldiers enter first)
	type soldierScore struct {
		soldier *Soldier
		score   float64
	}

	scored := make([]soldierScore, len(alive))
	for i, m := range alive {
		// Entry team needs discipline + courage (low fear)
		score := m.profile.Skills.Discipline*0.6 + (1.0-m.profile.Psych.EffectiveFear())*0.4
		scored[i] = soldierScore{soldier: m, score: score}
	}

	// Sort descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Entry team: top 2-3 soldiers (or half the squad, whichever is smaller)
	entrySize := 2
	if len(alive) >= 6 {
		entrySize = 3
	}
	if entrySize > len(alive)/2 {
		entrySize = len(alive) / 2
	}
	if entrySize < 1 {
		entrySize = 1
	}

	entryTeam := make([]*Soldier, entrySize)
	for i := 0; i < entrySize; i++ {
		entryTeam[i] = scored[i].soldier
	}

	// Overwatch team: everyone else
	overwatchTeam := make([]*Soldier, len(alive)-entrySize)
	for i := entrySize; i < len(alive); i++ {
		overwatchTeam[i-entrySize] = scored[i].soldier
	}

	// Find entry point (door closest to squad)
	fp := footprints[buildingIdx]
	entryX, entryY := findBestEntryPoint(fp, alive, enemyBearing)

	return &BuildingEntryPlan{
		TargetBuildingIdx: buildingIdx,
		State:             EntryStateApproaching,
		EntryTeam:         entryTeam,
		OverwatchTeam:     overwatchTeam,
		EntryPointX:       entryX,
		EntryPointY:       entryY,
		InitiatedTick:     tick,
		StateChangeTick:   tick,
	}
}

func (plan *BuildingEntryPlan) entryTeamAllNearPoint() bool {
	for _, s := range plan.EntryTeam {
		if s.state == SoldierStateDead {
			continue
		}
		dist := math.Hypot(s.x-plan.EntryPointX, s.y-plan.EntryPointY)
		if dist > float64(cellSize)*3 {
			return false
		}
	}
	return true
}

func (plan *BuildingEntryPlan) entryTeamAnyInside() bool {
	for _, s := range plan.EntryTeam {
		if s.state == SoldierStateDead {
			continue
		}
		if s.blackboard.AtInterior {
			return true
		}
	}
	return false
}

func (plan *BuildingEntryPlan) entryTeamClearStatus() (allInside, anyThreats bool) {
	allInside = true
	for _, s := range plan.EntryTeam {
		if s.state == SoldierStateDead {
			continue
		}
		if !s.blackboard.AtInterior {
			allInside = false
		}
		if s.blackboard.VisibleThreatCount() > 0 {
			anyThreats = true
		}
	}
	return allInside, anyThreats
}

// findBestEntryPoint identifies the best door/breach point for entry.
// Prefers doors on the side away from enemy (safer approach).
func findBestEntryPoint(
	building rect,
	_ []*Soldier,
	enemyBearing float64,
) (float64, float64) {
	// Building center
	bx := float64(building.x + building.w/2)
	by := float64(building.y + building.h/2)

	// Approach from opposite side of enemy bearing
	// If enemy is east (0), approach from west (π)
	approachBearing := enemyBearing + math.Pi

	// Entry point offset from building center
	offsetDist := float64(building.w+building.h) / 4.0
	entryX := bx + math.Cos(approachBearing)*offsetDist
	entryY := by + math.Sin(approachBearing)*offsetDist

	// Clamp to building perimeter
	margin := float64(cellSize)
	entryX = math.Max(float64(building.x)+margin, math.Min(float64(building.x+building.w)-margin, entryX))
	entryY = math.Max(float64(building.y)+margin, math.Min(float64(building.y+building.h)-margin, entryY))

	return entryX, entryY
}

// UpdateEntryState advances the entry plan based on team positions and readiness.
func (plan *BuildingEntryPlan) UpdateEntryState(tick int, footprints []rect) {
	if plan == nil || plan.TargetBuildingIdx >= len(footprints) {
		return
	}

	switch plan.State {
	case EntryStateApproaching:
		if plan.entryTeamAllNearPoint() {
			plan.State = EntryStateStacking
			plan.StateChangeTick = tick
		}

	case EntryStateStacking:
		// Stack for 1-2 seconds before breach
		elapsed := tick - plan.StateChangeTick
		if elapsed > 90 { // ~1.5 seconds at 60 TPS
			plan.State = EntryStateBreaching
			plan.StateChangeTick = tick
		}

	case EntryStateBreaching:
		if plan.entryTeamAnyInside() {
			plan.State = EntryStateClearing
			plan.StateChangeTick = tick
		}

	case EntryStateClearing:
		allInside, anyThreats := plan.entryTeamClearStatus()

		// Secured if all inside and no threats for 3+ seconds
		elapsed := tick - plan.StateChangeTick
		if allInside && !anyThreats && elapsed > 180 {
			plan.State = EntryStateSecured
			plan.StateChangeTick = tick
		}
	}
}

// GetOptimalDefensivePosition finds the best position within a building for defense.
// Considers: window coverage, corner positions, sector assignment, enemy bearing.
func GetOptimalDefensivePosition(
	buildingFootprint rect,
	assignedSector Sector,
	enemyBearing float64,
	tacticalMap *TacticalMap,
	occupiedPositions [][2]float64,
) (float64, float64, bool) {
	if tacticalMap == nil {
		return 0, 0, false
	}

	// Get base position for sector
	sectorX, sectorY, ok := GetSectorPosition(buildingFootprint, assignedSector, tacticalMap, enemyBearing, true)
	if !ok {
		return 0, 0, false
	}

	// Scan for best tactical position near sector target
	// Prioritize: window-adjacent > corner > wall-adjacent > interior
	bestX, bestY, score := tacticalMap.ScanBestNearby(
		sectorX, sectorY, 6, enemyBearing, true, -1, nil, occupiedPositions,
	)

	// Verify position is inside building
	if bestX < float64(buildingFootprint.x) || bestX > float64(buildingFootprint.x+buildingFootprint.w) ||
		bestY < float64(buildingFootprint.y) || bestY > float64(buildingFootprint.y+buildingFootprint.h) {
		// Position outside building, use sector position
		return sectorX, sectorY, true
	}

	if score > -0.3 {
		return bestX, bestY, true
	}

	return sectorX, sectorY, true
}

// ShouldInitiateEntry determines if squad should begin coordinated building entry.
func ShouldInitiateEntry(
	buildingIdx int,
	footprints []rect,
	buildingIntel *BuildingIntelMap,
	squadX, squadY float64,
	hasContact bool,
	squadPhase SquadPhase,
) bool {
	if buildingIdx < 0 || buildingIdx >= len(footprints) {
		return false
	}

	// Only initiate entry during assault or bound phases
	if squadPhase != SquadPhaseAssault && squadPhase != SquadPhaseBound {
		return false
	}

	// Check if building is known to be occupied by enemy
	intel := buildingIntel.GetIntel(buildingIdx)
	if intel != nil && intel.Cleared {
		return false // already cleared, just occupy normally
	}

	// Distance check: close enough to initiate
	fp := footprints[buildingIdx]
	bx := float64(fp.x + fp.w/2)
	by := float64(fp.y + fp.h/2)
	dist := math.Hypot(bx-squadX, by-squadY)

	return dist < 150 && hasContact
}
