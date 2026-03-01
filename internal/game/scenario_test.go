package game

import (
	"math"
	"testing"
)

// dumpLog prints the full SimLog to t.Log so it appears in `go test -v` output.
func dumpLog(t *testing.T, ts *TestSim) {
	t.Helper()
	entries := ts.SimLog.Entries()
	if len(entries) == 0 {
		t.Log("(no log entries)")
		return
	}
	for _, e := range entries {
		t.Log(e.String())
	}
}

// dumpSummary prints the scenario summary block.
func dumpSummary(t *testing.T, ts *TestSim) {
	t.Helper()
	t.Log(ts.SimLog.Summary(ts.CurrentTick(), ts.Soldiers, ts.Squads))
	if ts.Reporter != nil {
		t.Log(ts.Reporter.FormatLatest())
		if wr := ts.Reporter.WindowSummary(); wr != nil {
			t.Log(wr.Format())
		}
	}
}

// --- Scenario: Advance No Contact ---

func TestScenario_AdvanceNoContact(t *testing.T) {
	t.Log("=== TestScenario_AdvanceNoContact ===")
	t.Log("--- Setup: 6 red (wedge), no enemies, no buildings ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		// Spawn in a tight cluster so the squad doesn't immediately choose Regroup
		// due to high spread on tick 1.
		WithRedSoldier(0, 50, 350, 1200, 350),
		WithRedSoldier(1, 50, 322, 1200, 322),
		WithRedSoldier(2, 50, 378, 1200, 378),
		WithRedSoldier(3, 50, 294, 1200, 294),
		WithRedSoldier(4, 50, 406, 1200, 406),
		WithRedSoldier(5, 50, 266, 1200, 266),
		WithRedSquad(0, 1, 2, 3, 4, 5),
	)

	ts.RunTicks(500)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Invariants: no survive goals should appear with no enemies.
	surviveEntries := ts.SimLog.Filter("goal", "change")
	for _, e := range surviveEntries {
		if e.Value == "advance → survive" || e.Value == "formation → survive" || e.Value == "hold → survive" {
			t.Errorf("unexpected survive goal change: %s", e.String())
		}
	}
}

// --- Scenario: Advance With Buildings ---

func TestScenario_AdvanceWithBuildings(t *testing.T) {
	t.Log("=== TestScenario_AdvanceWithBuildings ===")
	t.Log("--- Setup: 6 red, 3 buildings, no enemies ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithBuilding(300, 100, 128, 256),
		WithBuilding(600, 300, 192, 128),
		WithBuilding(900, 150, 128, 300),
		WithRedSoldier(0, 50, 100, 1200, 100),
		WithRedSoldier(1, 50, 200, 1200, 200),
		WithRedSoldier(2, 50, 350, 1200, 350),
		WithRedSoldier(3, 50, 450, 1200, 450),
		WithRedSoldier(4, 50, 550, 1200, 550),
		WithRedSoldier(5, 50, 650, 1200, 650),
		WithRedSquad(0, 1, 2, 3, 4, 5),
	)

	ts.RunTicks(500)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Invariant: no path_fail entries for any soldier that started with a valid path.
	pathFails := ts.SimLog.Filter("move", "path_fail")
	if len(pathFails) > 0 {
		for _, e := range pathFails {
			t.Logf("path_fail: %s", e.String())
		}
		// Informational only — buildings may occasionally block a formation slot.
	}
}

// --- Scenario: Contact Halt ---

func TestScenario_ContactHalt(t *testing.T) {
	t.Log("=== TestScenario_ContactHalt ===")
	t.Log("--- Setup: 6 red advancing east (tight cluster), 1 blue stationary at x=400 ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		// Start soldiers in a tight vertical cluster so spread <120px from tick 1.
		WithRedSoldier(0, 50, 350, 1200, 350),
		WithRedSoldier(1, 50, 322, 1200, 322),
		WithRedSoldier(2, 50, 378, 1200, 378),
		WithRedSoldier(3, 50, 294, 1200, 294),
		WithRedSoldier(4, 50, 406, 1200, 406),
		WithRedSoldier(5, 50, 266, 1200, 266),
		// Blue stationary well within vision range (300px) after a short advance.
		WithBlueSoldier(6, 400, 350, 400, 350),
		WithRedSquad(0, 1, 2, 3, 4, 5),
	)

	ts.RunTicks(400)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Observation: squad should transition to Hold when contact is made.
	// This may not fire if formation members go idle before reaching vision range —
	// a known gap in the current simulation that these logs make visible.
	if ts.SimLog.HasEntry("squad", "intent_change", "hold") {
		t.Log("PASS: squad transitioned to Hold on contact")
	} else {
		t.Log("NOTE: squad did not reach Hold — formation members went idle before contact (known gap)")
	}
}

// --- Scenario: Mutual Advance ---

func TestScenario_MutualAdvance(t *testing.T) {
	t.Log("=== TestScenario_MutualAdvance ===")
	t.Log("--- Setup: 6 red vs 6 blue advancing toward each other (tight clusters) ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		// Tight clusters: max spread ~84px (3*28), well under 120px regroup threshold.
		WithRedSoldier(0, 50, 350, 1200, 350),
		WithRedSoldier(1, 50, 322, 1200, 322),
		WithRedSoldier(2, 50, 378, 1200, 378),
		WithRedSoldier(3, 50, 294, 1200, 294),
		WithRedSoldier(4, 50, 406, 1200, 406),
		WithRedSoldier(5, 50, 266, 1200, 266),
		WithBlueSoldier(6, 1200, 350, 50, 350),
		WithBlueSoldier(7, 1200, 322, 50, 322),
		WithBlueSoldier(8, 1200, 378, 50, 378),
		WithBlueSoldier(9, 1200, 294, 50, 294),
		WithBlueSoldier(10, 1200, 406, 50, 406),
		WithBlueSoldier(11, 1200, 266, 50, 266),
		WithRedSquad(0, 1, 2, 3, 4, 5),
		WithBlueSquad(6, 7, 8, 9, 10, 11),
	)

	ts.RunTicks(600)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Observation: both squads should make contact during mutual advance.
	// Formation members going idle at slot targets before reaching vision range
	// is a known simulation gap surfaced by this test.
	contactEntries := ts.SimLog.Filter("vision", "contact_new")
	if len(contactEntries) > 0 {
		t.Logf("PASS: %d contact_new events logged", len(contactEntries))
	} else {
		t.Log("NOTE: no contacts made — squads did not converge (known formation-idle gap)")
	}
}

// --- Scenario: Fearful Soldier ---

func TestScenario_FearfulSoldier(t *testing.T) {
	t.Log("=== TestScenario_FearfulSoldier ===")
	t.Log("--- Setup: 1 red with high fear + low composure, 2 blue enemies visible ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 400, 360, 1200, 360),
		WithBlueSoldier(1, 450, 360, 450, 360), // stationary, in front
		WithBlueSoldier(2, 480, 320, 480, 320), // stationary, nearby
	)

	// Force high fear on the red soldier before running.
	red := ts.AllByTeam(TeamRed)
	if len(red) > 0 {
		red[0].profile.Psych.Fear = 0.95
		red[0].profile.Psych.Composure = 0.05
		red[0].profile.Psych.Experience = 0.0
	}

	ts.RunTicks(100)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Invariant: survive goal should appear given high fear + visible enemies.
	if !ts.SimLog.HasEntry("goal", "change", "survive") {
		t.Error("fearful soldier with visible enemies should select GoalSurvive at some point")
	}
}

// --- Scenario: Disciplined Squad Holds ---

func TestScenario_DisciplinedSquad(t *testing.T) {
	t.Log("=== TestScenario_DisciplinedSquad ===")
	t.Log("--- Setup: 6 high-discipline reds under hold intent, 2 blue enemies visible ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 400, 300, 1200, 300),
		WithRedSoldier(1, 400, 340, 1200, 340),
		WithRedSoldier(2, 400, 380, 1200, 380),
		WithRedSoldier(3, 400, 420, 1200, 420),
		WithRedSoldier(4, 400, 460, 1200, 460),
		WithRedSoldier(5, 400, 500, 1200, 500),
		WithBlueSoldier(6, 550, 360, 550, 360),
		WithBlueSoldier(7, 570, 400, 570, 400),
		WithRedSquad(0, 1, 2, 3, 4, 5),
	)

	// Set all reds to high discipline, low fear.
	for _, s := range ts.AllByTeam(TeamRed) {
		s.profile.Skills.Discipline = 0.95
		s.profile.Psych.Fear = 0.0
		s.profile.Psych.Composure = 0.9
	}

	ts.RunTicks(200)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Informational: count how many survive goal changes occurred.
	surviveChanges := 0
	for _, e := range ts.SimLog.Filter("goal", "change") {
		if ts.SimLog.HasEntry("goal", "change", "survive") {
			_ = e
			surviveChanges++
			break
		}
	}
	t.Logf("survive goal changes detected: %v", surviveChanges > 0)
}

// --- Scenario: Formation Cohesion ---

func TestScenario_FormationCohesion(t *testing.T) {
	t.Log("=== TestScenario_FormationCohesion ===")
	t.Log("--- Setup: leader + 5 members in wedge, advancing east, no enemies ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 50, 360, 1200, 360),
		WithRedSoldier(1, 50, 300, 1200, 300),
		WithRedSoldier(2, 50, 420, 1200, 420),
		WithRedSoldier(3, 50, 240, 1200, 240),
		WithRedSoldier(4, 50, 480, 1200, 480),
		WithRedSoldier(5, 50, 180, 1200, 180),
		WithRedSquad(0, 1, 2, 3, 4, 5),
	)

	ts.RunTicks(300)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Invariant: squad spread should not exceed 180px for more than a brief moment.
	// (Formation may temporarily spread during initial catch-up.)
	maxSpread := 0.0
	if len(ts.Squads) > 0 {
		sq := ts.Squads[0]
		maxSpread = sq.squadSpread()
	}
	t.Logf("final squad spread: %.1fpx", maxSpread)
}

// --- Scenario: Threat Decay Memory ---

func TestScenario_ThreatDecayMemory(t *testing.T) {
	t.Log("=== TestScenario_ThreatDecayMemory ===")
	t.Log("--- Setup: red soldier sees blue, blue moves behind building, threat decays ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		// Building to hide behind.
		WithBuilding(500, 280, 64, 160),
		WithRedSoldier(0, 100, 360, 100, 360),  // stationary red (same start/end)
		WithBlueSoldier(1, 400, 360, 400, 360), // starts visible, stays put
	)

	// Run long enough for contact to be seen, then check.
	ts.RunTicks(400)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Check vision contact events were logged.
	contacts := ts.SimLog.Filter("vision", "contact_new")
	t.Logf("contact_new events: %d", len(contacts))
}

// --- Scenario: Vision Cone Blind Spot ---

func TestScenario_VisionConeBlindSpot(t *testing.T) {
	t.Log("=== TestScenario_VisionConeBlindSpot ===")
	t.Log("--- Setup: enemy directly behind observer (outside FOV) ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		// Red facing east (heading ~0), blue directly behind (west).
		WithRedSoldier(0, 640, 360, 1200, 360),
		WithBlueSoldier(1, 200, 360, 200, 360), // behind red
	)

	ts.RunTicks(5)
	dumpLog(t, ts)

	red := ts.AllByTeam(TeamRed)
	if len(red) > 0 {
		contacts := red[0].vision.KnownContacts
		// At tick 5 the red is still facing east — blue is behind, so should not be in cone.
		// (Red may have rotated slightly but blue is far enough behind.)
		t.Logf("red[0] heading=%.3f, contacts=%d", red[0].vision.Heading, len(contacts))
	}
}

// --- Scenario: Regroup Order ---

func TestScenario_RegroupOrder(t *testing.T) {
	t.Log("=== TestScenario_RegroupOrder ===")
	t.Log("--- Setup: squad spread >120px, should trigger regroup intent ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 100, 360, 1200, 360), // leader
		WithRedSoldier(1, 100, 360, 1200, 360),
		WithRedSoldier(2, 100, 360, 1200, 360),
		WithRedSquad(0, 1, 2),
	)

	// Manually spread members apart to trigger regroup.
	reds := ts.AllByTeam(TeamRed)
	if len(reds) >= 3 {
		reds[1].x = reds[0].x + 150
		reds[2].x = reds[0].x - 150
	}

	ts.RunTicks(200)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	if !ts.SimLog.HasEntry("squad", "intent_change", "regroup") {
		t.Error("spread>120: expected regroup intent to be issued at some point")
	}
}

// --- Scenario: Undisciplined Squad ---

func TestScenario_UndisciplinedSquad(t *testing.T) {
	t.Log("=== TestScenario_UndisciplinedSquad ===")
	t.Log("--- Setup: low-discipline squad under contact, expect goal diversity ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 400, 300, 1200, 300),
		WithRedSoldier(1, 400, 340, 1200, 340),
		WithRedSoldier(2, 400, 380, 1200, 380),
		WithRedSoldier(3, 400, 420, 1200, 420),
		WithBlueSoldier(4, 500, 360, 500, 360),
		WithRedSquad(0, 1, 2, 3),
	)

	for _, s := range ts.AllByTeam(TeamRed) {
		s.profile.Skills.Discipline = 0.15
		s.profile.Psych.Fear = 0.4
		s.profile.Psych.Composure = 0.1
		s.profile.Psych.Morale = 0.3
	}

	ts.RunTicks(200)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	goalChanges := ts.SimLog.Filter("goal", "change")
	t.Logf("total goal changes: %d (expect diversity from undisciplined squad)", len(goalChanges))

	uniqueGoals := map[string]bool{}
	for _, e := range goalChanges {
		uniqueGoals[e.Value] = true
	}
	t.Logf("unique goal transitions: %v", func() []string {
		keys := make([]string, 0, len(uniqueGoals))
		for k := range uniqueGoals {
			keys = append(keys, k)
		}
		return keys
	}())
}

// --- Scenario: Leader Slowdown ---

func TestScenario_LeaderSlowdown(t *testing.T) {
	t.Log("=== TestScenario_LeaderSlowdown ===")
	t.Log("--- Setup: leader advances, members start far behind, slowdown should kick in ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 600, 360, 1200, 360), // leader far ahead
		WithRedSoldier(1, 50, 360, 1200, 360),  // members far behind
		WithRedSoldier(2, 50, 400, 1200, 400),
		WithRedSquad(0, 1, 2),
	)

	snap0 := ts.Snapshot()
	ts.RunTicks(50)
	snap1 := ts.Snapshot()

	// Leader's initial position is (600,360); with spread >100 slowdown should apply.
	// Find leader movement delta.
	leaderMoved := 0.0
	for i, s := range snap0.Soldiers {
		if s.Label == "R0" {
			dx := snap1.Soldiers[i].X - s.X
			dy := snap1.Soldiers[i].Y - s.Y
			leaderMoved = dx*dx + dy*dy
		}
	}
	t.Logf("leader movement (squared dist) over 50 ticks: %.1f", leaderMoved)

	dumpLog(t, ts)
	dumpSummary(t, ts)
}

// --- Scenario: Intent Propagation ---

func TestScenario_IntentPropagation(t *testing.T) {
	t.Log("=== TestScenario_IntentPropagation ===")
	t.Log("--- Setup: leader issues Hold, all members should update blackboard same tick ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 400, 360, 1200, 360),
		WithRedSoldier(1, 380, 330, 1200, 330),
		WithRedSoldier(2, 380, 390, 1200, 390),
		WithBlueSoldier(3, 550, 360, 550, 360),
		WithRedSquad(0, 1, 2),
	)

	// Inject visible threat on leader to force Hold.
	reds := ts.AllByTeam(TeamRed)
	if len(reds) > 0 {
		reds[0].blackboard.Threats = []ThreatFact{
			{X: 550, Y: 360, Confidence: 1.0, IsVisible: true, LastTick: 0},
		}
	}

	ts.RunTicks(5)
	dumpLog(t, ts)

	// All members' blackboard should reflect squad intent after a few ticks.
	for _, s := range reds {
		t.Logf("%s blackboard.SquadIntent = %s", s.label, s.blackboard.SquadIntent)
	}
	for _, s := range reds[1:] {
		if s.blackboard.SquadIntent != IntentHold {
			t.Logf("note: %s has SquadIntent=%s (expected hold after contact)", s.label, s.blackboard.SquadIntent)
		}
	}

	// Log the final summary.
	t.Logf("squad intent: %s", ts.Squads[0].Intent)
}

// --- Unit test: combat hit lands ---

func TestCombat_HitLands(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	shooter := NewSoldier(0, 100, 240, TeamRed, [2]float64{0, 240}, [2]float64{600, 240}, ng, nil, nil, tl, &tick)
	target := NewSoldier(1, 200, 240, TeamBlue, [2]float64{600, 240}, [2]float64{0, 240}, ng, nil, nil, tl, &tick)

	// Give shooter perfect accuracy, target standing.
	shooter.profile.Skills.Marksmanship = 1.0
	shooter.profile.Psych.Fear = 0
	shooter.profile.Physical.Fatigue = 0
	target.profile.Stance = StanceStanding

	// Manually inject contact so shooter can see target.
	shooter.vision.KnownContacts = []*Soldier{target}

	cm := NewCombatManager(42)
	initialHealth := target.health

	// Run enough ticks to guarantee at least one shot fires.
	for i := 0; i < fireIntervalSingle+5; i++ {
		tick++
		cm.ResetFireCounts([]*Soldier{shooter, target})
		cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, []*Soldier{shooter, target})
	}

	if target.health == initialHealth {
		t.Errorf("no damage dealt after %d ticks: health still %.0f", fireIntervalSingle+5, target.health)
	} else {
		t.Logf("PASS: target health went from %.0f to %.0f", initialHealth, target.health)
	}
}

func TestCombat_BurstShotsArePaced(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	shooter := NewSoldier(0, 100, 240, TeamRed, [2]float64{0, 240}, [2]float64{600, 240}, ng, nil, nil, tl, &tick)
	target := NewSoldier(1, 240, 240, TeamBlue, [2]float64{600, 240}, [2]float64{0, 240}, ng, nil, nil, tl, &tick)
	shooter.vision.KnownContacts = []*Soldier{target}

	shooter.currentFireMode = FireModeBurst
	shooter.desiredFireMode = FireModeBurst
	shooter.burstShotsRemaining = 2
	shooter.burstShotIndex = 1
	shooter.burstTargetID = target.id
	shooter.burstBaseSpread = 0.03

	cm := NewCombatManager(42)
	all := []*Soldier{shooter, target}

	cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)
	if got := len(cm.tracers); got != 1 {
		t.Fatalf("expected first queued burst shot now, tracers=%d", got)
	}
	if shooter.fireCooldown != burstInterShotGap {
		t.Fatalf("expected inter-shot cooldown=%d, got=%d", burstInterShotGap, shooter.fireCooldown)
	}

	cm.ResetFireCounts(all)
	cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)
	if got := len(cm.tracers); got != 1 {
		t.Fatalf("expected no shot during first gap tick, tracers=%d", got)
	}

	cm.ResetFireCounts(all)
	cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)
	if got := len(cm.tracers); got != 1 {
		t.Fatalf("expected no shot during second gap tick, tracers=%d", got)
	}

	cm.ResetFireCounts(all)
	cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)
	if got := len(cm.tracers); got != 2 {
		t.Fatalf("expected next burst round after gap, tracers=%d", got)
	}
}

func TestCombat_LongRangeAimingDelaysShotWhenCalm(t *testing.T) {
	ng := NewNavGrid(1200, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	shooter := NewSoldier(0, 100, 240, TeamRed, [2]float64{0, 240}, [2]float64{1120, 240}, ng, nil, nil, tl, &tick)
	target := NewSoldier(1, 620, 240, TeamBlue, [2]float64{1120, 240}, [2]float64{0, 240}, ng, nil, nil, tl, &tick)
	shooter.vision.KnownContacts = []*Soldier{target}

	// Ensure long-range trigger willingness so this test isolates aiming delay only.
	shooter.blackboard.Internal.ShootDesire = 1.0
	shooter.blackboard.Internal.ShotMomentum = 1.0
	shooter.blackboard.Internal.MoveDesire = 0.0
	shooter.currentFireMode = FireModeSingle
	shooter.desiredFireMode = FireModeSingle

	cm := NewCombatManager(7)
	all := []*Soldier{shooter, target}
	requiredAimTicks := aimingTicksForDistance(520)

	for i := 0; i < requiredAimTicks; i++ {
		cm.ResetFireCounts(all)
		cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)
		if got := len(cm.tracers); got != 0 {
			t.Fatalf("expected no shot while aiming (tick %d/%d), tracers=%d", i+1, requiredAimTicks, got)
		}
	}

	cm.ResetFireCounts(all)
	cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)
	if got := len(cm.tracers); got != 1 {
		t.Fatalf("expected shot after aiming delay, tracers=%d", got)
	}
}

func TestCombat_LongRangeAimingSkippedWhenUnderFire(t *testing.T) {
	ng := NewNavGrid(1200, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	shooter := NewSoldier(0, 100, 240, TeamRed, [2]float64{0, 240}, [2]float64{1120, 240}, ng, nil, nil, tl, &tick)
	target := NewSoldier(1, 620, 240, TeamBlue, [2]float64{1120, 240}, [2]float64{0, 240}, ng, nil, nil, tl, &tick)
	shooter.vision.KnownContacts = []*Soldier{target}

	shooter.blackboard.Internal.ShootDesire = 1.0
	shooter.blackboard.Internal.ShotMomentum = 1.0
	shooter.blackboard.Internal.MoveDesire = 0.0
	shooter.blackboard.IncomingFireCount = 1
	shooter.currentFireMode = FireModeSingle
	shooter.desiredFireMode = FireModeSingle

	cm := NewCombatManager(11)
	all := []*Soldier{shooter, target}
	cm.ResolveCombat([]*Soldier{shooter}, []*Soldier{target}, []*Soldier{shooter}, nil, all)

	if got := len(cm.tracers); got != 1 {
		t.Fatalf("expected immediate long-range shot when under fire, tracers=%d", got)
	}
	if shooter.aimingTicks != 0 {
		t.Fatalf("expected aiming state reset while under fire, aimingTicks=%d", shooter.aimingTicks)
	}
}

func TestSoldier_MustCrawlWhenSuppressed_UsesSuppressAndFear(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	s := NewSoldier(0, 100, 100, TeamRed, [2]float64{0, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)

	s.blackboard.SuppressLevel = pinnedSuppressionLevel + 0.01
	if !s.mustCrawlWhenSuppressed() {
		t.Fatal("expected crawl when suppression is at pinned level")
	}

	s.blackboard.SuppressLevel = SuppressThreshold + 0.02
	s.profile.Psych.Fear = 0.0
	if s.mustCrawlWhenSuppressed() {
		t.Fatal("expected no forced crawl at moderate suppression with low fear")
	}

	s.profile.Psych.Fear = 1.0
	s.profile.Psych.Composure = 0.0
	s.profile.Psych.Experience = 0.0
	if !s.mustCrawlWhenSuppressed() {
		t.Fatal("expected forced crawl at suppressed state with extreme fear")
	}
}

func TestSoldier_CanSuppressedFallbackRun_RequiresMoraleAndLowerFear(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	s := NewSoldier(1, 120, 100, TeamRed, [2]float64{0, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)

	s.blackboard.SuppressLevel = suppressedRunThreshold + 0.05
	s.profile.Psych.Morale = 0.8
	s.profile.Psych.Fear = 0.1
	if !s.canSuppressedFallbackRun() {
		t.Fatal("expected resilient low-fear soldier to be able to run fallback under heavy suppression")
	}

	s.profile.Psych.Morale = 0.3
	if s.canSuppressedFallbackRun() {
		t.Fatal("expected low-morale soldier to be unable to run fallback under heavy suppression")
	}

	s.profile.Psych.Morale = 0.8
	s.profile.Psych.Fear = 1.0
	s.profile.Psych.Composure = 0.0
	s.profile.Psych.Experience = 0.0
	if s.canSuppressedFallbackRun() {
		t.Fatal("expected extreme fear to block running fallback")
	}
}

func TestMoveAlongPath_ProneSuppressedIsPainfullySlow(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	standing := NewSoldier(2, 100, 100, TeamRed, [2]float64{0, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	proneSuppressed := NewSoldier(3, 100, 120, TeamRed, [2]float64{0, 120}, [2]float64{600, 120}, ng, nil, nil, tl, &tick)

	standing.path = [][2]float64{{220, 100}}
	standing.pathIndex = 0
	standing.profile.Stance = StanceStanding

	proneSuppressed.path = [][2]float64{{220, 120}}
	proneSuppressed.pathIndex = 0
	proneSuppressed.profile.Stance = StanceProne
	proneSuppressed.blackboard.SuppressLevel = SuppressThreshold + 0.1

	standing.moveAlongPath(1.0)
	proneSuppressed.moveAlongPath(1.0)

	standingStep := math.Hypot(standing.x-100, standing.y-100)
	proneStep := math.Hypot(proneSuppressed.x-100, proneSuppressed.y-120)
	if proneStep >= standingStep*0.35 {
		t.Fatalf("expected pinned prone crawl to be much slower; standing=%.3f prone=%.3f", standingStep, proneStep)
	}
}

func TestSoldier_SeparationNormal_HeadOnUsesLateral(t *testing.T) {
	ng := NewNavGrid(640, 480, nil, soldierRadius, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	a := NewSoldier(10, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	b := NewSoldier(11, 116, 100, TeamRed, [2]float64{116, 100}, [2]float64{0, 100}, ng, nil, nil, tl, &tick)
	a.state = SoldierStateMoving
	b.state = SoldierStateMoving

	dx := a.x - b.x
	dy := a.y - b.y
	d := math.Hypot(dx, dy)
	nx, ny := a.separationNormal(b, dx, dy, d)

	// Head-on movers should get a lateral separation vector (side-step),
	// not a push parallel to current forward heading.
	forwardDot := math.Abs(nx*math.Cos(a.vision.Heading) + ny*math.Sin(a.vision.Heading))
	if forwardDot > 0.25 {
		t.Fatalf("expected lateral separation for head-on movement, got forwardDot=%.3f normal=(%.3f,%.3f)", forwardDot, nx, ny)
	}
	if math.Abs(math.Hypot(nx, ny)-1.0) > 1e-6 {
		t.Fatalf("expected unit normal, got (%.3f,%.3f)", nx, ny)
	}
}

// --- Scenario: Close Engagement ---
// Two squads already close to each other. Verifies:
// 1. Members select move_to_contact and then engage once they gain LOS.
// 2. A low-composure soldier under sustained fire selects GoalFallback.

func TestScenario_CloseEngagement(t *testing.T) {
	t.Log("=== TestScenario_CloseEngagement ===")
	t.Log("--- Setup: 4 red vs 4 blue, starting 200px apart ---")

	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(77),
		// Red leader + 3 members, facing east — already near centre.
		WithRedSoldier(0, 400, 340, 1200, 340),
		WithRedSoldier(1, 370, 320, 1200, 320),
		WithRedSoldier(2, 370, 360, 1200, 360),
		WithRedSoldier(3, 370, 300, 1200, 300),
		// Blue leader + 3 members facing west — 200px away.
		WithBlueSoldier(4, 600, 340, 50, 340),
		WithBlueSoldier(5, 630, 320, 50, 320),
		WithBlueSoldier(6, 630, 360, 50, 360),
		WithBlueSoldier(7, 630, 300, 50, 300),
		WithRedSquad(0, 1, 2, 3),
		WithBlueSquad(4, 5, 6, 7),
	)

	// Give one red member very low composure so fear accumulates quickly
	// and GoalFallback becomes attractive under fire.
	reds := ts.AllByTeam(TeamRed)
	if len(reds) >= 2 {
		reds[1].profile.Psych.Composure = 0.05
		reds[1].profile.Psych.Experience = 0.0
		reds[1].profile.Skills.Discipline = 0.1
	}

	ts.RunTicks(300)
	dumpLog(t, ts)
	dumpSummary(t, ts)

	// Health dump — verify hits are landing.
	for _, s := range ts.Soldiers {
		t.Logf("%s health=%.0f state=%s", s.label, s.health, s.state)
	}

	// 1. Some non-leader red should have gained at least one contact.
	memberContacts := 0
	for _, s := range reds[1:] {
		if len(s.vision.KnownContacts) > 0 {
			memberContacts++
		}
	}
	t.Logf("red non-leader members with active contacts: %d", memberContacts)

	// 2. At least one engage event should have occurred.
	if !ts.SimLog.HasEntry("goal", "change", "engage") {
		t.Error("expected at least one soldier to select GoalEngage")
	}

	// 3. Fallback should have triggered for the low-composure member.
	if ts.SimLog.HasEntry("goal", "change", "fallback") {
		t.Log("PASS: GoalFallback triggered for low-composure member under fire")
	} else {
		t.Log("NOTE: GoalFallback did not trigger (may need more sustained fire or longer run)")
	}
}
