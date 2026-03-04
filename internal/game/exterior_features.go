package game

import (
	"math/rand"
)

// ExteriorFeatureType represents different types of building attachments.
type ExteriorFeatureType uint8

const (
	// FeatureTypeNone indicates no exterior feature.
	FeatureTypeNone ExteriorFeatureType = iota
	// FeatureTypePorch indicates a covered residential entrance.
	FeatureTypePorch // Residential covered entrance
	// FeatureTypeBalcony indicates a second-floor platform.
	FeatureTypeBalcony // Residential second-floor platform
	// FeatureTypeLoadingDock indicates an industrial loading area.
	FeatureTypeLoadingDock // Industrial truck loading area
	// FeatureTypeFencedYard indicates an enclosed exterior yard.
	FeatureTypeFencedYard // Residential/agricultural enclosed area
	// FeatureTypeParking indicates a parking area.
	FeatureTypeParking // Commercial parking area
	// FeatureTypeWalkway indicates a connecting path.
	FeatureTypeWalkway // Path between buildings
	// FeatureTypeAwning indicates a storefront covering.
	FeatureTypeAwning // Commercial storefront covering
	// FeatureTypeShed indicates detached storage.
	FeatureTypeShed // Small detached storage
	// FeatureTypeCarport indicates covered parking.
	FeatureTypeCarport // Covered parking structure
)

// ExteriorFeature represents an attached structure to a building.
type ExteriorFeature struct {
	AttachedTo  rect       // Building footprint this attaches to.
	FeatureRect rect       // Dimensions of the feature itself.
	ObjectType  ObjectType // What ObjectType to place in tiles.
	GroundType  GroundType // Ground surface for the feature.

	Type         ExteriorFeatureType // Type of exterior feature
	AttachedFace int                 // Which building face (0=N, 1=S, 2=E, 3=W)
}

// generateExteriorFeatures creates appropriate exterior attachments for buildings.
func generateExteriorFeatures(buildings []rect, buildingTypes []BuildingType, rng *rand.Rand) []ExteriorFeature {
	features := make([]ExteriorFeature, 0, len(buildings))

	for i, building := range buildings {
		var buildingType BuildingType
		if i < len(buildingTypes) {
			buildingType = buildingTypes[i]
		} else {
			buildingType = BuildingTypeGeneric
		}

		// Generate 1-3 exterior features per building based on type and size
		featureCount := getExteriorFeatureCount(building, buildingType, rng)

		for j := 0; j < featureCount; j++ {
			feature := generateSingleExteriorFeature(building, buildingType, rng)
			if feature.Type != FeatureTypeNone {
				features = append(features, feature)
			}
		}
	}

	return features
}

// getExteriorFeatureCount determines how many exterior features a building should have.
func getExteriorFeatureCount(building rect, buildingType BuildingType, rng *rand.Rand) int {
	area := building.w * building.h

	switch buildingType {
	case BuildingTypeResidential:
		// Houses usually have 1-2 features (porch, shed, carport)
		if area > 20000 { // Large houses
			return 1 + rng.Intn(3) // 1-3 features
		}
		return 1 + rng.Intn(2) // 1-2 features

	case BuildingTypeCommercial:
		// Commercial buildings usually have storefronts, awnings
		return 1 + rng.Intn(2) // 1-2 features

	case BuildingTypeIndustrial:
		// Industrial buildings have loading docks, yards
		if area > 30000 {
			return 2 + rng.Intn(3) // 2-4 features
		}
		return 1 + rng.Intn(2) // 1-2 features

	case BuildingTypeMilitary:
		// Military buildings have defensive features
		return 1 + rng.Intn(2) // 1-2 features

	case BuildingTypeAgricultural:
		// Farms have multiple outbuildings
		if area > 25000 {
			return 2 + rng.Intn(3) // 2-4 features
		}
		return 1 + rng.Intn(2) // 1-2 features

	default:
		return 1
	}
}

// generateSingleExteriorFeature creates one exterior feature for a building.
func generateSingleExteriorFeature(building rect, buildingType BuildingType, rng *rand.Rand) ExteriorFeature {
	// Choose feature type based on building type
	featureTypes := getFeatureTypesForBuilding(buildingType)
	if len(featureTypes) == 0 {
		return ExteriorFeature{Type: FeatureTypeNone}
	}

	featureType := featureTypes[rng.Intn(len(featureTypes))]

	// Choose which face of building to attach to
	attachedFace := rng.Intn(4) // 0=N, 1=S, 2=E, 3=W

	// Generate feature geometry based on type and attached face
	feature := createFeatureGeometry(building, featureType, attachedFace, rng)

	return feature
}

// getFeatureTypesForBuilding returns appropriate exterior feature types for building type.
func getFeatureTypesForBuilding(buildingType BuildingType) []ExteriorFeatureType {
	switch buildingType {
	case BuildingTypeResidential:
		return []ExteriorFeatureType{
			FeatureTypePorch,
			FeatureTypeFencedYard,
			FeatureTypeShed,
			FeatureTypeCarport,
		}

	case BuildingTypeCommercial:
		return []ExteriorFeatureType{
			FeatureTypeAwning,
			FeatureTypeParking,
			FeatureTypeWalkway,
		}

	case BuildingTypeIndustrial:
		return []ExteriorFeatureType{
			FeatureTypeLoadingDock,
			FeatureTypeParking,
			FeatureTypeFencedYard,
			FeatureTypeShed,
		}

	case BuildingTypeMilitary:
		return []ExteriorFeatureType{
			FeatureTypeFencedYard, // Security perimeter
			FeatureTypeWalkway,    // Guard paths
		}

	case BuildingTypeAgricultural:
		return []ExteriorFeatureType{
			FeatureTypeShed,
			FeatureTypeFencedYard,
			FeatureTypeCarport, // Equipment shelter
		}

	default:
		return []ExteriorFeatureType{FeatureTypeShed}
	}
}

// createFeatureGeometry generates the geometry for an exterior feature.
func createFeatureGeometry(building rect, featureType ExteriorFeatureType, attachedFace int, rng *rand.Rand) ExteriorFeature {
	var featureRect rect
	var objectType ObjectType
	var groundType GroundType

	switch featureType {
	case FeatureTypePorch:
		// Residential porch along building face
		featureRect = generatePorchGeometry(building, attachedFace, rng)
		objectType = ObjectNone // Open porch
		groundType = GroundWood

	case FeatureTypeLoadingDock:
		// Industrial loading dock
		featureRect = generateLoadingDockGeometry(building, attachedFace, rng)
		objectType = ObjectCrate // Represent loading area with crates
		groundType = GroundConcrete

	case FeatureTypeFencedYard:
		// Enclosed area with fence perimeter
		featureRect = generateYardGeometry(building, attachedFace, rng)
		objectType = ObjectFence
		groundType = GroundGravel

	case FeatureTypeParking:
		// Paved parking area
		featureRect = generateParkingGeometry(building, attachedFace, rng)
		objectType = ObjectNone // Open parking
		groundType = GroundTarmac

	case FeatureTypeAwning:
		// Commercial storefront awning
		featureRect = generateAwningGeometry(building, attachedFace, rng)
		objectType = ObjectNone // Cosmetic awning
		groundType = GroundPavement

	case FeatureTypeShed:
		// Small detached storage building
		featureRect = generateShedGeometry(building, attachedFace, rng)
		objectType = ObjectWall // Small building
		groundType = GroundGravel

	case FeatureTypeCarport:
		// Covered parking structure
		featureRect = generateCarportGeometry(building, attachedFace, rng)
		objectType = ObjectPillar // Support pillars
		groundType = GroundConcrete

	case FeatureTypeWalkway:
		// Connecting path
		featureRect = generateWalkwayGeometry(building, attachedFace, rng)
		objectType = ObjectNone // Open path
		groundType = GroundPavement

	default:
		return ExteriorFeature{Type: FeatureTypeNone}
	}

	// Ensure feature has valid dimensions
	if featureRect.w <= 0 || featureRect.h <= 0 {
		return ExteriorFeature{Type: FeatureTypeNone}
	}

	return ExteriorFeature{
		Type:         featureType,
		AttachedTo:   building,
		FeatureRect:  featureRect,
		AttachedFace: attachedFace,
		ObjectType:   objectType,
		GroundType:   groundType,
	}
}

// generatePorchGeometry creates a porch along a building face.
func generatePorchGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	porchDepth := 32 + rng.Intn(32) // 32-64 pixel porch depth

	switch attachedFace {
	case 0: // North face
		porchW := int(float64(building.w) * (0.5 + rng.Float64()*0.4)) // 50-90% building width
		porchX := building.x + (building.w-porchW)/2                   // Center on building
		return rect{x: porchX, y: building.y - porchDepth, w: porchW, h: porchDepth}

	case 1: // South face
		porchW := int(float64(building.w) * (0.5 + rng.Float64()*0.4))
		porchX := building.x + (building.w-porchW)/2
		return rect{x: porchX, y: building.y + building.h, w: porchW, h: porchDepth}

	case 2: // East face
		porchH := int(float64(building.h) * (0.5 + rng.Float64()*0.4))
		porchY := building.y + (building.h-porchH)/2
		return rect{x: building.x + building.w, y: porchY, w: porchDepth, h: porchH}

	case 3: // West face
		porchH := int(float64(building.h) * (0.5 + rng.Float64()*0.4))
		porchY := building.y + (building.h-porchH)/2
		return rect{x: building.x - porchDepth, y: porchY, w: porchDepth, h: porchH}

	default:
		return rect{}
	}
}

// generateLoadingDockGeometry creates a loading dock for industrial buildings.
func generateLoadingDockGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	dockDepth := 48 + rng.Intn(48) // 48-96 pixel dock depth

	switch attachedFace {
	case 0: // North face
		dockW := int(float64(building.w) * (0.3 + rng.Float64()*0.5)) // 30-80% building width
		dockX := building.x + rng.Intn(building.w-dockW)              // Random position along face
		return rect{x: dockX, y: building.y - dockDepth, w: dockW, h: dockDepth}

	case 1: // South face
		dockW := int(float64(building.w) * (0.3 + rng.Float64()*0.5))
		dockX := building.x + rng.Intn(building.w-dockW)
		return rect{x: dockX, y: building.y + building.h, w: dockW, h: dockDepth}

	case 2: // East face
		dockH := int(float64(building.h) * (0.3 + rng.Float64()*0.5))
		dockY := building.y + rng.Intn(building.h-dockH)
		return rect{x: building.x + building.w, y: dockY, w: dockDepth, h: dockH}

	case 3: // West face
		dockH := int(float64(building.h) * (0.3 + rng.Float64()*0.5))
		dockY := building.y + rng.Intn(building.h-dockH)
		return rect{x: building.x - dockDepth, y: dockY, w: dockDepth, h: dockH}

	default:
		return rect{}
	}
}

// generateYardGeometry creates a fenced yard around a building.
func generateYardGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	yardDepth := 64 + rng.Intn(96) // 64-160 pixel yard depth

	switch attachedFace {
	case 0: // North face
		return rect{x: building.x - 16, y: building.y - yardDepth, w: building.w + 32, h: yardDepth}

	case 1: // South face
		return rect{x: building.x - 16, y: building.y + building.h, w: building.w + 32, h: yardDepth}

	case 2: // East face
		return rect{x: building.x + building.w, y: building.y - 16, w: yardDepth, h: building.h + 32}

	case 3: // West face
		return rect{x: building.x - yardDepth, y: building.y - 16, w: yardDepth, h: building.h + 32}

	default:
		return rect{}
	}
}

// generateParkingGeometry creates a parking area near a building.
func generateParkingGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	parkingDepth := 80 + rng.Intn(80) // 80-160 pixel parking depth

	switch attachedFace {
	case 0: // North face
		parkingW := int(float64(building.w) * (0.6 + rng.Float64()*0.4)) // 60-100% building width
		parkingX := building.x + rng.Intn(building.w-parkingW)
		return rect{x: parkingX, y: building.y - parkingDepth, w: parkingW, h: parkingDepth}

	case 1: // South face
		parkingW := int(float64(building.w) * (0.6 + rng.Float64()*0.4))
		parkingX := building.x + rng.Intn(building.w-parkingW)
		return rect{x: parkingX, y: building.y + building.h, w: parkingW, h: parkingDepth}

	case 2: // East face
		parkingH := int(float64(building.h) * (0.6 + rng.Float64()*0.4))
		parkingY := building.y + rng.Intn(building.h-parkingH)
		return rect{x: building.x + building.w, y: parkingY, w: parkingDepth, h: parkingH}

	case 3: // West face
		parkingH := int(float64(building.h) * (0.6 + rng.Float64()*0.4))
		parkingY := building.y + rng.Intn(building.h-parkingH)
		return rect{x: building.x - parkingDepth, y: parkingY, w: parkingDepth, h: parkingH}

	default:
		return rect{}
	}
}

// generateShedGeometry creates a small detached storage building.
func generateShedGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	shedSize := 32 + rng.Intn(48) // 32-80 pixel shed (small)
	distance := 48 + rng.Intn(64) // 48-112 pixels from main building

	switch attachedFace {
	case 0: // North face
		shedX := building.x + rng.Intn(building.w-shedSize)
		return rect{x: shedX, y: building.y - distance - shedSize, w: shedSize, h: shedSize}

	case 1: // South face
		shedX := building.x + rng.Intn(building.w-shedSize)
		return rect{x: shedX, y: building.y + building.h + distance, w: shedSize, h: shedSize}

	case 2: // East face
		shedY := building.y + rng.Intn(building.h-shedSize)
		return rect{x: building.x + building.w + distance, y: shedY, w: shedSize, h: shedSize}

	case 3: // West face
		shedY := building.y + rng.Intn(building.h-shedSize)
		return rect{x: building.x - distance - shedSize, y: shedY, w: shedSize, h: shedSize}

	default:
		return rect{}
	}
}

// generateCarportGeometry creates a covered parking structure.
func generateCarportGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	carportDepth := 64 + rng.Intn(32) // 64-96 pixel carport depth

	switch attachedFace {
	case 0: // North face
		carportW := int(float64(building.w) * (0.4 + rng.Float64()*0.4)) // 40-80% building width
		carportX := building.x + rng.Intn(building.w-carportW)
		return rect{x: carportX, y: building.y - carportDepth, w: carportW, h: carportDepth}

	case 1: // South face
		carportW := int(float64(building.w) * (0.4 + rng.Float64()*0.4))
		carportX := building.x + rng.Intn(building.w-carportW)
		return rect{x: carportX, y: building.y + building.h, w: carportW, h: carportDepth}

	case 2: // East face
		carportH := int(float64(building.h) * (0.4 + rng.Float64()*0.4))
		carportY := building.y + rng.Intn(building.h-carportH)
		return rect{x: building.x + building.w, y: carportY, w: carportDepth, h: carportH}

	case 3: // West face
		carportH := int(float64(building.h) * (0.4 + rng.Float64()*0.4))
		carportY := building.y + rng.Intn(building.h-carportH)
		return rect{x: building.x - carportDepth, y: carportY, w: carportDepth, h: carportH}

	default:
		return rect{}
	}
}

// generateAwningGeometry creates a storefront awning for commercial buildings.
func generateAwningGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	awningDepth := 16 + rng.Intn(16) // 16-32 pixel awning depth (shallow)

	switch attachedFace {
	case 0: // North face
		awningW := int(float64(building.w) * (0.7 + rng.Float64()*0.3)) // 70-100% building width
		awningX := building.x + (building.w-awningW)/2                  // Center on building
		return rect{x: awningX, y: building.y - awningDepth, w: awningW, h: awningDepth}

	case 1: // South face
		awningW := int(float64(building.w) * (0.7 + rng.Float64()*0.3))
		awningX := building.x + (building.w-awningW)/2
		return rect{x: awningX, y: building.y + building.h, w: awningW, h: awningDepth}

	case 2: // East face
		awningH := int(float64(building.h) * (0.7 + rng.Float64()*0.3))
		awningY := building.y + (building.h-awningH)/2
		return rect{x: building.x + building.w, y: awningY, w: awningDepth, h: awningH}

	case 3: // West face
		awningH := int(float64(building.h) * (0.7 + rng.Float64()*0.3))
		awningY := building.y + (building.h-awningH)/2
		return rect{x: building.x - awningDepth, y: awningY, w: awningDepth, h: awningH}

	default:
		return rect{}
	}
}

// generateWalkwayGeometry creates a connecting path between buildings.
func generateWalkwayGeometry(building rect, attachedFace int, rng *rand.Rand) rect {
	walkwayWidth := 16 + rng.Intn(16)  // 16-32 pixel walkway width
	walkwayLength := 48 + rng.Intn(96) // 48-144 pixel walkway length

	switch attachedFace {
	case 0, 1: // North or South face - horizontal walkway
		walkwayX := building.x + rng.Intn(building.w-walkwayWidth)
		var walkwayY int
		if attachedFace == 0 {
			walkwayY = building.y - walkwayLength
		} else {
			walkwayY = building.y + building.h
		}
		return rect{x: walkwayX, y: walkwayY, w: walkwayWidth, h: walkwayLength}

	case 2, 3: // East or West face - vertical walkway
		walkwayY := building.y + rng.Intn(building.h-walkwayWidth)
		var walkwayX int
		if attachedFace == 2 {
			walkwayX = building.x + building.w
		} else {
			walkwayX = building.x - walkwayLength
		}
		return rect{x: walkwayX, y: walkwayY, w: walkwayLength, h: walkwayWidth}

	default:
		return rect{}
	}
}

// stampExteriorFeature renders an exterior feature into the tile map.
func _stampExteriorFeature(tm *TileMap, feature ExteriorFeature, rng *rand.Rand) {
	cMin := feature.FeatureRect.x / cellSize
	rMin := feature.FeatureRect.y / cellSize
	cMax := (feature.FeatureRect.x + feature.FeatureRect.w - 1) / cellSize
	rMax := (feature.FeatureRect.y + feature.FeatureRect.h - 1) / cellSize

	switch feature.Type {
	case FeatureTypeFencedYard:
		// Place fence perimeter only
		for col := cMin; col <= cMax; col++ {
			if tm.inBounds(col, rMin) {
				tm.SetObject(col, rMin, ObjectFence)
				tm.SetGround(col, rMin, feature.GroundType)
			}
			if tm.inBounds(col, rMax) {
				tm.SetObject(col, rMax, ObjectFence)
				tm.SetGround(col, rMax, feature.GroundType)
			}
		}
		for row := rMin; row <= rMax; row++ {
			if tm.inBounds(cMin, row) {
				tm.SetObject(cMin, row, ObjectFence)
				tm.SetGround(cMin, row, feature.GroundType)
			}
			if tm.inBounds(cMax, row) {
				tm.SetObject(cMax, row, ObjectFence)
				tm.SetGround(cMax, row, feature.GroundType)
			}
		}

	case FeatureTypeShed:
		// Place shed walls as mini-building
		for col := cMin; col <= cMax; col++ {
			if tm.inBounds(col, rMin) {
				tm.SetObject(col, rMin, ObjectWall)
			}
			if tm.inBounds(col, rMax) {
				tm.SetObject(col, rMax, ObjectWall)
			}
		}
		for row := rMin + 1; row <= rMax-1; row++ {
			if tm.inBounds(cMin, row) {
				tm.SetObject(cMin, row, ObjectWall)
			}
			if tm.inBounds(cMax, row) {
				tm.SetObject(cMax, row, ObjectWall)
			}
		}
		// Fill interior with appropriate ground
		for row := rMin + 1; row <= rMax-1; row++ {
			for col := cMin + 1; col <= cMax-1; col++ {
				if tm.inBounds(col, row) {
					tm.SetGround(col, row, GroundConcrete)
				}
			}
		}

	case FeatureTypeCarport:
		// Place support pillars and ground
		pillarSpacing := 3 // Pillars every 3 tiles
		for row := rMin; row <= rMax; row += pillarSpacing {
			for col := cMin; col <= cMax; col += pillarSpacing {
				if tm.inBounds(col, row) {
					tm.SetObject(col, row, ObjectPillar)
					tm.SetGround(col, row, feature.GroundType)
				}
			}
		}
		// Fill remaining area with ground type
		for row := rMin; row <= rMax; row++ {
			for col := cMin; col <= cMax; col++ {
				if tm.inBounds(col, row) && tm.ObjectAt(col, row) == ObjectNone {
					tm.SetGround(col, row, feature.GroundType)
				}
			}
		}

	default:
		// For other feature types, just set ground type
		for row := rMin; row <= rMax; row++ {
			for col := cMin; col <= cMax; col++ {
				if tm.inBounds(col, row) {
					tm.SetGround(col, row, feature.GroundType)
					if feature.ObjectType != ObjectNone && rng.Float64() < 0.3 {
						// Occasionally place the feature object
						tm.SetObject(col, row, feature.ObjectType)
					}
				}
			}
		}
	}
}
