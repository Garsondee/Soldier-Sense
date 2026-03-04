package game

import "math/rand"

// furnishBuilding populates interior rooms with furniture, pillars, and doors.
// Called after addBuildingWalls has created perimeter walls and BSP partitions.
// It stamps objects directly into the TileMap.
func furnishBuilding(tm *TileMap, rng *rand.Rand, fp rect, rooms []interiorRoom) {
	if tm == nil {
		return
	}

	// Assign a random floor type per building.
	floorType := GroundConcrete
	roll := rng.Float64()
	if roll < 0.25 {
		floorType = GroundTile
	} else if roll < 0.45 {
		floorType = GroundWood
	}

	// Stamp building floor type (override the default concrete from initTileMap).
	cMin := fp.x / cellSize
	rMin := fp.y / cellSize
	cMax := (fp.x + fp.w - 1) / cellSize
	rMax := (fp.y + fp.h - 1) / cellSize
	for r := rMin; r <= rMax; r++ {
		for c := cMin; c <= cMax; c++ {
			if tm.inBounds(c, r) && tm.Tiles[r*tm.Cols+c].Flags&TileFlagIndoor != 0 {
				// Don't overwrite walls/windows.
				if tm.ObjectAt(c, r) == ObjectNone {
					tm.SetGround(c, r, floorType)
				}
			}
		}
	}

	for _, rm := range rooms {
		furnishRoom(tm, rng, rm)
	}
}

// RoomType represents the function of a room for furniture placement.
type RoomType uint8

const (
	// RoomTypeGeneric is the default or unknown room type.
	RoomTypeGeneric RoomType = iota // Default/unknown room
	// RoomTypeLiving is a living room.
	RoomTypeLiving // Living room (tables, chairs, sofa)
	// RoomTypeKitchen is a kitchen.
	RoomTypeKitchen // Kitchen (counters, refrigerator)
	// RoomTypeBedroom is a bedroom.
	RoomTypeBedroom // Bedroom (bed, dresser)
	// RoomTypeOffice is an office.
	RoomTypeOffice // Office (desk, bookshelf, filing)
	// RoomTypeStorage is a storage room.
	RoomTypeStorage // Storage (crates, shelving)
	// RoomTypeWorkshop is a workshop.
	RoomTypeWorkshop // Workshop (workbench, machinery, tools)
	// RoomTypeBreakroom is a breakroom.
	RoomTypeBreakroom // Break room (tables, chairs, appliances)
)

// interiorRoom represents a rectangular room within a building.
type interiorRoom struct {
	rx, ry, rw, rh int  // pixel coordinates
	hasDoorway     bool // true if this room has an identified doorway gap
	doorX, doorY   int  // pixel position of the doorway (if hasDoorway)
}

// classifyRoomType determines the room type based on size, building type, and context.
func classifyRoomTypeForBuilding(rm interiorRoom, roomIndex, totalRooms int, buildingType BuildingType, rng *rand.Rand) RoomType {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize
	area := rmCols * rmRows

	// Building type influences room classification
	switch buildingType {
	case BuildingTypeResidential:
		return classifyResidentialRoom(area, roomIndex, totalRooms, rng)
	case BuildingTypeCommercial:
		return classifyCommercialRoom(area, roomIndex, totalRooms, rng)
	case BuildingTypeIndustrial:
		return classifyIndustrialRoom(area, roomIndex, totalRooms, rng)
	case BuildingTypeMilitary:
		return classifyMilitaryRoom(area, roomIndex, totalRooms, rng)
	case BuildingTypeAgricultural:
		return classifyAgriculturalRoom(area, roomIndex, totalRooms, rng)
	default:
		return classifyGenericRoom(area, roomIndex, totalRooms, rng)
	}
}

// classifyResidentialRoom classifies rooms in residential buildings.
func classifyResidentialRoom(area, _, _ int, rng *rand.Rand) RoomType {
	// Very small rooms are likely storage/bathrooms
	if area <= 6 {
		return RoomTypeStorage
	}

	// Small rooms (7-12 tiles) - bedrooms or home office
	if area <= 12 {
		if rng.Float64() < 0.75 {
			return RoomTypeBedroom
		}
		return RoomTypeOffice
	}

	// Medium rooms (13-20 tiles) - living areas or bedrooms
	if area <= 20 {
		roomTypeRoll := rng.Float64()
		switch {
		case roomTypeRoll < 0.4:
			return RoomTypeLiving
		case roomTypeRoll < 0.7:
			return RoomTypeBedroom
		case roomTypeRoll < 0.85:
			return RoomTypeKitchen
		default:
			return RoomTypeOffice
		}
	}

	// Large rooms (21+ tiles) - main living areas
	if rng.Float64() < 0.6 {
		return RoomTypeLiving
	}
	return RoomTypeKitchen
}

// classifyCommercialRoom classifies rooms in commercial buildings.
func classifyCommercialRoom(area, _, _ int, rng *rand.Rand) RoomType {
	// Small rooms - offices or storage
	if area <= 12 {
		if rng.Float64() < 0.7 {
			return RoomTypeOffice
		}
		return RoomTypeStorage
	}

	// Medium to large rooms - offices or break areas
	roomTypeRoll := rng.Float64()
	switch {
	case roomTypeRoll < 0.6:
		return RoomTypeOffice
	case roomTypeRoll < 0.8:
		return RoomTypeBreakroom
	default:
		return RoomTypeStorage
	}
}

// classifyIndustrialRoom classifies rooms in industrial buildings.
func classifyIndustrialRoom(area, _, _ int, rng *rand.Rand) RoomType {
	// Small rooms - offices or storage
	if area <= 12 {
		if rng.Float64() < 0.5 {
			return RoomTypeStorage
		}
		return RoomTypeOffice
	}

	// Large rooms - workshops or storage
	roomTypeRoll := rng.Float64()
	switch {
	case roomTypeRoll < 0.5:
		return RoomTypeWorkshop
	case roomTypeRoll < 0.8:
		return RoomTypeStorage
	case roomTypeRoll < 0.9:
		return RoomTypeBreakroom
	default:
		return RoomTypeOffice
	}
}

// classifyMilitaryRoom classifies rooms in military buildings.
func classifyMilitaryRoom(area, _, _ int, rng *rand.Rand) RoomType {
	// Military buildings have specialized rooms
	if area <= 8 {
		return RoomTypeStorage // Small storage/equipment rooms
	}

	// Medium to large rooms
	roomTypeRoll := rng.Float64()
	switch {
	case roomTypeRoll < 0.4:
		return RoomTypeOffice // Command/admin
	case roomTypeRoll < 0.6:
		return RoomTypeBreakroom // Mess hall/break area
	case roomTypeRoll < 0.8:
		return RoomTypeStorage // Equipment storage
	default:
		return RoomTypeBedroom // Barracks sleeping
	}
}

// classifyAgriculturalRoom classifies rooms in agricultural buildings.
func classifyAgriculturalRoom(area, _, _ int, rng *rand.Rand) RoomType {
	// Agricultural buildings are mostly storage and work areas
	if area <= 10 {
		return RoomTypeStorage
	}

	// Larger rooms
	if rng.Float64() < 0.8 {
		return RoomTypeStorage // Grain storage, equipment
	}
	return RoomTypeWorkshop // Repair/maintenance area
}

// classifyGenericRoom is the fallback room classification (old logic).
func classifyGenericRoom(area, _, totalRooms int, rng *rand.Rand) RoomType {
	// Very small rooms are likely storage
	if area <= 6 {
		return RoomTypeStorage
	}

	// Small rooms (7-12 tiles)
	if area <= 12 {
		if rng.Float64() < 0.6 {
			return RoomTypeBedroom
		}
		return RoomTypeOffice
	}

	// Medium rooms (13-20 tiles)
	if area <= 20 {
		roomTypeRoll := rng.Float64()
		switch {
		case roomTypeRoll < 0.3:
			return RoomTypeLiving
		case roomTypeRoll < 0.5:
			return RoomTypeOffice
		case roomTypeRoll < 0.7:
			return RoomTypeWorkshop
		case roomTypeRoll < 0.85:
			return RoomTypeBedroom
		default:
			return RoomTypeKitchen
		}
	}

	// Large rooms (21+ tiles)
	if totalRooms <= 3 {
		if rng.Float64() < 0.4 {
			return RoomTypeLiving
		}
		return RoomTypeWorkshop
	}

	roomTypeRoll := rng.Float64()
	switch {
	case roomTypeRoll < 0.25:
		return RoomTypeLiving
	case roomTypeRoll < 0.45:
		return RoomTypeWorkshop
	case roomTypeRoll < 0.6:
		return RoomTypeOffice
	case roomTypeRoll < 0.75:
		return RoomTypeBreakroom
	default:
		return RoomTypeStorage
	}
}

// furnishRoom places furniture and features inside a single room.
func furnishRoom(tm *TileMap, rng *rand.Rand, rm interiorRoom) { //nolint:gocognit,gocyclo
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize
	if rmCols < 2 || rmRows < 2 {
		return
	}

	// --- Doors in doorways ---
	// Place a closed door in the doorway if identified.
	if rm.hasDoorway {
		dc := rm.doorX / cellSize
		dr := rm.doorY / cellSize
		if tm.inBounds(dc, dr) && tm.ObjectAt(dc, dr) == ObjectNone {
			doorOpen := rng.Float64() < 0.25 // 25% chance open
			if doorOpen {
				tm.SetObject(dc, dr, ObjectDoorOpen)
			} else {
				tm.SetObject(dc, dr, ObjectDoor)
			}
		}
	}

	// --- Classify room type for context-aware furniture placement ---
	// Note: Room classification would ideally happen during BSP generation
	// For now, classify based on size and randomness with generic building type
	roomType := classifyRoomTypeForBuilding(rm, 0, 1, BuildingTypeGeneric, rng) // TODO: Pass actual building type, room index and count

	// --- Pillars (only in large rooms) ---
	// Large rooms (≥5×5 tiles) get a pillar near center.
	if rmCols >= 5 && rmRows >= 5 && rng.Float64() < 0.4 {
		pc := rm.rx/cellSize + rmCols/2
		pr := rm.ry/cellSize + rmRows/2
		if tm.inBounds(pc, pr) && tm.ObjectAt(pc, pr) == ObjectNone {
			tm.SetObject(pc, pr, ObjectPillar)
		}
	}

	// --- Room-type-specific furniture placement ---
	switch roomType {
	case RoomTypeLiving:
		placeLivingRoomFurniture(tm, rng, rm)
	case RoomTypeKitchen:
		placeKitchenFurniture(tm, rng, rm)
	case RoomTypeBedroom:
		placeBedroomFurniture(tm, rng, rm)
	case RoomTypeOffice:
		placeOfficeFurniture(tm, rng, rm)
	case RoomTypeStorage:
		placeStorageFurniture(tm, rng, rm)
	case RoomTypeWorkshop:
		placeWorkshopFurniture(tm, rng, rm)
	case RoomTypeBreakroom:
		placeBreakroomFurniture(tm, rng, rm)
	default: // RoomTypeGeneric
		// Fallback to old random placement
		if rng.Float64() < 0.70 && rmCols >= 3 && rmRows >= 3 {
			placeTableCluster(tm, rng, rm)
		}
		if rng.Float64() < 0.45 && rmCols >= 3 && rmRows >= 3 {
			placeCrates(tm, rng, rm)
		}
		if rng.Float64() < 0.35 && rmCols >= 4 && rmRows >= 4 {
			placeScatteredFurniture(tm, rng, rm)
		}
		if rng.Float64() < 0.25 && rmCols >= 3 && rmRows >= 3 {
			placeWallFurniture(tm, rng, rm)
		}
	}
}

// placeTableCluster places 1-2 table tiles with adjacent chairs.
func placeTableCluster(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	// Pick a position away from walls (at least 1 tile inset).
	startCol := rm.rx/cellSize + 1
	startRow := rm.ry/cellSize + 1
	endCol := (rm.rx+rm.rw)/cellSize - 1
	endRow := (rm.ry+rm.rh)/cellSize - 1
	if endCol <= startCol || endRow <= startRow {
		return
	}

	tc := startCol + rng.Intn(endCol-startCol)
	tr := startRow + rng.Intn(endRow-startRow)

	if !tm.inBounds(tc, tr) || tm.ObjectAt(tc, tr) != ObjectNone {
		return
	}
	tm.SetObject(tc, tr, ObjectTable)

	// Second table tile (horizontal or vertical extension).
	if rng.Float64() < 0.5 {
		tc2, tr2 := tc+1, tr
		if rng.Intn(2) == 0 {
			tc2, tr2 = tc, tr+1
		}
		if tm.inBounds(tc2, tr2) && tm.ObjectAt(tc2, tr2) == ObjectNone {
			tm.SetObject(tc2, tr2, ObjectTable)
		}
	}

	// Place chairs around the table.
	chairOffsets := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	rng.Shuffle(len(chairOffsets), func(i, j int) {
		chairOffsets[i], chairOffsets[j] = chairOffsets[j], chairOffsets[i]
	})
	chairCount := 1 + rng.Intn(3) // 1-3 chairs
	placed := 0
	for _, off := range chairOffsets {
		if placed >= chairCount {
			break
		}
		cc, cr := tc+off[0], tr+off[1]
		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			tm.SetObject(cc, cr, ObjectChair)
			placed++
		}
	}
}

// placeCrates places 1-3 crate tiles along a room wall.
func placeCrates(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	// Choose a wall to place crates against.
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	count := 1 + rng.Intn(3) // 1-3 crates
	side := rng.Intn(4)

	for i := 0; i < count; i++ {
		var cc, cr int
		switch side {
		case 0: // north wall
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = startRow
		case 1: // south wall
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = endRow
		case 2: // west wall
			cc = startCol
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		default: // east wall
			cc = endCol
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		}
		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			tm.SetObject(cc, cr, ObjectCrate)
		}
	}
}

// placeDoorInDoorway stamps a door object into the TileMap at a doorway gap.
// Called during building generation when a doorway gap is created.
func placeDoorInDoorway(tm *TileMap, rng *rand.Rand, doorX, doorY, unit int, isExterior bool) {
	if tm == nil {
		return
	}
	// Place a door in the middle of the doorway gap (unit-wide).
	// Use the center cell of the gap.
	midX := doorX + unit/2
	midY := doorY + unit/2
	dc := midX / cellSize
	dr := midY / cellSize

	if !tm.inBounds(dc, dr) {
		return
	}
	if tm.ObjectAt(dc, dr) != ObjectNone {
		return
	}

	if isExterior {
		// Exterior doors: 70% closed, 30% open.
		if rng.Float64() < 0.30 {
			tm.SetObject(dc, dr, ObjectDoorOpen)
		} else {
			tm.SetObject(dc, dr, ObjectDoor)
		}
	} else {
		// Interior doors: 80% closed, 20% open.
		if rng.Float64() < 0.20 {
			tm.SetObject(dc, dr, ObjectDoorOpen)
		} else {
			tm.SetObject(dc, dr, ObjectDoor)
		}
	}
}

// placeScatteredFurniture adds random furniture scattered throughout the room.
func placeScatteredFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	startCol := rm.rx/cellSize + 1
	startRow := rm.ry/cellSize + 1
	endCol := (rm.rx+rm.rw)/cellSize - 1
	endRow := (rm.ry+rm.rh)/cellSize - 1

	if endCol <= startCol || endRow <= startRow {
		return
	}

	// Place 1-3 random furniture pieces
	count := 1 + rng.Intn(3)
	for i := 0; i < count; i++ {
		for attempts := 0; attempts < 10; attempts++ {
			cc := startCol + rng.Intn(endCol-startCol)
			cr := startRow + rng.Intn(endRow-startRow)

			if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
				// Choose random furniture type from new variety
				furnitureType := rng.Intn(8)
				switch furnitureType {
				case 0:
					tm.SetObject(cc, cr, ObjectTable) // Standard table
				case 1:
					tm.SetObject(cc, cr, ObjectDesk) // Desk/workstation
				case 2:
					tm.SetObject(cc, cr, ObjectChair) // Single chair
				case 3:
					tm.SetObject(cc, cr, ObjectCrate) // Storage box
				case 4:
					tm.SetObject(cc, cr, ObjectBarrel) // Industrial barrel
				case 5:
					tm.SetObject(cc, cr, ObjectBookshelf) // Bookshelf
				case 6:
					tm.SetObject(cc, cr, ObjectCounter) // Counter/cabinet
				case 7:
					tm.SetObject(cc, cr, ObjectShelving) // Metal shelving
				}
				break
			}
		}
	}
}

// placeWallFurniture adds furniture along room walls.
func placeWallFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) { //nolint:gocognit,gocyclo
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	// Choose a wall to place furniture against
	side := rng.Intn(4)
	count := 1 + rng.Intn(2) // 1-2 items

	for i := 0; i < count; i++ {
		var cc, cr int
		switch side {
		case 0: // north wall (interior side)
			if startRow+1 >= endRow {
				continue
			}
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = startRow + 1
		case 1: // south wall (interior side)
			if endRow-1 <= startRow {
				continue
			}
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = endRow - 1
		case 2: // west wall (interior side)
			if startCol+1 >= endCol {
				continue
			}
			cc = startCol + 1
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		default: // east wall (interior side)
			if endCol-1 <= startCol {
				continue
			}
			cc = endCol - 1
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		}

		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			// Wall furniture is typically storage/shelving - choose appropriate type
			wallFurnitureType := rng.Intn(5)
			switch wallFurnitureType {
			case 0:
				tm.SetObject(cc, cr, ObjectBookshelf) // Bookshelf against wall
			case 1:
				tm.SetObject(cc, cr, ObjectShelving) // Metal shelving unit
			case 2:
				tm.SetObject(cc, cr, ObjectCounter) // Counter/cabinet
			case 3:
				tm.SetObject(cc, cr, ObjectRefrigerator) // Large appliance
			case 4:
				tm.SetObject(cc, cr, ObjectCrate) // Storage crate
			}
		}
	}
}

// placeLivingRoomFurniture creates a living room with sofa, coffee table, and seating.
func placeLivingRoomFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	// Living rooms need space for furniture
	if rmCols < 3 || rmRows < 3 {
		return
	}

	// Place sofa (2×1) against a wall
	if rmCols >= 4 && rmRows >= 3 {
		placeSofaAgainstWall(tm, rng, rm)
	}

	// Place coffee table in center area
	if rmCols >= 4 && rmRows >= 4 {
		placeTableCluster(tm, rng, rm)
	}

	// Add 2-4 chairs around the seating area
	chairCount := 2 + rng.Intn(3)
	placeChairsInRoom(tm, rng, rm, chairCount)
}

// placeKitchenFurniture creates a kitchen with counters, appliances, and storage.
func placeKitchenFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	if rmCols < 2 || rmRows < 2 {
		return
	}

	// Place counters along walls (kitchen staple)
	placeCountersAlongWalls(tm, rng, rm, 2+rng.Intn(3))

	// Place refrigerator in corner or against wall
	if rmCols >= 3 && rmRows >= 3 {
		placeRefrigerator(tm, rng, rm)
	}

	// Small table for dining (if space allows)
	if rmCols >= 4 && rmRows >= 4 && rng.Float64() < 0.6 {
		placeTableCluster(tm, rng, rm)
	}
}

// placeBedroomFurniture creates a bedroom with bed, dresser, and storage.
func placeBedroomFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	if rmCols < 3 || rmRows < 3 {
		return
	}

	// Place bed (2×1) - bedroom centerpiece
	placeBedInRoom(tm, rng, rm)

	// Place dresser/wardrobe against wall
	if rng.Float64() < 0.8 {
		placeWallStorage(tm, rng, rm, ObjectBookshelf) // Represents dresser/wardrobe
	}

	// Small table (nightstand) next to bed
	if rmCols >= 4 && rng.Float64() < 0.5 {
		placeNightstand(tm, rng, rm)
	}

	// Chair (40% chance)
	if rng.Float64() < 0.4 {
		placeChairsInRoom(tm, rng, rm, 1)
	}
}

// placeOfficeFurniture creates an office with desk, chair, filing, and bookshelf.
func placeOfficeFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	if rmCols < 3 || rmRows < 3 {
		return
	}

	// Place desk (2×1) - office centerpiece
	placeDeskInRoom(tm, rng, rm)

	// Office chair with desk (90% chance)
	if rng.Float64() < 0.9 {
		placeChairsInRoom(tm, rng, rm, 1)
	}

	// Filing cabinet/bookshelf along walls (80% chance)
	if rng.Float64() < 0.8 {
		count := 1 + rng.Intn(2)
		for i := 0; i < count; i++ {
			if rng.Float64() < 0.5 {
				placeWallStorage(tm, rng, rm, ObjectBookshelf)
			} else {
				placeWallStorage(tm, rng, rm, ObjectShelving) // Filing cabinet
			}
		}
	}
}

// placeStorageFurniture creates a storage room with shelves, crates, and containers.
func placeStorageFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	// Storage rooms are densely packed
	placeCrates(tm, rng, rm) // Always have crates

	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	// Add shelving units along walls (80% chance)
	if rng.Float64() < 0.8 && rmCols >= 3 && rmRows >= 3 {
		count := 2 + rng.Intn(3)
		for i := 0; i < count; i++ {
			placeWallStorage(tm, rng, rm, ObjectShelving)
		}
	}

	// Add barrels and pallets (60% chance)
	if rng.Float64() < 0.6 && rmCols >= 3 && rmRows >= 3 {
		placeIndustrialStorage(tm, rng, rm)
	}
}

// placeWorkshopFurniture creates a workshop with workbenches, tools, and machinery.
func placeWorkshopFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	if rmCols < 3 || rmRows < 3 {
		return
	}

	// Place workbench (3×1) if room is wide enough
	if rmCols >= 5 && rng.Float64() < 0.8 {
		placeWorkbench(tm, rng, rm)
	}

	// Machinery in larger workshops
	if rmCols >= 4 && rmRows >= 4 && rng.Float64() < 0.5 {
		placeMachinery(tm, rng, rm)
	}

	// Tool storage (crates and shelving)
	if rng.Float64() < 0.9 {
		placeWallStorage(tm, rng, rm, ObjectShelving) // Tool racks
	}

	// Scattered supplies
	if rng.Float64() < 0.6 {
		placeIndustrialStorage(tm, rng, rm)
	}
}

// placeBreakroomFurniture creates a break room with tables, chairs, and appliances.
func placeBreakroomFurniture(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	if rmCols < 3 || rmRows < 3 {
		return
	}

	// Central table for eating/meeting
	if rng.Float64() < 0.9 {
		placeTableCluster(tm, rng, rm)
	}

	// Multiple chairs around table
	chairCount := 3 + rng.Intn(3) // 3-5 chairs
	placeChairsInRoom(tm, rng, rm, chairCount)

	// Appliances against walls (refrigerator, etc.)
	if rng.Float64() < 0.7 {
		placeWallStorage(tm, rng, rm, ObjectRefrigerator)
	}

	// Counter/cabinet for storage
	if rng.Float64() < 0.5 {
		placeWallStorage(tm, rng, rm, ObjectCounter)
	}
}

// Helper functions for room-type-specific furniture placement

// placeSofaAgainstWall places a 2×1 sofa against a room wall.
func placeSofaAgainstWall(tm *TileMap, rng *rand.Rand, rm interiorRoom) { //nolint:gocognit,gocyclo
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	// Try each wall
	sides := []int{0, 1, 2, 3}
	rng.Shuffle(len(sides), func(i, j int) { sides[i], sides[j] = sides[j], sides[i] })

	for _, side := range sides {
		var cc, cr int
		var horizontal bool
		switch side {
		case 0: // north wall
			if endCol-startCol < 3 {
				continue
			}
			cc = startCol + 1 + rng.Intn(endCol-startCol-2)
			cr = startRow + 1
			horizontal = true
		case 1: // south wall
			if endCol-startCol < 3 {
				continue
			}
			cc = startCol + 1 + rng.Intn(endCol-startCol-2)
			cr = endRow - 1
			horizontal = true
		case 2: // west wall
			if endRow-startRow < 3 {
				continue
			}
			cc = startCol + 1
			cr = startRow + 1 + rng.Intn(endRow-startRow-2)
			horizontal = false
		case 3: // east wall
			if endRow-startRow < 3 {
				continue
			}
			cc = endCol - 1
			cr = startRow + 1 + rng.Intn(endRow-startRow-2)
			horizontal = false
		}

		// Place 2×1 sofa
		if horizontal {
			if tm.inBounds(cc, cr) && tm.inBounds(cc+1, cr) &&
				tm.ObjectAt(cc, cr) == ObjectNone && tm.ObjectAt(cc+1, cr) == ObjectNone {
				tm.SetObject(cc, cr, ObjectSofa)
				tm.SetObject(cc+1, cr, ObjectSofa)
				return
			}
		} else {
			if tm.inBounds(cc, cr) && tm.inBounds(cc, cr+1) &&
				tm.ObjectAt(cc, cr) == ObjectNone && tm.ObjectAt(cc, cr+1) == ObjectNone {
				tm.SetObject(cc, cr, ObjectSofa)
				tm.SetObject(cc, cr+1, ObjectSofa)
				return
			}
		}
	}
}

// placeChairsInRoom places the specified number of chairs in random positions.
func placeChairsInRoom(tm *TileMap, rng *rand.Rand, rm interiorRoom, count int) {
	startCol := rm.rx/cellSize + 1
	startRow := rm.ry/cellSize + 1
	endCol := (rm.rx+rm.rw)/cellSize - 1
	endRow := (rm.ry+rm.rh)/cellSize - 1

	if endCol <= startCol || endRow <= startRow {
		return
	}

	placed := 0
	for i := 0; i < count && placed < count; i++ {
		for attempts := 0; attempts < 15; attempts++ {
			cc := startCol + rng.Intn(endCol-startCol)
			cr := startRow + rng.Intn(endRow-startRow)

			if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
				tm.SetObject(cc, cr, ObjectChair)
				placed++
				break
			}
		}
	}
}

// placeCountersAlongWalls places kitchen counters along room walls.
func placeCountersAlongWalls(tm *TileMap, rng *rand.Rand, rm interiorRoom, count int) {
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	placed := 0
	for attempts := 0; attempts < count*5 && placed < count; attempts++ {
		side := rng.Intn(4)
		var cc, cr int

		switch side {
		case 0: // north wall
			if startRow+1 >= endRow {
				continue
			}
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = startRow + 1
		case 1: // south wall
			if endRow-1 <= startRow {
				continue
			}
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = endRow - 1
		case 2: // west wall
			if startCol+1 >= endCol {
				continue
			}
			cc = startCol + 1
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		default: // east wall
			if endCol-1 <= startCol {
				continue
			}
			cc = endCol - 1
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		}

		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			tm.SetObject(cc, cr, ObjectCounter)
			placed++
		}
	}
}

// placeRefrigerator places a refrigerator in a corner or against a wall.
func placeRefrigerator(tm *TileMap, _ *rand.Rand, rm interiorRoom) {
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	// Prefer corners for refrigerators
	corners := [][2]int{
		{startCol + 1, startRow + 1}, // NW
		{endCol - 1, startRow + 1},   // NE
		{startCol + 1, endRow - 1},   // SW
		{endCol - 1, endRow - 1},     // SE
	}

	for _, corner := range corners {
		cc, cr := corner[0], corner[1]
		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			tm.SetObject(cc, cr, ObjectRefrigerator)
			return
		}
	}
}

// placeBedInRoom places a 2×1 bed in the room.
func placeBedInRoom(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	startCol := rm.rx/cellSize + 1
	startRow := rm.ry/cellSize + 1
	endCol := (rm.rx+rm.rw)/cellSize - 1
	endRow := (rm.ry+rm.rh)/cellSize - 1

	if endCol <= startCol+1 || endRow <= startRow+1 {
		return
	}

	// Try horizontal and vertical orientations
	for attempts := 0; attempts < 10; attempts++ {
		horizontal := rng.Intn(2) == 0

		if horizontal {
			// 2×1 bed horizontally
			cc := startCol + rng.Intn(endCol-startCol-1)
			cr := startRow + rng.Intn(endRow-startRow)

			if tm.inBounds(cc+1, cr) && tm.ObjectAt(cc, cr) == ObjectNone && tm.ObjectAt(cc+1, cr) == ObjectNone {
				tm.SetObject(cc, cr, ObjectBed)
				tm.SetObject(cc+1, cr, ObjectBed)
				return
			}
		} else {
			// 2×1 bed vertically
			cc := startCol + rng.Intn(endCol-startCol)
			cr := startRow + rng.Intn(endRow-startRow-1)

			if tm.inBounds(cc, cr+1) && tm.ObjectAt(cc, cr) == ObjectNone && tm.ObjectAt(cc, cr+1) == ObjectNone {
				tm.SetObject(cc, cr, ObjectBed)
				tm.SetObject(cc, cr+1, ObjectBed)
				return
			}
		}
	}
}

// placeWallStorage places a single storage object against a wall.
func placeWallStorage(tm *TileMap, rng *rand.Rand, rm interiorRoom, objectType ObjectType) {
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	// Try each wall
	sides := []int{0, 1, 2, 3}
	rng.Shuffle(len(sides), func(i, j int) { sides[i], sides[j] = sides[j], sides[i] })

	for _, side := range sides {
		var cc, cr int
		switch side {
		case 0: // north wall
			if startRow+1 >= endRow {
				continue
			}
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = startRow + 1
		case 1: // south wall
			if endRow-1 <= startRow {
				continue
			}
			cc = startCol + 1 + rng.Intn(max(1, endCol-startCol-1))
			cr = endRow - 1
		case 2: // west wall
			if startCol+1 >= endCol {
				continue
			}
			cc = startCol + 1
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		default: // east wall
			if endCol-1 <= startCol {
				continue
			}
			cc = endCol - 1
			cr = startRow + 1 + rng.Intn(max(1, endRow-startRow-1))
		}

		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			tm.SetObject(cc, cr, objectType)
			return
		}
	}
}

// placeNightstand places a small table (nightstand) near the bed.
func placeNightstand(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	// Simple implementation: place a table somewhere in the room
	startCol := rm.rx/cellSize + 1
	startRow := rm.ry/cellSize + 1
	endCol := (rm.rx+rm.rw)/cellSize - 1
	endRow := (rm.ry+rm.rh)/cellSize - 1

	if endCol <= startCol || endRow <= startRow {
		return
	}

	for attempts := 0; attempts < 8; attempts++ {
		cc := startCol + rng.Intn(endCol-startCol)
		cr := startRow + rng.Intn(endRow-startRow)

		if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
			tm.SetObject(cc, cr, ObjectTable) // Small table as nightstand
			return
		}
	}
}

// placeDeskInRoom places a 2×1 desk in the room.
func placeDeskInRoom(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
	startCol := rm.rx/cellSize + 1
	startRow := rm.ry/cellSize + 1
	endCol := (rm.rx+rm.rw)/cellSize - 1
	endRow := (rm.ry+rm.rh)/cellSize - 1

	if endCol <= startCol+1 || endRow <= startRow+1 {
		return
	}

	// Try horizontal and vertical orientations
	for attempts := 0; attempts < 8; attempts++ {
		horizontal := rng.Intn(2) == 0

		if horizontal {
			cc := startCol + rng.Intn(endCol-startCol-1)
			cr := startRow + rng.Intn(endRow-startRow)

			if tm.inBounds(cc+1, cr) && tm.ObjectAt(cc, cr) == ObjectNone && tm.ObjectAt(cc+1, cr) == ObjectNone {
				tm.SetObject(cc, cr, ObjectDesk)
				tm.SetObject(cc+1, cr, ObjectDesk)
				return
			}
		} else {
			cc := startCol + rng.Intn(endCol-startCol)
			cr := startRow + rng.Intn(endRow-startRow-1)

			if tm.inBounds(cc, cr+1) && tm.ObjectAt(cc, cr) == ObjectNone && tm.ObjectAt(cc, cr+1) == ObjectNone {
				tm.SetObject(cc, cr, ObjectDesk)
				tm.SetObject(cc, cr+1, ObjectDesk)
				return
			}
		}
	}
}

// placeIndustrialStorage places barrels and pallets in industrial rooms.
func placeIndustrialStorage(tm *TileMap, rng *rand.Rand, rm interiorRoom) { //nolint:gocognit,gocyclo
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	// Place 1-3 barrels
	barrelCount := 1 + rng.Intn(3)
	for i := 0; i < barrelCount; i++ {
		for attempts := 0; attempts < 8; attempts++ {
			cc := rm.rx/cellSize + rng.Intn(rmCols)
			cr := rm.ry/cellSize + rng.Intn(rmRows)

			if tm.inBounds(cc, cr) && tm.ObjectAt(cc, cr) == ObjectNone {
				tm.SetObject(cc, cr, ObjectBarrel)
				break
			}
		}
	}

	// Place 1-2 pallets (2×2) if room is large enough
	if rmCols >= 4 && rmRows >= 4 && rng.Float64() < 0.6 {
		palletCount := 1 + rng.Intn(2)
		for i := 0; i < palletCount; i++ {
			for attempts := 0; attempts < 8; attempts++ {
				cc := rm.rx/cellSize + rng.Intn(rmCols-1)
				cr := rm.ry/cellSize + rng.Intn(rmRows-1)

				// Check 2×2 area is clear
				if tm.inBounds(cc+1, cr+1) {
					isClear := true
					for dc := 0; dc < 2 && isClear; dc++ {
						for dr := 0; dr < 2 && isClear; dr++ {
							if tm.ObjectAt(cc+dc, cr+dr) != ObjectNone {
								isClear = false
							}
						}
					}
					if isClear {
						for dc := 0; dc < 2; dc++ {
							for dr := 0; dr < 2; dr++ {
								tm.SetObject(cc+dc, cr+dr, ObjectPallet)
							}
						}
						break
					}
				}
			}
		}
	}
}

// placeWorkbench places a 3×1 workbench in the workshop.
func placeWorkbench(tm *TileMap, rng *rand.Rand, rm interiorRoom) { //nolint:gocognit
	startCol := rm.rx / cellSize
	startRow := rm.ry / cellSize
	endCol := (rm.rx + rm.rw - 1) / cellSize
	endRow := (rm.ry + rm.rh - 1) / cellSize

	// Try to place against walls (horizontal 3×1)
	sides := []int{0, 1} // north and south walls (horizontal workbench)
	rng.Shuffle(len(sides), func(i, j int) { sides[i], sides[j] = sides[j], sides[i] })

	for _, side := range sides {
		var cr int
		if side == 0 { // north wall
			cr = startRow + 1
		} else { // south wall
			cr = endRow - 1
		}

		if endCol-startCol < 4 { // Need at least 3 units + margins
			continue
		}

		cc := startCol + 1 + rng.Intn(endCol-startCol-3)

		// Check 3×1 area is clear
		if tm.inBounds(cc+2, cr) {
			isClear := true
			for dc := 0; dc < 3; dc++ {
				if tm.ObjectAt(cc+dc, cr) != ObjectNone {
					isClear = false
					break
				}
			}
			if isClear {
				for dc := 0; dc < 3; dc++ {
					tm.SetObject(cc+dc, cr, ObjectWorkbench)
				}
				return
			}
		}
	}
}

// placeMachinery places 2×2 machinery in workshops.
func placeMachinery(tm *TileMap, rng *rand.Rand, rm interiorRoom) { //nolint:gocognit
	rmCols := rm.rw / cellSize
	rmRows := rm.rh / cellSize

	if rmCols < 4 || rmRows < 4 {
		return
	}

	// Place machinery away from walls
	for attempts := 0; attempts < 8; attempts++ {
		cc := rm.rx/cellSize + 1 + rng.Intn(rmCols-3)
		cr := rm.ry/cellSize + 1 + rng.Intn(rmRows-3)

		// Check 2×2 area is clear
		if tm.inBounds(cc+1, cr+1) {
			isClear := true
			for dc := 0; dc < 2 && isClear; dc++ {
				for dr := 0; dr < 2 && isClear; dr++ {
					if tm.ObjectAt(cc+dc, cr+dr) != ObjectNone {
						isClear = false
					}
				}
			}
			if isClear {
				for dc := 0; dc < 2; dc++ {
					for dr := 0; dr < 2; dr++ {
						tm.SetObject(cc+dc, cr+dr, ObjectMachinery)
					}
				}
				return
			}
		}
	}
}
