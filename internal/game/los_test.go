package game

import "testing"

func TestLOS_ClearLine(t *testing.T) {
	buildings := []rect{}
	if !HasLineOfSight(0, 0, 100, 100, buildings) {
		t.Fatal("expected clear LOS with no buildings")
	}
}

func TestLOS_BlockedByBuilding(t *testing.T) {
	buildings := []rect{{x: 40, y: 0, w: 20, h: 200}}
	// Ray from left to right passes through the building.
	if HasLineOfSight(0, 100, 200, 100, buildings) {
		t.Fatal("expected LOS blocked by building")
	}
}

func TestLOS_AdjacentBuilding_NotBlocked(t *testing.T) {
	// Building is entirely to the right of the ray's endpoint.
	buildings := []rect{{x: 300, y: 0, w: 64, h: 64}}
	if !HasLineOfSight(0, 32, 200, 32, buildings) {
		t.Fatal("building beyond endpoint should not block LOS")
	}
}

func TestLOS_VerticalRay_Blocked(t *testing.T) {
	buildings := []rect{{x: 0, y: 40, w: 200, h: 20}}
	if HasLineOfSight(100, 0, 100, 200, buildings) {
		t.Fatal("expected vertical ray blocked by horizontal building")
	}
}

func TestLOS_HorizontalRay_ClearAbove(t *testing.T) {
	buildings := []rect{{x: 40, y: 50, w: 20, h: 100}}
	// Ray at y=10 passes above the building.
	if !HasLineOfSight(0, 10, 200, 10, buildings) {
		t.Fatal("ray above building should have clear LOS")
	}
}

func TestLOS_DiagonalRay_Blocked(t *testing.T) {
	buildings := []rect{{x: 80, y: 80, w: 40, h: 40}}
	// Diagonal ray from (0,0) to (200,200) passes through building at ~(80-120,80-120).
	if HasLineOfSight(0, 0, 200, 200, buildings) {
		t.Fatal("diagonal ray should be blocked by building")
	}
}

func TestLOS_ZeroLength(t *testing.T) {
	buildings := []rect{{x: 0, y: 0, w: 100, h: 100}}
	// Same start and end: ray is a point. Should not panic.
	_ = HasLineOfSight(50, 50, 50, 50, buildings)
}

func TestRayIntersectsAABB_InsideBox(t *testing.T) {
	// Ray entirely inside box — both endpoints inside.
	result := rayIntersectsAABB(10, 10, 20, 20, 0, 0, 100, 100)
	if !result {
		t.Fatal("ray with both endpoints inside AABB should intersect")
	}
}

func TestRayIntersectsAABB_Miss(t *testing.T) {
	// Ray passes entirely to the left of the box.
	result := rayIntersectsAABB(0, 0, 0, 100, 50, 0, 150, 100)
	if result {
		t.Fatal("ray to the left of AABB should not intersect")
	}
}
