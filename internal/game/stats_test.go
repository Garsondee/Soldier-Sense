package game

import (
	"math"
	"testing"
)

// --- Stance ---

func TestStanceProfile_Standing(t *testing.T) {
	p := StanceStanding.Profile()
	if p.SpeedMul != 1.0 {
		t.Fatalf("standing speed mul should be 1.0, got %.2f", p.SpeedMul)
	}
}

func TestStanceProfile_Crouching(t *testing.T) {
	p := StanceCrouching.Profile()
	if p.SpeedMul != 0.5 {
		t.Fatalf("crouching speed mul should be 0.5, got %.2f", p.SpeedMul)
	}
}

// --- PhysicalStats ---

func TestEffectiveFitness_Fresh(t *testing.T) {
	p := PhysicalStats{FitnessBase: 0.8, Fatigue: 0.0}
	ef := p.EffectiveFitness()
	if math.Abs(ef-0.8) > 1e-9 {
		t.Fatalf("expected 0.8, got %.4f", ef)
	}
}

func TestEffectiveFitness_Fatigued(t *testing.T) {
	p := PhysicalStats{FitnessBase: 1.0, Fatigue: 1.0}
	ef := p.EffectiveFitness()
	// FitnessBase * (1 - 1.0*0.8) = 1.0 * 0.2 = 0.2
	if math.Abs(ef-0.2) > 1e-9 {
		t.Fatalf("expected 0.2, got %.4f", ef)
	}
}

func TestAccumulateFatigue_Increases(t *testing.T) {
	p := PhysicalStats{FitnessBase: 0.6, Fatigue: 0.0}
	p.AccumulateFatigue(1.0, 1.0)
	if p.Fatigue <= 0 {
		t.Fatal("fatigue should increase with exertion")
	}
}

func TestAccumulateFatigue_Recovers(t *testing.T) {
	p := PhysicalStats{FitnessBase: 0.6, Fatigue: 0.5}
	p.AccumulateFatigue(0, 1.0)
	if p.Fatigue >= 0.5 {
		t.Fatal("fatigue should decrease when resting")
	}
}

func TestAccumulateFatigue_Cap(t *testing.T) {
	p := PhysicalStats{FitnessBase: 0.1, Fatigue: 0.99}
	for i := 0; i < 1000; i++ {
		p.AccumulateFatigue(1.0, 1.0)
	}
	if p.Fatigue > 1.0 {
		t.Fatalf("fatigue should cap at 1.0, got %.4f", p.Fatigue)
	}
}

func TestAccumulateFatigue_Floor(t *testing.T) {
	p := PhysicalStats{FitnessBase: 1.0, Fatigue: 0.001}
	for i := 0; i < 1000; i++ {
		p.AccumulateFatigue(0, 1.0)
	}
	if p.Fatigue < 0 {
		t.Fatalf("fatigue should not go below 0, got %.4f", p.Fatigue)
	}
}

// --- PsychState ---

func TestEffectiveFear_NoComposure(t *testing.T) {
	ps := PsychState{Fear: 1.0, Composure: 0.0, Experience: 0.0}
	ef := ps.EffectiveFear()
	if ef <= 0 {
		t.Fatal("effective fear should be positive with raw fear=1.0 and no composure")
	}
}

func TestEffectiveFear_HighComposure(t *testing.T) {
	ps := PsychState{Fear: 1.0, Composure: 1.0, Experience: 1.0}
	low := ps.EffectiveFear()
	ps2 := PsychState{Fear: 1.0, Composure: 0.0, Experience: 0.0}
	high := ps2.EffectiveFear()
	if low >= high {
		t.Fatal("high composure+experience should dampen effective fear below low composure")
	}
}

func TestApplyStress_Caps(t *testing.T) {
	ps := PsychState{Fear: 0.9}
	ps.ApplyStress(0.5)
	if ps.Fear > 1.0 {
		t.Fatalf("fear should cap at 1.0, got %.4f", ps.Fear)
	}
}

func TestRecoverFear_Decays(t *testing.T) {
	ps := PsychState{Fear: 0.5, Composure: 0.5, Morale: 0.7}
	before := ps.Fear
	ps.RecoverFear(1.0)
	if ps.Fear >= before {
		t.Fatal("fear should decay after RecoverFear")
	}
}

func TestRecoverFear_Floor(t *testing.T) {
	ps := PsychState{Fear: 0.0001}
	for i := 0; i < 100; i++ {
		ps.RecoverFear(1.0)
	}
	if ps.Fear < 0 {
		t.Fatalf("fear should not go below 0, got %.4f", ps.Fear)
	}
}

func TestRecoverMorale_RisesWhenCalm(t *testing.T) {
	ps := PsychState{Fear: 0.1, Morale: 0.5}
	before := ps.Morale
	ps.RecoverMorale(1.0)
	if ps.Morale <= before {
		t.Fatal("morale should rise when fear is below threshold")
	}
}

func TestRecoverMorale_NoRiseUnderFear(t *testing.T) {
	ps := PsychState{Fear: 0.5, Morale: 0.5}
	before := ps.Morale
	ps.RecoverMorale(1.0)
	if ps.Morale != before {
		t.Fatal("morale should not recover when fear >= 0.2")
	}
}

func TestUpdateMorale_DropsUnderFireAndIsolation(t *testing.T) {
	ps := PsychState{Fear: 0.5, Morale: 0.6, Composure: 0.2, Experience: 0.1}
	before := ps.Morale

	ps.UpdateMorale(1.0, 0.2, MoraleContext{
		UnderFire:         true,
		IncomingFireCount: 3,
		SuppressLevel:     0.8,
		VisibleThreats:    2,
		VisibleAllies:     0,
		IsolatedTicks:     120,
		SquadAvgFear:      0.6,
		SquadFearDelta:    0.08,
		CloseAllyPressure: 0.4,
		ShotMomentum:      -0.5,
		LocalSightline:    0.2,
		HasContact:        true,
	})

	if ps.Morale >= before {
		t.Fatalf("morale should drop under heavy fire pressure and isolation: before=%.4f after=%.4f", before, ps.Morale)
	}
}

func TestUpdateMorale_RisesWhenSupportedAndCalm(t *testing.T) {
	ps := PsychState{Fear: 0.1, Morale: 0.4, Composure: 0.7, Experience: 0.6}
	before := ps.Morale

	ps.UpdateMorale(1.0, 0.8, MoraleContext{
		UnderFire:         false,
		IncomingFireCount: 0,
		SuppressLevel:     0.0,
		VisibleThreats:    0,
		VisibleAllies:     3,
		IsolatedTicks:     0,
		SquadAvgFear:      0.2,
		SquadFearDelta:    -0.02,
		CloseAllyPressure: 0.1,
		ShotMomentum:      0.6,
		LocalSightline:    0.8,
		HasContact:        false,
	})

	if ps.Morale <= before {
		t.Fatalf("morale should rise in calm supported conditions: before=%.4f after=%.4f", before, ps.Morale)
	}
}

func TestUpdateMorale_LowMoraleUnderFireAddsStress(t *testing.T) {
	ps := PsychState{Fear: 0.2, Morale: 0.1, Composure: 0.1, Experience: 0.1}
	beforeFear := ps.Fear

	ps.UpdateMorale(1.0, 0.1, MoraleContext{
		UnderFire:         true,
		IncomingFireCount: 2,
		SuppressLevel:     0.6,
		VisibleThreats:    1,
		VisibleAllies:     0,
		IsolatedTicks:     80,
		SquadAvgFear:      0.7,
		SquadFearDelta:    0.03,
		CloseAllyPressure: 0.0,
		ShotMomentum:      -0.4,
		LocalSightline:    0.3,
		HasContact:        true,
	})

	if ps.Fear <= beforeFear {
		t.Fatalf("fear should increase when morale is critically low under fire: before=%.4f after=%.4f", beforeFear, ps.Fear)
	}
}

func TestUpdateMorale_HighMoraleCalmRecoversFear(t *testing.T) {
	ps := PsychState{Fear: 0.4, Morale: 0.9, Composure: 0.5, Experience: 0.5}
	beforeFear := ps.Fear

	ps.UpdateMorale(1.0, 0.7, MoraleContext{
		UnderFire:         false,
		IncomingFireCount: 0,
		SuppressLevel:     0.0,
		VisibleThreats:    0,
		VisibleAllies:     2,
		IsolatedTicks:     0,
		SquadAvgFear:      0.2,
		SquadFearDelta:    -0.01,
		CloseAllyPressure: 0.1,
		ShotMomentum:      0.2,
		LocalSightline:    0.7,
		HasContact:        false,
	})

	if ps.Fear >= beforeFear {
		t.Fatalf("fear should recover when morale is high in calm conditions: before=%.4f after=%.4f", beforeFear, ps.Fear)
	}
}

func TestWillComply_HighDiscipline(t *testing.T) {
	ps := PsychState{Fear: 0.0, Morale: 0.7}
	if !ps.WillComply(0.8, 0.0) {
		t.Fatal("high discipline, no fear: soldier should comply")
	}
}

func TestWillComply_HighFear(t *testing.T) {
	ps := PsychState{Fear: 1.0, Composure: 0.0, Experience: 0.0, Morale: 0.3}
	if ps.WillComply(0.1, 0.8) {
		t.Fatal("low discipline, high fear, high fatigue: soldier should not comply")
	}
}

// --- SoldierProfile ---

func TestEffectiveSpeed_StandingFresh(t *testing.T) {
	sp := DefaultProfile()
	speed := sp.EffectiveSpeed(1.5)
	if speed <= 0 {
		t.Fatal("effective speed should be positive")
	}
	if speed > 1.5 {
		t.Fatal("effective speed should not exceed base speed for a default soldier")
	}
}

func TestEffectiveSpeed_CrouchingSlower(t *testing.T) {
	sp := DefaultProfile()
	sp.Stance = StanceStanding
	standing := sp.EffectiveSpeed(1.5)
	sp.Stance = StanceCrouching
	crouching := sp.EffectiveSpeed(1.5)
	if crouching >= standing {
		t.Fatal("crouching should be slower than standing")
	}
}

func TestEffectiveAccuracy_Bounded(t *testing.T) {
	sp := DefaultProfile()
	acc := sp.EffectiveAccuracy()
	if acc < 0 || acc > 1 {
		t.Fatalf("accuracy should be in [0,1], got %.4f", acc)
	}
}

func TestEffectiveAccuracy_ProneMoreAccurate(t *testing.T) {
	sp := DefaultProfile()
	sp.Stance = StanceStanding
	standing := sp.EffectiveAccuracy()
	sp.Stance = StanceProne
	prone := sp.EffectiveAccuracy()
	if prone <= standing {
		t.Fatal("prone should be more accurate than standing")
	}
}

func TestClamp01(t *testing.T) {
	if clamp01(-0.5) != 0 {
		t.Fatal("clamp01 should floor at 0")
	}
	if clamp01(1.5) != 1 {
		t.Fatal("clamp01 should ceil at 1")
	}
	if clamp01(0.5) != 0.5 {
		t.Fatal("clamp01 should pass through mid-range values")
	}
}
