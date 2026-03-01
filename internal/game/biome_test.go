package game

import (
	"math/rand"
	"testing"
)

func TestValueNoise2D_Range(t *testing.T) {
	// Verify noise output is in [0,1].
	seed := int64(12345)
	for y := -10.0; y < 10.0; y += 0.37 {
		for x := -10.0; x < 10.0; x += 0.37 {
			v := valueNoise2D(x, y, seed)
			if v < 0 || v > 1 {
				t.Fatalf("noise at (%.2f,%.2f) = %f, out of [0,1]", x, y, v)
			}
		}
	}
}

func TestValueNoise2D_Deterministic(t *testing.T) {
	seed := int64(99999)
	a := valueNoise2D(3.7, 8.2, seed)
	b := valueNoise2D(3.7, 8.2, seed)
	if a != b {
		t.Fatalf("noise not deterministic: %f != %f", a, b)
	}
}

func TestGenerateBiome_PlacesVegetation(t *testing.T) {
	tm := NewTileMap(100, 60)
	rng := rand.New(rand.NewSource(42))
	generateBiome(tm, rng, defaultBiomeConfig)

	counts := make(map[ObjectType]int)
	groundCounts := make(map[GroundType]int)
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			obj := tm.ObjectAt(col, row)
			if obj != ObjectNone {
				counts[obj]++
			}
			groundCounts[tm.Ground(col, row)]++
		}
	}

	t.Logf("Objects: %v", counts)
	t.Logf("Grounds: %v", groundCounts)

	// Should have placed at least some vegetation.
	vegCount := counts[ObjectTreeTrunk] + counts[ObjectTreeCanopy] + counts[ObjectBush] + counts[ObjectHedgerow]
	if vegCount == 0 {
		t.Fatal("generateBiome placed no vegetation at all")
	}

	// Should have varied ground types beyond just grass.
	if len(groundCounts) < 3 {
		t.Fatalf("expected at least 3 ground types, got %d", len(groundCounts))
	}
}

func TestGenerateBiome_SkipsRoadsAndBuildings(t *testing.T) {
	tm := NewTileMap(50, 50)
	rng := rand.New(rand.NewSource(42))

	// Place a road and building first.
	for c := 0; c < 50; c++ {
		tm.SetGround(c, 25, GroundTarmac)
	}
	for r := 10; r < 20; r++ {
		for c := 10; c < 20; c++ {
			tm.SetGround(c, r, GroundConcrete)
			tm.AddFlag(c, r, TileFlagIndoor)
		}
	}

	generateBiome(tm, rng, defaultBiomeConfig)

	// Road tiles should still be tarmac.
	for c := 0; c < 50; c++ {
		if tm.Ground(c, 25) != GroundTarmac {
			t.Fatalf("road tile at (%d,25) was changed to %d", c, tm.Ground(c, 25))
		}
	}

	// Building tiles should still be concrete + indoor.
	for r := 10; r < 20; r++ {
		for c := 10; c < 20; c++ {
			if tm.Ground(c, r) != GroundConcrete {
				t.Fatalf("building tile at (%d,%d) was changed to %d", c, r, tm.Ground(c, r))
			}
			if !tm.IsIndoor(c, r) {
				t.Fatalf("building tile at (%d,%d) lost indoor flag", c, r)
			}
		}
	}
}

func TestCanPlaceTree_RespectsBuildings(t *testing.T) {
	tm := NewTileMap(10, 10)
	// Mark centre as indoor.
	tm.AddFlag(5, 5, TileFlagIndoor)
	if canPlaceTree(tm, 5, 5) {
		t.Fatal("should not place tree on indoor tile")
	}
	// Adjacent to indoor.
	if canPlaceTree(tm, 4, 5) {
		t.Fatal("should not place tree adjacent to indoor tile")
	}
}
