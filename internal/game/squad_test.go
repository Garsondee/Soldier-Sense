package game

import (
	"math"
	"testing"
)

func makeSquadSim(count int) (*TestSim, *Squad) {
	opts := []SimOption{
		WithMapSize(1280, 720),
		WithSeed(42),
	}
	// Red soldiers advancing east.
	for i := 0; i < count; i++ {
		opts = append(opts, WithRedSoldier(i, 50, float64(100+i*60), 1200, float64(100+i*60)))
	}
	ids := make([]int, count)
	for i := range ids {
		ids[i] = i
	}
	opts = append(opts, WithRedSquad(ids...))
	ts := NewTestSim(opts...)
	if len(ts.Squads) == 0 {
		return ts, nil
	}
	return ts, ts.Squads[0]
}

func TestSquadSpread_AllTogether(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})
	spread := sq.squadSpread()
	if spread != 0 {
		t.Fatalf("soldiers at same position should have spread=0, got %.2f", spread)
	}
}

func TestSquadSpread_Separated(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 200, 100, TeamRed, [2]float64{200, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})
	spread := sq.squadSpread()
	if math.Abs(spread-100) > 1e-6 {
		t.Fatalf("expected spread=100, got %.2f", spread)
	}
}

func TestSquadSpread_DeadMembersIgnored(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 500, 500, TeamRed, [2]float64{500, 500}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1.state = SoldierStateDead
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})
	spread := sq.squadSpread()
	if spread != 0 {
		t.Fatalf("dead member should not contribute to spread; got %.2f", spread)
	}
}

func TestLeaderCohesionSlowdown_Thresholds(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})

	// Spread = 0 → full speed
	m1.x, m1.y = 100, 100
	if sq.LeaderCohesionSlowdown() != 1.0 {
		t.Fatal("spread=0 should give full speed")
	}
	// Spread > 420 → stop
	m1.x, m1.y = 100+430, 100
	if sq.LeaderCohesionSlowdown() != 0.0 {
		t.Fatal("spread>420 should stop leader")
	}
	// Spread in (340,420] → crawl
	m1.x, m1.y = 100+380, 100
	v := sq.LeaderCohesionSlowdown()
	if v != 0.3 {
		t.Fatalf("spread=380 should give 0.3 slowdown, got %.2f", v)
	}
	// Spread in (280,340] → slow
	m1.x, m1.y = 100+310, 100
	v = sq.LeaderCohesionSlowdown()
	if v != 0.6 {
		t.Fatalf("spread=310 should give 0.6 slowdown, got %.2f", v)
	}
}

func TestSquadThink_AdvanceWhenClear(t *testing.T) {
	ts, sq := makeSquadSim(3)
	if sq == nil {
		t.Fatal("squad not created")
	}
	// No enemies — leader has no threats → intent should be advance.
	ts.RunTicks(1)
	if sq.Intent != IntentAdvance {
		t.Fatalf("no threats: expected IntentAdvance, got %s", sq.Intent)
	}
}

func TestSquadThink_HoldWhenContact(t *testing.T) {
	ng := NewNavGrid(1280, 720, nil, 0, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	leader := NewSoldier(0, 640, 360, TeamRed, [2]float64{50, 360}, [2]float64{1230, 360}, ng, nil, nil, tl, &tick)
	leader.profile.Psych.Fear = 0
	sq := NewSquad(0, TeamRed, []*Soldier{leader})

	// Manually inject a visible threat close to the leader.
	leader.blackboard.Threats = []ThreatFact{
		{X: 700, Y: 360, Confidence: 1.0, IsVisible: true, LastTick: 1},
	}

	sq.SquadThink(nil)
	if sq.Intent != IntentEngage {
		t.Fatalf("visible threat at <320px: expected IntentEngage, got %s", sq.Intent)
	}
}

func TestSquadThink_RegroupWhenSpread(t *testing.T) {
	ng := NewNavGrid(1280, 720, nil, 0, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	leader := NewSoldier(0, 100, 360, TeamRed, [2]float64{50, 360}, [2]float64{1230, 360}, ng, nil, nil, tl, &tick)
	// Member very far away (>120px).
	member := NewSoldier(1, 100+150, 360, TeamRed, [2]float64{50, 360}, [2]float64{1230, 360}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{leader, member})

	sq.SquadThink(nil)
	if sq.Intent != IntentRegroup {
		t.Fatalf("spread>120: expected IntentRegroup, got %s", sq.Intent)
	}
}

func TestNewSquad_LeaderIsFirst(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 200, 100, TeamRed, [2]float64{200, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})

	if sq.Leader != m0 {
		t.Fatal("first member should be designated leader")
	}
	if !m0.isLeader {
		t.Fatal("first member's isLeader flag should be true")
	}
	if m1.isLeader {
		t.Fatal("second member should not be leader")
	}
}

func TestNewSquad_MembersGetFormationSlots(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	NewSquad(0, TeamRed, []*Soldier{m0, m1})

	if m1.formationMember != true {
		t.Fatal("non-leader should be a formation member")
	}
	if m1.slotIndex != 1 {
		t.Fatal("non-leader slot index should be 1")
	}
}

func TestSquad_Alive(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 200, 100, TeamRed, [2]float64{200, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1.state = SoldierStateDead
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})

	alive := sq.Alive()
	if len(alive) != 1 {
		t.Fatalf("expected 1 alive member, got %d", len(alive))
	}
	if alive[0] != m0 {
		t.Fatal("wrong alive member returned")
	}
}

func TestSquad_CasualtyCount(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	m0 := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 200, 100, TeamRed, [2]float64{200, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	m1.state = SoldierStateDead
	sq := NewSquad(0, TeamRed, []*Soldier{m0, m1})

	if sq.CasualtyCount() != 1 {
		t.Fatalf("expected 1 casualty, got %d", sq.CasualtyCount())
	}
}

func TestNewSquad_InitialCohesionIsFull(t *testing.T) {
	ng := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	leader := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng, nil, nil, tl, &tick)
	member := NewSoldier(1, 100, 120, TeamRed, [2]float64{100, 120}, [2]float64{600, 120}, ng, nil, nil, tl, &tick)

	sq := NewSquad(0, TeamRed, []*Soldier{leader, member})
	if math.Abs(sq.Cohesion-1.0) > 1e-9 {
		t.Fatalf("new squad should start at full cohesion, got %.4f", sq.Cohesion)
	}
}

func TestSquadThink_ImmediateOrderObedienceAtHighCohesion(t *testing.T) {
	ng := NewNavGrid(1280, 720, nil, 0, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	leader := NewSoldier(0, 120, 360, TeamRed, [2]float64{120, 360}, [2]float64{1100, 360}, ng, nil, nil, tl, &tick)
	member := NewSoldier(1, 120, 390, TeamRed, [2]float64{120, 390}, [2]float64{1100, 390}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{leader, member})

	sq.Cohesion = 1.0
	sq.SquadThink(nil)

	if !member.blackboard.OfficerOrderActive {
		t.Fatal("expected active officer order")
	}
	if !member.blackboard.OfficerOrderImmediate {
		t.Fatal("expected immediate obedience at full cohesion")
	}
	if math.Abs(member.blackboard.OfficerOrderObedienceChance-1.0) > 1e-6 {
		t.Fatalf("expected obedience chance near 1.0, got %.4f", member.blackboard.OfficerOrderObedienceChance)
	}
	if math.Abs(member.blackboard.OfficerOrderPriority-sq.ActiveOrder.Priority) > 1e-6 {
		t.Fatalf("expected unscaled order priority at high cohesion, got %.4f vs %.4f", member.blackboard.OfficerOrderPriority, sq.ActiveOrder.Priority)
	}
}

func TestSquadThink_ImmediateOrderObedienceDropsAtLowCohesion(t *testing.T) {
	ng := NewNavGrid(1280, 720, nil, 0, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	leader := NewSoldier(0, 120, 360, TeamRed, [2]float64{120, 360}, [2]float64{1100, 360}, ng, nil, nil, tl, &tick)
	member := NewSoldier(1, 120, 390, TeamRed, [2]float64{120, 390}, [2]float64{1100, 390}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{leader, member})

	sq.Cohesion = 0.0
	sq.SquadThink(nil)

	if !member.blackboard.OfficerOrderActive {
		t.Fatal("expected active officer order")
	}
	if member.blackboard.OfficerOrderImmediate {
		t.Fatal("expected low cohesion to suppress immediate obedience")
	}
	if member.blackboard.OfficerOrderObedienceChance > 0.01 {
		t.Fatalf("expected obedience chance to remain very low, got %.4f", member.blackboard.OfficerOrderObedienceChance)
	}
	expectedPriority := sq.ActiveOrder.Priority * 0.25
	if math.Abs(member.blackboard.OfficerOrderPriority-expectedPriority) > 1e-6 {
		t.Fatalf("expected scaled order priority %.4f, got %.4f", expectedPriority, member.blackboard.OfficerOrderPriority)
	}
}

func TestSquadThink_MoveOrdersAssignedToFollowersNotLeader(t *testing.T) {
	ng := NewNavGrid(1280, 720, nil, 0, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	leader := NewSoldier(0, 100, 340, TeamRed, [2]float64{100, 340}, [2]float64{1100, 340}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 90, 310, TeamRed, [2]float64{90, 310}, [2]float64{1100, 310}, ng, nil, nil, tl, &tick)
	m2 := NewSoldier(2, 90, 370, TeamRed, [2]float64{90, 370}, [2]float64{1100, 370}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{leader, m1, m2})

	sq.SquadThink(nil)

	if leader.blackboard.HasMoveOrder {
		t.Fatal("leader should not receive follower move-order slot")
	}
	if !m1.blackboard.HasMoveOrder && !m2.blackboard.HasMoveOrder {
		t.Fatal("expected follower move orders to be assigned")
	}
}

func TestSquadThink_AbandonsClaimedBuildingAfterNoContactOccupancy(t *testing.T) {
	footprint := rect{x: 220, y: 180, w: 96, h: 96}
	ng := NewNavGrid(1280, 720, nil, 0, nil, nil)
	tl := NewThoughtLog()
	tick := 0

	leader := NewSoldier(0, 230, 220, TeamRed, [2]float64{230, 220}, [2]float64{1100, 220}, ng, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 240, 240, TeamRed, [2]float64{240, 240}, [2]float64{1100, 240}, ng, nil, nil, tl, &tick)
	sq := NewSquad(0, TeamRed, []*Soldier{leader, m1})
	sq.buildingFootprints = []rect{footprint}
	sq.ClaimedBuildingIdx = 0

	for i := 0; i < 460; i++ {
		tick++
		leader.blackboard.AtInterior = true
		m1.blackboard.AtInterior = true
		leader.blackboard.SquadHasContact = false
		m1.blackboard.SquadHasContact = false
		leader.blackboard.HeardGunfire = false
		m1.blackboard.HeardGunfire = false
		sq.SquadThink(nil)
	}

	if sq.ClaimedBuildingIdx != -1 {
		t.Fatalf("expected claimed building to be abandoned after no-contact occupancy, got %d", sq.ClaimedBuildingIdx)
	}
}

func TestSquad_LeaderAdvances_WithFormationFollowers(t *testing.T) {
	// Regression test: leader should not get stuck due to cohesion slowdown
	// thresholds when the squad is in a normal wedge formation.
	ts := NewTestSim(
		WithMapSize(1280, 720),
		WithSeed(42),
		// Tight cluster (same pattern as scenario tests).
		WithRedSoldier(0, 50, 350, 1200, 350),
		WithRedSoldier(1, 50, 322, 1200, 322),
		WithRedSoldier(2, 50, 378, 1200, 378),
		WithRedSoldier(3, 50, 294, 1200, 294),
		WithRedSoldier(4, 50, 406, 1200, 406),
		WithRedSoldier(5, 50, 266, 1200, 266),
		WithRedSquad(0, 1, 2, 3, 4, 5),
	)
	if len(ts.Squads) == 0 || ts.Squads[0].Leader == nil {
		t.Fatal("expected red squad with a leader")
	}
	leader := ts.Squads[0].Leader
	startX := leader.x
	ts.RunTicks(200)
	endX := leader.x
	if endX-startX < 40 {
		t.Fatalf("expected leader to advance; startX=%.1f endX=%.1f", startX, endX)
	}
}

func TestAdjustFormationTarget_SnapsFromBlockedCell(t *testing.T) {
	// Building blocks the desired slot cell.
	buildings := []rect{{x: 160, y: 160, w: 64, h: 64}}
	ng := NewNavGrid(640, 480, buildings, soldierRadius, nil, nil)

	// Dummy leader + members.
	ng2 := NewNavGrid(800, 600, nil, 6, nil, nil)
	tl := NewThoughtLog()
	tick := 0
	leader := NewSoldier(0, 100, 100, TeamRed, [2]float64{100, 100}, [2]float64{600, 100}, ng2, nil, nil, tl, &tick)
	m1 := NewSoldier(1, 120, 100, TeamRed, [2]float64{120, 100}, [2]float64{600, 100}, ng2, nil, nil, tl, &tick)
	members := []*Soldier{leader, m1}
	assigned := map[int][2]float64{}

	// Desired is inside the building.
	desiredX, desiredY := 180.0, 180.0
	cx, cy := WorldToCell(desiredX, desiredY)
	if !ng.IsBlocked(cx, cy) {
		t.Fatal("precondition failed: desired cell should be blocked")
	}

	adjX, adjY := adjustFormationTarget(ng, desiredX, desiredY, leader, members, assigned)
	acx, acy := WorldToCell(adjX, adjY)
	if ng.IsBlocked(acx, acy) {
		t.Fatalf("adjusted target should be unblocked, got blocked cell (%d,%d)", acx, acy)
	}
}
