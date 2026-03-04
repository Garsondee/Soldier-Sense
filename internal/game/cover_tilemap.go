package game

import "math"

type lineWalkState struct {
	col, row int
	xStep    int
	yStep    int
	err      int
	step     int
}

type coverSearchBest struct {
	score float64
	x     float64
	y     float64
	def   float64
}

// TileMapCoverBetween returns whether the target at (sx,sy) has meaningful cover
// from a threat at (tx,ty), and the best defense fraction found.
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

	walk := newLineWalkState(sc, sr, tc, tr, dc, dr)

	best := 0.0
	found := false
	for {
		found, best = evaluateCoverLineCell(tm, walk.col, walk.row, walk.step, totalSteps, sc, sr, tx, ty, sx, sy, found, best)

		if walk.col == sc && walk.row == sr {
			break
		}
		advanceLineWalkState(&walk, dc, dr)
	}

	if best > 0.90 {
		best = 0.90
	}
	return found && best > 0.01, best
}

func newLineWalkState(sc, sr, tc, tr, dc, dr int) lineWalkState {
	walk := lineWalkState{col: tc, row: tr, xStep: -1, yStep: -1, err: dc - dr}
	if tc < sc {
		walk.xStep = 1
	}
	if tr < sr {
		walk.yStep = 1
	}
	return walk
}

func evaluateCoverLineCell(
	tm *TileMap,
	col, row, step, totalSteps, sc, sr int,
	tx, ty, sx, sy float64,
	found bool,
	best float64,
) (bool, float64) {
	if !tm.inBounds(col, row) {
		return found, best
	}
	frac := float64(step) / float64(totalSteps)
	if frac < 0.35 && !(col == sc && row == sr) {
		return found, best
	}
	cv := directionalTileCover(tm, col, row, tx, ty, sx, sy)
	if cv <= best {
		return found, best
	}
	return true, cv
}

func advanceLineWalkState(walk *lineWalkState, dc, dr int) {
	e2 := walk.err * 2
	if e2 > -dr {
		walk.err -= dr
		walk.col += walk.xStep
	}
	if e2 < dc {
		walk.err += dc
		walk.row += walk.yStep
	}
	walk.step++
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

	var perp float64 // set by switch below
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

// FindTileMapCoverForThreat searches nearby passable tiles and returns the best.
// Standing position that provides cover against a threat direction.
//
// ThreatAngle is the angle from soldier -> threat (radians).
func FindTileMapCoverForThreat(tm *TileMap, sx, sy, threatAngle, maxSearchDist float64) (px, py, defense float64, ok bool) {
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

	best := coverSearchBest{score: -math.MaxFloat64}

	for dr := -radiusCells; dr <= radiusCells; dr++ {
		for dc := -radiusCells; dc <= radiusCells; dc++ {
			updated, candidate := evaluateTileMapCoverCandidate(tm, sc+dc, sr+dr, sx, sy, tx, ty, maxSearchDist)
			if !updated {
				continue
			}
			if candidate.score > best.score {
				best = candidate
			}
		}
	}

	if best.score == -math.MaxFloat64 {
		return 0, 0, 0, false
	}
	return best.x, best.y, best.def, true
}

func evaluateTileMapCoverCandidate(
	tm *TileMap,
	c, r int,
	sx, sy, tx, ty, maxSearchDist float64,
) (bool, coverSearchBest) {
	if !tm.inBounds(c, r) || !tm.IsPassable(c, r) {
		return false, coverSearchBest{}
	}

	cx, cy := CellToWorld(c, r)
	dx := cx - sx
	dy := cy - sy
	dist := math.Hypot(dx, dy)
	if dist > maxSearchDist || dist < float64(cellSize)*0.25 {
		return false, coverSearchBest{}
	}

	inCover, def := TileMapCoverBetween(tm, cx, cy, tx, ty)
	if !inCover || def < 0.10 {
		return false, coverSearchBest{}
	}

	moveCost := tm.MovementCost(c, r)
	if moveCost <= 0 {
		return false, coverSearchBest{}
	}

	score := def*2.1 - (dist/maxSearchDist)*0.75 - (1.0-moveCost)*0.25
	return true, coverSearchBest{score: score, x: cx, y: cy, def: def}
}
