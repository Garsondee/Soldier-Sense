package game

import (
	"fmt"
	"sort"
)

// DecisionTracker records goal utility calculations and selection decisions
// to help identify decision-making issues and oscillations.
type DecisionTracker struct {
	entries []DecisionEntry
	enabled bool
}

// DecisionEntry captures one goal selection decision with utility scores.
type DecisionEntry struct {
	Utilities  map[GoalKind]float64
	Label      string
	Context    DecisionContext
	WinnerUtil float64
	RunnerUtil float64
	Margin     float64 // winner - runnerup
	Tick       int
	SoldierID  int
	Team       Team
	PrevGoal   GoalKind
	NewGoal    GoalKind
	Winner     GoalKind
	RunnerUp   GoalKind
}

// DecisionContext captures relevant state at decision time.
type DecisionContext struct {
	Fear   float64
	Morale float64
	Health float64

	VisibleThreats   int
	IncomingFire     int
	SquadIntent      SquadIntentKind
	SquadHasContact  bool
	UnderSuppression bool
	SquadBroken      bool
	IsActivated      bool
}

// NewDecisionTracker creates a decision tracker.
func NewDecisionTracker(enabled bool) *DecisionTracker {
	return &DecisionTracker{enabled: enabled}
}

// Record captures a goal selection decision with utility scores.
func (dt *DecisionTracker) Record(tick int, s *Soldier, prevGoal, newGoal GoalKind, utilities map[GoalKind]float64) {
	if !dt.enabled {
		return
	}

	// Find winner and runner-up
	type scored struct {
		goal GoalKind
		util float64
	}
	scores := make([]scored, 0, len(utilities))
	for g, u := range utilities {
		scores = append(scores, scored{g, u})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].util > scores[j].util
	})

	winner := newGoal
	winnerUtil := 0.0
	runnerUp := GoalKind(0)
	runnerUtil := 0.0
	if len(scores) > 0 {
		winner = scores[0].goal
		winnerUtil = scores[0].util
	}
	if len(scores) > 1 {
		runnerUp = scores[1].goal
		runnerUtil = scores[1].util
	}

	entry := DecisionEntry{
		Tick:       tick,
		SoldierID:  s.id,
		Label:      s.label,
		Team:       s.team,
		PrevGoal:   prevGoal,
		NewGoal:    newGoal,
		Utilities:  utilities,
		Winner:     winner,
		WinnerUtil: winnerUtil,
		RunnerUp:   runnerUp,
		RunnerUtil: runnerUtil,
		Margin:     winnerUtil - runnerUtil,
		Context: DecisionContext{
			VisibleThreats:   s.blackboard.VisibleThreatCount(),
			SquadHasContact:  s.blackboard.SquadHasContact,
			IncomingFire:     s.blackboard.IncomingFireCount,
			UnderSuppression: s.blackboard.IsSuppressed(),
			Fear:             s.profile.Psych.EffectiveFear(),
			Morale:           s.profile.Psych.Morale,
			Health:           s.body.HealthFraction(),
			SquadIntent:      s.blackboard.SquadIntent,
			SquadBroken:      s.blackboard.SquadBroken,
			IsActivated:      s.blackboard.IsActivated(),
		},
	}

	dt.entries = append(dt.entries, entry)
}

// Entries returns all recorded decisions.
func (dt *DecisionTracker) Entries() []DecisionEntry {
	return dt.entries
}

// FindOscillations detects goal oscillations (A→B→A within N ticks).
func (dt *DecisionTracker) FindOscillations(soldierID, windowTicks int) []OscillationReport {
	var reports []OscillationReport

	// Group entries by soldier
	soldierEntries := make(map[int][]DecisionEntry)
	for i := range dt.entries {
		e := dt.entries[i]
		soldierEntries[e.SoldierID] = append(soldierEntries[e.SoldierID], e)
	}

	entries := soldierEntries[soldierID]
	if len(entries) < 3 {
		return nil
	}

	// Look for A→B→A patterns
	for i := 0; i < len(entries)-2; i++ {
		e1 := entries[i]
		e2 := entries[i+1]
		e3 := entries[i+2]

		if e1.NewGoal != e2.NewGoal && e1.NewGoal == e3.NewGoal {
			if e3.Tick-e1.Tick <= windowTicks {
				reports = append(reports, OscillationReport{
					SoldierID:  soldierID,
					Label:      e1.Label,
					StartTick:  e1.Tick,
					EndTick:    e3.Tick,
					Duration:   e3.Tick - e1.Tick,
					GoalA:      e1.NewGoal,
					GoalB:      e2.NewGoal,
					Entries:    []DecisionEntry{e1, e2, e3},
					AvgMarginA: (e1.Margin + e3.Margin) / 2,
					AvgMarginB: e2.Margin,
				})
			}
		}
	}

	return reports
}

// OscillationReport describes a detected goal oscillation.
type OscillationReport struct {
	Label      string
	Entries    []DecisionEntry
	AvgMarginA float64 // average utility margin when selecting A
	AvgMarginB float64 // average utility margin when selecting B
	SoldierID  int
	StartTick  int
	EndTick    int
	Duration   int
	GoalA      GoalKind
	GoalB      GoalKind
}

// String formats the oscillation report.
func (or OscillationReport) String() string {
	return fmt.Sprintf("%s oscillation T=%d-%d (%dt): %s ↔ %s (margins: %.2f, %.2f)",
		or.Label, or.StartTick, or.EndTick, or.Duration,
		or.GoalA, or.GoalB, or.AvgMarginA, or.AvgMarginB)
}

// FindLowMarginDecisions finds decisions where winner barely beat runner-up.
func (dt *DecisionTracker) FindLowMarginDecisions(threshold float64) []DecisionEntry {
	var low []DecisionEntry
	for i := range dt.entries {
		e := dt.entries[i]
		if e.Margin < threshold && e.Margin > 0 {
			low = append(low, e)
		}
	}
	return low
}

// FindGoalChanges returns all entries where goal actually changed.
func (dt *DecisionTracker) FindGoalChanges() []DecisionEntry {
	var changes []DecisionEntry
	for i := range dt.entries {
		e := dt.entries[i]
		if e.PrevGoal != e.NewGoal {
			changes = append(changes, e)
		}
	}
	return changes
}

// SummarizeOscillations generates a report of all oscillations found.
func (dt *DecisionTracker) SummarizeOscillations(windowTicks int) string {
	soldierIDs := make(map[int]bool)
	for i := range dt.entries {
		e := dt.entries[i]
		soldierIDs[e.SoldierID] = true
	}

	var allReports []OscillationReport
	for id := range soldierIDs {
		reports := dt.FindOscillations(id, windowTicks)
		allReports = append(allReports, reports...)
	}

	if len(allReports) == 0 {
		return "No oscillations detected.\n"
	}

	result := fmt.Sprintf("Found %d oscillations:\n", len(allReports))
	for _, r := range allReports {
		result += "  " + r.String() + "\n"
	}
	return result
}
