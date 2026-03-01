package game

import (
	"math/rand"
	"testing"
)

func TestGenerateGridRoads_StampsTarmac(t *testing.T) {
	tm := NewTileMap(192, 108) // 3072/16 x 1728/16
	rng := rand.New(rand.NewSource(42))
	paths := generateGridRoads(tm, rng, defaultRoadConfig)

	if len(paths) == 0 {
		t.Fatal("expected at least one road path")
	}

	// Count tarmac and pavement tiles.
	tarmac := 0
	pavement := 0
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			switch tm.Ground(col, row) {
			case GroundTarmac:
				tarmac++
			case GroundPavement:
				pavement++
			}
		}
	}

	if tarmac == 0 {
		t.Fatal("no tarmac tiles were stamped")
	}
	t.Logf("roads=%d tarmac=%d pavement=%d total=%d", len(paths), tarmac, pavement, tm.Cols*tm.Rows)

	// Verify roads are grid-aligned: all tarmac tiles should be within bounds.
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			if tm.Ground(col, row) == GroundTarmac {
				if col < 0 || col >= tm.Cols || row < 0 || row >= tm.Rows {
					t.Fatalf("tarmac tile out of bounds at (%d,%d)", col, row)
				}
			}
		}
	}
}

func TestTileOnRoad(t *testing.T) {
	tm := NewTileMap(10, 10)
	tm.SetGround(5, 5, GroundTarmac)
	tm.SetGround(6, 5, GroundPavement)

	if !tileOnRoad(tm, 5, 5) {
		t.Fatal("tarmac tile should be on road")
	}
	if !tileOnRoad(tm, 6, 5) {
		t.Fatal("pavement tile should be on road")
	}
	if tileOnRoad(tm, 0, 0) {
		t.Fatal("grass tile should not be on road")
	}
}

func TestRectOverlapsRoadTiles(t *testing.T) {
	tm := NewTileMap(20, 20)
	// Stamp a road at row 10, cols 5-15.
	for c := 5; c <= 15; c++ {
		tm.SetGround(c, 10, GroundTarmac)
	}

	// Rect that overlaps the road.
	overlap := rect{x: 6 * cellSize, y: 10 * cellSize, w: 2 * cellSize, h: 2 * cellSize}
	if !rectOverlapsRoadTiles(tm, overlap) {
		t.Fatal("rect should overlap road tiles")
	}

	// Rect that doesn't overlap.
	noOverlap := rect{x: 0, y: 0, w: 2 * cellSize, h: 2 * cellSize}
	if rectOverlapsRoadTiles(tm, noOverlap) {
		t.Fatal("rect should not overlap road tiles")
	}
}

func TestBuildingCandidatesAlongGridRoads(t *testing.T) {
	tm := NewTileMap(192, 108)
	rng := rand.New(rand.NewSource(42))
	generateGridRoads(tm, rng, defaultRoadConfig)

	candidates := buildingCandidatesAlongGridRoads(tm, rng, 256, 256, 32, 192)
	if len(candidates) == 0 {
		t.Fatal("expected building candidates near grid roads")
	}
	t.Logf("candidates=%d", len(candidates))

	// All candidates should be within map bounds.
	mapW := tm.Cols * cellSize
	mapH := tm.Rows * cellSize
	for _, c := range candidates {
		if c.x < 0 || c.y < 0 || c.x+c.w > mapW || c.y+c.h > mapH {
			t.Fatalf("candidate out of bounds: %+v (map %dx%d)", c, mapW, mapH)
		}
	}
}
