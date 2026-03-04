package game

import (
	"fmt"
	"math/rand"
)

// BuildingName represents a procedurally generated building identifier.
type BuildingName struct {
	DisplayName string        // Full display name (e.g., "Warehouse A", "Small House")
	ShortName   string        // Abbreviated name (e.g., "WH-A", "House")
	Type        BuildingType  // Building type for context
	Shape       BuildingShape // Building shape for description
	ID          int           // Unique identifier number
}

// generateBuildingNames creates contextual names for all buildings.
func generateBuildingNames(buildings []rect, buildingTypes []BuildingType, buildingShapes []BuildingShape, rng *rand.Rand) []BuildingName {
	names := make([]BuildingName, 0, len(buildings))

	// Track naming counters for each type
	typeCounters := make(map[BuildingType]int)

	for i, building := range buildings {
		var buildingType BuildingType
		var buildingShape BuildingShape

		if i < len(buildingTypes) {
			buildingType = buildingTypes[i]
		} else {
			buildingType = BuildingTypeGeneric
		}

		if i < len(buildingShapes) {
			buildingShape = buildingShapes[i]
		} else {
			buildingShape = ShapeRectangular
		}

		typeCounters[buildingType]++

		name := generateSingleBuildingName(building, buildingType, buildingShape, typeCounters[buildingType], rng)
		name.ID = i + 1 // 1-based building IDs

		names = append(names, name)
	}

	return names
}

// generateSingleBuildingName creates a contextual name for one building.
func generateSingleBuildingName(building rect, buildingType BuildingType, buildingShape BuildingShape, typeIndex int, rng *rand.Rand) BuildingName {
	area := building.w * building.h

	// Base name components
	var sizePrefix, typeString, shapeSuffix string

	// Size-based prefixes
	switch {
	case area < 15000:
		sizePrefix = "Small"
	case area < 35000:
		sizePrefix = "" // No prefix for medium buildings
	case area < 80000:
		sizePrefix = "Large"
	default:
		sizePrefix = "Massive"
	}

	// Type-based naming
	switch buildingType {
	case BuildingTypeResidential:
		typeString = getResidentialBuildingName(area, rng)
	case BuildingTypeCommercial:
		typeString = getCommercialBuildingName(area, rng)
	case BuildingTypeIndustrial:
		typeString = getIndustrialBuildingName(area, rng)
	case BuildingTypeMilitary:
		typeString = getMilitaryBuildingName(area, rng)
	case BuildingTypeAgricultural:
		typeString = getAgriculturalBuildingName(area, rng)
	default:
		typeString = "Building"
	}

	// Shape-based suffixes for complex buildings
	switch buildingShape {
	case ShapeLShaped:
		shapeSuffix = " (L-Wing)"
	case ShapeTShaped:
		shapeSuffix = " (T-Block)"
	case ShapeUShaped:
		shapeSuffix = " (U-Complex)"
	default:
		shapeSuffix = "" // No suffix for rectangular
	}

	// Construct display name
	var displayName string
	if sizePrefix != "" {
		displayName = fmt.Sprintf("%s %s", sizePrefix, typeString)
	} else {
		displayName = typeString
	}

	// Add alphabetic identifier for same-type buildings
	if typeIndex > 1 {
		letter := string(rune('A' + (typeIndex-1)%26))
		displayName = fmt.Sprintf("%s %s", displayName, letter)
	}

	displayName += shapeSuffix

	// Create short name
	shortName := generateShortName(buildingType, typeIndex, buildingShape)

	return BuildingName{
		DisplayName: displayName,
		ShortName:   shortName,
		Type:        buildingType,
		Shape:       buildingShape,
	}
}

// getResidentialBuildingName returns context-appropriate residential building names.
func getResidentialBuildingName(area int, rng *rand.Rand) string {
	switch {
	case area < 15000:
		// Small residential
		options := []string{"Cottage", "House", "Cabin", "Bungalow"}
		return options[rng.Intn(len(options))]
	case area < 35000:
		// Medium residential
		options := []string{"House", "Residence", "Home", "Villa"}
		return options[rng.Intn(len(options))]
	default:
		// Large residential
		options := []string{"Mansion", "Estate", "Manor", "Complex"}
		return options[rng.Intn(len(options))]
	}
}

// getCommercialBuildingName returns context-appropriate commercial building names.
func getCommercialBuildingName(area int, rng *rand.Rand) string {
	switch {
	case area < 20000:
		// Small commercial
		options := []string{"Shop", "Store", "Office", "Clinic"}
		return options[rng.Intn(len(options))]
	case area < 50000:
		// Medium commercial
		options := []string{"Office Building", "Shopping Center", "Market", "Retail Store"}
		return options[rng.Intn(len(options))]
	default:
		// Large commercial
		options := []string{"Mall", "Corporate Center", "Plaza", "Complex"}
		return options[rng.Intn(len(options))]
	}
}

// getIndustrialBuildingName returns context-appropriate industrial building names.
func getIndustrialBuildingName(area int, rng *rand.Rand) string {
	switch {
	case area < 30000:
		// Small industrial
		options := []string{"Workshop", "Garage", "Plant", "Facility"}
		return options[rng.Intn(len(options))]
	case area < 80000:
		// Medium industrial
		options := []string{"Warehouse", "Factory", "Plant", "Depot"}
		return options[rng.Intn(len(options))]
	default:
		// Large industrial
		options := []string{"Manufacturing Complex", "Industrial Facility", "Processing Plant", "Distribution Center"}
		return options[rng.Intn(len(options))]
	}
}

// getMilitaryBuildingName returns context-appropriate military building names.
func getMilitaryBuildingName(area int, rng *rand.Rand) string {
	switch {
	case area < 25000:
		// Small military
		options := []string{"Bunker", "Guard Post", "Outpost", "Checkpoint"}
		return options[rng.Intn(len(options))]
	case area < 60000:
		// Medium military
		options := []string{"Barracks", "Command Post", "Operations Center", "Armory"}
		return options[rng.Intn(len(options))]
	default:
		// Large military
		options := []string{"Command Complex", "Base Headquarters", "Operations Facility", "Military Installation"}
		return options[rng.Intn(len(options))]
	}
}

// getAgriculturalBuildingName returns context-appropriate agricultural building names.
func getAgriculturalBuildingName(area int, rng *rand.Rand) string {
	switch {
	case area < 25000:
		// Small agricultural
		options := []string{"Shed", "Coop", "Storage", "Outbuilding"}
		return options[rng.Intn(len(options))]
	case area < 60000:
		// Medium agricultural
		options := []string{"Barn", "Silo", "Stable", "Equipment Shed"}
		return options[rng.Intn(len(options))]
	default:
		// Large agricultural
		options := []string{"Agricultural Complex", "Farm Center", "Processing Facility", "Storage Complex"}
		return options[rng.Intn(len(options))]
	}
}

// generateShortName creates abbreviated building names for UI display.
func generateShortName(buildingType BuildingType, typeIndex int, buildingShape BuildingShape) string {
	var typeAbbrev string

	switch buildingType {
	case BuildingTypeResidential:
		typeAbbrev = "RES"
	case BuildingTypeCommercial:
		typeAbbrev = "COM"
	case BuildingTypeIndustrial:
		typeAbbrev = "IND"
	case BuildingTypeMilitary:
		typeAbbrev = "MIL"
	case BuildingTypeAgricultural:
		typeAbbrev = "AGR"
	default:
		typeAbbrev = "BLD"
	}

	// Add index if multiple buildings of same type
	if typeIndex > 1 {
		letter := string(rune('A' + (typeIndex-1)%26))
		typeAbbrev = fmt.Sprintf("%s-%s", typeAbbrev, letter)
	}

	// Add shape suffix for complex buildings
	switch buildingShape {
	case ShapeLShaped:
		typeAbbrev += "-L"
	case ShapeTShaped:
		typeAbbrev += "-T"
	case ShapeUShaped:
		typeAbbrev += "-U"
	}

	return typeAbbrev
}
