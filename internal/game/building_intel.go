package game

import "math"

// BuildingIntel tracks squad leader's mental map of enemy building occupation.
type BuildingIntel struct {
	FootprintIdx     int     // index into buildingFootprints
	EnemyPresence    float64 // 0-1 confidence that enemies occupy this building
	LastObservedTick int     // when evidence was last updated
	ThreatLevel      float64 // 0-1 how dangerous this building is
	Cleared          bool    // true if squad has cleared this building
	ClearedTick      int     // when building was cleared
}

// BuildingIntelMap is the squad leader's mental model of building occupation.
type BuildingIntelMap struct {
	buildings map[int]*BuildingIntel // footprintIdx -> intel
}

// NewBuildingIntelMap creates an empty intel map.
func NewBuildingIntelMap() *BuildingIntelMap {
	return &BuildingIntelMap{
		buildings: make(map[int]*BuildingIntel),
	}
}

// UpdateFromGunfire records evidence of enemy activity from a building.
// Called when shots are heard/seen from a building location.
func (bim *BuildingIntelMap) UpdateFromGunfire(
	footprintIdx int,
	shooterX, shooterY float64,
	footprints []rect,
	tick int,
	confidence float64,
) {
	if footprintIdx < 0 || footprintIdx >= len(footprints) {
		return
	}

	intel, exists := bim.buildings[footprintIdx]
	if !exists {
		intel = &BuildingIntel{
			FootprintIdx: footprintIdx,
		}
		bim.buildings[footprintIdx] = intel
	}

	// Increase presence confidence (shots from building = strong evidence)
	intel.EnemyPresence = math.Min(1.0, intel.EnemyPresence+confidence*0.4)
	intel.LastObservedTick = tick
	intel.Cleared = false // no longer cleared if enemies are shooting from it

	// Threat level based on presence and recency
	intel.ThreatLevel = intel.EnemyPresence * 0.8
}

// UpdateFromVisualContact records seeing an enemy in/near a building.
func (bim *BuildingIntelMap) UpdateFromVisualContact(
	footprintIdx int,
	enemyX, enemyY float64,
	footprints []rect,
	tick int,
) {
	if footprintIdx < 0 || footprintIdx >= len(footprints) {
		return
	}

	intel, exists := bim.buildings[footprintIdx]
	if !exists {
		intel = &BuildingIntel{
			FootprintIdx: footprintIdx,
		}
		bim.buildings[footprintIdx] = intel
	}

	// Visual contact is strong evidence
	intel.EnemyPresence = math.Min(1.0, intel.EnemyPresence+0.5)
	intel.LastObservedTick = tick
	intel.Cleared = false
	intel.ThreatLevel = intel.EnemyPresence * 0.9
}

// UpdateFromProximity records being near a building without contact.
// Reduces confidence if no enemies are seen (building might be empty).
func (bim *BuildingIntelMap) UpdateFromProximity(
	footprintIdx int,
	tick int,
) {
	intel, exists := bim.buildings[footprintIdx]
	if !exists {
		return
	}

	// No contact while close = reduces presence confidence
	intel.EnemyPresence = math.Max(0, intel.EnemyPresence-0.1)
	intel.LastObservedTick = tick

	if intel.EnemyPresence < 0.1 {
		intel.ThreatLevel = 0
	}
}

// MarkCleared marks a building as cleared by friendly forces.
func (bim *BuildingIntelMap) MarkCleared(footprintIdx int, tick int) {
	intel, exists := bim.buildings[footprintIdx]
	if !exists {
		intel = &BuildingIntel{
			FootprintIdx: footprintIdx,
		}
		bim.buildings[footprintIdx] = intel
	}

	intel.Cleared = true
	intel.ClearedTick = tick
	intel.EnemyPresence = 0
	intel.ThreatLevel = 0
}

// DecayIntel reduces confidence over time for stale information.
// Should be called periodically (e.g., every 60 ticks).
func (bim *BuildingIntelMap) DecayIntel(tick int) {
	decayRate := 0.02 // per decay cycle

	for _, intel := range bim.buildings {
		if intel.Cleared {
			continue // cleared buildings don't decay
		}

		age := tick - intel.LastObservedTick
		if age > 300 { // 5 seconds stale
			intel.EnemyPresence = math.Max(0, intel.EnemyPresence-decayRate)
			intel.ThreatLevel = math.Max(0, intel.ThreatLevel-decayRate)
		}
	}
}

// GetIntel returns intel for a building, or nil if none exists.
func (bim *BuildingIntelMap) GetIntel(footprintIdx int) *BuildingIntel {
	return bim.buildings[footprintIdx]
}

// GetThreatBuildings returns buildings with significant threat level.
func (bim *BuildingIntelMap) GetThreatBuildings(minThreat float64) []*BuildingIntel {
	threats := []*BuildingIntel{}

	for _, intel := range bim.buildings {
		if intel.ThreatLevel >= minThreat && !intel.Cleared {
			threats = append(threats, intel)
		}
	}

	return threats
}

// FindBuildingForPosition returns the building footprint index containing a position.
// Returns -1 if position is not in any building.
func FindBuildingForPosition(x, y float64, footprints []rect) int {
	for i, fp := range footprints {
		if x >= float64(fp.x) && x < float64(fp.x+fp.w) &&
			y >= float64(fp.y) && y < float64(fp.y+fp.h) {
			return i
		}
	}
	return -1
}

// FindNearestBuilding returns the index of the nearest building to a position.
// Returns -1 if no buildings exist.
func FindNearestBuilding(x, y float64, footprints []rect) int {
	if len(footprints) == 0 {
		return -1
	}

	bestIdx := -1
	bestDist := math.MaxFloat64

	for i, fp := range footprints {
		cx := float64(fp.x + fp.w/2)
		cy := float64(fp.y + fp.h/2)
		dist := math.Hypot(cx-x, cy-y)

		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}

	return bestIdx
}

// ShouldSuppressBuilding returns true if squad should suppress this building before advancing.
func (bim *BuildingIntelMap) ShouldSuppressBuilding(
	footprintIdx int,
	squadX, squadY float64,
	footprints []rect,
) bool {
	intel := bim.GetIntel(footprintIdx)
	if intel == nil || intel.Cleared {
		return false
	}

	// Suppress if high threat and building is between squad and objective
	if intel.ThreatLevel < 0.4 {
		return false
	}

	if footprintIdx >= len(footprints) {
		return false
	}

	fp := footprints[footprintIdx]
	buildingX := float64(fp.x + fp.w/2)
	buildingY := float64(fp.y + fp.h/2)

	// Building is close enough to be a threat
	dist := math.Hypot(buildingX-squadX, buildingY-squadY)
	return dist < 400 && intel.ThreatLevel > 0.5
}
