package game

import (
	"fmt"
	"math"
	"testing"
)

// --- Invariant helpers ---

// checkNoStuck verifies that every non-idle, non-dead soldier moves at least
// minDist pixels over any window of windowTicks consecutive ticks.
// It works by sampling positions from verbose-mode logs.
//
//nolint:unused // reserved for future invariant tests
func checkNoStuck(t *testing.T, ts *TestSim, windowTicks int, minDist float64) {
	t.Helper()
	posEntries := ts.SimLog.Filter("move", "position")
	if len(posEntries) == 0 {
		t.Log("checkNoStuck: no position entries (run with verbose SimLog)")
		return
	}

	// Group by soldier label.
	byLabel := map[string][]SimLogEntry{}
	for _, e := range posEntries {
		byLabel[e.Soldier] = append(byLabel[e.Soldier], e)
	}

	for label, entries := range byLabel {
		for i := 0; i+windowTicks < len(entries); i++ {
			start := entries[i]
			end := entries[i+windowTicks]

			// Skip if soldier is in cover/idle state during this window — not expected to move.
			stateEntries := ts.SimLog.FilterSoldier(label)
			inCover := false
			for _, se := range stateEntries {
				if se.Category == "state" && se.Key == "change" && se.Tick >= start.Tick && se.Tick <= end.Tick {
					if se.Value == "moving → idle" || se.Value == "moving → cover" {
						inCover = true
						break
					}
				}
			}
			if inCover {
				continue
			}

			var sx, sy, ex, ey float64
			if _, err := fmt.Sscanf(start.Value, "(%f,%f)", &sx, &sy); err != nil {
				t.Logf("checkNoStuck: could not parse start position %q: %v", start.Value, err)
				continue
			}
			if _, err := fmt.Sscanf(end.Value, "(%f,%f)", &ex, &ey); err != nil {
				t.Logf("checkNoStuck: could not parse end position %q: %v", end.Value, err)
				continue
			}
			dx := ex - sx
			dy := ey - sy
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < minDist {
				t.Errorf("soldier %s appears stuck: moved only %.2fpx over %d ticks (T=%d→%d)",
					label, dist, windowTicks, start.Tick, end.Tick)
				break // one report per soldier
			}
		}
	}
}

// checkGoalOscillation verifies that no soldier flips goal more than maxFlips
// times within any window of windowTicks consecutive ticks.
func checkGoalOscillation(t *testing.T, ts *TestSim, windowTicks int, maxFlips int) {
	t.Helper()
	goalChanges := ts.SimLog.Filter("goal", "change")

	byLabel := map[string][]SimLogEntry{}
	for _, e := range goalChanges {
		byLabel[e.Soldier] = append(byLabel[e.Soldier], e)
	}

	for label, entries := range byLabel {
		for i := 0; i < len(entries); i++ {
			windowEnd := entries[i].Tick + windowTicks
			count := 0
			for j := i; j < len(entries) && entries[j].Tick <= windowEnd; j++ {
				count++
			}
			if count > maxFlips {
				t.Errorf("soldier %s oscillating: %d goal changes in %d ticks (starting T=%d)",
					label, count, windowTicks, entries[i].Tick)
				break
			}
		}
	}
}

// checkFearBounded verifies fear never leaves [0, 1] for any soldier.
func checkFearBounded(t *testing.T, ts *TestSim) {
	t.Helper()
	for _, s := range ts.Soldiers {
		if s.profile.Psych.Fear < 0 || s.profile.Psych.Fear > 1.0 {
			t.Errorf("soldier %s has out-of-bounds fear: %.4f", s.label, s.profile.Psych.Fear)
		}
	}
}

// checkFatigueBounded verifies fatigue never leaves [0, 1].
func checkFatigueBounded(t *testing.T, ts *TestSim) {
	t.Helper()
	for _, s := range ts.Soldiers {
		if s.profile.Physical.Fatigue < 0 || s.profile.Physical.Fatigue > 1.0 {
			t.Errorf("soldier %s has out-of-bounds fatigue: %.4f", s.label, s.profile.Physical.Fatigue)
		}
	}
}

// checkIntentPropagated verifies that all alive members of each squad have the
// same SquadIntent as their leader's squad.
func checkIntentPropagated(t *testing.T, ts *TestSim) {
	t.Helper()
	for _, sq := range ts.Squads {
		expected := sq.Intent
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				continue
			}
			if m.blackboard.SquadIntent != expected {
				t.Errorf("squad %d member %s has intent %s, expected %s",
					sq.ID, m.label, m.blackboard.SquadIntent, expected)
			}
		}
	}
}

// checkFormationBounded verifies squad spread never exceeds maxSpread for more
// than maxConsecutive consecutive ticks (uses verbose spread logs).
func checkFormationBounded(t *testing.T, ts *TestSim, maxSpread float64, maxConsecutive int) {
	t.Helper()
	spreadEntries := ts.SimLog.Filter("squad", "spread")
	if len(spreadEntries) == 0 {
		return
	}

	consecutive := 0
	for _, e := range spreadEntries {
		if e.NumVal > maxSpread {
			consecutive++
			if consecutive > maxConsecutive {
				t.Errorf("squad spread exceeded %.0fpx for >%d consecutive ticks (T=%d, spread=%.1fpx)",
					maxSpread, maxConsecutive, e.Tick, e.NumVal)
				return
			}
		} else {
			consecutive = 0
		}
	}
}

// --- Invariant test scenarios (run with verbose logging) ---

func TestInvariant_FearBounded_AdvanceNoContact(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(99),
		WithVerbose(true),
		WithRedSoldier(0, 50, 300, 1200, 300),
		WithRedSoldier(1, 50, 360, 1200, 360),
		WithRedSoldier(2, 50, 420, 1200, 420),
		WithRedSquad(0, 1, 2),
	)
	ts.RunTicks(300)
	checkFearBounded(t, ts)
	checkFatigueBounded(t, ts)
}

func TestInvariant_FatigueBounded_LongRun(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(7),
		WithRedSoldier(0, 50, 300, 1200, 300),
		WithRedSoldier(1, 50, 400, 1200, 400),
	)
	ts.RunTicks(2000)
	checkFatigueBounded(t, ts)
	checkFearBounded(t, ts)
}

func TestInvariant_IntentPropagated_AfterContact(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 400, 340, 1200, 340),
		WithRedSoldier(1, 400, 360, 1200, 360),
		WithRedSoldier(2, 400, 380, 1200, 380),
		WithBlueSoldier(3, 550, 360, 550, 360),
		WithRedSquad(0, 1, 2),
	)
	ts.RunTicks(100)
	checkIntentPropagated(t, ts)
	checkFearBounded(t, ts)
}

func TestInvariant_GoalOscillation_StableFormation(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 50, 360, 1200, 360),
		WithRedSoldier(1, 50, 300, 1200, 300),
		WithRedSoldier(2, 50, 420, 1200, 420),
		WithRedSoldier(3, 50, 240, 1200, 240),
		WithRedSquad(0, 1, 2, 3),
	)
	ts.RunTicks(300)
	// Allow max 6 goal flips in any 20-tick window — more than that is oscillation.
	checkGoalOscillation(t, ts, 20, 6)
	checkFearBounded(t, ts)
}

func TestInvariant_FormationBounded_WedgeAdvance(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithVerbose(true),
		WithRedSoldier(0, 50, 360, 1200, 360),
		WithRedSoldier(1, 50, 300, 1200, 300),
		WithRedSoldier(2, 50, 420, 1200, 420),
		WithRedSoldier(3, 50, 240, 1200, 240),
		WithRedSoldier(4, 50, 480, 1200, 480),
		WithRedSquad(0, 1, 2, 3, 4),
	)
	ts.RunTicks(400)
	// Formation should not exceed 200px for more than 30 consecutive ticks.
	checkFormationBounded(t, ts, 200, 30)
	checkFearBounded(t, ts)
}

func TestInvariant_DeadSoldiersStayDead(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		WithRedSoldier(0, 50, 360, 1200, 360),
		WithRedSoldier(1, 50, 300, 1200, 300),
	)

	// Kill one soldier manually before running.
	reds := ts.AllByTeam(TeamRed)
	if len(reds) > 1 {
		reds[1].state = SoldierStateDead
	}

	ts.RunTicks(100)

	// Dead soldier should remain dead.
	for _, s := range ts.AllByTeam(TeamRed) {
		if s.id == 1 && s.state != SoldierStateDead {
			t.Errorf("soldier %s should remain dead", s.label)
		}
	}
}
