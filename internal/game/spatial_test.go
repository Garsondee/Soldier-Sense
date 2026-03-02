package game

import (
	"math"
	"testing"
)

func TestSpatialHash_Insert_Query(t *testing.T) {
	sh := NewSpatialHash(100.0)

	// Create test soldiers at various positions
	soldiers := []*Soldier{
		{id: 1, x: 0, y: 0},
		{id: 2, x: 50, y: 50},
		{id: 3, x: 200, y: 200},
		{id: 4, x: 500, y: 500},
	}

	// Insert all soldiers
	for _, s := range soldiers {
		sh.Insert(s)
	}

	// Query near origin - should find soldiers 1 and 2
	results := sh.QueryRadius(0, 0, 100)
	if len(results) != 2 {
		t.Errorf("Expected 2 soldiers near origin, got %d", len(results))
	}

	// Verify correct soldiers found
	foundIDs := make(map[int]bool)
	for _, s := range results {
		foundIDs[s.id] = true
	}
	if !foundIDs[1] || !foundIDs[2] {
		t.Errorf("Expected to find soldiers 1 and 2, got IDs: %v", foundIDs)
	}

	// Query far away - should find soldier 4
	results = sh.QueryRadius(500, 500, 50)
	if len(results) != 1 {
		t.Errorf("Expected 1 soldier at (500,500), got %d", len(results))
	}
	if results[0].id != 4 {
		t.Errorf("Expected soldier 4, got soldier %d", results[0].id)
	}
}

func TestSpatialHash_Clear(t *testing.T) {
	sh := NewSpatialHash(100.0)

	s := &Soldier{id: 1, x: 0, y: 0}
	sh.Insert(s)

	results := sh.QueryRadius(0, 0, 100)
	if len(results) != 1 {
		t.Errorf("Expected 1 soldier before clear, got %d", len(results))
	}

	sh.Clear()
	results = sh.QueryRadius(0, 0, 100)
	if len(results) != 0 {
		t.Errorf("Expected 0 soldiers after clear, got %d", len(results))
	}
}

func TestSpatialHash_Performance(t *testing.T) {
	sh := NewSpatialHash(200.0)

	// Create many soldiers spread across a large area
	const numSoldiers = 100
	soldiers := make([]*Soldier, numSoldiers)
	for i := 0; i < numSoldiers; i++ {
		soldiers[i] = &Soldier{
			id: i,
			x:  float64(i * 50),
			y:  float64(i * 50),
		}
		sh.Insert(soldiers[i])
	}

	// Query should only return nearby soldiers, not all 100
	results := sh.QueryRadius(0, 0, 300)

	// Should find soldiers within ~300 pixels
	// At (0,0), (50,50), (100,100), (150,150), (200,200), (250,250)
	// Distance check: sqrt(x^2 + y^2) <= 300
	expectedCount := 0
	for _, s := range soldiers {
		dist := math.Hypot(s.x, s.y)
		if dist <= 300 {
			expectedCount++
		}
	}

	if len(results) != expectedCount {
		t.Errorf("Expected %d soldiers within radius, got %d", expectedCount, len(results))
	}

	// Verify all results are actually within range
	for _, s := range results {
		dist := math.Hypot(s.x, s.y)
		if dist > 300 {
			t.Errorf("Soldier %d at distance %.1f should not be in results (max 300)", s.id, dist)
		}
	}
}

func TestSpatialHash_EdgeCases(t *testing.T) {
	sh := NewSpatialHash(100.0)

	// Empty hash
	results := sh.QueryRadius(0, 0, 100)
	if len(results) != 0 {
		t.Errorf("Expected 0 results from empty hash, got %d", len(results))
	}

	// Negative coordinates
	s := &Soldier{id: 1, x: -100, y: -100}
	sh.Insert(s)
	results = sh.QueryRadius(-100, -100, 50)
	if len(results) != 1 {
		t.Errorf("Expected 1 soldier at negative coords, got %d", len(results))
	}

	// Very large coordinates
	s2 := &Soldier{id: 2, x: 10000, y: 10000}
	sh.Insert(s2)
	results = sh.QueryRadius(10000, 10000, 50)
	if len(results) != 1 {
		t.Errorf("Expected 1 soldier at large coords, got %d", len(results))
	}
}
