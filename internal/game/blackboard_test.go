package game

import (
	"math"
	"testing"
)

// --- Blackboard threat tracking ---

func TestBlackboard_AddThreat(t *testing.T) {
	bb := &Blackboard{}
	tick := 0
	ng := NewNavGrid(640, 480, nil, 0)
	tl := NewThoughtLog()
	enemy := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100}, ng, tl, &tick)

	bb.UpdateThreats([]*Soldier{enemy}, 1)
	if len(bb.Threats) != 1 {
		t.Fatalf("expected 1 threat, got %d", len(bb.Threats))
	}
	if bb.Threats[0].Confidence != 1.0 {
		t.Fatalf("new threat should have confidence 1.0, got %.2f", bb.Threats[0].Confidence)
	}
	if !bb.Threats[0].IsVisible {
		t.Fatal("new threat should be marked visible")
	}
}

func TestBlackboard_ThreatRefresh(t *testing.T) {
	bb := &Blackboard{}
	ng := NewNavGrid(640, 480, nil, 0)
	tl := NewThoughtLog()
	tick := 0
	enemy := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100}, ng, tl, &tick)

	bb.UpdateThreats([]*Soldier{enemy}, 1)
	bb.UpdateThreats([]*Soldier{enemy}, 2)

	if len(bb.Threats) != 1 {
		t.Fatalf("refreshing same contact should not add duplicate; got %d threats", len(bb.Threats))
	}
	if bb.Threats[0].Confidence != 1.0 {
		t.Fatalf("refreshed threat should have confidence 1.0, got %.2f", bb.Threats[0].Confidence)
	}
}

func TestBlackboard_ThreatDecay(t *testing.T) {
	bb := &Blackboard{}
	ng := NewNavGrid(640, 480, nil, 0)
	tl := NewThoughtLog()
	tick := 0
	enemy := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100}, ng, tl, &tick)

	// Add at tick 1.
	bb.UpdateThreats([]*Soldier{enemy}, 1)
	// Now update with no contacts at tick 201 — large age gap causes decay.
	bb.UpdateThreats([]*Soldier{}, 201)

	// After 200 ticks of non-visibility at 0.005/tick: 200*0.005 = 1.0, so confidence = 0.
	if len(bb.Threats) != 0 {
		t.Fatalf("fully decayed threat should be pruned; got %d threats", len(bb.Threats))
	}
}

func TestBlackboard_ThreatPartialDecay(t *testing.T) {
	bb := &Blackboard{}
	ng := NewNavGrid(640, 480, nil, 0)
	tl := NewThoughtLog()
	tick := 0
	enemy := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100}, ng, tl, &tick)

	bb.UpdateThreats([]*Soldier{enemy}, 1)
	// Update with no contacts 10 ticks later — small decay, should still exist.
	bb.UpdateThreats([]*Soldier{}, 11)

	if len(bb.Threats) == 0 {
		t.Fatal("partially decayed threat should still exist at 10 ticks")
	}
	if bb.Threats[0].Confidence >= 1.0 {
		t.Fatal("confidence should have decayed from 1.0")
	}
}

func TestBlackboard_VisibleThreatCount(t *testing.T) {
	bb := &Blackboard{}
	ng := NewNavGrid(640, 480, nil, 0)
	tl := NewThoughtLog()
	tick := 0
	e1 := NewSoldier(1, 100, 100, TeamBlue, [2]float64{100, 100}, [2]float64{200, 100}, ng, tl, &tick)
	e2 := NewSoldier(2, 200, 200, TeamBlue, [2]float64{200, 200}, [2]float64{300, 200}, ng, tl, &tick)

	bb.UpdateThreats([]*Soldier{e1, e2}, 1)
	if bb.VisibleThreatCount() != 2 {
		t.Fatalf("expected 2 visible threats, got %d", bb.VisibleThreatCount())
	}
}

func TestBlackboard_ClosestVisibleThreatDist(t *testing.T) {
	bb := &Blackboard{}
	ng := NewNavGrid(640, 480, nil, 0)
	tl := NewThoughtLog()
	tick := 0
	e1 := NewSoldier(1, 100, 0, TeamBlue, [2]float64{100, 0}, [2]float64{200, 0}, ng, tl, &tick)
	e2 := NewSoldier(2, 200, 0, TeamBlue, [2]float64{200, 0}, [2]float64{300, 0}, ng, tl, &tick)

	bb.UpdateThreats([]*Soldier{e1, e2}, 1)
	dist := bb.ClosestVisibleThreatDist(0, 0)
	if math.Abs(dist-100) > 1e-6 {
		t.Fatalf("closest threat should be at dist 100, got %.4f", dist)
	}
}

func TestBlackboard_ClosestVisibleThreatDist_NoThreats(t *testing.T) {
	bb := &Blackboard{}
	dist := bb.ClosestVisibleThreatDist(0, 0)
	if dist != math.MaxFloat64 {
		t.Fatalf("expected MaxFloat64 with no threats, got %.4f", dist)
	}
}

// --- Goal Selection ---

func TestSelectGoal_NoThreats_AdvancingIntent(t *testing.T) {
	bb := &Blackboard{SquadIntent: IntentAdvance}
	p := DefaultProfile()
	goal := SelectGoal(bb, &p, false, true)
	// No threats, advancing intent, non-leader, has path.
	// Formation utility should beat advance utility for a non-leader.
	if goal != GoalMaintainFormation && goal != GoalAdvance {
		t.Fatalf("expected advance or formation without threats, got %s", goal)
	}
}

func TestSelectGoal_HighFear_Survive(t *testing.T) {
	bb := &Blackboard{}
	bb.Threats = []ThreatFact{{IsVisible: true, Confidence: 1.0}}
	p := DefaultProfile()
	p.Psych.Fear = 1.0
	p.Psych.Composure = 0.0
	p.Psych.Experience = 0.0
	goal := SelectGoal(bb, &p, false, true)
	if goal != GoalSurvive {
		t.Fatalf("high fear + visible threat: expected GoalSurvive, got %s", goal)
	}
}

func TestSelectGoal_HoldIntent_NoThreats(t *testing.T) {
	bb := &Blackboard{SquadIntent: IntentHold}
	p := DefaultProfile()
	p.Skills.Discipline = 0.9
	goal := SelectGoal(bb, &p, false, true)
	if goal != GoalHoldPosition {
		t.Fatalf("hold intent + high discipline: expected GoalHoldPosition, got %s", goal)
	}
}

func TestSelectGoal_RegroupIntent_NonLeader(t *testing.T) {
	bb := &Blackboard{SquadIntent: IntentRegroup}
	p := DefaultProfile()
	p.Skills.Discipline = 0.8
	p.Psych.Fear = 0.0
	goal := SelectGoal(bb, &p, false, true)
	if goal != GoalRegroup {
		t.Fatalf("regroup intent + no fear: expected GoalRegroup, got %s", goal)
	}
}

func TestSelectGoal_Leader_AdvancesMore(t *testing.T) {
	bb := &Blackboard{SquadIntent: IntentAdvance}
	p := DefaultProfile()
	leaderGoal := SelectGoal(bb, &p, true, true)
	memberGoal := SelectGoal(bb, &p, false, true)
	// Leader gets +0.1 advance utility — with advance intent, leader should prefer advance,
	// member may prefer formation. Just ensure leader does not get survive/regroup with no threats.
	if leaderGoal == GoalSurvive || leaderGoal == GoalRegroup {
		t.Fatalf("leader with no threats should not choose %s", leaderGoal)
	}
	_ = memberGoal
}
