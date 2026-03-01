package game

import (
	"math"
	"math/rand"
	"testing"
)

func TestNewBodyMap_FullHealth(t *testing.T) {
	bm := NewBodyMap()
	if bm.BloodVolume != 1.0 {
		t.Fatalf("expected blood volume 1.0, got %.2f", bm.BloodVolume)
	}
	if frac := bm.HealthFraction(); math.Abs(frac-1.0) > 0.001 {
		t.Fatalf("expected health fraction 1.0, got %.4f", frac)
	}
	if bm.IsInjured() {
		t.Fatal("new body should not be injured")
	}
	if bm.WoundCount() != 0 {
		t.Fatal("new body should have no wounds")
	}
}

func TestHitRegionDistribution_Standing(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	var noCover [regionCount]float64
	counts := [regionCount]int{}
	n := 100_000

	for i := 0; i < n; i++ {
		r := rollHitRegion(StanceStanding, noCover, rng)
		counts[r]++
	}

	expected := stanceHitWeights[StanceStanding]
	for i := 0; i < int(regionCount); i++ {
		got := float64(counts[i]) / float64(n)
		exp := expected[i]
		if math.Abs(got-exp) > 0.015 {
			t.Errorf("region %s: expected ~%.3f, got %.3f", BodyRegion(i), exp, got)
		}
	}
}

func TestHitRegionDistribution_Prone(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	var noCover [regionCount]float64
	counts := [regionCount]int{}
	n := 100_000

	for i := 0; i < n; i++ {
		r := rollHitRegion(StanceProne, noCover, rng)
		counts[r]++
	}

	// Prone: head should be ~0.18, torso ~0.10.
	headFrac := float64(counts[RegionHead]) / float64(n)
	torsoFrac := float64(counts[RegionTorso]) / float64(n)
	if headFrac < torsoFrac {
		t.Errorf("prone head fraction (%.3f) should exceed torso (%.3f)", headFrac, torsoFrac)
	}
}

func TestCoverMask_ShieldsRegions(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// Mask legs and abdomen fully.
	var mask [regionCount]float64
	mask[RegionLegLeft] = 1.0
	mask[RegionLegRight] = 1.0
	mask[RegionAbdomen] = 1.0

	n := 50_000
	for i := 0; i < n; i++ {
		r := rollHitRegion(StanceStanding, mask, rng)
		if r == RegionLegLeft || r == RegionLegRight || r == RegionAbdomen {
			t.Fatalf("hit shielded region %s with full cover mask", r)
		}
	}
}

func TestApplyHit_HeadKill(t *testing.T) {
	bm := NewBodyMap()
	rng := rand.New(rand.NewSource(1))

	// Force a head hit by masking everything else.
	var mask [regionCount]float64
	for i := 1; i < int(regionCount); i++ {
		mask[i] = 1.0
	}

	// Apply massive damage to guarantee head reaches 0.
	w, dead := bm.ApplyHit(100, StanceStanding, mask, 0, rng)
	if w.Region != RegionHead {
		t.Fatalf("expected head hit, got %s", w.Region)
	}
	if !dead {
		t.Fatal("head at 0 HP should cause instant death")
	}
}

func TestApplyHit_LimbNotFatal(t *testing.T) {
	bm := NewBodyMap()
	rng := rand.New(rand.NewSource(3))

	// Force a left leg hit.
	var mask [regionCount]float64
	for i := 0; i < int(regionCount); i++ {
		if BodyRegion(i) != RegionLegLeft {
			mask[i] = 1.0
		}
	}

	w, dead := bm.ApplyHit(25, StanceStanding, mask, 0, rng)
	if w.Region != RegionLegLeft {
		t.Fatalf("expected leg hit, got %s", w.Region)
	}
	if dead {
		t.Fatal("leg hit should not cause instant death")
	}
	if bm.HealthFraction() >= 1.0 {
		t.Fatal("health fraction should be < 1.0 after a hit")
	}
}

func TestTickBleed_UntreatedCriticalKills(t *testing.T) {
	bm := NewBodyMap()
	rng := rand.New(rand.NewSource(5))

	// Force torso hit with enough damage to create a severe/critical wound.
	var mask [regionCount]float64
	for i := 0; i < int(regionCount); i++ {
		if BodyRegion(i) != RegionTorso {
			mask[i] = 1.0
		}
	}
	bm.ApplyHit(40, StanceStanding, mask, 0, rng)

	// Let it bleed. Should eventually die.
	died := false
	for tick := 0; tick < 2000; tick++ {
		amb, con, alive := bm.TickBleed()
		if !alive {
			died = true
			t.Logf("died at tick %d (blood=%.3f, amb=%v, con=%v)", tick, bm.BloodVolume, amb, con)
			break
		}
	}
	if !died {
		t.Errorf("untreated torso wound should eventually kill; blood=%.3f, torsoHP=%.1f",
			bm.BloodVolume, bm.HP[RegionTorso])
	}
}

func TestTickBleed_TreatedWoundStopsBleed(t *testing.T) {
	bm := NewBodyMap()
	rng := rand.New(rand.NewSource(11))

	var mask [regionCount]float64
	for i := 0; i < int(regionCount); i++ {
		if BodyRegion(i) != RegionArmLeft {
			mask[i] = 1.0
		}
	}
	bm.ApplyHit(15, StanceStanding, mask, 0, rng)

	// Treat the wound immediately.
	bm.Wounds[0].Treated = true
	bloodBefore := bm.BloodVolume

	for tick := 0; tick < 100; tick++ {
		bm.TickBleed()
	}

	if bm.BloodVolume < bloodBefore {
		t.Errorf("treated wound should not drain blood: before=%.3f after=%.3f",
			bloodBefore, bm.BloodVolume)
	}
}

func TestMobilityMul_LegWound(t *testing.T) {
	bm := NewBodyMap()
	bm.HP[RegionLegLeft] = 10 // below 60% of 30

	mul := bm.MobilityMul()
	if mul >= 1.0 {
		t.Fatalf("expected mobility < 1.0 with leg damage, got %.2f", mul)
	}
	if mul <= 0 {
		t.Fatalf("expected mobility > 0 with one damaged leg, got %.2f", mul)
	}
}

func TestMobilityMul_BothLegsDisabled(t *testing.T) {
	bm := NewBodyMap()
	bm.HP[RegionLegLeft] = 0
	bm.HP[RegionLegRight] = 0

	mul := bm.MobilityMul()
	if mul != 0.0 {
		t.Fatalf("expected mobility 0 with both legs disabled, got %.2f", mul)
	}
}

func TestAccuracyMul_ArmWound(t *testing.T) {
	bm := NewBodyMap()
	bm.HP[RegionArmRight] = 10 // below 60% of 25

	mul := bm.AccuracyMul()
	if mul >= 1.0 {
		t.Fatalf("expected accuracy < 1.0 with arm damage, got %.2f", mul)
	}
}

func TestCanSelfAid(t *testing.T) {
	bm := NewBodyMap()
	if !bm.CanSelfAid(true) {
		t.Fatal("healthy conscious soldier should be able to self-aid")
	}
	if bm.CanSelfAid(false) {
		t.Fatal("unconscious soldier should not self-aid")
	}

	bm.HP[RegionArmLeft] = 0
	bm.HP[RegionArmRight] = 0
	if bm.CanSelfAid(true) {
		t.Fatal("soldier with both arms disabled should not self-aid")
	}
}

func TestSeverityDetermination(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	// High damage to head should usually produce severe/critical.
	severeOrCrit := 0
	n := 1000
	for i := 0; i < n; i++ {
		sev := determineSeverity(50, RegionHead, rng)
		if sev == WoundSevere || sev == WoundCritical {
			severeOrCrit++
		}
	}
	frac := float64(severeOrCrit) / float64(n)
	if frac < 0.5 {
		t.Errorf("expected >50%% severe/critical for 50dmg head hit, got %.1f%%", frac*100)
	}

	// Low damage to leg should usually produce minor/moderate.
	minorOrMod := 0
	for i := 0; i < n; i++ {
		sev := determineSeverity(5, RegionLegLeft, rng)
		if sev == WoundMinor || sev == WoundModerate {
			minorOrMod++
		}
	}
	frac = float64(minorOrMod) / float64(n)
	if frac < 0.5 {
		t.Errorf("expected >50%% minor/moderate for 5dmg leg hit, got %.1f%%", frac*100)
	}
}

func TestHealthFraction_DegradesWithDamage(t *testing.T) {
	bm := NewBodyMap()
	full := bm.HealthFraction()

	bm.HP[RegionTorso] -= 25
	partial := bm.HealthFraction()
	if partial >= full {
		t.Fatalf("health fraction should decrease after torso damage: full=%.3f partial=%.3f", full, partial)
	}
}

func TestTotalPain_Capped(t *testing.T) {
	bm := NewBodyMap()
	// Add many wounds to exceed pain cap.
	for i := 0; i < 10; i++ {
		bm.Wounds = append(bm.Wounds, Wound{
			Severity: WoundCritical,
			Pain:     severityPain[WoundCritical],
		})
	}
	pain := bm.TotalPain()
	if pain > 1.0 {
		t.Fatalf("total pain should be capped at 1.0, got %.2f", pain)
	}
}
