package game

import "math"

// TileMapCoverBetween returns whether the target at (sx,sy) has meaningful cover
// from a threat at (tx,ty), and the best defence fraction found.
//
// It samples cover along the threat->target line, but biases toward cover that is
// closer to the target ("being behind cover"), and includes the target tile so
// vegetation on the occupied cell can contribute.
func TileMapCoverBetween(tm *TileMap, sx, sy, tx, ty float64) (bool, float64) {
	if tm == nil {
		return false, 0
	}

	sc, sr := WorldToCell(sx, sy)
	tc, tr := WorldToCell(tx, ty)
	if !tm.inBounds(sc, sr) || !tm.inBounds(tc, tr) {
		return false, 0
	}

	dc := absInt(sc - tc)
	dr := absInt(sr - tr)
	totalSteps := dc
	if dr > totalSteps {
		totalSteps = dr
	}
	if totalSteps == 0 {
		cover := directionalTileCover(tm, sc, sr, tx, ty, sx, sy)
		return cover > 0.01, cover
	}

	col, row := tc, tr
	xStep := -1
	if tc < sc {
		xStep = 1
	}
	yStep := -1
	if tr < sr {
		yStep = 1
	}
	err := dc - dr

	best := 0.0
	found := false
	step := 0
	for {
		if tm.inBounds(col, row) {
			frac := float64(step) / float64(totalSteps)
			// Ignore early-line cover near the shooter; keep cover that is actually
			// interposing for the target half of the line.
			if frac >= 0.35 || (col == sc && row == sr) {
				cv := directionalTileCover(tm, col, row, tx, ty, sx, sy)
				if cv > best {
					best = cv
					found = true
				}
			}
		}

		if col == sc && row == sr {
			break
		}
		e2 := err * 2
		if e2 > -dr {
			err -= dr
			col += xStep
		}
		if e2 < dc {
			err += dc
			row += yStep
		}
		step++
	}

	if best > 0.90 {
		best = 0.90
	}
	return found && best > 0.01, best
}

func directionalTileCover(tm *TileMap, col, row int, threatX, threatY, targetX, targetY float64) float64 {
	if !tm.inBounds(col, row) {
		return 0
	}
	t := tm.At(col, row)
	if t == nil {
		return 0
	}

	ground := groundCoverValue(t.Ground)
	obj := objectCoverValue(t.Object)
	if obj > 0 {
		dx := targetX - threatX
		dy := targetY - threatY
		invLen := 1.0 / math.Max(1e-6, math.Hypot(dx, dy))
		obj *= directionalObjectFactor(tm, col, row, t.Object, dx*invLen, dy*invLen)
	}
	total := ground + obj
	if total > 0.90 {
		total = 0.90
	}
	return total
}

func directionalObjectFactor(tm *TileMap, col, row int, obj ObjectType, inX, inY float64) float64 {
	minFactor, directional := directionalCoverProfile(obj)
	if !directional {
		return 1.0
	}

	hasL := col > 0 && tm.ObjectAt(col-1, row) == obj
	hasR := col < tm.Cols-1 && tm.ObjectAt(col+1, row) == obj
	hasU := row > 0 && tm.ObjectAt(col, row-1) == obj
	hasD := row < tm.Rows-1 && tm.ObjectAt(col, row+1) == obj

	perp := 0.75 // default for isolated/ambiguous pieces
	horizontal := (hasL || hasR) && !(hasU || hasD)
	vertical := (hasU || hasD) && !(hasL || hasR)
	switch {
	case horizontal:
		// Horizontal run means cover face is mostly north/south.
		perp = math.Abs(inY)
	case vertical:
		// Vertical run means cover face is mostly east/west.
		perp = math.Abs(inX)
	default:
		// Isolated or junction: retain moderate directional benefit.
		perp = 0.5 + 0.5*math.Max(math.Abs(inX), math.Abs(inY))
	}

	return minFactor + (1.0-minFactor)*clamp01(perp)
}

func directionalCoverProfile(obj ObjectType) (minFactor float64, directional bool) {
	switch obj {
	case ObjectATBarrier:
		return 0.35, true
	case ObjectSandbag, ObjectChestWall:
		return 0.45, true
	case ObjectFence:
		return 0.20, true
	case ObjectHedgerow:
		return 0.60, true
	default:
		return 1.0, false
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// FindTileMapCoverForThreat searches nearby passable tiles and returns the best
// standing position that provides cover against a threat direction.
//
// threatAngle is the angle from soldier -> threat (radians).
func FindTileMapCoverForThreat(tm *TileMap, sx, sy, threatAngle, maxSearchDist float64) (px, py, defence float64, ok bool) {
	if tm == nil {
		return 0, 0, 0, false
	}

	sc, sr := WorldToCell(sx, sy)
	if !tm.inBounds(sc, sr) {
		return 0, 0, 0, false
	}

	radiusCells := int(maxSearchDist / float64(cellSize))
	if radiusCells < 1 {
		radiusCells = 1
	}

	// Synthetic distant threat point used for directional cover tests.
	tx := sx + math.Cos(threatAngle)*maxSearchDist*2.0
	ty := sy + math.Sin(threatAngle)*maxSearchDist*2.0

	bestScore := -math.MaxFloat64
	bestX, bestY, bestDef := 0.0, 0.0, 0.0

	for dr := -radiusCells; dr <= radiusCells; dr++ {
		for dc := -radiusCells; dc <= radiusCells; dc++ {
			c := sc + dc
			r := sr + dr
			if !tm.inBounds(c, r) || !tm.IsPassable(c, r) {
				continue
			}

			cx, cy := CellToWorld(c, r)
			dx := cx - sx
			dy := cy - sy
			dist := math.Hypot(dx, dy)
			if dist > maxSearchDist || dist < float64(cellSize)*0.25 {
				continue
			}

			inCover, def := TileMapCoverBetween(tm, cx, cy, tx, ty)
			if !inCover || def < 0.10 {
				continue
			}

			// Prefer higher defence, then shorter movement, and slightly prefer
			// tiles with modest movement cost so soldiers don't route into wire/mud.
			moveCost := tm.MovementCost(c, r)
			if moveCost <= 0 {
				continue
			}
			score := def*2.1 - (dist/maxSearchDist)*0.75 - (1.0-moveCost)*0.25
			if score > bestScore {
				bestScore = score
				bestX, bestY, bestDef = cx, cy, def
			}
		}
	}

	if bestScore == -math.MaxFloat64 {
		return 0, 0, 0, false
	}
	return bestX, bestY, bestDef, true
}
