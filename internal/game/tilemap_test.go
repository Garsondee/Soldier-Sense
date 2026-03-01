package game

import "testing"

func TestNewTileMap_DefaultGrass(t *testing.T) {
	tm := NewTileMap(10, 8)
	if tm.Cols != 10 || tm.Rows != 8 {
		t.Fatalf("expected 10x8, got %dx%d", tm.Cols, tm.Rows)
	}
	for row := 0; row < tm.Rows; row++ {
		for col := 0; col < tm.Cols; col++ {
			if g := tm.Ground(col, row); g != GroundGrass {
				t.Fatalf("tile (%d,%d) ground=%d, want GroundGrass", col, row, g)
			}
			if o := tm.ObjectAt(col, row); o != ObjectNone {
				t.Fatalf("tile (%d,%d) object=%d, want ObjectNone", col, row, o)
			}
			if !tm.IsPassable(col, row) {
				t.Fatalf("tile (%d,%d) should be passable", col, row)
			}
		}
	}
}

func TestTileMap_SetWallBlocks(t *testing.T) {
	tm := NewTileMap(5, 5)
	tm.SetObject(2, 2, ObjectWall)
	if tm.IsPassable(2, 2) {
		t.Fatal("wall tile should not be passable")
	}
	if tm.MovementCost(2, 2) != 0 {
		t.Fatal("wall tile movement cost should be 0")
	}
	if tm.LOSOpacity(2, 2) != 1.0 {
		t.Fatal("wall should be fully opaque")
	}
}

func TestTileMap_BreakableWindow(t *testing.T) {
	tm := NewTileMap(5, 5)
	tm.SetObject(1, 1, ObjectWindow)
	if tm.IsPassable(1, 1) {
		t.Fatal("intact window should block movement")
	}
	if tm.LOSOpacity(1, 1) != 0.0 {
		t.Fatal("window should be transparent to LOS")
	}
	// Damage it enough to break.
	tm.DamageTile(1, 1, 50)
	if tm.ObjectAt(1, 1) != ObjectWindowBroken {
		t.Fatalf("window should be broken, got %d", tm.ObjectAt(1, 1))
	}
	if !tm.IsPassable(1, 1) {
		t.Fatal("broken window should be passable")
	}
}

func TestTileMap_BreakableDoor(t *testing.T) {
	tm := NewTileMap(5, 5)
	tm.SetObject(1, 1, ObjectDoor)
	if tm.IsPassable(1, 1) {
		t.Fatal("closed door should block movement")
	}
	if tm.LOSOpacity(1, 1) != 1.0 {
		t.Fatal("closed door should block LOS")
	}
	// Break it.
	tm.DamageTile(1, 1, 100)
	if tm.ObjectAt(1, 1) != ObjectDoorBroken {
		t.Fatalf("door should be broken, got %d", tm.ObjectAt(1, 1))
	}
	if !tm.IsPassable(1, 1) {
		t.Fatal("broken door should be passable")
	}
}

func TestTileMap_MovementCostVariation(t *testing.T) {
	tm := NewTileMap(3, 1)
	tm.SetGround(0, 0, GroundTarmac)
	tm.SetGround(1, 0, GroundMud)
	tm.SetGround(2, 0, GroundRubbleHeavy)

	tarmac := tm.MovementCost(0, 0)
	mud := tm.MovementCost(1, 0)
	rubble := tm.MovementCost(2, 0)

	if tarmac <= mud {
		t.Fatalf("tarmac (%f) should be faster than mud (%f)", tarmac, mud)
	}
	if mud <= rubble {
		t.Fatalf("mud (%f) should be faster than heavy rubble (%f)", mud, rubble)
	}
}

func TestTileMap_CoverValue(t *testing.T) {
	tm := NewTileMap(3, 1)
	tm.SetGround(0, 0, GroundGrass)
	tm.SetGround(1, 0, GroundRubbleHeavy)
	tm.SetObject(2, 0, ObjectSandbag)

	if cv := tm.CoverValue(0, 0); cv != 0.0 {
		t.Fatalf("plain grass cover should be 0, got %f", cv)
	}
	if cv := tm.CoverValue(1, 0); cv < 0.2 {
		t.Fatalf("heavy rubble cover should be >= 0.2, got %f", cv)
	}
	if cv := tm.CoverValue(2, 0); cv < 0.6 {
		t.Fatalf("sandbag cover should be >= 0.6, got %f", cv)
	}
}

func TestTileMap_IndoorFlag(t *testing.T) {
	tm := NewTileMap(5, 5)
	tm.AddFlag(2, 2, TileFlagIndoor)
	if !tm.IsIndoor(2, 2) {
		t.Fatal("tile should be indoor after AddFlag")
	}
	if tm.IsIndoor(0, 0) {
		t.Fatal("tile (0,0) should not be indoor")
	}
}

func TestTileMap_OutOfBounds(t *testing.T) {
	tm := NewTileMap(3, 3)
	if tm.At(-1, 0) != nil {
		t.Fatal("out of bounds At should return nil")
	}
	if tm.IsPassable(-1, 0) {
		t.Fatal("out of bounds should not be passable")
	}
	// Should not panic.
	tm.SetGround(99, 99, GroundTarmac)
	tm.SetObject(-1, -1, ObjectWall)
	tm.DamageTile(99, 99, 100)
}
