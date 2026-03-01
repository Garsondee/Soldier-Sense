package game

import (
	"math"
	"testing"
)

func TestVisionCone_InCone_Directly_Ahead(t *testing.T) {
	v := NewVisionState(0) // heading = 0 (right)
	// Target directly ahead at (100, 0), observer at (0, 0)
	if !v.InCone(0, 0, 100, 0) {
		t.Fatal("target directly in front should be in cone")
	}
}

func TestVisionCone_Behind_Observer(t *testing.T) {
	v := NewVisionState(0) // heading = right
	// Target directly behind at (-100, 0)
	if v.InCone(0, 0, -100, 0) {
		t.Fatal("target directly behind should not be in cone")
	}
}

func TestVisionCone_Edge_Of_FOV(t *testing.T) {
	v := NewVisionState(0)
	halfFOV := v.FOV / 2.0
	// Target exactly at half-FOV boundary (just inside)
	ex := math.Cos(halfFOV-0.001) * 100
	ey := math.Sin(halfFOV-0.001) * 100
	if !v.InCone(0, 0, ex, ey) {
		t.Fatal("target just inside FOV edge should be in cone")
	}
	// Target just outside FOV boundary
	ox := math.Cos(halfFOV+0.001) * 100
	oy := math.Sin(halfFOV+0.001) * 100
	if v.InCone(0, 0, ox, oy) {
		t.Fatal("target just outside FOV edge should not be in cone")
	}
}

func TestVisionCone_MaxRange(t *testing.T) {
	v := NewVisionState(0)
	// Target at exactly MaxRange - should NOT be in cone (dist not > MaxRange, but == check)
	// Target at MaxRange+1 — must be excluded
	if v.InCone(0, 0, v.MaxRange+1, 0) {
		t.Fatal("target beyond max range should not be in cone")
	}
}

func TestVisionCone_TooClose(t *testing.T) {
	v := NewVisionState(0)
	// dist < 1e-6 — same position
	if v.InCone(50, 50, 50, 50) {
		t.Fatal("zero-distance target should not be in cone")
	}
}

func TestUpdateHeading_SmallDiff(t *testing.T) {
	v := NewVisionState(0)
	// Diff smaller than turnRate — should snap to target.
	v.UpdateHeading(0.05, 0.12)
	if v.Heading != 0.05 {
		t.Fatalf("expected heading to snap to 0.05, got %.4f", v.Heading)
	}
}

func TestUpdateHeading_LargeDiff_Positive(t *testing.T) {
	v := NewVisionState(0)
	rate := 0.12
	v.UpdateHeading(math.Pi, rate)
	// Should step by rate in the positive direction
	expected := normalizeAngle(0 + rate)
	if math.Abs(v.Heading-expected) > 1e-9 {
		t.Fatalf("expected heading %.4f got %.4f", expected, v.Heading)
	}
}

func TestUpdateHeading_LargeDiff_Negative(t *testing.T) {
	v := NewVisionState(0)
	rate := 0.12
	v.UpdateHeading(-math.Pi, rate)
	// Should step by rate in the negative direction
	expected := normalizeAngle(0 - rate)
	if math.Abs(v.Heading-expected) > 1e-9 {
		t.Fatalf("expected heading %.4f got %.4f", expected, v.Heading)
	}
}

func TestNormalizeAngle_Positive(t *testing.T) {
	a := normalizeAngle(3 * math.Pi)
	if math.Abs(a+math.Pi) > 1e-9 {
		t.Fatalf("3π should normalize to -π, got %.4f", a)
	}
}

func TestNormalizeAngle_Negative(t *testing.T) {
	a := normalizeAngle(-3 * math.Pi)
	// -3π → -π (or +π, both equivalent)
	if math.Abs(math.Abs(a)-math.Pi) > 1e-9 {
		t.Fatalf("-3π should normalize to ±π, got %.4f", a)
	}
}

func TestNormalizeAngle_Zero(t *testing.T) {
	if normalizeAngle(0) != 0 {
		t.Fatal("0 should normalize to 0")
	}
}

func TestHeadingTo(t *testing.T) {
	h := HeadingTo(0, 0, 1, 0)
	if h != 0 {
		t.Fatalf("heading to east should be 0, got %.4f", h)
	}
	h = HeadingTo(0, 0, 0, 1)
	if math.Abs(h-math.Pi/2) > 1e-9 {
		t.Fatalf("heading to south should be π/2, got %.4f", h)
	}
}

func TestPerformVisionScan_NoBuildings(t *testing.T) {
	v := NewVisionState(0)
	tick := 0
	observer := NewSoldier(0, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	target := NewSoldier(1, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{0, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	v.PerformVisionScan(0, 0, []*Soldier{target}, nil, nil)
	if len(v.KnownContacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(v.KnownContacts))
	}
	_ = observer
}

func TestPerformVisionScan_BlockedByBuilding(t *testing.T) {
	v := NewVisionState(0)
	buildings := []rect{{x: 50, y: -20, w: 10, h: 40}}
	tick := 0
	target := NewSoldier(1, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{0, 0},
		NewNavGrid(640, 480, buildings, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	v.PerformVisionScan(0, 0, []*Soldier{target}, buildings, nil)
	if len(v.KnownContacts) != 0 {
		t.Fatalf("expected 0 contacts (building blocks LOS), got %d", len(v.KnownContacts))
	}
}
