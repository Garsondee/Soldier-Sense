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

// interiorRoom represents a rectangular room within a building.
type interiorRoom struct {
	rx, ry, rw, rh int  // pixel coordinates
	hasDoorway     bool // true if this room has an identified doorway gap
	doorX, doorY   int  // pixel position of the doorway (if hasDoorway)
}

// furnishRoom places furniture and features inside a single room.
func furnishRoom(tm *TileMap, rng *rand.Rand, rm interiorRoom) {
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

	// --- Pillars ---
	// Large rooms (≥5×5 tiles) get a pillar near centre.
	if rmCols >= 5 && rmRows >= 5 && rng.Float64() < 0.4 {
		pc := rm.rx/cellSize + rmCols/2
		pr := rm.ry/cellSize + rmRows/2
		if tm.inBounds(pc, pr) && tm.ObjectAt(pc, pr) == ObjectNone {
			tm.SetObject(pc, pr, ObjectPillar)
		}
	}

	// --- Tables + Chairs ---
	// 30% of rooms get a table cluster.
	if rng.Float64() < 0.30 && rmCols >= 3 && rmRows >= 3 {
		placeTableCluster(tm, rng, rm)
	}

	// --- Crates ---
	// 15% of rooms get crates along a wall.
	if rng.Float64() < 0.15 && rmCols >= 3 && rmRows >= 3 {
		placeCrates(tm, rng, rm)
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
	// Use the centre cell of the gap.
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
