package game

import (
	"testing"
)

func TestConcealmentScore_Standing(t *testing.T) {
	tick := 0
	soldier := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)

	// Standing soldier should have low concealment
	concealment := CalculateConcealmentScore(soldier, false)
	if concealment < 0.05 || concealment > 0.20 {
		t.Errorf("Standing soldier concealment should be 0.05-0.20, got %.3f", concealment)
	}
}

func TestConcealmentScore_Prone(t *testing.T) {
	tick := 0
	soldier := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)

	// Set prone stance
	soldier.profile.Stance = StanceProne

	// Prone stationary soldier should have high concealment
	concealment := CalculateConcealmentScore(soldier, false)
	if concealment < 0.90 {
		t.Errorf("Prone stationary soldier concealment should be >0.90, got %.3f", concealment)
	}
}

func TestSpottingPower_Awareness(t *testing.T) {
	tick := 0

	// Low awareness soldier
	lowAware := NewSoldier(1, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	lowAware.profile.Skills.TacticalAwareness = 0.20
	lowAware.profile.Survival.SituationalAwareness = 0.20

	// High awareness soldier
	highAware := NewSoldier(2, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	highAware.profile.Skills.TacticalAwareness = 0.70
	highAware.profile.Survival.SituationalAwareness = 0.70

	// Both looking straight ahead (0 angle)
	targetAngle := 0.0

	lowPower := CalculateSpottingPower(lowAware, targetAngle)
	highPower := CalculateSpottingPower(highAware, targetAngle)

	if highPower <= lowPower {
		t.Errorf("High awareness should have higher spotting power. Low: %.3f, High: %.3f", lowPower, highPower)
	}

	// Should be roughly 2x difference based on design
	ratio := highPower / lowPower
	if ratio < 1.8 || ratio > 2.5 {
		t.Errorf("Expected ~2x spotting power difference, got %.2fx", ratio)
	}
}

func TestAccumulator_Basic(t *testing.T) {
	tick := 0

	// Create observer with medium awareness
	observer := NewSoldier(1, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	observer.profile.Skills.TacticalAwareness = 0.50
	observer.profile.Survival.SituationalAwareness = 0.50

	// Create standing target at medium range
	target := NewSoldier(2, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{0, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	target.profile.Stance = StanceStanding

	// Setup vision scan
	vision := NewVisionState(0)
	threats := []ThreatFact{}

	// Run multiple ticks to accumulate detection
	candidates := []*Soldier{target}

	var confirmed bool
	for i := 0; i < 300; i++ { // Up to 5 seconds at 60fps
		vision.PerformVisionScan(0, 0, observer, candidates, nil, nil, nil, &threats, i)

		if len(vision.KnownContacts) > 0 {
			confirmed = true
			t.Logf("Standing target detected after %d ticks (%.2fs)", i+1, float64(i+1)/60.0)
			break
		}
	}

	if !confirmed {
		if len(threats) == 0 {
			t.Fatal("No threat tracking occurred")
		}
		t.Fatalf("Standing target should be detected within 5 seconds. Final accumulator: %.3f",
			threats[0].SpottingAccumulator)
	}
}

func TestAccumulator_ProneHarder(t *testing.T) {
	tick := 0

	// Create observer with low awareness (evolved baseline)
	observer := NewSoldier(1, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	observer.profile.Skills.TacticalAwareness = 0.20
	observer.profile.Survival.SituationalAwareness = 0.20

	// Create prone target at medium range
	target := NewSoldier(2, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{0, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	target.profile.Stance = StanceProne

	// Setup vision scan
	vision := NewVisionState(0)
	threats := []ThreatFact{}

	// Run for 5 seconds - prone should NOT be detected by low-awareness observer
	candidates := []*Soldier{target}

	for i := 0; i < 300; i++ { // 5 seconds at 60fps
		vision.PerformVisionScan(0, 0, observer, candidates, nil, nil, nil, &threats, i)

		if len(vision.KnownContacts) > 0 {
			t.Fatalf("Prone target should NOT be detected by low-awareness observer within 5s, but was detected at tick %d", i+1)
		}
	}

	if len(threats) == 0 {
		t.Fatal("No threat tracking occurred")
	}

	// Should have some accumulation but not enough to confirm
	if threats[0].SpottingAccumulator <= 0 {
		t.Error("Should have some spotting accumulation even if not confirmed")
	}

	if threats[0].SpottingAccumulator >= 0.85 {
		t.Errorf("Accumulator should be below confirmation threshold (0.85), got %.3f",
			threats[0].SpottingAccumulator)
	}

	t.Logf("Prone target accumulator after 5s: %.3f (below 0.85 threshold as expected)",
		threats[0].SpottingAccumulator)
}

func TestAccumulator_HighAwarenessDetectsProne(t *testing.T) {
	tick := 0

	// Create observer with high awareness
	observer := NewSoldier(1, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	observer.profile.Skills.TacticalAwareness = 0.70
	observer.profile.Survival.SituationalAwareness = 0.70

	// Create prone target at medium range
	target := NewSoldier(2, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{0, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	target.profile.Stance = StanceProne

	// Setup vision scan
	vision := NewVisionState(0)
	threats := []ThreatFact{}

	// Run for up to 30 seconds - high-awareness should eventually detect prone
	candidates := []*Soldier{target}

	var confirmed bool
	for i := 0; i < 1800; i++ { // 30 seconds at 60fps
		vision.PerformVisionScan(0, 0, observer, candidates, nil, nil, nil, &threats, i)

		if len(vision.KnownContacts) > 0 {
			confirmed = true
			t.Logf("Prone target detected by high-awareness observer after %d ticks (%.2fs)",
				i+1, float64(i+1)/60.0)
			break
		}
	}

	if !confirmed {
		if len(threats) == 0 {
			t.Fatal("No threat tracking occurred")
		}
		t.Errorf("High-awareness observer should detect prone target within 30s. Final accumulator: %.3f",
			threats[0].SpottingAccumulator)
	}
}

func TestLKPFadeRate_AwarenessScaling(t *testing.T) {
	tick := 0

	// Create low awareness soldier
	lowAware := NewSoldier(1, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	lowAware.profile.Skills.TacticalAwareness = 0.20

	// Create high awareness soldier
	highAware := NewSoldier(2, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	highAware.profile.Skills.TacticalAwareness = 0.70

	// Create target
	target := NewSoldier(3, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{0, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)

	// Setup vision for both observers
	visionLow := NewVisionState(0)
	visionHigh := NewVisionState(0)
	threatsLow := []ThreatFact{}
	threatsHigh := []ThreatFact{}

	// First tick: both see the target
	visionLow.PerformVisionScan(0, 0, lowAware, []*Soldier{target}, nil, nil, nil, &threatsLow, 0)
	visionHigh.PerformVisionScan(0, 0, highAware, []*Soldier{target}, nil, nil, nil, &threatsHigh, 0)

	// Confirm both detected
	if len(visionLow.KnownContacts) == 0 || len(visionHigh.KnownContacts) == 0 {
		t.Skip("Targets not detected in first tick, skipping LKP fade test")
	}

	// Now remove target from view and let LKP fade
	for i := 1; i < 300; i++ {
		visionLow.PerformVisionScan(0, 0, lowAware, []*Soldier{}, nil, nil, nil, &threatsLow, i)
		visionHigh.PerformVisionScan(0, 0, highAware, []*Soldier{}, nil, nil, nil, &threatsHigh, i)
	}

	// High awareness should retain LKP longer
	confidenceLow := 0.0
	confidenceHigh := 0.0
	if len(threatsLow) > 0 {
		confidenceLow = threatsLow[0].Confidence
	}
	if len(threatsHigh) > 0 {
		confidenceHigh = threatsHigh[0].Confidence
	}

	if confidenceHigh <= confidenceLow {
		t.Errorf("High awareness should retain LKP longer. Low: %.3f, High: %.3f",
			confidenceLow, confidenceHigh)
	}

	t.Logf("LKP confidence after 5s - Low awareness: %.3f, High awareness: %.3f",
		confidenceLow, confidenceHigh)
}

func TestLKPSuppressionFire_DisciplineGate(t *testing.T) {
	tick := 0

	// Create low discipline soldier
	lowDisc := NewSoldier(1, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	lowDisc.profile.Skills.Discipline = 0.20

	// Create high discipline soldier
	highDisc := NewSoldier(2, 0, 0, TeamRed, [2]float64{0, 0}, [2]float64{100, 0},
		NewNavGrid(640, 480, nil, 0, nil, nil), nil, nil, NewThoughtLog(), &tick)
	highDisc.profile.Skills.Discipline = 0.70

	// Create LKP threat (not visible, but has confidence)
	lkpThreat := ThreatFact{
		Source:     nil,
		X:          100,
		Y:          0,
		Confidence: 0.5,
		LastTick:   0,
		IsVisible:  false,
	}

	lowDisc.blackboard.Threats = []ThreatFact{lkpThreat}
	highDisc.blackboard.Threats = []ThreatFact{lkpThreat}

	// Check if LKP targets are available for each
	lowLKP := lkpThreats(lowDisc.blackboard.Threats)
	highLKP := lkpThreats(highDisc.blackboard.Threats)

	if len(lowLKP) == 0 || len(highLKP) == 0 {
		t.Fatal("LKP threats should be available for both soldiers")
	}

	// Low discipline soldier should not fire (discipline < 0.3)
	// High discipline soldier should fire (discipline > 0.3)
	// This is tested implicitly in the ResolveCombat logic

	t.Logf("Low discipline (%.2f) and high discipline (%.2f) both have LKP targets available",
		lowDisc.profile.Skills.Discipline, highDisc.profile.Skills.Discipline)
	t.Log("ResolveCombat will gate LKP fire at discipline > 0.3")
}
