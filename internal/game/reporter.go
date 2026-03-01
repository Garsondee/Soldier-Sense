package game

import (
	"fmt"
	"math"
	"strings"
)

// reportWindowTicks is the default sliding window for recent-behaviour reports (~10s at 60TPS).
const reportWindowTicks = 600

// --- Snapshot types ---

// SquadReport captures a single squad's state at one point in time.
type SquadReport struct {
	Team               Team
	SquadID            int
	Alive              int
	Dead               int
	Intent             SquadIntentKind
	OutnumberedFactor  float64
	Posture            float64 // -1 defensive .. +1 offensive
	MembersWithContact int
	TotalEnemiesSeen   int
	AvgFear            float64
	AvgMorale          float64
}

// SoldierReport captures a single soldier's state.
type SoldierReport struct {
	ID                          int
	Label                       string
	Team                        Team
	Goal                        GoalKind
	State                       SoldierState
	Health                      float64
	Fear                        float64
	Morale                      float64
	AtCorner                    bool
	AtWall                      bool
	AtDoor                      bool
	Posture                     float64
	StalledInCombat             bool
	DetachedFromSquadEngagement bool
	LeaderDistance              float64
}

// SimReport is a full snapshot of the simulation at one tick.
type SimReport struct {
	Tick int

	// Per-team goal distributions (GoalKind → count).
	RedGoals  map[GoalKind]int
	BlueGoals map[GoalKind]int

	// Per-team aggregate stats.
	RedAlive, BlueAlive     int
	RedDead, BlueDead       int
	RedInjured, BlueInjured int // health < max but > 0

	// Visibility ratios.
	RedMembersWithContact  int // red soldiers who can see at least one enemy
	BlueMembersWithContact int
	RedTotalEnemiesSeen    int // total enemy sightings across red
	BlueTotalEnemiesSeen   int
	RedStalledInCombat     int
	BlueStalledInCombat    int
	RedDetached            int
	BlueDetached           int

	// Squad-level summaries.
	Squads []SquadReport

	// Soldiers detail (optional, for verbose mode).
	Soldiers []SoldierReport

	// Posture summary: average across all squads per team.
	RedAvgPosture  float64
	BlueAvgPosture float64
}

// --- Reporter ---

// SimReporter collects periodic reports from the simulation and can produce
// summaries over sliding time windows.
type SimReporter struct {
	history     []SimReport
	windowTicks int
	verbose     bool
}

// NewSimReporter creates a reporter with the given window size.
func NewSimReporter(windowTicks int, verbose bool) *SimReporter {
	if windowTicks <= 0 {
		windowTicks = reportWindowTicks
	}
	return &SimReporter{
		windowTicks: windowTicks,
		verbose:     verbose,
	}
}

// Collect gathers a snapshot from the current simulation state.
// Call this periodically (e.g. every 60 ticks / 1s).
func (r *SimReporter) Collect(tick int, soldiers []*Soldier, opfor []*Soldier, squads []*Squad) {
	report := SimReport{
		Tick:      tick,
		RedGoals:  make(map[GoalKind]int),
		BlueGoals: make(map[GoalKind]int),
	}

	// Soldiers.
	for _, s := range soldiers {
		r.tallySoldier(s, &report, TeamRed)
	}
	for _, s := range opfor {
		r.tallySoldier(s, &report, TeamBlue)
	}

	// Squads.
	for _, sq := range squads {
		sr := SquadReport{
			Team:    sq.Team,
			SquadID: sq.ID,
			Intent:  sq.Intent,
		}
		for _, m := range sq.Members {
			if m.state == SoldierStateDead {
				sr.Dead++
			} else {
				sr.Alive++
				sr.AvgFear += m.profile.Psych.EffectiveFear()
				sr.AvgMorale += m.profile.Psych.Morale
				if m.blackboard.VisibleThreatCount() > 0 {
					sr.MembersWithContact++
				}
				sr.TotalEnemiesSeen += m.blackboard.VisibleThreatCount()
				sr.OutnumberedFactor = m.blackboard.OutnumberedFactor
				sr.Posture = m.blackboard.SquadPosture
			}
		}
		if sr.Alive > 0 {
			sr.AvgFear /= float64(sr.Alive)
			sr.AvgMorale /= float64(sr.Alive)
		}
		report.Squads = append(report.Squads, sr)

		if sq.Team == TeamRed {
			report.RedAvgPosture += sr.Posture
		} else {
			report.BlueAvgPosture += sr.Posture
		}
	}

	// Average posture across squads per team.
	redSquads, blueSquads := 0, 0
	for _, sq := range squads {
		if sq.Team == TeamRed {
			redSquads++
		} else {
			blueSquads++
		}
	}
	if redSquads > 0 {
		report.RedAvgPosture /= float64(redSquads)
	}
	if blueSquads > 0 {
		report.BlueAvgPosture /= float64(blueSquads)
	}

	r.history = append(r.history, report)

	// Prune old history beyond 2x window to prevent unbounded growth.
	maxKeep := r.windowTicks / 60 * 2 // reports per second * 2 windows
	if maxKeep < 100 {
		maxKeep = 100
	}
	if len(r.history) > maxKeep {
		r.history = r.history[len(r.history)-maxKeep:]
	}
}

func (r *SimReporter) tallySoldier(s *Soldier, report *SimReport, team Team) {
	goals := report.RedGoals
	if team == TeamBlue {
		goals = report.BlueGoals
	}

	if s.state == SoldierStateDead {
		if team == TeamRed {
			report.RedDead++
		} else {
			report.BlueDead++
		}
		return
	}

	if team == TeamRed {
		report.RedAlive++
	} else {
		report.BlueAlive++
	}

	goals[s.blackboard.CurrentGoal]++

	if s.health < soldierMaxHP && s.health > 0 {
		if team == TeamRed {
			report.RedInjured++
		} else {
			report.BlueInjured++
		}
	}

	if s.blackboard.VisibleThreatCount() > 0 {
		if team == TeamRed {
			report.RedMembersWithContact++
			report.RedTotalEnemiesSeen += s.blackboard.VisibleThreatCount()
		} else {
			report.BlueMembersWithContact++
			report.BlueTotalEnemiesSeen += s.blackboard.VisibleThreatCount()
		}
	}

	leaderDist := 0.0
	detached := false
	if s.squad != nil && s.squad.Leader != nil && s.squad.Leader != s {
		leaderDist = math.Hypot(s.squad.Leader.x-s.x, s.squad.Leader.y-s.y)
		detached = squadHasContact(s.squad) && s.blackboard.VisibleThreatCount() == 0 && leaderDist > effectivenessDetachedLeaderDist
	}
	stalled := (s.blackboard.VisibleThreatCount() > 0 || s.blackboard.SquadHasContact || s.blackboard.IsActivated()) &&
		s.state == SoldierStateIdle &&
		reporterMobilityGoal(s.blackboard.CurrentGoal)

	if stalled {
		if team == TeamRed {
			report.RedStalledInCombat++
		} else {
			report.BlueStalledInCombat++
		}
	}
	if detached {
		if team == TeamRed {
			report.RedDetached++
		} else {
			report.BlueDetached++
		}
	}

	if r.verbose {
		report.Soldiers = append(report.Soldiers, SoldierReport{
			ID:                          s.id,
			Label:                       s.label,
			Team:                        team,
			Goal:                        s.blackboard.CurrentGoal,
			State:                       s.state,
			Health:                      s.health,
			Fear:                        s.profile.Psych.EffectiveFear(),
			Morale:                      s.profile.Psych.Morale,
			AtCorner:                    s.blackboard.AtCorner,
			AtWall:                      s.blackboard.AtWall,
			AtDoor:                      s.blackboard.AtDoorway,
			Posture:                     s.blackboard.SquadPosture,
			StalledInCombat:             stalled,
			DetachedFromSquadEngagement: detached,
			LeaderDistance:              leaderDist,
		})
	}
}

func reporterMobilityGoal(g GoalKind) bool {
	switch g {
	case GoalAdvance, GoalMaintainFormation, GoalMoveToContact, GoalRegroup, GoalFallback, GoalFlank:
		return true
	default:
		return false
	}
}

// Latest returns the most recent report, or nil if none collected yet.
func (r *SimReporter) Latest() *SimReport {
	if len(r.history) == 0 {
		return nil
	}
	return &r.history[len(r.history)-1]
}

// WindowSummary returns an aggregated summary over the recent time window.
// It averages goal proportions, injuries, visibility, and posture across all
// reports in the window.
func (r *SimReporter) WindowSummary() *WindowReport {
	if len(r.history) == 0 {
		return nil
	}

	// Find reports within the window.
	latestTick := r.history[len(r.history)-1].Tick
	cutoff := latestTick - r.windowTicks
	var window []SimReport
	for i := len(r.history) - 1; i >= 0; i-- {
		if r.history[i].Tick < cutoff {
			break
		}
		window = append(window, r.history[i])
	}
	if len(window) == 0 {
		return nil
	}

	n := float64(len(window))
	wr := &WindowReport{
		FromTick:    window[len(window)-1].Tick,
		ToTick:      window[0].Tick,
		SampleCount: len(window),
		RedGoalPct:  make(map[GoalKind]float64),
		BlueGoalPct: make(map[GoalKind]float64),
	}

	redGoalTotal := make(map[GoalKind]float64)
	blueGoalTotal := make(map[GoalKind]float64)
	var redTotal, blueTotal float64

	for _, rpt := range window {
		for g, c := range rpt.RedGoals {
			redGoalTotal[g] += float64(c)
			redTotal += float64(c)
		}
		for g, c := range rpt.BlueGoals {
			blueGoalTotal[g] += float64(c)
			blueTotal += float64(c)
		}

		wr.AvgRedAlive += float64(rpt.RedAlive)
		wr.AvgBlueAlive += float64(rpt.BlueAlive)
		wr.AvgRedInjured += float64(rpt.RedInjured)
		wr.AvgBlueInjured += float64(rpt.BlueInjured)
		wr.AvgRedWithContact += float64(rpt.RedMembersWithContact)
		wr.AvgBlueWithContact += float64(rpt.BlueMembersWithContact)
		wr.AvgRedEnemiesSeen += float64(rpt.RedTotalEnemiesSeen)
		wr.AvgBlueEnemiesSeen += float64(rpt.BlueTotalEnemiesSeen)
		wr.AvgRedPosture += rpt.RedAvgPosture
		wr.AvgBluePosture += rpt.BlueAvgPosture
		wr.AvgRedStalledInCombat += float64(rpt.RedStalledInCombat)
		wr.AvgBlueStalledInCombat += float64(rpt.BlueStalledInCombat)
		wr.AvgRedDetached += float64(rpt.RedDetached)
		wr.AvgBlueDetached += float64(rpt.BlueDetached)
		wr.TotalRedDead += rpt.RedDead
		wr.TotalBlueDead += rpt.BlueDead
	}

	// Compute goal percentages.
	if redTotal > 0 {
		for g, c := range redGoalTotal {
			wr.RedGoalPct[g] = c / redTotal * 100
		}
	}
	if blueTotal > 0 {
		for g, c := range blueGoalTotal {
			wr.BlueGoalPct[g] = c / blueTotal * 100
		}
	}

	// Averages.
	wr.AvgRedAlive /= n
	wr.AvgBlueAlive /= n
	wr.AvgRedInjured /= n
	wr.AvgBlueInjured /= n
	wr.AvgRedWithContact /= n
	wr.AvgBlueWithContact /= n
	wr.AvgRedEnemiesSeen /= n
	wr.AvgBlueEnemiesSeen /= n
	wr.AvgRedPosture /= n
	wr.AvgBluePosture /= n
	wr.AvgRedStalledInCombat /= n
	wr.AvgBlueStalledInCombat /= n
	wr.AvgRedDetached /= n
	wr.AvgBlueDetached /= n

	return wr
}

// WindowReport is an aggregated summary over a time window.
type WindowReport struct {
	FromTick, ToTick int
	SampleCount      int

	// Goal distribution as percentages (0-100).
	RedGoalPct  map[GoalKind]float64
	BlueGoalPct map[GoalKind]float64

	// Averages over the window.
	AvgRedAlive, AvgBlueAlive                     float64
	AvgRedInjured, AvgBlueInjured                 float64
	AvgRedWithContact, AvgBlueWithContact         float64
	AvgRedEnemiesSeen, AvgBlueEnemiesSeen         float64
	AvgRedPosture, AvgBluePosture                 float64
	AvgRedStalledInCombat, AvgBlueStalledInCombat float64
	AvgRedDetached, AvgBlueDetached               float64

	// Cumulative.
	TotalRedDead, TotalBlueDead int
}

// Format returns a human-readable multi-line string of the window summary.
func (wr *WindowReport) Format() string {
	if wr == nil {
		return "No data collected yet.\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Behaviour Report (T=%d..%d, %d samples) ===\n",
		wr.FromTick, wr.ToTick, wr.SampleCount)

	// Goal proportions.
	allGoals := []GoalKind{
		GoalAdvance, GoalMaintainFormation, GoalRegroup, GoalHoldPosition,
		GoalSurvive, GoalEngage, GoalMoveToContact, GoalFallback, GoalFlank, GoalOverwatch,
	}
	sb.WriteString("\n--- RED Goal Distribution ---\n")
	for _, g := range allGoals {
		if pct, ok := wr.RedGoalPct[g]; ok && pct > 0.5 {
			fmt.Fprintf(&sb, "  %-18s %5.1f%%\n", g, pct)
		}
	}
	sb.WriteString("\n--- BLUE Goal Distribution ---\n")
	for _, g := range allGoals {
		if pct, ok := wr.BlueGoalPct[g]; ok && pct > 0.5 {
			fmt.Fprintf(&sb, "  %-18s %5.1f%%\n", g, pct)
		}
	}

	// Casualties & injuries.
	sb.WriteString("\n--- Casualties & Health ---\n")
	fmt.Fprintf(&sb, "  Red:  alive=%.0f  injured=%.1f  dead=%d\n",
		wr.AvgRedAlive, wr.AvgRedInjured, wr.TotalRedDead)
	fmt.Fprintf(&sb, "  Blue: alive=%.0f  injured=%.1f  dead=%d\n",
		wr.AvgBlueAlive, wr.AvgBlueInjured, wr.TotalBlueDead)

	// Visibility.
	sb.WriteString("\n--- Visibility & Contact ---\n")
	fmt.Fprintf(&sb, "  Red:  members w/ contact=%.1f  total enemies seen=%.1f\n",
		wr.AvgRedWithContact, wr.AvgRedEnemiesSeen)
	fmt.Fprintf(&sb, "  Blue: members w/ contact=%.1f  total enemies seen=%.1f\n",
		wr.AvgBlueWithContact, wr.AvgBlueEnemiesSeen)

	// Posture.
	sb.WriteString("\n--- Squad Posture (offensive +1 / defensive -1) ---\n")
	fmt.Fprintf(&sb, "  Red:  avg posture=%+.2f (%s)\n",
		wr.AvgRedPosture, postureLabel(wr.AvgRedPosture))
	fmt.Fprintf(&sb, "  Blue: avg posture=%+.2f (%s)\n",
		wr.AvgBluePosture, postureLabel(wr.AvgBluePosture))

	// Combat effectiveness.
	sb.WriteString("\n--- Combat Effectiveness Alerts ---\n")
	fmt.Fprintf(&sb, "  Red:  stalled_in_combat=%.1f  detached_from_engagement=%.1f\n",
		wr.AvgRedStalledInCombat, wr.AvgRedDetached)
	fmt.Fprintf(&sb, "  Blue: stalled_in_combat=%.1f  detached_from_engagement=%.1f\n",
		wr.AvgBlueStalledInCombat, wr.AvgBlueDetached)

	return sb.String()
}

func postureLabel(p float64) string {
	switch {
	case p > 0.5:
		return "aggressive push"
	case p > 0.15:
		return "offensive"
	case p > -0.15:
		return "balanced"
	case p > -0.5:
		return "defensive"
	default:
		return "full defensive"
	}
}

// FormatLatest returns a concise snapshot of the most recent collected report.
func (r *SimReporter) FormatLatest() string {
	rpt := r.Latest()
	if rpt == nil {
		return "No data.\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- Snapshot T=%d ---\n", rpt.Tick)
	fmt.Fprintf(&sb, "Red:  alive=%d dead=%d injured=%d  contact=%d enemies_seen=%d  posture=%+.2f\n",
		rpt.RedAlive, rpt.RedDead, rpt.RedInjured,
		rpt.RedMembersWithContact, rpt.RedTotalEnemiesSeen, rpt.RedAvgPosture)
	fmt.Fprintf(&sb, "      stalled_in_combat=%d detached=%d\n", rpt.RedStalledInCombat, rpt.RedDetached)
	fmt.Fprintf(&sb, "Blue: alive=%d dead=%d injured=%d  contact=%d enemies_seen=%d  posture=%+.2f\n",
		rpt.BlueAlive, rpt.BlueDead, rpt.BlueInjured,
		rpt.BlueMembersWithContact, rpt.BlueTotalEnemiesSeen, rpt.BlueAvgPosture)
	fmt.Fprintf(&sb, "      stalled_in_combat=%d detached=%d\n", rpt.BlueStalledInCombat, rpt.BlueDetached)

	sb.WriteString("Red goals:  ")
	for g, c := range rpt.RedGoals {
		fmt.Fprintf(&sb, "%s=%d ", g, c)
	}
	sb.WriteString("\nBlue goals: ")
	for g, c := range rpt.BlueGoals {
		fmt.Fprintf(&sb, "%s=%d ", g, c)
	}
	sb.WriteByte('\n')
	return sb.String()
}

// History returns all collected reports.
func (r *SimReporter) History() []SimReport {
	return r.history
}

// GoalProportions computes the proportion of each goal across all alive soldiers
// at the current moment. Returns a map of GoalKind → fraction (0-1).
func GoalProportions(soldiers []*Soldier) map[GoalKind]float64 {
	counts := make(map[GoalKind]int)
	total := 0
	for _, s := range soldiers {
		if s.state == SoldierStateDead {
			continue
		}
		counts[s.blackboard.CurrentGoal]++
		total++
	}
	props := make(map[GoalKind]float64, len(counts))
	if total > 0 {
		for g, c := range counts {
			props[g] = float64(c) / float64(total)
		}
	}
	return props
}

// OutnumberedRatio computes the ratio of enemies seen by friendlies vs friendlies
// who can see an enemy. Returns (enemies_seen, friendlies_with_contact, ratio).
func OutnumberedRatio(soldiers []*Soldier) (int, int, float64) {
	enemiesSeen := 0
	withContact := 0
	for _, s := range soldiers {
		if s.state == SoldierStateDead {
			continue
		}
		n := s.blackboard.VisibleThreatCount()
		if n > 0 {
			withContact++
			enemiesSeen += n
		}
	}
	ratio := 1.0
	if withContact > 0 {
		ratio = float64(enemiesSeen) / float64(withContact)
	}
	return enemiesSeen, withContact, ratio
}

// ensure math import is used
var _ = math.Max
