package game

import (
	"math"
	"math/rand"
)

// LotType represents different categories of land subdivision.
type LotType uint8

const (
	// LotTypeResidential is single/multi-family housing.
	LotTypeResidential LotType = iota // Single/multi-family housing
	// LotTypeCommercial is shops, offices, and services.
	LotTypeCommercial // Shops, offices, services
	// LotTypeIndustrial is manufacturing and warehouses.
	LotTypeIndustrial // Manufacturing, warehouses
	// LotTypeMilitary is bases, compounds, and bunkers.
	LotTypeMilitary // Bases, compounds, bunkers
	// LotTypeCivic is public/institutional buildings.
	LotTypeCivic // Public/institutional buildings
	// LotTypeAgricultural is farms and agricultural uses.
	LotTypeAgricultural // Farms, agricultural uses
	// LotTypeRecreational is parks and recreation.
	LotTypeRecreational // Parks, recreation
	// LotTypeTransportation is rail yards, airports.
	LotTypeTransportation // Rail yards, airports
)

// Lot represents a subdivision of land that can contain buildings.
type Lot struct { //nolint:govet
	Boundary    []Point // Polygon vertices defining lot boundaries
	Type        LotType // Intended use of the lot
	RoadAccess  bool    // True if lot connects to a road
	AccessPoint Point   // Point where lot connects to nearest road
	Buildings   []rect  // Buildings placed within this lot
	Setback     float64 // Minimum distance from lot boundary to building
	Area        float64 // Total lot area in pixels²
}

// VoronoiSite represents a seed point for Voronoi lot generation.
type VoronoiSite struct {
	Point Point
	Type  LotType
}

// generateLotSubdivision creates organic lot boundaries using Voronoi diagrams.
func generateLotSubdivision(tm *TileMap, roads []OrganicRoad, rng *rand.Rand) []Lot {
	mapW := float64(tm.Cols * cellSize)
	mapH := float64(tm.Rows * cellSize)

	// Generate Voronoi seed points
	sites := generateVoronoiSites(mapW, mapH, roads, rng)

	// Create simplified Voronoi cells (using distance-based approach)
	lots := make([]Lot, 0, len(sites))

	for i, site := range sites {
		lot := generateLotFromSite(site, sites, mapW, mapH, roads, rng)
		lot.Type = site.Type

		// Calculate road access
		lot.RoadAccess, lot.AccessPoint = findNearestRoadAccess(lot, roads)

		lots = append(lots, lot)
		_ = i // Lot index for future enhancements
	}

	return lots
}

// generateVoronoiSites creates seed points for lot subdivision.
func generateVoronoiSites(mapW, mapH float64, roads []OrganicRoad, rng *rand.Rand) []VoronoiSite {
	sites := make([]VoronoiSite, 0, 40)

	// Site density: 1 site per ~80,000 pixels² (reasonable lot sizes)
	targetSites := int((mapW * mapH) / 80000)
	if targetSites < 15 {
		targetSites = 15
	}
	if targetSites > 50 {
		targetSites = 50
	}

	// Place sites with some clustering around roads
	roadBias := 0.7 // 70% of sites near roads
	roadSites := int(float64(targetSites) * roadBias)

	// Sites near roads (higher development density)
	for i := 0; i < roadSites && len(roads) > 0; i++ {
		road := roads[rng.Intn(len(roads))]
		if len(road.Points) == 0 {
			continue
		}

		// Pick random point along road
		roadPoint := road.Points[rng.Intn(len(road.Points))]

		// Offset from road (30-200 pixels)
		offset := 30 + rng.Float64()*170
		angle := rng.Float64() * 2 * math.Pi

		siteX := roadPoint[0] + offset*math.Cos(angle)
		siteY := roadPoint[1] + offset*math.Sin(angle)

		// Clamp to map bounds
		siteX = math.Max(64, math.Min(mapW-64, siteX))
		siteY = math.Max(64, math.Min(mapH-64, siteY))

		// Determine lot type based on road type and location
		lotType := determineLotType(road, Point{siteX, siteY}, mapW, mapH, rng)

		sites = append(sites, VoronoiSite{
			Point: Point{X: siteX, Y: siteY},
			Type:  lotType,
		})
	}

	// Random sites away from roads (rural/sparse development)
	remainingSites := targetSites - len(sites)
	for i := 0; i < remainingSites; i++ {
		siteX := 64 + rng.Float64()*(mapW-128)
		siteY := 64 + rng.Float64()*(mapH-128)

		// Rural areas tend to be agricultural or recreational
		var lotType LotType
		switch {
		case rng.Float64() < 0.6:
			lotType = LotTypeAgricultural
		case rng.Float64() < 0.8:
			lotType = LotTypeRecreational
		default:
			lotType = LotTypeResidential // Sparse rural housing
		}

		sites = append(sites, VoronoiSite{
			Point: Point{X: siteX, Y: siteY},
			Type:  lotType,
		})
	}

	return sites
}

// determineLotType classifies lot type based on road type and location context.
func determineLotType(nearestRoad OrganicRoad, location Point, mapW, mapH float64, rng *rand.Rand) LotType { //nolint:gocritic
	// Center proximity (0.0 = edge, 1.0 = center)
	centerDist := math.Min(
		math.Min(location.X, mapW-location.X)/mapW,
		math.Min(location.Y, mapH-location.Y)/mapH,
	)

	// Road type influences lot type
	switch nearestRoad.Type {
	case RoadTypeHighway:
		// Near highways: commercial or industrial
		if rng.Float64() < 0.6 {
			return LotTypeCommercial
		}
		return LotTypeIndustrial

	case RoadTypeMainRoad:
		// Main roads: mixed commercial/residential
		typeRoll := rng.Float64()
		switch {
		case typeRoll < 0.4:
			return LotTypeCommercial
		case typeRoll < 0.8:
			return LotTypeResidential
		default:
			return LotTypeCivic
		}

	case RoadTypeSideStreet:
		// Side streets: mostly residential
		if centerDist > 0.3 && rng.Float64() < 0.8 {
			return LotTypeResidential
		}
		if rng.Float64() < 0.6 {
			return LotTypeCommercial
		}
		return LotTypeResidential

	default:
		// Minor roads: residential or agricultural
		if rng.Float64() < 0.7 {
			return LotTypeResidential
		}
		return LotTypeAgricultural
	}
}

// generateLotFromSite creates a lot polygon around a Voronoi site.
func generateLotFromSite(site VoronoiSite, _ []VoronoiSite, mapW, mapH float64, roads []OrganicRoad, rng *rand.Rand) Lot {
	lotSize := 150 + rng.Float64()*200 // 150-350 pixel lots

	// Default lot dimensions
	w := lotSize * (0.7 + rng.Float64()*0.6) // 0.7-1.3 aspect ratio
	h := lotSize * (0.7 + rng.Float64()*0.6)

	// Center lot around site point
	minX := site.Point.X - w/2
	minY := site.Point.Y - h/2
	maxX := site.Point.X + w/2
	maxY := site.Point.Y + h/2

	// Clamp to map bounds
	minX = math.Max(32, minX)
	minY = math.Max(32, minY)
	maxX = math.Min(mapW-32, maxX)
	maxY = math.Min(mapH-32, maxY)

	// Create rectangular boundary
	boundary := []Point{
		{X: minX, Y: minY},
		{X: maxX, Y: minY},
		{X: maxX, Y: maxY},
		{X: minX, Y: maxY},
	}

	// Calculate area
	area := (maxX - minX) * (maxY - minY)

	// Determine setback based on lot type
	setback := getLotSetback(site.Type)

	// Check if this lot overlaps with any roads and adjust if needed
	for _, road := range roads {
		if !lotOverlapsRoadPath(minX, minY, maxX, maxY, road) {
			continue
		}
		// Reduce lot size if it overlaps with roads
		w *= 0.7 // Make lot 30% smaller to avoid roads
		h *= 0.7
		minX = site.Point.X - w/2
		minY = site.Point.Y - h/2
		maxX = site.Point.X + w/2
		maxY = site.Point.Y + h/2

		// Re-clamp to bounds
		minX = math.Max(64, minX) // Larger margin to avoid roads
		minY = math.Max(64, minY)
		maxX = math.Min(mapW-64, maxX)
		maxY = math.Min(mapH-64, maxY)

		// Recompute boundary and area
		boundary = []Point{{X: minX, Y: minY}, {X: maxX, Y: minY}, {X: maxX, Y: maxY}, {X: minX, Y: maxY}}
		area = (maxX - minX) * (maxY - minY)
	}

	return Lot{
		Boundary:    boundary,
		Type:        site.Type,
		RoadAccess:  false, // Will be calculated later
		AccessPoint: Point{},
		Buildings:   make([]rect, 0),
		Setback:     setback,
		Area:        area,
	}
}

// getLotSetback returns the minimum building setback for different lot types.
func getLotSetback(lotType LotType) float64 {
	switch lotType {
	case LotTypeResidential:
		return 20.0 // 20 pixel setback for houses
	case LotTypeCommercial:
		return 15.0 // Smaller setback for commercial
	case LotTypeIndustrial:
		return 30.0 // Larger setback for industrial
	case LotTypeMilitary:
		return 40.0 // Large setback for security
	case LotTypeAgricultural:
		return 25.0 // Medium setback for farm buildings
	case LotTypeCivic:
		return 25.0 // Medium setback for civic buildings
	case LotTypeRecreational:
		return 50.0 // Large setback for parks
	default:
		return 20.0 // Default setback
	}
}

// findNearestRoadAccess determines if a lot has road access and where.
func findNearestRoadAccess(lot Lot, roads []OrganicRoad) (bool, Point) { //nolint:gocritic
	if len(roads) == 0 {
		return false, Point{}
	}

	minDist := math.Inf(1)
	var accessPoint Point
	hasAccess := false

	// Check distance from lot boundary to all roads
	for _, road := range roads {
		for _, roadPoint := range road.Points {
			for _, boundaryPoint := range lot.Boundary {
				dist := math.Sqrt(
					math.Pow(roadPoint[0]-boundaryPoint.X, 2) +
						math.Pow(roadPoint[1]-boundaryPoint.Y, 2),
				)

				if dist < minDist {
					minDist = dist
					accessPoint = Point{X: roadPoint[0], Y: roadPoint[1]}
				}
			}
		}
	}

	// Consider lot to have road access if within 100 pixels of a road
	if minDist < 100 {
		hasAccess = true
	}

	return hasAccess, accessPoint
}

// buildingCandidatesInLots returns building candidates that fit within lots with setbacks.
func buildingCandidatesInLots(lots []Lot, rng *rand.Rand) []rect {
	candidates := make([]rect, 0, len(lots)*3) // Up to 3 buildings per lot

	for _, lot := range lots {
		// Only generate buildings in lots with road access to prevent isolated buildings
		if !lot.RoadAccess {
			continue
		}

		// Determine how many buildings this lot can support
		buildingCount := getBuildingCountForLot(lot, rng)

		// Generate building candidates within lot boundaries
		for i := 0; i < buildingCount; i++ {
			candidate := generateBuildingInLot(lot, rng)
			if candidate.w > 0 && candidate.h > 0 {
				candidates = append(candidates, candidate)
			}
		}
	}

	return candidates
}

// getBuildingCountForLot determines how many buildings a lot should contain.
func getBuildingCountForLot(lot Lot, rng *rand.Rand) int { //nolint:gocritic
	switch lot.Type {
	case LotTypeResidential:
		// 1 main building, sometimes 1-2 outbuildings
		if lot.Area > 40000 { // Large residential lots
			return 1 + rng.Intn(3) // 1-3 buildings
		}
		return 1

	case LotTypeCommercial:
		// Usually 1 main building
		if lot.Area > 30000 {
			return 1 + rng.Intn(2) // 1-2 buildings
		}
		return 1

	case LotTypeIndustrial:
		// Multiple buildings common
		if lot.Area > 50000 {
			return 2 + rng.Intn(4) // 2-5 buildings
		}
		return 1 + rng.Intn(2) // 1-2 buildings

	case LotTypeMilitary:
		// Multiple structures
		if lot.Area > 35000 {
			return 2 + rng.Intn(3) // 2-4 buildings
		}
		return 1

	case LotTypeAgricultural:
		// Farm complexes with multiple buildings
		if lot.Area > 60000 {
			return 3 + rng.Intn(4) // 3-6 buildings
		}
		return 1 + rng.Intn(2) // 1-2 buildings

	default:
		return 1
	}
}

// generateBuildingInLot places a building within lot boundaries respecting setbacks.
func generateBuildingInLot(lot Lot, rng *rand.Rand) rect { //nolint:gocritic
	if len(lot.Boundary) < 4 {
		return rect{} // Invalid lot
	}

	// For simplified rectangular lots, calculate buildable area
	minX := lot.Boundary[0].X + lot.Setback
	minY := lot.Boundary[0].Y + lot.Setback
	maxX := lot.Boundary[2].X - lot.Setback
	maxY := lot.Boundary[2].Y - lot.Setback

	if maxX <= minX || maxY <= minY {
		return rect{} // No buildable space
	}

	// Generate building size appropriate for lot type
	buildingW, buildingH := getBuildingSizeForLot(lot, rng)

	// Ensure building fits within buildable area
	availableW := maxX - minX
	availableH := maxY - minY

	if float64(buildingW) > availableW {
		buildingW = int(availableW * 0.9) // Leave some margin
	}
	if float64(buildingH) > availableH {
		buildingH = int(availableH * 0.9)
	}

	// Randomly position within buildable area
	buildingX := minX + rng.Float64()*(availableW-float64(buildingW))
	buildingY := minY + rng.Float64()*(availableH-float64(buildingH))

	return rect{
		x: int(buildingX),
		y: int(buildingY),
		w: buildingW,
		h: buildingH,
	}
}

// getBuildingSizeForLot returns appropriate building dimensions for lot type.
func getBuildingSizeForLot(lot Lot, rng *rand.Rand) (int, int) { //nolint:gocritic
	unit := 64 // 64-pixel unit size

	switch lot.Type {
	case LotTypeResidential:
		// Houses: 3×3 to 7×6 units
		w := (3 + rng.Intn(5)) * unit // 3-7 units
		h := (3 + rng.Intn(4)) * unit // 3-6 units
		return w, h

	case LotTypeCommercial:
		// Shops/offices: 4×4 to 8×7 units
		w := (4 + rng.Intn(5)) * unit // 4-8 units
		h := (4 + rng.Intn(4)) * unit // 4-7 units
		return w, h

	case LotTypeIndustrial:
		// Warehouses/factories: 6×6 to 16×12 units
		w := (6 + rng.Intn(11)) * unit // 6-16 units
		h := (6 + rng.Intn(7)) * unit  // 6-12 units
		return w, h

	case LotTypeMilitary:
		// Military buildings: 4×4 to 12×10 units
		w := (4 + rng.Intn(9)) * unit // 4-12 units
		h := (4 + rng.Intn(7)) * unit // 4-10 units
		return w, h

	case LotTypeAgricultural:
		// Barns/silos: 5×5 to 14×10 units
		w := (5 + rng.Intn(10)) * unit // 5-14 units
		h := (5 + rng.Intn(6)) * unit  // 5-10 units
		return w, h

	default:
		// Generic: 4×4 to 8×6 units
		w := (4 + rng.Intn(5)) * unit
		h := (4 + rng.Intn(3)) * unit
		return w, h
	}
}

// generateDriveways creates paths connecting buildings to roads.
func _generateDriveways(lots []Lot, _ []OrganicRoad, _ *TileMap, _ *rand.Rand) []OrganicRoad { //nolint:unused
	driveways := make([]OrganicRoad, 0, len(lots))

	for _, lot := range lots {
		if !lot.RoadAccess || len(lot.Buildings) == 0 {
			continue
		}

		// For each building in the lot, create a driveway to the road access point
		for _, building := range lot.Buildings {
			// Building center
			buildingCenter := Point{
				X: float64(building.x + building.w/2),
				Y: float64(building.y + building.h/2),
			}

			// Create straight driveway to access point
			driveway := OrganicRoad{
				Type: RoadTypeDriveway,
				Points: [][2]float64{
					{buildingCenter.X, buildingCenter.Y},
					{lot.AccessPoint.X, lot.AccessPoint.Y},
				},
				Width:      1, // Single lane
				GroundType: GroundPavement,
				Priority:   10,
				Connected:  make([]int, 0),
			}

			driveways = append(driveways, driveway)
		}
	}

	return driveways
}

// lotOverlapsRoadPath checks if a rectangular lot overlaps with an organic road path.
func lotOverlapsRoadPath(minX, minY, maxX, maxY float64, road OrganicRoad) bool { //nolint:gocritic
	// Check if any road point is within the lot boundaries
	for _, point := range road.Points {
		// Expand road width to account for road thickness
		roadRadius := float64(road.Width * cellSize / 2)

		// Check if road point + radius overlaps with lot rectangle
		if point[0]+roadRadius >= minX && point[0]-roadRadius <= maxX &&
			point[1]+roadRadius >= minY && point[1]-roadRadius <= maxY {
			return true
		}
	}
	return false
}
