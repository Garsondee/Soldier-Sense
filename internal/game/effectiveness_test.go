package game

import (
	"strings"
	"testing"
)

func TestTestSim_LogCombatEffectiveness_StalledAndDetached(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 500, 360, 1200, 360),
		WithRedSoldier(1, 100, 360, 1200, 360),
		WithBlueSoldier(2, 900, 360, 50, 360),
		WithRedSquad(0, 1),
	)

	reds := ts.AllByTeam(TeamRed)
	if len(reds) != 2 {
		t.Fatalf("expected 2 red soldiers, got %d", len(reds))
	}
	leader := reds[0]
	member := reds[1]
	blues := ts.AllByTeam(TeamBlue)
	if len(blues) != 1 {
		t.Fatalf("expected 1 blue soldier, got %d", len(blues))
	}
	threat := blues[0]

	leader.blackboard.Threats = []ThreatFact{{
		Source:     threat,
		X:          threat.x,
		Y:          threat.y,
		Confidence: 1.0,
		LastTick:   1,
		IsVisible:  true,
	}}
	member.blackboard.Threats = nil
	member.blackboard.SquadHasContact = true
	member.blackboard.CurrentGoal = GoalMoveToContact
	member.state = SoldierStateIdle
	member.x, member.y = 100, 360
	leader.x, leader.y = 500, 360
	ts.effProbes[member.id] = &effectivenessProbe{lastX: member.x, lastY: member.y}

	for tick := 1; tick <= effectivenessStalledTicks; tick++ {
		ts.logCombatEffectiveness(tick, member)
	}

	var stalledCount, detachedCount int
	for _, e := range ts.SimLog.Filter("effectiveness", "") {
		if e.Soldier != member.label {
			continue
		}
		switch e.Key {
		case "stalled_in_combat":
			stalledCount++
		case "detached_from_engagement":
			detachedCount++
		}
	}

	if stalledCount != 1 {
		t.Fatalf("expected 1 stalled_in_combat event for %s, got %d", member.label, stalledCount)
	}
	if detachedCount != 1 {
		t.Fatalf("expected 1 detached_from_engagement event for %s, got %d", member.label, detachedCount)
	}
}

func TestSimReporter_Collect_CombatEffectivenessMetrics(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 500, 360, 1200, 360),
		WithRedSoldier(1, 100, 360, 1200, 360),
		WithBlueSoldier(2, 900, 360, 50, 360),
		WithRedSquad(0, 1),
	)

	reds := ts.AllByTeam(TeamRed)
	leader := reds[0]
	member := reds[1]
	threat := ts.AllByTeam(TeamBlue)[0]

	leader.blackboard.Threats = []ThreatFact{{
		Source:     threat,
		X:          threat.x,
		Y:          threat.y,
		Confidence: 1.0,
		LastTick:   60,
		IsVisible:  true,
	}}
	member.blackboard.Threats = nil
	member.blackboard.SquadHasContact = true
	member.blackboard.CurrentGoal = GoalMoveToContact
	member.state = SoldierStateIdle
	leader.x, leader.y = 500, 360
	member.x, member.y = 100, 360

	ts.Reporter.Collect(60, ts.AllByTeam(TeamRed), ts.AllByTeam(TeamBlue), ts.Squads)

	latest := ts.Reporter.Latest()
	if latest == nil {
		t.Fatal("expected latest report")
	}
	if latest.RedStalledInCombat != 1 {
		t.Fatalf("expected RedStalledInCombat=1, got %d", latest.RedStalledInCombat)
	}
	if latest.RedDetached != 1 {
		t.Fatalf("expected RedDetached=1, got %d", latest.RedDetached)
	}
	if latest.BlueStalledInCombat != 0 || latest.BlueDetached != 0 {
		t.Fatalf("expected blue effectiveness counters at zero, got stalled=%d detached=%d", latest.BlueStalledInCombat, latest.BlueDetached)
	}

	wr := ts.Reporter.WindowSummary()
	if wr == nil {
		t.Fatal("expected window summary")
	}
	if wr.AvgRedStalledInCombat != 1 || wr.AvgRedDetached != 1 {
		t.Fatalf("expected red averages to be 1/1, got stalled=%.1f detached=%.1f", wr.AvgRedStalledInCombat, wr.AvgRedDetached)
	}

	formatted := wr.Format()
	if !strings.Contains(formatted, "Combat Effectiveness Alerts") {
		t.Fatalf("expected formatted window report to include effectiveness section, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "stalled_in_combat") || !strings.Contains(formatted, "detached_from_engagement") {
		t.Fatalf("expected formatted window report to include effectiveness metrics, got:\n%s", formatted)
	}
}
