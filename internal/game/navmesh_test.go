package game

import (
	"math"
	"testing"
)

func TestNavGrid_UnblockedByDefault(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, 0)
	if ng.IsBlocked(0, 0) {
		t.Fatal("empty grid should have no blocked cells")
	}
	if ng.IsBlocked(ng.cols-1, ng.rows-1) {
		t.Fatal("corner cell should not be blocked")
	}
}

func TestNavGrid_BuildingBlocksCells(t *testing.T) {
	// Building at pixel (64,64) size 64×64. cellSize=16, so cells (4,4)-(7,7).
	buildings := []rect{{x: 64, y: 64, w: 64, h: 64}}
	ng := NewNavGrid(640, 480, buildings, 0)
	if !ng.IsBlocked(4, 4) {
		t.Fatal("cell inside building should be blocked")
	}
	if !ng.IsBlocked(7, 7) {
		t.Fatal("cell at far corner of building should be blocked")
	}
}

func TestNavGrid_PaddingBlocksAdjacentCells(t *testing.T) {
	buildings := []rect{{x: 64, y: 64, w: 64, h: 64}}
	pad := 8
	ng := NewNavGrid(640, 480, buildings, pad)
	// Cell just outside the building but within pad should be blocked.
	// Building starts at x=64, pad=8 → padded start at x=56 → cell 56/16=3.
	if !ng.IsBlocked(3, 4) {
		t.Fatal("cell within soldier-radius padding should be blocked")
	}
}

func TestNavGrid_OOB_IsBlocked(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, 0)
	if !ng.IsBlocked(-1, 0) {
		t.Fatal("out-of-bounds cell should be blocked")
	}
	if !ng.IsBlocked(0, -1) {
		t.Fatal("out-of-bounds cell should be blocked")
	}
	if !ng.IsBlocked(ng.cols, 0) {
		t.Fatal("out-of-bounds cell should be blocked")
	}
}

func TestWorldToCell(t *testing.T) {
	cx, cy := WorldToCell(24, 40)
	// cellSize=16: 24/16=1, 40/16=2
	if cx != 1 || cy != 2 {
		t.Fatalf("expected (1,2) got (%d,%d)", cx, cy)
	}
}

func TestCellToWorld(t *testing.T) {
	wx, wy := CellToWorld(2, 3)
	// center of cell (2,3): 2*16+8=40, 3*16+8=56
	if wx != 40 || wy != 56 {
		t.Fatalf("expected (40,56) got (%.0f,%.0f)", wx, wy)
	}
}

func TestNavGrid_FindPath_Straight(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, 0)
	path := ng.FindPath(8, 8, 600, 8)
	if path == nil {
		t.Fatal("expected a path on open grid")
	}
	if len(path) < 2 {
		t.Fatal("expected at least 2 waypoints")
	}
	// Last waypoint should be near goal.
	last := path[len(path)-1]
	dx := last[0] - 600
	dy := last[1] - 8
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist > float64(cellSize)*2 {
		t.Fatalf("last waypoint (%.0f,%.0f) too far from goal (600,8)", last[0], last[1])
	}
}

func TestNavGrid_FindPath_AroundBuilding(t *testing.T) {
	// Building blocks the direct path but leaves a gap at the bottom.
	buildings := []rect{{x: 200, y: 0, w: 32, h: 300}}
	ng := NewNavGrid(640, 480, buildings, 0)
	// Path from left side to right side; must route below the building.
	path := ng.FindPath(8, 100, 600, 100)
	if path == nil {
		t.Fatal("expected a path routing around the building")
	}
}

func TestNavGrid_FindPath_NoPath(t *testing.T) {
	// Block the start cell: building covers the top row entirely.
	buildingsFull := []rect{{x: 0, y: 0, w: 640, h: 16}}
	ng := NewNavGrid(640, 480, buildingsFull, 0)
	path := ng.FindPath(8, 8, 8, 400)
	if path != nil {
		t.Fatal("expected nil path when start is blocked")
	}
}

func TestNavGrid_FindPath_StartEqualsGoal(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, 0)
	// Same start and goal: A* should return a trivial path or nil (both valid).
	_ = ng.FindPath(100, 100, 100, 100)
	// Just ensure no panic.
}

func TestNavGrid_FindPath_Deterministic(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, 0)
	p1 := ng.FindPath(8, 8, 600, 400)
	p2 := ng.FindPath(8, 8, 600, 400)
	if len(p1) != len(p2) {
		t.Fatalf("path lengths differ between identical calls: %d vs %d", len(p1), len(p2))
	}
}
