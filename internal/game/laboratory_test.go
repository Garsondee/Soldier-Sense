package game

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

type InjuryTestCase struct {
	Name             string
	Region           BodyRegion
	Damage           float64
	ExpectedSeverity WoundSeverity
}

type MedicalRunStats struct {
	InjuriesSustained int

	BuddyAidAttempts int
	BuddyAidSuccess  int
	MedicAidAttempts int
	MedicAidSuccess  int

	SavedFromDeath int
}

func TestLaboratory_MedicalStats_5Runs(t *testing.T) {
	agg := MedicalRunStats{}

	for i := 0; i < 5; i++ {
		stats := runLaboratoryMedicalStats(t, int64(100+i))
		agg.InjuriesSustained += stats.InjuriesSustained
		agg.BuddyAidAttempts += stats.BuddyAidAttempts
		agg.BuddyAidSuccess += stats.BuddyAidSuccess
		agg.MedicAidAttempts += stats.MedicAidAttempts
		agg.MedicAidSuccess += stats.MedicAidSuccess
		agg.SavedFromDeath += stats.SavedFromDeath
	}

	sep := strings.Repeat("=", 80)
	t.Logf("\n%s", sep)
	t.Logf("LABORATORY MEDICAL STATS (5 RUNS)")
	t.Logf("%s", sep)
	t.Logf("Injuries sustained: %d", agg.InjuriesSustained)
	t.Logf("Buddy aid attempts: %d", agg.BuddyAidAttempts)
	t.Logf("Buddy aid successes: %d", agg.BuddyAidSuccess)
	t.Logf("Medic aid attempts: %d", agg.MedicAidAttempts)
	t.Logf("Medic aid successes: %d", agg.MedicAidSuccess)
	t.Logf("Injured soldiers saved from death: %d", agg.SavedFromDeath)
	t.Logf("%s\n", sep)
}

func runLaboratoryMedicalStats(_ *testing.T, seed int64) MedicalRunStats { //nolint:gocognit
	startX := 100.0
	startY := 300.0
	targetX := 700.0
	targetY := 300.0

	ts := NewTestSim(
		WithMapSize(800, 600),
		WithSeed(seed),
		WithRedSoldier(0, startX, startY, targetX, targetY),
		WithRedSoldier(1, startX+10, startY+5, targetX, targetY+5),
		WithRedSoldier(2, startX+5, startY-5, targetX, targetY-5),
		WithRedSoldier(3, startX-5, startY+10, targetX, targetY+10),
		WithRedSquad(0, 1, 2, 3),
	)

	reds := ts.AllByTeam(TeamRed)
	for _, s := range reds {
		s.profile.Skills.FirstAid = 0.35
	}
	if len(reds) > 0 {
		reds[0].isMedic = true
		reds[0].profile.Skills.FirstAid = 0.90
	}

	stats := MedicalRunStats{}
	wouldDieUntreated := map[int]bool{}

	injuryTick := 120
	injuriesToApply := 3
	injurySpacingTicks := 240
	maxTicks := 1800

	for tick := 0; tick < maxTicks; tick++ {
		// Apply a few injuries spaced out to stress medical response.
		injurySlot := (tick - injuryTick) / injurySpacingTicks
		isInjuryTick := tick >= injuryTick && (tick-injuryTick)%injurySpacingTicks == 0 && injurySlot >= 0 && injurySlot < injuriesToApply
		if isInjuryTick {
			victims := ts.AllByTeam(TeamRed)
			aliveVictims := make([]*Soldier, 0, len(victims))
			for _, s := range victims {
				if s.state != SoldierStateDead {
					aliveVictims = append(aliveVictims, s)
				}
			}
			if len(aliveVictims) > 0 {
				v := aliveVictims[ts.rng.Intn(len(aliveVictims))]
				v.casualty = NewCasualtyState(tick)

				region := BodyRegion(ts.rng.Intn(int(regionCount)))
				damage := 20.0 + ts.rng.Float64()*15.0 // moderate..critical-ish

				// Force region.
				coverMask := [regionCount]float64{}
				for i := 0; i < int(regionCount); i++ {
					coverMask[i] = 1.0
				}
				coverMask[region] = 0.0

				// Counterfactual: would they die without treatment (within horizon)?
				pre := cloneBodyMap(v.body)
				cfRNG := rand.New(rand.NewSource(seed + int64(tick*1000) + int64(v.id*17)))

				_, instantDeath := v.body.ApplyHit(damage, v.profile.Stance, coverMask, tick, ts.rng)
				stats.InjuriesSustained++
				if instantDeath {
					v.state = SoldierStateDead
				}

				// Use a longer horizon (~10 minutes) since bleed-out is measured in minutes.
				cfWouldDie := simulateUntreatedDeath(pre, damage, v.profile.Stance, coverMask, tick, cfRNG, 36000)
				wouldDieUntreated[v.id] = cfWouldDie
			}
		}

		ts.RunTicks(1)
		integrateBuddyAidTick(ts.AllByTeam(TeamRed), ts.tick)
		integrateBuddyAidTick(ts.AllByTeam(TeamBlue), ts.tick)
	}

	// Aggregate medical stats from all casualties.
	for _, s := range ts.AllByTeam(TeamRed) {
		stats.BuddyAidAttempts += s.casualty.BuddyAidAttempts
		stats.BuddyAidSuccess += s.casualty.BuddyAidSuccess
		stats.MedicAidAttempts += s.casualty.MedicAidAttempts
		stats.MedicAidSuccess += s.casualty.MedicAidSuccess

		// Saved-from-death: would have died untreated but ended alive.
		if wouldDieUntreated[s.id] && s.state != SoldierStateDead {
			stats.SavedFromDeath++
		}
	}

	return stats
}

func cloneBodyMap(b BodyMap) BodyMap {
	out := b
	if b.Wounds != nil {
		out.Wounds = make([]Wound, len(b.Wounds))
		copy(out.Wounds, b.Wounds)
	}
	return out
}

func simulateUntreatedDeath(pre BodyMap, damage float64, stance Stance, coverMask [regionCount]float64, tick int, rng *rand.Rand, horizonTicks int) bool {
	b := cloneBodyMap(pre)
	_, instantDeath := b.ApplyHit(damage, stance, coverMask, tick, rng)
	if instantDeath {
		return true
	}
	for i := 0; i < horizonTicks; i++ {
		_, _, alive := b.TickBleed()
		if !alive {
			return true
		}
	}
	return false
}

type InjuryObservation struct { //nolint:govet
	Tick               int
	VictimLabel        string
	InjuryTick         int
	Region             BodyRegion
	Severity           WoundSeverity
	InitialBloodVolume float64

	PreInjuryState  SoldierState
	PostInjuryState SoldierState

	TimeToStateChange int
	FinalState        SoldierState

	SquadReaction    string
	BuddyAidAttempts int
	VictimSelfAid    bool

	TimeToFirstProviderTicks     int
	TimeToMedicProviderTicks     int
	TimeToTreatmentStartTicks    int
	TimeToFirstWoundTreatedTicks int

	BloodVolumeAtEnd float64
	MobilityMulAtEnd float64
	AccuracyMulAtEnd float64
	PainAtEnd        float64

	Ambulatory bool
	Conscious  bool
	Alive      bool

	DetailedLog []string
}

func TestLaboratory_InjuryResponse(t *testing.T) {
	testCases := []InjuryTestCase{
		{
			Name:             "Minor Leg Wound",
			Region:           RegionLegRight,
			Damage:           8.0,
			ExpectedSeverity: WoundMinor,
		},
		{
			Name:             "Moderate Arm Wound",
			Region:           RegionArmLeft,
			Damage:           12.0,
			ExpectedSeverity: WoundModerate,
		},
		{
			Name:             "Severe Torso Wound",
			Region:           RegionTorso,
			Damage:           25.0,
			ExpectedSeverity: WoundSevere,
		},
		{
			Name:             "Critical Abdomen Wound",
			Region:           RegionAbdomen,
			Damage:           30.0,
			ExpectedSeverity: WoundCritical,
		},
		{
			Name:             "Leg Destruction",
			Region:           RegionLegLeft,
			Damage:           35.0,
			ExpectedSeverity: WoundCritical,
		},
		{
			Name:             "Severe Head Wound",
			Region:           RegionHead,
			Damage:           15.0,
			ExpectedSeverity: WoundSevere,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			obs := runLaboratoryInjuryTest(t, tc)
			printInjuryObservation(t, tc, obs)
		})
	}
}

func runLaboratoryInjuryTest(_ *testing.T, tc InjuryTestCase) InjuryObservation { //nolint:gocognit
	startX := 100.0
	startY := 300.0
	targetX := 700.0
	targetY := 300.0

	ts := NewTestSim(
		WithMapSize(800, 600),
		WithSeed(42),
		WithRedSoldier(0, startX, startY, targetX, targetY),
		WithRedSoldier(1, startX+10, startY+5, targetX, targetY+5),
		WithRedSoldier(2, startX+5, startY-5, targetX, targetY-5),
		WithRedSoldier(3, startX-5, startY+10, targetX, targetY+10),
		WithRedSquad(0, 1, 2, 3),
	)

	// Make medical aid active and effective in this distraction-free lab.
	// Soldier 0 is the designated medic.
	reds := ts.AllByTeam(TeamRed)
	for _, s := range reds {
		s.profile.Skills.FirstAid = 0.35
	}
	if len(reds) > 0 {
		reds[0].isMedic = true
		reds[0].profile.Skills.FirstAid = 0.90
	}

	obs := InjuryObservation{
		DetailedLog: make([]string, 0),
	}

	injuryTick := 120
	var victim *Soldier
	injuryApplied := false

	maxTicks := 1200

	var prevState SoldierState
	injuredTick := 0
	firstProviderTick := -1
	medicProviderTick := -1
	treatmentStartTick := -1
	firstWoundTreatedTick := -1

	for tick := 0; tick < maxTicks; tick++ {
		if tick == injuryTick && !injuryApplied {
			redSoldiers := ts.AllByTeam(TeamRed)
			if len(redSoldiers) > 0 {
				victimIdx := ts.rng.Intn(len(redSoldiers))
				victim = redSoldiers[victimIdx]

				obs.VictimLabel = victim.label
				obs.InjuryTick = tick
				obs.PreInjuryState = victim.state
				obs.InitialBloodVolume = victim.body.BloodVolume
				prevState = victim.state
				injuredTick = tick

				// Initialize casualty tracking so buddy-aid has state to write into.
				victim.casualty = NewCasualtyState(tick)

				// Encourage quick response: allow immediate re-evaluation.
				for _, s := range ts.AllByTeam(TeamRed) {
					if s != nil {
						s.blackboard.NextDecisionTick = tick
					}
				}

				// Force the requested region to be hit by masking all others.
				coverMask := [regionCount]float64{}
				for i := 0; i < int(regionCount); i++ {
					coverMask[i] = 1.0
				}
				coverMask[tc.Region] = 0.0
				wound, instantDeath := victim.body.ApplyHit(tc.Damage, victim.profile.Stance, coverMask, tick, ts.rng)

				obs.Region = wound.Region
				obs.Severity = wound.Severity

				obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
					"[T=%04d] INJURY APPLIED: %s hit in %s, damage=%.1f, severity=%s, bleed=%.3f/tick, pain=%.2f",
					tick, victim.label, wound.Region, tc.Damage, wound.Severity, wound.BleedRate, wound.Pain,
				))

				if instantDeath {
					obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
						"[T=%04d] %s INSTANT DEATH (head/neck destroyed)",
						tick, victim.label,
					))
				}

				injuryApplied = true
			}
		}

		ts.RunTicks(1)

		// Mirror the full Game loop's medical integration so buddy-aid / medic-aid progresses.
		integrateBuddyAidTick(ts.AllByTeam(TeamRed), ts.tick)
		integrateBuddyAidTick(ts.AllByTeam(TeamBlue), ts.tick)

		if injuryApplied && victim != nil {
			// Debug: confirm helpers recognize casualty and pick GoalHelpCasualty.
			if tick >= injuryTick && tick <= injuryTick+240 && tick%30 == 0 {
				for _, s := range ts.AllByTeam(TeamRed) {
					if s == nil || s == victim {
						continue
					}
					obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
						"[T=%04d] HELPER %s: goal=%s squadHasCasualties=%t suppressed=%t incoming=%d",
						tick, s.label, s.blackboard.CurrentGoal, s.blackboard.SquadHasCasualties, s.blackboard.IsSuppressed(), s.blackboard.IncomingFireCount,
					))
				}
			}
		}

		// Capture medical response timings.
		if injuryApplied && victim != nil {
			if firstProviderTick < 0 && len(victim.casualty.Providers) > 0 {
				firstProviderTick = tick
				obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
					"[T=%04d] AID STARTED: providers=%d first=%s medic=%t",
					tick, len(victim.casualty.Providers), victim.casualty.Providers[0].label, victim.casualty.Providers[0].isMedic,
				))
			}
			if medicProviderTick < 0 {
				for _, p := range victim.casualty.Providers {
					if p != nil && p.isMedic {
						medicProviderTick = tick
						break
					}
				}
			}
			if treatmentStartTick < 0 && victim.casualty.CurrentTreat != nil {
				treatmentStartTick = tick
				obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
					"[T=%04d] TREATMENT START: action=%s provider=%s medic=%t",
					tick, victim.casualty.CurrentTreat.Action, victim.casualty.CurrentTreat.Provider.label, victim.casualty.CurrentTreat.Provider.isMedic,
				))
			}
			if firstWoundTreatedTick < 0 {
				for i := range victim.body.Wounds {
					w := &victim.body.Wounds[i]
					if w.Treated && w.TreatedTick > 0 {
						firstWoundTreatedTick = tick
						obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
							"[T=%04d] WOUND TREATED: region=%s severity=%s treated_tick=%d",
							tick, w.Region, w.Severity, w.TreatedTick,
						))
						break
					}
				}
			}
		}

		if victim != nil && victim.state != prevState {
			ambulatory, conscious, alive := victim.body.TickBleed()
			obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
				"[T=%04d] %s STATE CHANGE: %s -> %s (blood=%.2f, ambulatory=%t, conscious=%t, alive=%t)",
				tick, victim.label, prevState, victim.state, victim.body.BloodVolume, ambulatory, conscious, alive,
			))
			prevState = victim.state

			if obs.TimeToStateChange == 0 && tick > injuryTick {
				obs.TimeToStateChange = tick - injuryTick
			}
		}

		if injuryApplied && victim != nil && tick%60 == 0 {
			obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
				"[T=%04d] %s STATUS: state=%s, blood=%.2f, mobility=%.2f, accuracy=%.2f, pain=%.2f, wounds=%d",
				tick, victim.label, victim.state, victim.body.BloodVolume,
				victim.body.MobilityMul(), victim.body.AccuracyMul(), victim.body.TotalPain(),
				victim.body.WoundCount(),
			))
		}

		if injuryApplied && victim != nil && victim.state == SoldierStateDead {
			obs.DetailedLog = append(obs.DetailedLog, fmt.Sprintf(
				"[T=%04d] %s DIED (blood=%.3f)",
				tick, victim.label, victim.body.BloodVolume,
			))
			break
		}
	}

	if victim != nil {
		obs.Tick = ts.Tick
		obs.PostInjuryState = victim.state
		obs.FinalState = victim.state
		obs.BloodVolumeAtEnd = victim.body.BloodVolume
		obs.MobilityMulAtEnd = victim.body.MobilityMul()
		obs.AccuracyMulAtEnd = victim.body.AccuracyMul()
		obs.PainAtEnd = victim.body.TotalPain()

		ambulatory, conscious, alive := victim.body.TickBleed()
		obs.Ambulatory = ambulatory
		obs.Conscious = conscious
		obs.Alive = alive

		if injuredTick > 0 {
			if firstProviderTick >= 0 {
				obs.TimeToFirstProviderTicks = firstProviderTick - injuredTick
			}
			if medicProviderTick >= 0 {
				obs.TimeToMedicProviderTicks = medicProviderTick - injuredTick
			}
			if treatmentStartTick >= 0 {
				obs.TimeToTreatmentStartTicks = treatmentStartTick - injuredTick
			}
			if firstWoundTreatedTick >= 0 {
				obs.TimeToFirstWoundTreatedTicks = firstWoundTreatedTick - injuredTick
			}
		}
	}

	return obs
}

func printInjuryObservation(t *testing.T, tc InjuryTestCase, obs InjuryObservation) {
	sep := strings.Repeat("=", 80)
	t.Logf("\n%s", sep)
	t.Logf("LABORATORY INJURY TEST: %s", tc.Name)
	t.Logf("%s", sep)
	t.Logf("Target Region: %s", tc.Region)
	t.Logf("Damage Applied: %.1f", tc.Damage)
	t.Logf("Expected Severity: %s", tc.ExpectedSeverity)
	t.Logf("")

	t.Logf("INJURY DETAILS:")
	t.Logf("  Victim: %s", obs.VictimLabel)
	t.Logf("  Injury Tick: %d", obs.InjuryTick)
	t.Logf("  Actual Region Hit: %s", obs.Region)
	t.Logf("  Actual Severity: %s", obs.Severity)
	t.Logf("  Initial Blood Volume: %.2f", obs.InitialBloodVolume)
	t.Logf("")

	t.Logf("STATE PROGRESSION:")
	t.Logf("  Pre-Injury State: %s", obs.PreInjuryState)
	t.Logf("  Post-Injury State: %s", obs.PostInjuryState)
	t.Logf("  Final State: %s", obs.FinalState)
	if obs.TimeToStateChange > 0 {
		t.Logf("  Time to State Change: %d ticks (%.1f seconds)", obs.TimeToStateChange, float64(obs.TimeToStateChange)/60.0)
	}
	t.Logf("")

	t.Logf("AID RESPONSE (time from injury):")
	if obs.TimeToFirstProviderTicks > 0 {
		t.Logf("  First Provider: %d ticks (%.2f sec)", obs.TimeToFirstProviderTicks, float64(obs.TimeToFirstProviderTicks)/60.0)
	} else {
		t.Logf("  First Provider: (none)")
	}
	if obs.TimeToMedicProviderTicks > 0 {
		t.Logf("  Medic Provider: %d ticks (%.2f sec)", obs.TimeToMedicProviderTicks, float64(obs.TimeToMedicProviderTicks)/60.0)
	} else {
		t.Logf("  Medic Provider: (none)")
	}
	if obs.TimeToTreatmentStartTicks > 0 {
		t.Logf("  Treatment Start: %d ticks (%.2f sec)", obs.TimeToTreatmentStartTicks, float64(obs.TimeToTreatmentStartTicks)/60.0)
	} else {
		t.Logf("  Treatment Start: (none)")
	}
	if obs.TimeToFirstWoundTreatedTicks > 0 {
		t.Logf("  First Wound Treated: %d ticks (%.2f sec)", obs.TimeToFirstWoundTreatedTicks, float64(obs.TimeToFirstWoundTreatedTicks)/60.0)
	} else {
		t.Logf("  First Wound Treated: (none)")
	}
	t.Logf("")

	t.Logf("FINAL CONDITION:")
	t.Logf("  Blood Volume: %.2f", obs.BloodVolumeAtEnd)
	t.Logf("  Mobility Multiplier: %.2f", obs.MobilityMulAtEnd)
	t.Logf("  Accuracy Multiplier: %.2f", obs.AccuracyMulAtEnd)
	t.Logf("  Pain Level: %.2f", obs.PainAtEnd)
	t.Logf("  Ambulatory: %t", obs.Ambulatory)
	t.Logf("  Conscious: %t", obs.Conscious)
	t.Logf("  Alive: %t", obs.Alive)
	t.Logf("")

	t.Logf("DETAILED EVENT LOG:")
	for _, logLine := range obs.DetailedLog {
		t.Logf("  %s", logLine)
	}
	t.Logf("%s\n", sep)
}

// ===== BEHAVIORAL LABORATORY TESTS =====

func TestLaboratory_SuppressionResponse(t *testing.T) {
	test := SuppressionResponseTest()
	obs := RunLaboratoryTest(test)

	printBehavioralTestResult(t, test, obs)

	passed, reason := test.Validate(obs)
	if !passed {
		t.Errorf("Test failed: %s", reason)
	}
}

func TestLaboratory_FearThreshold(t *testing.T) {
	test := FearThresholdTest()
	obs := RunLaboratoryTest(test)

	printBehavioralTestResult(t, test, obs)

	passed, reason := test.Validate(obs)
	if !passed {
		t.Errorf("Test failed: %s", reason)
	}
}

func TestLaboratory_FormationMaintenance(t *testing.T) {
	test := FormationMaintenanceTest()
	obs := RunLaboratoryTest(test)

	printBehavioralTestResult(t, test, obs)

	passed, reason := test.Validate(obs)
	if !passed {
		t.Errorf("Test failed: %s", reason)
	}
}

func TestLaboratory_FirstContactResponse(t *testing.T) {
	test := FirstContactResponseTest()
	obs := RunLaboratoryTest(test)

	printBehavioralTestResult(t, test, obs)

	passed, reason := test.Validate(obs)
	if !passed {
		t.Errorf("Test failed: %s", reason)
	}
}

func TestLaboratory_CohesionCollapse(t *testing.T) {
	test := CohesionCollapseTest()
	obs := RunLaboratoryTest(test)

	printBehavioralTestResult(t, test, obs)

	passed, reason := test.Validate(obs)
	if !passed {
		t.Errorf("Test failed: %s", reason)
	}
}

func printBehavioralTestResult(t *testing.T, test *LaboratoryTest, obs *LaboratoryObservation) {
	sep := strings.Repeat("=", 80)
	t.Logf("\n%s", sep)
	t.Logf("LABORATORY BEHAVIORAL TEST: %s", test.Name)
	t.Logf("%s", sep)
	t.Logf("Description: %s", test.Description)
	t.Logf("Expected: %s", test.Expected)
	t.Logf("Duration: %d ticks (%.1f seconds)", test.DurationTicks, float64(test.DurationTicks)/60.0)
	t.Logf("")

	t.Logf("KEY OBSERVATIONS:")
	if obs.FirstContactTick >= 0 {
		t.Logf("  First Contact: tick %d (%.1f sec)", obs.FirstContactTick, float64(obs.FirstContactTick)/60.0)
	}
	if obs.FirstFearIncreaseTick >= 0 {
		t.Logf("  First Fear Increase: tick %d", obs.FirstFearIncreaseTick)
	}
	if obs.FirstGoalChangeTick >= 0 {
		t.Logf("  First Goal Change: tick %d", obs.FirstGoalChangeTick)
	}
	if obs.FirstStanceChangeTick >= 0 {
		t.Logf("  First Stance Change: tick %d", obs.FirstStanceChangeTick)
	}
	if obs.FirstPanicTick >= 0 {
		t.Logf("  First Panic: tick %d", obs.FirstPanicTick)
	}
	if obs.FirstDisobeyTick >= 0 {
		t.Logf("  First Disobedience: tick %d", obs.FirstDisobeyTick)
	}
	if obs.CohesionBreakTick >= 0 {
		t.Logf("  Cohesion Break: tick %d", obs.CohesionBreakTick)
	}
	t.Logf("")

	t.Logf("STATISTICS:")
	t.Logf("  Max Fear: %.3f (at tick %d)", obs.MaxFear, obs.MaxFearTick)
	t.Logf("  Final Fear: %.3f", obs.FinalFear)
	t.Logf("  Goal Changes: %d", obs.GoalChanges)
	t.Logf("  Stance Changes: %d", obs.StanceChanges)
	if obs.FormationSpreadMax > 0 {
		t.Logf("  Max Formation Spread: %.1f px", obs.FormationSpreadMax)
		t.Logf("  Final Formation Spread: %.1f px", obs.FormationSpreadFinal)
	}
	t.Logf("")

	if len(obs.Metrics) > 0 {
		t.Logf("CUSTOM METRICS:")
		for key, val := range obs.Metrics {
			t.Logf("  %s: %.3f", key, val)
		}
		t.Logf("")
	}

	if len(obs.Flags) > 0 {
		t.Logf("FLAGS:")
		for key, val := range obs.Flags {
			t.Logf("  %s: %t", key, val)
		}
		t.Logf("")
	}

	t.Logf("EVENTS (%d total):", len(obs.Events))
	for i, evt := range obs.Events {
		if i >= 20 {
			t.Logf("  ... (%d more events)", len(obs.Events)-20)
			break
		}
		t.Logf("  [T=%03d] %s: %s", evt.Tick, evt.SoldierLabel, evt.Description)
	}
	t.Logf("")

	passed, reason := test.Validate(obs)
	if passed {
		t.Logf("RESULT: PASSED")
		t.Logf("  %s", reason)
	} else {
		t.Logf("RESULT: FAILED")
		t.Logf("  %s", reason)
	}
	t.Logf("%s\n", sep)
}
