package game

import (
	"math/rand"
	"testing"
)

func TestGenerateFortifications_PlacesObjects(t *testing.T) {
	tm := NewTileMap(100, 60)
	rng := rand.New(rand.NewSource(42))
	generateFortifications(tm, rng, defaultFortConfig)

	counts := make(map[ObjectType]int)
	trenchTiles := 0
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			obj := tm.ObjectAt(col, row)
			if obj != ObjectNone {
				counts[obj]++
			}
			if tm.At(col, row).Flags&TileFlagTrench != 0 {
				trenchTiles++
			}
		}
	}

	t.Logf("Fortification objects: %v", counts)
	t.Logf("Trench tiles: %d", trenchTiles)

	if counts[ObjectSlitTrench] == 0 {
		t.Fatal("expected slit trenches")
	}
	if counts[ObjectSandbag] == 0 {
		t.Fatal("expected sandbags")
	}
	if counts[ObjectWire] == 0 {
		t.Fatal("expected wire")
	}
	if trenchTiles == 0 {
		t.Fatal("expected trench flag set on trench tiles")
	}
}

func TestFortCanPlace_RespectsRoadsAndBuildings(t *testing.T) {
	tm := NewTileMap(10, 10)

	// Road tile.
	tm.SetGround(5, 5, GroundTarmac)
	if fortCanPlace(tm, 5, 5) {
		t.Fatal("should not place on road")
	}

	// Indoor tile.
	tm.AddFlag(3, 3, TileFlagIndoor)
	if fortCanPlace(tm, 3, 3) {
		t.Fatal("should not place indoors")
	}

	// Occupied tile.
	tm.SetObject(1, 1, ObjectWall)
	if fortCanPlace(tm, 1, 1) {
		t.Fatal("should not place on existing object")
	}

	// Clear tile.
	if !fortCanPlace(tm, 0, 0) {
		t.Fatal("should be able to place on clear grass")
	}
}

func TestTrenchElevation(t *testing.T) {
	tm := NewTileMap(20, 20)
	rng := rand.New(rand.NewSource(42))
	placeTrenchLine(tm, rng, 4, 8)

	found := false
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			if tm.ObjectAt(col, row) == ObjectSlitTrench {
				tile := tm.At(col, row)
				if tile.Elevation != -1 {
					t.Fatalf("trench at (%d,%d) should have elevation -1, got %d", col, row, tile.Elevation)
				}
				if tile.Flags&TileFlagTrench == 0 {
					t.Fatalf("trench at (%d,%d) should have trench flag", col, row)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("no trench tiles placed")
	}
}
