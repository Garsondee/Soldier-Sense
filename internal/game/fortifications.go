package game

import "math/rand"

// fortConfig holds tuneable parameters for fortification generation.
type fortConfig struct {
	TrenchCount    int // number of slit trench lines to attempt
	TrenchMinLen   int // minimum tiles per trench run
	TrenchMaxLen   int // maximum tiles per trench run
	SandbagCount   int // number of sandbag clusters
	WireCount      int // number of barbed wire patches
	ATBarrierCount int // number of anti-tank barrier clusters
	FenceCount     int // number of fence runs
}

var defaultFortConfig = fortConfig{
	TrenchCount:    6,
	TrenchMinLen:   4,
	TrenchMaxLen:   12,
	SandbagCount:   8,
	WireCount:      5,
	ATBarrierCount: 3,
	FenceCount:     4,
}

// generateFortifications places military fortifications into the TileMap.
// Runs after biome generation, skipping roads, buildings, and existing objects.
func generateFortifications(tm *TileMap, rng *rand.Rand, cfg fortConfig) {
	// --- Slit trenches ---
	for i := 0; i < cfg.TrenchCount; i++ {
		placeTrenchLine(tm, rng, cfg.TrenchMinLen, cfg.TrenchMaxLen)
	}

	// --- Sandbag clusters ---
	for i := 0; i < cfg.SandbagCount; i++ {
		placeSandbagCluster(tm, rng)
	}

	// --- Barbed wire ---
	for i := 0; i < cfg.WireCount; i++ {
		placeWireRun(tm, rng)
	}

	// --- Anti-tank barriers ---
	for i := 0; i < cfg.ATBarrierCount; i++ {
		placeATBarrierCluster(tm, rng)
	}

	// --- Fences ---
	for i := 0; i < cfg.FenceCount; i++ {
		placeFenceRun(tm, rng)
	}
}

// fortCanPlace returns true if the tile at (col,row) is free outdoor ground.
func fortCanPlace(tm *TileMap, col, row int) bool {
	if !tm.inBounds(col, row) {
		return false
	}
	t := &tm.Tiles[row*tm.Cols+col]
	if t.Object != ObjectNone {
		return false
	}
	if t.Flags&TileFlagIndoor != 0 {
		return false
	}
	if t.Ground == GroundTarmac || t.Ground == GroundPavement {
		return false
	}
	if t.Ground == GroundConcrete || t.Ground == GroundWater {
		return false
	}
	return true
}

// placeTrenchLine places a slit trench run (horizontal or vertical).
func placeTrenchLine(tm *TileMap, rng *rand.Rand, minLen, maxLen int) {
	col := 4 + rng.Intn(max(1, tm.Cols-8))
	row := 4 + rng.Intn(max(1, tm.Rows-8))
	horizontal := rng.Intn(2) == 0
	length := minLen + rng.Intn(max(1, maxLen-minLen+1))

	for i := 0; i < length; i++ {
		c, r := col, row
		if horizontal {
			c = col + i
		} else {
			r = row + i
		}
		if !fortCanPlace(tm, c, r) {
			break
		}
		tm.SetObject(c, r, ObjectSlitTrench)
		t := tm.At(c, r)
		t.Elevation = -1
		t.Flags |= TileFlagTrench
	}
}

// placeSandbagCluster places a small L-shaped or straight sandbag position.
func placeSandbagCluster(tm *TileMap, rng *rand.Rand) {
	col := 4 + rng.Intn(max(1, tm.Cols-8))
	row := 4 + rng.Intn(max(1, tm.Rows-8))

	// Straight run of 2-4 sandbags.
	horizontal := rng.Intn(2) == 0
	length := 2 + rng.Intn(3) // 2-4

	placed := 0
	for i := 0; i < length; i++ {
		c, r := col, row
		if horizontal {
			c = col + i
		} else {
			r = row + i
		}
		if !fortCanPlace(tm, c, r) {
			break
		}
		tm.SetObject(c, r, ObjectSandbag)
		placed++
	}

	// 40% chance of an L-turn.
	if placed >= 2 && rng.Float64() < 0.4 {
		turnLen := 1 + rng.Intn(2) // 1-2 extra
		for i := 1; i <= turnLen; i++ {
			var c, r int
			if horizontal {
				c = col + placed - 1
				r = row + i
			} else {
				c = col + i
				r = row + placed - 1
			}
			if !fortCanPlace(tm, c, r) {
				break
			}
			tm.SetObject(c, r, ObjectSandbag)
		}
	}
}

// placeWireRun places a short barbed wire entanglement.
func placeWireRun(tm *TileMap, rng *rand.Rand) {
	col := 4 + rng.Intn(max(1, tm.Cols-8))
	row := 4 + rng.Intn(max(1, tm.Rows-8))
	horizontal := rng.Intn(2) == 0
	length := 3 + rng.Intn(5) // 3-7

	for i := 0; i < length; i++ {
		c, r := col, row
		if horizontal {
			c = col + i
		} else {
			r = row + i
		}
		if !fortCanPlace(tm, c, r) {
			break
		}
		tm.SetObject(c, r, ObjectWire)
	}
}

// placeATBarrierCluster places 2-4 anti-tank barriers in a short line.
func placeATBarrierCluster(tm *TileMap, rng *rand.Rand) {
	col := 4 + rng.Intn(max(1, tm.Cols-8))
	row := 4 + rng.Intn(max(1, tm.Rows-8))
	horizontal := rng.Intn(2) == 0
	length := 2 + rng.Intn(3) // 2-4

	for i := 0; i < length; i++ {
		c, r := col, row
		if horizontal {
			c = col + i*2 // spaced every other tile
		} else {
			r = row + i*2
		}
		if !fortCanPlace(tm, c, r) {
			break
		}
		tm.SetObject(c, r, ObjectATBarrier)
	}
}

// placeFenceRun places a fence line of 4-10 tiles.
func placeFenceRun(tm *TileMap, rng *rand.Rand) {
	col := 3 + rng.Intn(max(1, tm.Cols-6))
	row := 3 + rng.Intn(max(1, tm.Rows-6))
	horizontal := rng.Intn(2) == 0
	length := 4 + rng.Intn(7) // 4-10

	for i := 0; i < length; i++ {
		c, r := col, row
		if horizontal {
			c = col + i
		} else {
			r = row + i
		}
		if !fortCanPlace(tm, c, r) {
			break
		}
		tm.SetObject(c, r, ObjectFence)
	}
}
