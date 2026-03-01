package game

import (
	"math"
	"testing"
)

func TestTileMapCoverBetween_BushProvidesQuarterCover(t *testing.T) {
	tm := NewTileMap(20, 10)
	// Target stands in bush.
	tm.SetObject(10, 5, ObjectBush)

	sx, sy := CellToWorld(10, 5)
	tx, ty := CellToWorld(2, 5)
	inCover, def := TileMapCoverBetween(tm, sx, sy, tx, ty)
	if !inCover {
		t.Fatal("expected bush tile to provide cover")
	}
	if math.Abs(def-0.25) > 1e-6 {
		t.Fatalf("expected 0.25 bush cover, got %.3f", def)
	}
}

func TestTileMapCoverBetween_TarmacNoCover(t *testing.T) {
	tm := NewTileMap(20, 10)
	for c := 0; c < tm.Cols; c++ {
		tm.SetGround(c, 5, GroundTarmac)
	}

	sx, sy := CellToWorld(10, 5)
	tx, ty := CellToWorld(2, 5)
	inCover, def := TileMapCoverBetween(tm, sx, sy, tx, ty)
	if inCover || def > 0 {
		t.Fatalf("expected no cover on tarmac, got inCover=%v def=%.3f", inCover, def)
	}
}

func TestTileMapCoverBetween_ATBarrierDirectional(t *testing.T) {
	tm := NewTileMap(20, 20)
	// Vertical barrier run: stronger cover from east/west than north/south.
	for r := 8; r <= 12; r++ {
		tm.SetObject(10, r, ObjectATBarrier)
	}

	sx, sy := CellToWorld(10, 10)
	// Threat from west -> should be high directional effectiveness.
	westX, westY := CellToWorld(2, 10)
	_, westDef := TileMapCoverBetween(tm, sx, sy, westX, westY)
	// Threat from north -> should be weaker against a vertical run.
	northX, northY := CellToWorld(10, 2)
	_, northDef := TileMapCoverBetween(tm, sx, sy, northX, northY)

	if westDef <= northDef {
		t.Fatalf("expected westDef > northDef for vertical AT barrier, got west=%.3f north=%.3f", westDef, northDef)
	}
	if westDef < 0.3 {
		t.Fatalf("expected meaningful frontal AT barrier cover, got %.3f", westDef)
	}
}

func TestFindTileMapCoverForThreat_FindsNearbyCover(t *testing.T) {
	tm := NewTileMap(30, 20)
	// Place a sandbag strip a few tiles ahead of the soldier.
	for c := 12; c <= 14; c++ {
		tm.SetObject(c, 10, ObjectSandbag)
	}

	sx, sy := CellToWorld(8, 10)
	threatAngle := 0.0 // threat to the east
	px, py, def, ok := FindTileMapCoverForThreat(tm, sx, sy, threatAngle, 120)
	if !ok {
		t.Fatal("expected to find tilemap cover candidate")
	}
	if def < 0.20 {
		t.Fatalf("expected candidate defence >= 0.20, got %.3f", def)
	}
	if math.Hypot(px-sx, py-sy) > 120 {
		t.Fatalf("cover candidate beyond max search distance")
	}
}
