package game

import (
	"math/rand"
)

// CompoundType represents different types of building complexes.
type CompoundType uint8

const (
	// CompoundTypeResidential represents housing developments and suburban clusters.
	CompoundTypeResidential CompoundType = iota // Housing development, suburban cluster
	// CompoundTypeIndustrial represents factory and warehouse complexes.
	CompoundTypeIndustrial // Factory complex, warehouse district
	// CompoundTypeMilitary represents fortified military compounds.
	CompoundTypeMilitary // Base, fortified compound
	// CompoundTypeAgricultural represents farm complexes with multiple buildings.
	CompoundTypeAgricultural // Farm complex with multiple buildings
	// CompoundTypeCivic represents government or institutional campuses.
	CompoundTypeCivic // Government, institutional campus
)

// Compound represents a complex of multiple buildings with shared perimeter.
type Compound struct {
	Buildings      []rect            // Individual buildings within compound
	BuildingTypes  []BuildingType    // Types of each building
	PerimeterWalls []rect            // Defensive/boundary wall segments
	Gates          []Point           // Entry/exit points in perimeter
	InternalPaths  []rect            // Roads/paths within compound
	ScatterObjects []ExteriorFeature // Context-appropriate objects
	Perimeter      rect              // Overall boundary rectangle
	Type           CompoundType      // Type of compound
}

// generateCompounds creates building complexes with multiple structures.
func generateCompounds(tm *TileMap, existingBuildings []rect, rng *rand.Rand) []Compound {
	compounds := make([]Compound, 0, 8)

	mapW := tm.Cols * cellSize
	mapH := tm.Rows * cellSize

	// Generate 2-6 compounds per map
	compoundCount := 2 + rng.Intn(5)

	for i := 0; i < compoundCount; i++ {
		compound := generateSingleCompound(tm, existingBuildings, mapW, mapH, rng)
		if len(compound.Buildings) > 0 {
			compounds = append(compounds, compound)
			// Add compound buildings to existing list to avoid overlaps
			existingBuildings = append(existingBuildings, compound.Buildings...)
		}
	}

	return compounds
}

// generateSingleCompound creates one compound complex.
func generateSingleCompound(_ *TileMap, existingBuildings []rect, mapW, mapH int, rng *rand.Rand) Compound {
	// Choose compound type
	compoundType := chooseCompoundType(rng)

	// Choose compound location and size
	compoundSize := getCompoundSize(compoundType, rng)
	compoundX := 128 + rng.Intn(mapW-256-compoundSize.w)
	compoundY := 128 + rng.Intn(mapH-256-compoundSize.h)

	perimeter := rect{x: compoundX, y: compoundY, w: compoundSize.w, h: compoundSize.h}

	// Check for overlaps with existing buildings
	if overlapsAnyRect(perimeter, existingBuildings) {
		return Compound{} // Skip if overlapping
	}

	// Generate buildings within compound
	buildings := generateCompoundBuildings(perimeter, compoundType, rng)
	buildingTypes := make([]BuildingType, len(buildings))

	// Assign building types based on compound type
	for i := range buildings {
		buildingTypes[i] = getBuildingTypeForCompound(compoundType, i, len(buildings), rng)
	}

	// Generate perimeter walls if needed
	perimeterWalls := generatePerimeterWalls(perimeter, compoundType, rng)

	// Generate gates in perimeter
	gates := generateCompoundGates(perimeter, compoundType, rng)

	// Generate internal paths connecting buildings
	internalPaths := generateInternalPaths(buildings, perimeter, rng)

	// Generate context-appropriate scatter objects
	scatterObjects := generateCompoundScatterObjects(buildings, compoundType, perimeter, rng)

	return Compound{
		Type:           compoundType,
		Perimeter:      perimeter,
		Buildings:      buildings,
		BuildingTypes:  buildingTypes,
		PerimeterWalls: perimeterWalls,
		Gates:          gates,
		InternalPaths:  internalPaths,
		ScatterObjects: scatterObjects,
	}
}

// chooseCompoundType selects compound type based on probability.
func chooseCompoundType(rng *rand.Rand) CompoundType {
	typeRoll := rng.Float64()
	switch {
	case typeRoll < 0.3:
		return CompoundTypeIndustrial // 30% industrial
	case typeRoll < 0.5:
		return CompoundTypeResidential // 20% residential
	case typeRoll < 0.7:
		return CompoundTypeMilitary // 20% military
	case typeRoll < 0.85:
		return CompoundTypeAgricultural // 15% agricultural
	default:
		return CompoundTypeCivic // 15% civic
	}
}

// getCompoundSize returns appropriate compound dimensions.
func getCompoundSize(compoundType CompoundType, rng *rand.Rand) rect {
	switch compoundType {
	case CompoundTypeIndustrial:
		// Large industrial complexes
		w := 400 + rng.Intn(600) // 400-1000 pixels
		h := 300 + rng.Intn(500) // 300-800 pixels
		return rect{w: w, h: h}

	case CompoundTypeResidential:
		// Medium residential developments
		w := 250 + rng.Intn(400) // 250-650 pixels
		h := 200 + rng.Intn(350) // 200-550 pixels
		return rect{w: w, h: h}

	case CompoundTypeMilitary:
		// Fortified military compounds
		w := 300 + rng.Intn(500) // 300-800 pixels
		h := 300 + rng.Intn(500) // 300-800 pixels (tend to be square)
		return rect{w: w, h: h}

	case CompoundTypeAgricultural:
		// Large farm complexes
		w := 350 + rng.Intn(700) // 350-1050 pixels
		h := 250 + rng.Intn(450) // 250-700 pixels
		return rect{w: w, h: h}

	case CompoundTypeCivic:
		// Medium civic campuses
		w := 200 + rng.Intn(400) // 200-600 pixels
		h := 200 + rng.Intn(400) // 200-600 pixels
		return rect{w: w, h: h}

	default:
		return rect{w: 300, h: 300}
	}
}

// generateCompoundBuildings creates multiple buildings within a compound perimeter.
func generateCompoundBuildings(perimeter rect, compoundType CompoundType, rng *rand.Rand) []rect {
	buildings := make([]rect, 0, 8)

	// Number of buildings based on compound type and size
	buildingCount := getCompoundBuildingCount(perimeter, compoundType, rng)

	// Leave margins for perimeter walls and paths
	margin := 48
	buildableArea := rect{
		x: perimeter.x + margin,
		y: perimeter.y + margin,
		w: perimeter.w - 2*margin,
		h: perimeter.h - 2*margin,
	}

	// Generate buildings with spacing
	attempts := 0
	maxAttempts := buildingCount * 5

	for len(buildings) < buildingCount && attempts < maxAttempts {
		attempts++

		// Generate random building size appropriate for compound type
		buildingW, buildingH := getBuildingSizeForCompound(compoundType, rng)

		// Random position within buildable area
		if buildingW >= buildableArea.w || buildingH >= buildableArea.h {
			continue // Building too large for compound
		}

		buildingX := buildableArea.x + rng.Intn(buildableArea.w-buildingW)
		buildingY := buildableArea.y + rng.Intn(buildableArea.h-buildingH)

		candidate := rect{x: buildingX, y: buildingY, w: buildingW, h: buildingH}

		// Check for overlaps with existing buildings (with minimum spacing)
		minSpacing := 32 // 32 pixel minimum spacing between buildings
		if !overlapsAnyRectWithSpacing(candidate, buildings, minSpacing) {
			buildings = append(buildings, candidate)
		}
	}

	return buildings
}

// getCompoundBuildingCount returns the number of buildings for a compound.
func getCompoundBuildingCount(perimeter rect, compoundType CompoundType, rng *rand.Rand) int {
	area := perimeter.w * perimeter.h

	switch compoundType {
	case CompoundTypeIndustrial:
		// Industrial: 3-8 buildings (warehouses, offices, workshops)
		if area > 400000 {
			return 5 + rng.Intn(4) // 5-8 buildings
		}
		return 3 + rng.Intn(3) // 3-5 buildings

	case CompoundTypeResidential:
		// Residential: 4-12 houses
		if area > 300000 {
			return 8 + rng.Intn(5) // 8-12 buildings
		}
		return 4 + rng.Intn(5) // 4-8 buildings

	case CompoundTypeMilitary:
		// Military: 3-6 specialized buildings
		if area > 350000 {
			return 4 + rng.Intn(3) // 4-6 buildings
		}
		return 3 + rng.Intn(2) // 3-4 buildings

	case CompoundTypeAgricultural:
		// Agricultural: 2-6 farm buildings
		if area > 500000 {
			return 4 + rng.Intn(3) // 4-6 buildings
		}
		return 2 + rng.Intn(3) // 2-4 buildings

	case CompoundTypeCivic:
		// Civic: 2-4 institutional buildings
		return 2 + rng.Intn(3) // 2-4 buildings

	default:
		return 2 + rng.Intn(3)
	}
}

// getBuildingSizeForCompound returns appropriate building dimensions for compound type.
func getBuildingSizeForCompound(compoundType CompoundType, rng *rand.Rand) (int, int) {
	unit := 64

	switch compoundType {
	case CompoundTypeIndustrial:
		// Industrial: mix of large and medium buildings
		if rng.Float64() < 0.4 {
			// Large warehouse/factory
			w := (8 + rng.Intn(8)) * unit // 8-15 units
			h := (6 + rng.Intn(6)) * unit // 6-11 units
			return w, h
		}
		// Medium office/workshop
		w := (4 + rng.Intn(4)) * unit // 4-7 units
		h := (4 + rng.Intn(4)) * unit // 4-7 units
		return w, h

	case CompoundTypeResidential:
		// Residential: consistent house sizes
		w := (3 + rng.Intn(4)) * unit // 3-6 units
		h := (3 + rng.Intn(3)) * unit // 3-5 units
		return w, h

	case CompoundTypeMilitary:
		// Military: mix of specialized building sizes
		if rng.Float64() < 0.3 {
			// Large barracks/command center
			w := (6 + rng.Intn(6)) * unit // 6-11 units
			h := (5 + rng.Intn(5)) * unit // 5-9 units
			return w, h
		}
		// Small bunker/guard post
		w := (3 + rng.Intn(3)) * unit // 3-5 units
		h := (3 + rng.Intn(3)) * unit // 3-5 units
		return w, h

	case CompoundTypeAgricultural:
		// Agricultural: mix of barns and outbuildings
		if rng.Float64() < 0.4 {
			// Large barn/silo
			w := (8 + rng.Intn(6)) * unit // 8-13 units
			h := (6 + rng.Intn(4)) * unit // 6-9 units
			return w, h
		}
		// Small outbuilding
		w := (3 + rng.Intn(4)) * unit // 3-6 units
		h := (3 + rng.Intn(3)) * unit // 3-5 units
		return w, h

	case CompoundTypeCivic:
		// Civic: medium institutional buildings
		w := (5 + rng.Intn(5)) * unit // 5-9 units
		h := (4 + rng.Intn(4)) * unit // 4-7 units
		return w, h

	default:
		w := (4 + rng.Intn(4)) * unit
		h := (4 + rng.Intn(4)) * unit
		return w, h
	}
}

// getBuildingTypeForCompound assigns building types within compounds.
func getBuildingTypeForCompound(compoundType CompoundType, buildingIndex, _ int, rng *rand.Rand) BuildingType {
	switch compoundType {
	case CompoundTypeIndustrial:
		// Mix of warehouses, offices, workshops
		typeRoll := rng.Float64()
		switch {
		case typeRoll < 0.5:
			return BuildingTypeIndustrial // Warehouse/factory
		case typeRoll < 0.8:
			return BuildingTypeCommercial // Office building
		default:
			return BuildingTypeIndustrial // Workshop
		}

	case CompoundTypeResidential:
		return BuildingTypeResidential // All residential

	case CompoundTypeMilitary:
		// Mix of command, barracks, support
		if buildingIndex == 0 {
			return BuildingTypeMilitary // First building is command center
		}
		typeRoll := rng.Float64()
		switch {
		case typeRoll < 0.6:
			return BuildingTypeMilitary // Barracks/bunker
		case typeRoll < 0.8:
			return BuildingTypeCommercial // Admin building
		default:
			return BuildingTypeIndustrial // Workshop/maintenance
		}

	case CompoundTypeAgricultural:
		return BuildingTypeAgricultural // All agricultural

	case CompoundTypeCivic:
		return BuildingTypeCommercial // Government/institutional buildings

	default:
		return BuildingTypeGeneric
	}
}

// generatePerimeterWalls creates defensive walls around compounds that need them.
func generatePerimeterWalls(perimeter rect, compoundType CompoundType, rng *rand.Rand) []rect {
	walls := make([]rect, 0, 20)

	// Only military and some industrial compounds get perimeter walls
	needsWalls := false
	switch compoundType {
	case CompoundTypeMilitary:
		needsWalls = true // Always wall military compounds
	case CompoundTypeIndustrial:
		needsWalls = rng.Float64() < 0.4 // 40% chance for industrial
	case CompoundTypeAgricultural:
		needsWalls = rng.Float64() < 0.6 // 60% chance for farms (livestock fencing)
	default:
		needsWalls = rng.Float64() < 0.1 // 10% chance for others
	}

	if !needsWalls {
		return walls
	}

	wallThickness := 16

	// North wall
	for x := perimeter.x; x < perimeter.x+perimeter.w; x += wallThickness {
		walls = append(walls, rect{x: x, y: perimeter.y, w: wallThickness, h: wallThickness})
	}

	// South wall
	for x := perimeter.x; x < perimeter.x+perimeter.w; x += wallThickness {
		walls = append(walls, rect{x: x, y: perimeter.y + perimeter.h - wallThickness, w: wallThickness, h: wallThickness})
	}

	// West wall
	for y := perimeter.y + wallThickness; y < perimeter.y+perimeter.h-wallThickness; y += wallThickness {
		walls = append(walls, rect{x: perimeter.x, y: y, w: wallThickness, h: wallThickness})
	}

	// East wall
	for y := perimeter.y + wallThickness; y < perimeter.y+perimeter.h-wallThickness; y += wallThickness {
		walls = append(walls, rect{x: perimeter.x + perimeter.w - wallThickness, y: y, w: wallThickness, h: wallThickness})
	}

	return walls
}

// generateCompoundGates creates entry/exit points in compound perimeters.
func generateCompoundGates(perimeter rect, compoundType CompoundType, rng *rand.Rand) []Point {
	gates := make([]Point, 0, 4)

	// Number of gates based on compound type
	var gateCount int
	switch compoundType {
	case CompoundTypeIndustrial:
		gateCount = 2 + rng.Intn(2) // 2-3 gates (truck access)
	case CompoundTypeResidential:
		gateCount = 1 + rng.Intn(2) // 1-2 gates
	case CompoundTypeMilitary:
		gateCount = 1 + rng.Intn(2) // 1-2 gates (security)
	case CompoundTypeAgricultural:
		gateCount = 2 + rng.Intn(3) // 2-4 gates (equipment access)
	default:
		gateCount = 1 + rng.Intn(2)
	}

	// Place gates on different faces
	faces := []int{0, 1, 2, 3} // N, S, E, W
	rng.Shuffle(len(faces), func(i, j int) { faces[i], faces[j] = faces[j], faces[i] })

	for i := 0; i < gateCount && i < len(faces); i++ {
		face := faces[i]

		var gateX, gateY float64
		switch face {
		case 0: // North
			gateX = float64(perimeter.x + rng.Intn(perimeter.w))
			gateY = float64(perimeter.y)
		case 1: // South
			gateX = float64(perimeter.x + rng.Intn(perimeter.w))
			gateY = float64(perimeter.y + perimeter.h)
		case 2: // East
			gateX = float64(perimeter.x + perimeter.w)
			gateY = float64(perimeter.y + rng.Intn(perimeter.h))
		case 3: // West
			gateX = float64(perimeter.x)
			gateY = float64(perimeter.y + rng.Intn(perimeter.h))
		}

		gates = append(gates, Point{X: gateX, Y: gateY})
	}

	return gates
}

// generateInternalPaths creates roads/paths connecting buildings within compounds.
func generateInternalPaths(buildings []rect, perimeter rect, rng *rand.Rand) []rect {
	paths := make([]rect, 0, 10)

	if len(buildings) < 2 {
		return paths
	}

	pathWidth := 32 // 32-pixel internal paths

	// Create a main internal road if compound is large enough
	if perimeter.w > 400 && perimeter.h > 300 {
		// Central road running lengthwise
		if perimeter.w > perimeter.h {
			// Horizontal central road
			roadY := perimeter.y + perimeter.h/2 - pathWidth/2
			paths = append(paths, rect{
				x: perimeter.x + 48,
				y: roadY,
				w: perimeter.w - 96,
				h: pathWidth,
			})
		} else {
			// Vertical central road
			roadX := perimeter.x + perimeter.w/2 - pathWidth/2
			paths = append(paths, rect{
				x: roadX,
				y: perimeter.y + 48,
				w: pathWidth,
				h: perimeter.h - 96,
			})
		}
	}

	// Create connecting paths between some buildings
	connectionCount := len(buildings) / 3 // Connect about 1/3 of building pairs
	for i := 0; i < connectionCount; i++ {
		if len(buildings) < 2 {
			break
		}

		building1 := buildings[rng.Intn(len(buildings))]
		building2 := buildings[rng.Intn(len(buildings))]

		if building1 == building2 {
			continue
		}

		// Create simple connecting path
		path := generateConnectingPath(building1, building2, pathWidth/2) // Narrower connecting paths
		if path.w > 0 && path.h > 0 {
			paths = append(paths, path)
		}
	}

	return paths
}

// generateConnectingPath creates a path between two buildings.
func generateConnectingPath(building1, building2 rect, pathWidth int) rect {
	// Simple L-shaped path between building centers
	center1X := building1.x + building1.w/2
	center1Y := building1.y + building1.h/2
	center2X := building2.x + building2.w/2
	center2Y := building2.y + building2.h/2

	// Create path from building1 to building2 center
	minX := min(center1X, center2X) - pathWidth/2
	minY := min(center1Y, center2Y) - pathWidth/2
	maxX := max(center1X, center2X) + pathWidth/2
	maxY := max(center1Y, center2Y) + pathWidth/2

	return rect{
		x: minX,
		y: minY,
		w: maxX - minX,
		h: maxY - minY,
	}
}

// generateCompoundScatterObjects creates context-appropriate objects within compounds.
func generateCompoundScatterObjects(buildings []rect, compoundType CompoundType, perimeter rect, rng *rand.Rand) []ExteriorFeature {
	objects := make([]ExteriorFeature, 0, 20)

	// Generate scatter objects based on compound type
	objectCount := getScatterObjectCount(perimeter, compoundType, rng)

	for i := 0; i < objectCount; i++ {
		obj := generateSingleScatterObject(perimeter, compoundType, buildings, rng)
		if obj.Type != FeatureTypeNone {
			objects = append(objects, obj)
		}
	}

	return objects
}

// getScatterObjectCount returns the number of scatter objects for a compound.
func getScatterObjectCount(perimeter rect, compoundType CompoundType, rng *rand.Rand) int {
	area := perimeter.w * perimeter.h
	baseCount := int(float64(area) / 20000) // 1 object per 20,000 pixels²

	switch compoundType {
	case CompoundTypeIndustrial:
		return baseCount + rng.Intn(baseCount) // High object density
	case CompoundTypeMilitary:
		return baseCount/2 + rng.Intn(baseCount) // Medium density (defensive objects)
	case CompoundTypeAgricultural:
		return baseCount + rng.Intn(baseCount*2) // Very high density (equipment, storage)
	default:
		return baseCount/2 + rng.Intn(baseCount/2) // Lower density
	}
}

// generateSingleScatterObject creates one context-appropriate scatter object.
func generateSingleScatterObject(perimeter rect, compoundType CompoundType, buildings []rect, rng *rand.Rand) ExteriorFeature {
	// Choose random position within perimeter (avoiding buildings)
	attempts := 0
	maxAttempts := 10

	for attempts < maxAttempts {
		attempts++

		objSize := 16 + rng.Intn(32) // 16-48 pixel objects
		objX := perimeter.x + 32 + rng.Intn(perimeter.w-64-objSize)
		objY := perimeter.y + 32 + rng.Intn(perimeter.h-64-objSize)

		candidate := rect{x: objX, y: objY, w: objSize, h: objSize}

		// Check if position is clear of buildings
		if !overlapsAnyRect(candidate, buildings) {
			return createScatterObjectForType(candidate, compoundType, rng)
		}
	}

	return ExteriorFeature{Type: FeatureTypeNone}
}

type scatterChoice struct {
	threshold  float64
	objectType ObjectType
	groundType GroundType
}

var scatterChoicesByCompound = map[CompoundType][]scatterChoice{
	CompoundTypeIndustrial: {
		{threshold: 0.4, objectType: ObjectCrate, groundType: GroundConcrete},
		{threshold: 0.6, objectType: ObjectPallet, groundType: GroundConcrete},
		{threshold: 0.8, objectType: ObjectMachinery, groundType: GroundConcrete},
		{threshold: 1.0, objectType: ObjectBarrel, groundType: GroundGravel},
	},
	CompoundTypeMilitary: {
		{threshold: 0.5, objectType: ObjectSandbag, groundType: GroundDirt},
		{threshold: 0.7, objectType: ObjectCrate, groundType: GroundConcrete},
		{threshold: 0.9, objectType: ObjectWire, groundType: GroundDirt},
		{threshold: 1.0, objectType: ObjectRubblePile, groundType: GroundRubbleLight},
	},
	CompoundTypeAgricultural: {
		{threshold: 0.4, objectType: ObjectCrate, groundType: GroundDirt},
		{threshold: 0.6, objectType: ObjectBarrel, groundType: GroundDirt},
		{threshold: 0.8, objectType: ObjectFence, groundType: GroundGrass},
		{threshold: 1.0, objectType: ObjectPallet, groundType: GroundDirt},
	},
	CompoundTypeResidential: {
		{threshold: 0.3, objectType: ObjectFence, groundType: GroundGrass},
		{threshold: 0.6, objectType: ObjectCrate, groundType: GroundPavement},
		{threshold: 0.8, objectType: ObjectChair, groundType: GroundPavement},
		{threshold: 1.0, objectType: ObjectTable, groundType: GroundGrass},
	},
}

func chooseScatterObjectType(compoundType CompoundType, rng *rand.Rand) (ObjectType, GroundType) {
	choices, ok := scatterChoicesByCompound[compoundType]
	if !ok {
		return ObjectCrate, GroundGrass
	}
	roll := rng.Float64()
	for i := range choices {
		if roll < choices[i].threshold {
			return choices[i].objectType, choices[i].groundType
		}
	}
	last := choices[len(choices)-1]
	return last.objectType, last.groundType
}

// createScatterObjectForType creates appropriate scatter objects for compound types.
func createScatterObjectForType(position rect, compoundType CompoundType, rng *rand.Rand) ExteriorFeature {
	objectType, groundType := chooseScatterObjectType(compoundType, rng)

	return ExteriorFeature{
		Type:        FeatureTypeNone, // Use FeatureTypeNone for scatter objects
		AttachedTo:  rect{},          // Not attached to specific building
		FeatureRect: position,
		ObjectType:  objectType,
		GroundType:  groundType,
	}
}

// overlapsAnyRect checks if a rectangle overlaps with any rectangle in a slice.
func overlapsAnyRect(candidate rect, existingRects []rect) bool {
	for _, existing := range existingRects {
		if rectsOverlap(candidate, existing) {
			return true
		}
	}
	return false
}

// overlapsAnyRectWithSpacing checks overlaps with minimum spacing requirement.
func overlapsAnyRectWithSpacing(candidate rect, existingRects []rect, minSpacing int) bool {
	// Expand candidate by minSpacing to check spacing
	expanded := rect{
		x: candidate.x - minSpacing,
		y: candidate.y - minSpacing,
		w: candidate.w + 2*minSpacing,
		h: candidate.h + 2*minSpacing,
	}

	return overlapsAnyRect(expanded, existingRects)
}

// rectsOverlap checks if two rectangles overlap.
func rectsOverlap(a, b rect) bool {
	return a.x < b.x+b.w && a.x+a.w > b.x && a.y < b.y+b.h && a.y+a.h > b.y
}
