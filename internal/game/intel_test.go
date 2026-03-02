package game

import "testing"

func TestIntelStore_SafeTerritory_ExploredLowThreat(t *testing.T) {
	intel := NewIntelStore(256, 256)
	im := intel.For(TeamRed)
	if im == nil {
		t.Fatal("expected intel map")
	}

	// Mark one cell as explored.
	im.ClearSeen(32, 32)

	intel.Update(nil, nil, nil)
	v := im.Layer(IntelSafeTerritory).SampleAt(32, 32)
	if v <= 0 {
		t.Fatalf("expected safe territory > 0 at explored low-threat cell, got %v", v)
	}

	// Introduce enemy contact heat at the same cell and recompute.
	im.WriteContact(32, 32)
	intel.Update(nil, nil, nil)
	v2 := im.Layer(IntelSafeTerritory).SampleAt(32, 32)
	if v2 >= v {
		t.Fatalf("expected safe territory to decrease under threat, before=%v after=%v", v, v2)
	}
}

func TestIntelStore_CasualtyDanger_StampedFromWounded(t *testing.T) {
	tick := 0
	ng := NewNavGrid(256, 256, nil, 0, nil, nil)
	s := NewSoldier(1, 64, 64, TeamRed, [2]float64{64, 64}, [2]float64{200, 64}, ng, nil, nil, NewThoughtLog(), &tick)
	s.state = SoldierStateWoundedAmbulatory

	intel := NewIntelStore(256, 256)
	intel.Update([]*Soldier{s}, nil, nil)

	im := intel.For(TeamRed)
	if im == nil {
		t.Fatal("expected intel map")
	}
	v := im.Layer(IntelCasualtyDanger).SampleAt(64, 64)
	if v <= 0 {
		t.Fatalf("expected casualty danger > 0 at wounded location, got %v", v)
	}
}
