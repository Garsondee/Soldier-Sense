package game

import (
	"math"
	"math/rand"
)

// gridRoadConfig holds tuneable parameters for grid road generation.
type gridRoadConfig struct {
	MainRoadCount   int     // number of main roads (horizontal + vertical)
	MainRoadWidth   int     // width in tiles (odd numbers centre nicely)
	SideStreetCount int     // maximum side streets
	SideStreetWidth int     // width in tiles
	PavementChance  float64 // probability of pavement on each road edge
	MinStraightRun  int     // minimum tiles before a road can turn
}

var defaultRoadConfig = gridRoadConfig{
	MainRoadCount:   4, // 2 horizontal + 1-2 vertical
	MainRoadWidth:   5,
	SideStreetCount: 3,
	SideStreetWidth: 3,
	PavementChance:  0.7,
	MinStraightRun:  10,
}

// gridRoadPath stores a single road as an ordered list of tile coordinates.
type gridRoadPath struct {
	tiles [][2]int // (col, row) centres
	width int      // total width in tiles
}

// generateGridRoads creates grid-aligned roads and stamps them into the tile map.
// Roads are axis-aligned paths that traverse the map with occasional 90° turns.
func generateGridRoads(tm *TileMap, rng *rand.Rand, cfg gridRoadConfig) []gridRoadPath {
	paths := make([]gridRoadPath, 0, cfg.MainRoadCount+cfg.SideStreetCount)

	// Horizontal-ish roads.
	numH := 2
	numV := cfg.MainRoadCount - numH
	if numV < 1 {
		numV = 1
	}

	// Spread horizontal roads across the map height.
	hSlots := spreadSlots(tm.Rows, numH, rng)
	for _, baseRow := range hSlots {
		path := generateSingleRoad(tm, rng, true, baseRow, cfg.MainRoadWidth, cfg.MinStraightRun)
		if len(path.tiles) > 0 {
			stampRoad(tm, path, cfg.PavementChance, rng)
			paths = append(paths, path)
		}
	}

	// Vertical-ish roads.
	vSlots := spreadSlots(tm.Cols, numV, rng)
	for _, baseCol := range vSlots {
		path := generateSingleRoad(tm, rng, false, baseCol, cfg.MainRoadWidth, cfg.MinStraightRun)
		if len(path.tiles) > 0 {
			stampRoad(tm, path, cfg.PavementChance, rng)
			paths = append(paths, path)
		}
	}

	// Side streets: short stubs branching off main roads.
	for i := 0; i < cfg.SideStreetCount; i++ {
		if len(paths) == 0 {
			break
		}
		parent := paths[rng.Intn(len(paths))]
		side := generateSideStreet(tm, rng, parent, cfg.SideStreetWidth, cfg.MinStraightRun)
		if len(side.tiles) > 0 {
			stampRoad(tm, side, cfg.PavementChance*0.5, rng)
			paths = append(paths, side)
		}
	}

	return paths
}

// spreadSlots distributes n slots evenly across mapSize with jitter.
func spreadSlots(mapSize, n int, rng *rand.Rand) []int {
	slots := make([]int, 0, n)
	margin := mapSize / 8
	usable := mapSize - 2*margin
	if usable < n*10 {
		usable = mapSize
		margin = 0
	}
	for i := 0; i < n; i++ {
		base := margin + (usable*(2*i+1))/(2*n)
		jitter := rng.Intn(max(1, usable/(n*4))) - usable/(n*8)
		pos := base + jitter
		if pos < margin {
			pos = margin
		}
		if pos >= mapSize-margin {
			pos = mapSize - margin - 1
		}
		slots = append(slots, pos)
	}
	return slots
}

// generateSingleRoad creates a road path that traverses the map.
// If horizontal=true, it goes left→right; otherwise top→bottom.
// The road mostly follows basePos but can shift by ±1 row/col occasionally.
func generateSingleRoad(tm *TileMap, rng *rand.Rand, horizontal bool, basePos, width, minStraight int) gridRoadPath {
	var path gridRoadPath
	path.width = width

	var maxLen int
	if horizontal {
		maxLen = tm.Cols
	} else {
		maxLen = tm.Rows
	}

	pos := basePos                                               // current cross-axis position
	straightCount := 0                                           // tiles since last turn
	nextTurnAfter := minStraight + rng.Intn(max(1, minStraight)) // tiles until eligible to turn

	for along := 0; along < maxLen; along++ {
		var col, row int
		if horizontal {
			col, row = along, pos
		} else {
			col, row = pos, along
		}
		path.tiles = append(path.tiles, [2]int{col, row})
		straightCount++

		// Possibly shift cross-axis position.
		if straightCount >= nextTurnAfter && along < maxLen-minStraight {
			shift := rng.Intn(3) - 1 // -1, 0, +1
			newPos := pos + shift
			// Clamp within margins.
			hw := width / 2
			if newPos-hw < 0 {
				newPos = hw
			}
			limit := tm.Rows
			if !horizontal {
				limit = tm.Cols
			}
			if newPos+hw >= limit {
				newPos = limit - hw - 1
			}
			if newPos != pos {
				// Stamp the transition tiles (fill the gap between old and new pos).
				if horizontal {
					if shift > 0 {
						for r := pos + 1; r <= newPos; r++ {
							path.tiles = append(path.tiles, [2]int{col, r})
						}
					} else {
						for r := pos - 1; r >= newPos; r-- {
							path.tiles = append(path.tiles, [2]int{col, r})
						}
					}
				} else {
					if shift > 0 {
						for c := pos + 1; c <= newPos; c++ {
							path.tiles = append(path.tiles, [2]int{c, along})
						}
					} else {
						for c := pos - 1; c >= newPos; c-- {
							path.tiles = append(path.tiles, [2]int{c, along})
						}
					}
				}
				pos = newPos
				straightCount = 0
				nextTurnAfter = minStraight + rng.Intn(max(1, minStraight*2))
			}
		}
	}
	return path
}

// generateSideStreet creates a short road branching perpendicular from a parent road.
func generateSideStreet(tm *TileMap, rng *rand.Rand, parent gridRoadPath, width, minStraight int) gridRoadPath {
	var path gridRoadPath
	path.width = width

	if len(parent.tiles) < 20 {
		return path
	}

	// Pick a random point along the parent to branch from.
	branchIdx := len(parent.tiles)/4 + rng.Intn(len(parent.tiles)/2)
	branchCol, branchRow := parent.tiles[branchIdx][0], parent.tiles[branchIdx][1]

	// Determine parent direction to branch perpendicular.
	// Measure dominant axis of parent near branch point.
	spanStart := max(0, branchIdx-5)
	spanEnd := min(len(parent.tiles)-1, branchIdx+5)
	dCol := parent.tiles[spanEnd][0] - parent.tiles[spanStart][0]
	dRow := parent.tiles[spanEnd][1] - parent.tiles[spanStart][1]

	var horizontal bool // side street direction
	if intAbs(dCol) >= intAbs(dRow) {
		// Parent is horizontal, branch vertical.
		horizontal = false
	} else {
		horizontal = true
	}

	// Choose direction: +1 or -1.
	dir := 1
	if rng.Intn(2) == 0 {
		dir = -1
	}

	// Length: 15-40 tiles.
	length := 15 + rng.Intn(26)

	for i := 0; i < length; i++ {
		var col, row int
		if horizontal {
			col = branchCol + i*dir
			row = branchRow
		} else {
			col = branchCol
			row = branchRow + i*dir
		}
		if col < 0 || col >= tm.Cols || row < 0 || row >= tm.Rows {
			break
		}
		path.tiles = append(path.tiles, [2]int{col, row})
	}
	return path
}

// stampRoad writes road tiles into the tile map, expanding from centre line by width.
func stampRoad(tm *TileMap, path gridRoadPath, pavementChance float64, rng *rand.Rand) {
	hw := path.width / 2
	hasPavement := rng.Float64() < pavementChance

	// Build a set for fast lookup of road centre tiles.
	centreSet := make(map[[2]int]bool, len(path.tiles))
	for _, t := range path.tiles {
		centreSet[t] = true
	}

	for _, t := range path.tiles {
		col, row := t[0], t[1]
		for dc := -hw; dc <= hw; dc++ {
			for dr := -hw; dr <= hw; dr++ {
				c, r := col+dc, row+dr
				if !tm.inBounds(c, r) {
					continue
				}
				// Check if this is an edge tile (for pavement).
				isEdge := intAbs(dc) == hw || intAbs(dr) == hw
				if isEdge && hasPavement {
					// Only stamp pavement if not already tarmac (intersection preservation).
					if tm.Ground(c, r) != GroundTarmac {
						tm.SetGround(c, r, GroundPavement)
						tm.AddFlag(c, r, TileFlagRoadEdge)
					}
				} else {
					tm.SetGround(c, r, GroundTarmac)
				}
			}
		}
	}
}

// tileOnRoad returns true if the given tile is a road tile (tarmac or pavement).
func tileOnRoad(tm *TileMap, col, row int) bool {
	g := tm.Ground(col, row)
	return g == GroundTarmac || g == GroundPavement
}

// rectOverlapsRoadTiles returns true if any tile within the pixel rect is a road tile.
func rectOverlapsRoadTiles(tm *TileMap, r rect) bool {
	cMin := r.x / cellSize
	rMin := r.y / cellSize
	cMax := (r.x + r.w - 1) / cellSize
	rMax := (r.y + r.h - 1) / cellSize
	for row := rMin; row <= rMax; row++ {
		for col := cMin; col <= cMax; col++ {
			if tileOnRoad(tm, col, row) {
				return true
			}
		}
	}
	return false
}

// buildingCandidatesAlongGridRoads returns a shuffled list of candidate building
// positions that sit adjacent to road tiles in the tile map.
func buildingCandidatesAlongGridRoads(tm *TileMap, rng *rand.Rand, unitW, unitH, minGap, maxGap int) []rect {
	results := make([]rect, 0, 128)
	cs := cellSize

	// Scan for road tiles and attempt placement nearby.
	// To avoid scanning every tile, sample at intervals.
	step := 8 // sample every 8 tiles
	gap := float64(minGap + rng.Intn(max(1, maxGap-minGap+1)))

	for row := 0; row < tm.Rows; row += step {
		for col := 0; col < tm.Cols; col += step {
			if !tileOnRoad(tm, col, row) {
				continue
			}
			// Try placing buildings on all four sides of this road tile.
			offsets := [][2]int{
				{0, -1}, // above
				{0, 1},  // below
				{-1, 0}, // left
				{1, 0},  // right
			}
			for _, off := range offsets {
				bx := (col + off[0]*int(math.Ceil(gap/float64(cs)+float64(unitW/cs)/2))) * cs
				by := (row + off[1]*int(math.Ceil(gap/float64(cs)+float64(unitH/cs)/2))) * cs
				// Snap to 16px grid.
				bx = (bx / cs) * cs
				by = (by / cs) * cs
				if bx < 64 || by < 64 || bx+unitW > tm.Cols*cs-64 || by+unitH > tm.Rows*cs-64 {
					continue
				}
				results = append(results, rect{x: bx, y: by, w: unitW, h: unitH})
			}
		}
	}

	// Also add some off-road candidates in the spaces between roads.
	mapW := tm.Cols * cs
	mapH := tm.Rows * cs
	offRoadCount := len(results) / 3
	for i := 0; i < offRoadCount; i++ {
		bx := 96 + rng.Intn(max(1, mapW-192-unitW))
		by := 96 + rng.Intn(max(1, mapH-192-unitH))
		bx = (bx / cs) * cs
		by = (by / cs) * cs
		results = append(results, rect{x: bx, y: by, w: unitW, h: unitH})
	}

	// Shuffle so we don't always place in the same order.
	rng.Shuffle(len(results), func(i, j int) {
		results[i], results[j] = results[j], results[i]
	})
	return results
}

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
