package game

import "math"

// Sector represents a directional arc around a building for assignment to soldiers.
type Sector int

const (
	SectorNone Sector = iota
	SectorN          // North (0°)
	SectorNE         // Northeast (45°)
	SectorE          // East (90°)
	SectorSE         // Southeast (135°)
	SectorS          // South (180°)
	SectorSW         // Southwest (225°)
	SectorW          // West (270°)
	SectorNW         // Northwest (315°)
)

func (s Sector) String() string {
	switch s {
	case SectorN:
		return "N"
	case SectorNE:
		return "NE"
	case SectorE:
		return "E"
	case SectorSE:
		return "SE"
	case SectorS:
		return "S"
	case SectorSW:
		return "SW"
	case SectorW:
		return "W"
	case SectorNW:
		return "NW"
	default:
		return "none"
	}
}

// SectorBearing returns the center bearing (radians) for this sector.
func (s Sector) Bearing() float64 {
	switch s {
	case SectorN:
		return -math.Pi / 2 // North is up (-90°)
	case SectorNE:
		return -math.Pi / 4
	case SectorE:
		return 0
	case SectorSE:
		return math.Pi / 4
	case SectorS:
		return math.Pi / 2
	case SectorSW:
		return 3 * math.Pi / 4
	case SectorW:
		return math.Pi
	case SectorNW:
		return -3 * math.Pi / 4
	default:
		return 0
	}
}

// BuildingState tracks the squad's use of a claimed building.
type BuildingState struct {
	FootprintIdx    int            // index into buildingFootprints
	ClaimTick       int            // when building was claimed
	AssignedSectors map[int]Sector // soldierID -> assigned sector
	LastRotateTick  int            // last sector rotation to avoid predictability
	OccupantCount   int            // current soldiers inside
	LastContactTick int            // last time contact occurred while in building
}

// AssignSectors distributes soldiers across building sectors based on threat direction.
// Priority sectors (facing enemy) get the best soldiers (highest discipline/marksmanship).
func AssignSectors(
	buildingCenterX, buildingCenterY float64,
	members []*Soldier,
	enemyBearing float64,
	hasEnemy bool,
) map[int]Sector {
	assignments := make(map[int]Sector)
	
	if len(members) == 0 {
		return assignments
	}
	
	// Determine priority sectors based on enemy bearing
	prioritySectors := []Sector{}
	if hasEnemy {
		// Convert enemy bearing to sector
		primarySector := bearingToSector(enemyBearing)
		prioritySectors = append(prioritySectors, primarySector)
		
		// Add adjacent sectors for coverage
		prioritySectors = append(prioritySectors, adjacentSectors(primarySector)...)
	} else {
		// No enemy: distribute evenly around perimeter
		prioritySectors = []Sector{SectorN, SectorE, SectorS, SectorW, SectorNE, SectorSE, SectorSW, SectorNW}
	}
	
	// Sort soldiers by combat effectiveness (discipline + marksmanship)
	type soldierScore struct {
		soldier *Soldier
		score   float64
	}
	
	scored := make([]soldierScore, 0, len(members))
	for _, m := range members {
		if m.state == SoldierStateDead {
			continue
		}
		score := m.profile.Skills.Discipline*0.5 + m.profile.Skills.Marksmanship*0.5
		scored = append(scored, soldierScore{soldier: m, score: score})
	}
	
	// Sort by score descending (best soldiers first)
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	
	// Assign best soldiers to priority sectors
	sectorIdx := 0
	for _, ss := range scored {
		if sectorIdx >= len(prioritySectors) {
			// Wrap around if more soldiers than sectors
			sectorIdx = 0
		}
		assignments[ss.soldier.id] = prioritySectors[sectorIdx]
		sectorIdx++
	}
	
	return assignments
}

// bearingToSector converts a bearing (radians) to the nearest sector.
func bearingToSector(bearing float64) Sector {
	// Normalize bearing to 0-2π
	for bearing < 0 {
		bearing += 2 * math.Pi
	}
	for bearing >= 2*math.Pi {
		bearing -= 2 * math.Pi
	}
	
	// Convert to degrees for easier comparison
	degrees := bearing * 180 / math.Pi
	
	// Map to 8 sectors (45° each)
	if degrees < 22.5 || degrees >= 337.5 {
		return SectorE
	} else if degrees < 67.5 {
		return SectorSE
	} else if degrees < 112.5 {
		return SectorS
	} else if degrees < 157.5 {
		return SectorSW
	} else if degrees < 202.5 {
		return SectorW
	} else if degrees < 247.5 {
		return SectorNW
	} else if degrees < 292.5 {
		return SectorN
	} else {
		return SectorNE
	}
}

// adjacentSectors returns the two sectors adjacent to the given sector.
func adjacentSectors(s Sector) []Sector {
	switch s {
	case SectorN:
		return []Sector{SectorNE, SectorNW}
	case SectorNE:
		return []Sector{SectorN, SectorE}
	case SectorE:
		return []Sector{SectorNE, SectorSE}
	case SectorSE:
		return []Sector{SectorE, SectorS}
	case SectorS:
		return []Sector{SectorSE, SectorSW}
	case SectorSW:
		return []Sector{SectorS, SectorW}
	case SectorW:
		return []Sector{SectorSW, SectorNW}
	case SectorNW:
		return []Sector{SectorW, SectorN}
	default:
		return []Sector{}
	}
}

// GetSectorPosition returns a target position within a building for a given sector.
// This helps soldiers spread out to cover different directions.
func GetSectorPosition(
	buildingFootprint rect,
	sector Sector,
	tacticalMap *TacticalMap,
	enemyBearing float64,
	hasEnemy bool,
) (float64, float64, bool) {
	if tacticalMap == nil {
		return 0, 0, false
	}
	
	// Calculate offset from building center based on sector
	cx := float64(buildingFootprint.x + buildingFootprint.w/2)
	cy := float64(buildingFootprint.y + buildingFootprint.h/2)
	
	// Offset toward the sector direction (about 40% of building size)
	offsetDist := math.Min(float64(buildingFootprint.w), float64(buildingFootprint.h)) * 0.4
	bearing := sector.Bearing()
	
	targetX := cx + math.Cos(bearing)*offsetDist
	targetY := cy + math.Sin(bearing)*offsetDist
	
	// Clamp to building bounds
	minX := float64(buildingFootprint.x) + float64(cellSize)
	maxX := float64(buildingFootprint.x+buildingFootprint.w) - float64(cellSize)
	minY := float64(buildingFootprint.y) + float64(cellSize)
	maxY := float64(buildingFootprint.y+buildingFootprint.h) - float64(cellSize)
	
	targetX = math.Max(minX, math.Min(maxX, targetX))
	targetY = math.Max(minY, math.Min(maxY, targetY))
	
	// Use tactical map to find best nearby position (window-adjacent, corner, etc.)
	bestX, bestY, score := tacticalMap.ScanBestNearby(
		targetX, targetY, 4, enemyBearing, hasEnemy, -1, nil, nil,
	)
	
	if score > -0.5 {
		return bestX, bestY, true
	}
	
	return targetX, targetY, true
}

// RotateSectors shifts all sector assignments by one position to avoid predictability.
// Should be called periodically (every 2-3 minutes) when soldiers are static.
func RotateSectors(currentAssignments map[int]Sector) map[int]Sector {
	rotated := make(map[int]Sector)
	
	for soldierID, sector := range currentAssignments {
		rotated[soldierID] = rotateSectorClockwise(sector)
	}
	
	return rotated
}

// rotateSectorClockwise moves a sector one position clockwise.
func rotateSectorClockwise(s Sector) Sector {
	switch s {
	case SectorN:
		return SectorNE
	case SectorNE:
		return SectorE
	case SectorE:
		return SectorSE
	case SectorSE:
		return SectorS
	case SectorS:
		return SectorSW
	case SectorSW:
		return SectorW
	case SectorW:
		return SectorNW
	case SectorNW:
		return SectorN
	default:
		return SectorNone
	}
}
