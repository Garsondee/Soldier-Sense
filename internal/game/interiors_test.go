package game

import (
	"math/rand"
	"testing"
)

func TestFurnishBuilding_PlacesFurniture(t *testing.T) {
	tm := NewTileMap(30, 30)
	rng := rand.New(rand.NewSource(42))

	// Simulate a building footprint at (64,64) of size 320x320 (20x20 tiles).
	fp := rect{x: 64, y: 64, w: 320, h: 320}

	// Mark footprint as indoor concrete.
	for r := fp.y / cellSize; r <= (fp.y+fp.h-1)/cellSize; r++ {
		for c := fp.x / cellSize; c <= (fp.x+fp.w-1)/cellSize; c++ {
			tm.SetGround(c, r, GroundConcrete)
			tm.AddFlag(c, r, TileFlagIndoor)
		}
	}

	// Create some interior rooms.
	rooms := []interiorRoom{
		{rx: 80, ry: 80, rw: 128, rh: 128},
		{rx: 224, ry: 80, rw: 128, rh: 128},
		{rx: 80, ry: 224, rw: 272, rh: 128},
	}

	furnishBuilding(tm, rng, fp, rooms)

	// Count objects placed.
	counts := make(map[ObjectType]int)
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			obj := tm.ObjectAt(col, row)
			if obj != ObjectNone {
				counts[obj]++
			}
		}
	}

	t.Logf("Objects placed: %v", counts)

	// Should have placed at least some objects.
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		t.Fatal("furnishBuilding placed no objects at all")
	}
}

func TestFurnishRoom_SmallRoomSafe(t *testing.T) {
	tm := NewTileMap(10, 10)
	rng := rand.New(rand.NewSource(99))

	// Very small room — should not panic.
	rm := interiorRoom{rx: 16, ry: 16, rw: 16, rh: 16} // 1x1 tile room
	furnishRoom(tm, rng, rm)
	// No assertions needed — just shouldn't panic.
}

func TestPlaceDoorInDoorway(t *testing.T) {
	tm := NewTileMap(10, 10)
	rng := rand.New(rand.NewSource(42))

	// Place an exterior door.
	placeDoorInDoorway(tm, rng, 64, 64, 64, true)

	// Check that a door object was placed at the centre of the gap.
	dc := (64 + 32) / cellSize // 6
	dr := (64 + 32) / cellSize // 6
	obj := tm.ObjectAt(dc, dr)
	if obj != ObjectDoor && obj != ObjectDoorOpen {
		t.Fatalf("expected door at (%d,%d), got object type %d", dc, dr, obj)
	}
}

func TestFurnishBuilding_FloorTypeVariation(t *testing.T) {
	// Run with multiple seeds and check we get different floor types.
	floorTypes := make(map[GroundType]bool)
	for seed := int64(0); seed < 50; seed++ {
		tm := NewTileMap(20, 20)
		rng := rand.New(rand.NewSource(seed))
		fp := rect{x: 32, y: 32, w: 256, h: 256}
		for r := fp.y / cellSize; r <= (fp.y+fp.h-1)/cellSize; r++ {
			for c := fp.x / cellSize; c <= (fp.x+fp.w-1)/cellSize; c++ {
				tm.AddFlag(c, r, TileFlagIndoor)
			}
		}
		furnishBuilding(tm, rng, fp, nil)
		// Sample the floor type at the centre of the footprint.
		cc := (fp.x + fp.w/2) / cellSize
		cr := (fp.y + fp.h/2) / cellSize
		floorTypes[tm.Ground(cc, cr)] = true
	}
	if len(floorTypes) < 2 {
		t.Fatalf("expected at least 2 different floor types across seeds, got %v", floorTypes)
	}
	t.Logf("Floor types seen: %v", floorTypes)
}
