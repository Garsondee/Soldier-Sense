package game

import (
	"testing"
)

func TestDecisionTracker_RecordAndRetrieve(t *testing.T) {
	dt := NewDecisionTracker(true)

	// Create a test soldier
	s := &Soldier{
		id:    1,
		label: "R0",
		team:  TeamRed,
		blackboard: Blackboard{
			CurrentGoal: GoalAdvance,
		},
		profile: SoldierProfile{
			Psych: PsychState{
				Fear:   0.3,
				Morale: 0.7,
			},
		},
		body: NewBodyMap(),
	}

	utilities := map[GoalKind]float64{
		GoalAdvance:  0.8,
		GoalEngage:   0.6,
		GoalFallback: 0.2,
	}

	dt.Record(10, s, GoalAdvance, GoalEngage, utilities)

	entries := dt.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Tick != 10 {
		t.Errorf("expected tick 10, got %d", e.Tick)
	}
	if e.PrevGoal != GoalAdvance {
		t.Errorf("expected prev goal Advance, got %s", e.PrevGoal)
	}
	if e.NewGoal != GoalEngage {
		t.Errorf("expected new goal Engage, got %s", e.NewGoal)
	}
	if e.Winner != GoalAdvance {
		t.Errorf("expected winner Advance (highest util), got %s", e.Winner)
	}
}

func TestDecisionTracker_FindOscillations(t *testing.T) {
	dt := NewDecisionTracker(true)

	s := &Soldier{
		id:         1,
		label:      "R0",
		team:       TeamRed,
		blackboard: Blackboard{},
		profile: SoldierProfile{
			Psych: PsychState{},
		},
		body: NewBodyMap(),
	}

	// Create oscillation: Advance → Engage → Advance
	dt.Record(10, s, GoalAdvance, GoalAdvance, map[GoalKind]float64{GoalAdvance: 0.8})
	dt.Record(15, s, GoalAdvance, GoalEngage, map[GoalKind]float64{GoalEngage: 0.7})
	dt.Record(20, s, GoalEngage, GoalAdvance, map[GoalKind]float64{GoalAdvance: 0.75})

	oscillations := dt.FindOscillations(1, 30)
	if len(oscillations) != 1 {
		t.Fatalf("expected 1 oscillation, got %d", len(oscillations))
	}

	osc := oscillations[0]
	if osc.GoalA != GoalAdvance || osc.GoalB != GoalEngage {
		t.Errorf("expected Advance ↔ Engage oscillation, got %s ↔ %s", osc.GoalA, osc.GoalB)
	}
	if osc.Duration != 10 {
		t.Errorf("expected duration 10, got %d", osc.Duration)
	}
}

func TestDecisionTracker_FindLowMarginDecisions(t *testing.T) {
	dt := NewDecisionTracker(true)

	s := &Soldier{
		id:         1,
		label:      "R0",
		team:       TeamRed,
		blackboard: Blackboard{},
		profile: SoldierProfile{
			Psych: PsychState{},
		},
		body: NewBodyMap(),
	}

	// High margin decision
	dt.Record(10, s, GoalAdvance, GoalAdvance, map[GoalKind]float64{
		GoalAdvance: 0.9,
		GoalEngage:  0.3,
	})

	// Low margin decision
	dt.Record(20, s, GoalAdvance, GoalEngage, map[GoalKind]float64{
		GoalEngage:  0.51,
		GoalAdvance: 0.50,
	})

	lowMargin := dt.FindLowMarginDecisions(0.1)
	if len(lowMargin) != 1 {
		t.Fatalf("expected 1 low-margin decision, got %d", len(lowMargin))
	}

	if lowMargin[0].Tick != 20 {
		t.Errorf("expected low-margin decision at tick 20, got %d", lowMargin[0].Tick)
	}
}

func TestDecisionTracker_Disabled(t *testing.T) {
	dt := NewDecisionTracker(false)

	s := &Soldier{
		id:         1,
		label:      "R0",
		team:       TeamRed,
		blackboard: Blackboard{},
		profile: SoldierProfile{
			Psych: PsychState{},
		},
		body: NewBodyMap(),
	}

	dt.Record(10, s, GoalAdvance, GoalEngage, map[GoalKind]float64{GoalEngage: 0.7})

	entries := dt.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries when disabled, got %d", len(entries))
	}
}
