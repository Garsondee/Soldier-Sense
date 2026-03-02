package game

import "testing"

func TestSelectGoal_SearchEligibleWhenCalmAndDriveHigh(t *testing.T) {
	bb := &Blackboard{}
	bb.CurrentGoal = GoalHoldPosition
	bb.SquadIntent = IntentHold
	bb.SquadStress = 0.2
	bb.SquadBroken = false
	bb.VisibleAllyCount = 2
	bb.SearchDrive = 1.0
	bb.HeardGunfire = false
	bb.SquadHasContact = false
	bb.CombatMemoryStrength = 0
	bb.SuppressLevel = 0
	bb.IncomingFireCount = 0
	bb.OfficerOrderActive = false

	p := DefaultProfile()
	p.Psych.Fear = 0.15

	g := SelectGoal(bb, &p, false, true)
	if g != GoalSearch {
		t.Fatalf("expected GoalSearch, got %s", g)
	}
}

func TestSelectGoal_SearchIneligibleUnderContactOrHighStress(t *testing.T) {
	p := DefaultProfile()
	p.Psych.Fear = 0.15

	// Under contact should suppress search.
	bb := &Blackboard{}
	bb.SquadHasContact = true
	bb.SquadStress = 0.2
	bb.VisibleAllyCount = 2
	bb.SearchDrive = 1.0
	g := SelectGoal(bb, &p, false, true)
	if g == GoalSearch {
		t.Fatal("did not expect GoalSearch under contact")
	}

	// High squad stress should suppress search.
	bb2 := &Blackboard{}
	bb2.SquadHasContact = false
	bb2.SquadStress = 0.9
	bb2.VisibleAllyCount = 2
	bb2.SearchDrive = 1.0
	g2 := SelectGoal(bb2, &p, false, true)
	if g2 == GoalSearch {
		t.Fatal("did not expect GoalSearch under high squad stress")
	}
}
