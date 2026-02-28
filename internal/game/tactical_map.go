package game

import "math"

// CellTrait flags describe tactical properties of a grid cell.
type CellTrait uint8

const (
	CellTraitNone      CellTrait = 0
	CellTraitWallAdj   CellTrait = 1 << iota // adjacent to a wall segment
	CellTraitCorner                          // next to a building corner (two perpendicular walls meet)
	CellTraitDoorway                         // inside a doorway gap in a wall run
	CellTraitDoorAdj                         // adjacent to a doorway (good firing position)
	CellTraitInterior                        // inside a building footprint
	CellTraitWindow                          // cell IS a window (blocks movement, transparent to LOS)
	CellTraitWindowAdj                       // interior cell adjacent to a window (high-value overwatch)
)

// TacticalMap pre-computes per-cell tactical properties for the entire battlefield.
// Built once at init from building geometry; soldiers query it at runtime.
type TacticalMap struct {
	cols, rows int
	traits     []CellTrait
	// desirability is a pre-computed -1..+1 score per cell.
	// Positive = good place to stop (corner cover, door-adjacent).
	// Negative = bad place to stop (in a doorway, open ground near buildings).
	// Zero = neutral open ground.
	desirability []float64
}

// NewTacticalMap analyses building walls, windows, and footprints to produce a TacticalMap.
func NewTacticalMap(mapW, mapH int, buildings []rect, windows []rect, footprints []rect) *TacticalMap {
	cols := mapW / cellSize
	rows := mapH / cellSize
	tm := &TacticalMap{
		cols:         cols,
		rows:         rows,
		traits:       make([]CellTrait, cols*rows),
		desirability: make([]float64, cols*rows),
	}

	// Step 1: Build a wall occupancy set for fast neighbour queries.
	wallSet := make(map[[2]int]bool, len(buildings))
	for _, b := range buildings {
		cx := b.x / cellSize
		cy := b.y / cellSize
		wallSet[[2]int{cx, cy}] = true
	}

	// Step 1b: Build a window occupancy set.
	windowSet := make(map[[2]int]bool, len(windows))
	for _, w := range windows {
		cx := w.x / cellSize
		cy := w.y / cellSize
		windowSet[[2]int{cx, cy}] = true
	}

	// Step 2: Build a footprint interior set.
	interiorSet := make(map[[2]int]bool)
	for _, fp := range footprints {
		cxMin := fp.x / cellSize
		cyMin := fp.y / cellSize
		cxMax := (fp.x + fp.w - 1) / cellSize
		cyMax := (fp.y + fp.h - 1) / cellSize
		for cy := cyMin; cy <= cyMax; cy++ {
			for cx := cxMin; cx <= cxMax; cx++ {
				interiorSet[[2]int{cx, cy}] = true
			}
		}
	}

	// isBlocked returns true for walls and windows (both are impassable).
	isBlocked := func(cx, cy int) bool {
		return wallSet[[2]int{cx, cy}] || windowSet[[2]int{cx, cy}]
	}

	// Step 3: For each walkable cell, classify traits.
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			idx := cy*cols + cx

			// Mark window cells.
			if windowSet[[2]int{cx, cy}] {
				tm.traits[idx] |= CellTraitWindow
				continue // windows are not walkable
			}
			if wallSet[[2]int{cx, cy}] {
				continue // wall cell itself — not walkable
			}

			if interiorSet[[2]int{cx, cy}] {
				tm.traits[idx] |= CellTraitInterior
			}

			// Check 4-directional neighbours for walls (including windows as wall-like).
			adjWalls := 0
			hasN := isBlocked(cx, cy-1)
			hasS := isBlocked(cx, cy+1)
			hasW := isBlocked(cx-1, cy)
			hasE := isBlocked(cx+1, cy)
			if hasN {
				adjWalls++
			}
			if hasS {
				adjWalls++
			}
			if hasW {
				adjWalls++
			}
			if hasE {
				adjWalls++
			}

			if adjWalls > 0 {
				tm.traits[idx] |= CellTraitWallAdj
			}

			// Corner detection: two perpendicular adjacent walls.
			if (hasN && hasW) || (hasN && hasE) || (hasS && hasW) || (hasS && hasE) {
				tm.traits[idx] |= CellTraitCorner
			}

			// Window-adjacent: interior cell next to a window = high-value overwatch.
			if interiorSet[[2]int{cx, cy}] {
				if windowSet[[2]int{cx, cy - 1}] || windowSet[[2]int{cx, cy + 1}] ||
					windowSet[[2]int{cx - 1, cy}] || windowSet[[2]int{cx + 1, cy}] {
					tm.traits[idx] |= CellTraitWindowAdj
				}
			}
		}
	}

	// Step 4: Detect doorways.
	// A doorway is a walkable cell in a wall run gap: it has walls on two
	// opposite sides (N+S or W+E) but the cell itself and perpendicular
	// neighbours are open. The cell is effectively a chokepoint.
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			if wallSet[[2]int{cx, cy}] {
				continue
			}
			idx := cy*cols + cx
			hasN := wallSet[[2]int{cx, cy - 1}]
			hasS := wallSet[[2]int{cx, cy + 1}]
			hasW := wallSet[[2]int{cx - 1, cy}]
			hasE := wallSet[[2]int{cx + 1, cy}]

			isDoorway := false
			// Horizontal wall run with gap: walls to W+E, open N or S.
			if hasW && hasE && !hasN && !hasS {
				isDoorway = true
			}
			// Vertical wall run with gap: walls to N+S, open W or E.
			if hasN && hasS && !hasW && !hasE {
				isDoorway = true
			}

			if isDoorway {
				tm.traits[idx] |= CellTraitDoorway
				// Mark adjacent open cells as door-adjacent (good cover positions).
				for _, d := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
					nx, ny := cx+d[0], cy+d[1]
					if nx < 0 || ny < 0 || nx >= cols || ny >= rows {
						continue
					}
					if wallSet[[2]int{nx, ny}] {
						continue
					}
					nIdx := ny*cols + nx
					tm.traits[nIdx] |= CellTraitDoorAdj
				}
			}
		}
	}

	// Step 5: Compute desirability scores.
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			idx := cy*cols + cx
			if wallSet[[2]int{cx, cy}] || windowSet[[2]int{cx, cy}] {
				tm.desirability[idx] = -1.0 // wall/window — impassable
				continue
			}
			score := 0.0
			t := tm.traits[idx]

			// Corners are highly desirable — they provide cover and peek angles.
			if t&CellTraitCorner != 0 {
				score += 0.7
			}
			// Wall-adjacent is moderately good — provides concealment.
			if t&CellTraitWallAdj != 0 {
				score += 0.2
			}
			// Door-adjacent is good — can cover the doorway.
			if t&CellTraitDoorAdj != 0 {
				score += 0.4
			}
			// Doorways themselves are bad places to stop — exposed chokepoint.
			if t&CellTraitDoorway != 0 {
				score -= 0.6
			}
			// Window-adjacent interior cells are prime overwatch positions.
			// Soldier can fire through the window while protected by the building.
			if t&CellTraitWindowAdj != 0 {
				score += 0.85
			}
			// Interior cells are generally safer than open ground.
			if t&CellTraitInterior != 0 && score < 0.1 {
				score += 0.15
			}

			tm.desirability[idx] = math.Max(-1, math.Min(1, score))
		}
	}

	return tm
}

// ScanBestNearby searches nearby walkable cells for the best tactical position.
// It considers desirability, distance to enemy (prefer closer/perpendicular), and
// whether the cell is inside a claimed building. Returns world coords and score.
//
//	enemyBearing: bearing toward known enemy (radians), or 0 if none.
//	hasEnemy:     true if an enemy direction is known.
//	claimedIdx:   index of claimed building footprint, or -1.
//	footprints:   building footprints slice.
func (tm *TacticalMap) ScanBestNearby(wx, wy float64, radius int, enemyBearing float64, hasEnemy bool, claimedIdx int, footprints []rect) (float64, float64, float64) {
	cx, cy := WorldToCell(wx, wy)
	bestScore := -999.0
	bestX, bestY := wx, wy

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			nx, ny := cx+dx, cy+dy
			if nx < 0 || ny < 0 || nx >= tm.cols || ny >= tm.rows {
				continue
			}
			idx := ny*tm.cols + nx
			if tm.desirability[idx] <= -1.0 {
				continue // impassable
			}

			score := tm.desirability[idx]

			// Distance penalty — prefer closer positions.
			dist := math.Sqrt(float64(dx*dx + dy*dy))
			if dist > float64(radius) {
				continue
			}
			score -= dist * 0.02

			// Directional bias when enemy is known.
			if hasEnemy && dist > 0 {
				cellAngle := math.Atan2(float64(dy), float64(dx))
				// Forward bias: cells toward the enemy get a bonus.
				angleDiff := math.Abs(normalizeAngle(cellAngle - enemyBearing))
				if angleDiff < math.Pi/3 {
					score += 0.20 * (1.0 - angleDiff/(math.Pi/3))
				}
				// Perpendicular bias: cells roughly 90° from enemy = flanking value.
				perpDiff := math.Abs(angleDiff - math.Pi/2)
				if perpDiff < math.Pi/4 {
					score += 0.15 * (1.0 - perpDiff/(math.Pi/4))
				}
			}

			// Bonus for cells inside the claimed building.
			if claimedIdx >= 0 && claimedIdx < len(footprints) {
				fp := footprints[claimedIdx]
				pwx, pwy := CellToWorld(nx, ny)
				if pwx >= float64(fp.x) && pwx < float64(fp.x+fp.w) &&
					pwy >= float64(fp.y) && pwy < float64(fp.y+fp.h) {
					score += 0.50
				}
			}

			if score > bestScore {
				bestScore = score
				bestX, bestY = CellToWorld(nx, ny)
			}
		}
	}
	return bestX, bestY, bestScore
}

// FindBoundCover searches for the best cover position along a bearing from (wx,wy).
// It scans cells in a corridor ±corridorHalf cells wide, from minDist to maxDist
// cells ahead along the bearing. Returns (x, y, score, found).
// Used by cover-to-cover bounding: soldiers sprint to the nearest good cover
// along their advance direction rather than one long dash to the final target.
func (tm *TacticalMap) FindBoundCover(wx, wy, bearing float64, minDist, maxDist int) (float64, float64, float64, bool) {
	cx, cy := WorldToCell(wx, wy)
	cosB := math.Cos(bearing)
	sinB := math.Sin(bearing)

	corridorHalf := 3 // cells either side of the bearing line

	bestScore := -999.0
	var bestX, bestY float64
	found := false

	for ahead := minDist; ahead <= maxDist; ahead++ {
		for lateral := -corridorHalf; lateral <= corridorHalf; lateral++ {
			// Rotate (ahead, lateral) by bearing.
			fx := float64(ahead)*cosB - float64(lateral)*sinB
			fy := float64(ahead)*sinB + float64(lateral)*cosB
			nx := cx + int(math.Round(fx))
			ny := cy + int(math.Round(fy))
			if nx < 0 || ny < 0 || nx >= tm.cols || ny >= tm.rows {
				continue
			}
			idx := ny*tm.cols + nx
			if tm.desirability[idx] <= -1.0 {
				continue
			}

			score := tm.desirability[idx]

			// Strong preference for cover traits.
			t := tm.traits[idx]
			if t&CellTraitCorner != 0 {
				score += 0.40
			}
			if t&CellTraitWallAdj != 0 {
				score += 0.20
			}
			if t&CellTraitDoorAdj != 0 {
				score += 0.25
			}
			if t&CellTraitWindowAdj != 0 {
				score += 0.50
			}
			// Doorways are terrible stopping points.
			if t&CellTraitDoorway != 0 {
				score -= 0.80
			}

			// Slight preference for cells closer (shorter bound = safer).
			score -= float64(ahead) * 0.01
			// Penalise lateral drift — prefer staying on the bearing line.
			score -= math.Abs(float64(lateral)) * 0.03

			if score > bestScore {
				bestScore = score
				bestX, bestY = CellToWorld(nx, ny)
				found = true
			}
		}
	}
	return bestX, bestY, bestScore, found
}

// TraitAt returns the CellTrait flags for a world position.
func (tm *TacticalMap) TraitAt(wx, wy float64) CellTrait {
	cx, cy := WorldToCell(wx, wy)
	if cx < 0 || cy < 0 || cx >= tm.cols || cy >= tm.rows {
		return CellTraitNone
	}
	return tm.traits[cy*tm.cols+cx]
}

// DesirabilityAt returns the pre-computed desirability score at a world position.
func (tm *TacticalMap) DesirabilityAt(wx, wy float64) float64 {
	cx, cy := WorldToCell(wx, wy)
	if cx < 0 || cy < 0 || cx >= tm.cols || cy >= tm.rows {
		return 0
	}
	return tm.desirability[cy*tm.cols+cx]
}

// IsCorner returns true if the world position is at a building corner.
func (tm *TacticalMap) IsCorner(wx, wy float64) bool {
	return tm.TraitAt(wx, wy)&CellTraitCorner != 0
}

// IsDoorway returns true if the world position is inside a doorway.
func (tm *TacticalMap) IsDoorway(wx, wy float64) bool {
	return tm.TraitAt(wx, wy)&CellTraitDoorway != 0
}

// IsWallAdjacent returns true if the world position is next to a wall.
func (tm *TacticalMap) IsWallAdjacent(wx, wy float64) bool {
	return tm.TraitAt(wx, wy)&CellTraitWallAdj != 0
}

// IsDoorAdjacent returns true if the world position is next to a doorway.
func (tm *TacticalMap) IsDoorAdjacent(wx, wy float64) bool {
	return tm.TraitAt(wx, wy)&CellTraitDoorAdj != 0
}

// CornerPeekDirections returns the angles a soldier at (wx,wy) could peek around
// a corner. Each direction is an angle in radians where a gap exists adjacent to
// the corner walls. Returns nil if not at a corner.
func (tm *TacticalMap) CornerPeekDirections(wx, wy float64) []float64 {
	cx, cy := WorldToCell(wx, wy)
	if cx < 0 || cy < 0 || cx >= tm.cols || cy >= tm.rows {
		return nil
	}
	if tm.traits[cy*tm.cols+cx]&CellTraitCorner == 0 {
		return nil
	}

	// Determine which walls form the corner and compute peek directions.
	// Peek direction = angle from the cell into the open area around the corner.
	var dirs []float64
	var wallN, wallS, wallW, wallE bool

	// Actually we need to check if the neighbouring cell IS a wall (blocked),
	// not if its trait is zero. Use the navgrid-style check against the building set.
	// Since we don't store the wall set, recompute from traits: a wall cell has
	// desirability -1.
	isWall := func(cx2, cy2 int) bool {
		if cx2 < 0 || cy2 < 0 || cx2 >= tm.cols || cy2 >= tm.rows {
			return true
		}
		return tm.desirability[cy2*tm.cols+cx2] == -1.0
	}

	wallN = isWall(cx, cy-1)
	wallS = isWall(cx, cy+1)
	wallW = isWall(cx-1, cy)
	wallE = isWall(cx+1, cy)

	// For a NW corner (walls N+W): peek east along north wall, peek south along west wall.
	if wallN && wallW {
		dirs = append(dirs, 0)         // east
		dirs = append(dirs, math.Pi/2) // south
	}
	if wallN && wallE {
		dirs = append(dirs, math.Pi)   // west
		dirs = append(dirs, math.Pi/2) // south
	}
	if wallS && wallW {
		dirs = append(dirs, 0)          // east
		dirs = append(dirs, -math.Pi/2) // north
	}
	if wallS && wallE {
		dirs = append(dirs, math.Pi)    // west
		dirs = append(dirs, -math.Pi/2) // north
	}

	return dirs
}
