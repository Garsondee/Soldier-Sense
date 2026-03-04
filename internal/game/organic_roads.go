package game

import (
	"math"
	"math/rand"
)

// RoadType represents different categories of roads with distinct properties.
type RoadType uint8

const (
	// RoadTypeHighway is a major highway (6-8 lanes).
	RoadTypeHighway RoadType = iota // Major highway (6-8 lanes)
	// RoadTypeMainRoad is a primary road (4-5 lanes).
	RoadTypeMainRoad // Primary road (4-5 lanes)
	// RoadTypeSideStreet is a secondary street (2-3 lanes).
	RoadTypeSideStreet // Secondary street (2-3 lanes)
	// RoadTypeAlley is a narrow alley (1-2 lanes).
	RoadTypeAlley // Narrow alley (1-2 lanes)
	// RoadTypeCulDeSac is a dead-end residential street.
	RoadTypeCulDeSac // Dead-end residential street
	// RoadTypeDirtRoad is an unpaved rural road.
	RoadTypeDirtRoad // Unpaved rural road
	// RoadTypePath is a pedestrian/vehicle path.
	RoadTypePath // Pedestrian/vehicle path
	// RoadTypeDriveway is a building access path.
	RoadTypeDriveway // Building access path
)

// RoadProperties defines the characteristics of each road type.
type RoadProperties struct {
	Width      int        // Width in tiles
	GroundType GroundType // Surface material
	Speed      float64    // Movement speed multiplier
	Curved     bool       // Allows curved segments
	Priority   int        // Hierarchy priority (higher = major road)
	MinLength  int        // Minimum length in tiles
	MaxLength  int        // Maximum length in tiles
}

// getRoadProperties returns the properties for a given road type.
func getRoadProperties(roadType RoadType) RoadProperties {
	switch roadType {
	case RoadTypeHighway:
		return RoadProperties{
			Width:      7, // 6-8 lanes
			GroundType: GroundTarmac,
			Speed:      1.1, // Faster movement
			Curved:     true,
			Priority:   100,
			MinLength:  200,
			MaxLength:  400,
		}
	case RoadTypeMainRoad:
		return RoadProperties{
			Width:      5, // 4-5 lanes
			GroundType: GroundTarmac,
			Speed:      1.0,
			Curved:     true,
			Priority:   80,
			MinLength:  150,
			MaxLength:  350,
		}
	case RoadTypeSideStreet:
		return RoadProperties{
			Width:      3, // 2-3 lanes
			GroundType: GroundTarmac,
			Speed:      1.0,
			Curved:     true,
			Priority:   60,
			MinLength:  80,
			MaxLength:  200,
		}
	case RoadTypeAlley:
		return RoadProperties{
			Width:      1, // 1-2 lanes
			GroundType: GroundTarmac,
			Speed:      0.9,   // Slightly slower
			Curved:     false, // Alleys tend to be straight
			Priority:   40,
			MinLength:  30,
			MaxLength:  100,
		}
	case RoadTypeCulDeSac:
		return RoadProperties{
			Width:      3,
			GroundType: GroundTarmac,
			Speed:      1.0,
			Curved:     true, // Curves at the end
			Priority:   50,
			MinLength:  40,
			MaxLength:  80,
		}
	case RoadTypeDirtRoad:
		return RoadProperties{
			Width:      2,
			GroundType: GroundDirt,
			Speed:      0.8, // Slower on dirt
			Curved:     true,
			Priority:   30,
			MinLength:  60,
			MaxLength:  180,
		}
	case RoadTypePath:
		return RoadProperties{
			Width:      1,
			GroundType: GroundDirt,
			Speed:      0.95,
			Curved:     true,
			Priority:   20,
			MinLength:  20,
			MaxLength:  60,
		}
	case RoadTypeDriveway:
		return RoadProperties{
			Width:      1,
			GroundType: GroundPavement,
			Speed:      1.0,
			Curved:     false, // Simple straight driveways
			Priority:   10,
			MinLength:  10,
			MaxLength:  30,
		}
	default:
		return RoadProperties{} // Default empty properties
	}
}

// OrganicRoad represents a curved road with hierarchy and connections.
type OrganicRoad struct {
	Type       RoadType     // Type of road
	Points     [][2]float64 // Control points for curved path (world coordinates)
	Width      int          // Width in tiles
	GroundType GroundType   // Surface material
	Priority   int          // Hierarchy level
	Connected  []int        // Indices of connected roads
}

// Point represents a 2D coordinate.
type Point struct {
	X, Y float64
}

// generateOrganicRoadNetwork creates a realistic road network with hierarchy and curves.
func generateOrganicRoadNetwork(tm *TileMap, rng *rand.Rand) []OrganicRoad {
	roads := make([]OrganicRoad, 0, 20)

	mapW := float64(tm.Cols * cellSize)
	mapH := float64(tm.Rows * cellSize)

	// Generate primary roads first (highways and main roads)
	roads = append(roads, generatePrimaryRoads(mapW, mapH, rng)...)

	// Generate secondary roads connecting to primary roads
	roads = append(roads, generateSecondaryRoads(roads, mapW, mapH, rng)...)

	// Generate tertiary roads (alleys, paths) connecting to secondary roads
	roads = append(roads, generateTertiaryRoads(roads, mapW, mapH, rng)...)

	// Stamp all roads into the tile map
	for _, road := range roads {
		stampOrganicRoad(tm, road, rng)
	}

	// Stamp intersections where roads meet.
	intersections := generateIntersections(roads, rng)
	for _, intersection := range intersections {
		stampIntersection(tm, intersection, rng)
	}

	return roads
}

// generatePrimaryRoads creates 1-2 major roads traversing the map.
func generatePrimaryRoads(mapW, mapH float64, rng *rand.Rand) []OrganicRoad {
	roads := make([]OrganicRoad, 0, 2)

	// Generate 1-2 primary roads
	roadCount := 1 + rng.Intn(2)

	for i := 0; i < roadCount; i++ {
		var road OrganicRoad

		if i == 0 || rng.Float64() < 0.6 {
			// First road or 60% chance: horizontal main road
			road.Type = RoadTypeMainRoad
			road = generateCurvedRoad(
				Point{0, mapH * (0.3 + rng.Float64()*0.4)},    // Start somewhere in middle-left
				Point{mapW, mapH * (0.3 + rng.Float64()*0.4)}, // End somewhere in middle-right
				getRoadProperties(RoadTypeMainRoad),
				RoadTypeMainRoad,
				rng,
			)
		} else {
			// Second road: vertical main road
			road.Type = RoadTypeMainRoad
			road = generateCurvedRoad(
				Point{mapW * (0.3 + rng.Float64()*0.4), 0},    // Start somewhere in middle-top
				Point{mapW * (0.3 + rng.Float64()*0.4), mapH}, // End somewhere in middle-bottom
				getRoadProperties(RoadTypeMainRoad),
				RoadTypeMainRoad,
				rng,
			)
		}

		roads = append(roads, road)
	}

	return roads
}

// generateSecondaryRoads creates side streets branching from primary roads.
func generateSecondaryRoads(primaryRoads []OrganicRoad, mapW, mapH float64, rng *rand.Rand) []OrganicRoad {
	roads := make([]OrganicRoad, 0, 8)

	// Generate 4-8 secondary roads
	roadCount := 4 + rng.Intn(5)

	for i := 0; i < roadCount; i++ {
		if len(primaryRoads) == 0 {
			break
		}

		// Choose a random primary road to branch from
		parentRoad := primaryRoads[rng.Intn(len(primaryRoads))]

		// Choose a branch point along the parent road
		if len(parentRoad.Points) < 2 {
			continue
		}
		branchIdx := 1 + rng.Intn(len(parentRoad.Points)-2)
		branchPoint := parentRoad.Points[branchIdx]

		// Generate perpendicular branch
		props := getRoadProperties(RoadTypeSideStreet)

		// Create endpoint for the branch
		length := float64(props.MinLength + rng.Intn(props.MaxLength-props.MinLength))

		// Choose direction perpendicular to parent road direction
		var endPoint Point
		if branchIdx > 0 {
			parentDir := math.Atan2(
				parentRoad.Points[branchIdx][1]-parentRoad.Points[branchIdx-1][1],
				parentRoad.Points[branchIdx][0]-parentRoad.Points[branchIdx-1][0],
			)
			// Perpendicular direction (±90°)
			perpDir := parentDir + math.Pi/2
			if rng.Intn(2) == 0 {
				perpDir = parentDir - math.Pi/2
			}

			endPoint = Point{
				X: branchPoint[0] + length*math.Cos(perpDir),
				Y: branchPoint[1] + length*math.Sin(perpDir),
			}

			// Clamp to map bounds
			endPoint.X = math.Max(32, math.Min(mapW-32, endPoint.X))
			endPoint.Y = math.Max(32, math.Min(mapH-32, endPoint.Y))
		}

		road := generateCurvedRoad(
			Point{branchPoint[0], branchPoint[1]},
			endPoint,
			props,
			RoadTypeSideStreet,
			rng,
		)
		road.Type = RoadTypeSideStreet
		roads = append(roads, road)
	}

	return roads
}

// generateTertiaryRoads creates alleys and paths for detailed connectivity.
func generateTertiaryRoads(_ []OrganicRoad, mapW, mapH float64, rng *rand.Rand) []OrganicRoad {
	roads := make([]OrganicRoad, 0, 12)

	// Generate 8-12 tertiary roads (alleys, paths)
	roadCount := 8 + rng.Intn(5)

	for i := 0; i < roadCount; i++ {
		var roadType RoadType
		var props RoadProperties

		// Choose road type
		typeRoll := rng.Float64()
		switch {
		case typeRoll < 0.4:
			roadType = RoadTypeAlley
			props = getRoadProperties(RoadTypeAlley)
		case typeRoll < 0.7:
			roadType = RoadTypePath
			props = getRoadProperties(RoadTypePath)
		case typeRoll < 0.85:
			roadType = RoadTypeCulDeSac
			props = getRoadProperties(RoadTypeCulDeSac)
		default:
			roadType = RoadTypeDirtRoad
			props = getRoadProperties(RoadTypeDirtRoad)
		}

		// Generate random endpoints within the map
		start := Point{
			X: 64 + rng.Float64()*(mapW-128),
			Y: 64 + rng.Float64()*(mapH-128),
		}

		length := float64(props.MinLength + rng.Intn(props.MaxLength-props.MinLength))
		angle := rng.Float64() * 2 * math.Pi

		end := Point{
			X: start.X + length*math.Cos(angle),
			Y: start.Y + length*math.Sin(angle),
		}

		// Clamp to bounds
		end.X = math.Max(32, math.Min(mapW-32, end.X))
		end.Y = math.Max(32, math.Min(mapH-32, end.Y))

		road := generateCurvedRoad(start, end, props, roadType, rng)
		road.Type = roadType
		roads = append(roads, road)
	}

	return roads
}

// generateCurvedRoad creates a curved road between two points using Bezier curves.
func generateCurvedRoad(start, end Point, props RoadProperties, roadType RoadType, rng *rand.Rand) OrganicRoad {
	points := make([][2]float64, 0, 20)

	if !props.Curved {
		// Simple straight road
		points = append(points, [2]float64{start.X, start.Y}, [2]float64{end.X, end.Y})
	} else {
		// Generate curved road using control points
		// Add some randomness to the curve
		midX := (start.X + end.X) / 2
		midY := (start.Y + end.Y) / 2

		// Add curve control points with random deviation
		deviation := 50 + rng.Float64()*100 // 50-150 pixel curve deviation

		// Perpendicular offset for curve
		dx := end.X - start.X
		dy := end.Y - start.Y
		length := math.Sqrt(dx*dx + dy*dy)
		if length > 0 {
			// Normalized perpendicular vector
			perpX := -dy / length
			perpY := dx / length

			// Apply random curve
			curveDir := 1.0
			if rng.Intn(2) == 0 {
				curveDir = -1.0
			}

			midX += curveDir * perpX * deviation
			midY += curveDir * perpY * deviation
		}

		// Generate points along the curve
		segments := 10 + int(length/100) // More segments for longer roads
		for i := 0; i <= segments; i++ {
			t := float64(i) / float64(segments)

			// Quadratic Bezier curve: P = (1-t)²P₀ + 2(1-t)tP₁ + t²P₂
			x := (1-t)*(1-t)*start.X + 2*(1-t)*t*midX + t*t*end.X
			y := (1-t)*(1-t)*start.Y + 2*(1-t)*t*midY + t*t*end.Y

			points = append(points, [2]float64{x, y})
		}
	}

	return OrganicRoad{
		Type:       roadType,
		Points:     points,
		Width:      props.Width,
		GroundType: props.GroundType,
		Priority:   props.Priority,
		Connected:  make([]int, 0),
	}
}

// Intersection represents a road junction with specific geometry.
type Intersection struct {
	Center         Point            // Center point of intersection
	Type           IntersectionType // Type of intersection
	ConnectedRoads []int            // Indices of roads that connect here
	Radius         float64          // Size of intersection area
}

// IntersectionType represents different intersection geometries.
type IntersectionType uint8

const (
	// IntersectionTJunction is a T-shaped intersection.
	IntersectionTJunction IntersectionType = iota // T-shaped intersection
	// IntersectionCrossroads is a 4-way cross intersection.
	IntersectionCrossroads // 4-way cross intersection
	// IntersectionRoundabout is a circular roundabout intersection.
	IntersectionRoundabout // Circular roundabout
	// IntersectionYJunction is a Y-shaped intersection.
	IntersectionYJunction // Y-shaped intersection
)

// generateIntersections creates realistic intersections where roads meet.
func generateIntersections(roads []OrganicRoad, rng *rand.Rand) []Intersection {
	intersections := make([]Intersection, 0, 8)

	// Find road intersection points
	for i := 0; i < len(roads); i++ {
		for j := i + 1; j < len(roads); j++ {
			intersection := findRoadIntersection(&roads[i], &roads[j], i, j, rng)
			if intersection.Radius > 0 {
				intersections = append(intersections, intersection)
			}
		}
	}

	return intersections
}

// findRoadIntersection determines if two roads intersect and creates intersection geometry.
func findRoadIntersection(road1, road2 *OrganicRoad, idx1, idx2 int, rng *rand.Rand) Intersection {
	minDist := 50.0 // Minimum distance to consider an intersection
	var bestIntersection Intersection

	// Check all point pairs between the two roads for proximity
	for _, p1 := range road1.Points {
		for _, p2 := range road2.Points {
			dist := math.Sqrt(
				math.Pow(p1[0]-p2[0], 2) +
					math.Pow(p1[1]-p2[1], 2),
			)

			if dist < minDist {
				// Found intersection point
				center := Point{
					X: (p1[0] + p2[0]) / 2,
					Y: (p1[1] + p2[1]) / 2,
				}

				// Determine intersection type based on road priorities
				intersectionType := determineIntersectionType(road1, road2, rng)
				radius := getIntersectionRadius(intersectionType, road1, road2)

				bestIntersection = Intersection{
					Center:         center,
					Type:           intersectionType,
					ConnectedRoads: []int{idx1, idx2},
					Radius:         radius,
				}
				minDist = dist
			}
		}
	}

	return bestIntersection
}

// determineIntersectionType chooses appropriate intersection geometry.
func determineIntersectionType(road1, road2 *OrganicRoad, rng *rand.Rand) IntersectionType {
	// Higher priority roads get more complex intersections
	totalPriority := road1.Priority + road2.Priority

	switch {
	case totalPriority >= 160: // Two major roads
		if rng.Float64() < 0.3 {
			return IntersectionRoundabout // 30% chance for roundabout
		}
		return IntersectionCrossroads
	case totalPriority >= 100: // Major + minor road
		if rng.Float64() < 0.1 {
			return IntersectionRoundabout // 10% chance for roundabout
		}
		return IntersectionTJunction
	default:
		// Minor roads - simple intersections.
		if rng.Float64() < 0.7 {
			return IntersectionTJunction
		}
		return IntersectionYJunction
	}
}

// getIntersectionRadius returns the size of intersection area based on type and road widths.
func getIntersectionRadius(intersectionType IntersectionType, road1, road2 *OrganicRoad) float64 {
	avgWidth := float64(road1.Width+road2.Width) / 2

	switch intersectionType {
	case IntersectionRoundabout:
		return avgWidth * 2.5 // Large roundabouts
	case IntersectionCrossroads:
		return avgWidth * 1.5 // Medium crossroads
	case IntersectionTJunction:
		return avgWidth * 1.2 // Small T-junctions
	case IntersectionYJunction:
		return avgWidth * 1.0 // Minimal Y-junctions
	default:
		return avgWidth
	}
}

// stampIntersection renders intersection geometry into the tile map.
func stampIntersection(tm *TileMap, intersection Intersection, rng *rand.Rand) {
	centerCol := int(intersection.Center.X) / cellSize
	centerRow := int(intersection.Center.Y) / cellSize
	radius := int(intersection.Radius) / cellSize

	switch intersection.Type {
	case IntersectionRoundabout:
		// Create circular roundabout
		for dc := -radius; dc <= radius; dc++ {
			for dr := -radius; dr <= radius; dr++ {
				col := centerCol + dc
				row := centerRow + dr

				if !tm.inBounds(col, row) {
					continue
				}
				dist := math.Sqrt(float64(dc*dc + dr*dr))

				switch {
				case dist <= float64(radius)*0.4:
					// Inner circle - grass island.
					tm.SetGround(col, row, GroundGrass)
				case dist <= float64(radius)*0.8:
					// Driving surface.
					tm.SetGround(col, row, GroundTarmac)
				case dist <= float64(radius):
					// Outer edge - pavement.
					tm.SetGround(col, row, GroundPavement)
				}
			}
		}

	case IntersectionCrossroads:
		// Create square intersection
		for dc := -radius; dc <= radius; dc++ {
			for dr := -radius; dr <= radius; dr++ {
				col := centerCol + dc
				row := centerRow + dr

				if tm.inBounds(col, row) {
					tm.SetGround(col, row, GroundTarmac)
				}
			}
		}

	case IntersectionTJunction, IntersectionYJunction:
		// Create smaller junction
		smallRadius := radius * 2 / 3
		for dc := -smallRadius; dc <= smallRadius; dc++ {
			for dr := -smallRadius; dr <= smallRadius; dr++ {
				col := centerCol + dc
				row := centerRow + dr

				if tm.inBounds(col, row) {
					tm.SetGround(col, row, GroundTarmac)
				}
			}
		}
	}
}

// stampOrganicRoad renders a curved road into the tile map with continuous coverage.
func stampOrganicRoad(tm *TileMap, road OrganicRoad, rng *rand.Rand) {
	hw := road.Width / 2

	// Interpolate between road points to ensure continuous coverage
	for i := 0; i < len(road.Points)-1; i++ {
		point1 := road.Points[i]
		point2 := road.Points[i+1]

		// Calculate distance between points
		dx := point2[0] - point1[0]
		dy := point2[1] - point1[1]
		distance := math.Sqrt(dx*dx + dy*dy)

		// Interpolate every 4 pixels for continuous road coverage
		steps := int(distance/4) + 1
		if steps < 2 {
			steps = 2
		}

		for step := 0; step <= steps; step++ {
			t := float64(step) / float64(steps)

			// Linear interpolation between points
			interpX := point1[0] + t*dx
			interpY := point1[1] + t*dy

			centerCol := int(interpX) / cellSize
			centerRow := int(interpY) / cellSize

			// Stamp road tiles in a square pattern around each interpolated point
			for dc := -hw; dc <= hw; dc++ {
				for dr := -hw; dr <= hw; dr++ {
					col := centerCol + dc
					row := centerRow + dr

					if tm.inBounds(col, row) {
						// Distance from center for smooth road edges
						dist := math.Sqrt(float64(dc*dc + dr*dr))

						if dist <= float64(hw) {
							// Core road area
							tm.SetGround(col, row, road.GroundType)
						} else if dist <= float64(hw)+0.5 {
							// Road edge - add pavement if not already road
							if tm.Ground(col, row) != road.GroundType && tm.Ground(col, row) != GroundPavement {
								tm.SetGround(col, row, GroundPavement)
								tm.AddFlag(col, row, TileFlagRoadEdge)
							}
						}
					}
				}
			}
		}
	}
}
