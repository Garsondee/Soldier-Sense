package game

import (
	"math"
	"testing"
)

func TestFormationOffsets_LeaderAlwaysZero(t *testing.T) {
	for _, ft := range []FormationType{FormationLine, FormationWedge, FormationColumn, FormationEchelon} {
		offsets := formationOffsets(ft, 6)
		if offsets[0][0] != 0 || offsets[0][1] != 0 {
			t.Fatalf("formation %d: leader slot 0 should be (0,0), got (%.1f,%.1f)",
				ft, offsets[0][0], offsets[0][1])
		}
	}
}

func TestFormationOffsets_Count(t *testing.T) {
	for _, count := range []int{0, 1, 3, 6} {
		offsets := formationOffsets(FormationWedge, count)
		if len(offsets) != count {
			t.Fatalf("expected %d offsets, got %d", count, len(offsets))
		}
	}
}

func TestFormationOffsets_Column_SingleFile(t *testing.T) {
	offsets := formationOffsets(FormationColumn, 4)
	// Column: members go directly behind leader (right=0).
	for i := 1; i < 4; i++ {
		if offsets[i][1] != 0 {
			t.Fatalf("column slot %d: right offset should be 0, got %.1f", i, offsets[i][1])
		}
		if offsets[i][0] >= 0 {
			t.Fatalf("column slot %d: forward offset should be negative (behind), got %.1f", i, offsets[i][0])
		}
	}
}

func TestFormationOffsets_Wedge_MembersTrailBack(t *testing.T) {
	offsets := formationOffsets(FormationWedge, 5)
	// All non-leader members should trail behind (forward offset < 0).
	for i := 1; i < 5; i++ {
		if offsets[i][0] >= 0 {
			t.Fatalf("wedge slot %d: forward offset should be negative, got %.1f", i, offsets[i][0])
		}
	}
}

func TestFormationOffsets_Line_SameForwardDepth(t *testing.T) {
	offsets := formationOffsets(FormationLine, 5)
	// All non-leader slots in a line should be at forward=0.
	for i := 1; i < 5; i++ {
		if offsets[i][0] != 0 {
			t.Fatalf("line slot %d: forward offset should be 0, got %.1f", i, offsets[i][0])
		}
	}
}

func TestSlotWorld_FacingEast(t *testing.T) {
	// Leader at (100,100), heading=0 (east).
	// fwd=0, right=28 (one slot to the right in world space).
	wx, wy := SlotWorld(100, 100, 0, 0, 28)
	// heading=0: forward=(1,0), right=(0,1) — actually right is 90° clockwise from forward.
	// right unit = (-sin(0), cos(0)) = (0,1)
	// wx = 100 + 1*0 + 0*28 = 100
	// wy = 100 + 0*0 + 1*28 = 128
	if math.Abs(wx-100) > 1e-9 || math.Abs(wy-128) > 1e-9 {
		t.Fatalf("expected (100,128), got (%.2f,%.2f)", wx, wy)
	}
}

func TestSlotWorld_FacingSouth(t *testing.T) {
	// Leader at (100,100), heading=π/2 (south / +y).
	wx, wy := SlotWorld(100, 100, math.Pi/2, slotSpacing, 0)
	// forward = (cos(π/2), sin(π/2)) = (0,1)
	// wy should increase by slotSpacing
	if math.Abs(wx-100) > 0.01 || math.Abs(wy-(100+slotSpacing)) > 0.01 {
		t.Fatalf("expected (100,%.1f), got (%.2f,%.2f)", 100+slotSpacing, wx, wy)
	}
}

func TestSlotWorld_LeaderOrigin(t *testing.T) {
	// fwd=0, right=0 should return leader position unchanged.
	wx, wy := SlotWorld(200, 300, 1.23, 0, 0)
	if math.Abs(wx-200) > 1e-9 || math.Abs(wy-300) > 1e-9 {
		t.Fatalf("zero offset should return leader pos (200,300), got (%.2f,%.2f)", wx, wy)
	}
}
