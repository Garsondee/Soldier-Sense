package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Performance grading thresholds.
const (
	perfOutnumberedThresh  = 1.2
	perfOutnumberingThresh = 0.8
	perfMinCombatTicks     = 30
	perfMinUnderFireTicks  = 10
)

// ---------------------------------------------------------------------------
// PerfTracker — per-soldier, per-tick accumulator
// ---------------------------------------------------------------------------

// PerfTracker accumulates per-tick performance metrics for one soldier.
type PerfTracker struct {
	Label string
	Team  Team
	ID    int

	// Lifecycle.
	TicksAlive int
	Survived   bool

	// Situation time (ticks).
	TicksPreCombat        int
	TicksInCombat         int
	TicksWithEnemyVisible int
	TicksUnderFire        int
	TicksSuppressed       int
	TicksOutnumbered      int
	TicksOutnumbering     int
	TicksAtCloseRange     int
	TicksAtMidRange       int
	TicksAtLongRange      int

	// Goal-time counters.
	TicksEngaging      int
	TicksFlanking      int
	TicksMoveToContact int
	TicksFallback      int
	TicksSurvive       int
	TicksOverwatch     int
	TicksHold          int
	TicksAdvance       int
	TicksFormation     int

	// State-time counters.
	TicksMoving  int
	TicksIdle    int
	TicksInCover int

	// Quality metrics.
	TicksStalledInCombat    int
	TicksDetached           int
	TicksFrozenInOpen       int
	TicksInGoodPosition     int
	TicksCoverWhileEngaged  int
	TicksAdvancingLongRange int
	TicksPanicLocked        int
	TicksEngagingUnderFire  int

	// Map context — set once so grading can adapt to scenario terrain.
	HasBuildingAccess bool

	// Aggregates.
	GoalChanges      int
	DistanceTraveled float64
	PeakFear         float64
	FearSum          float64
	DamageTaken      float64
	HealthAtEnd      float64

	// Internal — change detection.
	prevGoal GoalKind
	prevX    float64
	prevY    float64
}

// NewPerfTracker creates a tracker seeded from the soldier's initial state.
// hasBuildingAccess should be true when the map contains buildings/cover objects.
func NewPerfTracker(s *Soldier, hasBuildingAccess bool) *PerfTracker {
	return &PerfTracker{
		Label:             s.label,
		Team:              s.team,
		ID:                s.id,
		prevX:             s.x,
		prevY:             s.y,
		prevGoal:          s.blackboard.CurrentGoal,
		HasBuildingAccess: hasBuildingAccess,
	}
}

// Update accumulates one tick of data from the soldier's current state.
func (pt *PerfTracker) Update(s *Soldier) {
	if s.state == SoldierStateDead {
		return
	}
	pt.TicksAlive++

	// Movement distance.
	dist := math.Hypot(s.x-pt.prevX, s.y-pt.prevY)
	pt.DistanceTraveled += dist
	pt.prevX, pt.prevY = s.x, s.y

	// Goal change detection.
	if s.blackboard.CurrentGoal != pt.prevGoal {
		pt.GoalChanges++
		pt.prevGoal = s.blackboard.CurrentGoal
	}

	// Fear.
	ef := s.profile.Psych.EffectiveFear()
	pt.FearSum += ef
	if ef > pt.PeakFear {
		pt.PeakFear = ef
	}

	// Damage.
	healthLost := soldierMaxHP - s.health
	if healthLost > pt.DamageTaken {
		pt.DamageTaken = healthLost
	}
	pt.HealthAtEnd = s.health

	if s.blackboard.PanicLocked {
		pt.TicksPanicLocked++
	}

	// --- Situation classification ---
	combatContext := s.blackboard.VisibleThreatCount() > 0 ||
		s.blackboard.SquadHasContact ||
		s.blackboard.IsActivated()
	underFire := s.blackboard.IncomingFireCount > 0
	visibleThreats := s.blackboard.VisibleThreatCount()

	if combatContext {
		pt.TicksInCombat++
	} else {
		pt.TicksPreCombat++
	}
	if visibleThreats > 0 {
		pt.TicksWithEnemyVisible++
	}
	if underFire {
		pt.TicksUnderFire++
	}
	if s.blackboard.IsSuppressed() {
		pt.TicksSuppressed++
	}

	// Outnumbered / outnumbering.
	of := s.blackboard.OutnumberedFactor
	if of > perfOutnumberedThresh {
		pt.TicksOutnumbered++
	} else if of > 0 && of < perfOutnumberingThresh {
		pt.TicksOutnumbering++
	}

	// Range classification (only when threats visible).
	closestDist := math.MaxFloat64
	if visibleThreats > 0 {
		closestDist = s.blackboard.ClosestVisibleThreatDist(s.x, s.y)
		switch {
		case closestDist <= float64(autoRange):
			pt.TicksAtCloseRange++
		case closestDist <= float64(burstRange):
			pt.TicksAtMidRange++
		default:
			pt.TicksAtLongRange++
		}
	}

	// --- Goal-time counters ---
	switch s.blackboard.CurrentGoal {
	case GoalEngage:
		pt.TicksEngaging++
	case GoalFlank:
		pt.TicksFlanking++
	case GoalMoveToContact:
		pt.TicksMoveToContact++
	case GoalFallback:
		pt.TicksFallback++
	case GoalSurvive:
		pt.TicksSurvive++
	case GoalOverwatch:
		pt.TicksOverwatch++
	case GoalHoldPosition:
		pt.TicksHold++
	case GoalAdvance:
		pt.TicksAdvance++
	case GoalMaintainFormation:
		pt.TicksFormation++
	}

	// --- State-time counters ---
	switch s.state {
	case SoldierStateMoving:
		pt.TicksMoving++
	case SoldierStateIdle:
		pt.TicksIdle++
	case SoldierStateCover:
		pt.TicksInCover++
	}

	// --- Quality metrics ---
	inGoodPosition := s.blackboard.AtCorner ||
		s.blackboard.AtWall ||
		s.blackboard.AtWindowAdj ||
		(s.blackboard.AtInterior && !s.blackboard.AtDoorway)

	if combatContext && inGoodPosition {
		pt.TicksInGoodPosition++
	}

	// Stalled in combat: idle + combat context + mobility goal.
	if combatContext && s.state == SoldierStateIdle && reporterMobilityGoal(s.blackboard.CurrentGoal) {
		pt.TicksStalledInCombat++
	}

	// Frozen in open: idle under fire without good position.
	if underFire && s.state == SoldierStateIdle && !inGoodPosition {
		pt.TicksFrozenInOpen++
	}

	// Cover while engaged: engaging from a protected position.
	if s.blackboard.CurrentGoal == GoalEngage && (inGoodPosition || s.state == SoldierStateCover) {
		pt.TicksCoverWhileEngaged++
	}

	// Engaging under fire: returning fire while receiving.
	if underFire && s.blackboard.CurrentGoal == GoalEngage {
		pt.TicksEngagingUnderFire++
	}

	// Advancing at long range: closing when beyond effective range.
	if visibleThreats > 0 && closestDist > float64(burstRange) {
		if s.blackboard.CurrentGoal == GoalMoveToContact || s.blackboard.CurrentGoal == GoalFlank {
			pt.TicksAdvancingLongRange++
		}
	}

	// Detached from squad engagement.
	if s.squad != nil && s.squad.Leader != nil && s.squad.Leader != s {
		leaderDist := math.Hypot(s.squad.Leader.x-s.x, s.squad.Leader.y-s.y)
		if squadHasContact(s.squad) && visibleThreats == 0 && leaderDist > effectivenessDetachedLeaderDist {
			pt.TicksDetached++
		}
	}
}

// Finalize snapshots end-of-run state.
func (pt *PerfTracker) Finalize(s *Soldier) {
	pt.Survived = s.state != SoldierStateDead
	pt.HealthAtEnd = s.health
}

// ---------------------------------------------------------------------------
// SoldierGrade — computed performance result
// ---------------------------------------------------------------------------

// SoldierGrade is the computed performance grade for one soldier.
type SoldierGrade struct {
	Label    string
	Team     Team
	ID       int
	Grade    string  // A+, A, B+, B, C+, C, D, F
	Score    float64 // 0-100
	Survived bool

	// Situation scores (0-100; -1 = not enough data to grade).
	FirefightScore   float64
	UnderFireScore   float64
	PositioningScore float64
	AggressionScore  float64
	ComposureScore   float64
	TeamworkScore    float64

	// Observed traits.
	GoodTraits []string
	BadTraits  []string

	// Key stats.
	CombatTimePct float64
	PeakFear      float64
	AvgFear       float64
	DamageTaken   float64
}

// ---------------------------------------------------------------------------
// Grading logic
// ---------------------------------------------------------------------------

// GradePerformance computes grades from accumulated tracker data.
func GradePerformance(trackers map[int]*PerfTracker) []SoldierGrade {
	grades := make([]SoldierGrade, 0, len(trackers))
	for _, pt := range trackers {
		grades = append(grades, computeGrade(pt))
	}
	sort.Slice(grades, func(i, j int) bool {
		if grades[i].Team != grades[j].Team {
			return grades[i].Team < grades[j].Team
		}
		return grades[i].Score > grades[j].Score
	})
	return grades
}

func computeGrade(pt *PerfTracker) SoldierGrade {
	g := SoldierGrade{
		Label:            pt.Label,
		Team:             pt.Team,
		ID:               pt.ID,
		Survived:         pt.Survived,
		PeakFear:         pt.PeakFear,
		DamageTaken:      pt.DamageTaken,
		FirefightScore:   -1,
		UnderFireScore:   -1,
		PositioningScore: -1,
		AggressionScore:  -1,
		ComposureScore:   -1,
		TeamworkScore:    -1,
	}

	if pt.TicksAlive > 0 {
		g.AvgFear = pt.FearSum / float64(pt.TicksAlive)
		g.CombatTimePct = float64(pt.TicksInCombat) / float64(pt.TicksAlive) * 100
	}

	combat := math.Max(1, float64(pt.TicksInCombat))
	uf := math.Max(1, float64(pt.TicksUnderFire))

	// --- Firefight: how effectively the soldier fights when enemies are visible ---
	if pt.TicksWithEnemyVisible >= perfMinCombatTicks {
		s := 50.0
		s += 25.0 * perfFrac(pt.TicksEngaging, pt.TicksWithEnemyVisible)
		s += 15.0 * perfFrac(pt.TicksCoverWhileEngaged, pt.TicksWithEnemyVisible)
		s -= 30.0 * perfFrac(pt.TicksStalledInCombat, pt.TicksInCombat)
		s += 10.0 * perfFrac(pt.TicksFlanking, pt.TicksInCombat)
		g.FirefightScore = perfClamp(s)
	}

	// --- Under fire: behaviour while receiving rounds ---
	if pt.TicksUnderFire >= perfMinUnderFireTicks {
		s := 50.0
		s += 30.0 * (1.0 - float64(pt.TicksFrozenInOpen)/uf)
		s += 20.0 * float64(pt.TicksEngagingUnderFire) / uf
		s -= 25.0 * float64(pt.TicksPanicLocked) / uf
		g.UnderFireScore = perfClamp(s)
	}

	// --- Positioning: use of tactical terrain ---
	// Only grade positioning when the map has buildings to use.
	if pt.TicksInCombat >= perfMinCombatTicks && pt.HasBuildingAccess {
		s := 50.0
		s += 30.0 * float64(pt.TicksInGoodPosition) / combat
		s -= 15.0 * float64(pt.TicksFrozenInOpen) / combat
		g.PositioningScore = perfClamp(s)
	} else if pt.TicksInCombat >= perfMinCombatTicks {
		// Open field: base score with minor frozen-in-open penalty.
		s := 65.0
		s -= 15.0 * float64(pt.TicksFrozenInOpen) / combat
		g.PositioningScore = perfClamp(s)
	}

	// --- Aggression: willingness to close and press advantage ---
	if pt.TicksInCombat >= perfMinCombatTicks {
		s := 50.0
		s += 20.0 * float64(pt.TicksMoveToContact) / combat
		if pt.TicksAtLongRange > 0 {
			s += 15.0 * perfFrac(pt.TicksAdvancingLongRange, pt.TicksAtLongRange)
		}
		if pt.TicksOutnumbering > 60 {
			passive := float64(pt.TicksOverwatch + pt.TicksHold)
			s -= 15.0 * passive / float64(pt.TicksOutnumbering)
		}
		s -= 10.0 * float64(pt.TicksFallback) / combat
		g.AggressionScore = perfClamp(s)
	}

	// --- Composure: fear management under pressure ---
	if pt.TicksAlive > 60 {
		s := 50.0
		s += 30.0 * (1.0 - pt.PeakFear)
		if pt.TicksInCombat > 0 {
			s -= 25.0 * float64(pt.TicksSurvive) / combat
			s -= 15.0 * float64(pt.TicksPanicLocked) / combat
		}
		g.ComposureScore = perfClamp(s)
	}

	// --- Teamwork: squad cohesion ---
	if pt.TicksInCombat >= perfMinCombatTicks {
		s := 70.0
		s -= 40.0 * float64(pt.TicksDetached) / combat
		g.TeamworkScore = perfClamp(s)
	}

	// --- Overall weighted average ---
	type scoredWeight struct {
		score  float64
		weight float64
	}
	var items []scoredWeight
	if g.FirefightScore >= 0 {
		items = append(items, scoredWeight{g.FirefightScore, 0.30})
	}
	if g.UnderFireScore >= 0 {
		items = append(items, scoredWeight{g.UnderFireScore, 0.25})
	}
	if g.PositioningScore >= 0 {
		items = append(items, scoredWeight{g.PositioningScore, 0.15})
	}
	if g.AggressionScore >= 0 {
		items = append(items, scoredWeight{g.AggressionScore, 0.10})
	}
	if g.ComposureScore >= 0 {
		items = append(items, scoredWeight{g.ComposureScore, 0.10})
	}
	if g.TeamworkScore >= 0 {
		items = append(items, scoredWeight{g.TeamworkScore, 0.10})
	}

	if len(items) > 0 {
		totalW := 0.0
		totalS := 0.0
		for _, it := range items {
			totalW += it.weight
			totalS += it.score * it.weight
		}
		g.Score = totalS / totalW
	} else {
		g.Score = 50.0
		if pt.TicksAlive > 0 {
			g.Score += perfFrac(pt.TicksMoving, pt.TicksAlive) * 30.0
		}
	}

	if pt.Survived {
		g.Score = math.Min(100, g.Score+5)
	}

	g.Grade = PerfLetterGrade(g.Score)
	g.GoodTraits, g.BadTraits = perfDetectTraits(pt)
	return g
}

// ---------------------------------------------------------------------------
// Trait detection
// ---------------------------------------------------------------------------

func perfDetectTraits(pt *PerfTracker) (good, bad []string) {
	combat := math.Max(1, float64(pt.TicksInCombat))
	uf := math.Max(1, float64(pt.TicksUnderFire))

	// ----- GOOD traits -----

	// Disciplined fire: high engage rate with cover usage.
	if pt.TicksWithEnemyVisible >= perfMinCombatTicks {
		vis := math.Max(1, float64(pt.TicksWithEnemyVisible))
		engR := float64(pt.TicksEngaging) / vis
		covR := float64(pt.TicksCoverWhileEngaged) / vis
		if engR > 0.40 && covR > 0.25 {
			good = append(good, "disciplined_fire")
		}
	}

	// Good cover usage under fire.
	if pt.TicksUnderFire >= perfMinUnderFireTicks {
		covered := float64(pt.TicksInGoodPosition+pt.TicksInCover) / uf
		if covered > 0.40 {
			good = append(good, "good_cover_usage")
		}
	}

	// Effective flanker.
	if pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksFlanking)/combat > 0.10 {
		good = append(good, "effective_flanker")
	}

	// Composure under fire.
	if pt.PeakFear < 0.45 && pt.TicksUnderFire > 30 {
		good = append(good, "composure_under_fire")
	}

	// Tactical positioning (only meaningful when buildings exist).
	if pt.HasBuildingAccess && pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksInGoodPosition)/combat > 0.30 {
		good = append(good, "tactical_positioning")
	}

	// Team player: stays with squad.
	if pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksDetached)/combat < 0.05 {
		good = append(good, "team_player")
	}

	// Aggressive closer: advances when at long range.
	if pt.TicksAtLongRange > 30 && perfFrac(pt.TicksAdvancingLongRange, pt.TicksAtLongRange) > 0.30 {
		good = append(good, "aggressive_closer")
	}

	// Decisive: low goal-change frequency during combat.
	if pt.TicksInCombat > 120 {
		changesPerSec := float64(pt.GoalChanges) / (combat / 60.0)
		if changesPerSec < 0.5 {
			good = append(good, "decisive")
		}
	}

	// Returns fire under pressure.
	if pt.TicksUnderFire >= perfMinUnderFireTicks && float64(pt.TicksEngagingUnderFire)/uf > 0.30 {
		good = append(good, "returns_fire_under_pressure")
	}

	// Steady advance: good movement during pre-combat.
	if pt.TicksPreCombat > 60 {
		moveR := perfFrac(pt.TicksMoving, pt.TicksPreCombat+pt.TicksInCombat)
		if moveR > 0.50 {
			good = append(good, "steady_advance")
		}
	}

	// ----- BAD traits -----

	// Frozen in open under fire.
	if pt.TicksUnderFire >= perfMinUnderFireTicks && float64(pt.TicksFrozenInOpen)/uf > 0.10 {
		bad = append(bad, "frozen_in_open")
	}

	// Stalled in combat.
	if pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksStalledInCombat)/combat > 0.15 {
		bad = append(bad, "stalled_in_combat")
	}

	// Detached from squad.
	if pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksDetached)/combat > 0.20 {
		bad = append(bad, "detached_from_squad")
	}

	// Panic prone.
	if pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksSurvive)/combat > 0.25 {
		bad = append(bad, "panic_prone")
	}

	// Indecisive: excessive goal switching.
	if pt.TicksInCombat > 120 {
		changesPerSec := float64(pt.GoalChanges) / (combat / 60.0)
		if changesPerSec > 2.0 {
			bad = append(bad, "indecisive")
		}
	}

	// Passive when advantaged.
	if pt.TicksOutnumbering > 60 {
		passive := float64(pt.TicksOverwatch+pt.TicksHold) / float64(pt.TicksOutnumbering)
		if passive > 0.50 {
			bad = append(bad, "passive_when_advantaged")
		}
	}

	// Poor positioning (only when buildings are available).
	if pt.HasBuildingAccess && pt.TicksInCombat >= perfMinCombatTicks && pt.TicksUnderFire >= perfMinUnderFireTicks {
		posR := float64(pt.TicksInGoodPosition) / combat
		if posR < 0.10 {
			bad = append(bad, "poor_positioning")
		}
	}

	// Passive at long range: not closing when should.
	if pt.TicksAtLongRange > 60 && perfFrac(pt.TicksAdvancingLongRange, pt.TicksAtLongRange) < 0.10 {
		bad = append(bad, "passive_at_long_range")
	}

	// Excessive fallback.
	if pt.TicksInCombat >= perfMinCombatTicks && float64(pt.TicksFallback)/combat > 0.30 {
		bad = append(bad, "excessive_fallback")
	}

	return
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

// FormatGrades returns a human-readable performance report.
func FormatGrades(grades []SoldierGrade) string {
	var sb strings.Builder
	sb.WriteString("\n=== Soldier Performance Grades ===\n")

	currentTeam := Team(-1)
	for _, g := range grades {
		if g.Team != currentTeam {
			currentTeam = g.Team
			teamName := "RED"
			if g.Team == TeamBlue {
				teamName = "BLUE"
			}
			fmt.Fprintf(&sb, "\n--- %s Team ---\n", teamName)
		}

		status := "survived"
		if !g.Survived {
			status = "KIA"
		}
		fmt.Fprintf(&sb, "  %-3s  %-4s  [%s]  dmg=%.0f  peak_fear=%.2f  avg_fear=%.2f  combat=%.0f%%\n",
			g.Grade, g.Label, status, g.DamageTaken, g.PeakFear, g.AvgFear, g.CombatTimePct)

		if len(g.GoodTraits) > 0 {
			fmt.Fprintf(&sb, "       Good: %s\n", strings.Join(g.GoodTraits, ", "))
		}
		if len(g.BadTraits) > 0 {
			fmt.Fprintf(&sb, "       Bad:  %s\n", strings.Join(g.BadTraits, ", "))
		}

		var scores []string
		if g.FirefightScore >= 0 {
			scores = append(scores, fmt.Sprintf("Fight=%.0f", g.FirefightScore))
		}
		if g.UnderFireScore >= 0 {
			scores = append(scores, fmt.Sprintf("UnderFire=%.0f", g.UnderFireScore))
		}
		if g.PositioningScore >= 0 {
			scores = append(scores, fmt.Sprintf("Position=%.0f", g.PositioningScore))
		}
		if g.AggressionScore >= 0 {
			scores = append(scores, fmt.Sprintf("Aggression=%.0f", g.AggressionScore))
		}
		if g.ComposureScore >= 0 {
			scores = append(scores, fmt.Sprintf("Composure=%.0f", g.ComposureScore))
		}
		if g.TeamworkScore >= 0 {
			scores = append(scores, fmt.Sprintf("Teamwork=%.0f", g.TeamworkScore))
		}
		if len(scores) > 0 {
			fmt.Fprintf(&sb, "       Scores: %s\n", strings.Join(scores, "  "))
		}
	}

	return sb.String()
}

// FormatGradesSummary returns a compact team-level summary.
func FormatGradesSummary(grades []SoldierGrade) string {
	var sb strings.Builder

	type teamStats struct {
		count     int
		scoreSum  float64
		survived  int
		goodCount map[string]int
		badCount  map[string]int
	}
	teams := map[Team]*teamStats{}
	for _, g := range grades {
		ts, ok := teams[g.Team]
		if !ok {
			ts = &teamStats{goodCount: map[string]int{}, badCount: map[string]int{}}
			teams[g.Team] = ts
		}
		ts.count++
		ts.scoreSum += g.Score
		if g.Survived {
			ts.survived++
		}
		for _, t := range g.GoodTraits {
			ts.goodCount[t]++
		}
		for _, t := range g.BadTraits {
			ts.badCount[t]++
		}
	}

	for _, team := range []Team{TeamRed, TeamBlue} {
		ts, ok := teams[team]
		if !ok {
			continue
		}
		teamName := "RED"
		if team == TeamBlue {
			teamName = "BLUE"
		}
		avg := 0.0
		if ts.count > 0 {
			avg = ts.scoreSum / float64(ts.count)
		}
		fmt.Fprintf(&sb, "  %s: avg_score=%.1f (%s)  survived=%d/%d\n",
			teamName, avg, PerfLetterGrade(avg), ts.survived, ts.count)

		if len(ts.goodCount) > 0 {
			fmt.Fprintf(&sb, "    Top good: %s\n", perfTopTraits(ts.goodCount, 4))
		}
		if len(ts.badCount) > 0 {
			fmt.Fprintf(&sb, "    Top bad:  %s\n", perfTopTraits(ts.badCount, 4))
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func perfFrac(num, denom int) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

func perfClamp(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}

// PerfLetterGrade maps a 0-100 score to a letter grade.
func PerfLetterGrade(score float64) string {
	switch {
	case score >= 93:
		return "A+"
	case score >= 85:
		return "A"
	case score >= 78:
		return "B+"
	case score >= 70:
		return "B"
	case score >= 62:
		return "C+"
	case score >= 55:
		return "C"
	case score >= 45:
		return "D"
	default:
		return "F"
	}
}

func perfTopTraits(counts map[string]int, n int) string {
	type kv struct {
		trait string
		count int
	}
	var items []kv
	for k, v := range counts {
		items = append(items, kv{k, v})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].count > items[j].count
	})
	if len(items) > n {
		items = items[:n]
	}
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = fmt.Sprintf("%s(%d)", it.trait, it.count)
	}
	return strings.Join(parts, ", ")
}

// ensure sort import is used
var _ = sort.Strings
