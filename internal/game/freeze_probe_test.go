package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

type freezeProbeState struct {
	lastX      float64
	lastY      float64
	stagnant   int
	reportedAt int
}

func transitionTick(entries []SimLogEntry, category, key, needle string) int {
	for _, e := range entries {
		if e.Category != category || e.Key != key {
			continue
		}
		if needle == "" || strings.Contains(e.Value, needle) {
			return e.Tick
		}
	}
	return -1
}

func formatTopCounts(title string, m map[string]int, n int) string {
	if len(m) == 0 {
		return fmt.Sprintf("%s: none", title)
	}
	type kv struct {
		k string
		v int
	}
	items := make([]kv, 0, len(m))
	for k, v := range m {
		items = append(items, kv{k: k, v: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v == items[j].v {
			return items[i].k < items[j].k
		}
		return items[i].v > items[j].v
	})
	if n > len(items) {
		n = len(items)
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("%s=%d", items[i].k, items[i].v))
	}
	return fmt.Sprintf("%s: %s", title, strings.Join(parts, ", "))
}

func freezeReason(boundMover bool, pathRemain int, distToSlot, leaderDist float64, intent SquadIntentKind) string {
	if !boundMover {
		return "overwatch_gate"
	}
	if pathRemain <= 1 {
		return "path_terminal"
	}
	if distToSlot > 120 {
		return "slot_far_no_progress"
	}
	if leaderDist > 220 {
		return "cohesion_pullback"
	}
	if intent == IntentRegroup {
		return "regroup_lock"
	}
	return "unknown"
}

func dumpFreezeNarrative(t *testing.T, ts *TestSim, freezeTicks []int, freezeReasonCounts, freezeBySoldier map[string]int) {
	t.Helper()
	entries := ts.SimLog.Entries()
	firstContact := transitionTick(entries, "vision", "contact_new", "")
	firstEngageIntent := transitionTick(entries, "squad", "intent_change", "engage")
	firstRegroupIntent := transitionTick(entries, "squad", "intent_change", "regroup")
	firstDeath := transitionTick(entries, "state", "change", "→ dead")
	firstFreeze := -1
	if len(freezeTicks) > 0 {
		firstFreeze = freezeTicks[0]
	}

	t.Log("=== Freeze Narrative ===")
	t.Logf("phase markers: contact=%d engage_intent=%d regroup_intent=%d first_death=%d first_freeze=%d", firstContact, firstEngageIntent, firstRegroupIntent, firstDeath, firstFreeze)
	t.Logf("event totals: intent_change=%d goal_change=%d state_change=%d contact_new=%d contact_lost=%d",
		ts.SimLog.CountCategory("squad", "intent_change"),
		ts.SimLog.CountCategory("goal", "change"),
		ts.SimLog.CountCategory("state", "change"),
		ts.SimLog.CountCategory("vision", "contact_new"),
		ts.SimLog.CountCategory("vision", "contact_lost"))
	t.Log(formatTopCounts("freeze reasons", freezeReasonCounts, 6))
	t.Log(formatTopCounts("soldiers with most freeze events", freezeBySoldier, 6))

	if wr := ts.Reporter.WindowSummary(); wr != nil {
		t.Log("=== Window Behaviour Story ===")
		t.Log(wr.Format())
	}

	if len(freezeTicks) == 0 {
		return
	}
	t.Log("=== Focused Event Windows Around Freeze Moments ===")
	maxWindows := 3
	if maxWindows > len(freezeTicks) {
		maxWindows = len(freezeTicks)
	}
	for i := 0; i < maxWindows; i++ {
		tick := freezeTicks[i]
		from := tick - 20
		if from < 1 {
			from = 1
		}
		to := tick + 10
		t.Logf("--- Freeze window #%d (T=%d, range %d..%d) ---", i+1, tick, from, to)
		window := ts.SimLog.FormatRange(from, to)
		if window == "" {
			t.Log("(no events in window)")
			continue
		}
		for _, ln := range strings.Split(strings.TrimSpace(window), "\n") {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			t.Log(ln)
		}
	}
}

func isMobilityGoal(g GoalKind) bool {
	switch g {
	case GoalAdvance, GoalMaintainFormation, GoalMoveToContact, GoalRegroup, GoalFallback, GoalFlank:
		return true
	default:
		return false
	}
}

// TestScenario_FreezeProbe_MutualAdvance runs a long headless duel and logs
// internal soldier state when a soldier appears "frozen": not moving for a
// prolonged window while in a movement-oriented goal during active combat context.
func TestScenario_FreezeProbe_MutualAdvance(t *testing.T) {
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
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

	probes := make(map[int]*freezeProbeState, len(ts.Soldiers))
	for _, s := range ts.Soldiers {
		probes[s.id] = &freezeProbeState{lastX: s.x, lastY: s.y, reportedAt: -1000}
	}

	const (
		totalTicks      = 3600
		stillThreshold  = 75
		minMoveDistance = 0.20
		maxReports      = 40
	)

	boundMoverFalseReports := 0
	boundMoverTrueReports := 0
	pathNearlyDoneReports := 0
	noPathReports := 0
	highSpreadReports := 0
	freezeReasonCounts := map[string]int{}
	freezeBySoldier := map[string]int{}
	freezeTicks := make([]int, 0, maxReports)

	reports := 0
	for i := 0; i < totalTicks; i++ {
		ts.RunTicks(1)
		tick := ts.CurrentTick()

		for _, s := range ts.Soldiers {
			if s.state == SoldierStateDead {
				continue
			}

			p := probes[s.id]
			moved := math.Hypot(s.x-p.lastX, s.y-p.lastY)
			if moved < minMoveDistance {
				p.stagnant++
			} else {
				p.stagnant = 0
			}
			p.lastX, p.lastY = s.x, s.y

			combatContext := s.blackboard.SquadHasContact || s.blackboard.VisibleThreatCount() > 0 || s.blackboard.IsActivated()
			if !combatContext || !isMobilityGoal(s.blackboard.CurrentGoal) || p.stagnant < stillThreshold {
				continue
			}
			if tick-p.reportedAt < stillThreshold {
				continue
			}

			pathLen := len(s.path)
			pathRemain := 0
			if s.path != nil && s.pathIndex >= 0 && s.pathIndex < pathLen {
				pathRemain = pathLen - s.pathIndex
			}
			if s.path == nil {
				noPathReports++
			}
			if pathRemain <= 1 {
				pathNearlyDoneReports++
			}

			distToSlot := math.Hypot(s.slotTargetX-s.x, s.slotTargetY-s.y)
			leaderDist := 0.0
			squadIntent := IntentAdvance
			if s.squad != nil {
				squadIntent = s.squad.Intent
				if s.squad.squadSpread() > 250 {
					highSpreadReports++
				}
				if s.squad.Leader != nil && s.squad.Leader != s {
					leaderDist = math.Hypot(s.squad.Leader.x-s.x, s.squad.Leader.y-s.y)
				}
			}
			if s.blackboard.BoundMover {
				boundMoverTrueReports++
			} else {
				boundMoverFalseReports++
			}
			reason := freezeReason(s.blackboard.BoundMover, pathRemain, distToSlot, leaderDist, squadIntent)
			freezeReasonCounts[reason]++
			freezeBySoldier[s.label]++
			freezeTicks = append(freezeTicks, tick)

			t.Logf("FREEZE tick=%d soldier=%s team=%s goal=%s state=%s intent=%s stagnantTicks=%d moved=%.3f pos=(%.1f,%.1f) pathIdx=%d pathLen=%d pathRemain=%d distToSlot=%.1f leaderDist=%.1f", tick, s.label, teamLabel(s.team), s.blackboard.CurrentGoal, s.state, squadIntent, p.stagnant, moved, s.x, s.y, s.pathIndex, pathLen, pathRemain, distToSlot, leaderDist)
			t.Logf("  internal move=%.2f shoot=%.2f cover=%.2f momentum=%.2f lastRange=%.1f lastHit=%.2f", s.blackboard.Internal.MoveDesire, s.blackboard.Internal.ShootDesire, s.blackboard.Internal.CoverDesire, s.blackboard.Internal.ShotMomentum, s.blackboard.Internal.LastRange, s.blackboard.Internal.LastEstimatedHitChance)
			t.Logf("  signals vis=%d squadContact=%t hasMoveOrder=%t incoming=%d suppress=%.2f combatMemory=%.2f heardGunfire=%t", s.blackboard.VisibleThreatCount(), s.blackboard.SquadHasContact, s.blackboard.HasMoveOrder, s.blackboard.IncomingFireCount, s.blackboard.SuppressLevel, s.blackboard.CombatMemoryStrength, s.blackboard.HeardGunfire)
			t.Logf("  timers goalPause=%d dashOverwatch=%d postArrival=%d fireCooldown=%d modeSwitch=%d boundMover=%t suppressionAbort=%t", s.goalPauseTimer, s.dashOverwatchTimer, s.postArrivalTimer, s.fireCooldown, s.modeSwitchTimer, s.blackboard.BoundMover, s.suppressionAbort)
			t.Logf("  reason=%s", reason)

			p.reportedAt = tick
			reports++
			if reports >= maxReports {
				break
			}
		}
		if reports >= maxReports {
			break
		}
	}

	t.Log(ts.SimLog.Summary(ts.CurrentTick(), ts.Soldiers, ts.Squads))
	dumpFreezeNarrative(t, ts, freezeTicks, freezeReasonCounts, freezeBySoldier)
	t.Logf("freeze aggregates: boundMover=false:%d boundMover=true:%d pathRemain<=1:%d noPath:%d spread>250:%d",
		boundMoverFalseReports, boundMoverTrueReports, pathNearlyDoneReports, noPathReports, highSpreadReports)
	t.Logf("freeze reports captured: %d", reports)
}
